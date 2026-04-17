# 验收需求矩阵

| 模块 | PRD功能点 | 页面 | 接口 | 数据表 | 权限点 | 异常点 | 是否可自动化 |
|------|-----------|------|------|--------|--------|--------|------------|
| 登录认证 | 账号密码登录 | Login.tsx | POST /api/v1/auth/login | login_logs, user_sessions | auth:login | 用户名或密码错误 | 是 |
| 登录认证 | 钉钉内免登 | Login.tsx | POST /api/v1/auth/dingtalk/in-app | login_logs, user_sessions, dingtalk_bindings | auth:dingtalk | 钉钉登录失败 | 是 |
| 登录认证 | 扫码登录 | Login.tsx | GET /api/v1/auth/dingtalk/qr/start | login_logs, user_sessions, dingtalk_bindings | auth:dingtalk | 扫码失败 | 是 |
| 登录认证 | 登出 | 全局导航 | POST /api/v1/auth/logout | user_sessions | auth:logout | 登出失败 | 是 |
| 登录认证 | 获取当前用户信息 | 全局导航 | GET /api/v1/auth/me | users | auth:me | 未授权 | 是 |
| 组织架构 | 同步部门信息 | SyncJobs.tsx | POST /api/v1/sync/departments | departments, sync_status | sync:departments | 同步失败 | 是 |
| 组织架构 | 同步员工信息 | SyncJobs.tsx | POST /api/v1/sync/users | users, sync_status | sync:users | 同步失败 | 是 |
| 组织架构 | 获取部门树 | DepartmentTree.tsx | GET /api/v1/org/departments/tree | departments | org:departments:tree | 无权限 | 是 |
| 组织架构 | 获取员工列表 | EmployeeList.tsx | GET /api/v1/org/employees | users | org:employees:list | 无权限 | 是 |
| 组织架构 | 获取员工详情 | EmployeeDetail.tsx | GET /api/v1/org/employees/:id | users | org:employees:detail | 无权限 | 是 |
| 组织架构 | 同步组织数据 | SyncJobs.tsx | POST /api/v1/org/sync | departments, users, sync_status | org:sync | 同步失败 | 是 |
| 考勤管理 | 获取考勤记录 | Attendance.tsx | GET /api/v1/attendance/records | attendance | attendance:records | 无权限 | 是 |
| 考勤管理 | 获取考勤统计 | AttendanceStats.tsx | GET /api/v1/attendance/stats | attendance | attendance:stats | 无权限 | 是 |
| 考勤管理 | 同步考勤数据 | Attendance.tsx | POST /api/v1/attendance/sync | attendance, sync_status | attendance:sync | 同步失败 | 是 |
| 考勤管理 | 导出考勤数据 | AttendanceExport.tsx | POST /api/v1/attendance/export | attendance_export | attendance:export | 导出失败 | 是 |
| 考勤管理 | 获取导出记录 | AttendanceExport.tsx | GET /api/v1/attendance/exports | attendance_export | attendance:exports | 无权限 | 是 |
| 考勤管理 | 获取最近同步时间 | Attendance.tsx | GET /api/v1/attendance/last-sync | sync_status | attendance:last-sync | 无权限 | 是 |
| 审批管理 | 获取审批模板 | ApprovalTemplate.tsx | GET /api/v1/approvals/templates | approval_template | approvals:templates | 无权限 | 是 |
| 审批管理 | 获取审批实例 | ApprovalInstance.tsx | GET /api/v1/approvals/instances | approval | approvals:instances | 无权限 | 是 |
| 审批管理 | 获取审批详情 | ApprovalDetail.tsx | GET /api/v1/approvals/:id | approval | approvals:detail | 无权限 | 是 |
| 审批管理 | 同步审批数据 | Approval.tsx | POST /api/v1/approvals/sync | approval, sync_status | approvals:sync | 同步失败 | 是 |
| 权限管理 | 获取角色列表 | RoleManagement.tsx | GET /api/v1/permission/roles | role | permission:roles | 无权限 | 是 |
| 权限管理 | 创建角色 | RoleManagement.tsx | POST /api/v1/permission/roles | role | permission:roles:create | 创建失败 | 是 |
| 权限管理 | 获取权限列表 | Permission.tsx | GET /api/v1/permission/permissions | permission | permission:permissions | 无权限 | 是 |
| 审计日志 | 获取审计日志 | AuditLogs.tsx | GET /api/v1/audit/logs | operation_log | audit:logs | 无权限 | 是 |
| 任务中心 | 获取任务列表 | SyncJobs.tsx | GET /api/v1/jobs | - | jobs:list | 无权限 | 是 |
| 任务中心 | 运行任务 | SyncJobs.tsx | POST /api/v1/jobs/:id/run | job_run_logs | jobs:run | 运行失败 | 是 |
| 员工档案 | 获取员工档案列表 | EmployeeProfile.tsx | GET /api/v1/employee/profiles | employee_profile | employee:profiles | 无权限 | 是 |
| 员工档案 | 获取员工档案详情 | EmployeeProfile.tsx | GET /api/v1/employee/profiles/:id | employee_profile | employee:profiles:detail | 无权限 | 是 |
| 员工档案 | 创建员工档案 | EmployeeProfile.tsx | POST /api/v1/employee/profiles | employee_profile | employee:profiles:create | 创建失败 | 是 |
| 员工档案 | 更新员工档案 | EmployeeProfile.tsx | PUT /api/v1/employee/profiles/:id | employee_profile | employee:profiles:update | 更新失败 | 是 |
| 员工转岗 | 获取转岗列表 | EmployeeFlow.tsx | GET /api/v1/employee/transfers | employee_transfer | employee:transfers | 无权限 | 是 |
| 员工转岗 | 创建转岗申请 | EmployeeFlow.tsx | POST /api/v1/employee/transfers | employee_transfer | employee:transfers:create | 创建失败 | 是 |
| 员工离职 | 获取离职列表 | EmployeeFlow.tsx | GET /api/v1/employee/resignations | employee_resignation | employee:resignations | 无权限 | 是 |
| 员工离职 | 创建离职申请 | EmployeeFlow.tsx | POST /api/v1/employee/resignations | employee_resignation | employee:resignations:create | 创建失败 | 是 |
| 员工入职 | 获取入职列表 | EmployeeFlow.tsx | GET /api/v1/employee/onboardings | employee_onboarding | employee:onboardings | 无权限 | 是 |
| 员工入职 | 创建入职申请 | EmployeeFlow.tsx | POST /api/v1/employee/onboardings | employee_onboarding | employee:onboardings:create | 创建失败 | 是 |
| 人才分析 | 获取人才分析列表 | TalentAnalysis.tsx | GET /api/v1/talent/analysis | talent_analysis | talent:analysis | 无权限 | 是 |
| 人才分析 | 获取人才分析详情 | TalentAnalysis.tsx | GET /api/v1/talent/analysis/:id | talent_analysis | talent:analysis:detail | 无权限 | 是 |
| 人才分析 | 创建人才分析 | TalentAnalysis.tsx | POST /api/v1/talent/analysis | talent_analysis | talent:analysis:create | 创建失败 | 是 |