/**
 * PerformanceOverview 组件交互测试
 *
 * 覆盖真实渲染 + 用户交互场景：
 * - 页面初始渲染与数据加载
 * - 活动列表展示与状态标签
 * - 搜索与筛选交互
 * - 统计卡片点击筛选
 * - 活动操作按钮（按状态）
 * - 权限控制（按钮禁用/启用）
 */
import React from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { message, Modal } from 'antd'
import PerformanceOverview from './PerformanceOverview'
import { useAuthStore } from '../store/authStore'
import { hasPermission } from '../utils/permission'

/** 模块加载时捕获真实 Modal.confirm，避免 spy 后 beforeEach 递归 */
const realModalConfirm = Modal.confirm.bind(Modal)

// ==================== jsdom Polyfills for Antd ====================

// jsdom 缺少 matchMedia，antd Row/Col 的 useBreakpoint 依赖它
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

// jsdom 缺少 getComputedStyle 对伪元素的支持
const originalGetComputedStyle = window.getComputedStyle
window.getComputedStyle = (elt: Element, pseudoElt?: string | null) => {
  if (pseudoElt) {
    return {} as CSSStyleDeclaration
  }
  return originalGetComputedStyle(elt)
}

// jsdom 的 Element 没有 scrollIntoView
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = vi.fn()
}

// ==================== Mocks ====================

// Mock react-router-dom
const mockNavigate = vi.fn()
vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  BrowserRouter: ({ children }: any) => <div>{children}</div>,
  Routes: ({ children }: any) => <div>{children}</div>,
  Route: ({ children }: any) => <div>{children}</div>,
  Link: ({ children, ...props }: any) => <a {...props}>{children}</a>,
  NavLink: ({ children, ...props }: any) => <a {...props}>{children}</a>,
}))

// Mock 权限 — 默认全部通过
vi.mock('../utils/permission', () => ({
  hasPermission: vi.fn(() => true),
  hasMenuPermission: vi.fn(() => true),
}))

// Mock performanceAPI
const mockGetActivities = vi.fn()
const mockGetParticipants = vi.fn()
const mockGetMyParticipants = vi.fn()
const mockGetParticipantVersions = vi.fn()
const mockGetResultSummary = vi.fn()
const mockGetDistributionCheck = vi.fn()
const mockGetDistributionRules = vi.fn()
const mockGetHRConfirmDeadlineStatus = vi.fn()
const mockGetScopeOptions = vi.fn()
const mockGetIndicatorLibraries = vi.fn()
const mockGetTemplates = vi.fn()
const mockStartActivity = vi.fn()
const mockOpenSelfEvaluation = vi.fn()
const mockOpenManagerEvaluation = vi.fn()
const mockOpenTargetSetting = vi.fn()
const mockOpenEmployeeConfirmation = vi.fn()
const mockOpenManagerConfirmation = vi.fn()
const mockOpenHRConfirmation = vi.fn()
const mockOpenHRReview = vi.fn()
const mockOpenResultPublish = vi.fn()
const mockLockActivity = vi.fn()
const mockArchiveActivity = vi.fn()
const mockPublishActivity = vi.fn()
const mockSendSelfEvalReminder = vi.fn()
const mockSendManagerEvalReminder = vi.fn()
const mockSendHRConfirmReminder = vi.fn()
const mockForceLockOverdueHR = vi.fn()
const mockCreateActivity = vi.fn()
const mockUpdateActivity = vi.fn()
const mockImportParticipants = vi.fn()
const mockCloseActivity = vi.fn()
const mockGetActivity = vi.fn()
const mockApproveGoalRecords = vi.fn()
const mockRejectGoalRecords = vi.fn()
const mockConfirmHRResult = vi.fn()
const mockBatchSubmitManagerEvaluations = vi.fn()
const mockUpdateAssessmentManager = vi.fn()
const mockBatchUpdateAssessmentManagers = vi.fn()
const mockGetAssessmentManagerCandidates = vi.fn()
const mockPutDistributionRules = vi.fn()
const mockGetPreviousParticipantResult = vi.fn()

vi.mock('../services/api', () => ({
  performanceAPI: {
    getActivities: (...args: any[]) => mockGetActivities(...args),
    getScopeOptions: (...args: any[]) => mockGetScopeOptions(...args),
    getParticipants: (...args: any[]) => mockGetParticipants(...args),
    getMyParticipants: (...args: any[]) => mockGetMyParticipants(...args),
    getParticipantVersions: (...args: any[]) => mockGetParticipantVersions(...args),
    getResultSummary: (...args: any[]) => mockGetResultSummary(...args),
    getDistributionCheck: (...args: any[]) => mockGetDistributionCheck(...args),
    getDistributionRules: (...args: any[]) => mockGetDistributionRules(...args),
    getHRConfirmDeadlineStatus: (...args: any[]) => mockGetHRConfirmDeadlineStatus(...args),
    getIndicatorLibraries: (...args: any[]) => mockGetIndicatorLibraries(...args),
    getTemplates: (...args: any[]) => mockGetTemplates(...args),
    startActivity: (...args: any[]) => mockStartActivity(...args),
    openSelfEvaluation: (...args: any[]) => mockOpenSelfEvaluation(...args),
    openManagerEvaluation: (...args: any[]) => mockOpenManagerEvaluation(...args),
    openTargetSetting: (...args: any[]) => mockOpenTargetSetting(...args),
    openEmployeeConfirmation: (...args: any[]) => mockOpenEmployeeConfirmation(...args),
    openManagerConfirmation: (...args: any[]) => mockOpenManagerConfirmation(...args),
    openHRConfirmation: (...args: any[]) => mockOpenHRConfirmation(...args),
    openHRReview: (...args: any[]) => mockOpenHRReview(...args),
    openResultPublish: (...args: any[]) => mockOpenResultPublish(...args),
    lockActivity: (...args: any[]) => mockLockActivity(...args),
    archiveActivity: (...args: any[]) => mockArchiveActivity(...args),
    publishActivity: (...args: any[]) => mockPublishActivity(...args),
    sendSelfEvalReminder: (...args: any[]) => mockSendSelfEvalReminder(...args),
    sendManagerEvalReminder: (...args: any[]) => mockSendManagerEvalReminder(...args),
    sendHRConfirmReminder: (...args: any[]) => mockSendHRConfirmReminder(...args),
    forceLockOverdueHR: (...args: any[]) => mockForceLockOverdueHR(...args),
    createActivity: (...args: any[]) => mockCreateActivity(...args),
    updateActivity: (...args: any[]) => mockUpdateActivity(...args),
    importParticipants: (...args: any[]) => mockImportParticipants(...args),
    closeActivity: (...args: any[]) => mockCloseActivity(...args),
    getActivity: (...args: any[]) => mockGetActivity(...args),
    approveGoalRecords: (...args: any[]) => mockApproveGoalRecords(...args),
    rejectGoalRecords: (...args: any[]) => mockRejectGoalRecords(...args),
    confirmHRResult: (...args: any[]) => mockConfirmHRResult(...args),
    batchSubmitManagerEvaluations: (...args: any[]) => mockBatchSubmitManagerEvaluations(...args),
    updateAssessmentManager: (...args: any[]) => mockUpdateAssessmentManager(...args),
    batchUpdateAssessmentManagers: (...args: any[]) => mockBatchUpdateAssessmentManagers(...args),
    getAssessmentManagerCandidates: (...args: any[]) => mockGetAssessmentManagerCandidates(...args),
    putDistributionRules: (...args: any[]) => mockPutDistributionRules(...args),
    getPreviousParticipantResult: (...args: any[]) => mockGetPreviousParticipantResult(...args),
  },

}))

// Mock PerformanceActivityEditor — 简化为一个 placeholder，但暴露 userOptions 供范围断言
vi.mock('../components/PerformanceActivityEditor', () => ({
  default: ({ visible, onSave, onCancel, userOptions }: any) =>
    visible ? (
      <div data-testid="activity-editor-mock">
        <div data-testid="activity-editor-user-options">
          {(userOptions || []).map((option: any) => (
            <div
              key={String(option.value)}
              data-testid={`activity-editor-user-option-${option.value}`}
            >
              {option.label}
            </div>
          ))}
        </div>
        <button data-testid="activity-editor-save" onClick={onSave}>保存</button>
        <button data-testid="activity-editor-cancel" onClick={onCancel}>取消</button>
      </div>
    ) : null,
}))

// ==================== Mock 数据 ====================

