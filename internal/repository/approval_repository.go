package repository

import (
	"encoding/json"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type ApprovalRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewApprovalRepository(db *gorm.DB) *ApprovalRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &ApprovalRepository{db: db, orgID: orgID, orgErr: err}
}

func NewApprovalRepositoryWithOrgID(db *gorm.DB, orgID string) *ApprovalRepository {
	normalized, err := RequireOrgID(orgID)
	return &ApprovalRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *ApprovalRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func (r *ApprovalRepository) scoped() *gorm.DB {
	orgID, err := r.requireOrgID()
	if err != nil {
		// Fail closed: never return unscoped tenant rows.
		return r.db.Where("1 = 0")
	}
	return r.db.Where("org_id = ?", orgID)
}

func (r *ApprovalRepository) Create(approval *database.Approval) error {
	if approval == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, approval.OrgID)
	if err != nil {
		return err
	}
	approval.OrgID = merged
	return r.createApproval(approval)
}

// UpsertByOrgProcessID creates or updates an approval by (org_id, process_id).
// Existing Title/ApplicantID/ApplicantName/CreateTime are preserved when incoming values are empty.
func (r *ApprovalRepository) UpsertByOrgProcessID(approval *database.Approval) error {
	if approval == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, approval.OrgID)
	if err != nil {
		return err
	}
	approval.OrgID = merged
	approval.ProcessID = strings.TrimSpace(approval.ProcessID)
	if approval.ProcessID == "" {
		return gorm.ErrInvalidData
	}

	var existing database.Approval
	err = r.db.Where("org_id = ? AND process_id = ?", approval.OrgID, approval.ProcessID).First(&existing).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return r.createApproval(approval)
		}
		return err
	}

	if approval.Title != "" {
		existing.Title = approval.Title
	}
	if approval.ApplicantID != "" {
		existing.ApplicantID = approval.ApplicantID
	}
	if incomingName := strings.TrimSpace(approval.ApplicantName); incomingName != "" {
		existingName := strings.TrimSpace(existing.ApplicantName)
		incomingIsFallback := incomingName == strings.TrimSpace(approval.ApplicantID)
		existingIsFallback := existingName == "" || existingName == strings.TrimSpace(existing.ApplicantID)
		if !incomingIsFallback || existingIsFallback {
			existing.ApplicantName = incomingName
		}
	}
	if !approval.CreateTime.IsZero() {
		existing.CreateTime = approval.CreateTime
	}
	if approval.Status != "" {
		existing.Status = approval.Status
	}
	if !approval.FinishTime.IsZero() {
		existing.FinishTime = approval.FinishTime
	}
	if approval.Content != nil {
		existing.Content = approval.Content
	}
	if approval.Extension != nil {
		existing.Extension = mergeApprovalExtension(existing.Extension, approval.Extension)
	}
	return r.db.Save(&existing).Error
}

// createApproval inserts a new approval row.
// RUNNING instances often have empty finish_time; omit the zero time.Time so MySQL
// does not reject '0000-00-00' under strict datetime mode.
func (r *ApprovalRepository) createApproval(approval *database.Approval) error {
	if approval == nil {
		return gorm.ErrInvalidData
	}
	tx := r.db
	if approval.FinishTime.IsZero() {
		tx = tx.Omit("FinishTime")
	}
	return tx.Create(approval).Error
}

func mergeApprovalExtension(base, patch map[string]interface{}) map[string]interface{} {
	if base == nil && patch == nil {
		return nil
	}
	out := make(map[string]interface{}, len(base)+len(patch))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		out[k] = v
	}
	return out
}

func (r *ApprovalRepository) FindByID(id string) (*database.Approval, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, err
	}
	var approval database.Approval
	err := r.scoped().First(&approval, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	decorateApproval(&approval)
	return &approval, nil
}

func (r *ApprovalRepository) FindByUintID(id uint) (*database.Approval, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, err
	}
	var approval database.Approval
	err := r.scoped().First(&approval, id).Error
	if err != nil {
		return nil, err
	}
	return &approval, nil
}

func (r *ApprovalRepository) FindByProcessID(processID string) (*database.Approval, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, err
	}
	var approval database.Approval
	err := r.scoped().Where("process_id = ?", strings.TrimSpace(processID)).First(&approval).Error
	if err != nil {
		return nil, err
	}
	return &approval, nil
}

