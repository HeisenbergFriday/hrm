# 环境变量清单

本文按当前代码实现维护。后端通过 `github.com/joho/godotenv` 从项目根目录 `.env` 加载环境变量。

## 后端基础变量

| 变量名 | 必填 | 默认值 | 描述 |
|---|---|---|---|
| `PORT` | 否 | `8080` | Go 服务监听端口 |
| `DATABASE_URL` | 是 | 无 | MySQL DSN，传给 `gorm.io/driver/mysql` |
| `REDIS_URL` | 否 | 无 | Redis 地址，格式为 `host:port`，例如 `localhost:6379` |
| `REDIS_PASSWORD` | 否 | 空 | Redis 密码 |
| `JWT_SECRET` | 是 | 无 | JWT 签名密钥，建议使用 `openssl rand -base64 48` 生成 |

### MySQL DSN 示例

```env
DATABASE_URL=peopleops_app:<strong_mysql_password>@tcp(localhost:3306)/peopleops?charset=utf8mb4&parseTime=True&loc=Local
```

MySQL 建议使用专用低权限账号和强随机密码，不要在测试或生产环境复用 `root` 账号。

启动时如果 MySQL 连接失败，后端会尝试按 DSN 中的库名创建数据库后重连。

### Redis 地址示例

```env
REDIS_URL=localhost:6379
REDIS_PASSWORD=
```

当前代码把 `REDIS_URL` 直接作为 `redis.Options.Addr`，所以不要写成 `redis://localhost:6379`。Redis 初始化失败不会阻止服务启动，但缓存相关能力会不可用。

## 钉钉集成变量

| 变量名 | 必填 | 描述 |
|---|---|---|
| `DINGTALK_APP_KEY` | 是 | 钉钉应用 App Key |
| `DINGTALK_APP_SECRET` | 是 | 钉钉应用 App Secret |
| `DINGTALK_CORP_ID` | 钉钉免登必填 | 钉钉企业 ID |
| `DINGTALK_AGENT_ID` | 通知/待办相关必填 | 钉钉应用 Agent ID |
| `DINGTALK_ADMIN_USER_ID` | 部分同步回写必填 | 钉钉管理员用户 ID |
| `DINGTALK_REDIRECT_URI` | 扫码登录必填 | OAuth 回调地址，通常指向前端 `/callback` |
| `DINGTALK_APP_HOME_URL` | 微应用必填 | 钉钉微应用首页地址 |
| `DINGTALK_QR_DEFAULT_ORG_ID` | 多企业域名固定入口可选 | 电脑扫码未显式传 `org_id` 时写入 state 的默认本地企业 ID |
| `APP_BASE_URL` | 可选 | 后端服务对外地址 |
| `FRONTEND_BASE_URL` | 可选 | 前端服务对外地址 |

## 考勤工具箱钉钉同步附加变量

考勤工具箱里的“钉钉同步 / 花名册 / 异动流程”会优先读取以下变量作为审批流程编码；如果前端没有显式传参，就会回退到这些环境变量。

| 变量名 | 必填 | 说明 |
|---|---|---|
| `DINGTALK_PROCESS_LEAVE` | 请假同步时必填 | 请假审批流程 code |
| `DINGTALK_PROCESS_OVERTIME` | 加班同步时必填 | 加班审批流程 code |
| `DINGTALK_PROCESS_ATTENDANCE_CORRECTION` | 补卡同步时必填 | 补卡审批流程 code |
| `DINGTALK_PROCESS_POSITION_TRANSFER` | 花名册/异动流程同步时必填 | 岗位异动审批流程 code |

## Linux / Docker 部署说明

如果你在 Linux 上使用 `docker-compose.prod.yml` 部署，容器实际读取的是 `deploy/peopleops.env`，不是项目根目录的 `.env`。

也就是说：

