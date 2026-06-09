import { describe, it, expect } from 'vitest'

// 测试辅助函数
const LEVEL_OPTIONS = [
  { value: 'S', label: 'S - 杰出', color: '#f50' },
  { value: 'A', label: 'A - 优秀', color: '#2db7f5' },
  { value: 'B', label: 'B - 良好', color: '#87d068' },
  { value: 'C', label: 'C - 待改进', color: '#faad14' },
  { value: 'D', label: 'D - 不合格', color: '#ff4d4f' },
]

const calcPerformanceLevel = (score: number) => {
  if (score >= 100) return 'S'
  if (score >= 90) return 'A'
  if (score >= 80) return 'B'
  if (score >= 60) return 'C'
  return 'D'
}

const calcTotalManagerScore = (items: any[]) => {
  const total = items.reduce((sum, i) => sum + (i.manager_score || 0) * (i.weight || 0), 0)
  return Math.round(total * 100) / 100
}

describe('PerformanceManagerEval - LEVEL_OPTIONS', () => {
  it('should have all level options', () => {
    expect(LEVEL_OPTIONS).toHaveLength(5)
    expect(LEVEL_OPTIONS[0].value).toBe('S')
    expect(LEVEL_OPTIONS[1].value).toBe('A')
    expect(LEVEL_OPTIONS[2].value).toBe('B')
    expect(LEVEL_OPTIONS[3].value).toBe('C')
    expect(LEVEL_OPTIONS[4].value).toBe('D')
  })

  it('should have correct labels', () => {
    expect(LEVEL_OPTIONS[0].label).toBe('S - 杰出')
    expect(LEVEL_OPTIONS[1].label).toBe('A - 优秀')
    expect(LEVEL_OPTIONS[2].label).toBe('B - 良好')
    expect(LEVEL_OPTIONS[3].label).toBe('C - 待改进')
    expect(LEVEL_OPTIONS[4].label).toBe('D - 不合格')
  })

  it('should have correct colors', () => {
    expect(LEVEL_OPTIONS[0].color).toBe('#f50')
    expect(LEVEL_OPTIONS[1].color).toBe('#2db7f5')
    expect(LEVEL_OPTIONS[2].color).toBe('#87d068')
    expect(LEVEL_OPTIONS[3].color).toBe('#faad14')
    expect(LEVEL_OPTIONS[4].color).toBe('#ff4d4f')
  })
})

describe('PerformanceManagerEval - calcPerformanceLevel', () => {
  it('should return S for score >= 100', () => {
    expect(calcPerformanceLevel(100)).toBe('S')
    expect(calcPerformanceLevel(110)).toBe('S')
    expect(calcPerformanceLevel(120)).toBe('S')
  })

  it('should return A for score >= 90', () => {
    expect(calcPerformanceLevel(90)).toBe('A')
    expect(calcPerformanceLevel(95)).toBe('A')
    expect(calcPerformanceLevel(99)).toBe('A')
  })

  it('should return B for score >= 80', () => {
    expect(calcPerformanceLevel(80)).toBe('B')
    expect(calcPerformanceLevel(85)).toBe('B')
    expect(calcPerformanceLevel(89)).toBe('B')
  })

  it('should return C for score >= 60', () => {
    expect(calcPerformanceLevel(60)).toBe('C')
    expect(calcPerformanceLevel(70)).toBe('C')
    expect(calcPerformanceLevel(79)).toBe('C')
  })

  it('should return D for score < 60', () => {
    expect(calcPerformanceLevel(0)).toBe('D')
    expect(calcPerformanceLevel(30)).toBe('D')
    expect(calcPerformanceLevel(59)).toBe('D')
  })
})

describe('PerformanceManagerEval - calcTotalManagerScore', () => {
  it('should calculate total manager score', () => {
    const items = [
      { manager_score: 90, weight: 0.35 },
      { manager_score: 80, weight: 0.35 },
      { manager_score: 85, weight: 0.15 },
      { manager_score: 90, weight: 0.15 },
    ]
    const total = calcTotalManagerScore(items)
    expect(total).toBeCloseTo(85.75, 2)
  })

  it('should handle empty items', () => {
    const items: any[] = []
    const total = calcTotalManagerScore(items)
    expect(total).toBe(0)
  })

  it('should handle missing manager_score', () => {
    const items = [
      { manager_score: 0, weight: 0.35 },
      { weight: 0.35 },
    ]
    const total = calcTotalManagerScore(items)
    expect(total).toBe(0)
  })

  it('should handle missing weight', () => {
    const items = [
      { manager_score: 90, weight: 0 },
      { manager_score: 80 },
    ]
    const total = calcTotalManagerScore(items)
    expect(total).toBe(0)
  })
})

describe('PerformanceManagerEval - 自动评分等级映射', () => {
  it('should map score to level correctly', () => {
    expect(calcPerformanceLevel(100)).toBe('S')
    expect(calcPerformanceLevel(90)).toBe('A')
    expect(calcPerformanceLevel(80)).toBe('B')
    expect(calcPerformanceLevel(60)).toBe('C')
    expect(calcPerformanceLevel(50)).toBe('D')
  })

  it('should handle boundary values', () => {
    expect(calcPerformanceLevel(99.99)).toBe('A')
    expect(calcPerformanceLevel(79.99)).toBe('C')
    expect(calcPerformanceLevel(59.99)).toBe('D')
  })
})

describe('PerformanceManagerEval - 权重百分比转换', () => {
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
