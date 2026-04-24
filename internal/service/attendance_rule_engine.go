package service

import (
	"errors"
	"peopleops/internal/config"
	"peopleops/internal/database"
	"strings"
	"time"

	"gorm.io/gorm"
)

type attendanceSchedule struct {
	CheckIn  string
	CheckOut string
	ShiftID  int64
	Source   string
}

// AttendanceRuleEngine calculates expected workdays first, then overlays punches.
type AttendanceRuleEngine struct {
	weekScheduleService *WeekScheduleService
	holidayFinder       func(date string) (*database.StatutoryHoliday, bool)
	scheduleFinder      func(userID, departmentID, date string) (attendanceSchedule, error)
}

// NewAttendanceRuleEngine creates a new attendance rule engine.
func NewAttendanceRuleEngine(weekScheduleService *WeekScheduleService) *AttendanceRuleEngine {
	engine := &AttendanceRuleEngine{
		weekScheduleService: weekScheduleService,
	}
	engine.holidayFinder = engine.lookupHoliday
	engine.scheduleFinder = engine.lookupSchedule
	return engine
}

// AttendanceStatus describes the primary status of a workday.
type AttendanceStatus string

const (
	AttendanceStatusNormal     AttendanceStatus = "normal"
	AttendanceStatusLate       AttendanceStatus = "late"
	AttendanceStatusLeaveEarly AttendanceStatus = "leave_early"
	AttendanceStatusAbsent     AttendanceStatus = "absent"
	AttendanceStatusHoliday    AttendanceStatus = "holiday"
	AttendanceStatusRestDay    AttendanceStatus = "rest_day"
)

// DailyAttendance is the computed per-day attendance snapshot.
type DailyAttendance struct {
	Date              string                     `json:"date"`
	DayOfWeek         int                        `json:"day_of_week"`
	ShouldWork        bool                       `json:"should_work"`
	Holiday           *database.StatutoryHoliday `json:"holiday,omitempty"`
	WeekType          string                     `json:"week_type,omitempty"`
	IsOverride        bool                       `json:"is_override,omitempty"`
	ScheduledCheckIn  string                     `json:"scheduled_check_in,omitempty"`
	ScheduledCheckOut string                     `json:"scheduled_check_out,omitempty"`
	ScheduleSource    string                     `json:"schedule_source,omitempty"`
	CheckInTime       *time.Time                 `json:"check_in_time,omitempty"`
	CheckOutTime      *time.Time                 `json:"check_out_time,omitempty"`
	IsLate            bool                       `json:"is_late,omitempty"`
	IsLeaveEarly      bool                       `json:"is_leave_early,omitempty"`
	Status            AttendanceStatus           `json:"status"`
	StatusReason      string                     `json:"status_reason,omitempty"`
}

// CalculateAttendance builds the expected attendance calendar for the date range.
func (e *AttendanceRuleEngine) CalculateAttendance(userID, departmentID string, startDate, endDate string) ([]DailyAttendance, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, err
	}

	result := make([]DailyAttendance, 0, int(end.Sub(start).Hours()/24)+1)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		dayOfWeek := int(d.Weekday())

		if holiday, isHoliday := e.getHoliday(dateStr); isHoliday {
			daily := DailyAttendance{
				Date:       dateStr,
				DayOfWeek:  dayOfWeek,
				ShouldWork: holiday.Type == "workday",
				Holiday:    holiday,
				Status:     AttendanceStatusHoliday,
			}
			if daily.ShouldWork {
				daily.Status = AttendanceStatusNormal
				if err := e.attachSchedule(&daily, userID, departmentID, dateStr); err != nil {
					return nil, err
				}
			}
			result = append(result, daily)
			continue
		}

		switch d.Weekday() {
		case time.Sunday:
			result = append(result, DailyAttendance{
				Date:       dateStr,
				DayOfWeek:  dayOfWeek,
				ShouldWork: false,
				Status:     AttendanceStatusRestDay,
			})
		case time.Saturday:
			weekType, err := e.resolveWeekType(userID, departmentID, dateStr)
			if err != nil {
				return nil, err
			}
			daily := DailyAttendance{
				Date:       dateStr,
				DayOfWeek:  dayOfWeek,
				ShouldWork: weekType == "small",
				WeekType:   weekType,
				Status:     AttendanceStatusRestDay,
			}
			if daily.ShouldWork {
				daily.Status = AttendanceStatusNormal
				if err := e.attachSchedule(&daily, userID, departmentID, dateStr); err != nil {
					return nil, err
				}
			}
			result = append(result, daily)
		default:
			daily := DailyAttendance{
				Date:       dateStr,
				DayOfWeek:  dayOfWeek,
				ShouldWork: true,
				Status:     AttendanceStatusNormal,
			}
			if err := e.attachSchedule(&daily, userID, departmentID, dateStr); err != nil {
				return nil, err
			}
			result = append(result, daily)
		}
	}

	return result, nil
}

