import { chromium } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const outDir = path.resolve(__dirname, '../../.wolf/designqc-captures')
const baseURL = process.env.DESIGNQC_URL || 'http://localhost:3000'

const menuKeys = [
  'menu:home',
  'menu:organization-group',
  'menu:organization-dashboard',
  'menu:department-tree',
  'menu:employees',
  'menu:employee-profile',
  'menu:employee-flow',
  'menu:talent-analysis',
  'menu:sync-log',
  'menu:attendance-group',
  'menu:attendance',
  'menu:attendance-stats',
  'menu:attendance-export',
  'menu:attendance-processing',
  'menu:attendance-external-sync',
  'menu:leave-overtime',
  'menu:attendance-toolbox',
  'menu:week-schedule',
  'menu:employee-shift-config',
  'menu:approval-group',
  'menu:approval-templates',
  'menu:approval-instances',
  'menu:approval-stats',
  'menu:permission',
  'menu:jobs-group',
  'menu:sync-jobs',
  'menu:audit-group',
  'menu:audit-logs',
  'menu:performance-group',
  'menu:performance-overview',
  'menu:performance-reports',
  'menu:performance-interviews',
  'menu:performance-appeals',
  'menu:performance-indicator-library',
  'menu:setting',
]

const permissions = [
  'attendance_manage',
  'attendance:view',
  'attendance:sync',
  'performance:activity:manage',
  'performance:activity:import',
  'performance:distribution:manage',
  'performance:manager_eval:submit',
  'performance:goal:manage',
  'performance:self_eval:submit',
  'performance:result:view',
  'performance:assessment_manager:update',
  'performance:assessment_manager:batch_update',
  'performance:hr_confirm:submit',
  'role:manage',
  'user:view',
]

const routes = [
  { path: '/', name: 'home' },
  { path: '/attendance', name: 'attendance' },
  { path: '/attendance-toolbox', name: 'attendance-toolbox' },
  { path: '/performance-overview', name: 'performance-overview' },
  { path: '/leave-overtime', name: 'leave-overtime' },
  { path: '/employees', name: 'employees' },
]

function ok(data, message = 'success') {
  return {
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ code: 200, message, data }),
  }
}

function activity(id, name, status) {
  return {
    id,
    name,
    cycle_type: 'quarterly',
    start_date: '2026-04-01',
    end_date: '2026-06-30',
    target_set_start_at: '2026-04-01',
    target_set_end_at: '2026-04-10',
    self_eval_start_at: '2026-06-20',
    self_eval_end_at: '2026-06-25',
    manager_eval_start_at: '2026-06-26',
    manager_eval_end_at: '2026-06-28',
    result_confirm_start_at: '2026-06-29',
    result_confirm_end_at: '2026-06-30',
    employee_confirm_start_at: '2026-06-29',
    employee_confirm_end_at: '2026-06-30',
    manager_confirm_start_at: '2026-06-29',
    manager_confirm_end_at: '2026-06-30',
    hr_confirm_start_at: '2026-06-29',
    hr_confirm_end_at: '2026-06-30',
    status,
    description: 'UI QC mock activity',
    target_department_ids: ['dept-1'],
    target_employee_ids: ['emp-1'],
    default_assessment_manager_source: 'DIRECT_MANAGER',
    enable_bonus_score: true,
    strict_time_mode: false,
    created_at: '2026-04-01T00:00:00Z',
    updated_at: '2026-04-01T00:00:00Z',
    created_by: 'tester',
    updated_by: 'tester',
  }
}

