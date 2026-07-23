# 阶段四：org_id 复合唯一索引迁移说明

## 目标

把历史「全局 / 单列唯一」安全切换为「以 `org_id` 为首列的复合唯一」，并保证：

- 同组织内业务键冲突时**停止迁移**，输出表名 / org_id / 业务键 / 重复数 / 样例 ID
- **禁止**自动删除、自动合并、自动挑选保留行
- 跨组织业务键相同是**合法数据**
- 迁移可重复执行；目标索引已正确时安全跳过
- 启动时先 `PrepareOrgCompositeUniqueIndexes`（AutoMigrate 前），再 `MigrateOrgCompositeUniqueIndexes`（AutoMigrate 后），避免「先建错索引再修」的窗口

## 代码入口

| 函数 | 时机 | 作用 |
|---|---|---|
| `database.PrepareOrgCompositeUniqueIndexes(db)` | `migrate()` 内、AutoMigrate 前 | 已有表：补 `org_id`、空 org 回填、空可选业务键归一 NULL、同组织冲突审计、原子替换旧唯一索引 |
| `database.MigrateOrgCompositeUniqueIndexes(db)` | `InitDB` 在 `migrate()` 成功后 | 全量校验 / 补齐 / 幂等收口 |
| `database.AllOrgCompositeUniqueSpecs()` | 任意 | 完整索引迁移矩阵 |
| `database.ReadonlyAllOrgUniqueConflictAuditSQL()` | 生产前人工审计 | **只读** SELECT 脚本（含空 org 按 default 分组） |
| `database.AuditOrgCompositeUniqueConflicts(db, spec)` | 迁移内 / 测试 | 同组织冲突审计 |

两个入口均可注入 `*gorm.DB`，可用测试库验证。

## 单条索引迁移顺序

对矩阵中每条 `OrgCompositeUniqueSpec`：

1. 校验 spec（必须 `org_id` 为首列）
2. 表不存在 → 跳过（留给 AutoMigrate 建新表）
3. 缺 `org_id`（仅 Prepare）→ `ADD COLUMN org_id`
4. 缺业务列 → **失败**（拒绝未审计建索引）
5. 空 `org_id` → 回填 `'default'`（仅 unique-index 契约路径）
6. `EmptyNullableCols`：列改为可空 + `''` → `NULL`（不改非空业务值）
7. 按 `org_id + 业务列` 分组冲突审计；有冲突则停止
8. 原子 `ALTER TABLE ... DROP INDEX ..., ADD UNIQUE INDEX ...`
9. 校验新索引存在且列顺序正确

## 生产执行前：只读审计

1. 使用只读账号连接目标库。
2. 生成审计 SQL（本地/跳板机即可）：

```go
// 任意可 import peopleops/internal/database 的小工具，或在调试会话打印：
fmt.Print(database.ReadonlyAllOrgUniqueConflictAuditSQL())
```

也可在 Go test 中调用同一函数做快照比对。

3. 对每个目标表检查：
   - 列是否齐全（含 `org_id` 与业务键）
   - 现有索引定义
   - 空 `org_id` 行数与样例 ID
   - **按回填后 org 分组的重复业务键**（脚本把空 org 视为 `default`）
4. 若出现同组织重复：
   - 记录表名、org_id、业务键、duplicate_count、sample_ids
   - **人工**决定保留/改键/归档，**不要**让迁移自动删合
5. 跨组织相同业务键：忽略（合法）

## 生产执行

1. 备份库（逻辑备份或快照）。
2. 在维护窗口重启应用；启动路径自动执行 Prepare + AutoMigrate + Migrate。
3. 若启动失败且错误含 `same-organization duplicates`：按审计结果人工处理后再重启。
4. 若 DDL 失败且错误含 `rollback SQL`：按错误信息中的 `ALTER TABLE ...` 回滚索引定义（仅索引层；数据未被删除）。

## 回滚说明

- **数据层**：本迁移不删除/不合并业务行；`org_id` 空值回填与可选业务键 `''→NULL` 是归一化。
  - 若必须还原空 `org_id`，需运维按备份恢复；应用侧假定唯一契约下 org 非空。
  - `''→NULL` 对业务含义等价于「无值」，通常无需回滚。
- **索引层**：迁移失败时错误消息附带 `rollback SQL`，形态类似：

```sql
ALTER TABLE `users`
  DROP INDEX `idx_org_user_id`,
  ADD UNIQUE INDEX `uni_users_user_id` (`user_id`);
```

- **应用回滚**：回退到不含复合唯一契约的版本前，须先确认是否仍依赖旧全局唯一；建议先修冲突再前进，而不是回退到全局唯一。

## 安全边界（必须遵守）

- 禁止 `DELETE` / 自动 `GROUP BY` 留最小 id 等去重脚本进入启动路径
- 禁止对 unresolved 业务行 `SET org_id='default'`（历史 `tools/migrate_multitenant` 规则不变；唯一索引迁移对**空 org** 回填 default 是契约特例，且会先审计同组织冲突）
- AutoMigrate 模型 tag 与 `AllOrgCompositeUniqueSpecs()` 必须一致（有单测 `TestOrgUniqueSpecsMatchGormModelIndexes`）

