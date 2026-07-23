---
purpose: 开发、测试、构建、lint 命令
last_updated: 2026-07-20
source_of_truth:
  - frontend/package.json（前端命令）
  - README.md（项目说明）
  - tools/（运维脚本）
update_when:
  - 新增开发命令时
  - 修改测试命令时
  - 新增构建命令时
  - 修改 lint 命令时
  - 新增运维脚本时
---

# 常用命令

## 后端

### 启动服务
```bash
go run ./cmd/main.go
```

### 运行测试
```bash
# 所有测试
go test ./...

# 指定包
go test ./internal/service

# 带覆盖率
go test -cover ./...

# 跳过集成测试
SKIP_INTEGRATION_TESTS=true go test ./...
```

### 代码检查
```bash
# 格式化
go fmt ./...

# Lint
golangci-lint run

# Vet
go vet ./...
```

### 依赖管理
```bash
# 安装依赖
go mod download

# 整理依赖
go mod tidy

# 查看依赖
go mod graph
```

---

## 前端

### 启动开发服务器
```bash
cd frontend
npm run dev
```

默认端口 `3000`，通过 Vite 代理 `/api` 到 `http://localhost:8080`。

### 构建
```bash
cd frontend
npm run build
```

产物输出到 `frontend/dist`。

### 预览构建产物
```bash
cd frontend
npm run preview
```

### 运行测试
```bash
cd frontend
npm run test
npm run test:e2e
```

说明：`npm run test` 使用 Vitest 配置 `vite.config.test.ts`；`npm run test:e2e` 使用 Playwright，会按 `playwright.config.ts` 自动启动专用 Vite dev server，并通过用例内 mock API 避免依赖真实后端。 On Windows, set `PLAYWRIGHT_REUSE_SERVER=1` only when a dedicated server is already listening on port 5273; the default remains an isolated auto-started server.

### 代码检查
```bash
cd frontend

# Lint
npm run lint
```

说明：当前没有单独的 `type-check` script，类型检查由 `npm run build` 中的 `tsc` 执行。

### 依赖管理
```bash
cd frontend

# 安装依赖
npm install

# 安装指定包
npm install <package-name>

# 更新依赖
npm update
```

---

## Git Hooks

### 安装 hooks
```bash
bash tools/install-hooks.sh
```

安装后，每次 `git commit` 时会自动检查结构性变更，提醒更新 CLAUDE.md。

---

## 数据库

### 连接数据库
```bash
mysql -h <host> -u <user> -p <database>
```

### 导出数据库
```bash
mysqldump -h <host> -u <user> -p <database> > backup.sql
```

### 导入数据库
```bash
mysql -h <host> -u <user> -p <database> < backup.sql
```

---

## 运维脚本

### 钉钉考勤与审批权限预检
```powershell
# Windows PowerShell：先构建固定 exe，避免 go run 临时程序被安全软件拦截
go build -o .\dingtalk_attendance_preflight.exe .\tools\ops\dingtalk_attendance_preflight
.\dingtalk_attendance_preflight.exe -user <测试员工user_id> -process-code <审批模板process_code> -start 2026-07-01 -end 2026-07-14
```

```bash
# Linux / macOS
go run ./tools/ops/dingtalk_attendance_preflight -user <测试员工user_id> -process-code <审批模板process_code> -start 2026-07-01 -end 2026-07-14
```

