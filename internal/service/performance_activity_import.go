package service

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"peopleops/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const performanceActivityImportMaxBytes = 10 * 1024 * 1024

const (
	performanceImportSourceXiaotie = "xiaotie"
	performanceImportSourceMuteng  = "muteng"
)

type PerformanceImportIssue struct {
	Level    string `json:"level"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	DraftKey string `json:"draft_key,omitempty"`
	Sheet    string `json:"sheet,omitempty"`
	Row      int    `json:"row,omitempty"`
}

type PerformanceImportItemDraft struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Weight      float64 `json:"weight"`
	MaxScore    float64 `json:"max_score"`
}

type PerformanceImportSectionDraft struct {
	Name              string                       `json:"name"`
	SectionType       string                       `json:"section_type"`
	Weight            float64                      `json:"weight"`
	IsScoreRequired   bool                         `json:"is_score_required"`
	IsCommentRequired bool                         `json:"is_comment_required"`
	Items             []PerformanceImportItemDraft `json:"items"`
}

type PerformanceImportGoalDraft struct {
	SectionType    string  `json:"section_type"`
	GoalType       string  `json:"goal_type"`
	FixedKey       string  `json:"fixed_key,omitempty"`
	IsFixed        bool    `json:"is_fixed"`
	ItemName       string  `json:"item_name"`
	ItemDefinition string  `json:"item_definition,omitempty"`
	Weight         float64 `json:"weight"`
	RedLineValue   string  `json:"red_line_value,omitempty"`
	TargetValue    string  `json:"target_value,omitempty"`
	ChallengeValue string  `json:"challenge_value,omitempty"`
	ScoringRule    string  `json:"scoring_rule,omitempty"`
	SortOrder      int     `json:"sort_order"`
}

type PerformanceImportActivityDraft struct {
	DraftKey          string                          `json:"draft_key"`
	Selected          bool                            `json:"selected"`
	SourceSheet       string                          `json:"source_sheet"`
	TemplateName      string                          `json:"template_name"`
	ActivityName      string                          `json:"activity_name"`
	FlowType          string                          `json:"flow_type"`
	ActivityKind      string                          `json:"activity_kind,omitempty"`
	CycleType         string                          `json:"cycle_type"`
	StartDate         string                          `json:"start_date"`
	EndDate           string                          `json:"end_date"`
	EnableBonusScore  bool                            `json:"enable_bonus_score"`
	EmployeeName      string                          `json:"employee_name,omitempty"`
	EmployeeUserID    string                          `json:"employee_user_id,omitempty"`
	EmployeeMatch     string                          `json:"employee_match"`
	Sections          []PerformanceImportSectionDraft `json:"sections"`
	Goals             []PerformanceImportGoalDraft    `json:"goals"`
	SourceWeightTotal float64                         `json:"source_weight_total"`
}

type PerformanceActivityImportPreview struct {
	SourceType     string                           `json:"source_type"`
	SourceLabel    string                           `json:"source_label"`
	FileName       string                           `json:"file_name"`
	FileSHA256     string                           `json:"file_sha256"`
	Drafts         []PerformanceImportActivityDraft `json:"drafts"`
	Issues         []PerformanceImportIssue         `json:"issues"`
	RequiresReview bool                             `json:"requires_review"`
}

type PerformanceImportCommitDraft struct {
	DraftKey       string `json:"draft_key" binding:"required"`
	Selected       bool   `json:"selected"`
	TemplateName   string `json:"template_name"`
	ActivityName   string `json:"activity_name"`
	CycleType      string `json:"cycle_type"`
	StartDate      string `json:"start_date"`
	EndDate        string `json:"end_date"`
	EmployeeUserID string `json:"employee_user_id,omitempty"`
}

type PerformanceActivityImportCommitRequest struct {
	Drafts []PerformanceImportCommitDraft `json:"drafts"`
}

type PerformanceImportCreatedResult struct {
	DraftKey       string `json:"draft_key"`
	TemplateID     uint   `json:"template_id"`
	TemplateReused bool   `json:"template_reused"`
	ActivityID     uint   `json:"activity_id"`
	ActivityName   string `json:"activity_name"`
	ParticipantID  uint   `json:"participant_id,omitempty"`
	EmployeeUserID string `json:"employee_user_id,omitempty"`
	GoalCount      int    `json:"goal_count"`
}

type PerformanceActivityImportCommitResult struct {
	BatchID  string                           `json:"batch_id"`
	Created  []PerformanceImportCreatedResult `json:"created"`
	Warnings []string                         `json:"warnings,omitempty"`
}

type PerformanceActivityImportBatchView struct {
	BatchID        string                                 `json:"batch_id"`
	Status         string                                 `json:"status"`
	Preview        *PerformanceActivityImportPreview      `json:"preview,omitempty"`
	Result         *PerformanceActivityImportCommitResult `json:"result,omitempty"`
	FailureMessage string                                 `json:"failure_message,omitempty"`
	ExpiresAt      *time.Time                             `json:"expires_at,omitempty"`
	CreatedAt      time.Time                              `json:"created_at"`
	CommittedAt    *time.Time                             `json:"committed_at,omitempty"`
}

type performanceImportWorkbookSheet struct {
	Name string
	Rows []xlsxImportRow
}

func (s *PerformanceService) AnalyzePerformanceActivityImport(fileName string, r io.Reader, userID string) (*PerformanceActivityImportBatchView, error) {
	if strings.TrimSpace(s.tenantOrgID()) == "" {
		return nil, errors.New("缺少组织上下文")
	}
	if !strings.EqualFold(filepath.Ext(strings.TrimSpace(fileName)), ".xlsx") {
		return nil, errors.New("仅支持 .xlsx 文件")
	}
	data, err := readLimited(r, performanceActivityImportMaxBytes)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("文件内容为空")
	}
	hash := sha256.Sum256(data)
	preview, err := s.parsePerformanceActivityImportWorkbook(fileName, data)
	if err != nil {
		return nil, err
	}
	preview.FileSHA256 = hex.EncodeToString(hash[:])
	previewBytes, err := json.Marshal(preview)
	if err != nil {
		return nil, err
	}
	batchKey, err := newPerformanceImportBatchKey()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	batch := &database.PerformanceImportBatch{
		OrgID:       s.tenantOrgID(),
		BatchKey:    batchKey,
		FileName:    strings.TrimSpace(fileName),
		FileSHA256:  preview.FileSHA256,
		SourceType:  preview.SourceType,
		Status:      "analyzed",
		PreviewJSON: string(previewBytes),
		CreatedBy:   strings.TrimSpace(userID),
		ExpiresAt:   &expiresAt,
	}
	if err := s.db.Create(batch).Error; err != nil {
		return nil, err
	}
	return performanceImportBatchView(batch, preview, nil), nil
}

func (s *PerformanceService) GetPerformanceActivityImportBatch(batchKey string) (*PerformanceActivityImportBatchView, error) {
	var batch database.PerformanceImportBatch
	if err := s.scopedDB().Where("batch_key = ? AND deleted_at IS NULL", strings.TrimSpace(batchKey)).First(&batch).Error; err != nil {
		return nil, err
	}
	preview, result, err := decodePerformanceImportBatch(&batch)
	if err != nil {
		return nil, err
	}
	return performanceImportBatchView(&batch, preview, result), nil
}
func (s *PerformanceService) parsePerformanceActivityImportWorkbook(fileName string, data []byte) (*PerformanceActivityImportPreview, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("无法解析 Excel 文件，请确认上传的是有效的 .xlsx 文件")
	}
	sharedStrings, err := readSharedStrings(zr)
	if err != nil {
		return nil, err
	}
	sheets, issues, err := readPerformanceImportWorkbookSheets(zr, sharedStrings)
	if err != nil {
		return nil, err
	}
	allText := strings.Builder{}
	for _, sheet := range sheets {
		allText.WriteString(sheet.Name)
		for _, row := range sheet.Rows {
			allText.WriteString(strings.Join(row.Values, " "))
		}
	}
	text := allText.String()
	var preview *PerformanceActivityImportPreview
	switch {
	case strings.Contains(text, "下季度目标计划") && strings.Contains(text, "价值观及工作纪律"):
		preview, err = s.parseMutengPerformanceImport(fileName, sheets)
	case strings.Contains(text, "PARTB: 个人绩效") || strings.Contains(text, "小铁自助台球文化价值观评分表"):
		preview, err = s.parseXiaotiePerformanceImport(fileName, sheets)
	default:
		return nil, errors.New("无法识别该绩效模板，目前仅支持小铁文娱和沐腾科技模板")
	}
	if err != nil {
		return nil, err
	}
	preview.FileName = strings.TrimSpace(fileName)
	preview.Issues = append(issues, preview.Issues...)
	preview.RequiresReview = len(preview.Issues) > 0
	return preview, nil
}

func readPerformanceImportWorkbookSheets(zr *zip.Reader, sharedStrings []string) ([]performanceImportWorkbookSheet, []PerformanceImportIssue, error) {
	workbookData, err := readZipEntry(zr, "xl/workbook.xml")
	if err != nil {
		return nil, nil, err
	}
	var workbook xlsxWorkbook
	if err := xml.Unmarshal(workbookData, &workbook); err != nil {
		return nil, nil, fmt.Errorf("解析 workbook 失败: %w", err)
	}
	relsData, err := readZipEntry(zr, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, nil, err
	}
	var rels xlsxRelationships
	if err := xml.Unmarshal(relsData, &rels); err != nil {
		return nil, nil, fmt.Errorf("解析 workbook relationships 失败: %w", err)
	}
	targetByRID := make(map[string]string, len(rels.Relationships))
	for _, rel := range rels.Relationships {
		targetByRID[rel.ID] = resolveXLSXRelationshipTarget("xl/workbook.xml", rel.Target)
	}
	sheets := make([]performanceImportWorkbookSheet, 0, len(workbook.Sheets))
	issues := []PerformanceImportIssue{}
	for _, workbookSheet := range workbook.Sheets {
		target := targetByRID[workbookSheet.RID]
		if strings.TrimSpace(target) == "" {
			issues = append(issues, PerformanceImportIssue{Level: "warning", Code: "missing_sheet_relationship", Sheet: workbookSheet.Name, Message: "工作表关系已失效，已跳过"})
			continue
		}
		rows, readErr := readWorksheetRows(zr, target, sharedStrings)
		if readErr != nil {
			if errors.Is(readErr, errZipEntryNotFound) {
				issues = append(issues, PerformanceImportIssue{Level: "warning", Code: "missing_sheet_file", Sheet: workbookSheet.Name, Message: "工作表文件不存在，已跳过"})
				continue
			}
			return nil, nil, readErr
		}
		sheets = append(sheets, performanceImportWorkbookSheet{Name: strings.TrimSpace(workbookSheet.Name), Rows: rows})
	}
	if len(sheets) == 0 {
		return nil, nil, errors.New("文件中没有可读取的 Excel 工作表")
	}
	return sheets, issues, nil
}

func (s *PerformanceService) parseXiaotiePerformanceImport(fileName string, sheets []performanceImportWorkbookSheet) (*PerformanceActivityImportPreview, error) {
	preview := &PerformanceActivityImportPreview{SourceType: performanceImportSourceXiaotie, SourceLabel: "小铁文娱", Drafts: []PerformanceImportActivityDraft{}, Issues: []PerformanceImportIssue{}}
	for _, sheet := range sheets {
		if strings.Contains(sheet.Name, "示例") || strings.Contains(sheet.Name, "示范") || strings.Contains(sheet.Name, "案例") {
			continue
		}
		if performanceImportSheetContains(sheet, "小铁自助台球文化价值观评分表") {
			draft := parseXiaotieValueDraft(sheet)
			preview.Drafts = append(preview.Drafts, draft)
			continue
		}
		if !performanceImportSheetContains(sheet, "PARTB: 个人绩效") {
			continue
		}
		draft, draftIssues := parseXiaotieScoreDraft(sheet)
		preview.Drafts = append(preview.Drafts, draft)
		preview.Issues = append(preview.Issues, draftIssues...)
	}
	if len(preview.Drafts) == 0 {
		return nil, errors.New("未在小铁文娱模板中找到可导入的绩效表")
	}
	for i := range preview.Drafts {
		draft := &preview.Drafts[i]
		nameCell := ""
		if draft.DraftKey != "xiaotie_values" {
			nameCell = sheetValue(sheets, draft.SourceSheet, 3, 3)
		}
		draft.EmployeeName = performanceImportEmployeeName(fileName, nameCell)
		s.resolvePerformanceImportEmployee(draft, &preview.Issues)
	}
	return preview, nil
}

func parseXiaotieScoreDraft(sheet performanceImportWorkbookSheet) (PerformanceImportActivityDraft, []PerformanceImportIssue) {
	role := "普通员工"
	if performanceImportSheetContains(sheet, "负责人绩效") || strings.Contains(sheet.Name, "负责人") {
		role = "部门负责人"
	}
	draftKey := "xiaotie_employee"
	if role == "部门负责人" {
		draftKey = "xiaotie_manager"
	}
	draft := PerformanceImportActivityDraft{
		DraftKey: draftKey, Selected: true, SourceSheet: sheet.Name,
		TemplateName: "小铁文娱-" + role + "绩效模板", ActivityName: "小铁文娱-" + role + "绩效活动",
		FlowType: PerformanceFlowOld, CycleType: "monthly", EmployeeMatch: "unmatched",
		Sections: []PerformanceImportSectionDraft{}, Goals: []PerformanceImportGoalDraft{},
	}
	quantitative := []PerformanceImportGoalDraft{}
	keyActions := []PerformanceImportGoalDraft{}
	quantTotal, keyTotal := 0.0, 0.0
	currentSection := ""
	qIndex, kIndex := 0, 0
	issues := []PerformanceImportIssue{}
	for _, row := range sheet.Rows {
		category := strings.TrimSpace(valueAt(row.Values, 0))
		if strings.Contains(category, "量化指标") {
			currentSection = "quantitative"
		}
		if strings.Contains(category, "关键行动") {
			currentSection = "key_action"
		}
		if strings.Contains(category, "合计") {
			break
		}
		weight := performanceImportWeight(valueAt(row.Values, 3))
		if weight <= 0 || (currentSection != "quantitative" && currentSection != "key_action") {
			continue
		}
		name := strings.TrimSpace(valueAt(row.Values, 1))
		if currentSection == "quantitative" {
			qIndex++
			if name == "" {
				name = fmt.Sprintf("量化指标%d", qIndex)
			}
			quantTotal += weight
		} else {
			kIndex++
			if name == "" {
				name = fmt.Sprintf("关键行动%d", kIndex)
			}
			keyTotal += weight
		}
		redLineValue := strings.TrimSpace(valueAt(row.Values, 4))
		targetValue := strings.TrimSpace(valueAt(row.Values, 5))
		challengeValue := strings.TrimSpace(valueAt(row.Values, 6))
		scoringRule := strings.TrimSpace(valueAt(row.Values, 7))
		if currentSection == "key_action" {
			if scoringRule == "" {
				scoringRule = redLineValue
			}
			redLineValue = ""
			targetValue = ""
			challengeValue = ""
		}
		goal := PerformanceImportGoalDraft{
			SectionType: currentSection, GoalType: "kpi", ItemName: name,
			ItemDefinition: strings.TrimSpace(valueAt(row.Values, 2)), Weight: weight,
			RedLineValue: redLineValue, TargetValue: targetValue,
			ChallengeValue: challengeValue, ScoringRule: scoringRule,
			SortOrder: qIndex + kIndex,
		}
		if weight < 10 {
			issues = append(issues, PerformanceImportIssue{Level: "warning", Code: "weight_below_template_minimum", DraftKey: draftKey, Sheet: sheet.Name, Row: row.Number, Message: fmt.Sprintf("指标“%s”源权重为 %.0f%%，低于表头写的单项 10%%", name, weight)})
		}
		if currentSection == "quantitative" {
			quantitative = append(quantitative, goal)
		} else {
			keyActions = append(keyActions, goal)
		}
	}
	if quantTotal > 0 && keyTotal > 0 && (performanceImportAbs(quantTotal-70) > 0.01 || performanceImportAbs(keyTotal-30) > 0.01) {
		issues = append(issues, PerformanceImportIssue{Level: "warning", Code: "section_weight_conflict", DraftKey: draftKey, Sheet: sheet.Name, Message: fmt.Sprintf("模板文字写量化70%%/关键行动30%%，实际空白行是 %.0f%%/%.0f%%；预览已按70%%/30%%换算，提交前请确认", quantTotal, keyTotal)})
	}
	quantitative = rescalePerformanceImportGoals(quantitative, quantTotal, 70)
	keyActions = rescalePerformanceImportGoals(keyActions, keyTotal, 30)
	draft.Goals = append(draft.Goals, quantitative...)
	draft.Goals = append(draft.Goals, keyActions...)
	draft.Sections = append(draft.Sections, performanceImportSectionFromGoals("量化指标", "quantitative", 70, quantitative, 100))
	draft.Sections = append(draft.Sections, performanceImportSectionFromGoals("关键行动", "key_action", 30, keyActions, 100))
	draft.SourceWeightTotal = quantTotal + keyTotal
	return draft, issues
}
func parseXiaotieValueDraft(sheet performanceImportWorkbookSheet) PerformanceImportActivityDraft {
	draft := PerformanceImportActivityDraft{
		DraftKey: "xiaotie_values", Selected: false, SourceSheet: sheet.Name,
		TemplateName: "小铁文娱-季度价值观模板", ActivityName: "小铁文娱-季度价值观活动",
		FlowType: PerformanceFlowOld, CycleType: "quarterly", EmployeeMatch: "unmatched",
		Sections: []PerformanceImportSectionDraft{}, Goals: []PerformanceImportGoalDraft{}, SourceWeightTotal: 100,
	}
	for _, row := range sheet.Rows {
		sequence, err := strconv.Atoi(strings.TrimSpace(valueAt(row.Values, 0)))
		if err != nil || sequence < 1 || sequence > 10 {
			continue
		}
		name := strings.TrimSpace(valueAt(row.Values, 1))
		if name == "" {
			continue
		}
		draft.Goals = append(draft.Goals, PerformanceImportGoalDraft{
			SectionType: "values", GoalType: "fixed", FixedKey: fmt.Sprintf("xiaotie_value_%d", sequence), IsFixed: true,
			ItemName: name, ItemDefinition: strings.TrimSpace(valueAt(row.Values, 2)), Weight: 10,
			ScoringRule: "每项满分10分", SortOrder: sequence,
		})
	}
	draft.Sections = append(draft.Sections, performanceImportSectionFromGoals("文化价值观", "values", 100, draft.Goals, 10))
	return draft
}

func (s *PerformanceService) parseMutengPerformanceImport(fileName string, sheets []performanceImportWorkbookSheet) (*PerformanceActivityImportPreview, error) {
	var sourceSheet *performanceImportWorkbookSheet
	for i := range sheets {
		if performanceImportSheetContains(sheets[i], "下季度目标计划") && performanceImportSheetContains(sheets[i], "价值观及工作纪律") {
			sourceSheet = &sheets[i]
			break
		}
	}
	if sourceSheet == nil {
		return nil, errors.New("未在沐腾科技模板中找到“下季度目标计划”区域")
	}
	draft := PerformanceImportActivityDraft{
		DraftKey: "muteng_goal_setting", Selected: true, SourceSheet: sourceSheet.Name,
		TemplateName: "沐腾科技-目标设定模板", ActivityName: strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName)) + "-目标设定",
		FlowType: PerformanceFlowNew, ActivityKind: PerformanceActivityKindGoalSetting, CycleType: "quarterly",
		EnableBonusScore: true, EmployeeName: performanceImportEmployeeName(fileName, ""), EmployeeMatch: "unmatched",
		Sections: []PerformanceImportSectionDraft{}, Goals: []PerformanceImportGoalDraft{},
	}
	startRow := 0
	for _, row := range sourceSheet.Rows {
		if performanceImportRowContains(row, "下季度目标计划") {
			startRow = row.Number
		}
		if startRow == 0 || row.Number <= startRow {
			continue
		}
		name := strings.TrimSpace(valueAt(row.Values, 1))
		if strings.Contains(name, "合计") {
			break
		}
		weight := performanceImportWeight(valueAt(row.Values, 2))
		if weight <= 0 || name == "" || strings.Contains(name, "目标/关键职责事项") {
			continue
		}
		isBonus := strings.Contains(name, "加分项")
		sectionType := "goal"
		goalType := "kpi"
		fixedKey := ""
		isFixed := false
		if strings.Contains(name, "价值观及工作纪律") {
			goalType, fixedKey, isFixed = "fixed", "muteng_values_discipline", true
		}
		if isBonus {
			sectionType, goalType, fixedKey, isFixed = "bonus_penalty", "fixed", "muteng_bonus", true
		}
		draft.Goals = append(draft.Goals, PerformanceImportGoalDraft{
			SectionType: sectionType, GoalType: goalType, FixedKey: fixedKey, IsFixed: isFixed,
			ItemName: name, ItemDefinition: strings.TrimSpace(valueAt(row.Values, 3)), Weight: weight,
			ScoringRule: strings.TrimSpace(valueAt(row.Values, 3)), SortOrder: len(draft.Goals) + 1,
		})
		draft.SourceWeightTotal += weight
	}
	regularGoals := []PerformanceImportGoalDraft{}
	bonusGoals := []PerformanceImportGoalDraft{}
	for _, goal := range draft.Goals {
		if goal.SectionType == "bonus_penalty" {
			bonusGoals = append(bonusGoals, goal)
		} else {
			regularGoals = append(regularGoals, goal)
		}
	}
	if len(regularGoals) == 0 {
		return nil, errors.New("沐腾科技模板的下季度目标计划中没有可导入指标")
	}
	draft.Sections = append(draft.Sections, performanceImportSectionFromGoals("目标/关键职责事项", "goal", 100, regularGoals, 10))
	if len(bonusGoals) > 0 {
		draft.Sections = append(draft.Sections, performanceImportSectionFromGoals("加分项", "bonus_penalty", 0, bonusGoals, 10))
	}
	preview := &PerformanceActivityImportPreview{
		SourceType: performanceImportSourceMuteng, SourceLabel: "沐腾科技", Drafts: []PerformanceImportActivityDraft{draft},
		Issues: []PerformanceImportIssue{
			{Level: "warning", Code: "period_requires_confirmation", DraftKey: draft.DraftKey, Sheet: sourceSheet.Name, Message: "文件名、表内季度和“下季度”可能不一致，年份、季度和起止日期必须由 HR 确认"},
			{Level: "info", Code: "bonus_weight_allowed", DraftKey: draft.DraftKey, Sheet: sourceSheet.Name, Message: fmt.Sprintf("普通指标100%%，加分项额外%.0f%%；系统会启用附加分，不把总权重%.0f%%当成错误", performanceImportBonusWeight(draft.Goals), draft.SourceWeightTotal)},
		},
	}
	s.resolvePerformanceImportEmployee(&preview.Drafts[0], &preview.Issues)
	return preview, nil
}

func (s *PerformanceService) resolvePerformanceImportEmployee(draft *PerformanceImportActivityDraft, issues *[]PerformanceImportIssue) {
	if draft == nil || strings.TrimSpace(draft.EmployeeName) == "" {
		return
	}
	var users []database.User
	if err := s.scopedDB().Where("name = ? AND status = ? AND deleted_at IS NULL", strings.TrimSpace(draft.EmployeeName), "active").Find(&users).Error; err != nil {
		*issues = append(*issues, PerformanceImportIssue{Level: "warning", Code: "employee_lookup_failed", DraftKey: draft.DraftKey, Message: "员工自动匹配失败，提交前请手工选择"})
		return
	}
	switch len(users) {
	case 1:
		draft.EmployeeUserID = strings.TrimSpace(users[0].UserID)
		draft.EmployeeMatch = "matched"
	case 0:
		draft.EmployeeMatch = "unmatched"
		*issues = append(*issues, PerformanceImportIssue{Level: "warning", Code: "employee_not_matched", DraftKey: draft.DraftKey, Message: fmt.Sprintf("未匹配到员工“%s”，可先创建无参与人的草稿活动，或提交前手工选择员工", draft.EmployeeName)})
	default:
		draft.EmployeeMatch = "ambiguous"
		*issues = append(*issues, PerformanceImportIssue{Level: "warning", Code: "employee_ambiguous", DraftKey: draft.DraftKey, Message: fmt.Sprintf("存在多个同名员工“%s”，提交前必须手工选择", draft.EmployeeName)})
	}
}

func performanceImportSectionFromGoals(name, sectionType string, sectionWeight float64, goals []PerformanceImportGoalDraft, maxScore float64) PerformanceImportSectionDraft {
	items := make([]PerformanceImportItemDraft, 0, len(goals))
	total := 0.0
	for _, goal := range goals {
		total += goal.Weight
	}
	for _, goal := range goals {
		weight := 0.0
		if total > 0 {
			weight = goal.Weight * 100 / total
		}
		items = append(items, PerformanceImportItemDraft{Name: goal.ItemName, Description: goal.ItemDefinition, Weight: performanceImportRound(weight), MaxScore: maxScore})
	}
	return PerformanceImportSectionDraft{Name: name, SectionType: sectionType, Weight: sectionWeight, IsScoreRequired: true, Items: items}
}

func rescalePerformanceImportGoals(goals []PerformanceImportGoalDraft, sourceTotal, targetTotal float64) []PerformanceImportGoalDraft {
	if sourceTotal <= 0 {
		return goals
	}
	for i := range goals {
		goals[i].Weight = performanceImportRound(goals[i].Weight * targetTotal / sourceTotal)
	}
	return goals
}

func performanceImportWeight(value string) float64 {
	text := strings.TrimSpace(strings.ReplaceAll(value, "%", ""))
	if text == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0
	}
	if !strings.Contains(value, "%") && parsed > 0 && parsed <= 1 {
		parsed *= 100
	}
	return performanceImportRound(parsed)
}

func performanceImportBonusWeight(goals []PerformanceImportGoalDraft) float64 {
	total := 0.0
	for _, goal := range goals {
		if goal.SectionType == "bonus_penalty" {
			total += goal.Weight
		}
	}
	return total
}

func performanceImportRound(value float64) float64 {
	rounded, _ := strconv.ParseFloat(strconv.FormatFloat(value, 'f', 4, 64), 64)
	return rounded
}

func performanceImportAbs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func performanceImportSheetContains(sheet performanceImportWorkbookSheet, text string) bool {
	for _, row := range sheet.Rows {
		if performanceImportRowContains(row, text) {
			return true
		}
	}
	return false
}

func performanceImportRowContains(row xlsxImportRow, text string) bool {
	for _, value := range row.Values {
		if strings.Contains(strings.TrimSpace(value), text) {
			return true
		}
	}
	return false
}

func sheetValue(sheets []performanceImportWorkbookSheet, sheetName string, rowNumber, columnNumber int) string {
	for _, sheet := range sheets {
		if sheet.Name != sheetName {
			continue
		}
		for _, row := range sheet.Rows {
			if row.Number == rowNumber {
				return strings.TrimSpace(valueAt(row.Values, columnNumber-1))
			}
		}
	}
	return ""
}

func performanceImportEmployeeName(fileName, cellValue string) string {
	cellValue = strings.TrimSpace(cellValue)
	if !performanceImportPlaceholder(cellValue) {
		return cellValue
	}
	base := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	parts := strings.Split(base, "-")
	candidate := strings.TrimSpace(parts[len(parts)-1])
	if performanceImportPlaceholder(candidate) {
		return ""
	}
	return candidate
}

func performanceImportPlaceholder(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return value == "" || value == "xxx" || value == "xx" || value == "姓名" || strings.Contains(value, "模版") || strings.Contains(value, "模板")
}
func (s *PerformanceService) CommitPerformanceActivityImport(batchKey string, req PerformanceActivityImportCommitRequest, userID string) (*PerformanceActivityImportCommitResult, error) {
	if strings.TrimSpace(s.tenantOrgID()) == "" {
		return nil, errors.New("缺少组织上下文")
	}
	var committedResult *PerformanceActivityImportCommitResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var batch database.PerformanceImportBatch
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("org_id = ? AND batch_key = ? AND deleted_at IS NULL", s.tenantOrgID(), strings.TrimSpace(batchKey))
		if err := query.First(&batch).Error; err != nil {
			return err
		}
		if batch.Status == "committed" {
			_, result, err := decodePerformanceImportBatch(&batch)
			if err != nil {
				return err
			}
			if result == nil {
				return errors.New("导入批次已提交，但结果数据缺失")
			}
			committedResult = result
			return nil
		}
		if batch.ExpiresAt != nil && batch.ExpiresAt.Before(time.Now()) {
			return errors.New("导入预览已过期，请重新上传分析")
		}
		if batch.Status != "analyzed" && batch.Status != "failed" {
			return fmt.Errorf("当前导入批次状态不允许提交：%s", batch.Status)
		}
		preview, _, err := decodePerformanceImportBatch(&batch)
		if err != nil {
			return err
		}
		if preview == nil {
			return errors.New("导入预览不存在")
		}
		drafts, err := mergePerformanceImportCommitDrafts(preview.Drafts, req.Drafts)
		if err != nil {
			return err
		}
		selectedCount := 0
		for _, draft := range drafts {
			if draft.Selected {
				selectedCount++
			}
		}
		if selectedCount == 0 {
			return errors.New("请至少选择一个要创建的绩效活动")
		}

		result := &PerformanceActivityImportCommitResult{BatchID: batch.BatchKey, Created: []PerformanceImportCreatedResult{}, Warnings: []string{}}
		for _, draft := range drafts {
			if !draft.Selected {
				continue
			}
			if err := validatePerformanceImportDraft(draft); err != nil {
				return fmt.Errorf("%s：%w", draft.TemplateName, err)
			}
			template, reused, err := s.ensurePerformanceImportTemplateTx(tx, preview.SourceType, draft, userID)
			if err != nil {
				return err
			}
			var employee *database.User
			if strings.TrimSpace(draft.EmployeeUserID) != "" {
				var matched database.User
				if err := tx.Where("org_id = ? AND user_id = ? AND status = ? AND deleted_at IS NULL", s.tenantOrgID(), strings.TrimSpace(draft.EmployeeUserID), "active").First(&matched).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return fmt.Errorf("员工 %s 不存在、已离职或不属于当前企业", draft.EmployeeUserID)
					}
					return err
				}
				employee = &matched
			}
			var duplicateCount int64
			duplicateQuery := tx.Model(&database.PerformanceActivity{}).Where(
				"org_id = ? AND template_id = ? AND cycle_type = ? AND start_date = ? AND end_date = ? AND flow_type = ? AND deleted_at IS NULL",
				s.tenantOrgID(), template.ID, draft.CycleType, draft.StartDate, draft.EndDate, draft.FlowType,
			)
			if err := duplicateQuery.Count(&duplicateCount).Error; err != nil {
				return err
			}
			if duplicateCount > 0 {
				return fmt.Errorf("活动“%s”在相同周期已存在，请勿重复创建", draft.ActivityName)
			}

			targetEmployeeIDs := []string{}
			if employee != nil {
				targetEmployeeIDs = []string{strings.TrimSpace(employee.UserID)}
			}
			activity := &database.PerformanceActivity{
				OrgID: s.tenantOrgID(), Name: strings.TrimSpace(draft.ActivityName), CycleType: strings.TrimSpace(draft.CycleType),
				StartDate: strings.TrimSpace(draft.StartDate), EndDate: strings.TrimSpace(draft.EndDate), FlowType: draft.FlowType,
				ActivityKind: draft.ActivityKind, TargetSetStartAt: draft.StartDate, TargetSetEndAt: draft.EndDate,
				SelfEvalStartAt: draft.StartDate, SelfEvalEndAt: draft.EndDate, ManagerEvalStartAt: draft.StartDate, ManagerEvalEndAt: draft.EndDate,
				ResultConfirmStartAt: draft.StartDate, ResultConfirmEndAt: draft.EndDate,
				EmployeeConfirmStartAt: draft.StartDate, EmployeeConfirmEndAt: draft.EndDate,
				ManagerConfirmStartAt: draft.StartDate, ManagerConfirmEndAt: draft.EndDate,
				HRConfirmStartAt: draft.StartDate, HRConfirmEndAt: draft.EndDate, HRConfirmDeadline: draft.EndDate,
				Status: "draft", Description: fmt.Sprintf("由绩效 Excel 导入批次 %s 创建，来源：%s", batch.BatchKey, preview.SourceLabel),
				TargetEmployeeIDs: targetEmployeeIDs, DefaultAssessmentManagerSource: ManagerSourceDirectManager,
				SnapshotSource: "current_user", PublishMode: "manual", EnableBonusScore: draft.EnableBonusScore,
				CreatedBy: strings.TrimSpace(userID), UpdatedBy: strings.TrimSpace(userID),
			}
			applyTemplateSnapshotToActivity(activity, template, draft.FlowType)
			applyActivityKindWorkflowDefaults(activity)
			if err := tx.Create(activity).Error; err != nil {
				return err
			}
			if err := seedDefaultDistributionRules(tx, activity, userID); err != nil {
				return err
			}

			created := PerformanceImportCreatedResult{DraftKey: draft.DraftKey, TemplateID: template.ID, TemplateReused: reused, ActivityID: activity.ID, ActivityName: activity.Name}
			if employee != nil {
				if _, err := s.refreshParticipantsTx(tx, activity, userID); err != nil {
					return err
				}
				activityID := strconv.FormatUint(uint64(activity.ID), 10)
				var participant database.PerformanceParticipant
				if err := tx.Where("org_id = ? AND activity_id = ? AND employee_id = ? AND deleted_at IS NULL", s.tenantOrgID(), activityID, employee.UserID).First(&participant).Error; err != nil {
					return err
				}
				goalCount, err := createPerformanceImportGoalsTx(tx, activity, &participant, draft.Goals)
				if err != nil {
					return err
				}
				created.ParticipantID = participant.ID
				created.EmployeeUserID = employee.UserID
				created.GoalCount = goalCount
			} else {
				result.Warnings = append(result.Warnings, fmt.Sprintf("活动“%s”未匹配员工，已创建为空参与人的草稿活动", activity.Name))
			}
			result.Created = append(result.Created, created)
		}
		resultBytes, err := json.Marshal(result)
		if err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&database.PerformanceImportBatch{}).Where("id = ?", batch.ID).Updates(map[string]interface{}{
			"status": "committed", "result_json": string(resultBytes), "failure_message": "", "committed_by": strings.TrimSpace(userID), "committed_at": &now,
		}).Error; err != nil {
			return err
		}
		committedResult = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return committedResult, nil
}

func mergePerformanceImportCommitDrafts(source []PerformanceImportActivityDraft, updates []PerformanceImportCommitDraft) ([]PerformanceImportActivityDraft, error) {
	result := append([]PerformanceImportActivityDraft(nil), source...)
	if len(updates) == 0 {
		return result, nil
	}
	updateByKey := make(map[string]PerformanceImportCommitDraft, len(updates))
	for _, update := range updates {
		key := strings.TrimSpace(update.DraftKey)
		if key == "" {
			return nil, errors.New("draft_key 不能为空")
		}
		if _, exists := updateByKey[key]; exists {
			return nil, fmt.Errorf("draft_key 重复：%s", key)
		}
		updateByKey[key] = update
	}
	for i := range result {
		update, exists := updateByKey[result[i].DraftKey]
		if !exists {
			continue
		}
		result[i].Selected = update.Selected
		result[i].TemplateName = strings.TrimSpace(update.TemplateName)
		result[i].ActivityName = strings.TrimSpace(update.ActivityName)
		result[i].CycleType = strings.TrimSpace(update.CycleType)
		result[i].StartDate = strings.TrimSpace(update.StartDate)
		result[i].EndDate = strings.TrimSpace(update.EndDate)
		result[i].EmployeeUserID = strings.TrimSpace(update.EmployeeUserID)
		delete(updateByKey, result[i].DraftKey)
	}
	if len(updateByKey) > 0 {
		return nil, errors.New("提交内容包含不属于本批次的草稿")
	}
	return result, nil
}

func validatePerformanceImportDraft(draft PerformanceImportActivityDraft) error {
	if strings.TrimSpace(draft.TemplateName) == "" {
		return errors.New("模板名称不能为空")
	}
	if strings.TrimSpace(draft.ActivityName) == "" {
		return errors.New("活动名称不能为空")
	}
	if draft.CycleType != "monthly" && draft.CycleType != "quarterly" && draft.CycleType != "annual" {
		return errors.New("考核周期类型不正确")
	}
	if strings.TrimSpace(draft.StartDate) == "" || strings.TrimSpace(draft.EndDate) == "" {
		return errors.New("必须确认开始日期和结束日期")
	}
	start, err := time.Parse("2006-01-02", draft.StartDate)
	if err != nil {
		return errors.New("开始日期格式应为 YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", draft.EndDate)
	if err != nil {
		return errors.New("结束日期格式应为 YYYY-MM-DD")
	}
	if end.Before(start) {
		return errors.New("结束日期不能早于开始日期")
	}
	if len(draft.Sections) == 0 {
		return errors.New("没有可创建的模板维度")
	}
	return nil
}
func (s *PerformanceService) ensurePerformanceImportTemplateTx(tx *gorm.DB, sourceType string, draft PerformanceImportActivityDraft, userID string) (*database.PerformanceTemplate, bool, error) {
	fingerprintPayload := struct {
		Source       string                          `json:"source"`
		TemplateName string                          `json:"template_name"`
		FlowType     string                          `json:"flow_type"`
		Sections     []PerformanceImportSectionDraft `json:"sections"`
	}{Source: sourceType, TemplateName: draft.TemplateName, FlowType: draft.FlowType, Sections: draft.Sections}
	payload, err := json.Marshal(fingerprintPayload)
	if err != nil {
		return nil, false, err
	}
	hash := sha256.Sum256(payload)
	code := fmt.Sprintf("excel_%s_%s", sourceType, hex.EncodeToString(hash[:8]))
	var existing database.PerformanceTemplate
	if err := tx.Where("org_id = ? AND code = ? AND deleted_at IS NULL", s.tenantOrgID(), code).First(&existing).Error; err == nil {
		return &existing, true, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	workflowConfig, formConfig, levelRuleConfig, distributionConfig, permissionConfig, publishConfig := defaultPerformanceConfig(draft.FlowType)
	template := &database.PerformanceTemplate{
		OrgID: s.tenantOrgID(), Name: strings.TrimSpace(draft.TemplateName), Code: code,
		Description: fmt.Sprintf("由%s绩效 Excel 模板识别生成", sourceType), FlowType: draft.FlowType,
		Status: "active", CycleTypes: []string{draft.CycleType}, WorkflowConfig: workflowConfig,
		FormConfig: formConfig, LevelRuleConfig: levelRuleConfig, DistributionConfig: distributionConfig,
		PermissionConfig: permissionConfig, PublishConfig: publishConfig,
		CreatedBy: strings.TrimSpace(userID), UpdatedBy: strings.TrimSpace(userID),
	}
	if err := tx.Create(template).Error; err != nil {
		return nil, false, err
	}
	for sectionIndex, sectionDraft := range draft.Sections {
		section := &database.PerformanceTemplateSection{
			OrgID: s.tenantOrgID(), TemplateID: template.ID, Name: sectionDraft.Name, SectionType: sectionDraft.SectionType,
			Weight: sectionDraft.Weight, SortOrder: sectionIndex + 1, IsScoreRequired: sectionDraft.IsScoreRequired,
			IsCommentRequired: sectionDraft.IsCommentRequired,
		}
		if err := tx.Create(section).Error; err != nil {
			return nil, false, err
		}
		for itemIndex, itemDraft := range sectionDraft.Items {
			item := &database.PerformanceTemplateItem{
				OrgID: s.tenantOrgID(), SectionID: section.ID, Name: itemDraft.Name, Description: itemDraft.Description,
				MaxScore: itemDraft.MaxScore, Weight: itemDraft.Weight, SortOrder: itemIndex + 1,
			}
			if err := tx.Create(item).Error; err != nil {
				return nil, false, err
			}
		}
	}
	return template, false, nil
}

func createPerformanceImportGoalsTx(tx *gorm.DB, activity *database.PerformanceActivity, participant *database.PerformanceParticipant, drafts []PerformanceImportGoalDraft) (int, error) {
	if activity == nil || participant == nil || len(drafts) == 0 {
		return 0, nil
	}
	activityID := strconv.FormatUint(uint64(activity.ID), 10)
	goalPhase := targetSettingGoalPhaseForActivity(activity)
	records := make([]database.PerformanceGoalRecord, 0, len(drafts))
	for _, draft := range drafts {
		records = append(records, database.PerformanceGoalRecord{
			OrgID: strings.TrimSpace(activity.OrgID), ActivityID: activityID, ParticipantID: participant.ID,
			SectionType: draft.SectionType, GoalPhase: goalPhase, GoalType: draft.GoalType, FixedKey: draft.FixedKey,
			IsFixed: draft.IsFixed, ItemName: draft.ItemName, ItemDefinition: draft.ItemDefinition, Weight: draft.Weight,
			RedLineValue: draft.RedLineValue, TargetValue: draft.TargetValue, ChallengeValue: draft.ChallengeValue,
			ScoringRule: draft.ScoringRule, ApprovalStatus: "pending", VisibilityScope: "department_only", SortOrder: draft.SortOrder,
		})
	}
	if err := tx.Create(&records).Error; err != nil {
		return 0, err
	}
	return len(records), nil
}

func decodePerformanceImportBatch(batch *database.PerformanceImportBatch) (*PerformanceActivityImportPreview, *PerformanceActivityImportCommitResult, error) {
	if batch == nil {
		return nil, nil, nil
	}
	var preview *PerformanceActivityImportPreview
	if strings.TrimSpace(batch.PreviewJSON) != "" {
		decoded := &PerformanceActivityImportPreview{}
		if err := json.Unmarshal([]byte(batch.PreviewJSON), decoded); err != nil {
			return nil, nil, fmt.Errorf("解析导入预览失败: %w", err)
		}
		preview = decoded
	}
	var result *PerformanceActivityImportCommitResult
	if strings.TrimSpace(batch.ResultJSON) != "" {
		decoded := &PerformanceActivityImportCommitResult{}
		if err := json.Unmarshal([]byte(batch.ResultJSON), decoded); err != nil {
			return nil, nil, fmt.Errorf("解析导入结果失败: %w", err)
		}
		result = decoded
	}
	return preview, result, nil
}

func performanceImportBatchView(batch *database.PerformanceImportBatch, preview *PerformanceActivityImportPreview, result *PerformanceActivityImportCommitResult) *PerformanceActivityImportBatchView {
	if batch == nil {
		return nil
	}
	return &PerformanceActivityImportBatchView{
		BatchID: batch.BatchKey, Status: batch.Status, Preview: preview, Result: result,
		FailureMessage: batch.FailureMessage, ExpiresAt: batch.ExpiresAt, CreatedAt: batch.CreatedAt, CommittedAt: batch.CommittedAt,
	}
}

func newPerformanceImportBatchKey() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return "pimp_" + hex.EncodeToString(data), nil
}
