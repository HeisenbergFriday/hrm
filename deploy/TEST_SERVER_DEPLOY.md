# PeopleOps HR 测试服隔离部署说明

这份说明只用于测试服务器，目标是避免和服务器上已有 app 混用镜像、容器、目录、端口或环境变量。

## 隔离约定

| 项目 | 测试服约定 |
|---|---|
| SSH 入口 | `ssh ubuntu@113.240.65.185 -p 16388` |
| 服务器目录 | `/opt/peopleops-hr-test`，无 sudo 时使用 `/home/ubuntu/peopleops-hr-test` |
| Compose 项目名 | `peopleops-hr-test` |
| Compose app service | `peopleops-hr` |
| Docker 镜像 | `peopleops-hr:test` |
| Docker 容器 | `peopleops-hr-test` |
| MySQL 容器 | `peopleops-hr-test-mysql` |
| Redis 容器 | `peopleops-hr-test-redis` |
| Compose 文件 | `docker-compose.test.yml` |
| 环境文件 | `deploy/peopleops.test.env` |
| 宿主机端口 | `18080` |
| 容器端口 | `8080` |
| 上传目录 | `/opt/peopleops-hr-test/uploads` |
| MySQL 数据目录 | `/opt/peopleops-hr-test/mysql-data` |
| Redis 数据目录 | `/opt/peopleops-hr-test/redis-data` |

测试服数据库镜像使用 `mysql:5.7`，用于兼容较老 CPU 的测试服务器。

## 本地打包

在项目根目录执行：

```powershell
docker build -t peopleops-hr:test .
docker save peopleops-hr:test -o peopleops-hr-test.tar
```

把以下文件上传到测试服务器的 `/opt/peopleops-hr-test`：

```text
peopleops-hr-test.tar
docker-compose.test.yml
deploy/peopleops.test.env.example
deploy/TEST_SERVER_DEPLOY.md
```

## 服务器准备

```bash
mkdir -p /opt/peopleops-hr-test/deploy /opt/peopleops-hr-test/uploads
cd /opt/peopleops-hr-test
cp deploy/peopleops.test.env.example deploy/peopleops.test.env
```

如果当前用户没有 sudo 权限，可以把上面的 `/opt/peopleops-hr-test` 换成 `/home/ubuntu/peopleops-hr-test`。

编辑 `deploy/peopleops.test.env`，至少替换：

```env
MYSQL_PASSWORD=...
MYSQL_ROOT_PASSWORD=...
DATABASE_URL=peopleops_test:<MYSQL_PASSWORD>@tcp(mysql:3306)/peopleops_test?charset=utf8mb4&parseTime=True&loc=Local
JWT_SECRET=...
ADMIN_USER_ID=admin
ADMIN_PASSWORD=...
DINGTALK_APP_HOME_URL=http://服务器IP或域名:18080/
DINGTALK_REDIRECT_URI=http://服务器IP或域名:18080/callback
DINGTALK_PROCESS_LEAVE=请假审批流程码
DINGTALK_PROCESS_OVERTIME=加班审批流程码
DINGTALK_PROCESS_ATTENDANCE_CORRECTION=补卡审批流程码
DINGTALK_PROCESS_POSITION_TRANSFER=岗位异动审批流程码
APP_BASE_URL=http://服务器IP或域名:18080
FRONTEND_BASE_URL=http://服务器IP或域名:18080
CORS_ALLOW_ORIGINS=http://服务器IP或域名:18080
```

### 环境变量分级（不复制敏感值）

**完整变量清单与占位符以 `deploy/peopleops.test.env.example` 为准**；本文只说明分级与启用条件，禁止在文档中粘贴真实密码/密钥。

