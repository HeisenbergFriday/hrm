#!/usr/bin/env pwsh
<#
.SYNOPSIS
    多企业支持快速设置脚本

.DESCRIPTION
    自动化多租户迁移流程：
    1. 备份数据库
    2. 运行数据库迁移
    3. 重新构建和部署应用

.EXAMPLE
    .\setup-multitenant.ps1
#>

param(
    [string]$ServerHost = "ubuntu@113.240.65.185",
    [int]$ServerPort = 16388,
    [string]$DBHost = "peopleops-hr-test-mysql",
    [string]$DBUser = "root",
    [string]$DBPassword = "root123",
    [string]$DBName = "peopleops_test",
    [string]$DefaultOrgID = "xiaotie"
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message, [string]$Color = "Cyan")
    Write-Host ""
    Write-Host "======================================" -ForegroundColor $Color
    Write-Host "  $Message" -ForegroundColor $Color
    Write-Host "======================================" -ForegroundColor $Color
}

function Write-Success {
    param([string]$Message)
    Write-Host "  [OK] $Message" -ForegroundColor Green
}

function Write-Error {
    param([string]$Message)
    Write-Host "  [ERROR] $Message" -ForegroundColor Red
}

function Write-Warning {
    param([string]$Message)
    Write-Host "  [WARN] $Message" -ForegroundColor Yellow
}

Write-Host ""
Write-Step "多企业支持设置向导" "Green"
Write-Host ""

# 确认操作
Write-Warning "此脚本将对数据库和应用进行重大修改"
Write-Warning "请确保已经："
Write-Host "  1. 阅读了 deploy/多企业支持实施指南.md"
Write-Host "  2. 配置了 deploy/peopleops.env 中的 DINGTALK_ORGANIZATIONS"
Write-Host "  3. 系统当前处于可维护状态（非业务高峰期）"
Write-Host ""

$confirm = Read-Host "是否继续？(yes/no)"
if ($confirm -ne "yes") {
    Write-Host "操作已取消" -ForegroundColor Yellow
    exit 0
}

# 步骤 1：备份数据库
Write-Step "[1/5] 备份数据库"
$backupFile = "backup_before_multitenant_$(Get-Date -Format 'yyyyMMdd_HHmmss').sql"
$backupCmd = @"
cd /home/ubuntu/peopleops-hr-test && \
docker compose -p peopleops-hr-test -f docker-compose.test.yml exec -T peopleops-hr-test-mysql \
mysqldump -u $DBUser -p$DBPassword $DBName > /tmp/$backupFile && \
docker cp peopleops-hr-test-mysql:/tmp/$backupFile ./$backupFile
"@

Write-Host "  正在备份数据库到服务器..."
ssh -p $ServerPort $ServerHost $backupCmd
if ($LASTEXITCODE -ne 0) {
    Write-Error "数据库备份失败"
    exit 1
}
Write-Success "数据库已备份: $backupFile"

# 步骤 2：构建迁移工具
Write-Step "[2/5] 构建数据库迁移工具"
Push-Location tools/migrate_multitenant
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o migrate_multitenant main.go
if ($LASTEXITCODE -ne 0) {
    Pop-Location
    Write-Error "迁移工具构建失败"
    exit 1
}
Pop-Location
Write-Success "迁移工具已构建"

# 步骤 3：上传并运行迁移
Write-Step "[3/5] 运行数据库迁移"
Write-Host "  上传迁移工具到服务器..."
scp -P $ServerPort tools/migrate_multitenant/migrate_multitenant "${ServerHost}:/home/ubuntu/peopleops-hr-test/"
if ($LASTEXITCODE -ne 0) {
    Write-Error "上传迁移工具失败"
    exit 1
}

Write-Host "  执行数据库迁移..."
$migrateCmd = @"
cd /home/ubuntu/peopleops-hr-test && \
chmod +x migrate_multitenant && \
docker compose -p peopleops-hr-test -f docker-compose.test.yml exec -T peopleops-hr \
sh -c 'DATABASE_DSN=\"$DBUser:$DBPassword@tcp($DBHost:3306)/$DBName?charset=utf8mb4&parseTime=True&loc=Local\" \
DEFAULT_ORG_ID=\"$DefaultOrgID\" \
DINGTALK_CORP_ID=\"\$DINGTALK_CORP_ID\" \
DINGTALK_APP_KEY=\"\$DINGTALK_APP_KEY\" \
DINGTALK_APP_SECRET=\"\$DINGTALK_APP_SECRET\" \
DINGTALK_AGENT_ID=\"\$DINGTALK_AGENT_ID\" \
/app/migrate_multitenant'
"@

ssh -p $ServerPort $ServerHost $migrateCmd
if ($LASTEXITCODE -ne 0) {
    Write-Error "数据库迁移失败"
    Write-Host ""
    Write-Warning "可以使用以下命令回滚："
    Write-Host "  ssh -p $ServerPort ${ServerHost}" -ForegroundColor Cyan
    Write-Host "  cd /home/ubuntu/peopleops-hr-test" -ForegroundColor Cyan
    Write-Host "  docker compose -p peopleops-hr-test -f docker-compose.test.yml exec -T peopleops-hr-test-mysql \\" -ForegroundColor Cyan
    Write-Host "    mysql -u $DBUser -p$DBPassword $DBName < $backupFile" -ForegroundColor Cyan
    exit 1
}
Write-Success "数据库迁移完成"

# 步骤 4：重新构建和部署应用
Write-Step "[4/5] 重新构建和部署应用"
Write-Host "  开始构建..."
.\deploy\build-and-deploy.ps1
if ($LASTEXITCODE -ne 0) {
    Write-Error "应用部署失败"
    exit 1
}
Write-Success "应用已重新部署"

# 步骤 5：验证部署
Write-Step "[5/5] 验证部署"
Write-Host "  检查服务状态..."
Start-Sleep -Seconds 5
$statusCmd = "cd /home/ubuntu/peopleops-hr-test && docker compose -p peopleops-hr-test -f docker-compose.test.yml ps"
ssh -p $ServerPort $ServerHost $statusCmd

Write-Host ""
Write-Host "  检查日志..."
$logCmd = "cd /home/ubuntu/peopleops-hr-test && docker compose -p peopleops-hr-test -f docker-compose.test.yml logs --tail=20 | grep -E '(组织|org_id|organization)'"
ssh -p $ServerPort $ServerHost $logCmd

# 完成
Write-Host ""
Write-Step "多企业支持设置完成！" "Green"
Write-Host ""

Write-Host "下一步操作：" -ForegroundColor Yellow
Write-Host "  1. 测试登录（小铁企业）"
Write-Host "     http://hr-platform.motern.com/?org_id=xiaotie"
Write-Host ""
Write-Host "  2. 查看组织列表"
Write-Host "     ssh -p $ServerPort ${ServerHost}"
Write-Host "     docker compose -p peopleops-hr-test -f docker-compose.test.yml exec peopleops-hr-test-mysql \\"
Write-Host "       mysql -u $DBUser -p$DBPassword $DBName -e 'SELECT * FROM organizations;'"
Write-Host ""
Write-Host "  3. 如需添加第二个企业，请参考："
Write-Host "     deploy/多企业支持实施指南.md"
Write-Host ""

Write-Host "备份文件位置：" -ForegroundColor Cyan
Write-Host "  服务器: /home/ubuntu/peopleops-hr-test/$backupFile"
Write-Host ""
