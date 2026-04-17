# 钉钉一体化人事后台后端API设计

## 1. 技术栈

- Go 1.20+
- Gin 1.9+
- PostgreSQL 15+
- Redis 7+
- GORM 1.25+
- JWT 5+

## 2. API 设计原则

- **RESTful 风格**：使用 HTTP 方法和 URL 表示资源和操作
- **版本控制**：API 路径包含版本号，如 `/api/v1`
- **统一响应格式**：所有 API 返回统一的 JSON 格式
- **错误处理**：统一的错误码和错误信息
- **分页设计**：支持分页查询，使用 `page` 和 `page_size` 参数
- **排序设计**：支持排序，使用 `sort` 参数
- **过滤设计**：支持过滤，使用查询参数

## 3. 统一响应格式

```json
{
  "code": 200,
  "message": "success",
  "data": {}
}
```

- **code**：HTTP 状态码
- **message**：响应消息
- **data**：响应数据

## 4. API 接口设计

### 4.1 认证模块

| API 路径 | 方法 | 功能描述 | 请求体 (JSON) | 响应体 (JSON) |
|---------|------|---------|--------------|---------------|
| `/api/v1/auth/login` | `POST` | 账号密码登录 | `{"username": "admin", "password": "123456"}` | `{"code": 200, "message": "success", "data": {"token": "...", "user": {...}}}` |
| `/api/v1/auth/dingtalk` | `POST` | 钉钉登录 | `{"code": "..."}` | `{"code": 200, "message": "success", "data": {"token": "...", "user": {...}}}` |
| `/api/v1/auth/logout` | `POST` | 登出 | N/A | `{"code": 200, "message": "success"}` |
| `/api/v1/auth/me` | `GET` | 获取当前用户信息 | N/A | `{"code": 200, "message": "success", "data": {"user": {...}}}` |

### 4.2 组织架构模块

| API 路径 | 方法 | 功能描述 | 请求体 (JSON) | 响应体 (JSON) |
|---------|------|---------|--------------|---------------|
| `/api/v1/departments` | `GET` | 获取部门列表 | N/A | `{"code": 200, "message": "success", "data": {"departments": [...]}}` |
| `/api/v1/departments/:id` | `GET` | 获取部门详情 | N/A | `{"code": 200, "message": "success", "data": {"department": {...}}}` |
| `/api/v1/users` | `GET` | 获取用户列表 | N/A | `{"code": 200, "message": "success", "data": {"users": [...], "total": 100}}` |
| `/api/v1/users/:id` | `GET` | 获取用户详情 | N/A | `{"code": 200, "message": "success", "data": {"user": {...}}}` |
| `/api/v1/users/:id` | `PUT` | 更新用户信息（本地扩展字段） | `{"extension": {...}}` | `{"code": 200, "message": "success", "data": {"user": {...}}}` |

### 4.3 考勤模块

| API 路径 | 方法 | 功能描述 | 请求体 (JSON) | 响应体 (JSON) |
|---------|------|---------|--------------|---------------|
| `/api/v1/attendance` | `GET` | 获取考勤记录 | N/A | `{"code": 200, "message": "success", "data": {"attendance": [...], "total": 100}}` |
| `/api/v1/attendance/statistics` | `GET` | 获取考勤统计 | N/A | `{"code": 200, "message": "success", "data": {"statistics": {...}}}` |

### 4.4 审批模块

| API 路径 | 方法 | 功能描述 | 请求体 (JSON) | 响应体 (JSON) |
|---------|------|---------|--------------|---------------|
| `/api/v1/approvals` | `GET` | 获取审批列表 | N/A | `{"code": 200, "message": "success", "data": {"approvals": [...], "total": 100}}` |
| `/api/v1/approvals/:id` | `GET` | 获取审批详情 | N/A | `{"code": 200, "message": "success", "data": {"approval": {...}}}` |

### 4.5 权限模块

| API 路径 | 方法 | 功能描述 | 请求体 (JSON) | 响应体 (JSON) |
|---------|------|---------|--------------|---------------|
| `/api/v1/roles` | `GET` | 获取角色列表 | N/A | `{"code": 200, "message": "success", "data": {"roles": [...]}}` |
| `/api/v1/roles` | `POST` | 创建角色 | `{"name": "管理员", "permissions": [...]}` | `{"code": 201, "message": "success", "data": {"role": {...}}}` |
| `/api/v1/roles/:id` | `PUT` | 更新角色 | `{"name": "管理员", "permissions": [...]}` | `{"code": 200, "message": "success", "data": {"role": {...}}}` |
| `/api/v1/roles/:id` | `DELETE` | 删除角色 | N/A | `{"code": 200, "message": "success"}` |
| `/api/v1/permissions` | `GET` | 获取权限列表 | N/A | `{"code": 200, "message": "success", "data": {"permissions": [...]}}` |
| `/api/v1/users/:id/roles` | `GET` | 获取用户角色 | N/A | `{"code": 200, "message": "success", "data": {"roles": [...]}}` |
| `/api/v1/users/:id/roles` | `POST` | 分配角色给用户 | `{"roles": [...]}` | `{"code": 200, "message": "success"}` |

