// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dao

import (
	"fmt"
	dbtesting "github.com/dsx-ai-factory/run-ai/runai-common-packages/runaidb/testing"
	"github.com/google/uuid"
)

func expectUnmanagedQuotaUpdate(m dbtesting.Mocker, kind string, resources bool) {
	m.ExpectQuery(fmt.Sprintf(`SELECT org_unit_data_id FROM %s WHERE tenant_id = ? AND cluster_id = ? AND id = ? AND deleted_at IS NULL ORDER BY %s.org_unit_data_id LIMIT ? FOR UPDATE`, kind, kind)).Returns([]struct{ OrgUnitDataID uuid.UUID }{{testOrgUnitDataID()}})
	if resources {
		m.ExpectQuery(`SELECT * FROM resources WHERE (org_unit_data_id = ? AND deleted_at IS NULL) AND resources.deleted_at IS NULL`).Returns([]struct{ ID uuid.UUID }{})
	}
	m.ExpectQuery(`SELECT * FROM quota_control WHERE tenant_id = ? AND cluster_id = ? AND kind = ? AND org_unit_id = ?`).Returns([]struct{ Revision int64 }{})
}
func expectQuotaMembershipLocks(m dbtesting.Mocker) {
	m.ExpectQuery(`SELECT * FROM resources WHERE (id IN (?) AND deleted_at IS NULL) AND resources.deleted_at IS NULL`).Returns([]struct{ ID uuid.UUID }{})
	for _, kind := range []string{"project", "department"} {
		m.ExpectQuery(fmt.Sprintf(`SELECT * FROM %s WHERE org_unit_data_id IN (?) AND deleted_at IS NULL ORDER BY tenant_id,cluster_id,id FOR UPDATE`, kind)).Returns([]quotaOrgRow{})
	}
}
func expectQuotaResetLocks(m dbtesting.Mocker, kind string, parent bool) {
	m.ExpectExec("LOCK TABLE " + kind + " IN SHARE ROW EXCLUSIVE MODE").Returns(0)
	sql := fmt.Sprintf(`SELECT * FROM %s WHERE tenant_id = ? AND cluster_id = ? AND deleted_at IS NULL`, kind)
	if parent {
		sql = fmt.Sprintf(`SELECT * FROM %s WHERE (tenant_id = ? AND cluster_id = ? AND deleted_at IS NULL) AND parent_id = ?`, kind)
	}
	m.ExpectQuery(sql + " ORDER BY id FOR UPDATE").Returns([]quotaOrgRow{})
}
