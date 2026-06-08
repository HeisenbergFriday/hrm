/**
 * PerformanceResultView 组件交互测试
 *
 * 覆盖：
 * - 页面渲染与 loading 状态
 * - 评分明细表格
 * - 绩效结果面板
 * - 确认进度 Timeline
 * - 确认按钮（员工/主管/HR）
 * - 接口失败
 * - 打印/导出按钮
 */
import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import PerformanceResultView from './PerformanceResultView'

// ==================== Mocks ====================

const mockNavigate = vi.fn()
const mockGetGoalRecords = vi.fn()
const mockGetParticipant = vi.fn()
const mockConfirmEmployeeResult = vi.fn()
const mockConfirmManagerResult = vi.fn()
const mockConfirmHRResult = vi.fn()
const mockSetBonusPenaltyScore = vi.fn()

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ activityId: '1', participantId: '101' }),
}))

vi.mock('../services/api', () => ({
  performanceAPI: {
    getGoalRecords: (...args: any[]) => mockGetGoalRecords(...args),
    getParticipant: (...args: any[]) => mockGetParticipant(...args),
    confirmEmployeeResult: (...args: any[]) => mockConfirmEmployeeResult(...args),
    confirmManagerResult: (...args: any[]) => mockConfirmManagerResult(...args),
    confirmHRResult: (...args: any[]) => mockConfirmHRResult(...args),
    setBonusPenaltyScore: (...args: any[]) => mockSetBonusPenaltyScore(...args),
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
    department_name: '技术部',
    position: '工程师',
    level: 'P6',
    manager_name: '李主管',
    status: 'manager_submitted',
    is_locked: false,
    self_score: 85,
    manager_score: 88,
    total_self_score: 85,
    total_manager_score: 88,
    adjusted_score: 88,
    bonus_score: 0,
    penalty_score: 0,
    final_level: 'B',
    suggested_level: 'B',
    self_evaluation_good: '表现良好',
    self_evaluation_improvement: '需要改进沟通',
    manager_evaluation_good: '技术能力强',
    manager_evaluation_improvement: '需要提升领导力',
    employee_confirmed_at: null,
    manager_confirmed_at: null,
    hr_confirmed_at: null,
    confirmed_at: null,
    locked_by: null,
    ...overrides,
  }
}

function makeRecords() {
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
          manager_score: 90,
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
          self_score: 80,
          manager_score: 85,
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
          manager_score: 90,
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
          manager_score: 92,
          sort_order: 3,
        },
      ],
    },
  }
}

function setupDefaultMocks() {
  mockGetGoalRecords.mockResolvedValue(makeRecords())
  mockGetParticipant.mockResolvedValue({
    data: {
      participant: makeParticipant(),
      activity: {
        id: 1,
        name: '2026年Q2绩效',
        status: 'manager_evaluation',
        start_date: '2026-04-01',
        end_date: '2026-06-30',
        enable_bonus_score: false,
      },
    },
  })
  mockConfirmEmployeeResult.mockResolvedValue({})
  mockConfirmManagerResult.mockResolvedValue({})
  mockConfirmHRResult.mockResolvedValue({})
  mockSetBonusPenaltyScore.mockResolvedValue({})
}

// ==================== 测试 ====================

