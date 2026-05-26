# People Ops 后台系统

企业内部人事后台系统，集成钉钉组织、员工、考勤、审批等数据，并在本地扩展员工档案、年假调休、大小周排班、绩效管理、权限与审计能力。

## 系统定位

- 钉钉是组织、部门、员工、考勤、审批等主数据来源。
- 本地 MySQL 保存同步缓存和本系统扩展业务数据。
- 后端可托管 `frontend/dist`，用于钉钉微应用或统一端口部署。
- 当前支持账号密码登录、钉钉扫码登录和钉钉内免登。

## 技术栈

### 后端

- Go 1.20
- Gin
- GORM
- MySQL
- Redis 可选缓存
- JWT
- 钉钉开放平台 API

### 前端

- React 18
- TypeScript
- Vite 4
- Ant Design 5
- React Query
- Zustand
- Axios

## 目录结构

```text
peopleops/
├─ cmd/main.go                 # 后端入口
├─ internal/                    # 后端内部代码
│  ├─ api/                      # 路由与 handlers
│  ├─ cache/                    # Redis 初始化
│  ├─ config/                   # 配置加载与 holidays.json
│  ├─ database/                 # GORM 初始化、迁移、模型
│  ├─ dingtalk/                 # 钉钉客户端封装
│  ├─ middleware/               # JWT 中间件
│  ├─ repository/               # 数据访问层
│  └─ service/                  # 业务服务层
├─ frontend/                    # Vite + React 前端
├─ tests/                       # 测试辅助与验收报告
├─ tools/                       # 运维、修复、初始化脚本
├─ scripts/                     # 本地脚本
├─ api-docs/                    # 设计文档与历史接口文档
├─ .ai/                         # AI 协作文档
└─ .env                         # 本地环境变量，不提交敏感值
```

## 快速开始

### 1. 环境准备

- Go 1.20+
- Node.js 18+ 推荐
- MySQL 5.7+ 或 8.x
- Redis 可选，未连接时服务仍可启动但缓存相关能力不可用
- 钉钉开发者账号

### 2. 后端配置

在项目根目录创建或编辑 `.env`：

```env
PORT=8080
DATABASE_URL=root:password@tcp(localhost:3306)/peopleops?charset=utf8mb4&parseTime=True&loc=Local
REDIS_URL=localhost:6379
REDIS_PASSWORD=

DINGTALK_APP_KEY=your_app_key
DINGTALK_APP_SECRET=your_app_secret
DINGTALK_CORP_ID=dingxxxxxxxx
DINGTALK_AGENT_ID=123456

JWT_SECRET=your_jwt_secret
```

### 3. 启动后端

```bash
go mod download
go run ./cmd/main.go
```

服务默认监听 `http://localhost:8080`。健康检查：

```bash
curl http://localhost:8080/health
```

### 4. 启动前端开发服务器

```bash
cd frontend
npm install
npm run dev
```

前端默认监听 `http://localhost:3000`，开发代理会把 `/api` 转发到 `http://localhost:8080`。

### 5. 构建并由后端托管前端

```bash
cd frontend
npm run build
cd ..
go run ./cmd/main.go
```

构建后可从 `http://localhost:8080/` 访问前端页面。

## 默认账号

首次启动并成功连接数据库后会创建默认管理员：

- 用户名：`admin`
- 密码：`admin123`

## API 入口

所有业务接口统一使用 `/api/v1` 前缀。路由真实来源是 `internal/api/router.go`，前端封装入口是 `frontend/src/services/api.ts`。

| 模块 | 当前主要接口 |
|---|---|
| 认证 | `POST /api/v1/auth/login`、`POST /api/v1/auth/logout`、`GET /api/v1/auth/me` |
| 钉钉登录 | `GET /api/v1/auth/dingtalk/qr/start`、`POST /api/v1/auth/dingtalk/in-app`、`GET /api/v1/auth/dingtalk/callback`、`GET /api/v1/auth/dingtalk/config` |
| 用户 | `GET /api/v1/users`、`GET /api/v1/users/:id`、`PUT /api/v1/users/:id` |
| 部门 | `GET /api/v1/departments`、`GET /api/v1/departments/:id` |
| 同步 | `POST /api/v1/sync/departments`、`POST /api/v1/sync/users`、`GET /api/v1/sync/status` |
| 组织 | `GET /api/v1/org/overview`、`GET /api/v1/org/departments/tree`、`GET /api/v1/org/employees` |
| 考勤 | `GET /api/v1/attendance/records`、`GET /api/v1/attendance/stats`、`POST /api/v1/attendance/sync`、`POST /api/v1/attendance/export` |
| 审批 | `GET /api/v1/approvals/templates`、`GET /api/v1/approvals/instances`、`GET /api/v1/approvals/:id`、`POST /api/v1/approvals/sync` |
| 权限 | `GET /api/v1/permission/roles`、`POST /api/v1/permission/roles`、`GET /api/v1/permission/permissions` |
| 审计 | `GET /api/v1/audit/logs` |
| 年假调休 | `/api/v1/leave/*`、`/api/v1/overtime/*`、`/api/v1/comp-time/*` |
| 排班 | `/api/v1/week-schedule/*`、`/api/v1/shift-config/*` |
| 绩效 | `/api/v1/performance/*` |

`api-docs/swagger.json` 目前只覆盖早期基础接口，不是完整 API 清单。

## 常用命令

```bash
# 后端
go run ./cmd/main.go
go test ./...
go fmt ./...
go vet ./...

# 前端
cd frontend
npm run dev
npm run build
npm run lint
npm run e2e
```

## 注意事项

- `DATABASE_URL` 使用 MySQL DSN，不是 PostgreSQL URL。
- `REDIS_URL` 当前传给 go-redis 的 `Addr`，格式使用 `localhost:6379`，不要写成 `redis://localhost:6379`。
- 钉钉微应用部署建议使用后端统一托管的 `8080` 端口页面，不要把生产回调指向 Vite 开发端口 `3000`。
- 修改路由、模型、启动方式后，请同步更新 `.ai/PROJECT_MAP.md`、`.ai/ARCHITECTURE.md` 和对应模块文档。
