package service

import (
	"strings"
)

// ApprovalCategoryKey 审批流程分类白名单键。
type ApprovalCategoryKey string

const (
	ApprovalCategoryLeave        ApprovalCategoryKey = "leave"
	ApprovalCategoryOvertime     ApprovalCategoryKey = "overtime"
	ApprovalCategoryExpense      ApprovalCategoryKey = "expense"
	ApprovalCategoryBusinessTrip ApprovalCategoryKey = "business_trip"
	ApprovalCategoryOuting       ApprovalCategoryKey = "outing"
	ApprovalCategoryPunchFix     ApprovalCategoryKey = "punch_fix"
	ApprovalCategoryOther        ApprovalCategoryKey = "other"
)

// approvalCategoryKeywords 分类到审批标题关键字的映射。
// 采用标题匹配是因为库里 extension->>'$.template_id' 大量为空，模板同步也不总跟得上；
// 而钉钉审批实例的 title 通常已带"请假/加班/补卡"等语义词。
var approvalCategoryKeywords = map[ApprovalCategoryKey][]string{
	ApprovalCategoryLeave:        {"请假", "年假", "病假", "事假", "调休", "婚假", "产假", "陪产", "丧假", "休假"},
	ApprovalCategoryOvertime:     {"加班"},
	ApprovalCategoryExpense:      {"报销", "费用"},
	ApprovalCategoryBusinessTrip: {"出差"},
	ApprovalCategoryOuting:       {"外出"},
	ApprovalCategoryPunchFix:     {"补卡", "打卡异常"},
}

// ParseApprovalCategory 将客户端传入字符串归一为白名单分类键。ok=false 表示未命中白名单，调用方应忽略该参数。
func ParseApprovalCategory(raw string) (ApprovalCategoryKey, bool) {
	key := ApprovalCategoryKey(strings.ToLower(strings.TrimSpace(raw)))
	switch key {
	case ApprovalCategoryLeave, ApprovalCategoryOvertime, ApprovalCategoryExpense,
		ApprovalCategoryBusinessTrip, ApprovalCategoryOuting, ApprovalCategoryPunchFix, ApprovalCategoryOther:
		return key, true
	}
	return "", false
}

// ApprovalCategoryTitleKeywords 返回分类的标题关键字集合。空切片表示"未知分类"，调用方应忽略。
func ApprovalCategoryTitleKeywords(key ApprovalCategoryKey) []string {
	return approvalCategoryKeywords[key]
}

// AllApprovalCategoryTitleKeywords 返回所有已知分类的关键字并集，用于 "other" 分类的排除匹配。
func AllApprovalCategoryTitleKeywords() []string {
	total := 0
	for _, kws := range approvalCategoryKeywords {
		total += len(kws)
	}
	out := make([]string, 0, total)
	for _, kws := range approvalCategoryKeywords {
		out = append(out, kws...)
	}
	return out
}