func (r *ApprovalRepository) FindAll(page, pageSize int, filters map[string]string) ([]database.Approval, int64, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, 0, err
	}
	var approvals []database.Approval
	var total int64

	query := r.scoped().Model(&database.Approval{})
	query = applyApprovalFilters(query, filters)
	needsBusinessDateFilter := hasApprovalBusinessDateFilter(filters)

	if needsBusinessDateFilter {
		loadQuery := query
		if !isBusinessSortField(approvalSortField(filters)) {
			loadQuery = loadQuery.Order(approvalOrder(filters))
		}
		if err := loadQuery.Find(&approvals).Error; err != nil {
			return nil, 0, err
		}
		decorateApprovals(approvals)
		approvals = filterApprovalsByBusinessDate(approvals, filters)
		total = int64(len(approvals))
		if isBusinessSortField(approvalSortField(filters)) {
			sortApprovalsByBusinessTime(approvals, approvalSortField(filters), approvalSortAscending(filters))
		}
		return paginateApprovals(approvals, page, pageSize), total, nil
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if isBusinessSortField(approvalSortField(filters)) {
		if err := query.Find(&approvals).Error; err != nil {
			return nil, 0, err
		}
		decorateApprovals(approvals)
		sortApprovalsByBusinessTime(approvals, approvalSortField(filters), approvalSortAscending(filters))
		approvals = paginateApprovals(approvals, page, pageSize)
		return approvals, total, nil
	}

	offset := (page - 1) * pageSize
	if err := query.Order(approvalOrder(filters)).Offset(offset).Limit(pageSize).Find(&approvals).Error; err != nil {
		return nil, 0, err
	}
	decorateApprovals(approvals)
	return approvals, total, nil
}

func applyApprovalFilters(query *gorm.DB, filters map[string]string) *gorm.DB {
	if v, ok := filters["status"]; ok && v != "" {
		query = query.Where("status = ?", v)
	}
	if v, ok := filters["template_id"]; ok && v != "" {
		query = query.Where("extension->>'$.process_code' = ? OR extension->>'$.template_id' = ?", v, v)
	}
	if v, ok := filters["applicant_id"]; ok && v != "" {
		query = query.Where("applicant_id = ?", v)
	}
	if v, ok := filters["title"]; ok && v != "" {
		query = query.Where("title LIKE ?", "%"+v+"%")
	}
	return query
}

func parseApprovalDateFilter(filters map[string]string, key string) (time.Time, bool) {
	value := strings.TrimSpace(filters[key])
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, dingtalk.ApprovalBusinessLocation())
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func hasApprovalBusinessDateFilter(filters map[string]string) bool {
	_, hasStart := parseApprovalDateFilter(filters, "start_date")
	_, hasEnd := parseApprovalDateFilter(filters, "end_date")
	return hasStart || hasEnd
}

func filterApprovalsByBusinessDate(approvals []database.Approval, filters map[string]string) []database.Approval {
	startDate, hasStart := parseApprovalDateFilter(filters, "start_date")
	endDate, hasEnd := parseApprovalDateFilter(filters, "end_date")
	if !hasStart && !hasEnd {
		return approvals
	}

	filtered := make([]database.Approval, 0, len(approvals))
	for _, approval := range approvals {
		businessStart, startOK := parseApprovalTime(approval.BusinessStartTime)
		businessEnd, endOK := parseApprovalTime(approval.BusinessEndTime)
		if hasStart && (!startOK || businessStart.Before(startDate)) {
			continue
		}
		if hasEnd && (!endOK || !businessEnd.Before(endDate.AddDate(0, 0, 1))) {
			continue
		}
		filtered = append(filtered, approval)
	}
	return filtered
}

func approvalSortField(filters map[string]string) string {
	switch strings.TrimSpace(filters["sort_field"]) {
	case "finish_time":
		return "finish_time"
	case "business_start_time":
		return "business_start_time"
	case "business_end_time":
		return "business_end_time"
	default:
		return "create_time"
	}
}

func approvalSortAscending(filters map[string]string) bool {
	return strings.EqualFold(strings.TrimSpace(filters["sort_order"]), "asc")
}

func isBusinessSortField(field string) bool {
	return field == "business_start_time" || field == "business_end_time"
}

func approvalOrder(filters map[string]string) string {
	field := approvalSortField(filters)
	direction := "DESC"
	if approvalSortAscending(filters) {
		direction = "ASC"
	}
	return field + " " + direction + ", id " + direction
}

func paginateApprovals(approvals []database.Approval, page, pageSize int) []database.Approval {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	start := (page - 1) * pageSize
	if start >= len(approvals) {
		return []database.Approval{}
	}
	end := start + pageSize
	if end > len(approvals) {
		end = len(approvals)
	}
	return approvals[start:end]
}

