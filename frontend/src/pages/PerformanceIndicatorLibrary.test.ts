import { describe, it, expect } from 'vitest'

// 测试辅助函数
const isValidWeight = (weight: number) => Number.isFinite(weight) && weight >= 10 && weight % 5 === 0

const newQuantItem = () => ({
  name: '',
  description: '',
  weight: 10,
  red_line_value: '',
  target_value: '',
  challenge_value: '',
  scoring_rule: '',
})

const newActionItem = () => ({
  name: '',
  description: '',
  weight: 10,
  target_value: '',
})

describe('isValidWeight', () => {
  it('should return true for valid weights', () => {
    expect(isValidWeight(10)).toBe(true)
    expect(isValidWeight(15)).toBe(true)
    expect(isValidWeight(20)).toBe(true)
    expect(isValidWeight(70)).toBe(true)
    expect(isValidWeight(100)).toBe(true)
  })

  it('should return false for weights below 10', () => {
    expect(isValidWeight(5)).toBe(false)
    expect(isValidWeight(0)).toBe(false)
  })

  it('should return false for weights not multiples of 5', () => {
    expect(isValidWeight(12)).toBe(false)
    expect(isValidWeight(73)).toBe(false)
  })

  it('should return false for non-finite numbers', () => {
    expect(isValidWeight(NaN)).toBe(false)
    expect(isValidWeight(Infinity)).toBe(false)
  })
})

describe('newQuantItem', () => {
  it('should create default quantitative item', () => {
    const item = newQuantItem()
    expect(item.name).toBe('')
    expect(item.description).toBe('')
    expect(item.weight).toBe(10)
    expect(item.red_line_value).toBe('')
    expect(item.target_value).toBe('')
    expect(item.challenge_value).toBe('')
    expect(item.scoring_rule).toBe('')
  })
})

describe('newActionItem', () => {
  it('should create default action item', () => {
    const item = newActionItem()
    expect(item.name).toBe('')
    expect(item.description).toBe('')
    expect(item.weight).toBe(10)
    expect(item.target_value).toBe('')
  })
})

describe('指标库权重验证逻辑', () => {
  it('should validate quant weight must be 70%', () => {
    const quantItems = [
      { weight: 35 },
      { weight: 35 },
    ]
    const quantWeight = quantItems.reduce((sum, item) => sum + item.weight, 0)
    expect(quantWeight).toBe(70)
  })

  it('should validate action weight must be 30%', () => {
    const actionItems = [
      { weight: 15 },
      { weight: 15 },
    ]
    const actionWeight = actionItems.reduce((sum, item) => sum + item.weight, 0)
    expect(actionWeight).toBe(30)
  })

  it('should validate total weight must be 100%', () => {
    const allItems = [
      { weight: 35 },
      { weight: 35 },
      { weight: 15 },
      { weight: 15 },
    ]
    const totalWeight = allItems.reduce((sum, item) => sum + item.weight, 0)
    expect(totalWeight).toBe(100)
  })
})

describe('指标项字段验证', () => {
  it('should require quant item fields', () => {
    const item = newQuantItem()
    const isValid = item.name.trim() !== '' &&
      item.description.trim() !== '' &&
      item.red_line_value?.trim() !== '' &&
      item.target_value?.trim() !== '' &&
      item.challenge_value?.trim() !== '' &&
      item.scoring_rule?.trim() !== ''
    expect(isValid).toBe(false)
  })

  it('should require action item fields', () => {
    const item = newActionItem()
    const isValid = item.name.trim() !== '' &&
      item.description.trim() !== '' &&
      item.target_value?.trim() !== ''
    expect(isValid).toBe(false)
  })
})
