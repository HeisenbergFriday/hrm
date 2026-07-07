# PeopleOps 生产环境部署检查清单

**版本：** 2026-6-23-10-13  
**更新日期：** 2026年7月7日  
**检查人员：** ________  
**部署日期：** ________

---

## 一、部署前准备

### 1.1 服务器环境

- [ ] 服务器操作系统：Ubuntu 20.04+ / CentOS 7+ / Debian 11+
- [ ] Go 版本：1.26.4+
- [ ] Node.js 版本：18+
- [ ] Docker 版本：20.10+ / Docker Compose 2.0+
- [ ] MySQL 版本：5.7+ / 8.0+
- [ ] Redis 版本：7.0+
- [ ] 磁盘空间：至少 20 GB 可用
- [ ] 内存：至少 4 GB
- [ ] CPU：至少 2 核

### 1.2 网络配置

- [ ] 域名已备案（如需）
- [ ] DNS 解析已配置
- [ ] SSL 证书已申请（HTTPS 必需）
- [ ] 防火墙规则：
  - [ ] 开放 80 端口（HTTP，用于重定向）
  - [ ] 开放 443 端口（HTTPS）
  - [ ] 限制 3306 端口（MySQL，仅应用服务器）
  - [ ] 限制 6379 端口（Redis，仅应用服务器）

### 1.3 钉钉应用配置

- [ ] 钉钉应用已创建
- [ ] 应用权限已授权：
  - [ ] 通讯录管理（读取组织架构）
  - [ ] 考勤管理（读取考勤数据）
  - [ ] 审批管理（读取审批数据）
  - [ ] 企业消息（发送通知）
- [ ] 应用首页设置为：`https://your-domain.com/`
- [ ] OAuth 回调地址设置为：`https://your-domain.com/callback`
- [ ] 获取到 AppKey、AppSecret、CorpID、AgentID

---

## 二、安全配置

### 2.1 生成强随机密钥

```bash
# 生成 JWT Secret（至少 48 字节）
openssl rand -base64 48

# 生成管理员密码（至少 16 字符）
openssl rand -base64 24
```

### 2.2 环境变量配置

#### 必需配置项
- [ ] `JWT_SECRET` - ✅ 使用强随机密钥（不要使用示例值）
- [ ] `ADMIN_PASSWORD` - ✅ 使用强随机密码（不要使用示例值）
- [ ] `DATABASE_URL` - ✅ 使用专用低权限账号（不要使用 root）
- [ ] `DINGTALK_APP_KEY` - 钉钉 AppKey
- [ ] `DINGTALK_APP_SECRET` - 钉钉 AppSecret
- [ ] `DINGTALK_CORP_ID` - 钉钉 CorpID
- [ ] `DINGTALK_AGENT_ID` - 钉钉 AgentID

#### 安全增强配置
- [ ] `AUTH_COOKIE_SECURE=true` - ✅ 启用 HTTPS Cookie（生产必需）
- [ ] `AUTH_COOKIE_SAMESITE=lax` - ✅ CSRF 防护
- [ ] `JWT_TTL_MINUTES=480` - JWT 过期时间（默认 8 小时）
- [ ] `AUTH_SESSION_VERSION=cookie-v1` - 会话版本

#### 文件上传安全
- [ ] `CLAMAV_ADDR=127.0.0.1:3310` - ClamAV 地址（如已部署）
- [ ] `UPLOAD_REQUIRE_ANTIVIRUS=true` - 强制病毒扫描（推荐）

### 2.3 数据库安全

- [ ] 创建专用数据库账号（非 root）
- [ ] 仅授予必需权限：
  ```sql
  CREATE USER 'peopleops_app'@'%' IDENTIFIED BY 'strong_random_password';
  CREATE DATABASE peopleops CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
  GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX ON peopleops.* TO 'peopleops_app'@'%';
  FLUSH PRIVILEGES;
  ```
- [ ] 数据库密码足够强（16+ 字符，包含大小写字母、数字、特殊字符）
- [ ] 限制数据库访问 IP（仅应用服务器）

