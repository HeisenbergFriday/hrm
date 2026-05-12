package api

import (
	"fmt"
	"net/http"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/service"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func GetPerformanceActivities(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	status := c.Query("status")
	keyword := c.Query("keyword")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	svc := service.NewPerformanceService(database.DB)
	items, total, err := svc.ListActivities(page, pageSize, status, keyword, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取绩效活动列表失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items, "total": total}})
}

func CreatePerformanceActivity(c *gin.Context) {
	var req struct {
		Name                 string `json:"name" binding:"required"`
		CycleType            string `json:"cycle_type" binding:"required"`
		StartDate            string `json:"start_date" binding:"required"`
		EndDate              string `json:"end_date" binding:"required"`
		SelfEvalStartAt      string `json:"self_eval_start_at" binding:"required"`
		SelfEvalEndAt        string `json:"self_eval_end_at" binding:"required"`
		ManagerEvalStartAt   string `json:"manager_eval_start_at" binding:"required"`
		ManagerEvalEndAt     string `json:"manager_eval_end_at" binding:"required"`
		ResultConfirmStartAt string `json:"result_confirm_start_at" binding:"required"`
		ResultConfirmEndAt   string `json:"result_confirm_end_at" binding:"required"`
		Status               string `json:"status" binding:"required"`
		TemplateID           *uint  `json:"template_id"`
		Description          string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	activity, err := svc.CreateActivity(struct {
		Name                 string
		CycleType            string
		StartDate            string
		EndDate              string
		SelfEvalStartAt      string
		SelfEvalEndAt        string
		ManagerEvalStartAt   string
		ManagerEvalEndAt     string
		ResultConfirmStartAt string
		ResultConfirmEndAt   string
		Status               string
		TemplateID           *uint
		Description          string
	}{
		Name:                 req.Name,
		CycleType:            req.CycleType,
		StartDate:            req.StartDate,
		EndDate:              req.EndDate,
		SelfEvalStartAt:      req.SelfEvalStartAt,
		SelfEvalEndAt:        req.SelfEvalEndAt,
		ManagerEvalStartAt:   req.ManagerEvalStartAt,
		ManagerEvalEndAt:     req.ManagerEvalEndAt,
		ResultConfirmStartAt: req.ResultConfirmStartAt,
		ResultConfirmEndAt:   req.ResultConfirmEndAt,
		Status:               req.Status,
		TemplateID:           req.TemplateID,
		Description:          req.Description,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"activity": activity}})
}

func GetPerformanceActivity(c *gin.Context) {
	id := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	activity, err := svc.GetActivity(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "绩效活动不存在", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"activity": activity}})
}

func UpdatePerformanceActivity(c *gin.Context) {
	id := c.Param("activity_id")
	var req struct {
		Name                 string `json:"name" binding:"required"`
		CycleType            string `json:"cycle_type" binding:"required"`
		StartDate            string `json:"start_date" binding:"required"`
		EndDate              string `json:"end_date" binding:"required"`
		SelfEvalStartAt      string `json:"self_eval_start_at" binding:"required"`
		SelfEvalEndAt        string `json:"self_eval_end_at" binding:"required"`
		ManagerEvalStartAt   string `json:"manager_eval_start_at" binding:"required"`
		ManagerEvalEndAt     string `json:"manager_eval_end_at" binding:"required"`
		ResultConfirmStartAt string `json:"result_confirm_start_at" binding:"required"`
		ResultConfirmEndAt   string `json:"result_confirm_end_at" binding:"required"`
		Status               string `json:"status" binding:"required"`
		TemplateID           *uint  `json:"template_id"`
		Description          string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	activity, err := svc.UpdateActivity(id, struct {
		Name                 string
		CycleType            string
		StartDate            string
		EndDate              string
		SelfEvalStartAt      string
		SelfEvalEndAt        string
		ManagerEvalStartAt   string
		ManagerEvalEndAt     string
		ResultConfirmStartAt string
		ResultConfirmEndAt   string
		Status               string
		TemplateID           *uint
		Description          string
	}{
		Name:                 req.Name,
		CycleType:            req.CycleType,
		StartDate:            req.StartDate,
		EndDate:              req.EndDate,
		SelfEvalStartAt:      req.SelfEvalStartAt,
		SelfEvalEndAt:        req.SelfEvalEndAt,
		ManagerEvalStartAt:   req.ManagerEvalStartAt,
		ManagerEvalEndAt:     req.ManagerEvalEndAt,
		ResultConfirmStartAt: req.ResultConfirmStartAt,
		ResultConfirmEndAt:   req.ResultConfirmEndAt,
		Status:               req.Status,
		TemplateID:           req.TemplateID,
		Description:          req.Description,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"activity": activity}})
}

