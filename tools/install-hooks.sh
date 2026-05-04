#!/usr/bin/env bash
# 将 tools/hooks/pre-commit 安装到 .git/hooks/
# 用法：bash tools/install-hooks.sh

REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
if [ -z "$REPO_ROOT" ]; then
  echo "错误：请在 git 仓库根目录下运行此脚本"
  exit 1
fi

SRC="$REPO_ROOT/tools/hooks/pre-commit"
DEST="$REPO_ROOT/.git/hooks/pre-commit"

cp "$SRC" "$DEST"
chmod +x "$DEST"
echo "已安装 pre-commit hook -> $DEST"
