/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package quotaownership

import (
	"fmt"
	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func controlProject(source, phase, epoch string, revision int, value float64) *kai.Project {
	p := &kai.Project{ObjectMeta: metav1.ObjectMeta{Name: "p", UID: "uid"}}
	p.Annotations = map[string]string{Annotation: fmt.Sprintf(`{"epoch":%q,"kubernetesUid":"uid","pools":[{"nodePoolId":"1","nodePoolName":"pool","queueName":"q"}],"source":%q,"phase":%q,"revision":%d}`, epoch, source, phase, revision)}
	p.Spec.Queues = []kai.QueueConfig{{Name: "q", Nodepool: "pool", Resources: &kai.QueueResourcesConfig{GPU: kai.SystemResource{Deserved: value, Limit: value * 2, OverQuotaWeight: value}, CPU: kai.SystemResource{Deserved: value, Limit: value * 2, OverQuotaWeight: value}, Memory: kai.SystemResource{Deserved: value, Limit: value * 2, OverQuotaWeight: value}}}}
	return p
}

func TestLegacyControlCompatibility(t *testing.T) {
	old := Control{Source: "kubernetes", Phase: "stable", Epoch: "old"}
	next := old
	next.Epoch = "rotated"
	if err := Fence(old, next, true); err != nil {
		t.Fatal(err)
	}
	old.Revision = 3
	if err := Fence(old, next, true); err == nil {
		t.Fatal("legacy command replaced native marker")
	}
}

func TestNativeDesired(t *testing.T) {
	for _, tc := range []struct {
		name, oldSource, oldPhase, oldEpoch, source, phase, epoch string
		oldRev, rev                                               int
		want                                                      float64
		fail                                                      bool
	}{
		{"initial seed", "", "", "", "kubernetes", "freeze", "e", 0, 1, 2, false},
		{"Runai switches to Kubernetes", "runai", "stable", "old", "kubernetes", "freeze", "e", 3, 4, 2, false},
		{"freeze retry", "kubernetes", "freeze", "e", "kubernetes", "freeze", "e", 1, 1, 9, false},
		{"release sealed", "kubernetes", "freeze", "e", "kubernetes", "release", "e", 1, 2, 9, false},
		{"stable sealed", "runai", "release", "e", "runai", "stable", "e", 2, 3, 9, false},
		{"stable Kubernetes sealed", "kubernetes", "release", "e", "kubernetes", "stable", "e", 2, 3, 9, false},
		{"stable runai reconcile", "runai", "stable", "e", "runai", "stable", "e", 3, 3, 2, false},
		{"stable runai refresh", "runai", "stable", "e", "runai", "stable", "e", 3, 4, 2, false},
		{"stable Kubernetes revision jump", "kubernetes", "stable", "e", "kubernetes", "stable", "e", 3, 4, 0, true},
		{"stable Runai refresh wrong epoch", "runai", "stable", "e", "runai", "stable", "other", 3, 4, 0, true},
		{"stable kubernetes", "kubernetes", "stable", "e", "kubernetes", "stable", "e", 3, 3, 9, false},
		{"return freezes latest", "kubernetes", "stable", "old", "runai", "freeze", "e", 3, 4, 9, false},
		{"stale command", "runai", "stable", "e", "kubernetes", "freeze", "old", 3, 1, 0, true},
		{"same revision wrong phase", "runai", "stable", "e", "runai", "freeze", "e", 3, 3, 0, true},
		{"wrong epoch release", "kubernetes", "freeze", "e", "kubernetes", "release", "wrong", 1, 2, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			live := controlProject(tc.oldSource, tc.oldPhase, tc.oldEpoch, tc.oldRev, 9)
			if tc.oldRev == 0 {
				live.Annotations = nil
			}
			desired := controlProject(tc.source, tc.phase, tc.epoch, tc.rev, 2)
			got, err := Desired(desired, live)
			if tc.fail {
				if err == nil {
					t.Fatal("accepted invalid command")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range []kai.SystemResource{Queues(got)[0].Resources.GPU, Queues(got)[0].Resources.CPU, Queues(got)[0].Resources.Memory} {
				if r.Deserved != tc.want || r.Limit != tc.want*2 || r.OverQuotaWeight != tc.want {
					t.Fatalf("got %v want %v", r.Deserved, tc.want)
				}
			}
		})
	}
}

func TestNativeIdentityAndEnrollmentFailClosed(t *testing.T) {
	for _, mode := range []string{"wrong UID", "missing backend marker", "wrong binding", "protected pool ID"} {
		t.Run(mode, func(t *testing.T) {
			live := controlProject("runai", "stable", "e", 3, 9)
			desired := controlProject("kubernetes", "freeze", "next", 4, 2)
			switch mode {
			case "wrong UID":
				live.SetUID("replacement")
			case "missing backend marker":
				desired.SetAnnotations(nil)
			case "wrong binding":
				desired.Spec.Queues[0].Nodepool = "other"
			case "protected pool ID":
				live = controlProject("kubernetes", "stable", "e", 3, 9)
				desired.Annotations[Annotation] = strings.Replace(desired.Annotations[Annotation], `"nodePoolId":"1"`, `"nodePoolId":"fabricated"`, 1)
			}
			if _, err := Desired(desired, live); err == nil {
				t.Fatal("invalid control accepted")
			}
		})
	}
}

func TestNativeStableRunaiStructuralReconciliation(t *testing.T) {
	live := controlProject("runai", "stable", "e", 3, 9)
	desired := controlProject("runai", "stable", "e", 3, 2)
	desired.Spec.Queues = append(desired.Spec.Queues, kai.QueueConfig{Name: "other", Nodepool: "other", Resources: &kai.QueueResourcesConfig{GPU: kai.SystemResource{Deserved: 4, Limit: -1}}})
	desired.Annotations[Annotation] = `{"epoch":"e","kubernetesUid":"uid","pools":[{"nodePoolId":"1","nodePoolName":"pool","queueName":"q"},{"nodePoolId":"2","nodePoolName":"other","queueName":"other"}],"source":"runai","phase":"stable","revision":3}`
	got, err := Desired(desired, live)
	if err != nil {
		t.Fatal(err)
	}
	if len(Queues(got)) != 2 || Queues(got)[0].Resources.GPU.Deserved != 2 {
		t.Fatal("stable Runai structure or numbers blocked")
	}
}

func TestNativeStableRunaiMembershipRefresh(t *testing.T) {
	for _, kind := range []string{"project", "department"} {
		t.Run(kind, func(t *testing.T) {
			live := controlProject("runai", "stable", "e", 6, 9)
			live.Spec.Queues = append(live.Spec.Queues, kai.QueueConfig{Name: "other", Nodepool: "other", Resources: &kai.QueueResourcesConfig{GPU: kai.SystemResource{Deserved: 9}}})
			live.Annotations[Annotation] = strings.Replace(live.Annotations[Annotation], `}],`, `},{"nodePoolId":"2","nodePoolName":"other","queueName":"other"}],`, 1)
			asKind := func(p *kai.Project) client.Object {
				if kind == "department" {
					return &kai.Department{ObjectMeta: p.ObjectMeta, Spec: kai.DepartmentSpec{Queues: p.Spec.Queues}}
				}
				return p
			}
			removed := controlProject("runai", "stable", "e", 7, 2)
			got, err := Desired(asKind(removed), asKind(live))
			if err != nil {
				t.Fatal(err)
			}
			if len(Queues(got)) != 1 || Queues(got)[0].Resources.GPU.Deserved != 2 {
				t.Fatal("pool removal or numeric reconciliation blocked")
			}
			added := live.DeepCopy()
			added.Annotations[Annotation] = strings.Replace(added.Annotations[Annotation], `"revision":6`, `"revision":8`, 1)
			got, err = Desired(asKind(added), got)
			if err != nil {
				t.Fatal(err)
			}
			if len(Queues(got)) != 2 || Queues(got)[0].Resources.GPU.Deserved != 9 {
				t.Fatal("pool readdition or numeric reconciliation blocked")
			}
			if _, err := Desired(asKind(removed), got); err == nil {
				t.Fatal("delayed old marker accepted")
			}
		})
	}
}

func TestLegacyMixedFreeze(t *testing.T) {
	live := controlProject("kubernetes", "stable", "old", 0, 9)
	live.Annotations[Annotation] = `{"epoch":"old","kubernetesUid":"uid","pools":[{"nodePoolId":"1","nodePoolName":"pool","queueName":"q"}]}`
	desired := controlProject("kubernetes", "freeze", "new", 1, 2)
	extra := kai.QueueConfig{Name: "other", Nodepool: "other", Resources: &kai.QueueResourcesConfig{GPU: kai.SystemResource{Deserved: 3, Limit: -1}}}
	live.Spec.Queues = append(live.Spec.Queues, extra)
	extra.Resources = &kai.QueueResourcesConfig{GPU: kai.SystemResource{Deserved: 4, Limit: -1}}
	desired.Spec.Queues = append(desired.Spec.Queues, extra)
	desired.Annotations[Annotation] = `{"epoch":"new","kubernetesUid":"uid","pools":[{"nodePoolId":"1","nodePoolName":"pool","queueName":"q"},{"nodePoolId":"2","nodePoolName":"other","queueName":"other"}],"source":"kubernetes","phase":"freeze","revision":1}`
	got, err := Desired(desired, live)
	if err != nil {
		t.Fatal(err)
	}
	if Queues(got)[0].Resources.GPU.Deserved != 9 || Queues(got)[1].Resources.GPU.Deserved != 4 {
		t.Fatal("mixed authority lost")
	}
	desired.Spec.Queues[1].Resources.GPU.Deserved = 0
	retry, err := Desired(desired, got)
	if err != nil {
		t.Fatal(err)
	}
	if Queues(retry)[0].Resources.GPU.Deserved != 9 || Queues(retry)[1].Resources.GPU.Deserved != 4 {
		t.Fatal("retry reseeded newly covered pool")
	}
}
