#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Upload an already-built PeopleOps HR test image tar and restart the test stack.

.DESCRIPTION
    Fast path for when local build already succeeded (e.g. build-and-deploy.ps1 failed
    at scp, or you want to re-push the same verified tar without rebuilding).

    This script does NOT build Go/frontend/Docker images.
    It does NOT modify or call deploy/build-and-deploy.ps1.

    Safety checks:
    - Local tar must exist
    - Optional max age warning / hard fail for stale tar
    - Delete incomplete remote tar before upload
    - Compare local vs remote byte size after scp
    - Health check on http://127.0.0.1:18080/health

.PARAMETER TarFile
    Local image tar path (default: peopleops-hr-test.tar in repo root).

.PARAMETER ServerHost
    SSH target (default: ubuntu@113.240.65.185).

.PARAMETER ServerPort
    SSH port (default: 16388).

.PARAMETER MaxAgeHours
    Warn (or fail with -FailOnStaleTar) when local tar is older than this many hours.
    Default: 24.

.PARAMETER FailOnStaleTar
    Exit if tar is older than MaxAgeHours (default: warn only).

.PARAMETER FullStack
    Use compose down + up -d for the whole test stack (same as build-and-deploy.ps1).
    Default recreates peopleops-hr and dingtalk-stream so MySQL/Redis stay up.

.PARAMETER SkipHealthCheck
    Skip /health polling after restart (not recommended).

.PARAMETER CleanupLocal
    After success, delete local tar and leftover Dockerfile.deploy / peopleops binary
    (same cleanup set as build-and-deploy.ps1). Default: keep local tar for retries.

.EXAMPLE
    # After build-and-deploy failed at upload:
    .\deploy\upload-and-restart.ps1

.EXAMPLE
    .\deploy\upload-and-restart.ps1 -FullStack -CleanupLocal

.EXAMPLE
    .\deploy\upload-and-restart.ps1 -TarFile .\peopleops-hr-test.tar -FailOnStaleTar
#>

