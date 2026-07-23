import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { message } from 'antd'
import PerformanceExcelImportWizard from './PerformanceExcelImportWizard'
import { performanceAPI, type PerformanceActivityImportBatch } from '../services/api'

const makeDraft = (overrides: Record<string, unknown> = {}) => ({
  draft_key: 'xiaotie_employee',
  selected: true,
  source_sheet: '员工绩效',
  template_name: '员工绩效模板',
  activity_name: '员工绩效活动',
  flow_type: 'old',
  cycle_type: 'monthly',
  start_date: '',
  end_date: '',
  enable_bonus_score: false,
  employee_name: '张三',
  employee_user_id: 'user-1',
  employee_match: 'matched',
  sections: [{
    name: '量化指标',
    section_type: 'quantitative',
    weight: 70,
    is_score_required: true,
    is_comment_required: false,
    items: [],
  }],
  goals: [{
    section_type: 'quantitative',
    goal_type: 'kpi',
    is_fixed: false,
    item_name: '销售目标',
    weight: 100,
    sort_order: 1,
  }],
  source_weight_total: 100,
  ...overrides,
})

const makeBatch = (withDates = false): PerformanceActivityImportBatch => ({
  batch_id: 'batch-1',
  status: 'analyzed',
  created_at: '2026-07-17T10:00:00+08:00',
  preview: {
    source_type: 'xiaotie',
    source_label: '小铁文娱',
    file_name: 'performance.xlsx',
    file_sha256: 'hash',
    requires_review: true,
    issues: [{
      level: 'warning',
      code: 'section_weight_conflict',
      message: '模板文字为70/30，源表空白行实际为60/40，已按70/30换算',
    }],
    drafts: [
      makeDraft(withDates ? { start_date: '2026-07-01', end_date: '2026-07-31' } : {}),
      makeDraft({
        draft_key: 'xiaotie_values',
        selected: false,
        source_sheet: '季度价值观',
        template_name: '季度价值观模板',
        activity_name: '季度价值观活动',
        cycle_type: 'quarterly',
        employee_name: '',
        employee_user_id: '',
        employee_match: 'unmatched',
      }),
    ],
  },
})

function fileInput() {
  const input = document.querySelector('input[type="file"]')
  if (!input) throw new Error('file input not found')
  return input as HTMLInputElement
}

async function analyzeWorkbook(batch: PerformanceActivityImportBatch) {
  vi.spyOn(performanceAPI, 'analyzeActivityImport').mockResolvedValue({ data: batch } as any)
  const user = userEvent.setup()
  await user.upload(
    fileInput(),
    new File(['xlsx'], 'performance.xlsx', { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }),
  )
  await user.click(screen.getByRole('button', { name: '开始识别' }))
  await screen.findByText('已识别：小铁文娱')
  return user
}

describe('PerformanceExcelImportWizard', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(message, 'success').mockImplementation(() => undefined as any)
    vi.spyOn(message, 'warning').mockImplementation(() => undefined as any)
    vi.spyOn(message, 'error').mockImplementation(() => undefined as any)
  })

  it('识别后展示警告，并保持季度价值观默认不导入；日期为空时不提交', async () => {
    const commitSpy = vi.spyOn(performanceAPI, 'commitActivityImport').mockResolvedValue({} as any)
    vi.spyOn(performanceAPI, 'getScopeOptions').mockResolvedValue({ data: { employees: [] } } as any)
    render(
      <PerformanceExcelImportWizard open onCancel={vi.fn()} onCommitted={vi.fn()} />,
    )

    const user = await analyzeWorkbook(makeBatch(false))
    expect(screen.getByText(/模板文字为70\/30/)).toBeInTheDocument()
    expect(screen.getByText('默认不导入')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '去补充确认' }))
    expect(screen.getByRole('checkbox', { name: '员工绩效' })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: '季度价值观' })).not.toBeChecked()

    await user.click(screen.getByRole('button', { name: /确认创建/ }))
    expect(commitSpy).not.toHaveBeenCalled()
  })

  it('确认后提交全部草稿选择状态，并展示创建结果', async () => {
    const onCommitted = vi.fn()
    vi.spyOn(performanceAPI, 'getScopeOptions').mockResolvedValue({ data: { employees: [] } } as any)
    const commitSpy = vi.spyOn(performanceAPI, 'commitActivityImport').mockResolvedValue({
      data: {
        batch_id: 'batch-1',
        created: [{
          draft_key: 'xiaotie_employee',
          template_id: 10,
          template_reused: false,
          activity_id: 20,
          activity_name: '员工绩效活动',
          employee_user_id: 'user-1',
          goal_count: 1,
        }],
      },
    } as any)
    render(
      <PerformanceExcelImportWizard open onCancel={vi.fn()} onCommitted={onCommitted} />,
    )

    const user = await analyzeWorkbook(makeBatch(true))
    await user.click(screen.getByRole('button', { name: '去补充确认' }))
    await user.click(screen.getByRole('button', { name: /确认创建/ }))

    await waitFor(() => expect(commitSpy).toHaveBeenCalledTimes(1))
    expect(commitSpy).toHaveBeenCalledWith('batch-1', expect.arrayContaining([
      expect.objectContaining({ draft_key: 'xiaotie_employee', selected: true, employee_user_id: 'user-1' }),
      expect.objectContaining({ draft_key: 'xiaotie_values', selected: false }),
    ]))
    expect(await screen.findByText('绩效模板和草稿活动已创建')).toBeInTheDocument()
    expect(screen.getByText(/重复提交该批次不会重复创建/)).toBeInTheDocument()
    expect(onCommitted).toHaveBeenCalledTimes(1)
  })
})