### 4.6 操作日志模块

| API 路径 | 方法 | 功能描述 | 请求体 (JSON) | 响应体 (JSON) |
|---------|------|---------|--------------|---------------|
| `/api/v1/logs` | `GET` | 获取操作日志 | N/A | `{"code": 200, "message": "success", "data": {"logs": [...], "total": 100}}` |
| `/api/v1/logs/:id` | `GET` | 获取日志详情 | N/A | `{"code": 200, "message": "success", "data": {"log": {...}}}` |

### 4.7 同步模块

| API 路径 | 方法 | 功能描述 | 请求体 (JSON) | 响应体 (JSON) |
|---------|------|---------|--------------|---------------|
| `/api/v1/sync/departments` | `POST` | 同步部门 | N/A | `{"code": 200, "message": "success", "data": {"count": 10}}` |
| `/api/v1/sync/users` | `POST` | 同步用户 | N/A | `{"code": 200, "message": "success", "data": {"count": 100}}` |
| `/api/v1/sync/attendance` | `POST` | 同步考勤 | `{"start_date": "2024-01-01", "end_date": "2024-01-31"}` | `{"code": 200, "message": "success", "data": {"count": 1000}}` |
| `/api/v1/sync/approvals` | `POST` | 同步审批 | `{"start_date": "2024-01-01", "end_date": "2024-01-31"}` | `{"code": 200, "message": "success", "data": {"count": 100}}` |
| `/api/v1/sync/status` | `GET` | 获取同步状态 | N/A | `{"code": 200, "message": "success", "data": {"status": {...}}}` |

## 5. 数据模型设计

### 5.1 用户模型 (User)

| 字段名 | 数据类型 | 描述 | 类型 |
|-------|---------|------|------|
| `id` | `UUID` | 主键 | 本地字段 |
| `user_id` | `VARCHAR(64)` | 钉钉用户ID | 钉钉原始字段 |
| `name` | `VARCHAR(128)` | 姓名 | 钉钉原始字段 |
| `email` | `VARCHAR(128)` | 邮箱 | 钉钉原始字段 |
| `mobile` | `VARCHAR(32)` | 手机号 | 钉钉原始字段 |
| `department_id` | `VARCHAR(64)` | 部门ID | 钉钉原始字段 |
| `position` | `VARCHAR(128)` | 职位 | 钉钉原始字段 |
| `avatar` | `VARCHAR(256)` | 头像URL | 钉钉原始字段 |
| `status` | `VARCHAR(32)` | 状态 | 钉钉原始字段 |
| `extension` | `JSONB` | 本地扩展字段 | 本地扩展字段 |
| `created_at` | `TIMESTAMP` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | 删除时间 | 本地字段 |

### 5.2 部门模型 (Department)

| 字段名 | 数据类型 | 描述 | 类型 |
|-------|---------|------|------|
| `id` | `UUID` | 主键 | 本地字段 |
| `department_id` | `VARCHAR(64)` | 钉钉部门ID | 钉钉原始字段 |
| `name` | `VARCHAR(128)` | 部门名称 | 钉钉原始字段 |
| `parent_id` | `VARCHAR(64)` | 父部门ID | 钉钉原始字段 |
| `order` | `INTEGER` | 排序 | 钉钉原始字段 |
| `extension` | `JSONB` | 本地扩展字段 | 本地扩展字段 |
| `created_at` | `TIMESTAMP` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | 删除时间 | 本地字段 |

### 5.3 考勤模型 (Attendance)

| 字段名 | 数据类型 | 描述 | 类型 |
|-------|---------|------|------|
| `id` | `UUID` | 主键 | 本地字段 |
| `user_id` | `VARCHAR(64)` | 钉钉用户ID | 钉钉原始字段 |
| `user_name` | `VARCHAR(128)` | 用户名 | 钉钉原始字段 |
| `check_time` | `TIMESTAMP` | 打卡时间 | 钉钉原始字段 |
| `check_type` | `VARCHAR(32)` | 打卡类型 | 钉钉原始字段 |
| `location` | `VARCHAR(256)` | 打卡地点 | 钉钉原始字段 |
| `extension` | `JSONB` | 本地扩展字段 | 本地扩展字段 |
| `created_at` | `TIMESTAMP` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | 删除时间 | 本地字段 |

### 5.4 审批模型 (Approval)

| 字段名 | 数据类型 | 描述 | 类型 |
|-------|---------|------|------|
| `id` | `UUID` | 主键 | 本地字段 |
| `process_id` | `VARCHAR(64)` | 钉钉审批流程ID | 钉钉原始字段 |
| `title` | `VARCHAR(256)` | 审批标题 | 钉钉原始字段 |
| `applicant_id` | `VARCHAR(64)` | 申请人ID | 钉钉原始字段 |
| `applicant_name` | `VARCHAR(128)` | 申请人姓名 | 钉钉原始字段 |
| `status` | `VARCHAR(32)` | 审批状态 | 钉钉原始字段 |
| `create_time` | `TIMESTAMP` | 创建时间 | 钉钉原始字段 |
| `finish_time` | `TIMESTAMP` | 完成时间 | 钉钉原始字段 |
| `content` | `JSONB` | 审批内容 | 钉钉原始字段 |
| `extension` | `JSONB` | 本地扩展字段 | 本地扩展字段 |
| `created_at` | `TIMESTAMP` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | 删除时间 | 本地字段 |

