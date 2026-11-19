-- SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
-- SPDX-License-Identifier: Apache-2.0

CREATE TABLE quota_mirror (
    tenant_id text NOT NULL,
    cluster_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('project', 'department')),
    org_unit_id bigint NOT NULL,
    epoch text NOT NULL,
    kubernetes_uid uuid NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    ownership jsonb NOT NULL,
    snapshot jsonb NOT NULL,
    PRIMARY KEY (tenant_id, cluster_id, kind, org_unit_id, epoch)
);
