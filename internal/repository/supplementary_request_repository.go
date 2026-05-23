package repository

import (
	"peopleops/internal/database"
	"time"

	"gorm.io/gorm"
)

type SupplementaryRequestRepository struct {
	db *gorm.DB
}

func NewSupplementaryRequestRepository(db *gorm.DB) *SupplementaryRequestRepository {
	return &SupplementaryRequestRepository{db: db}
}

func (r *SupplementaryRequestRepository) Create(req *database.OvertimeSupplementaryRequest) error {
	return r.db.Create(req).Error
}

func (r *SupplementaryRequestRepository) FindByMatchResultID(matchResultID uint) (*database.OvertimeSupplementaryRequest, error) {
	var req database.OvertimeSupplementaryRequest
	err := r.db.Where("match_result_id = ?", matchResultID).
		Order("created_at desc").First(&req).Error
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *SupplementaryRequestRepository) FindPendingByMatchResultID(matchResultID uint) (*database.OvertimeSupplementaryRequest, error) {
	var req database.OvertimeSupplementaryRequest
	err := r.db.Where("match_result_id = ? AND status IN ?", matchResultID, []string{"pending", "approved"}).
		Order("created_at desc").First(&req).Error
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *SupplementaryRequestRepository) FindByUserID(userID, startDate, endDate string) ([]database.OvertimeSupplementaryRequest, error) {
	var reqs []database.OvertimeSupplementaryRequest
	query := r.db.Order("created_at desc")
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if startDate != "" {
		query = query.Where("work_date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("work_date <= ?", endDate)
	}
	err := query.Find(&reqs).Error
	return reqs, err
}

func (r *SupplementaryRequestRepository) FindByID(id uint) (*database.OvertimeSupplementaryRequest, error) {
	var req database.OvertimeSupplementaryRequest
	err := r.db.First(&req, id).Error
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *SupplementaryRequestRepository) Approve(id uint, approvedBy string, clockIn, clockOut time.Time) error {
	now := time.Now()
	return r.db.Model(&database.OvertimeSupplementaryRequest{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":                   "approved",
		"approved_by":              approvedBy,
		"approved_at":              &now,
		"supplementary_clock_in":   clockIn,
		"supplementary_clock_out":  clockOut,
	}).Error
}

func (r *SupplementaryRequestRepository) Reject(id uint, rejectedReason string) error {
	return r.db.Model(&database.OvertimeSupplementaryRequest{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":          "rejected",
		"rejected_reason": rejectedReason,
	}).Error
}

func (r *SupplementaryRequestRepository) UpdateDingtalkProcessID(id uint, processID string) error {
	return r.db.Model(&database.OvertimeSupplementaryRequest{}).Where("id = ?", id).Update("dingtalk_process_id", processID).Error
}

func (r *SupplementaryRequestRepository) FindByStatus(status string) ([]database.OvertimeSupplementaryRequest, error) {
	var reqs []database.OvertimeSupplementaryRequest
	err := r.db.Where("status = ?", status).Order("created_at desc").Find(&reqs).Error
	return reqs, err
}
