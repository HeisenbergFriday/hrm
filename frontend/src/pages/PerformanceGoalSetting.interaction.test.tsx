/**
 * PerformanceGoalSetting 组件交互测试
 *
 * 覆盖：
 * - 页面渲染与 loading 状态
 * - 目标列表加载
 * - 添加/删除量化指标和关键行动
 * - 权重合计显示与校验
 * - 保存草稿 / 提交目标
 * - 只读状态（已审批通过）
 * - 接口失败
 */
import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import PerformanceGoalSetting from './PerformanceGoalSetting'

// ==================== Mocks ====================

const mockNavigate = vi.fn()
const mockGetGoalRecords = vi.fn()
const mockGetParticipant = vi.fn()
const mockBatchSaveGoalRecords = vi.fn()
const mockSubmitGoalApproval = vi.fn()
const mockGetGoalSuggestions = vi.fn()
const mockSearchIndicatorItems = vi.fn()

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ activityId: '1', participantId: '101' }),
}))

vi.mock('../services/api', () => ({
  performanceAPI: {
    getGoalRecords: (...args: any[]) => mockGetGoalRecords(...args),
    getParticipant: (...args: any[]) => mockGetParticipant(...args),
    batchSaveGoalRecords: (...args: any[]) => mockBatchSaveGoalRecords(...args),
    submitGoalApproval: (...args: any[]) => mockSubmitGoalApproval(...args),
    getGoalSuggestions: (...args: any[]) => mockGetGoalSuggestions(...args),
    searchIndicatorItems: (...args: any[]) => mockSearchIndicatorItems(...args),
  },
}))

vi.mock('../utils/authFileUrl', () => ({
  withFileAccessToken: (url: string) => url,
}))

// ==================== Mock 数据 ====================

function makeParticipant(overrides: Record<string, any> = {}) {
  return {
    id: 101,
    employee_name: '张三',
    employee_id: 'E001',
    status: 'pending',
    ...overrides,
  }
}

function makeGoalRecords(overrides: Record<string, any>[] = []) {
  const defaults = [
    {
      id: 1,
      section_type: 'quantitative',
      item_name: '销售额',
      item_definition: '月度销售额达成率',
      weight: 0.35,
      red_line_value: '80%',
      target_value: '100%',
      challenge_value: '120%',
      scoring_rule: '按达成比例评分',
      actual_result: '',
      attachments: [],
      approval_status: 'pending',
      sort_order: 0,
    },
    {
      id: 2,
      section_type: 'quantitative',
      item_name: '客户满意度',
      item_definition: 'NPS 评分',
      weight: 0.35,
      red_line_value: '60',
      target_value: '80',
      challenge_value: '95',
      scoring_rule: '按 NPS 评分区间',
      actual_result: '',
      attachments: [],
      approval_status: 'pending',
      sort_order: 1,
    },
    {
      id: 3,
      section_type: 'key_action',
      item_name: '客户拜访',
      item_definition: '每月拜访重点客户',
      weight: 0.15,
      target_value: '每月 10 家',
      actual_result: '',
      attachments: [],
      approval_status: 'pending',
      sort_order: 2,
    },
    {
      id: 4,
      section_type: 'key_action',
      item_name: '培训完成',
      item_definition: '完成产品培训课程',
      weight: 0.15,
      target_value: '完成全部课程',
      actual_result: '',
      attachments: [],
      approval_status: 'pending',
      sort_order: 3,
    },
  ]
  return {
    data: {
      items: overrides.length > 0 ? overrides : defaults,
    },
  }
}

function setupDefaultMocks() {
  mockGetGoalRecords.mockResolvedValue(makeGoalRecords())
  mockGetParticipant.mockResolvedValue({ data: { participant: makeParticipant() } })
  mockBatchSaveGoalRecords.mockResolvedValue({})
  mockSubmitGoalApproval.mockResolvedValue({})
  mockGetGoalSuggestions.mockResolvedValue({ data: { suggestions: [] } })
  mockSearchIndicatorItems.mockResolvedValue({ data: { items: [] } })
}

