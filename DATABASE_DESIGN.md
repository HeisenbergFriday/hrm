# 钉钉一体化人事后台数据库表结构设计

## 1. 数据库选型

- **数据库**：PostgreSQL 15+
- **字符集**：UTF-8
- **排序规则**：C

## 2. 表结构设计

### 2.1 用户表 (`users`)

| 字段名 | 数据类型 | 约束 | 描述 | 类型 |
|-------|---------|------|------|------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | 主键 | 本地字段 |
| `user_id` | `VARCHAR(64)` | `UNIQUE NOT NULL` | 钉钉用户ID | 钉钉原始字段 |
| `name` | `VARCHAR(128)` | `NOT NULL` | 姓名 | 钉钉原始字段 |
| `email` | `VARCHAR(128)` | `UNIQUE` | 邮箱 | 钉钉原始字段 |
| `mobile` | `VARCHAR(32)` | `UNIQUE` | 手机号 | 钉钉原始字段 |
| `department_id` | `VARCHAR(64)` | `NOT NULL` | 部门ID | 钉钉原始字段 |
| `position` | `VARCHAR(128)` | | 职位 | 钉钉原始字段 |
| `avatar` | `VARCHAR(256)` | | 头像URL | 钉钉原始字段 |
| `status` | `VARCHAR(32)` | `NOT NULL` | 状态 | 钉钉原始字段 |
| `extension` | `JSONB` | `DEFAULT '{}'` | 本地扩展字段 | 本地扩展字段 |
| `created_at` | `TIMESTAMP` | `DEFAULT NOW()` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | `DEFAULT NOW()` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | | 删除时间 | 本地字段 |

### 2.2 部门表 (`departments`)

| 字段名 | 数据类型 | 约束 | 描述 | 类型 |
|-------|---------|------|------|------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | 主键 | 本地字段 |
| `department_id` | `VARCHAR(64)` | `UNIQUE NOT NULL` | 钉钉部门ID | 钉钉原始字段 |
| `name` | `VARCHAR(128)` | `NOT NULL` | 部门名称 | 钉钉原始字段 |
| `parent_id` | `VARCHAR(64)` | | 父部门ID | 钉钉原始字段 |
| `order` | `INTEGER` | `DEFAULT 0` | 排序 | 钉钉原始字段 |
| `extension` | `JSONB` | `DEFAULT '{}'` | 本地扩展字段 | 本地扩展字段 |
| `created_at` | `TIMESTAMP` | `DEFAULT NOW()` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | `DEFAULT NOW()` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | | 删除时间 | 本地字段 |

### 2.3 考勤表 (`attendance`)

| 字段名 | 数据类型 | 约束 | 描述 | 类型 |
|-------|---------|------|------|------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | 主键 | 本地字段 |
| `user_id` | `VARCHAR(64)` | `NOT NULL` | 钉钉用户ID | 钉钉原始字段 |
| `user_name` | `VARCHAR(128)` | `NOT NULL` | 用户名 | 钉钉原始字段 |
| `check_time` | `TIMESTAMP` | `NOT NULL` | 打卡时间 | 钉钉原始字段 |
| `check_type` | `VARCHAR(32)` | `NOT NULL` | 打卡类型 | 钉钉原始字段 |
| `location` | `VARCHAR(256)` | | 打卡地点 | 钉钉原始字段 |
| `extension` | `JSONB` | `DEFAULT '{}'` | 本地扩展字段 | 本地扩展字段 |
| `created_at` | `TIMESTAMP` | `DEFAULT NOW()` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | `DEFAULT NOW()` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | | 删除时间 | 本地字段 |

### 2.4 审批表 (`approvals`)

