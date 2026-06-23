package api

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"peopleops/internal/service"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func currentOperatorID(c *gin.Context) string {
	userID := strings.TrimSpace(c.GetString("userID"))
	if userID == "" {
		return "system"
	}
	if database.DB != nil {
		if user, err := loadUserByAuthID(userID); err == nil && strings.TrimSpace(user.UserID) != "" {
			return user.UserID
		}
	}
	return userID
}

func addPerformanceIdentityValues(values map[string]struct{}, rawValues ...string) {
	for _, value := range rawValues {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		values[value] = struct{}{}
	}
}

func performanceIdentityKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	return keys
}

func currentOperatorIdentityValues(c *gin.Context) map[string]struct{} {
	values := map[string]struct{}{}
	rawUserID := strings.TrimSpace(c.GetString("userID"))
	if rawUserID == "" {
		addPerformanceIdentityValues(values, "system")
	} else {
		addPerformanceIdentityValues(values, rawUserID)
	}
	if database.DB == nil {
		return values
	}

	for _, value := range performanceIdentityKeys(values) {
		user, err := loadUserByAuthID(value)
		if err != nil {
			continue
		}
		addPerformanceIdentityValues(values, user.UserID, strconv.FormatUint(uint64(user.ID), 10))
	}

	lookupIDs := performanceIdentityKeys(values)
	if len(lookupIDs) == 0 {
		return values
	}
	var profiles []database.EmployeeProfile
	if err := database.DB.Where("(user_id IN ? OR employee_id IN ?) AND deleted_at IS NULL", lookupIDs, lookupIDs).Find(&profiles).Error; err != nil {
		return values
	}
	for _, profile := range profiles {
		addPerformanceIdentityValues(values, profile.UserID, profile.EmployeeID)
	}
	return values
}

func currentOperatorMatchesIdentity(c *gin.Context, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	rawUserID := strings.TrimSpace(c.GetString("userID"))
	operatorID := currentOperatorID(c)
	if value == rawUserID || value == operatorID {
		return true
	}
	_, ok := currentOperatorIdentityValues(c)[value]
	return ok
}

func logPerformanceNotifyError(action, userID string, err error) {
	if err == nil {
		return
	}
	if dingtalk.IsUserNotNotifiableError(err) {
		logrus.Infof("%s skipped for non-notifiable user %s: %v", action, userID, err)
		return
	}
	logrus.Warnf("%s failed: %v", action, err)
}

// resolvePerformanceScope 获取用户的数据范围（绩效模块专用）
func resolvePerformanceScope(c *gin.Context) (*service.OrgDataScope, error) {
	userID := currentOperatorID(c)
	svc := service.NewPermissionService(database.DB)
	return svc.GetUserPerformanceScope(userID)
}

// requirePermission 检查当前用户是否具有指定权限码，不满足则返回 403 并中止
func requirePermission(c *gin.Context, codes ...string) bool {
	userID := currentOperatorID(c)
	if userID == "admin" || userID == "system" {
		return true
	}
	svc := service.NewPermissionService(database.DB)
	ok, err := svc.HasAnyPermission(userID, codes...)
	if err != nil || !ok {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "权限不足", Data: nil})
		return false
	}
	return true
}

func hasPerformancePermission(c *gin.Context, codes ...string) (bool, error) {
	if len(codes) == 0 {
		return false, nil
	}
	userID := currentOperatorID(c)
	if userID == "admin" || userID == "system" {
		return true, nil
	}
	svc := service.NewPermissionService(database.DB)
	return svc.HasAnyPermission(userID, codes...)
}

// resolveAndVerifyScope 获取 scope 并验证指定部门是否在可见范围内
func resolveAndVerifyScope(c *gin.Context, departmentID string) (*service.OrgDataScope, error) {
	scope, err := resolvePerformanceScope(c)
	if err != nil {
		return nil, err
	}
	if scope != nil && !scope.IsAll() && departmentID != "" && !scope.AllowsDepartment(departmentID) {
		return nil, service.ErrOrgAccessDenied
	}
	return scope, nil
}

func respondIndicatorScopeError(c *gin.Context, err error, forbiddenMessage string) {
	if errors.Is(err, service.ErrOrgAccessDenied) {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: forbiddenMessage, Data: nil})
		return
	}
	c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取数据范围失败", Data: gin.H{"error": err.Error()}})
}

func verifyIndicatorDepartmentAccess(c *gin.Context, departmentID string, forbiddenMessage string) bool {
	if _, err := resolveAndVerifyScope(c, departmentID); err != nil {
		respondIndicatorScopeError(c, err, forbiddenMessage)
		return false
	}
	return true
}

func loadIndicatorLibraryForScope(c *gin.Context, svc *service.PerformanceIndicatorService, id uint, forbiddenMessage string) (*database.PerformanceIndicatorLibrary, bool) {
	lib, err := svc.GetLibrary(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "指标库不存在", Data: gin.H{"error": err.Error()}})
		return nil, false
	}
	if !verifyIndicatorDepartmentAccess(c, lib.DepartmentID, forbiddenMessage) {
		return nil, false
	}
	return lib, true
}

func loadIndicatorItemForScope(c *gin.Context, svc *service.PerformanceIndicatorService, id uint, forbiddenMessage string) (*database.PerformanceIndicatorItem, bool) {
	item, err := svc.GetItem(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "指标项不存在", Data: gin.H{"error": err.Error()}})
		return nil, false
	}
	if _, ok := loadIndicatorLibraryForScope(c, svc, item.LibraryID, forbiddenMessage); !ok {
		return nil, false
	}
	return item, true
}

// verifySelfParticipant 验证当前用户是指定参与人的员工本人
func verifySelfParticipant(c *gin.Context, participant *database.PerformanceParticipant) bool {
	userID := currentOperatorID(c)
	if userID == "admin" || userID == "system" {
		return true
	}
	if !currentOperatorMatchesIdentity(c, participant.EmployeeID) {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "只能操作自己的绩效数据", Data: nil})
		return false
	}
	return true
}

// verifyManagerOfParticipant 验证当前用户是指定参与人的考核上级
func verifyManagerOfParticipant(c *gin.Context, participant *database.PerformanceParticipant) bool {
	userID := currentOperatorID(c)
	if userID == "admin" || userID == "system" {
		return true
	}
	if participant.ManagerID == nil || !currentOperatorMatchesIdentity(c, *participant.ManagerID) {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "只能操作自己作为考核上级的绩效", Data: nil})
		return false
	}
	return true
}

func verifyPerformanceParticipantAccess(c *gin.Context, participant *database.PerformanceParticipant, privilegedCodes, selfCodes, managerCodes []string) bool {
	if participant == nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return false
	}

	userID := currentOperatorID(c)
	if userID == "admin" || userID == "system" {
		return true
	}

	hasSelfPermission, err := hasPerformancePermission(c, selfCodes...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "权限检查失败", Data: nil})
		return false
	}
	if hasSelfPermission && currentOperatorMatchesIdentity(c, participant.EmployeeID) {
		return true
	}

	hasManagerPermission, err := hasPerformancePermission(c, managerCodes...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "权限检查失败", Data: nil})
		return false
	}
	if hasManagerPermission && participant.ManagerID != nil && currentOperatorMatchesIdentity(c, *participant.ManagerID) {
		return true
	}

	hasPrivilegedPermission, err := hasPerformancePermission(c, privilegedCodes...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "权限检查失败", Data: nil})
		return false
	}
	if hasPrivilegedPermission {
		if _, err := resolveAndVerifyScope(c, participant.DepartmentID); err != nil {
			c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权访问该参与人数据", Data: nil})
			return false
		}
		return true
	}

	c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权访问该参与人数据", Data: nil})
	return false
}

func verifyPerformanceActivityAccess(c *gin.Context, activity *database.PerformanceActivity) bool {
	if activity == nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "绩效活动不存在", Data: nil})
		return false
	}

	userID := currentOperatorID(c)
	if userID == "admin" || userID == "system" {
		return true
	}

	scope, err := resolvePerformanceScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取数据范围失败", Data: gin.H{"error": err.Error()}})
		return false
	}
	if scope == nil || scope.IsAll() {
		return true
	}

	activityID := strconv.FormatUint(uint64(activity.ID), 10)
	if scope.IsSelf() {
		userIDs := uniqueNonEmptyStrings(scope.UserIDs, []string{userID})
		if len(userIDs) == 0 {
			c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权访问该活动", Data: nil})
			return false
		}
		var count int64
		if err := database.DB.Model(&database.PerformanceParticipant{}).
			Where("activity_id = ? AND employee_id IN ? AND deleted_at IS NULL", activityID, userIDs).
			Count(&count).Error; err != nil {
			c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "活动访问校验失败", Data: gin.H{"error": err.Error()}})
			return false
		}
		if count > 0 || stringSlicesIntersect(activity.TargetEmployeeIDs, userIDs) {
			return true
		}
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权访问该活动", Data: nil})
		return false
	}

	departmentIDs := uniqueNonEmptyStrings(scope.DepartmentIDs)
	if len(departmentIDs) == 0 {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权访问该活动", Data: nil})
		return false
	}
	var count int64
	if err := database.DB.Model(&database.PerformanceParticipant{}).
		Where("activity_id = ? AND department_id IN ? AND deleted_at IS NULL", activityID, departmentIDs).
		Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "活动访问校验失败", Data: gin.H{"error": err.Error()}})
		return false
	}
	if count > 0 ||
		stringSlicesIntersect(activity.TargetDepartmentIDs, departmentIDs) ||
		stringSlicesIntersect(activity.ApplicableOrgScope, departmentIDs) ||
		stringSliceContains(departmentIDs, activity.OrganizationID) {
		return true
	}

	c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权访问该活动", Data: nil})
	return false
}

func verifyPerformanceActivityIDAccess(c *gin.Context, activityID string) bool {
	svc := service.NewPerformanceService(database.DB)
	activity, err := svc.GetActivity(activityID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "绩效活动不存在", Data: gin.H{"error": err.Error()}})
		return false
	}
	return verifyPerformanceActivityAccess(c, activity)
}

