import { describe, it, expect } from 'vitest'

// 测试辅助函数
const calcTotalSelfScore = (items: any[]) => {
  const total = items.reduce((sum: number, i: any) => sum + (i.self_score || 0) * (i.weight || 0), 0)
  return Math.round(total * 100) / 100
}

const filterItemsBySectionType = (items: any[], sectionType: string) => {
  return items.filter(item => item.section_type === sectionType)
}

const buildSubmitPayload = (values: any) => {
  const items = values.items.map((i: any) => ({
    record_id: i.record_id,
    actual_result: i.actual_result,
    self_score: i.self_score
  }))

  const bonusItems = (values.bonus_items || []).map((i: any) => ({
    record_id: i.record_id,
    self_score: i.self_score || 0
  }))

  return {
    items,
    bonus_items: bonusItems,
    evaluation_good: values.evaluation_good || '',
    evaluation_improvement: values.evaluation_improvement || ''
  }
}

describe('PerformanceSelfEval - calcTotalSelfScore', () => {
  it('should calculate total self score', () => {
    const items = [
      { self_score: 90, weight: 0.35 },
      { self_score: 80, weight: 0.35 },
      { self_score: 85, weight: 0.15 },
      { self_score: 90, weight: 0.15 },
    ]
    const total = calcTotalSelfScore(items)
    expect(total).toBeCloseTo(85.75, 2)
  })

  it('should handle empty items', () => {
    const items: any[] = []
    const total = calcTotalSelfScore(items)
    expect(total).toBe(0)
  })

  it('should handle missing self_score', () => {
    const items = [
      { self_score: 0, weight: 0.35 },
      { weight: 0.35 },
    ]
    const total = calcTotalSelfScore(items)
    expect(total).toBe(0)
  })

  it('should handle missing weight', () => {
    const items = [
      { self_score: 90, weight: 0 },
      { self_score: 80 },
    ]
    const total = calcTotalSelfScore(items)
    expect(total).toBe(0)
  })
})

describe('PerformanceSelfEval - filterItemsBySectionType', () => {
  it('should filter quantitative items', () => {
    const items = [
      { section_type: 'quantitative', item_name: '指标1' },
      { section_type: 'key_action', item_name: '行动1' },
      { section_type: 'quantitative', item_name: '指标2' },
    ]
    const result = filterItemsBySectionType(items, 'quantitative')
    expect(result).toHaveLength(2)
    expect(result[0].item_name).toBe('指标1')
    expect(result[1].item_name).toBe('指标2')
  })

  it('should filter key_action items', () => {
    const items = [
      { section_type: 'quantitative', item_name: '指标1' },
      { section_type: 'key_action', item_name: '行动1' },
    ]
    const result = filterItemsBySectionType(items, 'key_action')
    expect(result).toHaveLength(1)
    expect(result[0].item_name).toBe('行动1')
  })

  it('should return empty for no match', () => {
    const items = [
      { section_type: 'quantitative', item_name: '指标1' },
    ]
    const result = filterItemsBySectionType(items, 'bonus_penalty')
    expect(result).toHaveLength(0)
  })
})

describe('PerformanceSelfEval - buildSubmitPayload', () => {
  it('should build submit payload', () => {
    const values = {
      items: [
        { record_id: 1, actual_result: '结果1', self_score: 90 },
        { record_id: 2, actual_result: '结果2', self_score: 80 },
      ],
      bonus_items: [
        { record_id: 3, self_score: 10 },
      ],
      evaluation_good: '表现良好',
      evaluation_improvement: '需要改进'
    }

    const payload = buildSubmitPayload(values)
    expect(payload.items).toHaveLength(2)
    expect(payload.bonus_items).toHaveLength(1)
    expect(payload.evaluation_good).toBe('表现良好')
    expect(payload.evaluation_improvement).toBe('需要改进')
  })

  it('should handle missing bonus_items', () => {
    const values = {
      items: [{ record_id: 1, actual_result: '结果1', self_score: 90 }],
      evaluation_good: '',
      evaluation_improvement: ''
    }

    const payload = buildSubmitPayload(values)
    expect(payload.items).toHaveLength(1)
    expect(payload.bonus_items).toEqual([])
  })

  it('should handle missing evaluation fields', () => {
    const values = {
      items: [{ record_id: 1, actual_result: '结果1', self_score: 90 }],
    }

    const payload = buildSubmitPayload(values)
    expect(payload.evaluation_good).toBe('')
    expect(payload.evaluation_improvement).toBe('')
  })
})

describe('PerformanceSelfEval - 权重百分比转换', () => {
  it('should convert weight to percent', () => {
    const weight = 0.35
    const percent = (weight * 100).toFixed(0)
    expect(percent).toBe('35')
  })

  it('should handle zero weight', () => {
    const weight = 0
    const percent = (weight * 100).toFixed(0)
    expect(percent).toBe('0')
  })
})
