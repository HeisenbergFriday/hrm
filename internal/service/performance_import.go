package service

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"peopleops/internal/database"
)

const performanceParticipantImportMaxBytes = 10 * 1024 * 1024

var (
	zeroDecimalPattern = regexp.MustCompile(`^[+-]?\d+\.0+$`)
	scientificPattern  = regexp.MustCompile(`^[+-]?\d+(\.\d+)?[eE][+-]?\d+$`)
)

type PerformanceParticipantImportSkippedRow struct {
	Row    int    `json:"row"`
	Reason string `json:"reason"`
}

type PerformanceParticipantImportEmployee struct {
	UserID                      string `json:"user_id"`
	EmployeeID                  string `json:"employee_id"`
	Name                        string `json:"name"`
	DepartmentID                string `json:"department_id"`
	DepartmentName              string `json:"department_name"`
	AssessmentManagerUserID     string `json:"assessment_manager_user_id,omitempty"`
	AssessmentManagerEmployeeID string `json:"assessment_manager_employee_id,omitempty"`
	AssessmentManagerName       string `json:"assessment_manager_name,omitempty"`
	AssessmentManagerSource     string `json:"assessment_manager_source,omitempty"`
	ManagerOverrideReason       string `json:"manager_override_reason,omitempty"`
}

type PerformanceParticipantImportManagerAssignment struct {
	Row                         int    `json:"row"`
	UserID                      string `json:"user_id"`
	EmployeeID                  string `json:"employee_id"`
	AssessmentManagerUserID     string `json:"assessment_manager_user_id"`
	AssessmentManagerEmployeeID string `json:"assessment_manager_employee_id"`
	AssessmentManagerName       string `json:"assessment_manager_name"`
	AssessmentManagerSource     string `json:"assessment_manager_source"`
	ManagerOverrideReason       string `json:"manager_override_reason"`
}

type PerformanceParticipantImportResult struct {
	ActivityName                 string                                          `json:"activity_name"`
	EmployeeIDs                  []string                                        `json:"employee_ids"`
	Employees                    []PerformanceParticipantImportEmployee          `json:"employees"`
	ManagerAssignments           []PerformanceParticipantImportManagerAssignment `json:"manager_assignments"`
	ManagerAssignmentSkippedRows []PerformanceParticipantImportSkippedRow        `json:"manager_assignment_skipped_rows"`
	ParsedCount                  int                                             `json:"parsed_count"`
	ImportedCount                int                                             `json:"imported_count"`
	DuplicateCount               int                                             `json:"duplicate_count"`
	MissingEmployeeIDs           []string                                        `json:"missing_employee_ids"`
	InactiveEmployeeIDs          []string                                        `json:"inactive_employee_ids"`
	SkippedRows                  []PerformanceParticipantImportSkippedRow        `json:"skipped_rows"`
	Warnings                     []string                                        `json:"warnings"`
	rawEmployeeIDs               []string
	rawRows                      []performanceParticipantImportRawRow
}

type performanceParticipantImportRawRow struct {
	Row                         int
	EmployeeID                  string
	AssessmentManagerEmployeeID string
	AssessmentManagerName       string
	AssessmentManagerSource     string
	ManagerOverrideReason       string
}

type xlsxWorkbook struct {
	Sheets []xlsxWorkbookSheet `xml:"sheets>sheet"`
}

type xlsxWorkbookSheet struct {
	Name string `xml:"name,attr"`
	RID  string `xml:"id,attr"`
}

type xlsxRelationships struct {
	Relationships []xlsxRelationship `xml:"Relationship"`
}

type xlsxRelationship struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
}

type xlsxSharedStrings struct {
	Items []xlsxTextItem `xml:"si"`
}

type xlsxWorksheet struct {
	Rows []xlsxRow `xml:"sheetData>row"`
}

type xlsxRow struct {
	Index int        `xml:"r,attr"`
	Cells []xlsxCell `xml:"c"`
}

type xlsxCell struct {
	Ref    string       `xml:"r,attr"`
	Type   string       `xml:"t,attr"`
	Value  string       `xml:"v"`
	Inline xlsxTextItem `xml:"is"`
}

type xlsxTextItem struct {
	Text string
}

type xlsxImportRow struct {
	Number int
	Values []string
}

func (item *xlsxTextItem) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var parts []string
	depth := 1
	for depth > 0 {
		token, err := d.Token()
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local != "t" {
				continue
			}
			var text string
			if err := d.DecodeElement(&text, &t); err != nil {
				return err
			}
			parts = append(parts, text)
			depth--
		case xml.EndElement:
			depth--
		}
	}
	item.Text = strings.Join(parts, "")
	return nil
}

