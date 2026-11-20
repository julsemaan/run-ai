/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	orgclient "github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/client"
	api "github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/client/generated"
	backend "github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/backend-client"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/handlers/parsers"
	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func quotaBackend(url string) *backend.BackendClient {
	return &backend.BackendClient{OrgUnitClient: orgclient.NewOrgUnitExternalClientWithConfig(&api.Configuration{Servers: api.ServerConfigurations{{URL: url}}, HTTPClient: http.DefaultClient})}
}

func quotaFixture(t *testing.T, department bool) (client.Client, client.Object) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := kai.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	m := metav1.ObjectMeta{Name: "unit", UID: "uid", Generation: 1, Labels: map[string]string{"runai/cluster-project-id": "1", "runai/department-id": "1"}, Annotations: map[string]string{parsers.QuotaOwnershipAnnotation: `{"epoch":"epoch","kubernetesUid":"uid","pools":[{"nodePoolId":"7","nodePoolName":"pool","queueName":"queue"}]}`}}
	r := kai.SystemResource{Deserved: 1.5, Limit: 2, OverQuotaWeight: 0}
	queues := []kai.QueueConfig{{Name: "queue", Nodepool: "pool", Resources: &kai.QueueResourcesConfig{GPU: r, CPU: r, Memory: r}}}
	var obj client.Object
	if department {
		d := &kai.Department{ObjectMeta: m}
		d.Spec.Queues = queues
		obj = d
	} else {
		p := &kai.Project{ObjectMeta: m}
		p.Spec.Queues = queues
		obj = p
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
	return c, obj
}

func TestQuotaUploadsRetryLatestAndRestart(t *testing.T) {
	for _, department := range []bool{false, true} {
		name := "project"
		if department {
			name = "department"
		}
		t.Run(name, func(t *testing.T) {
			c, obj := quotaFixture(t, department)
			var response atomic.Int64
			response.Store(http.StatusServiceUnavailable)
			requests := make(chan *api.QuotaSnapshot, 10)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					QuotaSnapshot *api.QuotaSnapshot `json:"quotaSnapshot"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "invalid body", 400)
					return
				}
				requests <- body.QuotaSnapshot
				w.WriteHeader(int(response.Load()))
			}))
			defer server.Close()
			b := quotaBackend(server.URL)
			run := func() error {
				key := client.ObjectKeyFromObject(obj)
				if department {
					return NewDepartmentHandler(b, c).HandleUpdateItem(key)
				}
				return NewProjectHandler(b, c).HandleUpdateItem(key)
			}
			if err := run(); err == nil {
				t.Fatal("failed commit was acknowledged")
			}
			initial := <-requests
			if initial == nil || initial.Generation != 1 {
				t.Fatalf("initial snapshot: %+v", initial)
			}
			current := obj.DeepCopyObject().(client.Object)
			if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), current); err != nil {
				t.Fatal(err)
			}
			if current.GetAnnotations()[parsers.QuotaAckAnnotation] != "" || current.GetAnnotations()[parsers.LastSyncTimestampAnnotation] != "" {
				t.Fatal("failed upload recorded synchronization")
			}
			current.SetGeneration(3)
			switch x := current.(type) {
			case *kai.Project:
				x.Spec.Queues[0].Resources.GPU.Deserved = 2
			case *kai.Department:
				x.Spec.Queues[0].Resources.GPU.Deserved = 2
			}
			if err := c.Update(context.Background(), current); err != nil {
				t.Fatal(err)
			}
			response.Store(http.StatusOK)
			if department {
				response.Store(http.StatusNoContent)
			}
			if err := run(); err != nil {
				t.Fatal(err)
			}
			latest := <-requests
			if latest == nil || latest.Generation != 3 || latest.Pools[0].Gpu.GetDeserved() != 2 {
				t.Fatalf("stale retry: %+v", latest)
			}
			if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), current); err != nil {
				t.Fatal(err)
			}
			var ack parsers.QuotaAck
			if err := json.Unmarshal([]byte(current.GetAnnotations()[parsers.QuotaAckAnnotation]), &ack); err != nil {
				t.Fatal(err)
			}
			if ack.Generation != 3 || ack.KubernetesUID != "uid" || ack.Epoch != "epoch" {
				t.Fatalf("wrong acknowledgment %+v", ack)
			}
			// Each call constructs a fresh handler, including a retry after an already
			// acknowledged generation. Backend idempotency handles restart duplicates.
			if err := run(); err != nil {
				t.Fatal(err)
			}
			if retry := <-requests; retry == nil || retry.Generation != 3 {
				t.Fatalf("restart lost snapshot %+v", retry)
			}
			// A malformed ownership marker cannot downgrade the request to legacy status.
			if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), current); err != nil {
				t.Fatal(err)
			}
			current.GetAnnotations()[parsers.QuotaOwnershipAnnotation] = "{}"
			if err := c.Update(context.Background(), current); err != nil {
				t.Fatal(err)
			}
			if err := run(); err == nil {
				t.Fatal("malformed ownership accepted")
			}
			select {
			case <-requests:
				t.Fatal("malformed ownership sent legacy status")
			default:
			}
		})
	}
}

func TestNativeQuotaPhaseUploads(t *testing.T) {
	for _, department := range []bool{false, true} {
		c, obj := quotaFixture(t, department)
		var snapshot *api.QuotaSnapshot
		var uid string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				QuotaSnapshot *api.QuotaSnapshot `json:"quotaSnapshot"`
				Status        struct {
					KubernetesUid string `json:"kubernetesUid"`
				} `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			snapshot, uid = body.QuotaSnapshot, body.Status.KubernetesUid
			w.WriteHeader(http.StatusNoContent)
		}))
		b := quotaBackend(server.URL)
		for operation, source := range []string{"kubernetes", "runai"} {
			for step, phase := range []string{"freeze", "release", "stable"} {
				if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
					t.Fatal(err)
				}
				var ownership api.QuotaOwnership
				if err := json.Unmarshal([]byte(obj.GetAnnotations()[parsers.QuotaOwnershipAnnotation]), &ownership); err != nil {
					t.Fatal(err)
				}
				ownership.SetSource(source)
				ownership.SetPhase(phase)
				ownership.SetRevision(int64(operation*3 + step + 1))
				encoded, _ := json.Marshal(ownership)
				obj.GetAnnotations()[parsers.QuotaOwnershipAnnotation] = string(encoded)
				if err := c.Update(context.Background(), obj); err != nil {
					t.Fatal(err)
				}
				var err error
				if department {
					err = NewDepartmentHandler(b, c).HandleUpdateItem(client.ObjectKeyFromObject(obj))
				} else {
					err = NewProjectHandler(b, c).HandleUpdateItem(client.ObjectKeyFromObject(obj))
				}
				if err != nil {
					t.Fatal(err)
				}
				if uid != "uid" || snapshot == nil || snapshot.GetSource() != source || snapshot.GetControlPhase() != phase || snapshot.GetControlRevision() != ownership.GetRevision() {
					t.Fatalf("lost trusted control snapshot: %+v UID %s", snapshot, uid)
				}
				if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
					t.Fatal(err)
				}
				var ack parsers.QuotaAck
				if err := json.Unmarshal([]byte(obj.GetAnnotations()[parsers.QuotaAckAnnotation]), &ack); err != nil {
					t.Fatal(err)
				}
				if ack.ControlRevision == nil || *ack.ControlRevision != ownership.GetRevision() || ack.ControlPhase == nil || *ack.ControlPhase != phase || ack.Source == nil || *ack.Source != source {
					t.Fatalf("wrong native ACK: %+v", ack)
				}
			}
		}
		server.Close()
	}
}

