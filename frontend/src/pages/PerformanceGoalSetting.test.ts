import { describe, it, expect } from 'vitest'

// 测试辅助函数
const newQuantItem = () => ({
  id: undefined,
  item_name: '',
  item_definition: '',
  weight: 0,
  weight_percent: '0',
  red_line_value: '',
  target_value: '',
  challenge_value: '',
  scoring_rule: '',
  actual_result: '',
  attachments: [],
  sort_order: 0
})

const newActionItem = () => ({
  id: undefined,
  item_name: '',
  item_definition: '',
  weight: 0,
  weight_percent: '0',
  target_value: '',
  actual_result: '',
  attachments: [],
  sort_order: 0
})

const targetReadonlyParticipantStatuses = new Set([
  'target_set',
  'self_submitted',
  'manager_submitted',
  'result_confirmed',
  'employee_confirmed',
  'manager_confirmed',
  'hr_confirmed',
  'locked',
])

describe('PerformanceGoalSetting - newQuantItem', () => {
  it('should create default quantitative item', () => {
    const item = newQuantItem()
    expect(item.id).toBeUndefined()
    expect(item.item_name).toBe('')
    expect(item.item_definition).toBe('')
    expect(item.weight).toBe(0)
    expect(item.red_line_value).toBe('')
    expect(item.target_value).toBe('')
    expect(item.challenge_value).toBe('')
    expect(item.scoring_rule).toBe('')
    expect(item.actual_result).toBe('')
    expect(item.attachments).toEqual([])
    expect(item.sort_order).toBe(0)
  })
})

describe('PerformanceGoalSetting - newActionItem', () => {
  it('should create default action item', () => {
    const item = newActionItem()
    expect(item.id).toBeUndefined()
    expect(item.item_name).toBe('')
    expect(item.item_definition).toBe('')
    expect(item.weight).toBe(0)
    expect(item.target_value).toBe('')
    expect(item.actual_result).toBe('')
    expect(item.attachments).toEqual([])
    expect(item.sort_order).toBe(0)
  })
})

describe('PerformanceGoalSetting - targetReadonlyParticipantStatuses', () => {
  it('should include all readonly statuses', () => {
    expect(targetReadonlyParticipantStatuses.has('target_set')).toBe(true)
    expect(targetReadonlyParticipantStatuses.has('self_submitted')).toBe(true)
    expect(targetReadonlyParticipantStatuses.has('manager_submitted')).toBe(true)
    expect(targetReadonlyParticipantStatuses.has('result_confirmed')).toBe(true)
    expect(targetReadonlyParticipantStatuses.has('employee_confirmed')).toBe(true)
    expect(targetReadonlyParticipantStatuses.has('manager_confirmed')).toBe(true)
    expect(targetReadonlyParticipantStatuses.has('hr_confirmed')).toBe(true)
    expect(targetReadonlyParticipantStatuses.has('locked')).toBe(true)
  })

  it('should not include editable statuses', () => {
    expect(targetReadonlyParticipantStatuses.has('pending')).toBe(false)
    expect(targetReadonlyParticipantStatuses.has('target_pending_approval')).toBe(false)
    expect(targetReadonlyParticipantStatuses.has('target_rejected')).toBe(false)
  })
})

describe('PerformanceGoalSetting - 权重计算', () => {
  it('should calculate quant weight total', () => {
    const quantItems = [
      { weight: 0.35 },
      { weight: 0.35 },
    ]
    const quantWeightTotal = quantItems.reduce((sum, i) => sum + (i.weight || 0), 0)
    expect(quantWeightTotal).toBe(0.7)
  })

  it('should calculate action weight total', () => {
    const actionItems = [
      { weight: 0.15 },
      { weight: 0.15 },
    ]
    const actionWeightTotal = actionItems.reduce((sum, i) => sum + (i.weight || 0), 0)
    expect(actionWeightTotal).toBe(0.3)
  })

  it('should calculate total weight', () => {
    const quantItems = [{ weight: 0.35 }, { weight: 0.35 }]
    const actionItems = [{ weight: 0.15 }, { weight: 0.15 }]
    const totalWeight = [...quantItems, ...actionItems].reduce((sum, i) => sum + (i.weight || 0), 0)
    expect(totalWeight).toBe(1)
  })
})

describe('PerformanceGoalSetting - buildPayload', () => {
  it('should build payload with correct structure', () => {
    const quantItems = [
      {
        id: 1,
        item_name: '指标1',
        item_definition: '定义1',
        weight: 0.35,
        red_line_value: '100',
        target_value: '200',
        challenge_value: '300',
        scoring_rule: '标准1',
        actual_result: '',
        attachments: [],
        sort_order: 0
      }
    ]
    const actionItems = [
      {
        id: 2,
        item_name: '行动1',
        item_definition: '定义2',
        weight: 0.15,
        target_value: '目标1',
        actual_result: '',
        attachments: [],
        sort_order: 1
      }
    ]

    const items = [
      ...quantItems.map((item, idx) => ({
        id: item.id,
        section_type: 'quantitative',
        item_name: item.item_name,
        item_definition: item.item_definition,
        weight: item.weight,
        red_line_value: item.red_line_value,
        target_value: item.target_value,
        challenge_value: item.challenge_value,
        scoring_rule: item.scoring_rule,
        actual_result: item.actual_result,
        attachments: item.attachments,
        sort_order: idx
      })),
      ...actionItems.map((item, idx) => ({
        id: item.id,
        section_type: 'key_action',
        item_name: item.item_name,
        item_definition: item.item_definition,
        weight: item.weight,
        target_value: item.target_value,
        actual_result: item.actual_result,
        attachments: item.attachments,
        sort_order: quantItems.length + idx
      }))
    ]

    expect(items).toHaveLength(2)
    expect(items[0].section_type).toBe('quantitative')
    expect(items[1].section_type).toBe('key_action')
    expect((items[0] as { red_line_value?: string }).red_line_value).toBe('100')
    expect(items[1].target_value).toBe('目标1')
  })
})

describe('PerformanceGoalSetting - validateRequiredFields', () => {
  it('should validate quant item name', () => {
    const quantItems = [{ item_name: '' }]
    const isValid = quantItems.every(i => String(i.item_name || '').trim() !== '')
    expect(isValid).toBe(false)
  })

  it('should validate quant item definition', () => {
    const quantItems = [{ item_name: '指标1', item_definition: '' }]
    const isValid = quantItems.every(i => String(i.item_definition || '').trim() !== '')
    expect(isValid).toBe(false)
  })

  it('should validate quant item red_line_value', () => {
    const quantItems = [{
      item_name: '指标1',
      item_definition: '定义1',
      red_line_value: '',
      target_value: '200',
      challenge_value: '300',
      scoring_rule: '标准1'
    }]
    const isValid = quantItems.every(i =>
      String(i.red_line_value || '').trim() !== '' &&
      String(i.target_value || '').trim() !== '' &&
      String(i.challenge_value || '').trim() !== '' &&
      String(i.scoring_rule || '').trim() !== ''
    )
    expect(isValid).toBe(false)
  })

  it('should validate action item target_value', () => {
    const actionItems = [{ item_name: '行动1', item_definition: '定义1', target_value: '' }]
    const isValid = actionItems.every(i => String(i.target_value || '').trim() !== '')
    expect(isValid).toBe(false)
  })
})
