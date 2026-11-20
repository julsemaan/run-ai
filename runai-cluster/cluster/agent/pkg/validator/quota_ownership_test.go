/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package validator

import (
	"context"
	"encoding/json"
	api "github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/client/generated"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/agent/pkg/quotaownership"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/common/constants"
	runaiv1 "github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/sdk/apis/run/v1"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"testing"
)

func quotaFixture() map[string]interface{} {
	o := api.QuotaOwnership{Epoch: "epoch", KubernetesUid: "uid", Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "pool", QueueName: "queue"}}}
	encoded, _ := json.Marshal(o)
	return map[string]interface{}{"apiVersion": "kai.resources/v1alpha1", "kind": "Project", "metadata": map[string]interface{}{"name": "p", "uid": "uid", "resourceVersion": "1", "annotations": map[string]interface{}{quotaownership.Annotation: string(encoded)}}, "spec": map[string]interface{}{"parent": "d", "namespace": "ns", "queues": []interface{}{map[string]interface{}{"name": "queue", "nodepool": "pool", "priority": float64(1), "resources": map[string]interface{}{"gpu": map[string]interface{}{"deserved": 0.5, "limit": -1.0, "overQuotaWeight": 0.0}, "cpu": map[string]interface{}{"deserved": 250.5, "limit": -1.0, "overQuotaWeight": 2.0}, "memory": map[string]interface{}{"deserved": 123.5, "limit": 1000.0, "overQuotaWeight": 1.0}}}}}}
}

func quotaRequest(t *testing.T, mutate func(map[string]interface{})) admission.Request {
	t.Helper()
	old := quotaFixture()
	new := runtime.DeepCopyJSON(old)
	mutate(new)
	a, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(new)
	if err != nil {
		t.Fatal(err)
	}
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{Operation: admissionv1.Update, Kind: metav1.GroupVersionKind{Group: "kai.resources", Version: "v1alpha1", Kind: "Project"}, OldObject: runtime.RawExtension{Raw: a}, Object: runtime.RawExtension{Raw: b}, UserInfo: authenticationv1.UserInfo{UID: "editor"}}}
}

