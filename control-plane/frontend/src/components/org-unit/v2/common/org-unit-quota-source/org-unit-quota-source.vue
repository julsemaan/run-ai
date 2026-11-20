<!--
  SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
  SPDX-License-Identifier: Apache-2.0
-->
<template>
  <section class="q-pa-md" aria-label="Quota management">
    <div class="row items-center q-gutter-md">
      <runai-select
        aid="quota-source-select"
        class="col-12 col-sm-3"
        label="Quota source"
        :model-value="management.source"
        :options="options"
        :disable="!canUpdate || busy || management.syncStatus.phase === 'pending'"
        emit-value
        @update:model-value="selectSource"
      />
      <div role="status" aria-live="polite">
        <div>
          Quota sync:
          <runai-status
            :options="{
              status: phaseLabel,
              displayAnimation: management.syncStatus.phase === 'pending',
              color:
                management.syncStatus.phase === 'error'
                  ? 'negative'
                  : management.syncStatus.phase === 'pending'
                    ? 'warning'
                    : 'success',
            }"
          />
        </div>
        <div v-if="management.syncStatus.phase === 'pending'">
          Requested source: {{ sourceLabel(management.requestedSource) }}
        </div>
        <div v-if="management.syncStatus.message">{{ management.syncStatus.message }}</div>
        <div v-if="management.syncStatus.lastSyncedAt">Last synced: {{ management.syncStatus.lastSyncedAt }}</div>
      </div>
    </div>
    <q-btn aid="quota-source-refresh" flat label="Refresh quotas" :disable="busy" @click="emit('refresh')" />
    <div class="text-caption q-mt-sm">Applies to this {{ entityType }} across all node pools.</div>
    <div v-if="locked" class="text-caption">
      Quota values and node pool membership are locked. Wait for pending synchronization to complete, or switch to Run:ai
      when synchronized.
    </div>
    <runai-base-dialog :model-value="target !== null" size="width-sm" @close="target = null">
      <template #header>Change quota source?</template>
      <template #body>
        Change {{ entityType }} "{{ name }}" to {{ target ? sourceLabel(target) : "" }} across all node pools? The latest
        effective quota values, including CPU, memory and weights, will be preserved. You can switch back after
        synchronization completes. Unsaved changes must be discarded or saved separately before switching.
      </template>
      <template #footer>
        <q-btn aid="quota-source-cancel" flat label="Cancel" @click="target = null" />
        <q-btn
          aid="quota-source-confirm"
          color="primary"
          label="Change source"
          :disable="!canUpdate || busy || management.syncStatus.phase === 'pending'"
          @click="confirm"
        />
      </template>
    </runai-base-dialog>
  </section>
</template>
<script setup lang="ts">
import { computed, ref } from "vue";
import { RunaiStatus } from "@/components/common/runai-status";
import { RunaiSelect } from "@/components/common/runai-select";
import { RunaiBaseDialog } from "@/components/common/runai-base-dialog";
import { quotasLocked, type QuotaManagement, type QuotaSource } from "@/models/quota-management.model";

export interface Props {
  management: QuotaManagement;
  entityType: "project" | "department";
  name: string;
  canUpdate: boolean;
  busy?: boolean;
}
const props = defineProps<Props>();
const emit = defineEmits<{ (e: "change-source", source: QuotaSource): void; (e: "refresh"): void }>();
const target = ref<QuotaSource | null>(null);
const options = [
  { label: "Run:ai", value: "runai" },
  { label: "Kubernetes", value: "kubernetes" },
];
const sourceLabel = (source: QuotaSource) => (source === "kubernetes" ? "Kubernetes" : "Run:ai");
const phaseLabel = computed(
  () => ({ pending: "Pending", synced: "Synced", error: "Error" })[props.management.syncStatus.phase],
);
const locked = computed(() => quotasLocked({ quotaManagement: props.management }));
const selectSource = (source: QuotaSource) => {
  if (
    !props.canUpdate ||
    props.busy ||
    props.management.syncStatus.phase === "pending" ||
    source === props.management.source
  )
    return;
  target.value = source;
};
const confirm = () => {
  if (!target.value || !props.canUpdate || props.busy || props.management.syncStatus.phase === "pending") return;
  emit("change-source", target.value);
  target.value = null;
};
</script>
