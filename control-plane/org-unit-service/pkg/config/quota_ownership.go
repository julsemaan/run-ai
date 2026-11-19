// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/generated/pkg/api"
	commonconfig "github.com/dsx-ai-factory/run-ai/runai-common-packages/config"
	"github.com/google/uuid"
)

// QuotaEnrollment is operator configuration, never object annotations or request data.
type QuotaEnrollment struct {
	TenantId      string                   `json:"tenantId"`
	ClusterId     string                   `json:"clusterId"`
	Kind          string                   `json:"kind"`
	Id            int64                    `json:"id"`
	KubernetesUid string                   `json:"kubernetesUid"`
	Epoch         string                   `json:"epoch"`
	State         string                   `json:"state"`
	Pools         []api.QuotaOwnershipPool `json:"pools"`
}

func ParseQuotaEnrollments(raw string) ([]QuotaEnrollment, error) {
	if raw == "" {
		return nil, nil
	}
	var entries []QuotaEnrollment
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("invalid quota_poc_enrollments: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("quota_poc_enrollments has trailing data")
	}
	if entries == nil {
		return nil, fmt.Errorf("quota_poc_enrollments must be a JSON array")
	}
	seen := map[string]bool{}
	for _, e := range entries {
		key := fmt.Sprintf("%s/%s/%s/%d", e.TenantId, e.ClusterId, e.Kind, e.Id)
		if e.TenantId == "" || e.Id <= 0 || (e.Kind != "project" && e.Kind != "department") || e.Epoch == "" || (e.State != "prepared" && e.State != "active") || len(e.Pools) == 0 || seen[key] {
			return nil, fmt.Errorf("invalid or duplicate quota enrollment %s", key)
		}
		if _, err := uuid.Parse(e.ClusterId); err != nil {
			return nil, fmt.Errorf("invalid enrollment cluster: %w", err)
		}
		if _, err := uuid.Parse(e.KubernetesUid); err != nil {
			return nil, fmt.Errorf("invalid enrollment UID: %w", err)
		}
		seen[key] = true
		ids, names, queues := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, p := range e.Pools {
			if p.NodePoolId == "" || p.NodePoolName == "" || p.QueueName == "" || ids[p.NodePoolId] || names[p.NodePoolName] || queues[p.QueueName] {
				return nil, fmt.Errorf("invalid or duplicate pool in %s", key)
			}
			ids[p.NodePoolId], names[p.NodePoolName], queues[p.QueueName] = true, true, true
		}
	}
	return entries, nil
}

func QuotaEnrollments() ([]QuotaEnrollment, error) {
	return ParseQuotaEnrollments(commonconfig.Get[*General]().QuotaPocEnrollments)
}

func FindQuotaEnrollment(tenantId, clusterId, kind string, id int64) (*QuotaEnrollment, error) {
	entries, err := QuotaEnrollments()
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.TenantId == tenantId && e.ClusterId == clusterId && e.Kind == kind && e.Id == id {
			return &e, nil
		}
	}
	return nil, nil
}

func (e QuotaEnrollment) Ownership() *api.QuotaOwnership {
	if e.State != "active" {
		return nil
	}
	pools := append([]api.QuotaOwnershipPool(nil), e.Pools...)
	sort.Slice(pools, func(i, j int) bool { return pools[i].NodePoolId < pools[j].NodePoolId })
	return &api.QuotaOwnership{Epoch: e.Epoch, KubernetesUid: e.KubernetesUid, Pools: pools}
}
