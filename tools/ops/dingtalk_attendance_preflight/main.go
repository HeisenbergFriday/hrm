package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"peopleops/internal/config"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
)

const (
	statusPass    = "PASS"
	statusFail    = "FAIL"
	statusSkipped = "SKIPPED"
	statusManual  = "MANUAL"
)

type checkResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Scope      string `json:"scope,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	Detail     string `json:"detail"`
	NextAction string `json:"next_action,omitempty"`
}

func main() {
	if err := config.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "load config warning: %v\n", err)
	}

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	defaultOperator := strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID"))
	defaultUser := firstNonEmpty(
		os.Getenv("DINGTALK_PREFLIGHT_USER_ID"),
		defaultOperator,
	)
	defaultProcessCode := firstNonEmpty(
		os.Getenv("DINGTALK_ATTENDANCE_APPROVAL_PROCESS_CODE"),
		os.Getenv("DINGTALK_OVERTIME_PROCESS_CODE"),
	)

	orgID := flag.String("org", database.DefaultOrganizationID, "organization ID; default uses the environment DingTalk app config")
	userID := flag.String("user", defaultUser, "explicit DingTalk user_id used for the read-only attendance query")
	operatorID := flag.String("operator", defaultOperator, "DingTalk administrator/operator user_id used for vacation type queries and process list")
	processCode := flag.String("process-code", defaultProcessCode, "DingTalk approval process code used for the read-only approval query")
	startDate := flag.String("start", yesterday, "query start date, YYYY-MM-DD")
	endDate := flag.String("end", today, "query end date, YYYY-MM-DD")
	jsonOutput := flag.Bool("json", false, "print results as JSON")
	listProcesses := flag.Bool("list-processes", false, "list manageable approval process templates (read-only)")
	targetProcessCode := flag.String("target-process-code", "", "exact process_code to match in -list-processes output")
	flag.Parse()

	results := make([]checkResult, 0, 16)
	if err := dingtalk.Init(); err != nil {
		results = append(results, checkResult{
			Name:       "应用配置与访问令牌",
			Status:     statusFail,
			Detail:     "钉钉应用配置不可用：" + err.Error(),
			NextAction: "配置 DINGTALK_APP_KEY、DINGTALK_APP_SECRET、DINGTALK_CORP_ID；消息通知还需要 DINGTALK_AGENT_ID。",
		})
		printResults(results, *jsonOutput)
		return
	}

	cfg := dingtalk.DefaultConfig().NormalizedForAPI()
	configDetail := fmt.Sprintf(
		"AppKey/AppSecret 已配置；CorpID=%s；AgentID=%s",
		configuredLabel(cfg.CorpID),
		configuredLabel(cfg.AgentID),
	)
	results = append(results, checkResult{
		Name:       "应用基础配置",
		Status:     statusPass,
		Detail:     configDetail,
		NextAction: "不要在工单、截图或日志中提供 AppSecret/access token。",
	})

	if _, err := dingtalk.GetAccessTokenForOrg(strings.TrimSpace(*orgID)); err != nil {
		results = append(results, checkResult{
			Name:       "获取应用访问令牌",
			Status:     statusFail,
			Endpoint:   "/gettoken 或 /v1.0/oauth2/accessToken",
			Detail:     err.Error(),
			NextAction: "检查应用 Client ID、Client Secret、企业归属及应用是否已发布。",
		})
		if !*listProcesses {
			appendManualChecks(&results)
		}
		printResults(results, *jsonOutput)
		return
	}
	results = append(results, checkResult{
		Name:     "获取应用访问令牌",
		Status:   statusPass,
		Endpoint: "/gettoken 或 /v1.0/oauth2/accessToken",
		Detail:   "访问令牌获取成功，未输出令牌内容。",
	})

	if *listProcesses {
		runListProcesses(strings.TrimSpace(*orgID), strings.TrimSpace(*operatorID), strings.TrimSpace(*targetProcessCode), *jsonOutput, results)
		return
	}

	results = append(results, checkAttendanceGroups(strings.TrimSpace(*orgID)))
	results = append(results, checkShifts(strings.TrimSpace(*orgID)))
	results = append(results, checkAttendance(strings.TrimSpace(*orgID), strings.TrimSpace(*userID), *startDate, *endDate))
	results = append(results, checkApprovals(strings.TrimSpace(*orgID), strings.TrimSpace(*processCode), *startDate, *endDate))
	results = append(results, checkVacationTypes(strings.TrimSpace(*orgID), strings.TrimSpace(*operatorID)))
	appendManualChecks(&results)
	printResults(results, *jsonOutput)
}

func runListProcesses(orgID, operatorID, targetCode string, asJSON bool, base []checkResult) {
	if operatorID == "" {
		base = append(base, checkResult{
			Name:       "列出可管理审批模板",
			Status:     statusFail,
			Scope:      "Workflow.Form.Read",
			Endpoint:   "/topapi/process/listbyuserid",
			Detail:     "缺少管理员 user_id。",
			NextAction: "配置 DINGTALK_ADMIN_USER_ID 或使用 -operator。",
		})
		printResults(base, asJSON)
		return
	}

	templates, err := dingtalk.ListManageableApprovalProcessesForOrg(orgID, operatorID)
	if err != nil {
		base = append(base, checkResult{
			Name:       "列出可管理审批模板",
			Status:     statusFail,
			Scope:      "Workflow.Form.Read",
			Endpoint:   "/topapi/process/listbyuserid",
			Detail:     err.Error(),
			NextAction: "确认已申请 Workflow.Form.Read，且 operator 具备审批管理员可见范围。",
		})
		printResults(base, asJSON)
		return
	}

	base = append(base, checkResult{
		Name:     "列出可管理审批模板",
		Status:   statusPass,
		Scope:    "Workflow.Form.Read",
		Endpoint: "/topapi/process/listbyuserid",
		Detail:   fmt.Sprintf("读取成功，共 %d 个模板。", len(templates)),
	})

	match := findExactProcessTemplate(templates, targetCode)
	if targetCode != "" {
		if match != nil {
			base = append(base, checkResult{
				Name:     "目标 process_code 匹配",
				Status:   statusPass,
				Endpoint: "/topapi/process/listbyuserid",
				Detail: fmt.Sprintf(
					"精确匹配：name=%s process_code=%s 推测业务类型=%s",
					match.Name,
					match.ProcessCode,
					guessProcessBusinessType(match.Name),
				),
			})
		} else {
			base = append(base, checkResult{
				Name:     "目标 process_code 匹配",
				Status:   statusFail,
				Endpoint: "/topapi/process/listbyuserid",
				Detail: fmt.Sprintf(
					"当前管理员可管理模板中未找到精确匹配 process_code=%s（不能据此直接判定无效，可能是可见范围问题）。",
					targetCode,
				),
				NextAction: "将继续执行 schema 二次验证；若仍失败，请在钉钉 OA 模板管理页核对真实 process_code。",
			})
			// Secondary readonly validation: schema by processCode.
			schema, schemaErr := dingtalk.GetApprovalProcessSchemaForOrg(orgID, targetCode)
			if schemaErr != nil {
				base = append(base, checkResult{
					Name:       "process_code schema 二次验证",
					Status:     statusFail,
					Scope:      "Workflow.Form.Read",
					Endpoint:   "GET /v1.0/workflow/forms/schemas/processCodes",
					Detail:     schemaErr.Error(),
					NextAction: "确认 Workflow.Form.Read；该值可能是 formUuid/实例ID/错误复制，而不是 process_code。不要创建模板。",
				})
			} else {
				base = append(base, checkResult{
					Name:     "process_code schema 二次验证",
					Status:   statusPass,
					Scope:    "Workflow.Form.Read",
					Endpoint: "GET /v1.0/workflow/forms/schemas/processCodes",
					Detail: fmt.Sprintf(
						"schema 可访问：name=%s form_uuid=%s status=%s hint=%s",
						emptyDash(schema.Name),
						emptyDash(schema.FormUUID),
						emptyDash(schema.Status),
						emptyDash(schema.RawHint),
					),
					NextAction: "列表未命中但 schema 可访问，说明 code 可能有效但不在当前管理员可管理列表中。",
				})
			}
			// Heuristic identity type notes (readonly).
			base = append(base, checkResult{
				Name:   "目标值形态判断",
				Status: statusManual,
				Detail: describeCodeShape(targetCode),
			})
		}
	}

	printResults(base, asJSON)
	if !asJSON {
		printProcessTable(templates, targetCode)
		printProcessSummary(templates, targetCode, match)
	}
}

func printProcessTable(templates []dingtalk.ApprovalProcessTemplate, targetCode string) {
	fmt.Println()
	fmt.Println("审批模板列表（只读）:")
	fmt.Printf("%-4s %-28s %-40s %-8s %-10s\n", "#", "模板名称", "process_code", "匹配", "推测类型")
	fmt.Println(strings.Repeat("-", 100))
	for i, tpl := range templates {
		matched := "否"
		if targetCode != "" && tpl.ProcessCode == targetCode {
			matched = "是"
		}
		fmt.Printf(
			"%-4d %-28s %-40s %-8s %-10s\n",
			i+1,
			truncateRunes(tpl.Name, 28),
			truncateRunes(tpl.ProcessCode, 40),
			matched,
			guessProcessBusinessType(tpl.Name),
		)
	}
	if len(templates) == 0 {
		fmt.Println("(空)")
	}
}

func printProcessSummary(templates []dingtalk.ApprovalProcessTemplate, targetCode string, match *dingtalk.ApprovalProcessTemplate) {
	fmt.Println()
	fmt.Printf("模板总数: %d\n", len(templates))
	if targetCode == "" {
		fmt.Println("目标 process_code: （未指定 -target-process-code）")
		fmt.Println("下一步: 从上方列表选择请假/加班/补卡模板的 process_code，用于 Stream/审批同步。")
		return
	}
	fmt.Printf("目标 process_code: %s\n", targetCode)
	if match != nil {
		fmt.Printf("匹配结果: 命中\n")
		fmt.Printf("匹配模板名称: %s\n", match.Name)
		fmt.Printf("推测业务类型: %s\n", guessProcessBusinessType(match.Name))
		fmt.Printf("下一步应使用 process_code: %s\n", match.ProcessCode)
		return
	}
	fmt.Println("匹配结果: 未在可管理列表中精确命中")
	// Suggest candidates by keyword for next step.
	suggestions := suggestAttendanceProcessCodes(templates)
	if len(suggestions) == 0 {
		fmt.Println("下一步: 无法从名称明确推断请假/加班/补卡模板；请到钉钉 OA 审批模板管理页复制真实 process_code。")
		return
	}
	fmt.Println("名称可识别的候选 process_code（仅供人工确认）:")
	for _, s := range suggestions {
		fmt.Printf("  - [%s] %s => %s\n", s.Type, s.Name, s.ProcessCode)
	}
	fmt.Println("下一步: 人工确认业务类型后，使用对应 process_code；不要盲目使用目标值。")
}

type processSuggestion struct {
	Type        string
	Name        string
	ProcessCode string
}

func suggestAttendanceProcessCodes(templates []dingtalk.ApprovalProcessTemplate) []processSuggestion {
	out := make([]processSuggestion, 0)
	for _, tpl := range templates {
		typ := guessProcessBusinessType(tpl.Name)
		switch typ {
		case "请假", "加班", "补卡", "出差", "外出":
			out = append(out, processSuggestion{Type: typ, Name: tpl.Name, ProcessCode: tpl.ProcessCode})
		}
	}
	return out
}

func findExactProcessTemplate(templates []dingtalk.ApprovalProcessTemplate, target string) *dingtalk.ApprovalProcessTemplate {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	for i := range templates {
		if templates[i].ProcessCode == target {
			return &templates[i]
		}
	}
	return nil
}

// guessProcessBusinessType only uses explicit template name keywords.
// Uncertain names must return 待确认.
func guessProcessBusinessType(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return "待确认"
	}
	// Order matters: more specific first.
	switch {
	case strings.Contains(n, "补卡") || strings.Contains(n, "补打卡"):
		return "补卡"
	case strings.Contains(n, "加班"):
		return "加班"
	case strings.Contains(n, "请假") || strings.Contains(n, "休假") || strings.Contains(n, "年假") || strings.Contains(n, "调休"):
		return "请假"
	case strings.Contains(n, "出差"):
		return "出差"
	case strings.Contains(n, "外出"):
		return "外出"
	default:
		return "待确认"
	}
}

func describeCodeShape(code string) string {
	code = strings.TrimSpace(code)
	switch {
	case strings.HasPrefix(strings.ToUpper(code), "PROC-"):
		return "形态像标准 process_code（PROC- 前缀）。"
	case len(code) == 32 && isHex(code):
		return "形态像 32 位十六进制（可能是 formUuid/其他内部 ID，不一定是 process_code）。"
	case strings.Contains(code, "-") && len(code) > 20:
		return "形态像带连字符的长 ID；可能是实例 ID 或模板 UUID，需人工核对。"
	default:
		return "形态无法明确归类；请在钉钉后台模板详情中核对 process_code 字段。"
	}
}

func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return s != ""
}

func emptyDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func truncateRunes(s string, max int) string {
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	if max <= 1 {
		return string(rs[:max])
	}
	return string(rs[:max-1]) + "…"
}

func checkAttendanceGroups(orgID string) checkResult {
	const scope = "qyapi_attendance_group_read"
	groups, err := dingtalk.GetAttendanceGroupsForOrg(orgID)
	if err != nil {
		return failedPermissionCheck(
			"读取考勤组",
			scope,
			"/topapi/attendance/getsimplegroups",
			err,
			"申请“考勤组查询权限”。如需从新系统修改考勤组或排班，再申请 qyapi_attendance_group_manage。",
		)
	}
	return checkResult{
		Name:     "读取考勤组",
		Status:   statusPass,
		Scope:    scope,
		Endpoint: "/topapi/attendance/getsimplegroups",
		Detail:   fmt.Sprintf("读取成功，共 %d 个考勤组。", len(groups)),
	}
}

func checkShifts(orgID string) checkResult {
	const scope = "qyapi_attendance_group_read"
	shifts, err := dingtalk.GetShiftListForOrg(orgID)
	if err != nil {
		return failedPermissionCheck(
			"读取班次",
			scope,
			"/topapi/attendance/shift/list",
			err,
			"申请“考勤组查询权限”。如需创建班次和写入排班，再申请 qyapi_attendance_group_manage。",
		)
	}
	return checkResult{
		Name:     "读取班次",
		Status:   statusPass,
		Scope:    scope,
		Endpoint: "/topapi/attendance/shift/list",
		Detail:   fmt.Sprintf("读取成功，共 %d 个班次。", len(shifts)),
	}
}

func checkAttendance(orgID, userID, startDate, endDate string) checkResult {
	const scope = "qyapi_get_attendance_data"
	if userID == "" {
		return checkResult{
			Name:       "读取员工打卡记录",
			Status:     statusSkipped,
			Scope:      scope,
			Endpoint:   "/attendance/listRecord",
			Detail:     "未提供测试员工 user_id，未查询个人打卡记录。",
			NextAction: "使用 -user <测试员工user_id> 重跑；申请“考勤数据读权限”。",
		}
	}
	records, err := dingtalk.GetAttendanceForOrg(orgID, []string{userID}, startDate, endDate)
	if err != nil {
		return failedPermissionCheck(
			"读取员工打卡记录",
			scope,
			"/attendance/listRecord",
			err,
			"申请“考勤数据读权限”，并把测试员工加入应用可见范围。",
		)
	}
	return checkResult{
		Name:     "读取员工打卡记录",
		Status:   statusPass,
		Scope:    scope,
		Endpoint: "/attendance/listRecord",
		Detail:   fmt.Sprintf("读取成功，日期范围内返回 %d 条记录。", len(records)),
	}
}

func checkApprovals(orgID, processCode, startDate, endDate string) checkResult {
	const scope = "Workflow.Instance.Read"
	if processCode == "" {
		return checkResult{
			Name:       "读取考勤审批实例",
			Status:     statusSkipped,
			Scope:      scope,
			Endpoint:   "/topapi/processinstance/listids + /topapi/processinstance/get",
			Detail:     "未提供审批模板 process_code，未查询真实审批实例。",
			NextAction: "从钉钉 OA 审批模板管理页取得请假/加班/补卡模板 code，使用 -process-code 重跑；申请“工作流实例读权限”。",
		}
	}
	instances, err := dingtalk.GetApprovalsForOrg(orgID, processCode, startDate, endDate)
	if err != nil {
		return failedPermissionCheck(
			"读取考勤审批实例",
			scope,
			"/topapi/processinstance/listids + /topapi/processinstance/get",
			err,
			"申请“工作流实例读权限”，并确认应用可见范围覆盖审批发起人。",
		)
	}
	return checkResult{
		Name:     "读取考勤审批实例",
		Status:   statusPass,
		Scope:    scope,
		Endpoint: "/topapi/processinstance/listids + /topapi/processinstance/get",
		Detail:   fmt.Sprintf("读取成功，日期范围内返回 %d 个审批实例。", len(instances)),
	}
}

func checkVacationTypes(orgID, operatorID string) checkResult {
	const scope = "qyapi_holiday_readonly"
	if operatorID == "" {
		return checkResult{
			Name:       "读取假期类型",
			Status:     statusSkipped,
			Scope:      scope,
			Endpoint:   "/topapi/attendance/vacation/type/list",
			Detail:     "未提供 OA/考勤管理员 user_id，未查询假期类型。",
			NextAction: "配置 DINGTALK_ADMIN_USER_ID 或使用 -operator；申请“钉钉假期读权限”。",
		}
	}
	types, err := dingtalk.ListVacationTypesForOrg(orgID, operatorID)
	if err != nil {
		return failedPermissionCheck(
			"读取假期类型",
			scope,
			"/topapi/attendance/vacation/type/list",
			err,
			"申请“钉钉假期读权限”，并确认 operator 是 OA/考勤管理员。写余额还需 qyapi_holiday_manage。",
		)
	}
	return checkResult{
		Name:     "读取假期类型",
		Status:   statusPass,
		Scope:    scope,
		Endpoint: "/topapi/attendance/vacation/type/list",
		Detail:   fmt.Sprintf("读取成功，共 %d 个假期类型。", len(types)),
	}
}

func appendManualChecks(results *[]checkResult) {
	*results = append(*results,
		checkResult{
			Name:       "审批实时事件订阅",
			Status:     statusManual,
			Scope:      "Workflow.Instance.Read",
			Endpoint:   "bpms_instance_change / bpms_task_change",
			Detail:     "需要在开发者后台配置 Stream 模式或 HTTP 回调，API 只读预检无法确认后台是否已订阅。",
			NextAction: "优先启用 Stream 模式；订阅审批实例开始/结束/终止/删除和审批任务开始/结束/转交事件。",
		},
		checkResult{
			Name:       "审批实例与任务写操作",
			Status:     statusManual,
			Scope:      "Workflow.Instance.Write",
			Detail:     "未执行发起、撤销、评论、同意或拒绝等真实写操作。钉钉官方说明假勤等套件暂不支持通过通用接口直接发起审批实例。",
			NextAction: "仅在隔离测试模板和测试人员上做写入 POC；请假/加班优先保留钉钉入口，或评估自定义 OA 审批/审批托管模式。",
		},
		checkResult{
			Name:       "假期余额写操作",
			Status:     statusManual,
			Scope:      "qyapi_holiday_manage",
			Endpoint:   "/topapi/attendance/vacation/quota/update",
			Detail:     "项目已有写回能力，但本工具不会修改员工假期余额。",
			NextAction: "申请“钉钉假期管理权限”，使用专门测试假期类型和测试员工做最小额度联调，并验证回滚。",
		},
		checkResult{
			Name:       "排班与考勤组写操作",
			Status:     statusManual,
			Scope:      "qyapi_attendance_group_manage",
			Endpoint:   "/topapi/attendance/group/schedule/async",
			Detail:     "项目已有排班写回能力，本工具只验证读取，不写入真实排班。",
			NextAction: "申请“考勤组管理权限”，建立测试考勤组并配置 DINGTALK_ATTENDANCE_GROUP_ID 后，验证写入、覆盖、休息日和回滚。",
		},
		checkResult{
			Name:       "打卡记录写操作",
			Status:     statusManual,
			Scope:      "Attendance.Permission.Read + Pro.AttendanceRecord.Write",
			Detail:     "当前项目没有通用打卡记录写回实现，未执行上传或修改打卡记录。",
			NextAction: "先申请“考勤授权信息读权限”确认企业是否开放写能力；确有业务需要时再申请“考勤打卡记录写权限”。",
		},
	)
}

func failedPermissionCheck(name, scope, endpoint string, err error, nextAction string) checkResult {
	return checkResult{
		Name:       name,
		Status:     statusFail,
		Scope:      scope,
		Endpoint:   endpoint,
		Detail:     err.Error(),
		NextAction: nextAction,
	}
}

func printResults(results []checkResult, asJSON bool) {
	if asJSON {
		payload, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal results failed: %v\n", err)
			return
		}
		out := string(payload)
		// Hard guard: never print secrets even if upstream error includes them.
		out = strings.ReplaceAll(out, "access_token=", "access_token=***redacted***&")
		fmt.Println(out)
		return
	}

	for _, result := range results {
		fmt.Printf("[%s] %s\n", result.Status, result.Name)
		if result.Scope != "" {
			fmt.Printf("  权限: %s\n", result.Scope)
		}
		if result.Endpoint != "" {
			fmt.Printf("  接口/事件: %s\n", result.Endpoint)
		}
		fmt.Printf("  结果: %s\n", result.Detail)
		if result.NextAction != "" {
			fmt.Printf("  下一步: %s\n", result.NextAction)
		}
	}

	pass, fail, skipped, manual := 0, 0, 0, 0
	for _, result := range results {
		switch result.Status {
		case statusPass:
			pass++
		case statusFail:
			fail++
		case statusSkipped:
			skipped++
		case statusManual:
			manual++
		}
	}
	fmt.Printf("\n汇总: PASS=%d FAIL=%d SKIPPED=%d MANUAL=%d\n", pass, fail, skipped, manual)
}

func configuredLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "未配置"
	}
	return "已配置"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
