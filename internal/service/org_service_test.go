package service

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupOrgServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("create org test db: %v", err)
	}

	if err := db.AutoMigrate(
		&database.User{},
		&database.Department{},
		&database.EmployeeProfile{},
		&database.EmployeeTransfer{},
		&database.EmployeeResignation{},
		&database.EmployeeOnboarding{},
		&database.OperationLog{},
		&database.DepartmentChangeLog{},
	); err != nil {
		t.Fatalf("migrate org test db: %v", err)
	}

	return db
}

func createOrgDepartment(t *testing.T, db *gorm.DB, departmentID, name, parentID string) {
	t.Helper()
	if err := db.Create(&database.Department{
		DepartmentID: departmentID,
		Name:         name,
		ParentID:     parentID,
	}).Error; err != nil {
		t.Fatalf("create department %s: %v", departmentID, err)
	}
}

func createOrgUser(t *testing.T, db *gorm.DB, user *database.User) *database.User {
	t.Helper()
	if user.Email == "" {
		user.Email = user.UserID + "@example.test"
	}
	if user.Mobile == "" {
		user.Mobile = "mobile-" + user.UserID
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user %s: %v", user.UserID, err)
	}
	return user
}

func createOrgProfile(t *testing.T, db *gorm.DB, profile *database.EmployeeProfile) {
	t.Helper()
	if err := db.Create(profile).Error; err != nil {
		t.Fatalf("create profile %s: %v", profile.UserID, err)
	}
}

func hasOrgWarningType(warnings []OrgWarningItem, warningType string) bool {
	for _, warning := range warnings {
		if warning.Type == warningType {
			return true
		}
	}
	return false
}

func distributionCount(items []OrgDistributionItem, label string) int {
	for _, item := range items {
		if item.Label == label {
			return item.Count
		}
	}
	return 0
}

func TestResolveScopeForUserDefaultsToOwnDepartmentTree(t *testing.T) {
	db := setupOrgServiceTestDB(t)
	createOrgDepartment(t, db, "root", "总部", "")
	createOrgDepartment(t, db, "dept-a", "研发中心", "root")
	createOrgDepartment(t, db, "dept-b", "平台组", "dept-a")

	currentUser := createOrgUser(t, db, &database.User{
		UserID:       "u-scope",
		Name:         "Scope User",
		DepartmentID: "dept-a",
		Status:       "active",
	})

	svc := NewOrgService(db)
	scope, err := svc.ResolveScopeForUser(strconv.FormatUint(uint64(currentUser.ID), 10))
	if err != nil {
		t.Fatalf("resolve scope: %v", err)
	}

	if scope.Mode != "department" {
		t.Fatalf("expected department scope, got %s", scope.Mode)
	}
	if !scope.AllowsDepartment("dept-a") || !scope.AllowsDepartment("dept-b") {
		t.Fatalf("expected scope to include current department and descendants: %#v", scope.DepartmentIDs)
	}
	if scope.AllowsDepartment("root") {
		t.Fatalf("root department should not be visible in narrowed scope: %#v", scope.DepartmentIDs)
	}
}

