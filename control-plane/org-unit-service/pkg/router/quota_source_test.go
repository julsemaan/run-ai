// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQuotaSourceRequestPresence(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"zero revision", `{"source":"kubernetes","expectedRevision":0}`, 204},
		{"missing revision", `{"source":"kubernetes"}`, 400},
		{"null revision", `{"source":"runai","expectedRevision":null}`, 400},
		{"negative revision", `{"source":"runai","expectedRevision":-1}`, 400},
		{"invalid source", `{"source":"Kubernetes","expectedRevision":0}`, 400},
		{"malformed", `{`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := validateQuotaSourceRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(b) != tc.body {
					t.Fatalf("body changed: %s", b)
				}
				w.WriteHeader(204)
			}))
			request := httptest.NewRequest(http.MethodPut, "/api/v1/org-unit/projects/1/quota-source", strings.NewReader(tc.body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d, expected %d: %s", response.Code, tc.status, response.Body.String())
			}
		})
	}
}