func requirePerformanceActivityAccess(c *gin.Context, activityID string, codes ...string) bool {
	if !requirePermission(c, codes...) {
		return false
	}
	return verifyPerformanceActivityIDAccess(c, activityID)
}

func uniqueNonEmptyStrings(groups ...[]string) []string {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, group := range groups {
		for _, raw := range group {
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	return values
}

func stringSlicesIntersect(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	rightSet := make(map[string]struct{}, len(right))
	for _, raw := range right {
		value := strings.TrimSpace(raw)
		if value != "" {
			rightSet[value] = struct{}{}
		}
	}
	for _, raw := range left {
		if _, ok := rightSet[strings.TrimSpace(raw)]; ok {
			return true
		}
	}
	return false
}

func stringSliceContains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func GetPerformanceActivities(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	status := c.Query("status")
	keyword := c.Query("keyword")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	// 获取用户的数据范围（部门隔离）
	scope, err := resolvePerformanceScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取数据范围失败", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	items, total, err := svc.ListActivities(page, pageSize, status, keyword, startDate, endDate, scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取绩效活动列表失败", Data: gin.H{"error": err.Error()}})
		return
	}

	responseItems, err := attachMyParticipantToActivities(c, items)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取我的绩效参与信息失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": responseItems, "total": total}})
}

type performanceActivityListItem struct {
	database.PerformanceActivity
	MyParticipant *database.PerformanceParticipant `json:"my_participant,omitempty"`
}

func attachMyParticipantToActivities(c *gin.Context, activities []database.PerformanceActivity) ([]performanceActivityListItem, error) {
	items := make([]performanceActivityListItem, 0, len(activities))
	activityIDs := make([]string, 0, len(activities))
	for _, activity := range activities {
		items = append(items, performanceActivityListItem{PerformanceActivity: activity})
		if activity.ID > 0 {
			activityIDs = append(activityIDs, strconv.FormatUint(uint64(activity.ID), 10))
		}
	}
	if len(activityIDs) == 0 {
		return items, nil
	}

	myParticipants, err := findMyPerformanceParticipantsByActivity(c, activityIDs)
	if err != nil {
		return nil, err
	}
	if len(myParticipants) == 0 {
		return items, nil
	}

	for i := range items {
		activityID := strconv.FormatUint(uint64(items[i].ID), 10)
		if participant, ok := myParticipants[activityID]; ok {
			participantCopy := participant
			items[i].MyParticipant = &participantCopy
		}
	}
	return items, nil
}

func CreatePerformanceActivity(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	var req struct {
		Name                           string                                          `json:"name" binding:"required"`
		CycleType                      string                                          `json:"cycle_type" binding:"required"`
		StartDate                      string                                          `json:"start_date" binding:"required"`
		EndDate                        string                                          `json:"end_date" binding:"required"`
		TemplateID                     *uint                                           `json:"template_id"`
		FlowType                       string                                          `json:"flow_type"`
		OrganizationID                 string                                          `json:"organization_id"`
		ApplicableOrgScope             []string                                        `json:"applicable_org_scope"`
		TargetSetStartAt               string                                          `json:"target_set_start_at"`
		TargetSetEndAt                 string                                          `json:"target_set_end_at"`
		SelfEvalStartAt                string                                          `json:"self_eval_start_at" binding:"required"`
		SelfEvalEndAt                  string                                          `json:"self_eval_end_at" binding:"required"`
		ManagerEvalStartAt             string                                          `json:"manager_eval_start_at" binding:"required"`
		ManagerEvalEndAt               string                                          `json:"manager_eval_end_at" binding:"required"`
		ResultConfirmStartAt           string                                          `json:"result_confirm_start_at" binding:"required"`
		ResultConfirmEndAt             string                                          `json:"result_confirm_end_at" binding:"required"`
		EmployeeConfirmStartAt         string                                          `json:"employee_confirm_start_at"`
		EmployeeConfirmEndAt           string                                          `json:"employee_confirm_end_at"`
		ManagerConfirmStartAt          string                                          `json:"manager_confirm_start_at"`
		ManagerConfirmEndAt            string                                          `json:"manager_confirm_end_at"`
		HRConfirmStartAt               string                                          `json:"hr_confirm_start_at"`
		HRConfirmEndAt                 string                                          `json:"hr_confirm_end_at"`
		HRConfirmDeadline              string                                          `json:"hr_confirm_deadline"`
		Status                         string                                          `json:"status" binding:"required"`
		TargetDepartmentIDs            []string                                        `json:"target_department_ids"`
		TargetEmployeeIDs              []string                                        `json:"target_employee_ids"`
		ManagerAssignments             []database.PerformanceActivityManagerAssignment `json:"manager_assignments"`
		IndicatorLibraryID             *uint                                           `json:"indicator_library_id"`
		Description                    string                                          `json:"description"`
		DefaultAssessmentManagerSource string                                          `json:"default_assessment_manager_source"`
		SnapshotAsOfDate               string                                          `json:"snapshot_as_of_date"`
		SnapshotSource                 string                                          `json:"snapshot_source"`
		TargetPlanActivityID           *uint                                           `json:"target_plan_activity_id"`
		PreviousReviewActivityID       *uint                                           `json:"previous_review_activity_id"`
		PublishMode                    string                                          `json:"publish_mode"`
		PublishAt                      string                                          `json:"publish_at"`
		ReminderConfig                 map[string]interface{}                          `json:"reminder_config"`
		EnableBonusScore               bool                                            `json:"enable_bonus_score"`
		StrictTimeMode                 bool                                            `json:"strict_time_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	activity, err := svc.CreateActivity(service.CreateActivityRequest{
		Name:                           req.Name,
		CycleType:                      req.CycleType,
		StartDate:                      req.StartDate,
		EndDate:                        req.EndDate,
		TemplateID:                     req.TemplateID,
		FlowType:                       req.FlowType,
		OrganizationID:                 req.OrganizationID,
		ApplicableOrgScope:             req.ApplicableOrgScope,
		TargetSetStartAt:               req.TargetSetStartAt,
		TargetSetEndAt:                 req.TargetSetEndAt,
		SelfEvalStartAt:                req.SelfEvalStartAt,
		SelfEvalEndAt:                  req.SelfEvalEndAt,
		ManagerEvalStartAt:             req.ManagerEvalStartAt,
		ManagerEvalEndAt:               req.ManagerEvalEndAt,
		ResultConfirmStartAt:           req.ResultConfirmStartAt,
		ResultConfirmEndAt:             req.ResultConfirmEndAt,
		EmployeeConfirmStartAt:         req.EmployeeConfirmStartAt,
		EmployeeConfirmEndAt:           req.EmployeeConfirmEndAt,
		ManagerConfirmStartAt:          req.ManagerConfirmStartAt,
		ManagerConfirmEndAt:            req.ManagerConfirmEndAt,
		HRConfirmStartAt:               req.HRConfirmStartAt,
		HRConfirmEndAt:                 req.HRConfirmEndAt,
		HRConfirmDeadline:              req.HRConfirmDeadline,
		Status:                         req.Status,
		TargetDepartmentIDs:            req.TargetDepartmentIDs,
		TargetEmployeeIDs:              req.TargetEmployeeIDs,
		ManagerAssignments:             req.ManagerAssignments,
		IndicatorLibraryID:             req.IndicatorLibraryID,
		Description:                    req.Description,
		DefaultAssessmentManagerSource: req.DefaultAssessmentManagerSource,
		SnapshotAsOfDate:               req.SnapshotAsOfDate,
		SnapshotSource:                 req.SnapshotSource,
		TargetPlanActivityID:           req.TargetPlanActivityID,
		PreviousReviewActivityID:       req.PreviousReviewActivityID,
		PublishMode:                    req.PublishMode,
		PublishAt:                      req.PublishAt,
		ReminderConfig:                 req.ReminderConfig,
		EnableBonusScore:               req.EnableBonusScore,
		StrictTimeMode:                 req.StrictTimeMode,
	}, currentOperatorID(c))
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
	if !verifyPerformanceActivityAccess(c, activity) {
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"activity": activity}})
}

func UpdatePerformanceActivity(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	id := c.Param("activity_id")
	var req struct {
		Name                           string                                          `json:"name" binding:"required"`
		CycleType                      string                                          `json:"cycle_type" binding:"required"`
		StartDate                      string                                          `json:"start_date" binding:"required"`
		EndDate                        string                                          `json:"end_date" binding:"required"`
		TemplateID                     *uint                                           `json:"template_id"`
		FlowType                       string                                          `json:"flow_type"`
		OrganizationID                 string                                          `json:"organization_id"`
		ApplicableOrgScope             []string                                        `json:"applicable_org_scope"`
		TargetSetStartAt               string                                          `json:"target_set_start_at"`
		TargetSetEndAt                 string                                          `json:"target_set_end_at"`
		SelfEvalStartAt                string                                          `json:"self_eval_start_at" binding:"required"`
		SelfEvalEndAt                  string                                          `json:"self_eval_end_at" binding:"required"`
		ManagerEvalStartAt             string                                          `json:"manager_eval_start_at" binding:"required"`
		ManagerEvalEndAt               string                                          `json:"manager_eval_end_at" binding:"required"`
		ResultConfirmStartAt           string                                          `json:"result_confirm_start_at" binding:"required"`
		ResultConfirmEndAt             string                                          `json:"result_confirm_end_at" binding:"required"`
		EmployeeConfirmStartAt         string                                          `json:"employee_confirm_start_at"`
		EmployeeConfirmEndAt           string                                          `json:"employee_confirm_end_at"`
		ManagerConfirmStartAt          string                                          `json:"manager_confirm_start_at"`
		ManagerConfirmEndAt            string                                          `json:"manager_confirm_end_at"`
		HRConfirmStartAt               string                                          `json:"hr_confirm_start_at"`
		HRConfirmEndAt                 string                                          `json:"hr_confirm_end_at"`
		HRConfirmDeadline              string                                          `json:"hr_confirm_deadline"`
		TargetDepartmentIDs            []string                                        `json:"target_department_ids"`
		TargetEmployeeIDs              []string                                        `json:"target_employee_ids"`
		ManagerAssignments             []database.PerformanceActivityManagerAssignment `json:"manager_assignments"`
		IndicatorLibraryID             *uint                                           `json:"indicator_library_id"`
		Description                    string                                          `json:"description"`
		DefaultAssessmentManagerSource string                                          `json:"default_assessment_manager_source"`
		SnapshotAsOfDate               string                                          `json:"snapshot_as_of_date"`
		SnapshotSource                 string                                          `json:"snapshot_source"`
		TargetPlanActivityID           *uint                                           `json:"target_plan_activity_id"`
		PreviousReviewActivityID       *uint                                           `json:"previous_review_activity_id"`
		PublishMode                    string                                          `json:"publish_mode"`
		PublishAt                      string                                          `json:"publish_at"`
		ReminderConfig                 map[string]interface{}                          `json:"reminder_config"`
		EnableBonusScore               bool                                            `json:"enable_bonus_score"`
		StrictTimeMode                 bool                                            `json:"strict_time_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}
	if !verifyPerformanceActivityIDAccess(c, id) {
		return
	}

	svc := service.NewPerformanceService(database.DB)
	activity, err := svc.UpdateActivity(id, service.CreateActivityRequest{
		Name:                           req.Name,
		CycleType:                      req.CycleType,
		StartDate:                      req.StartDate,
		EndDate:                        req.EndDate,
		TemplateID:                     req.TemplateID,
		FlowType:                       req.FlowType,
		OrganizationID:                 req.OrganizationID,
		ApplicableOrgScope:             req.ApplicableOrgScope,
		TargetSetStartAt:               req.TargetSetStartAt,
		TargetSetEndAt:                 req.TargetSetEndAt,
		SelfEvalStartAt:                req.SelfEvalStartAt,
		SelfEvalEndAt:                  req.SelfEvalEndAt,
		ManagerEvalStartAt:             req.ManagerEvalStartAt,
		ManagerEvalEndAt:               req.ManagerEvalEndAt,
		ResultConfirmStartAt:           req.ResultConfirmStartAt,
		ResultConfirmEndAt:             req.ResultConfirmEndAt,
		EmployeeConfirmStartAt:         req.EmployeeConfirmStartAt,
		EmployeeConfirmEndAt:           req.EmployeeConfirmEndAt,
		ManagerConfirmStartAt:          req.ManagerConfirmStartAt,
		ManagerConfirmEndAt:            req.ManagerConfirmEndAt,
		HRConfirmStartAt:               req.HRConfirmStartAt,
		HRConfirmEndAt:                 req.HRConfirmEndAt,
		HRConfirmDeadline:              req.HRConfirmDeadline,
		TargetDepartmentIDs:            req.TargetDepartmentIDs,
		TargetEmployeeIDs:              req.TargetEmployeeIDs,
		ManagerAssignments:             req.ManagerAssignments,
		IndicatorLibraryID:             req.IndicatorLibraryID,
		Description:                    req.Description,
		DefaultAssessmentManagerSource: req.DefaultAssessmentManagerSource,
		SnapshotAsOfDate:               req.SnapshotAsOfDate,
		SnapshotSource:                 req.SnapshotSource,
		TargetPlanActivityID:           req.TargetPlanActivityID,
		PreviousReviewActivityID:       req.PreviousReviewActivityID,
		PublishMode:                    req.PublishMode,
		PublishAt:                      req.PublishAt,
		ReminderConfig:                 req.ReminderConfig,
		EnableBonusScore:               req.EnableBonusScore,
		StrictTimeMode:                 req.StrictTimeMode,
	}, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"activity": activity}})
}