func TestOrgOverviewCollectsWarningsAndPendingCounts(t *testing.T) {
	db := setupOrgServiceTestDB(t)
	createOrgDepartment(t, db, "dept-a", "研发中心", "")

	createOrgUser(t, db, &database.User{
		UserID:       "user-1",
		Name:         "Alice",
		DepartmentID: "dept-a",
		Status:       "active",
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:             "user-1",
		EmployeeID:         "E-001",
		EntryDate:          "2026-01-10",
		PlannedRegularDate: "2026-05-05",
		ContractEndDate:    "2026-05-10",
		EmploymentType:     "full-time",
		JobLevel:           "P5",
		JobFamily:          "engineering",
		ProfileStatus:      "active",
	})

	createOrgUser(t, db, &database.User{
		UserID:       "user-2",
		Name:         "Bob",
		DepartmentID: "dept-a",
		Status:       "active",
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:            "user-2",
		EmployeeID:        "E-002",
		EntryDate:         "2026-02-15",
		ActualRegularDate: "2026-04-10",
		EmploymentType:    "full-time",
		JobLevel:          "P5",
		JobFamily:         "engineering",
		ProfileStatus:     "active",
	})

	createOrgUser(t, db, &database.User{
		UserID:       "user-3",
		Name:         "Carol",
		DepartmentID: "dept-a",
		Status:       "active",
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:             "user-3",
		EmployeeID:         "E-003",
		EntryDate:          "2026-04-01",
		PlannedRegularDate: "2026-06-15",
		EmploymentType:     "intern",
		JobLevel:           "P3",
		JobFamily:          "design",
		ProfileStatus:      "active",
	})

	if err := db.Create(&database.EmployeeOnboarding{
		OnboardingID:   "onb-1",
		EmployeeID:     "E-001",
		Name:           "Alice",
		DepartmentID:   "dept-a",
		DepartmentName: "研发中心",
		Position:       "工程师",
		EntryDate:      "2026-05-01",
		EmploymentType: "全职",
		Status:         "pending",
	}).Error; err != nil {
		t.Fatalf("create onboarding: %v", err)
	}

	if err := db.Create(&database.EmployeeTransfer{
		TransferID:        "tr-1",
		UserID:            "user-1",
		UserName:          "Alice",
		OldDepartmentID:   "dept-a",
		OldDepartmentName: "研发中心",
		OldPosition:       "工程师",
		NewDepartmentID:   "dept-a",
		NewDepartmentName: "研发中心",
		NewPosition:       "高级工程师",
		TransferDate:      "2026-05-03",
		Status:            "pending",
	}).Error; err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	if err := db.Create(&database.EmployeeResignation{
		ResignationID:  "res-1",
		UserID:         "user-1",
		UserName:       "Alice",
		DepartmentID:   "dept-a",
		DepartmentName: "研发中心",
		Position:       "工程师",
		ResignDate:     "2026-05-15",
		LastWorkingDay: "2026-05-20",
		Status:         "pending",
	}).Error; err != nil {
		t.Fatalf("create resignation: %v", err)
	}

	svc := NewOrgService(db)
	svc.nowFn = func() time.Time {
		return time.Date(2026, 4, 29, 10, 0, 0, 0, time.Local)
	}

	scope := &OrgDataScope{Mode: "all", all: true}
	scope.init()
	overview, err := svc.GetOverview(scope, "")
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if overview.Summary.TotalEmployees != 3 {
		t.Fatalf("expected 3 employees, got %d", overview.Summary.TotalEmployees)
	}
	if overview.Summary.ActiveEmployees != 3 {
		t.Fatalf("expected 3 active employees, got %d", overview.Summary.ActiveEmployees)
	}
	if overview.Summary.ProbationEmployeeCount != 2 {
		t.Fatalf("expected 2 probation employees, got %d", overview.Summary.ProbationEmployeeCount)
	}
	if overview.Summary.ProbationDueCount != 1 {
		t.Fatalf("expected 1 probation warning, got %d", overview.Summary.ProbationDueCount)
	}
	if overview.Summary.PlannedRegularizationCount != 1 {
		t.Fatalf("expected 1 planned regularization warning, got %d", overview.Summary.PlannedRegularizationCount)
	}
	if overview.Summary.ContractExpiringCount != 1 {
		t.Fatalf("expected 1 contract warning, got %d", overview.Summary.ContractExpiringCount)
	}
	if overview.Summary.PendingOnboardingCount != 1 || overview.Summary.PendingTransferCount != 1 || overview.Summary.PendingResignationCount != 1 {
		t.Fatalf("unexpected pending counts: %+v", overview.Summary)
	}
	if distributionCount(overview.EmployeeTypeDistribution, "full-time") != 2 || distributionCount(overview.EmployeeTypeDistribution, "intern") != 1 {
		t.Fatalf("unexpected employment type distribution: %+v", overview.EmployeeTypeDistribution)
	}
	if distributionCount(overview.JobLevelDistribution, "P5") != 2 || distributionCount(overview.JobLevelDistribution, "P3") != 1 {
		t.Fatalf("unexpected job level distribution: %+v", overview.JobLevelDistribution)
	}
	if distributionCount(overview.JobFamilyDistribution, "engineering") != 2 || distributionCount(overview.JobFamilyDistribution, "design") != 1 {
		t.Fatalf("unexpected job family distribution: %+v", overview.JobFamilyDistribution)
	}
	if len(overview.Warnings) < 3 {
		t.Fatalf("expected multiple warnings, got %d", len(overview.Warnings))
	}
}

