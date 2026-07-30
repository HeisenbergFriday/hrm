# Cerebrum

> OpenWolf's learning memory. Updated automatically as the AI learns from interactions.
> Do not edit manually unless correcting an error.
> Last updated: 2026-05-25

## User Preferences

- [2026-07-16] UI 改版默认走「纯前端、零业务逻辑」：列表主操作必须复用已有 handler，禁止新建一套阶段条件；不要改默认权限/数据范围语义。
- [2026-07-16] 绩效总览首屏偏好扁平：Hero 一行（角色+待办数字+CTA），统计用 pill 芯片筛选，避免 4 层总览卡片。
- [2026-07-18] UI 问题修复优先做「不影响业务逻辑」项：写操作二次确认、权限按钮展示、token 色、空状态；不改状态机/权限码/数据范围。

## Key Learnings

- [2026-07-21] 吴列德「又没权限」根因：`migrateLiedeOrganizationAdminRoles` 每次启动强制写管理员，但 `ensureRolePreset` 只按 `name` 取第一条角色（通常 default 的管理员），导致 muteng/xiaotie 的 `user_roles.role_id` 指向跨 org 角色；权限 JOIN 要求 `roles.org_id=user_roles.org_id` 后返回 0 条。不是人工清空，也不是钉钉同步覆盖。
- [2026-07-21] 已修：`ensureRolePresetInOrg` + Liede 按 org 授权 + `ensureUserRoleInOrg` 拒绝跨 org role_id + 启动 `remapCrossOrgUserRoleBindings`。角色 preset/菜单/数据权限写入禁止只按 role_id 不带 org。
- [2026-07-20] 钉钉企业内部消息必须用 `topapi/message/corpconversation/asyncsend_v2`；旧 `asyncsend` 对嵌套 `msg.msgtype` 返回 Missing required arguments:msgtype。媒体上传 `media/upload?type=image` 正常。
- [2026-07-20] 作息表个人推送：页面按钮改为「作息表推送」，前端 Canvas 生成月历 PNG；后端 `/week-schedule/push/personal` multipart 上传图片后 `media/upload` + 逐人 text+image；优先 DingTalkUserID 回退 UserID；单人失败不阻断；旧 `/sync/to-dingtalk` 仅保留排班写入，页面不再调用。
- [2026-07-20] 开发问题复盘闭环：`docs/DEVELOPMENT_ISSUES.md` 为防复发真源；开发前读索引+模块条目，修改计划写明约束；收尾前判断新增/更新；`AGENTS.md` 与 `.ai/AI_WORKFLOW.md` 已强制；AI_WORKFLOW 内一律 `AGENTS.md`（无 CLAUDE.md）。实体 `OrgID` 不能代替 `NewXxxWithOrgID` 仓储绑定。
- [2026-07-21] UI 审计执行序**已收口**（零业务逻辑/零配置改动）：Setting 密钥假表单→只读说明；SyncJobs 真 sync+confirm；推送默认空选；org sync 统一 confirmOrgSync；写按钮 hasPermission+Tooltip；export downloadAuthorizedFile；logout queryClient.clear；Login error 白名单；列表 PII 脱敏；绩效附件 AuthorizedImage；router toolbox 细权限（禁 menu-only）、upload/departments 门闩、files+TenantContext。见 DEVELOPMENT_ISSUES 2026-07-21 UI 交互安全执行序收口。
- [2026-07-21] Header 退出登录必须 `authAPI.logout()`（CSRF+credentials）；禁止裸 `axios.post('/api/v1/auth/logout')`。组织切换路径已正确；已在 App.tsx 对齐。见 DEVELOPMENT_ISSUES 2026-07-21 logout CSRF 条目。
- [2026-07-21] 交互/UI 安全**第三遍**历史记录：曾 open 项已在同日执行序修复收口（见上条）。
- [2026-07-20] 交互/UI 风险**复审**（第二遍，直接读源码）：上轮结论几乎全部 still_open，无代码修复痕迹。**新增** SyncLog「手动同步」同 orgAPI.syncOrg 无 confirm/权限门闩（org sync 入口现为 5 处：EmployeeList 已好；DepartmentTree/EmployeeDetail/Setting/SyncLog 未好）。其余：附件 fail-open、AttendanceExport window.open、推送写死人名、Processing/ShiftConfig/Profile/Flow 写按钮无 hasPermission、RunJob 假 success 均仍在。无 dangerouslySetInnerHTML。
- [2026-07-19] UI 危险向导（多 step Modal）：禁止先 setStep 再 mutate；成功再推进；失败回退 + footer 取消/重试 + 错误 Alert；Spin 必须绑 isPending，禁止 step 中间态 footer=[]。
- [2026-07-19] 绩效活动列表不得只拉 page_size=100：分页拉全量（上限页数）或服务端分页；芯片计数与列表同源；待办芯片需独立 filter sentinel（`__todo__`），不能 filter:undefined。
- [2026-07-19] 首页统计 query 按菜单分别 enabled；禁止 overview||attendance||approvals 任一 loading 整页 Skeleton；分卡 Skeleton/「…」；403 与网络失败区分。
- [2026-07-19] 写操作确认：考勤立即同步、年假重算/追溯补发/加班匹配、绩效钉钉提醒与阶段推进一样走 Modal.confirm；交互测 simple write guard 标题正则需含「发送」。
- [2026-07-19] 审批/部门仓储必须严格绑定 org：NewApprovalRepository/NewDepartmentRepository 走 RequireOrganizationIDFromDB；空 org Create 报错，查询 scoped 用 1=0；ScopeOrg 空 org 禁止放行。HTTP 一律 currentOrgIDOrAbort + NewXxxServiceWithOrgID。

