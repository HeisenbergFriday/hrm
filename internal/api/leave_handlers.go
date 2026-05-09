package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"peopleops/internal/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ========== 年假资格 ==========

func GetLeaveEligibility(c *gin.Context) {
	userID := c.Query("user_id")
	yearStr := c.Query("year")
	if userID == "" || yearStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id 和 year 必填"})
		return
	}
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "year 格式错误"})
		return
	}
	svc := service.NewAnnualLeaveService(database.DB)
	results, err := svc.GetEligibility(userID, year)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": results})
}

func RecalculateLeaveEligibility(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id"`
		Year   int    `json:"year"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc := service.NewAnnualLeaveService(database.DB)
	if err := svc.RecalculateEligibility(req.UserID, req.Year); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "资格重算完成"})
}

// ========== 年假发放 ==========

func GetLeaveGrants(c *gin.Context) {
	userID := c.Query("user_id")
	yearStr := c.Query("year")
	if userID == "" || yearStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id 和 year 必填"})
		return
	}
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "year 格式错误"})
		return
	}
	svc := service.NewAnnualLeaveGrantService(database.DB)
	records, err := svc.GetGrantLedger(userID, year)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": records})
}

func RunQuarterGrant(c *gin.Context) {
	var req struct {
		Year    int `json:"year"`
		Quarter int `json:"quarter"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Quarter < 1 || req.Quarter > 4 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quarter 必须为 1-4"})
		return
	}
	svc := service.NewAnnualLeaveGrantService(database.DB)
	result, err := svc.GrantQuarterWithResult(req.Year, req.Quarter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": buildGrantMessage("季度年假发放完成", result), "data": result})
}

// ========== 调休管理 ==========

func GetCompensatoryLeaveBalance(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id 必填"})
		return
	}
	svc := service.NewCompensatoryLeaveService(database.DB)
	balance, err := svc.GetBalance(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": balance})
}

func ManualGrantCompensatoryLeave(c *gin.Context) {
	var req struct {
		UserID        string `json:"user_id" binding:"required"`
		Minutes       int    `json:"minutes" binding:"required,gt=0"`
		EffectiveDate string `json:"effective_date"`
		Remark        string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc := service.NewCompensatoryLeaveService(database.DB)
	if err := svc.ManualCredit(req.UserID, req.Minutes, req.EffectiveDate, req.Remark); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "手动发放调休成功"})
}

func RegrantLeave(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id"`
		Year   int    `json:"year"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc := service.NewAnnualLeaveGrantService(database.DB)
	result, err := svc.RegrantForEligibilityChangeWithResult(req.UserID, req.Year)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": buildGrantMessage("追溯发放完成", result), "data": result})
}

func buildGrantMessage(prefix string, result *service.GrantOperationResult) string {
	if result == nil {
		return prefix
	}
	if result.CreatedCount == 0 && result.DingTalkSyncedCount == 0 {
		return prefix + "，无新增或待同步记录"
	}
	return prefix
}

// ========== 工具接口 ==========

func ListVacationTypes(c *gin.Context) {
	opUserID := c.Query("op_user_id")
	if opUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "op_user_id 必填（管理员钉钉 userid）"})
		return
	}
	types, err := dingtalk.ListVacationTypes(opUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": types})
}

func SyncGrantsToDingTalk(c *gin.Context) {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请设置 confirm:true 确认操作。注意：钉钉假期余额接口是增量叠加，请确认本地记录中没有已实际同步到钉钉但状态未标记为 success 的数据，否则会导致余额重复计入。",
		})
		return
	}
	svc := service.NewAnnualLeaveGrantService(database.DB)
	result, err := svc.SyncAllGrantsToDingTalk()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "data": result})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": buildGrantMessage("补同步完成", result),
		"data":    result,
	})
}


func ConsumeAnnualLeave(c *gin.Context) {
	var req struct {
		UserID      string  `json:"user_id" binding:"required"`
		Days        float64 `json:"days" binding:"required"`
		ApprovalRef string  `json:"approval_ref"`
		Remark      string  `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Days <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "days 必须大于0"})
		return
	}
	svc := service.NewAnnualLeaveGrantService(database.DB)
	if err := svc.ConsumeAnnualLeave(req.UserID, req.Days, req.ApprovalRef, req.Remark); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "年假消费记录成功"})
}

func GetConsumeLog(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id 必填"})
		return
	}
	svc := service.NewAnnualLeaveGrantService(database.DB)
	logs, err := svc.GetConsumeLog(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": logs})
}



func GetOvertimeMatches(c *gin.Context) {
	userID := c.Query("user_id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if userID == "" || startDate == "" || endDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id, start_date, end_date 必填"})
		return
	}
	svc := service.NewOvertimeMatchingService(database.DB)
	results, err := svc.GetMatchResults(userID, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": results})
}