func TestDepartmentTreeAndOverviewSupportDepartmentLevelStats(t *testing.T) {
	db := setupOrgServiceTestDB(t)
	createOrgDepartment(t, db, "root", "HQ", "")
	createOrgDepartment(t, db, "dept-a", "DeptA", "root")
	createOrgDepartment(t, db, "dept-a-1", "DeptAChild", "dept-a")
	createOrgDepartment(t, db, "dept-b", "DeptB", "root")

	createOrgUser(t, db, &database.User{
		UserID:       "dept-a-active",
		Name:         "Alice",
		DepartmentID: "dept-a",
		Status:       "active",
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:             "dept-a-active",
		EmployeeID:         "E-101",
		PlannedRegularDate: "2026-05-08",
		ProfileStatus:      "active",
	})

	createOrgUser(t, db, &database.User{
		UserID:       "dept-a-child-active",
		Name:         "Bob",
		DepartmentID: "dept-a-1",
		Status:       "active",
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:             "dept-a-child-active",
		EmployeeID:         "E-102",
		PlannedRegularDate: "2026-05-20",
		ProfileStatus:      "active",
	})

	createOrgUser(t, db, &database.User{
		UserID:       "dept-a-child-inactive",
		Name:         "Carol",
		DepartmentID: "dept-a-1",
		Status:       "inactive",
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:        "dept-a-child-inactive",
		EmployeeID:    "E-104",
		ProfileStatus: "inactive",
	})

	createOrgUser(t, db, &database.User{
		UserID:       "dept-b-active",
		Name:         "David",
		DepartmentID: "dept-b",
		Status:       "active",
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:             "dept-b-active",
		EmployeeID:         "E-103",
		PlannedRegularDate: "2026-05-12",
		ProfileStatus:      "active",
	})

	svc := NewOrgService(db)
	svc.nowFn = func() time.Time {
		return time.Date(2026, 4, 30, 9, 0, 0, 0, time.Local)
	}

	scope := &OrgDataScope{Mode: "all", all: true}
	scope.init()

	tree, err := svc.GetDepartmentTree(scope)
	if err != nil {
		t.Fatalf("get department tree: %v", err)
	}

	nodeByID := make(map[string]*OrgDepartmentTreeNode)
	var walk func(nodes []*OrgDepartmentTreeNode)
	walk = func(nodes []*OrgDepartmentTreeNode) {
		for _, node := range nodes {
			nodeByID[node.ID] = node
			if len(node.Children) > 0 {
				walk(node.Children)
			}
		}
	}
	walk(tree)

	deptA := nodeByID["dept-a"]
	if deptA == nil {
		t.Fatalf("expected dept-a node in tree")
	}
	if deptA.DirectHeadcount != 1 || deptA.DirectActiveCount != 1 {
		t.Fatalf("unexpected direct counts for dept-a: %+v", deptA)
	}
	if deptA.Headcount != 3 || deptA.ActiveCount != 2 || deptA.InactiveCount != 1 {
		t.Fatalf("unexpected rollup counts for dept-a: %+v", deptA)
	}

	deptAChild := nodeByID["dept-a-1"]
	if deptAChild == nil {
		t.Fatalf("expected dept-a-1 node in tree")
	}
	if deptAChild.DirectHeadcount != 2 || deptAChild.DirectActiveCount != 1 {
		t.Fatalf("unexpected direct counts for dept-a-1: %+v", deptAChild)
	}

	overview, err := svc.GetOverview(scope, "dept-a")
	if err != nil {
		t.Fatalf("get overview by department: %v", err)
	}

	if overview.Summary.TotalEmployees != 3 {
		t.Fatalf("expected 3 employees in dept-a subtree, got %d", overview.Summary.TotalEmployees)
	}
	if overview.Summary.ActiveEmployees != 2 {
		t.Fatalf("expected 2 active employees in dept-a subtree, got %d", overview.Summary.ActiveEmployees)
	}
	if overview.Summary.ProbationEmployeeCount != 2 {
		t.Fatalf("expected 2 probation employees in dept-a subtree, got %d", overview.Summary.ProbationEmployeeCount)
	}
	if overview.Summary.PlannedRegularizationCount != 2 {
		t.Fatalf("expected 2 planned regularization warnings in dept-a subtree, got %d", overview.Summary.PlannedRegularizationCount)
	}
	if overview.Scope == nil || len(overview.Scope.RootDepartmentIDs) != 1 || overview.Scope.RootDepartmentIDs[0] != "dept-a" {
		t.Fatalf("unexpected overview scope: %+v", overview.Scope)
	}
}

