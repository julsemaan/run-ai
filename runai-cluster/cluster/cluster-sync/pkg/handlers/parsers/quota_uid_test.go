/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package parsers

import (
	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestQuotaTrustedUIDWithoutOwnership(t *testing.T) {
	meta := metav1.ObjectMeta{UID: "actual-kubernetes-uid", Labels: map[string]string{ProjectIdLabel: "database-id"}}
	p := &kai.Project{ObjectMeta: meta}
	d := &kai.Department{ObjectMeta: meta}
	project, department, deleted := ParseKaiProjectToUpdateStatusRequest(p), ParseKaiDepartmentToUpdateStatusRequest(d), ParseKaiProjectToDeleteRequest(p)
	if project.Status.GetKubernetesUid() != string(meta.UID) || department.Status.GetKubernetesUid() != string(meta.UID) || deleted.Status.GetKubernetesUid() != string(meta.UID) {
		t.Fatal("status UID did not come from actual CR")
	}
}
