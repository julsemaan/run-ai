/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { shallowMount, mount, flushPromises, config } from "@vue/test-utils";
import { createTestingPinia } from "@pinia/testing";
import { Quasar } from "quasar";
import ProjectForm from "../project/project-edit-form-v2/project-edit-form-v2.vue";
import DepartmentForm from "../department/department-edit-form-v2/department-edit-form-v2.vue";
import QuotaInput from "./quota-input/quota-input.vue";
import { RunaiTableRow } from "@/components/common/runai-table/runai-table-row";
import WeightCell from "@/components/common/runai-table/runai-custom-table-columns/org-unit-editable-weight-col.vue";
import { EditableCellPopup } from "@/components/common/runai-table/runai-custom-table-columns/editable-cell-popup";
import { ECustomCell } from "@/models/table.model";
import { defaultQuotaManagement } from "@/models/quota-management.model";
import {
  mockProject,
  mockDepartment,
  mockFullCapabilitiesNodePools,
} from "../project/project-edit-form-v2/project-edit-form-v2.mock";
import { EStepNames } from "@/models/org-unit-forms.model";
import { usePermissionStore } from "@/stores/permissions.store";

const route = vi.hoisted(() => ({ query: { stepName: "" } }));
vi.mock("vue-router", () => ({ useRoute: () => route }));
vi.mock("@/utils/storage.util", () => ({ storageUtil: { get: vi.fn().mockReturnValue(true), save: vi.fn() } }));
vi.mock("@/services/control-plane/org-unit.service/org-unit.service", () => ({
  orgUnitService: { getQuotaSource: vi.fn(), updateQuotaSource: vi.fn() },
}));
vi.mock("@/services/control-plane/rbac/access-rule.service/access-rule.service", () => ({
  accessRuleService: { getAccessRulesCount: vi.fn().mockResolvedValue(0) },
}));
beforeEach(() => {
  config.global.renderStubDefaultSlot = true;
});
const mounted: { unmount: () => void }[] = [];
afterEach(() => {
  mounted.splice(0).forEach((wrapper) => wrapper.unmount());
  config.global.renderStubDefaultSlot = false;
});

const mountForm = (
  kind: "project" | "department",
  stepName: string,
  locked = true,
  canUpdate = true,
  singlePool = false,
) => {
  route.query.stepName = stepName;
  const pinia = createTestingPinia({ createSpy: vi.fn });
  vi.mocked(usePermissionStore(pinia).hasPermission).mockReturnValue(canUpdate);
  const management = {
    ...defaultQuotaManagement,
    source: locked ? ("kubernetes" as const) : ("runai" as const),
    canEditQuotas: !locked,
  };
  const common = {
    nodePoolsMinimal: singlePool ? mockFullCapabilitiesNodePools.slice(0, 1) : mockFullCapabilitiesNodePools,
  };
  const wrapper =
    kind === "project"
      ? shallowMount(ProjectForm, {
          props: {
            ...common,
            project: { ...mockProject, quotaManagement: management },
            department: mockDepartment,
            isNewProject: false,
          },
          global: {
            plugins: [Quasar, pinia],
            stubs: {
              RunaiWizardLayout: { template: '<div><slot/><slot name="content-footer"/></div>' },
              RunaiFormWrapper: { template: "<div><slot/></div>", methods: { validate: () => true } },
            },
          },
        })
      : shallowMount(DepartmentForm, {
          props: { ...common, department: { ...mockDepartment, quotaManagement: management }, isNewDepartment: false },
          global: {
            plugins: [Quasar, pinia],
            stubs: {
              RunaiWizardLayout: { template: '<div><slot/><slot name="content-footer"/></div>' },
              RunaiFormWrapper: { template: "<div><slot/></div>", methods: { validate: () => true } },
            },
          },
        });
  mounted.push(wrapper);
  return wrapper;
};