## 相关测试

```bash
go test ./internal/database/ -count=1
# 覆盖：空库、旧索引原子替换、目标已存在、跨组织合法、同组织冲突、空 org 回填后冲突、
# 列顺序错误替换、缺 org 列 Prepare、DDL 失败含 rollback SQL、空串可空归一、重复执行幂等
```


## 索引迁移矩阵

完整矩阵由 `database.AllOrgCompositeUniqueSpecs()` / `OrgCompositeUniqueIndexMatrixMarkdown()` 生成，覆盖：

- 阶段一核心：users / departments / organization_users / employee_profiles / attendances / sync_statuses / approvals / approval_templates / dingtalk_event_logs / roles / user_roles / menu_permissions / data_permissions
- 阶段二排班年假加班：employee_shift_configs / dingtalk_shift_catalogs / week_schedule_* / statutory_holidays / annual_leave_* / overtime_*
- 阶段三生命周期绑定：employee_transfers / resignations / onboardings / talent_analyses / ding_talk_bindings / idempotency_records / users.mobile
- 阶段四补充：users.ding_talk_user_id / departments.dingtalk_department_id / performance_reminder_logs / performance_import_batches / external_* 同步表

本地打印矩阵：

```bash
go test ./internal/database -run TestOrgUniqueMigrationMatrixStartsWithOrgID -v
# 或在临时代码中调用 database.OrgCompositeUniqueIndexMatrixMarkdown()
```

## 索引迁移矩阵（生成）

> 由 `database.OrgCompositeUniqueIndexMatrixMarkdown()` 生成，与代码矩阵保持一致。