func ParsePerformanceParticipantImportXLSX(r io.Reader) (*PerformanceParticipantImportResult, error) {
	data, err := readLimited(r, performanceParticipantImportMaxBytes)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("文件内容为空")
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("无法解析 Excel 文件，请确认上传的是 .xlsx 文件")
	}

	sharedStrings, err := readSharedStrings(zr)
	if err != nil {
		return nil, err
	}
	sheetName, err := resolveFirstWorksheetName(zr)
	if err != nil {
		return nil, err
	}
	rows, err := readWorksheetRows(zr, sheetName, sharedStrings)
	if err != nil {
		return nil, err
	}
	return parsePerformanceParticipantRows(rows)
}

func (s *PerformanceService) ResolveImportedPerformanceEmployees(result *PerformanceParticipantImportResult) error {
	if result == nil {
		return nil
	}
	sourceRows := result.rawRows
	if len(sourceRows) == 0 {
		sourceIDs := result.rawEmployeeIDs
		if len(sourceIDs) == 0 {
			sourceIDs = result.EmployeeIDs
		}
		for _, id := range sourceIDs {
			sourceRows = append(sourceRows, performanceParticipantImportRawRow{EmployeeID: id})
		}
	}
	result.ParsedCount = len(sourceRows)
	result.EmployeeIDs = []string{}
	result.Employees = []PerformanceParticipantImportEmployee{}
	result.ManagerAssignments = []PerformanceParticipantImportManagerAssignment{}
	result.ManagerAssignmentSkippedRows = []PerformanceParticipantImportSkippedRow{}
	result.ImportedCount = 0
	result.MissingEmployeeIDs = []string{}
	result.InactiveEmployeeIDs = []string{}
	if len(sourceRows) == 0 {
		return nil
	}

	lookupIDSet := make(map[string]struct{})
	for _, row := range sourceRows {
		if id := strings.TrimSpace(row.EmployeeID); id != "" {
			lookupIDSet[id] = struct{}{}
		}
		if id := strings.TrimSpace(row.AssessmentManagerEmployeeID); id != "" {
			lookupIDSet[id] = struct{}{}
		}
	}
	lookupIDs := make([]string, 0, len(lookupIDSet))
	for id := range lookupIDSet {
		lookupIDs = append(lookupIDs, id)
	}

	var profiles []database.EmployeeProfile
	if err := s.db.Where("(employee_id IN ? OR user_id IN ?) AND deleted_at IS NULL", lookupIDs, lookupIDs).Find(&profiles).Error; err != nil {
		return err
	}
	profileUserByEmployeeID := make(map[string]string)
	profileByEmployeeID := make(map[string]database.EmployeeProfile)
	profileByUserID := make(map[string]database.EmployeeProfile)
	for _, profile := range profiles {
		employeeID := strings.TrimSpace(profile.EmployeeID)
		userID := strings.TrimSpace(profile.UserID)
		if employeeID != "" && userID != "" {
			profileUserByEmployeeID[employeeID] = userID
		}
		if employeeID != "" {
			profileByEmployeeID[employeeID] = profile
		}
		if userID != "" {
			profileByUserID[userID] = profile
		}
	}

	candidateUserIDSet := make(map[string]struct{})
	for _, id := range lookupIDs {
		candidateUserIDSet[id] = struct{}{}
		if userID := profileUserByEmployeeID[id]; userID != "" {
			candidateUserIDSet[userID] = struct{}{}
		}
	}
	candidateUserIDs := make([]string, 0, len(candidateUserIDSet))
	for id := range candidateUserIDSet {
		candidateUserIDs = append(candidateUserIDs, id)
	}

	var users []database.User
	if err := s.db.Where("user_id IN ? AND deleted_at IS NULL", candidateUserIDs).Find(&users).Error; err != nil {
		return err
	}
	userByID := make(map[string]database.User)
	departmentIDSet := make(map[string]struct{})
	for _, user := range users {
		userID := strings.TrimSpace(user.UserID)
		userByID[userID] = user
		if departmentID := strings.TrimSpace(user.DepartmentID); departmentID != "" {
			departmentIDSet[departmentID] = struct{}{}
		}
	}

	departmentIDs := make([]string, 0, len(departmentIDSet))
	for id := range departmentIDSet {
		departmentIDs = append(departmentIDs, id)
	}
	departmentNameByID := make(map[string]string)
	if len(departmentIDs) > 0 {
		var departments []database.Department
		if err := s.db.Where("department_id IN ? AND deleted_at IS NULL", departmentIDs).Find(&departments).Error; err != nil {
			return err
		}
		for _, department := range departments {
			departmentNameByID[strings.TrimSpace(department.DepartmentID)] = department.Name
		}
	}

	seenResolved := make(map[string]struct{})
	managerSkippedCount := 0
	for _, row := range sourceRows {
		sourceID := strings.TrimSpace(row.EmployeeID)
		userID := sourceID
		user, ok := userByID[userID]
		if !ok {
			if mappedUserID := profileUserByEmployeeID[sourceID]; mappedUserID != "" {
				userID = mappedUserID
				user, ok = userByID[userID]
			}
		}
		if !ok {
			result.MissingEmployeeIDs = append(result.MissingEmployeeIDs, sourceID)
			continue
		}
		if strings.TrimSpace(user.Status) != "active" {
			result.InactiveEmployeeIDs = append(result.InactiveEmployeeIDs, sourceID)
			continue
		}
		if _, exists := seenResolved[userID]; exists {
			continue
		}

		managerUserID, managerEmployeeID, managerName, managerSource, managerReason, err := s.resolveImportedAssessmentManager(row, profileUserByEmployeeID, profileByUserID, userByID)
		if err != nil {
			result.ManagerAssignmentSkippedRows = append(result.ManagerAssignmentSkippedRows, PerformanceParticipantImportSkippedRow{
				Row:    row.Row,
				Reason: err.Error(),
			})
			managerSkippedCount++
			managerUserID = ""
			managerEmployeeID = ""
			managerName = ""
			managerSource = ""
			managerReason = ""
		}

		profile := profileByUserID[userID]
		if strings.TrimSpace(profile.EmployeeID) == "" {
			profile = profileByEmployeeID[sourceID]
		}
		employeeID := strings.TrimSpace(profile.EmployeeID)
		if employeeID == "" && sourceID != userID {
			employeeID = sourceID
		}
		departmentID := strings.TrimSpace(user.DepartmentID)
		seenResolved[userID] = struct{}{}
		result.EmployeeIDs = append(result.EmployeeIDs, userID)
		result.Employees = append(result.Employees, PerformanceParticipantImportEmployee{
			UserID:                      userID,
			EmployeeID:                  employeeID,
			Name:                        user.Name,
			DepartmentID:                departmentID,
			DepartmentName:              departmentNameByID[departmentID],
			AssessmentManagerUserID:     managerUserID,
			AssessmentManagerEmployeeID: managerEmployeeID,
			AssessmentManagerName:       managerName,
			AssessmentManagerSource:     managerSource,
			ManagerOverrideReason:       managerReason,
		})
		if managerUserID != "" {
			result.ManagerAssignments = append(result.ManagerAssignments, PerformanceParticipantImportManagerAssignment{
				Row:                         row.Row,
				UserID:                      userID,
				EmployeeID:                  employeeID,
				AssessmentManagerUserID:     managerUserID,
				AssessmentManagerEmployeeID: managerEmployeeID,
				AssessmentManagerName:       managerName,
				AssessmentManagerSource:     managerSource,
				ManagerOverrideReason:       managerReason,
			})
		}
	}
	result.ImportedCount = len(result.EmployeeIDs)
	if len(result.MissingEmployeeIDs) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("有 %d 个工号未匹配到员工", len(result.MissingEmployeeIDs)))
	}
	if len(result.InactiveEmployeeIDs) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("有 %d 个工号对应员工不是在职状态", len(result.InactiveEmployeeIDs)))
	}
	if managerSkippedCount > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("有 %d 行考核上级未匹配或不可用，已保留员工并跳过上级覆盖", managerSkippedCount))
	}
	return nil
}

