package service

import (
	"fmt"
	"os"
	"peopleops/internal/config"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ShiftConfigService struct {
	repo *repository.ShiftConfigRepository
	db   *gorm.DB
}

var shiftIDCache sync.Map

func NewShiftConfigService(db *gorm.DB) *ShiftConfigService {
	return &ShiftConfigService{
		repo: repository.NewShiftConfigRepository(db),
		db:   db,
	}
}

// EmployeeShiftItem 员工下班时间配置（含用户基础信息）
type EmployeeShiftItem struct {
	UserID         string `json:"user_id"`
	UserName       string `json:"user_name"`
	DepartmentID   string `json:"department_id"`
	DepartmentName string `json:"department_name"`
	EndTime        string `json:"end_time"` // "17:30" 或 "18:30"（默认）
	ShiftID        int64  `json:"shift_id"` // 0 表示使用默认
	Note           string `json:"note"`
	HasCustom      bool   `json:"has_custom"` // 是否有自定义配置
	ConfigID       uint   `json:"config_id"`  // EmployeeShiftConfig.ID，0 表示无
}

// GetAllWithUsers 获取全部员工的下班时间配置（含默认下班时间的员工）
func (s *ShiftConfigService) GetAllWithUsers() ([]EmployeeShiftItem, error) {
	var users []database.User
	if err := s.db.
		Order("name").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	configs, err := s.repo.FindAll()
	if err != nil {
		return nil, fmt.Errorf("查询配置失败: %w", err)
	}
	configMap := make(map[string]*database.EmployeeShiftConfig, len(configs))
	for i := range configs {
		configMap[configs[i].UserID] = &configs[i]
	}

	deptMap := make(map[string]string)
	var depts []database.Department
	if err := s.db.Find(&depts).Error; err == nil {
		for _, d := range depts {
			deptMap[d.DepartmentID] = d.Name
		}
	}

	items := make([]EmployeeShiftItem, 0, len(users))
	for _, u := range users {
		item := EmployeeShiftItem{
			UserID:         u.UserID,
			UserName:       u.Name,
			DepartmentID:   u.DepartmentID,
			DepartmentName: deptMap[u.DepartmentID],
			EndTime:        config.GetDefaultCheckOut(),
		}
		if cfg, ok := configMap[u.UserID]; ok {
			item.EndTime = cfg.EndTime
			item.ShiftID = cfg.ShiftID
			item.Note = cfg.Note
			item.HasCustom = true
			item.ConfigID = cfg.ID
		}
		items = append(items, item)
	}
	return items, nil
}

// SetShiftConfigInput 批量/单个设置下班时间的请求体
type SetShiftConfigInput struct {
	UserIDs []string `json:"user_ids" binding:"required,min=1"`
	ShiftID int64    `json:"shift_id" binding:"required"`
	EndTime string   `json:"end_time" binding:"required"` // "17:30"
	Note    string   `json:"note"`
}

type ApplyShiftConfigInput struct {
	UserIDs   []string `json:"user_ids" binding:"required,min=1"`
	ShiftID   int64    `json:"shift_id"`
	EndTime   string   `json:"end_time"`
	Note      string   `json:"note"`
	Name      string   `json:"name"`
	CheckIn   string   `json:"check_in"`
	CheckOut  string   `json:"check_out"`
	StartDate string   `json:"start_date" binding:"required"`
	EndDate   string   `json:"end_date" binding:"required"`
}

type PreviewShiftConfigInput struct {
	UserIDs   []string `json:"user_ids" binding:"required,min=1"`
	ShiftID   int64    `json:"shift_id"`
	EndTime   string   `json:"end_time"`
	Name      string   `json:"name"`
	CheckIn   string   `json:"check_in"`
	CheckOut  string   `json:"check_out"`
	StartDate string   `json:"start_date" binding:"required"`
	EndDate   string   `json:"end_date" binding:"required"`
}

type ApplyShiftConfigResult struct {
	ShiftID      int64                   `json:"shift_id"`
	UpdatedCount int                     `json:"updated_count"`
	SyncedCount  int                     `json:"synced_count"`
	FailedCount  int                     `json:"failed_count"`
	FailedItems  []dingtalk.ScheduleItem `json:"failed_items,omitempty"`
	GroupID      int64                   `json:"group_id,omitempty"`
	GroupName    string                  `json:"group_name,omitempty"`
	ErrorDetail  string                  `json:"error_detail,omitempty"`
	Status       string                  `json:"status"`
	Message      string                  `json:"message"`
}

type ShiftPreviewItem struct {
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	WorkDate    string `json:"work_date"`
	ShiftID     int64  `json:"shift_id"`
	ShiftName   string `json:"shift_name"`
	IsRest      bool   `json:"is_rest"`
	Reason      string `json:"reason"`
	WeekType    string `json:"week_type,omitempty"`
	HolidayName string `json:"holiday_name,omitempty"`
	HolidayType string `json:"holiday_type,omitempty"`
	WillSync    bool   `json:"will_sync"`
}

type PreviewShiftConfigResult struct {
	Items []ShiftPreviewItem `json:"items"`
}

type ShiftCatalogItem struct {
	ID              uint   `json:"id"`
	Name            string `json:"name"`
	ShiftID         int64  `json:"shift_id"`
	CheckIn         string `json:"check_in"`
	CheckOut        string `json:"check_out"`
	AttachedToGroup bool   `json:"attached_to_group"`
	GroupID         int64  `json:"group_id,omitempty"`
	GroupName       string `json:"group_name,omitempty"`
}

type partialShiftDecision struct {
	UserID      string
	UserName    string
	WorkDate    string
	IsRest      bool
	Reason      string
	WeekType    string
	HolidayName string
	HolidayType string
	WillSync    bool
}

// SetConfigs 批量/单个设置员工自定义下班时间（仅写本地 DB，无钉钉 API 调用）
func (s *ShiftConfigService) SetConfigs(input *SetShiftConfigInput) (int, error) {
	var users []database.User
	s.db.Where("user_id IN ?", input.UserIDs).Find(&users)
	nameMap := make(map[string]string, len(users))
	for _, u := range users {
		nameMap[u.UserID] = u.Name
	}

	count := 0
	for _, uid := range input.UserIDs {
		cfg := &database.EmployeeShiftConfig{
			UserID:   uid,
			UserName: nameMap[uid],
			ShiftID:  input.ShiftID,
			EndTime:  input.EndTime,
			Note:     input.Note,
		}
		if err := s.repo.Upsert(cfg); err != nil {
			return count, fmt.Errorf("设置用户 %s 失败: %w", uid, err)
		}
		count++
	}
	return count, nil
}

// DeleteConfig 删除员工自定义配置（恢复为默认下班时间）
func (s *ShiftConfigService) DeleteConfig(userID string) error {
	return s.repo.DeleteByUserID(userID)
}

// GetOrCreateShift 查找已有同名班次或创建新班次，返回钉钉班次 ID。
// 调用钉钉 API 次数：最多 2 次（GetShiftList + CreateShift）。
func (s *ShiftConfigService) GetOrCreateShift(name, checkIn, checkOut string) (int64, error) {
	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID == "" {
		return 0, fmt.Errorf("missing DINGTALK_ADMIN_USER_ID")
	}

	shiftKey := normalize(name, checkIn, checkOut)

	if cachedID, ok := getCachedShiftID(shiftKey); ok {
		return cachedID, nil
	}

	if persistedID, err := s.getPersistedShiftID(shiftKey); err != nil {
		return 0, err
	} else if persistedID > 0 {
		cacheShiftID(shiftKey, persistedID)
		return persistedID, nil
	}

	shifts, err := dingtalk.GetShiftList()
	if err != nil {
		return 0, fmt.Errorf("get shift list failed: %w", err)
	}
	for _, shift := range shifts {
		if shiftName, ok := shift["name"].(string); ok {
			if id := int64(shiftConfigFloatFromMap(shift, "id")); id > 0 {
				// 提取打卡时间
				checkInTime := ""
				checkOutTime := ""
				if timeRanges, ok := shift["time_ranges"].([]interface{}); ok && len(timeRanges) > 0 {
					if timeRange, ok := timeRanges[0].(map[string]interface{}); ok {
						if start, ok := timeRange["start_time"].(string); ok {
							checkInTime = start
						}
						if end, ok := timeRange["end_time"].(string); ok {
							checkOutTime = end
						}
					}
				}

				existingShiftKey := normalize(shiftName, checkInTime, checkOutTime)
				cacheShiftID(existingShiftKey, id)
				if existingShiftKey == shiftKey {
					if err := s.persistShiftID(name, existingShiftKey, id, checkIn, checkOut); err != nil {
						return 0, err
					}
					return id, nil
				}
			}
		}
	}

	shiftID, err := dingtalk.CreateShift(opUserID, name, checkIn, checkOut)
	if err != nil {
		return 0, fmt.Errorf("create shift failed: %w", err)
	}
	cacheShiftID(shiftKey, shiftID)
	if err := s.persistShiftID(name, shiftKey, shiftID, checkIn, checkOut); err != nil {
		return 0, err
	}
	return shiftID, nil
}

func getCachedShiftID(shiftKey string) (int64, bool) {
	if value, ok := shiftIDCache.Load(shiftKey); ok {
		if id, ok := value.(int64); ok && id > 0 {
			return id, true
		}
	}
	return 0, false
}

func cacheShiftID(shiftKey string, id int64) {
	if shiftKey == "" || id <= 0 {
		return
	}
	shiftIDCache.Store(shiftKey, id)
}

// normalize 生成班次的稳定签名
func normalize(name, checkIn, checkOut string) string {
	// 去除空格并转为小写
	name = strings.TrimSpace(strings.ToLower(name))
	checkIn = strings.TrimSpace(checkIn)
	checkOut = strings.TrimSpace(checkOut)
	return fmt.Sprintf("%s|%s|%s", name, checkIn, checkOut)
}

func (s *ShiftConfigService) ApplyAndSync(input *ApplyShiftConfigInput) (*ApplyShiftConfigResult, error) {
	shiftID := input.ShiftID
	endTime := input.EndTime

	if shiftID <= 0 {
		if input.Name == "" || input.CheckIn == "" || input.CheckOut == "" {
			return nil, fmt.Errorf("shift_id or shift creation fields are required")
		}
		var err error
		shiftID, err = s.GetOrCreateShift(input.Name, input.CheckIn, input.CheckOut)
		if err != nil {
			return nil, err
		}
		if endTime == "" {
			endTime = input.CheckOut
		}
	}

	if endTime == "" {
		return nil, fmt.Errorf("end_time is required")
	}

	updatedCount, err := s.SetConfigs(&SetShiftConfigInput{
		UserIDs: input.UserIDs,
		ShiftID: shiftID,
		EndTime: endTime,
		Note:    input.Note,
	})
	if err != nil {
		return nil, err
	}

	result := &ApplyShiftConfigResult{
		ShiftID:      shiftID,
		UpdatedCount: updatedCount,
		Status:       "saved",
		Message:      "saved locally",
	}

	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID == "" {
		result.Status = "partial"
		result.Message = "saved locally, but missing DINGTALK_ADMIN_USER_ID for sync"
		return result, nil
	}

	groups, err := dingtalk.GetAttendanceGroups()
	if err != nil {
		result.Status = "partial"
		result.Message = "saved locally, but get attendance groups failed: " + err.Error()
		return result, nil
	}
	groupID, err := dingtalk.FindScheduleGroupID(groups)
	if err != nil {
		result.Status = "partial"
		result.Message = "saved locally, but no available attendance group: " + err.Error()
		return result, nil
	}
	result.GroupID = groupID
	for _, group := range groups {
		gid, ok := group["group_id"].(float64)
		if !ok || int64(gid) != groupID {
			continue
		}
		if groupName, ok := group["group_name"].(string); ok {
			result.GroupName = groupName
		}
		break
	}

	groupDetail, err := dingtalk.GetAttendanceGroup(opUserID, groupID)
	if err != nil {
		result.Status = "partial"
		result.ErrorDetail = err.Error()
		result.Message = "saved locally, but query attendance group detail failed: " + err.Error()
		return result, nil
	}
	if !dingtalk.AttendanceGroupHasShift(groupDetail, shiftID) {
		result.Status = "partial"
		result.ErrorDetail = fmt.Sprintf("shift %d is not attached to attendance group %s(%d); add the shift to the group before syncing schedules", shiftID, result.GroupName, result.GroupID)
		result.Message = fmt.Sprintf("saved locally, but shift %d is not attached to attendance group %s(%d)", shiftID, result.GroupName, result.GroupID)
		return result, nil
	}

	items, err := s.buildPartialShiftScheduleItems(input.UserIDs, input.StartDate, input.EndDate, shiftID, false)
	if err != nil {
		result.Status = "partial"
		result.Message = "saved locally, but build partial schedules failed: " + err.Error()
		return result, nil
	}
	if len(items) == 0 {
		result.Status = "success"
		result.Message = "saved locally, no explicit schedule items to sync; holidays and rest days will follow default attendance rules"
		return result, nil
	}

	successCount, failedItems, batchErr := dingtalk.BatchSetAttendanceSchedule(opUserID, items, groupID)
	result.SyncedCount = successCount
	result.FailedCount = len(failedItems)
	result.FailedItems = failedItems

	switch {
	case batchErr != nil:
		result.Status = "partial"
		result.ErrorDetail = batchErr.Error()
		result.Message = "saved locally, but partial sync failed: " + batchErr.Error()
	case len(failedItems) == len(items) && successCount == 0:
		result.Status = "partial"
		result.ErrorDetail = fmt.Sprintf("all %d schedule items were rejected by attendance group %s(%d); usually this means the selected employees are not in that group or the group was auto-selected incorrectly", len(failedItems), result.GroupName, result.GroupID)
		result.Message = fmt.Sprintf("saved locally, synced 0 schedule items, %d failed in attendance group %s(%d)", len(failedItems), result.GroupName, result.GroupID)
	case len(failedItems) > 0:
		result.Status = "partial"
		result.Message = fmt.Sprintf("saved locally, synced %d schedule items, %d failed in attendance group %s(%d)", successCount, len(failedItems), result.GroupName, result.GroupID)
	default:
		result.Status = "success"
		result.Message = fmt.Sprintf("created/bound shift and synced %d schedule items", successCount)
	}

	return result, nil
}

func (s *ShiftConfigService) Preview(input *PreviewShiftConfigInput) (*PreviewShiftConfigResult, error) {
	shiftName, err := s.resolvePreviewShiftName(input)
	if err != nil {
		return nil, err
	}

	canSyncRest := false
	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID != "" {
		if groups, groupErr := dingtalk.GetAttendanceGroups(); groupErr == nil {
			if groupID, findErr := dingtalk.FindScheduleGroupID(groups); findErr == nil {
				if groupDetail, detailErr := dingtalk.GetAttendanceGroup(opUserID, groupID); detailErr == nil {
					canSyncRest = dingtalk.GetAttendanceGroupRestClassID(groupDetail) > 0
				}
			}
		}
	}

	decisions, err := s.buildPartialShiftDecisions(input.UserIDs, input.StartDate, input.EndDate, canSyncRest)
	if err != nil {
		return nil, err
	}

	items := make([]ShiftPreviewItem, 0, len(decisions))
	for _, decision := range decisions {
		item := ShiftPreviewItem{
			UserID:      decision.UserID,
			UserName:    decision.UserName,
			WorkDate:    decision.WorkDate,
			IsRest:      decision.IsRest,
			Reason:      decision.Reason,
			WeekType:    decision.WeekType,
			HolidayName: decision.HolidayName,
			HolidayType: decision.HolidayType,
			WillSync:    decision.WillSync,
		}
		if decision.IsRest {
			item.ShiftName = "休"
		} else {
			item.ShiftName = shiftName
			item.ShiftID = input.ShiftID
		}
		items = append(items, item)
	}

	return &PreviewShiftConfigResult{Items: items}, nil
}

func (s *ShiftConfigService) buildPartialShiftScheduleItems(userIDs []string, startDate, endDate string, shiftID int64, canSyncRest bool) ([]dingtalk.ScheduleItem, error) {
	decisions, err := s.buildPartialShiftDecisions(userIDs, startDate, endDate, canSyncRest)
	if err != nil {
		return nil, err
	}

	items := make([]dingtalk.ScheduleItem, 0, len(decisions))
	for _, decision := range decisions {
		if !decision.WillSync {
			continue
		}

		itemShiftID := int64(0)
		if !decision.IsRest {
			itemShiftID = shiftID
		}
		items = append(items, dingtalk.ScheduleItem{
			UserID:   decision.UserID,
			WorkDate: decision.WorkDate,
			ShiftID:  itemShiftID,
		})
	}
	return items, nil
}

func (s *ShiftConfigService) buildPartialShiftDecisions(userIDs []string, startDate, endDate string, canSyncRest bool) ([]partialShiftDecision, error) {
	_ = canSyncRest

	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start_date: %w", err)
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end_date: %w", err)
	}
	if end.Before(start) {
		return nil, fmt.Errorf("end_date must be on or after start_date")
	}
	if len(userIDs) == 0 {
		return nil, fmt.Errorf("user_ids is required")
	}

	seenUsers := make(map[string]struct{}, len(userIDs))
	normalizedUsers := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID == "" {
			continue
		}
		if _, exists := seenUsers[userID]; exists {
			continue
		}
		seenUsers[userID] = struct{}{}
		normalizedUsers = append(normalizedUsers, userID)
	}

	var users []database.User
	if err := s.db.Where("user_id IN ?", normalizedUsers).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("query users failed: %w", err)
	}
	userDeptMap := make(map[string]string, len(users))
	userNameMap := make(map[string]string, len(users))
	for _, user := range users {
		userDeptMap[user.UserID] = user.DepartmentID
		userNameMap[user.UserID] = user.Name
	}

	var holidays []database.StatutoryHoliday
	if err := s.db.Where("date >= ? AND date <= ?", startDate, endDate).Order("date ASC").Find(&holidays).Error; err != nil {
		return nil, fmt.Errorf("query holidays failed: %w", err)
	}
	holidayMap := make(map[string]database.StatutoryHoliday, len(holidays))
	for _, holiday := range holidays {
		holidayMap[holiday.Date] = holiday
	}

	weekService := NewWeekScheduleService(s.db)

	items := make([]partialShiftDecision, 0, len(normalizedUsers)*7)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		day := d.Format("2006-01-02")
		for _, userID := range normalizedUsers {
			if holiday, ok := holidayMap[day]; ok {
				items = append(items, partialShiftDecision{
					UserID:      userID,
					UserName:    userNameMap[userID],
					WorkDate:    day,
					IsRest:      holiday.Type == "holiday",
					Reason:      mapHolidayReason(holiday.Type),
					HolidayName: holiday.Name,
					HolidayType: holiday.Type,
					WillSync:    true,
				})
				continue
			}

			switch d.Weekday() {
			case time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday:
				items = append(items, partialShiftDecision{
					UserID:   userID,
					UserName: userNameMap[userID],
					WorkDate: day,
					Reason:   "weekday",
					WillSync: true,
				})
			case time.Saturday:
				weekType, err := weekService.GetWeekType(userID, userDeptMap[userID], day)
				if err != nil {
					return nil, fmt.Errorf("get week type failed for user %s on %s: %w", userID, day, err)
				}
				items = append(items, partialShiftDecision{
					UserID:   userID,
					UserName: userNameMap[userID],
					WorkDate: day,
					IsRest:   weekType != "small",
					Reason:   mapSaturdayReason(weekType),
					WeekType: weekType,
					WillSync: true,
				})
			case time.Sunday:
				items = append(items, partialShiftDecision{
					UserID:   userID,
					UserName: userNameMap[userID],
					WorkDate: day,
					IsRest:   true,
					Reason:   "sunday_rest",
					WillSync: false,
				})
			}
		}
	}
	return items, nil
}