func RunOvertimeMatch(c *gin.Context) {
	var req struct {
		StartDate string `json:"start_date"`
		UserID    string `json:"user_id"`
		EndDate   string `json:"end_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc := service.NewOvertimeMatchingService(database.DB)
	if err := svc.MatchApprovedOvertimeForUser(req.UserID, req.StartDate, req.EndDate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "加班匹配完成"})
}

func ForceOvertimeMatch(c *gin.Context) {
	var req struct {
		ApprovalID uint `json:"approval_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc := service.NewOvertimeMatchingService(database.DB)
	if err := svc.MatchApprovalWithForce(req.ApprovalID, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "强制加班匹配完成"})
}

func SyncAndMatch(c *gin.Context) {
	var req struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.StartDate == "" {
		req.StartDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	}
	if req.EndDate == "" {
		req.EndDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	}

	// 步骤 0：从配置表读取 process_code
	ruleRepo := repository.NewOvertimeRuleConfigRepository(database.DB)
	cfg, err := ruleRepo.FindByKey("overtime.process_code")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"step": "config", "error": "未找到 overtime.process_code 配置，请先在配置表中设置",
		})
		return
	}
	var codeCfg map[string]interface{}
	if err := json.Unmarshal([]byte(cfg.RuleValueJSON), &codeCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"step": "config", "error": "overtime.process_code 格式错误"})
		return
	}
	processCode, _ := codeCfg["code"].(string)
	if processCode == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"step": "config", "error": "overtime.process_code 为空"})
		return
	}

	// 步骤 1：从钉钉拉取加班审批
	instances, err := dingtalk.GetApprovals(processCode, req.StartDate, req.EndDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"step": "sync_approvals", "error": "拉取加班审批失败: " + err.Error()})
		return
	}
	// 预加载用户信息，用于替换 ApplicantName
	var users []database.User
	database.DB.Find(&users)
	userNameMap := make(map[string]string)
	for _, u := range users {
		userNameMap[u.UserID] = u.Name
	}
	approvalCount := 0
	for _, inst := range instances {
		createTime, _ := time.Parse("2006-01-02 15:04:05", inst.CreateTime)
		finishTime, _ := time.Parse("2006-01-02 15:04:05", inst.FinishTime)
		content := make(map[string]interface{})
		for _, fv := range inst.FormValues {
			name, _ := fv["name"].(string)
			value, _ := fv["value"].(string)
			if name != "" {
				content[name] = value
			}
		}
		// 获取真实姓名
		applicantName := inst.OriginatorUserID
		if name, ok := userNameMap[inst.OriginatorUserID]; ok && name != "" {
			applicantName = name
		}
		approval := &database.Approval{
			ProcessID:     inst.ProcessInstanceID,
			Title:         inst.Title,
			ApplicantID:   inst.OriginatorUserID,
			ApplicantName: applicantName,
			Status:        inst.Status,
			CreateTime:    createTime,
			FinishTime:    finishTime,
			Content:       content,
			Extension:     map[string]interface{}{"result": inst.Result, "process_code": processCode},
		}
		var existing database.Approval
		if err := database.DB.Where("process_id = ?", inst.ProcessInstanceID).First(&existing).Error; err != nil {
			database.DB.Create(approval)
		} else {
			existing.Status = inst.Status
			existing.FinishTime = finishTime
			existing.Content = content
			existing.ApplicantName = applicantName // 更新姓名
			database.DB.Save(&existing)
		}
		approvalCount++
	}

	// 步骤 2：从钉钉拉取打卡记录
	var userIDs []string
	for userID := range userNameMap {
		if userID != "" && userID != "admin" {
			userIDs = append(userIDs, userID)
		}
	}
	attendanceCount := 0
	if len(userIDs) > 0 {
		records, err := dingtalk.GetAttendance(userIDs, req.StartDate, req.EndDate)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"step": "sync_attendance", "error": "拉取打卡记录失败: " + err.Error()})
			return
		}
		attendanceSvc := service.NewAttendanceService(database.DB)
		attendanceCount, err = attendanceSvc.SyncRecords(records, userNameMap)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"step": "sync_attendance", "error": "写入打卡记录失败: " + err.Error()})
			return
		}
	}

	// 步骤 3：加班匹配（含调休同步到钉钉）
	overtimeSvc := service.NewOvertimeMatchingService(database.DB)
	if err := overtimeSvc.MatchApprovedOvertime(req.StartDate, req.EndDate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"step": "match", "error": "加班匹配失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "同步+匹配完成",
		"data": gin.H{
			"approval_count":   approvalCount,
			"attendance_count": attendanceCount,
			"start_date":       req.StartDate,
			"end_date":         req.EndDate,
		},
	})
}

func ClearAndRematchOvertime(c *gin.Context) {
	var req struct {
		UserID    string `json:"user_id"`
		StartDate string `json:"start_date" binding:"required"`
		EndDate   string `json:"end_date" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc := service.NewOvertimeMatchingService(database.DB)
	if err := svc.ClearAndRematch(req.UserID, req.StartDate, req.EndDate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "清空并重新匹配完成"})
}

func DeleteOvertimeMatchRecords(c *gin.Context) {
	var req struct {
		UserID    string `json:"user_id"`
		StartDate string `json:"start_date" binding:"required"`
		EndDate   string `json:"end_date" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc := service.NewOvertimeMatchingService(database.DB)
	count, err := svc.DeleteMatchRecords(req.UserID, req.StartDate, req.EndDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("已删除 %d 条匹配记录", count), "deleted_count": count})
}

