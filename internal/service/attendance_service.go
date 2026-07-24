package service

import (
	"fmt"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"strings"
	"time"

	"gorm.io/gorm"
)

type attendanceRepository interface {
	FindAll(page, pageSize int, filters map[string]string) ([]database.Attendance, int64, error)
	Upsert(record *database.Attendance) error
}

type AttendanceService struct {
	db             *gorm.DB
	attendanceRepo attendanceRepository
	exportRepo     *repository.AttendanceExportRepository
	syncRepo       *repository.SyncRepository
}

func NewAttendanceService(db *gorm.DB) *AttendanceService {
	// Prefer request/tenant org from DB session; empty context keeps repos fail-closed.
	orgID, _ := database.RequireOrganizationIDFromDB(db)
	return NewAttendanceServiceWithOrgID(db, orgID)
}

// NewAttendanceServiceWithOrgID binds attendance repositories to an explicit tenant.
// Prefer this for jobs/handlers that have orgID outside the DB session context.
func NewAttendanceServiceWithOrgID(db *gorm.DB, orgID string) *AttendanceService {
	orgID = strings.TrimSpace(orgID)
	return &AttendanceService{
		db:             db,
		attendanceRepo: repository.NewAttendanceRepositoryWithOrgID(db, orgID),
		exportRepo:     repository.NewAttendanceExportRepositoryWithOrgID(db, orgID),
		syncRepo:       repository.NewSyncRepositoryWithOrgID(db, orgID),
	}
}

func (s *AttendanceService) GetRecords(page, pageSize int, filters map[string]string) ([]database.Attendance, int64, error) {
	return s.attendanceRepo.FindAll(page, pageSize, filters)
}

func (s *AttendanceService) SaveRecord(record *database.Attendance) error {
	return s.attendanceRepo.Upsert(record)
}

func (s *AttendanceService) SyncRecords(orgID string, records []dingtalk.AttendanceRecord, userNameMap map[string]string) (int, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		// Fail-closed: never invent "default" for tenant attendance writes.
		return 0, repository.ErrMissingOrgID
	}
	orgID = database.NormalizeOrganizationID(orgID)
	// Always write through an org-bound repository so empty session context cannot
	// block legitimate SyncRecords(orgID, ...) nor fall open across tenants.
	repo := repository.NewAttendanceRepositoryWithOrgID(s.db, orgID)
	count := 0
	for _, r := range records {
		if r.UserCheckTime == "" {
			continue
		}

		checkType := "上班"
		if r.CheckType == "OffDuty" {
			checkType = "下班"
		} else if r.CheckType != "" && r.CheckType != "OnDuty" {
			checkType = r.CheckType
		}

		cst := time.FixedZone("CST", 8*3600)
		checkTime, err := time.ParseInLocation("2006-01-02 15:04:05", r.UserCheckTime, cst)
		if err != nil {
			return count, fmt.Errorf("parse attendance time for user %s failed: %w", r.UserID, err)
		}

		record := &database.Attendance{
			OrgID:     orgID,
			UserID:    r.UserID,
			UserName:  userNameMap[r.UserID],
			CheckTime: checkTime,
			CheckType: checkType,
			Location:  r.LocationResult,
			Extension: map[string]interface{}{
				"time_result":       r.TimeResult,
				"location_result":   r.LocationResult,
				"sourceType":        r.SourceType,
				"isLegal":           r.IsLegal,
				"invalidRecordType": r.InvalidRecordType,
				"invalidRecordMsg":  r.InvalidRecordMsg,
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

		if err := repo.Upsert(record); err != nil {
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

func (s *AttendanceService) GetLastSyncTime(orgID string) (*database.SyncStatus, error) {
	return s.syncRepo.FindByOrgAndType(orgID, "attendance")
}
