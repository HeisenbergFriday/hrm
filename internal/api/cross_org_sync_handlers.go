package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/middleware"
	"peopleops/internal/repository"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type crossOrgSyncLinkRequest struct {
	TargetOrgID    string   `json:"target_org_id" binding:"required"`
	BusinessScopes []string `json:"business_scopes"`
}

type crossOrgSyncRunRequest struct {
	TargetOrgID string `json:"target_org_id" binding:"required"`
}

type peopleDataCenterSyncRequest struct {
	SourceOrgID string `json:"source_org_id"`
}

// CreateCrossOrganizationSyncLink explicitly configures a source-org to
// target-org mirror. This is an administrative cross-org operation; ordinary
// business handlers still only use the JWT organization.
func CreateCrossOrganizationSyncLink(c *gin.Context) {
	sourceOrgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var input crossOrgSyncLinkRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "目标组织参数无效"})
		return
	}
	targetOrgID := strings.TrimSpace(input.TargetOrgID)
	if targetOrgID == "" || targetOrgID == sourceOrgID {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "源组织和目标组织必须不同"})
		return
	}
	var target database.Organization
	if err := middleware.RequestDB(c).Where("org_id = ? AND status = ? AND deleted_at IS NULL", targetOrgID, "active").First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "目标组织不存在或未启用"})
		return
	}
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	if err := svc.CreateOrUpdateLink(c.Request.Context(), sourceOrgID, targetOrgID, input.BusinessScopes); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "创建同步关系失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "同步关系已保存", Data: gin.H{
		"source_org_id": sourceOrgID, "target_org_id": targetOrgID,
	}})
}

func ListCrossOrganizationSyncLinks(c *gin.Context) {
	sourceOrgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	links, err := svc.ListLinks(c.Request.Context(), sourceOrgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "读取同步关系失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": links}})
}

