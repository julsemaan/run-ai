/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package validator

import (
	"encoding/json"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/agent/pkg/quotaownership"
	"k8s.io/apimachinery/pkg/runtime"
	"testing"
)

func TestNativeControlStatusFence(t *testing.T) {
	req := quotaRequest(t, func(map[string]interface{}) {})
	old := quotaFixture()
	before := old["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})
	legacy := before[quotaownership.Annotation].(string)
	before[quotaownership.Annotation] = legacy[:len(legacy)-1] + `,"source":"kubernetes","phase":"freeze","revision":4}`
	new := runtime.DeepCopyJSON(old)
	new["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})[quotaownership.Annotation] = legacy[:len(legacy)-1] + `,"source":"kubernetes","phase":"freeze","revision":3}`
	req.OldObject.Raw, _ = json.Marshal(old)
	req.Object.Raw, _ = json.Marshal(new)
	req.SubResource = "status"
	if err := quotaControlUpdate(req, true); err == nil {
		t.Fatal("trusted status update regressed the protected marker")
	}
}

func TestNativeRunaiMembershipRefreshAdmission(t *testing.T) {
	for _, kind := range []string{"Project", "Department"} {
		t.Run(kind, func(t *testing.T) {
			req := quotaRequest(t, func(map[string]interface{}) {})
			req.Kind.Kind = kind
			old := quotaFixture()
			old["kind"] = kind
			marker := `{"epoch":"epoch","kubernetesUid":"uid","source":"runai","phase":"stable","revision":6,"pools":[{"nodePoolId":"1","nodePoolName":"pool","queueName":"queue"}]}`
			old["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})[quotaownership.Annotation] = marker
			new := runtime.DeepCopyJSON(old)
			queues := new["spec"].(map[string]interface{})["queues"].([]interface{})
			extra := runtime.DeepCopyJSON(queues[0].(map[string]interface{}))
			extra["name"], extra["nodepool"] = "queue-other", "other"
			new["spec"].(map[string]interface{})["queues"] = append(queues, extra)
			new["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})[quotaownership.Annotation] = `{"epoch":"epoch","kubernetesUid":"uid","source":"runai","phase":"stable","revision":7,"pools":[{"nodePoolId":"1","nodePoolName":"pool","queueName":"queue"},{"nodePoolId":"2","nodePoolName":"other","queueName":"queue-other"}]}`
			req.OldObject.Raw, _ = json.Marshal(old)
			req.Object.Raw, _ = json.Marshal(new)
			if err := quotaControlUpdate(req, true); err != nil {
				t.Fatal(err)
			}
			req.OldObject, req.Object = req.Object, req.OldObject
			if err := quotaControlUpdate(req, true); err == nil {
				t.Fatal("delayed pre-refresh command admitted")
			}
		})
	}
}

func TestNativeControlAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, source, phase  string
		agent, change, allow bool
	}{
		{"freeze agent numeric", "kubernetes", "freeze", true, true, false},
		{"freeze editor numeric", "kubernetes", "freeze", false, true, false},
		{"release agent numeric", "runai", "release", true, true, false},
		{"release metadata", "runai", "release", false, false, true},
		{"stable Runai editor", "runai", "stable", false, true, false},
		{"stable Runai agent", "runai", "stable", true, true, true},
		{"stable Kubernetes editor", "kubernetes", "stable", false, true, true},
		{"stable Kubernetes stale agent", "kubernetes", "stable", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := quotaRequest(t, func(map[string]interface{}) {})
			var old map[string]interface{}
			if err := json.Unmarshal(req.OldObject.Raw, &old); err != nil {
				t.Fatal(err)
			}
			annotations := old["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})
			var marker map[string]interface{}
			if err := json.Unmarshal([]byte(annotations[quotaownership.Annotation].(string)), &marker); err != nil {
				t.Fatal(err)
			}
			marker["source"] = tc.source
			marker["phase"] = tc.phase
			marker["revision"] = float64(4)
			encoded, _ := json.Marshal(marker)
			annotations[quotaownership.Annotation] = string(encoded)
			new := runtime.DeepCopyJSON(old)
			if tc.change {
				wireQueue(new["spec"].(map[string]interface{})["queues"].([]interface{}), "queue", "pool")["resources"].(map[string]interface{})["gpu"].(map[string]interface{})["deserved"] = 0.75
			}
			req.OldObject.Raw, _ = json.Marshal(old)
			req.Object.Raw, _ = json.Marshal(new)
			if err := quotaControlUpdate(req, tc.agent); (err == nil) != tc.allow {
				t.Fatalf("allow=%v err=%v", tc.allow, err)
			}
			if quotaOnly(req) && (tc.source != "kubernetes" || tc.phase != "stable") {
				t.Fatal("nonstable editor bypass")
			}
		})
	}
}
