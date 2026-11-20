// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/generated/pkg/api"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/config"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/database/models"
	"github.com/dsx-ai-factory/run-ai/runai-common-packages/runaidb"
	"github.com/dsx-ai-factory/run-ai/runai-common-packages/runaidb/orm"
	"github.com/google/uuid"
)

var ErrQuotaRevision = errors.New("quota source revision conflicts with current ownership; reload and retry")

// QuotaControl survives return to Runai, fencing requests issued before a transfer.
type QuotaControl struct {
	TenantID         string
	ClusterID        string
	Kind             string
	OrgUnitID        int64
	KubernetesUID    string
	Source           string
	TargetSource     string
	Phase            string
	Revision         int64
	ObservedRevision int64
	Epoch            string
	Pools            []api.QuotaOwnershipPool `gorm:"serializer:json;type:jsonb"`
	LegacyState      string
	Message          string
	LastSyncedAt     *time.Time
	UpdatedAt        time.Time
}

func (QuotaControl) TableName() string { return "quota_control" }
func (c *QuotaControl) pending() bool {
	return c.Revision > 0 && (c.Phase != "stable" || c.ObservedRevision != c.Revision)
}
func (c *QuotaControl) management() *api.QuotaManagement {
	m := &api.QuotaManagement{Source: c.Source, RequestedSource: c.TargetSource, Revision: c.Revision, CanEditQuotas: c.Source == "runai" && !c.pending(), SyncStatus: api.QuotaManagementSyncStatus{Phase: "synced"}}
	if c.pending() || c.LegacyState == "prepared" {
		m.SyncStatus.Phase = "pending"
		m.CanEditQuotas = false
	}
	if c.Message != "" {
		m.SyncStatus.Phase = "error"
		m.SyncStatus.Message = c.Message
	}
	m.SyncStatus.LastSyncedAt = c.LastSyncedAt
	if c.pending() && c.KubernetesUID == "" && c.Message == "" {
		m.SyncStatus.Message = "Waiting for trusted cluster-sync to report the Kubernetes object UID"
	}
	return m
}
func (c *QuotaControl) ownership() *api.QuotaOwnership {
	if c.KubernetesUID == "" || len(c.Pools) == 0 || (c.Revision == 0 && c.LegacyState != "active") {
		return nil
	}
	o := &api.QuotaOwnership{Epoch: c.Epoch, KubernetesUid: c.KubernetesUID, Pools: c.Pools}
	if c.Revision > 0 {
		o.Source = c.TargetSource
		o.Phase = c.Phase
		o.Revision = c.Revision
	}
	return o
}
func canonicalPools(p []api.QuotaOwnershipPool) []api.QuotaOwnershipPool {
	r := append([]api.QuotaOwnershipPool(nil), p...)
	sort.Slice(r, func(i, j int) bool { return r[i].NodePoolId < r[j].NodePoolId })
	return r
}
func (c *QuotaControl) request(source string, revision int64, pools []api.QuotaOwnershipPool) error {
	if (source != "runai" && source != "kubernetes") || revision != c.Revision {
		return ErrQuotaRevision
	}
	if c.Message != "" {
		return fmt.Errorf("%w: %s", ErrQuotaSnapshot, c.Message)
	}
	if c.pending() {
		if source != c.TargetSource {
			return ErrQuotaRevision
		}
		return nil
	}
	pools = canonicalPools(pools)
	if source == c.Source && (c.LegacyState == "" || (c.LegacyState == "active" && reflect.DeepEqual(canonicalPools(c.Pools), pools))) {
		return nil
	}
	if len(pools) == 0 {
		return fmt.Errorf("quota source requires at least one existing node pool")
	}
	c.TargetSource = source
	c.Phase = "freeze"
	c.Revision++
	c.Epoch = uuid.NewString()
	c.Pools = pools
	c.Message = ""
	return nil
}
func (c *QuotaControl) observe(s api.QuotaSnapshot) error {
	if c.Revision == 0 || s.KubernetesUid != c.KubernetesUID || s.Epoch != c.Epoch || s.ControlRevision != c.Revision || s.ControlPhase != c.Phase || s.Source != c.TargetSource {
		return ErrQuotaSnapshot
	}
	c.ObservedRevision = c.Revision
	switch c.Phase {
	case "freeze":
		c.Phase = "release"
		c.Revision++
	case "release":
		c.Phase = "stable"
		c.Revision++
	case "stable":
		c.Source = c.TargetSource
		c.LegacyState = ""
		now := time.Now()
		c.LastSyncedAt = &now
	default:
		return ErrQuotaSnapshot
	}
	c.Message = ""
	return nil
}

