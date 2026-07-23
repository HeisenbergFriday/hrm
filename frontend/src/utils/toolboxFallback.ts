/**
 * Decide whether a structured toolbox API failure may fall back to the legacy blob API.
 *
 * Only version-mismatch signals (404 / 405 / 501) allow fallback.
 * Auth, validation, rate-limit, business errors, timeouts and network errors must NOT fall back
 * (they would re-run side-effectful calculations or hide real permission failures).
 */
export function shouldFallbackToLegacyToolboxAPI(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false

  const anyErr = error as {
    response?: { status?: number }
    status?: number
    code?: string
    message?: string
    name?: string
  }

  const status =
    typeof anyErr.response?.status === 'number'
      ? anyErr.response.status
      : typeof anyErr.status === 'number'
        ? anyErr.status
        : undefined

  if (status === 404 || status === 405 || status === 501) {
    return true
  }

  // Explicit denials / business failures — never fallback.
  if (
    status === 400 ||
    status === 401 ||
    status === 403 ||
    status === 409 ||
    status === 410 ||
    status === 422 ||
    status === 429 ||
    status === 500 ||
    status === 502 ||
    status === 503 ||
    status === 504
  ) {
    return false
  }

  // Network / timeout / cancel — never re-run via legacy.
  const code = String(anyErr.code || '')
  if (
    code === 'ECONNABORTED' ||
    code === 'ERR_NETWORK' ||
    code === 'ERR_CANCELED' ||
    code === 'ETIMEDOUT'
  ) {
    return false
  }

  const msg = String(anyErr.message || '').toLowerCase()
  if (
    msg.includes('timeout') ||
    msg.includes('network error') ||
    msg.includes('failed to fetch') ||
    msg.includes('aborted')
  ) {
    return false
  }

  return false
}