func TestGetEmployeeAggregateBuildsTimelineAndOrgRelation(t *testing.T) {
	db := setupOrgServiceTestDB(t)
	createOrgDepartment(t, db, "dept-a", "研发中心", "")

	manager := createOrgUser(t, db, &database.User{
		UserID:       "mgr-1",
		Name:         "Manager",
		DepartmentID: "dept-a",
		Position:     "经理",
		Status:       "active",
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:        "mgr-1",
		EmployeeID:    "M-001",
		ProfileStatus: "active",
	})

	employee := createOrgUser(t, db, &database.User{
		UserID:       "emp-1",
		Name:         "Bob",
		DepartmentID: "dept-a",
		Position:     "工程师",
		Status:       "active",
		Extension: map[string]interface{}{
			"manager_user_id": "mgr-1",
		},
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:            "emp-1",
		EmployeeID:        "E-002",
		EntryDate:         "2026-01-05",
		ActualRegularDate: "2026-04-05",
		ProfileStatus:     "active",
	})

	createOrgUser(t, db, &database.User{
		UserID:       "report-1",
		Name:         "Carol",
		DepartmentID: "dept-a",
		Position:     "工程师",
		Status:       "active",
		Extension: map[string]interface{}{
			"manager_user_id": "emp-1",
		},
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:        "report-1",
		EmployeeID:    "E-003",
		ProfileStatus: "active",
	})

	if err := db.Create(&database.EmployeeTransfer{
		TransferID:        "tr-2",
		UserID:            "emp-1",
		UserName:          "Bob",
		OldDepartmentID:   "dept-a",
		OldDepartmentName: "研发中心",
		OldPosition:       "工程师",
		NewDepartmentID:   "dept-a",
		NewDepartmentName: "研发中心",
		NewPosition:       "高级工程师",
		TransferDate:      "2026-04-20",
		Status:            "approved",
	}).Error; err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	if err := db.Create(&database.OperationLog{
		UserID:    strconv.FormatUint(uint64(manager.ID), 10),
		UserName:  "Manager",
		Operation: "employee.profile.updated",
		Resource:  "employee_profile:emp-1",
		IP:        "127.0.0.1",
	}).Error; err != nil {
		t.Fatalf("create audit log: %v", err)
	}

	svc := NewOrgService(db)
	scope := &OrgDataScope{Mode: "all", all: true}
	scope.init()

	detail, err := svc.GetEmployeeAggregate(scope, strconv.FormatUint(uint64(employee.ID), 10))
	if err != nil {
		t.Fatalf("get employee aggregate: %v", err)
	}

	if detail.OrgRelation.Manager == nil || detail.OrgRelation.Manager.UserID != "mgr-1" {
		t.Fatalf("expected manager relation, got %+v", detail.OrgRelation.Manager)
	}
	if len(detail.OrgRelation.DirectReports) != 1 || detail.OrgRelation.DirectReports[0].UserID != "report-1" {
		t.Fatalf("expected one direct report, got %+v", detail.OrgRelation.DirectReports)
	}
	if len(detail.Timeline) < 3 {
		t.Fatalf("expected timeline events, got %d", len(detail.Timeline))
	}

	var transferEvent *EmployeeTimelineEvent
	for i := range detail.Timeline {
		if detail.Timeline[i].Type == "transfer" {
			transferEvent = &detail.Timeline[i]
			break
		}
	}
	if transferEvent == nil {
		t.Fatalf("expected transfer timeline event, got %+v", detail.Timeline)
	}
	if transferEvent.From == nil || transferEvent.To == nil {
		t.Fatalf("expected transfer timeline from/to fields, got %+v", transferEvent)
	}
	if transferEvent.From.DepartmentName != "研发中心" || transferEvent.From.Position != "工程师" {
		t.Fatalf("unexpected transfer from endpoint: %+v", transferEvent.From)
	}
	if transferEvent.To.DepartmentName != "研发中心" || transferEvent.To.Position != "高级工程师" {
		t.Fatalf("unexpected transfer to endpoint: %+v", transferEvent.To)
	}
}