func PublishPerformanceActivity(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	shouldNotify := shouldNotifyOnSelfEvaluationOpen(svc, activityID)
	if err := svc.PublishActivity(activityID); err != nil {
		msg := err.Error()
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: msg, Data: nil})
		return
	}
	queueSelfEvaluationNotification(activityID, shouldNotify)
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func ClosePerformanceActivity(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	if err := svc.CloseActivity(activityID); err != nil {
		msg := err.Error()
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: msg, Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func PutDistributionRules(c *gin.Context) {
	activityID := c.Param("activity_id")

	var req struct {
		Rules []struct {
			Level               string  `json:"level" binding:"required"`
			DistributionPercent float64 `json:"distribution_percent" binding:"required"`
			Description         string  `json:"description"`
		} `json:"rules" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)

	// 手动构造匿名结构体切片
	rules := make([]struct {
		Level               string
		DistributionPercent float64
		Description         string
	}, len(req.Rules))
	for i, r := range req.Rules {
		rules[i].Level = r.Level
		rules[i].DistributionPercent = r.DistributionPercent
		rules[i].Description = r.Description
	}

	result, err := svc.SetDistributionRules(activityID, rules)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"rules": result}})
}

func GetDistributionRules(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	rules, err := svc.GetDistributionRules(activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取分布规则失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"rules": rules}})
}

func RefreshPerformanceParticipants(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	result, err := svc.RefreshParticipants(activityID)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"result": result}})
}

func GetPerformanceParticipants(c *gin.Context) {
	activityID := c.Param("activity_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	departmentID := c.Query("department_id")
	managerID := c.Query("manager_id")
	status := c.Query("status")
	employeeKeyword := c.Query("employee_keyword")

	svc := service.NewPerformanceService(database.DB)
	items, total, err := svc.ListParticipants(activityID, page, pageSize, departmentID, managerID, status, employeeKeyword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取参与人列表失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items, "total": total}})
}

func GetParticipant(c *gin.Context) {
	participantID := c.Param("participant_id")
	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"participant": participant}})
}

func SubmitSelfEvaluation(c *gin.Context) {
	participantID := c.Param("participant_id")
	var req struct {
		SelfScore       float64  `json:"self_score" binding:"required"`
		SelfLevel       string   `json:"self_level" binding:"required"`
		SelfSummary     string   `json:"self_summary" binding:"required"`
		SelfAttachments []string `json:"self_attachments"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	version, err := svc.SubmitSelfEvaluation(participantID, struct {
		SelfScore       float64
		SelfLevel       string
		SelfSummary     string
		SelfAttachments []string
	}{
		SelfScore:       req.SelfScore,
		SelfLevel:       req.SelfLevel,
		SelfSummary:     req.SelfSummary,
		SelfAttachments: req.SelfAttachments,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"version": version}})
}

func SubmitManagerEvaluation(c *gin.Context) {
	participantID := c.Param("participant_id")
	var req struct {
		ManagerScore    float64 `json:"manager_score" binding:"required"`
		SuggestedLevel  string  `json:"suggested_level" binding:"required"`
		ManagerComment  string  `json:"manager_comment" binding:"required"`
		EvaluationItems []struct {
			ItemKey   string  `json:"item_key"`
			ItemScore float64 `json:"item_score"`
			ItemValue string  `json:"item_value"`
		} `json:"evaluation_items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)

	// 手动构造匿名结构体
	evalItems := make([]struct {
		ItemKey   string
		ItemScore float64
		ItemValue string
	}, len(req.EvaluationItems))
	for i, item := range req.EvaluationItems {
		evalItems[i].ItemKey = item.ItemKey
		evalItems[i].ItemScore = item.ItemScore
		evalItems[i].ItemValue = item.ItemValue
	}

	version, err := svc.SubmitManagerEvaluation(participantID, struct {
		ManagerScore    float64
		SuggestedLevel  string
		ManagerComment  string
		EvaluationItems []struct {
			ItemKey   string
			ItemScore float64
			ItemValue string
		}
	}{
		ManagerScore:    req.ManagerScore,
		SuggestedLevel:  req.SuggestedLevel,
		ManagerComment:  req.ManagerComment,
		EvaluationItems: evalItems,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"version": version}})
}

func BatchSubmitManagerEvaluation(c *gin.Context) {
	activityID := c.Param("activity_id")
	var req struct {
		Evaluations []struct {
			ParticipantID   uint    `json:"participant_id" binding:"required"`
			ManagerScore    float64 `json:"manager_score" binding:"required"`
			SuggestedLevel  string  `json:"suggested_level" binding:"required"`
			ManagerComment  string  `json:"manager_comment" binding:"required"`
			EvaluationItems []struct {
				ItemKey   string  `json:"item_key"`
				ItemScore float64 `json:"item_score"`
				ItemValue string  `json:"item_value"`
			} `json:"evaluation_items"`
		} `json:"evaluations" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)

	// 手动构造匿名结构体切片
	evaluations := make([]struct {
		ParticipantID   uint
		ManagerScore    float64
		SuggestedLevel  string
		ManagerComment  string
		EvaluationItems []struct {
			ItemKey   string
			ItemScore float64
			ItemValue string
		}
	}, len(req.Evaluations))

	for i, eval := range req.Evaluations {
		evalItems := make([]struct {
			ItemKey   string
			ItemScore float64
			ItemValue string
		}, len(eval.EvaluationItems))
		for j, item := range eval.EvaluationItems {
			evalItems[j].ItemKey = item.ItemKey
			evalItems[j].ItemScore = item.ItemScore
			evalItems[j].ItemValue = item.ItemValue
		}

		evaluations[i].ParticipantID = eval.ParticipantID
		evaluations[i].ManagerScore = eval.ManagerScore
		evaluations[i].SuggestedLevel = eval.SuggestedLevel
		evaluations[i].ManagerComment = eval.ManagerComment
		evaluations[i].EvaluationItems = evalItems
	}

	versions, err := svc.BatchSubmitManagerEvaluations(activityID, evaluations)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"versions": versions}})
}

