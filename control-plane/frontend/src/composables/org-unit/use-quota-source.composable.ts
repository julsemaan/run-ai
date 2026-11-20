/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useQuasar } from "quasar";
import {
  defaultQuotaManagement,
  getQuotaManagement,
  quotasLocked,
  type QuotaSource,
} from "@/models/quota-management.model";
import { orgUnitService } from "@/services/control-plane/org-unit.service/org-unit.service";
import { requestToLeave } from "@/services/infra/router.service/router.service";
import { ErrorAlert } from "@/utils/error-alert.util";

export function useQuotaSource(options: {
  kind: "projects" | "departments";
  unit: () => object & { id?: string };
  canUpdate: () => boolean;
  dirty: () => boolean;
  editState?: () => string;
  invalidateRefresh?: () => void;
  refresh: (discardUnsaved?: boolean, editState?: string) => void | Promise<void>;
}) {
  const $q = useQuasar();
  const management = ref(getQuotaManagement(options.unit()) ?? defaultQuotaManagement);
  const busy = ref(false);
  const locked = computed(
    () => !options.canUpdate() || busy.value || quotasLocked({ quotaManagement: management.value }),
  );
  let timer: ReturnType<typeof setTimeout> | undefined;
  let disposed = false;

  const notifyError = (error: unknown) =>
    $q.notify(new ErrorAlert({ generalMessage: "Failed to update quota source" }).getNotification(error));
  const poll = async () => {
    const unit = options.unit();
    const id = unit.id;
    if (!id || disposed || busy.value) return;
    busy.value = true;
    try {
      const previous = management.value;
      const next = await orgUnitService.getQuotaSource(options.kind, id);
      if (disposed || options.unit() !== unit) return;
      management.value = next;
      // Status refresh must not replace local quota or peer edits, or advance their revision fence.
      if (previous.syncStatus.phase === "pending" && next.syncStatus.phase !== "pending" && !options.dirty())
        await options.refresh();
    } catch (error) {
      if (!disposed) notifyError(error);
    } finally {
      busy.value = false;
      schedule();
    }
  };
  const schedule = () => {
    clearTimeout(timer);
    if (!disposed && management.value.syncStatus.phase === "pending") timer = setTimeout(poll, 3000);
  };
  watch(
    () => options.unit(),
    (unit) => {
      management.value = getQuotaManagement(unit) ?? defaultQuotaManagement;
      schedule();
    },
  );
  schedule();

  const canChange = () =>
    !disposed && options.canUpdate() && !busy.value && management.value.syncStatus.phase !== "pending";
  const changeSource = async (source: QuotaSource) => {
    const unit = options.unit();
    const id = unit.id;
    if (!id || !canChange()) return;
    if (options.dirty() && !(await requestToLeave())) return;
    if (!canChange()) return;
    const editState = options.editState?.();
    busy.value = true;
    options.invalidateRefresh?.();
    try {
      const next = await orgUnitService.updateQuotaSource(options.kind, id, source, management.value.revision);
      if (disposed || options.unit() !== unit) return;
      management.value = next;
      await options.refresh(true, editState);
    } catch (error) {
      if (!disposed) notifyError(error);
      // Never replay a conflicting handoff or overwrite a dirty form on failure.
      try {
        const next = await orgUnitService.getQuotaSource(options.kind, id);
        if (!disposed && options.unit() === unit) management.value = next;
      } catch (pollError) {
        if (!disposed) notifyError(pollError);
      }
    } finally {
      busy.value = false;
      schedule();
    }
  };
  const refresh = async () => {
    if (disposed || busy.value || (options.dirty() && !(await requestToLeave()))) return;
    if (disposed || busy.value) return;
    const editState = options.editState?.();
    busy.value = true;
    try {
      await options.refresh(true, editState);
    } finally {
      busy.value = false;
      schedule();
    }
  };
  onBeforeUnmount(() => {
    disposed = true;
    clearTimeout(timer);
  });
  return { management, busy, locked, changeSource, refresh };
}
