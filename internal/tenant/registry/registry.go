// Package registry 定义多租户改造中"哪些表需要 org_id、哪些是平台全局表"的
// 单一权威清单。业务代码、CI 检查、迁移工具都应从这里读取，避免手工重复维护。
//
// 分类说明：
//   - TenantTable        : 属于某个组织；必须带 org_id 且业务写入时校验组织。
//   - MembershipTable    : 描述"用户 × 组织"关系，跨组织存在，但每行仍绑定一个 org_id。
//   - PlatformTable      : 平台级共享，不属于任何单一组织（例如 organizations 元数据、
//     全局 RBAC 定义、迁移运行记录）。禁止无脑加 org_id。
//
// 每张表都写明"当前是否已有 org_id 列"与"迁移优先级"。后续 discover 命令会拿它
// 与 information_schema 交叉，产出真实的差异报告。
package registry

// Kind 表征某张表在多租户模型中的角色。
type Kind int

const (
	// KindTenant 组织私有业务/主数据表；每一行都必须归属某个 org_id。
	KindTenant Kind = iota + 1
	// KindMembership 描述用户 × 组织关系；每一行绑定一个 org_id，但语义上跨组织存在。
	KindMembership
	// KindPlatform 平台级共享或全局定义表；不添加 org_id。
	KindPlatform
)

// Priority 表征当前分阶段迁移里的相对紧急度，供 report/apply 排序输出。
type Priority int

const (
	PriorityP0 Priority = 0 // 主数据/身份/审计：先解决，跨企业风险最大。
	PriorityP1 Priority = 1 // 审批与员工生命周期。
	PriorityP2 Priority = 2 // 考勤、排班、节假日、假期规则、加班/调休。
	PriorityP3 Priority = 3 // 绩效全链路。
)

// Table 描述一张表的多租户属性。
type Table struct {
	Name        string   // 数据库表名
	Kind        Kind     // 分类
	Priority    Priority // 优先级（仅 KindTenant/KindMembership 有意义）
	HasOrgID    bool     // 当前代码/schema 中是否已带 org_id 列（基线快照）
	ParentTable string   // 若存在强父表，用于 infer 阶段的归属传导；例如 sections -> templates
	Notes       string   // 供 report 展示的备注（例如"孤立配置表，需要显式克隆"）
}

// All 返回全部 tenant/membership/platform 表；顺序稳定，供 discover/report 输出一致。
func All() []Table {
	all := make([]Table, 0, len(entries))
	all = append(all, entries...)
	return all
}

// TenantTables 只返回需要携带 org_id 的业务表（含 KindTenant）。
func TenantTables() []Table {
	return filter(func(t Table) bool { return t.Kind == KindTenant })
}

// MembershipTables 返回描述用户 × 组织关系的表。
func MembershipTables() []Table {
	return filter(func(t Table) bool { return t.Kind == KindMembership })
}

// PlatformTables 返回平台全局表。
func PlatformTables() []Table {
	return filter(func(t Table) bool { return t.Kind == KindPlatform })
}

// TablesMissingOrgID 返回当前基线快照中尚未加 org_id 列的 tenant/membership 表。
// 迁移工具的 schema expand 阶段以此为输入。
func TablesMissingOrgID() []Table {
	return filter(func(t Table) bool {
		return (t.Kind == KindTenant || t.Kind == KindMembership) && !t.HasOrgID
	})
}

// Find 根据表名精确查找；未匹配返回 zero-value 和 false。
func Find(name string) (Table, bool) {
	for _, t := range entries {
		if t.Name == name {
			return t, true
		}
	}
	return Table{}, false
}

func filter(pred func(Table) bool) []Table {
	out := make([]Table, 0)
	for _, t := range entries {
		if pred(t) {
			out = append(out, t)
		}
	}
	return out
}

