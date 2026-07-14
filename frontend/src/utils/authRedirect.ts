const storageKey = 'peopleops-auth-redirect'
const orgStorageKey = 'peopleops-auth-org-id'

const authPathPrefixes = ['/login', '/callback', '/login-error']

export function normalizeAuthRedirectTarget(value?: string | null): string {
  const target = (value || '').trim()
  if (!target || !target.startsWith('/') || target.startsWith('//')) {
    return ''
  }
  if (authPathPrefixes.some(path => target === path || target.startsWith(`${path}?`) || target.startsWith(`${path}#`))) {
    return ''
  }
  return target
}

export function authRedirectTargetFromLocation(location: { pathname: string; search?: string; hash?: string }): string {
  return normalizeAuthRedirectTarget(`${location.pathname || ''}${location.search || ''}${location.hash || ''}`)
}

export function rememberAuthRedirect(target?: string | null) {
  const normalized = normalizeAuthRedirectTarget(target)
  if (!normalized) return

  try {
    window.sessionStorage.setItem(storageKey, normalized)
  } catch {
    // Ignore storage failures; login can still fall back to home.
  }
}

export function normalizeAuthOrgID(value?: string | null): string {
  return (value || '').trim()
}

export function rememberAuthOrgID(orgID?: string | null) {
  const normalized = normalizeAuthOrgID(orgID)
  if (!normalized) return

  try {
    window.sessionStorage.setItem(orgStorageKey, normalized)
  } catch {
    // Ignore storage failures; login requests can still include org_id directly.
  }
}

export function readRememberedAuthOrgID(): string {
  try {
    return normalizeAuthOrgID(window.sessionStorage.getItem(orgStorageKey))
  } catch {
    return ''
  }
}

export function consumeAuthRedirect(): string {
  try {
    const target = normalizeAuthRedirectTarget(window.sessionStorage.getItem(storageKey))
    window.sessionStorage.removeItem(storageKey)
    return target
  } catch {
    return ''
  }
}

export function loginPathWithRedirect(target?: string | null): string {
  const normalized = normalizeAuthRedirectTarget(target)
  if (!normalized) return '/login'
  return `/login?redirect=${encodeURIComponent(normalized)}`
}

export function alignAuthRedirectTargetWithOrg(target?: string | null, orgID?: string | null): string {
  const normalizedTarget = normalizeAuthRedirectTarget(target)
  const normalizedOrgID = normalizeAuthOrgID(orgID)

  if (!normalizedTarget) {
    return normalizedOrgID ? `/?org_id=${encodeURIComponent(normalizedOrgID)}` : '/'
  }

  if (!normalizedOrgID) {
    return normalizedTarget
  }

  try {
    const parsed = new URL(normalizedTarget, window.location.origin)
    parsed.searchParams.set('org_id', normalizedOrgID)
    parsed.searchParams.delete('org')
    return `${parsed.pathname}${parsed.search}${parsed.hash}`
  } catch {
    return normalizedTarget
  }
}

export function loginPathWithRedirectAndOrg(target?: string | null, orgID?: string | null, mode?: string | null): string {
  const params = new URLSearchParams()
  const normalizedMode = (mode || '').trim()
  const normalizedOrgID = normalizeAuthOrgID(orgID)
  const normalizedTarget = normalizeAuthRedirectTarget(target)

  if (normalizedMode) params.set('mode', normalizedMode)
  if (normalizedOrgID) params.set('org_id', normalizedOrgID)
  if (normalizedTarget) params.set('redirect', normalizedTarget)

  const query = params.toString()
  return query ? `/login?${query}` : '/login'
}

export function directAuthOrgIDFromSearchParams(searchParams: URLSearchParams): string {
  return (searchParams.get('org_id') || searchParams.get('org') || '').trim()
}

export function authOrgIDFromSearchParams(searchParams: URLSearchParams): string {
  const direct = directAuthOrgIDFromSearchParams(searchParams)
  if (direct) return direct

  const redirect = normalizeAuthRedirectTarget(searchParams.get('redirect'))
  if (!redirect) return ''

  try {
    const parsed = new URL(redirect, window.location.origin)
    return (parsed.searchParams.get('org_id') || parsed.searchParams.get('org') || '').trim()
  } catch {
    return ''
  }
}

export function authOrgIDFromSearchParamsOrStorage(searchParams: URLSearchParams): string {
  return authOrgIDFromSearchParams(searchParams) || readRememberedAuthOrgID()
}

export interface AuthOrganizationOption {
  org_id: string
}

function isKnownOrganization(
  orgID: string,
  organizations?: ReadonlyArray<AuthOrganizationOption> | null,
): boolean {
  if (!organizations || organizations.length === 0) {
    return true
  }
  return organizations.some((org) => normalizeAuthOrgID(org.org_id) === orgID)
}

export function resolveAuthOrgID(
  selectedOrgID?: string | null,
  rememberedOrgID?: string | null,
  organizations?: ReadonlyArray<AuthOrganizationOption> | null,
): string {
  const normalizedSelectedOrgID = normalizeAuthOrgID(selectedOrgID)
  if (normalizedSelectedOrgID && isKnownOrganization(normalizedSelectedOrgID, organizations)) {
    return normalizedSelectedOrgID
  }

  const normalizedRememberedOrgID = normalizeAuthOrgID(rememberedOrgID)
  if (normalizedRememberedOrgID && isKnownOrganization(normalizedRememberedOrgID, organizations)) {
    return normalizedRememberedOrgID
  }

  if (organizations?.length === 1) {
    return normalizeAuthOrgID(organizations[0]?.org_id)
  }

  return ''
}