func (s *PerformanceService) resolveImportedAssessmentManager(
	row performanceParticipantImportRawRow,
	profileUserByEmployeeID map[string]string,
	profileByUserID map[string]database.EmployeeProfile,
	userByID map[string]database.User,
) (string, string, string, string, string, error) {
	managerIDInput := strings.TrimSpace(row.AssessmentManagerEmployeeID)
	managerNameInput := strings.TrimSpace(row.AssessmentManagerName)
	sourceInput := strings.TrimSpace(row.AssessmentManagerSource)
	reason := strings.TrimSpace(row.ManagerOverrideReason)

	if managerIDInput == "" && managerNameInput == "" {
		if sourceInput != "" || reason != "" {
			return "", "", "", "", reason, errors.New("考核上级为空")
		}
		return "", "", "", "", reason, nil
	}

	source := ManagerSourceImport
	if sourceInput != "" {
		normalized, err := normalizeAssessmentManagerSource(sourceInput)
		if err != nil {
			return "", "", "", "", reason, err
		}
		source = normalized
	}

	var manager database.User
	var ok bool
	managerUserID := managerIDInput
	if managerUserID != "" {
		manager, ok = userByID[managerUserID]
		if !ok {
			if mappedUserID := profileUserByEmployeeID[managerIDInput]; mappedUserID != "" {
				managerUserID = mappedUserID
				manager, ok = userByID[managerUserID]
			}
		}
		if !ok {
			return "", "", "", "", reason, fmt.Errorf("考核上级工号未匹配: %s", managerIDInput)
		}
	} else {
		var managers []database.User
		if err := s.db.Where("name = ? AND status = ? AND deleted_at IS NULL", managerNameInput, "active").Find(&managers).Error; err != nil {
			return "", "", "", "", reason, err
		}
		if len(managers) == 0 {
			return "", "", "", "", reason, fmt.Errorf("考核上级姓名未匹配: %s", managerNameInput)
		}
		if len(managers) > 1 {
			return "", "", "", "", reason, fmt.Errorf("考核上级姓名不唯一: %s", managerNameInput)
		}
		manager = managers[0]
	}

	if strings.TrimSpace(manager.Status) != "active" {
		return "", "", "", "", reason, fmt.Errorf("考核上级不是在职状态: %s", managerIDInput)
	}
	managerEmployeeID := strings.TrimSpace(profileByUserID[manager.UserID].EmployeeID)
	if managerEmployeeID == "" && managerIDInput != manager.UserID {
		managerEmployeeID = managerIDInput
	}
	managerName := strings.TrimSpace(manager.Name)
	if managerName == "" {
		managerName = managerNameInput
	}
	return manager.UserID, managerEmployeeID, managerName, source, reason, nil
}