func controlKey(tx orm.DB, tenant, cluster, kind string, id int64) orm.DB {
	return tx.Table("quota_control").Where("tenant_id = ? AND cluster_id = ? AND kind = ? AND org_unit_id = ?", tenant, cluster, kind, id)
}

// Callers that mutate must already hold lockQuotaOrgUnit. Legacy import only adds
// protection; it must not expand pools or change the legacy wire marker.
func quotaControl(tx orm.DB, tenant, cluster, kind string, id int64, importLegacy bool) (*QuotaControl, error) {
	var rows []QuotaControl
	if err := controlKey(tx, tenant, cluster, kind, id).Find(&rows); err != nil {
		return nil, err
	}
	if len(rows) > 0 && (rows[0].Revision > 0 || rows[0].LegacyState != "") {
		return &rows[0], nil
	}
	e, err := config.FindQuotaEnrollment(tenant, cluster, kind, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		if len(rows) > 0 {
			return &rows[0], nil
		}
		return nil, nil
	}
	if len(rows) > 0 && rows[0].KubernetesUID != "" && rows[0].KubernetesUID != e.KubernetesUid {
		return nil, ErrQuotaSnapshot
	}
	c := &QuotaControl{TenantID: tenant, ClusterID: cluster, Kind: kind, OrgUnitID: id, KubernetesUID: e.KubernetesUid, Source: "kubernetes", TargetSource: "kubernetes", Phase: "stable", Epoch: e.Epoch, Pools: canonicalPools(e.Pools), LegacyState: e.State}
	if e.State == "prepared" {
		c.Source = "runai"
	}
	if importLegacy {
		if err := saveQuotaControl(tx, c); err != nil {
			return nil, err
		}
	}
	return c, nil
}
func saveQuotaControl(tx orm.DB, c *QuotaControl) error {
	c.UpdatedAt = time.Now()
	pools, err := json.Marshal(c.Pools)
	if err != nil {
		return err
	}
	// The enclosing organization lock serializes this upsert, including first bind.
	_, err = tx.Exec(`INSERT INTO quota_control (tenant_id,cluster_id,kind,org_unit_id,kubernetes_uid,source,target_source,phase,revision,observed_revision,epoch,pools,legacy_state,message,last_synced_at,updated_at)
 VALUES (?,?,?,?,?,?,?,?,?,?,?,?::jsonb,?,?,?,?) ON CONFLICT (tenant_id,cluster_id,kind,org_unit_id) DO UPDATE SET
 kubernetes_uid=EXCLUDED.kubernetes_uid,source=EXCLUDED.source,target_source=EXCLUDED.target_source,phase=EXCLUDED.phase,revision=EXCLUDED.revision,observed_revision=EXCLUDED.observed_revision,epoch=EXCLUDED.epoch,pools=EXCLUDED.pools,legacy_state=EXCLUDED.legacy_state,message=EXCLUDED.message,last_synced_at=EXCLUDED.last_synced_at,updated_at=EXCLUDED.updated_at`, c.TenantID, c.ClusterID, c.Kind, c.OrgUnitID, c.KubernetesUID, c.Source, c.TargetSource, c.Phase, c.Revision, c.ObservedRevision, c.Epoch, string(pools), c.LegacyState, c.Message, c.LastSyncedAt, c.UpdatedAt)
	return err
}

func GetQuotaControl(ctx context.Context, tenant, cluster, kind string, id int64) (*QuotaControl, error) {
	// Unsaved models have no ownership record.
	if id == 0 {
		return nil, nil
	}
	return quotaControl(runaidb.DB().WithContext(ctx), tenant, cluster, kind, id, false)
}
func (c *QuotaControl) Ownership() *api.QuotaOwnership { return c.ownership() }

func FindQuotaEnrollment(tenant, cluster, kind string, id int64) (*config.QuotaEnrollment, error) {
	c, err := GetQuotaControl(context.Background(), tenant, cluster, kind, id)
	if err != nil || c == nil {
		return nil, err
	}
	if c.Revision == 0 && c.LegacyState == "" {
		return nil, nil
	}
	state := c.LegacyState
	if c.Revision > 0 {
		state = "active"
	}
	return &config.QuotaEnrollment{TenantId: tenant, ClusterId: cluster, Kind: kind, Id: id, KubernetesUid: c.KubernetesUID, Epoch: c.Epoch, State: state, Pools: c.Pools}, nil
}

