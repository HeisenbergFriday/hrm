package service

import (
	"sort"
	"strings"

	"peopleops/internal/database"
)

var approvalTemplateLabels = map[string]string{
	"leave":                 "请假审批",
	"overtime":              "加班审批",
	"attendance_correction": "补卡审批",
	"punch_fix":             "补卡审批",
	"position_transfer":     "转岗审批",
	"expense":               "报销审批",
	"business_trip":         "出差审批",
	"outing":                "外出审批",
}

// mergeConfiguredApprovalTemplates completes the local template catalog from the
// current organization's configured approval-process whitelist. Persisted rows
// win because they may contain richer form and flow metadata.
func mergeConfiguredApprovalTemplates(orgID string, existing []database.ApprovalTemplate, configured map[string]string) []database.ApprovalTemplate {
	templates := append([]database.ApprovalTemplate(nil), existing...)
	seen := make(map[string]struct{}, len(existing)+len(configured))
	for _, template := range existing {
		if code := strings.TrimSpace(template.TemplateID); code != "" {
			seen[code] = struct{}{}
		}
	}

	keys := make([]string, 0, len(configured))
	for key := range configured {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, rawKey := range keys {
		code := strings.TrimSpace(configured[rawKey])
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(rawKey))
		name := approvalTemplateLabels[key]
		if name == "" {
			name = code
		}
		templates = append(templates, database.ApprovalTemplate{
			OrgID:       strings.TrimSpace(orgID),
			TemplateID:  code,
			Name:        name,
			Description: "当前企业已配置的钉钉审批流程",
			Category:    approvalTemplateCategory(key),
			Status:      "active",
			Extension: map[string]interface{}{
				"source":     "organization_config",
				"config_key": key,
			},
		})
		seen[code] = struct{}{}
	}

	sort.SliceStable(templates, func(i, j int) bool {
		leftName := strings.TrimSpace(templates[i].Name)
		rightName := strings.TrimSpace(templates[j].Name)
		if leftName == rightName {
			return strings.TrimSpace(templates[i].TemplateID) < strings.TrimSpace(templates[j].TemplateID)
		}
		return leftName < rightName
	})
	return templates
}

func approvalTemplateCategory(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "leave":
		return string(ApprovalCategoryLeave)
	case "overtime":
		return string(ApprovalCategoryOvertime)
	case "attendance_correction", "punch_fix":
		return string(ApprovalCategoryPunchFix)
	case "expense":
		return string(ApprovalCategoryExpense)
	case "business_trip":
		return string(ApprovalCategoryBusinessTrip)
	case "outing":
		return string(ApprovalCategoryOuting)
	default:
		return string(ApprovalCategoryOther)
	}
}
