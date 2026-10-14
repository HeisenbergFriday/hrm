package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

var ErrToolboxSyncInvalidRequest = errors.New("请检查日期范围、流程类型和拉取参数")

type ToolboxDingtalkSyncJob struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	RunID     string `json:"run_id,omitempty"`
}

type ToolboxDingtalkSyncJobService struct {
	repo    *repository.ToolboxSyncTaskRepository
	toolbox *AttendanceToolboxService
	orgID   string
	execute func(context.Context, string, string, string, map[string]interface{}) (*AttendanceToolboxRun, error)
}

func NewToolboxDingtalkSyncJobService(db *gorm.DB, orgID string) *ToolboxDingtalkSyncJobService {
	toolbox := NewAttendanceToolboxService()
	return &ToolboxDingtalkSyncJobService{
		repo: repository.NewToolboxSyncTaskRepository(db, orgID), toolbox: toolbox, orgID: orgID,
		execute: func(ctx context.Context, userID, orgID, module string, config map[string]interface{}) (*AttendanceToolboxRun, error) {
			return toolbox.RunStructured(ctx, userID, orgID, module, nil, config)
		},
	}
}

// Start only validates local input and persists running; no DingTalk/Python calls.
func (s *ToolboxDingtalkSyncJobService) Start(ctx context.Context, userID string, req DingtalkSyncRequest) (*ToolboxDingtalkSyncJob, error) {
	start, startErr := time.Parse("2006-01-02", req.StartDate)
	end, endErr := time.Parse("2006-01-02", req.EndDate)
	if startErr != nil || endErr != nil || start.After(end) || strings.TrimSpace(userID) == "" || strings.TrimSpace(s.orgID) == "" {
		return nil, ErrToolboxSyncInvalidRequest
	}
	if req.PaddingDays != nil && (*req.PaddingDays < 0 || *req.PaddingDays > 365) || req.MaxInstances != nil && *req.MaxInstances <= 0 {
		return nil, ErrToolboxSyncInvalidRequest
	}
	for _, flow := range req.FlowKeys {
		switch flow {
		case "leave", "overtime", "position_transfer", "attendance_correction":
		default:
			return nil, ErrToolboxSyncInvalidRequest
		}
	}
	requestID, err := newRunID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	task := &database.ApprovalSyncTask{
		Type: toolboxSyncTaskType(req.FlowKeys), RequestID: requestID, Message: "正在从钉钉拉取数据", StartedAt: now,
		Details: map[string]interface{}{"user_id": userID},
	}
	if err := s.repo.Acquire(ctx, task, now.Add(-s.staleAfter()), now); err != nil {
		return nil, err
	}
	return toolboxSyncJobView(task), nil
}

func toolboxSyncTaskType(flowKeys []string) string {
	keys := append([]string(nil), flowKeys...)
	if len(keys) == 0 {
		keys = []string{"leave", "overtime", "position_transfer", "attendance_correction"}
	}
	sort.Strings(keys)
	unique := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(unique) == 0 || unique[len(unique)-1] != key {
			unique = append(unique, key)
		}
	}
	hash := sha256.Sum256([]byte(strings.Join(unique, ",")))
	return fmt.Sprintf("%s:%x", repository.ToolboxDingtalkSyncType, hash[:4])
}

func (s *ToolboxDingtalkSyncJobService) staleAfter() time.Duration {
	return s.toolbox.timeout + time.Minute
}

func toolboxSyncJobView(task *database.ApprovalSyncTask) *ToolboxDingtalkSyncJob {
	message := "正在从钉钉拉取数据"
	switch task.Status {
	case "success":
		message = "钉钉数据拉取完成"
	case "failed":
		message = "钉钉数据拉取失败，请检查流程配置、应用权限或稍后重试"
		if task.ErrorCode == "TOOLBOX_SYNC_TIMEOUT" || task.ErrorCode == "TOOLBOX_SYNC_STALE" || task.ErrorCode == "APPROVAL_SYNC_STALE" {
			message = "钉钉数据拉取超时或任务已失效，请重新拉取"
		}
	}
	runID, _ := task.Details["run_id"].(string)
	return &ToolboxDingtalkSyncJob{RequestID: task.RequestID, Status: task.Status, Message: message, RunID: runID}
}

func (s *ToolboxDingtalkSyncJobService) Get(ctx context.Context, userID, requestID string) (*ToolboxDingtalkSyncJob, error) {
	task, err := s.repo.Find(ctx, requestID, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	changed, err := s.repo.FailIfStale(ctx, task, now.Add(-s.staleAfter()), now)
	if err != nil {
		return nil, err
	}
	if changed {
		task.Status, task.ErrorCode = "failed", "TOOLBOX_SYNC_STALE"
	}
	return toolboxSyncJobView(task), nil
}

func (s *ToolboxDingtalkSyncJobService) Run(parent context.Context, userID, requestID string, req DingtalkSyncRequest) {
	base := context.WithoutCancel(parent)
	ctx, cancel := context.WithTimeout(base, s.toolbox.timeout)
	defer cancel()
	task := &database.ApprovalSyncTask{
		Type: toolboxSyncTaskType(req.FlowKeys), RequestID: requestID, Status: "failed",
		ErrorCode: "TOOLBOX_SYNC_FAILED", Details: map[string]interface{}{"user_id": userID},
	}
	started := time.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			task.Status, task.ErrorCode = "failed", "TOOLBOX_SYNC_PANIC"
			log.Printf("[toolbox-sync] request_id=%s stage=execute panic=true", requestID)
		}
		if ctx.Err() != nil {
			task.Status, task.ErrorCode = "failed", "TOOLBOX_SYNC_TIMEOUT"
			delete(task.Details, "run_id")
		}
		finished := time.Now()
		task.FinishedAt, task.HeartbeatAt = &finished, finished
		task.DurationMS = finished.Sub(started).Milliseconds()
		task.Message = toolboxSyncJobView(task).Message
		finishCtx, finishCancel := context.WithTimeout(base, 5*time.Second)
		defer finishCancel()
		if err := s.repo.Complete(finishCtx, task); err != nil {
			log.Printf("[toolbox-sync] request_id=%s stage=complete error_type=%T", requestID, err)
		}
	}()
	config := map[string]interface{}{
		"dingtalk_sync_start_date": req.StartDate, "dingtalk_sync_end_date": req.EndDate,
	}
	if len(req.FlowKeys) > 0 {
		config["dingtalk_sync_flow_keys"] = req.FlowKeys
	}
	if req.PaddingDays != nil {
		config["dingtalk_sync_padding_days"] = *req.PaddingDays
	}
	if req.MaxInstances != nil {
		config["dingtalk_sync_max_instances"] = *req.MaxInstances
	}
	run, err := s.execute(ctx, userID, s.orgID, "dingtalk_sync", config)
	if err != nil {
		log.Printf("[toolbox-sync] request_id=%s stage=execute error_type=%T", requestID, err)
		return
	}
	if run == nil || run.RunID == "" {
		return
	}
	task.Status, task.ErrorCode = "success", ""
	task.Details["run_id"] = run.RunID
}