| Table | New unique index | Columns | Old indexes / single cols | Empty->NULL cols |
|---|---|---|---|---|
| organization_users | idx_org_user | org_id, user_id | uni_organization_users_user_id, idx_organization_users_user_id, user_id, single:user_id | - |
| users | idx_org_user_id | org_id, user_id | uni_users_user_id, user_id, idx_users_user_id, single:user_id | - |
| users | idx_org_email | org_id, email | uni_users_email, email, single:email | email |
| departments | idx_org_dept_id | org_id, department_id | uni_departments_department_id, department_id, idx_departments_department_id, single:department_id | - |
| employee_profiles | idx_employee_profiles_org_user | org_id, user_id | uni_employee_profiles_user_id, user_id, single:user_id | - |
| employee_profiles | idx_employee_profiles_org_employee | org_id, employee_id | uni_employee_profiles_employee_id, employee_id, single:employee_id | - |
| attendances | idx_org_user_time_type | org_id, user_id, check_time, check_type | idx_user_time_type, uni_attendances_user_time_type | - |
| sync_statuses | idx_org_sync_type | org_id, type | uni_sync_statuses_type, idx_sync_statuses_type, idx_sync_statuses_org_type, type, single:type | - |
| approvals | idx_approvals_org_process | org_id, process_id | uni_approvals_process_id, process_id, idx_approvals_process_id, single:process_id | - |
| approval_templates | idx_approval_templates_org_template | org_id, template_id | uni_approval_templates_template_id, template_id, idx_approval_templates_template_id, single:template_id | - |
| dingtalk_event_logs | idx_dingtalk_event_org_event | org_id, event_id | uni_dingtalk_event_logs_event_id, event_id, single:event_id | - |
| roles | idx_roles_org_name | org_id, name | uni_roles_name, name, single:name | - |
| user_roles | idx_user_roles_org_user | org_id, user_id | uni_user_roles_user_id, user_id, single:user_id | - |
| menu_permissions | idx_menu_permissions_org_role | org_id, role_id | uni_menu_permissions_role_id, role_id, single:role_id | - |
| data_permissions | idx_data_permissions_org_role | org_id, role_id | uni_data_permissions_role_id, role_id, single:role_id | - |
| employee_shift_configs | idx_employee_shift_configs_org_user | org_id, user_id | idx_employee_shift_configs_user_id, uni_employee_shift_configs_user_id, user_id, single:user_id | - |
| dingtalk_shift_catalogs | idx_dingtalk_shift_catalogs_org_shift_key | org_id, shift_key | idx_dingtalk_shift_catalogs_shift_key, uni_dingtalk_shift_catalogs_shift_key, shift_key, single:shift_key | - |
| week_schedule_rules | idx_week_schedule_rules_org_scope | org_id, scope_type, scope_id | idx_scope, uni_week_schedule_rules_scope | - |
| week_schedule_overrides | idx_week_schedule_overrides_org_scope_date | org_id, scope_type, scope_id, week_start_date | idx_scope_date, uni_week_schedule_overrides_scope_date | - |
| statutory_holidays | idx_statutory_holidays_org_date | org_id, date | uni_statutory_holidays_date, idx_statutory_holidays_date, date, single:date | - |
| annual_leave_eligibilities | idx_leave_elig_org_user_year_q | org_id, user_id, year, quarter | idx_leave_elig_user_year_q | - |
| annual_leave_grants | idx_leave_grant_org_user_year_q_type | org_id, user_id, year, quarter, grant_type | idx_leave_grant_user_year_q_type | - |
| overtime_rule_configs | idx_overtime_rule_org_key | org_id, rule_key | uni_overtime_rule_configs_rule_key, idx_overtime_rule_configs_rule_key, rule_key, single:rule_key | - |
| overtime_match_results | idx_overtime_match_org_user_work_date | org_id, user_id, work_date | idx_user_work_date | - |
| overtime_sync_histories | idx_overtime_sync_org_user_workdate | org_id, user_id, work_date | idx_overtime_sync_user_workdate | - |
| annual_leave_consume_logs | idx_leave_consume_org_request_grant | org_id, request_ref, grant_id | idx_leave_consume_request_grant, idx_leave_consume_approval_ref, idx_annual_leave_consume_logs_approval_ref, uni_annual_leave_consume_logs_approval_ref, approval_ref, single:approval_ref | - |
| employee_transfers | idx_employee_transfers_org_transfer | org_id, transfer_id | uni_employee_transfers_transfer_id, transfer_id, idx_employee_transfers_transfer_id, single:transfer_id | - |
| employee_resignations | idx_employee_resignations_org_resignation | org_id, resignation_id | uni_employee_resignations_resignation_id, resignation_id, idx_employee_resignations_resignation_id, single:resignation_id | - |
| employee_onboardings | idx_employee_onboardings_org_onboarding | org_id, onboarding_id | uni_employee_onboardings_onboarding_id, onboarding_id, idx_employee_onboardings_onboarding_id, single:onboarding_id | - |
| employee_onboardings | idx_employee_onboardings_org_employee | org_id, employee_id | uni_employee_onboardings_employee_id, employee_id, idx_employee_onboardings_employee_id, single:employee_id | - |
| talent_analyses | idx_talent_analyses_org_user | org_id, user_id | uni_talent_analyses_user_id, user_id, idx_talent_analyses_user_id, single:user_id | - |
| ding_talk_bindings | idx_dingtalk_bindings_org_user | org_id, user_id | uni_ding_talk_bindings_user_id, user_id, idx_ding_talk_bindings_user_id, single:user_id | - |
| ding_talk_bindings | idx_dingtalk_bindings_org_ding | org_id, ding_talk_user_id | uni_ding_talk_bindings_ding_talk_user_id, ding_talk_user_id, idx_ding_talk_bindings_ding_talk_user_id, single:ding_talk_user_id | - |
| ding_talk_bindings | idx_dingtalk_bindings_org_union | org_id, union_id | uni_ding_talk_bindings_union_id, union_id, idx_ding_talk_bindings_union_id, single:union_id | union_id |
| ding_talk_bindings | idx_dingtalk_bindings_org_open | org_id, open_id | uni_ding_talk_bindings_open_id, open_id, idx_ding_talk_bindings_open_id, single:open_id | open_id |
| idempotency_records | idx_idempotency_org_digest | org_id, digest | uni_idempotency_records_digest, digest, idx_idempotency_records_digest, single:digest | - |
| users | idx_users_org_mobile | org_id, mobile | uni_users_mobile, mobile, idx_users_mobile, single:mobile | mobile |
| users | idx_users_org_dingtalk_user | org_id, ding_talk_user_id | uni_users_ding_talk_user_id, idx_users_ding_talk_user_id, single:ding_talk_user_id | ding_talk_user_id |
| departments | idx_departments_org_dingtalk_department | org_id, dingtalk_department_id | uni_departments_dingtalk_department_id, idx_departments_dingtalk_department_id, single:dingtalk_department_id | dingtalk_department_id |
| performance_reminder_logs | idx_perf_reminder_org_round | org_id, activity_id, participant_id, stage, reminder_key, reminder_date | idx_perf_reminder_round | - |
| external_attendance_raw | uk_ext_att_org_row | org_id, source_row_key | uk_ext_att_row, uni_external_attendance_raw_source_row_key, single:source_row_key | - |
| external_attendance_approve_links | uk_ext_appr_item | org_id, source_row_key, item_key | uk_ext_appr_link, uk_ext_appr_item_legacy | - |
| external_user_department_raw | uk_ext_dept_org_row | org_id, source_row_key | uk_ext_dept_row, single:source_row_key | - |
| user_department_relations | uk_user_dept_rel | org_id, user_id, department_id | uk_user_dept_rel_legacy | - |
| external_sync_cursors | uk_ext_sync_cursor | org_id, source_table | uk_ext_sync_cursor_source, single:source_table | - |
| external_sync_locks | uk_ext_sync_lock | org_id, scope_key | uk_ext_sync_lock_scope, single:scope_key | - |
| performance_import_batches | uk_performance_import_batch_org_key | org_id, batch_key | uk_performance_import_batch_key, uni_performance_import_batches_batch_key, batch_key, single:batch_key | - |
