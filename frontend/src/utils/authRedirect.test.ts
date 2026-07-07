import { describe, expect, it, beforeEach } from 'vitest'
import {
  alignAuthRedirectTargetWithOrg,
  authOrgIDFromSearchParams,
  authOrgIDFromSearchParamsOrStorage,
  authRedirectTargetFromLocation,
  consumeAuthRedirect,
  directAuthOrgIDFromSearchParams,
  loginPathWithRedirectAndOrg,
  loginPathWithRedirect,
  normalizeAuthRedirectTarget,
  readRememberedAuthOrgID,
  rememberAuthOrgID,
  rememberAuthRedirect,
  resolveAuthOrgID,
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

  it('aligns redirect targets to the resolved organization', () => {
    expect(alignAuthRedirectTargetWithOrg('/employees?org_id=muteng&tab=all', 'xiaotie')).toBe('/employees?org_id=xiaotie&tab=all')
    expect(alignAuthRedirectTargetWithOrg('/employees?tab=all', 'xiaotie')).toBe('/employees?tab=all&org_id=xiaotie')
    expect(alignAuthRedirectTargetWithOrg('', 'xiaotie')).toBe('/?org_id=xiaotie')
  })

  it('builds login paths with org id for organization switching', () => {
    expect(loginPathWithRedirectAndOrg('/?org_id=muteng', 'muteng')).toBe('/login?org_id=muteng&redirect=%2F%3Forg_id%3Dmuteng')
    expect(loginPathWithRedirectAndOrg('/employees?org_id=xiaotie', 'xiaotie', 'scan')).toBe('/login?mode=scan&org_id=xiaotie&redirect=%2Femployees%3Forg_id%3Dxiaotie')
    expect(loginPathWithRedirectAndOrg('/login', 'muteng')).toBe('/login?org_id=muteng')
  })

  it('builds redirect target from a location-like object', () => {
    expect(authRedirectTargetFromLocation({
      pathname: '/performance-result/a/1',
      search: '?from=notice',
      hash: '#detail',
    })).toBe('/performance-result/a/1?from=notice#detail')
  })

  it('resolves org id from direct or redirected login params', () => {
    expect(authOrgIDFromSearchParams(new URLSearchParams('org_id=muteng'))).toBe('muteng')
    expect(authOrgIDFromSearchParams(new URLSearchParams('org=xiaotie'))).toBe('xiaotie')
    expect(authOrgIDFromSearchParams(new URLSearchParams('redirect=%2Femployees%3Forg_id%3Dmuteng'))).toBe('muteng')
    expect(authOrgIDFromSearchParams(new URLSearchParams('redirect=%2Flogin%3Forg_id%3Dmuteng'))).toBe('')
  })

  it('resolves direct login org id without reading redirect target', () => {
    expect(directAuthOrgIDFromSearchParams(new URLSearchParams('org_id=muteng'))).toBe('muteng')
    expect(directAuthOrgIDFromSearchParams(new URLSearchParams('org=xiaotie'))).toBe('xiaotie')
    expect(directAuthOrgIDFromSearchParams(new URLSearchParams('redirect=%2Femployees%3Forg_id%3Dmuteng'))).toBe('')
  })

  it('remembers org id during external auth redirects', () => {
    rememberAuthOrgID(' muteng ')

    expect(readRememberedAuthOrgID()).toBe('muteng')
    expect(authOrgIDFromSearchParamsOrStorage(new URLSearchParams('code=abc&state=xyz'))).toBe('muteng')
  })

  it('prefers explicit login org id over remembered or inferred org id', () => {
    expect(resolveAuthOrgID('xiaotie', 'muteng', [{ org_id: 'xiaotie' }, { org_id: 'muteng' }])).toBe('xiaotie')
  })

  it('falls back to remembered org id when no explicit selection exists', () => {
    expect(resolveAuthOrgID('', 'muteng', [{ org_id: 'default' }, { org_id: 'muteng' }])).toBe('muteng')
  })

  it('ignores stale org ids when active organizations are known', () => {
    expect(resolveAuthOrgID('old-org', 'muteng', [{ org_id: 'xiaotie' }, { org_id: 'robot' }])).toBe('')
  })

  it('falls back to the sole organization when there is only one option', () => {
    expect(resolveAuthOrgID('', '', [{ org_id: 'robot' }])).toBe('robot')
  })

  it('keeps org id empty when multiple organizations exist and none was chosen', () => {
    expect(resolveAuthOrgID('', '', [{ org_id: 'muteng' }, { org_id: 'xiaotie' }])).toBe('')
  })
})