func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(r, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("文件不能超过 %dMB", maxBytes/1024/1024)
	}
	return data, nil
}

func readSharedStrings(zr *zip.Reader) ([]string, error) {
	data, err := readZipEntry(zr, "xl/sharedStrings.xml")
	if err != nil {
		if errors.Is(err, errZipEntryNotFound) {
			return []string{}, nil
		}
		return nil, err
	}
	var shared xlsxSharedStrings
	if err := xml.Unmarshal(data, &shared); err != nil {
		return nil, fmt.Errorf("解析 sharedStrings 失败: %w", err)
	}
	values := make([]string, 0, len(shared.Items))
	for _, item := range shared.Items {
		values = append(values, item.Text)
	}
	return values, nil
}

func resolveFirstWorksheetName(zr *zip.Reader) (string, error) {
	workbookData, err := readZipEntry(zr, "xl/workbook.xml")
	if err != nil {
		if errors.Is(err, errZipEntryNotFound) {
			return "xl/worksheets/sheet1.xml", nil
		}
		return "", err
	}
	var workbook xlsxWorkbook
	if err := xml.Unmarshal(workbookData, &workbook); err != nil {
		return "", fmt.Errorf("解析 workbook 失败: %w", err)
	}
	if len(workbook.Sheets) == 0 || strings.TrimSpace(workbook.Sheets[0].RID) == "" {
		return "xl/worksheets/sheet1.xml", nil
	}

	relsData, err := readZipEntry(zr, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return "", err
	}
	var rels xlsxRelationships
	if err := xml.Unmarshal(relsData, &rels); err != nil {
		return "", fmt.Errorf("解析 workbook relationships 失败: %w", err)
	}
	for _, rel := range rels.Relationships {
		if rel.ID == workbook.Sheets[0].RID {
			return resolveXLSXRelationshipTarget("xl/workbook.xml", rel.Target), nil
		}
	}
	return "", errors.New("未找到第一个工作表")
}