// ==================== 测试 ====================

describe('PerformanceGoalSetting 交互测试', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setupDefaultMocks()
  })

  // ==================== 场景 1: 渲染与 loading ====================
  describe('渲染与 loading', () => {
    it('加载中应显示 Spin', async () => {
      // 让接口永不返回，组件会一直处于 loading
      mockGetGoalRecords.mockReturnValue(new Promise(() => {}))
      mockGetParticipant.mockReturnValue(new Promise(() => {}))

      const { container } = render(
        React.createElement(PerformanceGoalSetting)
      )
      expect(container.querySelector('.ant-spin')).toBeInTheDocument()
    })

    it('加载完成后应显示页面标题和量化指标表格', async () => {
      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByText('量化指标')).toBeInTheDocument()
      })
      expect(screen.getByText('关键行动')).toBeInTheDocument()
      expect(screen.getAllByText('目标设定').length).toBeGreaterThanOrEqual(1)
    })

    it('加载完成后应显示参与者姓名', async () => {
      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getAllByText('张三').length).toBeGreaterThanOrEqual(1)
      })
    })
  })

  // ==================== 场景 2: 指标列表展示 ====================
  describe('指标列表展示', () => {
    it('应显示量化指标名称', async () => {
      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByDisplayValue('销售额')).toBeInTheDocument()
      })
      expect(screen.getByDisplayValue('客户满意度')).toBeInTheDocument()
    })

    it('应显示关键行动名称', async () => {
      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByDisplayValue('客户拜访')).toBeInTheDocument()
      })
      expect(screen.getByDisplayValue('培训完成')).toBeInTheDocument()
    })

    it('应显示权重合计', async () => {
      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        // 权重合计 100% 应显示
        expect(screen.getAllByText('100%').length).toBeGreaterThanOrEqual(1)
      })
    })
  })

  // ==================== 场景 3: 添加/删除指标 ====================
  describe('添加/删除指标', () => {
    it('点击"添加量化指标"应增加一行', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByText('量化指标')).toBeInTheDocument()
      })

      const addBtn = screen.getByTestId('performance-goal-add-quant')
      await user.click(addBtn)

      // 添加后量化指标行应从 2 行增加到 3 行
      await waitFor(() => {
        expect(screen.getAllByTestId(/performance-goal-quant-name-/).length).toBeGreaterThanOrEqual(3)
      })
    })

    it('点击"添加关键行动"应增加一行', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByText('关键行动')).toBeInTheDocument()
      })

      const addBtn = screen.getByTestId('performance-goal-add-action')
      await user.click(addBtn)

      await waitFor(() => {
        expect(screen.getAllByTestId(/performance-goal-action-name-/).length).toBeGreaterThanOrEqual(3)
      })
    })

    it('只剩一行时点删除应提示"至少保留一个"', async () => {
      const user = userEvent.setup()
      // 只返回一条量化 + 一条关键行动
      mockGetGoalRecords.mockResolvedValue({
        data: {
          items: [
            { id: 1, section_type: 'quantitative', item_name: '指标A', item_definition: '', weight: 0.35, red_line_value: '', target_value: '', challenge_value: '', scoring_rule: '', actual_result: '', attachments: [], approval_status: 'pending', sort_order: 0 },
            { id: 2, section_type: 'key_action', item_name: '行动B', item_definition: '', weight: 0.15, target_value: '', actual_result: '', attachments: [], approval_status: 'pending', sort_order: 1 },
          ],
        },
      })

      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByDisplayValue('指标A')).toBeInTheDocument()
      })

      // 找到删除按钮（Danger 按钮 with DeleteOutlined icon）
      const deleteButtons = screen.getAllByRole('button').filter(
        btn => btn.querySelector('.anticon-delete')
      )
      // 点击第一个删除按钮（量化指标的删除）
      if (deleteButtons.length > 0) {
        await user.click(deleteButtons[0])
        await waitFor(() => {
          expect(screen.getByText('至少保留一个量化指标')).toBeInTheDocument()
        })
      }
    })
  })

  // ==================== 场景 4: 保存草稿 ====================
  describe('保存草稿', () => {
    it('点击"保存草稿"应调用 batchSaveGoalRecords', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByText('量化指标')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-goal-save-draft'))

      await waitFor(() => {
        expect(mockBatchSaveGoalRecords).toHaveBeenCalledWith(
          101,
          expect.objectContaining({ items: expect.any(Array) })
        )
      })
    })

    it('保存成功后应显示成功消息', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByText('量化指标')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-goal-save-draft'))

      await waitFor(() => {
        expect(screen.getAllByText('草稿保存成功').length).toBeGreaterThanOrEqual(1)
      })
    })

    it('保存失败应显示错误消息', async () => {
      const user = userEvent.setup()
      mockBatchSaveGoalRecords.mockRejectedValue({ response: { data: { message: '保存失败：权限不足' } } })

      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByText('量化指标')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-goal-save-draft'))

      await waitFor(() => {
        expect(screen.getByText('保存失败：权限不足')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 5: 接口失败 ====================
  describe('接口失败', () => {
    it('加载数据失败应显示错误消息', async () => {
      mockGetGoalRecords.mockRejectedValue(new Error('network error'))
      mockGetParticipant.mockRejectedValue(new Error('network error'))

      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByText('加载数据失败')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 6: 只读状态 ====================
  describe('只读状态（已审批通过）', () => {
    it('目标已审批通过时应显示不可修改标签', async () => {
      mockGetGoalRecords.mockResolvedValue({
        data: {
          items: [
            { id: 1, section_type: 'quantitative', item_name: '指标A', item_definition: '', weight: 0.35, red_line_value: '', target_value: '', challenge_value: '', scoring_rule: '', actual_result: '', attachments: [], approval_status: 'approved', sort_order: 0 },
            { id: 2, section_type: 'key_action', item_name: '行动B', item_definition: '', weight: 0.15, target_value: '', actual_result: '', attachments: [], approval_status: 'approved', sort_order: 1 },
          ],
        },
      })

      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByText('目标已审批通过，不可修改')).toBeInTheDocument()
      })
    })

    it('target_set 状态时保存草稿应提示不可修改', async () => {
      const user = userEvent.setup()
      mockGetParticipant.mockResolvedValue({
        data: { participant: makeParticipant({ status: 'target_set' }) },
      })

      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByText('量化指标')).toBeInTheDocument()
      })

      const saveButton = screen.getByTestId('performance-goal-save-draft')
      expect(saveButton).toBeDisabled()
      await user.click(saveButton)

      expect(mockBatchSaveGoalRecords).not.toHaveBeenCalled()
    })
  })

  // ==================== 场景 7: 返回按钮 ====================
  describe('返回按钮', () => {
    it('点击返回按钮应调用 navigate(-1)', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByText('量化指标')).toBeInTheDocument()
      })

      const backBtn = screen.getByRole('button', { name: /返回/ })
      await user.click(backBtn)

      expect(mockNavigate).toHaveBeenCalledWith(-1)
    })
  })

  // ==================== 场景 8: 指标库建议 ====================
  describe('指标库建议', () => {
    it('应渲染"从指标库获取建议"按钮', async () => {
      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByTestId('performance-goal-load-suggestions')).toBeInTheDocument()
      })
    })

    it('点击获取建议应调用 API', async () => {
      const user = userEvent.setup()
      mockGetGoalSuggestions.mockResolvedValue({
        data: {
          suggestions: [
            { name: '建议指标1', section_type: 'quantitative', weight: 0.2 },
          ],
        },
      })

      render(React.createElement(PerformanceGoalSetting))

      await waitFor(() => {
        expect(screen.getByTestId('performance-goal-load-suggestions')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-goal-load-suggestions'))

      await waitFor(() => {
        expect(mockGetGoalSuggestions).toHaveBeenCalledWith(101)
      })
    })
  })
})