func PublishPerformanceActivity(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	shouldNotify := shouldNotifyOnSelfEvaluationOpen(svc, activityID)
	if err := svc.PublishActivity(activityID, currentOperatorID(c)); err != nil {
		msg := err.Error()
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: msg, Data: nil})
		return
	}
	queueSelfEvaluationNotification(activityID, shouldNotify)
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func ClosePerformanceActivity(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.CloseActivity(activityID, currentOperatorID(c)); err != nil {
		msg := err.Error()
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: msg, Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func PutDistributionRules(c *gin.Context) {
	if !requirePermission(c, "performance:distribution:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}

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

	result, err := svc.SetDistributionRules(activityID, rules, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"rules": result}})
}

func GetDistributionRules(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:distribution:manage", "performance:activity:manage") {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	rules, err := svc.GetDistributionRules(activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取分布规则失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"rules": rules}})
}

func GetRealtimeDistributionCheck(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:distribution:manage", "performance:activity:manage") {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	teams, err := svc.GetRealtimeDistributionCheck(activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取实时分布检查失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"teams": teams}})
}

func RefreshPerformanceParticipants(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	result, err := svc.RefreshParticipants(activityID, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"result": result}})
}

func ImportPerformanceActivityParticipants(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "请上传 Excel 文件", Data: gin.H{"error": err.Error()}})
		return
	}
	if strings.ToLower(filepath.Ext(file.Filename)) != ".xlsx" {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "仅支持 .xlsx Excel 文件", Data: nil})
		return
	}

	reader, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "读取上传文件失败", Data: gin.H{"error": err.Error()}})
		return
	}
	defer func() {
		if err := reader.Close(); err != nil {
			logrus.Warnf("close performance participant import file failed: %v", err)
		}
	}()

	result, err := service.ParsePerformanceParticipantImportXLSX(reader)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	if err := svc.ResolveImportedPerformanceEmployees(result); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "匹配员工信息失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"result": result}})
}

func GetPerformanceParticipants(c *gin.Context) {
	if !requirePermission(c, "performance:result:view", "performance:activity:manage", "performance:goal:manage", "performance:manager_eval:submit", "performance:hr_confirm:submit") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	departmentID := c.Query("department_id")
	managerID := c.Query("manager_id")
	status := c.Query("status")
	employeeKeyword := c.Query("employee_keyword")

	// 获取用户的数据范围（部门隔离）
	scope, err := resolvePerformanceScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取数据范围失败", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	items, total, err := svc.ListParticipants(activityID, page, pageSize, departmentID, managerID, status, employeeKeyword, scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取参与人列表失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items, "total": total}})
}

func GetMyPerformanceParticipants(c *gin.Context) {
	rawActivityIDs := strings.TrimSpace(c.Query("activity_ids"))
	if rawActivityIDs == "" {
		c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items_by_activity": gin.H{}}})
		return
	}

	activityIDs := make([]string, 0)
	seenActivityIDs := make(map[string]struct{})
	for _, raw := range strings.Split(rawActivityIDs, ",") {
		activityID := strings.TrimSpace(raw)
		if activityID == "" {
			continue
		}
		if _, exists := seenActivityIDs[activityID]; exists {
			continue
		}
		seenActivityIDs[activityID] = struct{}{}
		activityIDs = append(activityIDs, activityID)
	}
	if len(activityIDs) == 0 {
		c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items_by_activity": gin.H{}}})
		return
	}

	itemsByActivity, err := findMyPerformanceParticipantsByActivity(c, activityIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取我的参与人失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items_by_activity": itemsByActivity}})
}

func findMyPerformanceParticipantsByActivity(c *gin.Context, activityIDs []string) (map[string]database.PerformanceParticipant, error) {
	result := make(map[string]database.PerformanceParticipant)
	rawUserID := strings.TrimSpace(c.GetString("userID"))
	if rawUserID == "" || rawUserID == "admin" || rawUserID == "system" {
		return result, nil
	}
	identitySet := currentOperatorIdentityValues(c)
	if _, ok := identitySet["admin"]; ok {
		return result, nil
	}
	identityValues := performanceIdentityKeys(identitySet)
	if len(activityIDs) == 0 || len(identityValues) == 0 {
		return result, nil
	}

	var participants []database.PerformanceParticipant
	if err := database.DB.
		Where("activity_id IN ? AND employee_id IN ? AND deleted_at IS NULL", activityIDs, identityValues).
		Order("id ASC").
		Find(&participants).Error; err != nil {
		return nil, err
	}
	for _, participant := range participants {
		activityID := strings.TrimSpace(participant.ActivityID)
		if activityID == "" {
			continue
		}
		if _, exists := result[activityID]; exists {
			continue
		}
		result[activityID] = participant
	}
	return result, nil
}

func GetParticipant(c *gin.Context) {
	participantID := c.Param("participant_id")
	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: gin.H{"error": err.Error()}})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:result:view", "performance:activity:manage", "performance:goal:manage", "performance:hr_confirm:submit"},
		[]string{"performance:self_eval:submit", "performance:employee_confirm:submit"},
		[]string{"performance:manager_eval:submit", "performance:manager_confirm:submit"},
	) {
		return
	}
	svc.HydrateParticipantTargetConfirmers(participant)
	normalizeParticipantConfirmers(participant)
	var activity *database.PerformanceActivity
	if participant.ActivityID != "" {
		activity, _ = svc.GetActivity(participant.ActivityID)
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"participant": participant, "activity": activity}})
}