func RunCrossOrganizationSync(c *gin.Context) {
	sourceOrgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var input crossOrgSyncRunRequest
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.TargetOrgID) == "" {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "目标组织参数无效"})
		return
	}
	targetOrgID := strings.TrimSpace(input.TargetOrgID)
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	run, err := svc.Prepare(c.Request.Context(), sourceOrgID, targetOrgID, c.GetString("userID"))
	if err != nil {
		if errors.Is(err, repository.ErrCrossOrgSyncRunning) {
			c.JSON(http.StatusConflict, Response{Code: http.StatusConflict, Message: "该同步关系正在执行"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "同步关系不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "启动同步失败"})
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 10*time.Minute)
		defer cancel()
		_, _ = svc.Execute(ctx, run)
	}()
	c.JSON(http.StatusAccepted, Response{Code: http.StatusAccepted, Message: "同步任务已启动", Data: gin.H{
		"request_id": run.RequestID, "status": run.Status, "source_org_id": run.SourceOrgID, "target_org_id": run.TargetOrgID,
	}})
}

func GetCrossOrganizationSyncRun(c *gin.Context) {
	sourceOrgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	run, err := svc.GetRun(c.Request.Context(), sourceOrgID, c.Param("request_id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "同步任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "读取同步任务失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: run})
}

func ListCrossOrganizationEmployeeMirrors(c *gin.Context) {
	targetOrgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	limit := parseCrossOrgSyncLimit(c.Query("limit"))
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	items, err := svc.ListEmployeeMirrors(c.Request.Context(), targetOrgID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "读取同步员工资料失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items}})
}

func ListCrossOrganizationBusinessMirrors(c *gin.Context) {
	targetOrgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	limit := parseCrossOrgSyncLimit(c.Query("limit"))
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	items, err := svc.ListBusinessMirrors(c.Request.Context(), targetOrgID, c.Query("entity_type"), limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "读取同步业务数据失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items}})
}

// GetPeopleDataCenterSummary is the read-only overview for the target
// organization. It combines local target data with source-organization
// snapshots without turning snapshots into executable target business rows.
func GetPeopleDataCenterSummary(c *gin.Context) {
	targetOrgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	if !ensurePeopleDataCenterTarget(c, targetOrgID) {
		return
	}
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	summary, err := svc.PeopleDataCenterSummary(c.Request.Context(), targetOrgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "读取人事数据汇总失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: summary})
}

func ListPeopleDataCenterEmployees(c *gin.Context) {
	targetOrgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	if !ensurePeopleDataCenterTarget(c, targetOrgID) {
		return
	}
	page, pageSize := parseCrossOrgSyncPage(c)
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	items, total, err := svc.ListPeopleDataCenterEmployees(c.Request.Context(), targetOrgID, c.Query("keyword"), c.Query("status"), c.Query("source"), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "读取人事员工资料失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items, "total": total, "page": page, "page_size": pageSize}})
}

func ListPeopleDataCenterBusiness(c *gin.Context) {
	targetOrgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	if !ensurePeopleDataCenterTarget(c, targetOrgID) {
		return
	}
	page, pageSize := parseCrossOrgSyncPage(c)
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	items, total, err := svc.ListPeopleDataCenterBusiness(c.Request.Context(), targetOrgID, c.Query("entity_type"), c.Query("source"), page, pageSize)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "读取人事业务汇总失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items, "total": total, "page": page, "page_size": pageSize}})
}

func ListPeopleDataCenterInboundLinks(c *gin.Context) {
	targetOrgID, ok := currentOrgIDOrAbort(c)
	if !ok || !ensurePeopleDataCenterTarget(c, targetOrgID) {
		return
	}
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	links, err := svc.ListInboundLinks(c.Request.Context(), targetOrgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "读取同步关系失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": links}})
}

func StartPeopleDataCenterSync(c *gin.Context) {
	targetOrgID, ok := currentOrgIDOrAbort(c)
	if !ok || !ensurePeopleDataCenterTarget(c, targetOrgID) {
		return
	}
	var input peopleDataCenterSyncRequest
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "同步参数无效"})
		return
	}
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	run, err := svc.PrepareInbound(c.Request.Context(), targetOrgID, input.SourceOrgID, c.GetString("userID"))
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrCrossOrgSyncRunning):
			c.JSON(http.StatusConflict, Response{Code: http.StatusConflict, Message: "同步任务正在执行"})
		case errors.Is(err, service.ErrCrossOrgSyncMultipleSources):
			c.JSON(http.StatusConflict, Response{Code: http.StatusConflict, Message: "存在多个同步来源，请先指定来源组织"})
		case errors.Is(err, service.ErrCrossOrgSyncSourceRequired):
			c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "尚未配置文娱到沐腾的同步关系"})
		default:
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "启动同步失败"})
		}
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 10*time.Minute)
		defer cancel()
		_, _ = svc.Execute(ctx, run)
	}()
	c.JSON(http.StatusAccepted, Response{Code: http.StatusAccepted, Message: "同步任务已启动", Data: gin.H{"request_id": run.RequestID, "status": run.Status, "source_org_id": run.SourceOrgID, "target_org_id": run.TargetOrgID}})
}

func GetPeopleDataCenterSyncRun(c *gin.Context) {
	targetOrgID, ok := currentOrgIDOrAbort(c)
	if !ok || !ensurePeopleDataCenterTarget(c, targetOrgID) {
		return
	}
	svc := service.NewCrossOrganizationSyncService(middleware.RequestDB(c))
	run, err := svc.GetInboundRun(c.Request.Context(), targetOrgID, c.Param("request_id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "同步任务不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "读取同步任务失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: run})
}

func ensurePeopleDataCenterTarget(c *gin.Context, orgID string) bool {
	if database.NormalizeOrganizationID(orgID) == database.OrgIDMuteng {
		return true
	}
	c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "人事数据中心仅对沐腾组织开放"})
	return false
}

func parseCrossOrgSyncLimit(value string) int {
	var limit int
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &limit); err != nil {
		return 100
	}
	if limit <= 0 || limit > 500 {
		return 100
	}
	return limit
}

func parseCrossOrgSyncPage(c *gin.Context) (int, int) {
	page := 1
	pageSize := 20
	if _, err := fmt.Sscanf(strings.TrimSpace(c.Query("page")), "%d", &page); err != nil || page < 1 {
		page = 1
	}
	if _, err := fmt.Sscanf(strings.TrimSpace(c.Query("page_size")), "%d", &pageSize); err != nil || pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
