package api

import (
	"net/http"
	"strings"

	"peopleops/internal/middleware"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
)

func GetApprovalStats(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	filters := map[string]string{
		"template_id": strings.TrimSpace(c.Query("template_id")),
		"start_date":  strings.TrimSpace(c.Query("start_date")),
		"end_date":    strings.TrimSpace(c.Query("end_date")),
	}
	if err := validateApprovalStatsDates(filters); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "统计日期范围无效"})
		return
	}
	stats, err := service.NewApprovalStatsServiceWithOrgID(middleware.RequestDB(c), orgID).Get(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取审批统计失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: stats})
}

func validateApprovalStatsDates(filters map[string]string) error {
	start := filters["start_date"]
	end := filters["end_date"]
	if start == "" && end == "" {
		return nil
	}
	if start == "" {
		start = end
	}
	if end == "" {
		end = start
	}
	return service.ValidateApprovalSyncDates(start, end, service.ApprovalSyncNow())
}
