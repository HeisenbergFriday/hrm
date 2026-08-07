package repository

import (
	"errors"
	"strings"
	"time"

	"peopleops/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrApprovalSyncTaskRunning = errors.New("approval sync task already running")

type ApprovalSyncTaskRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewApprovalSyncTaskRepositoryWithOrgID(db *gorm.DB, orgID string) *ApprovalSyncTaskRepository {
	normalized, err := RequireOrgID(orgID)
	return &ApprovalSyncTaskRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *ApprovalSyncTaskRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func approvalSyncActiveKey(orgID, syncType string) string {
	return orgID + "|" + strings.TrimSpace(syncType)
}

// Acquire creates an independent running task while atomically reclaiming a stale lock.
// A unique active_key closes the race between application instances.
func (r *ApprovalSyncTaskRepository) Acquire(task *database.ApprovalSyncTask, staleBefore, now time.Time) (*database.ApprovalSyncTask, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	if task == nil || strings.TrimSpace(task.RequestID) == "" || strings.TrimSpace(task.Type) == "" {
		return nil, gorm.ErrInvalidData
	}
	task.OrgID = orgID
	task.Type = strings.TrimSpace(task.Type)
	task.RequestID = strings.TrimSpace(task.RequestID)
	key := approvalSyncActiveKey(orgID, task.Type)
	task.ActiveKey = &key
	task.Status = "running"
	if task.StartedAt.IsZero() {
		task.StartedAt = now
	}
	task.HeartbeatAt = now

	var conflict *database.ApprovalSyncTask
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var current database.ApprovalSyncTask
		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("active_key = ?", key).First(&current).Error
		if findErr == nil {
			if current.Status == "running" && current.HeartbeatAt.After(staleBefore) {
				conflict = &current
				return ErrApprovalSyncTaskRunning
			}
			finishedAt := now
			if updateErr := tx.Model(&database.ApprovalSyncTask{}).
				Where("id = ? AND active_key = ?", current.ID, key).
				Updates(map[string]interface{}{
					"status": "failed", "message": "审批同步任务已失效，请重新发起",
					"error_code": "APPROVAL_SYNC_STALE", "active_key": nil,
					"finished_at": finishedAt, "heartbeat_at": now,
				}).Error; updateErr != nil {
				return updateErr
			}
		} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if createErr := tx.Create(task).Error; createErr != nil {
			var winner database.ApprovalSyncTask
			if winnerErr := tx.Where("active_key = ?", key).First(&winner).Error; winnerErr == nil {
				conflict = &winner
				return ErrApprovalSyncTaskRunning
			}
			return createErr
		}
		return nil
	})
	return conflict, err
}

func (r *ApprovalSyncTaskRepository) Find(syncType, requestID string) (*database.ApprovalSyncTask, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var task database.ApprovalSyncTask
	err = r.db.Where("org_id = ? AND type = ? AND request_id = ?", orgID, strings.TrimSpace(syncType), strings.TrimSpace(requestID)).First(&task).Error
	return &task, err
}

func (r *ApprovalSyncTaskRepository) FindActive(syncType string) (*database.ApprovalSyncTask, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var task database.ApprovalSyncTask
	err = r.db.Where("active_key = ?", approvalSyncActiveKey(orgID, syncType)).First(&task).Error
	return &task, err
}

func (r *ApprovalSyncTaskRepository) Complete(task *database.ApprovalSyncTask) error {
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	if task == nil || strings.TrimSpace(task.RequestID) == "" {
		return gorm.ErrInvalidData
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var current database.ApprovalSyncTask
		if err := tx.Where("org_id = ? AND type = ? AND request_id = ?", orgID, task.Type, task.RequestID).
			First(&current).Error; err != nil {
			return err
		}
		current.Status = task.Status
		current.Message = task.Message
		current.ErrorCode = task.ErrorCode
		current.SuccessCount = task.SuccessCount
		current.FailCount = task.FailCount
		current.FailedProcesses = task.FailedProcesses
		current.DurationMS = task.DurationMS
		current.HeartbeatAt = task.HeartbeatAt
		current.FinishedAt = task.FinishedAt
		current.Details = task.Details
		current.ActiveKey = nil
		return tx.Save(&current).Error
	})
}

func (r *ApprovalSyncTaskRepository) FailIfStale(task *database.ApprovalSyncTask, staleBefore, now time.Time, details map[string]interface{}) (bool, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return false, err
	}
	if task == nil || task.Status != "running" || task.HeartbeatAt.After(staleBefore) {
		return false, nil
	}
	var reclaimed bool
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var current database.ApprovalSyncTask
		if err := tx.Where("org_id = ? AND type = ? AND request_id = ?", orgID, task.Type, task.RequestID).
			First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if current.Status != "running" || current.HeartbeatAt.After(staleBefore) {
			return nil
		}
		finishedAt := now
		current.Status = "failed"
		current.Message = "审批同步任务已失效，请重新发起"
		current.ErrorCode = "APPROVAL_SYNC_STALE"
		current.ActiveKey = nil
		current.FinishedAt = &finishedAt
		current.HeartbeatAt = now
		current.Details = details
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		reclaimed = true
		return nil
	})
	return reclaimed, err
}