// AggregateAttendance overlays punch records onto the expected work calendar.
func (e *AttendanceRuleEngine) AggregateAttendance(dailyAttendances []DailyAttendance, records []database.Attendance) []DailyAttendance {
	recordMap := make(map[string][]database.Attendance)
	for _, record := range records {
		dateStr := record.CheckTime.Format("2006-01-02")
		recordMap[dateStr] = append(recordMap[dateStr], record)
	}

	for i := range dailyAttendances {
		da := &dailyAttendances[i]
		if !da.ShouldWork {
			continue
		}

		dailyRecords := recordMap[da.Date]
		if len(dailyRecords) == 0 {
			da.Status = AttendanceStatusAbsent
			da.StatusReason = "missing_attendance"
			continue
		}

		var checkIn *time.Time
		var checkOut *time.Time
		for j := range dailyRecords {
			record := dailyRecords[j]
			switch normalizeCheckType(record.CheckType) {
			case "check_in":
				if checkIn == nil || record.CheckTime.Before(*checkIn) {
					checkIn = &record.CheckTime
				}
			case "check_out":
				if checkOut == nil || record.CheckTime.After(*checkOut) {
					checkOut = &record.CheckTime
				}
			}
		}

		da.CheckInTime = checkIn
		da.CheckOutTime = checkOut

		if checkIn == nil && checkOut == nil {
			da.Status = AttendanceStatusAbsent
			da.StatusReason = "missing_check_in_and_check_out"
			continue
		}
		if checkIn == nil {
			da.Status = AttendanceStatusAbsent
			da.StatusReason = "missing_check_in"
			continue
		}
		if checkOut == nil {
			da.Status = AttendanceStatusAbsent
			da.StatusReason = "missing_check_out"
			continue
		}

		scheduledCheckIn, _ := parseScheduleTime(da.Date, da.ScheduledCheckIn)
		scheduledCheckOut, _ := parseScheduleTime(da.Date, da.ScheduledCheckOut)

		da.IsLate = scheduledCheckIn != nil && checkIn.After(*scheduledCheckIn)
		da.IsLeaveEarly = scheduledCheckOut != nil && checkOut.Before(*scheduledCheckOut)

		switch {
		case da.IsLate && da.IsLeaveEarly:
			da.Status = AttendanceStatusLate
			da.StatusReason = "late_and_leave_early"
		case da.IsLate:
			da.Status = AttendanceStatusLate
			da.StatusReason = "late"
		case da.IsLeaveEarly:
			da.Status = AttendanceStatusLeaveEarly
			da.StatusReason = "leave_early"
		default:
			da.Status = AttendanceStatusNormal
			da.StatusReason = ""
		}
	}

	return dailyAttendances
}

func (e *AttendanceRuleEngine) getHoliday(date string) (*database.StatutoryHoliday, bool) {
	if e.holidayFinder == nil {
		return nil, false
	}
	return e.holidayFinder(date)
}

func (e *AttendanceRuleEngine) attachSchedule(daily *DailyAttendance, userID, departmentID, date string) error {
	schedule, err := e.getSchedule(userID, departmentID, date)
	if err != nil {
		return err
	}
	daily.ScheduledCheckIn = schedule.CheckIn
	daily.ScheduledCheckOut = schedule.CheckOut
	daily.ScheduleSource = schedule.Source
	return nil
}

func (e *AttendanceRuleEngine) getSchedule(userID, departmentID, date string) (attendanceSchedule, error) {
	if e.scheduleFinder == nil {
		return attendanceSchedule{
			CheckIn:  config.GetDefaultCheckIn(),
			CheckOut: config.GetDefaultCheckOut(),
			Source:   "default",
		}, nil
	}
	return e.scheduleFinder(userID, departmentID, date)
}

