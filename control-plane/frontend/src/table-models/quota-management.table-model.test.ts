/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
import { describe, it, expect } from "vitest";
import { projectIndexColumns } from "./project.table-model/project.table-model";
import { departmentIndexColumns } from "./department.table-model/department.table-model";
import { quotaSourceColumn, quotaSyncColumn } from "./quota-management.table-model";
import { defaultQuotaManagement } from "@/models/quota-management.model";

describe("quota management list columns", () => {
  it.each([{ columns: projectIndexColumns }, { columns: departmentIndexColumns }])(
    "shows source and synchronization by default without sortable/filterable server fields",
    ({ columns }) => {
      for (const name of ["quotaSource", "quotaSync"])
        expect(columns.find((column) => column.name === name)).toMatchObject({
          display: true,
          sortable: false,
          hideFilter: true,
        });
    },
  );
  it("uses additive row metadata and defaults old records to Run:ai", () => {
    const source = quotaSourceColumn.field as (row: object) => string;
    const phase = quotaSyncColumn.field as (row: object) => string;
    expect(source({})).toBe("Run:ai");
    expect(phase({})).toBe("synced");
    expect(source({ quotaManagement: { ...defaultQuotaManagement, source: "kubernetes" } })).toBe("Kubernetes");
    expect(phase({ quotaManagement: { ...defaultQuotaManagement, syncStatus: { phase: "pending" } } })).toBe("pending");
  });
});
