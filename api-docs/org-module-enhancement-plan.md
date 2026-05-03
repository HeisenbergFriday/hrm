# 组织模块增强方案

## 一、现状分析

### 已有能力（可复用）

#### 1. 数据模型层
- ✅ `User` - 员工基础信息（钉钉同步）
- ✅ `Department` - 部门信息（钉钉同步）
- ✅ `EmployeeProfile` - 员工档案（本地业务字段，包含入职、转正、合同等关键日期）
- ✅ `EmployeeTransfer` - 转岗记录
- ✅ `EmployeeResignation` - 离职记录
- ✅ `EmployeeOnboarding` - 入职记录
- ✅ `OperationLog` - 操作审计日志（已有，可用于时间轴）

#### 2. Service 层
- ✅ `OrgService` - 已实现核心能力：
  - `GetOverview()` - 团队概览统计（在职/离职人数、部门数、预警计数）
  - `GetDepartmentTree()` - 组织架构树（含人数统计）
  - `GetEmployeeAggregate()` - 员工聚合详情（档案+部门路径+汇报关系+时间轴+预警）
  - `GetEmployees()` - 员工列表（支持部门/搜索/状态筛选）
  - `ResolveScopeForUser()` - 权限范围控制
- ✅ `EmployeeService` - 档案 CRUD
- ✅ `AuditService` - 审计日志记录

#### 3. API 层
- ✅ `/api/v1/org/overview` - 组织概览
- ✅ `/api/v1/org/departments/tree` - 组织架构树
- ✅ `/api/v1/org/employees` - 员工列表
- ✅ `/api/v1/org/employees/:id` - 员工详情聚合
- ✅ `/api/v1/employee/profiles` - 档案管理
- ✅ `/api/v1/employee/transfers` - 转岗记录
- ✅ `/api/v1/employee/resignations` - 离职记录
- ✅ `/api/v1/employee/onboardings` - 入职记录

#### 4. 前端页面
- ✅ `EmployeeList.tsx` - 员工花名册（含概览看板、预警列表、趋势图、部门统计）
- ✅ `EmployeeDetail.tsx` - 员工详情（档案+组织关系+时间轴+预警）
- ✅ `DepartmentTree.tsx` - 组织架构树（含人数统计）
- ✅ `EmployeeProfile.tsx` - 档案管理页面
- ✅ `Organization.tsx` - 组织架构页面（简单版）

### 现有实现深度评估

#### ✅ 已达标功能
1. **团队概览与组织统计** - `EmployeeList.tsx` 已实现：
   - 在职/离职/部门数统计
   - 试用期到期/合同到期/待入职/待转岗/待离职预警计数
   - 近 6 个月入职/转岗/离职趋势图
   - 部门人数统计表

2. **员工档案聚合展示** - `EmployeeDetail.tsx` 已实现：
   - 员工基础信息 + 档案信息聚合
   - 部门路径展示
   - 汇报关系（上级+下属列表）
   - 个人预警列表

3. **入转调离留痕** - `EmployeeDetail.tsx` 已实现：
   - 时间轴展示（入职/转正/转岗/离职/档案变更）
   - 从 `OperationLog` 和人事流程表聚合

4. **组织架构展示** - `DepartmentTree.tsx` 已实现：
   - 树形展示部门层级
   - 每个节点显示总人数/直属人数/在职/离职统计

5. **权限范围控制** - `OrgService.ResolveScopeForUser()` 已实现：
   - 支持全组织/部门范围模式
   - 所有查询接口已集成权限过滤

#### ⚠️ 需增强功能

1. **组织变化趋势** - 部分实现，需补充：
   - ✅ 已有：近 6 个月入职/转岗/离职趋势（`EmployeeList.tsx`）
   - ❌ 缺失：部门人数变化趋势（按月）
   - ❌ 缺失：人员流动热力图（哪些部门流入/流出多）

2. **关键预警** - 部分实现，需补充：
   - ✅ 已有：试用期到期、合同到期预警
   - ❌ 缺失：连续离职预警（某部门 30 天内离职 >3 人）
   - ❌ 缺失：管理幅度预警（直属下级 >15 人）

3. **页面整合** - 需优化：
   - `Organization.tsx` 功能简单，与 `DepartmentTree.tsx` 重复
   - `EmployeeProfile.tsx` 独立存在，未与 `EmployeeList.tsx` 整合

---

## 二、实现方案（按优先级）

### P0：补充预警和趋势（复用现有接口，前端增强）

#### 1. 后端：增强 `OrgService.GetOverview()`
**文件**：`internal/service/org_service.go`

**新增预警类型**：
```go
// 在 buildWarnings() 中补充
- 连续离职预警：查询 employee_resignations 表，按 department_id 分组，统计最近 30 天离职数 >= 3
- 管理幅度预警：查询 users 表，按 department_id 分组，统计直属下级数 >= 15
```

