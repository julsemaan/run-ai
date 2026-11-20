/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	api "github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/client/generated"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/handlers/parsers"
	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Read raw enrolled objects as well: the typed API uses float64 and loses the
// distinction between omitted numeric leaves and explicit zero values.
func getQuotaOrgUnit(c client.Client, key client.ObjectKey, object client.Object) error {
	ctx := context.Background()
	if err := c.Get(ctx, key, object); err != nil {
		return err
	}
	if _, present := object.GetAnnotations()[parsers.QuotaOwnershipAnnotation]; !present {
		return nil
	}
	kinds, _, err := c.Scheme().ObjectKinds(object)
	if err != nil {
		return err
	}
	raw := &unstructured.Unstructured{}
	raw.SetGroupVersionKind(kinds[0])
	if err := c.Get(ctx, key, raw); err != nil {
		return err
	}
	// Typed and raw informers advance independently. Retry rather than uploading
	// an older raw version after the typed watch delivered a newer quota patch.
	if raw.GetResourceVersion() != object.GetResourceVersion() {
		return fmt.Errorf("quota snapshot %s generation %d: object caches differ, retrying", key.Name, object.GetGeneration())
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw.Object, object); err != nil {
		return fmt.Errorf("quota snapshot %s: invalid object", key.Name)
	}
	snapshot, err := quotaSnapshot(object)
	if err != nil || snapshot == nil {
		return err
	}
	var ownership api.QuotaOwnership
	if err := json.Unmarshal([]byte(object.GetAnnotations()[parsers.QuotaOwnershipAnnotation]), &ownership); err != nil {
		return fmt.Errorf("quota snapshot %s: invalid ownership", key.Name)
	}
	queues, _, err := unstructured.NestedSlice(raw.Object, "spec", "queues")
	if err != nil {
		return fmt.Errorf("quota snapshot %s: invalid queues", key.Name)
	}
	for _, pool := range ownership.Pools {
		for _, item := range queues {
			q, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("quota snapshot %s: invalid queue", key.Name)
			}
			if q["name"] != pool.QueueName {
				continue
			}
			for _, resource := range []string{"gpu", "cpu", "memory"} {
				for _, leaf := range []string{"deserved", "limit", "overQuotaWeight"} {
					value, found, err := unstructured.NestedFieldNoCopy(q, "resources", resource, leaf)
					if err != nil || !found {
						return fmt.Errorf("quota snapshot %s generation %d: missing %s.%s", key.Name, object.GetGeneration(), resource, leaf)
					}
					switch value.(type) {
					case float64, int64:
					default:
						return fmt.Errorf("quota snapshot %s generation %d: nonnumeric %s.%s", key.Name, object.GetGeneration(), resource, leaf)
					}
				}
			}
		}
	}
	return nil
}

func quotaSnapshot(object client.Object) (*api.QuotaSnapshot, error) {
	switch obj := object.(type) {
	case *kai.Project:
		if obj.DeletionTimestamp != nil || string(obj.Status.Phase) == parsers.DeletingPhase {
			return nil, nil
		}
		return parsers.ParseQuotaSnapshot(obj, obj.Spec.Queues)
	case *kai.Department:
		return parsers.ParseQuotaSnapshot(obj, obj.Spec.Queues)
	default:
		return nil, fmt.Errorf("unsupported quota object")
	}
}

func recordOrgUnitSync(c client.Client, object client.Object, syncTime metav1.Time) error {
	snapshot, err := quotaSnapshot(object)
	if err != nil {
		return err
	}
	base := object.DeepCopyObject().(client.Object)
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[parsers.LastSyncTimestampAnnotation] = syncTime.UTC().Format(time.RFC3339)
	if snapshot != nil {
		ack, err := json.Marshal(parsers.QuotaAck{Epoch: snapshot.Epoch, KubernetesUID: snapshot.KubernetesUid, Generation: snapshot.Generation, ControlRevision: snapshot.ControlRevision, ControlPhase: snapshot.ControlPhase, Source: snapshot.Source})
		if err != nil {
			return err
		}
		annotations[parsers.QuotaAckAnnotation] = string(ack)
	}
	object.SetAnnotations(annotations)
	// A concurrent quota patch, recreation or ownership change must never receive
	// an acknowledgment for the object version we just uploaded.
	return c.Patch(context.Background(), object, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
}

func quotaHTTPError(object client.Object, res *http.Response, err error) error {
	if err == nil && res != nil && res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	code := 0
	if res != nil {
		code = res.StatusCode
	}
	// Do not include response bodies or transport errors which can contain secrets.
	return fmt.Errorf("quota snapshot %s UID %s generation %d: HTTP commit failed (status %d), retrying", object.GetName(), object.GetUID(), object.GetGeneration(), code)
}
