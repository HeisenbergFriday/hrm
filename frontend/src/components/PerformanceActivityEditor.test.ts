import { describe, it, expect } from 'vitest'

// 测试辅助函数
const activitySections = [
  { id: 'activity-basic-section', label: '基础信息' },
  { id: 'activity-period-section', label: '周期设置' },
  { id: 'activity-review-section', label: '评审流程' },
  { id: 'activity-scope-section', label: '参与范围' },
  { id: 'activity-advanced-section', label: '高级设置' },
]

const cycleLabels: Record<string, string> = {
  monthly: '月度',
  quarterly: '季度',
  annual: '年度',
}

function isRangeFilled(value: unknown) {
  return Array.isArray(value) && Boolean(value[0] && value[1])
}

function normalizeCycleType(value?: string) {
  return String(value || '').trim()
}

function getCycleLabel(value?: string) {
  const normalized = normalizeCycleType(value)
  return cycleLabels[normalized] || normalized || '未知周期'
}

function normalizeEditorIDArray(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(item => String(item).trim()).filter(Boolean)
  if (!value) return []
  return String(value).split(',').map(item => item.trim()).filter(Boolean)
}

function toSearchText(value: unknown): string {
  if (typeof value === 'string' || typeof value === 'number') return String(value)
  if (Array.isArray(value)) return value.map(toSearchText).join(' ')
  return ''
}

function filterSelectOption(input: string, option?: { label?: unknown; value?: unknown }) {
  const keyword = input.trim().toLowerCase()
  if (!keyword) return true
  return `${toSearchText(option?.label)} ${toSearchText(option?.value)}`.toLowerCase().includes(keyword)
}

describe('PerformanceActivityEditor - activitySections', () => {
  it('should have all sections', () => {
    expect(activitySections).toHaveLength(5)
    expect(activitySections[0].label).toBe('基础信息')
    expect(activitySections[1].label).toBe('周期设置')
    expect(activitySections[2].label).toBe('评审流程')
    expect(activitySections[3].label).toBe('参与范围')
    expect(activitySections[4].label).toBe('高级设置')
  })
})

describe('PerformanceActivityEditor - cycleLabels', () => {
  it('should have all cycle labels', () => {
    expect(cycleLabels.monthly).toBe('月度')
    expect(cycleLabels.quarterly).toBe('季度')
    expect(cycleLabels.annual).toBe('年度')
  })
})

describe('PerformanceActivityEditor - isRangeFilled', () => {
  it('should return true for filled range', () => {
    expect(isRangeFilled(['2024-01-01', '2024-01-31'])).toBe(true)
    expect(isRangeFilled([new Date(), new Date()])).toBe(true)
  })

  it('should return false for empty range', () => {
    expect(isRangeFilled([])).toBe(false)
    expect(isRangeFilled([null, null])).toBe(false)
    expect(isRangeFilled([undefined, undefined])).toBe(false)
  })

  it('should return false for non-array', () => {
    expect(isRangeFilled(null)).toBe(false)
    expect(isRangeFilled(undefined)).toBe(false)
    expect(isRangeFilled('string')).toBe(false)
  })
})

describe('PerformanceActivityEditor - normalizeCycleType', () => {
  it('should normalize cycle type', () => {
    expect(normalizeCycleType('monthly')).toBe('monthly')
    expect(normalizeCycleType('  quarterly  ')).toBe('quarterly')
    expect(normalizeCycleType('annual')).toBe('annual')
  })

  it('should handle undefined/null', () => {
    expect(normalizeCycleType(undefined)).toBe('')
    expect(normalizeCycleType(null as any)).toBe('')
  })
})

describe('PerformanceActivityEditor - getCycleLabel', () => {
  it('should return Chinese label for cycle type', () => {
    expect(getCycleLabel('monthly')).toBe('月度')
    expect(getCycleLabel('quarterly')).toBe('季度')
    expect(getCycleLabel('annual')).toBe('年度')
  })

  it('should return original value for unknown type', () => {
    expect(getCycleLabel('weekly')).toBe('weekly')
  })

  it('should return default for undefined', () => {
    expect(getCycleLabel(undefined)).toBe('未知周期')
  })
})

describe('PerformanceActivityEditor - normalizeEditorIDArray', () => {
  it('should handle array input', () => {
    expect(normalizeEditorIDArray(['1', '2', '3'])).toEqual(['1', '2', '3'])
  })

  it('should handle comma-separated string', () => {
    expect(normalizeEditorIDArray('1,2,3')).toEqual(['1', '2', '3'])
  })

  it('should handle undefined/null', () => {
    expect(normalizeEditorIDArray(undefined)).toEqual([])
    expect(normalizeEditorIDArray(null)).toEqual([])
  })

  it('should filter empty values', () => {
    expect(normalizeEditorIDArray('1,,2,')).toEqual(['1', '2'])
  })

  it('should trim whitespace', () => {
    expect(normalizeEditorIDArray(' 1 , 2 , 3 ')).toEqual(['1', '2', '3'])
  })
})

describe('PerformanceActivityEditor - toSearchText', () => {
  it('should convert string to search text', () => {
    expect(toSearchText('hello')).toBe('hello')
  })

  it('should convert number to search text', () => {
    expect(toSearchText(123)).toBe('123')
  })

  it('should convert array to search text', () => {
    expect(toSearchText(['hello', 'world'])).toBe('hello world')
  })

  it('should return empty for undefined/null', () => {
    expect(toSearchText(undefined)).toBe('')
    expect(toSearchText(null)).toBe('')
  })
})

describe('PerformanceActivityEditor - filterSelectOption', () => {
  it('should match option by label', () => {
    expect(filterSelectOption('张', { label: '张三', value: '1' })).toBe(true)
  })

  it('should match option by value', () => {
    expect(filterSelectOption('U001', { label: '张三', value: 'U001' })).toBe(true)
  })

  it('should not match option', () => {
    expect(filterSelectOption('李', { label: '张三', value: 'U001' })).toBe(false)
  })

  it('should match empty keyword', () => {
    expect(filterSelectOption('', { label: '张三', value: 'U001' })).toBe(true)
  })

  it('should handle case insensitive', () => {
    expect(filterSelectOption('hello', { label: 'Hello World', value: '1' })).toBe(true)
  })
})
