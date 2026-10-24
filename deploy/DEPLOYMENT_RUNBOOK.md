# PeopleOps HR 发布必读手册

这份手册是测试服 `hr-platform.motern.com` 的固定发布入口。涉及构建、上传、重启、回滚或线上验证时，先读本文，再执行命令。

## 目标与固定信息

- 公网地址：`https://hr-platform.motern.com/`
- SSH：`ubuntu@113.240.65.185`，端口 `16388`
- 远程目录：`/home/ubuntu/peopleops-hr-test`
- Compose 项目：`peopleops-hr-test`
- Compose 文件：`docker-compose.test.yml`
- 应用服务：`peopleops-hr`
- Stream 服务：`dingtalk-stream`
- 应用容器：`peopleops-hr-test`
- Stream 容器：`peopleops-hr-test-dingtalk-stream`
- 数据库容器：`peopleops-hr-test-mysql`
- Redis 容器：`peopleops-hr-test-redis`
- 宿主机端口：`18080`；容器端口：`8080`

## 凭据规则

当前工作区的部署密钥候选文件为：

```text
.tmp/deploy_key_copy
.tmp/deploy_key
.tmp/deploy_id_ed25519
```

优先使用 Git for Windows 自带 SSH 客户端：

```powershell
$ssh = 'D:\Git\usr\bin\ssh.exe'
$scp = 'D:\Git\usr\bin\scp.exe'
$key = (Resolve-Path .tmp\deploy_key_copy)
```

Windows 系统自带 OpenSSH 可能因临时密钥 ACL 报 `Permission denied`；遇到该情况不要修改密钥内容，改用上面的 Git SSH 客户端。不得读取、打印、提交或复制私钥内容；密钥文件仅用于 `ssh` / `scp` 身份验证。

## 发布前检查

```powershell
git status --short --branch
git log -1 --oneline
git diff --check
cd frontend
npm run lint
npm run build
npm run test -- --run src/pages/AttendanceToolbox.test.tsx
cd ..
```

只提交本次发布范围内的文件。已有用户未提交改动不得被覆盖或顺手带入发布包。

## 首选发布方式

本机已安装 Go、Docker、Node 时，使用项目脚本：

```powershell
.\deploy\build-and-deploy.ps1 -SkipConfigUpload
```

该脚本会构建 Go 后端、前端和 Docker 镜像，上传到测试服务器，重建应用与 Stream，并执行健康检查。配置未变更时使用 `-SkipConfigUpload`，避免覆盖服务器现有环境文件。

## 本机没有 Docker 时的发布方式

可以使用工作区已有的便携版工具：

```text
.tmp/go-portable/go/bin/go.exe
.tmp/uv-python/cpython-3.12.15-windows-x86_64-none/python.exe
```

流程固定为：

1. 用便携版 Go 构建 Linux `peopleops`、`dingtalk_stream`、`sync_org_data`。
2. 用 `frontend/dist` 和 `tools/attendance_toolbox` 打包发布 tar.gz。
3. 用 `$scp -i $key -P 16388` 上传到远程目录。
4. 远程 `docker build` 生成带提交号的镜像，例如 `peopleops-hr:<commit>-YYYYMMDD`。
5. 先把当前 `peopleops-hr:test` 标记为 `peopleops-hr:rollback-<commit>-pre`。
6. 使用环境变量指定新镜像，只重建 `peopleops-hr dingtalk-stream`，不要重建 MySQL/Redis：

```bash
PEOPLEOPS_TEST_IMAGE=peopleops-hr:<commit>-YYYYMMDD \
docker compose -p peopleops-hr-test -f docker-compose.test.yml \
  up -d --force-recreate --no-deps peopleops-hr dingtalk-stream
```

7. 健康检查失败时立即使用回滚镜像重新执行同一条 Compose 命令。

## 发布后检查

远程服务器检查：

```bash
cd /home/ubuntu/peopleops-hr-test
docker compose -p peopleops-hr-test -f docker-compose.test.yml ps
curl -fsS http://127.0.0.1:18080/health
docker inspect -f '{{.Name}} restart={{.RestartCount}} health={{.State.Health.Status}} image={{.Image}}' \
  peopleops-hr-test peopleops-hr-test-dingtalk-stream peopleops-hr-test-mysql peopleops-hr-test-redis
docker logs --tail=50 peopleops-hr-test-dingtalk-stream
```

公网检查：

```powershell
curl.exe -k -sS https://hr-platform.motern.com/health
curl.exe -k -I https://hr-platform.motern.com/
```

本次考勤工具箱改造还应确认：

- `/api/v1/attendance/toolbox/rules/preview` 未登录时返回 401，而不是 404；
- 公网 `AttendanceToolbox` 脚本哈希与本地 `frontend/dist/assets` 对应文件一致；
- 应用和 Stream 为 `healthy`，重启次数为 0；
- MySQL、Redis 容器未被重建。

## 发布记录

每次发布在最终总结中记录：提交号、分支、镜像标签、回滚标签、健康结果、重启次数、数据库/Redis 是否重建，以及未执行的验证项。不要把真实环境变量、密码、AppSecret、Token 或私钥写入本文档。
