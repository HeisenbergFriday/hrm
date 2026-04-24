package service

import (
	"fmt"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"sort"
	"time"

	"gorm.io/gorm"
)

type attendanceRepository interface {
	FindAll(page, pageSize int, filters map[string]string) ([]database.Attendance, int64, error)
	Upsert(record *database.Attendance) error
}

type abnormalUser struct {
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	Times    int    `json:"times"`
}

type abnormalDetail struct {
	Type  string         `json:"type"`
	Count int            `json:"count"`
	Users []abnormalUser `json:"users"`
}

type departmentStat struct {
	DepartmentID    string `json:"department_id"`
	DepartmentName  string `json:"department_name"`
	TotalUsers      int    `json:"total_users"`
	NormalCount     int    `json:"normal_count"`
	LateCount       int    `json:"late_count"`
	LeaveEarlyCount int    `json:"leave_early_count"`
	AbsentCount     int    `json:"absent_count"`
	NormalRate      string `json:"normal_rate"`
}

type departmentAccumulator struct {
	departmentStat
	totalWorkDays int
	userIDs       map[string]struct{}
}

type AttendanceService struct {
	db                   *gorm.DB
	attendanceRepo       attendanceRepository
	exportRepo           *repository.AttendanceExportRepository
	syncRepo             *repository.SyncRepository
	ruleEngine           *AttendanceRuleEngine
	userLoader           func(filters map[string]string) ([]database.User, error)
	departmentNameLoader func() (map[string]string, error)
}

func NewAttendanceService(db *gorm.DB) *AttendanceService {
	weekScheduleService := NewWeekScheduleService(db)
	svc := &AttendanceService{
		db:             db,
		attendanceRepo: repository.NewAttendanceRepository(db),
		exportRepo:     repository.NewAttendanceExportRepository(db),
		syncRepo:       repository.NewSyncRepository(db),
		ruleEngine:     NewAttendanceRuleEngine(weekScheduleService),
	}
	svc.userLoader = svc.loadUsers
	svc.departmentNameLoader = svc.loadDepartmentNames
	return svc
}

func (s *AttendanceService) GetRecords(page, pageSize int, filters map[string]string) ([]database.Attendance, int64, error) {
	return s.attendanceRepo.FindAll(page, pageSize, filters)
}

func (s *AttendanceService) GetStats(filters map[string]string) (map[string]interface{}, error) {
	startDate := filters["start_date"]
	endDate := filters["end_date"]
	if startDate == "" || endDate == "" {
		endDate = time.Now().Format("2006-01-02")
		startDate = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}

	users, err := s.getUsersForStats(filters)
	if err != nil {
		return nil, err
	}

	departmentNames, err := s.getDepartmentNames()
	if err != nil {
		return nil, err
	}

	records, _, err := s.attendanceRepo.FindAll(1, 100000, filters)
	if err != nil {
		return nil, err
	}

	recordMap := make(map[string][]database.Attendance)
	for _, record := range records {
		recordMap[record.UserID] = append(recordMap[record.UserID], record)
	}

	departmentStats := make(map[string]*departmentAccumulator)
	abnormalBuckets := map[string]map[string]*abnormalUser{
		"late":        {},
		"leave_early": {},
		"absent":      {},
	}

	totalUsers := len(users)
	totalWorkDays := 0
	normalCount := 0
	lateCount := 0
	leaveEarlyCount := 0
	absentCount := 0

	for _, user := range users {
		dailyAttendances, err := s.ruleEngine.CalculateAttendance(user.UserID, user.DepartmentID, startDate, endDate)
		if err != nil {
			return nil, fmt.Errorf("calculate attendance for user %s failed: %w", user.UserID, err)
		}

		dailyAttendances = s.ruleEngine.AggregateAttendance(dailyAttendances, recordMap[user.UserID])

		deptID := user.DepartmentID
		if _, ok := departmentStats[deptID]; !ok {
			departmentName := departmentNames[deptID]
			if departmentName == "" {
				departmentName = deptID
			}
			departmentStats[deptID] = &departmentAccumulator{
				departmentStat: departmentStat{
					DepartmentID:   deptID,
					DepartmentName: departmentName,
				},
				userIDs: make(map[string]struct{}),
			}
		}
		deptStat := departmentStats[deptID]
		deptStat.userIDs[user.UserID] = struct{}{}

		for _, daily := range dailyAttendances {
			if !daily.ShouldWork {
				continue
			}

			totalWorkDays++
			deptStat.totalWorkDays++

			switch {
			case daily.Status == AttendanceStatusAbsent:
				absentCount++
				deptStat.AbsentCount++
				incrementAbnormalBucket(abnormalBuckets["absent"], user.UserID, user.Name)
			case daily.IsLate || daily.IsLeaveEarly:
				if daily.IsLate {
					lateCount++
					deptStat.LateCount++
					incrementAbnormalBucket(abnormalBuckets["late"], user.UserID, user.Name)
				}
				if daily.IsLeaveEarly {
					leaveEarlyCount++
					deptStat.LeaveEarlyCount++
					incrementAbnormalBucket(abnormalBuckets["leave_early"], user.UserID, user.Name)
				}
			default:
				normalCount++
				deptStat.NormalCount++
			}
		}
	}

	departmentStatsList := make([]departmentStat, 0, len(departmentStats))
	for _, deptStat := range departmentStats {
		deptStat.TotalUsers = len(deptStat.userIDs)
		deptStat.NormalRate = formatRate(deptStat.NormalCount, deptStat.totalWorkDays)
		departmentStatsList = append(departmentStatsList, deptStat.departmentStat)
	}
	sort.Slice(departmentStatsList, func(i, j int) bool {
		if departmentStatsList[i].DepartmentName == departmentStatsList[j].DepartmentName {
			return departmentStatsList[i].DepartmentID < departmentStatsList[j].DepartmentID
		}
		return departmentStatsList[i].DepartmentName < departmentStatsList[j].DepartmentName
	})

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"total_users":       totalUsers,
			"normal_count":      normalCount,
			"late_count":        lateCount,
			"leave_early_count": leaveEarlyCount,
			"absent_count":      absentCount,
			"normal_rate":       formatRate(normalCount, totalWorkDays),
		},
		"department_stats": departmentStatsList,
		"abnormal_details": []abnormalDetail{
			buildAbnormalDetail("late", abnormalBuckets["late"]),
			buildAbnormalDetail("leave_early", abnormalBuckets["leave_early"]),
			buildAbnormalDetail("absent", abnormalBuckets["absent"]),
		},
		"start_date":    startDate,
		"end_date":      endDate,
		"department_id": filters["department_id"],
	}, nil
}

