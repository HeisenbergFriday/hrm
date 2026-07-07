#!/usr/bin/env bash
#
# 测试服一键部署脚本
# 用法: bash deploy/update.sh [服务器IP或域名]
#
# 前提:
#   1. 本脚本在项目根目录执行
#   2. 已配置 SSH 免密或能 ssh 到目标服务器
#   3. 服务器已预先放置 docker-compose.test.yml 与 deploy/peopleops.test.env
#
# 流程: 本地 docker build → docker save → ssh 传输至服务器 → 远端 docker load → 重启容器
#
# 强制使用 Windows 原生 OpenSSH, 避免 Git Bash 自带 ssh 找不到密钥
export PATH="/c/Windows/System32/OpenSSH:$PATH"

set -euo pipefail

# -------- 配置 --------
REMOTE_HOST="${1:-}"                      # ssh 目标, 例如 user@1.2.3.4
REMOTE_PORT="${2:-16388}"                 # SSH 端口 (服务器可能不是默认 22, 传 22 表示标准端口)
REMOTE_DIR="/home/ubuntu/peopleops-hr-test"
SKIP_HOST_CHECK="-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=/dev/null"
IMAGE_TAG="peopleops-hr:test"
LOCAL_TAR="peopleops-hr-test.tar"
COMPOSE_FILE="docker-compose.test.yml"
PROJECT_NAME="peopleops-hr-test"

# -------- 预检 --------
if [[ -z "${REMOTE_HOST}" ]]; then
  echo "[错误] 缺少服务器地址"
  echo "用法: bash deploy/update.sh [服务器IP或域名] [SSH端口(默认16388)]"
  echo "示例: bash deploy/update.sh ubuntu@1.2.3.4"
  echo "      bash deploy/update.sh ubuntu@1.2.3.4 22"
  exit 1
fi

cd "$(dirname "$0")/.."

echo "========================================"
echo "  PeopleOps HR 测试服一键部署"
echo "========================================"
echo "目标服务器 : ${REMOTE_HOST}"
echo "目标目录   : ${REMOTE_DIR}"
echo "目标端口   : ${REMOTE_PORT}"
echo ""

# -------- 第 1 步: 本地构建镜像 --------
echo "[1/5] 本地构建镜像 (docker build) ..."
if ! docker build -t "${IMAGE_TAG}" .; then
  echo "[失败] docker build 失败, 请本地修复构建错误后再重试"
  exit 1
fi
echo "[完成] 镜像构建成功"
echo ""

# -------- 第 2 步: 导出镜像 --------
echo "[2/5] 导出镜像到 ${LOCAL_TAR} ..."
docker save "${IMAGE_TAG}" -o "${LOCAL_TAR}"
echo "[完成] 导出完成 (大小: $(du -h "${LOCAL_TAR}" | cut -f1))"
echo ""

# -------- 第 3 步: 上传到服务器 (用 ssh+cat 代替 scp, 避免 Git Bash 密钥问题) --------
echo "[3/5] 上传镜像到服务器 ..."
if ! cat "${LOCAL_TAR}" | ssh -p "${REMOTE_PORT}" ${SKIP_HOST_CHECK} "${REMOTE_HOST}" "mkdir -p ${REMOTE_DIR} && cat > ${REMOTE_DIR}/${LOCAL_TAR}"; then
  echo "[失败] 上传失败, 请检查 SSH 连接和目标目录是否已创建"
  exit 1
fi
echo "[完成] 上传成功"
echo ""

# -------- 第 4 步: 远端加载镜像 + 重启 --------
echo "[4/5] 远端加载镜像并重启容器 ..."
ssh -p "${REMOTE_PORT}" ${SKIP_HOST_CHECK} "${REMOTE_HOST}" bash -euo pipefail <<REMOTE_SCRIPT
  set -euo pipefail

  cd "${REMOTE_DIR}"

  # 导入镜像
  docker load -i "${LOCAL_TAR}"

  # 启动 / 重启容器 (--force-recreate 强制使用新镜像)
  docker compose -p ${PROJECT_NAME} -f ${COMPOSE_FILE} up -d --force-recreate

  # 清理远端 tar
  rm -f "${LOCAL_TAR}"
REMOTE_SCRIPT

if [[ $? -ne 0 ]]; then
  echo "[失败] 远端加载/重启失败, 请手动 ssh 查看日志: docker logs peopleops-hr-test"
  exit 1
fi
echo "[完成] 容器已重启"
echo ""

# -------- 第 5 步: 本地清理 --------
echo "[5/5] 本地清理临时文件 ..."
rm -f "${LOCAL_TAR}"
echo "[完成] 已删除本地 ${LOCAL_TAR}"
echo ""

# -------- 健康检查 --------
echo "========================================"
echo "  部署完成"
echo "========================================"
echo ""
echo "健康检查命令 (可手动执行):"
echo "  ssh -p ${REMOTE_PORT} ${REMOTE_HOST} \"curl -s http://127.0.0.1:18080/health\""
echo ""
echo "查看日志:"
echo "  ssh -p ${REMOTE_PORT} ${REMOTE_HOST} \"docker logs -f --tail=50 peopleops-hr-test\""
echo ""
echo "浏览器访问:"
echo "  http://$(echo "${REMOTE_HOST}" | cut -d@ -f2):18080/"
