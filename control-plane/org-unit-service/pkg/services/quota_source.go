// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"net/http"
	"slices"
	"strconv"

	audit "github.com/dsx-ai-factory/run-ai/control-plane/audit-service/pkg/client"
	auth "github.com/dsx-ai-factory/run-ai/control-plane/authorization/pkg/client"
	clusterclient "github.com/dsx-ai-factory/run-ai/control-plane/cluster-service/pkg/client"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/generated/pkg/api"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/database/dao"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/database/models"
	"k8s.io/utils/ptr"
)

// The generated route applies existing object read/update authorization and audit.
func QuotaSource(ctx context.Context, kind, id string, request *api.QuotaSourceUpdateRequest, client clusterclient.IClusterExternalClient) (api.ImplResponse, error) {
	tenant, err := ParseTenantId(ctx)
	if err != nil {
		return HandleError(err, http.StatusBadRequest, "Failed to parse tenant id"), nil
	}
	numericID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return HandleError(err, http.StatusBadRequest, "Invalid organization id"), nil
	}
	var cluster, name string
	if kind == "project" {
		p, e := dao.GetProjectById(ctx, numericID, *tenant, models.SlimVerbosity)
		err = e
		if p != nil {
			cluster = p.ClusterId.String()
			name = p.Name
		}
	} else {
		d, e := dao.GetDepartmentById(ctx, numericID, *tenant, models.SlimVerbosity, nil)
		err = e
		if d != nil {
			cluster = d.ClusterId.String()
			name = d.Name
		}
	}
	if err != nil {
		return HandleDbError(ctx, err, "Failed to get organization"), nil
	}
	if request == nil {
		_, m, e := dao.QuotaPublicSnapshot(ctx, *tenant, cluster, kind, numericID)
		if e != nil {
			return HandleDbError(ctx, e, "Failed to read quota source"), nil
		}
		return api.Response(http.StatusOK, m), nil
	}
	enabled, err := IsLimitQuotaOverSubscriptionEnabled(ctx)
	if err != nil {
		return HandleError(err, http.StatusServiceUnavailable, "Failed to read quota capabilities"), nil
	}
	if enabled {
		return api.Response(http.StatusBadRequest, api.NewError(http.StatusBadRequest, "Quota source switching requires limitQuotaOverSubscription to be disabled", nil)), nil
	}
	metadata, err := GetNodePoolsMetadataMap(ctx, client, cluster)
	if err != nil {
		return HandleError(err, http.StatusServiceUnavailable, "Failed to read node pool capabilities"), nil
	}
	m, err := dao.UpdateQuotaSource(ctx, *tenant, cluster, kind, numericID, *request, func(r models.Resources) (models.Resources, error) {
		caps, err := GetSchedulingCapabilitiesPerNodePoolId(metadata, r.NodePoolId)
		if err != nil {
			return r, err
		}
		// Keep stored memory units, but derive weights in scheduler units exactly
		// as the stock sync mapper does. Omitted CPU/memory retain KAI defaults.
		effective := ApiResourceFromModel(r, caps.AdjustableWeights, caps.CpuQuota, false)
		sync := ApiResourceFromModel(r, caps.AdjustableWeights, caps.CpuQuota, true)
		r.GpuDeserved, r.GpuLimit, r.GpuOverQuotaWeight = ptr.To(effective.Gpu.Deserved), effective.Gpu.Limit, effective.Gpu.OverQuotaWeight
		r.CpuDeserved, r.CpuLimit, r.CpuOverQuotaWeight = nil, nil, nil
		if effective.Cpu != nil {
			r.CpuDeserved, r.CpuLimit, r.CpuOverQuotaWeight = effective.Cpu.Deserved, effective.Cpu.Limit, effective.Cpu.OverQuotaWeight
		}
		r.MemoryDeserved, r.MemoryLimit, r.MemoryOverQuotaWeight = nil, nil, nil
		if effective.Memory != nil {
			r.MemoryDeserved, r.MemoryLimit, r.MemoryOverQuotaWeight = effective.Memory.Deserved, effective.Memory.Limit, sync.Memory.OverQuotaWeight
		}
		return r, nil
	})
	if err != nil {
		return HandleError(err, http.StatusConflict, err.Error()), nil
	}
	audit.AuditFromCtx(ctx).WithEntityName(name).WithEntityType(kind).WithEntityId(id)
	return api.Response(http.StatusOK, m), nil
}

