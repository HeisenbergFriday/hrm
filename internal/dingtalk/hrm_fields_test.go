package dingtalk

import (
	"slices"
	"testing"

	"peopleops/internal/database"
)

func hrmTestField(code, name, value string) map[string]interface{} {
	return map[string]interface{}{
		"field_code": code,
		"field_name": name,
		"field_value_list": []interface{}{
			map[string]interface{}{"value": value},
		},
	}
}

func TestParseHRMEmployeeFieldsUsesConfiguredMappings(t *testing.T) {
	cfg := Config{
		HRMFieldCodes: map[string][]string{
			hrmFieldEmploymentType:   {"custom-employment-type"},
			hrmFieldJobLevel:         {"custom-job-level"},
			hrmFieldJobFamily:        {"custom-job-family"},
			hrmFieldProbationEndDate: {"custom-probation-end"},
			hrmFieldPosition:         {"custom-position"},
		},
	}
	fields := []interface{}{
		hrmTestField("sys01-planRegularTime", "计划转正日期", "2026-09-01"),
		hrmTestField("sys01-regularTime", "实际转正日期", ""),
		hrmTestField("custom-probation-end", "试用期结束日期", "2026-08-31"),
		hrmTestField("custom-employment-type", "员工类型", "正式"),
		hrmTestField("custom-job-level", "职级", "P6"),
		hrmTestField("custom-job-family", "岗位序列", "技术"),
		hrmTestField("custom-position", "岗位", "高级工程师"),
	}

	got := parseHRMEmployeeFields(fields, cfg)
	if got.Planned != "2026-09-01" || got.ProbationEndDate != "2026-08-31" {
		t.Fatalf("unexpected regularization dates: %#v", got)
	}
	if got.EmploymentType != "正式" || got.JobLevel != "P6" || got.JobFamily != "技术" {
		t.Fatalf("unexpected employee fields: %#v", got)
	}
	if got.Position != "高级工程师" || got.PositionSource != "custom-position" {
		t.Fatalf("unexpected position fields: %#v", got)
	}
}

func TestConfigFromOrganizationUsesTenantHRMFieldsWithoutEnvLeak(t *testing.T) {
	t.Setenv("DINGTALK_HRM_JOB_LEVEL_FIELD_CODES", "default-job-level")

	orgCfg := ConfigFromOrganization(database.Organization{
		OrgID: "org-a",
		Extension: map[string]interface{}{
			"dingtalk_hrm_field_codes": map[string]interface{}{
				"job_level":  []interface{}{"org-a-job-level"},
				"job_family": "org-a-job-family",
			},
			"dingtalk_hrm_field_options": map[string]interface{}{
				"employment_type": map[string]interface{}{"1": "组织 A 正式员工"},
			},
		},
	}).normalized()
	if !slices.Equal(orgCfg.HRMFieldCodes[hrmFieldJobLevel], []string{"org-a-job-level"}) {
		t.Fatalf("job level codes = %#v", orgCfg.HRMFieldCodes[hrmFieldJobLevel])
	}
	if !slices.Equal(orgCfg.HRMFieldCodes[hrmFieldJobFamily], []string{"org-a-job-family"}) {
		t.Fatalf("job family codes = %#v", orgCfg.HRMFieldCodes[hrmFieldJobFamily])
	}
	if slices.Contains(configuredHRMFieldCodes(orgCfg), "default-job-level") {
		t.Fatal("non-default organization must not inherit default HRM field codes")
	}
	if got := orgCfg.HRMFieldOptions[hrmFieldEmploymentType]["1"]; got != "组织 A 正式员工" {
		t.Fatalf("employment type option = %q", got)
	}
}

func TestDefaultOrganizationMergesHRMFieldCodesFromEnv(t *testing.T) {
	t.Setenv("DINGTALK_HRM_EMPLOYMENT_TYPE_FIELD_CODES", "employment-a,employment-b")
	cfg := ConfigFromOrganization(database.Organization{OrgID: database.DefaultOrganizationID}).normalized()

	codes := configuredHRMFieldCodes(cfg)
	for _, want := range []string{"sys01-planRegularTime", "sys01-regularTime", "employment-a", "employment-b"} {
		if !slices.Contains(codes, want) {
			t.Fatalf("configured HRM codes %#v missing %q", codes, want)
		}
	}

	appCfg := configFromAppConfig(AppConfig{OrgID: database.DefaultOrganizationID})
	if !slices.Contains(configuredHRMFieldCodes(appCfg), "employment-a") {
		t.Fatal("default AppConfig path must retain environment HRM field mappings")
	}
}

