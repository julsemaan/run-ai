/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
import { defaultQuotaManagement, getQuotaManagement } from "@/models/quota-management.model";
import { ECustomCell, type ITableColumn, type IStatusColOptions } from "@/models/table.model";

export const quotaSourceColumn: ITableColumn = {
  name: "quotaSource",
  label: "Quota source",
  field: (row: object) =>
    (getQuotaManagement(row) ?? defaultQuotaManagement).source === "kubernetes" ? "Kubernetes" : "Run:ai",
  align: "left",
  sortable: false,
  hideFilter: true,
};

export const quotaSyncColumn: ITableColumn = {
  name: "quotaSync",
  label: "Quota sync",
  field: (row: object) => (getQuotaManagement(row) ?? defaultQuotaManagement).syncStatus.phase,
  format: (phase: string): IStatusColOptions => ({
    status: { pending: "Pending", synced: "Synced", error: "Error" }[phase] ?? "Unknown",
    displayAnimation: phase === "pending",
    color: phase === "error" ? "negative" : phase === "pending" ? "warning" : "success",
  }),
  exportFormat: (row: object) => (getQuotaManagement(row) ?? defaultQuotaManagement).syncStatus.phase,
  customCell: ECustomCell.STATUS_COL,
  align: "left",
  sortable: false,
  hideFilter: true,
};