| 字段名 | 数据类型 | 约束 | 描述 | 类型 |
|-------|---------|------|------|------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | 主键 | 本地字段 |
| `process_id` | `VARCHAR(64)` | `UNIQUE NOT NULL` | 钉钉审批流程ID | 钉钉原始字段 |
| `title` | `VARCHAR(256)` | `NOT NULL` | 审批标题 | 钉钉原始字段 |
| `applicant_id` | `VARCHAR(64)` | `NOT NULL` | 申请人ID | 钉钉原始字段 |
| `applicant_name` | `VARCHAR(128)` | `NOT NULL` | 申请人姓名 | 钉钉原始字段 |
| `status` | `VARCHAR(32)` | `NOT NULL` | 审批状态 | 钉钉原始字段 |
| `create_time` | `TIMESTAMP` | `NOT NULL` | 创建时间 | 钉钉原始字段 |
| `finish_time` | `TIMESTAMP` | | 完成时间 | 钉钉原始字段 |
| `content` | `JSONB` | `DEFAULT '{}'` | 审批内容 | 钉钉原始字段 |
| `extension` | `JSONB` | `DEFAULT '{}'` | 本地扩展字段 | 本地扩展字段 |
| `created_at` | `TIMESTAMP` | `DEFAULT NOW()` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | `DEFAULT NOW()` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | | 删除时间 | 本地字段 |

### 2.5 角色表 (`roles`)

| 字段名 | 数据类型 | 约束 | 描述 | 类型 |
|-------|---------|------|------|------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | 主键 | 本地字段 |
| `name` | `VARCHAR(64)` | `UNIQUE NOT NULL` | 角色名称 | 本地字段 |
| `description` | `TEXT` | | 角色描述 | 本地字段 |
| `created_at` | `TIMESTAMP` | `DEFAULT NOW()` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | `DEFAULT NOW()` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | | 删除时间 | 本地字段 |

### 2.6 权限表 (`permissions`)

| 字段名 | 数据类型 | 约束 | 描述 | 类型 |
|-------|---------|------|------|------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | 主键 | 本地字段 |
| `name` | `VARCHAR(64)` | `UNIQUE NOT NULL` | 权限名称 | 本地字段 |
| `code` | `VARCHAR(64)` | `UNIQUE NOT NULL` | 权限代码 | 本地字段 |
| `description` | `TEXT` | | 权限描述 | 本地字段 |
| `created_at` | `TIMESTAMP` | `DEFAULT NOW()` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | `DEFAULT NOW()` | 更新时间 | 本地字段 |
| `deleted_at` | `TIMESTAMP` | | 删除时间 | 本地字段 |

### 2.7 角色权限表 (`role_permissions`)

| 字段名 | 数据类型 | 约束 | 描述 | 类型 |
|-------|---------|------|------|------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | 主键 | 本地字段 |
| `role_id` | `UUID` | `NOT NULL REFERENCES roles(id)` | 角色ID | 本地字段 |
| `permission_id` | `UUID` | `NOT NULL REFERENCES permissions(id)` | 权限ID | 本地字段 |
| `created_at` | `TIMESTAMP` | `DEFAULT NOW()` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | `DEFAULT NOW()` | 更新时间 | 本地字段 |

### 2.8 用户角色表 (`user_roles`)

| 字段名 | 数据类型 | 约束 | 描述 | 类型 |
|-------|---------|------|------|------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | 主键 | 本地字段 |
| `user_id` | `VARCHAR(64)` | `NOT NULL` | 钉钉用户ID | 本地字段 |
| `role_id` | `UUID` | `NOT NULL REFERENCES roles(id)` | 角色ID | 本地字段 |
| `created_at` | `TIMESTAMP` | `DEFAULT NOW()` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | `DEFAULT NOW()` | 更新时间 | 本地字段 |

### 2.9 操作日志表 (`operation_logs`)