func TestBuildWarnings_ContinuousResignation(t *testing.T) {
	db := setupOrgServiceTestDB(t)
	createOrgDepartment(t, db, "dept-a", "研发中心", "")

	for i := 1; i <= 3; i++ {
		if err := db.Create(&database.EmployeeResignation{
			ResignationID:  "res-cont-" + strconv.Itoa(i),
			UserID:         "res-user-" + strconv.Itoa(i),
			UserName:       "离职员工" + strconv.Itoa(i),
			DepartmentID:   "dept-a",
			DepartmentName: "研发中心",
			Position:       "工程师",
			ResignDate:     "2026-04-" + strconv.Itoa(10+i),
			LastWorkingDay: "2026-04-" + strconv.Itoa(15+i),
			Status:         "completed",
		}).Error; err != nil {
			t.Fatalf("create resignation %d: %v", i, err)
		}
	}

	svc := NewOrgService(db)
	svc.nowFn = func() time.Time {
		return time.Date(2026, 4, 29, 10, 0, 0, 0, time.Local)
	}
	scope := &OrgDataScope{Mode: "all", all: true}
	scope.init()

	overview, err := svc.GetOverview(scope, "")
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}
	if overview.Summary.ConsecutiveResignationCount != 1 {
		t.Fatalf("expected 1 continuous resignation department, got %d", overview.Summary.ConsecutiveResignationCount)
	}
	if !hasOrgWarningType(overview.Warnings, "consecutive_resignation") {
		t.Fatalf("expected consecutive resignation warning, got %+v", overview.Warnings)
	}
}

func TestBuildWarnings_ManagementSpan(t *testing.T) {
	db := setupOrgServiceTestDB(t)
	createOrgDepartment(t, db, "dept-a", "研发中心", "")

	createOrgUser(t, db, &database.User{
		UserID:       "mgr-span",
		Name:         "Manager",
		DepartmentID: "dept-a",
		Position:     "经理",
		Status:       "active",
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:        "mgr-span",
		EmployeeID:    "M-SPAN",
		ProfileStatus: "active",
	})

	for i := 1; i <= 10; i++ {
		userID := "report-span-" + strconv.Itoa(i)
		createOrgUser(t, db, &database.User{
			UserID:       userID,
			Name:         "Report " + strconv.Itoa(i),
			DepartmentID: "dept-a",
			Position:     "工程师",
			Status:       "active",
			Extension: map[string]interface{}{
				"manager_user_id": "mgr-span",
			},
		})
		createOrgProfile(t, db, &database.EmployeeProfile{
			UserID:        userID,
			EmployeeID:    "R-SPAN-" + strconv.Itoa(i),
			ProfileStatus: "active",
		})
	}

	svc := NewOrgService(db)
	scope := &OrgDataScope{Mode: "all", all: true}
	scope.init()

	overview, err := svc.GetOverview(scope, "")
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}
	if overview.Summary.OverspanManagerCount != 1 {
		t.Fatalf("expected 1 overspan manager, got %d", overview.Summary.OverspanManagerCount)
	}
	if !hasOrgWarningType(overview.Warnings, "overspan_manager") {
		t.Fatalf("expected overspan manager warning, got %+v", overview.Warnings)
	}
}

func TestBuildTimeline_TransferArrows(t *testing.T) {
	db := setupOrgServiceTestDB(t)
	createOrgDepartment(t, db, "dept-a", "研发中心", "")
	createOrgDepartment(t, db, "dept-b", "产品中心", "")

	employee := createOrgUser(t, db, &database.User{
		UserID:       "emp-transfer",
		Name:         "Transfer User",
		DepartmentID: "dept-b",
		Position:     "产品经理",
		Status:       "active",
	})
	createOrgProfile(t, db, &database.EmployeeProfile{
		UserID:        "emp-transfer",
		EmployeeID:    "E-TRANSFER",
		ProfileStatus: "active",
	})
	if err := db.Create(&database.EmployeeTransfer{
		TransferID:        "tr-arrow",
		UserID:            "emp-transfer",
		UserName:          "Transfer User",
		OldDepartmentID:   "dept-a",
		OldDepartmentName: "研发中心",
		OldPosition:       "工程师",
		NewDepartmentID:   "dept-b",
		NewDepartmentName: "产品中心",
		NewPosition:       "产品经理",
		TransferDate:      "2026-04-21",
		Reason:            "组织调整",
		Status:            "approved",
	}).Error; err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	svc := NewOrgService(db)
	scope := &OrgDataScope{Mode: "all", all: true}
	scope.init()
	detail, err := svc.GetEmployeeAggregate(scope, strconv.FormatUint(uint64(employee.ID), 10))
	if err != nil {
		t.Fatalf("get employee aggregate: %v", err)
	}

	var transferEvent *EmployeeTimelineEvent
	for i := range detail.Timeline {
		if detail.Timeline[i].Type == "transfer" {
			transferEvent = &detail.Timeline[i]
			break
		}
	}
	if transferEvent == nil || transferEvent.From == nil || transferEvent.To == nil {
		t.Fatalf("expected transfer event with arrows, got %+v", detail.Timeline)
	}
	if transferEvent.From.DepartmentName != "研发中心" || transferEvent.To.DepartmentName != "产品中心" {
		t.Fatalf("unexpected transfer departments: from=%+v to=%+v", transferEvent.From, transferEvent.To)
	}
	if transferEvent.From.Position != "工程师" || transferEvent.To.Position != "产品经理" {
		t.Fatalf("unexpected transfer positions: from=%+v to=%+v", transferEvent.From, transferEvent.To)
	}
}