func AdjustFinalLevel(c *gin.Context) {
	participantID := c.Param("participant_id")
	var req struct {
		FinalLevel string `json:"final_level" binding:"required"`
		Reason     string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	version, err := svc.AdjustFinalLevel(participantID, req.FinalLevel, req.Reason)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"version": version}})
}

func ConfirmResult(c *gin.Context) {
	participantID := c.Param("participant_id")
	var req struct {
		ConfirmComment string `json:"confirm_comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	version, err := svc.ConfirmResult(participantID, req.ConfirmComment)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"version": version}})
}

func GetParticipantVersions(c *gin.Context) {
	participantID := c.Param("participant_id")
	svc := service.NewPerformanceService(database.DB)
	versions, err := svc.GetParticipantVersions(participantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取版本记录失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"versions": versions}})
}

func GetParticipantRelationshipChangeLogs(c *gin.Context) {
	participantID := c.Param("participant_id")
	svc := service.NewPerformanceService(database.DB)
	logs, err := svc.GetParticipantRelationshipChangeLogs(participantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取关系变更日志失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"logs": logs}})
}

func GetActivityRelationshipChangeLogs(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	logs, err := svc.GetActivityRelationshipChangeLogs(activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取关系变更日志失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"logs": logs}})
}

func StartPerformanceActivity(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	shouldNotify := shouldNotifyOnSelfEvaluationOpen(svc, activityID)
	if err := svc.StartActivity(activityID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	queueSelfEvaluationNotification(activityID, shouldNotify)
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func OpenSelfEvaluation(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	if err := svc.OpenSelfEvaluation(activityID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	queueSelfEvaluationNotification(activityID, true)
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func OpenManagerEvaluation(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	if err := svc.OpenManagerEvaluation(activityID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func ConfirmActivityResults(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	if err := svc.ConfirmResults(activityID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func ArchivePerformanceActivity(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	if err := svc.ArchiveActivity(activityID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func BatchConfirmResults(c *gin.Context) {
	activityID := c.Param("activity_id")
	var req struct {
		ParticipantIDs []uint `json:"participant_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	results, err := svc.BatchConfirmResults(activityID, req.ParticipantIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"results": results}})
}

func SendSelfEvalReminder(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	if err := svc.SendSelfEvalReminders(activityID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func SendManagerEvalReminder(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	if err := svc.SendManagerEvalReminders(activityID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func TriggerPerformanceInterview(c *gin.Context) {
	participantID := c.Param("participant_id")
	var req struct {
		InterviewType string `json:"interview_type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	if err := svc.TriggerPerformanceInterview(participantID, req.InterviewType); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func GetPerformanceTemplates(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	status := c.Query("status")

	svc := service.NewPerformanceService(database.DB)
	items, total, err := svc.ListTemplates(page, pageSize, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取模板列表失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items, "total": total}})
}

func CreatePerformanceTemplate(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		Status      string `json:"status"`
		Sections    []struct {
			Name              string  `json:"name" binding:"required"`
			SectionType       string  `json:"section_type" binding:"required"`
			Weight            float64 `json:"weight" binding:"required"`
			SortOrder         int     `json:"sort_order"`
			IsScoreRequired   bool    `json:"is_score_required"`
			IsCommentRequired bool    `json:"is_comment_required"`
			Items             []struct {
				Name        string  `json:"name" binding:"required"`
				Description string  `json:"description"`
				MaxScore    float64 `json:"max_score" binding:"required"`
				Weight      float64 `json:"weight" binding:"required"`
				SortOrder   int     `json:"sort_order"`
			} `json:"items" binding:"required"`
		} `json:"sections" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := c.GetString("user_id")
	if userID == "" {
		userID = "system"
	}

	svc := service.NewPerformanceService(database.DB)

	sections := make([]struct {
		Name              string
		SectionType       string
		Weight            float64
		SortOrder         int
		IsScoreRequired   bool
		IsCommentRequired bool
		Items             []struct {
			Name        string
			Description string
			MaxScore    float64
			Weight      float64
			SortOrder   int
		}
	}, len(req.Sections))

	for i, sec := range req.Sections {
		sections[i].Name = sec.Name
		sections[i].SectionType = sec.SectionType
		sections[i].Weight = sec.Weight
		sections[i].SortOrder = sec.SortOrder
		sections[i].IsScoreRequired = sec.IsScoreRequired
		sections[i].IsCommentRequired = sec.IsCommentRequired
		sections[i].Items = make([]struct {
			Name        string
			Description string
			MaxScore    float64
			Weight      float64
			SortOrder   int
		}, len(sec.Items))
		for j, item := range sec.Items {
			sections[i].Items[j].Name = item.Name
			sections[i].Items[j].Description = item.Description
			sections[i].Items[j].MaxScore = item.MaxScore
			sections[i].Items[j].Weight = item.Weight
			sections[i].Items[j].SortOrder = item.SortOrder
		}
	}

	template, err := svc.CreateTemplate(struct {
		Name        string
		Description string
		Status      string
		Sections    []struct {
			Name              string
			SectionType       string
			Weight            float64
			SortOrder         int
			IsScoreRequired   bool
			IsCommentRequired bool
			Items             []struct {
				Name        string
				Description string
				MaxScore    float64
				Weight      float64
				SortOrder   int
			}
		}
	}{
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
		Sections:    sections,
	}, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"template": template}})
}

func GetPerformanceTemplate(c *gin.Context) {
	id := c.Param("id")
	templateID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的模板ID", Data: nil})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	result, err := svc.GetTemplate(uint(templateID))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "模板不存在", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: result})
}

func UpdatePerformanceTemplate(c *gin.Context) {
	id := c.Param("id")
	templateID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的模板ID", Data: nil})
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		Status      string `json:"status"`
		Sections    []struct {
			Name              string  `json:"name" binding:"required"`
			SectionType       string  `json:"section_type" binding:"required"`
			Weight            float64 `json:"weight" binding:"required"`
			SortOrder         int     `json:"sort_order"`
			IsScoreRequired   bool    `json:"is_score_required"`
			IsCommentRequired bool    `json:"is_comment_required"`
			Items             []struct {
				Name        string  `json:"name" binding:"required"`
				Description string  `json:"description"`
				MaxScore    float64 `json:"max_score" binding:"required"`
				Weight      float64 `json:"weight" binding:"required"`
				SortOrder   int     `json:"sort_order"`
			} `json:"items" binding:"required"`
		} `json:"sections"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := c.GetString("user_id")
	if userID == "" {
		userID = "system"
	}

	svc := service.NewPerformanceService(database.DB)

	sections := make([]struct {
		Name              string
		SectionType       string
		Weight            float64
		SortOrder         int
		IsScoreRequired   bool
		IsCommentRequired bool
		Items             []struct {
			Name        string
			Description string
			MaxScore    float64
			Weight      float64
			SortOrder   int
		}
	}, len(req.Sections))

	for i, sec := range req.Sections {
		sections[i].Name = sec.Name
		sections[i].SectionType = sec.SectionType
		sections[i].Weight = sec.Weight
		sections[i].SortOrder = sec.SortOrder
		sections[i].IsScoreRequired = sec.IsScoreRequired
		sections[i].IsCommentRequired = sec.IsCommentRequired
		sections[i].Items = make([]struct {
			Name        string
			Description string
			MaxScore    float64
			Weight      float64
			SortOrder   int
		}, len(sec.Items))
		for j, item := range sec.Items {
			sections[i].Items[j].Name = item.Name
			sections[i].Items[j].Description = item.Description
			sections[i].Items[j].MaxScore = item.MaxScore
			sections[i].Items[j].Weight = item.Weight
			sections[i].Items[j].SortOrder = item.SortOrder
		}
	}

	template, err := svc.UpdateTemplate(uint(templateID), struct {
		Name        string
		Description string
		Status      string
		Sections    []struct {
			Name              string
			SectionType       string
			Weight            float64
			SortOrder         int
			IsScoreRequired   bool
			IsCommentRequired bool
			Items             []struct {
				Name        string
				Description string
				MaxScore    float64
				Weight      float64
				SortOrder   int
			}
		}
	}{
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
		Sections:    sections,
	}, userID)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() == "模板已被活动引用，不允许修改结构" {
			statusCode = http.StatusConflict
		}
		c.JSON(statusCode, Response{Code: statusCode, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"template": template}})
}

// SubmitReviewSelfEvaluation 员工自评提交（带钉钉审批同步）
func SubmitReviewSelfEvaluation(c *gin.Context) {
	participantID := c.Param("participant_id")

	var req struct {
		SelfContentJSON struct {
			Content string `json:"content"`
		} `json:"self_content_json" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	userID := c.GetString("user_id")
	if userID == "" {
		userID = "system"
	}

	// 1. 写入自评版本记录
	_, submitErr := svc.SubmitSelfEvaluation(participantID, struct {
		SelfScore       float64
		SelfLevel       string
		SelfSummary     string
		SelfAttachments []string
	}{
		SelfSummary: req.SelfContentJSON.Content,
	})
	if submitErr != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: submitErr.Error(), Data: nil})
		return
	}

	// 2. 获取参与人信息，用于钉钉消息推送
	participant, err2 := svc.GetParticipant(participantID)
	if err2 == nil && participant != nil && participant.ManagerID != nil && *participant.ManagerID != "" {
		go func() {
			if notifyErr := dingtalk.SendCorpMessageToUser(*participant.ManagerID,
				fmt.Sprintf("【绩效提醒】%s 已提交自评", participant.EmployeeName),
				fmt.Sprintf("员工：%s\n部门：%s\n岗位：%s\n\n请及时进行主管评分。", participant.EmployeeName, participant.DepartmentName, participant.Position)); notifyErr != nil {
				logrus.Warnf("notify manager on self eval failed: %v", notifyErr)
			}
		}()
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

// SubmitReviewManagerEvaluation 主管评分提交（带钉钉审批同步）
func SubmitReviewManagerEvaluation(c *gin.Context) {
	participantID := c.Param("participant_id")

	var req struct {
		ManagerScoreJSON struct {
			KPI1 float64 `json:"KPI1,omitempty"`
		} `json:"manager_score_json"`
		ManagerComment   string `json:"manager_comment"`
		FinalLevel       string `json:"final_level" binding:"required"`
		FinalLevelReason string `json:"final_level_reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)

	managerScore := float64(0)
	scoreValues := make([]float64, 0)
	if req.ManagerScoreJSON.KPI1 > 0 {
		scoreValues = append(scoreValues, req.ManagerScoreJSON.KPI1)
	}
	if len(scoreValues) > 0 {
		var sum float64
		for _, s := range scoreValues {
			sum += s
		}
		managerScore = sum / float64(len(scoreValues))
	}

	// 1. 写入主管评分版本记录
	_, managerErr := svc.SubmitManagerEvaluation(participantID, struct {
		ManagerScore    float64
		SuggestedLevel  string
		ManagerComment  string
		EvaluationItems []struct {
			ItemKey   string
			ItemScore float64
			ItemValue string
		}
	}{
		ManagerScore:   managerScore,
		SuggestedLevel: req.FinalLevel,
		ManagerComment: req.ManagerComment,
	})
	if managerErr != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: managerErr.Error(), Data: nil})
		return
	}

	// 2. 通知员工评分结果
	go func() {
		if notifyErr := notifyEmployeeOnManagerEval(participantID, req.FinalLevel, req.ManagerComment); notifyErr != nil {
			logrus.Warnf("notify employee on manager eval failed: %v", notifyErr)
		}
	}()

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

// notifyEmployeeOnManagerEval 通知员工主管已评分
func notifyEmployeeOnManagerEval(participantID, finalLevel, comment string) error {
	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		return err
	}
	if participant == nil {
		return fmt.Errorf("participant %s not found", participantID)
	}
	return dingtalk.SendCorpMessageToUser(participant.EmployeeID,
		fmt.Sprintf("【绩效提醒】您的评分结果已出具"),
		fmt.Sprintf("员工：%s\n最终等级：%s\n主管评语：%s\n\n如对结果有疑问，请联系主管。", participant.EmployeeName, finalLevel, comment))
}

func shouldNotifyOnSelfEvaluationOpen(svc *service.PerformanceService, activityID string) bool {
	activity, err := svc.GetActivity(activityID)
	if err != nil || activity == nil {
		return false
	}
	return activity.Status != "self_evaluation"
}

func queueSelfEvaluationNotification(activityID string, shouldNotify bool) {
	if !shouldNotify {
		return
	}

	go func() {
		if err := notifyParticipantsOnSelfEvaluationOpen(activityID); err != nil {
			logrus.Warnf("notify participants on self evaluation open failed: %v", err)
		}
	}()
}

func notifyParticipantsOnSelfEvaluationOpen(activityID string) error {
	svc := service.NewPerformanceService(database.DB)
	activity, err := svc.GetActivity(activityID)
	if err != nil {
		return err
	}

	var participants []database.PerformanceParticipant
	if err := database.DB.
		Where("activity_id = ? AND deleted_at IS NULL", activityID).
		Find(&participants).Error; err != nil {
		return err
	}

	title := fmt.Sprintf("【绩效提醒】%s 已开启自评", activity.Name)
	window := formatSelfEvaluationWindow(activity.SelfEvalStartAt, activity.SelfEvalEndAt)
	sent := 0
	failures := make([]string, 0)

	for _, participant := range participants {
		if !shouldNotifyParticipant(participant) {
			continue
		}

		content := fmt.Sprintf(
			"员工：%s\n活动：%s\n自评时间：%s\n\n请及时完成自评。",
			participant.EmployeeName,
			activity.Name,
			window,
		)
		if err := dingtalk.SendCorpMessageToUser(participant.EmployeeID, title, content); err != nil {
			failures = append(failures, fmt.Sprintf("%s(%s): %v", participant.EmployeeName, participant.EmployeeID, err))
			continue
		}
		sent++
	}

	if len(failures) > 0 {
		if len(failures) > 3 {
			failures = failures[:3]
		}
		return fmt.Errorf("sent=%d failed=%d sample=%s", sent, len(failures), strings.Join(failures, "; "))
	}

	logrus.Infof("self evaluation notifications sent for activity %s: %d", activityID, sent)
	return nil
}

func shouldNotifyParticipant(participant database.PerformanceParticipant) bool {
	if strings.TrimSpace(participant.EmployeeID) == "" {
		return false
	}
	if !dingtalk.IsNotifiableUserID(participant.EmployeeID) {
		return false
	}

	if strings.TrimSpace(participant.Status) == "removed_from_scope" {
		return false
	}

	employeeStatus := strings.ToLower(strings.TrimSpace(participant.EmployeeStatus))
	return employeeStatus != "inactive" && employeeStatus != "exited"
}

func formatSelfEvaluationWindow(startAt, endAt string) string {
	start := strings.TrimSpace(startAt)
	end := strings.TrimSpace(endAt)
	switch {
	case start != "" && end != "":
		return start + " - " + end
	case start != "":
		return start
	case end != "":
		return end
	default:
		return "请查看绩效活动配置"
	}
}

func GetPerformanceResultSummary(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	result, err := svc.GetResultSummary(activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取统计摘要失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: result})
}

func GetPerformanceDistributionCheck(c *gin.Context) {
	activityID := c.Param("activity_id")
	svc := service.NewPerformanceService(database.DB)
	result, err := svc.GetDistributionCheck(activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取分布检查失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: result})
}
