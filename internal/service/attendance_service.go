package service

import (
	"fmt"
	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type AttendanceService struct {
	attendanceRepo *repository.AttendanceRepository
	exportRepo     *repository.AttendanceExportRepository
	syncRepo       *repository.SyncRepository
}

func NewAttendanceService(db *gorm.DB) *AttendanceService {
	return &AttendanceService{
		attendanceRepo: repository.NewAttendanceRepository(db),
		exportRepo:     repository.NewAttendanceExportRepository(db),
		syncRepo:       repository.NewSyncRepository(db),
	}
}

func (s *AttendanceService) GetRecords(page, pageSize int, filters map[string]string) ([]database.Attendance, int64, error) {
	return s.attendanceRepo.FindAll(page, pageSize, filters)
}

func (s *AttendanceService) GetStats(filters map[string]string) (map[string]interface{}, error) {
	// 获取所有匹配的考勤记录（不分页）
	records, _, err := s.attendanceRepo.FindAll(1, 100000, filters)
	if err != nil {
		return nil, err
	}

	// 按用户统计
	userStats := make(map[string]map[string]int) // user_id -> {normal, late, ...}
	userNames := make(map[string]string)

	for _, r := range records {
		if _, ok := userStats[r.UserID]; !ok {
			userStats[r.UserID] = map[string]int{"normal": 0, "late": 0, "leave_early": 0, "absent": 0}
			userNames[r.UserID] = r.UserName
		}
		ext := r.Extension
		if ext != nil {
			if abnormalType, ok := ext["abnormal_type"]; ok {
				switch abnormalType {
				case "迟到":
					userStats[r.UserID]["late"]++
				case "早退":
					userStats[r.UserID]["leave_early"]++
				case "缺勤":
					userStats[r.UserID]["absent"]++
				default:
					userStats[r.UserID]["normal"]++
				}
				continue
			}
		}
		userStats[r.UserID]["normal"]++
	}

	totalUsers := len(userStats)
	normalCount := 0
	lateCount := 0
	leaveEarlyCount := 0
	absentCount := 0

	for _, stats := range userStats {
		lateCount += stats["late"]
		leaveEarlyCount += stats["leave_early"]
		absentCount += stats["absent"]
		normalCount += stats["normal"]
	}

	normalRate := "0.00%"
	totalRecords := normalCount + lateCount + leaveEarlyCount + absentCount
	if totalRecords > 0 {
		rate := float64(normalCount) / float64(totalRecords) * 100
		normalRate = fmt.Sprintf("%.2f%%", rate)
	}

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"total_users":       totalUsers,
			"normal_count":      normalCount,
			"late_count":        lateCount,
			"leave_early_count": leaveEarlyCount,
			"absent_count":      absentCount,
			"normal_rate":       normalRate,
		},
		"department_stats":  []interface{}{},
		"abnormal_details":  []interface{}{},
	}, nil
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
