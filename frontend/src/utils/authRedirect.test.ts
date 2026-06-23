import { describe, expect, it, beforeEach } from 'vitest'
import {
  authRedirectTargetFromLocation,
  consumeAuthRedirect,
  loginPathWithRedirect,
  normalizeAuthRedirectTarget,
  rememberAuthRedirect,
} from './authRedirect'

describe('authRedirect', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
  })

  it('keeps only same-origin app paths', () => {
    expect(normalizeAuthRedirectTarget('/performance-self-eval/a/1')).toBe('/performance-self-eval/a/1')
    expect(normalizeAuthRedirectTarget('/performance-self-eval/a/1?tab=todo#top')).toBe('/performance-self-eval/a/1?tab=todo#top')
    expect(normalizeAuthRedirectTarget('https://example.com/a')).toBe('')
    expect(normalizeAuthRedirectTarget('//example.com/a')).toBe('')
    expect(normalizeAuthRedirectTarget('performance-self-eval/a/1')).toBe('')
  })

  it('rejects auth pages to avoid redirect loops', () => {
    expect(normalizeAuthRedirectTarget('/login')).toBe('')
    expect(normalizeAuthRedirectTarget('/login?redirect=%2Fperformance')).toBe('')
    expect(normalizeAuthRedirectTarget('/callback?code=1')).toBe('')
    expect(normalizeAuthRedirectTarget('/login-error')).toBe('')
  })

  it('stores and consumes a remembered target once', () => {
    rememberAuthRedirect('/performance-self-eval/activity/7')

    expect(consumeAuthRedirect()).toBe('/performance-self-eval/activity/7')
    expect(consumeAuthRedirect()).toBe('')
  })

  it('builds login paths with encoded redirect targets', () => {
    expect(loginPathWithRedirect('/performance-self-eval/activity /7')).toBe('/login?redirect=%2Fperformance-self-eval%2Factivity%20%2F7')
    expect(loginPathWithRedirect('/login')).toBe('/login')
  })

  it('builds redirect target from a location-like object', () => {
    expect(authRedirectTargetFromLocation({
      pathname: '/performance-result/a/1',
      search: '?from=notice',
      hash: '#detail',
    })).toBe('/performance-result/a/1?from=notice#detail')
  })
})
