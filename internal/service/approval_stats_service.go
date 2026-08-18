package service

import (
	"fmt"
	"sort"
	"strings"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type ApprovalStatsSummary struct {
	Total        int    `json:"total"`
	Completed    int    `json:"completed"`
	Refused      int    `json:"refused"`
	Running      int    `json:"running"`
	Terminated   int    `json:"terminated"`
	Canceled     int    `json:"canceled"`
	ApprovalRate string `json:"approval_rate"`
}

type ApprovalTemplateStats struct {
	TemplateID   string `json:"template_id"`
	TemplateName string `json:"template_name"`
	ApprovalStatsSummary
}

type ApprovalStatsResult struct {
	Summary       ApprovalStatsSummary    `json:"summary"`
	TemplateStats []ApprovalTemplateStats `json:"template_stats"`
}

type ApprovalStatsService struct {
	approvalRepo *repository.ApprovalRepository
	templateRepo *repository.ApprovalTemplateRepository
	orgID        string
	configForOrg func(string) (dingtalk.Config, error)
}

func NewApprovalStatsServiceWithOrgID(db *gorm.DB, orgID string) *ApprovalStatsService {
	return &ApprovalStatsService{
		approvalRepo: repository.NewApprovalRepositoryWithOrgID(db, orgID),
		templateRepo: repository.NewApprovalTemplateRepositoryWithOrgID(db, orgID),
		orgID:        orgID,
		configForOrg: dingtalk.ConfigForOrgID,
	}
}

func (s *ApprovalStatsService) Get(filters map[string]string) (ApprovalStatsResult, error) {
	approvals, err := s.approvalRepo.FindAllForStats(filters, ApprovalSyncLocation())
	if err != nil {
		return ApprovalStatsResult{}, err
	}
	templates, _, err := s.templateRepo.FindAll()
	if err != nil {
		return ApprovalStatsResult{}, err
	}
	if s.configForOrg != nil {
		if config, configErr := s.configForOrg(s.orgID); configErr == nil {
			templates = mergeConfiguredApprovalTemplates(s.orgID, templates, config.ProcessCodes)
		}
	}
	templateNames := make(map[string]string, len(templates))
	for _, template := range templates {
		templateNames[strings.TrimSpace(template.TemplateID)] = strings.TrimSpace(template.Name)
	}

	groups := make(map[string]*ApprovalTemplateStats)
	result := ApprovalStatsResult{}
	for _, approval := range approvals {
		applyApprovalStatsState(&result.Summary, approval)
		code := approvalProcessCode(approval.Extension)
		if code == "" {
			code = "unknown"
		}
		group := groups[code]
		if group == nil {
			name := templateNames[code]
			if name == "" {
				name = code
			}
			group = &ApprovalTemplateStats{TemplateID: code, TemplateName: name}
			groups[code] = group
		}
		applyApprovalStatsState(&group.ApprovalStatsSummary, approval)
	}
	finalizeApprovalRate(&result.Summary)
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		group := groups[key]
		finalizeApprovalRate(&group.ApprovalStatsSummary)
		result.TemplateStats = append(result.TemplateStats, *group)
	}
	return result, nil
}

func applyApprovalStatsState(stats *ApprovalStatsSummary, approval database.Approval) {
	stats.Total++
	status := strings.ToUpper(strings.TrimSpace(approval.Status))
	result := strings.ToLower(strings.TrimSpace(approvalResultFromExtension(approval.Extension)))
	switch {
	case status == "CANCELED" || status == "CANCELLED":
		stats.Canceled++
	case status == "TERMINATED":
		stats.Terminated++
	case status == "REFUSE" || status == "REFUSED" || status == "REJECTED" || isApprovalRefusalResult(result):
		stats.Refused++
	case status == "COMPLETED":
		stats.Completed++
	case status == "RUNNING":
		stats.Running++
	default:
		stats.Running++
	}
}

func isApprovalRefusalResult(result string) bool {
	switch result {
	case "refuse", "refused", "reject", "rejected", "deny", "denied", "拒绝", "不通过":
		return true
	default:
		return false
	}
}

func approvalProcessCode(extension map[string]interface{}) string {
	for _, key := range []string{"process_code", "template_id"} {
		if value, ok := extension[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func finalizeApprovalRate(stats *ApprovalStatsSummary) {
	if stats.Total == 0 {
		stats.ApprovalRate = "0.00%"
		return
	}
	stats.ApprovalRate = fmt.Sprintf("%.2f%%", float64(stats.Completed)*100/float64(stats.Total))
}
