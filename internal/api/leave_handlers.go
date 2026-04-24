package api

import (
	"net/http"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
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
		EndDate   string `json:"end_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	svc := service.NewOvertimeMatchingService(database.DB)
	if err := svc.MatchApprovedOvertime(req.StartDate, req.EndDate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "加班匹配完成"})
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
