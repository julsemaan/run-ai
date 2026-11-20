/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

package parsers

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestQuotaSnapshot(t *testing.T) {
	ownership := `{"epoch":"e1","kubernetesUid":"uid","pools":[{"nodePoolId":"7","nodePoolName":"pool","queueName":"queue"}]}`
	for _, tc := range []struct {
		name       string
		annotation string
		mutate     func(*metav1.ObjectMeta, []kai.QueueConfig)
		wantError  bool
		absent     bool
	}{
		{name: "legacy", absent: true},
		{name: "complete", annotation: ownership},
		{name: "freeze", annotation: ownership[:len(ownership)-1] + `,"source":"kubernetes","phase":"freeze","revision":1}`},
		{name: "release", annotation: ownership[:len(ownership)-1] + `,"source":"runai","phase":"release","revision":2}`},
		{name: "stable Runai", annotation: ownership[:len(ownership)-1] + `,"source":"runai","phase":"stable","revision":3}`},
		{name: "missing source", annotation: ownership[:len(ownership)-1] + `,"phase":"freeze","revision":1}`, wantError: true},
		{name: "zero revision", annotation: ownership[:len(ownership)-1] + `,"source":"runai","phase":"freeze","revision":0}`, wantError: true},
		{name: "wrong phase", annotation: ownership[:len(ownership)-1] + `,"source":"runai","phase":"wrong","revision":1}`, wantError: true},
		{name: "malformed", annotation: `{`, wantError: true},
		{name: "case alias", annotation: strings.Replace(ownership, `"epoch"`, `"Epoch"`, 1), wantError: true},
		{name: "empty pools", annotation: `{"epoch":"e1","kubernetesUid":"uid","pools":[]}`, wantError: true},
		{name: "duplicate pools", annotation: strings.Replace(ownership, `}]`, `},{"nodePoolId":"7","nodePoolName":"pool","queueName":"queue"}]`, 1), wantError: true},
		{name: "missing pool identity", annotation: strings.Replace(ownership, `"nodePoolId":"7",`, ``, 1), wantError: true},
		{name: "null", annotation: `null`, wantError: true},
		{name: "empty", annotation: ``, wantError: true},
		{name: "unknown field", annotation: ownership[:len(ownership)-1] + `,"forged":true}`, wantError: true},
		{name: "duplicate field", annotation: ownership[:len(ownership)-1] + `,"epoch":"e2"}`, wantError: true},
		{name: "trailing JSON", annotation: ownership + ` {}`, wantError: true},
		{name: "wrong UID", annotation: ownership, mutate: func(m *metav1.ObjectMeta, _ []kai.QueueConfig) { m.UID = "other" }, wantError: true},
		{name: "zero generation", annotation: ownership, mutate: func(m *metav1.ObjectMeta, _ []kai.QueueConfig) { m.Generation = 0 }, wantError: true},
		{name: "wrong queue", annotation: ownership, mutate: func(_ *metav1.ObjectMeta, q []kai.QueueConfig) { q[0].Name = "other" }, wantError: true},
		{name: "wrong pool", annotation: ownership, mutate: func(_ *metav1.ObjectMeta, q []kai.QueueConfig) { q[0].Nodepool = "other" }, wantError: true},
		{name: "missing resources", annotation: ownership, mutate: func(_ *metav1.ObjectMeta, q []kai.QueueConfig) { q[0].Resources = nil }, wantError: true},
		{name: "nonfinite", annotation: ownership, mutate: func(_ *metav1.ObjectMeta, q []kai.QueueConfig) { q[0].Resources.CPU.Limit = math.Inf(1) }, wantError: true},
		{name: "negative weight", annotation: ownership, mutate: func(_ *metav1.ObjectMeta, q []kai.QueueConfig) { q[0].Resources.Memory.OverQuotaWeight = -1 }, wantError: true},
		{name: "invalid sentinel", annotation: ownership, mutate: func(_ *metav1.ObjectMeta, q []kai.QueueConfig) { q[0].Resources.GPU.Deserved = -2 }, wantError: true},
		{name: "small limit", annotation: ownership, mutate: func(_ *metav1.ObjectMeta, q []kai.QueueConfig) { q[0].Resources.GPU.Limit = 0 }, wantError: true},
		{name: "unlimited deserved finite limit", annotation: ownership, mutate: func(_ *metav1.ObjectMeta, q []kai.QueueConfig) { q[0].Resources.GPU.Deserved = -1 }, wantError: true},
		{name: "unlimited", annotation: ownership, mutate: func(_ *metav1.ObjectMeta, q []kai.QueueConfig) {
			q[0].Resources.GPU.Deserved = -1
			q[0].Resources.GPU.Limit = -1
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := metav1.ObjectMeta{Name: "object", UID: "uid", Generation: 12, Annotations: map[string]string{QuotaOwnershipAnnotation: tc.annotation}}
			if tc.absent {
				delete(m.Annotations, QuotaOwnershipAnnotation)
			}
			r := kai.SystemResource{Deserved: 1.5, Limit: 2, OverQuotaWeight: 0}
			q := []kai.QueueConfig{{Name: "queue", Nodepool: "pool", Resources: &kai.QueueResourcesConfig{GPU: r, CPU: r, Memory: r}}}
			if tc.mutate != nil {
				tc.mutate(&m, q)
			}
			s, err := ParseQuotaSnapshot(&m, q)
			if (err != nil) != tc.wantError {
				t.Fatalf("snapshot=%+v error=%v", s, err)
			}
			if !tc.wantError && !tc.absent && (s == nil || s.Generation != 12 || s.Pools[0].NodePoolId != "7") {
				t.Fatalf("wrong snapshot: %+v", s)
			}
			if err == nil && s != nil {
				encoded, marshalErr := json.Marshal(s)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				if strings.Contains(tc.annotation, `"revision"`) {
					if s.GetControlRevision() < 1 || s.GetSource() == "" || s.GetControlPhase() == "" {
						t.Fatal("missing native fence")
					}
				} else if strings.Contains(string(encoded), "controlRevision") || strings.Contains(string(encoded), "controlPhase") || strings.Contains(string(encoded), `"source"`) {
					t.Fatal("legacy wire shape changed")
				}
			}
			if tc.absent && s != nil {
				t.Fatal("legacy object enrolled")
			}
		})
	}
}
