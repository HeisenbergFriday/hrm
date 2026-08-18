package api

import (
	"context"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"

	"gorm.io/gorm"
)

func TestApplyDingTalkProfileFieldsMapsHRMValues(t *testing.T) {
	profile := &database.EmployeeProfile{EntryDate: "2026-08-17", ProbationEndDate: "2026-08-01"}
	applyDingTalkProfileFields(profile, dingtalk.UserInfo{
		Email:              "employee@example.com",
		HiredDate:          "2026-06-01",
		PlannedRegularDate: "2026-09-01",
		ActualRegularDate:  "2026-09-02",
		ProbationEndDate:   "2026-08-31",
		EmploymentType:     "正式",
		EmploymentTypeCode: "A1",
		JobLevel:           "P6",
		JobFamily:          "技术",
	}, "active")

	if profile.EntryDate != "2026-06-01" || profile.PlannedRegularDate != "2026-09-01" || profile.ActualRegularDate != "2026-09-02" {
		t.Fatalf("unexpected profile dates: %#v", profile)
	}
	if profile.ProbationEndDate != "2026-08-31" {
		t.Fatalf("probation end date = %q", profile.ProbationEndDate)
	}
	if profile.EmploymentType != "正式" || profile.EmploymentTypeCode != "A1" || profile.JobLevel != "P6" || profile.JobFamily != "技术" {
		t.Fatalf("unexpected HRM profile fields: %#v", profile)
	}
}

func TestApplyDingTalkProfileFieldsDoesNotOverwriteManualValuesWithEmpty(t *testing.T) {
	profile := &database.EmployeeProfile{
		EntryDate:          "2024-11-11",
		ProbationEndDate:   "2026-08-31",
		EmploymentType:     "正式",
		EmploymentTypeCode: "manual-code",
		JobLevel:           "P6",
		JobFamily:          "技术",
	}
	applyDingTalkProfileFields(profile, dingtalk.UserInfo{ActualRegularDate: "2026-09-02"}, "active")

	if profile.ProbationEndDate != "2026-08-31" {
		t.Fatalf("actual regular date must not overwrite probation end date: %#v", profile)
	}
	if profile.EntryDate != "2024-11-11" {
		t.Fatalf("empty DingTalk hired date overwrote manual entry date: %#v", profile)
	}
	if profile.EmploymentType != "正式" || profile.EmploymentTypeCode != "manual-code" || profile.JobLevel != "P6" || profile.JobFamily != "技术" {
		t.Fatalf("empty DingTalk fields overwrote manual values: %#v", profile)
	}
}

func TestSyncDingTalkUsersReportsHRMPartialFailureAndCoverage(t *testing.T) {
	var createdProfile *database.EmployeeProfile
	deps := orgSyncUserDependencies{
		FindUser: func(string) (*database.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
		CreateUser: func(*database.User) error { return nil },
		UpdateUser: func(*database.User) error { return nil },
		AssignDefaultRole: func(string) (bool, error) {
			return false, nil
		},
		FindProfile: func(string) (*database.EmployeeProfile, error) {
			return nil, gorm.ErrRecordNotFound
		},
		CreateProfile: func(profile *database.EmployeeProfile) error {
			createdProfile = profile
			return nil
		},
		UpdateProfile: func(*database.EmployeeProfile) error { return nil },
	}

	result := syncDingTalkUsers(context.Background(), "org-a", []dingtalk.UserInfo{{
		UserID:             "user-1",
		Name:               "测试员工",
		Email:              "user-1@example.com",
		Mobile:             "13800000000",
		Active:             true,
		HRMFieldSyncStatus: "failed",
	}}, false, deps, "request-1", "org-*", time.Now())

	if result.Status != "partial_failed" || result.HRMFieldStatus != "failed" || result.FailCount != 0 || result.SuccessCount != 1 {
		t.Fatalf("unexpected sync result: %#v", result)
	}
	if result.EmploymentTypeMissingCount != 1 || result.JobLevelMissingCount != 1 || result.JobFamilyMissingCount != 1 || result.RegularizationDateMissingCount != 1 {
		t.Fatalf("unexpected HRM coverage counts: %#v", result)
	}
	if createdProfile == nil {
		t.Fatal("expected employee profile to be created")
	}
}

// TestSyncDingTalkUsersReportsHRMNoFieldsDiagnostic verifies that when the HRM
// API succeeds but returns no target fields, the result status is
// "success_no_fields" with a readable diagnostic message, and the overall
// sync status remains "success" (basic data was synced).
func TestSyncDingTalkUsersReportsHRMNoFieldsDiagnostic(t *testing.T) {
	deps := orgSyncUserDependencies{
		FindUser: func(string) (*database.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
		CreateUser: func(*database.User) error { return nil },
		UpdateUser: func(*database.User) error { return nil },
		AssignDefaultRole: func(string) (bool, error) {
			return false, nil
		},
		FindProfile: func(string) (*database.EmployeeProfile, error) {
			return nil, gorm.ErrRecordNotFound
		},
		CreateProfile: func(*database.EmployeeProfile) error { return nil },
		UpdateProfile: func(*database.EmployeeProfile) error { return nil },
	}

	result := syncDingTalkUsers(context.Background(), "org-a", []dingtalk.UserInfo{{
		UserID:             "user-1",
		Name:               "测试员工",
		Email:              "user-1@example.com",
		Mobile:             "13800000000",
		Active:             true,
		HRMFieldSyncStatus: dingtalk.HRMFieldSyncStatusNoFields,
	}}, false, deps, "request-1", "org-*", time.Now())

	if result.HRMFieldStatus != "success_no_fields" {
		t.Fatalf("hrm field status = %q, want %q", result.HRMFieldStatus, "success_no_fields")
	}
	if result.HRMFieldError == "" {
		t.Fatal("expected non-empty diagnostic message for success_no_fields")
	}
	if result.Status != "success" {
		t.Fatalf("overall status = %q, want %q (basic data synced successfully)", result.Status, "success")
	}
	if result.SuccessCount != 1 {
		t.Fatalf("success count = %d, want 1", result.SuccessCount)
	}
}
