/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

package controllers

import (
	"testing"

	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/handlers/parsers"
	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestQuotaEvents(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*kai.Project, *kai.Department)
		want   bool
	}{
		{name: "unchanged"},
		{name: "status only", mutate: func(p *kai.Project, d *kai.Department) {
			p.Status.Phase = kai.Ready
			d.Status.Conditions = []kai.DepartmentCondition{{Type: kai.DepartmentDeletionBlocked, Message: "updated"}}
		}, want: true},
		{name: "quota generation", mutate: func(p *kai.Project, d *kai.Department) { p.Generation++; d.Generation++ }, want: true},
		{name: "rapid latest generation", mutate: func(p *kai.Project, d *kai.Department) { p.Generation += 20; d.Generation += 20 }, want: true},
		{name: "epoch activation", mutate: func(p *kai.Project, d *kai.Department) {
			p.Annotations[parsers.QuotaOwnershipAnnotation] = "new"
			d.Annotations[parsers.QuotaOwnershipAnnotation] = "new"
		}, want: true},
		{name: "recreation", mutate: func(p *kai.Project, d *kai.Department) { p.UID = "new"; d.UID = "new" }, want: true},
		{name: "ack metadata only", mutate: func(p *kai.Project, d *kai.Department) {
			p.Annotations[parsers.QuotaAckAnnotation] = "ack"
			d.Annotations[parsers.QuotaAckAnnotation] = "ack"
		}},
		{name: "sync timestamp only", mutate: func(p *kai.Project, d *kai.Department) {
			p.Annotations[parsers.LastSyncTimestampAnnotation] = "later"
			d.Annotations[parsers.LastSyncTimestampAnnotation] = "later"
		}},
		{name: "unrelated metadata", mutate: func(p *kai.Project, d *kai.Department) {
			p.Annotations["other"] = "value"
			d.Annotations["other"] = "value"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &kai.Project{ObjectMeta: metav1.ObjectMeta{UID: "uid", Generation: 1, Annotations: map[string]string{parsers.QuotaOwnershipAnnotation: "ownership"}}}
			d := &kai.Department{ObjectMeta: *p.ObjectMeta.DeepCopy()}
			nextP, nextD := p.DeepCopy(), d.DeepCopy()
			if tc.mutate != nil {
				tc.mutate(nextP, nextD)
			}
			if got := shouldUpdate(p, nextP); got != tc.want {
				t.Fatalf("project event=%v want=%v", got, tc.want)
			}
			if got := shouldUpdateDepartment(d, nextD); got != tc.want {
				t.Fatalf("department event=%v want=%v", got, tc.want)
			}
		})
	}
	p := &kai.Project{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{parsers.QuotaOwnershipAnnotation: "ownership"}}}
	next := p.DeepCopy()
	next.Status.Phase = kai.Ready
	if !shouldUpdate(p, next) {
		t.Fatal("status-only project change ignored")
	}
}
