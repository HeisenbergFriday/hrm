# PeopleOps 前端设计

本文按当前前端代码维护。入口文件是 `frontend/src/main.tsx` 和 `frontend/src/App.tsx`。

## 技术栈

- React 18
- Vite 4
- TypeScript 5
- Ant Design 5
- React Query 5
- Zustand 4
- Axios
- React Router 6

## 应用入口

| 文件 | 说明 |
|---|---|
| `frontend/src/main.tsx` | 注册 `QueryClientProvider` 与 `BrowserRouter` |
| `frontend/src/App.tsx` | 主布局、侧边菜单、页面路由、钉钉内免登流程 |
| `frontend/src/services/api.ts` | Axios 实例与 API 封装，`baseURL=/api/v1` |
| `frontend/src/store/authStore.ts` | 登录态持久化，localStorage key 为 `peopleops-auth` |

## 布局结构

- 左侧固定 `Sider` 菜单。
- 顶部 `Header` 展示当前用户。
- 主内容区使用 React Router 渲染页面。
- 认证页 `/login`、`/callback`、`/login-error` 不进入主布局。
- 未登录访问业务页时展示登录页；钉钉环境下会优先尝试内免登。

## 当前页面路由

| 路由 | 页面 | 说明 |
|---|---|---|
| `/login` | `Login.tsx` | 登录 |
| `/callback` | `Callback.tsx` | 钉钉 OAuth 回调 |
| `/login-error` | `LoginError.tsx` | 登录错误 |
| `/` | `Home.tsx` | 首页 |
| `/organization` | `Organization.tsx` | 组织管理 |
| `/department-tree` | `DepartmentTree.tsx` | 部门树 |
| `/employees` | `EmployeeList.tsx` | 员工列表 |
| `/employees/:id` | `EmployeeDetail.tsx` | 员工详情 |
| `/employee-profile` | `EmployeeProfile.tsx` | 员工档案 |
| `/employee-flow` | `EmployeeFlow.tsx` | 入转调离 |
| `/talent-analysis` | `TalentAnalysis.tsx` | 人才分析 |
| `/attendance` | `Attendance.tsx` | 考勤查询 |
| `/attendance-stats` | `AttendanceStats.tsx` | 考勤统计 |
| `/attendance-export` | `AttendanceExport.tsx` | 考勤导出 |
| `/week-schedule` | `WeekSchedule.tsx` | 大小周与节假日 |
| `/employee-shift-config` | `EmployeeShiftConfig.tsx` | 员工下班时间 |
| `/approval-templates` | `ApprovalTemplate.tsx` | 审批模板 |
| `/approval-instances` | `ApprovalInstance.tsx` | 审批实例 |
| `/approval-detail/:id` | `ApprovalDetail.tsx` | 审批详情 |
| `/approval-stats` | `ApprovalStats.tsx` | 审批统计 |
| `/role-management` | `RoleManagement.tsx` | 角色管理 |
| `/menu-permission` | `MenuPermission.tsx` | 菜单权限 |
| `/data-permission` | `DataPermission.tsx` | 数据权限 |
| `/sync-jobs` | `SyncJobs.tsx` | 同步任务 |
| `/sync-log` | `SyncLog.tsx` | 同步日志 |
| `/audit-logs` | `AuditLogs.tsx` | 操作日志 |
| `/leave-overtime` | `LeaveOvertime.tsx` | 年假与调休 |
| `/performance-overview` | `PerformanceOverview.tsx` | 绩效活动 |
| `/performance-indicator-library` | `PerformanceIndicatorLibrary.tsx` | 指标库 |
| `/performance-result/:activityId/:participantId` | `PerformanceResultView.tsx` | 绩效结果 |
| `/performance-self-eval/:activityId/:participantId` | `PerformanceSelfEval.tsx` | 员工自评 |
| `/performance-manager-eval/:activityId/:participantId` | `PerformanceManagerEval.tsx` | 上级评分 |
| `/performance-goal-setting/:activityId/:participantId` | `PerformanceGoalSetting.tsx` | 目标设定 |
| `/permission` | `Permission.tsx` | 权限管理 |
| `/log` | `Log.tsx` | 日志查询 |
| `/setting` | `Setting.tsx` | 系统设置 |

## 状态管理

当前全局持久状态主要是认证状态：

```text
frontend/src/store/authStore.ts
```

包含：

- `user`
- `token`
- `isLoggedIn`
- `login(user, token)`
- `logout()`

其他业务数据主要通过 React Query 页面级缓存管理。

## API 调用

统一从 `frontend/src/services/api.ts` 进入：

- Axios `baseURL=/api/v1`
- timeout 为 10 秒
- 请求拦截器自动注入 `Authorization: Bearer <token>`
- 响应拦截器返回 `response.data`
- 401 时自动登出并跳转 `/login`

## 本地开发

```bash
cd frontend
npm install
npm run dev
```

默认端口 `3000`。`frontend/vite.config.ts` 将 `/api` 代理到 `http://localhost:8080`。

## 构建与验证

```bash
cd frontend
npm run build
npm run lint
npm run test
npm run e2e
```

说明：当前没有单独的 `type-check` script，`npm run build` 会执行 `tsc && vite build`。`npm run test` 使用 `vite.config.test.ts`，在没有前端单测文件时会通过空用例检查，后续新增 `src/**/*.{test,spec}.{ts,tsx}` 会自动纳入。

## 性能设计

- 页面组件使用 `React.lazy` 和 `Suspense` 懒加载。
- React Query 负责请求缓存与刷新。
- Vite build 中按 React、Ant Design、其他依赖拆分 vendor chunk。
- 大列表页面优先使用分页，避免一次性渲染过多数据。

## 设计注意事项

- 页面 UI 以 Ant Design 组件为主。
- 路由、菜单和页面文件变更时同步更新 `.ai/PROJECT_MAP.md`。
- 新增 API 时同步更新 `frontend/src/services/api.ts` 和后端接口文档。