const MOCK_ACTIVITIES = [
  {
    id: 1,
    name: '2026年Q2绩效',
    cycle_type: 'quarterly',
    status: 'self_evaluation',
    start_date: '2026-04-01',
    end_date: '2026-06-30',
    self_eval_start_at: '2026-05-01',
    self_eval_end_at: '2026-05-15',
    manager_eval_start_at: '2026-05-16',
    manager_eval_end_at: '2026-05-31',
    result_confirm_start_at: '2026-06-01',
    result_confirm_end_at: '2026-06-15',
    target_employee_ids: ['U001', 'U002'],
    target_department_ids: ['D001'],
    manager_assignments: [],
    default_assessment_manager_source: 'DIRECT_MANAGER',
    indicator_library_id: 1,
    description: '',
    enable_bonus_score: false,
    strict_time_mode: false,
  },
  {
    id: 2,
    name: '2026年月度考核-5月',
    cycle_type: 'monthly',
    status: 'draft',
    start_date: '2026-05-01',
    end_date: '2026-05-31',
    self_eval_start_at: '2026-05-20',
    self_eval_end_at: '2026-05-25',
    manager_eval_start_at: '2026-05-26',
    manager_eval_end_at: '2026-05-31',
    result_confirm_start_at: '2026-06-01',
    result_confirm_end_at: '2026-06-05',
    target_employee_ids: [],
    target_department_ids: [],
    manager_assignments: [],
    default_assessment_manager_source: 'DIRECT_MANAGER',
    indicator_library_id: null,
    description: '',
    enable_bonus_score: false,
    strict_time_mode: false,
  },
  {
    id: 3,
    name: '2026年Q1绩效',
    cycle_type: 'quarterly',
    status: 'locked',
    start_date: '2026-01-01',
    end_date: '2026-03-31',
    self_eval_start_at: '2026-04-01',
    self_eval_end_at: '2026-04-10',
    manager_eval_start_at: '2026-04-11',
    manager_eval_end_at: '2026-04-20',
    result_confirm_start_at: '2026-04-21',
    result_confirm_end_at: '2026-04-30',
    target_employee_ids: [],
    target_department_ids: [],
    manager_assignments: [],
    default_assessment_manager_source: 'DIRECT_MANAGER',
    indicator_library_id: null,
    description: '',
    enable_bonus_score: false,
    strict_time_mode: false,
  },
  {
    id: 4,
    name: '2025年Q4绩效',
    cycle_type: 'quarterly',
    status: 'archived',
    start_date: '2025-10-01',
    end_date: '2025-12-31',
    self_eval_start_at: '2026-01-01',
    self_eval_end_at: '2026-01-10',
    manager_eval_start_at: '2026-01-11',
    manager_eval_end_at: '2026-01-20',
    result_confirm_start_at: '2026-01-21',
    result_confirm_end_at: '2026-01-31',
    target_employee_ids: [],
    target_department_ids: [],
    manager_assignments: [],
    default_assessment_manager_source: 'DIRECT_MANAGER',
    indicator_library_id: null,
    description: '',
    enable_bonus_score: false,
    strict_time_mode: false,
  },
]

// ==================== 辅助函数 ====================

function setupDefaultMocks() {
  mockGetActivities.mockResolvedValue({
    data: { items: MOCK_ACTIVITIES, total: 4 },
  })
  mockGetMyParticipants.mockResolvedValue({ data: { items_by_activity: {} } })
  mockGetParticipants.mockResolvedValue({ data: { items: [], total: 0 } })
  mockGetParticipantVersions.mockResolvedValue({ data: { versions: [] } })
  mockGetResultSummary.mockResolvedValue({ data: null })
  mockGetDistributionCheck.mockResolvedValue({ data: null })
  mockGetDistributionRules.mockResolvedValue({ data: { rules: [] } })
  mockGetHRConfirmDeadlineStatus.mockRejectedValue(new Error('not found'))
  mockGetActivity.mockImplementation((activityId: number) => ({
    data: { activity: MOCK_ACTIVITIES.find(activity => activity.id === activityId) || MOCK_ACTIVITIES[0] },
  }))
  mockGetScopeOptions.mockResolvedValue({ data: { departments: [], employees: [], warnings: [] } })
  mockGetIndicatorLibraries.mockResolvedValue({ data: { items: [] } })
  mockGetTemplates.mockResolvedValue({
    data: {
      items: [
        { id: 1, name: '旧绩效流程模板', flow_type: 'old', status: 'active' },
        { id: 2, name: '新绩效流程模板', flow_type: 'new', status: 'active' },
      ],
    },
  })
}

function renderOverview() {
  return render(<PerformanceOverview />)
}

function collectReactText(node: React.ReactNode): string {
  if (node === null || node === undefined || typeof node === 'boolean') return ''
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(collectReactText).join('')
  if (React.isValidElement(node)) return collectReactText((node.props as { children?: React.ReactNode }).children)
  return ''
}

async function openActivityDetail(user: ReturnType<typeof userEvent.setup>, activityId: number) {
  await user.click(screen.getByTestId(`performance-activity-view-${activityId}`))
  await waitFor(() => {
    expect(screen.getByTestId('performance-detail-content')).toBeInTheDocument()
  })
}

// ==================== 测试 ====================

