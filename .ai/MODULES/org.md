---
purpose: 组织与员工模块业务规则说明
last_updated: 2026-04-30
source_of_truth:
  - internal/api/handlers.go（组织相关 handler）
  - internal/service/user_service.go（用户服务）
  - internal/service/org_service.go（组织服务）
  - internal/database/models.go（Department、User、EmployeeProfile 模型）
  - frontend/src/pages/DepartmentTree.tsx（部门树）
  - frontend/src/pages/EmployeeList.tsx（员工列表）
  - frontend/src/pages/EmployeeDetail.tsx（员工详情）
update_when:
  - 修改组织同步逻辑时
  - 修改员工查询逻辑时
  - 修改员工详情聚合结构时
  - 修改部门树展示逻辑时
---

# 组织架构模块

## 模块定位

管理部门树、员工列表、聚合员工详情，并从钉钉同步组织架构数据。

本次阶段 1A 在组织模块侧只沉淀员工详情聚合与档案字段补齐相关长期知识，不涉及组织分析、绩效、权限、强制分布、C/D 面谈等能力变更。

阶段 2A 已补充组织概览的最小统计能力，仅覆盖基础统计卡片，不扩展到趋势分析、复杂图表、绩效、假勤、权限、强制分布、C/D 面谈等内容。

### 阶段 2A：组织概览最小能力

`GET /api/v1/org/overview` 当前沉淀的最小统计项：
- 在职人数
- 试用期人数
- 计划转正预警数量
- 员工类型分布
- 职级分布
- 岗位序列分布

当前统计口径：
- 三组分布按在职员工统计。
- 试用期人数按“在职且未填写实际转正日期，并且存在计划转正日期或试用期结束日期”的员工统计。
- 计划转正预警数量按当前代码实际逻辑统计：`buildEmployeeWarnings()` 会先以 `planned_regular_date` 优先、`probation_end_date` 兜底生成 `probation_due` 预警；仅当员工在职、未填写 `actual_regular_date`、该日期可解析、且落在“今天到未来 30 天”窗口内时，`buildOverviewSummary()` 才会累计到 `planned_regularization_count`。当前该数量与 `probation_due_count` 使用同一触发条件。
- 前端当前只做统计卡片展示，不做趋势、复杂图表。

---

## 数据模型

### Department
部门模型

```go
type Department struct {
    ID           uint
    DepartmentID string  // 钉钉部门 ID（唯一键）
    Name         string
    ParentID     string  // 父部门钉钉 ID
    Order        int
    Extension    map[string]interface{}
    CreatedAt    time.Time
    UpdatedAt    time.Time
    DeletedAt    gorm.DeletedAt
}
```

### User
用户模型（与认证模块共用）

```go
type User struct {
    ID           uint
    UserID       string  // 钉钉用户 ID（唯一键）
    Name         string
    Email        string
    Mobile       string
    DepartmentID string  // 所属部门钉钉 ID
    Position     string
    Avatar       string
    Status       string  // active / inactive
    Extension    map[string]interface{}
    CreatedAt    time.Time
    UpdatedAt    time.Time
    DeletedAt    gorm.DeletedAt
}
```

### EmployeeAggregate
员工详情聚合视图（`GET /api/v1/org/employees/:id` 返回结构）

- `employee`：组织侧基础员工信息（`User`）
- `profile`：员工档案快照（`EmployeeProfile`，可为空）
- `scope`：当前登录人可见组织范围
- `department`：当前部门与组织路径
- `org_relation`：直属上级、直属下属、同部门人数
- `timeline`：入职、计划转正、实际转正、合同到期、调岗、离职、档案审计日志时间轴
- `warnings`：该员工关联的组织预警

---

## API 接口

### GET /api/v1/org/departments/tree
获取部门树

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": [
        {
            "id": 1,
            "department_id": "1",
            "name": "公司",
            "parent_id": "0",
            "children": [
                {
                    "id": 2,
                    "department_id": "2",
                    "name": "技术部",
                    "parent_id": "1",
                    "children": []
                }
            ]
        }
    ]
}
```

### GET /api/v1/org/overview
获取组织概览

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "overview": {
            "scope": {
                "mode": "all",
                "department_names": []
            },
            "summary": {
                "active_employees": 95,
                "probation_employee_count": 8,
                "planned_regularization_count": 3
            },
            "employee_type_distribution": [
                { "key": "正式", "label": "正式", "count": 80 }
            ],
            "job_level_distribution": [
                { "key": "P5", "label": "P5", "count": 20 }
            ],
            "job_family_distribution": [
                { "key": "技术", "label": "技术", "count": 40 }
            ]
        }
    }
}
```

### GET /api/v1/org/employees
获取员工列表