### 2.4 Redis 安全

- [ ] 设置 Redis 密码（如果 Redis 版本支持）
- [ ] 限制 Redis 访问 IP（仅应用服务器）
- [ ] 禁用危险命令：
  ```conf
  # redis.conf
  rename-command FLUSHALL ""
  rename-command FLUSHDB ""
  rename-command CONFIG ""
  ```

---

## 三、部署步骤

### 3.1 代码部署

#### 3.1.1 克隆代码
```bash
cd /opt
git clone https://github.com/HeisenbergFriday/hrm.git peopleops
cd peopleops
git checkout 2026-6-23-10-13
```

#### 3.1.2 构建前端
```bash
cd frontend
npm install
npm run build
cd ..
```

- [ ] 前端构建成功
- [ ] `frontend/dist` 目录已生成
- [ ] 检查构建产物大小（应约 1.8 MB）

#### 3.1.3 配置环境变量
```bash
cp deploy/peopleops.env.example .env
vim .env
```

- [ ] 所有必需配置项已填写
- [ ] 所有密钥已替换为强随机值
- [ ] 数据库连接信息正确
- [ ] 钉钉应用信息正确

#### 3.1.4 Docker 部署（推荐）

使用 Docker Compose 部署：
```bash
docker compose -f docker-compose.prod.yml up -d
```

- [ ] 容器启动成功
- [ ] 数据库初始化成功
- [ ] 应用服务健康检查通过

#### 3.1.5 手动部署

不使用 Docker 时：
```bash
# 编译后端
go build -o peopleops ./cmd/main.go

# 启动服务（使用 systemd 或 supervisor）
./peopleops
```

- [ ] 后端编译成功
- [ ] 服务启动成功
- [ ] 健康检查通过：`curl http://localhost:8080/health`

### 3.2 反向代理配置

#### 3.2.1 Nginx 配置示例

```nginx
server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    client_max_body_size 50M;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
    }
}
```

- [ ] Nginx 配置已部署
- [ ] SSL 证书已配置
- [ ] HTTPS 访问正常
- [ ] HTTP 自动重定向到 HTTPS

### 3.3 病毒扫描（可选但推荐）

#### 部署 ClamAV
```bash
# Ubuntu/Debian
apt-get install clamav clamav-daemon

# CentOS/RHEL
yum install clamav clamav-scanner-systemd

# 更新病毒库
freshclam

# 启动服务
systemctl start clamav-daemon
systemctl enable clamav-daemon
```

- [ ] ClamAV 已安装
- [ ] 病毒库已更新
- [ ] 服务正常运行
- [ ] 监听端口 3310

---

## 四、部署验证

### 4.1 基础功能验证

#### 4.1.1 健康检查
```bash
curl https://your-domain.com/health
```
预期响应：`{"status":"ok"}`

- [ ] 健康检查通过

#### 4.1.2 登录功能
- [ ] 访问 `https://your-domain.com/` 自动跳转到登录页
- [ ] 管理员账号密码登录成功（admin / ADMIN_PASSWORD）
- [ ] 钉钉扫码登录正常
- [ ] 钉钉内免登正常（如在钉钉微应用内）

#### 4.1.3 核心功能验证

**组织管理**
- [ ] 同步钉钉组织架构成功
- [ ] 部门树显示正常
- [ ] 员工列表显示正常

**考勤管理**
- [ ] 同步钉钉考勤数据成功
- [ ] 考勤记录查询正常
- [ ] 考勤统计正常

**审批管理**
- [ ] 同步钉钉审批数据成功
- [ ] 审批实例显示正常

**绩效管理**
- [ ] 创建绩效活动成功
- [ ] 导入参与人成功
- [ ] 目标设定流程正常
- [ ] 自评流程正常
- [ ] 上级评分流程正常
- [ ] 结果确认流程正常

**权限管理**
- [ ] 角色管理正常
- [ ] 权限分配正常
- [ ] 菜单权限生效
- [ ] 数据权限生效

### 4.2 性能验证

