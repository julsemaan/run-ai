/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package quotaownership

import (
	"encoding/json"
	"fmt"
	api "github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/client/generated"
	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const Annotation = "run.ai/quota-ownership"

func Set(object client.Object, ownership *api.QuotaOwnership) error {
	if ownership == nil {
		return nil
	}
	data, err := json.Marshal(ownership)
	if err != nil {
		return fmt.Errorf("encode quota ownership: %w", err)
	}
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[Annotation] = string(data)
	object.SetAnnotations(annotations)
	return nil
}

func Parse(value, uid string) (*api.QuotaOwnership, error) {
	var o api.QuotaOwnership
	if err := json.Unmarshal([]byte(value), &o); err != nil {
		return nil, fmt.Errorf("decode quota ownership: %w", err)
	}
	if uid == "" || o.KubernetesUid != uid || o.Epoch == "" || len(o.Pools) == 0 {
		return nil, fmt.Errorf("quota ownership identity mismatch")
	}
	ids, names, queues := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, p := range o.Pools {
		if p.NodePoolId == "" || p.NodePoolName == "" || p.QueueName == "" || ids[p.NodePoolId] || names[p.NodePoolName] || queues[p.QueueName] {
			return nil, fmt.Errorf("invalid quota ownership pool")
		}
		ids[p.NodePoolId] = true
		names[p.NodePoolName] = true
		queues[p.QueueName] = true
	}
	return &o, nil
}

func IsOrgUnit(object client.Object) bool {
	switch object.(type) {
	case *kai.Project, *kai.Department:
		return true
	}
	return false
}

func Queues(object client.Object) []kai.QueueConfig {
	switch o := object.(type) {
	case *kai.Project:
		return o.Spec.Queues
	case *kai.Department:
		return o.Spec.Queues
	}
	return nil
}

func Find(queues []kai.QueueConfig, name, pool string) (*kai.QueueConfig, error) {
	var found *kai.QueueConfig
	for i := range queues {
		if queues[i].Name == name && queues[i].Nodepool == pool {
			if found != nil {
				return nil, fmt.Errorf("duplicate enrolled queue %s", name)
			}
			found = &queues[i]
		}
	}
	if found == nil || found.Resources == nil {
		return nil, fmt.Errorf("missing enrolled queue resources %s", name)
	}
	return found, nil
}

// Desired uses only backend-provided ownership, never the live annotation, to select preserved leaves.
func Desired(backend, latest client.Object) (client.Object, error) {
	value := backend.GetAnnotations()[Annotation]
	if !IsOrgUnit(backend) {
		return backend, nil
	}
	if value == "" {
		// Enrollment removal is not a handoff. Keep live ownership until the
		// disposable object is deleted, even if its latest quota is not mirrored.
		if _, err := Parse(latest.GetAnnotations()[Annotation], string(latest.GetUID())); err == nil {
			return nil, fmt.Errorf("backend enrollment missing for Kubernetes-owned object %s", latest.GetName())
		}
		return backend, nil
	}
	o, err := Parse(value, string(latest.GetUID()))
	if err != nil {
		return nil, err
	}
	desired := backend.DeepCopyObject().(client.Object)
	for _, p := range o.Pools {
		source, err := Find(Queues(latest), p.QueueName, p.NodePoolName)
		if err != nil {
			return nil, err
		}
		target, err := Find(Queues(desired), p.QueueName, p.NodePoolName)
		if err != nil {
			return nil, err
		}
		*target.Resources = *source.Resources
	}
	return desired, nil
}
