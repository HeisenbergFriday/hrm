---
purpose: 项目整体架构、数据流、核心设计约束
last_updated: 2026-05-26
source_of_truth:
  - go.mod（后端技术栈）
  - frontend/package.json（前端技术栈）
  - internal/api/router.go（路由架构）
  - internal/database/database.go（数据库初始化）
  - frontend/src/services/api.ts（前端 API 封装）
  - frontend/src/store/authStore.ts（状态管理）
update_when:
  - 修改技术栈时
  - 修改分层架构时
  - 修改数据流时
  - 修改跨模块调用方式时
  - 新增架构约束时
---

# 架构设计

## 技术栈

### 后端
- Go 1.20
- Gin（HTTP 框架）
- GORM（ORM）
- MySQL（主业务库）
- Redis（缓存）
- JWT（认证）
- Logrus（日志）

### 前端
- React 18
- TypeScript
- Vite 4
- Ant Design 5
- Zustand（状态管理）
- Axios（HTTP 客户端）
- React Query（数据获取）

### 外部集成
- 钉钉开放平台 API
- 聚合数据节假日接口（可选）

---

## 数据流

```
钉钉开放平台
    ↓ 同步（定时/手动）
本地 MySQL
    ↓ API 查询
前端页面
```

### 核心流程

1. **组织架构同步**：钉钉部门/用户 → 本地 `departments` / `users` 表
2. **考勤同步**：钉钉打卡记录 → 本地 `attendances` 表
3. **审批同步**：钉钉审批实例 → 本地 `approvals` 表
4. **年假发放**：本地计算 → 写入 `annual_leave_grants` → 同步到钉钉假期配置
5. **加班匹配**：钉钉审批 + 本地打卡 → 计算有效加班时长 → 写入 `overtime_match_results` → 生成调休余额 → 同步到钉钉
6. **大小周排班**：本地配置 `week_schedule_rules` → 计算每周班次 → 同步到钉钉考勤组
7. **绩效管理**：活动创建 → 参与人刷新 → 目标设定与审批 → 员工自评 → 上级评分 → 三级确认 → 锁定 → 归档
8. **员工全生命周期**：入职 → 档案管理 → 转岗 → 离职
9. **补卡申请**：员工提交补卡 → 审批流程 → 钉钉同步

---

## 核心设计约定

### 后端

#### 分层架构
```
Handler (api/)
    ↓
Service (service/)
    ↓
Repository (repository/)
    ↓
Model (database/models.go)
```

- **Handler 层**：Gin handler 直接在 `api/` 包，无 controller 层分离
- **Service 层**：业务逻辑，调用 repository 和外部服务（钉钉）
- **Repository 层**：数据访问封装，GORM 操作
- **Model 层**：GORM 模型定义，集中在 `models.go`

#### 统一响应格式
```go
type Response struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}
```

#### 认证
- JWT Bearer token
- Claims 含 `UserID` + `UserName`
- Handler 内通过 `c.Get("userID")` 取当前用户
- 中间件：`internal/middleware/jwt.go`

#### 钉钉 ID 存储
- `User.UserID` 和 `Department.DepartmentID` 存钉钉原始 ID（字符串），不是本地自增主键
- 本地自增主键是 `ID` 字段（uint）

#### 软删除
- 主要模型使用 `gorm.DeletedAt` 软删除
- 查询时 GORM 自动过滤已删除记录

#### JSON 字段
- 扩展数据用 `map[string]interface{}` 配合 `gorm:"type:json;serializer:json"`
- 例如：`User.Extension`、`Approval.Content`

#### 幂等性设计
- **年假消费**：`AnnualLeaveConsumeLog.request_ref` 唯一索引防重
- **加班同步**：`OvertimeSyncHistory` 快照，避免重复同步
- **加班匹配**：`OvertimeMatchResult.match_ref` 用于当前幂等，历史数据仍兼容 `user_id+work_date` 口径

#### 数据库初始化
- 启动时自动迁移（`AutoMigrate`）
- 库为空时种默认管理员 `admin / admin123`
- MySQL 连接失败时会尝试自动创建数据库后重连

#### 容错设计
- Redis 或钉钉客户端初始化失败不会阻止服务启动
- 相关功能会受影响，但基础服务可用

---

### 前端

#### 状态管理
- 认证状态：Zustand store 持久化到 localStorage，key 为 `peopleops-auth`
- 包含：`user`、`token`、`isLoggedIn`
- 动作：`login(user, token)`、`logout()`

#### API 调用
- 统一从 `frontend/src/services/api.ts` 进入
- Axios 实例 baseURL=`/api/v1`，timeout=10s
- 请求拦截：自动从 authStore 读取 token 加入 `Authorization: Bearer`
- 响应拦截：401 自动 logout + 跳转 `/login`

#### 路由与菜单
- `frontend/src/App.tsx` 同时负责主路由、菜单和钉钉内免登流程
- `/employees/:id` 会复用 `/employees` 的菜单高亮

#### 本地开发
- Vite 代理 `/api` 到 `http://localhost:8080`
- 前端默认端口 `3000`

#### UI 库
- Ant Design 5
- 所有表格/表单/弹窗使用 antd 组件

---

## 关键业务流程

### 加班→调休流程
1. 从钉钉同步加班审批（`SyncApproval`）
2. 运行匹配（`RunOvertimeMatch`）：审批记录↔打卡记录，计算 `effective_overtime_minutes`
3. 写入 `CompensatoryLeaveLedger`（credit）
4. 同步到钉钉假期余额（`ResyncOvertimeToDingTalk`）