- [2026-07-18] 绩效钉钉通知必须 org 显式：`SendPerformanceActionCard(orgID,...)`→`SendCorpActionCardToUserForOrg`；链接 `BuildAppURLForOrg`；缺 org fail-closed 禁止 default。申诉收件人 JOIN `user_roles.org_id=users.org_id` 且 `roles.org_id=users.org_id`；scope 用 `NewPermissionServiceWithOrgID`+`ResolveUserScopeInOrg`，permissions 保持全局。

- [2026-07-18] 员工档案部门筛选：`users` 原生 SQL 子查询必须显式 `org_id = ? AND deleted_at IS NULL`；GORM 租户回调不会注入 raw subquery。HTTP 必须 `NewEmployeeServiceWithOrgID(RequestDB, jwtOrg)`；`NewEmployeeService` 仅迁移场景，读/写缺 org 一律 fail-closed（ErrMissingOrgID / 1=0）。
- [2026-07-18] 排班/班次钉钉写读必须 org 隔离：`GetScheduleListBatchByDayForOrg`；业务层禁止 `os.Getenv("DINGTALK_ADMIN_USER_ID")`，统一 `ResolveAdminUserID`；`shiftIDCache` key=`orgID|shiftKey`，测试用 `ClearShiftIDCacheForTest`；非 default 缺 admin fail-closed 且禁止写库。
- [2026-07-18] 通用文件上传必须元数据绑定 JWT org_id：磁盘 `uploads/<safe_org_id>/<stored_name>`，URL 仅暴露数字 `file_id`；下载路由 `JWTAuth+TenantContext`，跨 org 统一 404；旧 `/files/<filename>` 无所有权证明 fail-closed，禁止靠文件名随机性授权。
- [2026-07-18] 绩效阶段推进/目标通过/HR确认等写操作应经 Modal.confirm 护栏，仍调用同一 API；交互测试 mock Modal.confirm 时需在模块加载时捕获 realModalConfirm，禁止 spy 后再 bind 自己（会无限递归）。组织同步按钮权限码与路由一致为 attendance_manage。
- [2026-07-18] 角色分配必须双校验：user 属于 JWT org_id，且 Role.OrgID==org_id；UserRole 查询 JOIN 必须 roles.org_id=user_roles.org_id。默认员工角色按 (org_id,name) 查找/创建，禁止全局 name 命中。permissions/role_permissions 保持全局语义。跨 org role_id 返回 404（不区分缺失/外组织，防探测）。