async function mockAPIs(page) {
  await page.route('**/api/v1/**', async (route) => {
    const req = route.request()
    const url = new URL(req.url())
    const path = url.pathname.replace(/^\/api\/v1/, '')
    const method = req.method()

    if (method === 'GET' && (path === '/auth/me' || path === '/me')) {
      return route.fulfill(ok({
        user: {
          id: 'ui-qc',
          name: 'UI QC',
          menu_keys: menuKeys,
          permissions,
          org_id: 'org-ui',
          department_name: '产品中心',
        },
      }))
    }

    if (path === '/auth/menu-keys' || path === '/permissions/menu-keys') {
      return route.fulfill(ok({ menu_keys: menuKeys, permissions }))
    }

    if (path === '/org/overview' || path === '/organization/overview') {
      return route.fulfill(ok({
        total_users: 128,
        total_departments: 16,
        active_users: 120,
      }))
    }

    if (path.startsWith('/attendance/stats')) {
      return route.fulfill(ok({
        attendance_rate: 96.5,
        late_count: 8,
        early_leave_count: 3,
        absent_count: 1,
        leave_count: 12,
      }))
    }

    if (path.startsWith('/attendance/daily') || path.startsWith('/attendance/records') || path === '/attendance') {
      return route.fulfill(ok({
        list: [
          {
            user_id: 'u1',
            user_name: '张三',
            department_name: '产品中心',
            work_date: '2026-07-16',
            check_in_time: '09:02:11',
            check_out_time: '18:31:08',
            status: 'late',
            status_label: '迟到',
          },
          {
            user_id: 'u2',
            user_name: '李四',
            department_name: '研发中心',
            work_date: '2026-07-16',
            check_in_time: '08:56:02',
            check_out_time: '19:12:44',
            status: 'normal',
            status_label: '正常',
          },
        ],
        total: 2,
        page: 1,
        page_size: 20,
      }))
    }

    if (path.startsWith('/attendance/toolbox/defaults')) {
      return route.fulfill(ok({
        leave_special_names: ['梁伯林'],
        chengdu_schedule_names: ['费婷玉'],
        sub_dept_keywords: ['产品中心'],
        sub_late22_names: [],
        part_special_names: [],
      }))
    }

    if (path.startsWith('/attendance/toolbox')) {
      return route.fulfill(ok({ templates: [], warnings: [], runs: [] }))
    }

    if (path.startsWith('/performance/activities')) {
      return route.fulfill(ok({
        list: [
          activity(101, '2026 Q2 绩效评估', 'in_progress'),
          activity(102, '2026 Q1 绩效评估', 'archived'),
        ],
        total: 2,
        page: 1,
        page_size: 20,
      }))
    }

    if (path.startsWith('/performance/')) {
      return route.fulfill(ok({ list: [], total: 0, page: 1, page_size: 20 }))
    }

    if (path.startsWith('/leave') || path.startsWith('/overtime') || path.startsWith('/compensatory') || path.startsWith('/annual')) {
      return route.fulfill(ok({
        list: [
          {
            user_id: 'u1',
            user_name: '张三',
            annual_balance: 5.5,
            compensatory_balance: 1.0,
            pending_overtime: 2.0,
            status: 'matched',
          },
        ],
        total: 1,
        page: 1,
        page_size: 20,
      }))
    }

    if (path.startsWith('/employees') || path.startsWith('/users') || path.startsWith('/organization/users')) {
      return route.fulfill(ok({
        list: [
          {
            id: 'u1',
            name: '张三',
            job_number: 'MT001',
            department_name: '产品中心',
            position: '产品经理',
            status: 'active',
            hire_date: '2023-03-01',
          },
          {
            id: 'u2',
            name: '李四',
            job_number: 'MT002',
            department_name: '研发中心',
            position: '前端工程师',
            status: 'active',
            hire_date: '2024-01-15',
          },
        ],
        total: 2,
        page: 1,
        page_size: 20,
      }))
    }

    if (path.startsWith('/departments')) {
      return route.fulfill(ok({
        list: [
          { id: 'dept-1', name: '产品中心', parent_id: '' },
          { id: 'dept-2', name: '研发中心', parent_id: '' },
        ],
      }))
    }

    if (path.startsWith('/approvals')) {
      return route.fulfill(ok({ list: [], total: 0, page: 1, page_size: 20 }))
    }

    return route.fulfill(ok({}))
  })
}

async function main() {
  mkdirSync(outDir, { recursive: true })
  const browser = await chromium.launch({ headless: true })
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 1,
  })
  const page = await context.newPage()
  await mockAPIs(page)

  // Force session hydrate path by mocking /auth/me before first navigation.
  for (const route of routes) {
    await page.goto(`${baseURL}${route.path}`, { waitUntil: 'networkidle', timeout: 60000 })
    // Give lazy routes a beat to paint.
    await page.waitForTimeout(800)
    const file = path.join(outDir, `${route.name}_desktop_top.jpg`)
    await page.screenshot({ path: file, type: 'jpeg', quality: 72, fullPage: false })
    console.log(`captured ${route.path} -> ${file}`)
  }

  await browser.close()
  console.log(`done: ${outDir}`)
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