func (s *AttendanceService) SaveRecord(record *database.Attendance) error {
	return s.attendanceRepo.Upsert(record)
}

func (s *AttendanceService) SyncRecords(records []dingtalk.AttendanceRecord, userNameMap map[string]string) (int, error) {
	count := 0
	for _, r := range records {
		checkType := "上班"
		if r.CheckType == "OffDuty" {
			checkType = "下班"
		}

		checkTime, err := time.Parse("2006-01-02 15:04:05", r.UserCheckTime)
		if err != nil {
			return count, fmt.Errorf("parse attendance time for user %s failed: %w", r.UserID, err)
		}

		record := &database.Attendance{
			UserID:    r.UserID,
			UserName:  userNameMap[r.UserID],
			CheckTime: checkTime,
			CheckType: checkType,
			Location:  r.LocationResult,
			Extension: map[string]interface{}{
				"time_result":     r.TimeResult,
				"location_result": r.LocationResult,
			},
		}
		if r.TimeResult == "Late" || r.TimeResult == "Early" || r.TimeResult == "NotSigned" {
			abnormalType := "迟到"
			if r.TimeResult == "Early" {
				abnormalType = "早退"
			} else if r.TimeResult == "NotSigned" {
				abnormalType = "缺卡"
			}
			record.Extension["abnormal_type"] = abnormalType
		}

		if err := s.attendanceRepo.Upsert(record); err != nil {
			return count, fmt.Errorf("save attendance record for user %s at %s failed: %w", r.UserID, r.UserCheckTime, err)
		}
		count++
	}

	return count, nil
}

func (s *AttendanceService) CreateExport(export *database.AttendanceExport) error {
	return s.exportRepo.Create(export)
}

func (s *AttendanceService) GetExports(page, pageSize int) ([]database.AttendanceExport, int64, error) {
	return s.exportRepo.FindAll(page, pageSize)
}

func (s *AttendanceService) GetLastSyncTime() (*database.SyncStatus, error) {
	return s.syncRepo.FindByType("attendance")
}

func (s *AttendanceService) getUsersForStats(filters map[string]string) ([]database.User, error) {
	if s.userLoader == nil {
		return nil, nil
	}
	return s.userLoader(filters)
}

func (s *AttendanceService) getDepartmentNames() (map[string]string, error) {
	if s.departmentNameLoader == nil {
		return map[string]string{}, nil
	}
	return s.departmentNameLoader()
}

func (s *AttendanceService) loadUsers(filters map[string]string) ([]database.User, error) {
	if s.db == nil {
		return nil, nil
	}

	query := s.db.Model(&database.User{}).Where("user_id <> ?", "admin")
	if v := filters["department_id"]; v != "" {
		query = query.Where("department_id = ?", v)
	}
	if v := filters["user_id"]; v != "" {
		query = query.Where("user_id = ?", v)
	}

	var users []database.User
	if err := query.Order("name ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (s *AttendanceService) loadDepartmentNames() (map[string]string, error) {
	if s.db == nil {
		return map[string]string{}, nil
	}

	var departments []database.Department
	if err := s.db.Find(&departments).Error; err != nil {
		return nil, err
	}

	result := make(map[string]string, len(departments))
	for _, dept := range departments {
		result[dept.DepartmentID] = dept.Name
	}
	return result, nil
}

func incrementAbnormalBucket(bucket map[string]*abnormalUser, userID, userName string) {
	if _, ok := bucket[userID]; !ok {
		bucket[userID] = &abnormalUser{
			UserID:   userID,
			UserName: userName,
		}
	}
	bucket[userID].Times++
}

func buildAbnormalDetail(detailType string, bucket map[string]*abnormalUser) abnormalDetail {
	users := make([]abnormalUser, 0, len(bucket))
	for _, user := range bucket {
		users = append(users, *user)
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].Times == users[j].Times {
			return users[i].UserID < users[j].UserID
		}
		return users[i].Times > users[j].Times
	})
	return abnormalDetail{
		Type:  detailType,
		Count: len(users),
		Users: users,
	}
}

func formatRate(normalCount, totalCount int) string {
	if totalCount <= 0 {
		return "0.00%"
	}
	rate := float64(normalCount) / float64(totalCount) * 100
	return fmt.Sprintf("%.2f%%", rate)
}