describe("native quota ownership locks", () => {
  it.each(["project", "department"] as const)(
    "locks %s quota steps and guards emitted mutation events",
    async (kind) => {
      for (const [step, component, event, payload] of [
        [
          EStepNames.CPUQuota,
          "CpuQuotaStep",
          "save-cpu-quota",
          { nodePoolName: "default", cpuQuota: 2, memoryQuota: 1, memoryUnit: "MiB" },
        ],
        [EStepNames.GpuQuota, "GpuQuotaStep", "save-gpu-quota", { nodePoolName: "default", gpuQuota: 99 }],
        [EStepNames.OverQuota, "GpuOverQuotaStep", "save-over-quota", { nodePoolName: "default", limit: 99 }],
      ] as const) {
        const wrapper = mountForm(kind, step);
        const child = wrapper.findComponent({ name: component });
        expect(child.exists()).toBe(true);
        expect(child.props("readOnly")).toBe(true);
        child.vm.$emit(event, payload);
        await flushPromises();
        expect(wrapper.emitted("on-save")).toBeUndefined();
      }
    },
  );
  it.each(["project", "department"] as const)("keeps %s source visible and nonquota scheduling editable", (kind) => {
    const wrapper = mountForm(kind, EStepNames.SchedulingRules);
    expect(wrapper.findComponent({ name: "OrgUnitQuotaSource" }).exists()).toBe(true);
    expect(wrapper.findComponent({ name: "SchedulingRulesStep" }).props("readOnly") ?? false).toBe(false);
  });
  it.each(["project", "department"] as const)(
    "retains Close/Continue navigation for a locked single-pool %s",
    (kind) => {
      const wrapper = mountForm(kind, EStepNames.GpuQuota, true, true, true);
      const footer = wrapper.findComponent({ name: "OrgUnitFormFooter" });
      expect(footer.exists()).toBe(true);
      expect(footer.props("isFirstStep")).toBe(false);
      expect(footer.props("readOnly") ?? false).toBe(false);
    },
  );

  it("shows source without mutation control for a user without Update", () => {
    const wrapper = mountForm("department", EStepNames.ScopeName, false, false);
    expect(wrapper.findComponent({ name: "OrgUnitQuotaSource" }).props("canUpdate")).toBe(false);
  });
  it("disables a locked peer checkbox without removing the selection column", async () => {
    const wrapper = mount(RunaiTableRow, {
      props: { columns: [], row: { name: "peer" }, isSelectable: true, selectionDisabled: true },
      global: { plugins: [Quasar] },
    });
    mounted.push(wrapper);
    expect(wrapper.find("td").exists()).toBe(true);
    const checkbox = wrapper.findComponent({ name: "QCheckbox" });
    expect(checkbox.props("disable")).toBe(true);
    checkbox.vm.$emit("click", new MouseEvent("click"));
    await flushPromises();
    expect(wrapper.emitted("checkbox-clicked")).toBeUndefined();
  });

  it("locks a Kubernetes peer's weight without displaying a derived weight", async () => {
    const onSave = vi.fn();
    const wrapper = shallowMount(WeightCell, {
      props: {
        row: {
          id: "peer",
          orgUnitId: "peer",
          weight: 17,
          deserved: 2,
          quotaManagement: { ...defaultQuotaManagement, source: "kubernetes", canEditQuotas: false },
        },
        column: {
          name: "weight",
          label: "Weight",
          field: "weight",
          customCellConfig: { [ECustomCell.ORG_UNIT_EDITABLE_WEIGHT_COL]: { nodePoolId: "pool", onSave } },
        },
      },
    });
    mounted.push(wrapper);
    const popup = wrapper.findComponent(EditableCellPopup);
    expect(popup.props("disabled")).toBe(true);
    expect(popup.props("displayValue")).toBe("17");
    popup.vm.$emit("apply");
    await flushPromises();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("disables actual numeric and unit inputs and ignores imperative updates", async () => {
    const wrapper = mount(QuotaInput, {
      props: { modelValue: 0.125, unit: "MiB", unitOptions: [{ label: "MiB", value: "Mib" }] },
      attrs: { disable: true },
      global: { plugins: [Quasar] },
    });
    mounted.push(wrapper);
    expect(wrapper.find('input[type="number"]').attributes("disabled")).toBeDefined();
    expect(wrapper.findComponent({ name: "QSelect" }).props("disable")).toBe(true);
    wrapper.findComponent({ name: "QInput" }).vm.$emit("update:model-value", 9);
    wrapper.findComponent({ name: "QSelect" }).vm.$emit("update:model-value", "GB");
    await flushPromises();
    expect(wrapper.emitted("update:model-value")).toBeUndefined();
    expect(wrapper.emitted("update:unit")).toBeUndefined();
  });
});
