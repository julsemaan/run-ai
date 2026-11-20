/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
import { describe, it, expect } from "vitest";
import { orgUnitUtil } from "./org-unit.util";
import { defaultQuotaManagement } from "@/models/quota-management.model";

describe("quota write revision fences", () => {
  it.each(["kubernetes", "pending"] as const)("refuses to serialize %s peers in any numeric batch path", (state) => {
    const change = {
      orgUnitId: "peer",
      nodePoolId: "pool",
      nodePoolName: "default",
      gpuQuota: 2,
      originalLimit: null,
      originalOverQuotaWeight: 7,
      newRank: "High",
      originalRank: "Medium",
      newWeight: 9,
      originalWeight: 7,
      quotaManagement: {
        ...defaultQuotaManagement,
        source: state === "kubernetes" ? ("kubernetes" as const) : ("runai" as const),
        syncStatus: { phase: state === "pending" ? ("pending" as const) : ("synced" as const) },
      },
    };
    expect(() => orgUnitUtil.buildNodePoolBatchPatchRequest([change], false)).toThrow("locked");
    expect(() => orgUnitUtil.buildNodePoolOrgUnitChangesBatchPatchRequest([change])).toThrow("locked");
    expect(() => orgUnitUtil.buildNodePoolRankBatchPatchRequest([change])).toThrow("locked");
    expect(() => orgUnitUtil.buildNodePoolWeightBatchPatchRequest([change])).toThrow("locked");
  });
  it("carries the observed peer revision in GPU batch patches", () => {
    const request = orgUnitUtil.buildNodePoolBatchPatchRequest(
      [
        {
          orgUnitId: "peer",
          nodePoolId: "pool",
          nodePoolName: "default",
          gpuQuota: 2,
          originalLimit: null,
          originalOverQuotaWeight: 7,
          quotaManagementRevision: 42,
        },
      ],
      true,
    );
    expect(request.patchRequests[0]).toMatchObject({
      quotaManagementRevision: 42,
      resources: { gpu: { deserved: 2, overQuotaWeight: 7, limit: null } },
    });
  });
  it("carries the observed peer revision in combined rank and weight patches", () => {
    const request = orgUnitUtil.buildNodePoolOrgUnitChangesBatchPatchRequest([
      { orgUnitId: "peer", nodePoolId: "pool", newWeight: 9, newRank: "a", quotaManagementRevision: 12 },
    ]);
    expect(request.patchRequests[0]).toMatchObject({ quotaManagementRevision: 12, rank: "a" });
  });
});
