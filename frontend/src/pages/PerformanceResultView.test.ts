import { describe, it, expect } from 'vitest'

// 测试辅助函数
const LEVEL_COLOR: Record<string, string> = {
  S: 'red', A: 'orange', B: 'green', C: 'gold', D: 'volcano'
}

const SECTION_LABEL: Record<string, string> = {
  quantitative: '量化指标',
  key_action: '关键行动',
  bonus_penalty: '附加考核项'
}

function formatScore(value?: number) {
  if (value === undefined || value === null) return '-'
  return Number(value).toFixed(0)
}

function formatDecimal(value?: number) {
  if (value === undefined || value === null) return '-'
  return Number(value).toFixed(1)
}

function formatWeight(value?: number) {
  if (!value) return '-'
  return `${(value * 100).toFixed(0)}%`
}

function formatDate(value?: string) {
  if (!value) return '-'
  return value.substring(0, 10)
}

function isPlaceholderSignature(value?: string) {
  const normalized = value?.trim().toLowerCase()
  return !normalized
}

function firstRealSignatureName(...names: (string | undefined)[]) {
  return names.find(name => !isPlaceholderSignature(name))?.trim()
}

function formatSignature(name?: string, date?: string) {
  const normalizedName = firstRealSignatureName(name)
  const normalizedDate = formatDate(date)

  if (!normalizedName && normalizedDate === '-') return '-'
  return [normalizedName || '-', normalizedDate].filter(part => part && part !== '-').join(' ')
}

function formatPeriod(startDate?: string, endDate?: string) {
  if (!startDate || !endDate) return '-'

  const start = startDate.substring(0, 10)
  const end = endDate.substring(0, 10)
  const [startYear, startMonth] = start.split('-')
  const [endYear, endMonth] = end.split('-')

  if (startYear && startMonth && startYear === endYear && startMonth === endMonth) {
    return `${startYear}年${Number(startMonth)}月`
  }

  return `${start} 至 ${end}`
}

function getWeightedScore(record: any, scoreType: 'self' | 'manager') {
  const score = scoreType === 'self' ? record.self_score : record.manager_score
  return (score || 0) * (record.weight || 0)
}