Query 参数：
- `department_id`：部门 ID（可选）
- `status`：状态（可选，active/inactive）
- `keyword`：搜索关键词（可选，搜索姓名/手机/邮箱）
- `page`：页码（默认 1）
- `page_size`：每页数量（默认 20）

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "total": 100,
        "page": 1,
        "page_size": 20,
        "items": [
            {
                "id": 1,
                "user_id": "xxx",
                "name": "张三",
                "email": "zhangsan@example.com",
                "mobile": "13800138000",
                "department_id": "2",
                "position": "工程师",
                "avatar": "https://...",
                "status": "active"
            }
        ]
    }
}
```

### GET /api/v1/org/employees/:id
获取员工详情

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "detail": {
            "employee": {
                "id": 1,
                "user_id": "xxx",
                "name": "张三",
                "email": "zhangsan@example.com",
                "mobile": "13800138000",
                "department_id": "2",
                "position": "工程师",
                "avatar": "https://...",
                "status": "active"
            },
            "profile": {
                "employee_id": "EMP001",
                "profile_status": "active",
                "employment_type": "正式",
                "education": "本科",
                "job_level": "P5",
                "job_family": "技术",
                "entry_date": "2024-01-15",
                "planned_regular_date": "2024-04-15",
                "actual_regular_date": "2024-04-20"
            },
            "scope": {
                "mode": "department",
                "department_names": ["技术部"]
            },
            "department": {
                "id": "2",
                "name": "技术部",
                "path": [
                    { "id": "1", "name": "公司" },
                    { "id": "2", "name": "技术部" }
                ]
            },
            "org_relation": {
                "manager": {
                    "user_id": "leader-1",
                    "name": "李主管",
                    "department_name": "技术部",
                    "position": "技术经理"
                },
                "direct_reports": [],
                "same_department_count": 6
            },
            "timeline": [
                {
                    "type": "regularization_plan",
                    "title": "计划转正",
                    "date": "2024-04-15",
                    "status": "planned"
                },
                {
                    "type": "audit",
                    "title": "更新员工档案",
                    "date": "2026-04-30",
                    "operator_name": "管理员"
                }
            ],
            "warnings": []
        }
    }
}
```

### POST /api/v1/org/sync
同步组织架构

Body：
```json
{
    "sync_departments": true,
    "sync_users": true
}
```

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "departments_synced": 10,
        "users_synced": 100
    }
}
```

---

## 核心业务流程

### 同步组织架构流程

1. **同步部门**（`SyncDepartments`）
   - 调用钉钉 API 获取部门列表
   - 递归获取子部门
   - 写入 `departments` 表（upsert）

2. **同步用户**（`SyncUsers`）
   - 调用钉钉 API 获取用户列表
   - 按部门分页获取
   - 写入 `users` 表（upsert）
   - 如果用户不存在 `EmployeeProfile`，自动创建

### 员工详情聚合流程

1. **校验组织范围**
   - 根据当前登录人解析 `scope`
   - 非全组织账号只能访问授权部门及其下级部门员工

2. **聚合员工主数据**
   - 读取 `User`
   - 按 `user_id` 关联 `EmployeeProfile`
   - 计算部门路径与汇报关系

3. **生成详情扩展信息**
   - 时间轴聚合入职、计划转正、实际转正、合同到期、调岗、离职、档案审计日志
   - 预警信息按员工档案日期与组织规则生成

---

## 关键 Service

| Service | 文件 | 说明 |
|---|---|---|
| `OrgService` | `org_service.go` | 组织架构管理 |
| `UserService` | `user_service.go` | 用户管理 |

---

## 前端页面

### 部门树页面
`frontend/src/pages/DepartmentTree.tsx`

功能：
- 展示部门树
- 点击部门查看员工列表

### 员工列表页面
`frontend/src/pages/EmployeeList.tsx`

功能：
- 组织概览统计卡片：在职人数、试用期人数、计划转正预警
- 组织概览分布卡片：员工类型、职级、岗位序列
- 当前组织概览仅做卡片展示，不做趋势图和复杂图表
- 员工列表（支持分页、搜索、筛选）
- 点击员工查看详情

### 员工详情页面
`frontend/src/pages/EmployeeDetail.tsx`

功能：
- 员工基本信息
- 员工档案聚合信息
- 组织路径、汇报关系、时间轴、预警
- 直接编辑档案字段；无档案时走 `employeeAPI.createProfile`，有档案时走 `employeeAPI.updateProfile`
- 重点展示并维护 `employment_type`、`education`、`job_level`、`job_family`、`planned_regular_date`、`actual_regular_date`

---

## 钉钉 API

### 获取部门列表
```
GET /topapi/v2/department/listsub
```

参数：
- `dept_id`：父部门 ID（根部门为 1）

### 获取部门详情
```
GET /topapi/v2/department/get
```

参数：
- `dept_id`：部门 ID

### 获取部门用户列表
```
GET /topapi/v2/user/list
```

参数：
- `dept_id`：部门 ID
- `cursor`：分页游标
- `size`：每页数量

### 获取用户详情
```
GET /topapi/v2/user/get
```

参数：
- `userid`：用户 ID

---

## 环境变量

- `DINGTALK_APP_KEY`：钉钉应用 Key
- `DINGTALK_APP_SECRET`：钉钉应用 Secret
- `DINGTALK_CORP_ID`：钉钉企业 ID

---

## 常见问题

### 同步失败
- 检查钉钉应用权限（需要"通讯录读权限"）
- 检查 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`、`DINGTALK_CORP_ID`
- 查看日志：`logrus` 会输出详细错误信息

### 部门树不完整
- 检查钉钉部门结构是否正确
- 检查 `parent_id` 是否正确
- 重新同步部门

### 员工列表为空
- 检查是否已同步用户
- 检查 `department_id` 是否正确
- 检查用户 `status` 是否为 `active`

### 员工详情显示不全
- 检查 `User` 模型字段是否完整
- 检查钉钉用户信息是否完整
- 检查 `EmployeeProfile` 是否存在
- 检查 `planned_regular_date`、`actual_regular_date`、`job_level`、`job_family` 等档案字段是否已录入
- 检查时间轴与审计日志资源 `employee_profile:{user_id}` 是否存在数据
