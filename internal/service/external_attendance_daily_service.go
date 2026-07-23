package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"
)

type ExternalAttendanceDailyQuery struct {
	StartDate    string
	EndDate      string
	UserID       string
	DepartmentID string
	Status       string
	Page         int
	PageSize     int
}

type ExternalAttendanceDailySummary struct {
	Total        int `json:"total"`
	Normal       int `json:"normal"`
	Exception    int `json:"exception"`
	WithApproval int `json:"with_approval"`
}

type ExternalAttendanceDailyPunch struct {
	CheckType      string    `json:"check_type"`
	CheckTime      time.Time `json:"check_time"`
	TimeResult     string    `json:"time_result"`
	LocationResult string    `json:"location_result"`
	UserAddress    string    `json:"user_address"`
	SourceType     string    `json:"source_type"`
}

type ExternalAttendanceDailyStatus struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Level    string `json:"level"`    // error|warning|processing|success|default
	Category string `json:"category"` // attendance|approval
}

type ExternalAttendanceDailyApproval struct {
	ProcInstID   string     `json:"proc_inst_id"`
	TagName      string     `json:"tag_name"`
	SubType      string     `json:"sub_type"`
	Label        string     `json:"label"`
	BeginTime    *time.Time `json:"begin_time,omitempty"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	Duration     string     `json:"duration"`
	DurationUnit string     `json:"duration_unit"`
}

type ExternalAttendanceDailyResult struct {
	Key             string                            `json:"key"`
	WorkDate        string                            `json:"work_date"`
	UserID          string                            `json:"user_id"`
	ExternalUserID  string                            `json:"external_user_id"`
	UserName        string                            `json:"user_name"`
	DepartmentID    string                            `json:"department_id"`
	DepartmentName  string                            `json:"department_name"`
	OnDutyTime      *time.Time                        `json:"on_duty_time,omitempty"`
	OffDutyTime     *time.Time                        `json:"off_duty_time,omitempty"`
	Punches         []ExternalAttendanceDailyPunch    `json:"punches"`
	Statuses        []ExternalAttendanceDailyStatus   `json:"statuses"`
	Approvals       []ExternalAttendanceDailyApproval `json:"approvals"`
	HasException    bool                              `json:"has_exception"`
	SourceUpdatedAt time.Time                         `json:"source_updated_at"`
	statusKeys      map[string]struct{}
	punchKeys       map[string]struct{}
	approvalKeys    map[string]struct{}
}

func (s *ExternalAttendanceSyncService) ListDailyResults(query ExternalAttendanceDailyQuery) ([]ExternalAttendanceDailyResult, int, ExternalAttendanceDailySummary, error) {
	if s == nil || s.local == nil {
		return nil, 0, ExternalAttendanceDailySummary{}, fmt.Errorf("external attendance local repository unavailable")
	}
	rows, err := s.local.ListAttendanceRawForDaily(
		query.StartDate, query.EndDate, query.UserID, query.DepartmentID,
	)
	if err != nil {
		return nil, 0, ExternalAttendanceDailySummary{}, err
	}
	if len(rows) == 0 {
		return []ExternalAttendanceDailyResult{}, 0, ExternalAttendanceDailySummary{}, nil
	}

	sourceKeys := make([]string, 0, len(rows))
	userIDs := make([]string, 0, len(rows))
	seenSource := make(map[string]struct{}, len(rows))
	seenUser := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if _, ok := seenSource[row.SourceRowKey]; !ok {
			seenSource[row.SourceRowKey] = struct{}{}
			sourceKeys = append(sourceKeys, row.SourceRowKey)
		}
		if _, ok := seenUser[row.LocalUserID]; !ok {
			seenUser[row.LocalUserID] = struct{}{}
			userIDs = append(userIDs, row.LocalUserID)
		}
	}
	links, err := s.local.ListApproveLinksBySourceRowKeys(sourceKeys)
	if err != nil {
		return nil, 0, ExternalAttendanceDailySummary{}, err
	}
	profiles, err := s.local.LoadAttendanceUserProfiles(userIDs)
	if err != nil {
		return nil, 0, ExternalAttendanceDailySummary{}, err
	}

	items := BuildExternalAttendanceDailyResults(rows, links, profiles)
	summary := summarizeExternalAttendanceDailyResults(items)
	items = filterExternalAttendanceDailyResults(items, query.Status)
	total := len(items)
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start >= total {
		return []ExternalAttendanceDailyResult{}, total, summary, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return items[start:end], total, summary, nil
}

func summarizeExternalAttendanceDailyResults(items []ExternalAttendanceDailyResult) ExternalAttendanceDailySummary {
	summary := ExternalAttendanceDailySummary{Total: len(items)}
	for _, item := range items {
		if item.HasException {
			summary.Exception++
		} else if len(item.Approvals) == 0 {
			summary.Normal++
		}
		if len(item.Approvals) > 0 {
			summary.WithApproval++
		}
	}
	return summary
}

func filterExternalAttendanceDailyResults(items []ExternalAttendanceDailyResult, status string) []ExternalAttendanceDailyResult {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" || status == "all" {
		return items
	}
	filtered := make([]ExternalAttendanceDailyResult, 0, len(items))
	for _, item := range items {
		matched := false
		switch status {
		case "normal":
			matched = !item.HasException && len(item.Approvals) == 0
		case "exception":
			matched = item.HasException
		case "approval":
			matched = len(item.Approvals) > 0
		default:
			for _, dailyStatus := range item.Statuses {
				if dailyStatus.Code == status || strings.HasPrefix(dailyStatus.Code, status+":") {
					matched = true
					break
				}
			}
		}
		if matched {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func BuildExternalAttendanceDailyResults(
	rows []database.ExternalAttendanceRaw,
	links []database.ExternalAttendanceApproveLink,
	profiles map[string]repository.ExternalAttendanceUserProfile,
) []ExternalAttendanceDailyResult {
	linksBySource := make(map[string][]database.ExternalAttendanceApproveLink)
	for _, link := range links {
		linksBySource[link.SourceRowKey] = append(linksBySource[link.SourceRowKey], link)
	}

	resultMap := make(map[string]*ExternalAttendanceDailyResult)
	for _, row := range rows {
		key := row.LocalUserID + "|" + row.WorkDate
		item := resultMap[key]
		if item == nil {
			profile := profiles[row.LocalUserID]
			userName := strings.TrimSpace(profile.UserName)
			if userName == "" {
				userName = row.LocalUserID
			}
			item = &ExternalAttendanceDailyResult{
				Key:             key,
				WorkDate:        row.WorkDate,
				UserID:          row.LocalUserID,
				ExternalUserID:  row.ExternalUserID,
				UserName:        userName,
				DepartmentID:    profile.DepartmentID,
				DepartmentName:  profile.DepartmentName,
				Punches:         []ExternalAttendanceDailyPunch{},
				Statuses:        []ExternalAttendanceDailyStatus{},
				Approvals:       []ExternalAttendanceDailyApproval{},
				statusKeys:      make(map[string]struct{}),
				punchKeys:       make(map[string]struct{}),
				approvalKeys:    make(map[string]struct{}),
				SourceUpdatedAt: row.SourceUpdatedAt,
			}
			resultMap[key] = item
		}
		if row.SourceUpdatedAt.After(item.SourceUpdatedAt) {
			item.SourceUpdatedAt = row.SourceUpdatedAt
		}

		for _, punch := range collectDailyPunches(row) {
			punchKey := punch.CheckType + "|" + punch.CheckTime.UTC().Format(time.RFC3339Nano)
			if _, exists := item.punchKeys[punchKey]; exists {
				continue
			}
			item.punchKeys[punchKey] = struct{}{}
			item.Punches = append(item.Punches, punch)
			if status, ok := dailyTimeResultStatus(punch.TimeResult); ok {
				addDailyStatus(item, status)
			}
		}
		if status, ok := dailyTimeResultStatus(row.TimeResult); ok {
			addDailyStatus(item, status)
		}

		for _, link := range linksBySource[row.SourceRowKey] {
			approval := buildDailyApproval(link)
			approvalKey := link.ItemKey
			if approvalKey == "" {
				approvalKey = link.ProcInstID + "|" + approval.Label + "|" + approval.Duration + "|" + approval.DurationUnit
			}
			if _, exists := item.approvalKeys[approvalKey]; exists {
				continue
			}
			item.approvalKeys[approvalKey] = struct{}{}
			item.Approvals = append(item.Approvals, approval)
			addDailyStatus(item, dailyApprovalStatus(link, approval.Label))
		}
	}

	items := make([]ExternalAttendanceDailyResult, 0, len(resultMap))
	for _, item := range resultMap {
		sort.Slice(item.Punches, func(i, j int) bool {
			return item.Punches[i].CheckTime.Before(item.Punches[j].CheckTime)
		})
		for i := range item.Punches {
			punch := item.Punches[i]
			switch punch.CheckType {
			case "上班":
				if item.OnDutyTime == nil || punch.CheckTime.Before(*item.OnDutyTime) {
					t := punch.CheckTime
					item.OnDutyTime = &t
				}
			case "下班":
				if item.OffDutyTime == nil || punch.CheckTime.After(*item.OffDutyTime) {
					t := punch.CheckTime
					item.OffDutyTime = &t
				}
			}
		}
		if len(item.Statuses) == 0 {
			addDailyStatus(item, ExternalAttendanceDailyStatus{
				Code: "normal", Label: "正常", Level: "success", Category: "attendance",
			})
		}
		sort.SliceStable(item.Statuses, func(i, j int) bool {
			return dailyStatusRank(item.Statuses[i]) < dailyStatusRank(item.Statuses[j])
		})
		item.statusKeys = nil
		item.punchKeys = nil
		item.approvalKeys = nil
		items = append(items, *item)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].WorkDate != items[j].WorkDate {
			return items[i].WorkDate > items[j].WorkDate
		}
		if items[i].UserName != items[j].UserName {
			return items[i].UserName < items[j].UserName
		}
		return items[i].UserID < items[j].UserID
	})
	return items
}

func collectDailyPunches(row database.ExternalAttendanceRaw) []ExternalAttendanceDailyPunch {
	punches := make([]ExternalAttendanceDailyPunch, 0, 4)
	if row.UserCheckTime != nil && !row.UserCheckTime.IsZero() {
		punches = append(punches, ExternalAttendanceDailyPunch{
			CheckType:      normalizeExternalCheckType(row.CheckType),
			CheckTime:      *row.UserCheckTime,
			TimeResult:     row.TimeResult,
			LocationResult: row.LocationResult,
			UserAddress:    row.UserAddress,
			SourceType:     row.SourceType,
		})
	}

	resultCount := 0
	if raw := strings.TrimSpace(row.AttendanceResultListJSON); raw != "" && raw != "null" {
		var list []map[string]interface{}
		if json.Unmarshal([]byte(raw), &list) == nil {
			for _, entry := range list {
				t := parseFlexibleTime(stringField(entry, "userCheckTime", "user_check_time", "checkTime", "check_time"))
				checkType := stringField(entry, "checkType", "check_type")
				if t == nil || checkType == "" {
					continue
				}
				punches = append(punches, ExternalAttendanceDailyPunch{
					CheckType:      normalizeExternalCheckType(checkType),
					CheckTime:      *t,
					TimeResult:     defaultStr(stringField(entry, "timeResult", "time_result"), row.TimeResult),
					LocationResult: defaultStr(stringField(entry, "locationResult", "location_result"), row.LocationResult),
					UserAddress:    defaultStr(stringField(entry, "userAddress", "user_address"), row.UserAddress),
					SourceType:     defaultStr(stringField(entry, "sourceType", "source_type"), row.SourceType),
				})
				resultCount++
			}
		}
	}
	if resultCount == 0 {
		if raw := strings.TrimSpace(row.CheckRecordListJSON); raw != "" && raw != "null" {
			var list []map[string]interface{}
			if json.Unmarshal([]byte(raw), &list) == nil {
				for _, entry := range list {
					t := parseFlexibleTime(stringField(entry, "userCheckTime", "user_check_time", "checkTime", "check_time"))
					checkType := stringField(entry, "checkType", "check_type")
					if t == nil || checkType == "" {
						continue
					}
					punches = append(punches, ExternalAttendanceDailyPunch{
						CheckType:      normalizeExternalCheckType(checkType),
						CheckTime:      *t,
						TimeResult:     defaultStr(stringField(entry, "timeResult", "time_result"), row.TimeResult),
						LocationResult: defaultStr(stringField(entry, "locationResult", "location_result"), row.LocationResult),
						UserAddress:    defaultStr(stringField(entry, "userAddress", "user_address"), row.UserAddress),
						SourceType:     defaultStr(stringField(entry, "sourceType", "source_type"), row.SourceType),
					})
				}
			}
		}
	}
	return punches
}

func dailyTimeResultStatus(value string) (ExternalAttendanceDailyStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "late":
		return ExternalAttendanceDailyStatus{Code: "late", Label: "迟到", Level: "warning", Category: "attendance"}, true
	case "seriouslate", "serious_late":
		return ExternalAttendanceDailyStatus{Code: "serious_late", Label: "严重迟到", Level: "error", Category: "attendance"}, true
	case "early":
		return ExternalAttendanceDailyStatus{Code: "early", Label: "早退", Level: "warning", Category: "attendance"}, true
	case "notsigned", "not_signed":
		return ExternalAttendanceDailyStatus{Code: "not_signed", Label: "缺卡", Level: "error", Category: "attendance"}, true
	case "absenteeism", "absent":
		return ExternalAttendanceDailyStatus{Code: "absenteeism", Label: "旷工", Level: "error", Category: "attendance"}, true
	default:
		return ExternalAttendanceDailyStatus{}, false
	}
}

func buildDailyApproval(link database.ExternalAttendanceApproveLink) ExternalAttendanceDailyApproval {
	base := strings.TrimSpace(link.TagName)
	if base == "请假" && strings.TrimSpace(link.SubType) != "" {
		base = strings.TrimSpace(link.SubType)
	}
	label := base
	if duration := formatDailyDuration(link.Duration, link.DurationUnit); duration != "" {
		label = strings.TrimSpace(label + " " + duration)
	}
	if label == "" {
		label = "审批"
	}
	return ExternalAttendanceDailyApproval{
		ProcInstID:   link.ProcInstID,
		TagName:      link.TagName,
		SubType:      link.SubType,
		Label:        label,
		BeginTime:    link.BeginTime,
		EndTime:      link.EndTime,
		Duration:     link.Duration,
		DurationUnit: link.DurationUnit,
	}
}

func dailyApprovalStatus(link database.ExternalAttendanceApproveLink, label string) ExternalAttendanceDailyStatus {
	code := "approval"
	level := "processing"
	switch strings.TrimSpace(link.TagName) {
	case "请假":
		code = "leave:" + strings.TrimSpace(link.SubType)
	case "出差":
		code = "business_trip"
	case "外出":
		code = "outing"
	case "补卡申请":
		code = "card_correction"
		level = "default"
	case "加班":
		code = "overtime"
		level = "default"
	}
	return ExternalAttendanceDailyStatus{Code: code, Label: label, Level: level, Category: "approval"}
}

func formatDailyDuration(value, unit string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if n, err := strconv.ParseFloat(value, 64); err == nil {
		value = strconv.FormatFloat(n, 'f', -1, 64)
	}
	unitLabel := strings.TrimSpace(unit)
	switch strings.ToUpper(unitLabel) {
	case "HOUR", "HOURS":
		unitLabel = "小时"
	case "DAY", "DAYS":
		unitLabel = "天"
	case "MINUTE", "MINUTES":
		unitLabel = "分钟"
	}
	return value + unitLabel
}

func addDailyStatus(item *ExternalAttendanceDailyResult, status ExternalAttendanceDailyStatus) {
	if item == nil || strings.TrimSpace(status.Label) == "" {
		return
	}
	key := status.Code + "|" + status.Label
	if _, exists := item.statusKeys[key]; exists {
		return
	}
	item.statusKeys[key] = struct{}{}
	item.Statuses = append(item.Statuses, status)
	if status.Category == "attendance" && status.Code != "normal" {
		item.HasException = true
	}
}

func dailyStatusRank(status ExternalAttendanceDailyStatus) int {
	switch status.Code {
	case "absenteeism":
		return 10
	case "not_signed":
		return 20
	case "serious_late":
		return 30
	case "late":
		return 40
	case "early":
		return 50
	case "normal":
		return 1000
	default:
		if status.Category == "approval" {
			return 100
		}
		return 500
	}
}
