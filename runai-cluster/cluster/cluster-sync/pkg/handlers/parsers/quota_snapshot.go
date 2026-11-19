/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

package parsers

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	api "github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/client/generated"
	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const QuotaOwnershipAnnotation = "run.ai/quota-ownership"
const QuotaAckAnnotation = "run.ai/quota-ack"

type QuotaAck struct {
	Epoch         string `json:"epoch"`
	KubernetesUID string `json:"kubernetesUid"`
	Generation    int64  `json:"generation"`
}

// ParseQuotaSnapshot trusts annotation provenance to admission, not to the payload.
// The backend independently checks enrollment and authenticated cluster scope.
func ParseQuotaSnapshot(object metav1.Object, queues []kai.QueueConfig) (*api.QuotaSnapshot, error) {
	raw, present := object.GetAnnotations()[QuotaOwnershipAnnotation]
	if !present {
		return nil, nil
	}
	invalid := func(reason string) (*api.QuotaSnapshot, error) {
		return nil, fmt.Errorf("quota snapshot %s generation %d: %s", object.GetName(), object.GetGeneration(), reason)
	}
	// Token validation rejects duplicate keys, which encoding/json otherwise accepts.
	decoder := json.NewDecoder(strings.NewReader(raw))
	if err := uniqueJSONValue(decoder); err != nil {
		return invalid("invalid ownership JSON")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return invalid("trailing ownership JSON")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &fields) != nil || len(fields) != 3 || fields["epoch"] == nil || fields["kubernetesUid"] == nil || fields["pools"] == nil {
		return invalid("ownership requires exact epoch, kubernetesUid and pools fields")
	}
	var pools []map[string]json.RawMessage
	if json.Unmarshal(fields["pools"], &pools) != nil {
		return invalid("invalid ownership pools")
	}
	for _, pool := range pools {
		if len(pool) != 3 || pool["nodePoolId"] == nil || pool["nodePoolName"] == nil || pool["queueName"] == nil {
			return invalid("ownership requires exact pool identity fields")
		}
	}
	decoder = json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	// Use plain structs because generated UnmarshalJSON can accept unknown fields.
	var ownership struct {
		Epoch         string `json:"epoch"`
		KubernetesUID string `json:"kubernetesUid"`
		Pools         []struct {
			NodePoolID   string `json:"nodePoolId"`
			NodePoolName string `json:"nodePoolName"`
			QueueName    string `json:"queueName"`
		} `json:"pools"`
	}
	if err := decoder.Decode(&ownership); err != nil {
		return invalid("invalid ownership fields")
	}
	if ownership.Epoch == "" || ownership.KubernetesUID == "" || ownership.KubernetesUID != string(object.GetUID()) || object.GetGeneration() <= 0 || len(ownership.Pools) == 0 {
		return invalid("ownership UID, epoch, generation or pools invalid")
	}
	snapshot := &api.QuotaSnapshot{Epoch: ownership.Epoch, KubernetesUid: ownership.KubernetesUID, Generation: object.GetGeneration()}
	ids, names, queueNames := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, pool := range ownership.Pools {
		if pool.NodePoolID == "" || pool.NodePoolName == "" || pool.QueueName == "" || ids[pool.NodePoolID] || names[pool.NodePoolName] || queueNames[pool.QueueName] {
			return invalid("empty or duplicate pool identity")
		}
		ids[pool.NodePoolID] = true
		names[pool.NodePoolName] = true
		queueNames[pool.QueueName] = true
		var matched *kai.QueueConfig
		for i := range queues {
			q := &queues[i]
			if q.Name == pool.QueueName || q.Nodepool == pool.NodePoolName {
				if matched != nil || q.Name != pool.QueueName || q.Nodepool != pool.NodePoolName {
					return invalid("ambiguous queue identity")
				}
				matched = q
			}
		}
		if matched == nil || matched.Resources == nil {
			return invalid("missing enrolled queue or resources")
		}
		resources := matched.Resources
		gpu, err := snapshotResource(resources.GPU)
		if err != nil {
			return invalid("invalid GPU quota")
		}
		cpu, err := snapshotResource(resources.CPU)
		if err != nil {
			return invalid("invalid CPU quota")
		}
		memory, err := snapshotResource(resources.Memory)
		if err != nil {
			return invalid("invalid memory quota")
		}
		snapshot.Pools = append(snapshot.Pools, api.QuotaSnapshotPool{NodePoolId: pool.NodePoolID, Gpu: gpu, Cpu: cpu, Memory: memory})
	}
	return snapshot, nil
}

func snapshotResource(r kai.SystemResource) (api.QuotaSnapshotResource, error) {
	result := api.QuotaSnapshotResource{}
	for _, v := range []float64{r.Deserved, r.Limit, r.OverQuotaWeight} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return result, fmt.Errorf("nonfinite quota")
		}
	}
	if (r.Deserved < 0 && r.Deserved != -1) || (r.Limit < 0 && r.Limit != -1) || r.OverQuotaWeight < 0 || (r.Limit != -1 && (r.Deserved == -1 || r.Limit < r.Deserved)) {
		return result, fmt.Errorf("invalid quota range")
	}
	result.SetDeserved(r.Deserved)
	result.SetLimit(r.Limit)
	result.SetOverQuotaWeight(r.OverQuotaWeight)
	return result, nil
}

func uniqueJSONValue(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	keys := map[string]bool{}
	for d.More() {
		if delimiter == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return fmt.Errorf("duplicate JSON key")
			}
			keys[name] = true
		}
		if err := uniqueJSONValue(d); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}
