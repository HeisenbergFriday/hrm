package service

import (
	"testing"
	"time"

	"peopleops/internal/database"
)

func TestAttendanceRuleEngineCalculateAttendanceUsesHolidayAndSchedule(t *testing.T) {
	engine := NewAttendanceRuleEngine(nil)
	engine.holidayFinder = func(date string) (*database.StatutoryHoliday, bool) {
		if date != "2026-04-23" {
			return nil, false
		}
		return &database.StatutoryHoliday{
			Date: "2026-04-23",
			Name: "调休上班",
			Type: "workday",
		}, true
	}
	engine.scheduleFinder = func(userID, departmentID, date string) (attendanceSchedule, error) {
		return attendanceSchedule{
			CheckIn:  "09:30",
			CheckOut: "18:00",
			Source:   "custom",
		}, nil
	}

	daily, err := engine.CalculateAttendance("user-1", "dept-1", "2026-04-23", "2026-04-23")
	if err != nil {
		t.Fatalf("expected calculation to succeed, got error: %v", err)
	}
	if len(daily) != 1 {
		t.Fatalf("expected one day result, got %d", len(daily))
	}

	day := daily[0]
	if !day.ShouldWork {
		t.Fatal("expected workday adjustment holiday to require attendance")
	}
	if day.Holiday == nil || day.Holiday.Type != "workday" {
		t.Fatalf("expected holiday metadata to be preserved, got %#v", day.Holiday)
	}
	if day.ScheduledCheckIn != "09:30" || day.ScheduledCheckOut != "18:00" {
		t.Fatalf("expected custom schedule to be attached, got %+v", day)
	}
	if day.ScheduleSource != "custom" {
		t.Fatalf("expected custom schedule source, got %q", day.ScheduleSource)
	}
}

func TestAttendanceRuleEngineAggregateAttendanceMarksLateAndLeaveEarly(t *testing.T) {
	checkIn := time.Date(2026, 4, 23, 9, 15, 0, 0, time.Local)
	checkOut := time.Date(2026, 4, 23, 18, 0, 0, 0, time.Local)

	engine := NewAttendanceRuleEngine(nil)
	aggregated := engine.AggregateAttendance([]DailyAttendance{
		{
			Date:              "2026-04-23",
			ShouldWork:        true,
			ScheduledCheckIn:  "09:00",
			ScheduledCheckOut: "18:30",
			Status:            AttendanceStatusNormal,
		},
	}, []database.Attendance{
		{UserID: "user-1", CheckType: "上班", CheckTime: checkIn},
		{UserID: "user-1", CheckType: "下班", CheckTime: checkOut},
	})

	day := aggregated[0]
	if !day.IsLate {
		t.Fatal("expected on-duty punch after scheduled time to be marked late")
	}
	if !day.IsLeaveEarly {
		t.Fatal("expected off-duty punch before scheduled time to be marked leave early")
	}
	if day.Status != AttendanceStatusLate {
		t.Fatalf("expected primary status to be late when both late and leave early, got %q", day.Status)
	}
	if day.StatusReason != "late_and_leave_early" {
		t.Fatalf("expected combined status reason, got %q", day.StatusReason)
	}
}
