# 组织模块

## 当前范围

- 页面：
  - `frontend/src/pages/Organization.tsx`：组织驾驶舱
  - `frontend/src/pages/DepartmentTree.tsx`：部门树与部门概览
  - `frontend/src/pages/TalentAnalysis.tsx`：人才结构分析
- 后端入口：
  - `GET /api/v1/org/dashboard`
  - `GET /api/v1/org/overview`
  - `GET /api/v1/org/structure-analysis`
  - `GET /api/v1/org/departments/tree`
  - `GET /api/v1/org/departments/overview`
  - `GET /api/v1/org/departments/:id/history`
  - `GET /api/v1/org/employees`
  - `GET /api/v1/org/employees/:id`

## 数据来源

- 组织、部门、员工主数据：本地 `users` / `departments`
- 员工档案补充字段：本地 `employee_profiles`
- 调岗统计：本地 `employee_transfers`
- 离职统计：本地 `employee_resignations`
- 入职趋势与司龄：优先使用 `employee_profiles.entry_date`
- 不依赖实时钉钉查询
- 不依赖绩效后端

## MVP 统计口径

- 统一筛选参数：
  - `department_id`：组织 / 部门筛选，默认按当前可见组织范围
  - `month`：统计月份，格式 `YYYY-MM`
  - `start_date` / `end_date`：统计时间范围，格式 `YYYY-MM-DD`，同时传入时优先级高于 `month`
  - `employment_type`：员工类型筛选，来源 `employee_profiles.employment_type`
  - `position`：岗位筛选，来源 `users.position`
  - `job_level`：职级筛选，来源 `employee_profiles.job_level`
- 组织驾驶舱：
  - 接口：`GET /api/v1/org/dashboard`
  - 新增返回：
    - `applied_filters`
    - `filter_options`
    - `warnings`
    - `trends`
    - `new_hire_ledger`
    - `regularization_tracks`
    - `transfer_details`
  - `employee_count`：纳入组织统计的员工总数
  - `active_employee_count`：在职人数
  - `probation_count`：在职且仍处于试用期的人数
  - `pending_regularization_count`：未来 30 天内转正预警人数
  - `new_hire_count`：期间入职人数，按 `employee_profiles.entry_date` 统计
  - `resignation_count`：期间已生效离职人数，按离职表 `approved/completed` 统计
  - `transfer_count`：期间已生效调岗人数，按调岗表 `approved/completed` 统计
  - `department_count`：当前筛选结果内出现员工的部门数
  - `manager_count`：当前范围内有直属下级或岗位名称呈现管理属性的人数
  - `stats_period_label`：当前统计期间标签；默认无参数时为当前月份
  - `new_hire_ledger`：新员工入职台账，当前基于 `employee_profiles.entry_date`、岗位、职级、试用与转正字段生成
  - `regularization_tracks`：试用期跟踪与计划 / 实际转正展示，当前基于 `planned_regular_date`、`probation_end_date`、`actual_regular_date`
  - `transfer_details`：期间内 `approved/completed` 调岗明细，展示部门与岗位变更留痕
- 部门概览：
  - 接口：`GET /api/v1/org/departments/overview`
  - 支持与组织驾驶舱相同的 query 参数
  - 返回 `applied_filters`
  - 统计口径默认包含所选部门及其下级部门汇总
  - `level` 为当前可见树中的层级，从根节点开始记为 1
- 人才结构分析：
  - 接口：`GET /api/v1/org/structure-analysis`
  - 新增返回：
    - `applied_filters`
    - `filter_options`
    - `time_filter_notice`
  - 默认按在职员工统计
  - 缺失字段统一展示为 `待补录`
  - 异常日期（如未来日期）展示为 `待确认`
  - 角色类别优先识别管理人员，其次识别技术人员，其余归为专业人员；无法判断时归为 `待确认`
  - 当前没有历史员工快照表；时间范围 / 统计月份仅影响筛选透传与统一提示，不支持历史组织结构回放

## 页面能力

- `frontend/src/pages/Organization.tsx`
  - 已升级为统一的“我的团队”入口，默认 Tab 为“人才管理驾驶舱”
  - 当前 Tab：人才管理驾驶舱、成员、团队概览、人才结构分析、人才盘点
  - 未接入 Tab 使用禁用态或空态，不调用不存在接口
  - 多维筛选：组织 / 部门、月份、时间范围、员工类型、岗位、职级
  - 当前统一筛选区只影响 `Organization.tsx` 内部的 `GET /api/v1/org/dashboard` 与 `GET /api/v1/org/departments/overview` 请求
  - 关键指标卡片、期间趋势、重点提醒
  - 部门人数排行榜、试用期人数排行
  - 新员工入职台账、试用期跟踪 / 转正进度、期间调岗明细
  - 年龄、学历、全量在职员工结构与人才盘点等缺少真实数据时，仅展示空态，不伪造结果
- `frontend/src/pages/TalentAnalysis.tsx`
  - 当前作为“我的团队”中的嵌入 Tab，仍保留自身筛选逻辑
  - 与组织驾驶舱同口径的多维筛选
  - 员工类型、角色类别、年龄、司龄、学历、职级、岗位、岗位序列分布
  - 时间筛选受限提示
- `frontend/src/pages/DepartmentTree.tsx`
  - 当前作为“我的团队”中的嵌入 Tab，仍保留自身筛选逻辑
  - 维持组织树与部门概览联动
  - 点击部门查看部门汇总统计和成员列表

## 当前限制

- 汇报关系变更暂无专用历史表或聚合接口，本阶段不伪造“汇报关系变更留痕”明细
- 新员工入职台账当前优先基于员工档案字段生成，不额外拼装完整 onboarding 流程节点明细
- 当前没有全量在职员工类型、司龄、年龄、学历等快照字段时，组织首页对应结构卡仅展示空态
- 权限与数据范围控制暂未纳入本阶段，仍沿用现有组织可见范围解析
- 不修改登录、年假、假勤、加班、调休、排班、权限 / 角色 / 数据范围、绩效后端

## 本阶段不做

- 登录与登录态逻辑修改
- 年假、假勤、加班、调休、排班模块改造
- 权限控制、角色控制、数据范围控制扩展
- 绩效后端接入
- 实时钉钉查询