**新增趋势维度**：
```go
// 在 buildTrends() 中补充
- 部门人数变化趋势：从 operation_logs 表聚合部门人数变化（入职/离职事件）
```

**预计工作量**：2-3 小时
- 修改 `buildWarnings()` 函数，新增 2 个预警查询
- 修改 `buildTrends()` 函数，新增部门人数趋势查询
- 补充单元测试

#### 2. 前端：增强 `EmployeeList.tsx` 预警和趋势展示
**文件**：`frontend/src/pages/EmployeeList.tsx`

**预警卡片增强**：
```tsx
// 在现有预警列表基础上，新增：
- 连续离职预警：红色标签，显示部门名称和离职人数
- 管理幅度预警：橙色标签，显示管理者姓名和下属数
```

**趋势图增强**：
```tsx
// 在现有趋势图基础上，新增：
- 部门人数变化趋势：折线图，展示 Top 5 部门的人数变化
```

**预计工作量**：2-3 小时

---

### P1：页面整合优化（提升用户体验）

#### 1. 合并 `Organization.tsx` 到 `DepartmentTree.tsx`
**目标**：`Organization.tsx` 功能简单且与 `DepartmentTree.tsx` 重复，合并后统一入口。

**方案**：
- 保留 `DepartmentTree.tsx` 作为主页面
- 删除 `Organization.tsx`
- 在 `DepartmentTree.tsx` 右侧面板增强：
  - 选中部门后，显示该部门员工列表（复用 `orgAPI.getEmployees()`）
  - 支持点击员工跳转到 `EmployeeDetail.tsx`

**预计工作量**：1-2 小时

#### 2. 整合 `EmployeeProfile.tsx` 到 `EmployeeList.tsx`
**目标**：档案管理不应独立页面，应作为员工列表的操作入口。

**方案**：
- 在 `EmployeeList.tsx` 表格中，"操作"列增加"编辑档案"按钮
- 点击后弹出 Modal，复用 `EmployeeProfile.tsx` 的表单逻辑
- 保留 `EmployeeProfile.tsx` 作为独立档案管理页面（供 HR 批量操作）

**预计工作量**：2 小时

---

### P2：补充人事流程时间轴（增强留痕能力）

#### 1. 后端：增强 `OrgService.buildTimeline()`
**文件**：`internal/service/org_service.go`

**当前问题**：
- 时间轴仅从 `OperationLog` 聚合，缺少人事流程表（`EmployeeTransfer`/`EmployeeResignation`/`EmployeeOnboarding`）的详细信息

**方案**：
```go
// 在 buildTimeline() 中补充查询
- 从 employee_transfers 表查询该员工的转岗记录，补充到时间轴
- 从 employee_resignations 表查询离职记录
- 从 employee_onboardings 表查询入职记录
- 按时间倒序排列
```

**预计工作量**：2-3 小时

#### 2. 前端：增强 `EmployeeDetail.tsx` 时间轴展示
**文件**：`frontend/src/pages/EmployeeDetail.tsx`

**方案**：
- 时间轴节点增加更多细节：
  - 转岗：显示"从 XX 部门 → YY 部门"
  - 离职：显示离职类型和原因
  - 入职：显示入职渠道和岗位

**预计工作量**：1 小时

---

### P3：补充组织调整追溯（可选，视业务需求）

#### 背景
当前系统未记录部门本身的变更历史（如部门改名、合并、拆分）。如果需要追溯组织调整过程，需要新增功能。

#### 方案（如需要）
1. 新增 `DepartmentChangeLog` 表：
   ```go
   type DepartmentChangeLog struct {
       ID              uint
       DepartmentID    string
       ChangeType      string // rename, merge, split, create, delete
       OldValue        map[string]interface{} // JSON
       NewValue        map[string]interface{} // JSON
       OperatorID      string
       OperatorName    string
       ChangeDate      time.Time
   }
   ```

2. 在 `DepartmentService` 中，每次部门变更时记录日志

3. 在 `DepartmentTree.tsx` 中，选中部门后显示变更历史

**预计工作量**：4-6 小时（需新增表和逻辑）

**建议**：暂不实现，等业务明确需求后再补充。

---

## 三、测试补充计划

### 1. 后端单元测试
**文件**：`internal/service/org_service_test.go`

**补充测试用例**：
- `TestBuildWarnings_ContinuousResignation` - 连续离职预警
- `TestBuildWarnings_ManagementSpan` - 管理幅度预警
- `TestBuildTrends_DepartmentHeadcount` - 部门人数趋势
- `TestBuildTimeline_WithTransfers` - 时间轴包含转岗记录

