import { describe, expect, it } from 'vitest'
import { getNewFlowResultProgressPhases } from './PerformanceResultView'

describe('getNewFlowResultProgressPhases', () => {
  it('keeps independent interview stage outside result confirmation progress', () => {
    expect(getNewFlowResultProgressPhases('interview')).toEqual(['pending', 'pending', 'pending'])
  })

  it('resets later result progress when participant was manually moved back', () => {
    expect(getNewFlowResultProgressPhases('interview', 'manager_submitted')).toEqual(['pending', 'pending', 'pending'])
  })

  it('keeps completed participant confirmations while independent interview is active', () => {
    expect(getNewFlowResultProgressPhases('interview', 'result_confirmed')).toEqual(['done', 'done', 'pending'])
  })
})
