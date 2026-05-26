# 环境变量清单

本文按当前代码实现维护。后端通过 `github.com/joho/godotenv` 从项目根目录 `.env` 加载环境变量。

## 后端基础变量

| 变量名 | 必填 | 默认值 | 描述 |
|---|---|---|---|
| `PORT` | 否 | `8080` | Go 服务监听端口 |
| `DATABASE_URL` | 是 | 无 | MySQL DSN，传给 `gorm.io/driver/mysql` |
| `REDIS_URL` | 否 | 无 | Redis 地址，格式为 `host:port`，例如 `localhost:6379` |
| `REDIS_PASSWORD` | 否 | 空 | Redis 密码 |
| `JWT_SECRET` | 建议必填 | 代码默认值 | JWT 签名密钥 |

### MySQL DSN 示例

```env
DATABASE_URL=root:password@tcp(localhost:3306)/peopleops?charset=utf8mb4&parseTime=True&loc=Local
```

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
| `APP_BASE_URL` | 可选 | 后端服务对外地址 |
| `FRONTEND_BASE_URL` | 可选 | 前端服务对外地址 |

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
DATABASE_URL=root:password@tcp(localhost:3306)/peopleops?charset=utf8mb4&parseTime=True&loc=Local
REDIS_URL=localhost:6379
REDIS_PASSWORD=

DINGTALK_APP_KEY=your_app_key
DINGTALK_APP_SECRET=your_app_secret
DINGTALK_CORP_ID=dingxxxxxxxx
DINGTALK_AGENT_ID=123456
DINGTALK_ADMIN_USER_ID=manager001
DINGTALK_APP_HOME_URL=http://your-host:8080
DINGTALK_REDIRECT_URI=http://your-host:8080/callback

JWT_SECRET=change_me
```

## 钉钉地址配置建议

- `DINGTALK_APP_HOME_URL` 指向应用根地址，例如 `http://your-host:8080/`。
- `DINGTALK_REDIRECT_URI` 指向前端回调页，例如 `http://your-host:8080/callback`。
- 手机端和电脑端都必须能访问这些地址，生产或联调时不要使用只能本机访问的 `localhost`。
- 如果后端托管 `frontend/dist`，钉钉后台优先填写统一的后端地址，不要填写 Vite 开发端口 `3000`。
