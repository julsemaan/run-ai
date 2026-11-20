/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
import { describe, it, expect, vi } from "vitest";
import { mount } from "@vue/test-utils";
import QuotaSource from "./org-unit-quota-source.vue";
import { defaultQuotaManagement, type QuotaSource as Source } from "@/models/quota-management.model";

vi.mock("@/components/common/runai-base-dialog/runai-base-dialog.vue", () => ({
  default: {
    props: ["modelValue"],
    template: '<div v-if="modelValue"><slot name="header"/><slot name="body"/><slot name="footer"/></div>',
  },
}));

const mountSource = (source: Source = "runai", canUpdate = true, phase: "synced" | "pending" | "error" = "synced") =>
  mount(QuotaSource, {
    props: {
      name: "research",
      entityType: "project",
      canUpdate,
      management: {
        ...defaultQuotaManagement,
        source,
        requestedSource: source,
        syncStatus: { phase, message: phase === "error" ? "Sync failed" : undefined },
      },
    },
    global: {
      stubs: {
        RunaiSelect: true,
        "runai-base-dialog": {
          props: ["modelValue"],
          template: '<div v-if="modelValue"><slot name="header"/><slot name="body"/><slot name="footer"/></div>',
        },
        QBtn: {
          props: ["label", "disable"],
          emits: ["click"],
          template: '<button :disabled="disable" @click="$emit(\'click\')">{{label}}</button>',
        },
        RunaiStatus: { props: ["options"], template: "<span>{{options.status}}</span>" },
      },
    },
  });

describe("native quota source", () => {
  it.each(["runai", "kubernetes"] as const)(
    "confirms switching from %s and names all-pool value preservation",
    async (source) => {
      const wrapper = mountSource(source);
      const target = source === "runai" ? "kubernetes" : "runai";
      wrapper.findComponent({ name: "RunaiSelect" }).vm.$emit("update:model-value", target);
      await wrapper.vm.$nextTick();
      expect(wrapper.text()).toContain('"research"');
      expect(wrapper.text()).toContain("across all node pools");
      expect(wrapper.text()).toContain("latest effective quota values");
      expect(wrapper.text()).toContain("You can switch back");
      expect(wrapper.emitted("change-source")).toBeUndefined();
      await wrapper.find('[aid="quota-source-confirm"]').trigger("click");
      expect(wrapper.emitted("change-source")).toEqual([[target]]);
    },
  );
  it.each(["runai", "kubernetes"] as const)(
    "cancels switching from %s without changing the authoritative selection",
    async (source) => {
      const wrapper = mountSource(source);
      const select = wrapper.findComponent({ name: "RunaiSelect" });
      select.vm.$emit("update:model-value", source === "runai" ? "kubernetes" : "runai");
      await wrapper.vm.$nextTick();
      await wrapper.find('[aid="quota-source-cancel"]').trigger("click");
      expect(wrapper.emitted("change-source")).toBeUndefined();
      expect(select.props("modelValue")).toBe(source);
    },
  );
  it.each([
    [false, "synced"],
    [true, "pending"],
  ] as const)(
    "guards both selection and imperative confirmation when canUpdate=%s phase=%s",
    async (canUpdate, phase) => {
      const wrapper = mountSource("runai", canUpdate, phase);
      wrapper.findComponent({ name: "RunaiSelect" }).vm.$emit("update:model-value", "kubernetes");
      await wrapper.vm.$nextTick();
      expect(wrapper.find('[aid="quota-source-confirm"]').exists()).toBe(false);
      expect(wrapper.findComponent({ name: "RunaiSelect" }).props("disable")).toBe(true);
      expect(wrapper.text()).toContain(phase === "pending" ? "Pending" : "Synced");
    },
  );
  it("shows failure text while preserving the server source", () => {
    const wrapper = mountSource("kubernetes", true, "error");
    expect(wrapper.text()).toContain("Error");
    expect(wrapper.text()).toContain("Sync failed");
    expect(wrapper.findComponent({ name: "RunaiSelect" }).props("modelValue")).toBe("kubernetes");
  });
});