### 5.5 角色模型 (Role)

| 字段名 | 数据类型 | 描述 | 类型 |
|-------|---------|------|------|
| `id` | `UUID` | 主键 | 本地字段 |
| `name` | `VARCHAR(64)` | 角色名称 | 本地字段 |
| `description` | `TEXT` | 角色描述 | 本地字段 |
| `created_at` | `TIMESTAMP` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | 删除时间 | 本地字段 |

### 5.6 权限模型 (Permission)

| 字段名 | 数据类型 | 描述 | 类型 |
|-------|---------|------|------|
| `id` | `UUID` | 主键 | 本地字段 |
| `name` | `VARCHAR(64)` | 权限名称 | 本地字段 |
| `code` | `VARCHAR(64)` | 权限代码 | 本地字段 |
| `description` | `TEXT` | 权限描述 | 本地字段 |
| `created_at` | `TIMESTAMP` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | 删除时间 | 本地字段 |

### 5.7 角色权限模型 (RolePermission)

| 字段名 | 数据类型 | 描述 | 类型 |
|-------|---------|------|------|
| `id` | `UUID` | 主键 | 本地字段 |
| `role_id` | `UUID` | 角色ID | 本地字段 |
| `permission_id` | `UUID` | 权限ID | 本地字段 |
| `created_at` | `TIMESTAMP` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | 更新时间 | 本地字段 |

### 5.8 用户角色模型 (UserRole)

| 字段名 | 数据类型 | 描述 | 类型 |
|-------|---------|------|------|
| `id` | `UUID` | 主键 | 本地字段 |
| `user_id` | `VARCHAR(64)` | 钉钉用户ID | 本地字段 |
| `role_id` | `UUID` | 角色ID | 本地字段 |
| `created_at` | `TIMESTAMP` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | 更新时间 | 本地字段 |

### 5.9 操作日志模型 (OperationLog)

| 字段名 | 数据类型 | 描述 | 类型 |
|-------|---------|------|------|
| `id` | `UUID` | 主键 | 本地字段 |
| `user_id` | `VARCHAR(64)` | 操作用户ID | 本地字段 |
| `user_name` | `VARCHAR(128)` | 操作用户名 | 本地字段 |
| `operation` | `VARCHAR(128)` | 操作类型 | 本地字段 |
| `resource` | `VARCHAR(256)` | 操作资源 | 本地字段 |
| `ip` | `VARCHAR(64)` | 操作IP | 本地字段 |
| `details` | `JSONB` | 操作详情 | 本地字段 |
| `created_at` | `TIMESTAMP` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | 更新时间 | 本地字段 |

### 5.10 同步状态模型 (SyncStatus)

| 字段名 | 数据类型 | 描述 | 类型 |
|-------|---------|------|------|
| `id` | `UUID` | 主键 | 本地字段 |
| `type` | `VARCHAR(32)` | 同步类型 | 本地字段 |
| `last_sync_time` | `TIMESTAMP` | 上次同步时间 | 本地字段 |
| `status` | `VARCHAR(32)` | 同步状态 | 本地字段 |
| `message` | `TEXT` | 同步消息 | 本地字段 |
| `created_at` | `TIMESTAMP` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | 更新时间 | 本地字段 |

## 6. 错误码设计

| 错误码 | 描述 |
|-------|------|
| 200 | 成功 |
| 400 | 请求参数错误 |
| 401 | 未授权 |
| 403 | 禁止访问 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |
| 501 | 功能未实现 |
| 502 | 网关错误 |
| 503 | 服务不可用 |

## 7. 分页参数

| 参数名 | 类型 | 描述 | 默认值 |
|-------|------|------|--------|
| `page` | `integer` | 页码 | 1 |
| `page_size` | `integer` | 每页大小 | 10 |

## 8. 排序参数

| 参数名 | 类型 | 描述 | 默认值 |
|-------|------|------|--------|
| `sort` | `string` | 排序字段，如 `created_at:desc` | `created_at:desc` |

## 9. 过滤参数

根据不同的API接口，支持不同的过滤参数，如：

- `user_id`：用户ID
- `department_id`：部门ID
- `start_date`：开始日期
- `end_date`：结束日期
- `status`：状态

## 10. 安全设计

- **JWT 认证**：使用 JWT 进行身份验证
- **HTTPS**：使用 HTTPS 加密传输
- **CORS**：配置 CORS 策略
- **SQL 注入防护**：使用参数化查询
- **XSS 防护**：对输入进行过滤
- **CSRF 防护**：使用 CSRF Token

## 11. 性能优化

- **缓存**：使用 Redis 缓存热点数据
- **索引**：为常用查询字段创建索引
- **分页**：使用分页减少数据传输
- **批量操作**：支持批量同步和查询
- **异步处理**：使用异步任务处理耗时操作