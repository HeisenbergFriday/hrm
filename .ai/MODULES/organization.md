---
purpose: 组织模块兼容入口，当前以 org.md 为主
last_updated: 2026-05-26
source_of_truth:
  - .ai/MODULES/org.md
  - internal/api/router.go
  - internal/service/org_service.go
  - frontend/src/pages/Organization.tsx
update_when:
  - 修改组织模块路由时
  - 修改组织驾驶舱页面时
  - 修改组织统计口径时
---

# 组织模块

当前组织模块的主文档是 `.ai/MODULES/org.md`。本文件保留为兼容入口，避免历史引用继续指向旧的 `/api/v1/org/dashboard`、`/api/v1/org/structure-analysis`、`/api/v1/org/departments/overview` 等不存在接口。

## 当前后端入口

以 `internal/api/router.go` 为准，当前 `org` 分组实际注册：

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/v1/org/departments/tree` | 部门树 |
| `GET` | `/api/v1/org/departments/:id/history` | 部门变更历史 |
| `GET` | `/api/v1/org/overview` | 组织概览，支持 `department_id` |
| `GET` | `/api/v1/org/employees` | 员工列表 |
| `GET` | `/api/v1/org/employees/:id` | 聚合员工详情 |
| `POST` | `/api/v1/org/sync` | 同步组织数据 |

## 当前前端入口

| 路由 | 页面 | 说明 |
|---|---|---|
| `/organization` | `Organization.tsx` | 组织管理/驾驶舱入口 |
| `/department-tree` | `DepartmentTree.tsx` | 部门树与部门统计 |
| `/employees` | `EmployeeList.tsx` | 员工列表 |
| `/employees/:id` | `EmployeeDetail.tsx` | 聚合员工详情 |
| `/talent-analysis` | `TalentAnalysis.tsx` | 人才分析 |

## 维护说明

- 组织统计、员工详情聚合、部门树人数口径请优先更新 `.ai/MODULES/org.md`。
- 不要在新文档中继续引用 `/api/v1/org/dashboard`、`/api/v1/org/structure-analysis`、`/api/v1/org/departments/overview`，除非代码中重新注册这些路由。
