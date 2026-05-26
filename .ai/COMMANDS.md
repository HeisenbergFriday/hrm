---
purpose: 开发、测试、构建、lint 命令
last_updated: 2026-04-30
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
npm run e2e
```

说明：`npm run test` 使用 Vitest 配置 `vite.config.test.ts`；`npm run e2e` 使用 Playwright，通常需要前后端测试环境可用。

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