// TestConfiguredHRMFieldCodesIncludesStandardSystemCodes verifies that the
// standard DingTalk HRM field codes (sys01-employeeType, sys01-positionLevel,
// sys00-position) are always included in the field_filter_list sent to the API,
// even when no environment variables or organization extensions are configured.
func TestConfiguredHRMFieldCodesIncludesStandardSystemCodes(t *testing.T) {
	cfg := Config{}.normalized()
	codes := configuredHRMFieldCodes(cfg)

	expectedDefaults := []string{
		"sys01-planRegularTime",
		"sys01-regularTime",
		"sys01-employeeType",
		"sys01-positionLevel",
		"sys00-position",
	}
	for _, want := range expectedDefaults {
		if !slices.Contains(codes, want) {
			t.Fatalf("configured HRM codes %#v missing standard code %q", codes, want)
		}
	}
}

// TestParseHRMEmployeeFieldsWithStandardFieldCodes verifies that the standard
// DingTalk HRM system field codes are correctly matched and parsed without
// any custom configuration.
func TestParseHRMEmployeeFieldsWithStandardFieldCodes(t *testing.T) {
	cfg := Config{}.normalized()
	fields := []interface{}{
		hrmTestField("sys01-planRegularTime", "计划转正日期", "2026-09-01"),
		hrmTestField("sys01-regularTime", "实际转正日期", "2026-09-05"),
		hrmTestField("sys01-employeeType", "员工类型", "正式"),
		hrmTestField("sys01-positionLevel", "岗位职级", "P6"),
		hrmTestField("sys00-position", "职位", "高级工程师"),
	}

	got := parseHRMEmployeeFields(fields, cfg)
	if got.Planned != "2026-09-01" || got.Actual != "2026-09-05" {
		t.Fatalf("unexpected regularization dates: %#v", got)
	}
	if got.EmploymentType != "正式" {
		t.Fatalf("employment type = %q, want %q", got.EmploymentType, "正式")
	}
	if got.JobLevel != "P6" {
		t.Fatalf("job level = %q, want %q", got.JobLevel, "P6")
	}
	if got.Position != "高级工程师" || got.PositionSource != "sys00-position" {
		t.Fatalf("unexpected position: %#v", got)
	}
}

// TestParseHRMEmployeeFieldsWithChineseFieldNamesOnly verifies that fields are
// matched by Chinese field_name even when field_code does not match any
// configured or default code.
func TestParseHRMEmployeeFieldsWithChineseFieldNamesOnly(t *testing.T) {
	cfg := Config{}.normalized()
	fields := []interface{}{
		hrmTestField("sys01-employeeType", "员工类型", "正式"),
		hrmTestField("sys01-positionLevel", "职级", "P7"),
		hrmTestField("custom-xxx", "岗位序列", "技术"),
	}

	got := parseHRMEmployeeFields(fields, cfg)
	if got.EmploymentType != "正式" {
		t.Fatalf("employment type = %q, want %q", got.EmploymentType, "正式")
	}
	if got.JobLevel != "P7" {
		t.Fatalf("job level = %q, want %q", got.JobLevel, "P7")
	}
	if got.JobFamily != "技术" {
		t.Fatalf("job family = %q, want %q", got.JobFamily, "技术")
	}
}

// TestParseHRMEmployeeFieldsEmptyValuesDoNotOverwrite verifies that empty
// field values do not overwrite previously parsed non-empty values.
func TestParseHRMEmployeeFieldsEmptyValuesDoNotOverwrite(t *testing.T) {
	cfg := Config{}.normalized()
	fields := []interface{}{
		hrmTestField("sys01-employeeType", "员工类型", "正式"),
		hrmTestField("sys01-employeeType", "员工类型", ""),
		hrmTestField("sys01-positionLevel", "岗位职级", "P6"),
		hrmTestField("sys01-positionLevel", "岗位职级", "  "),
	}

	got := parseHRMEmployeeFields(fields, cfg)
	if got.EmploymentType != "正式" {
		t.Fatalf("empty value overwrote non-empty: employment type = %q", got.EmploymentType)
	}
	if got.JobLevel != "P6" {
		t.Fatalf("empty value overwrote non-empty: job level = %q", got.JobLevel)
	}
}

