/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package validator

import (
	"encoding/json"
	"math"
	"reflect"

	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/agent/pkg/quotaownership"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func ownershipAnnotation(raw []byte) string {
	var o unstructured.Unstructured
	if json.Unmarshal(raw, &o) != nil {
		return ""
	}
	return o.GetAnnotations()[quotaownership.Annotation]
}

// quotaOnly compares the entire wire object after replacing just the permitted numeric leaves.
// Raw maps deliberately retain omitted fields and unknown metadata/spec fields.
func quotaOnly(req admission.Request) bool {
	if req.Operation != admissionv1.Update || req.SubResource != "" || req.Kind.Group != "kai.resources" || (req.Kind.Kind != "Project" && req.Kind.Kind != "Department") {
		return false
	}
	var old, new unstructured.Unstructured
	if json.Unmarshal(req.OldObject.Raw, &old.Object) != nil || json.Unmarshal(req.Object.Raw, &new.Object) != nil {
		return false
	}
	ownership, err := quotaownership.Parse(old.GetAnnotations()[quotaownership.Annotation], string(old.GetUID()))
	if err != nil {
		return false
	}
	control, err := quotaownership.ParseControl(old.GetAnnotations()[quotaownership.Annotation])
	if err != nil || control.Source != "kubernetes" || control.Phase != "stable" {
		return false
	}
	oldQueues, _, err := unstructured.NestedSlice(old.Object, "spec", "queues")
	if err != nil {
		return false
	}
	newQueues, _, err := unstructured.NestedSlice(new.Object, "spec", "queues")
	if err != nil {
		return false
	}
	for _, pool := range ownership.Pools {
		a, b := wireQueue(oldQueues, pool.QueueName, pool.NodePoolName), wireQueue(newQueues, pool.QueueName, pool.NodePoolName)
		if a == nil || b == nil {
			return false
		}
		for _, resource := range []string{"gpu", "cpu", "memory"} {
			values := map[string]float64{}
			for _, leaf := range []string{"deserved", "limit", "overQuotaWeight"} {
				path := []string{"resources", resource, leaf}
				before, found, err := unstructured.NestedFloat64(a, path...)
				if err != nil || !found {
					return false
				}
				after, found, err := unstructured.NestedFloat64(b, path...)
				if err != nil || !found || math.IsNaN(after) || math.IsInf(after, 0) {
					return false
				}
				values[leaf] = after
				if err := unstructured.SetNestedField(b, before, path...); err != nil {
					return false
				}
			}
			d, l, w := values["deserved"], values["limit"], values["overQuotaWeight"]
			if (d < 0 && d != -1) || (l < 0 && l != -1) || w < 0 || (l != -1 && (d == -1 || d > l)) {
				return false
			}
		}
	}
	if err := unstructured.SetNestedSlice(new.Object, newQueues, "spec", "queues"); err != nil {
		return false
	}
	// The apiserver changes managedFields and increments generation before admission.
	// managedFields records field-manager bookkeeping, not user metadata or quota authority.
	old.SetManagedFields(nil)
	new.SetManagedFields(nil)
	// Keep every other metadata field in the comparison.
	oldGeneration, oldFound, err := unstructured.NestedFloat64(old.Object, "metadata", "generation")
	if err != nil {
		return false
	}
	newGeneration, newFound, err := unstructured.NestedFloat64(new.Object, "metadata", "generation")
	if err != nil || oldFound != newFound {
		return false
	}
	if oldFound {
		if oldGeneration < 1 || (newGeneration != oldGeneration && newGeneration != oldGeneration+1) {
			return false
		}
		if err := unstructured.SetNestedField(new.Object, oldGeneration, "metadata", "generation"); err != nil {
			return false
		}
	}
	return reflect.DeepEqual(old.Object, new.Object)
}

func wireQueue(queues []interface{}, name, pool string) map[string]interface{} {
	var found map[string]interface{}
	for _, q := range queues {
		m, ok := q.(map[string]interface{})
		if !ok {
			return nil
		}
		if m["name"] == name && m["nodepool"] == pool {
			if found != nil {
				return nil
			}
			found = m
		}
	}
	return found
}