// Both numbers and marker are read after acquiring the same lock as public
// writes and imports. Never attach a fresh control revision to an earlier view.
func quotaReadSnapshot(ctx context.Context, tenant, cluster, kind string, id int64) ([]models.Resources, *QuotaControl, error) {
	tx := runaidb.DB().WithContext(ctx).Begin()
	committed := false
	defer orm.FinalizeTransaction(tx, &committed)
	data, err := lockQuotaOrgUnit(tx, tenant, cluster, kind, id)
	if err != nil {
		return nil, nil, err
	}
	c, err := quotaControl(tx, tenant, cluster, kind, id, true)
	if err != nil {
		return nil, nil, err
	}
	// Never-managed objects keep the stock view's computed quotas and status.
	var resources []models.Resources
	if c != nil && (c.Revision > 0 || c.LegacyState != "") {
		resources, err = quotaResources(tx, data)
		if err != nil {
			return nil, nil, err
		}
	}
	if _, err := refreshRunaiPools(tx, data, c, resources); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	committed = true
	return resources, c, nil
}

func QuotaCommandSnapshot(ctx context.Context, tenant, cluster, kind string, id int64) ([]models.Resources, *api.QuotaOwnership, error) {
	resources, c, err := quotaReadSnapshot(ctx, tenant, cluster, kind, id)
	if err != nil {
		return nil, nil, err
	}
	var ownership *api.QuotaOwnership
	if c != nil {
		ownership = c.ownership()
	}
	return resources, ownership, nil
}

func QuotaPublicSnapshot(ctx context.Context, tenant, cluster, kind string, id int64) ([]models.Resources, *api.QuotaManagement, error) {
	resources, c, err := quotaReadSnapshot(ctx, tenant, cluster, kind, id)
	if err != nil {
		return nil, nil, err
	}
	if c == nil {
		c = &QuotaControl{Source: "runai", TargetSource: "runai", Phase: "stable"}
	}
	return resources, c.management(), nil
}

