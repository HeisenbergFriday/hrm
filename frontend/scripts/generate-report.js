import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const projectRoot = path.resolve(__dirname, '..', '..')

const report = {
  scope: [
    '登录认证：账号密码登录、钉钉内免登、扫码登录',
    '组织架构：部门树、员工列表、组织同步',
    '考勤管理：考勤记录、统计、同步、导出',
    '审批管理：模板、实例、详情、同步',
    '权限管理：角色、权限点、数据权限',
    '员工档案：档案、入职、转岗、离职',
    '人才分析、周排班、绩效管理等扩展模块',
  ],
  landed: [
    'frontend/tests/e2e/login.spec.ts',
    'frontend/tests/e2e/organization.spec.ts',
    'frontend/tests/e2e/attendance.spec.ts',
    'frontend/vite.config.test.ts',
    'frontend/src/setupTests.ts',
    'frontend/src/services/api.mock.ts',
    'internal/database/database_test.go',
    'internal/dingtalk/dingtalk_test.go',
    'internal/repository/user_repository_test.go',
    'internal/repository/week_schedule_repository_test.go',
    'internal/service/*_test.go',
    'tests/config/test_config.go',
    'tests/database/init_test_db.go',
    'tests/mock/dingtalk_mock.go',
  ],
  status: [
    ['后端单元/服务测试', '已落地', '使用 `go test ./...` 执行'],
    ['前端单测配置', '已落地', '使用 `npm run test` 执行；当前允许无单测文件时通过'],
    ['前端 E2E', '已落地', '登录、组织、考勤三条 Playwright 用例已存在'],
    ['API 契约测试', '待校准', '`tests/api-contract-test.go` 不是标准 `_test.go` 命名，需按当前路由和鉴权重新整理'],
    ['权限专项测试', '待补齐', '尚未形成独立自动化用例'],
    ['回归专项测试', '待补齐', '尚未形成独立目录和必跑集合'],
    ['覆盖率门禁', '待补齐', '尚未配置稳定阈值和前端 coverage provider'],
  ],
  commands: [
    'go test ./...',
    'cd frontend && npm run build',
    'cd frontend && npm run lint',
    'cd frontend && npm run test',
    'cd frontend && npm run e2e',
  ],
  risks: [
    'E2E 依赖前后端测试环境、测试账号和基础数据。',
    '钉钉开放能力仍需在真实企业环境中核验。',
    'API 契约草稿需要先和当前鉴权中间件、路由行为对齐。',
    '前端覆盖率命令需要补齐 coverage provider 后再纳入强制门禁。',
  ],
}

function list(items) {
  return items.map((item) => `- ${item}`).join('\n')
}

function table(rows) {
  return [
    '| 项目 | 状态 | 说明 |',
    '|------|------|------|',
    ...rows.map(([name, status, note]) => `| ${name} | ${status} | ${note} |`),
  ].join('\n')
}

// 生成验收报告
function generateReport() {
  const reportPath = path.join(projectRoot, 'tests', 'reports', 'acceptance-report.md')
  const reportDir = path.dirname(reportPath)

  if (!fs.existsSync(reportDir)) {
    fs.mkdirSync(reportDir, { recursive: true })
  }

  const reportContent = `# 自动验收报告

最后更新：2026-05-26

## 验收范围

${list(report.scope)}

## 当前自动化资产

${list(report.landed)}

## 覆盖状态

${table(report.status)}

## 推荐验证命令

${report.commands.map((command) => `- \`${command}\``).join('\n')}

## 风险与待补齐项

${list(report.risks)}
`

  fs.writeFileSync(reportPath, reportContent, 'utf8')
  console.log(`验收报告已生成：${reportPath}`)
}

if (process.argv[1] && path.resolve(process.argv[1]) === __filename) {
  generateReport()
}

export default generateReport
