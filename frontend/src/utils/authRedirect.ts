const storageKey = 'peopleops-auth-redirect'

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
