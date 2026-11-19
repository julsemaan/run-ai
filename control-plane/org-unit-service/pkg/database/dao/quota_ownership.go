// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/generated/pkg/api"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/config"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/database/models"
	"github.com/dsx-ai-factory/run-ai/runai-common-packages/runaidb"
	"github.com/dsx-ai-factory/run-ai/runai-common-packages/runaidb/orm"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

var ErrKubernetesQuota = errors.New("quotas are managed through Kubernetes for the enrolled node pools")
var ErrQuotaSnapshot = errors.New("quota snapshot conflicts with the active enrollment or accepted generation")

// Lock the organization before reading resources. Public writes and imports use the
// same lock, so a stale full-update payload cannot overwrite a committed snapshot.
func lockQuotaOrgUnit(tx orm.DB, tenantId, clusterId, kind string, id int64) (uuid.UUID, error) {
	table := models.ProjectTable
	if kind == "department" {
		table = models.DepartmentTable
	}
	var row struct{ OrgUnitDataID uuid.UUID }
	err := tx.Table(table).Where("tenant_id = ? AND cluster_id = ? AND id = ? AND deleted_at IS NULL", tenantId, clusterId, id).
		Clauses(clause.Locking{Strength: "UPDATE"}).Select("org_unit_data_id").First(&row)
	return row.OrgUnitDataID, err
}

func quotaResources(tx orm.DB, dataId uuid.UUID) ([]models.Resources, error) {
	var resources []models.Resources
	err := tx.Table(models.ResourcesTable).Where("org_unit_data_id = ? AND deleted_at IS NULL", dataId).Find(&resources)
	return resources, err
}

func numericQuotas(r models.Resources) map[string]interface{} {
	return map[string]interface{}{
		"gpu_deserved": r.GpuDeserved, "gpu_limit": r.GpuLimit, "gpu_over_quota_weight": r.GpuOverQuotaWeight,
		"cpu_deserved": r.CpuDeserved, "cpu_limit": r.CpuLimit, "cpu_over_quota_weight": r.CpuOverQuotaWeight,
		"memory_deserved": r.MemoryDeserved, "memory_limit": r.MemoryLimit, "memory_over_quota_weight": r.MemoryOverQuotaWeight,
	}
}

func quotaPtrEqual(a, b *float64, nilValue float64) bool {
	av, bv := nilValue, nilValue
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	return av == bv
}

func quotaMemoryUnits(units models.Units) models.Units {
	if units == "" {
		return models.Mib
	}
	return units
}

func quotaNumbersEqual(a, b models.Resources) bool {
	return quotaPtrEqual(a.GpuDeserved, b.GpuDeserved, -1) && quotaPtrEqual(a.GpuLimit, b.GpuLimit, -1) &&
		quotaPtrEqual(a.GpuOverQuotaWeight, b.GpuOverQuotaWeight, models.DefaultOverQuotaWeight) &&
		quotaPtrEqual(a.CpuDeserved, b.CpuDeserved, -1) && quotaPtrEqual(a.CpuLimit, b.CpuLimit, -1) &&
		quotaPtrEqual(a.CpuOverQuotaWeight, b.CpuOverQuotaWeight, models.DefaultOverQuotaWeight) &&
		quotaPtrEqual(a.MemoryDeserved, b.MemoryDeserved, -1) && quotaPtrEqual(a.MemoryLimit, b.MemoryLimit, -1) &&
		quotaPtrEqual(a.MemoryOverQuotaWeight, b.MemoryOverQuotaWeight, models.DefaultOverQuotaWeight)
}

func guardQuotaUpdate(tx orm.DB, tenantId, clusterId, kind string, id int64, proposed []models.Resources) error {
	e, err := config.FindQuotaEnrollment(tenantId, clusterId, kind, id)
	if err != nil || e == nil {
		return err
	}
	dataId, err := lockQuotaOrgUnit(tx, tenantId, clusterId, kind, id)
	if err != nil {
		return err
	}
	current, err := quotaResources(tx, dataId)
	if err != nil {
		return err
	}
	for _, pool := range e.Pools {
		var old, next *models.Resources
		for i := range current {
			if current[i].NodePoolId == pool.NodePoolId {
				old = &current[i]
			}
		}
		for i := range proposed {
			if proposed[i].NodePoolId == pool.NodePoolId {
				if next != nil {
					return ErrKubernetesQuota
				}
				next = &proposed[i]
			}
		}
		if old == nil || next == nil || old.NodePoolName != pool.NodePoolName || next.NodePoolName != pool.NodePoolName || quotaMemoryUnits(old.MemoryUnits) != quotaMemoryUnits(next.MemoryUnits) || !quotaNumbersEqual(*old, *next) {
			return ErrKubernetesQuota
		}
		// Preserve the stored representation for semantically unchanged nil/unlimited values.
		next.MemoryUnits = old.MemoryUnits
		next.GpuDeserved, next.GpuLimit, next.GpuOverQuotaWeight = old.GpuDeserved, old.GpuLimit, old.GpuOverQuotaWeight
		next.CpuDeserved, next.CpuLimit, next.CpuOverQuotaWeight = old.CpuDeserved, old.CpuLimit, old.CpuOverQuotaWeight
		next.MemoryDeserved, next.MemoryLimit, next.MemoryOverQuotaWeight = old.MemoryDeserved, old.MemoryLimit, old.MemoryOverQuotaWeight
	}
	return nil
}

