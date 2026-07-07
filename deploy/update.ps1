#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Update PeopleOps HR on the test server.

.DESCRIPTION
    Supports both deployment layouts:
    - Git mode: the remote deploy directory is a Git checkout.
    - TAR mode: the remote deploy directory stores docker-compose.test.yml and
      receives a locally built Docker image tar.

    The current test server uses TAR mode, so this script falls back to
    deploy/build-and-deploy.ps1 when no remote .git directory exists.
#>

param(
    [string]$ServerHost = "ubuntu@113.240.65.185",
    [int]$ServerPort = 16388,
    [string]$Branch = "",
    [string]$ConfigFile = "deploy/peopleops.test.env",
    [switch]$SkipConfigUpload
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

function Write-Warn {
    param([string]$Message)
    Write-Host "  [WARN] $Message" -ForegroundColor Yellow
}

function Get-CurrentBranch {
    $currentBranch = git branch --show-current 2>$null
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($currentBranch)) {
        return "master"
    }
    return $currentBranch.Trim()
}

function Invoke-Remote {
    param([string]$Command)
    ssh @SshOptions $ServerHost $Command
}

Write-Host ""
Write-Step "========================================"
Write-Step "  PeopleOps HR Deployment Script"
Write-Step "========================================"
Write-Host ""
Write-Step "Target: ${ServerHost}:${ServerPort}" "Yellow"
Write-Host ""

# Check SSH
$sshCmd = Get-Command ssh -ErrorAction SilentlyContinue
if (-not $sshCmd) {
    Write-Error "SSH command not found. Please install OpenSSH client."
    exit 1
}

# Config
$DEPLOY_DIR = "/home/ubuntu/peopleops-hr-test"
$COMPOSE_PROJECT = "peopleops-hr-test"
$COMPOSE_FILE = "docker-compose.test.yml"

if ([string]::IsNullOrWhiteSpace($Branch)) {
    $Branch = Get-CurrentBranch
}

if ($Branch -notmatch '^[A-Za-z0-9._/-]+$') {
    Write-Error "Invalid branch name: $Branch"
    exit 1
}

$SshOptions = @(
    "-p", "$ServerPort",
    "-o", "BatchMode=yes",
    "-o", "ConnectTimeout=10",
    "-o", "ConnectionAttempts=1",
    "-o", "ServerAliveInterval=5",
    "-o", "ServerAliveCountMax=2",
    "-o", "StrictHostKeyChecking=accept-new"
)

# Step 1: Test SSH
Write-Step "[1/6] Testing SSH connection..."
try {
    $sshOutput = ssh @SshOptions $ServerHost "echo OK" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw ($sshOutput -join "`n")
    }
    Write-Success "SSH connection OK"
} catch {
    Write-Error "SSH connection failed"
    if (-not [string]::IsNullOrWhiteSpace($_.Exception.Message)) {
        Write-Host "  $($_.Exception.Message)" -ForegroundColor Red
    }
    Write-Host "  Please check:" -ForegroundColor Yellow
    Write-Host "    1. Server address and port" -ForegroundColor Yellow
    Write-Host "    2. SSH key configured" -ForegroundColor Yellow
    Write-Host "    3. Server is accessible" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  Try manually: ssh -p $ServerPort $ServerHost" -ForegroundColor Cyan
    exit 1
}

# Step 2: Pick deployment mode
Write-Host ""
Write-Step "[2/6] Checking remote deployment layout..."
Invoke-Remote "test -d '$DEPLOY_DIR/.git'"
$hasRemoteGit = ($LASTEXITCODE -eq 0)

if (-not $hasRemoteGit) {
    Write-Warn "Remote deploy directory is not a Git repository: $DEPLOY_DIR"
    Write-Warn "Switching to TAR mode (local build, upload image, restart services)."
    Write-Host ""

    $buildAndDeployScript = Join-Path $PSScriptRoot "build-and-deploy.ps1"
    if (-not (Test-Path $buildAndDeployScript)) {
        Write-Error "Missing TAR deployment script: $buildAndDeployScript"
        exit 1
    }

    $deployArgs = @{
        ServerHost = $ServerHost
        ServerPort = $ServerPort
        ConfigFile = $ConfigFile
    }
    if ($SkipConfigUpload) {
        $deployArgs.SkipConfigUpload = $true
    }

    & $buildAndDeployScript @deployArgs
    exit $LASTEXITCODE
}

Write-Success "Remote Git repository found"

# Step 3: Pull code
Write-Host ""
Write-Step "[3/6] Pulling latest code..."
$pullCmd = "set -e; cd '$DEPLOY_DIR'; git fetch origin; git checkout '$Branch'; git pull --ff-only origin '$Branch'"
Invoke-Remote $pullCmd
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to pull code"
    exit 1
}
Write-Success "Code updated on branch $Branch"

# Step 4: Stop services
Write-Host ""
Write-Step "[4/6] Stopping services..."
$stopCmd = "cd '$DEPLOY_DIR' && docker compose -p '$COMPOSE_PROJECT' -f '$COMPOSE_FILE' down"
Invoke-Remote $stopCmd
if ($LASTEXITCODE -ne 0) {
    Write-Warn "Failed to stop (may not be running)"
} else {
    Write-Success "Services stopped"
}

# Step 5: Build image
Write-Host ""
Write-Step "[5/6] Building image..."
$buildCmd = "cd '$DEPLOY_DIR' && docker compose -p '$COMPOSE_PROJECT' -f '$COMPOSE_FILE' build --no-cache"
Invoke-Remote $buildCmd
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to build image"
    exit 1
}
Write-Success "Image built"

# Step 6: Start services and check status
Write-Host ""
Write-Step "[6/6] Starting services..."
$upCmd = "cd '$DEPLOY_DIR' && docker compose -p '$COMPOSE_PROJECT' -f '$COMPOSE_FILE' up -d"
Invoke-Remote $upCmd
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to start services"
    exit 1
}
Write-Success "Services started"

Write-Host ""
Write-Step "Checking status..."
Start-Sleep -Seconds 5
$psCmd = "cd '$DEPLOY_DIR' && docker compose -p '$COMPOSE_PROJECT' -f '$COMPOSE_FILE' ps"
Invoke-Remote $psCmd

# Done
Write-Host ""
Write-Step "========================================"
Write-Step "  Deployment Complete!"
Write-Step "========================================"
Write-Host ""

Write-Step "Next steps:" "Yellow"
Write-Host "  1. View logs:" -ForegroundColor White
Write-Host "     ssh -p $ServerPort $ServerHost" -ForegroundColor Cyan
Write-Host "     cd $DEPLOY_DIR" -ForegroundColor Cyan
Write-Host "     docker compose logs -f | grep dingtalk" -ForegroundColor Cyan
Write-Host ""
Write-Host "  2. Test DingTalk login and watch logs" -ForegroundColor White
Write-Host ""
