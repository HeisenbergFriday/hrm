package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"peopleops/internal/database"

	"gorm.io/gorm"
)

// Toolbox sync shares the existing persistent task schema, using a separate
// type/active key so it never contends with the approval import pipeline.
const ToolboxDingtalkSyncType = "toolbox_dingtalk"

type ToolboxSyncTaskRepository struct {
	db    *gorm.DB
	orgID string
}

func NewToolboxSyncTaskRepository(db *gorm.DB, orgID string) *ToolboxSyncTaskRepository {
	return &ToolboxSyncTaskRepository{db: db, orgID: orgID}
}

func (r *ToolboxSyncTaskRepository) scoped(ctx context.Context) *ApprovalSyncTaskRepository {
	if r.db == nil {
		return NewApprovalSyncTaskRepositoryWithOrgID(nil, r.orgID)
	}
	return NewApprovalSyncTaskRepositoryWithOrgID(r.db.WithContext(ctx), r.orgID)
}

func (r *ToolboxSyncTaskRepository) Acquire(ctx context.Context, task *database.ApprovalSyncTask, staleBefore, now time.Time) error {
	if !strings.HasPrefix(task.Type, ToolboxDingtalkSyncType+":") {
		return gorm.ErrInvalidData
	}
	_, err := r.scoped(ctx).Acquire(task, staleBefore, now)
	return err
}

func (r *ToolboxSyncTaskRepository) Find(ctx context.Context, requestID, userID string) (*database.ApprovalSyncTask, error) {
	orgID, err := r.scoped(ctx).requireOrgID()
	if err != nil {
		return nil, err
	}
	var task database.ApprovalSyncTask
	err = r.db.WithContext(ctx).Where("org_id = ? AND request_id = ? AND type LIKE ?", orgID, requestID, ToolboxDingtalkSyncType+":%").First(&task).Error
	if err != nil {
		return nil, err
	}
	if userID == "" || task.Details["user_id"] != userID {
		return nil, gorm.ErrRecordNotFound
	}
	return &task, nil
}

func (r *ToolboxSyncTaskRepository) FailIfStale(ctx context.Context, task *database.ApprovalSyncTask, staleBefore, now time.Time) (bool, error) {
	orgID, err := r.scoped(ctx).requireOrgID()
	if err != nil {
		return false, err
	}
	if task.Status != "running" || task.HeartbeatAt.After(staleBefore) {
		return false, nil
	}
	result := r.db.WithContext(ctx).Model(&database.ApprovalSyncTask{}).
		Where("org_id = ? AND type = ? AND request_id = ? AND status = ? AND heartbeat_at <= ?", orgID, task.Type, task.RequestID, "running", staleBefore).
		Updates(map[string]interface{}{
			"status": "failed", "message": "钉钉拉取任务已失效，请重新拉取", "error_code": "TOOLBOX_SYNC_STALE",
			"active_key": nil, "finished_at": now, "heartbeat_at": now,
		})
	return result.RowsAffected > 0, result.Error
}

// Complete cannot resurrect a task already reclaimed by a query/new start.
func (r *ToolboxSyncTaskRepository) Complete(ctx context.Context, task *database.ApprovalSyncTask) error {
	orgID, err := r.scoped(ctx).requireOrgID()
	if err != nil {
		return err
	}
	details, err := json.Marshal(task.Details)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&database.ApprovalSyncTask{}).
		Where("org_id = ? AND type = ? AND request_id = ? AND status = ?", orgID, task.Type, task.RequestID, "running").
		Updates(map[string]interface{}{
			"status": task.Status, "message": task.Message, "error_code": task.ErrorCode,
			"duration_ms": task.DurationMS, "finished_at": task.FinishedAt,
			"heartbeat_at": task.HeartbeatAt, "details": string(details), "active_key": nil,
		}).Error
}
