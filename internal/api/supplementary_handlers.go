package api

import (
	"net/http"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
)

// ========== 加班补卡申请 ==========

func SubmitSupplementaryClockIn(c *gin.Context) {
	var req struct {
		MatchResultID uint   `json:"match_result_id" binding:"required"`
		ClockIn       string `json:"clock_in" binding:"required"`
		ClockOut      string `json:"clock_out" binding:"required"`
		Reason        string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	clockIn, err := time.ParseInLocation("2006-01-02 15:04", req.ClockIn, time.Local)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "补卡上班时间格式错误，应为 YYYY-MM-DD HH:MM"})
		return
	}
	clockOut, err := time.ParseInLocation("2006-01-02 15:04", req.ClockOut, time.Local)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "补卡下班时间格式错误，应为 YYYY-MM-DD HH:MM"})
		return
	}
	if !clockOut.After(clockIn) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "补卡下班时间必须晚于上班时间"})
		return
	}

	var match database.OvertimeMatchResult
	if err := database.DB.First(&match, req.MatchResultID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "匹配记录不存在"})
		return
	}
	if match.MatchStatus != "no_clock_record" && match.MatchStatus != "insufficient_clock_record" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该记录状态不允许提交补卡申请"})
		return
	}

	suppRepo := repository.NewSupplementaryRequestRepository(database.DB)
	existing, _ := suppRepo.FindPendingByMatchResultID(match.ID)
	if existing != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该记录已有待审批的补卡申请"})
		return
	}

	suppReq := &database.OvertimeSupplementaryRequest{
		MatchResultID:         match.ID,
		UserID:                match.UserID,
		WorkDate:              match.WorkDate,
		ApprovalID:            match.ApprovalID,
		SupplementaryClockIn:  clockIn,
		SupplementaryClockOut: clockOut,
		SupplementaryReason:   req.Reason,
		Status:                "pending",
	}
	if err := suppRepo.Create(suppReq); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建补卡申请失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "补卡申请已提交", "data": suppReq})
}

func ApproveSupplementaryClockIn(c *gin.Context) {
	var req struct {
		RequestID      uint   `json:"request_id" binding:"required"`
		Approved       bool   `json:"approved"`
		RejectedReason string `json:"rejected_reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	svc := service.NewOvertimeMatchingService(database.DB)
	if req.Approved {
		suppRepo := repository.NewSupplementaryRequestRepository(database.DB)
		suppReq, err := suppRepo.FindByID(req.RequestID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "补卡申请不存在"})
			return
		}
		approvedBy := currentOperatorID(c)
		if err := svc.ApproveSupplementaryRequest(req.RequestID, suppReq.SupplementaryClockIn, suppReq.SupplementaryClockOut, approvedBy); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "审批失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "补卡审批已通过，调休已重新计算"})
	} else {
		if err := svc.RejectSupplementaryRequest(req.RequestID, req.RejectedReason); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "拒绝失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "补卡申请已拒绝"})
	}
}

func GetSupplementaryRequests(c *gin.Context) {
	userID := c.Query("user_id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	svc := service.NewOvertimeMatchingService(database.DB)
	results, err := svc.GetSupplementaryRequests(userID, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": results})
}

func SyncSupplementaryFromDingTalk(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "钉钉补卡审批同步功能待实现，请先提供补卡审批的 process_code"})
}