func flexibleJSONString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(v, 'f', -1, 64))
	case float32:
		return strings.TrimSpace(strconv.FormatFloat(float64(v), 'f', -1, 32))
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func UpdateParticipantAssessmentManager(c *gin.Context) {
	if !requirePermission(c, "performance:assessment_manager:update") {
		return
	}
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}
	var req struct {
		ManagerUserID interface{} `json:"manager_user_id" binding:"required"`
		ManagerSource string      `json:"manager_source"`
		Reason        string      `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if _, err := resolveAndVerifyScope(c, participant.DepartmentID); err != nil {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权调整该参与人的考核上级", Data: nil})
		return
	}

	updated, err := svc.UpdateParticipantAssessmentManager(uint(participantID), flexibleJSONString(req.ManagerUserID), req.ManagerSource, req.Reason, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"participant": updated}})
}

func BatchUpdateAssessmentManagers(c *gin.Context) {
	if !requirePermission(c, "performance:assessment_manager:batch_update") {
		return
	}
	activityID := c.Param("activity_id")
	var req struct {
		Items []struct {
			ParticipantID uint        `json:"participant_id" binding:"required"`
			ManagerUserID interface{} `json:"manager_user_id" binding:"required"`
			ManagerSource string      `json:"manager_source"`
			Reason        string      `json:"reason"`
		} `json:"items" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	items := make([]service.AssessmentManagerUpdateRequest, 0, len(req.Items))
	for _, item := range req.Items {
		participant, err := svc.GetParticipant(strconv.FormatUint(uint64(item.ParticipantID), 10))
		if err != nil {
			c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: gin.H{"participant_id": item.ParticipantID}})
			return
		}
		if participant.ActivityID != activityID {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参与人不属于当前活动", Data: gin.H{"participant_id": item.ParticipantID}})
			return
		}
		if _, err := resolveAndVerifyScope(c, participant.DepartmentID); err != nil {
			c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权调整部分参与人的考核上级", Data: gin.H{"participant_id": item.ParticipantID}})
			return
		}
		items = append(items, service.AssessmentManagerUpdateRequest{
			ParticipantID: item.ParticipantID,
			ManagerUserID: flexibleJSONString(item.ManagerUserID),
			ManagerSource: item.ManagerSource,
			Reason:        item.Reason,
		})
	}

	results, err := svc.BatchUpdateActivityAssessmentManagers(activityID, items, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"results": results}})
}

func GetAssessmentManagerCandidates(c *gin.Context) {
	if !requirePermission(c, "performance:assessment_manager:update") {
		return
	}
	activityID := c.Param("activity_id")
	var participantID uint
	if raw := strings.TrimSpace(c.Query("participant_id")); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
			return
		}
		participantID = uint(parsed)
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	svc := service.NewPerformanceService(database.DB)
	if participantID > 0 {
		participant, err := svc.GetParticipant(strconv.FormatUint(uint64(participantID), 10))
		if err != nil {
			c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
			return
		}
		if _, err := resolveAndVerifyScope(c, participant.DepartmentID); err != nil {
			c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权访问该参与人数据", Data: nil})
			return
		}
	}
	candidates, err := svc.ListAssessmentManagerCandidates(activityID, participantID, c.Query("source"), c.Query("keyword"), limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	sources, err := svc.ListAssessmentManagerCandidateSourceGroups(activityID, participantID, c.Query("keyword"), limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": candidates, "sources": sources}})
}

func SubmitSelfEvaluation(c *gin.Context) {
	if !requirePermission(c, "performance:self_eval:submit") {
		return
	}
	participantID := c.Param("participant_id")

	// 身份校验：验证当前用户是该参与人的员工本人
	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifySelfParticipant(c, participant) {
		return
	}

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
	}, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"version": version}})
}

func SubmitManagerEvaluation(c *gin.Context) {
	if !requirePermission(c, "performance:manager_eval:submit") {
		return
	}
	participantID := c.Param("participant_id")

	// 身份校验：验证当前用户是该参与人的主管
	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyManagerOfParticipant(c, participant) {
		return
	}

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
	}, currentOperatorID(c))
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
			BonusScore      float64 `json:"bonus_score"`
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

	// 查询活动配置，判断是否启用附加分
	activity, _ := svc.GetActivity(activityID)
	enableBonus := activity != nil && activity.EnableBonusScore

	for _, eval := range req.Evaluations {
		participant, err := svc.GetParticipant(strconv.FormatUint(uint64(eval.ParticipantID), 10))
		if err != nil {
			c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: gin.H{"participant_id": eval.ParticipantID}})
			return
		}
		if participant.ActivityID != activityID {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参与人不属于当前活动", Data: gin.H{"participant_id": eval.ParticipantID}})
			return
		}
		if !verifyManagerOfParticipant(c, participant) {
			return
		}
	}

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

		// 如果活动启用了附加分，将附加分加入总分并重新计算等级
		if enableBonus && eval.BonusScore != 0 {
			adjustedScore := eval.ManagerScore + eval.BonusScore
			evaluations[i].SuggestedLevel = service.PerformanceLevelByActivity(adjustedScore, activity)
		}
	}

	versions, err := svc.BatchSubmitManagerEvaluations(activityID, evaluations, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"versions": versions}})
}

func AdjustFinalLevel(c *gin.Context) {
	if !requirePermission(c, "performance:level_adjust:manage") {
		return
	}
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
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:level_adjust:manage", "performance:activity:manage"},
		nil,
		nil,
	) {
		return
	}
	version, err := svc.AdjustFinalLevel(participantID, req.FinalLevel, req.Reason, currentOperatorID(c))
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
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyManagerOfParticipant(c, participant) {
		return
	}
	version, err := svc.ConfirmResult(participantID, req.ConfirmComment, currentOperatorName(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"version": version}})
}

func currentOperatorName(c *gin.Context) string {
	userID := strings.TrimSpace(c.GetString("userID"))
	if userID != "" {
		if user, err := loadUserByAuthID(userID); err == nil && strings.TrimSpace(user.Name) != "" {
			return strings.TrimSpace(user.Name)
		}
	}

	userName := strings.TrimSpace(c.GetString("userName"))
	if userName != "" {
		return userName
	}
	if userID != "" {
		return userID
	}

	return "system"
}

func displayUserName(userID string) string {
	value := strings.TrimSpace(userID)
	if value == "" {
		return ""
	}
	if user, err := loadUserByAuthID(value); err == nil && strings.TrimSpace(user.Name) != "" {
		return strings.TrimSpace(user.Name)
	}
	return value
}

func normalizeParticipantConfirmers(participant *database.PerformanceParticipant) {
	if participant == nil {
		return
	}
	if name := displayUserName(participant.EmployeeConfirmedBy); name != "" {
		participant.EmployeeConfirmedBy = name
	}
	if name := displayUserName(participant.ManagerConfirmedBy); name != "" {
		participant.ManagerConfirmedBy = name
	}
	if name := displayUserName(participant.HRConfirmedBy); name != "" {
		participant.HRConfirmedBy = name
	}
	if name := displayUserName(participant.EmployeeTargetConfirmedBy); name != "" {
		participant.EmployeeTargetConfirmedBy = name
	}
	if name := displayUserName(participant.ManagerTargetConfirmedBy); name != "" {
		participant.ManagerTargetConfirmedBy = name
	}
	if name := displayUserName(participant.HRTargetConfirmedBy); name != "" {
		participant.HRTargetConfirmedBy = name
	}
	if name := displayUserName(participant.ConfirmedBy); name != "" {
		participant.ConfirmedBy = name
	}
	if name := displayUserName(participant.LockedBy); name != "" {
		participant.LockedBy = name
	}
	if name := displayUserName(participant.UpdatedBy); name != "" {
		participant.UpdatedBy = name
	}
}

func ConfirmEmployeeResultHandler(c *gin.Context) {
	if !requirePermission(c, "performance:employee_confirm:submit") {
		return
	}
	participantID, err := strconv.Atoi(c.Param("participant_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: nil})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.Itoa(participantID))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifySelfParticipant(c, participant) {
		return
	}
	if err := svc.ConfirmEmployeeResult(uint(participantID), currentOperatorName(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "员工确认成功", Data: nil})
}

func ConfirmManagerResultHandler(c *gin.Context) {
	if !requirePermission(c, "performance:manager_confirm:submit") {
		return
	}
	participantID, err := strconv.Atoi(c.Param("participant_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: nil})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.Itoa(participantID))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyManagerOfParticipant(c, participant) {
		return
	}
	if err := svc.ConfirmManagerResult(uint(participantID), currentOperatorName(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	go func() {
		participant, err := svc.GetParticipant(strconv.Itoa(participantID))
		if err == nil && participant != nil && participant.EmployeeID != "" {
			if err := dingtalk.SendCorpActionCardToUser(participant.EmployeeID,
				"绩效结果锁定通知",
				fmt.Sprintf("您的绩效结果已锁定，最终等级为 %s。", participant.FinalLevel),
				"查看绩效结果",
				service.PerformanceResultURL(participant.ActivityID, participant.ID)); err != nil {
				logPerformanceNotifyError("notify employee on manager confirm lock", participant.EmployeeID, err)
			}
		}
	}()
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "主管确认成功", Data: nil})
}

func ConfirmHRResultHandler(c *gin.Context) {
	if !requirePermission(c, "performance:hr_confirm:submit") {
		return
	}
	participantID, err := strconv.Atoi(c.Param("participant_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: nil})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.Itoa(participantID))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:hr_confirm:submit", "performance:activity:manage"},
		nil,
		nil,
	) {
		return
	}
	if err := svc.ConfirmHRResult(uint(participantID), currentOperatorName(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "HR确认成功", Data: nil})
}

func GetParticipantVersions(c *gin.Context) {
	participantID := c.Param("participant_id")
	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:result:view", "performance:activity:manage", "performance:hr_confirm:submit"},
		[]string{"performance:result:view", "performance:self_eval:submit", "performance:employee_confirm:submit"},
		[]string{"performance:result:view", "performance:manager_eval:submit", "performance:manager_confirm:submit"},
	) {
		return
	}
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
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:activity:manage", "performance:assessment_manager:update", "performance:assessment_manager:batch_update"},
		[]string{"performance:result:view"},
		[]string{"performance:result:view", "performance:manager_eval:submit", "performance:manager_confirm:submit"},
	) {
		return
	}
	logs, err := svc.GetParticipantRelationshipChangeLogs(participantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取关系变更日志失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"logs": logs}})
}

func GetActivityRelationshipChangeLogs(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:activity:manage", "performance:assessment_manager:update", "performance:assessment_manager:batch_update") {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	logs, err := svc.GetActivityRelationshipChangeLogs(activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取关系变更日志失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"logs": logs}})
}

