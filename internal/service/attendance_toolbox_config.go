package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"peopleops/internal/database"

	"gorm.io/gorm"
)

const attendanceToolboxConfigExtensionKey = "attendance_toolbox_config"

type MaternityLeaveOverride struct {
	EmployeeNo string `json:"employee_no"`
	Name       string `json:"name"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
}

// AttendanceToolboxConfig is the organization-shared fixed configuration.
// Lists are stored as normalized arrays so every browser receives the same
// source of truth and calculation requests do not depend on localStorage.
type AttendanceToolboxConfig struct {
	MaternityLeaveOverrides []MaternityLeaveOverride `json:"maternity_leave_overrides"`
	LeaveSpecialNames       []string                 `json:"leave_special_names"`
	ChengduScheduleNames    []string                 `json:"chengdu_schedule_names"`
	SubDeptKeywords         []string                 `json:"sub_dept_keywords"`
	SubLate22Names          []string                 `json:"sub_late22_names"`
	PartSpecialNames        []string                 `json:"part_special_names"`
	OvertimeRulesJSON       string                   `json:"overtime_rules_json,omitempty"`
}

type AttendanceToolboxConfigService struct {
	db    *gorm.DB
	orgID string
}

func NewAttendanceToolboxConfigService(db *gorm.DB, orgID string) *AttendanceToolboxConfigService {
	return &AttendanceToolboxConfigService{db: db, orgID: strings.TrimSpace(orgID)}
}

func (s *AttendanceToolboxConfigService) Get(ctx context.Context) (map[string]interface{}, error) {
	if s == nil || s.db == nil || s.orgID == "" {
		return nil, errors.New("attendance toolbox config requires organization context")
	}
	var org database.Organization
	if err := s.db.WithContext(ctx).Where("org_id = ? AND status = ?", s.orgID, "active").First(&org).Error; err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}

	config := defaultAttendanceToolboxConfig()
	if org.Extension == nil {
		config["configured"] = false
		return config, nil
	}
	value, ok := org.Extension[attendanceToolboxConfigExtensionKey]
	if !ok {
		config["configured"] = false
		return config, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("read attendance toolbox config: %w", err)
	}
	var saved AttendanceToolboxConfig
	if err := json.Unmarshal(raw, &saved); err != nil {
		return nil, fmt.Errorf("read attendance toolbox config: %w", err)
	}
	merged := mergeAttendanceToolboxConfig(config, saved)
	merged["configured"] = true
	return merged, nil
}

func (s *AttendanceToolboxConfigService) Save(ctx context.Context, input AttendanceToolboxConfig) (map[string]interface{}, error) {
	if s == nil || s.db == nil || s.orgID == "" {
		return nil, errors.New("attendance toolbox config requires organization context")
	}
	normalized, err := normalizeAttendanceToolboxConfig(input)
	if err != nil {
		return nil, err
	}
	if normalized.OvertimeRulesJSON != "" && !json.Valid([]byte(normalized.OvertimeRulesJSON)) {
		return nil, errors.New("加班规则配置格式无效")
	}

	var org database.Organization
	db := s.db.WithContext(ctx)
	if err := db.Where("org_id = ? AND status = ?", s.orgID, "active").First(&org).Error; err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}
	if org.Extension == nil {
		org.Extension = map[string]interface{}{}
	}
	org.Extension[attendanceToolboxConfigExtensionKey] = normalized
	if err := db.Model(&database.Organization{}).
		Where("id = ? AND org_id = ? AND status = ?", org.ID, s.orgID, "active").
		Update("extension", org.Extension).Error; err != nil {
		return nil, fmt.Errorf("save attendance toolbox config: %w", err)
	}
	return s.Get(ctx)
}

func defaultAttendanceToolboxConfig() map[string]interface{} {
	return map[string]interface{}{
		"maternity_leave_overrides": []MaternityLeaveOverride{},
		"leave_special_names":       []string{},
		"chengdu_schedule_names":    []string{},
		"sub_dept_keywords":         []string{},
		"sub_late22_names":          []string{},
		"part_special_names":        []string{},
		"overtime_rules_json":       "",
	}
}

func mergeAttendanceToolboxConfig(base map[string]interface{}, saved AttendanceToolboxConfig) map[string]interface{} {
	base["maternity_leave_overrides"] = saved.MaternityLeaveOverrides
	base["leave_special_names"] = saved.LeaveSpecialNames
	base["chengdu_schedule_names"] = saved.ChengduScheduleNames
	base["sub_dept_keywords"] = saved.SubDeptKeywords
	base["sub_late22_names"] = saved.SubLate22Names
	base["part_special_names"] = saved.PartSpecialNames
	base["overtime_rules_json"] = saved.OvertimeRulesJSON
	return base
}

func normalizeAttendanceToolboxConfig(input AttendanceToolboxConfig) (AttendanceToolboxConfig, error) {
	input.LeaveSpecialNames = normalizeStringList(input.LeaveSpecialNames)
	input.ChengduScheduleNames = normalizeStringList(input.ChengduScheduleNames)
	input.SubDeptKeywords = normalizeStringList(input.SubDeptKeywords)
	input.SubLate22Names = normalizeStringList(input.SubLate22Names)
	input.PartSpecialNames = normalizeStringList(input.PartSpecialNames)
	input.OvertimeRulesJSON = strings.TrimSpace(input.OvertimeRulesJSON)

	seen := make(map[string]struct{}, len(input.MaternityLeaveOverrides))
	cleaned := make([]MaternityLeaveOverride, 0, len(input.MaternityLeaveOverrides))
	for _, item := range input.MaternityLeaveOverrides {
		item.EmployeeNo = strings.TrimSpace(item.EmployeeNo)
		item.Name = strings.TrimSpace(item.Name)
		item.StartDate = strings.TrimSpace(item.StartDate)
		item.EndDate = strings.TrimSpace(item.EndDate)
		if item.Name == "" && item.EmployeeNo == "" && item.StartDate == "" && item.EndDate == "" {
			continue
		}
		if item.Name == "" || item.StartDate == "" || item.EndDate == "" {
			return AttendanceToolboxConfig{}, errors.New("长期产假人员的姓名、开始日期和结束日期必须填写完整")
		}
		key := strings.Join([]string{item.EmployeeNo, item.Name, item.StartDate, item.EndDate}, "\x00")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, item)
	}
	input.MaternityLeaveOverrides = cleaned
	return input, nil
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