| 字段名 | 数据类型 | 约束 | 描述 | 类型 |
|-------|---------|------|------|------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | 主键 | 本地字段 |
| `user_id` | `VARCHAR(64)` | `NOT NULL` | 操作用户ID | 本地字段 |
| `user_name` | `VARCHAR(128)` | `NOT NULL` | 操作用户名 | 本地字段 |
| `operation` | `VARCHAR(128)` | `NOT NULL` | 操作类型 | 本地字段 |
| `resource` | `VARCHAR(256)` | `NOT NULL` | 操作资源 | 本地字段 |
| `ip` | `VARCHAR(64)` | `NOT NULL` | 操作IP | 本地字段 |
| `details` | `JSONB` | `DEFAULT '{}'` | 操作详情 | 本地字段 |
| `created_at` | `TIMESTAMP` | `DEFAULT NOW()` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | `DEFAULT NOW()` | 更新时间 | 本地字段 |

### 2.10 同步状态表 (`sync_status`)

| 字段名 | 数据类型 | 约束 | 描述 | 类型 |
|-------|---------|------|------|------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | 主键 | 本地字段 |
| `type` | `VARCHAR(32)` | `UNIQUE NOT NULL` | 同步类型 | 本地字段 |
| `last_sync_time` | `TIMESTAMP` | | 上次同步时间 | 本地字段 |
| `status` | `VARCHAR(32)` | `NOT NULL` | 同步状态 | 本地字段 |
| `message` | `TEXT` | | 同步消息 | 本地字段 |
| `created_at` | `TIMESTAMP` | `DEFAULT NOW()` | 创建时间 | 本地字段 |
| `updated_at` | `TIMESTAMP` | `DEFAULT NOW()` | 更新时间 | 本地字段 |

## 3. 索引设计

### 3.1 用户表索引

| 索引名 | 字段 | 类型 | 描述 |
|-------|------|------|------|
| `idx_users_user_id` | `user_id` | `UNIQUE` | 唯一索引，用于快速查找用户 |
| `idx_users_email` | `email` | `UNIQUE` | 唯一索引，用于快速查找用户 |
| `idx_users_mobile` | `mobile` | `UNIQUE` | 唯一索引，用于快速查找用户 |
| `idx_users_department_id` | `department_id` | `BTREE` | 普通索引，用于快速查找部门下的用户 |
| `idx_users_status` | `status` | `BTREE` | 普通索引，用于快速查找特定状态的用户 |

### 3.2 部门表索引

| 索引名 | 字段 | 类型 | 描述 |
|-------|------|------|------|
| `idx_departments_department_id` | `department_id` | `UNIQUE` | 唯一索引，用于快速查找部门 |
| `idx_departments_parent_id` | `parent_id` | `BTREE` | 普通索引，用于快速查找子部门 |
| `idx_departments_order` | `order` | `BTREE` | 普通索引，用于排序 |

### 3.3 考勤表索引

| 索引名 | 字段 | 类型 | 描述 |
|-------|------|------|------|
| `idx_attendance_user_id` | `user_id` | `BTREE` | 普通索引，用于快速查找用户的考勤记录 |
| `idx_attendance_check_time` | `check_time` | `BTREE` | 普通索引，用于快速查找特定时间的考勤记录 |
| `idx_attendance_user_id_check_time` | `user_id, check_time` | `BTREE` | 复合索引，用于快速查找用户在特定时间的考勤记录 |

### 3.4 审批表索引

| 索引名 | 字段 | 类型 | 描述 |
|-------|------|------|------|
| `idx_approvals_process_id` | `process_id` | `UNIQUE` | 唯一索引，用于快速查找审批流程 |
| `idx_approvals_applicant_id` | `applicant_id` | `BTREE` | 普通索引，用于快速查找申请人的审批记录 |
| `idx_approvals_status` | `status` | `BTREE` | 普通索引，用于快速查找特定状态的审批记录 |
| `idx_approvals_create_time` | `create_time` | `BTREE` | 普通索引，用于快速查找特定时间的审批记录 |

### 3.5 角色表索引

| 索引名 | 字段 | 类型 | 描述 |
|-------|------|------|------|
| `idx_roles_name` | `name` | `UNIQUE` | 唯一索引，用于快速查找角色 |