- [2026-07-17] 当前 auth 为 HttpOnly cookie + `/auth/me` 水合，不再读 localStorage token。`openwolf designqc` 未登录只会截到登录页；业务页 UI 走查需 Playwright route mock `/api/v1/auth/me`（可参考 `frontend/scripts/designqc-capture.mjs`）。
- [2026-07-17] 即便 `ConfigProvider locale={zhCN}`，部分 Table 仍可能显示英文 `No data`；列表页应显式 `locale={{ emptyText: '暂无数据' }}` 或包 `<Empty description="..." />`。
- [2026-07-17] UI 复检：运行时主色是 `#2563eb`（index.css + App theme），但 `.ai/DESIGN_SYSTEM.md` 与 cerebrum 旧条目仍写 `#1677ff`/`#4338ca`，文档需跟 token 对齐。侧栏折叠 trigger 在内容短时视觉上像左下角游离白卡；菜单 232px 下「人才管理驾…」「大小周与节…」截断。花名册副标题勿直接展示「正在加载数据范围…」。空状态最佳实践见 Attendance：`Empty` + 中文描述 + 行动按钮（清除筛选）。
- [2026-07-17] 多租户第五阶段收尾：绩效后台任务与请求内异步通知必须带 org。`NewPerformanceService` 解析 org 顺序为 `requestmeta.TenantID` → `RequestInfo.OrgID`；`performanceBackgroundDB` 两者都注入且缺 org 返回 nil；`PerformanceJobScheduler` 逐活跃组织 `NewPerformanceServiceWithOrgID`；`SendDueSelfEvalAutoReminders` 走 `scopedDB()`。`TenantDB` 必须同时 `SetOrgID(RequestInfo)` + `WithTenant`。
- 多租户业务唯一键阶段三（生命周期/人才/钉钉绑定/幂等/mobile）：Model `uniqueIndex`、启动迁移 `migrateLifecycleBindingBusinessUniqueIndexes`、幂等 middleware digest 三者必须同步；迁移只审计冲突并报错（含表名+样例），禁止自动删/合并业务行。`users.mobile` 禁止再创建全局 `uni_users_mobile`，空 mobile 归一 NULL 后建 `(org_id,mobile)` 唯一。`DingTalkBinding.union_id/open_id` 取组织内唯一（绑定是企业登录映射，同一自然人可跨 org 各绑一条）；空串归一 NULL。`IdempotencyRecord.digest` 计算必须含 org_id，DB 唯一为 `(org_id,digest)`。
- 多租户业务唯一键阶段二（排班/班次/年假/加班）：Model `uniqueIndex`、Repository/Service `OnConflict Columns`、启动迁移三者必须同步；迁移只审计冲突并报错（含表名+样例），禁止自动删/合并业务行。`CompensatoryLeaveLedger` / `OvertimeSupplementaryRequest` 暂不落 DB 唯一，依赖 org-scoped 应用幂等检查。
- 多租户普通接口一律以 JWT `orgID` 为唯一权威租户上下文；handler 不得从 body/query/header 读取 `org_id`/`target_org_id` 切换组织。跨组织操作必须走独立的受控运维入口。可复用 `internal/api/handlers.go` 中的 `currentOrgIDOrAbort` 与 `rejectCrossOrgParam` helper。
- 登录 handler 必须 fail-closed：`org_id` 使用 `binding:"required"`，先 `database.GetOrganizationByOrgID` 校验组织存在且 active，再按 `(org_id,user_id)` 精准查询；禁止用不带 org 的 `GetUserByUserID` 做全局兜底或退化为 `default`。
- 前端样式统一使用 CSS 变量（Design Token），定义在 `frontend/src/index.css` 的 `:root` 中。禁止在 TSX 中硬编码颜色值、像素值、圆角值等。必须引用 `var(--xxx)` 令牌。
- Ant Design 的 ConfigProvider theme 中的颜色值需要与 CSS 变量保持同步，但因为是 JS 对象不能直接引用 CSS 变量，所以手动保持一致。
- 页面背景色（2026-07 浅色改版）：`--color-bg-page: #f6f8fb`，卡片：`--color-bg-card: #fff`，表头：`--color-bg-card-header: #f7faff`
- 主色系（2026-07）：primary `#2563eb`、hover `#3b82f6`、active `#1d4ed8`、light `#7dd3fc`、bg `#eaf2ff`（旧靛蓝 `#4338ca` / antd 默认 `#1677ff` 已废弃）
- 圆角规范（2026-07 token）：xs=4, sm=6, md/lg/xl=8, 2xl=10, pill=999；旧 lg=12/xl=14/2xl=16 不再适用
- 页面必须使用 PageContainer 包裹（提供背景、padding、标题），不要手写根 div 样式
- 卡片必须使用 PageCard 包裹（提供统一的 borderRadius、border、boxShadow），不要手写 Card 样式
- 状态标签使用 StatusTag（封装 Tag + borderRadius:6 + fontWeight:600），不要手写 Tag 内联样式
- Ant Design Button 默认 borderRadius:8、fontWeight:600 已在 App.tsx 主题中配置，按钮不需要内联这些值
- Ant Design Tag 默认 borderRadiusSM:6 已在主题中配置
- 全站 37 个页面已完成 Design Token + 组件规范替换（2026-05-25），Login/Callback 为全屏居中布局不适用标准模式
- 批量替换页面时使用并行 agent 可大幅提速，每个 agent 负责 3-6 个文件

## Layout Rules

