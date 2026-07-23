package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"peopleops/internal/middleware"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
)

type performanceFollowupListResponse struct {
	Items   interface{}                        `json:"items"`
	Total   int64                              `json:"total"`
	Summary service.PerformanceFollowupSummary `json:"summary"`
}

func GetPerformanceInterviews(c *gin.Context) {
	if !requirePermission(c, "performance:result:view", "performance:interview:manage", "performance:activity:manage", "performance:department_eval:submit") {
		return
	}
	filter, ok := performanceFollowupFilterFromQuery(c, "performance:interview:manage", "performance:activity:manage", "performance:department_eval:submit")
	if !ok {
		return
	}
	svc := service.NewPerformanceFollowupService(middleware.RequestDB(c))
	items, total, summary, err := svc.ListInterviews(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取绩效面谈列表失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: performanceFollowupListResponse{Items: items, Total: total, Summary: summary}})
}

func CreatePerformanceInterview(c *gin.Context) {
	var req struct {
		ParticipantID   uint   `json:"participant_id" binding:"required"`
		InterviewType   string `json:"interview_type"`
		Status          string `json:"status"`
		InterviewerID   string `json:"interviewer_id"`
		InterviewerName string `json:"interviewer_name"`
		ScheduledAt     string `json:"scheduled_at"`
		Location        string `json:"location"`
		Summary         string `json:"summary"`
		Result          string `json:"result"`
		CancelReason    string `json:"cancel_reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}
	if !verifyParticipantForFollowupManage(c, req.ParticipantID, "performance:interview:manage") {
		return
	}
	scheduledAt, ok := parsePerformanceFollowupTime(c, req.ScheduledAt)
	if !ok {
		return
	}
	svc := service.NewPerformanceFollowupService(middleware.RequestDB(c))
	record, err := svc.ArrangeInterview(service.PerformanceInterviewPayload{
		ParticipantID:   req.ParticipantID,
		InterviewType:   req.InterviewType,
		Status:          req.Status,
		InterviewerID:   firstNonEmpty(req.InterviewerID, currentOperatorID(c)),
		InterviewerName: firstNonEmpty(req.InterviewerName, currentOperatorID(c)),
		ScheduledAt:     scheduledAt,
		Location:        req.Location,
		Summary:         req.Summary,
		Result:          req.Result,
		CancelReason:    req.CancelReason,
		OperatorID:      currentOperatorID(c),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: record})
}

func UpdatePerformanceInterview(c *gin.Context) {
	id, ok := parseUintParam(c, "id", "无效的面谈记录 ID")
	if !ok {
		return
	}
	canManage, err := hasPerformancePermission(c, "performance:appeal:manage", "performance:activity:manage")
	if err != nil {
		respondScopeError(c, err, "权限检查失败")
		return
	}
	if !canManage {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权处理绩效申诉", Data: nil})
		return
	}
	var req struct {
		InterviewType   string `json:"interview_type"`
		Status          string `json:"status"`
		InterviewerID   string `json:"interviewer_id"`
		InterviewerName string `json:"interviewer_name"`
		ScheduledAt     string `json:"scheduled_at"`
		Location        string `json:"location"`
		Summary         string `json:"summary"`
		Result          string `json:"result"`
		CancelReason    string `json:"cancel_reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}
	followupSvc := service.NewPerformanceFollowupService(middleware.RequestDB(c))
	existing, err := followupSvc.GetInterview(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "面谈记录不存在", Data: nil})
		return
	}
	if !verifyFollowupDepartmentAccess(c, existing.DepartmentID) {
		return
	}
	scheduledAt, ok := parsePerformanceFollowupTime(c, req.ScheduledAt)
	if !ok {
		return
	}
	record, err := followupSvc.UpdateInterview(uint(id), service.PerformanceInterviewPayload{
		InterviewType:   req.InterviewType,
		Status:          req.Status,
		InterviewerID:   firstNonEmpty(req.InterviewerID, existing.InterviewerID, currentOperatorID(c)),
		InterviewerName: firstNonEmpty(req.InterviewerName, existing.InterviewerName, currentOperatorID(c)),
		ScheduledAt:     scheduledAt,
		Location:        req.Location,
		Summary:         req.Summary,
		Result:          req.Result,
		CancelReason:    req.CancelReason,
		OperatorID:      currentOperatorID(c),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: record})
}

func GetPerformanceAppeals(c *gin.Context) {
	if !requirePermission(c, "performance:result:view", "performance:appeal:manage", "performance:activity:manage", "performance:hr_review:submit", "performance:result_publish:manage") {
		return
	}
	filter, ok := performanceFollowupFilterFromQuery(c, "performance:appeal:manage", "performance:activity:manage", "performance:hr_review:submit", "performance:result_publish:manage")
	if !ok {
		return
	}
	svc := service.NewPerformanceFollowupService(middleware.RequestDB(c))
	items, total, summary, err := svc.ListAppeals(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取绩效申诉列表失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: performanceFollowupListResponse{Items: items, Total: total, Summary: summary}})
}

