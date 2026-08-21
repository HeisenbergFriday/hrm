import { describe, expect, it } from 'vitest'
import { unwrapApiData, unwrapApiItems, unwrapApiList } from './apiResponse'

describe('apiResponse helpers', () => {
  it('unwraps nested and direct response data without changing the existing fallback rules', () => {
    expect(unwrapApiData({ data: { value: 1 } }, { value: 0 })).toEqual({ value: 1 })
    expect(unwrapApiData({ value: 2 }, { value: 0 })).toEqual({ value: 2 })
    expect(unwrapApiData(null, { value: 0 })).toEqual({ value: 0 })
  })

  it('unwraps paged items', () => {
    expect(unwrapApiItems<number>({ data: { items: [1, 2] } })).toEqual([1, 2])
    expect(unwrapApiItems<number>({ data: {} })).toEqual([])
  })

  it('unwraps a named list and falls back to items', () => {
    expect(unwrapApiList<number>({ data: { departments: [1] } }, 'departments')).toEqual([1])
    expect(unwrapApiList<number>({ data: { items: [2] } }, 'departments')).toEqual([2])
  })
})
