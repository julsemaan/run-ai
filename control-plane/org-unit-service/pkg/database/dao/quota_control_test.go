// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dao

import (
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/generated/pkg/api"
	"testing"
)

func TestQuotaControlTransfer(t *testing.T) {
	pools := []api.QuotaOwnershipPool{{NodePoolId: "p", NodePoolName: "default", QueueName: "default"}}
	c := QuotaControl{Source: "runai", TargetSource: "runai", Phase: "stable", KubernetesUID: "uid"}
	if err := c.request("kubernetes", 0, pools); err != nil {
		t.Fatal(err)
	}
	if c.Revision != 1 || c.Phase != "freeze" || c.management().CanEditQuotas {
		t.Fatalf("not frozen: %+v", c)
	}
	if err := c.request("runai", 1, pools); err == nil {
		t.Fatal("accepted opposite target while pending")
	}
	if err := c.request("kubernetes", 0, pools); err == nil {
		t.Fatal("accepted stale revision")
	}
	epoch := c.Epoch
	for _, phase := range []string{"freeze", "release", "stable"} {
		s := api.QuotaSnapshot{Epoch: c.Epoch, KubernetesUid: "uid", ControlRevision: c.Revision, ControlPhase: phase, Source: "kubernetes"}
		if err := c.observe(s); err != nil {
			t.Fatal(err)
		}
	}
	if c.Source != "kubernetes" || c.Revision != 3 || c.management().SyncStatus.Phase != "synced" {
		t.Fatalf("not synced: %+v", c)
	}
	if err := c.request("runai", 3, pools); err != nil {
		t.Fatal(err)
	}
	if c.Epoch == epoch {
		t.Fatal("transfer reused epoch")
	}
	for _, phase := range []string{"freeze", "release", "stable"} {
		s := api.QuotaSnapshot{Epoch: c.Epoch, KubernetesUid: "uid", ControlRevision: c.Revision, ControlPhase: phase, Source: "runai"}
		if err := c.observe(s); err != nil {
			t.Fatal(err)
		}
	}
	if !c.management().CanEditQuotas || c.Source != "runai" || c.ownership() == nil {
		t.Fatalf("return lost marker: %+v", c)
	}
}

func TestQuotaControlLegacyExpansion(t *testing.T) {
	old := api.QuotaOwnershipPool{NodePoolId: "p", NodePoolName: "default", QueueName: "default"}
	c := QuotaControl{Source: "kubernetes", TargetSource: "kubernetes", Phase: "stable", LegacyState: "active", Epoch: "old", KubernetesUID: "uid", Pools: []api.QuotaOwnershipPool{old}}
	if c.ownership().Revision != 0 || c.ownership().Source != "" {
		t.Fatal("changed legacy wire identity")
	}
	pools := append([]api.QuotaOwnershipPool{old}, api.QuotaOwnershipPool{NodePoolId: "p2", NodePoolName: "extra", QueueName: "extra"})
	if err := c.request("kubernetes", 0, pools); err != nil {
		t.Fatal(err)
	}
	if c.Phase != "freeze" || c.Revision != 1 || len(c.Pools) != 2 {
		t.Fatalf("legacy normalization was no-op: %+v", c)
	}
	if err := c.observe(api.QuotaSnapshot{Epoch: c.Epoch, KubernetesUid: "replacement", ControlRevision: 1, ControlPhase: "freeze", Source: "kubernetes"}); err == nil {
		t.Fatal("accepted replacement UID")
	}
	if err := c.observe(api.QuotaSnapshot{Epoch: c.Epoch, KubernetesUid: "uid", ControlRevision: 0, ControlPhase: "freeze", Source: "kubernetes"}); err == nil {
		t.Fatal("accepted old snapshot")
	}
}

func TestQuotaControlPreparedNeedsExplicitActivation(t *testing.T) {
	pools := []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "default", QueueName: "project"}}
	c := QuotaControl{Source: "runai", TargetSource: "kubernetes", Phase: "stable", LegacyState: "prepared", Pools: pools, KubernetesUID: "uid", Epoch: "legacy"}
	if c.ownership() != nil || c.management().CanEditQuotas || c.management().SyncStatus.Phase != "pending" {
		t.Fatalf("prepared import changed protection: %+v", c)
	}
	if err := c.request("kubernetes", 0, pools); err != nil {
		t.Fatal(err)
	}
	if c.Revision != 1 || c.Phase != "freeze" || c.Epoch == "legacy" {
		t.Fatalf("explicit activation did not freeze: %+v", c)
	}
}