func CreatePerformanceAppeal(c *gin.Context) {
	var req struct {
		ParticipantID uint   `json:"participant_id" binding:"required"`
		AppealReason  string `json:"appeal_reason" binding:"required"`
		DesiredResult string `json:"desired_result"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}
	if !verifyParticipantForAppealCreate(c, req.ParticipantID) {
		return
	}
	svc := service.NewPerformanceFollowupService(middleware.RequestDB(c))
	record, err := svc.SubmitAppeal(service.PerformanceAppealPayload{
		ParticipantID: req.ParticipantID,
		AppealReason:  req.AppealReason,
		DesiredResult: req.DesiredResult,
		OperatorID:    currentOperatorID(c),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: record})
}

func UpdatePerformanceAppeal(c *gin.Context) {
	id, ok := parseUintParam(c, "id", "无效的申诉记录 ID")
	if !ok {
		return
	}
	var req struct {
		Status        string `json:"status" binding:"required"`
		HandlerID     string `json:"handler_id"`
		HandlerName   string `json:"handler_name"`
		HandleComment string `json:"handle_comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}
	followupSvc := service.NewPerformanceFollowupService(middleware.RequestDB(c))
	existing, err := followupSvc.GetAppeal(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "申诉记录不存在", Data: nil})
		return
	}
	if !verifyFollowupDepartmentAccess(c, existing.DepartmentID) {
		return
	}
	record, err := followupSvc.UpdateAppeal(uint(id), service.PerformanceAppealPayload{
		Status:        req.Status,
		HandlerID:     firstNonEmpty(req.HandlerID, currentOperatorID(c)),
		HandlerName:   firstNonEmpty(req.HandlerName, currentOperatorID(c)),
		HandleComment: req.HandleComment,
		OperatorID:    currentOperatorID(c),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: record})
}

func WithdrawPerformanceAppeal(c *gin.Context) {
	id, ok := parseUintParam(c, "id", "无效的申诉记录 ID")
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}
	followupSvc := service.NewPerformanceFollowupService(middleware.RequestDB(c))
	existing, err := followupSvc.GetAppeal(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "申诉记录不存在", Data: nil})
		return
	}
	canManage, err := hasPerformancePermission(c, "performance:appeal:manage", "performance:activity:manage")
	if err != nil {
		respondScopeError(c, err, "权限检查失败")
		return
	}
	if canManage {
		if !verifyFollowupDepartmentAccess(c, existing.DepartmentID) {
			return
		}
	} else if !currentOperatorMatchesIdentity(c, existing.EmployeeID) {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "只能撤回自己的绩效申诉", Data: nil})
		return
	}
	record, err := followupSvc.WithdrawAppeal(uint(id), req.Reason, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: record})
}

func performanceFollowupFilterFromQuery(c *gin.Context, manageCodes ...string) (service.PerformanceFollowupListFilter, bool) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	scope, err := resolvePerformanceScope(c)
	if err != nil {
		respondScopeError(c, err, "获取数据范围失败")
		return service.PerformanceFollowupListFilter{}, false
	}
	canManage, err := hasPerformancePermission(c, manageCodes...)
	if err != nil {
		respondScopeError(c, err, "权限检查失败")
		return service.PerformanceFollowupListFilter{}, false
	}
	return service.PerformanceFollowupListFilter{
		Page:            page,
		PageSize:        pageSize,
		ActivityID:      c.Query("activity_id"),
		Status:          c.Query("status"),
		EmployeeKeyword: c.Query("employee_keyword"),
		Scope:           scope,
		IdentityValues:  currentOperatorIdentityValues(c),
		CanManage:       canManage,
	}, true
}

func verifyParticipantForFollowupManage(c *gin.Context, participantID uint, code string) bool {
	participant, err := service.NewPerformanceService(middleware.RequestDB(c)).GetParticipant(strconv.FormatUint(uint64(participantID), 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return false
	}
	return verifyPerformanceParticipantAccess(c, participant, []string{code, "performance:activity:manage"}, nil, nil)
}

func verifyParticipantForAppealCreate(c *gin.Context, participantID uint) bool {
	participant, err := service.NewPerformanceService(middleware.RequestDB(c)).GetParticipant(strconv.FormatUint(uint64(participantID), 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return false
	}
	return verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:appeal:manage", "performance:activity:manage"},
		[]string{"performance:result:view", "performance:employee_confirm:submit"},
		nil,
	)
}

func verifyFollowupDepartmentAccess(c *gin.Context, departmentID string) bool {
	if _, err := resolveAndVerifyScope(c, departmentID); err != nil {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权访问该绩效后续记录", Data: nil})
		return false
	}
	return true
}

func parsePerformanceFollowupTime(c *gin.Context, value string) (*time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, true
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"}
	for _, layout := range layouts {
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return &parsed, true
		}
	}
	c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "时间格式错误", Data: nil})
	return nil, false
}

func parseUintParam(c *gin.Context, key, message string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(key), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: message, Data: nil})
		return 0, false
	}
	return id, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
