/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package handlers

import (
	"context"
	"encoding/json"
	"testing"

	api "github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/client/generated"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/agent/pkg/quotaownership"
	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestQuotaReconciliation(t *testing.T) {
	for _, kind := range []string{"Project", "Department"} {
		t.Run(kind, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := kai.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			var desired client.Object
			var inspector ObjectInspector
			if kind == "Project" {
				desired = &kai.Project{}
				inspector = KaiProjectObjectInspector(true)
			} else {
				desired = &kai.Department{}
				inspector = KaiDepartmentObjectInspector(true)
			}
			desired.SetName("test")
			ownership := api.QuotaOwnership{Epoch: "epoch", KubernetesUid: "uid", Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "pool", QueueName: "queue"}}}
			if err := quotaownership.Set(desired, &ownership); err != nil {
				t.Fatal(err)
			}
			queues := []kai.QueueConfig{{Name: "queue", Nodepool: "pool", Resources: &kai.QueueResourcesConfig{GPU: kai.SystemResource{Limit: -1}}}, {Name: "other", Nodepool: "other", Resources: &kai.QueueResourcesConfig{GPU: kai.SystemResource{Deserved: 2, Limit: 2}}}}
			switch d := desired.(type) {
			case *kai.Project:
				d.Spec.Queues = queues
				d.Spec.Parent = "new-parent"
			case *kai.Department:
				d.Spec.Queues = queues
			}
			live := desired.DeepCopyObject().(client.Object)
			live.SetUID("uid")
			live.SetAnnotations(nil)
			if p, ok := live.(*kai.Project); ok {
				p.Spec.Parent = "old-parent"
			}
			quotaownership.Queues(live)[0].Resources.GPU.Deserved = 0.5
			quotaownership.Queues(live)[1].Resources.GPU.Deserved = 0
			stale := fake.NewClientBuilder().WithScheme(scheme).WithObjects(live.DeepCopyObject().(client.Object)).Build()
			attempts := 0
			apiClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(live).WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				attempts++
				if attempts == 1 {
					latest := live.DeepCopyObject().(client.Object)
					if err := c.Get(ctx, client.ObjectKeyFromObject(obj), latest); err != nil {
						return err
					}
					quotaownership.Queues(latest)[0].Resources.GPU.Deserved = 0.75
					quotaownership.Queues(latest)[0].Resources.CPU = kai.SystemResource{Deserved: 250.5, Limit: -1, OverQuotaWeight: 0}
					quotaownership.Queues(latest)[0].Resources.Memory = kai.SystemResource{Deserved: 123.5, Limit: 1000, OverQuotaWeight: 2}
					if err := c.Update(ctx, latest); err != nil {
						return err
					}
					return errors.NewConflict(schema.GroupResource{Group: "run.ai", Resource: "projects"}, obj.GetName(), nil)
				}
				return c.Update(ctx, obj, opts...)
			}}).Build()
			h := &CommonHandler{runaiClient: apiClient, cachedClient: stale, gvk: kai.GroupVersion.WithKind(kind), objectInspector: inspector}
			for i := 0; i < 2; i++ {
				if err := h.CreateOrUpdateObject(desired, ""); err != nil {
					t.Fatal(err)
				}
				got := live.DeepCopyObject().(client.Object)
				if err := apiClient.Get(context.Background(), client.ObjectKeyFromObject(live), got); err != nil {
					t.Fatal(err)
				}
				r := quotaownership.Queues(got)[0].Resources
				if r.GPU.Deserved != 0.75+float64(i)*0.125 || r.CPU.Deserved != 250.5 || r.CPU.Limit != -1 || r.Memory.Deserved != 123.5 || quotaownership.Queues(got)[1].Resources.GPU.Deserved != 2 {
					t.Fatalf("lost latest quotas or API pool: %+v", r)
				}
				var actual api.QuotaOwnership
				if err := json.Unmarshal([]byte(got.GetAnnotations()[quotaownership.Annotation]), &actual); err != nil || actual.Epoch != "epoch" {
					t.Fatalf("bad ownership: %v", err)
				}
				if p, ok := got.(*kai.Project); ok && p.Spec.Parent != "new-parent" {
					t.Fatal("nonquota spec not reconciled")
				}
				// A rapid quota patch followed by an API-owned priority update must keep the patch.
				quotaownership.Queues(got)[0].Resources.GPU.Deserved += 0.125
				if err := apiClient.Update(context.Background(), got); err != nil {
					t.Fatal(err)
				}
				priority := int32(7)
				quotaownership.Queues(desired)[0].Priority = &priority
			}
			if attempts < 3 {
				t.Fatal("conflict retry or nonquota update not exercised")
			}
		})
	}
}

func TestNativeStaleCommandConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := kai.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	live := &kai.Project{ObjectMeta: metav1.ObjectMeta{Name: "p", UID: "uid"}}
	live.Spec.Queues = []kai.QueueConfig{{Name: "q", Nodepool: "pool", Resources: &kai.QueueResourcesConfig{GPU: kai.SystemResource{Deserved: 9, Limit: -1}}}}
	marker := func(phase string, revision int64) string {
		o := api.QuotaOwnership{Epoch: "e", KubernetesUid: "uid", Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "pool", QueueName: "q"}}}
		o.SetSource("kubernetes")
		o.SetPhase(phase)
		o.SetRevision(revision)
		encoded, _ := json.Marshal(o)
		return string(encoded)
	}
	desired := live.DeepCopy()
	desired.Annotations = map[string]string{quotaownership.Annotation: marker("freeze", 1)}
	desired.Spec.Queues[0].Resources.GPU.Deserved = 2
	attempts := 0
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(live).WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
		attempts++
		latest := live.DeepCopy()
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), latest); err != nil {
			return err
		}
		latest.Annotations = map[string]string{quotaownership.Annotation: marker("release", 2)}
		if err := c.Update(ctx, latest); err != nil {
			return err
		}
		return errors.NewConflict(schema.GroupResource{Resource: "projects"}, obj.GetName(), nil)
	}}).Build()
	h := &CommonHandler{runaiClient: c, cachedClient: c, gvk: kai.GroupVersion.WithKind("Project"), objectInspector: KaiProjectObjectInspector(true)}
	if err := h.CreateOrUpdateObject(desired, ""); err == nil {
		t.Fatal("stale command accepted after conflict")
	}
	got := live.DeepCopy()
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(live), got); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || got.Spec.Queues[0].Resources.GPU.Deserved != 9 || got.Annotations[quotaownership.Annotation] != marker("release", 2) {
		t.Fatal("stale retry regressed live control or quotas")
	}
}

func TestQuotaLegacyWireShape(t *testing.T) {
	o := api.QuotaOwnership{Epoch: "e", KubernetesUid: "uid", Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "pool", QueueName: "q"}}}
	o.SetRevision(0)
	o.SetSource("kubernetes")
	o.SetPhase("stable")
	p := &kai.Project{}
	if err := quotaownership.Set(p, &o); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(p.Annotations[quotaownership.Annotation]), &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 || fields["revision"] != nil || fields["source"] != nil || fields["phase"] != nil {
		t.Fatal("legacy marker changed wire shape")
	}
}

func TestBackendOwnershipConversion(t *testing.T) {
	ownership := api.QuotaOwnership{Epoch: "epoch", KubernetesUid: "uid", Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "pool", QueueName: "queue"}}}
	p := api.ProjectSync{Name: "project"}
	p.QuotaOwnership.Set(&ownership)
	project, err := convertOrgUnitToKaiProject(p)
	if err != nil {
		t.Fatal(err)
	}
	d := api.DepartmentSync{}
	d.QuotaOwnership.Set(&ownership)
	department, err := convertOrgUnitToKaiDepartment(d)
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := json.Marshal(ownership)
	for _, o := range []metav1.Object{project, department} {
		if o.GetAnnotations()[quotaownership.Annotation] != string(expected) {
			t.Fatal("annotation differs from generated backend JSON")
		}
	}
}
