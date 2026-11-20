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
	// Revision zero must retain the old canonical wire shape during rollout.
	wire := *ownership
	if wire.GetRevision() == 0 {
		wire.Revision, wire.Source, wire.Phase = nil, nil, nil
	}
	data, err := json.Marshal(wire)
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

// Desired fences backend commands against the protected live marker before selecting numeric leaves.
func Desired(backend, latest client.Object) (client.Object, error) {
	value := backend.GetAnnotations()[Annotation]
	if !IsOrgUnit(backend) {
		return backend, nil
	}
	if value == "" {
		// A missing backend directive is not a handoff, including after return
		// to Runai or a UID mismatch. Never erase a live fence implicitly.
		if _, present := latest.GetAnnotations()[Annotation]; present {
			return nil, fmt.Errorf("backend enrollment missing for quota-controlled object %s", latest.GetName())
		}
		return backend, nil
	}
	o, err := Parse(value, string(latest.GetUID()))
	if err != nil {
		return nil, err
	}
	next, err := ParseControl(value)
	if err != nil {
		return nil, err
	}
	liveValue := latest.GetAnnotations()[Annotation]
	var live *api.QuotaOwnership
	previous := Control{}
	if liveValue != "" {
		live, err = Parse(liveValue, string(latest.GetUID()))
		if err != nil {
			return nil, err
		}
		previous, err = ParseControl(liveValue)
		if err != nil {
			return nil, err
		}
	}
	if err := Fence(previous, next, liveValue != ""); err != nil {
		return nil, err
	}
	runaiStable := live != nil && next.Phase == "stable" && next.Source == "runai" && previous.Phase == "stable" && previous.Source == "runai" && previous.Epoch == next.Epoch
	if live != nil && next.Revision > 0 && !runaiStable && (previous.Revision == next.Revision || previous.Epoch == next.Epoch) && !samePools(live.Pools, o.Pools) {
		return nil, fmt.Errorf("quota control pool bindings changed within operation")
	}
	if next.Revision > 0 {
		if (!runaiStable && len(o.Pools) != len(Queues(latest))) || len(o.Pools) != len(Queues(backend)) {
			return nil, fmt.Errorf("native quota control must cover all pools")
		}
		for _, p := range o.Pools {
			if !runaiStable {
				if _, err := Find(Queues(latest), p.QueueName, p.NodePoolName); err != nil {
					return nil, err
				}
			}
			if _, err := Find(Queues(backend), p.QueueName, p.NodePoolName); err != nil {
				return nil, err
			}
		}
	}
	desired := backend.DeepCopyObject().(client.Object)
	preserve := o.Pools
	if next.Revision > 0 {
		switch {
		case runaiStable:
			preserve = nil
		case next.Phase == "freeze" && (live == nil || previous.Epoch != next.Epoch):
			// Legacy normalization has mixed authority. Only its protected pools
			// retain live numbers; newly covered pools use frozen backend values.
			preserve = nil
			if live != nil && previous.Source != "runai" {
				preserve = live.Pools
			}
		}
	}
	for _, p := range preserve {
		if next.Revision > 0 && !containsPool(o.Pools, p) {
			return nil, fmt.Errorf("protected quota pool binding changed")
		}
		source, err := Find(Queues(latest), p.QueueName, p.NodePoolName)
		if err != nil {
			return nil, err
		}
		target, err := Find(Queues(desired), p.QueueName, p.NodePoolName)
		if err != nil {
			return nil, err
		}
		copyNumbers(&target.Resources.GPU, source.Resources.GPU)
		copyNumbers(&target.Resources.CPU, source.Resources.CPU)
		copyNumbers(&target.Resources.Memory, source.Resources.Memory)
	}
	return desired, nil
}

func samePools(a, b []api.QuotaOwnershipPool) bool {
	if len(a) != len(b) {
		return false
	}
	for _, pool := range a {
		if !containsPool(b, pool) {
			return false
		}
	}
	return true
}

func containsPool(pools []api.QuotaOwnershipPool, pool api.QuotaOwnershipPool) bool {
	for _, other := range pools {
		if pool == other {
			return true
		}
	}
	return false
}
