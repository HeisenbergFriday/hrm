package service

import (
	"fmt"
	"log"
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
	db                            *gorm.DB
	orgID                         string
	attendanceRepo                attendanceRepository
	exportRepo                    *repository.AttendanceExportRepository
	syncRepo                      *repository.SyncRepository
	retryableOvertimeRecalculator func([]repository.UserDatePair) (int, error)
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
	service := &AttendanceService{
		db:             db,
		orgID:          orgID,
		attendanceRepo: repository.NewAttendanceRepositoryWithOrgID(db, orgID),
		exportRepo:     repository.NewAttendanceExportRepositoryWithOrgID(db, orgID),
		syncRepo:       repository.NewSyncRepositoryWithOrgID(db, orgID),
	}
	service.retryableOvertimeRecalculator = func(pairs []repository.UserDatePair) (int, error) {
		return NewOvertimeMatchingServiceWithOrgID(db, orgID).RecalcRetryableForAttendancePairs(pairs)
	}
	return service
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
	boundOrgID, err := repository.EnsureSameOrg(strings.TrimSpace(s.orgID), orgID)
	if err != nil {
		return 0, err
	}
	orgID = boundOrgID
	// Always write through an org-bound repository so empty session context cannot
	// block legitimate SyncRecords(orgID, ...) nor fall open across tenants.
	repo := repository.NewAttendanceRepositoryWithOrgID(s.db, orgID)
	count := 0
	affected := make([]repository.UserDatePair, 0, len(records))
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
		affected = append(affected, attendanceAffectedUserDatePairs(record.UserID, record.CheckTime)...)
	}

	// Recalculation is deliberately outside the attendance write loop. A write
	// failure returns before this point, while a downstream failure never undoes
	// attendance records that were already persisted successfully.
	_, _ = s.RecalculateRetryableOvertime(deduplicateUserDatePairs(affected))
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

// RecalculateRetryableOvertime re-evaluates retryable overtime matches for the
// exact employee dates written by an attendance sync. Its error is intentionally
// not returned from SyncRecords because attendance persistence has already
// completed and the bounded scheduled compensation job can retry later.
func (s *AttendanceService) RecalculateRetryableOvertime(pairs []repository.UserDatePair) (int, error) {
	if len(pairs) == 0 {
		return 0, nil
	}
	orgID := strings.TrimSpace(s.orgID)
	if orgID == "" {
		return 0, repository.ErrMissingOrgID
	}
	if s.retryableOvertimeRecalculator == nil {
		return 0, fmt.Errorf("retryable overtime recalculator is not configured")
	}
	recalculated, err := s.retryableOvertimeRecalculator(deduplicateUserDatePairs(pairs))
	if err != nil {
		log.Printf("[AttendanceRecalc] org=%s affected_pairs=%d 加班重算失败，等待定时补偿: %s",
			orgID, len(pairs), sanitizeSyncError(err))
		return recalculated, err
	}
	if recalculated > 0 {
		log.Printf("[AttendanceRecalc] org=%s 成功重算 %d 条加班匹配", orgID, recalculated)
	}
	return recalculated, nil
}

// BuildUserDatePairsFromRecords builds deduplicated (user_id, work_date) pairs
// from dingtalk attendance records.
func BuildUserDatePairsFromRecords(records []dingtalk.AttendanceRecord) []repository.UserDatePair {
	var pairs []repository.UserDatePair
	for _, r := range records {
		uid := strings.TrimSpace(r.UserID)
		if uid == "" || r.UserCheckTime == "" {
			continue
		}
		checkTime, err := time.ParseInLocation("2006-01-02 15:04:05", r.UserCheckTime, dingtalk.ApprovalBusinessLocation())
		if err != nil {
			continue
		}
		pairs = append(pairs, attendanceAffectedUserDatePairs(uid, checkTime)...)
	}
	return deduplicateUserDatePairs(pairs)
}

// BuildUserDatePairsFromAttendanceModels builds deduplicated (user_id, work_date)
// pairs from local Attendance model records.
func BuildUserDatePairsFromAttendanceModels(records []database.Attendance) []repository.UserDatePair {
	var pairs []repository.UserDatePair
	for _, r := range records {
		uid := strings.TrimSpace(r.UserID)
		if uid == "" || r.CheckTime.IsZero() {
			continue
		}
		pairs = append(pairs, attendanceAffectedUserDatePairs(uid, r.CheckTime)...)
	}
	return deduplicateUserDatePairs(pairs)
}

func attendanceAffectedUserDatePairs(userID string, checkTime time.Time) []repository.UserDatePair {
	userID = strings.TrimSpace(userID)
	if userID == "" || checkTime.IsZero() {
		return nil
	}

	localTime := checkTime.In(dingtalk.ApprovalBusinessLocation())
	workDate := localTime.Format("2006-01-02")
	pairs := []repository.UserDatePair{{UserID: userID, WorkDate: workDate}}
	cutoff := time.Date(localTime.Year(), localTime.Month(), localTime.Day(), 6, 0, 0, 0, localTime.Location())
	if !localTime.After(cutoff) {
		pairs = append(pairs, repository.UserDatePair{
			UserID:   userID,
			WorkDate: localTime.AddDate(0, 0, -1).Format("2006-01-02"),
		})
	}
	return pairs
}

func deduplicateUserDatePairs(pairs []repository.UserDatePair) []repository.UserDatePair {
	seen := make(map[string]struct{}, len(pairs))
	result := make([]repository.UserDatePair, 0, len(pairs))
	for _, pair := range pairs {
		userID := strings.TrimSpace(pair.UserID)
		workDate := strings.TrimSpace(pair.WorkDate)
		if userID == "" || workDate == "" {
			continue
		}
		key := userID + "\x00" + workDate
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, repository.UserDatePair{UserID: userID, WorkDate: workDate})
	}
	return result
}