func TestQuotaAckCannotReplaceNewerObject(t *testing.T) {
	for _, department := range []bool{false, true} {
		c, obj := quotaFixture(t, department)
		stale := obj.DeepCopyObject().(client.Object)
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), stale); err != nil {
			t.Fatal(err)
		}
		current := stale.DeepCopyObject().(client.Object)
		current.SetGeneration(2)
		if err := c.Update(context.Background(), current); err != nil {
			t.Fatal(err)
		}
		if err := recordOrgUnitSync(c, stale, metav1.Now()); err == nil {
			t.Fatal("stale generation acknowledgment succeeded")
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), current); err != nil {
			t.Fatal(err)
		}
		if current.GetGeneration() != 2 || current.GetAnnotations()[parsers.QuotaAckAnnotation] != "" {
			t.Fatal("newer object overwritten")
		}
	}
}

func TestNativeQuotaAckCannotCrossPhase(t *testing.T) {
	for _, department := range []bool{false, true} {
		c, obj := quotaFixture(t, department)
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
			t.Fatal(err)
		}
		stale := obj.DeepCopyObject().(client.Object)
		var marker api.QuotaOwnership
		if err := json.Unmarshal([]byte(obj.GetAnnotations()[parsers.QuotaOwnershipAnnotation]), &marker); err != nil {
			t.Fatal(err)
		}
		marker.SetSource("runai")
		marker.SetPhase("freeze")
		marker.SetRevision(1)
		encoded, _ := json.Marshal(marker)
		obj.GetAnnotations()[parsers.QuotaOwnershipAnnotation] = string(encoded)
		if err := c.Update(context.Background(), obj); err != nil {
			t.Fatal(err)
		}
		if err := recordOrgUnitSync(c, stale, metav1.Now()); err == nil {
			t.Fatal("old phase ACK crossed metadata-only control update")
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
			t.Fatal(err)
		}
		if obj.GetAnnotations()[parsers.QuotaAckAnnotation] != "" || obj.GetGeneration() != stale.GetGeneration() {
			t.Fatal("ACK changed phase or generation")
		}
	}
}

