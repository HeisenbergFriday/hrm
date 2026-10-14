import { test, expect } from '@playwright/test'

test('leave pull starts once, polls running, then downloads only the business export', async ({ page }) => {
  const unexpected: string[] = []
  const errors: string[] = []
  let leaveStarts = 0
  let leaveQueries = 0
  const downloads: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace('/api/v1', '')
    const json = async (data: unknown, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify({ code: status, data }) })
    const xlsx = async () => route.fulfill({ status: 200, contentType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', body: Buffer.from('xlsx') })
    if (path === '/auth/me') {
      await json({ user: { id: 'e2e-user', name: '测试用户', org_id: 'org-e2e', menu_keys: ['menu:home', 'menu:attendance-toolbox'], permissions: ['attendance_toolbox_operate', 'attendance_toolbox_dingtalk_sync'] } })
    } else if (path === '/attendance/toolbox/defaults') {
      await json({ leave_special_names: [], chengdu_schedule_names: [], sub_dept_keywords: [], sub_late22_names: [], part_special_names: [] })
    } else if (path === '/attendance/toolbox/templates') {
      await json({ templates: [] })
    } else if (path === '/attendance/toolbox/roster/generate') {
      await xlsx()
    } else if (path === '/attendance/toolbox/audit') {
      await json({ warnings: [] })
    } else if (path === '/attendance/toolbox/dingtalk-sync/start') {
      const body = request.postDataJSON() as { flow_keys: string[]; padding_days: number }
      const flow = body.flow_keys[0]
      if (flow === 'leave') {
        leaveStarts++
        expect(body.padding_days).toBe(31)
      }
      await json({ request_id: `job-${flow}`, status: 'running' }, 202)
    } else if (path.startsWith('/attendance/toolbox/dingtalk-sync/jobs/')) {
      const flow = path.split('/job-')[1]
      if (flow === 'leave') leaveQueries++
      await json({ request_id: `job-${flow}`, status: flow === 'leave' && leaveQueries === 1 ? 'running' : 'success', run_id: `run-${flow}` })
    } else if (path.includes('/runs/') && path.includes('/files/')) {
      downloads.push(path)
      await xlsx()
    } else if (path.startsWith('/attendance/toolbox/runs/')) {
      const flow = path.split('/run-')[1]
      await json({ run_id: `run-${flow}`, module: 'dingtalk_sync', expires_at: '2099-01-01', files: [
        { file_key: 'export.xlsx', file_name: '业务表.xlsx', content_type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', size: 4, kind: 'export', flow_key: flow },
        { file_key: 'audit.xlsx', file_name: '审计表.xlsx', size: 4, kind: 'audit' },
      ] })
    } else {
      unexpected.push(`${request.method()} ${path}`)
      await json(null, 500)
    }
  })
  await page.goto('/attendance-toolbox')
  const button = page.getByRole('button', { name: '从钉钉拉取请假表' })
  await expect(button).toBeVisible()
  await button.click()
  await expect(button).toHaveClass(/ant-btn-loading/)
  await expect(page.getByText('请假系统导出_钉钉同步.xlsx', { exact: true })).toBeVisible({ timeout: 15000 })
  await expect(button).not.toHaveClass(/ant-btn-loading/)
  expect(leaveStarts).toBe(1)
  expect(leaveQueries).toBe(2)
  expect(downloads).toContain('/attendance/toolbox/runs/run-leave/files/export.xlsx')
  expect(downloads.some((path) => path.endsWith('/audit.xlsx'))).toBe(false)
  expect(unexpected).toEqual([])
  expect(errors).toEqual([])
})