#### 4.2.1 响应时间
- [ ] 首页加载时间 < 2 秒
- [ ] API 响应时间 < 500 毫秒（大部分请求）
- [ ] 列表查询响应时间 < 1 秒

#### 4.2.2 并发测试
使用 Apache Bench 进行简单压测：
```bash
# 100 并发，1000 请求
ab -n 1000 -c 100 https://your-domain.com/api/v1/org/overview
```

- [ ] 无请求失败
- [ ] 平均响应时间 < 1 秒
- [ ] 服务器负载正常

### 4.3 安全验证

#### 4.3.1 HTTPS 配置
使用 SSL Labs 测试：https://www.ssllabs.com/ssltest/

- [ ] SSL 评级 A 或 A+
- [ ] TLS 1.2+ 启用
- [ ] 不安全协议已禁用

#### 4.3.2 Cookie 安全
在浏览器开发者工具中检查 Cookie：

- [ ] `HttpOnly` 标志已设置
- [ ] `Secure` 标志已设置（HTTPS）
- [ ] `SameSite=Lax` 已设置

#### 4.3.3 文件上传安全
- [ ] 上传 `.exe` 文件被拒绝
- [ ] 上传 `.doc` 旧版 Office 文件被拒绝
- [ ] 上传正常 `.xlsx` 文件成功
- [ ] 上传超大文件被拒绝
- [ ] ClamAV 扫描正常（如已配置）

#### 4.3.4 权限验证
- [ ] 未登录访问 API 返回 401
- [ ] 无权限操作返回 403
- [ ] 跨组织数据访问被阻止

---

## 五、监控与告警

### 5.1 应用监控

#### 5.1.1 日志收集
- [ ] 应用日志输出到文件
- [ ] 日志轮转配置（logrotate）
- [ ] 日志级别设置为 INFO（生产环境）

#### 5.1.2 性能监控（可选）
- [ ] APM 工具部署（如 Prometheus + Grafana）
- [ ] 关键指标监控：
  - [ ] CPU 使用率
  - [ ] 内存使用率
  - [ ] 磁盘使用率
  - [ ] 网络流量
  - [ ] API 响应时间
  - [ ] 错误率

### 5.2 安全监控

#### 5.2.1 告警规则
- [ ] 登录失败告警（连续 5 次失败）
- [ ] 权限拒绝告警（异常访问）
- [ ] 文件上传告警（病毒检测）
- [ ] 服务异常告警（5xx 错误率 > 5%）
- [ ] 数据库慢查询告警（> 1 秒）

#### 5.2.2 日志分析
- [ ] 定期审查访问日志
- [ ] 定期审查错误日志
- [ ] 定期审查操作审计日志

### 5.3 备份策略

#### 5.3.1 数据库备份
```bash
# 每日全量备份
mysqldump -h mysql-host -u root -p peopleops > backup-$(date +%Y%m%d).sql

# 保留最近 7 天备份
```

- [ ] 数据库每日备份
- [ ] 备份文件自动清理（保留 7 天）
- [ ] 备份文件异地存储

#### 5.3.2 文件备份
- [ ] 上传文件目录备份
- [ ] 配置文件备份
- [ ] 应用代码备份

#### 5.3.3 恢复演练
- [ ] 每月进行一次恢复演练
- [ ] 验证备份可用性

---

## 六、运维文档

### 6.1 常用命令

#### 查看服务状态
```bash
# Docker Compose
docker compose -f docker-compose.prod.yml ps

# 查看日志
docker compose -f docker-compose.prod.yml logs -f --tail=100

# 重启服务
docker compose -f docker-compose.prod.yml restart peopleops-hr
```

#### 查看应用日志
```bash
# 实时查看
tail -f /var/log/peopleops/app.log

# 查看错误日志
grep -i error /var/log/peopleops/app.log | tail -50
```

#### 数据库操作
```bash
# 连接数据库
mysql -h mysql-host -u peopleops_app -p peopleops

# 查看表结构
SHOW TABLES;
DESCRIBE users;

# 查看慢查询
SHOW FULL PROCESSLIST;
```

### 6.2 故障排查

