#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Deploy PeopleOps HR to test server by uploading Docker image tar

.DESCRIPTION
    1. Build Docker image locally
    2. Export to tar file
    3. Upload to server via SCP
    4. Load image on server
    5. Restart services

.PARAMETER ServerHost
    Server address (default: ubuntu@113.240.65.185)

.PARAMETER ServerPort
    SSH port (default: 16388)

.EXAMPLE
    .\deploy\deploy-via-tar.ps1
#>

param(
    [string]$ServerHost = "ubuntu@113.240.65.185",
    [int]$ServerPort = 16388
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message, [string]$Color = "Cyan")
    Write-Host $Message -ForegroundColor $Color
}

function Write-Success {
    param([string]$Message)
    Write-Host "  [OK] $Message" -ForegroundColor Green
}

function Write-Error {
    param([string]$Message)
    Write-Host "  [ERROR] $Message" -ForegroundColor Red
}

Write-Host ""
Write-Step "========================================"
Write-Step "  PeopleOps HR Deployment (TAR Mode)"
Write-Step "========================================"
Write-Host ""
Write-Step "Target: ${ServerHost}:${ServerPort}" "Yellow"
Write-Host ""

# Config
$BUILD_DIR = "build/peopleops-hr-test-deploy"
$DEPLOY_DIR = "/home/ubuntu/peopleops-hr-test"
$COMPOSE_PROJECT = "peopleops-hr-test"
$COMPOSE_FILE = "docker-compose.test.yml"
$LOCAL_TAR = "peopleops-hr-test.tar"
$REMOTE_TAR = "$DEPLOY_DIR/$LOCAL_TAR"

# Check if docker-compose.test.yml exists
if (-not (Test-Path "$BUILD_DIR/$COMPOSE_FILE")) {
    Write-Error "$BUILD_DIR/$COMPOSE_FILE not found"
    exit 1
}

# Step 1: Build locally
Write-Step "[1/6] Building Docker image locally..."
docker compose -f "$BUILD_DIR/$COMPOSE_FILE" build --no-cache
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to build image"
    exit 1
}
Write-Success "Image built"

# Step 2: Export to tar
Write-Host ""
Write-Step "[2/6] Exporting image to tar..."
if (Test-Path $LOCAL_TAR) {
    Remove-Item $LOCAL_TAR -Force
}
docker save -o $LOCAL_TAR peopleops-hr-test-backend peopleops-hr-test-frontend
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to export image"
    exit 1
}
$tarSize = (Get-Item $LOCAL_TAR).Length / 1MB
Write-Success "Image exported ($([math]::Round($tarSize, 2)) MB)"

# Step 3: Upload to server
Write-Host ""
Write-Step "[3/6] Uploading to server..."
scp -P $ServerPort $LOCAL_TAR "${ServerHost}:${REMOTE_TAR}"
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to upload"
    exit 1
}
Write-Success "Uploaded"

# Step 4: Load image on server
Write-Host ""
Write-Step "[4/6] Loading image on server..."
ssh -p $ServerPort $ServerHost "docker load -i $REMOTE_TAR"
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to load image"
    exit 1
}
Write-Success "Image loaded"

# Step 5: Restart services
Write-Host ""
Write-Step "[5/6] Restarting services..."
$restartCmd = "cd $DEPLOY_DIR; docker compose -p $COMPOSE_PROJECT -f $COMPOSE_FILE down; docker compose -p $COMPOSE_PROJECT -f $COMPOSE_FILE up -d"
ssh -p $ServerPort $ServerHost $restartCmd
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to restart services"
    exit 1
}
Write-Success "Services restarted"

# Step 6: Check status
Write-Host ""
Write-Step "[6/6] Checking status..."
Start-Sleep -Seconds 5
ssh -p $ServerPort $ServerHost "cd $DEPLOY_DIR; docker compose -p $COMPOSE_PROJECT -f $COMPOSE_FILE ps"

# Cleanup
Write-Host ""
Write-Step "Cleaning up local tar file..."
Remove-Item $LOCAL_TAR -Force
Write-Success "Cleaned"

# Done
Write-Host ""
Write-Step "========================================"
Write-Step "  Deployment Complete!"
Write-Step "========================================"
Write-Host ""

Write-Step "Next steps:" "Yellow"
Write-Host "  1. View logs:" -ForegroundColor White
Write-Host "     ssh -p $ServerPort ${ServerHost}" -ForegroundColor Cyan
Write-Host "     cd $DEPLOY_DIR" -ForegroundColor Cyan
Write-Host "     docker compose logs -f | grep dingtalk" -ForegroundColor Cyan
Write-Host ""
Write-Host "  2. Test DingTalk login and watch logs" -ForegroundColor White
Write-Host ""