func TestQuotaOnlyAdmission(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(map[string]interface{})
		allowed bool
	}{
		{"fraction", func(o map[string]interface{}) {
			wireQueue(o["spec"].(map[string]interface{})["queues"].([]interface{}), "queue", "pool")["resources"].(map[string]interface{})["gpu"].(map[string]interface{})["deserved"] = 0.75
		}, true},
		{"zero", func(o map[string]interface{}) {
			wireQueue(o["spec"].(map[string]interface{})["queues"].([]interface{}), "queue", "pool")["resources"].(map[string]interface{})["gpu"].(map[string]interface{})["deserved"] = 0.0
		}, true},
		{"parent", func(o map[string]interface{}) { o["spec"].(map[string]interface{})["parent"] = "other" }, false},
		{"namespace", func(o map[string]interface{}) { o["spec"].(map[string]interface{})["namespace"] = "other" }, false},
		{"labels", func(o map[string]interface{}) {
			o["metadata"].(map[string]interface{})["labels"] = map[string]interface{}{"id": "other"}
		}, false},
		{"annotation", func(o map[string]interface{}) {
			o["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})["run.ai/last-updated"] = "forged"
		}, false},
		{"sync-ack", func(o map[string]interface{}) {
			o["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})["run.ai/quota-ack"] = "forged"
		}, false},
		{"sync-timestamp", func(o map[string]interface{}) {
			o["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})["run.ai/last-sync-timestamp"] = "forged"
		}, false},
		{"ownership", func(o map[string]interface{}) {
			delete(o["metadata"].(map[string]interface{})["annotations"].(map[string]interface{}), quotaownership.Annotation)
		}, false},
		{"uid", func(o map[string]interface{}) { o["metadata"].(map[string]interface{})["uid"] = "new" }, false},
		{"status", func(o map[string]interface{}) { o["status"] = map[string]interface{}{"phase": "Ready"} }, false},
		{"finalizers", func(o map[string]interface{}) {
			o["metadata"].(map[string]interface{})["finalizers"] = []interface{}{"controller"}
		}, false},
		{"priority", func(o map[string]interface{}) {
			o["spec"].(map[string]interface{})["queues"].([]interface{})[0].(map[string]interface{})["priority"] = 2.0
		}, false},
		{"pool", func(o map[string]interface{}) {
			o["spec"].(map[string]interface{})["queues"].([]interface{})[0].(map[string]interface{})["nodepool"] = "other"
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := quotaRequest(t, func(o map[string]interface{}) {
				// Every forbidden edit is bundled with an otherwise valid quota update.
				wireQueue(o["spec"].(map[string]interface{})["queues"].([]interface{}), "queue", "pool")["resources"].(map[string]interface{})["cpu"].(map[string]interface{})["deserved"] = 500.25
				tc.mutate(o)
			})
			if got := quotaOnly(req); got != tc.allowed {
				t.Fatalf("allowed=%v want %v", got, tc.allowed)
			}
		})
	}
	for _, v := range []interface{}{-0.5, -2.0, nil, "1"} {
		t.Run("invalid-number", func(t *testing.T) {
			req := quotaRequest(t, func(o map[string]interface{}) {
				wireQueue(o["spec"].(map[string]interface{})["queues"].([]interface{}), "queue", "pool")["resources"].(map[string]interface{})["cpu"].(map[string]interface{})["deserved"] = v
			})
			if quotaOnly(req) {
				t.Fatal("accepted invalid number")
			}
		})
	}
	for _, resource := range []string{"gpu", "cpu", "memory"} {
		for _, bad := range []string{"missing", "negative-weight", "limit", "unlimited-deserved"} {
			t.Run(resource+bad, func(t *testing.T) {
				req := quotaRequest(t, func(o map[string]interface{}) {
					r := wireQueue(o["spec"].(map[string]interface{})["queues"].([]interface{}), "queue", "pool")["resources"].(map[string]interface{})[resource].(map[string]interface{})
					switch bad {
					case "missing":
						delete(r, "deserved")
					case "negative-weight":
						r["overQuotaWeight"] = -1.0
					case "limit":
						r["limit"] = 0.0
					case "unlimited-deserved":
						r["deserved"] = -1.0
						r["limit"] = 1.0
					}
				})
				if quotaOnly(req) {
					t.Fatal("accepted invalid quota")
				}
			})
		}
	}
}

func TestQuotaAdmissionScope(t *testing.T) {
	for _, kind := range []string{"Project", "Department"} {
		t.Run(kind, func(t *testing.T) {
			req := quotaRequest(t, func(o map[string]interface{}) {
				wireQueue(o["spec"].(map[string]interface{})["queues"].([]interface{}), "queue", "pool")["resources"].(map[string]interface{})["gpu"].(map[string]interface{})["deserved"] = 0.75
			})
			req.Kind.Kind = kind
			for _, group := range []string{"run.ai", "scheduling.run.ai", "other"} {
				legacy := req
				legacy.Kind.Group = group
				if quotaOnly(legacy) {
					t.Fatalf("accepted public quota update for %s/%s", group, kind)
				}
			}
			if !quotaOnly(req) {
				t.Fatal("valid quota rejected")
			}
			for _, mode := range []string{"old-uid", "unenrolled", "nonenrolled-pool", "new-pool", "reorder"} {
				t.Run(mode, func(t *testing.T) {
					copy := req
					var old, new map[string]interface{}
					if err := json.Unmarshal(req.OldObject.Raw, &old); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(req.Object.Raw, &new); err != nil {
						t.Fatal(err)
					}
					switch mode {
					case "old-uid":
						old["metadata"].(map[string]interface{})["uid"] = "recreated"
						new["metadata"].(map[string]interface{})["uid"] = "recreated"
					case "unenrolled":
						delete(old["metadata"].(map[string]interface{})["annotations"].(map[string]interface{}), quotaownership.Annotation)
						delete(new["metadata"].(map[string]interface{})["annotations"].(map[string]interface{}), quotaownership.Annotation)
					default:
						oq := old["spec"].(map[string]interface{})["queues"].([]interface{})
						nq := new["spec"].(map[string]interface{})["queues"].([]interface{})
						other := runtime.DeepCopyJSON(oq[0].(map[string]interface{}))
						other["name"] = "other"
						other["nodepool"] = "other"
						if mode != "new-pool" {
							old["spec"].(map[string]interface{})["queues"] = append(oq, other)
						}
						changed := runtime.DeepCopyJSON(other)
						if mode == "nonenrolled-pool" {
							changed["resources"].(map[string]interface{})["cpu"].(map[string]interface{})["deserved"] = 999.0
						}
						nq = append(nq, changed)
						if mode == "reorder" {
							nq[0], nq[1] = nq[1], nq[0]
						}
						new["spec"].(map[string]interface{})["queues"] = nq
					}
					copy.OldObject.Raw, _ = json.Marshal(old)
					copy.Object.Raw, _ = json.Marshal(new)
					if quotaOnly(copy) {
						t.Fatal("accepted out-of-scope change")
					}
				})
			}
		})
	}
}

func TestOwnershipProtectionAndControllers(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	accounts := []struct{ name, uid string }{{constants.RunaiAgentName, "agent"}, {constants.RunaiClusterSyncName, "sync"}, {constants.RunaiOperatorName, "operator"}, {constants.KRMProjectControllerName, "controller"}}
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, a := range accounts {
		builder.WithObjects(&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: a.name, Namespace: runaiv1.DefaultNamespace, UID: types.UID(a.uid)}})
	}
	c := builder.Build()
	pv := NewProjectValidator(c, c, nil)
	for _, uid := range []string{"editor", "controller", "sync", "operator", "agent"} {
		t.Run(uid, func(t *testing.T) {
			forge := quotaRequest(t, func(o map[string]interface{}) {
				delete(o["metadata"].(map[string]interface{})["annotations"].(map[string]interface{}), quotaownership.Annotation)
			})
			forge.UserInfo.UID = uid
			if got := pv.Handle(context.Background(), forge).Allowed; got {
				t.Fatalf("ownership modification allowed=%v", got)
			}
			for _, operation := range []string{"status", "finalizers", "run.ai/quota-ack", "run.ai/last-sync-timestamp"} {
				req := quotaRequest(t, func(o map[string]interface{}) {
					if operation == "status" {
						o["status"] = map[string]interface{}{"phase": "Ready"}
					} else if operation == "finalizers" {
						o["metadata"].(map[string]interface{})["finalizers"] = []interface{}{"controller"}
					} else {
						o["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})[operation] = "internal-sync"
					}
				})
				req.UserInfo.UID = uid
				if operation == "status" {
					req.SubResource = "status"
				}
				if got := pv.Handle(context.Background(), req).Allowed; got != (uid != "editor") {
					t.Fatalf("%s allowed=%v", operation, got)
				}
			}
		})
	}
	req := quotaRequest(t, func(o map[string]interface{}) { _ = unstructured.SetNestedField(o, "changed", "spec", "parent") })
	// A forged annotation on a previously unenrolled object cannot enable public writes.
	var old map[string]interface{}
	if err := json.Unmarshal(req.OldObject.Raw, &old); err != nil {
		t.Fatal(err)
	}
	delete(old["metadata"].(map[string]interface{})["annotations"].(map[string]interface{}), quotaownership.Annotation)
	req.OldObject.Raw, _ = json.Marshal(old)
	if pv.Handle(context.Background(), req).Allowed {
		t.Fatal("public caller forged enrollment")
	}
}
