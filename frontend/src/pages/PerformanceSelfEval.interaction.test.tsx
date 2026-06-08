/**
 * PerformanceSelfEval 组件交互测试
 *
 * 覆盖：
 * - 页面渲染与 loading 状态
 * - 指标列表展示
 * - 自评总分计算
 * - 提交自评
 * - 接口失败
 */
import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import PerformanceSelfEval from './PerformanceSelfEval'

// ==================== Mocks ====================

const mockNavigate = vi.fn()
const mockGetGoalRecords = vi.fn()
const mockGetParticipant = vi.fn()
const mockSubmitGoalSelfEvaluation = vi.fn()

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ activityId: '1', participantId: '101' }),
}))

vi.mock('../services/api', () => ({
  performanceAPI: {
    getGoalRecords: (...args: any[]) => mockGetGoalRecords(...args),
    getParticipant: (...args: any[]) => mockGetParticipant(...args),
    submitGoalSelfEvaluation: (...args: any[]) => mockSubmitGoalSelfEvaluation(...args),
  },
}))

vi.mock('../utils/authFileUrl', () => ({
  withFileAccessToken: (url: string) => url,
}))

// ==================== Mock 数据 ====================

function makeParticipant() {
  return {
    id: 101,
    employee_name: '张三',
    employee_id: 'E001',
    status: 'target_set',
    self_evaluation_good: '',
    self_evaluation_improvement: '',
  }
}

function makeGoalRecords() {
  return {
    data: {
      items: [
        {
          id: 1,
          section_type: 'quantitative',
          item_name: '销售额',
          item_definition: '月度销售额',
          weight: 0.35,
          red_line_value: '80%',
          target_value: '100%',
          challenge_value: '120%',
          scoring_rule: '按比例',
          actual_result: '',
          self_score: null,
          attachments: [],
          sort_order: 0,
        },
        {
          id: 2,
          section_type: 'quantitative',
          item_name: '客户满意度',
          item_definition: 'NPS',
          weight: 0.35,
          red_line_value: '60',
          target_value: '80',
          challenge_value: '95',
          scoring_rule: '按区间',
          actual_result: '',
          self_score: null,
          attachments: [],
          sort_order: 1,
        },
        {
          id: 3,
          section_type: 'key_action',
          item_name: '客户拜访',
          item_definition: '每月拜访',
          weight: 0.15,
          target_value: '每月 10 家',
          actual_result: '',
          self_score: null,
          attachments: [],
          sort_order: 2,
        },
        {
          id: 4,
          section_type: 'key_action',
          item_name: '培训完成',
          item_definition: '完成培训',
          weight: 0.15,
          target_value: '全部完成',
          actual_result: '',
          self_score: null,
          attachments: [],
          sort_order: 3,
        },
      ],
    },
  }
}

function setupDefaultMocks() {
  mockGetGoalRecords.mockResolvedValue(makeGoalRecords())
  mockGetParticipant.mockResolvedValue({ data: { participant: makeParticipant() } })
  mockSubmitGoalSelfEvaluation.mockResolvedValue({})
}

// ==================== 测试 ====================

