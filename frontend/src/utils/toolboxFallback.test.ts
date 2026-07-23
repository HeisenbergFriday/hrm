import { describe, expect, it } from 'vitest'
import { shouldFallbackToLegacyToolboxAPI } from './toolboxFallback'

describe('shouldFallbackToLegacyToolboxAPI', () => {
  it('allows fallback only for 404/405/501', () => {
    expect(shouldFallbackToLegacyToolboxAPI({ response: { status: 404 } })).toBe(true)
    expect(shouldFallbackToLegacyToolboxAPI({ response: { status: 405 } })).toBe(true)
    expect(shouldFallbackToLegacyToolboxAPI({ response: { status: 501 } })).toBe(true)
    expect(shouldFallbackToLegacyToolboxAPI({ status: 404 })).toBe(true)
  })

  it('rejects auth / validation / business / server errors', () => {
    for (const status of [400, 401, 403, 409, 410, 422, 429, 500, 502, 503, 504]) {
      expect(shouldFallbackToLegacyToolboxAPI({ response: { status } })).toBe(false)
    }
  })

  it('rejects timeout and network errors', () => {
    expect(shouldFallbackToLegacyToolboxAPI({ code: 'ECONNABORTED', message: 'timeout of 600000ms exceeded' })).toBe(false)
    expect(shouldFallbackToLegacyToolboxAPI({ code: 'ERR_NETWORK', message: 'Network Error' })).toBe(false)
    expect(shouldFallbackToLegacyToolboxAPI({ code: 'ERR_CANCELED' })).toBe(false)
    expect(shouldFallbackToLegacyToolboxAPI({ message: 'Failed to fetch' })).toBe(false)
  })

  it('rejects unknown errors by default', () => {
    expect(shouldFallbackToLegacyToolboxAPI(null)).toBe(false)
    expect(shouldFallbackToLegacyToolboxAPI(new Error('boom'))).toBe(false)
    expect(shouldFallbackToLegacyToolboxAPI({})).toBe(false)
  })
})
