package service

import (
	"peopleops/internal/database"
	"strings"
)

func filterAttendanceRecordsForCalculation(records []database.Attendance, allowApproveClockRecord bool) []database.Attendance {
	filtered := make([]database.Attendance, 0, len(records))
	for _, record := range records {
		if isAttendanceRecordValid(record, allowApproveClockRecord) {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func isAttendanceRecordValid(record database.Attendance, allowApproveClockRecord bool) bool {
	if record.CheckTime.IsZero() {
		return false
	}

	ext := record.Extension
	if ext == nil {
		ext = map[string]interface{}{}
	}

	if strings.EqualFold(strings.TrimSpace(stringValue(ext["isLegal"])), "N") {
		return false
	}
	if strings.TrimSpace(stringValue(ext["invalidRecordType"])) != "" {
		return false
	}

	invalidMessage := strings.ToLower(strings.TrimSpace(stringValue(ext["invalidRecordMsg"])))
	if strings.Contains(invalidMessage, "无效") ||
		strings.Contains(invalidMessage, "作弊") ||
		strings.Contains(invalidMessage, "待确认") ||
		strings.Contains(invalidMessage, "二次确认") {
		return false
	}

	sourceType := strings.TrimSpace(stringValue(ext["sourceType"]))
	if sourceType == "" {
		sourceType = "USER"
	}
	if strings.EqualFold(sourceType, "USER") {
		return true
	}
	return allowApproveClockRecord && strings.EqualFold(sourceType, "APPROVE")
}

func stringValue(raw interface{}) string {
	if raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return value
	default:
		return ""
	}
}
