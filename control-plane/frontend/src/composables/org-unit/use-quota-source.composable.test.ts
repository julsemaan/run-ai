/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { defineComponent } from "vue";
import { useQuotaSource } from "./use-quota-source.composable";
import { defaultQuotaManagement } from "@/models/quota-management.model";
import { orgUnitService } from "@/services/control-plane/org-unit.service/org-unit.service";
import { requestToLeave } from "@/services/infra/router.service/router.service";

vi.mock("@/services/control-plane/org-unit.service/org-unit.service", () => ({
  orgUnitService: { getQuotaSource: vi.fn(), updateQuotaSource: vi.fn() },
}));
vi.mock("@/services/infra/router.service/router.service", () => ({ requestToLeave: vi.fn() }));
const notify = vi.hoisted(() => vi.fn());
vi.mock("quasar", () => ({ useQuasar: () => ({ notify }) }));

const setup = (dirty = false, pending = false, canUpdate = true) => {
  const refresh = vi.fn();
  const unit = {
    id: "project",
    quotaManagement: {
      ...defaultQuotaManagement,
      revision: 4,
      syncStatus: { phase: pending ? ("pending" as const) : ("synced" as const) },
    },
  };
  let source!: ReturnType<typeof useQuotaSource>;
  const wrapper = mount(
    defineComponent({
      setup() {
        source = useQuotaSource({
          kind: "projects",
          unit: () => unit,
          dirty: () => dirty,
          canUpdate: () => canUpdate,
          refresh,
        });
        return () => null;
      },
    }),
  );
  return { wrapper, source, refresh, unit };
};

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
});
afterEach(() => {
  vi.useRealTimers();
});
describe("quota source lifecycle", () => {
  it("resolves dirty flow before handoff without saving quota or peers", async () => {
    const { wrapper, source, refresh } = setup(true);
    vi.mocked(requestToLeave).mockResolvedValueOnce(false);
    await source.changeSource("kubernetes");
    expect(orgUnitService.updateQuotaSource).not.toHaveBeenCalled();
    vi.mocked(requestToLeave).mockResolvedValueOnce(true);
    vi.mocked(orgUnitService.updateQuotaSource).mockResolvedValueOnce({
      ...defaultQuotaManagement,
      syncStatus: { phase: "pending" },
    });
    await source.changeSource("kubernetes");
    expect(orgUnitService.updateQuotaSource).toHaveBeenCalledWith("projects", "project", "kubernetes", 4);
    expect(refresh).toHaveBeenCalledOnce();
    wrapper.unmount();
  });
  it("retains quotas and does not replay a failed handoff", async () => {
    const { wrapper, source, refresh, unit } = setup(true);
    vi.mocked(requestToLeave).mockResolvedValue(true);
    vi.mocked(orgUnitService.updateQuotaSource).mockRejectedValueOnce(new Error("Network Error"));
    vi.mocked(orgUnitService.getQuotaSource).mockResolvedValueOnce(unit.quotaManagement);
    await source.changeSource("kubernetes");
    expect(source.management.value.source).toBe("runai");
    expect(source.busy.value).toBe(false);
    expect(refresh).not.toHaveBeenCalled();
    expect(notify).toHaveBeenCalledOnce();
    expect(orgUnitService.updateQuotaSource).toHaveBeenCalledOnce();
    wrapper.unmount();
  });
  it.each([false, true])("polls pending status without clobbering dirty=%s and cleans up timer", async (dirty) => {
    const { wrapper, source, refresh, unit } = setup(dirty, true);
    vi.mocked(orgUnitService.getQuotaSource).mockResolvedValue({ ...defaultQuotaManagement, revision: 7 });
    await vi.advanceTimersByTimeAsync(3000);
    await flushPromises();
    expect(source.management.value.revision).toBe(7);
    expect(unit.quotaManagement.revision).toBe(4);
    expect(refresh).toHaveBeenCalledTimes(dirty ? 0 : 1);
    wrapper.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
  it("uses the existing discard flow for manual refresh without sending writes", async () => {
    const { wrapper, source, refresh } = setup(true);
    vi.mocked(requestToLeave).mockResolvedValueOnce(false);
    await source.refresh();
    expect(refresh).not.toHaveBeenCalled();
    vi.mocked(requestToLeave).mockResolvedValueOnce(true);
    await source.refresh();
    expect(refresh).toHaveBeenCalledWith(true, undefined);
    expect(orgUnitService.updateQuotaSource).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it.each(["manual", "handoff", "poll"])(
    "serializes handoff against an awaited %s object refresh",
    async (operation) => {
      const { wrapper, source, refresh } = setup(false, operation === "poll");
      let resolveRefresh!: () => void;
      refresh.mockReturnValueOnce(
        new Promise<void>((resolve) => {
          resolveRefresh = resolve;
        }),
      );
      vi.mocked(orgUnitService.updateQuotaSource).mockResolvedValue({
        ...defaultQuotaManagement,
        requestedSource: "kubernetes",
        revision: 5,
        canEditQuotas: false,
        syncStatus: { phase: "pending" },
      });
      let pending: Promise<void> | undefined;
      if (operation === "poll") {
        vi.mocked(orgUnitService.getQuotaSource).mockResolvedValue(defaultQuotaManagement);
        await vi.advanceTimersByTimeAsync(3000);
      } else {
        pending = operation === "manual" ? source.refresh() : source.changeSource("kubernetes");
        await flushPromises();
      }
      expect(source.busy.value).toBe(true);
      expect(source.locked.value).toBe(true);
      await source.changeSource("kubernetes");
      expect(orgUnitService.updateQuotaSource).toHaveBeenCalledTimes(operation === "handoff" ? 1 : 0);
      resolveRefresh();
      await pending;
      await flushPromises();
      expect(source.busy.value).toBe(false);
      wrapper.unmount();
    },
  );

  it("guards mutation without Update permission", async () => {
    const { wrapper, source } = setup(false, false, false);
    await source.changeSource("kubernetes");
    expect(source.locked.value).toBe(true);
    expect(orgUnitService.updateQuotaSource).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});