func StartPerformanceActivity(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.StartActivity(activityID, currentOperatorID(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func OpenSelfEvaluation(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	shouldNotify := shouldNotifyOnSelfEvaluationOpen(svc, activityID)
	if err := svc.OpenSelfEvaluation(activityID, currentOperatorID(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	queueSelfEvaluationNotification(activityID, shouldNotify)
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func OpenManagerEvaluation(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.OpenManagerEvaluation(activityID, currentOperatorID(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func ConfirmActivityResults(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.ConfirmResults(activityID, currentOperatorID(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func ArchivePerformanceActivity(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.ArchiveActivity(activityID, currentOperatorID(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func OpenTargetSettingHandler(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.OpenTargetSetting(activityID, currentOperatorID(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "目标设定已开启", Data: nil})
}

func OpenEmployeeConfirmationHandler(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.OpenEmployeeConfirmation(activityID, currentOperatorID(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	go func() {
		if err := notifyParticipantsResultReady(activityID); err != nil {
			logrus.Warnf("notify participants result ready failed: %v", err)
		}
	}()
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "员工确认已开启", Data: nil})
}

func OpenManagerConfirmationHandler(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.OpenManagerConfirmation(activityID, currentOperatorID(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "主管确认已开启", Data: nil})
}

func OpenHRConfirmationHandler(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.OpenHRConfirmation(activityID, currentOperatorID(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "HR确认已开启", Data: nil})
}

func LockPerformanceActivityHandler(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.LockActivity(activityID, currentOperatorID(c)); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	go func() {
		if err := notifyParticipantsResultLocked(activityID); err != nil {
			logrus.Warnf("notify participants result locked failed: %v", err)
		}
	}()
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "活动已锁定", Data: nil})
}

func ForceLockOverdueHRConfirmationHandler(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:activity:manage") {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	result, err := svc.ForceLockOverdueHRConfirmation(activityID, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	go func() {
		if err := notifyParticipantsResultLocked(activityID); err != nil {
			logrus.Warnf("notify participants result locked (force) failed: %v", err)
		}
	}()
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "逾期强制锁定已完成", Data: gin.H{"result": result}})
}

func BatchConfirmResults(c *gin.Context) {
	if !requirePermission(c, "performance:activity:manage") {
		return
	}
	activityID := c.Param("activity_id")
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	var req struct {
		ParticipantIDs []uint `json:"participant_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	results, err := svc.BatchConfirmResults(activityID, req.ParticipantIDs, currentOperatorName(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"results": results}})
}

func SendSelfEvalReminder(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:activity:manage") {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.SendSelfEvalReminders(activityID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func SendManagerEvalReminder(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:activity:manage") {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.SendManagerEvalReminders(activityID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func SendHRConfirmReminder(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:activity:manage") {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	if err := svc.SendHRConfirmReminders(activityID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func SetCompanyFinanceHandler(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:activity:manage") {
		return
	}
	var req struct {
		RevenueSign string `json:"revenue_sign"`
		Description string `json:"description"`
		Remark      string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	finance, err := svc.SetCompanyFinance(activityID, req.RevenueSign, req.Description, req.Remark, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"finance": finance}})
}

func GetCompanyFinanceHandler(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:activity:manage") {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	finance, err := svc.GetCompanyFinance(activityID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "未找到收支信息", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"finance": finance}})
}

func GetPendingHRConfirmHandler(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:activity:manage", "performance:hr_confirm:submit") {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	items, err := svc.GetPendingHRConfirm(activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取待 HR 确认列表失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items}})
}

func SetHRConfirmDeadlineHandler(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:activity:manage") {
		return
	}
	var req struct {
		Deadline string `json:"deadline" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	activity, err := svc.SetHRConfirmDeadline(activityID, req.Deadline, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"activity": activity}})
}

func GetHRConfirmDeadlineStatusHandler(c *gin.Context) {
	activityID := c.Param("activity_id")
	if !requirePerformanceActivityAccess(c, activityID, "performance:activity:manage", "performance:hr_confirm:submit") {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	status, err := svc.GetHRConfirmDeadlineStatus(activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取 HR 截止状态失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: status})
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
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:activity:manage"},
		nil,
		nil,
	) {
		return
	}
	if err := svc.TriggerPerformanceInterview(participantID, req.InterviewType); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
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
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		nil,
		[]string{"performance:self_eval:submit"},
		nil,
	) {
		return
	}

	// 1. 写入自评版本记录
	_, submitErr := svc.SubmitSelfEvaluation(participantID, struct {
		SelfScore       float64
		SelfLevel       string
		SelfSummary     string
		SelfAttachments []string
	}{
		SelfSummary: req.SelfContentJSON.Content,
	}, currentOperatorID(c))
	if submitErr != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: submitErr.Error(), Data: nil})
		return
	}

	// 2. 获取参与人信息，用于钉钉消息推送
	if participant != nil && participant.ManagerID != nil && *participant.ManagerID != "" {
		go func() {
			if notifyErr := dingtalk.SendCorpActionCardToUser(*participant.ManagerID,
				fmt.Sprintf("【绩效提醒】%s 已提交自评", participant.EmployeeName),
				fmt.Sprintf("员工：%s\n部门：%s\n岗位：%s\n\n请及时进行主管评分。", participant.EmployeeName, participant.DepartmentName, participant.Position),
				"去完成评分",
				service.PerformanceManagerEvalURL(participant.ActivityID, participant.ID)); notifyErr != nil {
				logPerformanceNotifyError("notify manager on self eval", *participant.ManagerID, notifyErr)
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
		ManagerComment   string  `json:"manager_comment"`
		FinalLevel       string  `json:"final_level" binding:"required"`
		FinalLevelReason string  `json:"final_level_reason"`
		BonusScore       float64 `json:"bonus_score"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(participantID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyManagerOfParticipant(c, participant) {
		return
	}

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

	// 如果活动启用了附加分，将附加分加入总分并重新计算等级
	finalLevel := req.FinalLevel
	if participant != nil {
		activity, _ := svc.GetActivity(participant.ActivityID)
		if activity != nil && activity.EnableBonusScore && req.BonusScore != 0 {
			adjustedScore := managerScore + req.BonusScore
			finalLevel = service.PerformanceLevelByActivity(adjustedScore, activity)
		}
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
		SuggestedLevel: finalLevel,
		ManagerComment: req.ManagerComment,
	}, currentOperatorID(c))
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

func SubmitGoalSelfEvaluationHandler(c *gin.Context) {
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}

	var req struct {
		Items                 []service.GoalSelfEvaluationItem `json:"items" binding:"required"`
		BonusItems            []service.GoalSelfEvaluationItem `json:"bonus_items"`
		EvaluationGood        string                           `json:"evaluation_good"`
		EvaluationImprovement string                           `json:"evaluation_improvement"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := currentOperatorID(c)

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		nil,
		[]string{"performance:self_eval:submit"},
		nil,
	) {
		return
	}
	if err := svc.SubmitGoalSelfEvaluation(uint(participantID), req.Items, req.BonusItems, req.EvaluationGood, req.EvaluationImprovement, userID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func AutoScoreGoalRecordsHandler(c *gin.Context) {
	var req struct {
		Items []service.AutoItemInput `json:"items" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}
	if len(req.Items) > 100 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "单次自动评分不能超过 100 项", Data: nil})
		return
	}

	result := service.CalculateAutoScores(req.Items)
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: result})
}

func SubmitGoalManagerEvaluationHandler(c *gin.Context) {
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}

	var req struct {
		Items                 []service.GoalManagerEvaluationItem `json:"items" binding:"required"`
		BonusItems            []service.GoalManagerEvaluationItem `json:"bonus_items"`
		SuggestedLevel        string                              `json:"suggested_level"`
		EvaluationGood        string                              `json:"evaluation_good"`
		EvaluationImprovement string                              `json:"evaluation_improvement"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := currentOperatorID(c)

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyManagerOfParticipant(c, participant) {
		return
	}
	if err := svc.SubmitGoalManagerEvaluation(uint(participantID), req.Items, req.BonusItems, req.SuggestedLevel, req.EvaluationGood, req.EvaluationImprovement, userID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

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
	if err := dingtalk.SendCorpActionCardToUser(participant.EmployeeID,
		"【绩效提醒】您的评分结果已出具",
		fmt.Sprintf("员工：%s\n最终等级：%s\n主管评语：%s\n\n如对结果有疑问，请联系主管。", participant.EmployeeName, finalLevel, comment),
		"查看绩效结果",
		service.PerformanceResultURL(participant.ActivityID, participant.ID)); err != nil {
		if dingtalk.IsUserNotNotifiableError(err) {
			logPerformanceNotifyError("notify employee on manager eval", participant.EmployeeID, err)
			return nil
		}
		return err
	}
	return nil
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
	skipped := 0
	failures := make([]string, 0)

	for _, participant := range participants {
		if !shouldNotifyParticipant(participant) {
			continue
		}

		content := fmt.Sprintf(
			"员工：%s\n活动：%s\n自评时间：%s\n%s\n\n请及时完成自评。提交完成后将不再收到后续自评提醒。",
			participant.EmployeeName,
			activity.Name,
			window,
			service.SelfEvalDeadlineReminderText(activity.SelfEvalEndAt),
		)
		if err := dingtalk.SendCorpActionCardToUser(participant.EmployeeID, title, content, "去完成自评", service.PerformanceSelfEvalURL(activityID, participant.ID)); err != nil {
			if dingtalk.IsUserNotNotifiableError(err) {
				logPerformanceNotifyError("notify participant on self evaluation open", participant.EmployeeID, err)
				skipped++
				continue
			}
			failures = append(failures, fmt.Sprintf("%s(%s): %v", participant.EmployeeName, participant.EmployeeID, err))
			continue
		}
		sent++
	}

	if len(failures) > 0 {
		failed := len(failures)
		if len(failures) > 3 {
			failures = failures[:3]
		}
		return fmt.Errorf("sent=%d skipped=%d failed=%d sample=%s", sent, skipped, failed, strings.Join(failures, "; "))
	}

	logrus.Infof("self evaluation notifications sent for activity %s: sent=%d skipped=%d", activityID, sent, skipped)
	return nil
}

// notifyParticipantsResultReady 通知所有参与者绩效结果已出，需登录确认
func notifyParticipantsResultReady(activityID string) error {
	svc := service.NewPerformanceService(database.DB)
	activity, err := svc.GetActivity(activityID)
	if err != nil || activity == nil {
		return fmt.Errorf("活动不存在: %v", err)
	}
	var participants []database.PerformanceParticipant
	if err := database.DB.
		Where("activity_id = ? AND deleted_at IS NULL AND status = ?", activityID, "manager_submitted").
		Find(&participants).Error; err != nil {
		return err
	}
	if len(participants) == 0 {
		return nil
	}
	var sent int
	var skipped int
	var failures []string
	for _, p := range participants {
		if !shouldNotifyParticipant(p) {
			continue
		}
		if err := dingtalk.SendCorpActionCardToUser(p.EmployeeID,
			"绩效结果确认通知",
			fmt.Sprintf("活动：%s\n您的绩效结果已出，请进入系统确认。", activity.Name),
			"去确认结果",
			service.PerformanceResultURL(activityID, p.ID)); err != nil {
			if dingtalk.IsUserNotNotifiableError(err) {
				logPerformanceNotifyError("notify participant result ready", p.EmployeeID, err)
				skipped++
				continue
			}
			failures = append(failures, fmt.Sprintf("%s(%s): %v", p.EmployeeName, p.EmployeeID, err))
			continue
		}
		sent++
	}
	if len(failures) > 0 {
		failed := len(failures)
		if len(failures) > 3 {
			failures = failures[:3]
		}
		return fmt.Errorf("sent=%d skipped=%d failed=%d sample=%s", sent, skipped, failed, strings.Join(failures, "; "))
	}
	logrus.Infof("result ready notifications sent for activity %s: sent=%d skipped=%d", activityID, sent, skipped)
	return nil
}

// notifyParticipantsResultLocked 通知所有参与者绩效结果已锁定
func notifyParticipantsResultLocked(activityID string) error {
	svc := service.NewPerformanceService(database.DB)
	activity, err := svc.GetActivity(activityID)
	if err != nil || activity == nil {
		return fmt.Errorf("活动不存在: %v", err)
	}
	var participants []database.PerformanceParticipant
	if err := database.DB.
		Where("activity_id = ? AND deleted_at IS NULL AND is_locked = ?", activityID, true).
		Find(&participants).Error; err != nil {
		return err
	}
	if len(participants) == 0 {
		return nil
	}
	var sent int
	var skipped int
	var failures []string
	for _, p := range participants {
		if !shouldNotifyParticipant(p) {
			continue
		}
		if err := dingtalk.SendCorpActionCardToUser(p.EmployeeID,
			"绩效结果锁定通知",
			fmt.Sprintf("活动：%s\n您的绩效结果已锁定，最终等级为 %s。", activity.Name, p.FinalLevel),
			"查看绩效结果",
			service.PerformanceResultURL(activityID, p.ID)); err != nil {
			if dingtalk.IsUserNotNotifiableError(err) {
				logPerformanceNotifyError("notify participant result locked", p.EmployeeID, err)
				skipped++
				continue
			}
			failures = append(failures, fmt.Sprintf("%s(%s): %v", p.EmployeeName, p.EmployeeID, err))
			continue
		}
		sent++
	}
	if len(failures) > 0 {
		failed := len(failures)
		if len(failures) > 3 {
			failures = failures[:3]
		}
		return fmt.Errorf("sent=%d skipped=%d failed=%d sample=%s", sent, skipped, failed, strings.Join(failures, "; "))
	}
	logrus.Infof("result locked notifications sent for activity %s: sent=%d skipped=%d", activityID, sent, skipped)
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
	if !requirePerformanceActivityAccess(c, activityID, "performance:activity:manage", "performance:hr_confirm:submit") {
		return
	}
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
	if !requirePerformanceActivityAccess(c, activityID, "performance:distribution:manage", "performance:activity:manage") {
		return
	}
	svc := service.NewPerformanceService(database.DB)
	result, err := svc.GetDistributionCheck(activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取分布检查失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: result})
}

type performanceTemplatePayload struct {
	Name               string                 `json:"name" binding:"required"`
	Code               string                 `json:"code"`
	Description        string                 `json:"description"`
	FlowType           string                 `json:"flow_type"`
	OrganizationID     string                 `json:"organization_id"`
	OrganizationScope  []string               `json:"organization_scope"`
	Status             string                 `json:"status"`
	CycleTypes         []string               `json:"cycle_types"`
	WorkflowConfig     map[string]interface{} `json:"workflow_config"`
	FormConfig         map[string]interface{} `json:"form_config"`
	LevelRuleConfig    map[string]interface{} `json:"level_rule_config"`
	DistributionConfig map[string]interface{} `json:"distribution_config"`
	PermissionConfig   map[string]interface{} `json:"permission_config"`
	PublishConfig      map[string]interface{} `json:"publish_config"`
	Sections           []struct {
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

func toPerformanceTemplateRequest(req performanceTemplatePayload) service.PerformanceTemplateRequest {
	sections := make([]service.PerformanceTemplateSectionRequest, 0, len(req.Sections))
	for _, section := range req.Sections {
		items := make([]service.PerformanceTemplateItemRequest, 0, len(section.Items))
		for _, item := range section.Items {
			items = append(items, service.PerformanceTemplateItemRequest{
				Name:        item.Name,
				Description: item.Description,
				MaxScore:    item.MaxScore,
				Weight:      item.Weight,
				SortOrder:   item.SortOrder,
			})
		}
		sections = append(sections, service.PerformanceTemplateSectionRequest{
			Name:              section.Name,
			SectionType:       section.SectionType,
			Weight:            section.Weight,
			SortOrder:         section.SortOrder,
			IsScoreRequired:   section.IsScoreRequired,
			IsCommentRequired: section.IsCommentRequired,
			Items:             items,
		})
	}

	return service.PerformanceTemplateRequest{
		Name:               req.Name,
		Code:               req.Code,
		Description:        req.Description,
		FlowType:           req.FlowType,
		OrganizationID:     req.OrganizationID,
		OrganizationScope:  req.OrganizationScope,
		Status:             req.Status,
		CycleTypes:         req.CycleTypes,
		WorkflowConfig:     req.WorkflowConfig,
		FormConfig:         req.FormConfig,
		LevelRuleConfig:    req.LevelRuleConfig,
		DistributionConfig: req.DistributionConfig,
		PermissionConfig:   req.PermissionConfig,
		PublishConfig:      req.PublishConfig,
		Sections:           sections,
	}
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
	var req performanceTemplatePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := currentOperatorID(c)

	svc := service.NewPerformanceService(database.DB)
	template, err := svc.CreateTemplate(toPerformanceTemplateRequest(req), userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"template": template}})
}

func GetPerformanceTemplate(c *gin.Context) {
	templateID, err := strconv.ParseUint(c.Param("id"), 10, 32)
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
	templateID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的模板ID", Data: nil})
		return
	}

	var req performanceTemplatePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := currentOperatorID(c)

	svc := service.NewPerformanceService(database.DB)
	template, err := svc.UpdateTemplate(uint(templateID), toPerformanceTemplateRequest(req), userID)
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

// ===================== 指标库管理 =====================

func parseOptionalUintQuery(c *gin.Context, key string) (*uint, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, true
	}
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: fmt.Sprintf("无效的%s", key), Data: nil})
		return nil, false
	}
	parsed := uint(value)
	return &parsed, true
}

func verifyPerformanceTemplateAvailable(c *gin.Context, templateID uint) bool {
	if templateID == 0 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "请选择绩效流程模板", Data: nil})
		return false
	}
	var template database.PerformanceTemplate
	if err := database.DB.Where("id = ? AND deleted_at IS NULL AND status = ?", templateID, "active").First(&template).Error; err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "绩效流程模板不存在或未启用", Data: nil})
		return false
	}
	return true
}

