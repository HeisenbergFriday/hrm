# 环境变量清单

## 1. 后端环境变量

| 变量名 | 类型 | 必填 | 默认值 | 描述 |
|-------|------|------|-------|------|
| `PORT` | string | 否 | 8080 | 服务器端口 |
| `DATABASE_URL` | string | 是 | - | PostgreSQL 数据库连接字符串 |
| `REDIS_URL` | string | 是 | - | Redis 连接地址 |
| `REDIS_PASSWORD` | string | 否 | "" | Redis 密码 |
| `DINGTALK_APP_KEY` | string | 是 | - | 钉钉应用 App Key |
| `DINGTALK_APP_SECRET` | string | 是 | - | 钉钉应用 App Secret |
| `JWT_SECRET` | string | 是 | - | JWT 密钥 |

## 2. 前端环境变量

| 变量名 | 类型 | 必填 | 默认值 | 描述 |
|-------|------|------|-------|------|
| `VITE_API_BASE_URL` | string | 否 | /api | API 基础路径 |
| `VITE_APP_TITLE` | string | 否 | 钉钉一体化人事后台 | 应用标题 |
| `VITE_APP_VERSION` | string | 否 | 1.0.0 | 应用版本 |

## 3. 数据库配置

### 3.1 PostgreSQL 数据库

| 配置项 | 建议值 | 描述 |
|-------|-------|------|
| 数据库名 | peopleops | 主数据库 |
| 用户名 | postgres | 数据库用户 |
| 密码 | password | 数据库密码 |
| 端口 | 5432 | 数据库端口 |
| 字符集 | UTF-8 | 数据库字符集 |

### 3.2 Redis 配置

| 配置项 | 建议值 | 描述 |
|-------|-------|------|
| 主机 | localhost | Redis 主机 |
| 端口 | 6379 | Redis 端口 |
| 密码 | "" | Redis 密码 |
| 数据库 | 0 | Redis 数据库 |

## 4. 钉钉配置

### 4.1 钉钉开发者平台配置

| 配置项 | 描述 |
|-------|------|
| App Key | 钉钉应用的 App Key |
| App Secret | 钉钉应用的 App Secret |
| 回调地址 | 钉钉登录回调地址，格式：http://your-domain/api/v1/auth/dingtalk |
| 权限范围 | 需要申请的权限：用户信息、部门信息、考勤信息、审批信息 |

## 5. 示例配置

### 5.1 后端 .env 文件示例

```env
# 服务器配置
PORT=8080

# 数据库配置
DATABASE_URL=postgres://postgres:password@localhost:5432/peopleops?sslmode=disable

# Redis配置
REDIS_URL=localhost:6379
REDIS_PASSWORD=

# 钉钉配置
DINGTALK_APP_KEY=your_app_key
DINGTALK_APP_SECRET=your_app_secret

# JWT配置
JWT_SECRET=your_jwt_secret
```

### 5.2 前端 .env 文件示例

```env
VITE_API_BASE_URL=/api
VITE_APP_TITLE=钉钉一体化人事后台
VITE_APP_VERSION=1.0.0
```

## 6. 注意事项

1. **敏感信息保护**：不要将包含敏感信息的环境变量文件提交到版本控制系统
2. **不同环境配置**：为不同环境（开发、测试、生产）创建不同的环境变量文件
3. **配置验证**：启动服务前确保所有必要的环境变量已正确配置
4. **安全存储**：对于生产环境，使用安全的方式存储环境变量，如环境变量管理服务或密钥管理系统

## 7. 环境变量加载顺序

1. 系统环境变量
2. `.env` 文件
3. 默认值

## 8. 常见问题

### 8.1 环境变量未生效
- **问题**：环境变量设置后未生效
- **解决方案**：
  1. 检查环境变量文件是否在正确的位置
  2. 检查环境变量名称是否正确
  3. 重启服务使环境变量生效

### 8.2 数据库连接失败
- **问题**：无法连接到数据库
- **解决方案**：
  1. 检查 `DATABASE_URL` 是否正确
  2. 确保数据库服务已启动
  3. 检查数据库用户权限

### 8.3 钉钉 API 调用失败
- **问题**：无法调用钉钉 API
- **解决方案**：
  1. 检查 `DINGTALK_APP_KEY` 和 `DINGTALK_APP_SECRET` 是否正确
  2. 确保网络连接正常
  3. 检查钉钉开发者平台的配置

## 9. 部署建议

### 9.1 开发环境
- 使用 `.env` 文件管理环境变量
- 配置本地数据库和 Redis

### 9.2 测试环境
- 使用环境变量管理服务
- 配置独立的测试数据库

### 9.3 生产环境
- 使用密钥管理系统存储敏感信息
- 配置高可用数据库和 Redis
- 使用 HTTPS 加密传输