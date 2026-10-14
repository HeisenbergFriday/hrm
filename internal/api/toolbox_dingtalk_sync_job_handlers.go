package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"peopleops/internal/middleware"
	"peopleops/internal/repository"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type toolboxDingtalkSyncJobs interface {
	Start(context.Context, string, service.DingtalkSyncRequest) (*service.ToolboxDingtalkSyncJob, error)
	Get(context.Context, string, string) (*service.ToolboxDingtalkSyncJob, error)
	Run(context.Context, string, string, service.DingtalkSyncRequest)
}

var newToolboxDingtalkSyncJobs = func(db *gorm.DB, orgID string) toolboxDingtalkSyncJobs {
	return service.NewToolboxDingtalkSyncJobService(db, orgID)
}

var launchToolboxDingtalkSync = func(run func()) { go run() }

func StartToolboxDingtalkSync(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	if !rejectCrossOrgParam(c, orgID, c.Query("org_id"), c.Query("target_org_id"), c.GetHeader("X-Org-ID"), c.GetHeader("X-Organization-ID")) {
		return
	}
	userID := strings.TrimSpace(c.GetString("userID"))
	if userID == "" {
		c.JSON(http.StatusUnauthorized, Response{Code: http.StatusUnauthorized, Message: "请先登录"})
		return
	}
	var req struct {
		StartDate    string   `json:"start_date"`
		EndDate      string   `json:"end_date"`
		FlowKeys     []string `json:"flow_keys"`
		PaddingDays  *int     `json:"padding_days"`
		MaxInstances *int     `json:"max_instances"`
		OrgID        *string  `json:"org_id"`
		TargetOrgID  *string  `json:"target_org_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "请求参数解析失败"})
		return
	}
	if !rejectClientOrganizationID(c, req.OrgID) || !rejectClientOrganizationID(c, req.TargetOrgID) {
		return
	}
	input := service.DingtalkSyncRequest{StartDate: req.StartDate, EndDate: req.EndDate, FlowKeys: req.FlowKeys, PaddingDays: req.PaddingDays, MaxInstances: req.MaxInstances}
	jobs := newToolboxDingtalkSyncJobs(middleware.RequestDB(c), orgID)
	job, err := jobs.Start(c.Request.Context(), userID, input)
	if err != nil {
		status, message := http.StatusInternalServerError, "拉取任务启动失败，请稍后重试"
		if errors.Is(err, service.ErrToolboxSyncInvalidRequest) {
			status, message = http.StatusBadRequest, service.ErrToolboxSyncInvalidRequest.Error()
		} else if errors.Is(err, repository.ErrApprovalSyncTaskRunning) {
			status, message = http.StatusConflict, "当前组织已有同类钉钉拉取任务正在执行，请稍候"
		}
		c.JSON(status, Response{Code: status, Message: message})
		return
	}
	// Flush acceptance before scheduling any external work.
	c.JSON(http.StatusAccepted, Response{Code: http.StatusAccepted, Message: "拉取任务已启动", Data: job})
	c.Writer.Flush()
	parent := context.WithoutCancel(c.Request.Context())
	launchToolboxDingtalkSync(func() { jobs.Run(parent, userID, job.RequestID, input) })
}

func GetToolboxDingtalkSyncJob(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	job, err := newToolboxDingtalkSyncJobs(middleware.RequestDB(c), orgID).Get(c.Request.Context(), c.GetString("userID"), c.Param("request_id"))
	if err != nil {
		status, message := http.StatusInternalServerError, "拉取任务状态查询失败，请稍后查询"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status, message = http.StatusNotFound, "拉取任务不存在或无权访问"
		}
		c.JSON(status, Response{Code: status, Message: message})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: job})
}
