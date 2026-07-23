# Ephemeral MySQL for org_id composite unique drill only.
# Safe to destroy: container name peopleops-org-unique-drill, DB peopleops_org_drill.
$ErrorActionPreference = "Stop"

$name = "peopleops-org-unique-drill"
$existing = docker ps -a --filter "name=^/${name}$" --format "{{.Names}}"
if ($existing) {
  Write-Host "Removing previous drill container..."
  docker rm -f $name | Out-Null
}

Write-Host "Starting ephemeral MySQL 5.7 drill container..."
docker run -d --name $name `
  -e MYSQL_ROOT_PASSWORD=drill_only_local `
  -e MYSQL_DATABASE=peopleops_org_drill `
  -e MYSQL_USER=drill `
  -e MYSQL_PASSWORD=drill_only_local `
  -p 13306:3306 `
  mysql:5.7 `
  --character-set-server=utf8mb4 `
  --collation-server=utf8mb4_unicode_ci `
  --default-time-zone=+08:00

Write-Host "Waiting for MySQL readiness..."
$ready = $false
for ($i = 1; $i -le 60; $i++) {
  Start-Sleep -Seconds 2
  $out = docker exec $name mysqladmin ping -h 127.0.0.1 -uroot -pdrill_only_local --silent 2>$null
  if ($LASTEXITCODE -eq 0) {
    $ready = $true
    break
  }
  Write-Host "  attempt $i ..."
}
if (-not $ready) {
  Write-Error "MySQL drill container failed to become ready"
  exit 1
}

docker exec $name mysql -uroot -pdrill_only_local -e "SELECT VERSION() AS version; SELECT DATABASE(); SHOW DATABASES;" 2>$null
Write-Host "READY name=$name host=127.0.0.1 port=13306 db=peopleops_org_drill user=drill"
Write-Host "DSN(user)=drill:drill_only_local@tcp(127.0.0.1:13306)/peopleops_org_drill?charset=utf8mb4&parseTime=True&loc=Local"
