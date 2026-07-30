---
purpose: 开发问题复盘日志——沉淀已定位根因且有复用价值的缺陷与防复发约束，供开发前查阅、开发后更新
last_updated: 2026-07-30
source_of_truth:
  - 本文件（问题条目与防复发索引）
  - AGENTS.md（开发前必读 / 开发后必记流程）
  - .ai/AI_WORKFLOW.md（步骤 3 / 步骤 16 闭环）
  - .ai/MODULES/*.md（模块长期约束）
update_when:
  - 定位到有复用价值的业务/代码/测试/审查/部署问题时
  - 同一根因修复方案或防复发约束变更时
  - 防复发索引需要增删改时
  - 问题状态从 open 变为 fixed / wontfix 时
---

# 开发问题复盘日志

## 开发前必读索引（防复发）

开发任何功能或修 Bug 前，先读本索引，再按模块跳到相关条目。

### 认证 / 多组织登录

| 标签 | 约束摘要 | 条目 |
|---|---|---|
| `auth` `logout` `csrf` | Header「退出登录」必须 `authAPI.logout()`（带 withCredentials + `X-CSRF-Token`）；禁止裸 `axios.post('/api/v1/auth/logout')`，否则 JWTAuth CSRF 校验 403 后仅清前端、HttpOnly 会话残留 | [2026-07-21 Header 退出登录未带 CSRF](#2026-07-21-p1-header-退出登录未带-csrf-导致服务端会话可能残留) |
| `ui` `security` `write-guard` | 危险写操作需 Modal.confirm + 与后端一致的 feature 权限门闩（disabled+Tooltip）；禁止假 success；org sync 统一 `confirmOrgSync`/`attendance_manage` | [2026-07-21 UI 交互安全执行序收口](#2026-07-21-p1p2-ui-交互安全执行序收口假成功写门闩导出会话) |
| `attendance-toolbox` `permission` | 工具箱写 API 必须 `attendance_manage` 或 `attendance_toolbox_operate`（钉钉同步含 dingtalk_sync）；禁止仅凭 `menu:attendance-toolbox` 绕过 | 同上 |
| `auth` `multi-org` `user-service` | 已解析 `orgID` 后的用户读写必须用 `NewUserServiceWithOrgID`；实体 `User.OrgID` 不能代替仓储构造绑定；空 org fail-closed | [2026-07-20 钉钉多组织登录更新用户缺少组织作用域](#2026-07-20-p1-钉钉多组织登录更新用户时缺少组织作用域) |
| `auth` `multi-org` `dingtalk-login` `first-login` | 首次自动补用户 `ensureLocalUserForDingTalkLogin` 必须校验非空 org 后用 `NewUserServiceWithOrgID` + `NewPermissionServiceWithOrgID`；禁止 `NormalizeOrganizationID("")` 成 default | [2026-07-20 首次补用户/登出审计/直接GORM fail-open 收口](#2026-07-20-p1p2-org_id-隔离缺口收口首次补用户登出审计直接-gorm-fail-open) |
| `auth` `logout` `audit` | `/auth/logout`、`/auth/me` 必须 `JWTAuth+TenantContext`；登出 `OperationLog` 必须写 JWT `org_id` | 同上 |
| `auth` `jwt` | JWT 必须携带 `org_id`；缺省返回 `code=token_missing_org_id`，禁止回退 `default` | 见 `.ai/MODULES/auth.md` 认证安全约束 |
| `auth` `dingtalk-login` | 多企业扫码/免登必须显式或可解析的本地 `org_id`；禁止静默落到 default；仅有 unionId/openId 不得跨企业反查自动选企 | 见 `.ai/MODULES/auth.md` 2026-07 多企业登录隔离 |
| `auth` `permission` `user-role` `multi-tenant` | 角色 preset / 用户角色绑定必须按 `org_id`；`user_roles.role_id` 不得指向外组织 `roles`；权限 JOIN 要求 `roles.org_id=user_roles.org_id` | [2026-07-21 列德管理员跨 org 角色中毒](#2026-07-21-p1-列德管理员角色跨-org-中毒导致权限查询-0-行) |

### 多租户 / 仓储构造

| 标签 | 约束摘要 | 条目 / 文档 |
|---|---|---|
| `multi-tenant` `repository` | tenant-scoped repository 缺 `orgID` 必须 fail-closed；优先 `NewXxxWithOrgID`；实体字段 `OrgID` ≠ 仓储绑定 | `.ai/ARCHITECTURE.md` 多租户隔离运行时边界 |
| `multi-tenant` `handler` | 普通接口只认 JWT `orgID`；禁止 body/query/header 切 org | `.ai/ARCHITECTURE.md` / cerebrum |
| `database-migration` `multi-tenant` `unique-index` | 租户业务唯一键必须以 `org_id` 开头；模型标签、迁移矩阵、旧兼容逻辑必须一致；启动迁移按 Prepare → AutoMigrate → Verify 执行；禁止自动改写组织归属、删行或合并 | [2026-07-21 多组织唯一索引与旧兼容迁移不一致导致连续部署失败](#2026-07-21-p1-多组织唯一索引与旧兼容迁移不一致导致连续部署失败) |
| `database-migration` `mysql` `collation` | 迁移 SQL 跨数值/文本 ID 关联时必须使用不依赖 collation 的精确比较；禁止把数值 `CAST AS CHAR` 后直接与文本列比较 | [2026-07-23 绩效迁移 ID 比较触发排序规则冲突](#2026-07-23-p1-绩效迁移-id-比较触发-mysql-排序规则冲突) |
| `shift-config` `attendance` `gorm` | 服务内直接 `s.db` 查用户/部门/节假日/班次目录必须 org 过滤；缺 org fail-closed 禁止全表；优先 `NewShiftConfigServiceWithOrgID` / `NewAttendanceServiceWithOrgID` | [2026-07-20 首次补用户/登出审计/直接GORM fail-open 收口](#2026-07-20-p1p2-org_id-隔离缺口收口首次补用户登出审计直接-gorm-fail-open) |

### 考勤工具箱

| 标签 | 约束摘要 | 条目 / 文档 |
|---|---|---|
| `attendance-toolbox` `run-store` | 结果绑定 `user_id+org_id`；磁盘仅 `rootDir/<runID>`；禁止返回服务器绝对路径 | `.ai/MODULES/attendance.md` |
| `attendance-toolbox` `permission` | 计算/审计/模板需 `attendance_toolbox_operate`；钉钉同步需 `attendance_toolbox_dingtalk_sync`；一键联动 AND | `.ai/MODULES/attendance.md` 权限矩阵 |
| `attendance-toolbox` `playwright` `locator` | 页面存在重复 placeholder/文案时，E2E 必须先用 tabpanel、form 或可访问名称缩小作用域；禁止使用全页面模糊定位 | [2026-07-30 考勤工具箱 E2E 重复日期占位符](#2026-07-30-p2-考勤工具箱-e2e-重复日期占位符导致-strict-mode-失败) |

### API / 路由契约

| 标签 | 约束摘要 | 条目 |
|---|---|---|
| `org-sync` `frontend` `timeout` `api-contract` `multi-tenant` `security` | 用户/部门/全量组织同步共享同组织门闩；JWT `org_id` 唯一可信；超过网关时限的全量同步必须短请求启动+轮询；执行上下文脱离客户端取消，终态用独立短上下文持久化；HTTP 207 必须刷新已成功数据；响应/状态不得回显原始错误 | [2026-07-27 组织全量同步被前端 10 秒超时误判失败](#2026-07-27-p1-组织全量同步被前端-10-秒超时误判失败) |
| `org-sync` `department` `stable-id` `transaction` `release` | 钉钉同步必须按租户内稳定外部 ID 匹配历史部门/员工，保留既有本地 ID 与引用；部门写入失败整事务回滚并跳过员工；发布镜像必须来自可追溯干净 Commit | [2026-07-28 组织同步历史 ID 冲突](#2026-07-28-p1-组织同步历史本地-id-与租户前缀-id-冲突导致部门落库失败) |
| `org-sync` `department-membership` `counting` `multi-tenant` | 完整部门归属写租户隔离关系表；查询仅在无关系时回退主部门；直属人数按完整关系，父级汇总按员工集合去重；**部署 membership 特性后所有已有组织必须重新同步，否则 0 条 membership 导致多部门员工被遗漏** | [2026-07-28 组织同步仅保存主部门导致部门人数偏少](#2026-07-28-p1-组织同步仅保存主部门导致部门人数偏少) |
| `org-sync` `dingtalk` `mobile` `unique-index` `null` | 钉钉空手机号不得转换为共享占位值；新员工空手机号写 `NULL`，已有真实手机号不得被空值或占位值覆盖 | [2026-07-28 组织同步共享手机号占位值冲突](#2026-07-28-p1-组织同步共享手机号占位值触发唯一索引冲突) |
| `attendance-toolbox` `timeout` `nginx` `gateway` | 工具箱长任务必须保持后端超时 < 网关超时 < 客户端超时；502/503/504 或 HTML 网关页只能映射为安全中文提示，禁止原样展示 | 同上（同根因复发记录） |
| `org-sync` `dingtalk` `hrm` `employee-profile` `field-filter-list` | 花名册职级/岗位序列/员工类型必须有钉钉 HRM 字段映射；**`field_filter_list` 必须包含标准系统字段代码（`sys01-employeeType`、`sys01-positionLevel`、`sys00-position`），否则 API 不返回对应字段**；空外部值不得覆盖本地人工值；HRM 接口成功但目标字段全空时返回 `success_no_fields` 诊断，禁止简单显示成功 | [2026-07-29 钉钉 HRM 字段代码未加入请求导致字段缺失](#2026-07-29-p1-钉钉-hrm-字段代码未加入-field_filter_list-导致员工类型职级全缺失) |
| `org-sync` `dingtalk` `deactivation` `fail-closed` `multi-tenant` `session` | 只有部门+员工完整拉取成功后才收口历史员工；钉钉源为空、请求失败、权限失败或同步被取消时禁止批量停用；停用仅作用于本组织 active 且有稳定 DingTalkUserID 的同步用户，admin/手工账号/无稳定 ID 账号不动；用户与档案状态同事务更新，会话撤销只针对实际被停用员工 | [2026-07-28 组织同步历史员工状态收口](#2026-07-28-p1-组织同步未收口历史员工状态导致离职员工仍为-active) |
| `api-contract` `router` `attendance` | 前端已调用且后端已有 Handler 的接口必须同时注册到 Router；新增 API 必须有路由清单回归测试，避免运行时 404 | [2026-07-23 考勤查询外部结果路由漏注册](#2026-07-23-p1-考勤查询外部结果路由漏注册导致进入页面-404) |
| `gorm` `update-columns` `terminal-status` `dingtalk` | `Updates/UpdateColumns(map)` 的 key 必须使用真实数据库列名；缩写字段优先显式 `column:` 或查 schema；终态日志写入错误不得忽略，更不得继续返回受理/成功 | [2026-07-29 群推送日志终态未落库](#2026-07-29-p2-gorm-缩写字段列名不一致导致群推送日志停留-processing) |
| `attendance` `data-contract` `metrics` `frontend` | 同一业务指标必须明确数据源、计数单位和分母；禁止首页和业务页面各自维护不同统计口径 | [2026-07-23 P2 考勤统计口径分裂](#2026-07-23-p2-考勤统计口径分裂导致数字不可比) |

### 前端 / 时间展示

| 标签 | 约束摘要 | 条目 / 文档 |
|---|---|---|
| `attendance` `frontend` `timezone` `test` | 考勤时间必须显式按业务时区 UTC+8 格式化；禁止依赖浏览器、Node 或 CI 宿主机时区 | [2026-07-24 考勤时间展示依赖宿主时区](#2026-07-24-p2-考勤时间展示依赖宿主时区导致-ci-失败) |
| `week-schedule` `notification-copy` `date-aware` | 作息表通知必须定位最近周六并读取实际日历状态；周五写“明天”、周六写“今天”、周一至周四写“本周六”、周日写“下周六”；是否上班置于首行，大/小周仅作补充 | [2026-07-29 作息表推送只判断明天导致周六提醒不灵活](#2026-07-29-p2-作息表推送只判断明天导致周六上班提醒不灵活) |

### 部署 / 配置

| 标签 | 约束摘要 | 条目 / 文档 |
|---|---|---|
| `deploy` `test-server` | 测试服隔离目录/端口/Compose 项目名；完整变量见 `deploy/peopleops.test.env.example`，文档不复制密钥 | `deploy/TEST_SERVER_DEPLOY.md` |
| `dingtalk-stream` `multi-tenant` `credentials` `app-home-url` `fail-closed` | Stream 显式组织必须读取同一组织的 AppKey/Secret；Compose 禁止默认覆盖为 `default`；非 default 组织群图片推送必须配置组织级公网 HTTPS AppHomeURL/RedirectURI；上线核对 org、healthy、重启次数并执行真实绑定/推送 | [2026-07-29 Stream 连接错误组织导致群机器人无响应](#2026-07-29-p1-dingtalk-stream-默认绑定错误组织导致群机器人无响应) |
| `deploy` `upload-and-restart` | 上传失败续传用独立脚本，禁止改 `build-and-deploy.ps1` 行为 | cerebrum Decision Log |
| `git` `merge` `release` | 跨分支冲突必须按语义单元核对生产代码、配套测试和 `go.mod/go.sum`；禁止按文件整侧选取后直接推送，至少执行编译、全量测试与双远端祖先检查 | [2026-07-23 双远端 master 合并遗漏依赖与安全配套代码](#2026-07-23-p1-双远端-master-合并遗漏依赖与安全配套代码) |

---

## 写入规则

### 开发前（必读）

1. 打开本文件，阅读「开发前必读索引」。
2. 按当前任务模块/标签，读取相关条目正文（不必通读全部历史）。
3. 在修改计划中写明：**适用的防复发约束**（引用条目日期或标签）。

### 开发后（必记）

任务收尾、写复盘总结前，必须判断：

| 情况 | 动作 |
|---|---|
| 已定位根因，且对后续开发有复用价值 | **新增**条目，或**更新**同根因已有条目 |
| 同根因再次出现 | 更新原条目的修复/验证/状态，禁止另开重复条目 |
| 仅一次性环境故障、外部工具临时不可用，且未形成项目内改进项 | 可不写入本文件，但最终总结必须说明原因 |
| 敏感信息（密码、Token、个人手机号/邮箱等） | **禁止**写入 |

### 记录范围

**应记录：**

- 业务缺陷（错误规则、错误状态流转）
- 代码缺陷（错误构造器、错误作用域、竞态、fail-open）
- 测试遗漏（本应覆盖却未覆盖的回归点）
- 代码审查发现的可复现问题
- 部署/配置问题（错误默认、错误回退、隔离失败）

**不记录：**

- 密码、Token、密钥、证书内容
- 可识别个人信息（完整手机号、邮箱、真实姓名+身份证等）
- 纯文案/样式微调且无行为影响
- 未定位根因的临时猜测

### 日志结构说明

- **索引区**（本文顶部）：短约束 + 跳转，避免每次全量读历史。
- **条目区**（下方）：完整复盘；按时间倒序，新条目置顶于「问题条目」第一节。
- 模块标签用于过滤；状态：`open` / `fixed` / `wontfix`。

---

## 统一条目模板

复制以下模板新增条目：

```markdown
### YYYY-MM-DD P? <简短标题>

| 字段 | 内容 |
|---|---|
| 日期 | YYYY-MM-DD |
| 级别 | P0 / P1 / P2 / P3 |
| 模块/标签 | `module` `tag` |
| 范围 | 后端 / 前端 / 全栈 / 部署 |
| 现象 | 用户可见或可复现的表现；可附错误文案（脱敏） |
| 根因 | 已验证的根本原因 |
| 修复 | 改了什么（文件/关键调用），不要贴密钥 |
| 验证 | 运行了哪些命令、结果 |
| 防复发 | 以后必须遵守的约束（可进索引） |
| 状态 | open / fixed / wontfix |
```

---

## 问题条目

### 2026-07-30 P2 考勤工具箱 E2E 重复日期占位符导致 strict mode 失败

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-30 |
| 级别 | P2 |
| 模块/标签 | `attendance-toolbox` `frontend` `playwright` `locator` `test` |
| 范围 | 前端 E2E 测试 |
| 现象 | Chromium E2E 的“一键同步”用例在填写开始日期时失败：Playwright strict mode 发现页面同时有“请假明细”和“钉钉同步”两个 `placeholder="开始日期"` 输入框，无法唯一定位。 |
| 根因 | 用例在切换到“钉钉同步”标签页后仍使用全页面 `getByPlaceholder`；Ant Design Tabs 保留了其他面板中的同名日期输入框，新增同名控件后原定位器不再唯一。 |
| 修复 | 先通过可访问名称定位“钉钉同步” `tabpanel`，再在该面板内查找开始/结束日期，保持用例与真实交互区域一致。 |
| 验证 | 定向 Chromium E2E `1/1` 通过；完整 Chromium E2E `15/15` 通过。 |
| 防复发 | 1. 存在重复 placeholder、按钮文案或表单标签时，先按 `tabpanel` / `form` / `dialog` / `region` 缩小作用域。<br>2. 优先使用稳定的可访问名称与 role，禁止依赖页面当前“恰好只有一个”的模糊定位。<br>3. 页面新增同名控件后必须回归相关 E2E 严格模式。 |
| 状态 | fixed |

### 2026-07-29 P2 作息表推送只判断明天导致周六上班提醒不灵活

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-29 |
| 级别 | P2 |
| 模块/标签 | `week-schedule` `notification-copy` `date-aware` `frontend` `jobs` `test` |
| 范围 | 作息表个人/群聊推送预览、周五自动提醒 |
| 现象 | 在周三等非周五日期推送月作息表时，文案只提示“明天上班/休息”，没有突出员工最关心的“本周六是否需要上班”；日期变化后措辞也不会自动切换为今天、明天、本周六或下周六。 |
| 根因 | 前端文案生成仅查找 `today + 1 day`，将明天状态作为通知重点；周五自动提醒则在固定句式中先显示大/小周，再显示明天状态。两处没有抽象“最近周六 + 与今天的日期关系”，也没有把周六实际 `saturday_work` / 节假日调班状态作为主信息。 |
| 修复 | 前端定位最近周六，使用对应周数据经 `getDayState` 解析真实工作/休息状态，并按发送日生成“今天/明天/本周六/下周六”；首行固定突出“需上班/休息”，大/小周和月作息表后置。周五自动提醒统一为“明天需上班/明天休息”优先。新增前端四种日期关系测试、实际 FormData 文案断言及后端上班/休息文案测试。 |
| 验证 | 前端定向测试 7/7、全量测试 25 files / 321 tests、`npm run lint`、`npm run build` 全部通过；Go 定向测试、`go test ./... -count=1`、`go vet ./...`、`golangci-lint run` 全部通过；新镜像部署后主 API 与 Stream 均 healthy、重启次数 0，公网首页和 `/health` 返回 200。 |
| 防复发 | 1. 作息表通知的核心问题是最近周六是否上班，禁止只读取明天状态。<br>2. 是否上班必须使用周六实际日历状态（含节假日调班），禁止仅由大周/小周文案推断。<br>3. 日期关系必须覆盖周一至周四、周五、周六、周日四类，并有固定文案测试。<br>4. 个人和群聊推送复用同一前端内容，自动提醒保持同一信息优先级。 |
| 状态 | fixed（代码、测试与测试服部署完成） |

### 2026-07-29 P1 dingtalk-stream 默认绑定错误组织导致群机器人无响应

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-29 |
| 级别 | P1 |
| 模块/标签 | `dingtalk-stream` `week-schedule` `multi-tenant` `credentials` `deploy` `fail-closed` |
| 范围 | 钉钉 Stream 群机器人回调、作息表群聊绑定、测试服部署配置 |
| 现象 | `dingtalk-stream` 容器显示 healthy、重启次数为 0，日志也显示 Stream 已连接，但群内 @机器人发送“绑定作息表”无回复，绑定表无新增记录。修正应用归属后群聊绑定成功，但首次群推送又提示“当前组织未配置可供钉钉访问的 HTTPS 应用地址”。 |
| 根因 | 测试 Compose 将 `DINGTALK_STREAM_ORG_ID` 默认强制为 `default`，Stream 因而连接了 default 组织的应用，而实际群机器人属于另一 active 组织。旧启动逻辑即使显式指定组织，仍要求该组织 AppKey 与主服务全局环境 AppKey 一致，导致单个共享 env_file 无法让 Stream 使用非 default 组织持久化的成套凭据。该目标组织在多组织环境配置和数据库中的 `app_home_url` / `redirect_uri` 同时仍为 HTTP；只修改全局 URL 对非 default 组织无效。健康检查只能证明进程和连接存活，不能证明应用归属及组织级公开 URL 正确。 |
| 修复 | `cmd/dingtalk_stream/main.go` 新增连接配置解析：显式 `DINGTALK_STREAM_ORG_ID` 时从该 active 组织读取配套 AppKey/Secret；未显式配置时才使用全局环境凭据唯一匹配组织；缺组织/凭据继续 fail-closed。机器人回调增加只记录组织、消息类型、@标志和命令匹配布尔值的脱敏日志。测试/生产 Compose 移除 `default` 强制覆盖，环境变量示例补充 `DINGTALK_STREAM_ORG_ID`。测试服显式设置目标组织并仅重建 Stream 容器；同时备份并更新多组织环境配置与数据库，将该组织 `app_home_url` / `redirect_uri` 持久化为公网 HTTPS 地址。 |
| 验证 | `go test ./cmd/dingtalk_stream ./internal/service ./internal/dingtalk ./internal/api -count=1`、`go test ./... -count=1`、`go vet ./...`、`golangci-lint run` 全部通过；测试与生产 Compose 静默校验通过；镜像确认同时包含主服务和 Stream 二进制。测试服 Stream 以目标组织连接成功，容器 healthy、重启次数 0，主 API `/health` 为 200；群内真实 @命令已收到成功绑定回复；组织级公开 URL 已回读为 HTTPS，无效临时图片 token 经公网返回 404；页面提交显示“已提交”，群聊实际收到文字与月作息表图片，最新群推送审计日志状态为 `submitted`。 |
| 防复发 | 1. Compose 不得用 `${VAR:-default}` 覆盖 Stream 组织；目标组织必须由部署环境显式声明。<br>2. 显式组织的 Stream 凭据必须全部来自同一组织配置，禁止把全局 AppKey/Secret 与其他组织 ID 混用。<br>3. 非 default 组织不得依赖全局应用 URL；启用机器人群图片推送前必须同时校验该组织 `app_home_url` 为公网 HTTPS、`redirect_uri` 为同域 HTTPS，并验证无效图片 token 返回 404 而非 502/503。<br>4. 上线验收除 healthy、重启次数和“Stream 已连接”外，必须核对日志中的 `org_id` 与机器人 Client ID 的掩码归属，并执行真实 @绑定和群推送。<br>5. 回调可观测日志禁止记录消息正文、SessionWebhook、AppSecret、Token 或完整 AppKey。 |
| 状态 | fixed（代码、测试服部署、群聊绑定及真实群推送验收全部完成） |

### 2026-07-28 P1 组织同步未收口历史员工状态导致离职员工仍为 active

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-28 |
| 级别 | P1 |
| 模块/标签 | `org-sync` `dingtalk` `deactivation` `fail-closed` `multi-tenant` `session` `audit` |
| 范围 | 钉钉完整通讯录同步、历史员工状态收口、员工档案状态、用户会话撤销、同步结果审计 |
| 现象 | 钉钉完整通讯录返回 530 名在职员工，本地数据库仍有 546 名 active 员工，其中 16 名已不在本次钉钉通讯录中（离职/转岗到不可见范围）。当前同步只新增/更新源数据中的员工，不处理源数据中已不存在的历史员工，导致本地长期保留离职 active 员工，仍可登录、仍计入花名册统计。 |
| 根因 | 组织同步缺少“源数据已不存在”的收口阶段：只在部门+员工都成功拉取时才应执行停用，但原实现无论源是否完整都只做 upsert；空源、请求失败、权限失败或同步被取消时若误停用会造成全员工停用事故。停用对象必须限定本组织 active 且有稳定 `ding_talk_user_id` 的同步用户，admin/本地手工账号/无稳定钉钉 ID 的账号不能动；用户 `status` 与 `EmployeeProfile.ProfileStatus` 必须事务一致，会话撤销只能针对实际被停用的员工。 |
| 修复 | 仓储新增 `DeactivateUsersMissingFromDingTalk(sourceDingTalkUserIDs)`：空源 fail-closed（`ErrInvalidData`）；只筛选本组织 `status=active` 且 `ding_talk_user_id <> ''` 的候选，源中不存在的标为 `inactive`；同一事务内更新 `users.status` 与 `employee_profiles.profile_status`，返回实际被停用的本地 `user_id` 列表。服务/仓储暴露 `GetUserByDingTalkUserID`、`ReplaceDepartmentMemberships`、`DeactivateUsersMissingFromDingTalk`。`syncDingTalkUsers` 在 `ctx.Err()==nil` 且去重后源列表非空时调用停用，`DeactivatedMissingCount` 计入响应；停用失败只标记 `partial_failed/EMPLOYEE_DEACTIVATION_FAILED`，不增加员工源 `fail_count`，会话撤销仅对实际停用员工执行。钉钉拉取层对任一部门员工请求失败、响应缺少/错误的 `result/list/has_more`、分页游标或用户 ID，以及完整源为空均 fail-closed；同一员工跨部门出现时合并全部部门 ID，禁止用部分源数据停用历史员工。 |
| 验证 | 仓储层 `TestDeactivateUsersMissingFromDingTalk_*` 覆盖正常停用、空源 fail-closed、手工账号不受影响、跨租户隔离、空 org fail-closed 和无候选；API 层覆盖空源/取消不执行停用、停用失败保持员工源计数、成功停用才撤销会话及完整多部门关系；钉钉层覆盖部门请求失败、空源、响应结构和分页异常。相关 Go 包、`go test ./...`、`go vet ./...`、`golangci-lint run` 均通过；前端定向与全量测试、lint、build 均通过。独立不可变镜像已部署到测试服，容器 healthy、重启次数 0、`/health` 200；真实页面同步数据验收仍待已登录会话触发。 |
| 防复发 | 1. 历史员工收口必须以前置阶段（部门+员工源）完整成功为前提；源为空、请求失败、权限失败或同步被取消时禁止批量停用。<br>2. 停用对象只限本组织 `active` 且有稳定 `ding_talk_user_id` 的同步用户；admin、本地手工账号、无稳定钉钉 ID 的账号不得停用。<br>3. 用户 `status` 与 `EmployeeProfile.ProfileStatus` 必须同一事务更新；会话撤销只针对实际被停用员工，停用失败不得撤销任何会话。<br>4. 所有停用 SQL 必须带 `org_id`；跨租户同名/同钉钉 ID 不得互相影响。<br>5. 同步结果必须输出 `deactivated_missing_count` 与停用子状态，便于审计；停用失败标记 partial，不得掩盖为基础通讯录成功。 |
| 状态 | fixed（代码、本地验证与测试服部署完成；真实页面同步验收待执行） |

### 2026-07-28 P1 组织同步仅保存主部门导致部门人数偏少

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-28 |
| 级别 | P1 |
| 模块/标签 | `org-sync` `department-membership` `department-count` `employee-list` `multi-tenant` `stable-id` `counting` |
| 范围 | 钉钉员工部门归属同步、员工—部门数据模型、部门树人数、部门员工列表、历史数据兼容 |
| 现象 | 钉钉 530 名在职员工共有 763 条部门成员关系，其中 149 人属于两个或多个部门；本地只保存 `DeptIDList[0]`，导致兼任部门直属人数和员工列表偏少，父部门简单累加时又存在重复统计风险。 |
| 根因 | `User.DepartmentID` 是单值历史模型，组织同步丢弃了 `DeptIDList` 的其余部门；部门查询与树统计只按主部门过滤和累加，没有“完整直属关系 + 父级员工集合去重”的统一口径，也没有区分“已生成关系”和“历史数据回退”。 |
| 修复 | 保留 `User.DepartmentID` 为主部门，新增按 `org_id + user_id + department_id` 唯一的 `user_department_memberships`。同步按钉钉顺序去重并解析当前租户有效部门，第一条有效部门标记主部门；关系替换在事务内校验同租户用户和部门。部门查询优先关系表，仅在员工完全没有关系记录时回退 `User.DepartmentID`；部门树直属人数按完整关系统计，含下级人数按 `user_id` 集合并集去重。关系表加入组织复合唯一索引 Prepare/Verify 迁移矩阵，迁移可重复执行且禁止把缺失组织的关系行回填为 default。新增 SQLite 行为测试覆盖单部门、多部门、重复 ID、历史本地 ID、同 ID 跨租户、兼任列表与父级汇总。 |
| 验证 | `go test ./internal/api ./internal/repository ./internal/service ./internal/database -count=1` 通过；其余静态检查见本次任务测试报告。2026-07-29 测试服数据库对账确认：muteng 组织 528 active 全部有 profile 和 membership、761 条关系、149 名多部门员工、根部门去重 active=528 与钉钉一致；104 个部门直属人数因多部门关系修复后高于旧主部门口径（差值 1-22 人不等），无脏关系/重复/孤儿部门。新增 5 个 Go 测试覆盖停用员工、兄弟部门同员工去重、无档案员工排除、三级树去重、零 membership 回退。前端树节点增加 Tooltip 与口径说明 Alert。 |
| 防复发 | 1. 第三方多值归属不得压缩进单值兼容字段；主字段与完整关系必须同时维护。<br>2. 有关系记录时关系表是部门归属事实来源；只有完全无关系的历史员工才回退主部门。<br>3. 直属人数可在多个部门分别计数，祖先汇总必须按员工标识求集合并集，禁止直接相加。<br>4. 关系表唯一键、读写条件、用户和部门校验都必须包含 `org_id`；相同用户/部门 ID 在不同租户下必须互不影响。迁移必须同时进入 Prepare/Verify 契约并可重复执行，禁止默认租户回填掩盖脏数据。<br>5. 外部部门 ID 必须先按当前租户解析为稳定本地 ID，查不到或跨租户的部门不得写入关系。<br>6. 部署 membership 特性后，所有已有组织必须重新执行组织同步以生成关系数据；未同步的组织 0 条 membership，全部依赖主部门回退，多部门员工会被遗漏。新增组织或同步后需验证 `user_department_memberships` 行数 > 0。 |
| 状态 | fixed |

### 2026-07-28 P1 组织同步共享手机号占位值触发唯一索引冲突

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-28 |
| 级别 | P1 |
| 模块/标签 | `org-sync` `dingtalk` `mobile` `unique-index` `null` `timeout` `operations` |
| 范围 | 钉钉员工解析、组织同步用户写入、租户手机号唯一索引、长任务请求生命周期、测试服数据恢复 |
| 现象 | 页面全量同步约 87 秒后收到 HTTP 504。服务端请求 `0a15bfab18ded125` 已完成 210 个部门，但外层连接取消后把 530 名员工全部误记失败。绕过代理再次执行时，请求 `51a059af681ab215` 暴露真实数据错误：529 名员工写入均触发 `idx_users_org_mobile` 唯一键冲突，仅 1 名成功。 |
| 根因 | 钉钉用户解析在手机号为空时为所有员工写入同一个 `10000000000`，与“生成唯一手机号”的注释相矛盾；新增和更新路径又无条件覆盖本地手机号，导致同组织复合唯一索引冲突。组织同步数据库上下文同时继承 HTTP 请求取消信号，外层代理断开后仍继续遍历剩余员工，形成大量 `context canceled` 查询和虚假失败计数。 |
| 修复 | 钉钉空手机号保持为空，共享占位值统一归一为空；新用户创建时仓储省略空 `mobile` 字段，使 MySQL 写入 `NULL` 并利用唯一索引允许多个空值；更新时空值/占位值不覆盖已有真实手机号。全量同步在完成认证和租户绑定后使用保留上下文值、脱离客户端取消且带 15 分钟上限的服务器执行上下文；员工循环仍在服务器执行超时或主动取消时立即停止。新增必须显式 `-confirm-sync` 的容器内运维工具用于绕过外层代理恢复数据。 |
| 验证 | 定向及完整相关包测试通过：`go test ./internal/api ./internal/middleware ./internal/dingtalk ./internal/repository -count=1`；`go vet` 通过；定向 `golangci-lint` 为 `0 issues`；`go build ./cmd/...` 通过。测试服真实同步请求 `4941356401dd17e3` 完成 210 个部门、530 名员工、员工失败 0；随后部署热修镜像并通过容器 healthy 与 `/health` 200 检查。 |
| 防复发 | 1. 外部手机号缺失时必须写数据库 `NULL`，禁止空字符串或共享假手机号占用业务唯一键。<br>2. 外部空值和已知占位值不得覆盖本地真实联系方式。<br>3. “生成唯一值”的兼容逻辑必须有多员工批量测试，禁止使用固定常量。<br>4. 长任务服务器执行上下文应保留租户和请求追踪值，但不得直接继承客户端断开信号；同时必须设置服务器执行上限。<br>5. 请求取消后循环必须立即停止，禁止把未处理项逐条累计为业务失败。<br>6. 数据恢复工具必须仅在服务器内部运行、显式确认组织、设置超时且不新增网络入口。 |
| 状态 | fixed |

### 2026-07-28 P1 组织同步历史本地 ID 与租户前缀 ID 冲突导致部门落库失败

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-28 |
| 级别 | P1 |
| 模块/标签 | `org-sync` `department` `employee` `stable-id` `multi-tenant` `transaction` `security` `release` |
| 范围 | 组织全量/独立部门同步、历史部门与员工身份兼容、同步状态与日志、发布可追溯性 |
| 现象 | 线上 `POST /api/v1/org/sync` 在钉钉成功返回 210 个部门后约 30 秒失败；数据库旧组织数据仍可读取。请求 `request_id=99b34d16ccd786ee` 的服务端日志显示部门写入触发租户内钉钉部门唯一键冲突，员工钉钉接口未继续调用。线上镜像使用可变 `test` 标签，且运行行为来自本地未提交工作树，无法由镜像内元数据追溯到干净 Commit。 |
| 根因 | 历史记录的本地 `department_id` / `user_id` 使用未加租户前缀的旧格式，新同步输入生成租户前缀 ID；同步只按新本地 ID 查找，未先按租户内稳定 `dingtalk_department_id` / `ding_talk_user_id` 匹配历史记录，因而把已有实体误判为新增并撞唯一索引。部门父级、员工部门/主管/档案引用也依赖历史本地 ID，不能通过直接改写线上数据解决。发布侧同时缺少不可变镜像标签和 Commit/构建时间元数据，脏工作树可能把无关改动一起打包。 |
| 修复 | 部门事务预加载当前组织全部记录，优先用稳定钉钉部门 ID 匹配并保留历史本地 ID；按稳定父部门 ID 解析真实本地父 ID，软删除记录可恢复，任一写入或变更日志失败整事务回滚。员工同步在新 scoped ID 未命中时按稳定钉钉用户 ID 回查，保留历史用户/档案/角色引用，并按稳定外部 ID 解析部门和主管。部门响应缺失/类型错误、空数组、根部门失败均 fail-closed；部门失败不覆盖旧数据，员工状态写 `skipped/EMPLOYEE_SYNC_SKIPPED`。DingTalk HTTP 边界统一脱敏 URL、Header、Token、Secret、Authorization、Cookie 与 DSN；API/同步状态返回安全错误码、`request_id`、耗时和计数。 |
| 验证 | 组织同步相关 Go 包与隔离构建上下文 `go test ./...` 通过；主工作区 `go test ./...`、`go vet ./...`、`golangci-lint run`（0 issues）通过。前端定向 3 files / 21 tests、全量 24 files / 313 tests、`npm run lint`、`npm run build` 通过。镜像从 `HEAD ea09a6f + 已审查组织补丁 d454f4d2` 的隔离上下文以 `--pull --no-cache` 构建，OCI revision/created 可追溯；测试服切换后容器 healthy、重启次数 0、`/health` 200。真实页面同步与人数口径仍待已登录会话验证。 |
| 防复发 | 1. 本地 ID 是内部引用，钉钉 ID 是租户内稳定外部身份；同步 upsert 必须先按 `org_id + DingTalk*ID` 匹配，禁止仅按新生成本地 ID 判断新增。<br>2. 兼容历史身份时保留本地 ID，并同步解析父部门、部门、主管、档案和角色引用；禁止手工批量改线上组织数据。<br>3. 部门源拉取、根部门校验和事务持久化任一步失败都不得覆盖旧数据，员工阶段必须 `skipped`。<br>4. 钉钉 HTTP 边界产生的错误在进入任何标准日志或 Logrus 前统一脱敏；响应和同步状态只保存稳定错误码与安全摘要。<br>5. 镜像必须从干净、已审查 Commit 构建，使用包含 Commit 的不可变标签并写入 OCI revision/created；禁止从混有无关改动的工作树直接部署。 |
| 状态 | fixed（代码、本地验证与测试服部署完成；真实页面同步验收待执行） |

### 2026-07-27 P1 组织全量同步被前端 10 秒超时误判失败

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-27 |
| 级别 | P1 |
| 模块/标签 | `org-sync` `attendance-toolbox` `frontend` `dingtalk` `timeout` `nginx` `gateway` `api-contract` `multi-tenant` `concurrency` `security` `counting` |
| 范围 | 全栈（组织同步链路 / 考勤工具箱钉钉同步 / 前端错误契约 / Nginx 与外层代理） |
| 现象 | 首轮修复解决了全量同步 10 秒超时和 200/207/500 契约，但二次审查仍发现发布阻断：任务中心用户/部门任务继续调用旧接口且继承 10 秒超时；三个入口没有共享锁；旧部门同步空 `org_id` 会归一到 default；旧接口把 `err.Error()` 写入响应和同步状态；部门落库失败后仍可能拉员工；默认角色与档案失败会让同一员工同时计入成功/失败或重复失败；HTTP 207 不刷新页面；日志只有错误类型，无法安全诊断。<br>同根因再次出现：考勤工具箱按整月实时拉取钉钉请假审批时，前端和后端均允许约 10 分钟，但生产 Nginx 仍按默认约 60 秒提前返回 504；前端又把 Nginx HTML 错误页原样展示在业务卡片中。 |
| 根因 | 修复范围只覆盖 `/org/sync`，没有把 `/sync/users`、`/sync/departments` 视为同一组织写链路；租户边界、锁、超时和错误契约分别散落在 Handler/页面。全量同步把部门源错误与部门阶段最终错误分开保存，落库错误未传给员工阶段。员工循环在角色、档案分支直接累加全局计数，没有每员工最终状态。前端完成回调与“全部成功”绑定。日志为了避敏只保留 `%T`，又缺少统一脱敏函数。<br>工具箱复发根因是长任务超时只在应用层配置，没有形成“后端最短、网关稍长、客户端最长”的部署契约；Blob 错误解析默认把非 JSON 文本直接透传，未识别 HTML 网关错误页。 |
| 修复 | **后端**：三个入口统一使用 JWT `org_id`，缺失返回 401，query/body/header 跨组织输入返回 403；共享按组织的进程内 `TryLock`，同组织冲突 409，不同组织并行，`defer` 保证成功、失败和 panic 展开时释放。旧接口升级为 200/207/500、安全文案、计数、`duration_ms`、`request_id` 契约。部门源拉取、校验或事务落库任一失败都会形成部门阶段最终错误并跳过员工接口。抽取员工逐项同步核心，去重后以 `itemFailed` 最终结算，角色失败可继续修复档案但每员工只计一次。响应/同步状态只写固定文案；`safeSyncErrorSummary` 脱敏 access token、Token/Secret/Password/Authorization/Bearer、AppKey/AppSecret、DSN/数据库密码、SQL 和控制字符，日志记录 request_id、脱敏 org、阶段、错误分类、摘要、耗时及计数。<br>**前端**：三个组织同步 API 复用 10 分钟独立超时；任务中心只映射后端实际返回的用户/部门/考勤任务，未知类型明确失败，不再回退全量同步；区分 200、207、500、409、401/403、超时和网络失败。`confirmOrgSync` 新增 `onCompleted(result)`，200/207 均刷新，500/超时/网络失败不盲目刷新，并保留刷新失败提示。考勤工具箱客户端等待统一为 660 秒；502/503/504、Axios 超时和 HTML Blob 错误统一映射为安全中文提示，禁止渲染网关 HTML。<br>**部署边界**：当前锁仅适用于单实例；多实例分布式锁或异步 job_id 任务作为后续方案。Nginx/外层代理对工具箱 `/api/` 使用 630 秒读写超时，形成后端 600 秒 < 网关 630 秒 < 客户端 660 秒；配置与 reload 检查写入 `deploy/README.md`。<br>**2026-07-28 同根因再次收口**：全量组织同步新增 `POST /org/sync/start` 与 `GET /org/sync/:request_id`，页面改为短启动+短轮询；后台复用原核心逻辑，使用脱离客户端取消且最多 15 分钟的执行上下文。同组织重复启动 409，不同组织互不影响；运行中和完整终态均持久化到 `SyncStatus`，查询同时绑定 JWT `org_id`、同步类型和 `request_id`。panic/超时写稳定安全错误码并释放门闩；执行上下文超时后，终态写入改用保留租户值且最多 5 秒的独立收尾上下文，避免记录永久停留在 `running`。原同步长入口继续供服务器内运维工具使用。 |
| 验证 | 后端异步定向与完整相关包测试通过；实际 Go 包全量 `go test ./internal/... ./cmd/... ./tools/... -count=1` 通过，`go vet ./internal/... ./cmd/... ./tools/...` 通过，`golangci-lint run ./internal/... ./cmd/... ./tools/...` 为 `0 issues`，`go build ./cmd/... ./tools/ops/sync_org_data` 通过。前端组织同步定向 2 files / 18 tests 通过，覆盖短请求启动、多次 running、success、partial_failed、failed、409、404/服务重启、页面轮询超时、网络失败、刷新回调失败与 HRM 原文脱敏；`npm run lint`、`npm run build` 通过。根目录 `go test ./...` 因既有无权限临时目录在包发现阶段阻塞，已改用实际包根执行，不将其记为代码测试失败。`go test -race` 本次未执行，不声称通过。未部署。 |
| 防复发 | 1. 所有组织写同步入口必须复用同一租户边界、并发门闩、安全错误契约和长任务超时，禁止只修全量入口。<br>2. JWT `org_id` 是唯一可信组织来源；空组织 fail-closed，禁止 `NormalizeOrganizationID("")` 回退 default。<br>3. 后续阶段是否执行必须依据前置阶段最终结果，包含源拉取、校验和事务落库。<br>4. 批量逐项处理先形成每项最终状态，再汇总成功/失败；禁止在多个子步骤直接累加全局计数。<br>5. HTTP 207 是有效完成结果，刷新已成功写入的数据；传输失败和 HTTP 500 不盲目刷新。<br>6. 响应与用户可见状态禁止原始 `err.Error()`、HTML 网关页或第三方原始错误；诊断日志必须统一脱敏并带 request_id。<br>7. 进程内锁只适用于单实例；多实例上线前必须实现分布式互斥或任务队列。<br>8. 所有预计超过 60 秒的页面操作优先设计为短启动+状态轮询，避免依赖任一层网关长连接；仍使用长请求时必须同时核对后端执行上限、Nginx/负载均衡/CDN 空闲超时和客户端超时。<br>9. 后台任务的运行中状态与完整终态必须持久化；结果查询同时使用当前 JWT `org_id`、任务类型和 `request_id`，禁止只按 request_id 或进程内 map 查询。<br>10. 业务执行上下文过期后不得继续用于终态落库；收尾写入应保留租户/追踪值、脱离已过期取消信号并设置独立短上限。 |
| 状态 | fixed |

### 2026-07-28 P1 钉钉 HRM 字段未映射导致花名册统计全未填写

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-28 |
| 级别 | P1 |
| 模块/标签 | `org-sync` `dingtalk` `hrm` `employee-profile` `data-contract` `multi-tenant` |
| 范围 | 全栈（钉钉 HRM 字段解析 / 员工档案写入 / 组织同步结果提示） |
| 现象 | 组织花名册已有 528 名在职员工且表格岗位正常，但员工类型、职级、岗位序列全部归入“未填写”，试用期人数为 0；点击同步组织数据后没有明确提示智能人事字段是否拉取成功。 |
| 根因 | `UserInfo` 和 HRM 解析只承载计划转正、实际转正与岗位，`applyDingTalkProfileFields` 没有写入 `employment_type/job_level/job_family`，因此概览只能统计空档案字段；HRM API 失败仅写 warning 后继续返回基础用户，主同步会被误认为完全成功；同时实际转正日期被错误写入 `probation_end_date`。截图顶部“岗位序列”读取 `EmployeeProfile.JobFamily`，与表格读取的 `User.Position` 不是同一字段。 |
| 修复 | 钉钉 HRM 同步新增员工类型、职级、岗位序列、试用期结束日期字段；默认组织支持环境变量字段代码/名称，多组织支持 `organizations.extension.dingtalk_hrm_field_codes/dingtalk_hrm_field_names`，非默认企业禁止继承默认映射。仅用非空钉钉值更新档案，保留人工维护值；实际转正日期不再覆盖试用期结束日期。同步响应新增四类缺失人数和 `hrm_field_status/hrm_field_error`；HRM 请求失败或响应结构异常时基础资料继续落库，但整体返回部分完成，禁止静默成功。前端同步提示展示字段缺失人数或智能人事权限错误；权限审批通过且响应有效时 HRM 子阶段及整体应为 success，仍审批中时只允许 HRM 子阶段导致 partial。 |
| 验证 | `go test ./internal/dingtalk ./internal/api -count=1` 通过；`go test ./... -count=1` 通过；`go vet ./...` 通过；`golangci-lint run` 为 `0 issues`；前端定向测试 1 file / 2 tests 通过；全量 `npm run test` 24 files / 305 tests 通过；`npm run lint`、`npm run build` 通过。 |
| 防复发 | 1. 第三方字段必须形成“请求字段代码 → 响应解析 → 本地模型写入 → 页面统计”的完整契约测试。<br>2. 外部系统空值不得覆盖本地人工维护字段；只有明确的全量覆盖操作才能清空。<br>3. 计划转正、实际转正、试用期结束日期语义分开存储，禁止相互代写。<br>4. 基础通讯录与 HRM 扩展字段属于不同同步子阶段；扩展字段失败必须显式 partial，禁止只记日志。<br>5. 企业自定义 HRM 字段按 `org_id` 配置，非默认企业不得回退全局环境变量。<br>6. 岗位 `User.Position` 与岗位序列 `EmployeeProfile.JobFamily` 必须在接口、文案和测试中保持区分。 |
| 状态 | fixed |

### 2026-07-29 P1 钉钉 HRM 字段代码未加入 field_filter_list 导致员工类型职级全缺失

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-29 |
| 级别 | P1 |
| 模块/标签 | `org-sync` `dingtalk` `hrm` `employee-profile` `field-filter-list` `diagnostic` |
| 范围 | 后端（钉钉 HRM 请求构建 / 字段解析 / 同步状态诊断）、前端（同步结果展示） |
| 现象 | 组织同步成功（210 部门、528 员工、失败 0），`hrm_field_status=success` 无权限错误，但 `employment_type_missing_count=528`、`job_level_missing_count=528`、`job_family_missing_count=528`——即所有员工的 HRM 扩展字段全部为空。 |
| 根因 | `configuredHRMFieldCodes` 构建 `field_filter_list` 时仅硬编码 `sys01-planRegularTime`、`sys01-regularTime`，未包含钉钉 HRM 标准系统字段代码 `sys01-employeeType`（员工类型）、`sys01-positionLevel`（岗位职级）、`sys00-position`（职位）。钉钉 HRM API `topapi/smartwork/hrm/employee/v2/list` 只返回 `field_filter_list` 中指定的字段，因此这些字段从未被请求，API 自然不返回。同时 `matchesConfiguredHRMField` 的候选列表未包含标准字段代码，即使 API 返回了 `field_code=sys01-employeeType` 也无法按 field_code 匹配（只能靠中文 `field_name` 匹配）。此外，HRM 接口成功但目标字段全空时仅显示 `success`，缺少诊断信息，导致问题难以发现。 |
| 修复 | 1. 新增 `defaultHRMFieldCodes` 函数，返回各字段对应的标准钉钉系统字段代码（`sys01-employeeType`、`sys01-positionLevel`、`sys00-position`）。2. `configuredHRMFieldCodes` 在遍历 `supportedHRMFieldKeys` 时附加 `defaultHRMFieldCodes(key)`，确保标准字段代码始终加入 `field_filter_list`。3. `matchesConfiguredHRMField` 候选列表增加 `defaultHRMFieldCodes(fieldKey)`，支持按 field_code 匹配。4. 新增 `hasAnyHRMTargetField` 检查同步后是否有任何员工获得了目标字段值；若无，标记 `HRMFieldSyncStatusNoFields`（`success_no_fields`）。5. `handlers.go` 处理 `success_no_fields` 状态，设置 `hrm_field_error` 诊断消息但不改变整体 success 状态。6. 前端 `formatHRMFieldSummary` 处理 `success_no_fields` 状态，展示诊断信息。 |
| 验证 | `go test ./internal/dingtalk/... ./internal/api/... -count=1` 通过；`go vet` 通过；`golangci-lint run` 0 issues；前端 `npm run test`、`npm run lint`、`npm run build` 通过。测试服组织同步验证：`employment_type_missing` 528→0，`job_level_missing` 528→20，`job_family_missing` 528→20，`regularization_date_missing` 236→18，`position_missing` 31→19，`hrm_field_status=success`（含字段值）。 |
| 防复发 | 1. 钉钉 HRM `field_filter_list` 必须包含所有目标字段的标准系统字段代码，不能仅依赖环境变量配置。2. 字段匹配候选列表必须同时包含标准 field_code 和中文 field_name。3. 第三方 API "成功但数据全空"属于异常状态，必须返回诊断信息而非简单 success。4. `field_filter_list` 变更时必须有测试验证标准字段代码被包含。 |
| 状态 | fixed |

### 2026-07-23 P1 考勤查询外部结果路由漏注册导致进入页面 404

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-23 |
| 级别 | P1 |
| 模块/标签 | `attendance` `api-contract` `router` `frontend` |
| 范围 | 全栈（前后端 API 契约） |
| 现象 | 进入考勤查询页面时，前端请求 `/attendance/external-sync/daily-results` 返回 404，页面显示“考勤结果加载失败”。状态、同步任务等同组外部考勤接口也未注册。 |
| 根因 | 前端 `attendanceAPI.externalSync` 和后端 `ExternalAttendance*` Handler 已实现，但 `internal/api/router.go` 的 `/attendance` 路由组遗漏了 `external-sync` 子路由；原有 Handler 单测未覆盖 Router 路径清单。 |
| 修复 | 在 `internal/api/router.go` 注册状态、每日结果、同步执行、任务列表和任务详情 5 个路由；查询沿用考勤菜单读权限或 `attendance_manage`，同步执行保留 `attendance_manage`；增加 `TestExternalAttendanceRoutesRegistered` 路由回归测试。 |
| 验证 | `go test ./internal/api -run 'TestExternalAttendance|Test.*Attendance.*Router' -count=1`、`go vet ./internal/api`、`golangci-lint run`、`cd frontend && npm run lint`、`cd frontend && npm run build` 均通过。 |
| 防复发 | 1. 新增或迁移前端 API 时，必须同时核对后端 Handler、Router 注册和权限中间件。<br>2. 每个跨前后端 API 子模块至少保留一条 Router 路径清单测试。<br>3. 页面首次加载使用的 GET 接口不能只依赖 Handler 单测，必须验证 `SetupRouter().Routes()` 中存在完整路径。 |
| 状态 | fixed |

### 2026-07-23 P1 绩效迁移 ID 比较触发 MySQL 排序规则冲突

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-23 |
| 级别 | P1 |
| 模块/标签 | `database-migration` `performance` `mysql` `collation` `deploy` |
| 范围 | 后端 / 数据库 / 部署 |
| 现象 | 测试服应用容器在启动迁移 `MigratePerformanceParticipantOrgIDsFromActivity` 的预检查询中失败，MySQL 返回 1267：`Illegal mix of collations (utf8mb4_general_ci,IMPLICIT) and (utf8mb4_unicode_ci,IMPLICIT) for operation '='`，导致健康检查失败。 |
| 根因 | `performance_activities.id` 是数值主键，`performance_participants.activity_id` 是 `varchar(64)`；迁移 SQL 使用 `CAST(a.id AS CHAR) = p.activity_id`。数值转字符表达式继承连接排序规则，而历史文本列使用另一排序规则，两个隐式字符串排序规则无法比较。原测试仅验证软删除、事务与关联表更新，没有锁定 JOIN 表达式的 collation 安全性。 |
| 修复 | `internal/database/schema_expand_migrations.go` 的预检和更新 JOIN 均改为 `CAST(a.id AS BINARY) = CAST(p.activity_id AS BINARY)`，保留原有文本精确匹配语义并绕开字符排序规则；`performance_org_migrate_test.go` 增加两条 SQL 的回归断言，禁止退回 `CAST(a.id AS CHAR)`。 |
| 验证 | `go test ./internal/database -run TestMigratePerformanceParticipantOrgIDs -count=1` 通过；`go test ./internal/database -count=1` 通过；`go vet ./internal/database` 通过；`go build ./cmd/...` 通过；`golangci-lint run --tests=false ./internal/database` 为 `0 issues`。全仓 `go vet ./...` 因工作区已有无权限目录 `codex_tmp_ascii/tmp*` 在包展开阶段阻塞，未作为代码失败处理。 |
| 防复发 | 1. MySQL 迁移中跨类型 ID 关联不得依赖连接/列默认 collation。<br>2. 需要保持文本精确语义时，双方显式转换为二进制字符串比较；禁止用宽松数值转换自动匹配异常历史值。<br>3. 启动迁移回归测试必须同时覆盖预检 SELECT 与事务内 UPDATE 的 JOIN 表达式。<br>4. 不通过全库改 collation 或修改生产数据来掩盖单条迁移 SQL 的类型问题。 |
| 状态 | fixed |

### 2026-07-24 P2 考勤时间展示依赖宿主时区导致 CI 失败

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-24 |
| 级别 | P2 |
| 模块/标签 | `attendance` `frontend` `timezone` `vitest` `ci` |
| 范围 | 前端 / 测试 / 发布 |
| 现象 | 考勤日结果测试在 Windows/CST 本地通过，但 GitHub Ubuntu/UTC 中把 `2026-07-16T09:12:00+08:00` 显示为 `01:12`，导致 `Attendance.daily.test.tsx` 找不到预期的 `09:12`，阻塞主分支 PR。生产页面在非 UTC+8 运行环境也会出现同类时间偏移。 |
| 根因 | `Attendance.tsx` 使用 `dayjs(value).format(...)`，格式化结果隐式采用宿主环境本地时区；代码没有声明 HR 业务时间统一使用 UTC+8。 |
| 修复 | `Attendance.tsx` 启用 Day.js `utc` 插件，`formatTime` / `formatDateTime` 在输出前显式切换到 UTC+8；未改测试期望。 |
| 验证 | `TZ=UTC npm run test -- src/pages/Attendance.daily.test.tsx` 2/2 通过；UTC 环境前端全量测试 18 files / 280 tests 通过；`npm run lint`、`npm run build` 通过。 |
| 防复发 | 1. 考勤与审批业务时间展示必须显式指定 UTC+8，不得依赖宿主时区。<br>2. 涉及时区的前端用例至少在 `TZ=UTC` 下验证一次。<br>3. 合并发布前必须执行 CI 同等的前端全量单测，不能只跑 lint/build。 |
| 状态 | fixed |

### 2026-07-23 P1 双远端 master 合并遗漏依赖与安全配套代码

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-23 |
| 级别 | P1 |
| 模块/标签 | `git` `merge` `release` `go-mod` `security` `multi-tenant` `test` |
| 范围 | 全栈 / 合并 / 发布 |
| 现象 | 将功能分支合入 GitHub `master` 时出现多文件冲突；初次按文件选择版本后，Go 构建缺少钉钉 Stream SDK 与 SQLite 测试依赖，同时已自动合入的安全测试找不到配套的路由白名单、管理员配置透传和迁移索引常量。若未执行完整验证，代码可能在推送后才暴露编译失败或 fail-open 回退。 |
| 根因 | 冲突解决以“文件选边”为单位，而实际变更跨越生产代码、测试和依赖清单：选择远端 `go.mod` 丢失当前分支依赖；选择当前生产文件又遗漏远端安全提交的配套符号。Git 自动合并测试文件不代表对应生产实现已经完整合入。 |
| 修复 | 合并依赖集合并保留 `golang.org/x/text v0.39.0`；恢复钉钉 `AppConfig.AdminUserID` 透传、空组织 fail-closed、按组织排班查询封装、路由只读菜单白名单和迁移索引常量；继续保留统一复合索引迁移以及 `DingTalkEventLog` AutoMigrate。 |
| 验证 | `go test ./cmd/... ./internal/... ./tools/...` 通过；`go vet` 通过；`golangci-lint` 0 issues；前端 lint、18 files / 280 tests、生产构建通过；Playwright 45 tests（三浏览器）通过。 |
| 防复发 | 1. 冲突按功能语义单元审查，生产代码、测试、`go.mod/go.sum` 必须联动。<br>2. 选择整侧版本后必须对照另一侧提交列出丢失符号，禁止仅凭“无冲突标记”判定完成。<br>3. 先跑冲突包的定向编译/测试，再跑全量 lint、单测、构建和关键 E2E。<br>4. 推送两个 `master` 前重新 fetch，并确认两个远端当前头都是集成提交的祖先；禁止强推。 |
| 状态 | fixed |

### 2026-07-21 P1/P2 UI 交互安全执行序收口（假成功/写门闩/导出/会话）

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-21 |
| 级别 | P1 / P2 |
| 模块/标签 | `ui` `security` `csrf` `logout` `org-sync` `attendance-toolbox` `export` `pii` |
| 范围 | 全栈（以前端门闩与路由权限对齐为主；不改业务状态机/权限码语义/配置文件） |
| 现象 | Setting 假保存密钥；SyncJobs 假 success；推送默认写死人名；org sync 多入口无 confirm/权限 UI；档案/流程写按钮无 hasPermission；导出 window.open；logout 后 RQ 缓存残留；Login `?error=` 反射；工具箱 API 可仅凭菜单绕过 feature 权限；upload/departments 门闩过松。 |
| 根因 | 前端写路径与后端权限不一致、假成功文案、硬编码测试数据、会话缓存未在 logout 清理；工具箱路由 `RequirePermissionOrMenu(..., menu)` 允许 menu-only。 |
| 修复 | Setting 改为密钥只读说明；SyncJobs 改真实 sync API+confirm；推送默认空选；`orgSyncAction` 统一 org sync；Profile/Flow/Detail/Shift/Processing 写门闩；export `downloadAuthorizedFile`；`queryClient.clear` on logout；`safeLoginErrorMessage`；列表 mobile/email 脱敏；绩效附件 `AuthorizedImage`；router toolbox 细权限 + upload/departments 门闩 + files TenantContext。 |
| 验证 | `cd frontend && npm run lint` 通过；`npm run test` 18 files / 280 tests 通过；`npm run build` 通过；`go test ./internal/api -run 'TestRunAttendanceToolbox\|TestRouter\|TestUpload\|TestLogout' -count=1` 通过。 |
| 防复发 | 1. 禁止 UI 假 success（不调 API 却提示成功）。<br>2. 写按钮缺 feature 权限用 disabled+Tooltip，与后端码一致。<br>3. 工具箱写路径禁止 menu-only 旁路。<br>4. logout 必须清 QueryClient；cookie 写请求走 api 实例 CSRF。<br>5. 登录错误文案走白名单，禁止 `?error=` 原样展示。 |
| 状态 | fixed |

### 2026-07-21 P1 Header 退出登录未带 CSRF 导致服务端会话可能残留

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-21 |
| 级别 | P1 |
| 模块/标签 | `auth` `logout` `csrf` `frontend` |
| 范围 | 前端 |
| 现象 | 点击 Header「退出登录」后前端已跳登录页，但服务端 HttpOnly 会话 cookie 可能仍有效；共享电脑或直接调 API 仍可带旧会话。 |
| 根因 | `App.tsx` `handleLogout` 使用裸 `axios.post('/api/v1/auth/logout')`，未走 `api` 实例拦截器，因而缺少 `X-CSRF-Token`。`JWTAuth` 对 cookie 会话的 POST 会校验 CSRF，失败返回 403；`finally` 仍清 zustand 并跳转，造成「客户端已退、服务端未退」。组织切换路径已正确使用 `authAPI.logout()`。 |
| 修复 | `frontend/src/App.tsx`：`handleLogout` 改为 `await authAPI.logout()`（自动 `withCredentials` + CSRF）；跳转与 remember org 行为不变。不改后端 Logout 业务语义。 |
| 验证 | `cd frontend && npm run lint`（含 App.tsx）通过。 |
| 防复发 | 1. 凡依赖 HttpOnly cookie 的写请求（含 logout）必须走 `frontend/src/services/api.ts` 的 `api` 实例或等价地附加 `csrfHeadersForMethod`。<br>2. 禁止在业务页对 `/api/v1/auth/logout` 使用裸 `axios.post`。<br>3. 退出/换组织两条路径应共用同一 logout 客户端实现。 |
| 状态 | fixed |

### 2026-07-21 P1 列德管理员角色跨 org 中毒导致权限查询 0 行

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-21 |
| 级别 | P1 |
| 模块/标签 | `auth` `permission` `user-role` `multi-tenant` `migration` `liede-admin` |
| 范围 | 后端 / 启动迁移 / 权限解析 |
| 现象 | 目标管理员登录 muteng/xiaotie 后 `permissions`/`menu_keys` SQL `rows=0`，前端表现为无权限；重启后若曾手工修好会再次丢失。 |
| 根因 | `migrateLiedeOrganizationAdminRoles` 启动时按姓名「列德」强制写管理员，但 `ensureRolePreset` 只按 `name` 取第一条角色（通常是 default 的管理员），再 `ensureUserRoleInOrg` 写入 `user_roles.org_id=muteng/xiaotie + role_id=default管理员`。权限查询要求 `roles.org_id = user_roles.org_id`，JOIN 失败返回空权限。 |
| 修复 | `ensureRolePresetInOrg` / `ensureRoleMenuPermissionInOrg` / `ensureRoleDataPermissionInOrg`；Liede 迁移对每个 org 取本组织管理员；`ensureUserRoleInOrg` 拒绝跨 org `role_id`；启动后 `remapCrossOrgUserRoleBindings` 修复已中毒绑定。单测：`liede_admin_role_org_isolation_test.go`。 |
| 验证 | `go build ./internal/database` 通过；`go test` 以 package sources + `liede_admin_role_org_isolation_test.go` 运行 4 用例通过；`go test ./internal/service -run 'TestAssignDefaultEmployeeRole_UsesCurrentOrgRole\|TestAssignUserRoleInOrg_OrgIsolation'` 通过。 |
| 防复发 | 1. 角色 preset/菜单/数据权限写入必须带 `org_id`，禁止只按 role 名或 role_id 跨 org 复用。<br>2. 写 `user_roles` 前必须校验 `roles.org_id == user_roles.org_id`。<br>3. 启动迁移若自动授权，必须按目标 org 取角色，不得把 default 角色 ID 绑到其他 org。<br>4. 权限解析 JOIN 保持 fail-closed（跨 org role_id 不得出权）。 |
| 状态 | fixed |

### 2026-07-21 P1 多组织唯一索引与旧兼容迁移不一致导致连续部署失败

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-21 |
| 级别 | P1 |
| 模块/标签 | `database-migration` `multi-tenant` `overtime` `dingtalk-binding` `user-role` `unique-index` `deploy` |
| 范围 | 后端 / 数据库 / 部署 |
| 现象 | 测试服连续三次在启动迁移阶段失败：先是 `overtime_rule_configs.rule_key`，再是 `ding_talk_bindings.ding_talk_user_id` 被错误按全局唯一处理；模型与统一迁移矩阵对齐后，旧角色兼容迁移又在 `(org_id, user_id)` 唯一索引已存在时执行 `UPDATE user_roles ... SET org_id = users.org_id`，触发 `idx_user_roles_org_user` 重复键，健康检查失败。 |
| 根因 | 多组织唯一约束没有形成单一迁移契约：部分 GORM 模型仍声明单列全局唯一，统一迁移矩阵未覆盖全部模型；同时 `migrateRolePermissionOrganizationScope`、`migrateUserRolesSingleRole`、`migrateMultitenantUniqueIndexes` 等旧兼容逻辑仍会在复合索引建立后重新改写 `org_id`、自动删除重复行或重复管理索引。模型标签、迁移矩阵和旧兼容回填三者不一致，导致每修复一张表就暴露下一处。另发现 `Department.DingTalkDepartmentID` 必须显式映射真实列名 `dingtalk_department_id`，不能依赖 GORM 默认拆词。 |
| 修复 | 统一对齐多组织模型的复合唯一标签与 `OrgCompositeUniqueSpec` 迁移矩阵；迁移顺序收口为 Prepare → AutoMigrate → Verify，并在旧兼容逻辑后做最终 Verify。角色和旧多租户兼容入口停止根据非唯一 `user_id` 推断/覆盖业务行 `org_id`，停止自动删除、合并重复 `user_roles`，停止重复创建统一矩阵已管理的索引；仅允许对缺失值做 `default` 归一。部署未上传或修改服务器运行配置。 |
| 验证 | `go build ./cmd/...` 通过；数据库非测试文件 `go vet` 通过；`golangci-lint run --tests=false --disable=unused ./internal/database` 为 `0 issues`（仅本机 lint 缓存目录无写权限警告）；独立 GORM schema 检查为 `mismatches=0`。`go test ./internal/database` 仍被工作区已有未定义测试辅助符号（`MigrateAnnualLeaveConsumeLogSchema`、`MigrateShiftCatalogSchema`、`backfillUserDingTalkIDDB`、`MigratePerformanceParticipantOrgIDsFromActivity`）阻塞。执行 `deploy/build-and-deploy.ps1 -SkipConfigUpload` 后应用、MySQL、Redis 容器均为 `healthy`，首次 `/health` 检查通过；服务器日志确认服务启动、定时任务启动且后续健康检查持续返回 200。 |
| 防复发 | 1. 多组织业务表的唯一键必须显式以 `org_id` 开头，禁止单列全局唯一。<br>2. 新增/修改租户模型时，必须同时校验 GORM schema 与统一迁移矩阵，列名特殊拆词必须显式写 `column:`。<br>3. 既有表迁移统一执行 Prepare → AutoMigrate → Verify；旧兼容入口不得在 Verify 后再次改写组织归属或索引。<br>4. 跨组织相同业务键合法；任何会改变 `org_id` 的回填必须先做目标状态冲突审计，歧义时 fail-closed 并人工确认。<br>5. 禁止通过自动删除、合并或选择保留业务记录来让唯一索引迁移通过；回归测试应覆盖模型标签、迁移矩阵、旧索引替换、跨组织同键和同组织冲突。 |
| 状态 | fixed |
### 2026-07-20 P1/P2 org_id 隔离缺口收口（首次补用户/登出审计/直接 GORM fail-open）

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-20 |
| 级别 | P1 / P2 |
| 模块/标签 | `auth` `multi-org` `dingtalk-login` `first-login` `logout` `audit` `shift-config` `attendance` `gorm` |
| 范围 | 后端 |
| 现象 | 1) 钉钉首次登录自动补用户仍用未绑定 org 的 `NewUserService`/`NewPermissionService`，与「已存在用户登录更新」同类缺口；2) `/auth/logout`、`/auth/me` 仅 JWTAuth 无 TenantContext，登出审计 `OperationLog` 缺 `org_id`；3) `ShiftConfigService`/`AttendanceService` 直接 `s.db` 查用户/部门/节假日/班次目录时可能无 org 过滤（fail-open 全表风险）。 |
| 根因 | 登录更新路径已改为 `WithOrgID`，但首次补用户与组织同步赋权路径未同步；会话路由未挂 TenantContext；服务构造只绑了 repository，业务内直接 GORM 未强制 org。 |
| 修复 | `handlers.go`：`ensureLocalUserForDingTalkLogin` 空 org fail-closed + `NewUserServiceWithOrgID`/`NewPermissionServiceWithOrgID`；`SyncUsers`/`SyncOrgData` permission 改为 WithOrgID；`Logout` 写 `OperationLog.OrgID`；`createOperationAuditLog` 带 OrgID。<br>`router.go`：`/auth/logout`、`/auth/me` 增加 `TenantContext`。<br>`shift_config_service.go`：`NewShiftConfigServiceWithOrgID` + `scopedDB`/`requireBoundOrgID` 覆盖用户/部门/节假日/catalog 查询。<br>`attendance_service.go`：构造注入 `orgID`；`loadUsers`/`loadDepartmentNames` 缺 org 返回 `ErrMissingOrgID`。 |
| 验证 | `go test ./internal/api -run 'Test.*(DingTalk\|Logout\|GetCurrentUser\|ShiftConfig\|MultiOrg\|EnsureLocalUser)' -count=1`<br>`go test ./internal/service -run 'Test.*(ShiftConfig\|Attendance\|Org\|GetAllWithUsers\|LoadUsers)' -count=1`<br>`go test ./internal/repository -run 'TestUserRepository_\|Test.*Org\|Test.*Isolation' -count=1`<br>`go vet ./internal/api ./internal/service ./internal/repository` |
| 防复发 | 1. 已解析 org 后用户/权限/业务读写必须 `NewXxxWithOrgID`。<br>2. 实体 `OrgID` 不能代替仓储/服务构造绑定。<br>3. 空 org fail-closed，禁止 `NormalizeOrganizationID("")` 发明 default 后继续写。<br>4. 直接 GORM 业务查询必须 org 条件或 1=0；禁止「if org!=\"\" 才过滤」的 fail-open。<br>5. 会话路由 logout/me 必须 TenantContext；审计日志必须写 org_id。 |
| 状态 | fixed |

### 2026-07-20 P1 钉钉多组织登录更新用户时缺少组织作用域

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-20 |
| 级别 | P1 |
| 模块/标签 | `auth` `multi-org` `dingtalk-login` `user-service` |
| 范围 | 后端（钉钉内免登 + 扫码回调） |
| 现象 | 任意组织登录失败，报错：`update local user failed: repository: orgID required for tenant-scoped operation` |
| 根因 | 钉钉内免登与扫码回调已解析 `orgID` / `resolvedOrgID`，但使用未绑定组织的 `NewUserService` 更新用户。实体字段 `User.OrgID` 不能代替仓储构造时的组织绑定；tenant-scoped repository 在空 org 下 fail-closed，因而更新失败。 |
| 修复 | `internal/api/handlers.go` 两处改为带组织构造：<br>- 内免登：`service.NewUserServiceWithOrgID(middleware.RequestDB(c), orgID)`<br>- 扫码回调：`service.NewUserServiceWithOrgID(middleware.RequestDB(c), resolvedOrgID)` |
| 验证 | `go test ./internal/repository -run '^TestUserRepository_' -count=1` 通过<br>`go test ./internal/api -run 'Test.*(DingTalk\|Dingtalk)' -count=1` 通过<br>`go vet ./internal/api ./internal/repository` 通过 |
| 防复发 | 1. 已解析组织后的用户读写必须用 `NewUserServiceWithOrgID`（或等价 `NewXxxWithOrgID`）。<br>2. 空组织继续 fail-closed，禁止发明 `default`。<br>3. 跨组织实体继续拒绝；禁止用实体上的 `OrgID` 字段代替仓储/服务构造绑定。<br>4. 相关回归：登录路径更新用户、缺 org、跨 org 写。 |
| 状态 | fixed |

### 2026-07-23 P2 考勤统计口径分裂导致数字不可比

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-23 |
| 级别 | P2 |
| 模块/标签 | `attendance` `data-contract` `metrics` `frontend` `api` |
| 范围 | 前端首页 / 考勤查询 / 旧异常统计页 / 后端统计接口 |
| 现象 | 考勤查询按外部同步每日结果展示员工日，旧异常统计页却按本地打卡记录和规则引擎重新计算；页面同时把员工人数、员工日和异常次数标成“人数”，导致同一日期范围的数字无法直接比较。首页考勤率继续依赖旧 `/attendance/stats`，进一步放大双口径问题。 |
| 根因 | 同一业务指标存在两套数据源和两套聚合逻辑，且没有固定“人数/人次/员工日”的展示契约；首页直接复用旧统计接口，使删除页面时仍残留后端依赖。 |
| 修复 | 统一以外部同步每日结果为准；首页读取 `external-sync/daily-results` 最近 30 天的 `summary`，按 `normal / total` 计算正常率，审批员工日保留在分母；删除旧异常统计页面、菜单、权限、路由、`/attendance/stats` Handler、旧规则引擎与相关文档。 |
| 验证 | 首页口径单测 4/4 通过；考勤 service/API 定向测试通过；前端 lint、build 通过；`go vet ./...` 通过；全量旧引用搜索为 0。`golangci-lint` 被本次范围外未跟踪文件 `internal/repository/external_approval_repository.go` 的 `rows.Close()` 未检查问题阻塞。 |
| 防复发 | 1. 考勤展示与首页指标统一读取外部每日结果，禁止另起规则引擎重复计算。<br>2. 新增统计字段必须写明数据源、时间范围、计数单位和分母。<br>3. 删除统计页面时必须同步核对首页、菜单权限、API 客户端、Router、Handler、Service 和文档引用。<br>4. 正常率固定为 `summary.normal / summary.total`，审批员工日保留在 `total` 分母。 |
| 状态 | fixed |

### 2026-07-29 P2 GORM 缩写字段列名不一致导致群推送日志停留 processing

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-29 |
| 级别 | P2 |
| 模块/标签 | `week-schedule` `dingtalk` `gorm` `update-columns` `terminal-status` `test` |
| 范围 | 后端 Service / 群推送日志 |
| 现象 | 钉钉受理或拒绝群消息后，接口业务流程已结束，但 `WeekScheduleGroupPushLog.status` 仍为 `processing`，请求 ID 和安全错误摘要未落库；若成功路径继续返回，将造成“页面已提交、审计日志未终态”的假受理风险。 |
| 根因 | 模型字段 `DingTalkRequestID` 按 GORM 默认命名映射为 `ding_talk_request_id`，终态更新却在 `UpdateColumns(map)` 中使用 `dingtalk_request_id`。map key 被视为原始列名，数据库报不存在列；原实现又丢弃更新错误，测试才观察到日志一直停留 `processing`。 |
| 修复 | 终态更新改用真实列名 `ding_talk_request_id`；`finishPushLog` 返回并检查 `result.Error` 与 `RowsAffected == 1`。钉钉受理路径只有在日志成功写为 `submitted` 后才返回；拒绝/失败路径保留原始第三方错误，同时尽力写安全终态。 |
| 验证 | `go test ./internal/service -run 'TestWeekScheduleGroupPushSubmittedAndDuplicateBlocked\|TestWeekScheduleGroupPushRejectionIsSafeAndRetryable' -count=1` 通过；相关包全量测试 `./internal/dingtalk ./internal/service ./internal/api ./internal/database ./cmd/dingtalk_stream` 通过。 |
| 防复发 | 1. GORM `Updates/UpdateColumns(map)` 的 key 必须核对真实 schema；含 `ID/API/URL/DingTalk` 等缩写的字段优先显式 `gorm:"column:..."` 或使用已确认列名。<br>2. 状态机终态、审计日志和幂等占位更新禁止忽略数据库错误，必须检查 `RowsAffected`。<br>3. 对外返回 success/submitted 前，先断言本地终态已持久化；不得用内存结构赋值掩盖落库失败。<br>4. 测试必须重新查询数据库，覆盖受理、拒绝和可重试状态，不能只断言 Service 返回值。 |
| 状态 | fixed |

---

### 2026-07-29 P3 dingtalk-stream 复用主镜像继承 HTTP 健康检查导致误报 unhealthy

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-29 |
| 级别 | P3 |
| 模块/标签 | `deploy` `dingtalk-stream` `docker` `healthcheck` |
| 范围 | 部署配置 / docker-compose |
| 现象 | dingtalk-stream 容器已启动并成功连接钉钉 Stream，但 `docker compose ps` 显示 `unhealthy`，干扰部署判断。 |
| 根因 | dingtalk-stream 与主服务共用同一镜像，镜像内 `HEALTHCHECK` 指向 `http://127.0.0.1:8080/health`；stream 进程不监听 8080，健康检查持续失败。 |
| 修复 | 在 `docker-compose.prod.yml` / `docker-compose.test.yml` 的 dingtalk-stream 服务上覆盖健康检查为进程存活检查 `kill -0 1 || exit 1`；stream 进程无 HTTP 健康端点时不得沿用主服务 HTTP 健康检查。 |
| 验证 | `docker compose -p peopleops-hr-test -f docker-compose.test.yml up -d` 后 dingtalk-stream 状态为 healthy，重启计数 0，日志含「钉钉 Stream 已连接」。 |
| 防复发 | 1. 复用主镜像的常驻进程类服务（stream/cron/worker）若不暴露 HTTP，必须在 compose 中显式覆盖 healthcheck 或 `disable: true`，不可沿用镜像默认 HTTP 健康检查。<br>2. 部署检查不仅看 `Up`，还要看 `healthy` 与重启计数是否增长。 |
| 状态 | fixed |

---

## 维护约定

- 新增条目后，同步更新顶部「开发前必读索引」对应行。
- 同一根因只保留一条主记录；历史过程写在同一条目内追加，不拆多条重复现象。
- 本文件由 AI 与人工共同维护；流程入口见 `AGENTS.md` 步骤 3 / 步骤 16 与 `.ai/AI_WORKFLOW.md`。
