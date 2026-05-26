# PeopleOps 联调与部署说明

本文描述当前仓库的实际启动方式。后端代码在项目根目录下，没有 `backend/` 子目录。

## 环境准备

### 后端

- Go 1.20+
- MySQL 5.7+ 或 8.x
- Redis 7+ 可选

### 前端

- Node.js 18+ 推荐
- npm 8+

## 本地配置

在项目根目录编辑 `.env`：

```env
PORT=8080
DATABASE_URL=root:password@tcp(localhost:3306)/peopleops?charset=utf8mb4&parseTime=True&loc=Local
REDIS_URL=localhost:6379
REDIS_PASSWORD=

DINGTALK_APP_KEY=your_app_key
DINGTALK_APP_SECRET=your_app_secret
DINGTALK_CORP_ID=dingxxxxxxxx
DINGTALK_AGENT_ID=123456

JWT_SECRET=change_me
```

注意：

- `DATABASE_URL` 是 MySQL DSN，不是 PostgreSQL URL。
- `REDIS_URL` 使用 `host:port` 格式，例如 `localhost:6379`。
- Redis 或钉钉客户端初始化失败时，后端仍会继续运行，相关能力会受影响。

## 启动后端

在项目根目录执行：

```bash
go mod download
go run ./cmd/main.go
```

健康检查：

```bash
curl http://localhost:8080/health
```

也可以使用脚本：

```bash
scripts/run.bat
```

## 启动前端开发服务器

```bash
cd frontend
npm install
npm run dev
```

默认端口是 `3000`。`frontend/vite.config.ts` 会把 `/api` 代理到 `http://localhost:8080`。

## 后端托管前端

需要钉钉微应用或统一端口访问时，先构建前端：

```bash
cd frontend
npm run build
cd ..
go run ./cmd/main.go
```

构建产物在 `frontend/dist`。后端启动后可访问：

```text
http://localhost:8080/
```

如果没有构建前端，访问非 API 路由时后端会提示 `frontend build not found`。

## 当前 API 入口

所有业务接口统一使用 `/api/v1` 前缀。

| 模块 | 接口示例 |
|---|---|
| 认证 | `POST /api/v1/auth/login`、`POST /api/v1/auth/logout`、`GET /api/v1/auth/me` |
| 钉钉登录 | `GET /api/v1/auth/dingtalk/qr/start`、`POST /api/v1/auth/dingtalk/in-app`、`GET /api/v1/auth/dingtalk/callback`、`GET /api/v1/auth/dingtalk/config` |
| 用户/部门 | `GET /api/v1/users`、`GET /api/v1/departments` |
| 同步 | `POST /api/v1/sync/departments`、`POST /api/v1/sync/users`、`GET /api/v1/sync/status` |
| 组织 | `GET /api/v1/org/overview`、`GET /api/v1/org/departments/tree`、`GET /api/v1/org/employees` |
| 考勤 | `GET /api/v1/attendance/records`、`GET /api/v1/attendance/stats`、`POST /api/v1/attendance/sync` |
| 审批 | `GET /api/v1/approvals/templates`、`GET /api/v1/approvals/instances`、`POST /api/v1/approvals/sync` |
| 权限与审计 | `GET /api/v1/permission/roles`、`GET /api/v1/permission/permissions`、`GET /api/v1/audit/logs` |
| 业务扩展 | `/api/v1/employee/*`、`/api/v1/talent/*`、`/api/v1/leave/*`、`/api/v1/overtime/*`、`/api/v1/week-schedule/*`、`/api/v1/performance/*` |

完整路由以 `internal/api/router.go` 为准。

## 默认账号

数据库初始化成功后，如果不存在管理员，会创建：

- 用户名：`admin`
- 密码：`admin123`

## 钉钉部署补充

如果使用钉钉扫码或钉钉内免登：

1. 执行 `cd frontend && npm run build`。
2. 用 Go 服务统一托管页面，例如 `http://your-host:8080/`。
3. 钉钉微应用首页配置为 `http://your-host:8080/`。
4. 钉钉 OAuth 回调地址配置为 `http://your-host:8080/callback`。
5. 不要把生产首页或回调地址配置成 `http://your-host:3000/...`，`3000` 只是本地 Vite 开发端口。

## 常见问题

### 数据库连接失败

- 检查 MySQL 是否启动。
- 检查 `DATABASE_URL` 是否为 MySQL DSN。
- 检查数据库用户是否有建库和建表权限。

### Redis 连接失败

- 检查 `REDIS_URL` 是否为 `host:port` 格式。
- 检查 Redis 是否启动。
- Redis 失败不会阻止后端启动。

### 前端无法访问后端

- 检查后端是否启动：`curl http://localhost:8080/health`。
- 检查 `frontend/vite.config.ts` 的代理目标是否仍是 `http://localhost:8080`。
- 检查前端请求是否走 `/api/v1`。

### 钉钉登录失败

- 检查 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`、`DINGTALK_CORP_ID`、`DINGTALK_AGENT_ID`。
- 检查钉钉后台首页和回调地址是否能被手机端访问。
- 检查应用权限是否包含通讯录、考勤、审批等所需权限。

## 验证命令

```bash
# 后端
go test ./...
go vet ./...

# 前端
cd frontend
npm run build
npm run lint
npm run e2e
```