- **导航栏层级**：如果页面有 sticky 导航栏，必须分两行——第一行放返回按钮+标题+姓名，第二行放状态摘要+操作按钮。禁止所有内容挤一行。
- **绩效详情 Drawer**：阶段推进按钮用 sticky 顶栏；Descriptions 不要重复 header 已展示的状态/流程类型。
- **考勤工具箱**：月末推荐顺序用 Steps 引导但不锁步；计算前必填 checklist 在 sticky 底栏；缺文件滚到字段并短暂高亮。
- **表格列宽**：表单型表格必须给**所有列**设明确的 width（可以用百分比 `'40%'` 或固定像素 `140`），不要留空让 Ant Design 自动分配。否则剩余空间平分会导致宽列独占大量空白，输入框只占一小半，视觉失衡。推荐比例：名称列 35-40%，描述列 30%，数值列 12-16%，操作列 48px。
- **展开行栅格**：展开行内多个字段用 `display: grid; gridTemplateColumns: 1fr 1fr 1fr 1fr; gap: 16` 四等分。标签用 `fontSize: xs, color: secondary, fontWeight: medium, marginBottom: 6`。
- **区块间距**：PageCard 之间用 `marginTop: 24`（不是 16），保持区块感。
- **"添加"按钮**：表格下方的 dashed 按钮用 `marginTop: 12`（不是 8），和表格保持适当距离。
- **表单标签**：展开行/表单内的字段标签统一用 `fontSize: var(--font-size-xs), color: var(--color-text-secondary), fontWeight: var(--font-weight-medium)`，不要用 `type="secondary"`（样式不够精确）。

## Do-Not-Repeat

<!-- Mistakes made and corrected. Each entry prevents the same mistake recurring. -->
<!-- Format: [YYYY-MM-DD] Description of what went wrong and what to do instead. -->
- [2026-07-19] 禁止 NewApprovalService/NewDepartmentService/NewWeekScheduleService(RequestDB) 用于 HTTP 读路径而不先 currentOrgIDOrAbort；禁止 ScopeOrg 空 org 返回无过滤 tx；禁止 holiday sync 用 orgIDFromDB 回退 default。
- [2026-07-18] 禁止绩效通知调用无 org 的 `SendCorpActionCardToUser`/`BuildAppURL`；禁止 handler 按 activity_id 列参与人而不带 `org_id`；禁止申诉 `NewPermissionService(db)` 无 org 解析 scope。
- [2026-07-16] 构造 antd Upload 的 RcFile 时，禁止对原生 `File` 使用 `Object.assign(file, { lastModifiedDate })`。现代浏览器里 `lastModifiedDate` 是只读 getter，赋值会抛 `Cannot set property lastModifiedDate of #<File> which has only a getter`。正确做法：`new File([blob], name, { type, lastModified })` 后只设置 `uid`。
- [2026-07-10] 多租户 handler 不允许出现“若目标组织不同于 JWT 组织，则用目标组织的同名 user_id 权限授权”的模式；这不是平台级授权，会造成跨组织越权。要么只用 JWT 组织，要么把入口迁到独立运维/平台层并使用不同的身份模型。
- [2026-07-10] 权限相关 service 构造器（例如 `NewPermissionServiceWithOrgID`）不能被业务 handler 用作“允许空 org 静默落 default”的通道；调用方必须先经过 `currentOrgIDOrAbort` 保证 JWT 有非空 orgID。
- [2026-07-10] 历史多租户迁移禁止 `UPDATE ... SET org_id = default WHERE org_id IS NULL` 全表回填。未能唯一推导的记录必须进入 unresolved 清单，由人工确认；`tools/migrate_multitenant` 已改为 discover/report 只读默认，`infer/apply/verify/contract` 显式占位报错，任何"填 default"实现都不得再次引入。
- [2026-07-13] D:\app→HR 迁移方向纠正：D:\app 的 Excel 解析"脏活"（单元格颜色识别、表头模糊匹配、名字规范化等）是**要保留的功能**，不是要丢弃的外壳；业务计算逻辑更不能改。迁移目标是"原样搬过来（含 Excel 解析 + 计算规则），结果逐字段一致"，而不是"只抽计算内核、丢解析层、重构规则"。之前方案 §3.1 主张丢弃解析层的判断是错的。
- [2026-07-16] 用户/会话查询禁止只按 `user_id`/`session_id` 操作：`loadUserByUserID`、`loadUserByAuthIDInOrg`、`revokeActiveSessionsForUser`、`Logout` 必须带 JWT `org_id`；空 org fail-closed，禁止 `NormalizeOrganizationID` 把空串变成 `default` 后继续查。
- [2026-07-17] 禁止后台 job 用 `NewPerformanceService(db)` 无 org 扫全库；禁止 `performanceBackgroundDB` 只写 RequestInfo 不写 WithTenant；禁止通知查询只按 `activity_id` 不带 `org_id`。
- [2026-07-18] 禁止排班/班次路径直接 `GetScheduleListBatchByDay`/`GetAccessToken` 默认版服务非 default 企业；禁止 `shiftIDCache` 仅用 shiftKey；禁止非 default 用全局 `DINGTALK_ADMIN_USER_ID` 兜底 op_user_id。
- [2026-07-18] 禁止 `NewEmployeeService(RequestDB(c))` 用于 HTTP；禁止 `FindAllProfiles` 在 org 空时用 `users WHERE department_id=?` 无 org 子查询；禁止 lifecycle 用 `CurrentOrganizationIDFromDB` 回退 default。

