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

考勤工具箱钉钉同步等请求最长会执行 10 分钟。Nginx 默认约 60 秒会提前返回 `504 Gateway Time-out`，生产代理必须让网关等待时间长于后端、客户端等待时间再长于网关。推荐顺序：后端 600 秒 < Nginx 630 秒 < 前端 660 秒。

```nginx
location /api/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;

    proxy_connect_timeout 30s;
    proxy_send_timeout 630s;
    proxy_read_timeout 630s;
    send_timeout 630s;
}
```

修改后先执行 `nginx -t`，通过后再执行 `systemctl reload nginx`。如果域名前还有云负载均衡、CDN 或宝塔反向代理，也必须把对应的空闲超时调整到至少 630 秒，否则外层代理仍会先返回 504。

钉钉应用后台按当前实际可访问入口配置公网地址：

```text
应用首页: http://hr.example.com/
回调地址: http://hr.example.com/callback
```

生产环境不要填写 `localhost`，也不要填写本地开发端口 `3000`。如果必须暴露 `8080` 给外部代理访问，应优先通过云安全组/防火墙限制来源，只允许可信代理或办公出口访问。

## 钉钉 Stream 服务（dingtalk-stream）

镜像内同时包含两个二进制：主服务 `/app/peopleops` 和钉钉长连接 `/app/dingtalk_stream`。`docker-compose.prod.yml` 里已经定义了独立的 `dingtalk-stream` 服务，它和主服务共用同一镜像、同一 `deploy/peopleops.env`，因此数据库与钉钉配置保持一致，只把启动命令指向 `dingtalk_stream`。

启动两个容器：

```bash
docker compose -f docker-compose.prod.yml up -d
```

`dingtalk-stream` 通过环境变量 `DINGTALK_STREAM_ORG_ID` 显式绑定组织（默认 `default`）。单个 stream 进程只能绑定一个组织的应用凭证；若有多组织，需要为每个组织各起一个 `dingtalk-stream` 容器，并改用对应组织的 `DINGTALK_APP_KEY` / `DINGTALK_APP_SECRET`（或在 `DINGTALK_ORGANIZATIONS` JSON 中配置后由启动脚本逐组织启动）。

> 不要在 `docker-compose.prod.yml` 或本文档中写入 AppSecret、Token、SessionWebhook 等钉钉密钥；这些只放在不提交的 `deploy/peopleops.env` 中。

## 作息表群聊推送的临时图片存储限制

群聊推送会把当月作息表图片临时保存在 **主服务进程内存**（`/api/v1/week-schedule/group-image?token=...`），TTL 10 分钟，钉钉服务器通过公网 HTTPS 回拉该图片。这意味着：

- **当前仅支持单个主服务实例**。如果主服务部署多副本/多实例，钉钉回拉图片时可能命中没有该 token 的副本，返回 404，导致群消息图片缺失。
- 如需水平扩展，应迁移到对象存储（OSS/COS/MinIO）后再取消该限制。**在未与负责人确认前，不要自行引入对象存储依赖。**
- `dingtalk-stream` 容器不托管图片，仅负责审批事件增量同步与群聊 @机器人绑定，因此 stream 容器可以按需多开。

## 部署后检查清单

每次部署后按顺序确认：

1. **主服务健康**：`curl -k https://你的域名/health` 返回 ok；`docker ps` 中 `peopleops` 状态为 healthy / running。
2. **dingtalk-stream 持续运行**：`docker ps` 中 `peopleops-dingtalk-stream` 状态为 running；用 `docker inspect -f '{{.RestartCount}}' peopleops-dingtalk-stream` 确认重启次数为 0 或未异常增长（`restart: unless-stopped` 下频繁重启说明连接失败）。
3. **Stream 连接成功**：`docker logs --tail=50 peopleops-dingtalk-stream` 应出现 `钉钉 Stream 已连接`，且无 `连接失败` / `解析 Stream 所属组织失败` / `缺少 DINGTALK_APP_KEY` 等 Fatal。
4. **HTTPS 群图片路径外部可达**：在浏览器或外部主机访问 `https://你的域名/api/v1/week-schedule/group-image`（无 token 时应返回 404 而非 502/504/连接超时），确认反向代理已放行该路径。
5. **日志安全**：`docker logs peopleops` 与 `docker logs peopleops-dingtalk-stream` 不得出现图片 token、`SessionWebhook`、AppSecret、access_token 等钉钉密钥明文。

## 后续更新

收到新的 `peopleops-latest.tar` 后：

```bash
docker compose -f docker-compose.prod.yml down
docker load -i peopleops-latest.tar
docker compose -f docker-compose.prod.yml up -d
```
