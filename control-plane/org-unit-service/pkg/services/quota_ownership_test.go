// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"encoding/json"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/generated/pkg/api"
	"k8s.io/utils/ptr"
	"math"
	"testing"
)

func TestQuotaSnapshotNumbers(t *testing.T) {
	good := func() api.QuotaSnapshot {
		r := api.QuotaSnapshotResource{Deserved: ptr.To(0.5), Limit: ptr.To(-1.0), OverQuotaWeight: ptr.To(0.0)}
		return api.QuotaSnapshot{Epoch: "epoch", KubernetesUid: "uid", Generation: 1, Pools: []api.QuotaSnapshotPool{{NodePoolId: "1", Gpu: r, Cpu: r, Memory: r}}}
	}
	if err := ValidateQuotaSnapshotNumbers(good()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*api.QuotaSnapshot)
	}{
		{"missing numeric", func(s *api.QuotaSnapshot) { s.Pools[0].Gpu.Deserved = nil }},
		{"missing weight", func(s *api.QuotaSnapshot) { s.Pools[0].Memory.OverQuotaWeight = nil }},
		{"negative fraction", func(s *api.QuotaSnapshot) { s.Pools[0].Cpu.Deserved = ptr.To(-0.5) }},
		{"negative weight", func(s *api.QuotaSnapshot) { s.Pools[0].Gpu.OverQuotaWeight = ptr.To(-1.0) }},
		{"nan", func(s *api.QuotaSnapshot) { s.Pools[0].Gpu.Limit = ptr.To(math.NaN()) }},
		{"infinite", func(s *api.QuotaSnapshot) { s.Pools[0].Memory.Limit = ptr.To(math.Inf(1)) }},
		{"limit", func(s *api.QuotaSnapshot) { s.Pools[0].Gpu.Limit = ptr.To(0.0) }},
		{"unlimited deserved finite limit", func(s *api.QuotaSnapshot) { s.Pools[0].Cpu.Deserved = ptr.To(-1.0); s.Pools[0].Cpu.Limit = ptr.To(1.0) }},
		{"generation", func(s *api.QuotaSnapshot) { s.Generation = 0 }},
		{"duplicate", func(s *api.QuotaSnapshot) { s.Pools = append(s.Pools, s.Pools[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := good()
			tc.change(&s)
			if err := ValidateQuotaSnapshotNumbers(s); err == nil {
				t.Fatal("accepted invalid snapshot")
			}
		})
	}
}

func TestLegacyQuotaSnapshotContracts(t *testing.T) {
	var project api.ProjectStatusUpdateRequest
	if err := json.Unmarshal([]byte(`{"id":"1","clusterId":"cluster","status":{"phase":"Ready"}}`), &project); err != nil {
		t.Fatal(err)
	}
	if project.QuotaSnapshot != nil {
		t.Fatal("legacy request unexpectedly enrolled")
	}
	for _, value := range []any{api.ProjectSync{}, api.DepartmentSync{}} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, present := fields["quotaOwnership"]; present {
			t.Fatal("ownership must be absent by default")
		}
	}
}
