package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupPerformanceServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.PerformanceActivity{},
		&database.PerformanceIndicatorLibrary{},
		&database.PerformanceParticipant{},
		&database.PerformanceReviewVersion{},
		&database.PerformanceGoalRecord{},
		&database.PerformanceGoalApprovalLog{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	return db
}

func TestActivityIncludesUserScopePriority(t *testing.T) {
	user := database.User{
		UserID:       "employee-1",
		DepartmentID: "dept-a",
		Status:       "active",
	}

	tests := []struct {
		name     string
		activity database.PerformanceActivity
		want     bool
	}{
		{
			name: "employee scope wins over department scope",
			activity: database.PerformanceActivity{
				TargetDepartmentIDs: []string{"dept-a"},
				TargetEmployeeIDs:   []string{"employee-2"},
			},
			want: false,
		},
		{
			name: "included by employee scope",
			activity: database.PerformanceActivity{
				TargetDepartmentIDs: []string{"dept-b"},
				TargetEmployeeIDs:   []string{"employee-1"},
			},
			want: true,
		},
		{
			name: "department scope applies when no employees selected",
			activity: database.PerformanceActivity{
				TargetDepartmentIDs: []string{"dept-a"},
			},
			want: true,
		},
		{
			name: "all active users when no scope selected",
			activity: database.PerformanceActivity{
				TargetDepartmentIDs: []string{},
				TargetEmployeeIDs:   []string{},
			},
			want: true,
		},
		{
			name: "blank scope values do not narrow to nobody",
			activity: database.PerformanceActivity{
				TargetDepartmentIDs: []string{" "},
				TargetEmployeeIDs:   []string{""},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := activityIncludesUser(&tt.activity, user); got != tt.want {
				t.Fatalf("activityIncludesUser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateActivityValidatesIndicatorLibraryCycle(t *testing.T) {
	db := setupPerformanceServiceTestDB(t)
	svc := NewPerformanceService(db)

	library := database.PerformanceIndicatorLibrary{
		DepartmentID:   "dept-a",
		DepartmentName: "dept-a",
		Name:           "月度指标库",
		DefaultCycle:   "monthly",
		Status:         "active",
	}
	if err := db.Create(&library).Error; err != nil {
		t.Fatalf("seed indicator library: %v", err)
	}

	if _, err := svc.CreateActivity(CreateActivityRequest{
		Name:                 "季度绩效",
		CycleType:            "quarterly",
		StartDate:            "2026-04-01",
		EndDate:              "2026-06-30",
		SelfEvalStartAt:      "2026-07-01",
		SelfEvalEndAt:        "2026-07-03",
		ManagerEvalStartAt:   "2026-07-04",
		ManagerEvalEndAt:     "2026-07-06",
		ResultConfirmStartAt: "2026-07-07",
		ResultConfirmEndAt:   "2026-07-08",
		Status:               "draft",
		IndicatorLibraryID:   &library.ID,
	}, "tester"); err == nil || !strings.Contains(err.Error(), "指标库周期与活动周期不一致") {
		t.Fatalf("CreateActivity() error = %v, want cycle mismatch", err)
	}

	activity, err := svc.CreateActivity(CreateActivityRequest{
		Name:                 "月度绩效",
		CycleType:            "monthly",
		StartDate:            "2026-05-01",
		EndDate:              "2026-05-31",
		SelfEvalStartAt:      "2026-06-01",
		SelfEvalEndAt:        "2026-06-03",
		ManagerEvalStartAt:   "2026-06-04",
		ManagerEvalEndAt:     "2026-06-06",
		ResultConfirmStartAt: "2026-06-07",
		ResultConfirmEndAt:   "2026-06-08",
		Status:               "draft",
		IndicatorLibraryID:   &library.ID,
	}, "tester")
	if err != nil {
		t.Fatalf("CreateActivity() matching cycle error = %v", err)
	}
	if activity.IndicatorLibraryID == nil || *activity.IndicatorLibraryID != library.ID {
		t.Fatalf("activity indicator library = %v, want %d", activity.IndicatorLibraryID, library.ID)
	}
}

func seedPerformanceActivity(t *testing.T, db *gorm.DB, activity database.PerformanceActivity) database.PerformanceActivity {
	t.Helper()
	if activity.Name == "" {
		activity.Name = fmt.Sprintf("activity-%d", time.Now().UnixNano())
	}
	if activity.CycleType == "" {
		activity.CycleType = "monthly"
	}
	if activity.StartDate == "" {
		activity.StartDate = "2026-05-01"
	}
	if activity.EndDate == "" {
		activity.EndDate = "2026-05-31"
	}
	if activity.SelfEvalStartAt == "" {
		activity.SelfEvalStartAt = "2026-06-01"
	}
	if activity.SelfEvalEndAt == "" {
		activity.SelfEvalEndAt = "2026-06-02"
	}
	if activity.ManagerEvalStartAt == "" {
		activity.ManagerEvalStartAt = "2026-06-03"
	}
	if activity.ManagerEvalEndAt == "" {
		activity.ManagerEvalEndAt = "2026-06-04"
	}
	if activity.ResultConfirmStartAt == "" {
		activity.ResultConfirmStartAt = "2026-06-05"
	}
	if activity.ResultConfirmEndAt == "" {
		activity.ResultConfirmEndAt = "2026-06-06"
	}
	if activity.Status == "" {
		activity.Status = "draft"
	}
	if err := db.Create(&activity).Error; err != nil {
		t.Fatalf("seed activity: %v", err)
	}
	return activity
}

func seedPerformanceParticipant(t *testing.T, db *gorm.DB, participant database.PerformanceParticipant) database.PerformanceParticipant {
	t.Helper()
	if participant.ActivityID == "" {
		t.Fatal("participant activity id is required")
	}
	if participant.EmployeeID == "" {
		participant.EmployeeID = fmt.Sprintf("employee-%d", time.Now().UnixNano())
	}
	if participant.EmployeeName == "" {
		participant.EmployeeName = participant.EmployeeID
	}
	if participant.DepartmentID == "" {
		participant.DepartmentID = "dept-a"
	}
	if participant.DepartmentName == "" {
		participant.DepartmentName = "dept-a"
	}
	if participant.Status == "" {
		participant.Status = "pending"
	}
	if err := db.Create(&participant).Error; err != nil {
		t.Fatalf("seed participant: %v", err)
	}
	return participant
}

func TestSubmitGoalApprovalRecordsTargetConfirmers(t *testing.T) {
	db := setupPerformanceServiceTestDB(t)
	svc := NewPerformanceService(db)
	activity := seedPerformanceActivity(t, db, database.PerformanceActivity{Status: "target_setting"})
	participant := seedPerformanceParticipant(t, db, database.PerformanceParticipant{
		ActivityID: fmt.Sprint(activity.ID),
		Status:     "pending",
	})

	if err := db.Create(&database.User{UserID: "employee-user", Name: "Employee Name", Email: "employee@example.com", Mobile: "10000000001", DepartmentID: "dept-a", Status: "active"}).Error; err != nil {
		t.Fatalf("seed employee user: %v", err)
	}
	if err := db.Create(&database.User{UserID: "manager-user", Name: "Manager Name", Email: "manager@example.com", Mobile: "10000000002", DepartmentID: "dept-a", Status: "active"}).Error; err != nil {
		t.Fatalf("seed manager user: %v", err)
	}
	if err := db.Create(&database.PerformanceGoalRecord{
		ActivityID:    fmt.Sprint(activity.ID),
		ParticipantID: participant.ID,
		SectionType:   "quantitative",
		ItemName:      "Revenue",
	}).Error; err != nil {
		t.Fatalf("seed goal record: %v", err)
	}

	if err := svc.SubmitGoalApproval(participant.ID, "submit", "", "employee-user"); err != nil {
		t.Fatalf("SubmitGoalApproval(submit) error = %v", err)
	}
	if err := svc.SubmitGoalApproval(participant.ID, "approve", "", "manager-user"); err != nil {
		t.Fatalf("SubmitGoalApproval(approve) error = %v", err)
	}

	var got database.PerformanceParticipant
	if err := db.First(&got, participant.ID).Error; err != nil {
		t.Fatalf("load participant: %v", err)
	}
	if got.EmployeeTargetConfirmedBy != "Employee Name" || got.EmployeeTargetConfirmedAt == nil {
		t.Fatalf("employee target confirmer = %q at %v, want Employee Name with timestamp", got.EmployeeTargetConfirmedBy, got.EmployeeTargetConfirmedAt)
	}
	if got.ManagerTargetConfirmedBy != "Manager Name" || got.ManagerTargetConfirmedAt == nil {
		t.Fatalf("manager target confirmer = %q at %v, want Manager Name with timestamp", got.ManagerTargetConfirmedBy, got.ManagerTargetConfirmedAt)
	}

	var approveLog database.PerformanceGoalApprovalLog
	if err := db.Where("participant_id = ? AND action = ?", participant.ID, "approve").First(&approveLog).Error; err != nil {
		t.Fatalf("load approve log: %v", err)
	}
	if approveLog.ApproverName != "Manager Name" {
		t.Fatalf("approve log approver name = %q, want Manager Name", approveLog.ApproverName)
	}
}

func TestLockActivityRequiresActualHRConfirmation(t *testing.T) {
	db := setupPerformanceServiceTestDB(t)
	svc := NewPerformanceService(db)
	activity := seedPerformanceActivity(t, db, database.PerformanceActivity{Status: "hr_confirmation"})
	participant := seedPerformanceParticipant(t, db, database.PerformanceParticipant{
		ActivityID: fmt.Sprint(activity.ID),
		Status:     "manager_confirmed",
	})

	err := svc.LockActivity(fmt.Sprint(activity.ID), "tester")
	if err == nil || !strings.Contains(err.Error(), "未完成当前阶段") {
		t.Fatalf("LockActivity() error = %v, want incomplete HR confirmation", err)
	}

	var got database.PerformanceParticipant
	if err := db.First(&got, participant.ID).Error; err != nil {
		t.Fatalf("load participant: %v", err)
	}
	if got.Status != "manager_confirmed" || got.IsLocked {
		t.Fatalf("participant after failed lock = status %s locked %v, want manager_confirmed unlocked", got.Status, got.IsLocked)
	}
}

func TestLockActivitySucceedsAfterHRConfirmation(t *testing.T) {
	db := setupPerformanceServiceTestDB(t)
	svc := NewPerformanceService(db)
	activity := seedPerformanceActivity(t, db, database.PerformanceActivity{Status: "hr_confirmation"})
	now := time.Now()
	participant := seedPerformanceParticipant(t, db, database.PerformanceParticipant{
		ActivityID:     fmt.Sprint(activity.ID),
		Status:         "hr_confirmed",
		HRConfirmedAt:  &now,
		HRConfirmedBy:  "hr-user",
		ManagerScore:   90,
		SuggestedLevel: "A",
		FinalLevel:     "A",
	})

	if err := svc.LockActivity(fmt.Sprint(activity.ID), "tester"); err != nil {
		t.Fatalf("LockActivity() error = %v", err)
	}

	var got database.PerformanceParticipant
	if err := db.First(&got, participant.ID).Error; err != nil {
		t.Fatalf("load participant: %v", err)
	}
	if got.Status != "locked" || !got.IsLocked || got.LockedAt == nil || got.LockedBy != "tester" {
		t.Fatalf("participant lock fields = status %s locked %v locked_at %v locked_by %s", got.Status, got.IsLocked, got.LockedAt, got.LockedBy)
	}
	var gotActivity database.PerformanceActivity
	if err := db.First(&gotActivity, activity.ID).Error; err != nil {
		t.Fatalf("load activity: %v", err)
	}
	if gotActivity.Status != "locked" {
		t.Fatalf("activity status = %s, want locked", gotActivity.Status)
	}
}

func TestForceLockOverdueHRConfirmationMarksPendingParticipants(t *testing.T) {
	db := setupPerformanceServiceTestDB(t)
	svc := NewPerformanceService(db)
	activity := seedPerformanceActivity(t, db, database.PerformanceActivity{
		Status:            "hr_confirmation",
		HRConfirmDeadline: time.Now().AddDate(0, 0, -2).Format("2006-01-02"),
	})
	pending := seedPerformanceParticipant(t, db, database.PerformanceParticipant{
		ActivityID: fmt.Sprint(activity.ID),
		Status:     "manager_confirmed",
	})
	now := time.Now()
	confirmed := seedPerformanceParticipant(t, db, database.PerformanceParticipant{
		ActivityID:    fmt.Sprint(activity.ID),
		Status:        "hr_confirmed",
		HRConfirmedAt: &now,
	})

	result, err := svc.ForceLockOverdueHRConfirmation(fmt.Sprint(activity.ID), "tester")
	if err != nil {
		t.Fatalf("ForceLockOverdueHRConfirmation() error = %v", err)
	}
	if result["force_locked_count"] != 1 || result["locked_count"] != 2 || result["total_count"] != 2 {
		t.Fatalf("force lock result = %#v, want force 1 locked 2 total 2", result)
	}

	var gotPending database.PerformanceParticipant
	if err := db.First(&gotPending, pending.ID).Error; err != nil {
		t.Fatalf("load pending participant: %v", err)
	}
	if gotPending.Status != "locked" || !gotPending.IsLocked || !gotPending.ForceLocked || !strings.Contains(gotPending.ForceLockedReason, activity.HRConfirmDeadline) {
		t.Fatalf("pending participant force lock fields = status %s locked %v force %v reason %q", gotPending.Status, gotPending.IsLocked, gotPending.ForceLocked, gotPending.ForceLockedReason)
	}

	var gotConfirmed database.PerformanceParticipant
	if err := db.First(&gotConfirmed, confirmed.ID).Error; err != nil {
		t.Fatalf("load confirmed participant: %v", err)
	}
	if gotConfirmed.Status != "locked" || !gotConfirmed.IsLocked || gotConfirmed.ForceLocked {
		t.Fatalf("confirmed participant fields = status %s locked %v force %v", gotConfirmed.Status, gotConfirmed.IsLocked, gotConfirmed.ForceLocked)
	}
}

func TestConfirmResultLocksParticipantInsteadOfResultConfirmed(t *testing.T) {
	db := setupPerformanceServiceTestDB(t)
	svc := NewPerformanceService(db)
	activity := seedPerformanceActivity(t, db, database.PerformanceActivity{Status: "employee_confirmation"})
	participant := seedPerformanceParticipant(t, db, database.PerformanceParticipant{
		ActivityID:   fmt.Sprint(activity.ID),
		Status:       "manager_submitted",
		FinalLevel:   "A",
		ManagerScore: 90,
	})

	version, err := svc.ConfirmResult(fmt.Sprint(participant.ID), "ok", "tester")
	if err != nil {
		t.Fatalf("ConfirmResult() error = %v", err)
	}
	if version == nil || version.ReviewType != "confirm_result" || version.ConfirmedAt == nil {
		t.Fatalf("version = %#v, want confirm_result with confirmed_at", version)
	}

	var got database.PerformanceParticipant
	if err := db.First(&got, participant.ID).Error; err != nil {
		t.Fatalf("load participant: %v", err)
	}
	if got.Status != "locked" || !got.IsLocked || got.LockedAt == nil || got.ConfirmedAt == nil {
		t.Fatalf("participant fields = status %s locked %v locked_at %v confirmed_at %v", got.Status, got.IsLocked, got.LockedAt, got.ConfirmedAt)
	}
	if got.Status == "result_confirmed" {
		t.Fatalf("participant status should not be result_confirmed")
	}
}
