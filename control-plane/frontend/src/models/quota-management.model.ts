/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

import type { QuotaManagement, QuotaSourceUpdateRequest } from "@/swagger-models/org-unit-service-client";
export type { QuotaManagement } from "@/swagger-models/org-unit-service-client";
export type QuotaSource = QuotaSourceUpdateRequest["source"];

export interface QuotaManaged {
  quotaManagement?: QuotaManagement;
}

export const defaultQuotaManagement: QuotaManagement = {
  source: "runai",
  requestedSource: "runai",
  revision: 0,
  canEditQuotas: true,
  syncStatus: { phase: "synced" },
};

export function getQuotaManagement(unit: object): QuotaManagement | undefined {
  return (unit as QuotaManaged).quotaManagement;
}

export function quotasLocked(unit: object): boolean {
  const management = getQuotaManagement(unit);
  return (
    !!management &&
    (management.source === "kubernetes" || !management.canEditQuotas || management.syncStatus.phase === "pending")
  );
}

export function quotaRevision(unit: object): { quotaManagementRevision?: number } {
  const management = getQuotaManagement(unit);
  return management ? { quotaManagementRevision: management.revision } : {};
}
