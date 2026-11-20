// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	auth "github.com/dsx-ai-factory/run-ai/control-plane/authorization/pkg/client"
	clusterapi "github.com/dsx-ai-factory/run-ai/control-plane/cluster-service/pkg/client/generated"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/generated/pkg/api"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/database/dao"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/database/models"
	"k8s.io/utils/ptr"
)

// Configuration is not timestamped in the org-unit views. While this scoped PoC
// is enrolled, return full sync pages so a configuration-only activation is visible.
func QuotaSyncUpdatedAfter(tenantId, clusterId string, updatedAfter time.Time) (time.Time, error) {
	entries, err := dao.QuotaEnrollments()
	if err != nil {
		return updatedAfter, err
	}
	for _, e := range entries {
		if e.TenantId == tenantId && e.ClusterId == clusterId {
			return time.Time{}, nil
		}
	}
	return updatedAfter, nil
}

func ValidateQuotaOwnershipCapabilities(ctx context.Context, ownership *api.QuotaOwnership) error {
	if ownership == nil {
		return nil
	}
	enabled, err := IsLimitQuotaOverSubscriptionEnabled(ctx)
	if err != nil {
		return err
	}
	if enabled {
		return fmt.Errorf("Kubernetes quota ownership requires limitQuotaOverSubscription to be disabled")
	}
	return nil
}

func QuotaOwnership(ctx context.Context, tenantId, clusterId, kind string, id int64) (*api.QuotaOwnership, error) {
	e, err := dao.FindQuotaEnrollment(tenantId, clusterId, kind, id)
	if err != nil || e == nil || e.State != "active" {
		return nil, err
	}
	enabled, err := IsLimitQuotaOverSubscriptionEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if enabled {
		return nil, fmt.Errorf("Kubernetes quota ownership requires limitQuotaOverSubscription to be disabled")
	}
	c, err := dao.GetQuotaControl(ctx, tenantId, clusterId, kind, id)
	if err != nil || c == nil {
		return nil, err
	}
	return c.Ownership(), nil
}

// QuotaNodePoolsMetadata prevents API capability flags from hiding or deriving
// Kubernetes-owned numeric leaves. It never changes the cluster's configuration.
func QuotaNodePoolsMetadata(tenantId, clusterId, kind string, id int64, metadata map[string]clusterapi.NodepoolMinimal) map[string]clusterapi.NodepoolMinimal {
	e, err := dao.FindQuotaEnrollment(tenantId, clusterId, kind, id)
	if err != nil || e == nil || e.State != "active" {
		return metadata
	}
	result := make(map[string]clusterapi.NodepoolMinimal, len(metadata))
	for id, pool := range metadata {
		result[id] = pool
	}
	pools := e.Pools
	c, err := dao.GetQuotaControl(context.Background(), tenantId, clusterId, kind, id)
	if err != nil {
		return metadata
	}
	if c != nil && c.Revision > 0 {
		pools = nil
		for id, pool := range metadata {
			pools = append(pools, api.QuotaOwnershipPool{NodePoolId: id, NodePoolName: pool.Name})
		}
	}
	for _, pool := range pools {
		entry, ok := result[pool.NodePoolId]
		if !ok {
			continue
		}
		caps, err := GetSchedulingCapabilitiesPerNodePoolId(metadata, pool.NodePoolId)
		if err != nil {
			continue
		}
		copy := *caps
		copy.Quota, copy.CpuQuota, copy.AdjustableWeights = true, true, true
		entry.SchedulingCapabilities = &copy
		result[pool.NodePoolId] = entry
	}
	return result
}

// ValidateQuotaSnapshotNumbers is also the numeric contract for admission. No
// omitted values, NaN/Inf, fractional negative sentinels or implicit defaults.
func ValidateQuotaSnapshotNumbers(snapshot api.QuotaSnapshot) error {
	if snapshot.Epoch == "" || snapshot.KubernetesUid == "" || snapshot.Generation < 1 || len(snapshot.Pools) == 0 {
		return fmt.Errorf("incomplete quota snapshot identity or generation")
	}
	seen := map[string]bool{}
	for _, p := range snapshot.Pools {
		if p.NodePoolId == "" || seen[p.NodePoolId] {
			return fmt.Errorf("missing or duplicate snapshot pool %q", p.NodePoolId)
		}
		seen[p.NodePoolId] = true
		for name, r := range map[string]api.QuotaSnapshotResource{"gpu": p.Gpu, "cpu": p.Cpu, "memory": p.Memory} {
			if r.Deserved == nil || r.Limit == nil || r.OverQuotaWeight == nil {
				return fmt.Errorf("pool %s %s requires all numeric leaves", p.NodePoolId, name)
			}
			for _, v := range []*float64{r.Deserved, r.Limit, r.OverQuotaWeight} {
				if math.IsNaN(*v) || math.IsInf(*v, 0) {
					return fmt.Errorf("pool %s %s must be finite", p.NodePoolId, name)
				}
			}
			if (*r.Deserved < 0 && *r.Deserved != -1) || (*r.Limit < 0 && *r.Limit != -1) || *r.OverQuotaWeight < 0 {
				return fmt.Errorf("pool %s %s has invalid negative value", p.NodePoolId, name)
			}
			if *r.Limit != -1 && (*r.Deserved == -1 || *r.Deserved > *r.Limit) {
				return fmt.Errorf("pool %s %s deserved exceeds limit", p.NodePoolId, name)
			}
		}
	}
	return nil
}

