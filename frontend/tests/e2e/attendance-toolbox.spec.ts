import { test, expect, type Page, type Route } from '@playwright/test'
import { Buffer } from 'node:buffer'

const toolboxMenu = ['menu:home', 'menu:attendance-toolbox']
let unexpectedAPIRequests: string[] = []
let runtimeErrors: string[] = []

async function json(route: Route, data: unknown, status = 200) {
  await route.fulfill({
    status: status === 410 ? 410 : 200,
    contentType: 'application/json',
    body: JSON.stringify({
      code: status,
      message: status === 410 ? '结果已过期，请重新计算' : status === 200 ? 'success' : 'error',
      data,
    }),
  })
}

async function mockToolboxAPIs(page: Page, opts: {
  permissions: string[]
  workflowHandler?: (route: Route) => Promise<void>
  downloadStatus?: number
}) {
  await page.route('**/api/v1/**', async (route) => {
    const req = route.request()
    const url = new URL(req.url())
    const path = url.pathname.replace('/api/v1', '')
    const method = req.method()

    if (method === 'GET' && path === '/auth/me') {
      await json(route, {
        user: {
          id: 'e2e-user',
          name: 'E2E User',
          menu_keys: toolboxMenu,
          permissions: opts.permissions,
          org_id: 'org-e2e',
        },
      })
      return
    }

    if (path === '/attendance/toolbox/defaults') {
      await json(route, {
        leave_special_names: ['梁伯林'],
        chengdu_schedule_names: ['费婷玉'],
        sub_dept_keywords: ['产品中心'],
        sub_late22_names: [],
        part_special_names: [],
      })
      return
    }

    if (path === '/attendance/toolbox/templates' && method === 'POST') {
      await json(route, { templates: [] })
      return
    }

    if (path === '/attendance/toolbox/audit') {
      await json(route, { warnings: [] })
      return
    }

    if (path === '/attendance/toolbox/dingtalk-sync' && method === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
        body: Buffer.from('fake-xlsx'),
        headers: { 'Content-Disposition': 'attachment; filename="sync.xlsx"' },
      })
      return
    }

    if (path.startsWith('/attendance/toolbox/workflows/') && method === 'POST') {
      if (opts.workflowHandler) {
        await opts.workflowHandler(route)
        return
      }
      await json(route, {
        run_id: 'run-e2e-1',
        module: 'leave',
        log: 'e2e-log',
        stats: { rows: 1 },
        files: [
          { file_key: '1_result.xlsx', file_name: '结果.xlsx', kind: 'export', size: 12, row_count: 1 },
        ],
        expires_at: '2099-01-01T00:00:00Z',
      })
      return
    }

    if (method === 'GET' && path.includes('/attendance/toolbox/runs/') && path.endsWith('/preview')) {
      await json(route, { rows: [{ name: '\u6d4b\u8bd5\u7532', employee_id: 'MT001' }] })
      return
    }
    if (path.includes('/attendance/toolbox/runs/') && path.includes('/files/')) {
      if (opts.downloadStatus === 410) {
        await json(route, null, 410)
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
        body: Buffer.from('fake-xlsx'),
      })
      return
    }

    if (path.includes('/attendance/toolbox/runs/') && path.endsWith('/zip')) {
      if (opts.downloadStatus === 410) {
        await json(route, null, 410)
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/zip',
        body: Buffer.from('PK fake'),
      })
      return
    }

    if (path === '/attendance/toolbox/rules/import-preview') {
      await json(route, {
        premium_rules: [{ priority: 1, date_type: 'LEGAL_HOLIDAY', department_group: '全部', action: '加班工资', multiplier: 3 }],
        department_rules: [],
        params: { standard_hours_per_day: 8 },
      })
      return
    }

    if (path === '/attendance/toolbox/rules/export') {
      await route.fulfill({
        status: 200,
        contentType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
        body: Buffer.from('rules'),
      })
      return
    }

    unexpectedAPIRequests.push(`${method} ${path}`)
    await route.fulfill({
      status: 500,
      contentType: 'application/json',
      body: JSON.stringify({ code: 500, message: `unexpected mocked API request: ${method} ${path}`, data: null }),
    })
  })
}

async function openToolbox(page: Page, permissions: string[]) {
  await mockToolboxAPIs(page, { permissions })
  await page.goto('/attendance-toolbox')
  await expect(page.getByText('考勤数据处理工具')).toBeVisible({ timeout: 30_000 })
}

