// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/generated/pkg/api"
	"github.com/dsx-ai-factory/run-ai/runai-common-packages/logging"
	"github.com/dsx-ai-factory/run-ai/runai-common-packages/runaictx"
)

// Generated scalar decoding cannot distinguish an omitted required integer from
// zero. Keep revision zero valid, but require its presence at the HTTP boundary.
func validateQuotaSourceRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, "/api/v1/org-unit/") || !strings.HasSuffix(r.URL.Path, "/quota-source") {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		var request struct {
			Source           string `json:"source"`
			ExpectedRevision *int64 `json:"expectedRevision"`
		}
		if err == nil {
			err = json.Unmarshal(body, &request)
		}
		if err != nil || request.ExpectedRevision == nil || *request.ExpectedRevision < 0 || (request.Source != "runai" && request.Source != "kubernetes") {
			status := http.StatusBadRequest
			if encodeErr := api.EncodeJSONResponse(api.NewError(status, "source and a non-negative expectedRevision are required", nil), &status, w); encodeErr != nil {
				logging.WithCtx(runaictx.LoggerHook(r.Context())).Errorf("write quota source validation error: %v", encodeErr)
			}
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}