| 分级 | 何时需要 | 代表变量（见 example） |
|---|---|---|
| **基础必填** | 任何可启动的测试栈 | `APP_ENV`、`PORT`、`TZ`、`MYSQL_*`、`DATABASE_URL`、`REDIS_URL`、`JWT_SECRET`、`ADMIN_USER_ID`、`ADMIN_PASSWORD`、`APP_BASE_URL`、`FRONTEND_BASE_URL`、`CORS_ALLOW_ORIGINS` |
| **钉钉 / 单组织** | 启用钉钉登录、同步、审批流程 | `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`、`DINGTALK_CORP_ID`、`DINGTALK_AGENT_ID`、`DINGTALK_ADMIN_USER_ID`（仅 default 兼容）、`DINGTALK_APP_HOME_URL`、`DINGTALK_REDIRECT_URI`、`DINGTALK_PROCESS_*` |
| **钉钉 / 多组织** | 多企业登录与按 org 读凭证/流程码 | `DINGTALK_ORGANIZATIONS`（JSON：各 org 的 `org_id`/`corp_id`/`app_key`/`app_secret`/`dingtalk_admin_user_id`/`process_codes`）；可选 `DINGTALK_QR_DEFAULT_ORG_ID`。非 default 禁止回退全局 `DINGTALK_ADMIN_USER_ID` / 全局流程码 |
| **假期 / 调休** | 年假、调休、假期类型同步 | `DINGTALK_LEAVE_SYNC_ENABLED`、`DINGTALK_COMP_TIME_SYNC_ENABLED`、`DINGTALK_*_LEAVE_CODE/NAME`、`DINGTALK_LEAVE_HOURS_PER_DAY`、`ANNUAL_LEAVE_APPROVAL_KEYWORD` 等 |
| **外部考勤同步（Doris）** | 一期外部仓 → 本地打卡 | `EXTERNAL_ATTENDANCE_SYNC_ENABLED` 及 `EXTERNAL_ATTENDANCE_DB_*`、`EXTERNAL_ATTENDANCE_SYNC_*` 等；密码只放 `peopleops.test.env` 或密钥库 |

真实环境变量文件不要提交，也不要和其他 app 共用。测试栈会启动自己的 MySQL 和 Redis，不依赖服务器上其他 app 的数据库或缓存。

> 单组织/默认组织可使用 `DINGTALK_PROCESS_*`。多组织必须在 `DINGTALK_ORGANIZATIONS` 对应组织对象内配置 `process_codes`（`leave` / `overtime` / `attendance_correction` / `position_transfer`），考勤工具箱会按当前 JWT `org_id` 读取该组织的应用凭证和流程码，禁止跨组织回退。使用 `-SkipConfigUpload` 只部署代码，不会上传新增或修改的组织流程码。

`ADMIN_PASSWORD` 只在首次初始化管理员时生效。如果数据库已经初始化过，需要先确认没有业务数据，再重建测试库或手动重置管理员密码。

## 启动

```bash
docker load -i peopleops-hr-test.tar
docker compose -p peopleops-hr-test -f docker-compose.test.yml up -d
```

检查：

```bash
docker compose -p peopleops-hr-test -f docker-compose.test.yml ps
docker compose -p peopleops-hr-test -f docker-compose.test.yml logs --tail=100 peopleops-hr
curl http://127.0.0.1:18080/health
```

浏览器访问：

```text
http://服务器IP或域名:18080/
```

## 更新

### 日常完整更新（有代码改动）

在开发机项目根目录：

```powershell
.\deploy\build-and-deploy.ps1 -SkipConfigUpload
```

会重新 `go build` + 前端 build + Docker 镜像 + 上传 + 重启。

### 仅重传已有镜像 tar（上传失败 / 同一包再推）

当本地已有刚构建成功的 `peopleops-hr-test.tar`（例如完整脚本在 scp 阶段断线），**不要**整包重 build，用独立快路径：

```powershell
.\deploy\upload-and-restart.ps1
```

说明：

- **不修改、不调用** `build-and-deploy.ps1`
- **不重新编译**代码；只 scp + 远端 size 校验 + `docker load` + 重启
- 默认只 `force-recreate` 应用服务 `peopleops-hr`（MySQL/Redis 不动）；需要整栈 down/up 时加 `-FullStack`
- 默认保留本地 tar 方便重试；成功后想清理加 `-CleanupLocal`
- tar 超过 24 小时会警告；要硬失败加 `-FailOnStaleTar`

收到新的 `peopleops-hr-test.tar` 后（服务器侧手工）：

```bash
cd /opt/peopleops-hr-test
docker compose -p peopleops-hr-test -f docker-compose.test.yml up -d
```

如果镜像已经导入过旧版本，先重新导入：

```bash
docker load -i peopleops-hr-test.tar
docker compose -p peopleops-hr-test -f docker-compose.test.yml up -d --force-recreate
```