func QuotaObjectSnapshot(ctx context.Context, tenant, cluster, kind string, id int64, existing []models.Resources) ([]models.Resources, api.QuotaManagement) {
	if id == 0 {
		return existing, QuotaManagementForObject(ctx, tenant, cluster, kind, id)
	}
	resources, m, err := dao.QuotaPublicSnapshot(ctx, tenant, cluster, kind, id)
	if err != nil {
		return existing, api.QuotaManagement{Source: "runai", RequestedSource: "runai", CanEditQuotas: false, SyncStatus: api.QuotaManagementSyncStatus{Phase: "error", Message: "Quota ownership is unavailable"}}
	}
	return MergeQuotaSnapshotResources(existing, resources), *m
}

// Fresh rows fence owned numbers and pool membership, not view-derived status.
// A nil snapshot means the object has never been managed and needs no overlay.
func MergeQuotaSnapshotResources(existing, fresh []models.Resources) []models.Resources {
	if fresh == nil {
		return existing
	}
	byPool := make(map[string]models.Resources, len(existing))
	for _, r := range existing {
		byPool[r.NodePoolId] = r
	}
	merged := make([]models.Resources, 0, len(fresh))
	for _, r := range fresh {
		view, found := byPool[r.NodePoolId]
		if !found {
			view = r
		}
		view.NodePoolName = r.NodePoolName
		view.GpuDeserved, view.GpuLimit, view.GpuOverQuotaWeight = r.GpuDeserved, r.GpuLimit, r.GpuOverQuotaWeight
		view.CpuDeserved, view.CpuLimit, view.CpuOverQuotaWeight = r.CpuDeserved, r.CpuLimit, r.CpuOverQuotaWeight
		view.MemoryDeserved, view.MemoryLimit, view.MemoryOverQuotaWeight = r.MemoryDeserved, r.MemoryLimit, r.MemoryOverQuotaWeight
		view.MemoryUnits = r.MemoryUnits
		merged = append(merged, view)
	}
	return merged
}

func QuotaManagementForObject(ctx context.Context, tenant, cluster, kind string, id int64) api.QuotaManagement {
	m, err := dao.GetQuotaManagement(ctx, tenant, cluster, kind, id)
	if err != nil {
		return api.QuotaManagement{Source: "runai", RequestedSource: "runai", CanEditQuotas: false, SyncStatus: api.QuotaManagementSyncStatus{Phase: "error", Message: "Quota ownership is unavailable"}}
	}
	return *m
}

// Full updates normally synthesize default resources for omitted pools. Do not
// silently reset an ever-managed object's quotas from an empty request.
func ValidateQuotaFullUpdate(ctx context.Context, tenant, cluster, kind string, id int64, resources []api.ResourcesNullable) error {
	if len(resources) != 0 {
		return nil
	}
	c, err := dao.GetQuotaControl(ctx, tenant, cluster, kind, id)
	if err != nil {
		return err
	}
	if c != nil && c.Revision > 0 {
		return dao.ErrQuotaEmptyPools
	}
	return nil
}

func QuotaResourceRevision(resources []api.ResourcesNullable) int64 {
	if len(resources) == 0 {
		return 0
	}
	revision := resources[0].QuotaManagementRevision
	for _, r := range resources {
		if revision != r.QuotaManagementRevision {
			return -1
		}
	}
	return revision
}

func BindQuotaUID(ctx context.Context, tenant, cluster, kind string, id int64, uid string) error {
	if uid == "" {
		return nil
	}
	scopes, err := auth.ParsePermittedScopes(ctx)
	if err != nil || scopes == nil || !slices.Contains(scopes.Clusters, cluster) {
		return dao.ErrQuotaSnapshot
	}
	actual, err := ParseTenantId(ctx)
	if err != nil || *actual != tenant {
		return dao.ErrQuotaSnapshot
	}
	return dao.BindQuotaUID(ctx, tenant, cluster, kind, id, uid)
}
