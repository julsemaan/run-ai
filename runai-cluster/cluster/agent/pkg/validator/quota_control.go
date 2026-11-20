/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package validator

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/agent/pkg/quotaownership"
	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func quotaControlUpdate(req admission.Request, agent bool) error {
	if req.Operation != "UPDATE" || req.Kind.Group != "kai.resources" || (req.Kind.Kind != "Project" && req.Kind.Kind != "Department") {
		return nil
	}
	var old, new client.Object
	if req.Kind.Kind == "Project" {
		old = &kai.Project{}
		new = &kai.Project{}
	} else {
		old = &kai.Department{}
		new = &kai.Department{}
	}
	if err := json.Unmarshal(req.OldObject.Raw, old); err != nil {
		return err
	}
	if err := json.Unmarshal(req.Object.Raw, new); err != nil {
		return err
	}
	before, after := old.GetAnnotations()[quotaownership.Annotation], new.GetAnnotations()[quotaownership.Annotation]
	if before == "" && after == "" {
		return nil
	}
	if after == "" {
		return fmt.Errorf("quota control marker cannot be removed")
	}
	if _, err := quotaownership.Parse(after, string(old.GetUID())); err != nil {
		return err
	}
	next, err := quotaownership.ParseControl(after)
	if err != nil {
		return err
	}
	previous := quotaownership.Control{}
	if before != "" {
		if _, err := quotaownership.Parse(before, string(old.GetUID())); err != nil {
			return err
		}
		previous, err = quotaownership.ParseControl(before)
		if err != nil {
			return err
		}
	}
	if err := quotaownership.Fence(previous, next, before != ""); err != nil {
		return err
	}
	if req.SubResource != "" {
		if before != after {
			_, err := quotaownership.Desired(new, old)
			return err
		}
		return nil
	}
	if agent {
		desired, err := quotaownership.Desired(new, old)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(quotaNumbers(desired), quotaNumbers(new)) {
			return fmt.Errorf("agent cannot change sealed quota numbers")
		}
	} else if before != "" && (previous.Phase != "stable" || previous.Source == "runai") && !reflect.DeepEqual(quotaNumbers(old), quotaNumbers(new)) {
		return fmt.Errorf("quota numbers are controlled by Runai or frozen")
	}
	return nil
}

func quotaNumbers(o client.Object) map[string][9]float64 {
	result := map[string][9]float64{}
	for _, q := range quotaownership.Queues(o) {
		if q.Resources == nil {
			continue
		}
		g, c, m := q.Resources.GPU, q.Resources.CPU, q.Resources.Memory
		result[q.Name+"/"+q.Nodepool] = [9]float64{g.Deserved, g.Limit, g.OverQuotaWeight, c.Deserved, c.Limit, c.OverQuotaWeight, m.Deserved, m.Limit, m.OverQuotaWeight}
	}
	return result
}
