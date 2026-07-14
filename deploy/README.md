# PeopleOps Docker 部署说明

这份说明用于交付一套镜像：镜像里同时包含 Go 后端和前端静态文件。

## 你本地打包

在项目根目录执行：

```powershell
docker build -t peopleops:latest .
docker save peopleops:latest -o peopleops-latest.tar
```

然后把这些文件发给服务器人员：

- `peopleops-latest.tar`
- `docker-compose.prod.yml`
- `deploy/peopleops.env.example`
- `deploy/README.md`

## 服务器准备目录

服务器人员创建部署目录：

```bash
mkdir -p /opt/peopleops/deploy /opt/peopleops/uploads
cd /opt/peopleops

# 当前生产域名依赖宿主机 8080 对外可达；如未配置反向代理，不要改成本机回环绑定。
# 完成 Nginx/宝塔/负载均衡反向代理后，再将 8080 收敛到 127.0.0.1。
```

把交付文件放到 `/opt/peopleops` 后，复制一份真实环境变量文件：

```bash
cp deploy/peopleops.env.example deploy/peopleops.env
```

然后编辑 `deploy/peopleops.env`，把数据库、Redis、JWT、钉钉等占位值替换成生产配置。真实的 `peopleops.env` 不要提交，也不要对外发送。

## 服务器导入并启动

```bash
docker load -i peopleops-latest.tar
docker compose -f docker-compose.prod.yml up -d
```

检查运行状态：

```bash
docker ps
docker logs -f peopleops
curl http://127.0.0.1:8080/health
```

如果服务器没有 `docker compose`，也可以用：

```bash
docker run -d \
  --name peopleops \
  --restart unless-stopped \
  -p 8080:8080 \
  --env-file /opt/peopleops/deploy/peopleops.env \
  -v /opt/peopleops/uploads:/app/uploads \
  peopleops:latest
```

## Nginx / HTTPS

当前生产域名如果依赖外部反向代理、云负载均衡或 DNS 直接访问宿主机 `8080`，需要保持 `8080:8080`，否则 `http://hr.example.com/` 这类入口会中断。

```text
http://hr.example.com -> http://服务器公网 IP:8080
```

如果后续把 Nginx、宝塔或负载均衡部署到同一台服务器，再将端口收敛为 `127.0.0.1:8080:8080`，并反向代理到本机端口：

```text
https://hr.example.com -> http://127.0.0.1:8080
```

钉钉应用后台按当前实际可访问入口配置公网地址：

```text
应用首页: http://hr.example.com/
回调地址: http://hr.example.com/callback
```

生产环境不要填写 `localhost`，也不要填写本地开发端口 `3000`。如果必须暴露 `8080` 给外部代理访问，应优先通过云安全组/防火墙限制来源，只允许可信代理或办公出口访问。

## 后续更新

收到新的 `peopleops-latest.tar` 后：

```bash
docker compose -f docker-compose.prod.yml down
docker load -i peopleops-latest.tar
docker compose -f docker-compose.prod.yml up -d
```
