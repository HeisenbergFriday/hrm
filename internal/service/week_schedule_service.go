package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"peopleops/internal/config"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// WeekInfo 单周信息
type WeekInfo struct {
	WeekStart    string    `json:"week_start"`    // 周一日期
	WeekEnd      string    `json:"week_end"`      // 周日日期
	WeekType     string    `json:"week_type"`     // big/small
	IsOverride   bool      `json:"is_override"`   // 是否手动覆盖
	SaturdayWork bool      `json:"saturday_work"` // 周六是否上班
	Holidays     []DayInfo `json:"holidays"`      // 本周内的节假日/调休上班日
}

// DayInfo 单日特殊信息
type DayInfo struct {
	Date string `json:"date"`
	Name string `json:"name"`
	Type string `json:"type"` // holiday/workday
}

// SyncResult 同步结果
type WeekSyncResult struct {
	UserCount int    `json:"user_count"`
	WeekCount int    `json:"week_count"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type effectiveShiftAssignment struct {
	ShiftID int64
	Source  string
}

type saturdayScheduleSignal struct {
	Date         string
	HolidayType  string
	WorkUsers    int
	RestUsers    int
	UnknownUsers int
}

type WeekScheduleService struct {
	scheduleRepo *repository.WeekScheduleRepository
	db           *gorm.DB
}

func NewWeekScheduleService(db *gorm.DB) *WeekScheduleService {
	return &WeekScheduleService{
		scheduleRepo: repository.NewWeekScheduleRepository(db),
		db:           db,
	}
}

// ===================== 规则管理 =====================

func (s *WeekScheduleService) CreateRule(rule *database.WeekScheduleRule) error {
	if rule.Status == "" {
		rule.Status = "active"
	}
	// 检查是否已存在相同scope的规则
	existing, err := s.scheduleRepo.FindRuleByScope(rule.ScopeType, rule.ScopeID)
	if err == nil && existing != nil {
		// 存在则更新
		existing.BaseDate = rule.BaseDate
		existing.Pattern = rule.Pattern
		existing.ShiftID = rule.ShiftID
		existing.Status = rule.Status
		existing.ScopeName = rule.ScopeName
		return s.scheduleRepo.UpdateRule(existing)
	}
	// 不存在则创建
	return s.scheduleRepo.CreateRule(rule)
}

func (s *WeekScheduleService) UpdateRule(rule *database.WeekScheduleRule) error {
	return s.scheduleRepo.UpdateRule(rule)
}

func (s *WeekScheduleService) DeleteRule(id uint) error {
	return s.scheduleRepo.DeleteRule(id)
}

func (s *WeekScheduleService) GetRuleByID(id uint) (*database.WeekScheduleRule, error) {
	return s.scheduleRepo.FindRuleByID(id)
}

func (s *WeekScheduleService) GetAllRules() ([]database.WeekScheduleRule, error) {
	return s.scheduleRepo.FindAllRules()
}

// ===================== 批量规则 =====================

type BatchSetUserRulesInput struct {
	UserIDs      []string `json:"user_ids"`
	BaseDate     string   `json:"base_date"`
	Pattern      string   `json:"pattern"`
	ShiftID      int64    `json:"shift_id"`
	ConflictMode string   `json:"conflict_mode"` // overwrite / skip
	DryRun       bool     `json:"dry_run"`
}

type BatchSetUserRulesResult struct {
	Total     int                 `json:"total"`
	Created   int                 `json:"created"`
	Updated   int                 `json:"updated"`
	Skipped   int                 `json:"skipped"`
	Conflicts []BatchConflictInfo `json:"conflicts,omitempty"`
}

type BatchConflictInfo struct {
	UserID       string                     `json:"user_id"`
	UserName     string                     `json:"user_name"`
	ExistingRule *database.WeekScheduleRule `json:"existing_rule"`
}

func (s *WeekScheduleService) BatchSetUserRules(input *BatchSetUserRulesInput, userMap map[string]database.User) (*BatchSetUserRulesResult, error) {
	existingRules, err := s.scheduleRepo.FindActiveRulesByUserIDs(input.UserIDs)
	if err != nil {
		return nil, fmt.Errorf("查询现有规则失败: %w", err)
	}

	ruleMap := make(map[string]*database.WeekScheduleRule, len(existingRules))
	for i := range existingRules {
		ruleMap[existingRules[i].ScopeID] = &existingRules[i]
	}

	result := &BatchSetUserRulesResult{Total: len(input.UserIDs)}
	var toCreate []*database.WeekScheduleRule
	var toUpdate []*database.WeekScheduleRule

	for _, uid := range input.UserIDs {
		user := userMap[uid]
		existing := ruleMap[uid]

		if existing != nil {
			result.Conflicts = append(result.Conflicts, BatchConflictInfo{
				UserID:       uid,
				UserName:     user.Name,
				ExistingRule: existing,
			})
			if input.ConflictMode == "skip" {
				result.Skipped++
				continue
			}
			existing.BaseDate = input.BaseDate
			existing.Pattern = input.Pattern
			existing.ShiftID = input.ShiftID
			existing.Status = "active"
			toUpdate = append(toUpdate, existing)
			result.Updated++
		} else {
			toCreate = append(toCreate, &database.WeekScheduleRule{
				ScopeType: "user",
				ScopeID:   uid,
				ScopeName: user.Name,
				BaseDate:  input.BaseDate,
				Pattern:   input.Pattern,
				ShiftID:   input.ShiftID,
				Status:    "active",
			})
			result.Created++
		}
	}

	if input.DryRun {
		return result, nil
	}

	for _, rule := range toCreate {
		if err := s.scheduleRepo.CreateRule(rule); err != nil {
			return nil, fmt.Errorf("创建规则失败(用户%s): %w", rule.ScopeID, err)
		}
	}
	for _, rule := range toUpdate {
		if err := s.scheduleRepo.UpdateRule(rule); err != nil {
			return nil, fmt.Errorf("更新规则失败(用户%s): %w", rule.ScopeID, err)
		}
	}

	return result, nil
}

// ===================== 覆盖管理 =====================

func (s *WeekScheduleService) SetOverride(override *database.WeekScheduleOverride) error {
	// 如果已存在相同范围和日期的覆盖，更新它
	existing, err := s.scheduleRepo.FindOverride(override.ScopeType, override.ScopeID, override.WeekStartDate)
	if err == nil && existing != nil {
		existing.WeekType = override.WeekType
		existing.Reason = override.Reason
		return s.db.Save(existing).Error
	}
	return s.scheduleRepo.CreateOverride(override)
}

func (s *WeekScheduleService) DeleteOverride(id uint) error {
	return s.scheduleRepo.DeleteOverride(id)
}

func (s *WeekScheduleService) GetOverridesByScope(scopeType, scopeID string) ([]database.WeekScheduleOverride, error) {
	return s.scheduleRepo.FindOverridesByScope(scopeType, scopeID)
}

// ===================== 核心计算 =====================

// GetWeekType 计算某用户某天属于大周还是小周
// 优先级：用户覆盖 > 用户规则 > 部门覆盖 > 部门规则 > 公司覆盖 > 公司规则
func (s *WeekScheduleService) GetWeekType(userID, departmentID, date string) (string, error) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", fmt.Errorf("日期格式错误: %w", err)
	}
	weekStart := getMonday(t).Format("2006-01-02")

	// 按优先级依次查覆盖和规则
	scopes := []struct {
		scopeType string
		scopeID   string
	}{
		{"user", userID},
		{"department", departmentID},
		{"company", ""},
	}

	for _, scope := range scopes {
		if scope.scopeID == "" && scope.scopeType != "company" {
			continue
		}

		// 先查覆盖
		override, err := s.scheduleRepo.FindOverride(scope.scopeType, scope.scopeID, weekStart)
		if err == nil && override != nil {
			return override.WeekType, nil
		}

		// 再查规则
		rule, err := s.scheduleRepo.FindRuleByScope(scope.scopeType, scope.scopeID)
		if err == nil && rule != nil {
			return calcWeekTypeByRule(rule, weekStart)
		}
	}

	// 无任何规则，默认大周（双休）
	return "big", nil
}

// GetWeekCalendar 获取未来N周的大小周日历（含节假日）
func (s *WeekScheduleService) GetWeekCalendar(userID, departmentID string, weeks int) ([]WeekInfo, error) {
	if weeks <= 0 {
		weeks = 8
	}

	now := time.Now()
	monday := getMonday(now)

	// 预加载整个时间范围的节假日
	startDate := monday.Format("2006-01-02")
	endDate := monday.AddDate(0, 0, weeks*7-1).Format("2006-01-02")
	holidays, _ := s.scheduleRepo.FindHolidaysByDateRange(startDate, endDate)

	// 按日期索引
	holidayMap := make(map[string]*database.StatutoryHoliday)
	for i := range holidays {
		holidayMap[holidays[i].Date] = &holidays[i]
	}

	var calendar []WeekInfo
	for i := 0; i < weeks; i++ {
		weekStart := monday.AddDate(0, 0, i*7)
		weekEnd := weekStart.AddDate(0, 0, 6)
		weekStartStr := weekStart.Format("2006-01-02")

		weekType, err := s.GetWeekType(userID, departmentID, weekStartStr)
		if err != nil {
			weekType = "big"
		}

		isOverride := s.isOverride(userID, departmentID, weekStartStr)

		// 收集本周内的节假日/调休日
		var weekHolidays []DayInfo
		saturdayWork := weekType == "small" // 默认：小周周六上班

		for d := 0; d < 7; d++ {
			day := weekStart.AddDate(0, 0, d)
			dayStr := day.Format("2006-01-02")
			if h, ok := holidayMap[dayStr]; ok {
				weekHolidays = append(weekHolidays, DayInfo{
					Date: h.Date,
					Name: h.Name,
					Type: h.Type,
				})
				// 如果周六被标记为节假日（放假），覆盖大小周的上班设置
				if d == 5 && h.Type == "holiday" {
					saturdayWork = false
				}
				// 如果周六被标记为调休上班，强制上班
				if d == 5 && h.Type == "workday" {
					saturdayWork = true
				}
			}
		}

		calendar = append(calendar, WeekInfo{
			WeekStart:    weekStartStr,
			WeekEnd:      weekEnd.Format("2006-01-02"),
			WeekType:     weekType,
			IsOverride:   isOverride,
			SaturdayWork: saturdayWork,
			Holidays:     weekHolidays,
		})
	}

	return calendar, nil
}

// ===================== 同步到钉钉 =====================

// SyncToDingTalk 将大小周配置同步到钉钉
func (s *WeekScheduleService) SyncToDingTalk(weeks int) (*WeekSyncResult, error) {
	if weeks <= 0 {
		weeks = 4
	}

	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID == "" {
		return nil, fmt.Errorf("未配置 DINGTALK_ADMIN_USER_ID 环境变量")
	}

	// 1. 获取所有活跃用户
	var users []database.User
	if err := s.db.Where("status = ? AND user_id != ?", "active", "admin").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("获取用户列表失败: %w", err)
	}

	// 2. 获取班次列表，找到第一个正常工作班次
	shifts, err := dingtalk.GetShiftList()
	if err != nil {
		return nil, fmt.Errorf("获取班次列表失败: %w", err)
	}

	var normalShiftID int64
	for _, shift := range shifts {
		shiftID := int64(getFloatFromMap(shift, "id"))
		if shiftID > 0 {
			normalShiftID = shiftID
			break
		}
	}
	if normalShiftID == 0 {
		return nil, fmt.Errorf("未找到可用班次，请先在钉钉后台创建班次")
	}

	// 3. 预取考勤组 ID（避免在循环内重复调用 GetAttendanceGroups）
	groups, err := dingtalk.GetAttendanceGroups()
	if err != nil {
		return nil, fmt.Errorf("获取考勤组失败: %w", err)
	}
	scheduleGroupID, err := dingtalk.FindScheduleGroupID(groups)
	if err != nil {
		return nil, fmt.Errorf("解析考勤组失败: %w", err)
	}

	// 4. 加载自定义班次规则，构建用户→班次映射
	shiftRules, _ := s.scheduleRepo.FindActiveRulesWithShift()
	userShiftMap := make(map[string]int64)
	deptShiftMap := make(map[string]int64)
	var companyShiftID int64
	for _, rule := range shiftRules {
		switch rule.ScopeType {
		case "user":
			userShiftMap[rule.ScopeID] = rule.ShiftID
		case "department":
			deptShiftMap[rule.ScopeID] = rule.ShiftID
		case "company":
			companyShiftID = rule.ShiftID
		}
	}

	// 4b. 加载员工自定义下班时间，优先级最高
	var customShiftConfigs []database.EmployeeShiftConfig
	s.db.Find(&customShiftConfigs)
	customShiftMap := make(map[string]int64, len(customShiftConfigs))
	for _, cfg := range customShiftConfigs {
		customShiftMap[cfg.UserID] = cfg.ShiftID
	}

	// 5. 计算未来N周的日历，收集所有排班条目后批量推送
	now := time.Now()
	monday := getMonday(now)
	syncedUsers := make(map[string]bool)

	// 预加载节假日
	startDate := monday.Format("2006-01-02")
	endDate := monday.AddDate(0, 0, weeks*7-1).Format("2006-01-02")
	holidays, _ := s.scheduleRepo.FindHolidaysByDateRange(startDate, endDate)
	holidayMap := make(map[string]*database.StatutoryHoliday)
	for i := range holidays {
		holidayMap[holidays[i].Date] = &holidays[i]
	}

	var items []dingtalk.ScheduleItem

	for i := 0; i < weeks; i++ {
		weekStart := monday.AddDate(0, 0, i*7)

		for _, user := range users {
			weekType, err := s.GetWeekType(user.UserID, user.DepartmentID, weekStart.Format("2006-01-02"))
			if err != nil {
				continue
			}

			// 解析该用户的有效班次（自定义下班时间 > 用户规则 > 部门规则 > 公司规则 > 默认）
			effectiveShiftID := normalShiftID
			if sid, ok := customShiftMap[user.UserID]; ok {
				effectiveShiftID = sid
			} else if sid, ok := userShiftMap[user.UserID]; ok {
				effectiveShiftID = sid
			} else if sid, ok := deptShiftMap[user.DepartmentID]; ok {
				effectiveShiftID = sid
			} else if companyShiftID > 0 {
				effectiveShiftID = companyShiftID
			}
			assignment := resolveEffectiveShiftAssignment(normalShiftID, user.UserID, user.DepartmentID, customShiftMap, userShiftMap, deptShiftMap, companyShiftID)
			effectiveShiftID = assignment.ShiftID
			needsWeekdaySync := assignment.Source != "default"

			for d := 0; d < 7; d++ {
				day := weekStart.AddDate(0, 0, d)
				dayStr := day.Format("2006-01-02")

				if day.Before(now) {
					continue
				}

				if h, ok := holidayMap[dayStr]; ok {
					if h.Type == "holiday" {
						// 法定节假日休息回落给钉钉考勤组默认规则，不做显式覆盖。
						continue
					}
					shiftID := effectiveShiftID
					items = append(items, dingtalk.ScheduleItem{UserID: user.UserID, WorkDate: dayStr, ShiftID: shiftID})
					syncedUsers[user.UserID] = true
					continue
				}

				// 周一至周五：仅同步有自定义下班时间的员工（其余人走钉钉考勤组默认班次，无需重复写入）
				if d < 5 {
					if !needsWeekdaySync {
						continue
					}
					items = append(items, dingtalk.ScheduleItem{UserID: user.UserID, WorkDate: dayStr, ShiftID: effectiveShiftID})
					syncedUsers[user.UserID] = true
					continue
				}

				// 周六：大小周逻辑
				if d == 5 {
					if weekType != "small" {
						// 大周周六休息同样回落给钉钉默认规则，不做显式覆盖。
						continue
					}
					shiftID := effectiveShiftID
					items = append(items, dingtalk.ScheduleItem{UserID: user.UserID, WorkDate: dayStr, ShiftID: shiftID})
					syncedUsers[user.UserID] = true
				}
			}
		}
	}

	successCount, failedItems, batchErr := dingtalk.BatchSetAttendanceSchedule(opUserID, items, scheduleGroupID)

	status := "success"
	message := fmt.Sprintf("成功同步 %d 条排班", successCount)
	if len(failedItems) > 0 || batchErr != nil {
		status = "partial"
		errDetail := ""
		if batchErr != nil {
			errDetail = ": " + batchErr.Error()
		}
		message = fmt.Sprintf("同步 %d 条成功，%d 条失败%s", successCount, len(failedItems), errDetail)
	}

	s.scheduleRepo.CreateSyncLog(&database.WeekScheduleSyncLog{
		SyncType:  "to_dingtalk",
		UserCount: len(syncedUsers),
		Status:    status,
		Message:   message,
	})

	return &WeekSyncResult{
		UserCount: len(syncedUsers),
		WeekCount: weeks,
		Status:    status,
		Message:   message,
	}, nil
}

// SyncFromDingTalk 从钉钉读取排班数据，推断大小周配置
func (s *WeekScheduleService) SyncFromDingTalk() (*WeekSyncResult, error) {
	// 1. 获取活跃用户（取第一个作为样本）
	var users []database.User
	if err := s.db.Where("status = ? AND user_id != ?", "active", "admin").Limit(5).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("获取用户列表失败: %w", err)
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("无活跃用户")
	}

	// 2. 读取过去4周和未来2周的排班数据
	now := time.Now()
	startDate := getMonday(now).AddDate(0, 0, -28)
	endDate := getMonday(now).AddDate(0, 0, 13)

	// 收集用户 ID
	userIDs := make([]string, len(users))
	for i, u := range users {
		userIDs[i] = u.UserID
	}

	// 统计各周六是否有排班（按天批量查询，而非按用户×天逐条查询）
	saturdayWork := make(map[string]bool) // date -> hasWork

	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		// 只关心周六，跳过其他天
		if d.Weekday() != time.Saturday {
			continue
		}
		dateStr := d.Format("2006-01-02")
		schedules, err := dingtalk.GetScheduleListBatchByDay(userIDs, dateStr)
		if err != nil {
			logrus.Warnf("批量获取 %s 排班失败: %v", dateStr, err)
			continue
		}

		for _, sch := range schedules {
			isRest, _ := sch["is_rest"].(string)
			shiftID := int64(getFloatFromMap(sch, "shift_id"))
			if isRest != "Y" && shiftID > 1 { // shift_id=1 在该接口表示休息班次
				saturdayWork[dateStr] = true
				break // 一个用户有排班即可确认该周六为工作日
			}
		}
	}

	if len(saturdayWork) == 0 {
		return &WeekSyncResult{
			Status:  "success",
			Message: "未从钉钉发现周六排班数据，可能尚未配置大小周",
		}, nil
	}

	// 3. 分析模式：找到最近一个有排班的周六，推断基准日期
	// 有排班的周六 → 所在周为小周 → 前一周为大周 → 那个周一是基准日期
	var latestWorkSaturday time.Time
	for dateStr := range saturdayWork {
		t, _ := time.Parse("2006-01-02", dateStr)
		if t.After(latestWorkSaturday) {
			latestWorkSaturday = t
		}
	}

	// 该周六所在周的周一
	smallWeekMonday := getMonday(latestWorkSaturday)
	// 前一周周一就是大周
	bigWeekMonday := smallWeekMonday.AddDate(0, 0, -7)

	// 4. 保存/更新公司级规则
	existingRule, _ := s.scheduleRepo.FindRuleByScope("company", "")
	if existingRule != nil {
		existingRule.BaseDate = bigWeekMonday.Format("2006-01-02")
		existingRule.Pattern = "big_first"
		s.scheduleRepo.UpdateRule(existingRule)
	} else {
		s.scheduleRepo.CreateRule(&database.WeekScheduleRule{
			ScopeType: "company",
			ScopeID:   "",
			ScopeName: "全公司",
			BaseDate:  bigWeekMonday.Format("2006-01-02"),
			Pattern:   "big_first",
			Status:    "active",
		})
	}

	message := fmt.Sprintf("从钉钉推断出大小周规则: 基准大周=%s, 检测到 %d 个工作周六",
		bigWeekMonday.Format("2006-01-02"), len(saturdayWork))

	s.scheduleRepo.CreateSyncLog(&database.WeekScheduleSyncLog{
		SyncType:  "from_dingtalk",
		UserCount: len(users),
		Status:    "success",
		Message:   message,
	})

	return &WeekSyncResult{
		UserCount: len(users),
		Status:    "success",
		Message:   message,
	}, nil
}

// GetSyncLogs 获取同步日志
func (s *WeekScheduleService) SyncFromDingTalkConservative() (*WeekSyncResult, error) {
	var users []database.User
	if err := s.db.Where("status = ? AND user_id != ?", "active", "admin").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("query active users failed: %w", err)
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("no active users found")
	}

	now := time.Now()
	startDate := getMonday(now).AddDate(0, 0, -28)
	endDate := getMonday(now).AddDate(0, 0, 13)
	startDateStr := startDate.Format("2006-01-02")
	endDateStr := endDate.Format("2006-01-02")

	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		if user.UserID != "" {
			userIDs = append(userIDs, user.UserID)
		}
	}

	holidayTypeByDate := make(map[string]string)
	if holidays, err := s.scheduleRepo.FindHolidaysByDateRange(startDateStr, endDateStr); err == nil {
		for _, holiday := range holidays {
			holidayTypeByDate[holiday.Date] = holiday.Type
		}
	}

	signals := make([]saturdayScheduleSignal, 0)
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday {
			continue
		}

		dateStr := d.Format("2006-01-02")
		schedules, err := fetchScheduleListBatchByDayChunked(userIDs, dateStr)
		if err != nil {
			logrus.Warnf("fetch saturday schedules failed for %s: %v", dateStr, err)
			continue
		}

		signals = append(signals, buildSaturdayScheduleSignal(dateStr, userIDs, schedules, holidayTypeByDate[dateStr]))
	}

	inference, err := inferCompanyWeekRuleFromSignals(signals)
	if err != nil {
		message := "could not infer company week rule safely: " + err.Error()
		s.scheduleRepo.CreateSyncLog(&database.WeekScheduleSyncLog{
			SyncType:  "from_dingtalk",
			UserCount: len(users),
			Status:    "partial",
			Message:   message,
		})
		return &WeekSyncResult{
			UserCount: len(users),
			Status:    "partial",
			Message:   message,
		}, nil
	}

	existingRule, _ := s.scheduleRepo.FindRuleByScope("company", "")
	if existingRule != nil {
		existingRule.BaseDate = inference.BaseDate
		existingRule.Pattern = inference.Pattern
		existingRule.Status = "active"
		if err := s.scheduleRepo.UpdateRule(existingRule); err != nil {
			return nil, fmt.Errorf("update inferred company rule failed: %w", err)
		}
	} else {
		if err := s.scheduleRepo.CreateRule(&database.WeekScheduleRule{
			ScopeType: "company",
			ScopeID:   "",
			ScopeName: "company",
			BaseDate:  inference.BaseDate,
			Pattern:   inference.Pattern,
			Status:    "active",
		}); err != nil {
			return nil, fmt.Errorf("create inferred company rule failed: %w", err)
		}
	}

	message := fmt.Sprintf("safely inferred company week rule: base big week=%s from %d clean Saturdays", inference.BaseDate, inference.CleanSaturdayCount)
	s.scheduleRepo.CreateSyncLog(&database.WeekScheduleSyncLog{
		SyncType:  "from_dingtalk",
		UserCount: len(users),
		Status:    "success",
		Message:   message,
	})

	return &WeekSyncResult{
		UserCount: len(users),
		Status:    "success",
		Message:   message,
	}, nil
}

func fetchScheduleListBatchByDayChunked(userIDs []string, workDate string) ([]map[string]interface{}, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	const batchSize = 50
	schedules := make([]map[string]interface{}, 0)
	for start := 0; start < len(userIDs); start += batchSize {
		end := start + batchSize
		if end > len(userIDs) {
			end = len(userIDs)
		}

		items, err := dingtalk.GetScheduleListBatchByDay(userIDs[start:end], workDate)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, items...)
	}

	return schedules, nil
}

type companyWeekRuleInference struct {
	BaseDate           string
	Pattern            string
	CleanSaturdayCount int
}

func buildSaturdayScheduleSignal(date string, userIDs []string, schedules []map[string]interface{}, holidayType string) saturdayScheduleSignal {
	userSet := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		userSet[userID] = struct{}{}
	}

	seenUsers := make(map[string]struct{}, len(schedules))
	workUsers := 0
	restUsers := 0
	for _, schedule := range schedules {
		userID := getScheduleUserID(schedule)
		if userID == "" {
			continue
		}
		if _, ok := userSet[userID]; !ok {
			continue
		}
		if _, ok := seenUsers[userID]; ok {
			continue
		}
		seenUsers[userID] = struct{}{}

		if isScheduleWorking(schedule) {
			workUsers++
		} else {
			restUsers++
		}
	}

	return saturdayScheduleSignal{
		Date:         date,
		HolidayType:  holidayType,
		WorkUsers:    workUsers,
		RestUsers:    restUsers,
		UnknownUsers: len(userIDs) - len(seenUsers),
	}
}

func inferCompanyWeekRuleFromSignals(signals []saturdayScheduleSignal) (*companyWeekRuleInference, error) {
	type cleanSaturday struct {
		date   time.Time
		status string
	}

	clean := make([]cleanSaturday, 0, len(signals))
	for _, signal := range signals {
		if signal.HolidayType != "" {
			continue
		}
		if signal.UnknownUsers > 0 {
			continue
		}
		switch {
		case signal.WorkUsers > 0 && signal.RestUsers == 0:
			t, err := time.Parse("2006-01-02", signal.Date)
			if err != nil {
				return nil, fmt.Errorf("invalid saturday date %s: %w", signal.Date, err)
			}
			clean = append(clean, cleanSaturday{date: t, status: "work"})
		case signal.RestUsers > 0 && signal.WorkUsers == 0:
			t, err := time.Parse("2006-01-02", signal.Date)
			if err != nil {
				return nil, fmt.Errorf("invalid saturday date %s: %w", signal.Date, err)
			}
			clean = append(clean, cleanSaturday{date: t, status: "rest"})
		}
	}

	if len(clean) < 2 {
		return nil, fmt.Errorf("not enough clean Saturdays to infer a company-wide pattern")
	}

	for i := 1; i < len(clean); i++ {
		daysDiff := int(clean[i].date.Sub(clean[i-1].date).Hours() / 24)
		if daysDiff <= 0 || daysDiff%7 != 0 {
			return nil, fmt.Errorf("detected a malformed Saturday sequence")
		}

		weeksDiff := daysDiff / 7
		sameStatus := clean[i].status == clean[i-1].status
		if sameStatus && weeksDiff%2 != 0 {
			return nil, fmt.Errorf("Saturday schedules do not alternate consistently")
		}
		if !sameStatus && weeksDiff%2 == 0 {
			return nil, fmt.Errorf("Saturday schedules do not alternate consistently")
		}
	}

	var latestWorkSaturday time.Time
	for _, saturday := range clean {
		if saturday.status != "work" {
			continue
		}
		if saturday.date.After(latestWorkSaturday) {
			latestWorkSaturday = saturday.date
		}
	}
	if latestWorkSaturday.IsZero() {
		return nil, fmt.Errorf("did not observe a clean company work Saturday")
	}

	baseBigWeek := getMonday(latestWorkSaturday).AddDate(0, 0, -7)
	return &companyWeekRuleInference{
		BaseDate:           baseBigWeek.Format("2006-01-02"),
		Pattern:            "big_first",
		CleanSaturdayCount: len(clean),
	}, nil
}

func getScheduleUserID(schedule map[string]interface{}) string {
	if userID, ok := schedule["userid"].(string); ok && userID != "" {
		return userID
	}
	if userID, ok := schedule["userId"].(string); ok && userID != "" {
		return userID
	}
	if userID, ok := schedule["user_id"].(string); ok && userID != "" {
		return userID
	}
	return ""
}

func isScheduleWorking(schedule map[string]interface{}) bool {
	isRest, _ := schedule["is_rest"].(string)
	shiftID := int64(getFloatFromMap(schedule, "shift_id"))
	return isRest != "Y" && shiftID > 1
}

func (s *WeekScheduleService) GetSyncLogs(page, pageSize int) ([]database.WeekScheduleSyncLog, int64, error) {
	return s.scheduleRepo.FindSyncLogs(page, pageSize)
}

// ===================== 法定节假日管理 =====================

func (s *WeekScheduleService) CreateHoliday(holiday *database.StatutoryHoliday) error {
	// 自动填充年份
	if holiday.Year == 0 {
		t, err := time.Parse("2006-01-02", holiday.Date)
		if err == nil {
			holiday.Year = t.Year()
		}
	}
	return s.scheduleRepo.CreateHoliday(holiday)
}

func (s *WeekScheduleService) UpdateHoliday(holiday *database.StatutoryHoliday) error {
	return s.scheduleRepo.UpdateHoliday(holiday)
}

func (s *WeekScheduleService) DeleteHoliday(id uint) error {
	return s.scheduleRepo.DeleteHoliday(id)
}

func (s *WeekScheduleService) GetHolidaysByYear(year int) ([]database.StatutoryHoliday, error) {
	return s.scheduleRepo.FindHolidaysByYear(year)
}

// BatchCreateHolidays 批量创建节假日（方便一次性导入全年节假日）
func (s *WeekScheduleService) BatchCreateHolidays(holidays []database.StatutoryHoliday) (int, error) {
	created := 0
	for i := range holidays {
		if holidays[i].Year == 0 {
			t, err := time.Parse("2006-01-02", holidays[i].Date)
			if err == nil {
				holidays[i].Year = t.Year()
			}
		}
		// 跳过已存在的日期
		existing, _ := s.scheduleRepo.FindHolidayByDate(holidays[i].Date)
		if existing != nil {
			continue
		}
		if err := s.scheduleRepo.CreateHoliday(&holidays[i]); err != nil {
			return created, fmt.Errorf("创建 %s 失败: %w", holidays[i].Date, err)
		}
		created++
	}
	return created, nil
}

// JuheHolidayResponse 聚合数据API响应结构
type JuheHolidayResponse struct {
	Reason string `json:"reason"`
	Result struct {
		Holiday []struct {
			Date string `json:"date"`
			Name string `json:"name"`
			Type int    `json:"type"`
			Desc string `json:"desc"`
			Rest int    `json:"rest"`
		} `json:"holiday"`
	} `json:"result"`
	Error_code int `json:"error_code"`
}

// SyncHolidaysFromJuhe 从聚合数据API同步节假日，失败时自动降级到硬编码数据
func (s *WeekScheduleService) SyncHolidaysFromJuhe() (int, error) {
	// 同步当前年份和下一年的节假日数据
	currentYear := time.Now().Year()
	years := []int{currentYear, currentYear + 1}
	created := 0

	for _, year := range years {
		// 先尝试从API获取数据
		apiCreated, err := s.syncHolidaysFromJuheAPI(year)
		if err == nil && apiCreated > 0 {
			created += apiCreated
			continue
		}

		// API失败，降级到配置文件数据
		logrus.Warnf("从聚合数据API同步 %d 年节假日失败，降级到配置文件数据: %v", year, err)
		configCreated, err := s.syncHolidaysFromConfig(year)
		if err == nil {
			created += configCreated
		}
	}

	logrus.Infof("同步完成，共创建 %d 个节假日", created)
	return created, nil
}

// syncHolidaysFromJuheAPI 从聚合数据API同步节假日
func (s *WeekScheduleService) syncHolidaysFromJuheAPI(year int) (int, error) {
	apiKey := os.Getenv("JUHE_API_KEY")
	if apiKey == "" {
		return 0, fmt.Errorf("未配置 JUHE_API_KEY 环境变量")
	}

	logrus.Infof("开始从聚合数据API同步 %d 年节假日数据", year)

	// 构建API请求URL
	url := fmt.Sprintf("http://v.juhe.cn/calendar/year?year=%d&key=%s", year, apiKey)

	// 发送HTTP请求
	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析响应
	var juheResp JuheHolidayResponse
	if err := json.Unmarshal(body, &juheResp); err != nil {
		return 0, fmt.Errorf("解析响应失败: %w", err)
	}

	// 检查API返回状态
	if juheResp.Error_code != 0 {
		return 0, fmt.Errorf("API返回错误: %s", juheResp.Reason)
	}

	// 处理节假日数据
	created := 0
	for _, item := range juheResp.Result.Holiday {
		// 跳过已存在的日期
		existing, _ := s.scheduleRepo.FindHolidayByDate(item.Date)
		if existing != nil {
			logrus.Infof("节假日 %s 已存在，跳过", item.Date)
			continue
		}

		// 转换类型
		holidayType := "holiday"
		if item.Type == 2 {
			holidayType = "workday"
		}

		t, err := time.Parse("2006-01-02", item.Date)
		if err != nil {
			logrus.Warnf("解析日期 %s 失败: %v", item.Date, err)
			continue
		}

		holiday := &database.StatutoryHoliday{
			Date: item.Date,
			Name: item.Name,
			Type: holidayType,
			Year: t.Year(),
		}

		if err := s.scheduleRepo.CreateHoliday(holiday); err != nil {
			logrus.Warnf("创建节假日 %s 失败: %v", item.Date, err)
			continue
		}
		logrus.Infof("成功从API创建节假日: %s - %s", item.Date, item.Name)
		created++
	}

	logrus.Infof("从API同步完成，共创建 %d 个节假日", created)
	return created, nil
}

// syncHolidaysFromConfig 从配置文件同步节假日
func (s *WeekScheduleService) syncHolidaysFromConfig(year int) (int, error) {
	logrus.Infof("开始从配置文件同步 %d 年节假日数据", year)

	// 读取配置文件
	cfg, err := config.LoadConfig()
	if err != nil {
		return 0, fmt.Errorf("加载配置失败: %w", err)
	}
	configFile := cfg.HolidaysFile
	content, err := os.ReadFile(configFile)
	if err != nil {
		return 0, fmt.Errorf("读取节假日配置文件失败: %w", err)
	}

	// 解析配置文件
	var config struct {
		Holidays map[string][]struct {
			Date string `json:"date"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"holidays"`
	}

	if err := json.Unmarshal(content, &config); err != nil {
		return 0, fmt.Errorf("解析节假日配置文件失败: %w", err)
	}

	// 获取指定年份的节假日
	yearStr := fmt.Sprintf("%d", year)
	holidays, ok := config.Holidays[yearStr]
	if !ok {
		logrus.Warnf("未找到 %d 年的节假日配置数据", year)
		return 0, nil
	}

	// 批量创建节假日（upsert，支持重复执行补充数据）
	created := 0
	for _, item := range holidays {
		t, err := time.Parse("2006-01-02", item.Date)
		if err != nil {
			logrus.Warnf("解析日期 %s 失败: %v", item.Date, err)
			continue
		}

		holiday := &database.StatutoryHoliday{
			Date: item.Date,
			Name: item.Name,
			Type: item.Type,
			Year: t.Year(),
		}

		if err := s.scheduleRepo.UpsertHoliday(holiday); err != nil {
			logrus.Warnf("创建节假日 %s 失败: %v", item.Date, err)
			continue
		}
		logrus.Infof("成功从配置文件创建节假日: %s - %s", item.Date, item.Name)
		created++
	}

	logrus.Infof("从配置文件同步完成，共创建 %d 个节假日", created)
	return created, nil
}

// ===================== 内部辅助函数 =====================

// getMonday 获取指定日期所在周的周一
func getMonday(t time.Time) time.Time {
	weekday := t.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-int(weekday-time.Monday), 0, 0, 0, 0, t.Location())
}

// calcWeekTypeByRule 根据规则计算某周的类型
func calcWeekTypeByRule(rule *database.WeekScheduleRule, weekStartStr string) (string, error) {
	baseDate, err := time.Parse("2006-01-02", rule.BaseDate)
	if err != nil {
		return "", fmt.Errorf("规则基准日期格式错误: %w", err)
	}
	// 将基准日归一化到所在周的周一，避免非周一日期导致计算错位
	baseDate = getMonday(baseDate)
	weekStart, err := time.Parse("2006-01-02", weekStartStr)
	if err != nil {
		return "", fmt.Errorf("查询日期格式错误: %w", err)
	}

	daysDiff := int(weekStart.Sub(baseDate).Hours() / 24)
	weeksDiff := daysDiff / 7

	// 负数周差也要正确处理
	if daysDiff < 0 {
		weeksDiff = -(-daysDiff / 7)
		if (-daysDiff)%7 != 0 {
			weeksDiff--
		}
	}

	isEven := weeksDiff%2 == 0

	switch rule.Pattern {
	case "big_first":
		if isEven {
			return "big", nil
		}
		return "small", nil
	case "small_first":
		if isEven {
			return "small", nil
		}
		return "big", nil
	default:
		return "big", nil
	}
}

// isOverride 检查某个日期是否有手动覆盖
func (s *WeekScheduleService) isOverride(userID, departmentID, weekStartStr string) bool {
	scopes := []struct {
		scopeType string
		scopeID   string
	}{
		{"user", userID},
		{"department", departmentID},
		{"company", ""},
	}

	for _, scope := range scopes {
		if scope.scopeID == "" && scope.scopeType != "company" {
			continue
		}
		override, err := s.scheduleRepo.FindOverride(scope.scopeType, scope.scopeID, weekStartStr)
		if err == nil && override != nil {
			return true
		}
	}
	return false
}

// getFloatFromMap 从 map 中安全提取 float64
func resolveEffectiveShiftAssignment(normalShiftID int64, userID, departmentID string, customShiftMap, userShiftMap, deptShiftMap map[string]int64, companyShiftID int64) effectiveShiftAssignment {
	assignment := effectiveShiftAssignment{
		ShiftID: normalShiftID,
		Source:  "default",
	}

	if sid, ok := customShiftMap[userID]; ok && sid > 0 {
		assignment.ShiftID = sid
		assignment.Source = "custom"
		return assignment
	}
	if sid, ok := userShiftMap[userID]; ok && sid > 0 {
		assignment.ShiftID = sid
		assignment.Source = "user_rule"
		return assignment
	}
	if sid, ok := deptShiftMap[departmentID]; ok && sid > 0 {
		assignment.ShiftID = sid
		assignment.Source = "department_rule"
		return assignment
	}
	if companyShiftID > 0 {
		assignment.ShiftID = companyShiftID
		assignment.Source = "company_rule"
	}

	return assignment
}

func getFloatFromMap(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}
