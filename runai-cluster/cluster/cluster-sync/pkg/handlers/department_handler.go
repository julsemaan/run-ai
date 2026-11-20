/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

package handlers

import (
	"context"
	"fmt"
	"sync"

	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/constants"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/handlers/parsers"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/metrics"
	clusterconstants "github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/common/constants"
	"go.uber.org/zap"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runaiBackendClient "github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/backend-client"
	kaiv1alpha1 "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
)

type DepartmentHandler struct {
	resourceName     constants.ClusterResource
	backendClient    *runaiBackendClient.BackendClient
	cachedClient     client.Client
	EnqueueQuotaSync func(client.ObjectKey)
}

func NewDepartmentHandler(runaiBackendClient *runaiBackendClient.BackendClient, cachedClient client.Client) *DepartmentHandler {
	return &DepartmentHandler{
		resourceName:  constants.DepartmentResourceName,
		backendClient: runaiBackendClient,
		cachedClient:  cachedClient,
	}
}

func (departmentHandler *DepartmentHandler) HandleUpdateItem(key client.ObjectKey) error {
	department := &kaiv1alpha1.Department{}
	err := getQuotaOrgUnit(departmentHandler.cachedClient, key, department)
	if err != nil {
		return err
	}

	return departmentHandler.HandleAddUpdate(department)
}

func (departmentHandler *DepartmentHandler) HandleAddUpdate(department *kaiv1alpha1.Department) error {
	syncTime := metav1.Now()
	updateStatusErr := departmentHandler.updateStatus(department)
	if updateStatusErr == nil {
		err := departmentHandler.updateSyncStatus(department, syncTime)
		if err != nil {
			if _, enrolled := department.Annotations[parsers.QuotaOwnershipAnnotation]; enrolled {
				return fmt.Errorf("record department %s synchronization: %w", department.Name, err)
			}
			zap.S().Warnf("Failed to update last sync time in department %s, err: %v", department.Name, err)
		}
	}
	return updateStatusErr
}

// updateSyncStatus records the last sync time as an annotation on the department.
// On the new kai.resources Department CRD the sync timestamp is no longer a status
// field; it is kept as an annotation managed exclusively by cluster-sync.
func (departmentHandler *DepartmentHandler) updateSyncStatus(department *kaiv1alpha1.Department, syncTime metav1.Time) error {
	return recordOrgUnitSync(departmentHandler.cachedClient, department, syncTime)
}

func (departmentHandler *DepartmentHandler) updateStatus(department *kaiv1alpha1.Department) error {
	defer metrics.GetMetricsExporter().MeasureExecutionTime(departmentHandler.resourceName, constants.UpdateOperation)()
	toUpdate := parsers.ParseKaiDepartmentToUpdateStatusRequest(department)
	snapshot, err := quotaSnapshot(department)
	if err != nil {
		return err
	}
	if snapshot != nil {
		toUpdate.SetQuotaSnapshot(*snapshot)
	}
	res, err := departmentHandler.backendClient.OrgUnitClient.UpdateDepartmentsStatus(
		departmentHandler.backendClient.Context(),
		department.Labels[clusterconstants.SyncedDepartmentIdLabel],
		toUpdate,
	)
	if res != nil {
		_ = res.Body.Close()
	}
	if commitErr := quotaHTTPError(department, res, err); commitErr != nil {
		return commitErr
	}
	return departmentHandler.backendClient.HandleHttpResponse(res, err, fmt.Sprintf("Updated status of department %s", department.Name))
}

func (departmentHandler *DepartmentHandler) HandleSyncEvent(waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()
	departments := &kaiv1alpha1.DepartmentList{}
	if err := departmentHandler.cachedClient.List(context.Background(), departments); err != nil {
		zap.S().Errorf("List departments for quota resync: %v", err)
		return
	}
	for _, department := range departments.Items {
		if _, enrolled := department.Annotations[parsers.QuotaOwnershipAnnotation]; enrolled && departmentHandler.EnqueueQuotaSync != nil {
			departmentHandler.EnqueueQuotaSync(client.ObjectKeyFromObject(&department))
		}
	}
}

func (departmentHandler *DepartmentHandler) HandleDelete(department *kaiv1alpha1.Department) {
	zap.S().Warnf("got delete on department name %s, id: %s", department.Name, department.Labels[clusterconstants.SyncedDepartmentIdLabel])
}
