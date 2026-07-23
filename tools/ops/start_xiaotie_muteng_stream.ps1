# Start Stream clients for xiaotie + muteng.
# Usage:
#   powershell -NoProfile -ExecutionPolicy Bypass -File tools\ops\start_xiaotie_muteng_stream.ps1
#
# Notes:
# - Does NOT print AppKey/AppSecret values (only masked prefixes/suffixes).
# - Does NOT touch an already-running default stream process.
# - Starts each org exe in its own visible console window so it stays alive.

$ErrorActionPreference = 'Stop'
$Root = 'D:\AITEAM\HR'
$Exe = Join-Path $Root 'dingtalk_stream.exe'
$EnvFile = Join-Path $Root '.env'

if (-not (Test-Path $Exe)) { throw "missing executable: $Exe" }
if (-not (Test-Path $EnvFile)) { throw "missing env file: $EnvFile" }

function Read-DotEnv([string]$path) {
    $raw = [System.IO.File]::ReadAllText($path).TrimStart([char]0xFEFF)
    $map = @{}
    foreach ($line in ($raw -split "`r?`n")) {
        $t = $line.Trim()
        if ($t -eq '' -or $t.StartsWith('#')) { continue }
        $i = $t.IndexOf('=')
        if ($i -lt 1) { continue }
        $k = $t.Substring(0, $i).Trim()
        $v = $t.Substring($i + 1)
        if (($v.StartsWith('"') -and $v.EndsWith('"')) -or ($v.StartsWith("'") -and $v.EndsWith("'"))) {
            $v = $v.Substring(1, $v.Length - 2)
        }
        $map[$k] = $v
    }
    return $map
}

function Mask-Key([string]$s) {
    if ([string]::IsNullOrWhiteSpace($s)) { return '<empty>' }
    if ($s.Length -le 8) { return ($s.Substring(0, 1) + '***') }
    return ($s.Substring(0, 4) + '***' + $s.Substring($s.Length - 2))
}

function Get-OrgCredentials {
    param($vars)

    $dsn = $vars['DATABASE_URL']
    if ([string]::IsNullOrWhiteSpace($dsn)) {
        throw 'DATABASE_URL missing in .env'
    }
    if ($dsn -notmatch '^(?<user>[^:]+):(?<pass>[^@]*)@tcp\((?<dbhost>[^:]+):(?<port>\d+)\)/(?<db>[^?]+)') {
        throw 'DATABASE_URL format not recognized'
    }
    $dbUser = $Matches['user']
    $dbPass = $Matches['pass']
    $dbHost = $Matches['dbhost']
    $dbPort = $Matches['port']
    $dbName = $Matches['db']

    # organizations 表无 deleted_at 列
    $sql = @"
SELECT org_id,
       COALESCE(NULLIF(ding_talk_app_key,''), app_key) AS app_key,
       COALESCE(NULLIF(ding_talk_secret,''), app_secret) AS app_secret,
       name
FROM organizations
WHERE status = 'active'
  AND org_id IN ('default','xiaotie','muteng')
ORDER BY FIELD(org_id,'default','xiaotie','muteng'), org_id
"@

    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = 'mysql'
    $psi.Arguments = "-h $dbHost -P $dbPort -u $dbUser -p$dbPass $dbName -N -B -e `"$sql`""
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $mysqlProc = New-Object System.Diagnostics.Process
    $mysqlProc.StartInfo = $psi
    [void]$mysqlProc.Start()
    $stdout = $mysqlProc.StandardOutput.ReadToEnd()
    $stderr = $mysqlProc.StandardError.ReadToEnd()
    $mysqlProc.WaitForExit()
    if ($mysqlProc.ExitCode -ne 0) {
        $stderr = $stderr -replace [regex]::Escape($dbPass), '***'
        throw "mysql failed: $stderr"
    }

    $orgs = @()
    foreach ($line in ($stdout -split "`r?`n")) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split "`t"
        if ($parts.Count -lt 3) { continue }
        $orgs += [pscustomobject]@{
            OrgID     = $parts[0]
            AppKey    = $parts[1]
            AppSecret = $parts[2]
            Name      = $(if ($parts.Count -ge 4) { $parts[3] } else { '' })
        }
    }
    return $orgs
}