func GetIndicatorLibraries(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	departmentID := c.Query("department_id")
	keyword := c.Query("keyword")
	status := c.Query("status")
	templateID, ok := parseOptionalUintQuery(c, "template_id")
	if !ok {
		return
	}

	// 获取用户的数据范围（部门隔离）
	scope, err := resolvePerformanceScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取数据范围失败", Data: gin.H{"error": err.Error()}})
		return
	}

	libRepo := repository.NewPerformanceIndicatorLibraryRepository(database.DB)
	svc := service.NewPerformanceIndicatorService(libRepo, nil)
	items, total, err := svc.ListLibraries(page, pageSize, departmentID, keyword, status, scope, templateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取指标库列表失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items, "total": total}})
}

func CreateIndicatorLibrary(c *gin.Context) {
	if !requirePermission(c, "performance:indicator:manage") {
		return
	}
	var req struct {
		DepartmentID   string `json:"department_id" binding:"required"`
		DepartmentName string `json:"department_name" binding:"required"`
		TemplateID     *uint  `json:"template_id" binding:"required"`
		Name           string `json:"name" binding:"required"`
		Description    string `json:"description"`
		DefaultCycle   string `json:"default_cycle"`
		Items          []struct {
			SectionType    string  `json:"section_type" binding:"required"`
			Name           string  `json:"name" binding:"required"`
			Description    string  `json:"description"`
			Weight         float64 `json:"weight"`
			RedLineValue   string  `json:"red_line_value"`
			TargetValue    string  `json:"target_value"`
			ChallengeValue string  `json:"challenge_value"`
			ScoringRule    string  `json:"scoring_rule"`
			IsDefault      bool    `json:"is_default"`
			SortOrder      int     `json:"sort_order"`
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := currentOperatorID(c)
	createLib := func() *database.PerformanceIndicatorLibrary {
		return &database.PerformanceIndicatorLibrary{
			DepartmentID:   req.DepartmentID,
			DepartmentName: req.DepartmentName,
			TemplateID:     req.TemplateID,
			Name:           req.Name,
			Description:    req.Description,
			DefaultCycle:   req.DefaultCycle,
			Status:         "active",
			CreatedBy:      userID,
			UpdatedBy:      userID,
		}
	}

	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "指标库至少需要包含一个指标项", Data: nil})
		return
	}
	if !verifyIndicatorDepartmentAccess(c, req.DepartmentID, "无权在该部门维护指标库") {
		return
	}
	if req.TemplateID == nil || !verifyPerformanceTemplateAvailable(c, *req.TemplateID) {
		return
	}

	lib := createLib()
	items := make([]database.PerformanceIndicatorItem, 0, len(req.Items))
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(lib).Error; err != nil {
			return err
		}
		for idx, raw := range req.Items {
			item := database.PerformanceIndicatorItem{
				LibraryID:         lib.ID,
				SectionType:       raw.SectionType,
				Name:              raw.Name,
				Description:       raw.Description,
				Weight:            raw.Weight,
				DefaultWeight:     raw.Weight,
				RedLineValue:      raw.RedLineValue,
				TargetValue:       raw.TargetValue,
				ChallengeValue:    raw.ChallengeValue,
				ScoringRule:       raw.ScoringRule,
				IsDefault:         raw.IsDefault,
				SortOrder:         raw.SortOrder,
				CalculationMethod: raw.Description,
				CreatedBy:         userID,
				UpdatedBy:         userID,
			}
			if item.SortOrder == 0 {
				item.SortOrder = idx + 1
			}
			items = append(items, item)
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"library": lib, "items": items}})
}

