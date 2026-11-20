-- SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
-- SPDX-License-Identifier: Apache-2.0

CREATE TABLE quota_control (
 tenant_id text NOT NULL,
 cluster_id uuid NOT NULL,
 kind text NOT NULL CHECK (kind IN ('project','department')),
 org_unit_id bigint NOT NULL,
 kubernetes_uid text NOT NULL DEFAULT '',
 source text NOT NULL DEFAULT 'runai' CHECK (source IN ('runai','kubernetes')),
 target_source text NOT NULL DEFAULT 'runai' CHECK (target_source IN ('runai','kubernetes')),
 phase text NOT NULL DEFAULT 'stable' CHECK (phase IN ('freeze','release','stable')),
 revision bigint NOT NULL DEFAULT 0,
 observed_revision bigint NOT NULL DEFAULT 0,
 epoch text NOT NULL DEFAULT '',
 pools jsonb NOT NULL DEFAULT '[]',
 legacy_state text NOT NULL DEFAULT '',
 message text NOT NULL DEFAULT '',
 last_synced_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,cluster_id,kind,org_unit_id)
);
