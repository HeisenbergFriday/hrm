#Requires -Version 5.1
<#
.SYNOPSIS
    PeopleOps HR test server one-click deploy script (PowerShell version).

.EXAMPLE
    .\deploy\update.ps1 ubuntu@1.2.3.4 16388
    .\deploy\update.ps1 ubuntu@1.2.3.4
#>

param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$RemoteHost,

    [Parameter(Position = 1)]
    [int]$RemotePort = 16388
)

$RemoteDir      = "/home/ubuntu/peopleops-hr-test"
$ImageTag       = "peopleops-hr:test"
$LocalTar       = "peopleops-hr-test.tar"
$ComposeFile    = "docker-compose.test.yml"
$ProjectName    = "peopleops-hr-test"

Set-Location (Split-Path -Parent $PSScriptRoot)

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  PeopleOps HR test deploy" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Target      : $RemoteHost"
Write-Host "SSH port    : $RemotePort"
Write-Host "Remote dir  : $RemoteDir"
Write-Host ""

# Step 1: build image locally
Write-Host "[1/5] Building image (docker build) ..." -ForegroundColor Yellow
docker build -t $ImageTag .
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAIL] docker build failed" -ForegroundColor Red
    exit 1
}
Write-Host "[OK] Image built" -ForegroundColor Green
Write-Host ""

# Step 2: export image
Write-Host "[2/5] Exporting image to $LocalTar ..." -ForegroundColor Yellow
docker save $ImageTag -o $LocalTar
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAIL] docker save failed" -ForegroundColor Red
    exit 1
}
$tarSize = [math]::Round((Get-Item $LocalTar).Length / 1MB, 1)
Write-Host "[OK] Exported ($tarSize MB)" -ForegroundColor Green
Write-Host ""

# Step 3: upload via scp (binary-safe)
Write-Host "[3/5] Uploading image to server ..." -ForegroundColor Yellow
$scpArgs = @(
    "-P", $RemotePort,
    "-o", "StrictHostKeyChecking=accept-new",
    $LocalTar,
    "${RemoteHost}:${RemoteDir}/"
)
scp @scpArgs
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAIL] Upload failed" -ForegroundColor Red
    exit 1
}
Write-Host "[OK] Uploaded" -ForegroundColor Green
Write-Host ""

# Step 4: load image and restart container on server
Write-Host "[4/5] Loading image and restarting container ..." -ForegroundColor Yellow
$remoteScript = @"
set -euo pipefail
cd $RemoteDir
docker load -i $LocalTar
docker compose -p $ProjectName -f $ComposeFile up -d --force-recreate
rm -f $LocalTar
"@
$sshArgs2 = @(
    "-p", $RemotePort,
    "-o", "StrictHostKeyChecking=accept-new",
    $RemoteHost,
    $remoteScript
)
ssh @sshArgs2
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAIL] Remote load/restart failed" -ForegroundColor Red
    exit 1
}
Write-Host "[OK] Container restarted" -ForegroundColor Green
Write-Host ""

# Step 5: local cleanup
Write-Host "[5/5] Cleaning up local files ..." -ForegroundColor Yellow
Remove-Item $LocalTar -Force
Write-Host "[OK] Removed local $LocalTar" -ForegroundColor Green
Write-Host ""

# Done
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Deploy complete" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Health check : ssh -p $RemotePort $RemoteHost `"curl -s http://127.0.0.1:18080/health`""
Write-Host "View logs    : ssh -p $RemotePort $RemoteHost `"docker logs -f --tail=50 peopleops-hr-test`""
$hostOnly = ($RemoteHost -split '@')[-1]
Write-Host "Browser      : http://${hostOnly}:18080/"
