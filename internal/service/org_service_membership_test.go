package service

import (
	"testing"
	"time"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openOrgServiceMembershipDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:org-service-membership-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Department{},
		&database.UserDepartmentMembership{},
		&database.EmployeeProfile{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func seedOrgServiceMembershipEmployee(t *testing.T, db *gorm.DB, orgID, userID, departmentID string) {
	t.Helper()
	seedOrgServiceMembershipEmployeeWithStatus(t, db, orgID, userID, departmentID, "active")
}

func seedOrgServiceMembershipEmployeeWithStatus(t *testing.T, db *gorm.DB, orgID, userID, departmentID, status string) {
	t.Helper()
	user := &database.User{
		OrgID:          orgID,
		UserID:         userID,
		DingTalkUserID: orgID + "-" + userID,
		Name:           orgID + " " + userID,
		Email:          orgID + "-" + userID + "@example.com",
		Mobile:         orgID + "-" + userID,
		DepartmentID:   departmentID,
		Status:         status,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user %s/%s: %v", orgID, userID, err)
	}
	profile := &database.EmployeeProfile{
		OrgID:         orgID,
		UserID:        userID,
		EmployeeID:    orgID + "-" + userID,
		ProfileStatus: status,
	}
	if err := db.Create(profile).Error; err != nil {
		t.Fatalf("seed profile %s/%s: %v", orgID, userID, err)
	}
}

// seedOrgServiceMembershipUserWithoutProfile creates a user without an employee_profile,
// simulating a partially-failed sync. Such users must NOT appear in department counts.
func seedOrgServiceMembershipUserWithoutProfile(t *testing.T, db *gorm.DB, orgID, userID, departmentID string) {
	t.Helper()
	user := &database.User{
		OrgID:          orgID,
		UserID:         userID,
		DingTalkUserID: orgID + "-" + userID,
		Name:           orgID + " " + userID,
		Email:          orgID + "-" + userID + "@example.com",
		Mobile:         orgID + "-" + userID,
		DepartmentID:   departmentID,
		Status:         "active",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user without profile %s/%s: %v", orgID, userID, err)
	}
}

func TestOrgServiceDepartmentMembershipListAndTreeCounts(t *testing.T) {
	db := openOrgServiceMembershipDB(t)
	departments := []database.Department{
		{OrgID: "org-a", DepartmentID: "root", DingTalkDepartmentID: "a-root", Name: "总部"},
		{OrgID: "org-a", DepartmentID: "left", DingTalkDepartmentID: "a-left", Name: "左部门", ParentID: "root"},
		{OrgID: "org-a", DepartmentID: "right", DingTalkDepartmentID: "a-right", Name: "右部门", ParentID: "root"},
		{OrgID: "org-a", DepartmentID: "stale", DingTalkDepartmentID: "a-stale", Name: "旧部门", ParentID: "root"},
		{OrgID: "org-b", DepartmentID: "root", DingTalkDepartmentID: "b-root", Name: "其他总部"},
		{OrgID: "org-b", DepartmentID: "right", DingTalkDepartmentID: "b-right", Name: "其他右部门", ParentID: "root"},
	}
	if err := db.Create(&departments).Error; err != nil {
		t.Fatalf("seed departments: %v", err)
	}

	seedOrgServiceMembershipEmployee(t, db, "org-a", "multi-user", "left")
	seedOrgServiceMembershipEmployee(t, db, "org-a", "right-user", "right")
	seedOrgServiceMembershipEmployee(t, db, "org-a", "legacy-user", "left")
	seedOrgServiceMembershipEmployee(t, db, "org-a", "stale-user", "stale")
	seedOrgServiceMembershipEmployee(t, db, "org-b", "multi-user", "right")

	memberships := []database.UserDepartmentMembership{
		{OrgID: "org-a", UserID: "multi-user", DepartmentID: "left", IsPrimary: true},
		{OrgID: "org-a", UserID: "multi-user", DepartmentID: "right"},
		{OrgID: "org-a", UserID: "right-user", DepartmentID: "right", IsPrimary: true},
		{OrgID: "org-a", UserID: "stale-user", DepartmentID: "left", IsPrimary: true},
		{OrgID: "org-b", UserID: "multi-user", DepartmentID: "right", IsPrimary: true},
	}
	if err := db.Create(&memberships).Error; err != nil {
		t.Fatalf("seed memberships: %v", err)
	}

	svc := NewOrgServiceWithOrgID(db, "org-a")
	rightUsers, total, err := svc.ListEmployees(nil, 1, 20, OrgEmployeeFilters{DepartmentID: "right"})
	if err != nil {
		t.Fatalf("list right employees: %v", err)
	}
	if total != 2 || len(rightUsers) != 2 {
		t.Fatalf("right employees = %#v total=%d, want multi-user and right-user", rightUsers, total)
	}
	found := map[string]bool{}
	for _, user := range rightUsers {
		found[user.UserID] = true
		if user.OrgID != "org-a" {
			t.Fatalf("cross-tenant employee leaked into list: %#v", user)
		}
	}
	if !found["multi-user"] || !found["right-user"] {
		t.Fatalf("right employees = %#v, want multi-user and right-user", rightUsers)
	}

	staleUsers, total, err := svc.ListEmployees(nil, 1, 20, OrgEmployeeFilters{DepartmentID: "stale"})
	if err != nil {
		t.Fatalf("list stale employees: %v", err)
	}
	if total != 0 || len(staleUsers) != 0 {
		t.Fatalf("generated memberships must disable stale User.DepartmentID fallback: %#v total=%d", staleUsers, total)
	}

	tree, err := svc.GetDepartmentTree(nil)
	if err != nil {
		t.Fatalf("get department tree: %v", err)
	}
	if len(tree) != 1 || tree[0].ID != "root" {
		t.Fatalf("tree roots = %#v, want org-a root only", tree)
	}
	root := tree[0]
	children := make(map[string]*OrgDepartmentTreeNode, len(root.Children))
	for _, child := range root.Children {
		children[child.ID] = child
	}
	if children["left"].DirectActiveCount != 3 {
		t.Fatalf("left direct active = %d, want multi + legacy fallback + stale relation", children["left"].DirectActiveCount)
	}
	if children["right"].DirectActiveCount != 2 {
		t.Fatalf("right direct active = %d, want multi + right", children["right"].DirectActiveCount)
	}
	if children["stale"].DirectActiveCount != 0 {
		t.Fatalf("stale direct active = %d, want no fallback after relation generation", children["stale"].DirectActiveCount)
	}
	if root.ActiveCount != 4 {
		t.Fatalf("root active = %d, want four unique org-a employees", root.ActiveCount)
	}
}

// TestOrgServiceDepartmentTreeInactiveEmployee verifies that inactive employees are
// counted in Headcount but excluded from ActiveCount, and that the root rollup
// respects the active/inactive split.
func TestOrgServiceDepartmentTreeInactiveEmployee(t *testing.T) {
	db := openOrgServiceMembershipDB(t)
	departments := []database.Department{
		{OrgID: "org-x", DepartmentID: "root", DingTalkDepartmentID: "x-root", Name: "总部"},
		{OrgID: "org-x", DepartmentID: "child", DingTalkDepartmentID: "x-child", Name: "子部门", ParentID: "root"},
	}
	if err := db.Create(&departments).Error; err != nil {
		t.Fatalf("seed departments: %v", err)
	}

	seedOrgServiceMembershipEmployeeWithStatus(t, db, "org-x", "active-user", "child", "active")
	seedOrgServiceMembershipEmployeeWithStatus(t, db, "org-x", "inactive-user", "child", "inactive")

	memberships := []database.UserDepartmentMembership{
		{OrgID: "org-x", UserID: "active-user", DepartmentID: "child", IsPrimary: true},
		{OrgID: "org-x", UserID: "inactive-user", DepartmentID: "child", IsPrimary: true},
	}
	if err := db.Create(&memberships).Error; err != nil {
		t.Fatalf("seed memberships: %v", err)
	}

	svc := NewOrgServiceWithOrgID(db, "org-x")
	tree, err := svc.GetDepartmentTree(nil)
	if err != nil {
		t.Fatalf("get department tree: %v", err)
	}
	if len(tree) != 1 || tree[0].ID != "root" {
		t.Fatalf("tree roots = %#v, want org-x root only", tree)
	}
	root := tree[0]
	if len(root.Children) != 1 {
		t.Fatalf("root children = %d, want 1", len(root.Children))
	}
	child := root.Children[0]
	if child.DirectActiveCount != 1 {
		t.Fatalf("child direct active = %d, want 1 (only active-user)", child.DirectActiveCount)
	}
	if child.DirectHeadcount != 2 {
		t.Fatalf("child direct headcount = %d, want 2 (active + inactive)", child.DirectHeadcount)
	}
	if child.InactiveCount != 1 {
		t.Fatalf("child inactive = %d, want 1", child.InactiveCount)
	}
	if root.ActiveCount != 1 {
		t.Fatalf("root active = %d, want 1 (only active-user)", root.ActiveCount)
	}
	if root.Headcount != 2 {
		t.Fatalf("root headcount = %d, want 2 (active + inactive)", root.Headcount)
	}
}

// TestOrgServiceDepartmentTreeMultiSubDeptDedup verifies that an employee who
// belongs to two sibling sub-departments is counted only once in the parent's
// rolled-up ActiveCount.
func TestOrgServiceDepartmentTreeMultiSubDeptDedup(t *testing.T) {
	db := openOrgServiceMembershipDB(t)
	departments := []database.Department{
		{OrgID: "org-y", DepartmentID: "root", DingTalkDepartmentID: "y-root", Name: "总部"},
		{OrgID: "org-y", DepartmentID: "sub-a", DingTalkDepartmentID: "y-sub-a", Name: "子A", ParentID: "root"},
		{OrgID: "org-y", DepartmentID: "sub-b", DingTalkDepartmentID: "y-sub-b", Name: "子B", ParentID: "root"},
	}
	if err := db.Create(&departments).Error; err != nil {
		t.Fatalf("seed departments: %v", err)
	}

	// shared-user is in both sub-a and sub-b; exclusive-a is only in sub-a
	seedOrgServiceMembershipEmployee(t, db, "org-y", "shared-user", "sub-a")
	seedOrgServiceMembershipEmployee(t, db, "org-y", "exclusive-a", "sub-a")

	memberships := []database.UserDepartmentMembership{
		{OrgID: "org-y", UserID: "shared-user", DepartmentID: "sub-a", IsPrimary: true},
		{OrgID: "org-y", UserID: "shared-user", DepartmentID: "sub-b"},
		{OrgID: "org-y", UserID: "exclusive-a", DepartmentID: "sub-a", IsPrimary: true},
	}
	if err := db.Create(&memberships).Error; err != nil {
		t.Fatalf("seed memberships: %v", err)
	}

	svc := NewOrgServiceWithOrgID(db, "org-y")
	tree, err := svc.GetDepartmentTree(nil)
	if err != nil {
		t.Fatalf("get department tree: %v", err)
	}
	root := tree[0]
	children := make(map[string]*OrgDepartmentTreeNode, len(root.Children))
	for _, child := range root.Children {
		children[child.ID] = child
	}

	if children["sub-a"].DirectActiveCount != 2 {
		t.Fatalf("sub-a direct active = %d, want 2 (shared + exclusive)", children["sub-a"].DirectActiveCount)
	}
	if children["sub-b"].DirectActiveCount != 1 {
		t.Fatalf("sub-b direct active = %d, want 1 (shared only)", children["sub-b"].DirectActiveCount)
	}
	// Root must deduplicate shared-user: 2 unique active employees, not 3
	if root.ActiveCount != 2 {
		t.Fatalf("root active = %d, want 2 (deduplicated: shared + exclusive)", root.ActiveCount)
	}
}

// TestOrgServiceDepartmentTreeUserWithoutProfile verifies that users without an
// employee_profile are excluded from all department counts (the baseEmployeeQuery
// JOINs employee_profiles).
func TestOrgServiceDepartmentTreeUserWithoutProfile(t *testing.T) {
	db := openOrgServiceMembershipDB(t)
	departments := []database.Department{
		{OrgID: "org-z", DepartmentID: "root", DingTalkDepartmentID: "z-root", Name: "总部"},
		{OrgID: "org-z", DepartmentID: "child", DingTalkDepartmentID: "z-child", Name: "子部门", ParentID: "root"},
	}
	if err := db.Create(&departments).Error; err != nil {
		t.Fatalf("seed departments: %v", err)
	}

	seedOrgServiceMembershipEmployee(t, db, "org-z", "with-profile", "child")
	seedOrgServiceMembershipUserWithoutProfile(t, db, "org-z", "no-profile", "child")

	memberships := []database.UserDepartmentMembership{
		{OrgID: "org-z", UserID: "with-profile", DepartmentID: "child", IsPrimary: true},
		{OrgID: "org-z", UserID: "no-profile", DepartmentID: "child", IsPrimary: true},
	}
	if err := db.Create(&memberships).Error; err != nil {
		t.Fatalf("seed memberships: %v", err)
	}

	svc := NewOrgServiceWithOrgID(db, "org-z")
	tree, err := svc.GetDepartmentTree(nil)
	if err != nil {
		t.Fatalf("get department tree: %v", err)
	}
	root := tree[0]
	child := root.Children[0]
	if child.DirectActiveCount != 1 {
		t.Fatalf("child direct active = %d, want 1 (no-profile excluded)", child.DirectActiveCount)
	}
	if root.ActiveCount != 1 {
		t.Fatalf("root active = %d, want 1 (no-profile excluded)", root.ActiveCount)
	}

	// Employee list must also exclude the no-profile user
	users, total, err := svc.ListEmployees(nil, 1, 20, OrgEmployeeFilters{DepartmentID: "child"})
	if err != nil {
		t.Fatalf("list child employees: %v", err)
	}
	if total != 1 || len(users) != 1 {
		t.Fatalf("child employees = %#v total=%d, want only with-profile", users, total)
	}
	if users[0].UserID != "with-profile" {
		t.Fatalf("child employee = %s, want with-profile", users[0].UserID)
	}
}

// TestOrgServiceDepartmentTreeDeepNestedDedup verifies deduplication across
// a three-level department tree where one employee appears in two grandchildren.
func TestOrgServiceDepartmentTreeDeepNestedDedup(t *testing.T) {
	db := openOrgServiceMembershipDB(t)
	departments := []database.Department{
		{OrgID: "org-d", DepartmentID: "root", DingTalkDepartmentID: "d-root", Name: "总部"},
		{OrgID: "org-d", DepartmentID: "div", DingTalkDepartmentID: "d-div", Name: "事业部", ParentID: "root"},
		{OrgID: "org-d", DepartmentID: "team-1", DingTalkDepartmentID: "d-team-1", Name: "团队一", ParentID: "div"},
		{OrgID: "org-d", DepartmentID: "team-2", DingTalkDepartmentID: "d-team-2", Name: "团队二", ParentID: "div"},
	}
	if err := db.Create(&departments).Error; err != nil {
		t.Fatalf("seed departments: %v", err)
	}

	// shared-user is in both team-1 and team-2; each team also has an exclusive member
	seedOrgServiceMembershipEmployee(t, db, "org-d", "shared-user", "team-1")
	seedOrgServiceMembershipEmployee(t, db, "org-d", "team1-only", "team-1")
	seedOrgServiceMembershipEmployee(t, db, "org-d", "team2-only", "team-2")

	memberships := []database.UserDepartmentMembership{
		{OrgID: "org-d", UserID: "shared-user", DepartmentID: "team-1", IsPrimary: true},
		{OrgID: "org-d", UserID: "shared-user", DepartmentID: "team-2"},
		{OrgID: "org-d", UserID: "team1-only", DepartmentID: "team-1", IsPrimary: true},
		{OrgID: "org-d", UserID: "team2-only", DepartmentID: "team-2", IsPrimary: true},
	}
	if err := db.Create(&memberships).Error; err != nil {
		t.Fatalf("seed memberships: %v", err)
	}

	svc := NewOrgServiceWithOrgID(db, "org-d")
	tree, err := svc.GetDepartmentTree(nil)
	if err != nil {
		t.Fatalf("get department tree: %v", err)
	}
	root := tree[0]
	if len(root.Children) != 1 {
		t.Fatalf("root children = %d, want 1 (div)", len(root.Children))
	}
	div := root.Children[0]
	teamMap := make(map[string]*OrgDepartmentTreeNode, len(div.Children))
	for _, team := range div.Children {
		teamMap[team.ID] = team
	}

	if teamMap["team-1"].DirectActiveCount != 2 {
		t.Fatalf("team-1 direct active = %d, want 2", teamMap["team-1"].DirectActiveCount)
	}
	if teamMap["team-2"].DirectActiveCount != 2 {
		t.Fatalf("team-2 direct active = %d, want 2", teamMap["team-2"].DirectActiveCount)
	}
	// div must deduplicate shared-user: 3 unique, not 4
	if div.ActiveCount != 3 {
		t.Fatalf("div active = %d, want 3 (deduplicated)", div.ActiveCount)
	}
	// root = div (no direct members in root itself)
	if root.ActiveCount != 3 {
		t.Fatalf("root active = %d, want 3", root.ActiveCount)
	}
}

// TestOrgServiceDepartmentTreeLegacyFallbackOnlyNoMembership verifies that
// when an org has zero membership records (never synced with new code),
// all employees fall back to User.DepartmentID and counts are still correct.
func TestOrgServiceDepartmentTreeLegacyFallbackOnlyNoMembership(t *testing.T) {
	db := openOrgServiceMembershipDB(t)
	departments := []database.Department{
		{OrgID: "org-l", DepartmentID: "root", DingTalkDepartmentID: "l-root", Name: "总部"},
		{OrgID: "org-l", DepartmentID: "child", DingTalkDepartmentID: "l-child", Name: "子部门", ParentID: "root"},
	}
	if err := db.Create(&departments).Error; err != nil {
		t.Fatalf("seed departments: %v", err)
	}

	// No memberships at all — simulates an org never synced after the membership feature
	seedOrgServiceMembershipEmployee(t, db, "org-l", "user-1", "child")
	seedOrgServiceMembershipEmployee(t, db, "org-l", "user-2", "child")
	seedOrgServiceMembershipEmployee(t, db, "org-l", "user-3", "root")

	svc := NewOrgServiceWithOrgID(db, "org-l")
	tree, err := svc.GetDepartmentTree(nil)
	if err != nil {
		t.Fatalf("get department tree: %v", err)
	}
	root := tree[0]
	if root.DirectActiveCount != 1 {
		t.Fatalf("root direct active = %d, want 1 (user-3)", root.DirectActiveCount)
	}
	if root.ActiveCount != 3 {
		t.Fatalf("root active = %d, want 3 (all unique employees)", root.ActiveCount)
	}
	if len(root.Children) != 1 {
		t.Fatalf("root children = %d, want 1", len(root.Children))
	}
	child := root.Children[0]
	if child.DirectActiveCount != 2 {
		t.Fatalf("child direct active = %d, want 2 (user-1 + user-2)", child.DirectActiveCount)
	}
}

// TestGetDepartmentTreeMergesOrphanedUsers verifies that employees whose
// DepartmentID points to a non-existent (deleted) department are still counted
// in the root node's headcount after the rollup. This ensures the root
// headcount equals the overview's total_employees.
func TestGetDepartmentTreeMergesOrphanedUsers(t *testing.T) {
	db := openOrgServiceMembershipDB(t)
	departments := []database.Department{
		{OrgID: "org-o", DepartmentID: "root", DingTalkDepartmentID: "o-root", Name: "总部"},
		{OrgID: "org-o", DepartmentID: "child", DingTalkDepartmentID: "o-child", Name: "子部门", ParentID: "root"},
	}
	if err := db.Create(&departments).Error; err != nil {
		t.Fatalf("seed departments: %v", err)
	}

	// Normal employee in "child" department
	seedOrgServiceMembershipEmployee(t, db, "org-o", "normal-user", "child")
	// Orphan employee: DepartmentID points to a department that doesn't exist in the departments table
	seedOrgServiceMembershipEmployee(t, db, "org-o", "orphan-user", "deleted-dept")
	// Another orphan: empty DepartmentID
	seedOrgServiceMembershipEmployeeWithStatus(t, db, "org-o", "empty-dept-user", "", "active")

	memberships := []database.UserDepartmentMembership{
		{OrgID: "org-o", UserID: "normal-user", DepartmentID: "child", IsPrimary: true},
		// orphan-user and empty-dept-user have NO memberships
	}
	if err := db.Create(&memberships).Error; err != nil {
		t.Fatalf("seed memberships: %v", err)
	}

	svc := NewOrgServiceWithOrgID(db, "org-o")
	tree, err := svc.GetDepartmentTree(nil)
	if err != nil {
		t.Fatalf("get department tree: %v", err)
	}
	if len(tree) != 1 {
		t.Fatalf("tree roots = %d, want 1", len(tree))
	}
	root := tree[0]
	if root.VirtualKind != "organization" || root.ID != organizationTreeRootID {
		t.Fatalf("root = %#v, want explicit organization root", root)
	}

	// Root headcount must include all 3 employees (normal + 2 orphans)
	if root.Headcount != 3 {
		t.Fatalf("root headcount = %d, want 3 (normal + orphan + empty-dept)", root.Headcount)
	}
	if root.ActiveCount != 3 {
		t.Fatalf("root active = %d, want 3", root.ActiveCount)
	}

	// The invalid/empty department users are represented explicitly instead of
	// being assigned to the first real root.
	if len(root.Children) != 2 {
		t.Fatalf("root children = %d, want real root + unassigned", len(root.Children))
	}
	var realRoot, unassigned *OrgDepartmentTreeNode
	for _, node := range root.Children {
		if node.VirtualKind == "unassigned" {
			unassigned = node
		} else if node.ID == "root" {
			realRoot = node
		}
	}
	if realRoot == nil || unassigned == nil || unassigned.Headcount != 2 {
		t.Fatalf("organization children = %#v", root.Children)
	}
	if len(realRoot.Children) != 1 {
		t.Fatalf("real root children = %d, want 1", len(realRoot.Children))
	}
	child := realRoot.Children[0]
	if child.DirectActiveCount != 1 {
		t.Fatalf("child direct active = %d, want 1 (normal-user only)", child.DirectActiveCount)
	}

	// Verify consistency: listEmployeeSnapshots (used by both overview and tree)
	// must return the same unique count as the tree root headcount.
	snapshots, err := svc.listEmployeeSnapshots(nil)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	snapshotSeen := make(map[string]struct{}, len(snapshots))
	snapshotActive := 0
	for _, snap := range snapshots {
		if _, dup := snapshotSeen[snap.UserID]; !dup {
			snapshotSeen[snap.UserID] = struct{}{}
			if snap.Status == "active" {
				snapshotActive++
			}
		}
	}
	if len(snapshotSeen) != root.Headcount {
		t.Fatalf("unique snapshot users = %d, tree root headcount = %d, want them equal",
			len(snapshotSeen), root.Headcount)
	}
	if snapshotActive != root.ActiveCount {
		t.Fatalf("unique snapshot active = %d, tree root active = %d, want them equal",
			snapshotActive, root.ActiveCount)
	}
}

// TestBuildOverviewSummaryDeduplicatesUserIDs verifies that the overview
// summary deduplicates snapshots by user_id, even if somehow duplicates appear.
func TestBuildOverviewSummaryDeduplicatesUserIDs(t *testing.T) {
	svc := &OrgService{nowFn: func() time.Time { return time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local) }}
	snapshots := []orgEmployeeSnapshot{
		{UserID: "user-1", Status: "active"},
		{UserID: "user-2", Status: "active"},
		{UserID: "user-1", Status: "active"}, // duplicate
		{UserID: "user-3", Status: "inactive"},
	}
	summary, _ := svc.buildOverviewSummary(snapshots, nil)
	if summary.TotalEmployees != 3 {
		t.Fatalf("total = %d, want 3 (deduplicated)", summary.TotalEmployees)
	}
	if summary.ActiveEmployees != 2 {
		t.Fatalf("active = %d, want 2", summary.ActiveEmployees)
	}
	if summary.InactiveEmployees != 1 {
		t.Fatalf("inactive = %d, want 1", summary.InactiveEmployees)
	}
}

func TestDepartmentTreeOrganizationRootUsesUnifiedEmployeePopulation(t *testing.T) {
	db := openOrgServiceMembershipDB(t)
	departments := []database.Department{
		{OrgID: "org-count", DepartmentID: "root-a", DingTalkDepartmentID: "1", Name: "第一根"},
		{OrgID: "org-count", DepartmentID: "root-b", DingTalkDepartmentID: "2", Name: "第二根"},
	}
	if err := db.Create(&departments).Error; err != nil {
		t.Fatalf("seed departments: %v", err)
	}
	seedOrgServiceMembershipEmployee(t, db, "org-count", "multi", "root-a")
	seedOrgServiceMembershipEmployeeWithStatus(t, db, "org-count", "inactive", "root-a", "inactive")
	seedOrgServiceMembershipEmployee(t, db, "org-count", "normal", "root-b")
	seedOrgServiceMembershipEmployee(t, db, "org-count", "no-department", "")
	seedOrgServiceMembershipEmployee(t, db, "org-count", "orphan-membership", "missing-department")
	seedOrgServiceMembershipEmployee(t, db, "org-count", "admin", "root-a")
	seedOrgServiceMembershipEmployee(t, db, "org-count", "soft-deleted", "root-a")
	if err := db.Where("org_id = ? AND user_id = ?", "org-count", "soft-deleted").Delete(&database.User{}).Error; err != nil {
		t.Fatalf("soft delete fixture: %v", err)
	}
	memberships := []database.UserDepartmentMembership{
		{OrgID: "org-count", UserID: "multi", DepartmentID: "root-a", IsPrimary: true},
		{OrgID: "org-count", UserID: "multi", DepartmentID: "root-b"},
		{OrgID: "org-count", UserID: "inactive", DepartmentID: "root-a", IsPrimary: true},
		{OrgID: "org-count", UserID: "normal", DepartmentID: "root-b", IsPrimary: true},
		{OrgID: "org-count", UserID: "orphan-membership", DepartmentID: "missing-department", IsPrimary: true},
		{OrgID: "org-count", UserID: "admin", DepartmentID: "root-a", IsPrimary: true},
		{OrgID: "org-count", UserID: "soft-deleted", DepartmentID: "root-a", IsPrimary: true},
	}
	if err := db.Create(&memberships).Error; err != nil {
		t.Fatalf("seed memberships: %v", err)
	}

	svc := NewOrgServiceWithOrgID(db, "org-count")
	tree, err := svc.GetDepartmentTree(nil)
	if err != nil {
		t.Fatalf("get tree: %v", err)
	}
	snapshots, err := svc.listEmployeeSnapshots(nil)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	summary, _ := svc.buildOverviewSummary(snapshots, nil)
	if len(tree) != 1 || tree[0].VirtualKind != "organization" {
		t.Fatalf("tree = %#v, want explicit organization root", tree)
	}
	root := tree[0]
	if root.Headcount != 5 || root.ActiveCount != 4 || root.InactiveCount != 1 {
		t.Fatalf("organization counts total=%d active=%d inactive=%d", root.Headcount, root.ActiveCount, root.InactiveCount)
	}
	if root.Headcount != summary.TotalEmployees || root.ActiveCount != summary.ActiveEmployees {
		t.Fatalf("tree total/active = %d/%d overview = %d/%d", root.Headcount, root.ActiveCount, summary.TotalEmployees, summary.ActiveEmployees)
	}
	children := make(map[string]*OrgDepartmentTreeNode)
	for _, child := range root.Children {
		children[child.ID] = child
	}
	if children["root-a"].Headcount != 2 || children["root-b"].Headcount != 2 {
		t.Fatalf("real root counts a=%d b=%d", children["root-a"].Headcount, children["root-b"].Headcount)
	}
	if children[unassignedEmployeesID].Headcount != 2 {
		t.Fatalf("unassigned = %d, want no-department + orphan relation", children[unassignedEmployeesID].Headcount)
	}
}