// ========== ManualLeave 重置与重放 ==========

const manualLeaveCode = "fd5600a2-d0df-4d9f-8022-7e5f0833130c"

func ResetManualLeave(c *gin.Context) {
	var req struct {
		DryRun bool `json:"dry_run"`
	}
	_ = c.ShouldBindJSON(&req)

	users, err := dingtalk.SyncUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取员工列表失败: " + err.Error()})
		return
	}

	type userInfo struct {
		UserID string `json:"user_id"`
		Name   string `json:"name"`
	}
	infos := make([]userInfo, 0, len(users))
	for _, u := range users {
		if u.UserID != "" {
			infos = append(infos, userInfo{UserID: u.UserID, Name: u.Name})
		}
	}

	if req.DryRun {
		c.JSON(http.StatusOK, gin.H{"count": len(infos), "users": infos})
		return
	}

	year := time.Now().Year()
	success, failed := 0, 0
	var errors []string
	for _, u := range infos {
		if err := dingtalk.InitVacationQuota(u.UserID, manualLeaveCode, year, 0, 0, "ManualLeave余额重置"); err != nil {
			failed++
			errors = append(errors, fmt.Sprintf("%s(%s): %v", u.Name, u.UserID, err))
		} else {
			success++
		}
	}

	// 钉钉余额已归零，同步将 DB 中所有记录标记为 pending，
	// 防止重放步骤跳过或后续常规同步任务重复叠加
	database.DB.Model(&database.OvertimeMatchResult{}).
		Where("effective_overtime_minutes > 0").
		Updates(map[string]interface{}{
			"dingtalk_sync_status":     "pending",
			"dingtalk_sync_request_id": "",
			"dingtalk_sync_error":      "",
		})

	c.JSON(http.StatusOK, gin.H{
		"success": success,
		"failed":  failed,
		"total":   success + failed,
		"errors":  errors,
	})
}

func ResyncOvertimeToDingTalk(c *gin.Context) {
	var req struct {
		DryRun    bool   `json:"dry_run"`
		UserID    string `json:"user_id"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}
	_ = c.ShouldBindJSON(&req)

	db := database.DB
	// 只处理未同步或失败的记录，防止重复发放
	// reset 步骤已将所有记录置为 pending，此处无需再手动重置状态
	query := db.Where("effective_overtime_minutes > 0 AND dingtalk_sync_status IN ?",
		[]string{"pending", "failed"})
	if req.UserID != "" {
		query = query.Where("user_id = ?", req.UserID)
	}
	if req.StartDate != "" {
		query = query.Where("work_date >= ?", req.StartDate)
	}
	if req.EndDate != "" {
		query = query.Where("work_date <= ?", req.EndDate)
	}

	var records []database.OvertimeMatchResult
	if err := query.Order("user_id asc, work_date asc").Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询记录失败: " + err.Error()})
		return
	}

	if req.DryRun {
		type recInfo struct {
			UserID    string `json:"user_id"`
			WorkDate  string `json:"work_date"`
			Minutes   int    `json:"minutes"`
			Status    string `json:"status"`
		}
		items := make([]recInfo, 0, len(records))
		for _, r := range records {
			items = append(items, recInfo{
				UserID:   r.UserID,
				WorkDate: r.WorkDate,
				Minutes:  r.EffectiveOvertimeMinutes,
				Status:   r.DingtalkSyncStatus,
			})
		}
		c.JSON(http.StatusOK, gin.H{"count": len(items), "records": items})
		return
	}

	success, failed := 0, 0
	var errors []string
	for _, r := range records {
		reason := fmt.Sprintf("休息日加班调休 %s %d分钟", r.WorkDate, r.EffectiveOvertimeMinutes)
		if err := dingtalk.UpdateCompensatoryLeaveQuota(r.UserID, r.EffectiveOvertimeMinutes, r.WorkDate, reason); err != nil {
			_ = db.Model(&database.OvertimeMatchResult{}).Where("id = ?", r.ID).Updates(map[string]interface{}{
				"dingtalk_sync_status": "failed",
				"dingtalk_sync_error":  err.Error(),
			})
			failed++
			errors = append(errors, fmt.Sprintf("%s %s: %v", r.UserID, r.WorkDate, err))
		} else {
			requestID := fmt.Sprintf("resync:%s:%s:%d", r.UserID, r.WorkDate, r.ID)
			_ = db.Model(&database.OvertimeMatchResult{}).Where("id = ?", r.ID).Updates(map[string]interface{}{
				"dingtalk_sync_status":     "success",
				"dingtalk_sync_request_id": requestID,
				"dingtalk_sync_error":      "",
				"match_status":             "synced",
			})
			success++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": success,
		"failed":  failed,
		"total":   success + failed,
		"errors":  errors,
	})
}

// ========== 调休余额 ==========

func GetCompTimeBalance(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id 必填"})
		return
	}
	svc := service.NewCompensatoryLeaveService(database.DB)
	balance, err := svc.GetBalance(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": balance})
}