func ImportQuotaSnapshot(ctx context.Context, tenantId, clusterId, kind string, id int64, snapshot api.QuotaSnapshot, current []models.Resources, nodePools map[string]clusterapi.NodepoolMinimal) error {
	scopes, err := auth.ParsePermittedScopes(ctx)
	if err != nil || scopes == nil || !slices.Contains(scopes.Clusters, clusterId) {
		return fmt.Errorf("quota snapshot requires an authorized cluster scope matching %s", clusterId)
	}
	ctxTenant, err := ParseTenantId(ctx)
	if err != nil || *ctxTenant != tenantId {
		return fmt.Errorf("quota snapshot tenant does not match authenticated tenant")
	}
	e, err := dao.FindQuotaEnrollment(tenantId, clusterId, kind, id)
	if err != nil {
		return err
	}
	if e == nil || e.State != "active" || e.Epoch != snapshot.Epoch || e.KubernetesUid != snapshot.KubernetesUid {
		return dao.ErrQuotaSnapshot
	}
	if _, err := QuotaOwnership(ctx, tenantId, clusterId, kind, id); err != nil {
		return err
	}
	if err := ValidateQuotaSnapshotNumbers(snapshot); err != nil {
		return err
	}
	c, err := dao.GetQuotaControl(ctx, tenantId, clusterId, kind, id)
	if err != nil {
		return err
	}
	if c != nil && c.Revision > 0 && c.Source == "runai" && c.TargetSource == "runai" && c.Phase == "stable" && c.ObservedRevision == c.Revision {
		return dao.ImportQuotaSnapshot(ctx, tenantId, clusterId, kind, id, snapshot, nil)
	}
	if len(snapshot.Pools) != len(e.Pools) {
		return fmt.Errorf("snapshot must contain exactly the enrolled pool set")
	}
	converted := make([]models.Resources, 0, len(e.Pools))
	for _, pool := range e.Pools {
		var values *api.QuotaSnapshotPool
		var original *models.Resources
		for i := range snapshot.Pools {
			if snapshot.Pools[i].NodePoolId == pool.NodePoolId {
				values = &snapshot.Pools[i]
			}
		}
		for i := range current {
			if current[i].NodePoolId == pool.NodePoolId {
				original = &current[i]
			}
		}
		metadata, found := nodePools[pool.NodePoolId]
		if values == nil || original == nil || original.NodePoolName != pool.NodePoolName || !found || metadata.Name != pool.NodePoolName {
			return fmt.Errorf("snapshot pool %s does not match the enrolled existing pool", pool.NodePoolId)
		}
		requested := api.ResourcesNullable{
			NodePool: &api.ResourcesNodePool{Id: pool.NodePoolId, Name: pool.NodePoolName},
			Gpu:      api.ResourcesNullableGpu{Deserved: values.Gpu.Deserved, Limit: values.Gpu.Limit, OverQuotaWeight: values.Gpu.OverQuotaWeight},
			Cpu:      &api.ResourcesCpu{Deserved: values.Cpu.Deserved, Limit: values.Cpu.Limit, OverQuotaWeight: values.Cpu.OverQuotaWeight},
			Memory:   &api.ResourcesMemory{Deserved: values.Memory.Deserved, Limit: values.Memory.Limit, OverQuotaWeight: values.Memory.OverQuotaWeight, Units: api.UNITS_MB},
		}
		managedPools := QuotaNodePoolsMetadata(tenantId, clusterId, kind, id, nodePools)
		if err := ValidateResources(ctx, []api.ResourcesNullable{requested}, managedPools); err != nil {
			return err
		}
		r := *original
		r.GpuDeserved, r.GpuLimit, r.GpuOverQuotaWeight = values.Gpu.Deserved, values.Gpu.Limit, values.Gpu.OverQuotaWeight
		r.CpuDeserved, r.CpuLimit, r.CpuOverQuotaWeight = values.Cpu.Deserved, values.Cpu.Limit, values.Cpu.OverQuotaWeight
		convert := func(v *float64) *float64 {
			if *v == -1 {
				return ptr.To(*v)
			}
			switch original.MemoryUnits {
			case models.MB:
				return ptr.To(*v)
			case models.GB:
				return ptr.To(*v / 1000)
			default:
				return ptr.To(MbToMib(*v))
			}
		}
		r.MemoryDeserved, r.MemoryLimit, r.MemoryOverQuotaWeight = convert(values.Memory.Deserved), convert(values.Memory.Limit), values.Memory.OverQuotaWeight
		converted = append(converted, r)
	}
	if err := dao.ImportQuotaSnapshot(ctx, tenantId, clusterId, kind, id, snapshot, converted); err != nil {
		return fmt.Errorf("import %s %d UID %s epoch %s generation %d: %w", kind, id, snapshot.KubernetesUid, snapshot.Epoch, snapshot.Generation, err)
	}
	return nil
}
