// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package departments

import (
	"context"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/generated/pkg/api"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/services"
)

func (s *DepartmentApiService) GetDepartmentQuotaSource(ctx context.Context, id string) (api.ImplResponse, error) {
	return services.QuotaSource(ctx, "department", id, nil, s.clusterClient)
}
func (s *DepartmentApiService) UpdateDepartmentQuotaSource(ctx context.Context, id string, request api.QuotaSourceUpdateRequest) (api.ImplResponse, error) {
	return services.QuotaSource(ctx, "department", id, &request, s.clusterClient)
}