func (e *AttendanceRuleEngine) resolveWeekType(userID, departmentID, date string) (string, error) {
	if e.weekScheduleService == nil {
		return "big", nil
	}
	weekType, err := e.weekScheduleService.GetWeekType(userID, departmentID, date)
	if err != nil {
		return "big", nil
	}
	return weekType, nil
}

func (e *AttendanceRuleEngine) lookupHoliday(date string) (*database.StatutoryHoliday, bool) {
	if e.weekScheduleService == nil || e.weekScheduleService.scheduleRepo == nil {
		return nil, false
	}
	holiday, err := e.weekScheduleService.scheduleRepo.FindHolidayByDate(date)
	if err != nil {
		return nil, false
	}
	return holiday, holiday != nil
}

func (e *AttendanceRuleEngine) lookupSchedule(userID, departmentID, _ string) (attendanceSchedule, error) {
	schedule := attendanceSchedule{
		CheckIn:  config.GetDefaultCheckIn(),
		CheckOut: config.GetDefaultCheckOut(),
		Source:   "default",
	}
	if e.weekScheduleService == nil || e.weekScheduleService.db == nil {
		return schedule, nil
	}

	var customConfig database.EmployeeShiftConfig
	err := e.weekScheduleService.db.Where("user_id = ?", userID).First(&customConfig).Error
	switch {
	case err == nil:
		schedule.Source = "custom"
		schedule.ShiftID = customConfig.ShiftID
		if catalog, found, err := e.lookupShiftCatalog(customConfig.ShiftID); err != nil {
			return schedule, err
		} else if found {
			if catalog.CheckIn != "" {
				schedule.CheckIn = strings.TrimSpace(catalog.CheckIn)
			}
			if catalog.CheckOut != "" {
				schedule.CheckOut = strings.TrimSpace(catalog.CheckOut)
			}
		}
		if strings.TrimSpace(customConfig.EndTime) != "" {
			schedule.CheckOut = strings.TrimSpace(customConfig.EndTime)
		}
		return schedule, nil
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		return schedule, err
	}

	scopes := []struct {
		scopeType string
		scopeID   string
	}{
		{scopeType: "user", scopeID: userID},
	}
	if departmentID != "" {
		scopes = append(scopes, struct {
			scopeType string
			scopeID   string
		}{scopeType: "department", scopeID: departmentID})
	}
	scopes = append(scopes, struct {
		scopeType string
		scopeID   string
	}{scopeType: "company", scopeID: ""})

	for _, scope := range scopes {
		rule, err := e.weekScheduleService.scheduleRepo.FindRuleByScope(scope.scopeType, scope.scopeID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return schedule, err
		}
		if rule == nil || rule.ShiftID <= 0 {
			continue
		}
		schedule.Source = "week_rule"
		schedule.ShiftID = rule.ShiftID
		if catalog, found, err := e.lookupShiftCatalog(rule.ShiftID); err != nil {
			return schedule, err
		} else if found {
			if catalog.CheckIn != "" {
				schedule.CheckIn = strings.TrimSpace(catalog.CheckIn)
			}
			if catalog.CheckOut != "" {
				schedule.CheckOut = strings.TrimSpace(catalog.CheckOut)
			}
		}
		return schedule, nil
	}

	return schedule, nil
}

func (e *AttendanceRuleEngine) lookupShiftCatalog(shiftID int64) (*database.DingTalkShiftCatalog, bool, error) {
	if shiftID <= 0 || e.weekScheduleService == nil || e.weekScheduleService.db == nil {
		return nil, false, nil
	}

	var catalog database.DingTalkShiftCatalog
	err := e.weekScheduleService.db.
		Where("shift_id = ?", shiftID).
		Order("updated_at DESC").
		First(&catalog).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &catalog, true, nil
}

func normalizeCheckType(checkType string) string {
	normalized := strings.ToLower(strings.TrimSpace(checkType))
	switch normalized {
	case "onduty", "on_duty":
		return "check_in"
	case "offduty", "off_duty":
		return "check_out"
	}
	if strings.Contains(checkType, "上班") {
		return "check_in"
	}
	if strings.Contains(checkType, "下班") {
		return "check_out"
	}
	return ""
}

func parseScheduleTime(date, clock string) (*time.Time, error) {
	clock = strings.TrimSpace(clock)
	if clock == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, time.Local)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
