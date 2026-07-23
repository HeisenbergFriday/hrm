import { test, expect, type Page, type Route } from '@playwright/test'
import { Buffer } from 'node:buffer'

const menuKeys = [
  'menu:home',
  'menu:performance-overview',
  'menu:performance-indicator-library',
]

const allPerformancePermissions = [
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
]

type MockOptions = {
  permissions?: string[]
  /** 默认完整绩效菜单；高风险入口用例可收窄或置空 */
  menuKeys?: string[]
}

async function seedAuth(
  page: Page,
  permissions = allPerformancePermissions,
  keys: string[] = menuKeys,
) {
  await page.addInitScript(({ nextKeys, perms }) => {
    window.localStorage.setItem(
      'peopleops-auth',
      JSON.stringify({
        state: {
          user: {
            id: 'tester',
            name: 'E2E Tester',
            menu_keys: nextKeys,
            permissions: perms,
          },
          token: 'mock-token',
          isLoggedIn: true,
          menuKeys: nextKeys,
          permissions: perms,
        },
        version: 0,
      }),
    )
  }, { nextKeys: keys, perms: permissions })
}

function activityFixture(id: number, name: string, status: string) {
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
    description: 'Mock performance cycle',
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

function participantFixture(id: number, status: string, name: string) {
  return {
    id,
    activity_id: 102,
    employee_id: `emp-${id}`,
    employee_name: name,
    department_id: 'dept-1',
    department_name: 'Product',
    position: 'Product Manager',
    level: 'P6',
    employee_status: 'active',
    manager_id: 'mgr-1',
    manager_name: 'Morgan Manager',
    direct_manager_id_snapshot: 'mgr-1',
    direct_manager_name_snapshot: 'Morgan Manager',
    manager_source: 'DIRECT_MANAGER',
    manager_overridden: false,
    manager_config_status: 'CONFIGURED',
    status,
    self_score: 90,
    self_level: 'A',
    self_summary: '',
    manager_score: 92,
    manager_comment: '',
    suggested_level: 'A',
    final_level: 'A',
    adjust_reason: '',
    self_evaluation_good: 'Strong delivery',
    self_evaluation_improvement: 'Sharper prioritization',
    manager_evaluation_good: 'Clear ownership',
    manager_evaluation_improvement: 'More stakeholder previews',
    total_self_score: 90,
    total_manager_score: 92,
    bonus_score: 0,
    penalty_score: 0,
    adjusted_score: 92,
    revenue_coefficient: 1,
    is_locked: false,
    confirmed_by: '',
    created_at: '2026-04-01T00:00:00Z',
    updated_at: '2026-04-01T00:00:00Z',
  }
}

function goalRecords(participantId: number, options: { emptySelf?: boolean; emptyManager?: boolean } = {}) {
  return [
    {
      id: participantId * 10 + 1,
      activity_id: '102',
      participant_id: participantId,
      section_type: 'quantitative',
      item_name: 'Revenue retention',
      item_definition: 'Keep key accounts retained',
      weight: 0.7,
      red_line_value: '80',
      target_value: '100',
      challenge_value: '120',
      scoring_rule: 'Linear by retention rate',
      actual_result: options.emptySelf ? '' : '108',
      attachments: [],
      self_score: options.emptySelf ? 0 : 90,
      manager_score: options.emptyManager ? 0 : 95,
      bonus_score: 0,
      is_from_superior: false,
      approval_status: 'draft',
      visibility_scope: 'all',
      sort_order: 0,
      created_at: '2026-04-01T00:00:00Z',
      updated_at: '2026-04-01T00:00:00Z',
    },
    {
      id: participantId * 10 + 2,
      activity_id: '102',
      participant_id: participantId,
      section_type: 'key_action',
      item_name: 'Launch planning',
      item_definition: 'Ship cross-functional launch plan',
      weight: 0.3,
      red_line_value: '',
      target_value: 'Launch plan approved',
      challenge_value: '',
      scoring_rule: 'Qualitative review',
      actual_result: options.emptySelf ? '' : 'Plan approved',
      attachments: [],
      self_score: options.emptySelf ? 0 : 88,
      manager_score: options.emptyManager ? 0 : 88,
      bonus_score: 0,
      is_from_superior: false,
      approval_status: 'draft',
      visibility_scope: 'all',
      sort_order: 1,
      created_at: '2026-04-01T00:00:00Z',
      updated_at: '2026-04-01T00:00:00Z',
    },
  ]
}

async function json(route: Route, data: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify({
      code: status,
      message: status === 200 ? 'success' : 'error',
      data,
    }),
  })
}

