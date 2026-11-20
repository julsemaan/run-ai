// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package testing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	authapi "github.com/dsx-ai-factory/run-ai/control-plane/authorization/pkg/client/generated"
	authstructs "github.com/dsx-ai-factory/run-ai/control-plane/authorization/pkg/client/structs"
	clusterapi "github.com/dsx-ai-factory/run-ai/control-plane/cluster-service/pkg/client/generated"
	clustermocks "github.com/dsx-ai-factory/run-ai/control-plane/cluster-service/pkg/client/mocks"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/generated/pkg/api"
	orgconfig "github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/config"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/database/dao"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/database/models"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/services"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/services/departments"
	"github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/services/projects"
	commonconfig "github.com/dsx-ai-factory/run-ai/runai-common-packages/config"
	"github.com/dsx-ai-factory/run-ai/runai-common-packages/runaidb"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"k8s.io/utils/ptr"
)

var _ = Describe("Native quota source", func() {
	var ctx context.Context
	var cluster string
	var project api.Project
	var department api.Department
	var ps api.ProjectsAPIServicer
	var ds api.DepartmentsAPIServicer
	var pools map[string]clusterapi.NodepoolMinimal
	var previous orgconfig.General
	var client *clustermocks.IClusterClient
	id := func(raw string) int64 {
		v, e := strconv.ParseInt(raw, 10, 64)
		Expect(e).NotTo(HaveOccurred())
		return v
	}
	BeforeEach(func() {
		previous = *commonconfig.Get[*orgconfig.General]()
		conf := previous
		conf.QuotaPocEnrollments = ""
		commonconfig.Set(&conf)
		cluster = uuid.NewString()
		ctx = context.WithValue(SetContext(TestTenant, TestUser), authstructs.PermittedScopesKey, &authapi.PermittedScopes{Clusters: []string{cluster}})
		client = &clustermocks.IClusterClient{}
		pools = map[string]clusterapi.NodepoolMinimal{}
		for _, p := range []clusterapi.NodepoolMinimal{{Id: "1", Name: "default", ClusterId: cluster, SchedulingCapabilities: NpSchedCapabilitiesDefault(true, true)}, {Id: "2", Name: "other", ClusterId: cluster, SchedulingCapabilities: NpSchedCapabilitiesDefault(true, true)}} {
			pools[p.Id] = p
		}
		client.On("GetNodePoolsMinimalByCluster", mock.Anything, cluster).Return(&clusterapi.GetNodepoolsMinimal200Response{Nodepools: []clusterapi.NodepoolMinimal{pools["1"], pools["2"]}}, nil)
		client.On("GetClusterMinimalDataById", mock.Anything, cluster).Return(&clusterapi.ClustersMinimalData{Uuid: cluster, Version: *clusterapi.NewNullableString(ptr.To("3.0"))}, nil)
		ds = departments.NewDepartmentApiService(client, nil)
		ps = projects.NewProjectApiService(services.NewProjectDeletionService(client, nil, nil), client, nil)
		dr := getDepartmentCreateRequest(createDepartmentName(), cluster)
		dr.Resources = []api.ResourcesNullable{CreateNodePoolApiResources(true, "1", "default"), CreateNodePoolApiResources(true, "2", "other")}
		response, e := ds.CreateDepartment(ctx, dr)
		Expect(e).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusCreated), fmt.Sprint(response.Body))
		department = response.Body.(api.Department)
		pr := getProjectCreateRequest(createProjectName(), "", department.Id, cluster)
		pr.Resources = dr.Resources
		response, e = ps.CreateProject(ctx, pr)
		Expect(e).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusCreated), fmt.Sprint(response.Body))
		project = response.Body.(api.Project)
	})
	AfterEach(func() {
		defer commonconfig.Set(&previous)
		if project.Id != "" {
			p, e := dao.GetProjectById(ctx, id(project.Id), TestTenant, models.SlimVerbosity)
			Expect(e).NotTo(HaveOccurred())
			_, _, e = dao.DeleteProjectInDb(ctx, p, nil, "", false)
			Expect(e).NotTo(HaveOccurred())
		}
		if department.Id != "" {
			_, e := dao.DeleteDepartment(ctx, id(department.Id), TestTenant, nil, services.DefaultDepartmentName)
			Expect(e).NotTo(HaveOccurred())
		}
	})
	resources := func(kind string) []models.Resources {
		if kind == "project" {
			p, e := dao.GetProjectById(ctx, id(project.Id), TestTenant, models.SlimVerbosity)
			Expect(e).NotTo(HaveOccurred())
			return p.OrgUnitData.Resources
		}
		d, e := dao.GetDepartmentById(ctx, id(department.Id), TestTenant, models.SlimVerbosity, nil)
		Expect(e).NotTo(HaveOccurred())
		return d.OrgUnitData.Resources
	}
	objectID := func(kind string) int64 {
		if kind == "project" {
			return id(project.Id)
		}
		return id(department.Id)
	}
	transfer := func(kind, source string, revision int64) api.QuotaManagement {
		var response api.ImplResponse
		var e error
		if kind == "project" {
			response, e = ps.UpdateProjectQuotaSource(ctx, project.Id, api.QuotaSourceUpdateRequest{Source: source, ExpectedRevision: revision})
		} else {
			response, e = ds.UpdateDepartmentQuotaSource(ctx, department.Id, api.QuotaSourceUpdateRequest{Source: source, ExpectedRevision: revision})
		}
		Expect(e).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusOK), fmt.Sprint(response.Body))
		return *response.Body.(*api.QuotaManagement)
	}
	snapshot := func(kind string, value float64) api.QuotaSnapshot {
		c, e := dao.GetQuotaControl(ctx, TestTenant, cluster, kind, objectID(kind))
		Expect(e).NotTo(HaveOccurred())
		Expect(c).NotTo(BeNil())
		s := api.QuotaSnapshot{Epoch: c.Epoch, KubernetesUid: c.KubernetesUID, Generation: 10 + c.Revision, ControlRevision: c.Revision, ControlPhase: c.Phase, Source: c.TargetSource}
		for _, p := range c.Pools {
			r := api.QuotaSnapshotResource{Deserved: ptr.To(value), Limit: ptr.To(2000.0), OverQuotaWeight: ptr.To(2.0)}
			s.Pools = append(s.Pools, api.QuotaSnapshotPool{NodePoolId: p.NodePoolId, Gpu: r, Cpu: r, Memory: r})
		}
		return s
	}
	observe := func(kind string, s api.QuotaSnapshot) error {
		return services.ImportQuotaSnapshot(ctx, TestTenant, cluster, kind, objectID(kind), s, resources(kind), pools)
	}

	DescribeTable("preserves all effective numbers on first native freeze",
		func(kind string, legacy, cpuQuota bool) {
			client.ExpectedCalls[0].Unset()
			names := []string{"default", "A100", "H100", "L4"}
			metadata := []clusterapi.NodepoolMinimal{}
			for i, name := range names {
				poolID := strconv.Itoa(i + 1)
				p := clusterapi.NodepoolMinimal{Id: poolID, Name: name, ClusterId: cluster, SchedulingCapabilities: NpSchedCapabilitiesDefault(cpuQuota, false)}
				pools[poolID] = p
				metadata = append(metadata, p)
			}
			client.On("GetNodePoolsMinimalByCluster", mock.Anything, cluster).Return(&clusterapi.GetNodepoolsMinimal200Response{Nodepools: metadata}, nil)
			rows := resources(kind)
			sort.Slice(rows, func(i, j int) bool { return rows[i].NodePoolId < rows[j].NodePoolId })
			for i := 0; i < 4; i++ {
				r := rows[0].Clone()
				if i < len(rows) {
					r = rows[i]
				} else {
					r.ID = uuid.New()
				}
				r.NodePoolId, r.NodePoolName = strconv.Itoa(i+1), names[i]
				r.GpuDeserved, r.GpuLimit = ptr.To(0.0), ptr.To(7.0)
				r.GpuOverQuotaWeight = ptr.To(float64(i + 3))
				r.CpuDeserved, r.CpuLimit, r.CpuOverQuotaWeight = ptr.To(3.125), ptr.To(19.75), ptr.To(8.0)
				r.MemoryDeserved, r.MemoryLimit, r.MemoryOverQuotaWeight = ptr.To(4.125), ptr.To(23.75), ptr.To(9.0)
				r.MemoryUnits = []models.Units{models.MB, models.GB, models.Mib, models.MB}[i]
				if i < len(rows) {
					_, err := runaidb.DB().Table(models.ResourcesTable).Where("id = ?", r.ID).Updates(&r)
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(dao.AddResources(ctx, []*models.Resources{&r})).To(Succeed())
				}
			}
			uid := uuid.NewString()
			if legacy {
				conf := *commonconfig.Get[*orgconfig.General]()
				base := project.Name
				if kind == "department" {
					base = "q-" + department.Id
				}
				entry := orgconfig.QuotaEnrollment{TenantId: TestTenant, ClusterId: cluster, Kind: kind, Id: objectID(kind), State: "active", KubernetesUid: uid, Epoch: "legacy", Pools: []api.QuotaOwnershipPool{{NodePoolId: "2", NodePoolName: "A100", QueueName: base + "-A100"}}}
				raw, err := json.Marshal([]orgconfig.QuotaEnrollment{entry})
				Expect(err).NotTo(HaveOccurred())
				conf.QuotaPocEnrollments = string(raw)
				commonconfig.Set(&conf)
			}
			Expect(dao.BindQuotaUID(ctx, TestTenant, cluster, kind, objectID(kind), uid)).To(Succeed())
			if legacy {
				r := api.QuotaSnapshotResource{Deserved: ptr.To(1.25), Limit: ptr.To(11.0), OverQuotaWeight: ptr.To(6.75)}
				latest := api.QuotaSnapshot{Epoch: "legacy", KubernetesUid: uid, Generation: 7, Pools: []api.QuotaSnapshotPool{{NodePoolId: "2", Gpu: r, Cpu: r, Memory: r}}}
				Expect(observe(kind, latest)).To(Succeed())
			}
			before := resources(kind)
			oldMetadata := services.QuotaNodePoolsMetadata(TestTenant, cluster, kind, objectID(kind), pools)
			// Compare actual scheduler numbers, including defaults for omitted CPU/memory.
			numbers := func(rs []api.ResourcesSync) map[string][9]float64 {
				out := map[string][9]float64{}
				for _, r := range rs {
					v := [9]float64{r.Gpu.Deserved, ptr.Deref(r.Gpu.Limit, -1), ptr.Deref(r.Gpu.OverQuotaWeight, 1), -1, -1, 1, -1, -1, 1}
					if r.Cpu != nil {
						v[3], v[4], v[5] = ptr.Deref(r.Cpu.Deserved, -1), ptr.Deref(r.Cpu.Limit, -1), ptr.Deref(r.Cpu.OverQuotaWeight, 1)
					}
					if r.Memory != nil {
						v[6], v[7], v[8] = ptr.Deref(r.Memory.Deserved, -1), ptr.Deref(r.Memory.Limit, -1), ptr.Deref(r.Memory.OverQuotaWeight, 1)
					}
					out[r.NodePool.Id] = v
				}
				return out
			}
			var preSwitch []api.ResourcesSync
			if kind == "project" {
				response, err := ps.GetSyncProjects(ctx, cluster, time.Time{}, 0, 100, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Code).To(Equal(http.StatusOK))
				preSwitch = response.Body.(api.GetSyncProjects200Response).Projects[0].Resources
			} else {
				response, err := ds.GetSyncDepartments(ctx, cluster, time.Time{}, 0, 100)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Code).To(Equal(http.StatusOK))
				preSwitch = response.Body.(api.GetSyncDepartments200Response).Departments[0].Resources
			}
			want := numbers(preSwitch)
			Expect(want).To(Equal(numbers(services.ApiResourcesFromModelSync(before, oldMetadata))))
			Expect(want["1"][2]).To(Equal(services.MinOverQuotaWeight))
			transfer(kind, "kubernetes", 0)
			frozen, ownership, err := dao.QuotaCommandSnapshot(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(err).NotTo(HaveOccurred())
			Expect(ownership.Phase).To(Equal("freeze"))
			Expect(ownership.Pools).To(HaveLen(4))
			got := numbers(services.ApiResourcesFromModelSync(frozen, services.QuotaNodePoolsMetadata(TestTenant, cluster, kind, objectID(kind), pools)))
			Expect(got).To(Equal(want))
			for _, r := range frozen {
				for _, old := range before {
					if r.NodePoolId == old.NodePoolId {
						Expect(r.MemoryUnits).To(Equal(old.MemoryUnits))
						if legacy && r.NodePoolId == "2" {
							Expect(models.ResourcesEqual(r, old)).To(BeTrue())
						}
					}
				}
			}
			// A same-target retry must not project the frozen rows again.
			transfer(kind, "kubernetes", 1)
			Expect(models.ResourcesArrayEqual(resources(kind), frozen)).To(BeTrue())
			for range 3 {
				Expect(observe(kind, snapshot(kind, 1))).To(Succeed())
			}
			transfer(kind, "runai", 3)
			for range 3 {
				Expect(observe(kind, snapshot(kind, 1))).To(Succeed())
			}
			returned := resources(kind)
			want = numbers(services.ApiResourcesFromModelSync(returned, services.QuotaNodePoolsMetadata(TestTenant, cluster, kind, objectID(kind), pools)))
			transfer(kind, "kubernetes", 6)
			command, _, err := dao.QuotaCommandSnapshot(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(err).NotTo(HaveOccurred())
			Expect(models.ResourcesArrayEqual(command, returned)).To(BeTrue())
			Expect(numbers(services.ApiResourcesFromModelSync(command, services.QuotaNodePoolsMetadata(TestTenant, cluster, kind, objectID(kind), pools)))).To(Equal(want))
		},
		Entry("project stock hidden CPU and weights", "project", false, false),
		Entry("project stock visible CPU and derived weights", "project", false, true),
		Entry("project partial legacy hidden CPU and weights", "project", true, false),
		Entry("project partial legacy visible CPU and derived weights", "project", true, true),
		Entry("department stock hidden CPU and weights", "department", false, false),
		Entry("department stock visible CPU and derived weights", "department", false, true),
		Entry("department partial legacy hidden CPU and weights", "department", true, false),
		Entry("department partial legacy visible CPU and derived weights", "department", true, true),
	)

	It("rolls back effective projection and the ownership fence together", func() {
		for _, kind := range []string{"project", "department"} {
			before := resources(kind)
			calls := 0
			_, err := dao.UpdateQuotaSource(ctx, TestTenant, cluster, kind, objectID(kind), api.QuotaSourceUpdateRequest{Source: "kubernetes"}, func(r models.Resources) (models.Resources, error) {
				calls++
				if calls == 2 {
					return r, fmt.Errorf("projection failed")
				}
				r.GpuOverQuotaWeight = ptr.To(services.MinOverQuotaWeight)
				return r, nil
			})
			Expect(err).To(MatchError("projection failed"))
			Expect(calls).To(Equal(2))
			Expect(models.ResourcesArrayEqual(resources(kind), before)).To(BeTrue())
			control, err := dao.GetQuotaControl(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(err).NotTo(HaveOccurred())
			Expect(control).To(BeNil())
		}
	})

	It("preserves view status and unmanaged numerics while fencing managed quota snapshots", func() {
		for _, kind := range []string{"project", "department"} {
			view := resources(kind)
			view[0].GpuOverQuotaWeight = ptr.To(37.0)
			view[0].ResourcesStatus = models.ResourcesStatus{AllocatedGpu: ptr.To(7.0), AllocatedCpu: ptr.To(8.0), AllocatedMemory: ptr.To(9.0), RequestedGpu: ptr.To(10.0), RequestedCpu: ptr.To(11.0), RequestedMemory: ptr.To(12.0), AllocatedNonPreemptibleGpu: ptr.To(13.0), AllocatedNonPreemptibleCpu: ptr.To(14.0), AllocatedNonPreemptibleMemory: ptr.To(15.0)}
			view[0].Rank = ptr.To("High")
			for _, bind := range []bool{false, true} {
				if bind {
					Expect(dao.BindQuotaUID(ctx, TestTenant, cluster, kind, objectID(kind), uuid.NewString())).To(Succeed())
				}
				got, management := services.QuotaObjectSnapshot(ctx, TestTenant, cluster, kind, objectID(kind), view)
				Expect(management.Revision).To(BeZero())
				Expect(got).To(Equal(view))
			}
			transfer(kind, "kubernetes", 0)
			got, management := services.QuotaObjectSnapshot(ctx, TestTenant, cluster, kind, objectID(kind), view)
			Expect(management.Revision).To(Equal(int64(1)))
			Expect(models.ResourcesArrayEqual(got, resources(kind))).To(BeTrue())
			for _, r := range got {
				if r.NodePoolId == view[0].NodePoolId {
					Expect(r.ResourcesStatus).To(Equal(view[0].ResourcesStatus))
					Expect(r.Rank).To(Equal(view[0].Rank))
				}
			}
			// A stale view must neither resurrect removed pools nor omit current pools.
			view[1].NodePoolId = "removed"
			got, _ = services.QuotaObjectSnapshot(ctx, TestTenant, cluster, kind, objectID(kind), view)
			Expect(got).To(HaveLen(2))
			Expect([]string{got[0].NodePoolId, got[1].NodePoolId}).To(ConsistOf("1", "2"))
		}
	})

	It("ignores deleted controls for incremental sync without erasing their fences", func() {
		updatedAfter := time.Now().Add(time.Hour)
		got, err := services.QuotaSyncUpdatedAfter(TestTenant, cluster, updatedAfter)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(updatedAfter))
		for _, kind := range []string{"project", "department"} {
			transfer(kind, "kubernetes", 0)
		}
		got, err = services.QuotaSyncUpdatedAfter(TestTenant, cluster, updatedAfter)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeZero())
		projectID, departmentID := objectID("project"), objectID("department")
		p, err := dao.GetProjectById(ctx, projectID, TestTenant, models.SlimVerbosity)
		Expect(err).NotTo(HaveOccurred())
		_, _, err = dao.DeleteProjectInDb(ctx, p, nil, "", false)
		Expect(err).NotTo(HaveOccurred())
		project.Id = ""
		got, err = services.QuotaSyncUpdatedAfter(TestTenant, cluster, updatedAfter)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeZero(), "the active department still requires full phase sync")
		_, err = dao.DeleteDepartment(ctx, departmentID, TestTenant, nil, services.DefaultDepartmentName)
		Expect(err).NotTo(HaveOccurred())
		department.Id = ""
		// Retained DB controls must suppress a legacy config entry for the same deleted object.
		conf := *commonconfig.Get[*orgconfig.General]()
		legacy := []orgconfig.QuotaEnrollment{{TenantId: TestTenant, ClusterId: cluster, Kind: "project", Id: projectID, KubernetesUid: uuid.NewString(), Epoch: "deleted-epoch", State: "active", Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "default", QueueName: p.Name}}}}
		raw, err := json.Marshal(legacy)
		Expect(err).NotTo(HaveOccurred())
		conf.QuotaPocEnrollments = string(raw)
		commonconfig.Set(&conf)
		got, err = services.QuotaSyncUpdatedAfter(TestTenant, cluster, updatedAfter)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(updatedAfter))
		c, err := dao.GetQuotaControl(ctx, TestTenant, cluster, "project", projectID)
		Expect(err).NotTo(HaveOccurred())
		Expect(c.Revision).To(Equal(int64(1)))
	})

	It("round trips both objects through every phase and fences stale writers", func() {
		for _, kind := range []string{"project", "department"} {
			staleProject, err := dao.GetProjectById(ctx, id(project.Id), TestTenant, models.SlimVerbosity)
			Expect(err).NotTo(HaveOccurred())
			m := transfer(kind, "kubernetes", 0)
			Expect(m.Revision).To(Equal(int64(1)))
			Expect(m.CanEditQuotas).To(BeFalse())
			_, ownership, e := dao.QuotaCommandSnapshot(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(e).NotTo(HaveOccurred())
			Expect(ownership).To(BeNil())
			uid := uuid.NewString()
			Expect(services.BindQuotaUID(SetContext(TestTenant, TestUser), TestTenant, cluster, kind, objectID(kind), uid)).To(HaveOccurred())
			var binding api.ImplResponse
			if kind == "project" {
				binding, e = ps.UpdateProjectStatus(ctx, api.ProjectStatusUpdateRequest{Id: project.Id, ClusterId: cluster, Status: api.ProjectStatus{Phase: api.PROJECTPHASE_READY, KubernetesUid: uid}})
			} else {
				binding, e = ds.UpdateDepartmentStatus(ctx, department.Id, api.DepartmentStatusUpdateRequest{ClusterId: cluster, Status: api.DepartmentStatus{KubernetesUid: uid}})
			}
			Expect(e).NotTo(HaveOccurred())
			Expect(binding.Code).To(BeNumerically(">=", 200))
			Expect(binding.Code).To(BeNumerically("<", 300))
			Expect(services.BindQuotaUID(ctx, TestTenant, cluster, kind, objectID(kind), uuid.NewString())).To(HaveOccurred())
			command, o, e := dao.QuotaCommandSnapshot(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(e).NotTo(HaveOccurred())
			Expect(command).To(HaveLen(2))
			Expect(o.Pools).To(HaveLen(2))
			Expect(o.Source).To(Equal("kubernetes"))
			for _, phase := range []string{"freeze", "release", "stable"} {
				s := snapshot(kind, 1)
				Expect(s.ControlPhase).To(Equal(phase))
				old := s
				old.ControlRevision--
				Expect(observe(kind, old)).To(HaveOccurred())
				Expect(observe(kind, s)).To(Succeed())
				if phase != "stable" {
					Expect(observe(kind, s)).To(HaveOccurred())
					management, err := dao.GetQuotaManagement(ctx, TestTenant, cluster, kind, objectID(kind))
					Expect(err).NotTo(HaveOccurred())
					Expect(management.CanEditQuotas).To(BeFalse())
					Expect(management.SyncStatus.Phase).To(Equal("pending"))
				}
			}
			c, e := dao.GetQuotaControl(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(e).NotTo(HaveOccurred())
			Expect(c.Source).To(Equal("kubernetes"))
			epoch := c.Epoch
			live := snapshot(kind, 2)
			live.Generation += 10
			Expect(observe(kind, live)).To(Succeed())
			m = transfer(kind, "runai", 3)
			Expect(m.Source).To(Equal("kubernetes"))
			Expect(m.RequestedSource).To(Equal("runai"))
			Expect(m.CanEditQuotas).To(BeFalse())
			c, e = dao.GetQuotaControl(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(e).NotTo(HaveOccurred())
			Expect(c.Epoch).NotTo(Equal(epoch))
			for _, phase := range []string{"freeze", "release", "stable"} {
				s := snapshot(kind, 2)
				Expect(s.ControlPhase).To(Equal(phase))
				if phase != "freeze" {
					Expect(observe(kind, snapshot(kind, 1))).To(HaveOccurred())
				}
				Expect(observe(kind, s)).To(Succeed())
			}
			mptr, e := dao.GetQuotaManagement(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(e).NotTo(HaveOccurred())
			Expect(mptr.CanEditQuotas).To(BeTrue())
			Expect(mptr.Revision).To(Equal(int64(6)))
			for _, r := range resources(kind) {
				Expect(*r.GpuDeserved).To(Equal(2.0))
				Expect(*r.CpuDeserved).To(Equal(2.0))
			}
			reported := snapshot(kind, 1)
			reported.Generation += 100
			Expect(observe(kind, reported)).To(Succeed())
			for _, r := range resources(kind) {
				Expect(*r.GpuDeserved).To(Equal(2.0))
			}
			if kind == "project" {
				p, e := dao.GetProjectById(ctx, objectID(kind), TestTenant, models.SlimVerbosity)
				Expect(e).NotTo(HaveOccurred())
				p.Description = "metadata"
				_, e = dao.UpdateProject(ctx, p, false, false)
				Expect(e).NotTo(HaveOccurred())
				p.OrgUnitData.Resources[0].GpuDeserved = ptr.To(1.0)
				_, e = dao.UpdateProject(ctx, p, false, false)
				Expect(e).To(MatchError(dao.ErrQuotaRevision))
				p.QuotaManagementRevision = 6
				_, e = dao.UpdateProject(ctx, p, false, false)
				Expect(e).NotTo(HaveOccurred())
				response, err := ps.GetProject(ctx, project.Id)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Code).To(Equal(http.StatusOK))
				value := response.Body.(api.Project)
				Expect(value.QuotaManagement.Revision).To(Equal(int64(6)))
				update := getProjectUpdateRequest(value)
				update.Resources[0].Gpu.Deserved = 1.25
				response, err = ps.UpdateProject(ctx, project.Id, update)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Code).To(Equal(http.StatusConflict))
				update.QuotaManagementRevision = 6
				response, err = ps.UpdateProject(ctx, project.Id, update)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Code).To(Equal(http.StatusOK), fmt.Sprint(response.Body))
				coherent := projects.MapModelToResponse(ctx, *staleProject, pools)
				Expect(coherent.QuotaManagement.Revision).To(Equal(int64(6)))
				for _, r := range coherent.Resources {
					if r.NodePool.Id == update.Resources[0].NodePool.Id {
						Expect(r.Gpu.Deserved).To(Equal(1.25))
					}
				}
			} else {
				d, err := dao.GetDepartmentById(ctx, objectID(kind), TestTenant, models.SlimVerbosity, nil)
				Expect(err).NotTo(HaveOccurred())
				d.Description = "metadata"
				_, err = dao.UpdateDepartment(ctx, d)
				Expect(err).NotTo(HaveOccurred())
				d.OrgUnitData.Resources[0].CpuDeserved = ptr.To(1.0)
				_, err = dao.UpdateDepartment(ctx, d)
				Expect(err).To(MatchError(dao.ErrQuotaRevision))
				d.QuotaManagementRevision = 6
				_, err = dao.UpdateDepartment(ctx, d)
				Expect(err).NotTo(HaveOccurred())
			}
			m = transfer(kind, "runai", 6)
			Expect(m.Revision).To(Equal(int64(6)))
		}
	})

	It("confirms Runai after missing the first stable report and agent precision reconciliation", func() {
		for _, kind := range []string{"project", "department"} {
			if kind == "project" {
				p, err := dao.GetProjectById(ctx, objectID(kind), TestTenant, models.SlimVerbosity)
				Expect(err).NotTo(HaveOccurred())
				p.OrgUnitData.Resources[0].MemoryUnits = models.MB
				p.OrgUnitData.Resources[1].MemoryUnits = models.Mib
				_, err = dao.UpdateProject(ctx, p, false, false)
				Expect(err).NotTo(HaveOccurred())
			}
			transfer(kind, "kubernetes", 0)
			Expect(dao.BindQuotaUID(ctx, TestTenant, cluster, kind, objectID(kind), uuid.NewString())).To(Succeed())
			for range 3 {
				Expect(observe(kind, snapshot(kind, 1))).To(Succeed())
			}
			transfer(kind, "runai", 3)
			precise := func() api.QuotaSnapshot {
				s := snapshot(kind, 1)
				for i := range s.Pools {
					s.Pools[i].Gpu = api.QuotaSnapshotResource{Deserved: ptr.To(0.123), Limit: ptr.To(2.345), OverQuotaWeight: ptr.To(1.234)}
					s.Pools[i].Cpu = api.QuotaSnapshotResource{Deserved: ptr.To(0.12345), Limit: ptr.To(2.34567), OverQuotaWeight: ptr.To(1.23456)}
					s.Pools[i].Memory = api.QuotaSnapshotResource{Deserved: ptr.To(123.45678), Limit: ptr.To(2345.67891), OverQuotaWeight: ptr.To(1.23456)}
				}
				return s
			}
			Expect(observe(kind, precise())).To(Succeed()) // freeze
			Expect(observe(kind, precise())).To(Succeed()) // release
			before := resources(kind)
			// Skip the first stable report. The next agent pass writes persisted
			// backend precision, and memory is reported in decimal MB on the wire.
			reconciled := snapshot(kind, 1)
			reconciled.Generation += 100
			for i := range reconciled.Pools {
				for _, r := range before {
					if r.NodePoolId != reconciled.Pools[i].NodePoolId {
						continue
					}
					Expect(*r.GpuDeserved).To(Equal(0.12))
					reconciled.Pools[i].Gpu = api.QuotaSnapshotResource{Deserved: r.GpuDeserved, Limit: r.GpuLimit, OverQuotaWeight: r.GpuOverQuotaWeight}
					reconciled.Pools[i].Cpu = api.QuotaSnapshotResource{Deserved: r.CpuDeserved, Limit: r.CpuLimit, OverQuotaWeight: r.CpuOverQuotaWeight}
					mb := func(v *float64) *float64 {
						switch r.MemoryUnits {
						case models.MB:
							return v
						case models.GB:
							return ptr.To(*v * 1000)
						default:
							return ptr.To(services.MibToMb(*v))
						}
					}
					reconciled.Pools[i].Memory = api.QuotaSnapshotResource{Deserved: mb(r.MemoryDeserved), Limit: mb(r.MemoryLimit), OverQuotaWeight: r.MemoryOverQuotaWeight}
				}
			}
			wrong := precise()
			wrong.Pools[0].Gpu.Deserved = ptr.To(0.5)
			Expect(observe(kind, wrong)).To(HaveOccurred())
			Expect(observe(kind, reconciled)).To(Succeed())
			Expect(resources(kind)).To(Equal(before))
			m, err := dao.GetQuotaManagement(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(err).NotTo(HaveOccurred())
			Expect(m.CanEditQuotas).To(BeTrue())
			Expect(m.SyncStatus.Phase).To(Equal("synced"))
		}
	})

	It("rejects last-pool deletion and empty full updates after returning to Runai", func() {
		for _, kind := range []string{"project", "department"} {
			transfer(kind, "kubernetes", 0)
			Expect(dao.BindQuotaUID(ctx, TestTenant, cluster, kind, objectID(kind), uuid.NewString())).To(Succeed())
			for range 3 {
				Expect(observe(kind, snapshot(kind, 2))).To(Succeed())
			}
			transfer(kind, "runai", 3)
			for range 3 {
				Expect(observe(kind, snapshot(kind, 2))).To(Succeed())
			}
			before := resources(kind)
			// The delete route accepts IDs alone, including a multi-row removal.
			Expect(dao.DeleteResources(ctx, []models.Resources{{BaseModel: before[0].BaseModel}, {BaseModel: before[1].BaseModel}})).To(MatchError(ContainSubstring("at least one node pool")))
			Expect(resources(kind)).To(Equal(before))
			if kind == "project" {
				p, err := dao.GetProjectById(ctx, objectID(kind), TestTenant, models.SlimVerbosity)
				Expect(err).NotTo(HaveOccurred())
				p.QuotaManagementRevision = 6
				p.OrgUnitData.Resources = nil
				_, err = dao.UpdateProject(ctx, p, false, false)
				Expect(err).To(MatchError(ContainSubstring("at least one node pool")))
				response, err := ps.GetProject(ctx, project.Id)
				Expect(err).NotTo(HaveOccurred())
				update := getProjectUpdateRequest(response.Body.(api.Project))
				update.QuotaManagementRevision = 6
				update.Resources = []api.ResourcesNullable{}
				response, err = ps.UpdateProject(ctx, project.Id, update)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Code).To(Equal(http.StatusBadRequest), fmt.Sprint(response.Body))
			} else {
				d, err := dao.GetDepartmentById(ctx, objectID(kind), TestTenant, models.SlimVerbosity, nil)
				Expect(err).NotTo(HaveOccurred())
				d.QuotaManagementRevision = 6
				d.OrgUnitData.Resources = nil
				_, err = dao.UpdateDepartment(ctx, d)
				Expect(err).To(MatchError(ContainSubstring("at least one node pool")))
				response, err := ds.GetDepartment(ctx, department.Id)
				Expect(err).NotTo(HaveOccurred())
				update := getDepartmentUpdateRequest(response.Body.(api.Department))
				update.QuotaManagementRevision = 6
				update.Resources = []api.ResourcesNullable{}
				response, err = ds.UpdateDepartment(ctx, department.Id, update)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Code).To(Equal(http.StatusBadRequest), fmt.Sprint(response.Body))
			}
			Expect(resources(kind)).To(Equal(before))
			Expect(dao.DeleteResources(ctx, before[1:])).To(Succeed())
			_, _, err := dao.QuotaCommandSnapshot(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(err).NotTo(HaveOccurred())
			Expect(observe(kind, snapshot(kind, 2))).To(Succeed())
			remaining := resources(kind)
			Expect(remaining).To(HaveLen(1))
			Expect(dao.DeleteResources(ctx, remaining)).To(MatchError(ContainSubstring("at least one node pool")))
			m, err := dao.GetQuotaManagement(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(err).NotTo(HaveOccurred())
			Expect(m.CanEditQuotas).To(BeTrue())
			Expect(m.Revision).To(Equal(int64(7)))
			if kind == "project" {
				p, err := dao.GetProjectById(ctx, objectID(kind), TestTenant, models.SlimVerbosity)
				Expect(err).NotTo(HaveOccurred())
				p.QuotaManagementRevision = m.Revision
				p.OrgUnitData.Resources[0].GpuDeserved = ptr.To(1.5)
				_, err = dao.UpdateProject(ctx, p, false, false)
				Expect(err).NotTo(HaveOccurred())
			} else {
				d, err := dao.GetDepartmentById(ctx, objectID(kind), TestTenant, models.SlimVerbosity, nil)
				Expect(err).NotTo(HaveOccurred())
				d.QuotaManagementRevision = m.Revision
				d.OrgUnitData.Resources[0].GpuDeserved = ptr.To(1.5)
				_, err = dao.UpdateDepartment(ctx, d)
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(*resources(kind)[0].GpuDeserved).To(Equal(1.5))
		}
	})

	It("uses canonical department queue names through the public source service", func() {
		Expect(department.Name).NotTo(Equal("q-" + department.Id))
		transfer("department", "kubernetes", 0)
		Expect(dao.BindQuotaUID(ctx, TestTenant, cluster, "department", objectID("department"), "department-uid")).To(Succeed())
		_, ownership, err := dao.QuotaCommandSnapshot(ctx, TestTenant, cluster, "department", objectID("department"))
		Expect(err).NotTo(HaveOccurred())
		Expect(ownership.Pools).To(ConsistOf(
			api.QuotaOwnershipPool{NodePoolId: "1", NodePoolName: "default", QueueName: "q-" + department.Id},
			api.QuotaOwnershipPool{NodePoolId: "2", NodePoolName: "other", QueueName: "q-" + department.Id + "-other"},
		))
	})

	It("imports Kubernetes edits made after the stable marker before its first report", func() {
		for _, kind := range []string{"project", "department"} {
			transfer(kind, "kubernetes", 0)
			Expect(dao.BindQuotaUID(ctx, TestTenant, cluster, kind, objectID(kind), uuid.NewString())).To(Succeed())
			Expect(observe(kind, snapshot(kind, 1))).To(Succeed())
			Expect(observe(kind, snapshot(kind, 1))).To(Succeed())
			m, err := dao.GetQuotaManagement(ctx, TestTenant, cluster, kind, objectID(kind))
			Expect(err).NotTo(HaveOccurred())
			Expect(m.CanEditQuotas).To(BeFalse())
			Expect(m.SyncStatus.Phase).To(Equal("pending"))
			Expect(observe(kind, snapshot(kind, 2))).To(Succeed())
			for _, r := range resources(kind) {
				Expect(*r.GpuDeserved).To(Equal(2.0))
			}
		}
	})

	It("fences removed and readded Runai pools after return before another handoff", func() {
		for _, kind := range []string{"project", "department"} {
			transfer(kind, "kubernetes", 0)
			Expect(dao.BindQuotaUID(ctx, TestTenant, cluster, kind, objectID(kind), uuid.NewString())).To(Succeed())
			for range 3 {
				Expect(observe(kind, snapshot(kind, 2))).To(Succeed())
			}
			transfer(kind, "runai", 3)
			for range 3 {
				Expect(observe(kind, snapshot(kind, 2))).To(Succeed())
			}
			var removed models.Resources
			for _, r := range resources(kind) {
				if r.NodePoolId == "2" {
					removed = r
				}
			}
			revision := int64(6)
			for _, add := range []bool{false, true} {
				old := snapshot(kind, 2)
				if add {
					removed.ID = uuid.New()
					Expect(dao.AddResources(ctx, []*models.Resources{&removed})).To(Succeed())
					// Public reads also publish the refresh when no source PUT
					// arrives, and must pair the new fence with current rows.
					public, err := services.QuotaSource(ctx, kind, strconv.FormatInt(objectID(kind), 10), nil, client)
					Expect(err).NotTo(HaveOccurred())
					Expect(public.Code).To(Equal(http.StatusOK))
					Expect(public.Body.(*api.QuotaManagement).Revision).To(Equal(revision + 1))
					Expect(public.Body.(*api.QuotaManagement).CanEditQuotas).To(BeFalse())
				} else {
					Expect(dao.DeleteResources(ctx, []models.Resources{removed})).To(Succeed())
				}
				// A source PUT must persist the refresh, not freeze a pool set
				// which the agent has not yet reconciled.
				response, err := services.QuotaSource(ctx, kind, strconv.FormatInt(objectID(kind), 10), &api.QuotaSourceUpdateRequest{Source: "kubernetes", ExpectedRevision: revision}, client)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Code).To(Equal(http.StatusConflict))
				revision++
				command, ownership, err := dao.QuotaCommandSnapshot(ctx, TestTenant, cluster, kind, objectID(kind))
				Expect(err).NotTo(HaveOccurred())
				Expect(ownership.Revision).To(Equal(revision))
				Expect(ownership.Epoch).To(Equal(old.Epoch))
				Expect(ownership.Phase).To(Equal("stable"))
				Expect(ownership.Source).To(Equal("runai"))
				Expect(ownership.Pools).To(HaveLen(len(command)))
				for _, pool := range ownership.Pools {
					base := project.Name
					if kind == "department" {
						base = "q-" + department.Id
					}
					if pool.NodePoolName != "default" {
						base += "-" + pool.NodePoolName
					}
					Expect(pool.QueueName).To(Equal(base))
				}
				Expect(observe(kind, old)).To(HaveOccurred())
				if kind == "project" {
					p, err := dao.GetProjectById(ctx, objectID(kind), TestTenant, models.SlimVerbosity)
					Expect(err).NotTo(HaveOccurred())
					p.QuotaManagementRevision = revision
					p.OrgUnitData.Resources[0].GpuDeserved = ptr.To(1.0)
					_, err = dao.UpdateProject(ctx, p, false, false)
					Expect(err).To(MatchError(dao.ErrKubernetesQuota))
				} else {
					d, err := dao.GetDepartmentById(ctx, objectID(kind), TestTenant, models.SlimVerbosity, nil)
					Expect(err).NotTo(HaveOccurred())
					d.QuotaManagementRevision = revision
					d.OrgUnitData.Resources[0].GpuDeserved = ptr.To(1.0)
					_, err = dao.UpdateDepartment(ctx, d)
					Expect(err).To(MatchError(dao.ErrKubernetesQuota))
				}
				response, err = services.QuotaSource(ctx, kind, strconv.FormatInt(objectID(kind), 10), &api.QuotaSourceUpdateRequest{Source: "kubernetes", ExpectedRevision: revision}, client)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Code).To(Equal(http.StatusConflict))
				wrong := snapshot(kind, 1)
				Expect(observe(kind, wrong)).To(HaveOccurred())
				for _, r := range resources(kind) {
					Expect(*r.GpuDeserved).To(Equal(2.0))
				}
				fresh := snapshot(kind, 2)
				Expect(observe(kind, fresh)).To(Succeed())
				m, err := dao.GetQuotaManagement(ctx, TestTenant, cluster, kind, objectID(kind))
				Expect(err).NotTo(HaveOccurred())
				Expect(m.CanEditQuotas).To(BeTrue())
				// A later report is diagnostic, never numeric authority.
				wrong.Generation = fresh.Generation + 1
				Expect(observe(kind, wrong)).To(Succeed())
				for _, r := range resources(kind) {
					Expect(*r.GpuDeserved).To(Equal(2.0))
				}
			}
			m := transfer(kind, "kubernetes", revision)
			Expect(m.Revision).To(Equal(revision + 1))
			Expect(m.CanEditQuotas).To(BeFalse())
		}
	})

	It("rejects stale requests and opposite targets while pending and locks every pool", func() {
		transfer("project", "kubernetes", 0)
		for _, request := range []api.QuotaSourceUpdateRequest{{Source: "runai", ExpectedRevision: 1}, {Source: "kubernetes", ExpectedRevision: 0}} {
			r, e := ps.UpdateProjectQuotaSource(ctx, project.Id, request)
			Expect(e).NotTo(HaveOccurred())
			Expect(r.Code).To(Equal(http.StatusConflict))
		}
		transfer("project", "kubernetes", 1)
		p, e := dao.GetProjectById(ctx, id(project.Id), TestTenant, models.SlimVerbosity)
		Expect(e).NotTo(HaveOccurred())
		p.OrgUnitData.Resources[1].GpuDeserved = ptr.To(0.5)
		_, e = dao.UpdateProject(ctx, p, false, false)
		Expect(e).To(MatchError(dao.ErrKubernetesQuota))
		p, e = dao.GetProjectById(ctx, id(project.Id), TestTenant, models.SlimVerbosity)
		Expect(e).NotTo(HaveOccurred())
		added := p.OrgUnitData.Resources[0]
		added.ID = uuid.New()
		added.NodePoolId = "new"
		added.NodePoolName = "new"
		Expect(dao.AddResources(ctx, []*models.Resources{&added})).To(MatchError(ContainSubstring("pool membership is locked")))
		Expect(dao.DeleteResources(ctx, []models.Resources{p.OrgUnitData.Resources[1]})).To(MatchError(ContainSubstring("pool membership is locked")))
		Expect(dao.ResetResources(ctx, TestTenant, cluster, "", []string{"2"}, api.RESOURCESRESETACTION_QUOTAS_TO_DEFAULT, true)).To(MatchError(dao.ErrKubernetesQuota))
		p.OrgUnitData.Resources = p.OrgUnitData.Resources[:1]
		_, e = dao.UpdateProject(ctx, p, false, false)
		Expect(e).To(MatchError(dao.ErrKubernetesQuota))
	})

	It("imports legacy protection unchanged and explicitly normalizes partial bindings", func() {
		uid := uuid.NewString()
		e := orgconfig.QuotaEnrollment{TenantId: TestTenant, ClusterId: cluster, Kind: "project", Id: id(project.Id), State: "active", KubernetesUid: uid, Epoch: "legacy", Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "default", QueueName: project.Name}}}
		raw, err := json.Marshal([]orgconfig.QuotaEnrollment{e})
		Expect(err).NotTo(HaveOccurred())
		conf := previous
		conf.QuotaPocEnrollments = string(raw)
		commonconfig.Set(&conf)
		_, o, err := dao.QuotaCommandSnapshot(ctx, TestTenant, cluster, "project", id(project.Id))
		Expect(err).NotTo(HaveOccurred())
		Expect(o).To(Equal(e.Ownership()))
		conf.QuotaPocEnrollments = ""
		commonconfig.Set(&conf)
		c, err := dao.GetQuotaControl(ctx, TestTenant, cluster, "project", id(project.Id))
		Expect(err).NotTo(HaveOccurred())
		Expect(c.Epoch).To(Equal("legacy"))
		Expect(c.KubernetesUID).To(Equal(uid))
		Expect(c.Pools).To(HaveLen(1))
		sync, e2 := ps.GetSyncProjects(ctx, cluster, time.Now().Add(time.Hour), 0, 100, true)
		Expect(e2).NotTo(HaveOccurred())
		Expect(sync.Code).To(Equal(http.StatusOK))
		Expect(sync.Body.(api.GetSyncProjects200Response).Projects).To(HaveLen(1))
		m := transfer("project", "kubernetes", 0)
		Expect(m.Revision).To(Equal(int64(1)))
		c, err = dao.GetQuotaControl(ctx, TestTenant, cluster, "project", id(project.Id))
		Expect(err).NotTo(HaveOccurred())
		Expect(c.Pools).To(HaveLen(2))
		Expect(c.Epoch).NotTo(Equal("legacy"))
		Expect(c.KubernetesUID).To(Equal(uid))
		for _, phase := range []string{"freeze", "release", "stable"} {
			s := snapshot("project", 1)
			s.Pools[0].Gpu.Deserved = ptr.To(2.0)
			Expect(s.ControlPhase).To(Equal(phase))
			if phase == "release" {
				wrong := snapshot("project", 0.5)
				Expect(observe("project", wrong)).To(HaveOccurred())
			}
			Expect(observe("project", s)).To(Succeed())
		}
		stored := resources("project")
		for _, r := range stored {
			if r.NodePoolId == "1" {
				Expect(*r.GpuDeserved).To(Equal(2.0))
			} else {
				Expect(*r.GpuDeserved).To(Equal(1.0))
			}
		}
	})
})
