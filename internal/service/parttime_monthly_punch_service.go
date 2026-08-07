package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
)

// attendanceCST is the fixed timezone used by DingTalk attendance records.
var attendanceCST = time.FixedZone("CST", 8*3600)

// ParttimeMonthlyPunchService orchestrates fetching the DingTalk "月度打卡记录"
// for one organisation and one calendar month, matching it against the org's
// part-time roster, and rendering an Excel compatible with the part-time
// summary module.
type ParttimeMonthlyPunchService struct {
	service    *AttendanceToolboxService
	dataSource ParttimePunchDataSource
}

// ParttimePunchDataSource abstracts the external lookups (DingTalk config,
// admin user, org roster, attendance records) so the service can be unit-tested
// without a live database or DingTalk tenant.
type ParttimePunchDataSource interface {
	Config(orgID string) (dingtalk.Config, error)
	AdminUserID(orgID string) (string, error)
	Roster(orgID string) ([]ParttimeEmployee, error)
	Attendance(orgID string, userIDs []string, startDate, endDate string) ([]dingtalk.AttendanceRecord, error)
}

// liveParttimePunchDataSource is the production implementation backed by the
// dingtalk package and the org database.
type liveParttimePunchDataSource struct{}

func (liveParttimePunchDataSource) Config(orgID string) (dingtalk.Config, error) {
	return dingtalk.ConfigForOrgID(orgID)
}

func (liveParttimePunchDataSource) AdminUserID(orgID string) (string, error) {
	return dingtalk.ResolveAdminUserID(orgID)
}

func (liveParttimePunchDataSource) Roster(orgID string) ([]ParttimeEmployee, error) {
	return fetchOrgRoster(orgID), nil
}

func (liveParttimePunchDataSource) Attendance(orgID string, userIDs []string, startDate, endDate string) ([]dingtalk.AttendanceRecord, error) {
	return dingtalk.GetAttendanceForOrg(orgID, userIDs, startDate, endDate)
}

// NewParttimeMonthlyPunchService builds a service backed by the shared
// attendance toolbox engine (Python subprocess) configuration and the live
// DingTalk/DB data source.
func NewParttimeMonthlyPunchService() *ParttimeMonthlyPunchService {
	return &ParttimeMonthlyPunchService{
		service:    NewAttendanceToolboxService(),
		dataSource: liveParttimePunchDataSource{},
	}
}

// NewParttimeMonthlyPunchServiceWithDataSource builds a service with an
// injected data source, used by tests to supply fake config/roster/attendance.
func NewParttimeMonthlyPunchServiceWithDataSource(ds ParttimePunchDataSource) *ParttimeMonthlyPunchService {
	return &ParttimeMonthlyPunchService{
		service:    NewAttendanceToolboxService(),
		dataSource: ds,
	}
}

// ParttimeMonthlyPunchRequest is the HTTP-level request body.
type ParttimeMonthlyPunchRequest struct {
	Month string `json:"month"` // "YYYY-MM"
}

// RenderParttimeMonthlyPunch validates org context + configuration, fetches the
// org roster and the DingTalk monthly punch records, matches them, and returns
// the rendered Excel bytes plus a human-readable audit summary.
func (s *ParttimeMonthlyPunchService) RenderParttimeMonthlyPunch(
	ctx context.Context,
	orgID string,
	req ParttimeMonthlyPunchRequest,
) ([]byte, string, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, "", errors.New("缺少组织上下文，请重新登录")
	}

	month := strings.TrimSpace(req.Month)
	year, monthNum, err := parseYearMonth(month)
	if err != nil {
		return nil, "", err
	}

	cfg, err := resolveDingtalkConfigWith(s.dataSource, orgID)
	if err != nil {
		return nil, "", err
	}

	adminID, err := s.dataSource.AdminUserID(orgID)
	if err != nil || strings.TrimSpace(adminID) == "" {
		return nil, "", fmt.Errorf("未配置钉钉管理员 UserId，请先在组织设置中配置钉钉管理员后重试: %w", err)
	}

	roster, err := s.dataSource.Roster(orgID)
	if err != nil {
		return nil, "", err
	}
	userIDs := make([]string, 0, len(roster))
	identityByUserID := make(map[string]ParttimeEmployee, len(roster))
	for _, emp := range roster {
		if id := strings.TrimSpace(emp.UserID); id != "" {
			userIDs = append(userIDs, id)
			identityByUserID[id] = emp
		}
	}

	startDate := fmt.Sprintf("%04d-%02d-01", year, monthNum)
	endDate := lastDayOfMonth(year, monthNum)

	var records []dingtalk.AttendanceRecord
	if len(userIDs) > 0 {
		records, err = s.dataSource.Attendance(orgID, userIDs, startDate, endDate)
		if err != nil {
			return nil, "", fmt.Errorf("从钉钉拉取月度打卡记录失败：%w", err)
		}
	}

	punched := buildEmployeePunchData(records, identityByUserID)
	match := MatchParttimeMonthlyPunch(roster, punched)

	excel, err := s.renderExcel(ctx, match, year, monthNum, cfg, adminID)
	if err != nil {
		return nil, "", err
	}

	audit := buildAudit(match, year, monthNum)
	return excel, audit, nil
}