func quotaEnrollments(tx orm.DB) ([]config.QuotaEnrollment, error) {
	entries, err := config.QuotaEnrollments()
	if err != nil {
		return nil, err
	}
	var controls []struct {
		QuotaControl  `gorm:"embedded"`
		OrgUnitExists bool
	}
	if err := tx.Table("quota_control").Select(`quota_control.*,
		(EXISTS (SELECT 1 FROM project p WHERE quota_control.kind = 'project'
		 AND p.id = quota_control.org_unit_id AND p.tenant_id = quota_control.tenant_id
		 AND p.cluster_id = quota_control.cluster_id AND p.deleted_at IS NULL)
		 OR EXISTS (SELECT 1 FROM department d WHERE quota_control.kind = 'department'
		 AND d.id = quota_control.org_unit_id AND d.tenant_id = quota_control.tenant_id
		 AND d.cluster_id = quota_control.cluster_id AND d.deleted_at IS NULL)) AS org_unit_exists`).Find(&controls); err != nil {
		return nil, err
	}
	for _, c := range controls {
		if c.Revision == 0 && c.LegacyState == "" {
			continue
		}
		filtered := entries[:0]
		for _, e := range entries {
			if e.TenantId != c.TenantID || e.ClusterId != c.ClusterID || e.Kind != c.Kind || e.Id != c.OrgUnitID {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
		// Deleted controls still override legacy config, but cannot require live sync.
		if c.OrgUnitExists {
			entries = append(entries, config.QuotaEnrollment{TenantId: c.TenantID, ClusterId: c.ClusterID, Kind: c.Kind, Id: c.OrgUnitID, KubernetesUid: c.KubernetesUID, Epoch: c.Epoch, State: "active", Pools: c.Pools})
		}
	}
	return entries, nil
}
func QuotaEnrollments() ([]config.QuotaEnrollment, error) { return quotaEnrollments(runaidb.DB()) }
func GetQuotaManagement(ctx context.Context, tenant, cluster, kind string, id int64) (*api.QuotaManagement, error) {
	c, err := GetQuotaControl(ctx, tenant, cluster, kind, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		c = &QuotaControl{Source: "runai", TargetSource: "runai", Phase: "stable"}
	}
	return c.management(), nil
}
func poolsForResources(resources []models.Resources, name string) []api.QuotaOwnershipPool {
	pools := make([]api.QuotaOwnershipPool, 0, len(resources))
	for _, r := range resources {
		q := name
		if r.NodePoolName != "default" {
			q += "-" + r.NodePoolName
		}
		pools = append(pools, api.QuotaOwnershipPool{NodePoolId: r.NodePoolId, NodePoolName: r.NodePoolName, QueueName: q})
	}
	return canonicalPools(pools)
}

// Read the canonical queue base from the locked object, not its public display name.
func quotaPoolBindings(tx orm.DB, data uuid.UUID, kind string, id int64, resources []models.Resources) ([]api.QuotaOwnershipPool, error) {
	name := fmt.Sprintf("q-%d", id)
	if kind == "project" {
		var project struct{ Name string }
		if err := tx.Table(models.ProjectTable).Where("org_unit_data_id = ? AND deleted_at IS NULL", data).Select("name").First(&project); err != nil {
			return nil, err
		}
		name = project.Name
	}
	return poolsForResources(resources, name), nil
}

// Membership CRUD must finish before publishing a new fence, otherwise its own
// subsequent guards would reject the transaction. Refresh only at read/command boundaries.
func refreshRunaiPools(tx orm.DB, data uuid.UUID, c *QuotaControl, resources []models.Resources) (bool, error) {
	if c == nil || c.Revision == 0 || c.Source != "runai" || c.TargetSource != "runai" || c.pending() {
		return false, nil
	}
	pools, err := quotaPoolBindings(tx, data, c.Kind, c.OrgUnitID, resources)
	if err != nil {
		return false, err
	}
	if reflect.DeepEqual(canonicalPools(c.Pools), pools) {
		return false, nil
	}
	c.Pools = pools
	c.Revision++
	return true, saveQuotaControl(tx, c)
}

func UpdateQuotaSource(ctx context.Context, tenant, cluster, kind string, id int64, request api.QuotaSourceUpdateRequest, projectEffective func(models.Resources) (models.Resources, error)) (*api.QuotaManagement, error) {
	tx := runaidb.DB().WithContext(ctx).Begin()
	committed := false
	defer orm.FinalizeTransaction(tx, &committed)
	data, err := lockQuotaOrgUnit(tx, tenant, cluster, kind, id)
	if err != nil {
		return nil, err
	}
	c, err := quotaControl(tx, tenant, cluster, kind, id, true)
	if err != nil {
		return nil, err
	}
	if c == nil {
		c = &QuotaControl{TenantID: tenant, ClusterID: cluster, Kind: kind, OrgUnitID: id, Source: "runai", TargetSource: "runai", Phase: "stable"}
	}
	resources, err := quotaResources(tx, data)
	if err != nil {
		return nil, err
	}
	refreshed, err := refreshRunaiPools(tx, data, c, resources)
	if err != nil {
		return nil, err
	}
	if refreshed {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		committed = true
		InvalidateAllCache(ctx, tenant, cluster)
		return nil, fmt.Errorf("%w: wait for Runai pool membership reconciliation", ErrQuotaRevision)
	}
	pools, err := quotaPoolBindings(tx, data, kind, id, resources)
	if err != nil {
		return nil, err
	}
	firstNative := c.Revision == 0
	legacyPools := c.Pools
	legacyActive := c.LegacyState == "active"
	if err := c.request(request.Source, request.ExpectedRevision, pools); err != nil {
		return nil, err
	}
	if firstNative && c.Revision > 0 {
		for _, r := range resources {
			owned := false
			for _, pool := range legacyPools {
				if legacyActive && pool.NodePoolId == r.NodePoolId {
					owned = true
				}
			}
			if owned {
				continue
			}
			// Writers share this lock. Project their latest committed rows before
			// publishing the native fence that stops capability-based projection.
			effective, err := projectEffective(r)
			if err != nil {
				return nil, err
			}
			if _, err := tx.Table(models.ResourcesTable).Where("id = ? AND org_unit_data_id = ? AND deleted_at IS NULL", r.ID, data).Updates(numericQuotas(effective)); err != nil {
				return nil, err
			}
		}
	}
	if err := saveQuotaControl(tx, c); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	InvalidateAllCache(ctx, tenant, cluster)
	return c.management(), nil
}

func BindQuotaUID(ctx context.Context, tenant, cluster, kind string, id int64, uid string) error {
	if uid == "" {
		return nil
	}
	tx := runaidb.DB().WithContext(ctx).Begin()
	committed := false
	defer orm.FinalizeTransaction(tx, &committed)
	if _, err := lockQuotaOrgUnit(tx, tenant, cluster, kind, id); err != nil {
		return err
	}
	c, err := quotaControl(tx, tenant, cluster, kind, id, true)
	if err != nil {
		return err
	}
	if c == nil {
		c = &QuotaControl{TenantID: tenant, ClusterID: cluster, Kind: kind, OrgUnitID: id, Source: "runai", TargetSource: "runai", Phase: "stable"}
	}
	if c.KubernetesUID != "" && c.KubernetesUID != uid && (c.Revision > 0 || c.LegacyState != "") {
		c.Message = "Kubernetes UID changed; explicit recovery is required"
		if err := saveQuotaControl(tx, c); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		return fmt.Errorf("%w: %s", ErrQuotaSnapshot, c.Message)
	}
	c.KubernetesUID = uid
	c.Message = ""
	if err := saveQuotaControl(tx, c); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
