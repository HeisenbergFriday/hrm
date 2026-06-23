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
import PerformanceOverview from './PerformanceOverview'
import { useAuthStore } from '../store/authStore'

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
const mockGetResultSummary = vi.fn()
const mockGetDistributionCheck = vi.fn()
const mockGetDistributionRules = vi.fn()
const mockGetHRConfirmDeadlineStatus = vi.fn()
const mockGetDepartments = vi.fn()
const mockGetUsers = vi.fn()
const mockGetIndicatorLibraries = vi.fn()
const mockGetTemplates = vi.fn()
const mockStartActivity = vi.fn()
const mockOpenSelfEvaluation = vi.fn()
const mockOpenManagerEvaluation = vi.fn()
const mockOpenTargetSetting = vi.fn()
const mockOpenEmployeeConfirmation = vi.fn()
const mockOpenManagerConfirmation = vi.fn()
const mockOpenHRConfirmation = vi.fn()
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

vi.mock('../services/api', () => ({
  performanceAPI: {
    getActivities: (...args: any[]) => mockGetActivities(...args),
    getParticipants: (...args: any[]) => mockGetParticipants(...args),
    getMyParticipants: (...args: any[]) => mockGetMyParticipants(...args),
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
  },
  departmentAPI: {
    getDepartments: (...args: any[]) => mockGetDepartments(...args),
  },
  userAPI: {
    getUsers: (...args: any[]) => mockGetUsers(...args),
  },
}))

// Mock PerformanceActivityEditor — 简化为一个 placeholder，避免复杂子组件的副作用
vi.mock('../components/PerformanceActivityEditor', () => ({
  default: ({ visible, onSave, onCancel }: any) =>
    visible ? (
      <div data-testid="activity-editor-mock">
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
  mockGetDepartments.mockResolvedValue({ data: { departments: [] } })
  mockGetUsers.mockResolvedValue({ data: { items: [] } })
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

// ==================== 测试 ====================

describe('PerformanceOverview 组件交互测试', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.setState({
      user: null,
      token: '',
      isLoggedIn: false,
      menuKeys: [],
      permissions: [],
    })
    setupDefaultMocks()
  })

  // ==================== 场景 1: 页面初始渲染 ====================
  describe('页面初始渲染', () => {
    it('应渲染页面标题 "绩效管理"', async () => {
      renderOverview()
      expect(screen.getByText('绩效管理')).toBeInTheDocument()
    })

    it('应渲染副标题 "绩效活动管理与评分工作台"', async () => {
      renderOverview()
      expect(screen.getByText('绩效活动管理与评分工作台')).toBeInTheDocument()
    })

    it('应显示 4 个统计卡片', async () => {
      renderOverview()
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

    it('状态筛选 Select 组件应渲染', async () => {
      renderOverview()

      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      // 验证筛选 Select 存在（antd Select 在 jsdom 中的交互较复杂，具体筛选通过统计卡片测试覆盖）
      const selectElements = screen.getAllByRole('combobox')
      expect(selectElements.length).toBeGreaterThanOrEqual(1)
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
    it('draft 状态活动应显示 "编辑参与人"、"开启目标" 按钮且不再显示手动刷新', async () => {
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年月度考核-5月')).toBeInTheDocument()
      })

      // draft 活动 id=2
      expect(screen.getByTestId('performance-activity-edit-2')).toBeInTheDocument()
      expect(screen.queryByTestId('performance-activity-refresh-2')).not.toBeInTheDocument()
      expect(screen.getByTestId('performance-activity-open-target-2')).toBeInTheDocument()
    })

    it('self_evaluation 状态活动应显示 "提醒自评"、"开启主管评分" 按钮', async () => {
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q2绩效')).toBeInTheDocument()
      })

      // self_evaluation 活动 id=1
      expect(screen.getByTestId('performance-activity-notify-self-1')).toBeInTheDocument()
      expect(screen.getByTestId('performance-activity-open-manager-1')).toBeInTheDocument()
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

    it('locked 状态活动应显示 "归档" 按钮', async () => {
      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年Q1绩效')).toBeInTheDocument()
      })

      // locked 活动 id=3
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

      await user.click(screen.getByTestId('performance-activity-open-manager-1'))

      await waitFor(() => {
        expect(mockOpenManagerEvaluation).toHaveBeenCalledWith(1)
      })
    })

    it('点击 "开启目标" 应调用 openTargetSetting', async () => {
      const user = userEvent.setup()
      mockOpenTargetSetting.mockResolvedValue({})

      renderOverview()
      await waitFor(() => {
        expect(screen.getByText('2026年月度考核-5月')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-activity-open-target-2'))

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

      await user.click(screen.getByTestId('performance-activity-archive-3'))

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

      await user.click(screen.getByTestId('performance-activity-archive-3'))

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

      await user.click(screen.getByTestId('performance-activity-archive-3'))

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

    it('新流程目标设定阶段应显示上一季度指标补录入口', async () => {
      const user = userEvent.setup()
      mockGetActivities.mockResolvedValue({
        data: {
          items: [{
            ...MOCK_ACTIVITIES[0],
            id: 9,
            name: '新流程',
            status: 'target_setting',
            flow_type: 'new',
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
        expect(screen.getAllByText('新流程').length).toBeGreaterThanOrEqual(1)
      })
      await user.click(screen.getByTestId('performance-activity-view-9'))

      const supplementButton = await screen.findByTestId('performance-participant-review-supplement-201')
      expect(supplementButton).toBeInTheDocument()
      await user.click(supplementButton)

      expect(mockNavigate).toHaveBeenCalledWith('/performance-goal-setting/9/201?phase=review')
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
})