**预计工作量**：2-3 小时

### 2. 前端集成测试
**方案**：
- 手动测试各页面功能
- 验证预警和趋势数据正确性
- 验证权限范围控制生效

**预计工作量**：2 小时

---

## 四、实施优先级和工作量估算

| 优先级 | 任务 | 工作量 | 依赖 |
|--------|------|--------|------|
| P0 | 后端：增强预警和趋势 | 2-3h | 无 |
| P0 | 前端：增强预警和趋势展示 | 2-3h | 后端完成 |
| P1 | 合并 Organization.tsx 到 DepartmentTree.tsx | 1-2h | 无 |
| P1 | 整合 EmployeeProfile.tsx 到 EmployeeList.tsx | 2h | 无 |
| P2 | 后端：增强时间轴 | 2-3h | 无 |
| P2 | 前端：增强时间轴展示 | 1h | 后端完成 |
| P3 | 组织调整追溯（可选） | 4-6h | 业务需求明确 |
| 测试 | 后端单元测试 | 2-3h | 对应功能完成 |
| 测试 | 前端集成测试 | 2h | 所有功能完成 |

**总工作量**：18-25 小时（不含 P3）

---

## 五、关键设计决策

### 1. 不新增表，优先复用现有数据
- ✅ `EmployeeProfile` 已包含入职、转正、合同等关键日期
- ✅ `EmployeeTransfer`/`EmployeeResignation`/`EmployeeOnboarding` 已记录人事流程
- ✅ `OperationLog` 已记录操作审计
- ❌ 不新增 `DepartmentChangeLog` 表（除非业务明确需要）

### 2. 页面增强而非新建
- ✅ `EmployeeList.tsx` 已是花名册主页面，增强预警和趋势即可
- ✅ `EmployeeDetail.tsx` 已是员工详情聚合页面，增强时间轴即可
- ✅ `DepartmentTree.tsx` 已是组织架构主页面，合并 `Organization.tsx` 功能
- ❌ 不新建平行页面

### 3. 权限范围控制已完备
- ✅ `OrgService.ResolveScopeForUser()` 已实现
- ✅ 所有查询接口已集成权限过滤
- ✅ 前端页面已显示当前数据范围
- ❌ 无需额外开发

---

## 六、实施建议

### 阶段 1：快速见效（P0，4-6 小时）
1. 增强预警和趋势（后端 + 前端）
2. 验证效果，收集用户反馈

### 阶段 2：体验优化（P1，3-4 小时）
1. 合并重复页面
2. 整合档案管理入口

### 阶段 3：深度留痕（P2，3-4 小时）
1. 增强时间轴
2. 补充单元测试

### 阶段 4：按需扩展（P3，视业务需求）
1. 如需组织调整追溯，再补充 `DepartmentChangeLog` 表

---

## 七、风险和注意事项

### 1. 数据质量依赖
- 预警和趋势依赖 `EmployeeProfile` 和人事流程表的数据完整性
- 建议先检查现有数据质量，必要时补充数据清洗逻辑

### 2. 性能考虑
- 部门人数趋势查询可能涉及大量历史数据
- 建议增加缓存或定时预计算

### 3. 权限测试
- 确保所有新增查询都经过权限范围过滤
- 重点测试部门范围模式下的数据隔离

---

## 八、总结

### 现有能力评估
- ✅ 数据模型完备（User, Department, EmployeeProfile, 人事流程表）
- ✅ Service 层核心能力已实现（概览、架构树、员工聚合、权限控制）
- ✅ 前端页面已覆盖主要场景（花名册、详情、架构树）

### 需补充的工作
- ⚠️ 预警和趋势维度不足（连续离职、管理幅度、部门人数趋势）
- ⚠️ 页面存在重复和割裂（Organization.tsx, EmployeeProfile.tsx）
- ⚠️ 时间轴留痕不够详细（缺少人事流程表的详细信息）

### 实施路径
1. **优先增强预警和趋势**（P0，快速见效）
2. **优化页面整合**（P1，提升体验）
3. **深化时间轴留痕**（P2，完善功能）
4. **按需扩展组织调整追溯**（P3，视业务需求）

### 预期效果
完成 P0-P2 后，组织模块将达到类似 Moka 的使用效果：
- ✅ 员工花名册清晰可查（EmployeeList.tsx）
- ✅ 员工信息聚合展示（EmployeeDetail.tsx）
- ✅ 入转调离全生命周期有时间轴和留痕（EmployeeDetail.tsx）
- ✅ 组织架构可视化、汇报关系清晰（DepartmentTree.tsx）
- ✅ 管理者有团队概览、预警和结构分析能力（EmployeeList.tsx）
- ⚠️ 组织调整过程可追溯（P3，可选）