- 修改本地 `.env` 后直接上传到服务器，通常不会自动生效
- 需要修改服务器上的 `deploy/peopleops.env`
- 修改后需要重建或重启容器，例如执行 `docker compose -f docker-compose.prod.yml up -d`

## 假期与调休同步变量

| 变量名 | 描述 |
|---|---|
| `DINGTALK_LEAVE_SYNC_ENABLED` | 是否启用年假同步，`true` 或 `false` |
| `DINGTALK_COMP_TIME_SYNC_ENABLED` | 是否启用调休同步，`true` 或 `false` |
| `DINGTALK_LEAVE_HOURS_PER_DAY` | 天数与小时换算 |
| `DINGTALK_ANNUAL_LEAVE_CODE` | 钉钉年假类型 Code |
| `DINGTALK_ANNUAL_LEAVE_NAME` | 钉钉年假类型名称 |
| `DINGTALK_LIEU_LEAVE_CODE` | 钉钉调休类型 Code |
| `DINGTALK_LIEU_LEAVE_NAME` | 钉钉调休类型名称 |
| `DINGTALK_COMPENSATORY_LEAVE_CODE` | 钉钉补偿假类型 Code |
| `DINGTALK_COMPENSATORY_LEAVE_NAME` | 钉钉补偿假类型名称 |
| `ANNUAL_LEAVE_APPROVAL_KEYWORD` | 年假审批关键词 |

## 排班与节假日变量

| 变量名 | 描述 |
|---|---|
| `DINGTALK_ATTENDANCE_GROUP_ID` | 钉钉考勤组 ID |
| `DINGTALK_ATTENDANCE_GROUP_NAME` | 钉钉考勤组名称 |
| `JUHE_API_KEY` | 聚合数据节假日接口 Key，可选 |

## 测试变量

| 变量名 | 描述 |
|---|---|
| `TEST_DATABASE_URL` | 后端测试数据库 MySQL DSN |
| `SKIP_INTEGRATION_TESTS` | 为 `true` 时跳过集成测试 |

## 前端变量

当前前端 API 实例在 `frontend/src/services/api.ts` 中固定使用 `baseURL=/api/v1`。以下 Vite 变量可作为页面标题、版本等扩展配置使用，但当前不是所有变量都被代码读取。

| 变量名 | 默认建议 | 描述 |
|---|---|---|
| `VITE_API_BASE_URL` | `/api` | API 基础路径，当前代码未统一使用 |
| `VITE_APP_TITLE` | `钉钉一体化人事后台` | 应用标题 |
| `VITE_APP_VERSION` | `1.0.0` | 应用版本 |

## 本地 `.env` 示例

```env
PORT=8080
DATABASE_URL=peopleops_app:<strong_mysql_password>@tcp(localhost:3306)/peopleops?charset=utf8mb4&parseTime=True&loc=Local
REDIS_URL=localhost:6379
REDIS_PASSWORD=

DINGTALK_APP_KEY=your_app_key
DINGTALK_APP_SECRET=your_app_secret
DINGTALK_CORP_ID=dingxxxxxxxx
DINGTALK_AGENT_ID=123456
DINGTALK_ADMIN_USER_ID=manager001
DINGTALK_APP_HOME_URL=http://your-host:8080
DINGTALK_REDIRECT_URI=http://your-host:8080/callback
DINGTALK_QR_DEFAULT_ORG_ID=

JWT_SECRET=<openssl_rand_base64_48>
```

## 钉钉地址配置建议

- `DINGTALK_APP_HOME_URL` 指向应用根地址，例如 `http://your-host:8080/`。
- `DINGTALK_REDIRECT_URI` 指向前端回调页，例如 `http://your-host:8080/callback`。
- 手机端和电脑端都必须能访问这些地址，生产或联调时不要使用只能本机访问的 `localhost`。
- 如果后端托管 `frontend/dist`，钉钉后台优先填写统一的后端地址，不要填写 Vite 开发端口 `3000`。
