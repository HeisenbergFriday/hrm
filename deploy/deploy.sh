#!/bin/bash
# 钉钉登录修复 - 自动部署脚本
# 用法: bash deploy.sh

set -e  # 遇到错误立即退出

echo "========================================"
echo "钉钉登录修复 - 自动部署脚本"
echo "========================================"
echo ""

# 检查当前目录
EXPECTED_DIR="/home/ubuntu/peopleops-hr-test"
if [ "$PWD" != "$EXPECTED_DIR" ]; then
    echo "❌ 错误：请在 $EXPECTED_DIR 目录下执行此脚本"
    echo "执行: cd $EXPECTED_DIR"
    exit 1
fi

echo "✅ 当前目录正确: $PWD"
echo ""

# 1. 检查 Git 状态
echo "📋 步骤 1/6: 检查 Git 状态..."
git status
echo ""

# 2. 保存当前 commit（用于回滚）
CURRENT_COMMIT=$(git rev-parse HEAD)
echo "📌 当前 commit: $CURRENT_COMMIT"
echo "   (如需回滚，执行: git reset --hard $CURRENT_COMMIT)"
echo ""

# 3. 拉取最新代码
echo "📥 步骤 2/6: 拉取最新代码..."
git pull origin master
if [ $? -ne 0 ]; then
    echo "❌ 代码拉取失败，请检查网络或 Git 配置"
    exit 1
fi
echo "✅ 代码更新成功"
echo ""

# 4. 查看最新的几个 commit
echo "📜 最新的提交记录:"
git log --oneline -5
echo ""

# 5. 停止服务
echo "🛑 步骤 3/6: 停止现有服务..."
docker compose -p peopleops-hr-test -f docker-compose.test.yml down
echo "✅ 服务已停止"
echo ""

# 6. 重新构建镜像
echo "🔨 步骤 4/6: 重新构建镜像（包含新代码）..."
echo "   提示: 这可能需要几分钟时间..."
docker compose -p peopleops-hr-test -f docker-compose.test.yml build --no-cache
if [ $? -ne 0 ]; then
    echo "❌ 镜像构建失败"
    exit 1
fi
echo "✅ 镜像构建成功"
echo ""

# 7. 启动服务
echo "🚀 步骤 5/6: 启动服务..."
docker compose -p peopleops-hr-test -f docker-compose.test.yml up -d
if [ $? -ne 0 ]; then
    echo "❌ 服务启动失败"
    exit 1
fi
echo "✅ 服务已启动"
echo ""

# 8. 等待服务就绪
echo "⏳ 等待服务启动..."
sleep 5
echo ""

# 9. 检查容器状态
echo "📊 步骤 6/6: 检查容器状态..."
docker compose -p peopleops-hr-test -f docker-compose.test.yml ps
echo ""

# 10. 测试健康检查
echo "🏥 测试服务健康状态..."
HEALTH_CHECK=$(curl -s http://127.0.0.1:18080/api/v1/health)
if echo "$HEALTH_CHECK" | grep -q "ok"; then
    echo "✅ 服务健康检查通过"
else
    echo "⚠️  健康检查响应: $HEALTH_CHECK"
fi
echo ""

# 11. 完成
echo "========================================"
echo "✅ 部署完成！"
echo "========================================"
echo ""
echo "📋 接下来的步骤:"
echo ""
echo "1️⃣  查看实时日志（推荐新开一个终端窗口执行）:"
echo "   docker compose -p peopleops-hr-test -f docker-compose.test.yml logs -f | grep 'dingtalk/callback'"
echo ""
echo "2️⃣  测试登录:"
echo "   - 清除浏览器缓存和 Cookie"
echo "   - 访问登录页面"
echo "   - 选择钉钉扫码登录"
echo "   - 观察日志输出"
echo ""
echo "3️⃣  查看最近日志:"
echo "   docker compose -p peopleops-hr-test -f docker-compose.test.yml logs --tail=100"
echo ""
echo "4️⃣  如需回滚到之前版本:"
echo "   git reset --hard $CURRENT_COMMIT"
echo "   docker compose -p peopleops-hr-test -f docker-compose.test.yml down"
echo "   docker compose -p peopleops-hr-test -f docker-compose.test.yml build --no-cache"
echo "   docker compose -p peopleops-hr-test -f docker-compose.test.yml up -d"
echo ""
echo "========================================"
echo ""
