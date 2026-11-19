/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

package controllers

import (
	"reflect"

	backendclient "github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/backend-client"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/cachedclient"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/handlers"
	"github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/cluster-sync/pkg/handlers/parsers"
	commonconstants "github.com/dsx-ai-factory/run-ai/runai-cluster/cluster/common/constants"
	kaiv1alpha1 "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
	"go.uber.org/zap"
	"k8s.io/client-go/tools/cache"
)

func AddDepartmentHandlerToCache(backendClient *backendclient.BackendClient, cachedClient cachedclient.CachedClient) {
	departmentHandler := handlers.NewDepartmentHandler(backendClient, cachedClient)

	rateLimitedController := cachedClient.NewRateLimitedController(cachedclient.DepartmentRL, departmentHandler)
	departmentHandler.EnqueueQuotaSync = rateLimitedController.AddRateLimited
	backendClient.RegisterSyncHandler(departmentHandler)

	departmentInformer := cachedClient.GetInformer(cachedclient.DepartmentType)
	_, err := departmentInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(object any) {
			department := object.(*kaiv1alpha1.Department)
			rateLimitedController.AddRateLimited(objectKeyFromObject(department))
		},
		UpdateFunc: func(oldObject any, newObject any) {
			if oldObject == nil {
				rateLimitedController.AddRateLimited(objectKeyFromObject(newObject.(*kaiv1alpha1.Department)))
				return
			}
			newDepartment := newObject.(*kaiv1alpha1.Department)
			shouldUpdateProj := shouldUpdateDepartment(oldObject.(*kaiv1alpha1.Department), newDepartment)
			if shouldUpdateProj {
				rateLimitedController.AddRateLimited(objectKeyFromObject(newDepartment))
			}
		},
		DeleteFunc: func(object any) {
			department, ok := deletedObject[*kaiv1alpha1.Department](object)
			if !ok {
				return
			}
			rateLimitedController.MarkItemDeleted(objectKeyFromObject(department))
			departmentHandler.HandleDelete(department)
		},
	})

	if err != nil {
		zap.S().Fatalf("Failed to add event handler to department informer, error: %s", err.Error())
	}

	zap.S().Debug("The Agent department controller has started")
}

func shouldUpdateDepartment(old, new *kaiv1alpha1.Department) bool {
	oldUpdatedAt := old.Annotations[commonconstants.UpdatedAtAnnotation]
	newUpdatedAt := new.Annotations[commonconstants.UpdatedAtAnnotation]
	oldOwnership, oldEnrolled := old.Annotations[parsers.QuotaOwnershipAnnotation]
	newOwnership, newEnrolled := new.Annotations[parsers.QuotaOwnershipAnnotation]
	if oldEnrolled || newEnrolled {
		return old.UID != new.UID || old.Generation != new.Generation || oldOwnership != newOwnership || oldEnrolled != newEnrolled || oldUpdatedAt != newUpdatedAt || !reflect.DeepEqual(old.Status, new.Status)
	}
	return oldUpdatedAt == "" || newUpdatedAt == "" || oldUpdatedAt != newUpdatedAt
}
