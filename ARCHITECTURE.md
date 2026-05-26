# PeopleOps 架构设计

PeopleOps 是一个以钉钉为主数据来源的人事后台系统。系统同步组织、员工、考勤、审批等基础数据，并在本地扩展员工档案、年假调休、大小周排班、绩效管理、权限与审计能力。

## 技术栈

### 后端

- Go 1.20
- Gin
- GORM
- MySQL
- Redis 可选缓存
- JWT
- Logrus
- 钉钉开放平台 API

### 前端

- React 18
- TypeScript
- Vite 4
- Ant Design 5
- React Query
- Zustand
- Axios

## 代码分层

```text
cmd/main.go
  -> internal/api/router.go
      -> Handler: internal/api/*.go
          -> Service: internal/service/*.go
              -> Repository: internal/repository/*.go
                  -> Model: internal/database/*.go
```

- `api`：Gin 路由与 HTTP handler。
- `service`：业务逻辑、钉钉同步编排、规则计算。
- `repository`：GORM 数据访问封装。
- `database`：数据库初始化、迁移和 GORM 模型。
- `dingtalk`：钉钉开放平台 API 封装。
- `middleware`：JWT 鉴权。
- `cache`：Redis 初始化与基础封装。

## 前端结构

```text
frontend/src/App.tsx             # 主布局、菜单、路由、钉钉内免登
frontend/src/main.tsx            # React Query Provider 与 BrowserRouter
frontend/src/services/api.ts     # Axios API 封装，baseURL=/api/v1
frontend/src/store/authStore.ts  # Zustand 登录态，localStorage key=peopleops-auth
frontend/src/pages/              # 页面组件
frontend/src/components/         # 公共组件
```

前端本地开发端口是 `3000`，Vite 代理 `/api` 到 `http://localhost:8080`。生产或钉钉微应用部署时，推荐先构建 `frontend/dist`，再由 Go 服务统一托管。

## 数据流

```text
钉钉开放平台
    ↓ 同步或回写
后端服务
    ↓ GORM
MySQL
    ↓ API
前端页面
```

Redis 用于缓存和加速部分能力，连接失败不会阻止服务启动。

## 核心模块

| 模块 | 说明 |
|---|---|
| 认证 | 账号密码登录、钉钉扫码、钉钉内免登、JWT |
| 组织与员工 | 部门树、组织概览、员工列表、聚合员工详情、组织同步 |
| 员工档案 | 档案、入职、调岗、离职、人才分析 |
| 考勤 | 打卡同步、记录查询、异常统计、导出 |
| 审批 | 模板、实例、详情、审批同步 |
| 权限与审计 | 角色、权限、操作日志 |
| 排班 | 大小周规则、法定节假日、钉钉班次、员工下班时间 |
| 年假与调休 | 年假资格、季度发放、消费台账、加班匹配、调休余额 |
| 绩效 | 活动、参与人、目标设定、自评、上级评分、三级确认、归档 |

## 后端运行时

1. `cmd/main.go` 加载 `.env`。
2. 初始化 MySQL；失败时尝试创建数据库后重连。
3. 执行 GORM `AutoMigrate` 和少量手写兼容迁移。
4. 初始化 Redis；失败时继续运行。
5. 初始化钉钉客户端；失败时继续运行。
6. 注册 Gin 路由。
7. 启动年假/调休定时任务。
8. 监听 `PORT`，默认 `8080`。

## API 约定

- 健康检查：`GET /health`
- 业务接口前缀：`/api/v1`
- 前端 API 封装：`frontend/src/services/api.ts`
- 路由真实来源：`internal/api/router.go`
- `api-docs/swagger.json` 当前只覆盖早期基础接口，不是完整 API 文档。

## 认证与状态

- JWT 使用 `Authorization: Bearer <token>`。
- JWT Claims 包含 `user_id` 和 `user_name`。
- 前端登录态通过 Zustand 持久化到 localStorage，key 为 `peopleops-auth`。
- 401 响应会触发前端登出并跳转到 `/login`。

## 数据库约定

- 主业务库为 MySQL。
- GORM 模型主键当前使用 `uint` 自增。
- JSON 扩展字段使用 MySQL JSON 类型。
- 主要业务表使用软删除。
- 迁移禁用自动外键约束，业务关联主要靠代码维护。

## 部署约定

- 本地开发：后端 `8080`，前端 `3000`。
- 钉钉微应用或生产联调：构建前端后由 Go 服务托管 `frontend/dist`。
- 钉钉首页建议配置为 `http://your-host:8080/`。
- 钉钉 OAuth 回调建议配置为 `http://your-host:8080/callback`。

## 维护原则

- 新增路由时同步更新 `BACKEND_API_DESIGN.md`、`.ai/PROJECT_MAP.md` 和对应模块文档。
- 修改模型时同步更新 `DATABASE_DESIGN.md` 和 `.ai/PROJECT_MAP.md`。
- 修改启动、部署、环境变量时同步更新 `README.md`、`DEPLOYMENT.md`、`ENVIRONMENT.md`、`.ai/COMMANDS.md`。
- 不再把 PostgreSQL、UUID 主键、`backend/` 目录写成当前事实，除非代码也完成对应迁移。
