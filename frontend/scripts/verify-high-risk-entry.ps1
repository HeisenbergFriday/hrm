# 验证高风险入口修复：单元测试 + 相关 e2e
# 用法：在 frontend 目录执行
#   powershell -File scripts/verify-high-risk-entry.ps1

$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot\..

Write-Host '== 1/2 Unit tests (RouteGuard / Home empty / 上期结果) ==' -ForegroundColor Cyan
npx vitest run --config vite.config.test.ts `
  src/components/RouteGuard.test.tsx `
  src/pages/Home.emptyPermission.test.tsx `
  src/pages/PerformanceOverview.interaction.test.tsx `
  --reporter=default 2>$null
if ($LASTEXITCODE -ne 0) {
  Write-Host "UNIT FAILED exit=$LASTEXITCODE" -ForegroundColor Red
  exit $LASTEXITCODE
}
Write-Host 'UNIT OK' -ForegroundColor Green

Write-Host '== 2/2 Playwright e2e (3 high-risk scenarios only) ==' -ForegroundColor Cyan
npx playwright test tests/e2e/performance.spec.ts --project=chromium `
  -g "empty menuKeys|deep-link self-eval|previous-result navigates"
if ($LASTEXITCODE -ne 0) {
  Write-Host "E2E FAILED exit=$LASTEXITCODE" -ForegroundColor Red
  exit $LASTEXITCODE
}
Write-Host 'E2E OK' -ForegroundColor Green
Write-Host 'All high-risk entry automations passed.' -ForegroundColor Green