func readWorksheetRows(zr *zip.Reader, sheetName string, sharedStrings []string) ([]xlsxImportRow, error) {
	data, err := readZipEntry(zr, sheetName)
	if err != nil {
		return nil, err
	}
	var sheet xlsxWorksheet
	if err := xml.Unmarshal(data, &sheet); err != nil {
		return nil, fmt.Errorf("解析工作表失败: %w", err)
	}
	rows := make([]xlsxImportRow, 0, len(sheet.Rows))
	for rowIndex, row := range sheet.Rows {
		number := row.Index
		if number <= 0 {
			number = rowIndex + 1
		}
		values := rowValues(row, sharedStrings)
		rows = append(rows, xlsxImportRow{Number: number, Values: values})
	}
	return rows, nil
}

func rowValues(row xlsxRow, sharedStrings []string) []string {
	valuesByColumn := make(map[int]string)
	maxColumn := -1
	for fallbackIndex, cell := range row.Cells {
		column := cellColumnIndex(cell.Ref, fallbackIndex)
		if column < 0 {
			continue
		}
		if column > maxColumn {
			maxColumn = column
		}
		valuesByColumn[column] = xlsxCellValue(cell, sharedStrings)
	}
	if maxColumn < 0 {
		return []string{}
	}
	values := make([]string, maxColumn+1)
	for column, value := range valuesByColumn {
		values[column] = strings.TrimSpace(value)
	}
	return values
}

func xlsxCellValue(cell xlsxCell, sharedStrings []string) string {
	switch cell.Type {
	case "s":
		index, err := strconv.Atoi(strings.TrimSpace(cell.Value))
		if err != nil || index < 0 || index >= len(sharedStrings) {
			return ""
		}
		return sharedStrings[index]
	case "inlineStr":
		return cell.Inline.Text
	default:
		return cell.Value
	}
}

func parsePerformanceParticipantRows(rows []xlsxImportRow) (*PerformanceParticipantImportResult, error) {
	headerRowIndex, headers := findPerformanceImportHeader(rows)
	if headerRowIndex < 0 {
		return nil, errors.New("未找到模板表头，请使用包含“绩效活动、工号、姓名、一级部门、二级部门、三级部门”的 Excel 模板")
	}

	activityColumn := headers["activity"]
	employeeIDColumn := headers["employee_id"]
	managerEmployeeIDColumn, hasManagerEmployeeIDColumn := headers["assessment_manager_employee_id"]
	managerNameColumn, hasManagerNameColumn := headers["assessment_manager_name"]
	managerSourceColumn, hasManagerSourceColumn := headers["assessment_manager_source"]
	managerReasonColumn, hasManagerReasonColumn := headers["manager_override_reason"]
	result := &PerformanceParticipantImportResult{
		EmployeeIDs:                  []string{},
		ManagerAssignments:           []PerformanceParticipantImportManagerAssignment{},
		ManagerAssignmentSkippedRows: []PerformanceParticipantImportSkippedRow{},
		MissingEmployeeIDs:           []string{},
		InactiveEmployeeIDs:          []string{},
		SkippedRows:                  []PerformanceParticipantImportSkippedRow{},
		Warnings:                     []string{},
		rawEmployeeIDs:               []string{},
		rawRows:                      []performanceParticipantImportRawRow{},
	}
	seenIDs := make(map[string]struct{})
	activityNames := make(map[string]struct{})

	for _, row := range rows[headerRowIndex+1:] {
		if rowIsEmpty(row.Values) {
			continue
		}

		activityName := strings.TrimSpace(valueAt(row.Values, activityColumn))
		if activityName != "" {
			activityNames[activityName] = struct{}{}
			if result.ActivityName == "" {
				result.ActivityName = activityName
			}
		}

		employeeID := normalizeImportedEmployeeID(valueAt(row.Values, employeeIDColumn))
		if employeeID == "" {
			result.SkippedRows = append(result.SkippedRows, PerformanceParticipantImportSkippedRow{
				Row:    row.Number,
				Reason: "工号为空",
			})
			continue
		}
		if _, exists := seenIDs[employeeID]; exists {
			result.DuplicateCount++
			continue
		}
		seenIDs[employeeID] = struct{}{}
		result.rawEmployeeIDs = append(result.rawEmployeeIDs, employeeID)
		rawRow := performanceParticipantImportRawRow{
			Row:        row.Number,
			EmployeeID: employeeID,
		}
		if hasManagerEmployeeIDColumn {
			rawRow.AssessmentManagerEmployeeID = normalizeImportedEmployeeID(valueAt(row.Values, managerEmployeeIDColumn))
		}
		if hasManagerNameColumn {
			rawRow.AssessmentManagerName = strings.TrimSpace(valueAt(row.Values, managerNameColumn))
		}
		if hasManagerSourceColumn {
			rawRow.AssessmentManagerSource = strings.TrimSpace(valueAt(row.Values, managerSourceColumn))
		}
		if hasManagerReasonColumn {
			rawRow.ManagerOverrideReason = strings.TrimSpace(valueAt(row.Values, managerReasonColumn))
		}
		result.rawRows = append(result.rawRows, rawRow)
		result.EmployeeIDs = append(result.EmployeeIDs, employeeID)
	}

	result.ParsedCount = len(result.EmployeeIDs)
	result.ImportedCount = len(result.EmployeeIDs)
	if len(result.EmployeeIDs) == 0 {
		return nil, errors.New("模板中未读取到有效工号")
	}
	if len(activityNames) > 1 {
		names := make([]string, 0, len(activityNames))
		for name := range activityNames {
			names = append(names, name)
		}
		sort.Strings(names)
		result.Warnings = append(result.Warnings, fmt.Sprintf("模板包含多个绩效活动名称，已使用：%s", result.ActivityName))
	}
	return result, nil
}