async function setupPerformanceMock(page: Page, options: MockOptions = {}) {
  const resolvedMenuKeys = options.menuKeys ?? menuKeys
  const resolvedPermissions = options.permissions ?? allPerformancePermissions
  await seedAuth(page, resolvedPermissions, resolvedMenuKeys)

  const activities = new Map<number, any>([
    [101, activityFixture(101, 'Draft Cycle', 'draft')],
    [102, activityFixture(102, 'Target Setting Cycle', 'target_setting')],
    [103, activityFixture(103, 'Manager Review Cycle', 'manager_evaluation')],
    // 沐腾新流程：用于「上期结果」跳转验收
    [110, {
      ...activityFixture(110, 'Muteng Current Cycle', 'manager_evaluation'),
      flow_type: 'new',
      previous_review_activity_id: 109,
    }],
    [109, {
      ...activityFixture(109, 'Muteng Previous Cycle', 'archived'),
      flow_type: 'new',
    }],
  ])
  const participants = new Map<number, any>([
    [201, participantFixture(201, 'pending', 'Alice Target')],
    [202, participantFixture(202, 'target_set', 'Sam Self')],
    [203, participantFixture(203, 'self_submitted', 'Mina Manager')],
    [204, participantFixture(204, 'manager_submitted', 'Riley Result')],
    [205, participantFixture(205, 'target_pending_approval', 'Taylor Approval')],
    [210, { ...participantFixture(210, 'manager_submitted', 'Prev Link User'), activity_id: 110, employee_id: 'emp-prev' }],
    [209, { ...participantFixture(209, 'result_confirmed', 'Prev Link User'), activity_id: 109, employee_id: 'emp-prev' }],
  ])

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace('/api/v1', '')
    const method = request.method()

    if (method === 'GET' && path === '/auth/me') {
      return json(route, {
        user: {
          id: 'tester',
          name: 'E2E Tester',
          menu_keys: resolvedMenuKeys,
          permissions: resolvedPermissions,
        },
      })
    }

    if (method === 'GET' && path === '/departments') {
      return json(route, {
        departments: [{ id: 'dept-1', department_id: 'dept-1', name: 'Product' }],
      })
    }

    if (method === 'GET' && path === '/users') {
      return json(route, {
        items: [{ id: 'emp-1', user_id: 'emp-1', employee_id: 'E001', name: 'Alice Target', department_name: 'Product', status: 'active' }],
        total: 1,
      })
    }

    if (method === 'GET' && path === '/performance/scope-options') {
      return json(route, {
        employees: [{ user_id: 'emp-1', name: 'Alice Target', department_name: 'Product', position: 'PM' }],
      })
    }

    if (method === 'POST' && path === '/performance/imports/analyze') {
      return json(route, {
        batch_id: 'excel-e2e-batch',
        status: 'analyzed',
        created_at: '2026-07-17T08:00:00Z',
        preview: {
          source_type: 'xiaotie',
          source_label: '小铁文娱',
          file_name: 'xiaotie-performance.xlsx',
          file_sha256: 'e2e-sha256',
          requires_review: true,
          issues: [{ level: 'warning', code: 'weight_rescaled', message: '权重已按模板比例换算' }],
          drafts: [{
            draft_key: 'xiaotie_staff',
            selected: true,
            source_sheet: '普通员工绩效',
            template_name: '普通员工月度绩效模板',
            activity_name: '2026年7月普通员工绩效',
            flow_type: 'old',
            activity_kind: 'review_scoring',
            cycle_type: 'monthly',
            start_date: '2026-07-01',
            end_date: '2026-07-31',
            enable_bonus_score: false,
            employee_name: 'Alice Target',
            employee_user_id: 'emp-1',
            employee_match: 'matched',
            source_weight_total: 100,
            sections: [{
              name: '业绩指标',
              section_type: 'performance',
              weight: 70,
              is_score_required: true,
              is_comment_required: false,
              items: [{ name: '目标交付', description: '按期完成', weight: 100, max_score: 100 }],
            }],
            goals: [{
              section_type: 'performance',
              goal_type: 'quantitative',
              is_fixed: false,
              item_name: '目标交付',
              item_definition: '按期完成',
              weight: 70,
              scoring_rule: '按完成度评分',
              sort_order: 1,
            }],
          }],
        },
      })
    }

    if (method === 'GET' && path === '/performance/imports/excel-e2e-batch') {
      return json(route, { batch_id: 'excel-e2e-batch', status: 'analyzed', created_at: '2026-07-17T08:00:00Z' })
    }

    if (method === 'POST' && path === '/performance/imports/excel-e2e-batch/commit') {
      return json(route, {
        batch_id: 'excel-e2e-batch',
        created: [{
          draft_key: 'xiaotie_staff',
          template_id: 301,
          template_reused: false,
          activity_id: 401,
          activity_name: '2026年7月普通员工绩效',
          participant_id: 501,
          employee_user_id: 'emp-1',
          goal_count: 1,
        }],
        warnings: [],
      })
    }

    if (method === 'GET' && path === '/performance/indicator-libraries') {
      return json(route, {
        items: [{ id: 1, name: 'Quarterly Product Library', department_id: 'dept-1', department_name: 'Product', default_cycle: 'quarterly', status: 'active' }],
        total: 1,
      })
    }

    if (method === 'GET' && path === '/performance/activities') {
      const items = Array.from(activities.values())
      return json(route, { items, total: items.length })
    }

    const activityParticipantsMatch = path.match(/^\/performance\/activities\/(\d+)\/participants$/)
    if (method === 'GET' && activityParticipantsMatch) {
      const activityId = Number(activityParticipantsMatch[1])
      let items: any[]
      if (activityId === 103) {
        items = [participants.get(203), participants.get(204)]
      } else if (activityId === 110) {
        items = [participants.get(210)]
      } else {
        items = [participants.get(201), participants.get(205)]
      }
      return json(route, { items, total: items.length })
    }

    const summaryMatch = path.match(/^\/performance\/activities\/(\d+)\/result-summary$/)
    if (method === 'GET' && summaryMatch) {
      return json(route, {
        total_participants: 2,
        self_submitted_count: 1,
        manager_submitted_count: 1,
        result_confirmed_count: 0,
        level_distribution: { A: 1, B: 1 },
      })
    }

    const distributionCheckMatch = path.match(/^\/performance\/activities\/(\d+)\/distribution-check$/)
    if (method === 'GET' && distributionCheckMatch) {
      return json(route, {
        passed: true,
        total_count: 2,
        exceeded_levels: [],
        distribution: {
          S: { expected_count: 0, actual_count: 0, expected_percent: 10, actual_percent: 0, progress: 0, status: 'ok' },
          A: { expected_count: 1, actual_count: 1, expected_percent: 20, actual_percent: 50, progress: 100, status: 'ok' },
          B: { expected_count: 1, actual_count: 1, expected_percent: 50, actual_percent: 50, progress: 100, status: 'ok' },
          C: { expected_count: 0, actual_count: 0, expected_percent: 15, actual_percent: 0, progress: 0, status: 'ok' },
          D: { expected_count: 0, actual_count: 0, expected_percent: 5, actual_percent: 0, progress: 0, status: 'ok' },
        },
        warnings: [],
      })
    }

    const distributionRulesMatch = path.match(/^\/performance\/activities\/(\d+)\/distribution-rules$/)
    if (method === 'GET' && distributionRulesMatch) {
      return json(route, { rules: [] })
    }
    if (method === 'PUT' && distributionRulesMatch) {
      let payload: any = { rules: [] }
      try {
        payload = route.request().postDataJSON()
      } catch {
        payload = { rules: [] }
      }
      return json(route, { rules: payload.rules || [] })
    }

    const deadlineMatch = path.match(/^\/performance\/activities\/(\d+)\/hr-confirm-deadline-status$/)
    if (method === 'GET' && deadlineMatch) {
      return json(route, { deadline: '2026-06-30', pending_count: 0, overdue: false, can_force_lock: false })
    }

    const realtimeDistributionMatch = path.match(/^\/performance\/activities\/(\d+)\/realtime-distribution-check$/)
    if (method === 'GET' && realtimeDistributionMatch) {
      return json(route, {
        teams: [{
          manager_id: 'mgr-1',
          manager_name: 'Morgan Manager',
          total: 5,
          levels: {
            S: { current: 0, max: 1, percent: 20 },
            A: { current: 0, max: 2, percent: 40 },
            B: { current: 1, max: 3, percent: 60 },
            CD: { current: 0, max: 1, percent: 20 },
          },
        }],
      })
    }

    const refreshParticipantsMatch = path.match(/^\/performance\/activities\/(\d+)\/refresh-participants$/)
    if (method === 'POST' && refreshParticipantsMatch) {
      const activity = activities.get(Number(refreshParticipantsMatch[1]))
      return json(route, {
        activity,
        refreshed_count: 2,
        created_count: 0,
        removed_count: 0,
      })
    }

    const activityActionStatusMap: Record<string, string> = {
      start: 'target_setting',
      publish: 'self_evaluation',
      close: 'archived',
      'open-target-setting': 'target_setting',
      'open-self-evaluation': 'self_evaluation',
      'open-manager-evaluation': 'manager_evaluation',
      'open-employee-confirmation': 'employee_confirmation',
      'open-manager-confirmation': 'manager_confirmation',
      'open-hr-confirmation': 'hr_confirmation',
      'confirm-results': 'result_confirmed',
      lock: 'locked',
      archive: 'archived',
      'force-lock-overdue-hr': 'locked',
    }
    const activityActionMatch = path.match(/^\/performance\/activities\/(\d+)\/([a-z-]+)$/)
    if (method === 'POST' && activityActionMatch && activityActionStatusMap[activityActionMatch[2]]) {
      const activity = activities.get(Number(activityActionMatch[1]))
      if (activity) activity.status = activityActionStatusMap[activityActionMatch[2]]
      return json(route, { activity })
    }

    const activityReminderMatch = path.match(/^\/performance\/activities\/(\d+)\/send-(self-eval|manager-eval|hr-confirm)-reminder$/)
    if (method === 'POST' && activityReminderMatch) {
      return json(route, { ok: true })
    }

    const activityMatch = path.match(/^\/performance\/activities\/(\d+)$/)
    if (method === 'GET' && activityMatch) {
      return json(route, { activity: activities.get(Number(activityMatch[1])) })
    }

    if (method === 'POST' && path === '/performance/participants/import') {
      return json(route, {
        result: {
          activity_name: 'Imported Performance Cycle',
          employee_ids: ['emp-1'],
          employees: [{ user_id: 'emp-1', employee_id: 'E001', name: 'Alice Target', department_name: 'Product' }],
          manager_assignments: [{ user_id: 'emp-1', assessment_manager_user_id: 'mgr-1', assessment_manager_name: 'Morgan Manager', assessment_manager_source: 'IMPORT' }],
          parsed_count: 1,
          imported_count: 1,
          duplicate_count: 0,
          missing_employee_ids: [],
          inactive_employee_ids: [],
          skipped_rows: [],
          warnings: [],
        },
      })
    }

    const assessmentCandidatesMatch = path.match(/^\/performance\/activities\/(\d+)\/assessment-manager-candidates$/)
    if (method === 'GET' && assessmentCandidatesMatch) {
      return json(route, {
        items: [{ user_id: 'mgr-1', employee_id: 'M001', name: 'Morgan Manager', department_name: 'Product' }],
        total: 1,
      })
    }

    const batchAssessmentMatch = path.match(/^\/performance\/activities\/(\d+)\/assessment-managers\/batch$/)
    if (method === 'POST' && batchAssessmentMatch) {
      return json(route, { results: [{ participant_id: 201, success: true }] })
    }

    const updateAssessmentMatch = path.match(/^\/performance\/participants\/(\d+)\/assessment-manager$/)
    if (method === 'PUT' && updateAssessmentMatch) {
      const participant = participants.get(Number(updateAssessmentMatch[1]))
      if (participant) {
        participant.manager_id = 'mgr-1'
        participant.manager_name = 'Morgan Manager'
        participant.manager_source = 'MANUAL'
        participant.manager_overridden = true
      }
      return json(route, { participant })
    }

    const suggestionsMatch = path.match(/^\/performance\/goal-records\/(\d+)\/suggestions$/)
    if (method === 'GET' && suggestionsMatch) {
      return json(route, {
        suggestions: [{ name: 'Suggested customer plan', section_type: 'key_action', description: 'Mock suggestion', target_value: 'Plan approved', weight: 0.3 }],
      })
    }

    const submitGoalMatch = path.match(/^\/performance\/goal-records\/(\d+)\/submit$/)
    if (method === 'POST' && submitGoalMatch) {
      const participant = participants.get(Number(submitGoalMatch[1]))
      if (participant) participant.status = 'target_pending_approval'
      return json(route, { ok: true })
    }

    const approveGoalMatch = path.match(/^\/performance\/goal-records\/(\d+)\/approve$/)
    if (method === 'POST' && approveGoalMatch) {
      const participant = participants.get(Number(approveGoalMatch[1]))
      if (participant) participant.status = 'target_set'
      return json(route, { ok: true })
    }

    const rejectGoalMatch = path.match(/^\/performance\/goal-records\/(\d+)\/reject$/)
    if (method === 'POST' && rejectGoalMatch) {
      const participant = participants.get(Number(rejectGoalMatch[1]))
      if (participant) participant.status = 'target_rejected'
      return json(route, { ok: true })
    }

    const saveGoalMatch = path.match(/^\/performance\/goal-records\/(\d+)$/)
    if (method === 'POST' && saveGoalMatch) {
      return json(route, { ok: true })
    }

    const goalRecordsMatch = path.match(/^\/performance\/goal-records\/(\d+)$/)
    if (method === 'GET' && goalRecordsMatch) {
      const participantId = Number(goalRecordsMatch[1])
      return json(route, {
        items: goalRecords(participantId, {
          emptySelf: participantId === 202,
          emptyManager: participantId === 203,
        }),
      })
    }

    if (method === 'GET' && path === '/performance/indicator-items/search') {
      return json(route, {
        items: [{ id: 9001, name: 'Revenue retention', description: 'Library result', section_type: url.searchParams.get('section_type') || 'quantitative' }],
      })
    }

    const participantMatch = path.match(/^\/performance\/participants\/(\d+)$/)
    if (method === 'GET' && participantMatch) {
      const participantId = Number(participantMatch[1])
      const participant = participants.get(participantId)
      let activity: any
      if (participantId === 204) {
        activity = { ...activities.get(103), status: 'employee_confirmation', enable_bonus_score: true }
      } else if (participantId === 209) {
        activity = activities.get(109)
      } else if (participantId === 210) {
        activity = activities.get(110)
      } else if (participantId === 203) {
        activity = activities.get(103)
      } else {
        activity = activities.get(102)
      }
      return json(route, { participant, activity })
    }

    const participantLogsMatch = path.match(/^\/performance\/participants\/(\d+)\/relationship-change-logs$/)
    if (method === 'GET' && participantLogsMatch) {
      return json(route, { items: [], total: 0 })
    }

    const bonusPenaltyMatch = path.match(/^\/performance\/participants\/(\d+)\/bonus-penalty$/)
    if (method === 'POST' && bonusPenaltyMatch) {
      const participant = participants.get(Number(bonusPenaltyMatch[1]))
      if (participant) {
        participant.bonus_score = 0
        participant.penalty_score = 0
      }
      return json(route, { participant })
    }

    const legacyConfirmMatch = path.match(/^\/performance\/participants\/(\d+)\/confirm-result$/)
    if (method === 'POST' && legacyConfirmMatch) {
      const participant = participants.get(Number(legacyConfirmMatch[1]))
      if (participant) participant.status = 'result_confirmed'
      return json(route, { ok: true })
    }

    const selfEvalMatch = path.match(/^\/performance\/(?:goal-reviews|reviews|participants)\/(\d+)\/self-evaluation$/)
    if (method === 'POST' && selfEvalMatch) {
      const participant = participants.get(Number(selfEvalMatch[1]))
      if (participant) participant.status = 'self_submitted'
      return json(route, { ok: true })
    }

    const managerEvalMatch = path.match(/^\/performance\/(?:goal-reviews|reviews|participants)\/(\d+)\/manager-evaluation$/)
    if (method === 'POST' && managerEvalMatch) {
      const participant = participants.get(Number(managerEvalMatch[1]))
      if (participant) participant.status = 'manager_submitted'
      return json(route, { ok: true })
    }

    if (method === 'POST' && path === '/performance/auto-score') {
      return json(route, {
        items: [
          { record_id: 2031, score: 96, breakdown: 'mocked automatic score', auto_scored: true },
        ],
      })
    }

    const confirmMatch = path.match(/^\/performance\/participants\/(\d+)\/confirm-(employee|manager|hr)$/)
    if (method === 'POST' && confirmMatch) {
      const participant = participants.get(Number(confirmMatch[1]))
      if (participant) {
        participant.status = confirmMatch[2] === 'employee' ? 'employee_confirmed' : `${confirmMatch[2]}_confirmed`
      }
      return json(route, { ok: true })
    }

    // 上期结果：当前参与人 210 → 上期活动 109 / 参与人 209；其它默认空
    const previousResultMatch = path.match(/^\/performance\/participants\/(\d+)\/previous-result$/)
    if (method === 'GET' && previousResultMatch) {
      const participantId = Number(previousResultMatch[1])
      if (participantId === 210) {
        return json(route, {
          activity: activities.get(109),
          participant: participants.get(209),
          goal_records: goalRecords(209),
          versions: [],
        })
      }
      return json(route, {})
    }

    return json(route, { message: `Unhandled mock route: ${method} ${path}` }, 404)
  })
}

