package repository

import (
	"errors"
	"testing"
	"time"

	"peopleops/internal/database"
)

// TestDepartmentRepository_WritesEnforceTenantOrg 覆盖三组织下部门写路径的组织一致性。
func TestDepartmentRepository_WritesEnforceTenantOrg(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org+"/create rejects mismatch", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewDepartmentRepositoryWithOrgID(db, org)
			err := repo.Create(&database.Department{DepartmentID: "dept-1", OrgID: otherOrg(org)})
			if !errors.Is(err, ErrOrgMismatch) {
				t.Fatalf("err = %v, want ErrOrgMismatch", err)
			}
		})
		t.Run(org+"/create inherits tenant org", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewDepartmentRepositoryWithOrgID(db, org)
			dept := &database.Department{DepartmentID: "dept-1"}
			_ = repo.Create(dept)
			if dept.OrgID != org {
				t.Fatalf("OrgID = %q, want %q", dept.OrgID, org)
			}
		})
		t.Run(org+"/update rejects mismatch", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewDepartmentRepositoryWithOrgID(db, org)
			err := repo.Update(&database.Department{DepartmentID: "dept-1", OrgID: otherOrg(org)})
			if !errors.Is(err, ErrOrgMismatch) {
				t.Fatalf("err = %v, want ErrOrgMismatch", err)
			}
		})
	}
}

// TestEmployeeRepository_ProfileWritesEnforceTenantOrg 覆盖三组织下员工档案写路径。
func TestEmployeeRepository_ProfileWritesEnforceTenantOrg(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org+"/create rejects mismatch", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewEmployeeRepositoryWithOrgID(db, org)
			err := repo.CreateProfile(&database.EmployeeProfile{UserID: "alice", OrgID: otherOrg(org)})
			if !errors.Is(err, ErrOrgMismatch) {
				t.Fatalf("err = %v, want ErrOrgMismatch", err)
			}
		})
		t.Run(org+"/create inherits tenant org", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewEmployeeRepositoryWithOrgID(db, org)
			profile := &database.EmployeeProfile{UserID: "alice"}
			_ = repo.CreateProfile(profile)
			if profile.OrgID != org {
				t.Fatalf("OrgID = %q, want %q", profile.OrgID, org)
			}
		})
		t.Run(org+"/update rejects mismatch", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewEmployeeRepositoryWithOrgID(db, org)
			err := repo.UpdateProfile(&database.EmployeeProfile{UserID: "alice", OrgID: otherOrg(org)})
			if !errors.Is(err, ErrOrgMismatch) {
				t.Fatalf("err = %v, want ErrOrgMismatch", err)
			}
		})
	}
}

// TestAttendanceRepository_UpsertEnforcesTenantOrg 覆盖三组织下考勤 Upsert 的组织一致性。
func TestAttendanceRepository_UpsertEnforcesTenantOrg(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org+"/rejects cross-org", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewAttendanceRepositoryWithOrgID(db, org)
			err := repo.Upsert(&database.Attendance{
				UserID:    "alice",
				OrgID:     otherOrg(org),
				CheckTime: time.Now(),
				CheckType: "OnDuty",
			})
			if !errors.Is(err, ErrOrgMismatch) {
				t.Fatalf("err = %v, want ErrOrgMismatch", err)
			}
		})
		t.Run(org+"/inherits tenant org", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewAttendanceRepositoryWithOrgID(db, org)
			record := &database.Attendance{
				UserID:    "alice",
				CheckTime: time.Now(),
				CheckType: "OnDuty",
			}
			_ = repo.Upsert(record) // DryRun 会失败，只关心 OrgID 补齐
			if record.OrgID != org {
				t.Fatalf("OrgID = %q, want %q", record.OrgID, org)
			}
		})
	}
}

// TestAttendanceRepository_LegacyEmptyOrgFallsBackToDefault 保留旧构造的迁移期语义
// —— 当且仅当没有租户上下文时，空 OrgID 才落 "default"。新代码请使用 WithOrgID 构造。
func TestAttendanceRepository_LegacyEmptyOrgFallsBackToDefault(t *testing.T) {
	db := newDryRunGORM(t)
	repo := NewAttendanceRepository(db)
	record := &database.Attendance{
		UserID:    "alice",
		CheckTime: time.Now(),
		CheckType: "OnDuty",
	}
	_ = repo.Upsert(record)
	if record.OrgID != "default" {
		t.Fatalf("OrgID = %q, want default", record.OrgID)
	}
}

// otherOrg 从三组织中挑一个与 org 不同的作为伪造的越权目标。
func otherOrg(org string) string {
	switch org {
	case "default":
		return "muteng"
	case "muteng":
		return "xiaotie"
	default:
		return "default"
	}
}