func findPerformanceImportHeader(rows []xlsxImportRow) (int, map[string]int) {
	for index, row := range rows {
		if index > 20 {
			break
		}
		headers := make(map[string]int)
		for column, value := range row.Values {
			switch normalizePerformanceImportHeader(value) {
			case "绩效活动", "绩效活动名称", "活动名称":
				headers["activity"] = column
			case "工号", "员工工号", "员工编号", "员工id", "employeeid", "userid":
				headers["employee_id"] = column
			case "姓名", "员工姓名":
				headers["name"] = column
			case "一级部门":
				headers["department_level_1"] = column
			case "二级部门":
				headers["department_level_2"] = column
			case "三级部门":
				headers["department_level_3"] = column
			case "考核上级工号", "考核上级员工工号", "考核上级编号", "上级工号", "上级员工工号", "assessmentmanageremployeeid", "assessmentmanageruserid", "manageremployeeid", "manageruserid", "managerid":
				headers["assessment_manager_employee_id"] = column
			case "考核上级姓名", "上级姓名", "assessmentmanagername", "managername":
				headers["assessment_manager_name"] = column
			case "考核上级来源", "上级来源", "assessmentmanagersource", "managersource":
				headers["assessment_manager_source"] = column
			case "调整原因", "考核上级调整原因", "上级调整原因", "原因", "manageroverridereason", "reason":
				headers["manager_override_reason"] = column
			}
		}
		if _, ok := headers["activity"]; !ok {
			continue
		}
		if _, ok := headers["employee_id"]; !ok {
			continue
		}
		return index, headers
	}
	return -1, nil
}

func normalizePerformanceImportHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsSpace(r) {
			continue
		}
		switch r {
		case ':', '：', '-', '_', '(', ')', '（', '）', '[', ']', '【', '】':
			continue
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func normalizeImportedEmployeeID(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))
	value = strings.TrimPrefix(value, "'")
	if value == "" {
		return ""
	}
	if zeroDecimalPattern.MatchString(value) {
		return strings.SplitN(value, ".", 2)[0]
	}
	if scientificPattern.MatchString(value) {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) && parsed == math.Trunc(parsed) {
			return strconv.FormatInt(int64(parsed), 10)
		}
	}
	return value
}

func valueAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return strings.TrimSpace(values[index])
}

func rowIsEmpty(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func cellColumnIndex(ref string, fallback int) int {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fallback
	}
	column := 0
	hasColumn := false
	for _, r := range ref {
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		if r < 'A' || r > 'Z' {
			break
		}
		hasColumn = true
		column = column*26 + int(r-'A'+1)
	}
	if !hasColumn {
		return fallback
	}
	return column - 1
}

var errZipEntryNotFound = errors.New("zip entry not found")

func readZipEntry(zr *zip.Reader, name string) ([]byte, error) {
	name = strings.TrimPrefix(path.Clean(strings.ReplaceAll(name, "\\", "/")), "/")
	for _, file := range zr.File {
		fileName := strings.TrimPrefix(path.Clean(strings.ReplaceAll(file.Name, "\\", "/")), "/")
		if fileName != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, errZipEntryNotFound
}

func resolveXLSXRelationshipTarget(sourceFile, target string) string {
	target = strings.ReplaceAll(strings.TrimSpace(target), "\\", "/")
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(path.Clean(target), "/")
	}
	return strings.TrimPrefix(path.Clean(path.Join(path.Dir(sourceFile), target)), "/")
}