describe('PerformanceOverview 组件交互测试', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // 仅对「简单二次确认」自动点确定；带表单的 Modal.confirm（调整进度/部门评分等）走真实实现
    vi.spyOn(Modal, 'confirm').mockImplementation((config: any) => {
      const title = String(config?.title || '')
      const isSimpleWriteGuard =
        /^(开启|锁定|归档|关闭|确认结果|直接开启自评|通过目标|HR审核|HR确认|逾期强制锁定|发送)/.test(title)
      if (isSimpleWriteGuard) {
        // 吞掉 onOk 拒绝，避免 mock 路径产生 unhandled rejection（真实 antd 会处理）
        void Promise.resolve(config?.onOk?.()).catch(() => undefined)
        return {
          destroy: vi.fn(),
          update: vi.fn(),
          then: (resolve: any) => Promise.resolve().then(resolve),
        } as any
      }
      return realModalConfirm(config)
    })
    vi.mocked(hasPermission).mockImplementation(() => true)
    useAuthStore.setState({
      user: null,
      token: '',
      isLoggedIn: false,
      menuKeys: [],
      permissions: [],
    })
    setupDefaultMocks()
  })

  describe('HR confirm reminder', () => {
    it('reads a nested API response without treating pending users as zero', async () => {
      const user = userEvent.setup()
      const successSpy = vi.spyOn(message, 'success')
      mockGetActivities.mockResolvedValue({
        data: {
          items: [{
            ...MOCK_ACTIVITIES[0],
            id: 6,
            name: 'HR confirm activity',
            status: 'hr_confirmation',
          }],
          total: 1,
        },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [{
            id: 601,
            employee_name: 'Alice',
            department_name: 'Product',
            position: 'Specialist',
            status: 'manager_confirmed',
            manager_config_status: 'CONFIGURED',
            manager_id: 'M001',
          }],
          total: 1,
        },
      })
      mockGetResultSummary.mockResolvedValue({
        data: {
          total_participants: 1,
          self_submitted_count: 1,
          manager_submitted_count: 1,
          result_confirmed_count: 0,
        },
      })
      mockGetHRConfirmDeadlineStatus.mockResolvedValue({
        data: {
          deadline: '',
          pending_count: 1,
          overdue: false,
          can_force_lock: false,
        },
      })
      mockSendHRConfirmReminder.mockResolvedValue({
        data: {
          data: {
            pending: 1,
            candidates: 1,
            sent: 1,
            skipped: 0,
            failed: 0,
          },
        },
      })

      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('HR confirm activity')).toBeInTheDocument()
      })

      await openActivityDetail(user, 6)
      const remindBtn = await screen.findByTestId('performance-detail-remind-hr-6')
      await user.click(remindBtn)
      // 提醒类写操作有 Modal.confirm；mock 会自动点确定，这里再兜底点一次
      const confirmBtn = screen.queryByRole('button', { name: '确认发送' })
      if (confirmBtn) await user.click(confirmBtn)

      await waitFor(() => {
        expect(mockSendHRConfirmReminder).toHaveBeenCalledWith(6)
      }, { timeout: 10_000 })
      await waitFor(() => {
        expect(successSpy).toHaveBeenCalled()
      })
      expect(String(successSpy.mock.calls[0]?.[0] || '')).toContain('1')
    }, 20_000)
  })

  describe('沐腾科技 HR审核动作', () => {
    it('HR权限按参与人独立审核，不再开启活动级HR审核', async () => {
      const user = userEvent.setup()
      const activity = {
        ...MOCK_ACTIVITIES[0],
        id: 7,
        name: 'New flow department review',
        flow_type: 'new',
        activity_kind: 'review_scoring',
        status: 'department_evaluation',
      }
      vi.mocked(hasPermission).mockImplementation((code: string) =>
        code === 'performance:hr_review:submit' || code === 'performance:result:view',
      )
      mockGetActivities.mockResolvedValue({
        data: { items: [activity], total: 1 },
      })
      mockGetActivity.mockResolvedValue({
        data: { activity },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [
            {
              id: 701,
              employee_name: 'Alice',
              department_name: 'Product',
              position: 'Specialist',
              status: 'manager_confirmed',
              manager_config_status: 'CONFIGURED',
              manager_id: 'M001',
            },
            {
              id: 702,
              employee_name: 'Bob',
              department_name: 'Product',
              position: 'Specialist',
              status: 'self_submitted',
              manager_config_status: 'CONFIGURED',
              manager_id: 'M001',
            },
          ],
          total: 2,
        },
      })
      mockGetResultSummary.mockResolvedValue({
        data: {
          total_participants: 2,
          self_submitted_count: 2,
          manager_submitted_count: 1,
          result_confirmed_count: 0,
        },
      })
      mockConfirmHRResult.mockResolvedValue({ data: { message: 'ok' } })

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('New flow department review')).toBeInTheDocument()
      })

      await openActivityDetail(user, 7)
      expect(screen.queryByTestId('performance-detail-open-hr-review-7')).not.toBeInTheDocument()
      const hrReviewButton = await screen.findByTestId('performance-participant-hr-confirm-701')
      expect(screen.queryByTestId('performance-participant-hr-confirm-702')).not.toBeInTheDocument()
      await user.click(hrReviewButton)

      await waitFor(() => {
        expect(mockConfirmHRResult).toHaveBeenCalledWith(701)
      })
    })

    it('系统未公布的参与人不显示手工屏蔽，已公布参与人才显示', async () => {
      const user = userEvent.setup()
      const activity = {
        ...MOCK_ACTIVITIES[0],
        id: 8,
        name: 'New flow HR review',
        flow_type: 'new',
        activity_kind: 'review_scoring',
        status: 'hr_review',
      }
      vi.mocked(hasPermission).mockImplementation((code: string) =>
        code === 'performance:result_visibility:manage' || code === 'performance:result:view',
      )
      mockGetActivities.mockResolvedValue({
        data: { items: [activity], total: 1 },
      })
      mockGetActivity.mockResolvedValue({
        data: { activity },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [
            {
              id: 801,
              employee_name: 'Alice',
              department_name: 'Product',
              position: 'Specialist',
              status: 'manager_confirmed',
              result_hidden: true,
              result_hidden_reason: 'system:unpublished',
              manager_config_status: 'CONFIGURED',
              manager_id: 'M001',
            },
            {
              id: 802,
              employee_name: 'Bob',
              department_name: 'Product',
              position: 'Specialist',
              status: 'hr_confirmed',
              result_hidden: false,
              manager_config_status: 'CONFIGURED',
              manager_id: 'M001',
            },
          ],
          total: 2,
        },
      })
      mockGetResultSummary.mockResolvedValue({
        data: {
          total_participants: 2,
          self_submitted_count: 2,
          manager_submitted_count: 1,
          result_confirmed_count: 1,
        },
      })

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('New flow HR review')).toBeInTheDocument()
      })

      await openActivityDetail(user, 8)

      expect(screen.queryByTestId('performance-participant-visibility-801')).not.toBeInTheDocument()
      expect(await screen.findByTestId('performance-participant-visibility-802')).toBeInTheDocument()
    })

    it('只有结果公布权限但没有屏蔽管理权限时不显示屏蔽按钮', async () => {
      const user = userEvent.setup()
      const activity = {
        ...MOCK_ACTIVITIES[0],
        id: 9,
        name: 'New flow publish permission only',
        flow_type: 'new',
        activity_kind: 'review_scoring',
        status: 'hr_review',
      }
      vi.mocked(hasPermission).mockImplementation((code: string) =>
        code === 'performance:result_publish:manage' || code === 'performance:result:view',
      )
      mockGetActivities.mockResolvedValue({
        data: { items: [activity], total: 1 },
      })
      mockGetActivity.mockResolvedValue({
        data: { activity },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [{
            id: 901,
            employee_name: 'Alice',
            department_name: 'Product',
            position: 'Specialist',
            status: 'manager_submitted',
            manager_config_status: 'CONFIGURED',
            manager_id: 'M001',
          }],
          total: 1,
        },
      })
      mockGetResultSummary.mockResolvedValue({
        data: {
          total_participants: 1,
          self_submitted_count: 1,
          manager_submitted_count: 1,
          result_confirmed_count: 0,
        },
      })

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('New flow publish permission only')).toBeInTheDocument()
      })

      await openActivityDetail(user, 9)

      expect(screen.queryByTestId('performance-participant-visibility-901')).not.toBeInTheDocument()
    })

    it('结果公布后应直接归档且不再开启绩效面谈或申诉', async () => {
      const user = userEvent.setup()
      const activity = {
        ...MOCK_ACTIVITIES[0],
        id: 13,
        name: 'New flow result published',
        flow_type: 'new',
        activity_kind: 'review_scoring',
        status: 'result_publish',
      }
      vi.mocked(hasPermission).mockImplementation((code: string) =>
        code === 'performance:activity:manage' || code === 'performance:result:view',
      )
      mockGetActivities.mockResolvedValue({
        data: { items: [activity], total: 1 },
      })
      mockGetActivity.mockResolvedValue({
        data: { activity },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [{
            id: 1301,
            employee_name: 'Alice',
            department_name: 'Product',
            position: 'Specialist',
            status: 'hr_confirmed',
            manager_config_status: 'CONFIGURED',
            manager_id: 'M001',
          }],
          total: 1,
        },
      })
      mockGetResultSummary.mockResolvedValue({
        data: {
          total_participants: 1,
          self_submitted_count: 1,
          manager_submitted_count: 1,
          result_confirmed_count: 1,
        },
      })
      mockArchiveActivity.mockResolvedValue({ data: { message: 'ok' } })

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('New flow result published')).toBeInTheDocument()
      })

      await openActivityDetail(user, 13)
      expect(screen.queryByTestId('performance-detail-open-interview-13')).not.toBeInTheDocument()
      expect(screen.queryByTestId('performance-detail-open-appeal-13')).not.toBeInTheDocument()
      await user.click(await screen.findByTestId('performance-detail-archive-13'))

      await waitFor(() => {
        expect(mockArchiveActivity).toHaveBeenCalledWith(13)
      })
    })
  })

  // ==================== 场景 1: 页面初始渲染 ====================
  describe('页面初始渲染', () => {
    it('应渲染页面标题 "绩效管理"', async () => {
      renderOverview()
      expect(screen.getByTestId('performance-overview-page')).toBeInTheDocument()
    })

    it('应渲染副标题 "绩效活动管理与评分工作台"', async () => {
      renderOverview()
      expect(screen.getByTestId('performance-overview-card')).toBeInTheDocument()
    })

    it('应显示精简筛选芯片（原统计卡片）', async () => {
      renderOverview()
      expect(screen.getByTestId('performance-stat-filters')).toBeInTheDocument()
      expect(screen.getByText('绩效活动总数')).toBeInTheDocument()
      expect(screen.getByText('进行中活动')).toBeInTheDocument()
      expect(screen.getByText('已确认结果')).toBeInTheDocument()
      expect(screen.getByText('已归档活动')).toBeInTheDocument()
    })

    it('应显示 "新建活动" 和 "刷新" 按钮', async () => {
      renderOverview()
      expect(screen.getByTestId('performance-create-activity')).toBeInTheDocument()
      expect(screen.getByTestId('performance-refresh-activities')).toBeInTheDocument()
    })

    it('应在挂载时调用 getActivities 加载数据', async () => {
      renderOverview()
      await waitFor(() => {
        expect(mockGetActivities).toHaveBeenCalled()
      })
    })
  })

  // ==================== 场景 2: 活动列表加载与展示 ====================
  describe('活动列表展示', () => {
    it('加载后表格应显示活动名称', async () => {
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })
      expect(screen.getByText('2026年月度考核-5月')).toBeInTheDocument()
      expect(screen.getByText('2026年Q1绩效')).toBeInTheDocument()
    })

    it('活动列表应按创建时选择的绩效模板展示', async () => {
      mockGetActivities.mockResolvedValue({
        data: {
          items: [{
            ...MOCK_ACTIVITIES[0],
            id: 19,
            name: '模板化绩效活动',
            template_id: 2,
            flow_type: 'new',
          }],
          total: 1,
        },
      })

      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('模板化绩效活动')).toBeInTheDocument()
      })
      expect(screen.getByText('沐腾科技流程模版')).toBeInTheDocument()
    })

    it('不同状态应显示不同标签文本', async () => {
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })
      // self_evaluation 状态 → "自评中"
      expect(screen.getByText('自评中')).toBeInTheDocument()
      // draft 状态 → "草稿"
      expect(screen.getByText('草稿')).toBeInTheDocument()
      // locked 状态 → "已锁定"
      expect(screen.getByText('已锁定')).toBeInTheDocument()
    })

    it('空数据时表格应正常渲染', async () => {
      mockGetActivities.mockResolvedValue({ data: { items: [], total: 0 } })
      renderOverview()
      await waitFor(() => {
        expect(mockGetActivities).toHaveBeenCalled()
      })
      // 不应崩溃，表格应存在（空状态）
      expect(screen.getByText('绩效活动')).toBeInTheDocument()
    })

    it('周期类型应显示中文标签', async () => {
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })
      // quarterly → 季度（出现多次，Q2 和 Q1 都是季度）, monthly → 月度
      expect(screen.getAllByText('季度').length).toBeGreaterThanOrEqual(2)
      expect(screen.getByText('月度')).toBeInTheDocument()
    })
  })

  // ==================== 场景 3: 搜索与筛选交互 ====================
  describe('搜索与筛选', () => {
    it('在搜索框输入文字后表格应过滤', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      // Input.Search 的 onSearch 需要按 Enter 才触发
      const searchInput = screen.getByPlaceholderText('搜索活动名称')
      await user.type(searchInput, 'Q2{Enter}')

      // "Q2" 匹配 "2026年Q2绩效" 和 "2026年Q1绩效"（都含 Q），但不匹配月度考核
      expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      expect(screen.queryByText('2026年月度考核-5月')).not.toBeInTheDocument()
    })

    it('清空搜索后应恢复全部显示', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      const searchInput = screen.getByPlaceholderText('搜索活动名称')
      await user.type(searchInput, 'Q2{Enter}')

      // 先确认过滤生效
      expect(screen.queryByText('2026年月度考核-5月')).not.toBeInTheDocument()

      // 清空输入框（触发 onChange → setActivitySearchText('')）
      await user.clear(searchInput)

      // 全部恢复
      expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      expect(screen.getByText('2026年月度考核-5月')).toBeInTheDocument()
      expect(screen.getByText('2026年Q1绩效')).toBeInTheDocument()
    })

    it('状态筛选由顶部芯片承担（列表区仅保留搜索）', async () => {
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      // P1：去掉列表区重复的状态 Select，筛选统一走 performance-stat-filters 芯片
      expect(screen.getByTestId('performance-stat-filters')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('搜索活动名称')).toBeInTheDocument()
      expect(screen.queryByText('筛选状态')).not.toBeInTheDocument()
    })
  })

  // ==================== 场景 4: 统计卡片点击筛选 ====================
  describe('统计卡片点击筛选', () => {
    it('点击 "进行中活动" 卡片应设置筛选', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      // 点击 "进行中活动" 卡片
      const inProgressCard = screen.getByLabelText('查看进行中活动')
      await user.click(inProgressCard)

      // 进行中状态包含 self_evaluation，所以 Q2 绩效应显示
      expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      // draft 不在进行中列表，月度考核不应显示
      expect(screen.queryByText('2026年月度考核-5月')).not.toBeInTheDocument()
      // locked 不在进行中列表
      expect(screen.queryByText('2026年Q1绩效')).not.toBeInTheDocument()
    })

    it('点击 "已归档活动" 卡片应只显示归档活动', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      const archivedCard = screen.getByLabelText('查看已归档活动')
      await user.click(archivedCard)

      // 只有 archived 状态的 2025年Q4 绩效应显示
      expect(screen.getByText('2025年Q4绩效')).toBeInTheDocument()
      expect(screen.queryByText('2026年Q2绩效')).not.toBeInTheDocument()
      expect(screen.queryByText('2026年月度考核-5月')).not.toBeInTheDocument()
      expect(screen.queryByText('2026年Q1绩效')).not.toBeInTheDocument()
    })

    it('点击 "已确认结果" 卡片应显示 locked + result_confirmed', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      const confirmedCard = screen.getByLabelText('查看已确认结果')
      await user.click(confirmedCard)

      // locked 在已确认列表中
      expect(screen.getByText('2026年Q1绩效')).toBeInTheDocument()
      // self_evaluation 不在已确认列表
      expect(screen.queryByText('2026年Q2绩效')).not.toBeInTheDocument()
    })
  })

  // ==================== 场景 5: 活动操作按钮 ====================
  describe('活动操作按钮', () => {
    it('draft 状态活动列表应展示阶段主操作「开启目标设定」', async () => {
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年月度考核-5月')).toBeInTheDocument()
      })

      // draft 活动 id=2：列表露出当前阶段主操作 + 详情
      expect(screen.getByTestId('performance-activity-view-2')).toBeInTheDocument()
      expect(screen.getByTestId('performance-activity-open-target-2')).toBeInTheDocument()
      expect(screen.queryByTestId('performance-activity-edit-2')).not.toBeInTheDocument()
      expect(screen.queryByTestId('performance-activity-refresh-2')).not.toBeInTheDocument()
    })

    it('self_evaluation 状态活动列表应展示「开启主管评分」主操作', async () => {
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      // self_evaluation 活动 id=1：列表主操作开启主管评分；提醒仍在详情
      expect(screen.getByTestId('performance-activity-view-1')).toBeInTheDocument()
      expect(screen.getByTestId('performance-activity-open-manager-1')).toBeInTheDocument()
      expect(screen.queryByTestId('performance-activity-notify-self-1')).not.toBeInTheDocument()
    })

    it('当前用户是活动参与人时应在列表直接显示自评入口并跳转', async () => {
      const user = userEvent.setup()
      useAuthStore.setState({
        user: { user_id: 'U001', employee_id: 'U001', name: '张三' },
        token: 'test-token',
        isLoggedIn: true,
        menuKeys: [],
        permissions: [],
      })
      mockGetActivities.mockResolvedValue({
        data: {
          items: [
            {
              ...MOCK_ACTIVITIES[0],
              my_participant: {
                id: 101,
                activity_id: 1,
                employee_id: 'U001',
                employee_name: '张三',
                department_id: 'D001',
                department_name: '技术部',
                position: '工程师',
                level: '',
                employee_status: 'active',
                status: 'target_set',
                self_score: 0,
                self_level: '',
                self_summary: '',
                manager_score: 0,
                manager_comment: '',
                suggested_level: '',
                final_level: '',
                adjust_reason: '',
                confirmed_by: '',
                created_at: '',
                updated_at: '',
              },
            },
          ],
          total: 1,
        },
      })

      renderOverview()

      await waitFor(() => {
        expect(screen.getByTestId('performance-activity-my-self-1')).toBeInTheDocument()
      })
      expect(mockGetMyParticipants).not.toHaveBeenCalled()
      expect(mockGetParticipants).not.toHaveBeenCalled()

      await user.click(screen.getByTestId('performance-activity-my-self-1'))
      expect(mockNavigate).toHaveBeenCalledWith('/performance-self-eval/1/101')
    })

    it('locked 状态活动列表应显示「归档」主操作', async () => {
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q1绩效')).toBeInTheDocument()
      })

      // locked 活动 id=3：列表露出归档主操作
      expect(screen.getByTestId('performance-activity-view-3')).toBeInTheDocument()
      expect(screen.getByTestId('performance-activity-archive-3')).toBeInTheDocument()
    })

    it('所有活动都应显示 "详情" 按钮', async () => {
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      expect(screen.getByTestId('performance-activity-view-1')).toBeInTheDocument()
      expect(screen.getByTestId('performance-activity-view-2')).toBeInTheDocument()
      expect(screen.getByTestId('performance-activity-view-3')).toBeInTheDocument()
    })

    it('点击 "详情" 按钮应打开详情抽屉', async () => {
      const user = userEvent.setup()
      mockGetParticipants.mockResolvedValue({ data: { items: [], total: 0 } })
      mockGetResultSummary.mockResolvedValue({ data: null })
      mockGetDistributionCheck.mockResolvedValue({ data: null })
      mockGetDistributionRules.mockResolvedValue({ data: { rules: [] } })
      mockGetHRConfirmDeadlineStatus.mockRejectedValue(new Error('not found'))

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      // 点击 id=1 的详情按钮
      await user.click(screen.getByTestId('performance-activity-view-1'))

      // 抽屉应打开，显示活动详情
      await waitFor(() => {
        expect(screen.getByText('活动详情：2026年Q2绩效')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 6: 状态流转操作 ====================
  describe('状态流转操作', () => {
    it('点击 "开启主管评分" 应调用 openManagerEvaluation', async () => {
      const user = userEvent.setup()
      mockOpenManagerEvaluation.mockResolvedValue({})

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      await openActivityDetail(user, 1)
      await user.click(screen.getByTestId('performance-detail-open-manager-1'))

      await waitFor(() => {
        expect(mockOpenManagerEvaluation).toHaveBeenCalledWith(1)
      })
    })

    it('点击 "提醒评分" 但没有待评分人员时应提示无需发送', async () => {
      const user = userEvent.setup()
      const warningSpy = vi.spyOn(message, 'warning')
      mockGetActivities.mockResolvedValue({
        data: {
          items: [{
            ...MOCK_ACTIVITIES[0],
            id: 7,
            name: '主管评分活动',
            status: 'manager_evaluation',
          }],
          total: 1,
        },
      })
      mockSendManagerEvalReminder.mockResolvedValue({
        data: {
          data: {
            pending: 0,
            candidates: 0,
            sent: 0,
            skipped: 0,
            failed: 0,
          },
        },
      })

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('主管评分活动')).toBeInTheDocument()
      })

      await openActivityDetail(user, 7)
      await user.click(screen.getByTestId('performance-detail-remind-manager-7'))

      await waitFor(() => {
        expect(mockSendManagerEvalReminder).toHaveBeenCalledWith(7)
      })
      await waitFor(() => {
        expect(warningSpy).toHaveBeenCalledWith('当前没有待主管评分人员，无需发送提醒')
      })
    })

    it('点击 "提醒自评" 有跳过和失败时应展示人员明细', async () => {
      const user = userEvent.setup()
      const errorSpy = vi.spyOn(message, 'error')
      const modalWarningSpy = vi.spyOn(Modal, 'warning').mockReturnValue({
        destroy: vi.fn(),
        update: vi.fn(),
      } as any)
      mockGetActivities.mockResolvedValue({
        data: {
          items: [{
            ...MOCK_ACTIVITIES[0],
            id: 8,
            name: '自评提醒活动',
            status: 'self_evaluation',
          }],
          total: 1,
        },
      })
      mockSendSelfEvalReminder.mockResolvedValue({
        data: {
          data: {
            pending: 2,
            candidates: 2,
            sent: 0,
            skipped: 1,
            failed: 1,
            skipped_recipients: [{ user_id: 'user-1', name: '张三' }],
            failed_recipients: [{ user_id: 'user-2', name: '李四' }],
          },
        },
      })

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('自评提醒活动')).toBeInTheDocument()
      })

      await openActivityDetail(user, 8)
      await user.click(screen.getByTestId('performance-detail-remind-self-8'))

      await waitFor(() => {
        expect(mockSendSelfEvalReminder).toHaveBeenCalledWith(8)
      })
      await waitFor(() => {
        expect(errorSpy).toHaveBeenCalledWith('自评提醒发送失败：1 人失败')
      })
      expect(modalWarningSpy).toHaveBeenCalled()
      const modalConfig = modalWarningSpy.mock.calls[0]?.[0]
      expect(modalConfig?.title).toBe('自评提醒发送明细')
      const modalText = collectReactText(modalConfig?.content)
      expect(modalText).toContain('李四（user-2）')
      expect(modalText).toContain('张三（user-1）')
      modalWarningSpy.mockRestore()
    })

    it('点击 "开启目标" 应调用 openTargetSetting', async () => {
      const user = userEvent.setup()
      mockOpenTargetSetting.mockResolvedValue({})

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年月度考核-5月')).toBeInTheDocument()
      })

      await openActivityDetail(user, 2)
      await user.click(screen.getByTestId('performance-detail-open-target-2'))

      await waitFor(() => {
        expect(mockOpenTargetSetting).toHaveBeenCalledWith(2)
      })
    })

    it('点击 "归档" 应调用 archiveActivity', async () => {
      const user = userEvent.setup()
      mockArchiveActivity.mockResolvedValue({})

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q1绩效')).toBeInTheDocument()
      })

      await openActivityDetail(user, 3)
      await user.click(screen.getByTestId('performance-detail-archive-3'))

      await waitFor(() => {
        expect(mockArchiveActivity).toHaveBeenCalledWith(3)
      })
    })

    it('操作成功后应重新加载活动列表', async () => {
      const user = userEvent.setup()
      mockArchiveActivity.mockResolvedValue({})

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q1绩效')).toBeInTheDocument()
      })

      await openActivityDetail(user, 3)
      await user.click(screen.getByTestId('performance-detail-archive-3'))

      await waitFor(() => {
        // 应被调用两次：初始化 + 操作后刷新
        expect(mockGetActivities).toHaveBeenCalledTimes(2)
      })
    })

    it('操作失败时应显示错误消息', async () => {
      const user = userEvent.setup()
      mockArchiveActivity.mockRejectedValue({ response: { data: { message: '归档失败：活动未锁定' } } })

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q1绩效')).toBeInTheDocument()
      })

      await openActivityDetail(user, 3)
      await user.click(screen.getByTestId('performance-detail-archive-3'))

      // antd message 会在 DOM 中显示错误消息
      await waitFor(() => {
        expect(screen.getByText('归档失败：活动未锁定')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 7: 新建活动 ====================
  describe('新建活动', () => {
    it('点击 "新建活动" 按钮应打开活动编辑器', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('绩效活动')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-create-activity'))

      // 编辑器 mock 应出现
      expect(screen.getByTestId('activity-editor-mock')).toBeInTheDocument()
    })

    it('点击 "刷新" 按钮应重新加载活动列表', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(mockGetActivities).toHaveBeenCalledTimes(1)
      })

      await user.click(screen.getByTestId('performance-refresh-activities'))

      await waitFor(() => {
        expect(mockGetActivities).toHaveBeenCalledTimes(2)
      })
    })
  })

  // ==================== 场景 8: 详情抽屉交互 ====================
  describe('详情抽屉交互', () => {
    beforeEach(() => {
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [
            {
              id: 101,
              employee_name: '张三',
              department_name: '技术部',
              position: '工程师',
              manager_name: '李主管',
              direct_manager_name_snapshot: '王总监',
              manager_source: 'DIRECT_MANAGER',
              manager_config_status: 'CONFIGURED',
              status: 'self_submitted',
              self_score: 85,
              manager_score: null,
              final_level: null,
              manager_id: 'M001',
              manager_overridden: false,
            },
            {
              id: 102,
              employee_name: '赵四',
              department_name: '产品部',
              position: '产品经理',
              manager_name: '',
              direct_manager_name_snapshot: '陈总监',
              manager_source: 'EMPTY',
              manager_config_status: 'PENDING',
              status: 'pending',
              self_score: null,
              manager_score: null,
              final_level: null,
              manager_id: null,
              manager_overridden: false,
            },
          ],
          total: 2,
        },
      })
      mockGetResultSummary.mockResolvedValue({
        data: {
          total_participants: 2,
          self_submitted_count: 1,
          manager_submitted_count: 0,
          result_confirmed_count: 0,
        },
      })
      mockGetDistributionCheck.mockResolvedValue({ data: null })
      mockGetDistributionRules.mockResolvedValue({ data: { rules: [] } })
      mockGetHRConfirmDeadlineStatus.mockRejectedValue(new Error('not found'))
    })

    it('非 HR 确认状态打开详情时不应请求 HR 截止状态', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      await openActivityDetail(user, 1)

      expect(mockGetParticipants).toHaveBeenCalledWith(1, { page: 1, page_size: 200 })
      expect(mockGetHRConfirmDeadlineStatus).not.toHaveBeenCalled()
      expect(mockGetDistributionCheck).not.toHaveBeenCalled()
      expect(mockGetDistributionRules).not.toHaveBeenCalled()
    })

    it('主管评分状态打开详情时仍应请求强制分布数据', async () => {
      const user = userEvent.setup()
      mockGetActivities.mockResolvedValue({
        data: { items: [{ ...MOCK_ACTIVITIES[0], status: 'manager_evaluation' }], total: 1 },
      })
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      await openActivityDetail(user, 1)

      await waitFor(() => {
        expect(mockGetDistributionCheck).toHaveBeenCalledWith(1)
      })
      expect(mockGetDistributionRules).toHaveBeenCalledWith(1)
    })

    it('HR 确认状态打开详情时仍应请求并展示 HR 截止状态', async () => {
      const user = userEvent.setup()
      mockGetActivities.mockResolvedValue({
        data: { items: [{ ...MOCK_ACTIVITIES[0], status: 'hr_confirmation' }], total: 1 },
      })
      mockGetHRConfirmDeadlineStatus.mockResolvedValue({
        data: {
          deadline: '2026-06-30 18:00:00',
          pending_count: 2,
          overdue: false,
          can_force_lock: false,
        },
      })
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      await openActivityDetail(user, 1)

      await waitFor(() => {
        expect(mockGetHRConfirmDeadlineStatus).toHaveBeenCalledWith(1)
      })
      expect(screen.getByText('HR确认截止：2026-06-30 18:00:00，待确认 2 人')).toBeInTheDocument()
    })

    it('打开详情抽屉后应显示参与人列表', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-activity-view-1'))

      await waitFor(() => {
        expect(screen.getByText('张三')).toBeInTheDocument()
      })
      expect(screen.getByText('赵四')).toBeInTheDocument()
    })

    it('点击参与人记录应打开评审记录抽屉并展示部门评分', async () => {
      const user = userEvent.setup()
      useAuthStore.setState({
        user: { user_id: 'admin', employee_id: 'admin', name: '管理员' },
        token: 'test-token',
        isLoggedIn: true,
        menuKeys: [],
        permissions: ['performance:result:view', 'performance:department_eval:submit'],
      })
      mockGetParticipantVersions.mockResolvedValue({
        data: {
          versions: [
            {
              id: 3,
              participant_id: 101,
              activity_id: '1',
              review_type: 'department_evaluation',
              created_by: 'dept-lead',
              manager_score: 91,
              suggested_level: 'A',
              final_level: 'S',
              adjust_reason: '部门校准到 S',
              created_at: '2026-06-12T11:00:00+08:00',
              updated_at: '2026-06-12T11:00:00+08:00',
              operation_meta: {
                old_total_manager_score: 91,
                old_adjusted_score: 92,
                new_department_score: 96,
                old_final_level: 'A',
                new_final_level: 'S',
                baseline_level: 'A',
                baseline_score: 92,
                department_adjusted: true,
                reason: '部门校准到 S',
              },
            },
            {
              id: 2,
              participant_id: 101,
              activity_id: '1',
              review_type: 'self',
              created_by: 'zhangsan',
              self_score: 90,
              self_level: 'A',
              self_summary: '',
              self_attachments: [],
              created_at: '2026-06-11T10:30:00+08:00',
              updated_at: '2026-06-11T10:30:00+08:00',
              operation_meta: {
                edit_after_manager_confirm: true,
                previous_total_self_score: 100,
                previous_evaluation_good: 'e网通',
                previous_evaluation_improvement: '温柔',
                evaluation_good: '新增成果',
                evaluation_improvement: '修正改进点',
              },
            },
            {
              id: 1,
              participant_id: 101,
              activity_id: '1',
              review_type: 'self',
              created_by: 'zhangsan',
              self_score: 85,
              self_level: 'B',
              self_summary: '首次自评总结',
              self_attachments: [],
              created_at: '2026-06-10T09:00:00+08:00',
              updated_at: '2026-06-10T09:00:00+08:00',
              operation_meta: { edit_after_manager_confirm: false },
            },
          ],
        },
      })

      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-activity-view-1'))

      const recordsButton = await screen.findByTestId('performance-participant-self-records-101')
      await user.click(recordsButton)

      expect(mockGetParticipantVersions).toHaveBeenCalledWith(101)
      await waitFor(() => {
        expect(screen.getByText('评审记录：张三')).toBeInTheDocument()
      })
      expect(screen.getByText('部门/中心评分')).toBeInTheDocument()
      expect(screen.getByText('已调整')).toBeInTheDocument()
      expect(screen.getByText('部门校准到 S')).toBeInTheDocument()
      expect(screen.getByText('96.0')).toBeInTheDocument()
      expect(screen.getByText('评分前')).toBeInTheDocument()
      expect(screen.getByText('评分后')).toBeInTheDocument()
      expect(screen.getByText('修改自评（主管确认后）')).toBeInTheDocument()
      expect(screen.getByText('提交自评')).toBeInTheDocument()
      expect(screen.getAllByText('修改人')).toHaveLength(2)
      expect(screen.getAllByText('张三').length).toBeGreaterThanOrEqual(1)
      expect(screen.queryByText('zhangsan')).not.toBeInTheDocument()
      expect(screen.getAllByText('修改时间')).toHaveLength(2)
      expect(screen.getAllByText('主管确认后修改')).toHaveLength(2)
      expect(screen.getAllByText('修改前').length).toBeGreaterThanOrEqual(1)
      expect(screen.getByText('修改后')).toBeInTheDocument()
      expect(screen.getByText('首次提交，无修改前内容')).toBeInTheDocument()
      expect(screen.getByText('e网通')).toBeInTheDocument()
      expect(screen.getByText('温柔')).toBeInTheDocument()
      expect(screen.getByText('新增成果')).toBeInTheDocument()
      expect(screen.getByText('修正改进点')).toBeInTheDocument()
      expect(screen.getAllByTestId('performance-self-review-version')).toHaveLength(2)
      expect(screen.getAllByTestId('performance-department-review-version')).toHaveLength(1)
    })

    it('详情抽屉应显示统计摘要', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-activity-view-1'))

      await waitFor(() => {
        expect(screen.getByText('参与人数')).toBeInTheDocument()
      })
      // 统计摘要标签应存在（"已自评" 同时出现在摘要和参与人状态列，用 getAllByText）
      expect(screen.getAllByText('已自评').length).toBeGreaterThanOrEqual(1)
      expect(screen.getAllByText('已评分').length).toBeGreaterThanOrEqual(1)
      expect(screen.getAllByText('已确认').length).toBeGreaterThanOrEqual(1)
    })

    it('新流程目标设定活动应只显示下季度目标入口', async () => {
      const user = userEvent.setup()
      mockGetActivities.mockResolvedValue({
        data: {
          items: [{
            ...MOCK_ACTIVITIES[0],
            id: 9,
            name: '新流程',
            status: 'target_setting',
            flow_type: 'new',
            activity_kind: 'goal_setting',
            workflow_config: {
              nodes: ['target_setting', 'target_approval', 'self_evaluation', 'manager_evaluation', 'department_evaluation', 'hr_confirmation', 'employee_confirmation', 'archive'],
            },
          }],
          total: 1,
        },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [{
            id: 201,
            employee_id: 'E201',
            employee_name: '列德',
            department_name: '机器人集合',
            position: '人事专员',
            status: 'target_set',
            manager_config_status: 'CONFIGURED',
            manager_id: 'M001',
          }],
          total: 1,
        },
      })
      mockGetResultSummary.mockResolvedValue({ data: { total_participants: 1, self_submitted_count: 0, manager_submitted_count: 0, result_confirmed_count: 0 } })
      mockGetDistributionCheck.mockResolvedValue({ data: null })
      mockGetDistributionRules.mockResolvedValue({ data: { rules: [] } })
      mockGetHRConfirmDeadlineStatus.mockRejectedValue(new Error('not found'))

      renderOverview()

      await waitFor(() => {
        expect(screen.getAllByText('沐腾科技流程模版').length).toBeGreaterThanOrEqual(1)
      })
      await user.click(screen.getByTestId('performance-activity-view-9'))

      const targetButton = await screen.findByTestId('performance-participant-target-201')
      expect(targetButton).toBeInTheDocument()
      expect(screen.queryByTestId('performance-participant-review-supplement-201')).not.toBeInTheDocument()
      await user.click(targetButton)

      expect(mockNavigate).toHaveBeenCalledWith('/performance-goal-setting/9/201')
    })

    it('目标提交后立即显示通过和驳回操作', async () => {
      const user = userEvent.setup()
      mockGetActivities.mockResolvedValue({
        data: {
          items: [{
            ...MOCK_ACTIVITIES[0],
            id: 11,
            name: '目标设定活动',
            status: 'target_setting',
            flow_type: 'new',
            activity_kind: 'goal_setting',
          }],
          total: 1,
        },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [{
            id: 211,
            employee_id: 'E211',
            employee_name: '列德',
            department_name: '机器人集合',
            position: '人事专员',
            status: 'target_pending_approval',
            manager_config_status: 'CONFIGURED',
            manager_id: 'M001',
          }],
          total: 1,
        },
      })
      mockGetResultSummary.mockResolvedValue({ data: { total_participants: 1, self_submitted_count: 0, manager_submitted_count: 0, result_confirmed_count: 0 } })
      mockGetDistributionCheck.mockResolvedValue({ data: null })
      mockGetDistributionRules.mockResolvedValue({ data: { rules: [] } })
      mockGetHRConfirmDeadlineStatus.mockRejectedValue(new Error('not found'))

      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('目标设定活动')).toBeInTheDocument()
      })
      await user.click(screen.getByTestId('performance-activity-view-11'))

      await screen.findByTestId('performance-participant-target-211')
      expect(screen.getByTestId('performance-participant-approve-211')).toBeInTheDocument()
      expect(screen.getByTestId('performance-participant-reject-211')).toBeInTheDocument()
    })

    it('已归档的沐腾目标设定活动仍应保留目标入口', async () => {
      const user = userEvent.setup()
      const activity = {
        ...MOCK_ACTIVITIES[0],
        id: 12,
        name: '已归档目标设定活动',
        status: 'archived',
        flow_type: 'new',
        activity_kind: 'goal_setting',
      }
      mockGetActivities.mockResolvedValue({
        data: {
          items: [activity],
          total: 1,
        },
      })
      mockGetActivity.mockResolvedValue({
        data: { activity },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [{
            id: 212,
            employee_id: 'E212',
            employee_name: '列德',
            department_name: '机器人集合',
            position: '人事专员',
            status: 'target_set',
            manager_config_status: 'CONFIGURED',
            manager_id: 'M001',
          }],
          total: 1,
        },
      })
      mockGetResultSummary.mockResolvedValue({
        data: {
          total_participants: 1,
          self_submitted_count: 0,
          manager_submitted_count: 0,
          result_confirmed_count: 0,
        },
      })
      mockGetDistributionCheck.mockResolvedValue({ data: null })
      mockGetDistributionRules.mockResolvedValue({ data: { rules: [] } })
      mockGetHRConfirmDeadlineStatus.mockRejectedValue(new Error('not found'))

      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('已归档目标设定活动')).toBeInTheDocument()
      })
      await openActivityDetail(user, 12)

      const targetButton = await screen.findByTestId('performance-participant-target-212')
      expect(screen.queryByTestId('performance-participant-result-212')).not.toBeInTheDocument()
      await user.click(targetButton)

      expect(mockNavigate).toHaveBeenCalledWith('/performance-goal-setting/12/212')
    })

    it('新流程调进度弹窗应使用新流程参与人状态文案', async () => {
      const user = userEvent.setup()
      const activity = {
        ...MOCK_ACTIVITIES[0],
        id: 10,
        name: '新流程员工确认',
        status: 'employee_confirmation',
        flow_type: 'new',
      }
      mockGetActivities.mockResolvedValue({
        data: { items: [activity], total: 1 },
      })
      mockGetActivity.mockResolvedValue({
        data: { activity },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [{
            id: 301,
            employee_id: 'E301',
            employee_name: '列德',
            department_name: '机器人集合',
            position: '人事专员',
            status: 'manager_submitted',
            manager_config_status: 'CONFIGURED',
            manager_id: 'M001',
          }],
          total: 1,
        },
      })
      mockGetResultSummary.mockResolvedValue({
        data: {
          total_participants: 1,
          self_submitted_count: 1,
          manager_submitted_count: 1,
          result_confirmed_count: 0,
        },
      })
      mockGetDistributionCheck.mockResolvedValue({ data: null })
      mockGetDistributionRules.mockResolvedValue({ data: { rules: [] } })

      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('新流程员工确认')).toBeInTheDocument()
      })
      await openActivityDetail(user, 10)

      await waitFor(() => {
        expect(screen.getAllByText('主管评分已完成').length).toBeGreaterThanOrEqual(1)
      })
      expect(screen.getAllByText('主管评分已完成').length).toBeGreaterThanOrEqual(1)
      expect(screen.queryByText('已主管评分')).not.toBeInTheDocument()
      expect(screen.queryByText('已评分')).not.toBeInTheDocument()

      await user.click(screen.getByTestId('performance-participant-progress-301'))
      await waitFor(() => {
        expect(screen.getAllByText('调整进度：列德').length).toBeGreaterThanOrEqual(1)
      })
      expect(screen.getAllByText('主管评分已完成').length).toBeGreaterThanOrEqual(1)
      expect(screen.queryByText('已员工确认')).not.toBeInTheDocument()
      expect(screen.queryByText('待领导复核')).not.toBeInTheDocument()

      const select = document.querySelector('.ant-modal .ant-select-selector') as HTMLElement
      await user.click(select)

      await waitFor(() => {
        expect(screen.getAllByText('结果已公布').length).toBeGreaterThanOrEqual(1)
      })
      expect(screen.queryByText('员工已确认')).not.toBeInTheDocument()
      expect(screen.queryByText('待领导复核')).not.toBeInTheDocument()
      expect(screen.queryByText('已离职')).not.toBeInTheDocument()
      expect(screen.queryByText('已移除')).not.toBeInTheDocument()
    })

    it('未配置考核上级的参与人应显示 "待配置考核上级" 标签', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-activity-view-1'))

      await waitFor(() => {
        // "待配置考核上级" 出现在考核上级列和配置状态列，至少 2 次
        expect(screen.getAllByText('待配置考核上级').length).toBeGreaterThanOrEqual(2)
      })
    })

    it('主管评分阶段未配置考核上级时评分按钮应禁用', async () => {
      const user = userEvent.setup()
      mockGetActivities.mockResolvedValue({
        data: { items: [{ ...MOCK_ACTIVITIES[0], status: 'manager_evaluation' }], total: 1 },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [
            {
              id: 102,
              employee_name: '赵四',
              department_name: '产品部',
              position: '产品经理',
              manager_name: '',
              direct_manager_name_snapshot: '陈总监',
              manager_source: 'EMPTY',
              manager_config_status: 'PENDING',
              status: 'self_submitted',
              self_score: 80,
              manager_score: null,
              final_level: null,
              manager_id: null,
              manager_overridden: false,
            },
          ],
          total: 1,
        },
      })

      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-activity-view-1'))

      const managerButton = await screen.findByTestId('performance-participant-manager-102')
      expect(managerButton).toBeDisabled()

      await user.click(managerButton)
      expect(mockNavigate).not.toHaveBeenCalledWith('/performance-manager-eval/1/102')
    })

    it('详情抽屉应显示流程步骤', async () => {
      const user = userEvent.setup()
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-activity-view-1'))

      await waitFor(() => {
        expect(screen.getByText('活动详情：2026年Q2绩效')).toBeInTheDocument()
      })
      // Steps 组件应渲染流程步骤文本（用 getAllByText 因为 "自评" 可能出现多次）
      expect(screen.getByText('目标设定')).toBeInTheDocument()
      expect(screen.getAllByText('自评').length).toBeGreaterThanOrEqual(1)
      expect(screen.getAllByText('评分').length).toBeGreaterThanOrEqual(1)
    })
  })

  // ==================== 场景 9: 详情参与人加载 ====================
  describe('详情参与人加载', () => {
    it('打开详情抽屉时应自动加载参与人', async () => {
      const user = userEvent.setup()
      mockGetParticipants.mockResolvedValue({ data: { items: [], total: 0 } })
      mockGetResultSummary.mockResolvedValue({ data: null })
      mockGetDistributionCheck.mockResolvedValue({ data: null })
      mockGetDistributionRules.mockResolvedValue({ data: { rules: [] } })
      mockGetHRConfirmDeadlineStatus.mockRejectedValue(new Error('not found'))

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      // 先打开抽屉
      await user.click(screen.getByTestId('performance-activity-view-1'))

      await waitFor(() => {
        expect(screen.getByText('活动详情：2026年Q2绩效')).toBeInTheDocument()
      })

      // 抽屉打开后，getParticipants 应该已经被调用了一次
      expect(mockGetParticipants).toHaveBeenCalled()
    })
  })

  // ==================== 场景 10: 初始化错误汇总与 scope-options ====================
  describe('页面初始化错误与 scope-options', () => {
    it('初始化三个接口全部成功：message.error 不调用', async () => {
      const errorSpy = vi.spyOn(message, 'error')
      renderOverview()
      await waitFor(() => {
        expect(mockGetActivities).toHaveBeenCalled()
        expect(mockGetScopeOptions).toHaveBeenCalled()
        expect(mockGetTemplates).toHaveBeenCalled()
      })
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })
      expect(errorSpy).not.toHaveBeenCalled()
    })

    it('一个接口失败：只弹一次准确错误', async () => {
      const errorSpy = vi.spyOn(message, 'error')
      mockGetScopeOptions.mockRejectedValue({
        response: { data: { message: '部门、员工选项加载失败：权限不足' } },
      })
      renderOverview()
      await waitFor(() => {
        expect(errorSpy).toHaveBeenCalledTimes(1)
      })
      expect(errorSpy).toHaveBeenCalledWith('部门、员工选项加载失败：权限不足')
      // 活动列表仍正常展示
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })
    })

    it('多个接口失败：只弹一次汇总错误', async () => {
      const errorSpy = vi.spyOn(message, 'error')
      mockGetActivities.mockRejectedValue({
        response: { data: { message: '加载活动列表失败' } },
      })
      mockGetTemplates.mockRejectedValue({
        response: { data: { message: '流程模板选项加载失败' } },
      })
      renderOverview()
      await waitFor(() => {
        expect(errorSpy).toHaveBeenCalledTimes(1)
      })
      const msg = String(errorSpy.mock.calls[0]?.[0] || '')
      expect(msg.startsWith('页面初始化失败：')).toBe(true)
      expect(msg).toContain('加载活动列表失败')
      expect(msg).toContain('流程模板选项加载失败')
    })

    it('后端 message 优先于 fallback', async () => {
      const errorSpy = vi.spyOn(message, 'error')
      mockGetActivities.mockRejectedValue({
        response: { data: { message: '后端返回的精确错误' } },
      })
      renderOverview()
      await waitFor(() => {
        expect(errorSpy).toHaveBeenCalledWith('后端返回的精确错误')
      })
    })

    it('scope-options 部分失败后，活动列表仍正常展示', async () => {
      mockGetScopeOptions.mockRejectedValue(new Error('scope boom'))
      const errorSpy = vi.spyOn(message, 'error')
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })
      expect(errorSpy).toHaveBeenCalled()
    })

    it('scope-options 返回 employee_id/id 但没有 user_id：不得进入员工 Select 选项', async () => {
      const user = userEvent.setup()
      mockGetScopeOptions.mockResolvedValue({
        data: {
          departments: [{ department_id: 'D001', name: '产品' }],
          employees: [
            { employee_id: 'E001', id: 9, name: '无UserID员工', department_id: 'D001', status: 'active' },
            { user_id: 'U001', name: '有UserID员工', department_id: 'D001', status: 'active' },
          ],
          warnings: ['有 1 名在职员工缺少 user_id，已从选项中跳过'],
        },
      })
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      // 打开新建活动编辑器，强制渲染员工 Select options
      const createBtn = await screen.findByTestId('performance-create-activity')
      await user.click(createBtn)
      await waitFor(() => {
        expect(screen.getByTestId('activity-editor-mock')).toBeInTheDocument()
      })

      // getUserOption 只认 user_id：U001 必须进入 options，E001 / id=9 不得进入
      await waitFor(() => {
        expect(screen.getByTestId('activity-editor-user-option-U001')).toBeInTheDocument()
      })
      expect(screen.queryByTestId('activity-editor-user-option-E001')).not.toBeInTheDocument()
      expect(screen.queryByTestId('activity-editor-user-option-9')).not.toBeInTheDocument()
      expect(screen.queryByText('无UserID员工')).not.toBeInTheDocument()
    })

    it('开启目标成功返回 warnings：只展示一次 warning', async () => {
      const user = userEvent.setup()
      const warningSpy = vi.spyOn(message, 'warning')
      const successSpy = vi.spyOn(message, 'success')
      mockOpenTargetSetting.mockResolvedValue({
        data: { warnings: ['以下员工不可用、已离职或不属于当前企业：ghost-1'] },
      })
      // draft 活动必须出现在列表
      mockGetActivities.mockResolvedValue({
        data: {
          items: [{
            ...MOCK_ACTIVITIES[1],
            id: 2,
            name: '2026年月度考核-5月',
            status: 'draft',
          }],
          total: 1,
        },
      })
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年月度考核-5月')).toBeInTheDocument()
      })

      // 打开详情 → 详情内有 data-testid 的开启目标按钮（必须存在）
      await openActivityDetail(user, 2)
      const openBtn = await screen.findByTestId('performance-detail-open-target-2')
      await user.click(openBtn)

      await waitFor(() => {
        expect(mockOpenTargetSetting).toHaveBeenCalledTimes(1)
      })
      expect(successSpy).toHaveBeenCalledWith('操作成功')
      expect(warningSpy).toHaveBeenCalledTimes(1)
      expect(String(warningSpy.mock.calls[0]?.[0] || '')).toContain('不可用、已离职或不属于当前企业')
    })
  })

  describe('上期结果导航', () => {
    it('有上期数据时跳转到上期 activity/participant，而不是当前 id', async () => {
      const user = userEvent.setup()
      const infoSpy = vi.spyOn(message, 'info')
      mockGetPreviousParticipantResult.mockResolvedValue({
        data: {
          activity: { id: 109, name: '上期活动' },
          participant: { id: 209, employee_name: '列德' },
        },
      })
      mockGetActivities.mockResolvedValue({
        data: {
          items: [{
            ...MOCK_ACTIVITIES[0],
            id: 110,
            name: '本期沐腾',
            status: 'manager_evaluation',
            flow_type: 'new',
            activity_kind: 'review',
          }],
          total: 1,
        },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [{
            id: 210,
            employee_id: 'E210',
            employee_name: '列德',
            department_name: '机器人集合',
            position: '人事专员',
            status: 'manager_submitted',
            manager_config_status: 'CONFIGURED',
            manager_id: 'M001',
          }],
          total: 1,
        },
      })
      mockGetResultSummary.mockResolvedValue({
        data: { total_participants: 1, self_submitted_count: 1, manager_submitted_count: 1, result_confirmed_count: 0 },
      })
      mockGetDistributionCheck.mockResolvedValue({ data: null })
      mockGetDistributionRules.mockResolvedValue({ data: { rules: [] } })
      mockGetHRConfirmDeadlineStatus.mockRejectedValue(new Error('not found'))

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('本期沐腾')).toBeInTheDocument()
      })
      await openActivityDetail(user, 110)

      const prevBtn = await screen.findByTestId('performance-participant-previous-result-210')
      await user.click(prevBtn)

      await waitFor(() => {
        expect(mockGetPreviousParticipantResult).toHaveBeenCalledWith(210)
      })
      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith('/performance-result/109/209')
      })
      expect(mockNavigate).not.toHaveBeenCalledWith('/performance-result/110/210')
      expect(infoSpy).not.toHaveBeenCalledWith('暂无上期绩效结果')
    })

    it('无上期数据时提示暂无，不跳转', async () => {
      const user = userEvent.setup()
      const infoSpy = vi.spyOn(message, 'info')
      mockGetPreviousParticipantResult.mockResolvedValue({ data: {} })
      mockGetActivities.mockResolvedValue({
        data: {
          items: [{
            ...MOCK_ACTIVITIES[0],
            id: 111,
            name: '无上期活动',
            status: 'manager_evaluation',
            flow_type: 'new',
          }],
          total: 1,
        },
      })
      mockGetParticipants.mockResolvedValue({
        data: {
          items: [{
            id: 211,
            employee_id: 'E211',
            employee_name: '列德',
            department_name: '机器人集合',
            position: '人事专员',
            status: 'manager_submitted',
            manager_config_status: 'CONFIGURED',
            manager_id: 'M001',
          }],
          total: 1,
        },
      })
      mockGetResultSummary.mockResolvedValue({
        data: { total_participants: 1, self_submitted_count: 1, manager_submitted_count: 1, result_confirmed_count: 0 },
      })
      mockGetDistributionCheck.mockResolvedValue({ data: null })
      mockGetDistributionRules.mockResolvedValue({ data: { rules: [] } })
      mockGetHRConfirmDeadlineStatus.mockRejectedValue(new Error('not found'))

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('无上期活动')).toBeInTheDocument()
      })
      await openActivityDetail(user, 111)

      await user.click(await screen.findByTestId('performance-participant-previous-result-211'))
      await waitFor(() => {
        expect(infoSpy).toHaveBeenCalledWith('暂无上期绩效结果')
      })
      expect(mockNavigate).not.toHaveBeenCalledWith(expect.stringMatching(/\/performance-result\//))
    })
  })
})