#### 服务无法启动
1. 检查日志：`docker compose logs`
2. 检查配置：`.env` 文件配置是否正确
3. 检查端口：`netstat -tunlp | grep 8080`
4. 检查数据库连接：`mysql -h mysql-host -u peopleops_app -p`

#### 钉钉登录失败
1. 检查钉钉应用配置
2. 检查回调地址是否可访问
3. 检查 AppKey、AppSecret、CorpID
4. 查看应用日志中的错误信息

#### 性能问题
1. 检查数据库慢查询：`SHOW FULL PROCESSLIST;`
2. 检查服务器资源：`top`, `htop`, `free -h`, `df -h`
3. 检查 Redis 连接
4. 查看应用日志中的慢请求

---

## 七、上线后任务

### 7.1 立即执行（上线后 24 小时内）

- [ ] 监控关键指标（CPU、内存、错误率）
- [ ] 检查所有核心功能是否正常
- [ ] 检查钉钉消息通知是否正常
- [ ] 检查用户登录是否正常
- [ ] 检查数据同步是否正常
- [ ] 处理上线后的用户反馈

### 7.2 短期任务（上线后 1 周内）

- [ ] 完成首次数据库备份验证
- [ ] 完成首次恢复演练
- [ ] 收集性能基线数据
- [ ] 优化慢查询（如有）
- [ ] 培训系统管理员

### 7.3 持续任务

#### 每日
- [ ] 检查服务状态
- [ ] 检查错误日志
- [ ] 检查告警通知

#### 每周
- [ ] 检查数据库备份
- [ ] 检查磁盘空间
- [ ] 查看性能趋势

#### 每月
- [ ] 更新依赖版本（go get -u、npm update）
- [ ] 执行恢复演练
- [ ] 安全审计（审查日志、权限）

#### 每季度
- [ ] 全面安全审计
- [ ] 性能优化
- [ ] 容量规划

---

## 八、回滚计划

### 8.1 回滚触发条件

- [ ] 服务无法启动
- [ ] 核心功能异常
- [ ] 数据丢失或损坏
- [ ] 严重性能问题
- [ ] 安全漏洞发现

### 8.2 回滚步骤

#### 8.2.1 应用回滚
```bash
# 停止当前服务
docker compose -f docker-compose.prod.yml down

# 切换到上一版本
git checkout <previous-version>

# 重新构建前端
cd frontend && npm run build && cd ..

# 重新启动服务
docker compose -f docker-compose.prod.yml up -d
```

#### 8.2.2 数据库回滚
```bash
# 恢复备份
mysql -h mysql-host -u root -p peopleops < backup-<date>.sql
```

### 8.3 回滚验证

- [ ] 服务启动成功
- [ ] 健康检查通过
- [ ] 核心功能正常
- [ ] 数据完整性检查通过

---

## 九、联系方式

### 9.1 技术支持

- **开发团队：** ________
- **运维团队：** ________
- **钉钉支持：** ________

### 9.2 紧急联系人

- **值班人员：** ________
- **联系电话：** ________
- **钉钉群：** ________

---

## 十、签署确认

### 部署前检查

- [ ] 所有配置项已检查完毕
- [ ] 安全配置已全部完成
- [ ] 测试验证已全部通过
- [ ] 备份恢复已测试通过
- [ ] 监控告警已配置完成

**检查人员：** ________  
**检查日期：** ________  
**签名：** ________

### 部署完成确认

- [ ] 服务部署成功
- [ ] 功能验证通过
- [ ] 性能验证通过
- [ ] 安全验证通过
- [ ] 监控正常运行

**部署人员：** ________  
**部署日期：** ________  
**签名：** ________

### 上线批准

- [ ] 技术负责人批准
- [ ] 运维负责人批准
- [ ] 业务负责人批准

**技术负责人：** ________  **日期：** ________  
**运维负责人：** ________  **日期：** ________  
**业务负责人：** ________  **日期：** ________

---

**文档版本：** 1.0  
**最后更新：** 2026年7月7日  
**下次审查：** 上线后 1 个月