## Decision Log

<!-- Significant technical decisions with rationale. Why X was chosen over Y. -->

- [2026-07-17] 测试服「上传失败续传」用独立脚本 `deploy/upload-and-restart.ps1`，禁止改 `build-and-deploy.ps1` 行为；快路径必须：本地 tar 存在、远端 size 校验、health；默认只 recreate `peopleops-hr`，`-FullStack` 才 down/up 整栈。改代码后仍走完整 build-and-deploy。
- [2026-07-15] 考勤工具箱加班自定义规则必须经适配层 `rules_adapter.resolve_overtime_config` 进入 `process_overtime`；禁止 `run_overtime` 写死 `get_default_config()`。导入预览 ≠ 应用规则。
- [2026-07-15] 工具箱结果下载用内存 run store 绑定 user_id+org_id+TTL；禁止返回服务器绝对路径。一键联动需 operate+dingtalk_sync 双权限。
- [2026-07-16] 结构化工具箱接口失败回退旧 blob：仅 404/405/501；403/业务/超时/网络不得回退，钉钉同步不得因错误重跑。
- [2026-07-16] run store 路径只用 rootDir/runID；user/org 仅元数据；重启清理孤儿目录；ZIP 缺文件必须失败。
- [2026-07-16] SOURCE_MANIFEST 必须由 compare_app_source.py 生成（含 difference_kind）；禁止手工补字段。
- [2026-07-16] compare 适配差异只允许对 overtime/fill_overtime_fields.py 与 finally/calc_finally.py 做文件级 canonical strip：精确 `_BASE` path block、excel_compat import、纯 `load_workbook_compat(`→`openpyxl.load_workbook(`；禁止宽正则放行；同行带业务表达式必须 business_divergence。
- [2026-07-16] Playwright 工具箱路由是 `/attendance-toolbox`；`/auth/me` 响应必须是 `{data:{user:{menu_keys,permissions,...}}}` 才能 login 水合。
- [2026-07-16] 最终表链路：`runner.run_final` → `calc_finally.generate`；SOURCE_MANIFEST 显示 leave/subsidy/rules 与 D:\app equal；fill_overtime/finally 仅 adapter_only。
- [2026-07-16] 加班时长解析必须支持「N小时/N天」；`raw_hours is None` 禁止默认整日 8h。产研 `_is_rd_dept` 须拼接多级部门路径；晚走排除覆盖部门/岗位「销售」。

- [2026-07-17] ��Ч��������������/��������/Ŀ��/�����RouteGuard �� menuOptional��ֻ�鹦��Ȩ�ޣ���ǿ�� menu:performance-overview���б�ҳ��ǿ�Ʋ˵����� menuKeys ʱ��ҳ���и� Home ��̬����ֹ 403 ��ѭ����
- [2026-07-17] �����ڽ��������� getPreviousParticipantResult���÷��ص� activity.id + participant.id ��ת������ message.info �������ڣ���ֹ����ǰ�����ˡ�


- [2026-07-17] Multi-tenant unique indexes phase-4: PrepareOrgCompositeUniqueIndexes before AutoMigrate, MigrateOrgCompositeUniqueIndexes after; conflict audit groups by org_id; no auto delete/merge; EmptyNullableCols normalizes empty string to NULL; docs/org_composite_unique_index_migration.md

- [2026-07-18] Real MySQL org isolation drill must use only 127.0.0.1:13306/peopleops_org_drill; Performance E2E needs departments table; goal upsert needs activity status target_setting; template list total!=1 due to system templates; never run migration and API drill concurrently on same drill DB.
