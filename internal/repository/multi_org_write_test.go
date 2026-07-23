package repository

import (
	"errors"
	"strings"
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

// TestAttendanceRepository_EmptyOrgConstructorFailClosed 无租户上下文时不得静默 default。
func TestAttendanceRepository_EmptyOrgConstructorFailClosed(t *testing.T) {
	db := newDryRunGORM(t)
	repo := NewAttendanceRepository(db)
	record := &database.Attendance{
		UserID:    "alice",
		CheckTime: time.Now(),
		CheckType: "OnDuty",
	}
	err := repo.Upsert(record)
	if err == nil || (!errors.Is(err, ErrMissingOrgID) && !strings.Contains(err.Error(), "missing organization") && !strings.Contains(err.Error(), "orgID required")) {
		t.Fatalf("Upsert err=%v, want missing org", err)
	}
}

// TestEmployeeRepository_LifecycleWritesEnforceTenantOrg 阶段三：转岗/离职/入职写路径组织隔离。
func TestEmployeeRepository_LifecycleWritesEnforceTenantOrg(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org+"/transfer rejects mismatch", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewEmployeeRepositoryWithOrgID(db, org)
			err := repo.CreateTransfer(&database.EmployeeTransfer{
				TransferID: "tf-1",
				OrgID:      otherOrg(org),
			})
			if !errors.Is(err, ErrOrgMismatch) {
				t.Fatalf("err = %v, want ErrOrgMismatch", err)
			}
		})
		t.Run(org+"/transfer inherits tenant org", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewEmployeeRepositoryWithOrgID(db, org)
			tr := &database.EmployeeTransfer{TransferID: "tf-1"}
			_ = repo.CreateTransfer(tr)
			if tr.OrgID != org {
				t.Fatalf("OrgID = %q, want %q", tr.OrgID, org)
			}
		})
		t.Run(org+"/resignation rejects mismatch", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewEmployeeRepositoryWithOrgID(db, org)
			err := repo.CreateResignation(&database.EmployeeResignation{
				ResignationID: "rs-1",
				OrgID:         otherOrg(org),
			})
			if !errors.Is(err, ErrOrgMismatch) {
				t.Fatalf("err = %v, want ErrOrgMismatch", err)
			}
		})
		t.Run(org+"/onboarding rejects mismatch", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewEmployeeRepositoryWithOrgID(db, org)
			err := repo.CreateOnboarding(&database.EmployeeOnboarding{
				OnboardingID: "ob-1",
				EmployeeID:   "E001",
				OrgID:        otherOrg(org),
			})
			if !errors.Is(err, ErrOrgMismatch) {
				t.Fatalf("err = %v, want ErrOrgMismatch", err)
			}
		})
		t.Run(org+"/onboarding inherits tenant org", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewEmployeeRepositoryWithOrgID(db, org)
			ob := &database.EmployeeOnboarding{OnboardingID: "ob-1", EmployeeID: "E001"}
			_ = repo.CreateOnboarding(ob)
			if ob.OrgID != org {
				t.Fatalf("OrgID = %q, want %q", ob.OrgID, org)
			}
		})
	}
}

// TestApprovalRepository_EmptyOrgAndMismatchFailClosed 审批仓储必须严格绑定组织。
func TestApprovalRepository_EmptyOrgAndMismatchFailClosed(t *testing.T) {
	db := newDryRunGORM(t)
	empty := NewApprovalRepository(db)
	if err := empty.Create(&database.Approval{ProcessID: "p1", Title: "t"}); !isMissingOrgErr(err) {
		t.Fatalf("empty create err = %v, want missing-org error", err)
	}
	if _, _, err := empty.FindAll(1, 10, nil); !isMissingOrgErr(err) {
		t.Fatalf("empty find err = %v, want missing-org error", err)
	}

	repo := NewApprovalRepositoryWithOrgID(db, "org-a")
	if err := repo.Create(&database.Approval{ProcessID: "p1", Title: "t", OrgID: "org-b"}); !errors.Is(err, ErrOrgMismatch) {
		t.Fatalf("mismatch create err = %v, want ErrOrgMismatch", err)
	}
	a := &database.Approval{ProcessID: "p1", Title: "t"}
	_ = repo.Create(a)
	if a.OrgID != "org-a" {
		t.Fatalf("OrgID = %q, want org-a", a.OrgID)
	}

	tplEmpty := NewApprovalTemplateRepository(db)
	if err := tplEmpty.Create(&database.ApprovalTemplate{Name: "n"}); !isMissingOrgErr(err) {
		t.Fatalf("empty template create err = %v, want missing-org error", err)
	}
}

// TestDepartmentRepository_EmptyOrgFailClosed 空 org 部门仓储不得全表读写。
func TestDepartmentRepository_EmptyOrgFailClosed(t *testing.T) {
	db := newDryRunGORM(t)
	repo := NewDepartmentRepository(db)
	if err := repo.Create(&database.Department{DepartmentID: "d1", Name: "D"}); !isMissingOrgErr(err) {
		t.Fatalf("empty create err = %v, want missing-org error", err)
	}
	// FindAll with empty org uses WHERE 1=0 (fail-closed). DryRun may surface
	// the query as an error; either empty rows or any error is acceptable.
	if rows, err := repo.FindAll(); err == nil && len(rows) != 0 {
		t.Fatalf("empty FindAll returned %d rows, want 0", len(rows))
	}
	if _, err := repo.FindAllChildDepartmentIDs("d1"); !isMissingOrgErr(err) {
		t.Fatalf("empty child ids err = %v, want missing-org error", err)
	}
}

// TestTalentRepository_WritesEnforceTenantOrg 阶段三：人才分析写路径组织隔离。
func TestTalentRepository_WritesEnforceTenantOrg(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org+"/create rejects mismatch", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewTalentRepositoryWithOrgID(db, org)
			err := repo.Create(&database.TalentAnalysis{UserID: "alice", OrgID: otherOrg(org)})
			if !errors.Is(err, ErrOrgMismatch) {
				t.Fatalf("err = %v, want ErrOrgMismatch", err)
			}
		})
		t.Run(org+"/create inherits tenant org", func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewTalentRepositoryWithOrgID(db, org)
			a := &database.TalentAnalysis{UserID: "alice"}
			_ = repo.Create(a)
			if a.OrgID != org {
				t.Fatalf("OrgID = %q, want %q", a.OrgID, org)
			}
		})
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