// FindAllForStats returns the complete, narrow projection required for server-side
// aggregation. It intentionally has no page limit, so totals cannot truncate at 10,000.
func (r *ApprovalRepository) FindAllForStats(filters map[string]string, location *time.Location) ([]database.Approval, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, err
	}
	query := r.scoped().Model(&database.Approval{}).Select("status", "extension")
	if v := strings.TrimSpace(filters["template_id"]); v != "" {
		query = query.Where("extension->>'$.process_code' = ? OR extension->>'$.template_id' = ? OR extension LIKE ?", v, v, "%"+v+"%")
	}
	if v := strings.TrimSpace(filters["start_date"]); v != "" {
		start, err := time.ParseInLocation("2006-01-02", v, location)
		if err != nil {
			return nil, err
		}
		query = query.Where("create_time >= ?", start)
	}
	if v := strings.TrimSpace(filters["end_date"]); v != "" {
		end, err := time.ParseInLocation("2006-01-02", v, location)
		if err != nil {
			return nil, err
		}
		query = query.Where("create_time < ?", end.AddDate(0, 0, 1))
	}
	var approvals []database.Approval
	if err := query.Find(&approvals).Error; err != nil {
		return nil, err
	}
	return approvals, nil
}

// FindAllByTitleKeywords 与 FindAll 逻辑一致，只是先按标题关键字命中/排除过滤。
// include=true 时匹配 title LIKE 任一关键字（用于具体分类）；
// include=false 时排除所有关键字（用于 "other" 分类）。
// keywords 为空且 include=true 时返回空结果集，避免退化为无条件查询。
func (r *ApprovalRepository) FindAllByTitleKeywords(page, pageSize int, keywords []string, include bool, filters map[string]string) ([]database.Approval, int64, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, 0, err
	}
	if include && len(keywords) == 0 {
		return []database.Approval{}, 0, nil
	}
	var approvals []database.Approval
	var total int64

	query := r.scoped().Model(&database.Approval{})
	if len(keywords) > 0 {
		clauses := make([]string, 0, len(keywords))
		args := make([]interface{}, 0, len(keywords))
		for _, kw := range keywords {
			if strings.TrimSpace(kw) == "" {
				continue
			}
			clauses = append(clauses, "title LIKE ?")
			args = append(args, "%"+kw+"%")
		}
		if len(clauses) > 0 {
			combined := strings.Join(clauses, " OR ")
			if include {
				query = query.Where(combined, args...)
			} else {
				query = query.Where("NOT ("+combined+")", args...)
			}
		}
	}
	query = applyApprovalFilters(query, filters)
	needsBusinessDateFilter := hasApprovalBusinessDateFilter(filters)

	if needsBusinessDateFilter {
		loadQuery := query
		if !isBusinessSortField(approvalSortField(filters)) {
			loadQuery = loadQuery.Order(approvalOrder(filters))
		}
		if err := loadQuery.Find(&approvals).Error; err != nil {
			return nil, 0, err
		}
		decorateApprovals(approvals)
		approvals = filterApprovalsByBusinessDate(approvals, filters)
		total = int64(len(approvals))
		if isBusinessSortField(approvalSortField(filters)) {
			sortApprovalsByBusinessTime(approvals, approvalSortField(filters), approvalSortAscending(filters))
		}
		return paginateApprovals(approvals, page, pageSize), total, nil
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if isBusinessSortField(approvalSortField(filters)) {
		if err := query.Find(&approvals).Error; err != nil {
			return nil, 0, err
		}
		decorateApprovals(approvals)
		sortApprovalsByBusinessTime(approvals, approvalSortField(filters), approvalSortAscending(filters))
		approvals = paginateApprovals(approvals, page, pageSize)
		return approvals, total, nil
	}

	offset := (page - 1) * pageSize
	if err := query.Order(approvalOrder(filters)).Offset(offset).Limit(pageSize).Find(&approvals).Error; err != nil {
		return nil, 0, err
	}
	decorateApprovals(approvals)
	return approvals, total, nil
}

var approvalBusinessStartAliases = map[string]struct{}{
	"开始时间": {}, "开始日期": {}, "请假开始时间": {}, "加班开始时间": {},
	"加班开始日期": {}, "外出开始时间": {}, "出差开始时间": {}, "出发时间": {},
	"starttime": {}, "start": {}, "startdate": {}, "startdatetime": {},
	"start_time": {}, "overtime_start_time": {}, "leave_start_time": {},
	"business_start_time": {}, "from": {}, "_from": {}, "punch_time": {},
	"补卡时间": {}, "打卡时间": {},
}

