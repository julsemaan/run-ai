/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
import { describe, it, expect, vi, afterEach } from "vitest";
import { authenticatedAxios } from "@/services/infra/https.service/auth-axios.service";
import { orgUnitService } from "./org-unit.service";
import { defaultQuotaManagement } from "@/models/quota-management.model";

vi.mock("@/services/infra/https.service/auth-axios.service", () => ({ authenticatedAxios: { request: vi.fn() } }));
vi.mock("@/services/infra/https.service/http.service", () => ({
  httpService: { handleHttpError: (error: unknown) => error },
}));
afterEach(() => vi.restoreAllMocks());
describe("quota-source public HTTP contract", () => {
  it.each(["projects", "departments"] as const)(
    "uses same-origin authenticated GET/PUT for %s with only source and expectedRevision",
    async (kind) => {
      const request = vi.spyOn(authenticatedAxios, "request").mockResolvedValue({ data: defaultQuotaManagement });
      expect(await orgUnitService.getQuotaSource(kind, "unit-id")).toEqual(defaultQuotaManagement);
      expect(request.mock.calls[0][0]).toMatchObject({
        method: "GET",
        url: `${window.location.origin}/api/v1/org-unit/${kind}/unit-id/quota-source`,
      });
      await orgUnitService.updateQuotaSource(kind, "unit-id", "kubernetes", 19);
      const sent = request.mock.calls[1][0];
      expect(sent).toMatchObject({
        method: "PUT",
        url: `${window.location.origin}/api/v1/org-unit/${kind}/unit-id/quota-source`,
      });
      expect(JSON.parse(String(sent.data))).toEqual({ source: "kubernetes", expectedRevision: 19 });
    },
  );
});
