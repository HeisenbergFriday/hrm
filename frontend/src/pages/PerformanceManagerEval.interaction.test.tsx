/**
 * PerformanceManagerEval 组件交互测试
 *
 * 覆盖：
 * - 页面渲染与 loading 状态
 * - 指标列表展示
 * - 一键评分
 * - 总分计算与等级自动映射
 * - 提交评分
 * - 接口失败
 */
import React from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import PerformanceManagerEval from './PerformanceManagerEval'

// ==================== Mocks ====================

const mockNavigate = vi.fn()
const mockGetGoalRecords = vi.fn()
const mockGetParticipant = vi.fn()
const mockGetRealtimeDistributionCheck = vi.fn()
const mockSubmitGoalManagerEvaluation = vi.fn()
const mockAutoScoreGoalRecords = vi.fn()

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ activityId: '1', participantId: '101' }),
}))

vi.mock('../services/api', () => ({
  performanceAPI: {
    getGoalRecords: (...args: any[]) => mockGetGoalRecords(...args),
    getParticipant: (...args: any[]) => mockGetParticipant(...args),
    getRealtimeDistributionCheck: (...args: any[]) => mockGetRealtimeDistributionCheck(...args),
    submitGoalManagerEvaluation: (...args: any[]) => mockSubmitGoalManagerEvaluation(...args),
    autoScoreGoalRecords: (...args: any[]) => mockAutoScoreGoalRecords(...args),
  },
}))

vi.mock('../utils/authFileUrl', () => ({
  withFileAccessToken: (url: string) => url,
  useAuthorizedFileUrl: (url?: string) => url || '',
}))

// ==================== Mock 数据 ====================

