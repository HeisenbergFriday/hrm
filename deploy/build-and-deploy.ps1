#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Build and deploy PeopleOps HR to test server

.DESCRIPTION
    Complete build and deployment process:
    1. Build Go backend
    2. Build React frontend
    3. Create Docker image
    4. Export to tar
    5. Upload to server
    6. Upload runtime config
    7. Load and restart

.EXAMPLE
    .\deploy\build-and-deploy.ps1
#>

param(
    [string]$ServerHost = "ubuntu@113.240.65.185",
    [int]$ServerPort = 16388,
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

function Read-EnvFile {
    param([string]$Path)

    $values = @{}
    foreach ($line in Get-Content -LiteralPath $Path -Encoding UTF8) {
        $trimmed = $line.Trim()
        if ($trimmed -eq "" -or $trimmed.StartsWith("#")) {
            continue
        }

        $equalsIndex = $trimmed.IndexOf("=")
        if ($equalsIndex -le 0) {
            continue
        }

        $key = $trimmed.Substring(0, $equalsIndex).Trim()
        $value = $trimmed.Substring($equalsIndex + 1).Trim()
        if (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        $values[$key] = $value
    }

    return $values
}

function Test-TestServerConfig {
    param([string]$Path)

    $values = Read-EnvFile -Path $Path
    $requiredKeys = @(
        "MYSQL_DATABASE",
        "MYSQL_USER",
        "MYSQL_PASSWORD",
        "MYSQL_ROOT_PASSWORD",
        "DATABASE_URL",
        "REDIS_URL",
        "JWT_SECRET",
        "PORT"
    )

    $missing = @()
    foreach ($key in $requiredKeys) {
        if (-not $values.ContainsKey($key) -or [string]::IsNullOrWhiteSpace($values[$key])) {
            $missing += $key
        }
    }
    if ($missing.Count -gt 0) {
        throw "Config missing required keys for test server: $($missing -join ', ')"
    }

    if ($values["DATABASE_URL"] -notmatch '@tcp\(mysql:3306\)') {
        throw "DATABASE_URL must point to mysql:3306 for this Docker Compose test deployment."
    }
    if ($values["REDIS_URL"].Trim() -ne "redis:6379") {
        throw "REDIS_URL must be redis:6379 for this Docker Compose test deployment."
    }
    if ($values["PORT"].Trim() -ne "8080") {
        throw "PORT must be 8080 because docker-compose.test.yml maps host port 18080 to container port 8080."
    }
    if ($values["JWT_SECRET"].Trim().Length -lt 32) {
        throw "JWT_SECRET must be at least 32 characters."
    }
}

Write-Host ""
Write-Step "========================================"
Write-Step "  PeopleOps HR Build & Deploy"
Write-Step "========================================"
Write-Host ""

$DEPLOY_DIR = "/home/ubuntu/peopleops-hr-test"
$REMOTE_CONFIG_DIR = "$DEPLOY_DIR/deploy"
$REMOTE_CONFIG_FILE = "$REMOTE_CONFIG_DIR/peopleops.test.env"

if (-not $SkipConfigUpload -and (Test-Path $ConfigFile)) {
    Write-Step "Prechecking runtime config..."
    try {
        Test-TestServerConfig -Path $ConfigFile
        Write-Success "Runtime config precheck OK: $ConfigFile"
    } catch {
        Write-Error $_.Exception.Message
        Write-Warn "Config was not uploaded. Fix $ConfigFile or rerun with -SkipConfigUpload to deploy code only."
        exit 1
    }
}

# Step 1: Build Go backend
Write-Step "[1/8] Building Go backend..."
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o peopleops ./cmd/main.go
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to build backend"
    exit 1
}
Write-Success "Backend built"

# Step 2: Build React frontend
Write-Host ""
Write-Step "[2/8] Building React frontend..."
Push-Location frontend
npm run build
if ($LASTEXITCODE -ne 0) {
    Pop-Location
    Write-Error "Failed to build frontend"
    exit 1
}
Pop-Location
Write-Success "Frontend built"

# Step 3: Create Dockerfile
Write-Host ""
Write-Step "[3/8] Creating Dockerfile..."
$dockerfile = @"
FROM python:3.12-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY peopleops /app/peopleops
COPY frontend/dist /app/frontend/dist
COPY internal/config/holidays.json /app/internal/config/holidays.json
COPY tools/attendance-processing /app/tools/attendance-processing
RUN python -m pip install --no-cache-dir -r /app/tools/attendance-processing/requirements.txt \
    && chmod +x /app/peopleops
EXPOSE 8080
CMD ["/app/peopleops"]
"@
Set-Content -Path "Dockerfile.deploy" -Value $dockerfile -Encoding UTF8

$dockerignore = @"
*
!peopleops
!frontend
!frontend/dist
!frontend/dist/**
!internal
!internal/config
!internal/config/holidays.json
!tools
!tools/attendance-processing
!tools/attendance-processing/**
!Dockerfile.deploy
"@
Set-Content -Path "Dockerfile.deploy.dockerignore" -Value $dockerignore -Encoding UTF8
Write-Success "Dockerfile created"
Write-Success "Docker build context limited to backend binary, frontend/dist, and runtime config"

# Step 4: Build Docker image
Write-Host ""
Write-Step "[4/8] Building Docker image..."
docker build --progress=plain -f Dockerfile.deploy -t peopleops-hr:test .
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to build Docker image"
    exit 1
}
Write-Success "Docker image built"

# Step 5: Export to tar
Write-Host ""
Write-Step "[5/8] Exporting image to tar..."
$tarFile = "peopleops-hr-test.tar"
if (Test-Path $tarFile) {
    Remove-Item $tarFile -Force
}
docker save -o $tarFile peopleops-hr:test
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to export image"
    exit 1
}
$tarSize = (Get-Item $tarFile).Length / 1MB
Write-Success "Image exported ($([math]::Round($tarSize, 2)) MB)"

# Step 6: Upload to server
Write-Host ""
Write-Step "[6/8] Uploading to server..."
scp -P $ServerPort $tarFile "${ServerHost}:/home/ubuntu/peopleops-hr-test/$tarFile"
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to upload"
    exit 1
}
Write-Success "Uploaded"

# Step 7: Upload runtime config
Write-Host ""
Write-Step "[7/8] Uploading runtime config..."
$configUploaded = $false
$remoteConfigBackup = ""
if ($SkipConfigUpload) {
    Write-Success "Skipped config upload"
} elseif (Test-Path $ConfigFile) {
    $remoteConfigBackup = "$REMOTE_CONFIG_FILE.bak.$(Get-Date -Format 'yyyyMMddHHmmss')"
    $remotePrepCmd = "mkdir -p '$REMOTE_CONFIG_DIR' && if [ -f '$REMOTE_CONFIG_FILE' ]; then cp '$REMOTE_CONFIG_FILE' '$remoteConfigBackup'; fi"
    ssh -p $ServerPort $ServerHost $remotePrepCmd
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to prepare remote config backup"
        exit 1
    }

    scp -P $ServerPort $ConfigFile "${ServerHost}:$REMOTE_CONFIG_FILE"
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to upload runtime config"
        exit 1
    }
    $configUploaded = $true
    Write-Success "Runtime config uploaded from $ConfigFile"
} else {
    Write-Warn "Config file not found, skipped: $ConfigFile"
}

# Step 8: Load and restart on server
Write-Host ""
Write-Step "[8/8] Deploying on server..."
$deployCmd = @"
cd $DEPLOY_DIR && \
docker load -i $tarFile && \
docker compose -p peopleops-hr-test -f docker-compose.test.yml down && \
docker compose -p peopleops-hr-test -f docker-compose.test.yml up -d && \
sleep 5 && \
docker compose -p peopleops-hr-test -f docker-compose.test.yml ps
"@
ssh -p $ServerPort $ServerHost $deployCmd
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to deploy"
    exit 1
}
Write-Success "Deployed"

Write-Host ""
Write-Step "Checking health..."
$healthCmd = "for i in 1 2 3 4 5 6; do curl -fsS --max-time 5 http://127.0.0.1:18080/health >/dev/null 2>&1 && exit 0; sleep 5; done; exit 1"
ssh -p $ServerPort $ServerHost $healthCmd
if ($LASTEXITCODE -eq 0) {
    Write-Success "Health check OK"
} elseif ($configUploaded -and $remoteConfigBackup -ne "") {
    Write-Warn "Health check failed after config upload; rolling back runtime config"
    $rollbackCmd = "cd $DEPLOY_DIR && cp '$remoteConfigBackup' '$REMOTE_CONFIG_FILE' && docker compose -p peopleops-hr-test -f docker-compose.test.yml up -d --force-recreate peopleops-hr"
    ssh -p $ServerPort $ServerHost $rollbackCmd
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Rollback failed"
        exit 1
    }

    Start-Sleep -Seconds 8
    ssh -p $ServerPort $ServerHost $healthCmd
    if ($LASTEXITCODE -eq 0) {
        Write-Error "New config failed health check. Rolled back to previous server config."
    } else {
        Write-Error "New config failed health check, and rollback health check also failed."
    }
    exit 1
} else {
    Write-Error "Health check failed"
    exit 1
}

# Cleanup
Write-Host ""
Write-Step "Cleaning up..."
Remove-Item $tarFile -Force -ErrorAction SilentlyContinue
Remove-Item "Dockerfile.deploy" -Force -ErrorAction SilentlyContinue
Remove-Item "Dockerfile.deploy.dockerignore" -Force -ErrorAction SilentlyContinue
Remove-Item "peopleops" -Force -ErrorAction SilentlyContinue
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
Write-Host "     cd /home/ubuntu/peopleops-hr-test" -ForegroundColor Cyan
Write-Host "     docker compose logs -f | grep dingtalk" -ForegroundColor Cyan
Write-Host ""
Write-Host "  2. Test DingTalk login" -ForegroundColor White
Write-Host ""