func GetIndicatorLibrary(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的 ID", Data: nil})
		return
	}

	libRepo := repository.NewPerformanceIndicatorLibraryRepository(database.DB)
	svc := service.NewPerformanceIndicatorService(libRepo, nil)
	lib, ok := loadIndicatorLibraryForScope(c, svc, uint(id), "无权访问该指标库")
	if !ok {
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"library": lib}})
}

func UpdateIndicatorLibrary(c *gin.Context) {
	if !requirePermission(c, "performance:indicator:manage") {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的 ID", Data: nil})
		return
	}

	var req struct {
		Name           string `json:"name"`
		Description    string `json:"description"`
		DepartmentName string `json:"department_name"`
		TemplateID     *uint  `json:"template_id"`
		DefaultCycle   string `json:"default_cycle"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	libRepo := repository.NewPerformanceIndicatorLibraryRepository(database.DB)
	svc := service.NewPerformanceIndicatorService(libRepo, nil)
	lib, ok := loadIndicatorLibraryForScope(c, svc, uint(id), "无权修改该指标库")
	if !ok {
		return
	}

	lib.Name = req.Name
	lib.Description = req.Description
	lib.DepartmentName = req.DepartmentName
	lib.DefaultCycle = req.DefaultCycle
	if req.TemplateID != nil {
		if !verifyPerformanceTemplateAvailable(c, *req.TemplateID) {
			return
		}
		lib.TemplateID = req.TemplateID
	}
	lib.UpdatedBy = currentOperatorID(c)

	if err := svc.UpdateLibrary(lib); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"library": lib}})
}

func ArchiveIndicatorLibrary(c *gin.Context) {
	if !requirePermission(c, "performance:indicator:manage") {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的 ID", Data: nil})
		return
	}

	libRepo := repository.NewPerformanceIndicatorLibraryRepository(database.DB)
	svc := service.NewPerformanceIndicatorService(libRepo, nil)
	if _, ok := loadIndicatorLibraryForScope(c, svc, uint(id), "无权归档该指标库"); !ok {
		return
	}
	if err := svc.ArchiveLibrary(uint(id), currentOperatorID(c)); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "归档失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func GetIndicatorLibrariesByDepartment(c *gin.Context) {
	departmentID := c.Param("department_id")
	if departmentID == "" {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "部门 ID 不能为空", Data: nil})
		return
	}
	if !verifyIndicatorDepartmentAccess(c, departmentID, "无权访问该部门指标库") {
		return
	}
	templateID, ok := parseOptionalUintQuery(c, "template_id")
	if !ok {
		return
	}

	libRepo := repository.NewPerformanceIndicatorLibraryRepository(database.DB)
	svc := service.NewPerformanceIndicatorService(libRepo, nil)
	items, err := svc.GetLibrariesByDepartment(departmentID, templateID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取部门指标库失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items}})
}

func InheritIndicatorLibrary(c *gin.Context) {
	if !requirePermission(c, "performance:indicator:manage") {
		return
	}
	var req struct {
		ParentLibraryID      uint   `json:"parent_library_id" binding:"required"`
		TargetDepartmentID   string `json:"target_department_id" binding:"required"`
		TargetDepartmentName string `json:"target_department_name" binding:"required"`
		Name                 string `json:"name"`
		Description          string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	libRepo := repository.NewPerformanceIndicatorLibraryRepository(database.DB)
	itemRepo := repository.NewPerformanceIndicatorItemRepository(database.DB)
	svc := service.NewPerformanceIndicatorService(libRepo, itemRepo)
	if _, ok := loadIndicatorLibraryForScope(c, svc, req.ParentLibraryID, "无权访问父指标库"); !ok {
		return
	}
	if !verifyIndicatorDepartmentAccess(c, req.TargetDepartmentID, "无权在目标部门维护指标库") {
		return
	}
	lib, err := svc.InheritLibrary(req.ParentLibraryID, req.TargetDepartmentID, req.TargetDepartmentName, req.Name, req.Description, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"library": lib}})
}

// ===================== 指标项管理 =====================

func GetIndicatorItems(c *gin.Context) {
	libraryID, err := strconv.ParseUint(c.Query("library_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的指标库 ID", Data: nil})
		return
	}
	sectionType := c.Query("section_type")

	libRepo := repository.NewPerformanceIndicatorLibraryRepository(database.DB)
	itemRepo := repository.NewPerformanceIndicatorItemRepository(database.DB)
	svc := service.NewPerformanceIndicatorService(libRepo, itemRepo)
	if _, ok := loadIndicatorLibraryForScope(c, svc, uint(libraryID), "无权访问该指标库"); !ok {
		return
	}
	items, err := svc.ListItemsByLibrary(uint(libraryID), sectionType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取指标项列表失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items}})
}

func CreateIndicatorItem(c *gin.Context) {
	if !requirePermission(c, "performance:indicator:manage") {
		return
	}
	var req struct {
		LibraryID         uint     `json:"library_id" binding:"required"`
		SectionType       string   `json:"section_type" binding:"required"`
		Name              string   `json:"name" binding:"required"`
		Description       string   `json:"description"`
		IndicatorType     string   `json:"indicator_type"`
		Keywords          []string `json:"keywords"`
		CalculationMethod string   `json:"calculation_method"`
		DataSource        string   `json:"data_source"`
		Cycle             string   `json:"cycle"`
		DefaultWeight     float64  `json:"default_weight"`
		Weight            float64  `json:"weight"`
		RedLineValue      string   `json:"red_line_value"`
		TargetValue       string   `json:"target_value"`
		ChallengeValue    string   `json:"challenge_value"`
		ScoringRule       string   `json:"scoring_rule"`
		IsDefault         bool     `json:"is_default"`
		SortOrder         int      `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	defaultWeight := req.DefaultWeight
	if defaultWeight == 0 {
		defaultWeight = req.Weight
	}
	item := &database.PerformanceIndicatorItem{
		LibraryID:         req.LibraryID,
		SectionType:       req.SectionType,
		Name:              req.Name,
		Description:       req.Description,
		IndicatorType:     req.IndicatorType,
		Keywords:          req.Keywords,
		CalculationMethod: req.CalculationMethod,
		DataSource:        req.DataSource,
		Cycle:             req.Cycle,
		DefaultWeight:     defaultWeight,
		Weight:            req.Weight,
		RedLineValue:      req.RedLineValue,
		TargetValue:       req.TargetValue,
		ChallengeValue:    req.ChallengeValue,
		ScoringRule:       req.ScoringRule,
		IsDefault:         req.IsDefault,
		SortOrder:         req.SortOrder,
		CreatedBy:         currentOperatorID(c),
		UpdatedBy:         currentOperatorID(c),
	}

	libRepo := repository.NewPerformanceIndicatorLibraryRepository(database.DB)
	itemRepo := repository.NewPerformanceIndicatorItemRepository(database.DB)
	svc := service.NewPerformanceIndicatorService(libRepo, itemRepo)
	if _, ok := loadIndicatorLibraryForScope(c, svc, req.LibraryID, "无权在该指标库添加指标项"); !ok {
		return
	}
	if err := svc.CreateItem(item); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"item": item}})
}