param(
    [string]$ServerHost = "ubuntu@113.240.65.185",
    [int]$ServerPort = 16388,
    [string]$TarFile = "peopleops-hr-test.tar",
    [double]$MaxAgeHours = 24,
    [switch]$FailOnStaleTar,
    [switch]$FullStack,
    [switch]$SkipHealthCheck,
    [switch]$CleanupLocal
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

function Write-Err {
    param([string]$Message)
    Write-Host "  [ERROR] $Message" -ForegroundColor Red
}

function Write-Warn {
    param([string]$Message)
    Write-Host "  [WARN] $Message" -ForegroundColor Yellow
}

function Invoke-Remote {
    param([string]$Command)
    # Keep remote snippets single-line LF-safe (no PowerShell here-string CRLF).
    ssh -p $ServerPort `
        -o BatchMode=yes `
        -o ConnectTimeout=15 `
        -o ServerAliveInterval=30 `
        -o ServerAliveCountMax=10 `
        -o StrictHostKeyChecking=accept-new `
        $ServerHost $Command
    return $LASTEXITCODE
}

Write-Host ""
Write-Step "========================================"
Write-Step "  PeopleOps HR Upload & Restart"
Write-Step "  (no rebuild — existing tar only)"
Write-Step "========================================"
Write-Host ""
Write-Step "Target: ${ServerHost}:${ServerPort}" "Yellow"
Write-Host ""

$DEPLOY_DIR = "/home/ubuntu/peopleops-hr-test"
$COMPOSE_PROJECT = "peopleops-hr-test"
$COMPOSE_FILE = "docker-compose.test.yml"
$APP_SERVICE = "peopleops-hr"
$STREAM_SERVICE = "dingtalk-stream"
$REMOTE_TAR = "$DEPLOY_DIR/$(Split-Path -Leaf $TarFile)"

# --- Preflight ---
Write-Step "[1/6] Checking local tar..."
if (-not (Test-Path -LiteralPath $TarFile)) {
    Write-Err "Local tar not found: $TarFile"
    Write-Warn "This script only uploads an existing image. Build first with:"
    Write-Host "    .\deploy\build-and-deploy.ps1 -SkipConfigUpload" -ForegroundColor Cyan
    exit 1
}
if (-not (Test-Path -LiteralPath $COMPOSE_FILE)) {
    Write-Err "Local Compose file not found: $COMPOSE_FILE"
    exit 1
}

$tarItem = Get-Item -LiteralPath $TarFile
$localBytes = [int64]$tarItem.Length
if ($localBytes -le 0) {
    Write-Err "Local tar is empty: $TarFile"
    exit 1
}

$age = (Get-Date) - $tarItem.LastWriteTime
$ageHours = [math]::Round($age.TotalHours, 2)
$tarMb = [math]::Round($localBytes / 1MB, 2)
Write-Success "Found $TarFile ($tarMb MB, last write $($tarItem.LastWriteTime.ToString('yyyy-MM-dd HH:mm:ss')), age ${ageHours}h)"

if ($MaxAgeHours -gt 0 -and $age.TotalHours -gt $MaxAgeHours) {
    $msg = "Tar is older than ${MaxAgeHours}h — may not include latest code. Rebuild with build-and-deploy.ps1 if unsure."
    if ($FailOnStaleTar) {
        Write-Err $msg
        exit 1
    }
    Write-Warn $msg
}

# --- SSH ---
Write-Host ""
Write-Step "[2/6] Testing SSH..."
$sshCode = Invoke-Remote "echo OK"
if ($sshCode -ne 0) {
    Write-Err "SSH connection failed"
    Write-Host "  Try: ssh -p $ServerPort $ServerHost" -ForegroundColor Cyan
    exit 1
}
Write-Success "SSH OK"

# --- Clear incomplete remote tar and back up Compose ---
Write-Host ""
Write-Step "[3/6] Preparing remote path..."
$remoteComposeFile = "$DEPLOY_DIR/$COMPOSE_FILE"
$remoteComposeBackup = "$remoteComposeFile.bak.$(Get-Date -Format 'yyyyMMddHHmmss')"
$prepCode = Invoke-Remote "mkdir -p '$DEPLOY_DIR' && rm -f '$REMOTE_TAR' && if [ -f '$remoteComposeFile' ]; then cp '$remoteComposeFile' '$remoteComposeBackup'; fi && df -h '$DEPLOY_DIR' | tail -1"
if ($prepCode -ne 0) {
    Write-Err "Failed to prepare remote deploy directory"
    exit 1
}
Write-Success "Remote incomplete tar removed (if any)"

# --- Upload ---
Write-Host ""
Write-Step "[4/6] Uploading tar (compressed scp, keepalive)..."
Write-Host "  Local:  $((Resolve-Path -LiteralPath $TarFile).Path)" -ForegroundColor DarkGray
Write-Host "  Remote: ${ServerHost}:$REMOTE_TAR" -ForegroundColor DarkGray

scp -C `
    -o ServerAliveInterval=30 `
    -o ServerAliveCountMax=10 `
    -o ConnectTimeout=15 `
    -o StrictHostKeyChecking=accept-new `
    -P $ServerPort `
    $TarFile `
    "${ServerHost}:$REMOTE_TAR"
if ($LASTEXITCODE -ne 0) {
    Write-Err "Upload failed (connection reset / broken pipe is common for ~200MB)."
    Write-Warn "Local tar was NOT deleted. Re-run this script to retry upload only:"
    Write-Host "    .\deploy\upload-and-restart.ps1" -ForegroundColor Cyan
    # Best-effort cleanup of partial remote file
    Invoke-Remote "rm -f '$REMOTE_TAR'" | Out-Null
    exit 1
}
Write-Success "Upload finished"

Write-Step "Uploading current Compose definition..."
scp -P $ServerPort `
    -o BatchMode=yes `
    -o ConnectTimeout=15 `
    -o StrictHostKeyChecking=accept-new `
    $COMPOSE_FILE `
    "${ServerHost}:$remoteComposeFile"
if ($LASTEXITCODE -ne 0) {
    Write-Err "Compose upload failed"
    Invoke-Remote "if [ -f '$remoteComposeBackup' ]; then cp '$remoteComposeBackup' '$remoteComposeFile'; fi" | Out-Null
    exit 1
}
Write-Success "Compose definition uploaded"

# --- Size verify ---
Write-Host ""
Write-Step "[5/6] Verifying remote size..."
$remoteSizeOut = ssh -p $ServerPort `
    -o BatchMode=yes `
    -o ConnectTimeout=15 `
    -o StrictHostKeyChecking=accept-new `
    $ServerHost "stat -c %s '$REMOTE_TAR' 2>/dev/null"
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($remoteSizeOut)) {
    Write-Err "Could not read remote tar size"
    exit 1
}
$remoteBytes = 0L
if (-not [int64]::TryParse($remoteSizeOut.Trim(), [ref]$remoteBytes)) {
    Write-Err "Unexpected remote size output: $remoteSizeOut"
    exit 1
}
if ($remoteBytes -ne $localBytes) {
    Write-Err "Size mismatch: local=$localBytes bytes, remote=$remoteBytes bytes"
    Write-Warn "Remote tar is incomplete/corrupt; removing it. Re-run this script."
    Invoke-Remote "rm -f '$REMOTE_TAR'" | Out-Null
    exit 1
}
Write-Success "Size match ($localBytes bytes)"

