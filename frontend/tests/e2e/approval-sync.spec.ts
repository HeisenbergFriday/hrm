import { expect, test, type Page, type Route } from '@playwright/test'

async function json(route: Route, data: unknown, status = 200, message = 'success') {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify({ code: status, message, data }),
  })
}

async function mockApprovalSync(page: Page) {
  let instanceQueries = 0
  let statusQueries = 0
  let startBody: Record<string, unknown> | null = null

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace('/api/v1', '')
    const method = request.method()

    if (method === 'GET' && path === '/auth/me') {
      await json(route, {
        user: {
          id: 'approval-e2e-user',
          name: '审批测试用户',
          org_id: 'org-approval-e2e',
          menu_keys: ['menu:home', 'menu:approval-instances'],
          permissions: ['approval:sync'],
        },
      })
      return
    }
    if (method === 'GET' && path === '/approvals/templates') {
      await json(route, { items: [{ template_id: 'PROC-OVERTIME', name: '加班审批' }], total: 1 })
      return
    }
    if (method === 'GET' && path === '/approvals/instances') {
      instanceQueries++
      await json(route, { items: [], total: 0 })
      return
    }
    if (method === 'POST' && path === '/approvals/sync/start') {
      startBody = request.postDataJSON() as Record<string, unknown>
      await json(route, { status: 'running', request_id: 'approval-e2e-request' }, 202, 'running')
      return
    }
    if (method === 'GET' && path === '/approvals/sync/approval-e2e-request') {
      statusQueries++
      if (statusQueries === 1) {
        await json(route, { status: 'running', request_id: 'approval-e2e-request' }, 202, 'running')
        return
      }
      await json(route, {
        status: 'partial',
        processes: [
          { process_code: 'PROC-LEAVE', status: 'success', fetched_count: 2, fetch_fail_count: 0, success_count: 2, fail_count: 0 },
          { process_code: 'PROC-OVERTIME', status: 'failed', fetched_count: 0, fetch_fail_count: 0, success_count: 0, fail_count: 0, error_code: 'DINGTALK_RESPONSE_INVALID', error: '钉钉审批同步失败，请检查应用配置或权限' },
        ],
        process_count: 2,
        succeeded_processes: 1,
        failed_processes: 1,
        fetched_count: 2,
        fetch_fail_count: 0,
        success_count: 2,
        fail_count: 0,
        start_date: '2026-07-05',
        end_date: '2026-08-05',
        sync_time: '2026-08-05T12:00:00+08:00',
        duration_ms: 2500,
        request_id: 'approval-e2e-request',
      }, 200, 'partial')
      return
    }

    await json(route, null, 500, `unexpected mocked API request: ${method} ${path}`)
  })

  return {
    getInstanceQueries: () => instanceQueries,
    getStatusQueries: () => statusQueries,
    getStartBody: () => startBody,
  }
}

test('approval instance page starts full sync, polls running, shows partial result, and refreshes', async ({ page }) => {
  const state = await mockApprovalSync(page)

  await page.goto('/approval-instances')
  const syncButton = page.getByRole('button', { name: /同步全部/ })
  await expect(syncButton).toBeVisible({ timeout: 30_000 })
  await expect.poll(state.getInstanceQueries).toBeGreaterThan(0)
  const queriesBeforeSync = state.getInstanceQueries()

  await syncButton.click()
  await expect(page.getByText('审批同步中')).toBeVisible()
  await expect.poll(state.getStatusQueries, { timeout: 10_000 }).toBeGreaterThanOrEqual(2)
  await expect(page.getByText('审批同步部分成功')).toBeVisible({ timeout: 10_000 })
  await expect.poll(state.getInstanceQueries).toBeGreaterThan(queriesBeforeSync)

  expect(state.getStartBody()).not.toHaveProperty('process_code')
})