var approvalBusinessEndAliases = map[string]struct{}{
	"结束时间": {}, "结束日期": {}, "请假结束时间": {}, "加班结束时间": {},
	"加班结束日期": {}, "外出结束时间": {}, "出差结束时间": {}, "返程时间": {},
	"endtime": {}, "finishtime": {}, "end": {}, "enddate": {}, "enddatetime": {},
	"end_time": {}, "overtime_end_time": {}, "leave_end_time": {},
	"business_end_time": {}, "to": {}, "_to": {},
}

func decorateApprovals(approvals []database.Approval) {
	for i := range approvals {
		decorateApproval(&approvals[i])
	}
}

func decorateApproval(approval *database.Approval) {
	if approval == nil {
		return
	}
	approval.BusinessStartTime, approval.BusinessEndTime = extractApprovalBusinessTimes(approval.Content)
}

func sortApprovalsByBusinessTime(approvals []database.Approval, field string, ascending bool) {
	sort.SliceStable(approvals, func(i, j int) bool {
		left := approvalBusinessTime(approvals[i], field)
		right := approvalBusinessTime(approvals[j], field)
		if left.valid != right.valid {
			return left.valid
		}
		if !left.valid {
			return approvals[i].ID < approvals[j].ID
		}
		if !left.value.Equal(right.value) {
			if ascending {
				return left.value.Before(right.value)
			}
			return left.value.After(right.value)
		}
		if ascending {
			return approvals[i].ID < approvals[j].ID
		}
		return approvals[i].ID > approvals[j].ID
	})
}

type approvalBusinessTimeValue struct {
	value time.Time
	valid bool
}

func approvalBusinessTime(approval database.Approval, field string) approvalBusinessTimeValue {
	value := approval.BusinessStartTime
	if field == "business_end_time" {
		value = approval.BusinessEndTime
	}
	parsed, ok := parseApprovalTime(value)
	return approvalBusinessTimeValue{value: parsed, valid: ok}
}

func extractApprovalBusinessTimes(content map[string]interface{}) (string, string) {
	var start, end string
	var visit func(label string, value interface{})
	visit = func(label string, value interface{}) {
		if start != "" && end != "" {
			return
		}
		if raw, ok := value.(string); ok {
			trimmed := strings.TrimSpace(raw)
			if (strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{")) && json.Valid([]byte(trimmed)) {
				var parsed interface{}
				if json.Unmarshal([]byte(trimmed), &parsed) == nil {
					visit(label, parsed)
					return
				}
			}
			if isCombinedBusinessLabel(label) {
				return
			}
			if parsed, ok := normalizeApprovalTime(trimmed); ok {
				switch approvalBusinessLabelType(label) {
				case "start":
					if start == "" {
						start = parsed
					}
				case "end":
					if end == "" {
						end = parsed
					}
				}
			}
			return
		}
		if parsed, ok := normalizeApprovalTime(value); ok {
			switch approvalBusinessLabelType(label) {
			case "start":
				if start == "" {
					start = parsed
				}
			case "end":
				if end == "" {
					end = parsed
				}
			}
			return
		}
		switch typed := value.(type) {
		case []interface{}:
			if isCombinedBusinessLabel(label) && len(typed) >= 2 {
				if parsed, ok := normalizeApprovalTime(typed[0]); ok && start == "" {
					start = parsed
				}
				if parsed, ok := normalizeApprovalTime(typed[1]); ok && end == "" {
					end = parsed
				}
				return
			}
			for _, item := range typed {
				visit(label, item)
			}
		case map[string]interface{}:
			alias := label
			for _, key := range []string{"bizAlias", "biz_alias", "name", "label", "key", "title", "componentName"} {
				if candidate, ok := typed[key].(string); ok && strings.TrimSpace(candidate) != "" {
					alias = candidate
					break
				}
			}
			if props, ok := typed["props"].(map[string]interface{}); ok {
				if candidate, ok := props["bizAlias"].(string); ok && strings.TrimSpace(candidate) != "" {
					alias = candidate
				}
			}
			for _, key := range []string{"value", "values", "content", "selectedValue", "date", "startTime", "finishTime"} {
				if nested, exists := typed[key]; exists {
					visit(alias, nested)
				}
			}
			for key, nested := range typed {
				if key != "value" && key != "values" && key != "content" && key != "selectedValue" && key != "date" && key != "startTime" && key != "finishTime" && key != "props" {
					visit(key, nested)
				}
			}
		}
	}
	for key, value := range content {
		visit(key, value)
	}
	return start, end
}

func normalizedApprovalLabel(label string) string {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return ""
	}
	var parsed interface{}
	if (strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{")) && json.Unmarshal([]byte(trimmed), &parsed) == nil {
		switch typed := parsed.(type) {
		case []interface{}:
			parts := make([]string, 0, len(typed))
			for _, item := range typed {
				if part, ok := item.(string); ok {
					parts = append(parts, part)
				}
			}
			if len(parts) > 0 {
				trimmed = strings.Join(parts, "/")
			}
		case map[string]interface{}:
			for _, key := range []string{"label", "name", "key", "bizAlias"} {
				if candidate, ok := typed[key].(string); ok {
					trimmed = candidate
					break
				}
			}
		}
	}
	return strings.ToLower(strings.NewReplacer(" ", "", "　", "", "-", "", "—", "", ":", "", "：", "").Replace(trimmed))
}

func isCombinedBusinessLabel(label string) bool {
	normalized := normalizedApprovalLabel(label)
	return (strings.Contains(normalized, "开始") && strings.Contains(normalized, "结束")) ||
		(strings.Contains(normalized, "start") && strings.Contains(normalized, "end"))
}

func approvalBusinessLabelType(label string) string {
	normalized := normalizedApprovalLabel(label)
	if normalized == "" {
		return ""
	}
	if _, ok := approvalBusinessStartAliases[normalized]; ok {
		return "start"
	}
	if _, ok := approvalBusinessEndAliases[normalized]; ok {
		return "end"
	}
	if isCombinedBusinessLabel(normalized) {
		return ""
	}
	if strings.Contains(normalized, "开始") || strings.Contains(normalized, "start") || strings.Contains(normalized, "出发") {
		return "start"
	}
	if strings.Contains(normalized, "结束") || strings.Contains(normalized, "end") || strings.Contains(normalized, "返程") {
		return "end"
	}
	return ""
}

func normalizeApprovalTime(value interface{}) (string, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed.In(dingtalk.ApprovalBusinessLocation()).Format("2006-01-02 15:04:05"), true
	case float64:
		if typed > 100000000000 {
			typed /= 1000
		}
		if typed > 1000000000 && typed < 100000000000 {
			return time.Unix(int64(typed), 0).In(dingtalk.ApprovalBusinessLocation()).Format("2006-01-02 15:04:05"), true
		}
		return "", false
	case json.Number:
		return normalizeApprovalTimeString(string(typed))
	case string:
		return normalizeApprovalTimeString(typed)
	default:
		return "", false
	}
}