function getDownloadFileName(activity: any, participant: any) {
  const base = `${activity?.name || '绩效考核'}-${participant?.employee_name || '员工'}-个人绩效考核表`
  return base.replace(/[\\/:*?"<>|]/g, '_')
}

describe('PerformanceResultView - LEVEL_COLOR', () => {
  it('should have all level colors', () => {
    expect(LEVEL_COLOR.S).toBe('red')
    expect(LEVEL_COLOR.A).toBe('orange')
    expect(LEVEL_COLOR.B).toBe('green')
    expect(LEVEL_COLOR.C).toBe('gold')
    expect(LEVEL_COLOR.D).toBe('volcano')
  })
})

describe('PerformanceResultView - SECTION_LABEL', () => {
  it('should have all section labels', () => {
    expect(SECTION_LABEL.quantitative).toBe('量化指标')
    expect(SECTION_LABEL.key_action).toBe('关键行动')
    expect(SECTION_LABEL.bonus_penalty).toBe('附加考核项')
  })
})

describe('PerformanceResultView - formatScore', () => {
  it('should format score', () => {
    expect(formatScore(90)).toBe('90')
    expect(formatScore(85.5)).toBe('86')
    expect(formatScore(0)).toBe('0')
  })

  it('should return dash for undefined/null', () => {
    expect(formatScore(undefined)).toBe('-')
    expect(formatScore(null)).toBe('-')
  })
})

describe('PerformanceResultView - formatDecimal', () => {
  it('should format decimal', () => {
    expect(formatDecimal(90)).toBe('90.0')
    expect(formatDecimal(85.5)).toBe('85.5')
    expect(formatDecimal(0)).toBe('0.0')
  })

  it('should return dash for undefined/null', () => {
    expect(formatDecimal(undefined)).toBe('-')
    expect(formatDecimal(null)).toBe('-')
  })
})

describe('PerformanceResultView - formatWeight', () => {
  it('should format weight', () => {
    expect(formatWeight(0.35)).toBe('35%')
    expect(formatWeight(0.7)).toBe('70%')
    expect(formatWeight(1)).toBe('100%')
  })

  it('should return dash for undefined/null/0', () => {
    expect(formatWeight(undefined)).toBe('-')
    expect(formatWeight(null)).toBe('-')
    expect(formatWeight(0)).toBe('-')
  })
})

describe('PerformanceResultView - formatDate', () => {
  it('should format date', () => {
    expect(formatDate('2024-01-15T10:30:00')).toBe('2024-01-15')
    expect(formatDate('2024-12-31')).toBe('2024-12-31')
  })

  it('should return dash for undefined', () => {
    expect(formatDate(undefined)).toBe('-')
  })
})

describe('PerformanceResultView - isPlaceholderSignature', () => {
  it('should return true for empty/whitespace', () => {
    expect(isPlaceholderSignature('')).toBe(true)
    expect(isPlaceholderSignature(' ')).toBe(true)
    expect(isPlaceholderSignature(undefined)).toBe(true)
  })

  it('should return false for non-empty', () => {
    expect(isPlaceholderSignature('张三')).toBe(false)
    expect(isPlaceholderSignature('  张三  ')).toBe(false)
  })
})

describe('PerformanceResultView - firstRealSignatureName', () => {
  it('should return first non-placeholder name', () => {
    expect(firstRealSignatureName('', '张三', '李四')).toBe('张三')
    expect(firstRealSignatureName(undefined, '王五')).toBe('王五')
  })

  it('should return undefined if all are placeholders', () => {
    expect(firstRealSignatureName('', undefined, ' ')).toBeUndefined()
  })
})

describe('PerformanceResultView - formatSignature', () => {
  it('should format signature with name and date', () => {
    expect(formatSignature('张三', '2024-01-15T10:30:00')).toBe('张三 2024-01-15')
  })

  it('should format signature with name only', () => {
    expect(formatSignature('张三', undefined)).toBe('张三')
  })

  it('should format signature with date only', () => {
    expect(formatSignature(undefined, '2024-01-15T10:30:00')).toBe('2024-01-15')
  })

  it('should return dash for empty', () => {
    expect(formatSignature(undefined, undefined)).toBe('-')
  })
})

describe('PerformanceResultView - formatPeriod', () => {
  it('should format same month period', () => {
    expect(formatPeriod('2024-01-01', '2024-01-31')).toBe('2024年1月')
  })

  it('should format different month period', () => {
    expect(formatPeriod('2024-01-01', '2024-03-31')).toBe('2024-01-01 至 2024-03-31')
  })

  it('should return dash for missing dates', () => {
    expect(formatPeriod(undefined, undefined)).toBe('-')
    expect(formatPeriod('2024-01-01', undefined)).toBe('-')
    expect(formatPeriod(undefined, '2024-01-31')).toBe('-')
  })
})

describe('PerformanceResultView - getWeightedScore', () => {
  it('should calculate weighted score for self', () => {
    const record = { self_score: 90, weight: 0.35 }
    expect(getWeightedScore(record, 'self')).toBeCloseTo(31.5, 2)
  })

  it('should calculate weighted score for manager', () => {
    const record = { manager_score: 85, weight: 0.35 }
    expect(getWeightedScore(record, 'manager')).toBeCloseTo(29.75, 2)
  })

  it('should handle missing scores', () => {
    const record = { weight: 0.35 }
    expect(getWeightedScore(record, 'self')).toBe(0)
    expect(getWeightedScore(record, 'manager')).toBe(0)
  })

  it('should handle missing weight', () => {
    const record = { self_score: 90 }
    expect(getWeightedScore(record, 'self')).toBe(0)
  })
})

describe('PerformanceResultView - getDownloadFileName', () => {
  it('should generate download file name', () => {
    const activity = { name: '2024年Q1绩效考核' }
    const participant = { employee_name: '张三' }
    expect(getDownloadFileName(activity, participant)).toBe('2024年Q1绩效考核-张三-个人绩效考核表')
  })

  it('should handle missing activity/participant', () => {
    expect(getDownloadFileName(null, null)).toBe('绩效考核-员工-个人绩效考核表')
  })

  it('should replace special characters', () => {
    const activity = { name: '2024年Q1/绩效?考核' }
    const participant = { employee_name: '张三' }
    expect(getDownloadFileName(activity, participant)).toBe('2024年Q1_绩效_考核-张三-个人绩效考核表')
  })
})