// entries 是清单主表。任何新增/删除表都改这里；测试会守护它的完整性。
// HasOrgID = true 表示当前分支模型 struct 已包含 OrgID 字段；不代表数据库里
// 一定已经存在该列（后者由 discover 从 information_schema 实测得到）。
var entries = []Table{
	// ===== 平台全局表：不加 org_id =====
	{Name: "organizations", Kind: KindPlatform, Notes: "组织元数据；本身就是租户注册表"},
	{Name: "roles", Kind: KindPlatform, Notes: "全局角色定义，UserRole 才按 org 分配"},
	{Name: "permissions", Kind: KindPlatform, Notes: "全局权限码定义"},
	{Name: "role_permissions", Kind: KindPlatform, Notes: "角色-权限关联"},
	{Name: "menu_permissions", Kind: KindPlatform, Notes: "角色-菜单关联"},
	{Name: "data_permissions", Kind: KindPlatform, Notes: "角色-数据范围关联"},

	// ===== 成员关系表 =====
	{Name: "organization_users", Kind: KindMembership, Priority: PriorityP0, HasOrgID: true, Notes: "唯一键 (org_id,user_id)"},

	// ===== P0 主数据 / 身份 / 审计 =====
	{Name: "users", Kind: KindTenant, Priority: PriorityP0, HasOrgID: true},
	{Name: "departments", Kind: KindTenant, Priority: PriorityP0, HasOrgID: true},
	{Name: "department_change_logs", Kind: KindTenant, Priority: PriorityP0, HasOrgID: true, ParentTable: "departments"},
	{Name: "user_roles", Kind: KindTenant, Priority: PriorityP0, HasOrgID: true, ParentTable: "users"},
	{Name: "employee_profiles", Kind: KindTenant, Priority: PriorityP0, HasOrgID: true, ParentTable: "users"},
	{Name: "dingtalk_bindings", Kind: KindTenant, Priority: PriorityP0, HasOrgID: true, ParentTable: "users", Notes: "钉钉绑定；union_id 跨组织语义待钉钉侧确认"},
	{Name: "user_sessions", Kind: KindTenant, Priority: PriorityP0, HasOrgID: true, ParentTable: "users"},
	{Name: "login_logs", Kind: KindTenant, Priority: PriorityP0, HasOrgID: true, Notes: "失败登录可能无用户，需人工归属"},
	{Name: "operation_logs", Kind: KindTenant, Priority: PriorityP0, HasOrgID: true, ParentTable: "users"},
	{Name: "sync_statuses", Kind: KindTenant, Priority: PriorityP0, HasOrgID: true, Notes: "现在只按 type 建键，需要扩展 (org_id,type)"},

	// ===== P1 审批 / 员工生命周期 =====
	{Name: "approvals", Kind: KindTenant, Priority: PriorityP1, HasOrgID: true, Notes: "按申请人推导；申请人多组织时不可自动归属"},
	{Name: "approval_templates", Kind: KindTenant, Priority: PriorityP1, HasOrgID: true, Notes: "每组织独立副本，可基准克隆"},
	{Name: "employee_transfers", Kind: KindTenant, Priority: PriorityP1, HasOrgID: true, ParentTable: "users"},
	{Name: "employee_resignations", Kind: KindTenant, Priority: PriorityP1, HasOrgID: true, ParentTable: "users"},
	{Name: "employee_onboardings", Kind: KindTenant, Priority: PriorityP1, HasOrgID: true, ParentTable: "departments", Notes: "候选人员无 user 锚点，靠部门推导"},
	{Name: "talent_analyses", Kind: KindTenant, Priority: PriorityP1, HasOrgID: true, ParentTable: "users"},

	// ===== P2 考勤 / 排班 / 节假日 / 假期规则 / 加班 / 调休 =====
	{Name: "attendances", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, ParentTable: "users"},
	{Name: "attendance_exports", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, ParentTable: "users"},
	{Name: "employee_shift_configs", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, ParentTable: "users"},
	{Name: "dingtalk_shift_catalogs", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, Notes: "钉钉 shift_id 每企业独立，不共享"},
	{Name: "week_schedule_rules", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, Notes: "company scope 行无法自动推导"},
	{Name: "week_schedule_overrides", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, ParentTable: "week_schedule_rules"},
	{Name: "week_schedule_sync_logs", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, Notes: "无锚点，建议不迁移历史，仅新数据带 org"},
	{Name: "statutory_holidays", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, Notes: "从平台基准克隆到各组织"},
	{Name: "leave_rule_configs", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, Notes: "组织级配置，克隆或人工指定"},
	{Name: "annual_leave_eligibilities", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, ParentTable: "users"},
	{Name: "annual_leave_grants", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, ParentTable: "annual_leave_eligibilities"},
	{Name: "annual_leave_consume_logs", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, ParentTable: "annual_leave_grants"},
	{Name: "overtime_rule_configs", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, Notes: "组织级配置"},
	{Name: "overtime_match_results", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, ParentTable: "approvals"},
	{Name: "overtime_sync_histories", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, ParentTable: "overtime_match_results"},
	{Name: "overtime_supplementary_requests", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, ParentTable: "overtime_match_results"},
	{Name: "compensatory_leave_ledgers", Kind: KindTenant, Priority: PriorityP2, HasOrgID: true, ParentTable: "overtime_match_results"},

	// ===== P3 绩效全链路 =====
	{Name: "performance_templates", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, Notes: "既有 organization_id 是企业内部范围，不能替代 tenant org_id"},
	{Name: "performance_template_sections", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_templates"},
	{Name: "performance_template_items", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_template_sections"},
	{Name: "performance_level_rules", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, Notes: "孤立规则，需要人工归属"},
	{Name: "performance_level_rule_items", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_level_rules"},
	{Name: "performance_activities", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true},
	{Name: "performance_distribution_rules", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_activities"},
	{Name: "performance_distribution_exceptions", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_activities"},
	{Name: "performance_reminder_logs", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_activities"},
	{Name: "performance_participants", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_activities"},
	{Name: "performance_reviews", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_participants"},
	{Name: "performance_review_versions", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_reviews"},
	{Name: "performance_relationship_change_logs", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_participants"},
	{Name: "performance_goal_records", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_participants"},
	{Name: "performance_goal_approval_logs", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_goal_records"},
	{Name: "performance_company_finances", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_activities"},
	{Name: "performance_indicator_libraries", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "departments"},
	{Name: "performance_indicator_items", Kind: KindTenant, Priority: PriorityP3, HasOrgID: true, ParentTable: "performance_indicator_libraries"},
}