### 3.6 权限表索引

| 索引名 | 字段 | 类型 | 描述 |
|-------|------|------|------|
| `idx_permissions_name` | `name` | `UNIQUE` | 唯一索引，用于快速查找权限 |
| `idx_permissions_code` | `code` | `UNIQUE` | 唯一索引，用于快速查找权限 |

### 3.7 角色权限表索引

| 索引名 | 字段 | 类型 | 描述 |
|-------|------|------|------|
| `idx_role_permissions_role_id` | `role_id` | `BTREE` | 普通索引，用于快速查找角色的权限 |
| `idx_role_permissions_permission_id` | `permission_id` | `BTREE` | 普通索引，用于快速查找权限的角色 |
| `idx_role_permissions_role_id_permission_id` | `role_id, permission_id` | `UNIQUE` | 唯一索引，确保角色和权限的组合唯一 |

### 3.8 用户角色表索引

| 索引名 | 字段 | 类型 | 描述 |
|-------|------|------|------|
| `idx_user_roles_user_id` | `user_id` | `BTREE` | 普通索引，用于快速查找用户的角色 |
| `idx_user_roles_role_id` | `role_id` | `BTREE` | 普通索引，用于快速查找角色的用户 |
| `idx_user_roles_user_id_role_id` | `user_id, role_id` | `UNIQUE` | 唯一索引，确保用户和角色的组合唯一 |

### 3.9 操作日志表索引

| 索引名 | 字段 | 类型 | 描述 |
|-------|------|------|------|
| `idx_operation_logs_user_id` | `user_id` | `BTREE` | 普通索引，用于快速查找用户的操作日志 |
| `idx_operation_logs_operation` | `operation` | `BTREE` | 普通索引，用于快速查找特定类型的操作日志 |
| `idx_operation_logs_created_at` | `created_at` | `BTREE` | 普通索引，用于快速查找特定时间的操作日志 |

### 3.10 同步状态表索引

| 索引名 | 字段 | 类型 | 描述 |
|-------|------|------|------|
| `idx_sync_status_type` | `type` | `UNIQUE` | 唯一索引，用于快速查找同步类型 |

## 4. 关系设计

### 4.1 用户与部门的关系
- 用户属于一个部门（多对一）
- 部门可以有多个用户（一对多）

### 4.2 用户与角色的关系
- 用户可以拥有多个角色（多对多）
- 角色可以分配给多个用户（多对多）
- 通过 `user_roles` 表实现

### 4.3 角色与权限的关系
- 角色可以拥有多个权限（多对多）
- 权限可以分配给多个角色（多对多）
- 通过 `role_permissions` 表实现

### 4.4 用户与考勤的关系
- 用户可以有多个考勤记录（一对多）
- 考勤记录属于一个用户（多对一）

### 4.5 用户与审批的关系
- 用户可以发起多个审批（一对多）
- 审批属于一个用户（多对一）

## 5. 数据同步策略

### 5.1 同步频率
- 部门和用户：每小时同步一次
- 考勤：每天同步一次
- 审批：每小时同步一次

### 5.2 同步方式
- 增量同步：只同步新增和变更的数据
- 全量同步：定期进行全量同步，确保数据一致性

### 5.3 同步状态管理
- 使用 `sync_status` 表记录同步状态
- 同步失败时记录错误信息，便于排查

## 6. 数据备份策略

### 6.1 备份频率
- 每日全量备份
- 每小时增量备份

### 6.2 备份存储
- 本地备份
- 云存储备份

## 7. 数据库优化

### 7.1 查询优化
- 使用索引加速查询
- 避免全表扫描
- 使用分页减少数据传输

### 7.2 存储优化
- 合理使用数据类型
- 定期清理过期数据
- 优化表结构

### 7.3 性能优化
- 配置适当的连接池
- 优化 PostgreSQL 配置
- 使用读写分离