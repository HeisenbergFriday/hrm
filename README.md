# People Ops 后台系统

企业内部后台管理系统，集成钉钉数据，提供员工、部门、考勤、审批等管理功能。

## 系统定位
- 企业内部后台，所有员工、部门、考勤、审批等主数据都来自钉钉
- 优先支持钉钉内免登，后续可扩展扫码登录/钉钉账号登录
- 本系统只做缓存、查询、统计、标签、权限映射，不手工修改原始主数据

## 技术栈
- 后端：Go 1.20
- 框架：Gin
- 数据库：MySQL
- ORM：GORM
- 认证：JWT
- 钉钉API集成

## 目录结构
```
peopleops/
├── cmd/             # 主入口
├── internal/        # 内部代码
│   ├── api/         # API路由和处理函数
│   ├── config/      # 配置管理
│   ├── database/    # 数据库操作
│   ├── dingtalk/    # 钉钉客户端
│   └── middleware/  # 中间件
├── config/          # 配置文件
├── scripts/         # 脚本文件
├── pkg/             # 可重用的包
├── go.mod           # Go模块定义
├── go.sum           # 依赖校验
└── .env             # 环境变量
```

## 功能模块
1. **登录认证**：支持钉钉内免登和账号密码登录
2. **组织架构**：同步钉钉部门和员工信息
3. **员工管理**：查询员工详细信息
4. **考勤管理**：查询员工考勤记录
5. **审批管理**：查询审批流程
6. **权限控制**：基于角色的权限管理
7. **操作日志**：记录系统操作日志

## 快速开始

### 1. 环境准备
- Go 1.20+
- MySQL 5.7+
- 钉钉开发者账号（获取App Key和App Secret）

### 2. 配置
编辑 `.env` 文件，填写以下配置：
```
# 服务器配置
PORT=8080

# 数据库配置
DATABASE_URL=root:password@tcp(localhost:3306)/peopleops?charset=utf8mb4&parseTime=True&loc=Local

# 钉钉配置
DINGTALK_APP_KEY=your_app_key
DINGTALK_APP_SECRET=your_app_secret

# JWT配置
JWT_SECRET=your_jwt_secret
```

### 3. 安装依赖
```bash
go mod tidy
```

### 4. 运行
```bash
go run cmd/main.go
```

## API文档

### 认证
- `POST /api/auth/login` - 账号密码登录
- `POST /api/auth/dingtalk` - 钉钉登录

### 用户管理
- `GET /api/users` - 获取用户列表
- `GET /api/users/:id` - 获取用户详情

### 部门管理
- `GET /api/departments` - 获取部门列表
- `GET /api/departments/:id` - 获取部门详情

### 考勤管理
- `GET /api/attendance` - 获取考勤信息（参数：user_id, start_date, end_date）

### 审批管理
- `GET /api/approvals` - 获取审批信息（参数：user_id, start_date, end_date）

### 权限管理
- `GET /api/permissions` - 获取权限列表
- `POST /api/permissions` - 创建权限
- `PUT /api/permissions/:id` - 更新权限
- `DELETE /api/permissions/:id` - 删除权限

### 操作日志
- `GET /api/logs` - 获取操作日志

### 同步
- `POST /api/sync/users` - 同步用户信息
- `POST /api/sync/departments` - 同步部门信息

## 注意事项
1. 系统依赖钉钉作为唯一数据来源，请确保钉钉API配置正确
2. 首次使用需要先同步组织架构数据
3. 系统只做数据缓存和查询，不修改原始数据
4. 权限管理基于角色和资源进行控制

## 后续规划
1. 扩展扫码登录功能
2. 增加数据统计和分析功能
3. 优化系统性能和安全性
4. 增加更多钉钉集成功能