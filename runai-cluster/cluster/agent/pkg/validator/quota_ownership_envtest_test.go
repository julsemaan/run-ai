/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package validator

import (
	"context"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/client/generated"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/agent/pkg/quotaownership"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/common/constants"
	runaiv1 "github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/sdk/apis/run/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestQuotaAdmissionEnvtest(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("requires isolated envtest assets")
	}
	// Use the real CRDs from the same pinned module as the agent's KAI types.
	module, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/kai-scheduler/kai-resource-management-api").Output()
	if err != nil {
		t.Fatal(err)
	}
	crds := filepath.Join(strings.TrimSpace(string(module)), "config", "crd")
	environment := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join(crds, "kai.resources_projects.yaml"), filepath.Join(crds, "kai.resources_departments.yaml")}, ErrorIfCRDPathMissing: true}
	config, err := environment.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := environment.Stop(); err != nil {
			t.Error(err)
		}
	})
	scheme := runtime.NewScheme()
	if err := admissionregistrationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	admin, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := admin.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: runaiv1.DefaultNamespace}}); err != nil {
		t.Fatal(err)
	}
	impersonate := func(name, uid string) client.Client {
		cfg := rest.CopyConfig(config)
		// Isolate webhook authorization from RBAC, preserving real admission identities.
		cfg.Impersonate = rest.ImpersonationConfig{UserName: name, UID: uid, Groups: []string{"system:masters"}}
		c, err := client.New(cfg, client.Options{Scheme: scheme})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	editor := impersonate("quota-editor", "editor")
	internal := map[string]client.Client{}
	for _, name := range []string{constants.RunaiAgentName, constants.RunaiClusterSyncName, constants.RunaiOperatorName, constants.KRMProjectControllerName} {
		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: runaiv1.DefaultNamespace}}
		if err := admin.Create(ctx, sa); err != nil {
			t.Fatal(err)
		}
		internal[name] = impersonate("system:serviceaccount:"+runaiv1.DefaultNamespace+":"+name, string(sa.UID))
	}
	server := httptest.NewTLSServer(&admission.Webhook{Handler: NewProjectValidator(admin, admin, nil)})
	defer server.Close()
	webhook := &admissionregistrationv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "quota-test"}, Webhooks: []admissionregistrationv1.ValidatingWebhook{{Name: "quota.test.run.ai", AdmissionReviewVersions: []string{"v1"}, SideEffects: ptr.To(admissionregistrationv1.SideEffectClassNone), FailurePolicy: ptr.To(admissionregistrationv1.Fail), ClientConfig: admissionregistrationv1.WebhookClientConfig{URL: &server.URL, CABundle: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})}, Rules: []admissionregistrationv1.RuleWithOperations{
		{Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update, admissionregistrationv1.Delete}, Rule: admissionregistrationv1.Rule{APIGroups: []string{"run.ai", "kai.resources"}, APIVersions: []string{"*"}, Resources: []string{"projects", "projects/status"}}},
		{Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update, admissionregistrationv1.Delete}, Rule: admissionregistrationv1.Rule{APIGroups: []string{"scheduling.run.ai", "kai.resources"}, APIVersions: []string{"*"}, Resources: []string{"departments", "departments/status"}}},
	}}}}
	if err := admin.Create(ctx, webhook); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"Project", "Department"} {
		t.Run(kind, func(t *testing.T) {
			agent := internal[constants.RunaiAgentName]
			o := &unstructured.Unstructured{Object: quotaFixture()}
			o.SetKind(kind)
			o.SetUID("")
			o.SetResourceVersion("")
			o.SetAnnotations(nil)
			if err := agent.Create(ctx, o); err != nil {
				t.Fatal(err)
			}
			ownership := api.QuotaOwnership{Epoch: "epoch", KubernetesUid: string(o.GetUID()), Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "pool", QueueName: "queue"}}}
			if err := quotaownership.Set(o, &ownership); err != nil {
				t.Fatal(err)
			}
			if err := agent.Update(ctx, o); err != nil {
				t.Fatal(err)
			}
			// Registration is asynchronous. A forbidden metadata write must be rejected.
			deadline := time.Now().Add(10 * time.Second)
			for {
				probe := o.DeepCopy()
				probe.SetLabels(map[string]string{"probe": "forbidden"})
				err := editor.Update(ctx, probe)
				if apierrors.IsForbidden(err) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				o = probe
				if time.Now().After(deadline) {
					t.Fatal("webhook did not activate")
				}
				time.Sleep(100 * time.Millisecond)
			}
			for _, key := range []string{quotaownership.Annotation, "run.ai/quota-ack", "run.ai/last-sync-timestamp"} {
				for _, remove := range []bool{false, true} {
					// Seed via its authorized writer, then prove editors cannot replace or remove it.
					writer := internal[constants.RunaiClusterSyncName]
					if key == quotaownership.Annotation {
						writer = agent
					} else {
						annotations := o.GetAnnotations()
						annotations[key] = "sync-value"
						o.SetAnnotations(annotations)
						if err := writer.Update(ctx, o); err != nil {
							t.Fatalf("internal sync write: %v", err)
						}
					}
					probe := o.DeepCopy()
					annotations := probe.GetAnnotations()
					if remove {
						delete(annotations, key)
					} else {
						annotations[key] = "forged"
					}
					probe.SetAnnotations(annotations)
					if err := editor.Update(ctx, probe); !apierrors.IsForbidden(err) {
						t.Fatalf("editor modified %s, remove=%v: %v", key, remove, err)
					}
					if key == quotaownership.Annotation {
						for name, c := range internal {
							attempt := probe.DeepCopy()
							attempt.SetResourceVersion(o.GetResourceVersion())
							err := c.Update(ctx, attempt)
							if !apierrors.IsForbidden(err) {
								t.Fatalf("%s replaced or removed protected ownership: %v", name, err)
							}
						}
					}
				}
			}
			q := wireQueue(o.Object["spec"].(map[string]interface{})["queues"].([]interface{}), "queue", "pool")
			q["resources"].(map[string]interface{})["gpu"].(map[string]interface{})["deserved"] = 0.75
			if err := editor.Update(ctx, o, client.FieldOwner("quota-editor")); err != nil {
				t.Fatalf("real KAI quota update rejected: %v", err)
			}
			before := o.DeepCopy()
			wireQueue(o.Object["spec"].(map[string]interface{})["queues"].([]interface{}), "queue", "pool")["resources"].(map[string]interface{})["gpu"].(map[string]interface{})["deserved"] = 0.875
			if err := editor.Patch(ctx, o, client.MergeFrom(before), client.FieldOwner("kubectl-patch")); err != nil {
				t.Fatalf("quota patch rejected: %v", err)
			}
			if err := admin.Get(ctx, client.ObjectKeyFromObject(o), o); err != nil {
				t.Fatal(err)
			}
			if o.GetGeneration() != before.GetGeneration()+1 {
				t.Fatal("quota patch did not increment generation")
			}
			for name, c := range internal {
				probe := o.DeepCopy()
				probe.SetFinalizers([]string{"test.run.ai/controller"})
				if err := c.Update(ctx, probe); err != nil {
					t.Fatalf("%s finalizer update: %v", name, err)
				}
				if err := admin.Get(ctx, client.ObjectKeyFromObject(o), o); err != nil {
					t.Fatal(err)
				}
				if err := unstructured.SetNestedField(o.Object, "queue", "status", "queue"); err != nil {
					t.Fatal(err)
				}
				if err := c.Status().Update(ctx, o); err != nil {
					t.Fatalf("%s status update: %v", name, err)
				}
			}
			// Both native directions use real admission and resourceVersion CAS.
			for operation, source := range []string{"runai", "kubernetes"} {
				for step, phase := range []string{"freeze", "release", "stable"} {
					if err := admin.Get(ctx, client.ObjectKeyFromObject(o), o); err != nil {
						t.Fatal(err)
					}
					old := o.DeepCopy()
					ownership.SetSource(source)
					ownership.SetPhase(phase)
					ownership.SetRevision(int64(operation*3 + step + 1))
					ownership.Epoch = source
					if err := quotaownership.Set(o, &ownership); err != nil {
						t.Fatal(err)
					}
					if err := agent.Update(ctx, o); err != nil {
						t.Fatalf("%s %s: %v", source, phase, err)
					}
					stale := old.DeepCopy()
					stale.SetResourceVersion(o.GetResourceVersion())
					if err := agent.Update(ctx, stale); !apierrors.IsForbidden(err) {
						t.Fatalf("stale marker accepted: %v", err)
					}
					for name, writer := range map[string]client.Client{"agent": agent, "editor": editor, "sync": internal[constants.RunaiClusterSyncName]} {
						probe := o.DeepCopy()
						gpu := wireQueue(probe.Object["spec"].(map[string]interface{})["queues"].([]interface{}), "queue", "pool")["resources"].(map[string]interface{})["gpu"].(map[string]interface{})
						gpu["deserved"] = gpu["deserved"].(float64) + 0.01
						err := writer.Update(ctx, probe)
						allowed := phase == "stable" && ((source == "runai" && name == "agent") || (source == "kubernetes" && name != "agent"))
						if allowed {
							if err != nil {
								t.Fatalf("stable writer %s: %v", name, err)
							}
							if err := admin.Get(ctx, client.ObjectKeyFromObject(o), o); err != nil {
								t.Fatal(err)
							}
						} else if !apierrors.IsForbidden(err) {
							t.Fatalf("%s changed %s quota: %v", name, phase, err)
						}
					}
				}
			}
		})
	}
}