// resolveDingtalkConfigWith resolves config through a data source (live or fake).
func resolveDingtalkConfigWith(ds ParttimePunchDataSource, orgID string) (dingtalk.Config, error) {
	cfg, err := ds.Config(orgID)
	if err != nil {
		return dingtalk.Config{}, fmt.Errorf("未找到组织（%s）的钉钉配置，请先在组织设置中完成钉钉对接: %w", orgID, err)
	}
	return validateDingtalkConfig(orgID, cfg)
}

func validateDingtalkConfig(orgID string, cfg dingtalk.Config) (dingtalk.Config, error) {
	cfg = cfg.NormalizedForAPI()
	if strings.TrimSpace(cfg.AppKey) == "" || strings.TrimSpace(cfg.AppSecret) == "" {
		return dingtalk.Config{}, fmt.Errorf("组织（%s）的钉钉 AppKey/AppSecret 未配置完整，请检查组织设置", orgID)
	}
	return cfg, nil
}

// fetchOrgRoster loads the organisation's part-time roster: every active user
// paired with their employee number (工号) from the employee profile. Users
// without a profile or employee number are still included (with an empty
// EmployeeNo) so the matching step can fall back to name.
func fetchOrgRoster(orgID string) []ParttimeEmployee {
	db := database.DB
	if db == nil {
		return nil
	}
	repo := repository.NewUserRepositoryWithOrgID(db, orgID)
	profiles := repository.NewEmployeeRepositoryWithOrgID(db, orgID)
	depts := repository.NewDepartmentRepositoryWithOrgID(db, orgID)

	const pageSize = 500
	users, _, err := repo.FindAll(1, pageSize)
	if err != nil {
		return nil
	}

	roster := make([]ParttimeEmployee, 0, len(users))
	for _, u := range users {
		if strings.TrimSpace(u.DingTalkUserID) == "" {
			continue
		}
		emp := ParttimeEmployee{
			Name:     strings.TrimSpace(u.Name),
			UserID:   strings.TrimSpace(u.DingTalkUserID),
			Position: strings.TrimSpace(u.Position),
		}
		if profile, perr := profiles.FindProfileByUserID(u.UserID); perr == nil && profile != nil {
			emp.EmployeeNo = strings.TrimSpace(profile.EmployeeID)
		}
		if dept, derr := depts.FindByDepartmentID(u.DepartmentID); derr == nil && dept != nil {
			emp.Department = strings.TrimSpace(dept.Name)
		}
		roster = append(roster, emp)
	}
	return roster
}

// buildEmployeePunchData aggregates raw DingTalk check records into per-employee
// daily status labels. Each aggregated user is enriched with their employee
// number (工号) and name from identityByUserID so the matching step can pair
// report rows against the org roster by 工号/姓名.
func buildEmployeePunchData(records []dingtalk.AttendanceRecord, identityByUserID map[string]ParttimeEmployee) []EmployeePunchData {
	type dayAgg struct {
		onDutyTime  string
		offDutyTime string
		timeResult  string
	}
	type userAgg struct {
		days map[int]*dayAgg
	}

	byUser := map[string]*userAgg{}
	userOrder := make([]string, 0, len(records))
	seen := map[string]bool{}
	ensure := func(userID string) *userAgg {
		a, ok := byUser[userID]
		if !ok {
			a = &userAgg{days: map[int]*dayAgg{}}
			byUser[userID] = a
		}
		if !seen[userID] {
			seen[userID] = true
			userOrder = append(userOrder, userID)
		}
		return a
	}

	for _, r := range records {
		userID := strings.TrimSpace(r.UserID)
		if userID == "" {
			continue
		}
		day := dayOf(r.WorkDate)
		if day <= 0 || day > 31 {
			continue
		}
		agg := ensure(userID)
		da, ok := agg.days[day]
		if !ok {
			da = &dayAgg{}
			agg.days[day] = da
		}
		switch r.CheckType {
		case "OnDuty":
			if da.onDutyTime == "" {
				da.onDutyTime = hhmm(r.UserCheckTime, attendanceCST)
			}
		case "OffDuty":
			if da.offDutyTime == "" {
				da.offDutyTime = hhmm(r.UserCheckTime, attendanceCST)
			}
		}
		if r.TimeResult != "" {
			da.timeResult = r.TimeResult
		}
	}

	punched := make([]EmployeePunchData, 0, len(byUser))
	for _, userID := range userOrder {
		agg := byUser[userID]
		identity := identityByUserID[userID]
		days := make(map[int]string, len(agg.days))
		for day, da := range agg.days {
			if label := statusLabel(da.onDutyTime, da.offDutyTime, da.timeResult); label != "" {
				days[day] = label
			}
		}
		punched = append(punched, EmployeePunchData{
			EmployeeNo: identity.EmployeeNo,
			Name:       identity.Name,
			Position:   identity.Position,
			Department: identity.Department,
			Days:       days,
		})
	}
	return punched
}