func hasRestScheduleItems(items []dingtalk.ScheduleItem) bool {
	for _, item := range items {
		if item.ShiftID == 0 {
			return true
		}
	}
	return false
}

func (s *ShiftConfigService) getPersistedShiftID(shiftKey string) (int64, error) {
	var record database.DingTalkShiftCatalog
	err := s.db.Where("shift_key = ?", shiftKey).First(&record).Error
	if err == nil {
		return record.ShiftID, nil
	}
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	return 0, fmt.Errorf("query local shift catalog failed: %w", err)
}

func (s *ShiftConfigService) persistShiftID(name, shiftKey string, shiftID int64, checkIn, checkOut string) error {
	if name == "" || shiftKey == "" || shiftID <= 0 {
		return nil
	}

	record := database.DingTalkShiftCatalog{
		Name:     name,
		ShiftKey: shiftKey,
		ShiftID:  shiftID,
		CheckIn:  checkIn,
		CheckOut: checkOut,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "shift_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "shift_id", "check_in", "check_out", "updated_at"}),
	}).Create(&record).Error
}

func (s *ShiftConfigService) ListShiftCatalogs() ([]ShiftCatalogItem, error) {
	var catalogs []database.DingTalkShiftCatalog
	if err := s.db.Order("updated_at DESC").Find(&catalogs).Error; err != nil {
		return nil, fmt.Errorf("query shift catalogs failed: %w", err)
	}

	items := make([]ShiftCatalogItem, 0, len(catalogs))
	for _, catalog := range catalogs {
		items = append(items, ShiftCatalogItem{
			ID:       catalog.ID,
			Name:     catalog.Name,
			ShiftID:  catalog.ShiftID,
			CheckIn:  catalog.CheckIn,
			CheckOut: catalog.CheckOut,
		})
	}

	if len(items) == 0 {
		return items, nil
	}

	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID == "" {
		return items, nil
	}

	groups, err := dingtalk.GetAttendanceGroups()
	if err != nil {
		return items, nil
	}
	groupID, err := dingtalk.FindScheduleGroupID(groups)
	if err != nil {
		return items, nil
	}

	var groupName string
	for _, group := range groups {
		gid, ok := group["group_id"].(float64)
		if !ok || int64(gid) != groupID {
			continue
		}
		groupName, _ = group["group_name"].(string)
		break
	}

	groupDetail, err := dingtalk.GetAttendanceGroup(opUserID, groupID)
	if err != nil {
		for i := range items {
			items[i].GroupID = groupID
			items[i].GroupName = groupName
		}
		return items, nil
	}

	for i := range items {
		items[i].GroupID = groupID
		items[i].GroupName = groupName
		items[i].AttachedToGroup = dingtalk.AttendanceGroupHasShift(groupDetail, items[i].ShiftID)
	}

	return items, nil
}

func shiftConfigFloatFromMap(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

func (s *ShiftConfigService) resolvePreviewShiftName(input *PreviewShiftConfigInput) (string, error) {
	if input.Name != "" {
		return input.Name, nil
	}
	if input.ShiftID > 0 {
		var catalog database.DingTalkShiftCatalog
		err := s.db.Where("shift_id = ?", input.ShiftID).Order("updated_at DESC").First(&catalog).Error
		if err == nil && catalog.Name != "" {
			return catalog.Name, nil
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return "", fmt.Errorf("query shift catalog failed: %w", err)
		}
		return fmt.Sprintf("班次#%d", input.ShiftID), nil
	}
	if input.EndTime != "" {
		return input.EndTime + "下班", nil
	}
	if input.CheckOut != "" {
		return input.CheckOut + "下班", nil
	}
	return "已选班次", nil
}

func mapHolidayReason(holidayType string) string {
	if holidayType == "workday" {
		return "workday_adjustment"
	}
	return "holiday"
}

func mapSaturdayReason(weekType string) string {
	if weekType == "small" {
		return "small_week_saturday"
	}
	return "big_week_saturday"
}
