package service

import (
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
)

type fakeAttendanceRepository struct {
	records        map[string]*database.Attendance
	findAllRecords []database.Attendance
}

func (r *fakeAttendanceRepository) FindAll(page, pageSize int, filters map[string]string) ([]database.Attendance, int64, error) {
	result := make([]database.Attendance, len(r.findAllRecords))
	copy(result, r.findAllRecords)
	return result, int64(len(result)), nil
}

func (r *fakeAttendanceRepository) Upsert(record *database.Attendance) error {
	if r.records == nil {
		r.records = make(map[string]*database.Attendance)
	}
	key := record.UserID + "|" + record.CheckType + "|" + record.CheckTime.Format("2006-01-02 15:04:05")
	copied := *record
	r.records[key] = &copied
	return nil
}

func TestAttendanceServiceSyncRecordsIsIdempotent(t *testing.T) {
	repo := &fakeAttendanceRepository{}
	svc := &AttendanceService{attendanceRepo: repo}

	input := []dingtalk.AttendanceRecord{
		{
			UserID:         "user-1",
			CheckType:      "OffDuty",
			UserCheckTime:  "2026-04-23 18:30:00",
			LocationResult: "Office",
			TimeResult:     "Early",
		},
	}
	userNameMap := map[string]string{"user-1": "Alice"}

	if _, err := svc.SyncRecords(input, userNameMap); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}
	if _, err := svc.SyncRecords(input, userNameMap); err != nil {
		t.Fatalf("second sync failed: %v", err)
	}

	if got := len(repo.records); got != 1 {
		t.Fatalf("expected one stored attendance record after duplicate sync, got %d", got)
	}

	for _, record := range repo.records {
		if record.UserName != "Alice" {
			t.Fatalf("expected user name to be preserved, got %q", record.UserName)
		}
		if abnormalType, ok := record.Extension["abnormal_type"]; !ok || abnormalType != "早退" {
			t.Fatalf("expected abnormal_type to be leave early, got %#v", record.Extension["abnormal_type"])
		}
	}
}

func TestAttendanceServiceGetStatsIncludesUsersWithoutPunches(t *testing.T) {
	engine := NewAttendanceRuleEngine(nil)
	engine.holidayFinder = func(date string) (*database.StatutoryHoliday, bool) {
		return nil, false
	}
	engine.scheduleFinder = func(userID, departmentID, date string) (attendanceSchedule, error) {
		return attendanceSchedule{
			CheckIn:  "09:00",
			CheckOut: "18:30",
			Source:   "default",
		}, nil
	}

	svc := &AttendanceService{
		attendanceRepo: &fakeAttendanceRepository{},
		ruleEngine:     engine,
		userLoader: func(filters map[string]string) ([]database.User, error) {
			return []database.User{
				{UserID: "user-1", Name: "Alice", DepartmentID: "dept-1"},
			}, nil
		},
		departmentNameLoader: func() (map[string]string, error) {
			return map[string]string{"dept-1": "Engineering"}, nil
		},
	}

	stats, err := svc.GetStats(map[string]string{
		"start_date": "2026-04-23",
		"end_date":   "2026-04-23",
	})
	if err != nil {
		t.Fatalf("expected stats to succeed, got error: %v", err)
	}

	summary := stats["summary"].(map[string]interface{})
	if got := summary["total_users"].(int); got != 1 {
		t.Fatalf("expected one user in scope, got %d", got)
	}
	if got := summary["absent_count"].(int); got != 1 {
		t.Fatalf("expected one absent workday for zero-punch user, got %d", got)
	}

	deptStats := stats["department_stats"].([]departmentStat)
	if len(deptStats) != 1 {
		t.Fatalf("expected one department stat row, got %d", len(deptStats))
	}
	if deptStats[0].DepartmentName != "Engineering" {
		t.Fatalf("expected department name to be populated, got %q", deptStats[0].DepartmentName)
	}

	abnormalDetails := stats["abnormal_details"].([]abnormalDetail)
	if len(abnormalDetails) != 3 {
		t.Fatalf("expected 3 abnormal buckets, got %d", len(abnormalDetails))
	}
	if abnormalDetails[2].Type != "absent" || abnormalDetails[2].Count != 1 {
		t.Fatalf("expected absent bucket to contain the missing user, got %#v", abnormalDetails[2])
	}
}
