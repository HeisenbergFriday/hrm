export const csrfCookieName = 'peopleops_csrf'
export const csrfHeaderName = 'X-CSRF-Token'

const unsafeMethods = new Set(['post', 'put', 'patch', 'delete'])

export function readCookie(name: string): string {
  if (typeof document === 'undefined') return ''

  const prefix = `${encodeURIComponent(name)}=`
  return document.cookie
    .split(';')
    .map(part => part.trim())
    .find(part => part.startsWith(prefix))
    ?.slice(prefix.length) || ''
}

export function csrfHeadersForMethod(method?: string): Record<string, string> {
  if (!unsafeMethods.has(String(method || 'get').toLowerCase())) {
    return {}
  }

  const token = readCookie(csrfCookieName)
  if (!token) return {}
  return { [csrfHeaderName]: token }
}