function makeParticipant(overrides: Record<string, any> = {}) {
  return {
    id: 101,
    employee_name: '张三',
    employee_id: 'E001',
    status: 'self_submitted',
    manager_id: 'M001',
    suggested_level: null,
    final_level: null,
    manager_evaluation_good: '',
    manager_evaluation_improvement: '',
    ...overrides,
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
          actual_result: '95%',
          self_score: 85,
          manager_score: null,
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
          actual_result: '88',
          self_score: 90,
          manager_score: null,
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
          actual_result: '完成 12 家',
          self_score: 88,
          manager_score: null,
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
          actual_result: '完成',
          self_score: 95,
          manager_score: null,
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
  mockGetRealtimeDistributionCheck.mockResolvedValue({ data: { teams: [] } })
  mockSubmitGoalManagerEvaluation.mockResolvedValue({})
  mockAutoScoreGoalRecords.mockResolvedValue({
    data: {
      items: [
        { record_id: 1, score: 90, breakdown: '达标', auto_scored: true },
        { record_id: 2, score: 88, breakdown: '区间', auto_scored: true },
      ],
    },
  })
}

// ==================== 测试 ====================

describe('PerformanceManagerEval 交互测试', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setupDefaultMocks()
  })

  // ==================== 场景 1: 渲染与 loading ====================
  describe('渲染与 loading', () => {
    it('加载中应显示 Spin', async () => {
      mockGetGoalRecords.mockReturnValue(new Promise(() => {}))
      mockGetParticipant.mockReturnValue(new Promise(() => {}))
      mockGetRealtimeDistributionCheck.mockReturnValue(new Promise(() => {}))

      const { container } = render(
        React.createElement(PerformanceManagerEval)
      )
      expect(container.querySelector('.ant-spin')).toBeInTheDocument()
    })

    it('加载完成后应显示页面容器', async () => {
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByTestId('performance-manager-eval-page')).toBeInTheDocument()
      })
    })

    it('加载完成后应显示指标名称', async () => {
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByText('销售额')).toBeInTheDocument()
      })
      expect(screen.getByText('客户满意度')).toBeInTheDocument()
      expect(screen.getByText('客户拜访')).toBeInTheDocument()
      expect(screen.getByText('培训完成')).toBeInTheDocument()
    })
  })

  // ==================== 场景 2: 一键评分 ====================
  describe('一键评分', () => {
    it('应渲染"一键评分"按钮', async () => {
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByTestId('performance-manager-auto-score')).toBeInTheDocument()
      })
    })

    it('点击"一键评分"应调用 autoScoreGoalRecords', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByTestId('performance-manager-auto-score')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-manager-auto-score'))

      await waitFor(() => {
        expect(mockAutoScoreGoalRecords).toHaveBeenCalled()
      })
    })

    it('一键评分成功后应显示成功消息', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByTestId('performance-manager-auto-score')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-manager-auto-score'))

      await waitFor(() => {
        expect(screen.getAllByText(/已自动评分/).length).toBeGreaterThanOrEqual(1)
      })
    })

    it('新流程一键评分应将旧口径自动分转换为 0-10 分制', async () => {
      const user = userEvent.setup()
      mockGetParticipant.mockResolvedValue({
        data: {
          participant: makeParticipant(),
          activity: { id: 1, flow_type: 'new', status: 'manager_evaluation' },
        },
      })
      mockAutoScoreGoalRecords.mockResolvedValue({
        data: {
          items: [
            { record_id: 1, score: 46.25, breakdown: '低于红线', auto_scored: true },
            { record_id: 2, score: 120, breakdown: '超越挑战', auto_scored: true },
          ],
        },
      })

      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByTestId('performance-manager-auto-score')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-manager-auto-score'))

      const firstScoreControl = await screen.findByTestId('performance-manager-score-0')
      const secondScoreControl = await screen.findByTestId('performance-manager-score-1')
      const firstScoreInput = firstScoreControl instanceof HTMLInputElement
        ? firstScoreControl
        : firstScoreControl.querySelector('input')
      const secondScoreInput = secondScoreControl instanceof HTMLInputElement
        ? secondScoreControl
        : secondScoreControl.querySelector('input')

      await waitFor(() => {
        expect(Number.parseFloat(firstScoreInput?.value || '')).toBe(4.6)
        expect(Number.parseFloat(secondScoreInput?.value || '')).toBe(10)
      })
      expect(firstScoreInput?.value || '').not.toBe('46.25')
    })

    it('一键评分失败应显示错误消息', async () => {
      const user = userEvent.setup()
      mockAutoScoreGoalRecords.mockRejectedValue(new Error('score error'))

      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByTestId('performance-manager-auto-score')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-manager-auto-score'))

      await waitFor(() => {
        expect(screen.getByText('自动评分失败')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 3: 总分与等级 ====================
  describe('总分与等级', () => {
    it('应显示总分区域', async () => {
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByText('上级评分总分：')).toBeInTheDocument()
      })
    })

    it('应显示绩效等级选择', async () => {
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByText('绩效等级')).toBeInTheDocument()
      })
    })

    it('配额进度应按当前选择等级预览占用', async () => {
      mockGetParticipant.mockResolvedValue({
        data: {
          participant: makeParticipant({ suggested_level: 'D', final_level: 'A' }),
          activity: { id: 1, flow_type: 'new', status: 'manager_evaluation' },
        },
      })
      mockGetRealtimeDistributionCheck.mockResolvedValue({
        data: {
          teams: [
            {
              manager_id: 'M001',
              manager_name: '列德',
              total: 1,
              levels: {
                S: { current: 0, max: 1, percent: 5 },
                A: { current: 1, max: 1, percent: 15 },
                B: { current: 0, max: 1, percent: 60 },
                CD: { current: 0, max: 1, percent: 20 },
              },
            },
          ],
        },
      })

      render(React.createElement(PerformanceManagerEval))

      const quotaA = await screen.findByTestId('performance-quota-A')
      const quotaCD = await screen.findByTestId('performance-quota-CD')

      expect(within(quotaA).getByText(/0 \/ 1/)).toBeInTheDocument()
      expect(within(quotaCD).getByText(/1 \/ 1/)).toBeInTheDocument()
    })

    it('应显示评语输入区域', async () => {
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByText('做得好的地方')).toBeInTheDocument()
      })
      expect(screen.getByText('需要改进的地方')).toBeInTheDocument()
    })
  })

  // ==================== 场景 4: 提交评分 ====================
  describe('提交评分', () => {
    it('应渲染"提交评分"按钮', async () => {
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByTestId('performance-manager-submit')).toBeInTheDocument()
      })
    })

    it('提交失败应显示错误消息', async () => {
      const user = userEvent.setup()
      mockSubmitGoalManagerEvaluation.mockRejectedValue({ response: { data: { message: '提交失败' } } })

      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByTestId('performance-manager-submit')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-manager-submit'))

      await waitFor(() => {
        expect(screen.getByText('提交失败')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 5: 接口失败 ====================
  describe('接口失败', () => {
    it('加载数据失败应显示错误消息', async () => {
      mockGetGoalRecords.mockRejectedValue(new Error('network'))
      mockGetParticipant.mockRejectedValue(new Error('network'))
      mockGetRealtimeDistributionCheck.mockRejectedValue(new Error('network'))

      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByText('加载数据失败')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 6: 返回按钮 ====================
  describe('返回按钮', () => {
    it('点击返回按钮应调用 navigate(-1)', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /返回/ })).toBeInTheDocument()
      })

      const backBtn = screen.getByRole('button', { name: /返回/ })
      await user.click(backBtn)

      expect(mockNavigate).toHaveBeenCalledWith(-1)
    })
  })

  // ==================== 场景 7: 目标/评分规则列 ====================
  describe('目标/评分规则列', () => {
    it('量化指标应显示红线值、目标值、挑战值', async () => {
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByText('销售额')).toBeInTheDocument()
      })

      expect(screen.getByText(/红线: 80%/)).toBeInTheDocument()
      expect(screen.getByText(/目标: 100%/)).toBeInTheDocument()
      expect(screen.getByText(/挑战: 120%/)).toBeInTheDocument()
    })

    it('关键行动应显示目标值', async () => {
      render(React.createElement(PerformanceManagerEval))

      await waitFor(() => {
        expect(screen.getByText('客户拜访')).toBeInTheDocument()
      })

      expect(screen.getByText(/每月 10 家/)).toBeInTheDocument()
    })
  })

  // ==================== 场景 8: 自评附件预览 ====================
  describe('自评附件预览', () => {
    it('点击附件查看应打开预览弹窗并保留长文件名', async () => {
      const user = userEvent.setup()
      const longFileName = '023e2b6f1e66f673583170392d4ec17c.pdf'
      const records = makeGoalRecords().data.items.map((item, index) => (
        index === 0 ? { ...item, attachments: [`/api/v1/files/${longFileName}`] } : item
      ))
      mockGetGoalRecords.mockResolvedValue({ data: { items: records } })

      render(React.createElement(PerformanceManagerEval))

      const viewButton = await screen.findByRole('button', { name: /查看/ })
      await user.click(viewButton)

      expect(await screen.findByText('附件预览')).toBeInTheDocument()
      expect(screen.getByTestId('performance-manager-attachment-item-0')).toHaveAttribute('title', longFileName)
      expect(screen.getByTestId('performance-manager-attachment-current-name')).toHaveAttribute('title', longFileName)
      expect(screen.getByTestId('performance-manager-attachment-preview-frame')).toBeInTheDocument()
    })
  })
})
