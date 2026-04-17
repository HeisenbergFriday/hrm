# 钉钉一体化人事后台联调说明

## 1. 环境准备

### 1.1 后端环境
- Go 1.20+
- PostgreSQL 15+
- Redis 7+

### 1.2 前端环境
- Node.js 16+
- npm 8+

## 2. 配置说明

### 2.1 后端配置
编辑 `backend/.env` 文件：

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

### 2.2 前端配置
编辑 `frontend/vite.config.ts` 文件：

```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  }
})
```

## 3. 启动步骤

### 3.1 启动后端服务

1. 进入后端目录：
   ```bash
   cd backend
   ```

2. 安装依赖：
   ```bash
   go mod tidy
   ```

3. 启动服务：
   ```bash
   go run cmd/main.go
   ```

   或者使用构建脚本：
   ```bash
   scripts/run.bat
   ```

### 3.2 启动前端服务

1. 进入前端目录：
   ```bash
   cd frontend
   ```

2. 安装依赖：
   ```bash
   npm install
   ```

3. 启动服务：
   ```bash
   npm run dev
   ```

## 4. API 调用方式

### 4.1 认证
- **登录**：`POST /api/v1/auth/login`
- **钉钉登录**：`POST /api/v1/auth/dingtalk`
- **登出**：`POST /api/v1/auth/logout`
- **获取当前用户**：`GET /api/v1/auth/me`

### 4.2 用户管理
- **获取用户列表**：`GET /api/v1/users`
- **获取用户详情**：`GET /api/v1/users/:id`
- **更新用户信息**：`PUT /api/v1/users/:id`

### 4.3 部门管理
- **获取部门列表**：`GET /api/v1/departments`
- **获取部门详情**：`GET /api/v1/departments/:id`

### 4.4 同步管理
- **同步部门**：`POST /api/v1/sync/departments`
- **同步用户**：`POST /api/v1/sync/users`
- **获取同步状态**：`GET /api/v1/sync/status`

## 5. 常见问题及解决方案

### 5.1 数据库连接失败
- **问题**：无法连接到 PostgreSQL 数据库
- **解决方案**：
  1. 确保 PostgreSQL 服务已启动
  2. 检查数据库连接字符串是否正确
  3. 确保数据库用户有足够的权限

### 5.2 Redis 连接失败
- **问题**：无法连接到 Redis 服务
- **解决方案**：
  1. 确保 Redis 服务已启动
  2. 检查 Redis 连接字符串是否正确
  3. 确保 Redis 服务可以正常访问

### 5.3 钉钉 API 调用失败
- **问题**：无法调用钉钉 API
- **解决方案**：
  1. 确保钉钉 App Key 和 App Secret 正确
  2. 确保网络连接正常
  3. 检查钉钉开发者平台的配置

### 5.4 前端代理配置错误
- **问题**：前端无法访问后端 API
- **解决方案**：
  1. 检查 `vite.config.ts` 中的代理配置
  2. 确保后端服务已启动
  3. 检查后端服务的端口是否正确

## 6. 测试账号

### 6.1 管理员账号
- **用户名**：admin
- **密码**：123456

### 6.2 测试用户账号
- **用户名**：test
- **密码**：123456

## 7. 部署建议

### 7.1 开发环境
- 使用本地开发环境，直接启动前后端服务

### 7.2 测试环境
- 使用 Docker 容器化部署
- 配置独立的测试数据库

### 7.3 生产环境
- 使用容器化部署
- 配置高可用数据库
- 使用 HTTPS 加密传输
- 配置监控和告警

## 8. 监控与日志

### 8.1 后端日志
- 日志文件：`backend/logs/app.log`
- 日志级别：info, warn, error

### 8.2 前端日志
- 浏览器控制台
- 前端错误监控

## 9. 性能优化

### 9.1 后端优化
- 使用 Redis 缓存热点数据
- 优化数据库查询
- 使用连接池

### 9.2 前端优化
- 使用 React.lazy 和 Suspense 实现组件懒加载
- 使用 React Query 缓存 API 响应
- 优化图片资源

## 10. 安全考虑

### 10.1 后端安全
- 使用 JWT 进行身份验证
- 实现基于角色的权限控制
- 对敏感数据进行加密存储
- 防止 SQL 注入和 XSS 攻击

### 10.2 前端安全
- 防止 XSS 攻击
- 防止 CSRF 攻击
- 安全存储用户凭证

## 11. 版本管理

### 11.1 后端版本
- 版本号：1.0.0
- 主要功能：登录、组织架构同步、员工信息查询、考勤查询、审批查询、权限控制、操作日志

### 11.2 前端版本
- 版本号：1.0.0
- 主要功能：登录、组织架构管理、考勤管理、审批管理、权限管理、操作日志、系统设置

## 12. 联系方式

- **开发团队**：People Ops 开发组
- **联系邮箱**：dev@peopleops.com
- **技术支持**：support@peopleops.com