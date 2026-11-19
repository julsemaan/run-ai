// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

func TestQuotaEnrollmentConfiguration(t *testing.T) {
	for _, raw := range []string{"", "[]"} {
		entries, err := ParseQuotaEnrollments(raw)
		if err != nil || len(entries) != 0 {
			t.Fatalf("default enrollment: %v %v", entries, err)
		}
	}
	valid := `[{"tenantId":"1001","clusterId":"eff41c05-6b5c-4bd3-bf0c-c1087dabc48a","kind":"project","id":4500008,"kubernetesUid":"80104648-a9e6-49ed-8c95-c55d33fe18af","epoch":"epoch-1","state":"prepared","pools":[{"nodePoolId":"100001","nodePoolName":"a100","queueName":"quota-poc-a100"}]}]`
	entries, err := ParseQuotaEnrollments(valid)
	if err != nil || len(entries) != 1 {
		t.Fatalf("valid enrollment: %v", err)
	}
	for _, raw := range []string{`[{}]`, `[null]`, valid[:len(valid)-1] + "," + valid[1:], `[{"state":"active"}]`, `{"unexpected":true}`} {
		if _, err := ParseQuotaEnrollments(raw); err == nil {
			t.Fatalf("accepted invalid configuration %s", raw)
		}
	}
}