# --- Load + restart ---
Write-Host ""
Write-Step "[6/6] Loading image and restarting..."
if ($FullStack) {
    Write-Host "  Mode: full stack (down + up -d)" -ForegroundColor DarkGray
    $deployCmd = "cd '$DEPLOY_DIR' && docker load -i '$REMOTE_TAR' && docker compose -p '$COMPOSE_PROJECT' -f '$COMPOSE_FILE' down && docker compose -p '$COMPOSE_PROJECT' -f '$COMPOSE_FILE' up -d && sleep 8 && docker compose -p '$COMPOSE_PROJECT' -f '$COMPOSE_FILE' ps"
} else {
    Write-Host "  Mode: app + Stream (force-recreate; MySQL/Redis stay up)" -ForegroundColor DarkGray
    $deployCmd = "cd '$DEPLOY_DIR' && docker load -i '$REMOTE_TAR' && docker compose -p '$COMPOSE_PROJECT' -f '$COMPOSE_FILE' up -d --force-recreate --no-deps '$APP_SERVICE' '$STREAM_SERVICE' && sleep 8 && docker compose -p '$COMPOSE_PROJECT' -f '$COMPOSE_FILE' ps"
}

$deployCode = Invoke-Remote $deployCmd
if ($deployCode -ne 0) {
    Write-Err "docker load / compose restart failed"
    exit 1
}
Write-Success "Image loaded and services updated"

# --- Health ---
if (-not $SkipHealthCheck) {
    Write-Host ""
    Write-Step "Checking health..."
    $healthCmd = "cd '$DEPLOY_DIR' && for attempt in 1 2 3 4 5 6 7 8 9 10 11 12; do printf '  health check attempt %s/12... ' `"`$attempt`"; if curl -fsS --connect-timeout 2 --max-time 5 http://127.0.0.1:18080/health >/dev/null 2>&1; then echo 'OK'; exit 0; fi; echo 'not ready'; sleep 5; done; echo 'health still failing; last app logs:'; docker compose -p '$COMPOSE_PROJECT' -f '$COMPOSE_FILE' logs --tail=80 '$APP_SERVICE'; exit 1"
    $healthCode = Invoke-Remote $healthCmd
    if ($healthCode -eq 0) {
        Write-Success "Health check OK"
    } else {
        Write-Err "Health check failed (containers may still be starting — check logs on server)"
        exit 1
    }
} else {
    Write-Warn "Skipped health check"
}

# --- Optional local cleanup (off by default so retries stay easy) ---
if ($CleanupLocal) {
    Write-Host ""
    Write-Step "Cleaning local build leftovers..."
    Remove-Item -LiteralPath $TarFile -Force -ErrorAction SilentlyContinue
    Remove-Item "Dockerfile.deploy" -Force -ErrorAction SilentlyContinue
    Remove-Item "Dockerfile.deploy.dockerignore" -Force -ErrorAction SilentlyContinue
    Remove-Item "peopleops" -Force -ErrorAction SilentlyContinue
    Remove-Item "dingtalk_stream" -Force -ErrorAction SilentlyContinue
    Write-Success "Local tar and deploy leftovers removed"
} else {
    Write-Host ""
    Write-Warn "Kept local tar for retries: $TarFile"
    Write-Host "  Pass -CleanupLocal to remove tar + Dockerfile.deploy + peopleops binary after success." -ForegroundColor DarkGray
}

Write-Host ""
Write-Step "========================================"
Write-Step "  Upload & Restart Complete!"
Write-Step "========================================"
Write-Host ""
Write-Step "Notes:" "Yellow"
Write-Host "  - This did NOT rebuild code. Tar age: ${ageHours}h." -ForegroundColor White
Write-Host "  - After code changes, use full build instead:" -ForegroundColor White
Write-Host "      .\deploy\build-and-deploy.ps1 -SkipConfigUpload" -ForegroundColor Cyan
Write-Host "  - Original build-and-deploy.ps1 is unchanged." -ForegroundColor White
Write-Host ""
