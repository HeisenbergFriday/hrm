package service

import (
	"strings"
	"testing"

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
		&database.PerformanceActivity{},
		&database.PerformanceIndicatorLibrary{},
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
	}); err == nil || !strings.Contains(err.Error(), "指标库周期与活动周期不一致") {
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
	})
	if err != nil {
		t.Fatalf("CreateActivity() matching cycle error = %v", err)
	}
	if activity.IndicatorLibraryID == nil || *activity.IndicatorLibraryID != library.ID {
		t.Fatalf("activity indicator library = %v, want %d", activity.IndicatorLibraryID, library.ID)
	}
}