### 年假发放流程
1. 按季度计算员工年假资格（`AnnualLeaveEligibility`）
2. 运行季度发放（`RunQuarterGrant`）→ 写入 `AnnualLeaveGrant`
3. 同步到钉钉假期配置（`SyncGrantsToDingTalk`）

### 大小周排班流程
1. 配置 `WeekScheduleRule`（基准周 + pattern）
2. 可手动覆盖特定周（`WeekScheduleOverride`）
3. 同步到钉钉班次（`SyncWeekToDingTalk`）：按用户分配钉钉 Shift

### 绩效管理流程
1. **活动配置**：HR 创建绩效活动，设置时间范围、参与人范围、关联指标库
2. **参与人刷新**：根据部门/员工范围筛选参与人
3. **目标设定**：上级为下属设定目标，员工确认，支持审批流程
4. **员工自评**：员工填写实际达成结果，系统计算自评总分
5. **上级评分**：上级为下属评分，系统自动计算等级（S/A/B/C/D），实时检查强制分布
6. **三级确认**：员工确认 → 上级确认（立即冻结结果）→ HR 确认
7. **锁定归档**：活动锁定，防止修改，归档保存历史

### 员工全生命周期
1. **入职**：新员工入职流程，写入 `EmployeeOnboarding`
2. **档案管理**：维护员工档案信息，写入 `EmployeeProfile`
3. **转岗**：员工转岗流程，写入 `EmployeeTransfer`
4. **离职**：员工离职流程，写入 `EmployeeResignation`

### 权限管理
- RBAC 模型：`Role` → `Permission` → `RolePermission` → `UserRole`
- 支持菜单权限和数据权限
- 前端页面：角色管理、权限管理、菜单权限、数据权限

### 审计日志
- 记录所有操作日志，写入 `OperationLog`
- 支持按用户、操作类型、时间范围查询

### 补卡申请
1. 员工提交补卡申请，写入 `OvertimeSupplementaryRequest`
2. 审批流程
3. 同步到钉钉

---

## 环境变量

### 基础运行
- `PORT`：服务端口，默认 8080
- `DATABASE_URL`：MySQL 连接串，格式 `user:pass@tcp(host:3306)/dbname?charset=utf8mb4&parseTime=True`
- `REDIS_URL`：Redis 地址，格式 `localhost:6379`（当前代码直接传给 `redis.Options.Addr`，不要带 `redis://` 前缀）
- `REDIS_PASSWORD`：Redis 密码（可选）
- `JWT_SECRET`：JWT 签名密钥

### 钉钉集成
- `DINGTALK_APP_KEY`：钉钉应用 Key
- `DINGTALK_APP_SECRET`：钉钉应用 Secret
- `DINGTALK_CORP_ID`：钉钉企业 ID
- `DINGTALK_AGENT_ID`：钉钉应用 Agent ID
- `DINGTALK_ADMIN_USER_ID`：钉钉管理员用户 ID
- `DINGTALK_REDIRECT_URI`：OAuth 回调地址
- `DINGTALK_APP_HOME_URL`：应用首页地址
- `APP_BASE_URL`：后端服务地址
- `FRONTEND_BASE_URL`：前端服务地址

### 假期/调休同步
- `DINGTALK_LEAVE_SYNC_ENABLED`：是否启用年假同步（true/false）
- `DINGTALK_COMP_TIME_SYNC_ENABLED`：是否启用调休同步（true/false）
- `DINGTALK_LEAVE_HOURS_PER_DAY`：每天工作小时数（用于天数换算）
- `DINGTALK_ANNUAL_LEAVE_CODE`：钉钉年假假期类型 Code
- `DINGTALK_ANNUAL_LEAVE_NAME`：钉钉年假假期类型名称
- `DINGTALK_LIEU_LEAVE_CODE`：钉钉调休假期类型 Code
- `DINGTALK_LIEU_LEAVE_NAME`：钉钉调休假期类型名称
- `DINGTALK_COMPENSATORY_LEAVE_CODE`：钉钉补偿假期类型 Code
- `DINGTALK_COMPENSATORY_LEAVE_NAME`：钉钉补偿假期类型名称
- `ANNUAL_LEAVE_APPROVAL_KEYWORD`：年假审批关键词（用于识别年假审批）

### 排班与节假日
- `DINGTALK_ATTENDANCE_GROUP_ID`：钉钉考勤组 ID
- `DINGTALK_ATTENDANCE_GROUP_NAME`：钉钉考勤组名称
- `JUHE_API_KEY`：聚合数据节假日接口 Key（可选）

### 测试
- `TEST_DATABASE_URL`：测试数据库连接串
- `SKIP_INTEGRATION_TESTS`：是否跳过集成测试（true/false）

---

## 数据与同步说明

- MySQL 是主业务库，启动时若数据库不存在会尝试自动创建
- Redis 当前主要用于缓存，初始化失败时服务仍可启动
- 钉钉客户端初始化失败时，服务仍可启动，但免登、同步、假期回写等能力会受影响
- 周排班、年假发放、加班匹配都依赖数据库迁移结果，修改模型时要同时考虑 migration 兼容性
- 后端静态托管的是已构建产物（`frontend/dist`），不会自动触发前端构建

---

## 协作建议

- 看接口入口时，优先从 `internal/api/router.go` 顺着 `handler → service → repository → model` 往下追
- 做排班、年假、调休相关改动时，先确认是否同时影响"本地台账"和"钉钉同步"
- 做前端联调时，注意很多页面依赖登录态和 `/api/v1/auth/me`
- 验证后端托管的前端路由前，记得先执行 `cd frontend && npm run build`