说明：
- 工具不会发起或审批流程，也不会修改排班、打卡记录或假期余额。
- PowerShell 不能使用 `\` 续行；请使用一整行命令，或使用 PowerShell 反引号续行。
- 未传 `-user` 时会尝试使用 `DINGTALK_PREFLIGHT_USER_ID` 或 `DINGTALK_ADMIN_USER_ID`。
- 未传 `-process-code` 时会尝试使用 `DINGTALK_ATTENDANCE_APPROVAL_PROCESS_CODE` 或 `DINGTALK_OVERTIME_PROCESS_CODE`。
- 可增加 `-json` 输出结构化检查结果。

### 外部 Doris 考勤同步环境变量
```text
EXTERNAL_ATTENDANCE_DATABASE_URL=user:pass@tcp(host:9030)/dwd?parseTime=true
# 或拆分：EXTERNAL_ATTENDANCE_DB_HOST/PORT/USER/PASSWORD/SCHEMA
EXTERNAL_ATTENDANCE_SYNC_ENABLED=true|false
EXTERNAL_ATTENDANCE_SYNC_INTERVAL=15m
EXTERNAL_ATTENDANCE_SYNC_LOOKBACK_MINUTES=30
EXTERNAL_ATTENDANCE_QUERY_TIMEOUT=30s
# 首次无 cursor 回填起点；未设置默认 Unix epoch（全量历史），格式 RFC3339 / "2006-01-02 15:04:05" / "2006-01-02"
EXTERNAL_ATTENDANCE_INITIAL_START_TIME=2026-04-01
```

只读联调（禁止 DDL/DML，勿打印 PII）：
```bash
# 只读冒烟：GET /api/v1/attendance/external-sync/status（连通性/启用态，不写库）
# 健康检查走后端 GET /api/v1/attendance/external-sync/status
# 手动同步 POST /api/v1/attendance/external-sync/run  body: {"source":"attendance"}
# 集成测试依赖外部时设置 SKIP_INTEGRATION_TESTS=true 跳过
```

### 钉钉 Stream 事件订阅客户端
```powershell
# Windows PowerShell
go build -o .\dingtalk_stream.exe .\cmd\dingtalk_stream
.\dingtalk_stream.exe
```

启动后看到“钉钉 Stream 已连接”或 SDK 的 `connect success` 日志，再到钉钉开发者后台点击“已完成接入，验证连接通道”。

说明：
- Stream 客户端默认订阅并确认所有已在开发者后台选择的事件，但暂不将事件写入数据库。
- 默认不输出事件原始内容；仅联调时可配置 `DINGTALK_STREAM_LOG_PAYLOAD=true`，日志可能包含审批业务信息。
- 如需代理，可配置 `DINGTALK_STREAM_PROXY`。
- 生产环境建议将该客户端作为独立常驻进程或服务运行。

### 重新同步加班到钉钉
```bash
cd tools/resync_overtime_to_dingtalk
go run main.go
```

### 重置假期配额
```bash
cd tools/reset_vacation_quota
go run main.go
```

### 设置调休余额
```bash
cd tools/set_comp_time_balance
go run main.go
```

### 重新同步调休
```bash
go run tools/ops/resync_comp_time/main.go
```

---

## Docker（如果使用）

### 构建镜像
```bash
docker build -t peopleops:latest .
```

### 测试服构建与部署
```powershell
.\deploy\build-and-deploy.ps1 -SkipConfigUpload
```

脚本生成的测试镜像必须包含 `tools/attendance_toolbox`、独立 Python 虚拟环境及运行依赖；镜像构建后会自动执行 `runner.py --defaults` 冒烟校验，失败时不会继续上传部署。

首次构建需要从 Docker Hub 拉取 `python:3.12-slim`。脚本会对 Docker 构建最多重试 3 次；若仍出现 `TLS handshake timeout`，可先执行 `docker pull python:3.12-slim`，成功后再重新运行部署命令。

### 运行容器
```bash
docker run -d -p 8080:8080 --env-file .env peopleops:latest
```

### 查看日志
```bash
docker logs -f <container-id>
```

---

## 常见问题

### 前端无法连接后端
- 检查后端是否启动：`curl http://localhost:8080/health`
- 检查 Vite 代理配置：`frontend/vite.config.ts`

### 数据库连接失败
- 检查 `DATABASE_URL` 环境变量
- 检查 MySQL 是否启动
- 检查数据库是否存在（启动时会自动创建）

### Redis 连接失败
- 检查 `REDIS_URL` 环境变量
- 检查 Redis 是否启动
- Redis 失败不会阻止服务启动，但缓存功能会受影响

### 钉钉同步失败
- 检查 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`、`DINGTALK_CORP_ID` 环境变量
- 检查钉钉应用权限
- 查看日志：`logrus` 会输出详细错误信息

### 前端构建失败
- 删除 `node_modules` 和 `package-lock.json`，重新 `npm install`
- 检查 Node.js 版本（推荐 18+）

### 后端测试失败
- 检查 `TEST_DATABASE_URL` 环境变量
- 确保测试数据库存在
- 使用 `SKIP_INTEGRATION_TESTS=true` 跳过集成测试
