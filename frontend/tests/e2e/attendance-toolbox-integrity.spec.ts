import { test, expect } from '@playwright/test'

test('upload checks block calculation until fixed, and pending downloads stay incomplete', async ({ page }) => {
  let invalid = true
  let calculations = 0
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  const report = () => ({
    status: invalid ? 'needs_review' : 'passed',
    days_in_month: 30,
    files: [{ field: 'leave_src', file_name: '考勤.xlsx', type: '审批明细', status: invalid ? '失败' : '正常', detail: invalid ? '缺少姓名列' : '识别 1 人' }],
    issues: invalid ? [{ kind: '未识别文件', person: '', file_name: '考勤.xlsx', detail: '缺少姓名列' }] : [],
  })
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace('/api/v1', '')
    const json = (data: unknown) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data }) })
    if (path === '/auth/me') {
      await json({ user: { id: 'integrity-test', name: '测试用户', org_id: 'org-test', menu_keys: ['menu:home', 'menu:attendance-toolbox'], permissions: ['attendance_toolbox_operate'] } })
    } else if (path === '/attendance/toolbox/defaults') {
      await json({ leave_special_names: [], chengdu_schedule_names: [], part_special_names: [] })
    } else if (path === '/attendance/toolbox/templates') {
      await json({ templates: [] })
    } else if (path === '/attendance/toolbox/roster/generate') {
      await route.fulfill({ status: 200, contentType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', body: Buffer.from('xlsx') })
    } else if (path === '/attendance/toolbox/audit') {
      await json({ warnings: [], meta: { integrity_audit: report() } })
    } else if (path === '/attendance/toolbox/workflows/leave') {
      calculations++
      expect(request.postData()).toContain('2026-06')
      await json({ run_id: 'pending-test', module: 'leave', meta: { integrity_audit: {
        ...report(), status: 'needs_review', expected_people: 63, recognized_people: 61, missing_people: 2, totals_verified: false,
        issues: [{ kind: '缺少人员', person: '', detail: '花名册中 2 人没有匹配到明细' }],
      } }, files: [{ file_key: 'check.xlsx', file_name: '待处理检查报告.xlsx', kind: 'audit', size: 4 }] })
    } else if (path.includes('/runs/pending-test/files/')) {
      await route.fulfill({ status: 200, contentType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', body: Buffer.from('audit') })
    } else {
      await json({})
    }
  })
  await page.goto('/attendance-toolbox')
  const panel = page.getByRole('tabpanel', { name: '请假明细' })
  await expect(panel).toBeVisible()
  const month = panel.getByLabel('请假明细处理月份')
  await month.fill('2026-06')
  await month.press('Enter')
  const upload = async (label: string, name: string) => {
    const card = panel.getByText(label, { exact: true }).first().locator('xpath=ancestor::div[contains(@class,"ant-card")][1]')
    await card.locator('input[type=file]').setInputFiles({ name, mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: Buffer.from('xlsx') })
  }
  await upload('请假系统导出表', '考勤.xlsx')
  await upload('作息表', '作息.xlsx')
  await expect(panel.getByText('文件检查未通过，请处理以下问题')).toBeVisible()
  await expect(panel.getByText('失败', { exact: true })).toBeVisible()
  await panel.getByRole('button', { name: '开始计算' }).click()
  await expect(panel.getByRole('button', { name: '开始计算' })).not.toHaveClass(/ant-btn-loading/)
  expect(calculations).toBe(0)
  invalid = false
  await upload('请假系统导出表', '考勤.xlsx')
  await expect(panel.getByText('正常', { exact: true })).toBeVisible()
  await panel.getByRole('button', { name: '开始计算' }).click()
  await expect(panel.getByText('计算未完成，请处理检查问题', { exact: true })).toBeVisible()
  await expect(panel.getByText('缺少人员：2 人', { exact: true })).toBeVisible()
  await expect(panel.getByText('计算完成', { exact: true })).toHaveCount(0)
  expect(calculations).toBe(1)
  expect(errors).toEqual([])
})
