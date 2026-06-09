import { test } from '@playwright/test'

const menuKeys = [
  'menu:home',
  'menu:performance-overview',
  'menu:performance-indicator-library',
]

const allPerformancePermissions = [
  'performance:activity:manage',
  'performance:distribution:manage',
  'performance:manager_eval:submit',
  'performance:goal:manage',
  'performance:self_eval:submit',
  'performance:result:view',
  'performance:assessment_manager:update',
  'performance:assessment_manager:batch_update',
  'performance:hr_confirm:submit',
]

test('probe overview useForm warning step', async ({ page }) => {
  const warnings: string[] = []

  page.on('console', msg => {
    const text = msg.text()
    if (text.includes('useForm') || text.includes('not connected')) {
      warnings.push(text)
      console.log(`[probe-warning] ${text}`)
    }
  })

  await page.addInitScript(({ keys, perms }) => {
    window.localStorage.setItem(
      'peopleops-auth',
      JSON.stringify({
        state: {
          user: { id: 'tester', name: 'E2E Tester', menu_keys: keys, permissions: perms },
          token: 'mock-token',
          isLoggedIn: true,
          menuKeys: keys,
          permissions: perms,
        },
        version: 0,
      }),
    )
  }, { keys: menuKeys, perms: allPerformancePermissions })

  await page.route('**/api/v1/**', async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace('/api/v1', '')
    const method = request.method()
    const ok = (data: unknown) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, message: 'success', data }) })

    if (method === 'GET' && path === '/auth/me') return ok({ user: { id: 'tester', name: 'E2E Tester', menu_keys: menuKeys, permissions: allPerformancePermissions } })
    if (method === 'GET' && path === '/departments') return ok({ departments: [{ id: 'dept-1', department_id: 'dept-1', name: 'Product' }] })
    if (method === 'GET' && path === '/users') return ok({ items: [{ id: 'emp-1', user_id: 'emp-1', employee_id: 'E001', name: 'Alice Target', department_name: 'Product', status: 'active' }], total: 1 })
    if (method === 'GET' && path === '/performance/indicator-libraries') return ok({ items: [], total: 0 })
    if (method === 'GET' && path === '/performance/activities') return ok({ items: [{ id: 101, name: 'Draft Cycle', cycle_type: 'quarterly', start_date: '2026-04-01', end_date: '2026-06-30', self_eval_start_at: '2026-06-20', self_eval_end_at: '2026-06-25', manager_eval_start_at: '2026-06-26', manager_eval_end_at: '2026-06-28', result_confirm_start_at: '2026-06-29', result_confirm_end_at: '2026-06-30', status: 'draft', target_department_ids: [], target_employee_ids: [], enable_bonus_score: true, strict_time_mode: false }], total: 1 })
    return ok({})
  })

  const mark = async (label: string) => {
    await page.waitForTimeout(500)
    console.log(`[probe-step] ${label}: warnings=${warnings.length}`)
  }

  await page.goto('/performance-overview')
  await mark('after goto')
  await page.getByTestId('performance-create-activity').click()
  await mark('after click create')
  await page.getByTestId('performance-editor-save').click()
  await mark('after save empty')
})