func UpdateIndicatorItem(c *gin.Context) {
	if !requirePermission(c, "performance:indicator:manage") {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的 ID", Data: nil})
		return
	}

	var req struct {
		Name              string   `json:"name"`
		Description       string   `json:"description"`
		IndicatorType     string   `json:"indicator_type"`
		Keywords          []string `json:"keywords"`
		CalculationMethod string   `json:"calculation_method"`
		DataSource        string   `json:"data_source"`
		Cycle             string   `json:"cycle"`
		DefaultWeight     float64  `json:"default_weight"`
		Weight            float64  `json:"weight"`
		RedLineValue      string   `json:"red_line_value"`
		TargetValue       string   `json:"target_value"`
		ChallengeValue    string   `json:"challenge_value"`
		ScoringRule       string   `json:"scoring_rule"`
		IsDefault         bool     `json:"is_default"`
		SortOrder         int      `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	libRepo := repository.NewPerformanceIndicatorLibraryRepository(database.DB)
	itemRepo := repository.NewPerformanceIndicatorItemRepository(database.DB)
	svc := service.NewPerformanceIndicatorService(libRepo, itemRepo)
	item, ok := loadIndicatorItemForScope(c, svc, uint(id), "无权修改该指标项")
	if !ok {
		return
	}

	item.Name = req.Name
	item.Description = req.Description
	item.IndicatorType = req.IndicatorType
	item.Keywords = req.Keywords
	item.CalculationMethod = req.CalculationMethod
	item.DataSource = req.DataSource
	item.Cycle = req.Cycle
	item.DefaultWeight = req.DefaultWeight
	item.Weight = req.Weight
	item.RedLineValue = req.RedLineValue
	item.TargetValue = req.TargetValue
	item.ChallengeValue = req.ChallengeValue
	item.ScoringRule = req.ScoringRule
	item.IsDefault = req.IsDefault
	item.SortOrder = req.SortOrder
	item.UpdatedBy = currentOperatorID(c)

	if err := svc.UpdateItem(item); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"item": item}})
}

func DeleteIndicatorItem(c *gin.Context) {
	if !requirePermission(c, "performance:indicator:manage") {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的 ID", Data: nil})
		return
	}

	libRepo := repository.NewPerformanceIndicatorLibraryRepository(database.DB)
	itemRepo := repository.NewPerformanceIndicatorItemRepository(database.DB)
	svc := service.NewPerformanceIndicatorService(libRepo, itemRepo)
	if _, ok := loadIndicatorItemForScope(c, svc, uint(id), "无权删除该指标项"); !ok {
		return
	}
	if err := svc.DeleteItem(uint(id), currentOperatorID(c)); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "删除失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func SearchIndicatorItems(c *gin.Context) {
	keyword := c.Query("keyword")
	libraryIDsStr := c.Query("library_ids")
	sectionType := c.Query("section_type")

	var libraryIDs []uint
	if libraryIDsStr != "" {
		for _, idStr := range strings.Split(libraryIDsStr, ",") {
			if id, err := strconv.ParseUint(idStr, 10, 32); err == nil {
				libraryIDs = append(libraryIDs, uint(id))
			}
		}
	}

	scope, err := resolvePerformanceScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取数据范围失败", Data: gin.H{"error": err.Error()}})
		return
	}

	itemRepo := repository.NewPerformanceIndicatorItemRepository(database.DB)
	svc := service.NewPerformanceIndicatorService(nil, itemRepo)
	items, err := svc.SearchItems(libraryIDs, keyword, sectionType, scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "搜索失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": items}})
}

// ===================== 目标记录管理 =====================

func GetGoalRecords(c *gin.Context) {
	if !requirePermission(c, "performance:result:view", "performance:goal:manage", "performance:self_eval:submit", "performance:manager_eval:submit") {
		return
	}
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:result:view", "performance:activity:manage", "performance:goal:manage", "performance:hr_confirm:submit"},
		[]string{"performance:self_eval:submit", "performance:employee_confirm:submit"},
		[]string{"performance:manager_eval:submit", "performance:manager_confirm:submit"},
	) {
		return
	}

	records, err := svc.GetGoalRecords(uint(participantID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取目标记录失败", Data: gin.H{"error": err.Error()}})
		return
	}

	var latestRejection *database.PerformanceGoalApprovalLog
	if participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10)); err == nil && participant != nil {
		if log, logErr := svc.GetLatestGoalRejection(uint(participantID), participant.ActivityID); logErr != nil {
			c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取目标驳回理由失败", Data: gin.H{"error": logErr.Error()}})
			return
		} else {
			latestRejection = log
		}
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": records, "latest_rejection": latestRejection}})
}

func BatchSaveGoalRecords(c *gin.Context) {
	if !requirePermission(c, "performance:goal:manage") {
		return
	}
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}

	var req struct {
		Records []service.GoalRecordRequest `json:"records"`
		Items   []service.GoalRecordRequest `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	payload := req.Records
	if len(payload) == 0 {
		payload = req.Items
	}
	if len(payload) == 0 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "目标记录不能为空", Data: nil})
		return
	}

	userID := currentOperatorID(c)

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:activity:manage", "performance:goal:manage", "performance:hr_confirm:submit"},
		[]string{"performance:goal:manage"},
		[]string{"performance:goal:manage"},
	) {
		return
	}
	records, err := svc.BatchSaveGoalRecords(uint(participantID), payload, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": records}})
}

func BatchSaveReviewGoalRecords(c *gin.Context) {
	if !requirePermission(c, "performance:goal:manage") {
		return
	}
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}

	var req struct {
		Records []service.GoalRecordRequest `json:"records"`
		Items   []service.GoalRecordRequest `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	payload := req.Records
	if len(payload) == 0 {
		payload = req.Items
	}
	if len(payload) == 0 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "补录指标不能为空", Data: nil})
		return
	}

	userID := currentOperatorID(c)

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:activity:manage", "performance:goal:manage", "performance:hr_confirm:submit"},
		[]string{"performance:goal:manage"},
		[]string{"performance:goal:manage"},
	) {
		return
	}
	records, err := svc.BatchSaveReviewGoalRecords(uint(participantID), payload, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": records}})
}

func SubmitGoalApprovalHandler(c *gin.Context) {
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}

	var req struct {
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := currentOperatorID(c)
	action := req.Action
	if strings.TrimSpace(action) == "" {
		action = "submit"
	}

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	selfCodes := []string{"performance:goal:manage"}
	managerCodes := []string{"performance:goal:manage"}
	if action == "approve" || action == "reject" {
		selfCodes = nil
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:activity:manage", "performance:goal:manage", "performance:hr_confirm:submit"},
		selfCodes,
		managerCodes,
	) {
		return
	}
	if err := svc.SubmitGoalApproval(uint(participantID), action, req.Comment, userID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func ApproveGoalRecords(c *gin.Context) {
	if !requirePermission(c, "performance:goal:manage") {
		return
	}
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}

	var req struct {
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := currentOperatorID(c)

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:activity:manage", "performance:goal:manage", "performance:hr_confirm:submit"},
		nil,
		[]string{"performance:goal:manage"},
	) {
		return
	}
	if err := svc.SubmitGoalApproval(uint(participantID), "approve", req.Comment, userID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func RejectGoalRecords(c *gin.Context) {
	if !requirePermission(c, "performance:goal:manage") {
		return
	}
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}

	var req struct {
		Comment string `json:"comment" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := currentOperatorID(c)

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:activity:manage", "performance:goal:manage", "performance:hr_confirm:submit"},
		nil,
		[]string{"performance:goal:manage"},
	) {
		return
	}
	if err := svc.SubmitGoalApproval(uint(participantID), "reject", req.Comment, userID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func GetManagerGoals(c *gin.Context) {
	if !requirePermission(c, "performance:goal:manage") {
		return
	}
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:activity:manage", "performance:goal:manage", "performance:hr_confirm:submit"},
		[]string{"performance:goal:manage"},
		[]string{"performance:goal:manage"},
	) {
		return
	}
	records, err := svc.GetManagerGoals(uint(participantID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取上级目标失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": records}})
}

func GetGoalSuggestions(c *gin.Context) {
	if !requirePermission(c, "performance:goal:manage") {
		return
	}
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyPerformanceParticipantAccess(
		c,
		participant,
		[]string{"performance:activity:manage", "performance:goal:manage", "performance:hr_confirm:submit"},
		[]string{"performance:goal:manage"},
		[]string{"performance:goal:manage"},
	) {
		return
	}
	suggestions, err := svc.GetGoalSuggestions(uint(participantID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取目标建议失败", Data: gin.H{"error": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": suggestions, "suggestions": suggestions}})
}

func BatchAssignGoals(c *gin.Context) {
	if !requirePermission(c, "performance:goal:manage") {
		return
	}
	activityID := c.Param("activity_id")

	var req struct {
		ManagerID      string                      `json:"manager_id"`
		Targets        []service.GoalRecordRequest `json:"targets"`
		Items          []service.GoalRecordRequest `json:"items"`
		ParticipantIDs []uint                      `json:"participant_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := currentOperatorID(c)
	targets := req.Targets
	if len(targets) == 0 {
		targets = req.Items
	}
	if len(targets) == 0 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "目标记录不能为空", Data: nil})
		return
	}
	managerID := strings.TrimSpace(req.ManagerID)
	if managerID == "" {
		managerID = userID
	}

	svc := service.NewPerformanceService(database.DB)
	if !verifyPerformanceActivityIDAccess(c, activityID) {
		return
	}
	for _, participantID := range req.ParticipantIDs {
		participant, err := svc.GetParticipant(strconv.FormatUint(uint64(participantID), 10))
		if err != nil {
			c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: gin.H{"participant_id": participantID}})
			return
		}
		if participant.ActivityID != activityID {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参与人不属于当前活动", Data: gin.H{"participant_id": participantID}})
			return
		}
		if !verifyPerformanceParticipantAccess(
			c,
			participant,
			[]string{"performance:activity:manage", "performance:goal:manage", "performance:hr_confirm:submit"},
			nil,
			[]string{"performance:goal:manage"},
		) {
			return
		}
	}
	if err := svc.BatchAssignGoals(activityID, managerID, targets, req.ParticipantIDs, userID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}

func SetBonusPenaltyScoreHandler(c *gin.Context) {
	participantID, err := strconv.ParseUint(c.Param("participant_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的参与人 ID", Data: nil})
		return
	}

	var req struct {
		BonusScore   float64 `json:"bonus_score"`
		PenaltyScore float64 `json:"penalty_score"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误", Data: gin.H{"error": err.Error()}})
		return
	}

	userID := currentOperatorID(c)

	svc := service.NewPerformanceService(database.DB)
	participant, err := svc.GetParticipant(strconv.FormatUint(participantID, 10))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "参与人不存在", Data: nil})
		return
	}
	if !verifyManagerOfParticipant(c, participant) {
		return
	}
	if err := svc.SetBonusPenaltyScore(uint(participantID), req.BonusScore, req.PenaltyScore, userID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: nil})
}
