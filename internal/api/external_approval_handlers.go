package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
)

// ExternalApprovalDetails GET /api/v1/approvals/oa-data
func ExternalApprovalDetails(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	if database.NormalizeOrganizationID(orgID) != database.OrgIDMuteng {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "OA approval data is only available to the muteng organization"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	cfg := database.LoadExternalAttendanceConfig()
	if strings.TrimSpace(cfg.DSN) == "" {
		c.JSON(http.StatusServiceUnavailable, Response{Code: http.StatusServiceUnavailable, Message: "external approval source is not configured"})
		return
	}
	if err := database.InitExternalAttendanceDB(); err != nil {
		c.JSON(http.StatusServiceUnavailable, Response{Code: http.StatusServiceUnavailable, Message: "external approval source is unavailable"})
		return
	}
	sourceDB := database.GetExternalAttendanceDB()
	if sourceDB == nil {
		c.JSON(http.StatusServiceUnavailable, Response{Code: http.StatusServiceUnavailable, Message: "external approval source is unavailable"})
		return
	}

	repo := repository.NewExternalApprovalSourceRepository(sourceDB, cfg.QueryTimeout)
	svc := service.NewExternalApprovalService(repo, orgID)
	ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.QueryTimeout+5*time.Second)
	defer cancel()
	items, total, err := svc.List(ctx, service.ExternalApprovalQuery{
		Keyword:  c.Query("keyword"),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		if errors.Is(err, service.ErrExternalApprovalForbidden) {
			c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: err.Error()})
			return
		}
		c.JSON(http.StatusBadGateway, Response{Code: http.StatusBadGateway, Message: "failed to query external approval data"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}})
}