describe('PerformanceSelfEval 交互测试', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setupDefaultMocks()
  })

  // ==================== 场景 1: 渲染与 loading ====================
  describe('渲染与 loading', () => {
    it('加载中应显示 Spin', async () => {
      mockGetGoalRecords.mockReturnValue(new Promise(() => {}))
      mockGetParticipant.mockReturnValue(new Promise(() => {}))

      const { container } = render(
        React.createElement(PerformanceSelfEval)
      )
      expect(container.querySelector('.ant-spin')).toBeInTheDocument()
    })

    it('加载完成后应显示页面标题', async () => {
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getAllByText('绩效自评').length).toBeGreaterThanOrEqual(1)
      })
    })

    it('加载完成后应显示指标名称', async () => {
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByText('销售额')).toBeInTheDocument()
      })
      expect(screen.getByText('客户满意度')).toBeInTheDocument()
      expect(screen.getByText('客户拜访')).toBeInTheDocument()
      expect(screen.getByText('培训完成')).toBeInTheDocument()
    })
  })

  // ==================== 场景 2: 指标列表 ====================
  describe('指标列表', () => {
    it('应显示权重百分比', async () => {
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getAllByText('35%').length).toBeGreaterThanOrEqual(1)
      })
      expect(screen.getAllByText('15%').length).toBeGreaterThanOrEqual(1)
    })

    it('应显示目标信息', async () => {
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByText('销售额')).toBeInTheDocument()
      })

      // 量化指标的目标值
      expect(screen.getByText(/红线: 80%/)).toBeInTheDocument()
      expect(screen.getByText(/目标: 100%/)).toBeInTheDocument()
    })

    it('应显示"实际达成结果"输入框', async () => {
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByText('销售额')).toBeInTheDocument()
      })

      const actualInputs = screen.getAllByPlaceholderText('描述实际完成情况')
      expect(actualInputs.length).toBeGreaterThanOrEqual(4)
    })

    it('应显示"自评得分"输入框', async () => {
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByText('销售额')).toBeInTheDocument()
      })

      // InputNumber 组件
      const scoreInputs = screen.getAllByRole('spinbutton')
      expect(scoreInputs.length).toBeGreaterThanOrEqual(4)
    })
  })

  // ==================== 场景 3: 自评总分 ====================
  describe('自评总分', () => {
    it('应显示自评总分区域', async () => {
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByText('自评总分：')).toBeInTheDocument()
      })
    })

    it('初始总分为 0', async () => {
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByText('自评总分：')).toBeInTheDocument()
      })
      // 初始 self_score 为 null，加权后为 0
      expect(screen.getByText('0')).toBeInTheDocument()
    })
  })

  // ==================== 场景 4: 员工自我评价 ====================
  describe('员工自我评价', () => {
    it('应显示评价输入区域', async () => {
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByText('做得好的地方')).toBeInTheDocument()
      })
      expect(screen.getByText('需要改进的地方')).toBeInTheDocument()
    })

    it('应有评价输入框', async () => {
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByTestId('performance-self-good')).toBeInTheDocument()
      })
      expect(screen.getByTestId('performance-self-improvement')).toBeInTheDocument()
    })
  })

  // ==================== 场景 5: 提交自评 ====================
  describe('提交自评', () => {
    it('应渲染"提交自评"按钮', async () => {
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByTestId('performance-self-submit')).toBeInTheDocument()
      })
    })

    it('提交失败应显示错误消息', async () => {
      const user = userEvent.setup()
      mockSubmitGoalSelfEvaluation.mockRejectedValue({ response: { data: { message: '提交失败' } } })

      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByTestId('performance-self-submit')).toBeInTheDocument()
      })

      const actualInputs = screen.getAllByPlaceholderText('描述实际完成情况')
      for (const input of actualInputs) {
        await user.type(input, '已完成')
      }

      const scoreInputs = screen.getAllByRole('spinbutton')
      for (const input of scoreInputs) {
        await user.clear(input)
        await user.type(input, '80')
      }

      await user.click(screen.getByTestId('performance-self-submit'))

      await waitFor(() => {
        expect(screen.getAllByText('提交失败').length).toBeGreaterThanOrEqual(1)
      })
    })
  })

  // ==================== 场景 6: 接口失败 ====================
  describe('接口失败', () => {
    it('加载数据失败应显示错误消息', async () => {
      mockGetGoalRecords.mockRejectedValue(new Error('network'))
      mockGetParticipant.mockRejectedValue(new Error('network'))

      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByText('加载目标指标失败')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 7: 返回按钮 ====================
  describe('返回按钮', () => {
    it('点击返回按钮应调用 navigate(-1)', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getAllByText('绩效自评').length).toBeGreaterThanOrEqual(1)
      })

      const backBtn = screen.getByRole('button', { name: /返回/ })
      await user.click(backBtn)

      expect(mockNavigate).toHaveBeenCalledWith(-1)
    })
  })

  // ==================== 场景 8: 附加考核项 ====================
  describe('附加考核项', () => {
    it('有附加考核项时应显示附加考核项表格', async () => {
      mockGetGoalRecords.mockResolvedValue({
        data: {
          items: [
            ...makeGoalRecords().data.items,
            {
              id: 5,
              section_type: 'bonus_penalty',
              item_name: '附加项A',
              weight: 0.05,
              self_score: null,
              attachments: [],
              sort_order: 4,
            },
          ],
        },
      })

      render(React.createElement(PerformanceSelfEval))

      await waitFor(() => {
        expect(screen.getByText('附加考核项')).toBeInTheDocument()
      })
      expect(screen.getByText('附加项A')).toBeInTheDocument()
    })
  })
})