func TestDepartmentChangeLog_OnSync(t *testing.T) {
	db := setupOrgServiceTestDB(t)
	createOrgDepartment(t, db, "root", "总部", "")
	createOrgDepartment(t, db, "dept-a", "研发中心", "root")

	svc := NewOrgService(db)
	svc.nowFn = func() time.Time {
		return time.Date(2026, 4, 30, 9, 0, 0, 0, time.Local)
	}

	result, err := svc.SyncDepartmentsWithChangeLog([]OrgDepartmentSyncItem{
		{DepartmentID: "dept-a", Name: "研发平台", ParentID: "root"},
	}, "dingtalk_sync")
	if err != nil {
		t.Fatalf("sync departments: %v", err)
	}
	if result.Count != 1 || result.ChangeLogCount != 1 {
		t.Fatalf("unexpected sync result: %+v", result)
	}

	history, err := svc.GetDepartmentHistory(nil, "dept-a", 10)
	if err != nil {
		t.Fatalf("get department history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 change log, got %+v", history)
	}
	if history[0].FieldName != "name" || history[0].OldValue != "研发中心" || history[0].NewValue != "研发平台" {
		t.Fatalf("unexpected change log: %+v", history[0])
	}
}

func TestGetEmployees_ScopeFilter(t *testing.T) {
	db := setupOrgServiceTestDB(t)
	createOrgDepartment(t, db, "root", "总部", "")
	createOrgDepartment(t, db, "dept-a", "研发中心", "root")
	createOrgDepartment(t, db, "dept-a-child", "平台组", "dept-a")
	createOrgDepartment(t, db, "dept-b", "市场部", "root")

	for _, item := range []struct {
		userID       string
		departmentID string
	}{
		{"user-a", "dept-a"},
		{"user-child", "dept-a-child"},
		{"user-b", "dept-b"},
	} {
		createOrgUser(t, db, &database.User{
			UserID:       item.userID,
			Name:         item.userID,
			DepartmentID: item.departmentID,
			Position:     "专员",
			Status:       "active",
		})
		createOrgProfile(t, db, &database.EmployeeProfile{
			UserID:        item.userID,
			EmployeeID:    "E-" + item.userID,
			ProfileStatus: "active",
		})
	}

	scope := &OrgDataScope{
		Mode:          "department",
		DepartmentIDs: []string{"dept-a", "dept-a-child"},
	}
	scope.init()
	svc := NewOrgService(db)

	users, total, err := svc.ListEmployees(scope, 1, 10, OrgEmployeeFilters{})
	if err != nil {
		t.Fatalf("list scoped employees: %v", err)
	}
	if total != 2 || len(users) != 2 {
		t.Fatalf("expected 2 scoped employees, got total=%d users=%+v", total, users)
	}
	for _, user := range users {
		if user.DepartmentID == "dept-b" {
			t.Fatalf("out-of-scope user leaked into result: %+v", user)
		}
	}

	_, _, err = svc.ListEmployees(scope, 1, 10, OrgEmployeeFilters{DepartmentID: "dept-b"})
	if !errors.Is(err, ErrOrgAccessDenied) {
		t.Fatalf("expected access denied for out-of-scope department, got %v", err)
	}
}