describe('PerformanceResultView 交互测试', () => {
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
        React.createElement(PerformanceResultView)
      )
      expect(container.querySelector('.ant-spin')).toBeInTheDocument()
    })

    it('加载完成后应显示页面标题', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('绩效结果查看')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 2: 评分明细表格 ====================
  describe('评分明细表格', () => {
    it('应显示指标名称', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('销售额')).toBeInTheDocument()
      })
      expect(screen.getByText('客户满意度')).toBeInTheDocument()
      expect(screen.getByText('客户拜访')).toBeInTheDocument()
      expect(screen.getByText('培训完成')).toBeInTheDocument()
    })

    it('应显示类别标签', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getAllByText('量化指标').length).toBeGreaterThanOrEqual(1)
      })
      expect(screen.getAllByText('关键行动').length).toBeGreaterThanOrEqual(1)
    })

    it('应显示合计行', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('合计')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 3: 绩效结果面板 ====================
  describe('绩效结果面板', () => {
    it('应显示基础分数', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('基础分数')).toBeInTheDocument()
      })
    })

    it('应显示绩效等级', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('绩效等级')).toBeInTheDocument()
      })
    })

    it('应显示调整后分数', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('调整后分数')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 4: 确认进度 ====================
  describe('确认进度', () => {
    it('应显示确认进度标题', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('确认进度')).toBeInTheDocument()
      })
    })

    it('应显示待确认状态', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('待员工确认')).toBeInTheDocument()
      })
      expect(screen.getByText('待主管确认并冻结')).toBeInTheDocument()
    })
  })

  // ==================== 场景 5: 确认按钮 ====================
  describe('确认按钮', () => {
    it('manager_submitted 状态应显示"员工确认结果"按钮', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByTestId('performance-result-confirm-employee')).toBeInTheDocument()
      })
    })

    it('点击员工确认应调用 confirmEmployeeResult', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByTestId('performance-result-confirm-employee')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-result-confirm-employee'))

      await waitFor(() => {
        expect(mockConfirmEmployeeResult).toHaveBeenCalledWith(101)
      })
    })

    it('员工确认成功后应显示成功消息', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByTestId('performance-result-confirm-employee')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-result-confirm-employee'))

      await waitFor(() => {
        expect(screen.getAllByText('确认成功').length).toBeGreaterThanOrEqual(1)
      })
    })

    it('employee_confirmed 状态应显示"主管确认并冻结"按钮', async () => {
      mockGetParticipant.mockResolvedValue({
        data: {
          participant: makeParticipant({ status: 'employee_confirmed', employee_confirmed_at: '2026-06-01' }),
          activity: { id: 1, name: '测试', status: 'manager_evaluation', start_date: '2026-04-01', end_date: '2026-06-30', enable_bonus_score: false },
        },
      })

      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByTestId('performance-result-confirm-manager')).toBeInTheDocument()
      })
    })

    it('主管确认应弹出确认对话框', async () => {
      const user = userEvent.setup()
      mockGetParticipant.mockResolvedValue({
        data: {
          participant: makeParticipant({ status: 'employee_confirmed', employee_confirmed_at: '2026-06-01' }),
          activity: { id: 1, name: '测试', status: 'manager_evaluation', start_date: '2026-04-01', end_date: '2026-06-30', enable_bonus_score: false },
        },
      })

      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByTestId('performance-result-confirm-manager')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-result-confirm-manager'))

      // Modal.confirm 弹出
      await waitFor(() => {
        expect(screen.getAllByText('主管确认并冻结绩效结果').length).toBeGreaterThanOrEqual(1)
      })
    })
  })

  // ==================== 场景 6: 接口失败 ====================
  describe('接口失败', () => {
    it('加载数据失败应显示错误消息', async () => {
      mockGetGoalRecords.mockRejectedValue(new Error('network'))
      mockGetParticipant.mockRejectedValue(new Error('network'))

      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('加载数据失败')).toBeInTheDocument()
      })
    })

    it('确认失败应显示错误消息', async () => {
      const user = userEvent.setup()
      mockConfirmEmployeeResult.mockRejectedValue({ response: { data: { message: '确认失败' } } })

      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByTestId('performance-result-confirm-employee')).toBeInTheDocument()
      })

      await user.click(screen.getByTestId('performance-result-confirm-employee'))

      await waitFor(() => {
        expect(screen.getByText('确认失败')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 7: 返回按钮 ====================
  describe('返回按钮', () => {
    it('点击返回按钮应调用 navigate(-1)', async () => {
      const user = userEvent.setup()
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('绩效结果查看')).toBeInTheDocument()
      })

      const backBtn = screen.getByRole('button', { name: /返回/ })
      await user.click(backBtn)

      expect(mockNavigate).toHaveBeenCalledWith(-1)
    })
  })

  // ==================== 场景 8: 打印/导出 ====================
  describe('打印/导出', () => {
    it('应显示"打印 / 导出 PDF"按钮', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText(/打印.*导出 PDF/)).toBeInTheDocument()
      })
    })

    it('应显示"导出 Excel"按钮', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('导出 Excel')).toBeInTheDocument()
      })
    })
  })

  // ==================== 场景 9: 员工/上级评价 ====================
  describe('员工/上级评价', () => {
    it('应显示员工自我评价', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('员工自我评价')).toBeInTheDocument()
      })
      expect(screen.getByText('表现良好')).toBeInTheDocument()
      expect(screen.getByText('需要改进沟通')).toBeInTheDocument()
    })

    it('应显示上级总体评价', async () => {
      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getByText('上级总体评价')).toBeInTheDocument()
      })
      expect(screen.getByText('技术能力强')).toBeInTheDocument()
      expect(screen.getByText('需要提升领导力')).toBeInTheDocument()
    })
  })

  // ==================== 场景 10: 已冻结状态 ====================
  describe('已冻结状态', () => {
    it('已锁定时应显示"已冻结"标签', async () => {
      mockGetParticipant.mockResolvedValue({
        data: {
          participant: makeParticipant({ is_locked: true, status: 'locked' }),
          activity: { id: 1, name: '测试', status: 'locked', start_date: '2026-04-01', end_date: '2026-06-30', enable_bonus_score: false },
        },
      })

      render(React.createElement(PerformanceResultView))

      await waitFor(() => {
        expect(screen.getAllByText('已冻结').length).toBeGreaterThanOrEqual(1)
      })
    })
  })
})
