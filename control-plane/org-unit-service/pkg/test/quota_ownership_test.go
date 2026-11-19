// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package testing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
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
	"github.com/dsx-ai-factory/run-ai/runai-common-packages/runaictx"
	"github.com/dsx-ai-factory/run-ai/runai-common-packages/runaidb"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"k8s.io/utils/ptr"
)

var _ = Describe("Kubernetes quota ownership", func() {
	var ctx context.Context
	var cluster string
	var project api.Project
	var department api.Department
	var projectService api.ProjectsAPIServicer
	var departmentService api.DepartmentsAPIServicer
	var enrollments []orgconfig.QuotaEnrollment
	var previous orgconfig.General
	var pools map[string]clusterapi.NodepoolMinimal
	setEnrollment := func(state string) {
		for i := range enrollments {
			enrollments[i].State = state
		}
		raw, err := json.Marshal(enrollments)
		Expect(err).NotTo(HaveOccurred())
		conf := previous
		conf.QuotaPocEnrollments = string(raw)
		commonconfig.Set(&conf)
	}
	snapshot := func(kind string, generation int64, gpu float64) api.QuotaSnapshot {
		e := enrollments[0]
		if kind == "project" {
			e = enrollments[1]
		}
		value := func(d, l float64) api.QuotaSnapshotResource {
			return api.QuotaSnapshotResource{Deserved: ptr.To(d), Limit: ptr.To(l), OverQuotaWeight: ptr.To(2.0)}
		}
		return api.QuotaSnapshot{Epoch: e.Epoch, KubernetesUid: e.KubernetesUid, Generation: generation, Pools: []api.QuotaSnapshotPool{{NodePoolId: "1", Gpu: value(gpu, 2), Cpu: value(1000, 2000), Memory: value(1500, 2000)}}}
	}
	readProject := func() *models.Project {
		id, err := strconv.ParseInt(project.Id, 10, 64)
		Expect(err).NotTo(HaveOccurred())
		p, err := dao.GetProjectById(ctx, id, TestTenant, models.SlimVerbosity)
		Expect(err).NotTo(HaveOccurred())
		return p
	}
	readDepartment := func() *models.Department {
		id, err := strconv.ParseInt(department.Id, 10, 64)
		Expect(err).NotTo(HaveOccurred())
		d, err := dao.GetDepartmentById(ctx, id, TestTenant, models.SlimVerbosity, nil)
		Expect(err).NotTo(HaveOccurred())
		return d
	}
	resource := func(resources []models.Resources, id string) models.Resources {
		for _, r := range resources {
			if r.NodePoolId == id {
				return r
			}
		}
		Fail("missing resource " + id)
		return models.Resources{}
	}
	apiResource := func(resources []api.Resources, id string) api.Resources {
		for _, r := range resources {
			if r.NodePool != nil && r.NodePool.Id == id {
				return r
			}
		}
		Fail("missing API resource " + id)
		return api.Resources{}
	}
	importSnapshot := func(c context.Context, kind string, s api.QuotaSnapshot) error {
		if kind == "project" {
			p := readProject()
			return services.ImportQuotaSnapshot(c, TestTenant, cluster, kind, p.ID, s, p.OrgUnitData.Resources, pools)
		}
		d := readDepartment()
		return services.ImportQuotaSnapshot(c, TestTenant, cluster, kind, d.ID, s, d.OrgUnitData.Resources, pools)
	}
	BeforeEach(func() {
		previous = *commonconfig.Get[*orgconfig.General]()
		cluster = uuid.NewString()
		ctx = context.WithValue(SetContext(TestTenant, TestUser), authstructs.PermittedScopesKey, &authapi.PermittedScopes{Clusters: []string{cluster}})
		client := &clustermocks.IClusterClient{}
		pools = map[string]clusterapi.NodepoolMinimal{}
		for _, p := range []clusterapi.NodepoolMinimal{{Id: "1", Name: "default", ClusterId: cluster, SchedulingCapabilities: NpSchedCapabilitiesDefault(true, true)}, {Id: "2", Name: "other", ClusterId: cluster, SchedulingCapabilities: NpSchedCapabilitiesDefault(true, true)}} {
			pools[p.Id] = p
		}
		client.On("GetNodePoolsMinimalByCluster", mock.Anything, cluster).Return(&clusterapi.GetNodepoolsMinimal200Response{Nodepools: []clusterapi.NodepoolMinimal{pools["1"], pools["2"]}}, nil)
		client.On("GetClusterMinimalDataById", mock.Anything, cluster).Return(&clusterapi.ClustersMinimalData{Uuid: cluster, Version: *clusterapi.NewNullableString(ptr.To("3.0"))}, nil)
		departmentService = departments.NewDepartmentApiService(client, nil)
		projectService = projects.NewProjectApiService(services.NewProjectDeletionService(client, nil, nil), client, nil)
		request := getDepartmentCreateRequest(createDepartmentName(), cluster)
		request.Resources = []api.ResourcesNullable{CreateNodePoolApiResources(true, "1", "default"), CreateNodePoolApiResources(true, "2", "other")}
		response, err := departmentService.CreateDepartment(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusCreated), fmt.Sprint(response.Body))
		department = response.Body.(api.Department)
		pr := getProjectCreateRequest(createProjectName(), "", department.Id, cluster)
		pr.Resources = request.Resources
		response, err = projectService.CreateProject(ctx, pr)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusCreated), fmt.Sprint(response.Body))
		project = response.Body.(api.Project)
		did, _ := strconv.ParseInt(department.Id, 10, 64)
		pid, _ := strconv.ParseInt(project.Id, 10, 64)
		enrollments = []orgconfig.QuotaEnrollment{
			{TenantId: TestTenant, ClusterId: cluster, Kind: "department", Id: did, KubernetesUid: uuid.NewString(), Epoch: uuid.NewString(), Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "default", QueueName: "department-default"}}},
			{TenantId: TestTenant, ClusterId: cluster, Kind: "project", Id: pid, KubernetesUid: uuid.NewString(), Epoch: uuid.NewString(), Pools: []api.QuotaOwnershipPool{{NodePoolId: "1", NodePoolName: "default", QueueName: "project-default"}}},
		}
		setEnrollment("active")
	})
	AfterEach(func() {
		defer commonconfig.Set(&previous)
		// Use the existing lifecycle DAOs while ownership is still enabled.
		// These fixtures must not leak into suites that count tenant projects.
		if project.Id != "" {
			_, _, err := dao.DeleteProjectInDb(ctx, readProject(), nil, "", false)
			Expect(err).NotTo(HaveOccurred())
		}
		if department.Id != "" {
			id, err := strconv.ParseInt(department.Id, 10, 64)
			Expect(err).NotTo(HaveOccurred())
			_, err = dao.DeleteDepartment(ctx, id, TestTenant, nil, services.DefaultDepartmentName)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("publishes only active trusted ownership and refreshes configuration-only enrollments", func() {
		for _, state := range []string{"prepared", "active"} {
			setEnrollment(state)
			response, err := projectService.GetSyncProjects(ctx, cluster, time.Now().Add(time.Hour), 0, 100, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Code).To(Equal(http.StatusOK), fmt.Sprint(response.Body))
			values := response.Body.(api.GetSyncProjects200Response).Projects
			Expect(values).To(HaveLen(1))
			if state == "prepared" {
				Expect(values[0].QuotaOwnership).To(BeNil())
			} else {
				Expect(values[0].QuotaOwnership).To(Equal(enrollments[1].Ownership()))
			}
			response, err = departmentService.GetSyncDepartments(ctx, cluster, time.Now().Add(time.Hour), 0, 100)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Code).To(Equal(http.StatusOK), fmt.Sprint(response.Body))
			ds := response.Body.(api.GetSyncDepartments200Response).Departments
			Expect(ds).To(HaveLen(1))
			if state == "prepared" {
				Expect(ds[0].QuotaOwnership).To(BeNil())
			} else {
				Expect(ds[0].QuotaOwnership).To(Equal(enrollments[0].Ownership()))
			}
		}
	})

	It("imports through status APIs, preserves other pools, units and ranks, and allows old clients", func() {
		original := resource(readProject().OrgUnitData.Resources, "2")
		ps := snapshot("project", 2, 1.5)
		response, err := projectService.UpdateProjectStatus(ctx, api.ProjectStatusUpdateRequest{Id: project.Id, ClusterId: cluster, Status: api.ProjectStatus{Phase: api.PROJECTPHASE_READY}, QuotaSnapshot: &ps})
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusOK), fmt.Sprint(response.Body))
		r := resource(readProject().OrgUnitData.Resources, "1")
		Expect(*r.GpuDeserved).To(Equal(1.5))
		Expect(*r.CpuDeserved).To(Equal(1000.0))
		Expect(*r.MemoryDeserved).To(Equal(1.5))
		Expect(r.MemoryUnits).To(Equal(models.GB))
		Expect(resource(readProject().OrgUnitData.Resources, "2")).To(Equal(original))
		updated := r.UpdatedAt
		Expect(importSnapshot(ctx, "project", ps)).To(Succeed())
		Expect(resource(readProject().OrgUnitData.Resources, "1").UpdatedAt).To(Equal(updated))
		ps.Generation++
		Expect(importSnapshot(ctx, "project", ps)).To(Succeed())
		Expect(resource(readProject().OrgUnitData.Resources, "1").UpdatedAt).To(Equal(updated))
		ds := snapshot("department", 4, 1.25)
		response, err = departmentService.UpdateDepartmentStatus(ctx, department.Id, api.DepartmentStatusUpdateRequest{ClusterId: cluster, QuotaSnapshot: &ds})
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusNoContent), fmt.Sprint(response.Body))
		Expect(*resource(readDepartment().OrgUnitData.Resources, "1").GpuDeserved).To(Equal(1.25))
		response, err = projectService.GetProject(ctx, project.Id)
		Expect(err).NotTo(HaveOccurred())
		Expect(apiResource(response.Body.(api.Project).Resources, "1").Gpu.Deserved).To(Equal(1.5))
		response, err = projectService.UpdateProjectStatus(ctx, api.ProjectStatusUpdateRequest{Id: project.Id, ClusterId: cluster, Status: api.ProjectStatus{Phase: api.PROJECTPHASE_READY}})
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(*resource(readProject().OrgUnitData.Resources, "1").GpuDeserved).To(Equal(1.5))
	})

	It("rejects stale, conflicting, spoofed, incomplete and prepared snapshots without writes", func() {
		for _, kind := range []string{"project", "department"} {
			good := snapshot(kind, 10, 1.5)
			Expect(importSnapshot(ctx, kind, good)).To(Succeed())
			for _, change := range []func(*api.QuotaSnapshot){func(s *api.QuotaSnapshot) { s.Generation-- }, func(s *api.QuotaSnapshot) { s.Pools[0].Gpu.Deserved = ptr.To(1.75) }, func(s *api.QuotaSnapshot) { s.KubernetesUid = uuid.NewString() }, func(s *api.QuotaSnapshot) { s.Epoch = "old" }, func(s *api.QuotaSnapshot) { s.Pools = nil }, func(s *api.QuotaSnapshot) { s.Pools[0].NodePoolId = "2" }, func(s *api.QuotaSnapshot) { s.Pools = append(s.Pools, s.Pools[0]) }, func(s *api.QuotaSnapshot) { s.Generation++; s.Pools[0].Cpu.Limit = nil }} {
				bad := snapshot(kind, 10, 1.5)
				change(&bad)
				Expect(importSnapshot(ctx, kind, bad)).NotTo(Succeed())
			}
			unauthorized := context.WithValue(ctx, authstructs.PermittedScopesKey, &authapi.PermittedScopes{Clusters: []string{uuid.NewString()}})
			Expect(importSnapshot(unauthorized, kind, snapshot(kind, 11, 1.75))).NotTo(Succeed())
			wrongTenant := context.WithValue(ctx, runaictx.CfTenantId, "another-tenant")
			Expect(importSnapshot(wrongTenant, kind, snapshot(kind, 11, 1.75))).NotTo(Succeed())
			Expect(importSnapshot(SetContext(TestTenant, TestUser), kind, snapshot(kind, 11, 1.75))).NotTo(Succeed())
			setEnrollment("prepared")
			Expect(importSnapshot(ctx, kind, snapshot(kind, 11, 1.75))).NotTo(Succeed())
			setEnrollment("active")
		}
		bad := snapshot("project", 11, 1.75)
		response, err := projectService.UpdateProjectStatus(ctx, api.ProjectStatusUpdateRequest{Id: project.Id, ClusterId: uuid.NewString(), Status: api.ProjectStatus{Phase: api.PROJECTPHASE_READY}, QuotaSnapshot: &bad})
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusForbidden))
		Expect(*resource(readProject().OrgUnitData.Resources, "1").GpuDeserved).To(Equal(1.5))
		Expect(*resource(readDepartment().OrgUnitData.Resources, "1").GpuDeserved).To(Equal(1.5))
	})

	It("mirrors literal numeric leaves even when API editing capabilities are disabled", func() {
		pools["1"].SchedulingCapabilities.CpuQuota = false
		pools["1"].SchedulingCapabilities.AdjustableWeights = false
		s := snapshot("project", 1, 1.5)
		s.Pools[0].Gpu.OverQuotaWeight = ptr.To(3.0)
		Expect(importSnapshot(ctx, "project", s)).To(Succeed())
		response, err := projectService.GetProject(ctx, project.Id)
		Expect(err).NotTo(HaveOccurred())
		value := response.Body.(api.Project)
		Expect(apiResource(value.Resources, "1").Gpu.OverQuotaWeight).To(Equal(ptr.To(3.0)))
		Expect(apiResource(value.Resources, "1").Cpu.Deserved).To(Equal(ptr.To(1000.0)))
		update := getProjectUpdateRequest(value)
		update.Description = "unchanged Kubernetes quotas"
		response, err = projectService.UpdateProject(ctx, project.Id, update)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusOK), fmt.Sprint(response.Body))
		Expect(pools["1"].SchedulingCapabilities.AdjustableWeights).To(BeFalse())
		Expect(pools["1"].SchedulingCapabilities.CpuQuota).To(BeFalse())
	})

	It("allows unchanged full payloads for legacy null memory units", func() {
		p := readProject()
		r := resource(p.OrgUnitData.Resources, "1")
		_, err := runaidb.DB().Table(models.ResourcesTable).Where("id = ?", r.ID).UpdateColumns(map[string]interface{}{"memory_units": nil, "memory_deserved": nil, "memory_limit": nil, "memory_over_quota_weight": nil})
		Expect(err).NotTo(HaveOccurred())
		Expect(importSnapshot(ctx, "project", snapshot("project", 1, 1.5))).To(Succeed())
		response, err := projectService.GetProject(ctx, project.Id)
		Expect(err).NotTo(HaveOccurred())
		update := getProjectUpdateRequest(response.Body.(api.Project))
		update.Description = "legacy units unchanged"
		response, err = projectService.UpdateProject(ctx, project.Id, update)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusOK), fmt.Sprint(response.Body))
	})

	It("requires a new epoch when UID or pool identities change", func() {
		Expect(importSnapshot(ctx, "project", snapshot("project", 3, 1.5))).To(Succeed())
		enrollments[1].KubernetesUid = uuid.NewString()
		setEnrollment("active")
		Expect(importSnapshot(ctx, "project", snapshot("project", 4, 1.75))).NotTo(Succeed())
		enrollments[1].Epoch = uuid.NewString()
		setEnrollment("active")
		Expect(importSnapshot(ctx, "project", snapshot("project", 1, 1.75))).To(Succeed())
		enrollments[1].Pools[0].QueueName = "different-queue"
		setEnrollment("active")
		Expect(importSnapshot(ctx, "project", snapshot("project", 2, 1.5))).NotTo(Succeed())
	})

	It("refuses active ownership and imports under the strict oversubscription policy", func() {
		setLimitQuotaOverSubscription(true)
		defer setLimitQuotaOverSubscription(false)
		Expect(importSnapshot(ctx, "project", snapshot("project", 1, 1.5))).NotTo(Succeed())
		response, err := projectService.GetSyncProjects(ctx, cluster, time.Time{}, 0, 100, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Code).To(Equal(http.StatusConflict))
	})

	It("rolls back the first pool and ordering metadata when a later pool fails", func() {
		enrollments[1].Pools = append(enrollments[1].Pools, api.QuotaOwnershipPool{NodePoolId: "2", NodePoolName: "other", QueueName: "project-other"})
		setEnrollment("active")
		p := readProject()
		before := resource(p.OrgUnitData.Resources, "1")
		s := snapshot("project", 1, 1.5)
		second := s.Pools[0]
		second.NodePoolId = "2"
		s.Pools = append(s.Pools, second)
		converted := append([]models.Resources(nil), p.OrgUnitData.Resources...)
		for i := range converted {
			if converted[i].NodePoolId == "1" {
				converted[i].GpuDeserved = ptr.To(1.5)
			}
			if converted[i].NodePoolId == "2" {
				converted[i].NodePoolName = "changed-after-validation"
			}
		}
		Expect(dao.ImportQuotaSnapshot(ctx, TestTenant, cluster, "project", p.ID, s, converted)).NotTo(Succeed())
		Expect(resource(readProject().OrgUnitData.Resources, "1")).To(Equal(before))
		var count int64
		Expect(runaidb.DB().Table("quota_mirror").Where("org_unit_id = ?", enrollments[1].Id).Count(&count)).To(Succeed())
		Expect(count).To(BeZero())
	})

	It("does not rewrite timestamps for wire precision already rounded by existing resource columns", func() {
		s := snapshot("project", 1, 1.125)
		Expect(importSnapshot(ctx, "project", s)).To(Succeed())
		before := resource(readProject().OrgUnitData.Resources, "1")
		Expect(*before.GpuDeserved).To(Equal(1.13))
		s.Generation++
		Expect(importSnapshot(ctx, "project", s)).To(Succeed())
		Expect(resource(readProject().OrgUnitData.Resources, "1").UpdatedAt).To(Equal(before.UpdatedAt))
	})

	It("protects indirect resets and resource membership without blocking harmless resets", func() {
		s := snapshot("project", 1, 1.5)
		Expect(importSnapshot(ctx, "project", s)).To(Succeed())
		Expect(dao.ResetResources(ctx, TestTenant, cluster, "", []string{"1"}, api.RESOURCESRESETACTION_WEIGHTS_TO_DEFAULT, true)).To(Succeed())
		s.Generation++
		s.Pools[0].Gpu.OverQuotaWeight = ptr.To(3.0)
		Expect(importSnapshot(ctx, "project", s)).To(Succeed())
		other := resource(readProject().OrgUnitData.Resources, "2")
		for _, action := range []api.ResourcesResetAction{api.RESOURCESRESETACTION_QUOTAS_TO_DEFAULT, api.RESOURCESRESETACTION_WEIGHTS_TO_DEFAULT, api.RESOURCESRESETACTION_WEIGHTS_TO_DESERVED} {
			Expect(dao.ResetResources(ctx, TestTenant, cluster, "", []string{"1", "2"}, action, true)).To(MatchError(dao.ErrKubernetesQuota))
		}
		Expect(dao.NodepoolResetResources(ctx, TestTenant, cluster, "1", api.NODEPOOLRESOURCESRESETACTION_CPU_QUOTAS_TO_DEFAULT)).To(MatchError(dao.ErrKubernetesQuota))
		Expect(dao.DisableCpuQuota(ctx, TestTenant, []clusterapi.ClustersMinimalData{{Uuid: cluster}})).To(MatchError(dao.ErrKubernetesQuota))
		Expect(resource(readProject().OrgUnitData.Resources, "2")).To(Equal(other))
		managed := resource(readProject().OrgUnitData.Resources, "1")
		Expect(dao.DeleteResources(ctx, []models.Resources{managed})).To(MatchError(dao.ErrKubernetesQuota))
		idOnly := managed
		idOnly.OrgUnitDataID, idOnly.NodePoolId = uuid.Nil, ""
		Expect(dao.DeleteResources(ctx, []models.Resources{idOnly})).To(MatchError(dao.ErrKubernetesQuota))
		Expect(dao.AddResources(ctx, []*models.Resources{&managed})).To(MatchError(dao.ErrKubernetesQuota))
		Expect(dao.ResetResources(ctx, TestTenant, cluster, "", []string{"1"}, api.RESOURCESRESETACTION_RANKS_TO_DEFAULT, true)).To(Succeed())
		Expect(dao.ResetResources(ctx, TestTenant, cluster, "", []string{"2"}, api.RESOURCESRESETACTION_QUOTAS_TO_DEFAULT, true)).To(Succeed())
		Expect(*resource(readProject().OrgUnitData.Resources, "1").GpuDeserved).To(Equal(1.5))
	})

	It("does not lock unrelated enrollments during membership changes", func() {
		p, d := readProject(), readDepartment()
		blockedCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		blocker := runaidb.DB().WithContext(ctx).Begin()
		defer blocker.Rollback()
		_, err := blocker.Exec("SELECT id FROM "+models.DepartmentTable+" WHERE id = ? AND tenant_id = ? FOR UPDATE", d.ID, TestTenant)
		Expect(err).NotTo(HaveOccurred())
		values := []models.Resources{resource(p.OrgUnitData.Resources, "2")}
		result := make(chan error, 1)
		go func() { result <- dao.DeleteResources(blockedCtx, values) }()
		Expect(<-result).To(Succeed())
	})

	It("orders concurrent membership and CPU-disable locks even for ID-only deletes", func() {
		p, d := readProject(), readDepartment()
		for _, dataID := range []uuid.UUID{p.OrgUnitDataID, d.OrgUnitDataID} {
			_, err := runaidb.DB().Table(models.ResourcesTable).Where("org_unit_data_id = ?", dataID).UpdateColumns(map[string]interface{}{
				"cpu_deserved": nil, "cpu_limit": nil, "cpu_over_quota_weight": nil,
				"memory_deserved": nil, "memory_limit": nil, "memory_over_quota_weight": nil,
			})
			Expect(err).NotTo(HaveOccurred())
		}
		blockedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		blocker := runaidb.DB().WithContext(blockedCtx).Begin()
		defer blocker.Rollback()
		var pid int
		Expect(blocker.Actual().Raw("SELECT pg_backend_pid()").Scan(&pid).Error).To(Succeed())
		_, err := blocker.Exec("SELECT id FROM "+models.ProjectTable+" WHERE id = ? AND tenant_id = ? FOR UPDATE", p.ID, TestTenant)
		Expect(err).NotTo(HaveOccurred())
		values := []models.Resources{resource(d.OrgUnitData.Resources, "2"), resource(p.OrgUnitData.Resources, "2")}
		for i := range values {
			values[i].OrgUnitDataID, values[i].NodePoolId = uuid.Nil, ""
		}
		membership := make(chan error, 1)
		go func() { membership <- dao.DeleteResources(blockedCtx, values) }()
		Eventually(func() int64 {
			var count int64
			Expect(runaidb.DB().Actual().Raw("SELECT count(*) FROM pg_stat_activity WHERE ? = ANY(pg_blocking_pids(pid))", pid).Scan(&count).Error).To(Succeed())
			return count
		}, 5*time.Second, 10*time.Millisecond).Should(Equal(int64(1)))
		// CPU disable locks projects before departments. Membership must not hold
		// the department while it waits for this project's lock.
		_, departmentLockErr := blocker.Exec("SELECT id FROM "+models.DepartmentTable+" WHERE id = ? AND tenant_id = ? FOR UPDATE NOWAIT", d.ID, TestTenant)
		cpu := make(chan error, 1)
		go func() {
			cpu <- dao.DisableCpuQuota(blockedCtx, TestTenant, []clusterapi.ClustersMinimalData{{Uuid: cluster}})
		}()
		Expect(blocker.Rollback()).To(Succeed())
		Expect(<-membership).To(Succeed())
		Expect(<-cpu).To(Succeed())
		Expect(departmentLockErr).NotTo(HaveOccurred())
	})

	It("serializes concurrent generations", func() {
		p := readProject()
		s5, s6 := snapshot("project", 5, 1.5), snapshot("project", 6, 1.75)
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, s := range []api.QuotaSnapshot{s5, s6} {
			wg.Add(1)
			go func(s api.QuotaSnapshot) {
				defer wg.Done()
				results <- services.ImportQuotaSnapshot(ctx, TestTenant, cluster, "project", p.ID, s, p.OrgUnitData.Resources, pools)
			}(s)
		}
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			}
		}
		Expect(successes).To(BeNumerically(">=", 1))
		Expect(*resource(readProject().OrgUnitData.Resources, "1").GpuDeserved).To(Equal(1.75))
		Expect(importSnapshot(ctx, "project", s5)).NotTo(Succeed())
	})

	It("freezes every public writer in both prepared and active states, not unchanged quotas or other pools", func() {
		for _, state := range []string{"prepared", "active"} {
			setEnrollment(state)
			pr := getProjectUpdateRequest(project)
			pr.Description = "unrelated edit"
			response, err := projectService.UpdateProject(ctx, project.Id, pr)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Code).To(Equal(http.StatusOK), fmt.Sprint(response.Body))
			dr := getDepartmentUpdateRequest(department)
			dr.Description = "unrelated edit"
			response, err = departmentService.UpdateDepartment(ctx, department.Id, dr)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Code).To(Equal(http.StatusOK), fmt.Sprint(response.Body))
			var projectPatch, departmentPatch api.ResourcesNullable
			for i := range pr.Resources {
				if pr.Resources[i].NodePool.Id == "1" {
					pr.Resources[i].Gpu.Deserved = 1.5
					projectPatch = pr.Resources[i]
				}
			}
			for i := range dr.Resources {
				if dr.Resources[i].NodePool.Id == "1" {
					dr.Resources[i].Gpu.Deserved = 1.5
					departmentPatch = dr.Resources[i]
				}
			}
			for _, call := range []func() (api.ImplResponse, error){
				func() (api.ImplResponse, error) { return projectService.UpdateProject(ctx, project.Id, pr) },
				func() (api.ImplResponse, error) {
					return projectService.UpdateProjectResources(ctx, project.Id, pr.Resources)
				},
				func() (api.ImplResponse, error) {
					return projectService.PatchProjectResources(ctx, project.Id, []api.ResourcesNullable{projectPatch})
				},
				func() (api.ImplResponse, error) { return departmentService.UpdateDepartment(ctx, department.Id, dr) },
				func() (api.ImplResponse, error) {
					return departmentService.UpdateDepartmentResources(ctx, department.Id, dr.Resources)
				},
				func() (api.ImplResponse, error) {
					return departmentService.PatchDepartmentResources(ctx, department.Id, []api.ResourcesNullable{departmentPatch})
				},
				func() (api.ImplResponse, error) {
					return projectService.ProjectsResetResources(ctx, api.ProjectsResourcesResetRequest{ClusterId: cluster, NodePoolIds: []string{"1"}, Action: api.RESOURCESRESETACTION_QUOTAS_TO_DEFAULT})
				},
				func() (api.ImplResponse, error) {
					return departmentService.DepartmentsResetResources(ctx, api.DepartmentsResourcesResetRequest{ClusterId: cluster, NodePoolIds: []string{"1"}, Action: api.RESOURCESRESETACTION_QUOTAS_TO_DEFAULT})
				},
			} {
				response, err := call()
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Code).To(Equal(http.StatusConflict), fmt.Sprint(response.Body))
				Expect(response.Body.(api.Error).Message).To(ContainSubstring("managed through Kubernetes"))
			}
			patch := api.NodePoolResourcesPatchRequest{OrgUnitId: project.Id, NodePoolId: "1", Resources: &api.ResourcesPerNodePoolNullable{Gpu: &api.ResourcesPerNodePoolNullableGpu{Deserved: ptr.To(1.5)}}}
			response, err = projectService.BatchPatchProjectNodePoolResources(ctx, api.BatchPatchNodePoolResourcesRequest{PatchRequests: []api.NodePoolResourcesPatchRequest{patch}})
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.(api.BatchPatchProjectResourcesResponse).TotalFailed).To(Equal(int32(1)))
			Expect(response.Body.(api.BatchPatchProjectResourcesResponse).Failed[0].Message).To(ContainSubstring("managed through Kubernetes"))
			patch.OrgUnitId = department.Id
			response, err = departmentService.BatchPatchDepartmentNodePoolResources(ctx, api.BatchPatchNodePoolResourcesRequest{PatchRequests: []api.NodePoolResourcesPatchRequest{patch}})
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.(api.BatchPatchDepartmentResourcesResponse).TotalFailed).To(Equal(int32(1)))
			Expect(response.Body.(api.BatchPatchDepartmentResourcesResponse).Failed[0].Message).To(ContainSubstring("managed through Kubernetes"))
			other := CreateNodePoolApiResources(true, "2", "other")
			other.Gpu.Deserved = 1.25
			response, err = projectService.PatchProjectResources(ctx, project.Id, []api.ResourcesNullable{other})
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Code).To(Equal(http.StatusOK), fmt.Sprint(response.Body))
			response, err = departmentService.PatchDepartmentResources(ctx, department.Id, []api.ResourcesNullable{other})
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Code).To(Equal(http.StatusOK), fmt.Sprint(response.Body))
			Expect(*resource(readProject().OrgUnitData.Resources, "1").GpuDeserved).To(Equal(apiResource(project.Resources, "1").Gpu.Deserved))
		}
	})
})