async function uploadByLabel(page: Page, label: string, fileName: string) {
  const labelNode = page.getByText(label, { exact: true }).first()
  await expect(labelNode).toBeVisible()
  const card = labelNode.locator('xpath=ancestor::div[contains(@class,"ant-card")][1]')
  const input = card.locator('input[type="file"]')
  await input.setInputFiles({
    name: fileName,
    mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    buffer: Buffer.from('fake-xlsx-content'),
  })
  await expect(page.getByText(fileName).first()).toBeVisible({ timeout: 10_000 })
}

test.describe('Attendance toolbox E2E', () => {
  test.beforeEach(async ({ page }) => {
    unexpectedAPIRequests = []
    runtimeErrors = []
    page.on('console', (entry) => {
      if (entry.type() !== 'error') return
      const text = entry.text()
      // Chromium logs the intentionally mocked 410 response as a resource error.
      if (text.includes('status of 410 (Gone)')) return
      runtimeErrors.push(`console.error: ${text}`)
    })
    page.on('pageerror', (error) => runtimeErrors.push(`pageerror: ${error.message}`))
  })

  test.afterEach(async () => {
    expect(unexpectedAPIRequests, 'all /api/v1 requests must be explicitly mocked').toEqual([])
    expect(runtimeErrors, 'page must not emit console.error or pageerror').toEqual([])
  })
  test('A. page loads with six tabs', async ({ page }) => {
    await openToolbox(page, [
      'attendance_toolbox_operate',
      'attendance_toolbox_dingtalk_sync',
      'attendance_toolbox_rules_edit',
    ])
    await expect(page.getByRole('tab', { name: /钉钉同步/ })).toBeVisible()
    await expect(page.getByRole('tab', { name: /请假明细/ })).toBeVisible()
    await expect(page.getByRole('tab', { name: /加班明细/ })).toBeVisible()
    await expect(page.getByRole('tab', { name: /补贴扣款/ })).toBeVisible()
    await expect(page.getByRole('tab', { name: /最终汇总/ })).toBeVisible()
    await expect(page.getByRole('tab', { name: /兼职汇总/ })).toBeVisible()
  })

  test('B. menu-only disables operate/sync actions', async ({ page }) => {
    await openToolbox(page, [])
    await expect(page.getByRole('button', { name: /开始计算/ })).toBeDisabled()
    await page.getByRole('tab', { name: /钉钉同步/ }).click()
    await expect(page.getByRole('button', { name: /从钉钉同步并生成中间表/ })).toBeDisabled()
  })

  test('C. operate user runs workflow and sees result files', async ({ page }) => {
    let workflowCalled = false
    await mockToolboxAPIs(page, {
      permissions: ['attendance_toolbox_operate'],
      workflowHandler: async (route) => {
        workflowCalled = true
        await json(route, {
          run_id: 'run-e2e-leave',
          module: 'leave',
          log: 'ok',
          stats: { rows: 2 },
          files: [
            { file_key: '1_result.xlsx', file_name: '结果.xlsx', kind: 'export', size: 10, row_count: 2 },
          ],
          expires_at: '2099-01-01T00:00:00Z',
        })
      },
    })
    await page.goto('/attendance-toolbox')
    await expect(page.getByText('考勤数据处理工具')).toBeVisible({ timeout: 30_000 })

    await uploadByLabel(page, '请假系统导出表', 'leave.xlsx')
    await uploadByLabel(page, '作息表', 'schedule.xlsx')

    await page.getByRole('button', { name: /开始计算/ }).click()
    await expect.poll(() => workflowCalled, { timeout: 20_000 }).toBe(true)
    await expect(page.getByText('计算成功')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByRole('button', { name: /结果\.xlsx/ })).toBeVisible()
    await expect(page.getByRole('button', { name: /全部 ZIP 下载/ })).toBeVisible()
    // run_id 在「技术信息」折叠内
    await page.getByText('技术信息').click()
    await expect(page.getByText(/run_id: run-e2e-leave/)).toBeVisible()
  })

  test('D. quick workflow submits dates, options, and schedule', async ({ page }) => {
    let quickRequestBody = ''
    await mockToolboxAPIs(page, {
      permissions: ['attendance_toolbox_operate', 'attendance_toolbox_dingtalk_sync'],
      workflowHandler: async (route) => {
        if (!route.request().url().endsWith('/attendance/toolbox/workflows/quick')) {
          throw new Error(`unexpected workflow route: ${route.request().url()}`)
        }
        quickRequestBody = route.request().postDataBuffer()?.toString('utf8') || ''
        await json(route, {
          run_id: 'run-e2e-quick',
          module: 'quick',
          log: 'quick-ok',
          stats: { rows: 2 },
          files: [{ file_key: 'quick.xlsx', file_name: '联动结果.xlsx', kind: 'export', size: 12 }],
          expires_at: '2099-01-01T00:00:00Z',
        })
      },
    })
    await page.goto('/attendance-toolbox')
    await expect(page.getByText('考勤数据处理工具')).toBeVisible({ timeout: 30_000 })
    await uploadByLabel(page, '作息表', 'schedule.xlsx')
    await page.getByRole('tab', { name: /钉钉同步/ }).click()
    await page.getByPlaceholder('开始日期').fill('2026-03-01')
    await page.getByPlaceholder('结束日期').fill('2026-03-31')
    await page.keyboard.press('Enter')
    await page.getByRole('button', { name: /一键同步并生成请假\/加班/ }).click()

    await expect.poll(() => quickRequestBody, { timeout: 20_000 }).not.toBe('')
    expect(quickRequestBody).toContain('name="dingtalk_sync_start_date"')
    expect(quickRequestBody).toContain('2026-03-01')
    expect(quickRequestBody).toContain('name="dingtalk_sync_end_date"')
    expect(quickRequestBody).toContain('2026-03-31')
    expect(quickRequestBody).toContain('name="run_leave"')
    expect(quickRequestBody).toContain('name="run_overtime"')
    expect(quickRequestBody).toContain('name="leave_schedule"')
    await expect(page.getByRole('button', { name: /\.xlsx/ })).toBeVisible({ timeout: 15_000 })
  })

  test('E. custom overtime rules are submitted with workflow request', async ({ page }) => {
    let overtimeRequestBody = ''
    await mockToolboxAPIs(page, {
      permissions: ['attendance_toolbox_operate', 'attendance_toolbox_rules_edit'],
      workflowHandler: async (route) => {
        if (!route.request().url().endsWith('/attendance/toolbox/workflows/overtime')) {
          throw new Error(`unexpected workflow route: ${route.request().url()}`)
        }
        overtimeRequestBody = route.request().postDataBuffer()?.toString('utf8') || ''
        await json(route, {
          run_id: 'run-e2e-overtime',
          module: 'overtime',
          log: 'overtime-ok',
          stats: { rows: 1 },
          files: [{ file_key: 'overtime.xlsx', file_name: '加班结果.xlsx', kind: 'export', size: 12 }],
          expires_at: '2099-01-01T00:00:00Z',
        })
      },
    })
    await page.goto('/attendance-toolbox')
    await expect(page.getByText('考勤数据处理工具')).toBeVisible({ timeout: 30_000 })
    await page.getByRole('tab', { name: /加班明细/ }).click()
    await uploadByLabel(page, '加班系统导出表', 'overtime.xlsx')
    await page.getByText('加班规则配置').first().click()

    await page.locator('input[type="file"][accept=".xlsx,.xls"]').setInputFiles({
      name: 'rules.xlsx',
      mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      buffer: Buffer.from('synthetic-rules'),
    })
    await page.getByRole('button', { name: /导入并预览/ }).click()
    await expect(page.getByText('倍数规则（只读）')).toBeVisible({ timeout: 10_000 })
    await page.getByRole('button', { name: /应用规则/ }).click()
    await expect(page.getByText('当前计算使用：自定义规则')).toBeVisible()
    await page.getByRole('button', { name: /开始计算/ }).click()

    await expect.poll(() => overtimeRequestBody, { timeout: 20_000 }).not.toBe('')
    expect(overtimeRequestBody).toContain('name="rules_json"')
    expect(overtimeRequestBody).toContain('LEGAL_HOLIDAY')
    await expect(page.getByText('计算成功')).toBeVisible({ timeout: 15_000 })
    await page.getByText('技术信息').click()
    await expect(page.getByText(/run_id: run-e2e-overtime/)).toBeVisible()
    await expect(page.getByText('测试甲')).toBeVisible({ timeout: 10_000 })
  })
  test('F. 410 download shows expired message', async ({ page }) => {
    await mockToolboxAPIs(page, {
      permissions: ['attendance_toolbox_operate'],
      downloadStatus: 410,
      workflowHandler: async (route) => {
        await json(route, {
          run_id: 'run-410',
          module: 'leave',
          files: [{ file_key: '1.xlsx', file_name: '结果.xlsx', kind: 'export', size: 1 }],
          expires_at: '2099-01-01T00:00:00Z',
        })
      },
    })
    await page.goto('/attendance-toolbox')
    await expect(page.getByText('考勤数据处理工具')).toBeVisible({ timeout: 30_000 })

    await uploadByLabel(page, '请假系统导出表', 'leave.xlsx')
    await uploadByLabel(page, '作息表', 'schedule.xlsx')
    await page.getByRole('button', { name: /开始计算/ }).click()
    // applyRunResponse downloads after workflow → 410 → message.error with 过期
    await expect(page.getByText(/过期/).first()).toBeVisible({ timeout: 20_000 })
  })
})
