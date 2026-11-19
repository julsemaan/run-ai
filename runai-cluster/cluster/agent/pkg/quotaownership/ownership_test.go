/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package quotaownership

import (
	"encoding/json"
	api "github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/client/generated"
	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestOwnershipRemovalDoesNotRevertUnmirroredQuota(t *testing.T) {
	o := api.QuotaOwnership{Epoch: "epoch", KubernetesUid: "uid", Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "pool", QueueName: "queue"}}}
	encoded, _ := json.Marshal(o)
	live := &kai.Project{ObjectMeta: metav1.ObjectMeta{Name: "project", UID: "uid", Annotations: map[string]string{Annotation: string(encoded)}}}
	live.Spec.Queues = []kai.QueueConfig{{Name: "queue", Nodepool: "pool", Resources: &kai.QueueResourcesConfig{GPU: kai.SystemResource{Deserved: 2}}}}
	backend := live.DeepCopy()
	delete(backend.Annotations, Annotation)
	backend.Spec.Queues[0].Resources.GPU.Deserved = 1
	if _, err := Desired(backend, live); err == nil {
		t.Fatal("missing backend enrollment must not overwrite newer Kubernetes quota")
	}
}

func TestPreserve(t *testing.T) {
	o := api.QuotaOwnership{Epoch: "epoch", KubernetesUid: "uid", Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "pool", QueueName: "queue"}}}
	encoded, _ := json.Marshal(o)
	desired := &kai.Project{ObjectMeta: metav1.ObjectMeta{Name: "project", Annotations: map[string]string{Annotation: string(encoded)}}}
	desired.Spec.Queues = []kai.QueueConfig{{Name: "queue", Nodepool: "pool", Resources: &kai.QueueResourcesConfig{}}, {Name: "other", Nodepool: "other", Resources: &kai.QueueResourcesConfig{}}}
	live := desired.DeepCopy()
	live.UID = "uid"
	live.Spec.Queues[0].Resources = &kai.QueueResourcesConfig{GPU: kai.SystemResource{Deserved: 0.5, Limit: -1, OverQuotaWeight: 0}, CPU: kai.SystemResource{Deserved: 100.5, Limit: -1, OverQuotaWeight: 2}, Memory: kai.SystemResource{Deserved: 0, Limit: 123.5, OverQuotaWeight: 1}}
	live.Spec.Queues[1].Resources.GPU.Deserved = 99
	got, err := Desired(desired, live)
	if err != nil {
		t.Fatal(err)
	}
	p := got.(*kai.Project)
	if *p.Spec.Queues[0].Resources != *live.Spec.Queues[0].Resources || p.Spec.Queues[1].Resources.GPU.Deserved != 0 {
		t.Fatal("quota scope or units changed")
	}
	live.UID = "recreated"
	if _, err := Desired(desired, live); err == nil {
		t.Fatal("accepted recreated object")
	}
	delete(desired.Annotations, Annotation)
	got, err = Desired(desired, live)
	if err != nil || got.(*kai.Project).Spec.Queues[0].Resources.GPU.Deserved != 0 {
		t.Fatal("annotation forged enrollment")
	}
}
