package database

// lifecycleBindingBusinessUniqueSpec describes one phase-3 composite unique for
// employee lifecycle / talent / dingtalk binding / idempotency / users.mobile.
type lifecycleBindingBusinessUniqueSpec struct {
	Table        string
	NewIndex     string
	Columns      []string
	SoftDelete   bool
	OldIndexes   []string // known legacy unique index names to drop after audit
	OldSingleCol string   // if set, also drop any single-column UNIQUE on this column
	// EmptyNullableCols: convert empty string to NULL so multi-null UNIQUE works
	// (MySQL UNIQUE allows multiple NULLs; empty string would collide).
	EmptyNullableCols []string
}

func lifecycleBindingBusinessUniqueSpecs() []lifecycleBindingBusinessUniqueSpec {
	return []lifecycleBindingBusinessUniqueSpec{
		{
			Table:        "employee_transfers",
			NewIndex:     "idx_employee_transfers_org_transfer",
			Columns:      []string{"org_id", "transfer_id"},
			SoftDelete:   true,
			OldIndexes:   []string{"uni_employee_transfers_transfer_id", "transfer_id", "idx_employee_transfers_transfer_id"},
			OldSingleCol: "transfer_id",
		},
		{
			Table:        "employee_resignations",
			NewIndex:     "idx_employee_resignations_org_resignation",
			Columns:      []string{"org_id", "resignation_id"},
			SoftDelete:   true,
			OldIndexes:   []string{"uni_employee_resignations_resignation_id", "resignation_id", "idx_employee_resignations_resignation_id"},
			OldSingleCol: "resignation_id",
		},
		{
			Table:        "employee_onboardings",
			NewIndex:     "idx_employee_onboardings_org_onboarding",
			Columns:      []string{"org_id", "onboarding_id"},
			SoftDelete:   true,
			OldIndexes:   []string{"uni_employee_onboardings_onboarding_id", "onboarding_id", "idx_employee_onboardings_onboarding_id"},
			OldSingleCol: "onboarding_id",
		},
		{
			Table:        "employee_onboardings",
			NewIndex:     "idx_employee_onboardings_org_employee",
			Columns:      []string{"org_id", "employee_id"},
			SoftDelete:   true,
			OldIndexes:   []string{"uni_employee_onboardings_employee_id", "employee_id", "idx_employee_onboardings_employee_id"},
			OldSingleCol: "employee_id",
		},
		{
			Table:        "talent_analyses",
			NewIndex:     "idx_talent_analyses_org_user",
			Columns:      []string{"org_id", "user_id"},
			SoftDelete:   true,
			OldIndexes:   []string{"uni_talent_analyses_user_id", "user_id", "idx_talent_analyses_user_id"},
			OldSingleCol: "user_id",
		},
		{
			// GORM default table name for DingTalkBinding is ding_talk_bindings.
			Table:        "ding_talk_bindings",
			NewIndex:     "idx_dingtalk_bindings_org_user",
			Columns:      []string{"org_id", "user_id"},
			SoftDelete:   false,
			OldIndexes:   []string{"uni_ding_talk_bindings_user_id", "user_id", "idx_ding_talk_bindings_user_id"},
			OldSingleCol: "user_id",
		},
		{
			Table:        "ding_talk_bindings",
			NewIndex:     "idx_dingtalk_bindings_org_ding",
			Columns:      []string{"org_id", "ding_talk_user_id"},
			SoftDelete:   false,
			OldIndexes:   []string{"uni_ding_talk_bindings_ding_talk_user_id", "ding_talk_user_id", "idx_ding_talk_bindings_ding_talk_user_id"},
			OldSingleCol: "ding_talk_user_id",
		},
		{
			// union_id: 组织内唯一。同一开放平台应用下 union_id 跨企业标识自然人，
			// 但本系统绑定是「企业内登录映射」，跨 org 允许同一 union_id 各存一条。
			// 空串归一 NULL，避免多个未绑定行互撞。
			Table:             "ding_talk_bindings",
			NewIndex:          "idx_dingtalk_bindings_org_union",
			Columns:           []string{"org_id", "union_id"},
			SoftDelete:        false,
			OldIndexes:        []string{"uni_ding_talk_bindings_union_id", "union_id", "idx_ding_talk_bindings_union_id"},
			OldSingleCol:      "union_id",
			EmptyNullableCols: []string{"union_id"},
		},
		{
			// open_id：与 union_id 同策略，组织内唯一 + 空串归一 NULL。
			Table:             "ding_talk_bindings",
			NewIndex:          "idx_dingtalk_bindings_org_open",
			Columns:           []string{"org_id", "open_id"},
			SoftDelete:        false,
			OldIndexes:        []string{"uni_ding_talk_bindings_open_id", "open_id", "idx_ding_talk_bindings_open_id"},
			OldSingleCol:      "open_id",
			EmptyNullableCols: []string{"open_id"},
		},
		{
			// digest 已在 middleware 纳入 org_id；DB 侧 (org_id, digest) 双重保险。
			Table:        "idempotency_records",
			NewIndex:     "idx_idempotency_org_digest",
			Columns:      []string{"org_id", "digest"},
			SoftDelete:   false,
			OldIndexes:   []string{"uni_idempotency_records_digest", "digest", "idx_idempotency_records_digest"},
			OldSingleCol: "digest",
		},
		{
			// 替换历史全局 uni_users_mobile。空手机号归一 NULL 避免多空值冲突。
			Table:             "users",
			NewIndex:          "idx_users_org_mobile",
			Columns:           []string{"org_id", "mobile"},
			SoftDelete:        true,
			OldIndexes:        []string{"uni_users_mobile", "mobile", "idx_users_mobile"},
			OldSingleCol:      "mobile",
			EmptyNullableCols: []string{"mobile"},
		},
	}
}