async function fillControl(page: Page, testId: string, value: string) {
  const control = page.getByTestId(testId)
  const nested = control.locator('input, textarea')
  if (await nested.count()) {
    await nested.first().fill(value)
    return
  }
  await control.fill(value)
}

test.describe('performance module', () => {
  test('renders overview, validates activity form, imports participants, and advances a stage', async ({ page }) => {
    await setupPerformanceMock(page)
    await page.goto('/performance-overview')

    await expect(page.getByTestId('performance-overview-page')).toBeVisible()
    await expect(page.getByText('Draft Cycle')).toBeVisible()
    await expect(page.getByText('Target Setting Cycle')).toBeVisible()

    await page.getByTestId('performance-create-activity').click()
    await expect(page.getByTestId('performance-activity-editor')).toBeVisible()

    await page.getByTestId('performance-editor-save').click()
    await expect(page.locator('.ant-form-item-explain-error').first()).toBeVisible()

    await page
      .getByTestId('performance-activity-editor')
      .locator('input[type="file"]')
      .setInputFiles({
        name: 'participants.xlsx',
        mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
        buffer: Buffer.from('mock workbook'),
      })
    await expect(page.getByTestId('performance-editor-activity-name')).toHaveValue('Imported Performance Cycle')

    await page.getByTestId('performance-editor-cancel').click()
    await expect(page.getByTestId('performance-activity-editor')).toBeHidden()

    await page.getByTestId('performance-activity-view-101').click()
    await expect(page.getByTestId('performance-detail-content')).toBeVisible()

    const openTargetResponse = page.waitForResponse(response =>
      response.url().includes('/performance/activities/101/open-target-setting') && response.request().method() === 'POST',
    )
    await page.getByTestId('performance-detail-open-target-101').click()
    // 阶段推进写操作有二次确认
    await page.getByRole('button', { name: '确认开启' }).click()
    await openTargetResponse

    await page.locator('.ant-drawer-close').click()
    await expect(page.locator('.ant-drawer-content-wrapper')).toBeHidden()

    await page.getByTestId('performance-activity-view-102').click()
    await expect(page.getByTestId('performance-detail-content')).toBeVisible()
    await expect(page.getByTestId('performance-participant-target-201')).toBeVisible()

    await page.getByTestId('performance-participant-target-201').click()
    await expect(page).toHaveURL(/\/performance-goal-setting\/102\/201/)
  })

  test('imports an Excel performance template through the four-step wizard', async ({ page }) => {
    await setupPerformanceMock(page)
    await page.goto('/performance-overview')

    await page.getByTestId('performance-import-excel').click()
    await expect(page.getByText('Excel 导入绩效活动')).toBeVisible()

    await page.locator('.ant-modal input[type="file"]').setInputFiles({
      name: 'xiaotie-performance.xlsx',
      mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      buffer: Buffer.from('mock xlsx content'),
    })
    await page.getByRole('button', { name: '开始识别' }).click()

    await expect(page.getByText('普通员工月度绩效模板')).toBeVisible()
    await expect(page.getByText('权重已按模板比例换算')).toBeVisible()
    await page.getByRole('button', { name: '去补充确认' }).click()

    await expect(page.getByText('请确认活动周期和日期')).toBeVisible()
    await page.getByRole('button', { name: '确认创建（1）' }).click()

    await expect(page.locator('.ant-result-title').filter({ hasText: '绩效模板和草稿活动已创建' })).toBeVisible()
    await expect(page.getByText('活动ID 401')).toBeVisible()
    await expect(page.getByText('目标 1 项')).toBeVisible()
  })

  test('disables protected controls and blocks guarded routes without operation permission', async ({ page }) => {
    await setupPerformanceMock(page, {
      permissions: ['performance:result:view'],
    })

    await page.goto('/performance-overview')
    await expect(page.getByTestId('performance-overview-page')).toBeVisible()
    await page.getByTestId('performance-activity-view-101').click()
    await expect(page.getByTestId('performance-detail-content')).toBeVisible()
    // 缺权仍展示按钮，但 disabled + Tooltip（不再 hide）
    const openTarget = page.getByTestId('performance-detail-open-target-101')
    await expect(openTarget).toBeVisible()
    await expect(openTarget).toBeDisabled()

    await page.goto('/performance-goal-setting/102/201')
    await expect(page.locator('.ant-result')).toBeVisible()
    await expect(page.getByTestId('performance-goal-setting-page')).toHaveCount(0)
  })

  test('edits goal records, saves a draft, loads suggestions, and submits target approval', async ({ page }) => {
    await setupPerformanceMock(page)
    await page.goto('/performance-goal-setting/102/201')

    await expect(page.getByTestId('performance-goal-setting-page')).toBeVisible()

    await page.getByTestId('performance-goal-load-suggestions').click()
    await expect(page.getByText('Suggested customer plan')).toBeVisible()

    const draftResponse = page.waitForResponse(response =>
      response.url().includes('/performance/goal-records/201') && response.request().method() === 'POST',
    )
    await page.getByTestId('performance-goal-save-draft').click()
    await draftResponse

    await page.getByTestId('performance-goal-submit').click()
    await expect(page.locator('.ant-modal-confirm')).toBeVisible()
    const submitResponse = page.waitForResponse(response =>
      response.url().includes('/performance/goal-records/201/submit') && response.request().method() === 'POST',
    )
    await page.locator('.ant-modal-confirm .ant-btn-primary').click()
    await submitResponse
  })

  test('submits self evaluation and manager evaluation with mocked auto scoring', async ({ page }) => {
    await setupPerformanceMock(page)

    await page.goto('/performance-overview')
    await page.goto('/performance-self-eval/102/202')
    await expect(page.getByTestId('performance-self-eval-page')).toBeVisible()
    await fillControl(page, 'performance-self-actual-0', 'Actual retention reached 110')
    await fillControl(page, 'performance-self-score-0', '92')
    await fillControl(page, 'performance-self-actual-1', 'Launch plan approved')
    await fillControl(page, 'performance-self-score-1', '88')
    await fillControl(page, 'performance-self-good', 'Delivered the critical milestones')
    await fillControl(page, 'performance-self-improvement', 'Improve stakeholder previews')

    const selfResponse = page.waitForResponse(response =>
      response.url().includes('/performance/goal-reviews/202/self-evaluation') && response.request().method() === 'POST',
    )
    await page.getByTestId('performance-self-submit').click()
    await selfResponse
    await expect(page).toHaveURL(/\/performance-overview/)

    await page.goto('/performance-manager-eval/103/203')
    await expect(page.getByTestId('performance-manager-eval-page')).toBeVisible()

    const autoScoreResponse = page.waitForResponse(response =>
      response.url().includes('/performance/auto-score') && response.request().method() === 'POST',
    )
    await page.getByTestId('performance-manager-auto-score').click()
    await autoScoreResponse

    await fillControl(page, 'performance-manager-score-1', '88')
    await fillControl(page, 'performance-manager-good', 'Strong ownership')
    await fillControl(page, 'performance-manager-improvement', 'Tighten cross-team risk tracking')

    const managerResponse = page.waitForResponse(response =>
      response.url().includes('/performance/goal-reviews/203/manager-evaluation') && response.request().method() === 'POST',
    )
    await page.getByTestId('performance-manager-submit').click()
    await managerResponse
  })

  test('confirms a performance result from the result page', async ({ page }) => {
    await setupPerformanceMock(page)
    await page.goto('/performance-result/103/204')

    await expect(page.getByTestId('performance-result-page')).toBeVisible()

    const confirmResponse = page.waitForResponse(response =>
      response.url().includes('/performance/participants/204/confirm-employee') && response.request().method() === 'POST',
    )
    await page.getByTestId('performance-result-confirm-employee').click()
    await confirmResponse
  })

  // ---------- 高风险入口：原手工 3 条，现自动化 ----------

  test('empty menuKeys: home shows no-role empty state and does not 403-loop', async ({ page }) => {
    await setupPerformanceMock(page, {
      menuKeys: [],
      permissions: [],
    })

    await page.goto('/')
    await expect(page.getByText('暂无数据权限')).toBeVisible()
    await expect(page.getByText('您尚未被分配任何角色，请联系管理员配置权限后再使用系统功能。')).toBeVisible()
    await expect(page.getByText('无访问权限')).toHaveCount(0)

    // 其它页 403 后返回首页应回到友好空态，而不是再次死循环
    await page.goto('/attendance')
    await expect(page.getByText('无访问权限')).toBeVisible()
    await page.getByRole('button', { name: '返回首页' }).click()
    await expect(page).toHaveURL(/\/$|\/\?/)
    await expect(page.getByText('暂无数据权限')).toBeVisible()
    await expect(page.getByText('无访问权限')).toHaveCount(0)
  })

  test('deep-link self-eval works with feature permission only (no overview menu)', async ({ page }) => {
    await setupPerformanceMock(page, {
      menuKeys: ['menu:home'],
      permissions: ['performance:self_eval:submit'],
    })

    await page.goto('/performance-self-eval/102/202')
    await expect(page.getByTestId('performance-self-eval-page')).toBeVisible()
    await expect(page.getByText('无访问权限')).toHaveCount(0)

    // 列表总览仍要求菜单
    await page.goto('/performance-overview')
    await expect(page.getByText('无访问权限')).toBeVisible()
  })

  test('previous-result navigates to previous activity ids, not current', async ({ page }) => {
    await setupPerformanceMock(page)
    await page.goto('/performance-overview')
    await expect(page.getByTestId('performance-overview-page')).toBeVisible()
    await expect(page.getByText('Muteng Current Cycle')).toBeVisible()

    await page.getByTestId('performance-activity-view-110').click()
    await expect(page.getByTestId('performance-detail-content')).toBeVisible()

    const previousResponse = page.waitForResponse(response =>
      response.url().includes('/performance/participants/210/previous-result')
      && response.request().method() === 'GET'
      && response.ok(),
    )
    await page.getByTestId('performance-participant-previous-result-210').click()
    await previousResponse

    await expect(page).toHaveURL(/\/performance-result\/109\/209/)
    await expect(page).not.toHaveURL(/\/performance-result\/110\/210/)
    await expect(page.getByTestId('performance-result-page')).toBeVisible()
  })
})