// Resource columns predate this feature: GPU uses two decimals and CPU/memory
// three. Compare their persisted representation, not extra wire precision, so
// metadata-only generations do not produce timestamp-only resource updates.
func persistedQuotaNumbersEqual(a, b models.Resources) bool {
	left, right := numericQuotas(a), numericQuotas(b)
	for key, value := range left {
		old, next := value.(*float64), right[key].(*float64)
		defaultValue := float64(-1)
		if strings.HasSuffix(key, "over_quota_weight") {
			defaultValue = models.DefaultOverQuotaWeight
		}
		av, bv := defaultValue, defaultValue
		if old != nil {
			av = *old
		}
		if next != nil {
			bv = *next
		}
		scale := float64(1000)
		if strings.HasPrefix(key, "gpu_") {
			scale = 100
		}
		if math.Round(av*scale) != math.Round(bv*scale) {
			return false
		}
	}
	return true
}

// ImportQuotaSnapshot commits only numeric leaves and ordering metadata. The
// converted values use the existing rows' memory units, checked again under lock.
func ImportQuotaSnapshot(ctx context.Context, tenantId, clusterId, kind string, id int64, snapshot api.QuotaSnapshot, converted []models.Resources) error {
	e, err := config.FindQuotaEnrollment(tenantId, clusterId, kind, id)
	if err != nil {
		return err
	}
	if e == nil || e.State != "active" || e.Epoch != snapshot.Epoch || e.KubernetesUid != snapshot.KubernetesUid || snapshot.Generation < 1 {
		return ErrQuotaSnapshot
	}
	pools := append([]api.QuotaSnapshotPool(nil), snapshot.Pools...)
	sort.Slice(pools, func(i, j int) bool { return pools[i].NodePoolId < pools[j].NodePoolId })
	snapshot.Pools = pools
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	ownership, err := json.Marshal(e.Ownership())
	if err != nil {
		return err
	}
	tx := runaidb.DB().WithContext(ctx).Begin()
	committed := false
	defer orm.FinalizeTransaction(tx, &committed)
	dataId, err := lockQuotaOrgUnit(tx, tenantId, clusterId, kind, id)
	if err != nil {
		return err
	}
	var accepted []struct {
		Generation int64
		Snapshot   json.RawMessage
		Ownership  json.RawMessage
	}
	key := "tenant_id = ? AND cluster_id = ? AND kind = ? AND org_unit_id = ? AND epoch = ?"
	args := []interface{}{tenantId, clusterId, kind, id, e.Epoch}
	if err := tx.Table("quota_mirror").Where(key, args...).Find(&accepted); err != nil {
		return err
	}
	if len(accepted) > 0 {
		var oldSnapshot, newSnapshot, oldOwnership, newOwnership any
		if err := json.Unmarshal(accepted[0].Snapshot, &oldSnapshot); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &newSnapshot); err != nil {
			return err
		}
		if err := json.Unmarshal(accepted[0].Ownership, &oldOwnership); err != nil {
			return err
		}
		if err := json.Unmarshal(ownership, &newOwnership); err != nil {
			return err
		}
		if !reflect.DeepEqual(oldOwnership, newOwnership) || snapshot.Generation < accepted[0].Generation {
			return ErrQuotaSnapshot
		}
		if snapshot.Generation == accepted[0].Generation {
			if !reflect.DeepEqual(oldSnapshot, newSnapshot) {
				return ErrQuotaSnapshot
			}
			return nil
		}
	}
	current, err := quotaResources(tx, dataId)
	if err != nil {
		return err
	}
	if len(converted) != len(e.Pools) || len(snapshot.Pools) != len(e.Pools) {
		return ErrQuotaSnapshot
	}
	for _, pool := range e.Pools {
		var old, next *models.Resources
		for i := range current {
			if current[i].NodePoolId == pool.NodePoolId {
				old = &current[i]
			}
		}
		for i := range converted {
			if converted[i].NodePoolId == pool.NodePoolId {
				if next != nil {
					return ErrQuotaSnapshot
				}
				next = &converted[i]
			}
		}
		if old == nil || next == nil || old.NodePoolName != pool.NodePoolName || next.NodePoolName != pool.NodePoolName || old.MemoryUnits != next.MemoryUnits {
			return ErrQuotaSnapshot
		}
		if !persistedQuotaNumbersEqual(*old, *next) {
			updates := numericQuotas(*next)
			updates["updated_at"] = time.Now()
			if _, err := tx.Table(models.ResourcesTable).Where("id = ? AND org_unit_data_id = ? AND deleted_at IS NULL", old.ID, dataId).Updates(updates); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(`INSERT INTO quota_mirror (tenant_id,cluster_id,kind,org_unit_id,epoch,kubernetes_uid,generation,ownership,snapshot)
 VALUES (?,?,?,?,?,?,?,?::jsonb,?::jsonb) ON CONFLICT (tenant_id,cluster_id,kind,org_unit_id,epoch)
 DO UPDATE SET generation=EXCLUDED.generation, snapshot=EXCLUDED.snapshot`, append(args, e.KubernetesUid, snapshot.Generation, string(ownership), string(raw))...); err != nil {
		return fmt.Errorf("persist quota snapshot: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	InvalidateAllCache(ctx, tenantId, clusterId)
	return nil
}

func guardQuotaPatch(tx orm.DB, tenantId, clusterId, departmentId, nodePoolId string, updates map[string]interface{}) error {
	id, err := strconv.ParseInt(departmentId, 10, 64)
	if err != nil {
		return err
	}
	e, err := config.FindQuotaEnrollment(tenantId, clusterId, "department", id)
	if err != nil || e == nil {
		return err
	}
	enrolled := false
	for _, pool := range e.Pools {
		if pool.NodePoolId == nodePoolId {
			enrolled = true
		}
	}
	if !enrolled {
		return nil
	}
	dataId, err := lockQuotaOrgUnit(tx, tenantId, clusterId, "department", id)
	if err != nil {
		return err
	}
	var old models.Resources
	if err := tx.Table(models.ResourcesTable).Where("org_unit_data_id = ? AND node_pool_id = ? AND deleted_at IS NULL", dataId, nodePoolId).First(&old); err != nil {
		return err
	}
	for key, stored := range numericQuotas(old) {
		if value, ok := updates[key]; ok {
			v, ok := value.(*float64)
			if !ok {
				return ErrKubernetesQuota
			}
			nilValue := float64(-1)
			if strings.HasSuffix(key, "over_quota_weight") {
				nilValue = models.DefaultOverQuotaWeight
			}
			if !quotaPtrEqual(stored.(*float64), v, nilValue) {
				return ErrKubernetesQuota
			}
			delete(updates, key)
		}
	}
	if value, ok := updates["memory_units"]; ok {
		units, ok := value.(models.Units)
		if !ok || quotaMemoryUnits(units) != quotaMemoryUnits(old.MemoryUnits) {
			return ErrKubernetesQuota
		}
		delete(updates, "memory_units")
	}
	return nil
}

// Match bulk reset's project-before-department order, independent of config or
// request order. Tenant and cluster come first because CPU disable spans clusters.
func orderedQuotaEnrollments() ([]config.QuotaEnrollment, error) {
	entries, err := config.QuotaEnrollments()
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.TenantId != b.TenantId {
			return a.TenantId < b.TenantId
		}
		if a.ClusterId != b.ClusterId {
			return a.ClusterId < b.ClusterId
		}
		if a.Kind != b.Kind {
			return a.Kind == "project"
		}
		return a.Id < b.Id
	})
	return entries, err
}

func guardQuotaMembership(tx orm.DB, resources []models.Resources) error {
	entries, err := orderedQuotaEnrollments()
	if err != nil || len(entries) == 0 || len(resources) == 0 {
		return err
	}
	var dataIds, resourceIds []uuid.UUID
	for _, r := range resources {
		if r.OrgUnitDataID != uuid.Nil {
			dataIds = append(dataIds, r.OrgUnitDataID)
		}
		if r.ID != uuid.Nil {
			resourceIds = append(resourceIds, r.ID)
		}
	}
	// Delete callers may supply only resource IDs. Resolve those as well as
	// proposed owners before choosing locks; re-read quotas under the org lock.
	var stored []models.Resources
	if len(resourceIds) > 0 {
		if err := tx.Table(models.ResourcesTable).Where("id IN ? AND deleted_at IS NULL", resourceIds).Find(&stored); err != nil {
			return err
		}
		for _, r := range stored {
			dataIds = append(dataIds, r.OrgUnitDataID)
		}
	}
	for _, e := range entries {
		table := models.ProjectTable
		if e.Kind == "department" {
			table = models.DepartmentTable
		}
		var count int64
		if err := tx.Table(table).Where("tenant_id = ? AND cluster_id = ? AND id = ? AND org_unit_data_id IN ? AND deleted_at IS NULL", e.TenantId, e.ClusterId, e.Id, dataIds).Count(&count); err != nil {
			return err
		}
		if count == 0 {
			continue
		}
		dataId, err := lockQuotaOrgUnit(tx, e.TenantId, e.ClusterId, e.Kind, e.Id)
		if errors.Is(err, orm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		current, err := quotaResources(tx, dataId)
		if err != nil {
			return err
		}
		for _, r := range resources {
			for _, p := range e.Pools {
				if r.OrgUnitDataID == dataId && r.NodePoolId == p.NodePoolId {
					return ErrKubernetesQuota
				}
				for _, old := range current {
					if old.NodePoolId == p.NodePoolId && r.ID == old.ID {
						return ErrKubernetesQuota
					}
				}
			}
		}
	}
	return nil
}

func guardQuotaDisableCpu(tx orm.DB, tenantId, clusterId string) error {
	entries, err := config.QuotaEnrollments()
	if err != nil {
		return err
	}
	var ids []string
	for _, e := range entries {
		if e.TenantId == tenantId && e.ClusterId == clusterId {
			for _, p := range e.Pools {
				ids = append(ids, p.NodePoolId)
			}
		}
	}
	for _, projects := range []bool{true, false} {
		if err := guardQuotaReset(tx, tenantId, clusterId, "", ids, CpuQuotasToDefault, projects); err != nil {
			return err
		}
	}
	return nil
}

func guardQuotaReset(tx orm.DB, tenantId, clusterId, departmentId string, nodepoolIds []string, action ResetResourcesAction, projects bool) error {
	if action == RanksToDefault {
		return nil
	}
	entries, err := orderedQuotaEnrollments()
	if err != nil {
		return err
	}
	kind, table := "department", models.DepartmentTable
	if projects {
		kind, table = "project", models.ProjectTable
	}
	for _, e := range entries {
		if e.TenantId != tenantId || e.ClusterId != clusterId || e.Kind != kind {
			continue
		}
		dataId, err := lockQuotaOrgUnit(tx, tenantId, clusterId, kind, e.Id)
		if errors.Is(err, orm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if projects && departmentId != "" {
			var count int64
			if err := tx.Table(table).Where("id = ? AND tenant_id = ? AND parent_id = ?", e.Id, tenantId, departmentId).Count(&count); err != nil {
				return err
			}
			if count == 0 {
				continue
			}
		}
		current, err := quotaResources(tx, dataId)
		if err != nil {
			return err
		}
		for _, pool := range e.Pools {
			selected := false
			for _, id := range nodepoolIds {
				if id == pool.NodePoolId {
					selected = true
				}
			}
			if !selected {
				continue
			}
			for _, r := range current {
				if r.NodePoolId == pool.NodePoolId {
					proposed := r
					zero, weight := float64(0), models.DefaultOverQuotaWeight
					switch action {
					case QuotasToDefault:
						proposed.GpuDeserved, proposed.GpuLimit = &zero, nil
						proposed.CpuDeserved, proposed.CpuLimit = nil, nil
						proposed.MemoryDeserved, proposed.MemoryLimit = nil, nil
					case WeightsToDefault:
						proposed.GpuOverQuotaWeight = &weight
						if r.CpuOverQuotaWeight != nil {
							proposed.CpuOverQuotaWeight = &weight
						}
						if r.MemoryOverQuotaWeight != nil {
							proposed.MemoryOverQuotaWeight = &weight
						}
					case WeightsToDeserved:
						proposed.GpuOverQuotaWeight = r.GpuDeserved
						proposed.CpuOverQuotaWeight = r.CpuDeserved
						proposed.MemoryOverQuotaWeight = r.MemoryDeserved
					case CpuQuotasToDefault:
						proposed.CpuDeserved, proposed.CpuLimit, proposed.CpuOverQuotaWeight = nil, nil, nil
						proposed.MemoryDeserved, proposed.MemoryLimit, proposed.MemoryOverQuotaWeight = nil, nil, nil
					default:
						return fmt.Errorf("invalid reset action %s", action)
					}
					if !quotaNumbersEqual(r, proposed) {
						return ErrKubernetesQuota
					}
				}
			}
		}
	}
	return nil
}