func normalizeApprovalTimeString(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return normalizeApprovalTime(float64(parsed))
	}
	layouts := []string{
		time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04",
		"2006/01/02 15:04:05", "2006/01/02 15:04", "2006-01-02",
		"2006/01/02",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, dingtalk.ApprovalBusinessLocation()); err == nil {
			if layout == "2006-01-02" || layout == "2006/01/02" {
				return parsed.Format("2006-01-02"), true
			}
			return parsed.In(dingtalk.ApprovalBusinessLocation()).Format("2006-01-02 15:04:05"), true
		}
	}
	return "", false
}

func parseApprovalTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, dingtalk.ApprovalBusinessLocation()); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// ApprovalTemplate Repository

type ApprovalTemplateRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewApprovalTemplateRepository(db *gorm.DB) *ApprovalTemplateRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &ApprovalTemplateRepository{db: db, orgID: orgID, orgErr: err}
}

func NewApprovalTemplateRepositoryWithOrgID(db *gorm.DB, orgID string) *ApprovalTemplateRepository {
	normalized, err := RequireOrgID(orgID)
	return &ApprovalTemplateRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *ApprovalTemplateRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func (r *ApprovalTemplateRepository) scoped() *gorm.DB {
	orgID, err := r.requireOrgID()
	if err != nil {
		return r.db.Where("1 = 0")
	}
	return r.db.Where("org_id = ?", orgID)
}

func (r *ApprovalTemplateRepository) Create(template *database.ApprovalTemplate) error {
	if template == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, template.OrgID)
	if err != nil {
		return err
	}
	template.OrgID = merged
	return r.db.Create(template).Error
}

func (r *ApprovalTemplateRepository) FindAll() ([]database.ApprovalTemplate, int64, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, 0, err
	}
	var templates []database.ApprovalTemplate
	var total int64

	if err := r.scoped().Model(&database.ApprovalTemplate{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := r.scoped().Find(&templates).Error; err != nil {
		return nil, 0, err
	}

	return templates, total, nil
}