// renderExcel invokes the Python engine to render the part-time attendance
// detail grid plus an audit sheet.
func (s *ParttimeMonthlyPunchService) renderExcel(
	ctx context.Context,
	match ParttimeMonthlyMatch,
	year, month int,
	cfg dingtalk.Config,
	adminID string,
) ([]byte, error) {
	daysInMonth := daysInMonth(year, month)
	config := map[string]interface{}{
		"year":            year,
		"month":           month,
		"days_in_month":   daysInMonth,
		"matched":         serializeMatched(match.Matched),
		"unmatched":       serializeUnmatched(match.Unmatched),
		"anomalies":       match.Anomalies,
		"matched_count":   len(match.Matched),
		"unmatched_count": len(match.Unmatched),
		"roster_count":    len(match.Matched) + len(match.Unmatched),
		"client_id":       cfg.AppKey,
		"client_secret":   cfg.AppSecret,
		"admin_user_id":   adminID,
	}

	result, err := s.service.runAction(ctx, "parttime-monthly-punch", config)
	if err != nil {
		return nil, err
	}
	for _, out := range result.Outputs {
		if strings.EqualFold(out.ContentType, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet") ||
			strings.HasSuffix(out.FileName, ".xlsx") {
			return out.Data, nil
		}
	}
	if len(result.Outputs) > 0 {
		return result.Outputs[0].Data, nil
	}
	return nil, errors.New("月度打卡记录渲染完成，但未生成 Excel 文件")
}

type matchedJSON struct {
	EmployeeNo string         `json:"employee_no"`
	Name       string         `json:"name"`
	Position   string         `json:"position"`
	Department string         `json:"department"`
	MatchedBy  string         `json:"matched_by"`
	Days       map[int]string `json:"days"`
}

func serializeMatched(list []MatchedParttimeEmployee) []matchedJSON {
	out := make([]matchedJSON, 0, len(list))
	for _, m := range list {
		out = append(out, matchedJSON{
			EmployeeNo: m.EmployeeNo,
			Name:       m.Name,
			Position:   m.Position,
			Department: m.Department,
			MatchedBy:  m.MatchedBy,
			Days:       m.Days,
		})
	}
	return out
}

type unmatchedJSON struct {
	EmployeeNo string `json:"employee_no"`
	Name       string `json:"name"`
	Position   string `json:"position"`
	Department string `json:"department"`
}

func serializeUnmatched(list []ParttimeEmployee) []unmatchedJSON {
	out := make([]unmatchedJSON, 0, len(list))
	for _, e := range list {
		out = append(out, unmatchedJSON{
			EmployeeNo: e.EmployeeNo,
			Name:       e.Name,
			Position:   e.Position,
			Department: e.Department,
		})
	}
	return out
}

func buildAudit(match ParttimeMonthlyMatch, year int, month int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "兼职月度打卡记录 %04d-%02d：共 %d 人，已匹配 %d 人",
		year, month, len(match.Matched)+len(match.Unmatched), len(match.Matched))
	if len(match.Unmatched) > 0 {
		fmt.Fprintf(&b, "，未匹配到打卡记录 %d 人", len(match.Unmatched))
	}
	if len(match.Anomalies) > 0 {
		fmt.Fprintf(&b, "，异常提示 %d 项", len(match.Anomalies))
	}
	b.WriteString("。")
	return b.String()
}

func parseYearMonth(s string) (int, int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, errors.New("请提供月份参数，格式为 YYYY-MM")
	}
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("月份格式不正确（%s），应为 YYYY-MM", s)
	}
	year, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("月份年份无效（%s）: %w", parts[0], err)
	}
	month, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || month < 1 || month > 12 {
		return 0, 0, fmt.Errorf("月份数字无效（%s），应为 1-12", parts[1])
	}
	return year, month, nil
}

func daysInMonth(year, month int) int {
	if month < 1 || month > 12 {
		return 30
	}
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	return first.AddDate(0, 1, -1).Day()
}

func lastDayOfMonth(year, month int) string {
	return fmt.Sprintf("%04d-%02d-%02d", year, month, daysInMonth(year, month))
}