function Start-OrgStreamDirect {
    param(
        [string]$OrgID,
        [string]$AppKey,
        [string]$AppSecret,
        [string]$DatabaseUrl
    )

    $procName = "dingtalk_stream_$OrgID"
    $existing = Get-Process -Name $procName -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Host ("already_running org={0} pid={1}" -f $OrgID, $existing[0].Id)
        return $existing[0]
    }

    $orgExe = Join-Path $Root "$procName.exe"
    Copy-Item $Exe $orgExe -Force

    $outLog = Join-Path $Root "dingtalk_stream_$OrgID.stdout.log"
    $errLog = Join-Path $Root "dingtalk_stream_$OrgID.stderr.log"
    Set-Content -Path $outLog -Value '' -Encoding UTF8
    Set-Content -Path $errLog -Value '' -Encoding UTF8

    # Launch via cmd.exe so env vars are set for the child process tree,
    # and keep a visible window. Redirect logs with cmd >> operators.
    $arg = "/k set DINGTALK_APP_KEY=$AppKey&& set DINGTALK_APP_SECRET=$AppSecret&& set DINGTALK_STREAM_ORG_ID=$OrgID&& set DATABASE_URL=$DatabaseUrl&& title Stream-$OrgID&& `"$orgExe`" 1>>`"$outLog`" 2>>`"$errLog`""
    $proc = Start-Process -FilePath 'cmd.exe' -ArgumentList $arg -WorkingDirectory $Root -PassThru -WindowStyle Normal
    Write-Host ("started org={0} cmd_pid={1} exe={2}" -f $OrgID, $proc.Id, $orgExe)
    return $proc
}

$vars = Read-DotEnv $EnvFile
$allOrgs = Get-OrgCredentials -vars $vars
$targets = $allOrgs | Where-Object { $_.OrgID -in @('xiaotie', 'muteng') }
if (-not $targets -or $targets.Count -eq 0) {
    throw 'no xiaotie/muteng credentials found in organizations'
}

Write-Host '=== credential fingerprint (compare with DingTalk console AppKey) ==='
foreach ($o in $allOrgs) {
    Write-Host ("org={0} name={1} key={2} secret_set={3}" -f $o.OrgID, $o.Name, (Mask-Key $o.AppKey), (-not [string]::IsNullOrWhiteSpace($o.AppSecret)))
}
Write-Host 'If console AppKey first4/last2 does not match key= above, verification will always fail.'
Write-Host ''

$default = Get-Process -Name 'dingtalk_stream' -ErrorAction SilentlyContinue
if ($default) {
    Write-Host ("default_stream_alive pid={0}" -f $default[0].Id)
} else {
    Write-Host 'default_stream_not_found (optional; start separately if needed)'
}

Write-Host ''
Write-Host '=== starting xiaotie / muteng ==='
foreach ($o in $targets) {
    if ([string]::IsNullOrWhiteSpace($o.AppKey) -or [string]::IsNullOrWhiteSpace($o.AppSecret)) {
        Write-Host ("skip org={0} reason=missing_credentials" -f $o.OrgID)
        continue
    }
    [void](Start-OrgStreamDirect -OrgID $o.OrgID -AppKey $o.AppKey -AppSecret $o.AppSecret -DatabaseUrl $vars['DATABASE_URL'])
}

Write-Host 'waiting for connect...'
Start-Sleep -Seconds 15

Write-Host ''
Write-Host '--- process list ---'
Get-Process | Where-Object { $_.ProcessName -like 'dingtalk_stream*' } |
    Select-Object Id, ProcessName, StartTime |
    Format-Table -AutoSize

Write-Host '--- readiness ---'
foreach ($orgId in @('xiaotie', 'muteng')) {
    $errLog = Join-Path $Root "dingtalk_stream_$orgId.stderr.log"
    $outLog = Join-Path $Root "dingtalk_stream_$orgId.stdout.log"
    $err = if (Test-Path $errLog) { Get-Content $errLog -Raw -ErrorAction SilentlyContinue } else { '' }
    $out = if (Test-Path $outLog) { Get-Content $outLog -Raw -ErrorAction SilentlyContinue } else { '' }
    $all = "$err`n$out"
    $ready = ($all -match 'connect success' -or $all -match '审批事件将增量同步' -or $all -match '钉钉 Stream 已连接')
    $failed = ($all -match '连接失败' -or $all -match '初始化数据库失败' -or $all -match '解析 Stream 所属组织失败' -or $all -match '缺少 DINGTALK' -or $all -match 'Fatal')
    $resolved = if ($all -match 'org_id=([A-Za-z0-9_\-]+)') { $Matches[1] } else { '' }
    $exeAlive = Get-Process -Name "dingtalk_stream_$orgId" -ErrorAction SilentlyContinue
    Write-Host ("org={0} ready={1} failed={2} resolved_org={3} exe_alive={4}{5}" -f `
        $orgId, $ready, $failed, $resolved, [bool]$exeAlive, $(if ($exeAlive) { ' pid=' + $exeAlive[0].Id } else { '' }))
    if (-not $ready) {
        Write-Host '  stderr tail:'
        if (Test-Path $errLog) {
            Get-Content $errLog -Tail 20 | ForEach-Object { Write-Host ("    " + $_) }
        } else {
            Write-Host '    <missing log>'
        }
    }
}

Write-Host ''
Write-Host 'NEXT STEPS:'
Write-Host '1) Keep the Stream-xiaotie / Stream-muteng console windows open.'
Write-Host '2) In DingTalk console, open each app Credentials page and compare AppKey with key= fingerprint above.'
Write-Host '3) Only after ready=True, click "验证连接通道" in THAT same app.'
Write-Host '4) Then subscribe bpms_instance_change + bpms_task_change and publish a new version.'