// TestHasAnyHRMTargetField verifies the diagnostic helper that distinguishes
// "API succeeded with target fields" from "API succeeded but no target fields".
func TestHasAnyHRMTargetField(t *testing.T) {
	usersWithFields := map[string]UserInfo{
		"user-1": {EmploymentType: "", JobLevel: "", JobFamily: ""},
		"user-2": {EmploymentType: "正式", JobLevel: "", JobFamily: ""},
	}
	if !hasAnyHRMTargetField(usersWithFields) {
		t.Fatal("expected true when at least one user has a target field")
	}

	usersAllEmpty := map[string]UserInfo{
		"user-1": {EmploymentType: "", JobLevel: "", JobFamily: ""},
		"user-2": {EmploymentType: "  ", JobLevel: "", JobFamily: ""},
	}
	if hasAnyHRMTargetField(usersAllEmpty) {
		t.Fatal("expected false when all users have empty target fields")
	}

	if hasAnyHRMTargetField(map[string]UserInfo{}) {
		t.Fatal("expected false for empty user map")
	}
}

func TestResolveHRMEmploymentTypeUsesReliableSources(t *testing.T) {
	tests := []struct {
		name      string
		field     map[string]interface{}
		cfg       Config
		wantCode  string
		wantLabel string
	}{
		{
			name: "response label wins",
			field: map[string]interface{}{
				"field_code":       "sys01-employeeType",
				"field_value_list": []interface{}{map[string]interface{}{"value": "1", "label": "正式员工"}},
			},
			cfg:       Config{HRMFieldOptions: map[string]map[string]string{hrmFieldEmploymentType: {"1": "配置名称"}}},
			wantCode:  "1",
			wantLabel: "正式员工",
		},
		{
			name: "value item field name",
			field: map[string]interface{}{
				"field_code":       "sys01-employeeType",
				"field_value_list": []interface{}{map[string]interface{}{"value": "A", "field_name": "顾问"}},
			},
			wantCode:  "A",
			wantLabel: "顾问",
		},
		{
			name: "response metadata",
			field: map[string]interface{}{
				"field_code":       "sys01-employeeType",
				"field_value_list": []interface{}{map[string]interface{}{"value": "B"}},
				"options":          []interface{}{map[string]interface{}{"value": "B", "name": "实习生"}},
			},
			wantCode:  "B",
			wantLabel: "实习生",
		},
		{
			name: "response option object metadata",
			field: map[string]interface{}{
				"field_code":       "sys01-employeeType",
				"field_value_list": []interface{}{map[string]interface{}{"value": "1"}},
				"options": map[string]interface{}{
					"1": map[string]interface{}{"label": "对象元数据名称"},
				},
			},
			wantCode:  "1",
			wantLabel: "对象元数据名称",
		},
		{
			name:  "organization mapping",
			field: hrmTestField("sys01-employeeType", "员工类型", "1"),
			cfg: Config{HRMFieldOptions: map[string]map[string]string{
				hrmFieldEmploymentType: {"1": "组织配置名称"},
			}},
			wantCode:  "1",
			wantLabel: "组织配置名称",
		},
		{
			name:      "unknown code is explicit",
			field:     hrmTestField("sys01-employeeType", "员工类型", "99"),
			wantCode:  "99",
			wantLabel: "未知类型（代码：99）",
		},
		{
			name: "numeric zero remains code",
			field: map[string]interface{}{
				"field_code":       "sys01-employeeType",
				"field_value_list": []interface{}{map[string]interface{}{"value": float64(0)}},
			},
			wantCode:  "0",
			wantLabel: "未知类型（代码：0）",
		},
		{
			name:      "string zero remains code",
			field:     hrmTestField("sys01-employeeType", "员工类型", "0"),
			wantCode:  "0",
			wantLabel: "未知类型（代码：0）",
		},
		{
			name:      "empty remains empty",
			field:     hrmTestField("sys01-employeeType", "员工类型", ""),
			wantCode:  "",
			wantLabel: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHRMEmployeeFields([]interface{}{tt.field}, tt.cfg)
			if got.EmploymentTypeCode != tt.wantCode || got.EmploymentType != tt.wantLabel {
				t.Fatalf("employment type = (%q, %q), want (%q, %q)", got.EmploymentTypeCode, got.EmploymentType, tt.wantCode, tt.wantLabel)
			}
		})
	}
}