type rawQuotaClient struct {
	client.Client
	object *unstructured.Unstructured
}

func (c rawQuotaClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if raw, ok := obj.(*unstructured.Unstructured); ok {
		*raw = *c.object.DeepCopy()
		return nil
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func TestQuotaRawCacheConsistency(t *testing.T) {
	c, obj := quotaFixture(t, false)
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
		t.Fatal(err)
	}
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatal(err)
	}
	raw := &unstructured.Unstructured{Object: data}
	reader := rawQuotaClient{Client: c, object: raw}
	if err := getQuotaOrgUnit(reader, client.ObjectKeyFromObject(obj), obj); err != nil {
		t.Fatal(err)
	}
	raw.SetResourceVersion("older-cache-version")
	if err := getQuotaOrgUnit(reader, client.ObjectKeyFromObject(obj), obj); err == nil || !strings.Contains(err.Error(), "caches differ") {
		t.Fatalf("stale raw cache accepted: %v", err)
	}
}

func TestQuotaRawLeavesRequired(t *testing.T) {
	for _, department := range []bool{false, true} {
		for _, resource := range []string{"gpu", "cpu", "memory"} {
			for _, leaf := range []string{"deserved", "limit", "overQuotaWeight"} {
				t.Run(resource+"/"+leaf, func(t *testing.T) {
					c, obj := quotaFixture(t, department)
					if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
						t.Fatal(err)
					}
					raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
					if err != nil {
						t.Fatal(err)
					}
					kinds, _, err := c.Scheme().ObjectKinds(obj)
					if err != nil {
						t.Fatal(err)
					}
					u := &unstructured.Unstructured{Object: raw}
					u.SetGroupVersionKind(kinds[0])
					queues := raw["spec"].(map[string]any)["queues"].([]any)
					delete(queues[0].(map[string]any)["resources"].(map[string]any)[resource].(map[string]any), leaf)
					rawClient := rawQuotaClient{Client: c, object: u}
					if err := getQuotaOrgUnit(rawClient, client.ObjectKeyFromObject(obj), obj); err == nil || strings.Contains(err.Error(), "caches differ") {
						t.Fatalf("omitted numeric leaf not rejected correctly: %v", err)
					}
				})
			}
		}
	}
}

func TestQuotaDeletionAndLegacyOmitSnapshot(t *testing.T) {
	_, obj := quotaFixture(t, false)
	p := obj.(*kai.Project)
	p.Status.Phase = parsers.DeletingPhase
	if snapshot, err := quotaSnapshot(p); err != nil || snapshot != nil {
		t.Fatalf("deletion snapshot %+v %v", snapshot, err)
	}
	if parsers.ParseKaiProjectToDeleteRequest(p).QuotaSnapshot.IsSet() {
		t.Fatal("delete request has snapshot")
	}
	p.Status.Phase = ""
	delete(p.Annotations, parsers.QuotaOwnershipAnnotation)
	if snapshot, err := quotaSnapshot(p); err != nil || snapshot != nil {
		t.Fatalf("legacy snapshot %+v %v", snapshot, err)
	}
	if err := quotaHTTPError(p, &http.Response{StatusCode: 500}, nil); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("bad error %v", err)
	}
}

func TestQuotaFullResyncQueuesOnlyEnrolledUnits(t *testing.T) {
	for _, department := range []bool{false, true} {
		c, obj := quotaFixture(t, department)
		legacy := obj.DeepCopyObject().(client.Object)
		legacy.SetName("legacy")
		legacy.SetResourceVersion("")
		legacy.SetAnnotations(nil)
		if err := c.Create(context.Background(), legacy); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
		defer server.Close()
		b := quotaBackend(server.URL)
		var queued []client.ObjectKey
		enqueue := func(key client.ObjectKey) { queued = append(queued, key) }
		wg := &sync.WaitGroup{}
		wg.Add(1)
		if department {
			h := NewDepartmentHandler(b, c)
			h.EnqueueQuotaSync = enqueue
			h.HandleSyncEvent(wg)
		} else {
			h := NewProjectHandler(b, c)
			h.EnqueueQuotaSync = enqueue
			h.HandleSyncEvent(wg)
		}
		wg.Wait()
		if len(queued) != 1 || queued[0].Name != obj.GetName() {
			t.Fatalf("bad resync queue %+v", queued)
		}
	}
}
