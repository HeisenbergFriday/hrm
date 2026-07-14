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
APP_BASE_URL=http://服务器IP或域名:18080
FRONTEND_BASE_URL=http://服务器IP或域名:18080
CORS_ALLOW_ORIGINS=http://服务器IP或域名:18080
```

真实环境变量文件不要提交，也不要和其他 app 共用。测试栈会启动自己的 MySQL 和 Redis，不依赖服务器上其他 app 的数据库或缓存。

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

收到新的 `peopleops-hr-test.tar` 后：

```bash
cd /opt/peopleops-hr-test
docker compose -p peopleops-hr-test -f docker-compose.test.yml up -d
```

如果镜像已经导入过旧版本，先重新导入：

```bash
docker load -i peopleops-hr-test.tar
docker compose -p peopleops-hr-test -f docker-compose.test.yml up -d --force-recreate
```
