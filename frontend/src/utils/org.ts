const ORG_ID_STORAGE_KEY = 'peopleops-org-id'

function sanitizeOrgId(rawValue: string | null | undefined): string {
  if (!rawValue) {
    return ''
  }
  const value = rawValue.trim()
  // org_id 只允许字母、数字、下划线、连字符，避免注入或异常值
  if (!/^[A-Za-z0-9_-]{1,64}$/.test(value)) {
    return ''
  }
  return value
}

function readOrgIdFromStorage(): string {
  try {
    if (typeof window === 'undefined' || !window.sessionStorage) {
      return ''
    }
    return window.sessionStorage.getItem(ORG_ID_STORAGE_KEY) || ''
  } catch {
    return ''
  }
}

function persistOrgId(orgId: string): void {
  try {
    if (typeof window === 'undefined' || !window.sessionStorage) {
      return
    }
    if (orgId) {
      window.sessionStorage.setItem(ORG_ID_STORAGE_KEY, orgId)
    } else {
      window.sessionStorage.removeItem(ORG_ID_STORAGE_KEY)
    }
  } catch {
    // ignore storage errors
  }
}

// 从当前 URL 的 query 中读取 org_id
function readOrgIdFromLocation(): string {
  try {
    if (typeof window === 'undefined') {
      return ''
    }
    const params = new URLSearchParams(window.location.search)
    return sanitizeOrgId(params.get('org_id'))
  } catch {
    return ''
  }
}

/**
 * 解析当前应该使用的 org_id：
 * 优先取 URL 上的 org_id（用户通过不同企业的微应用首页进入时携带），
 * 取到后写入 sessionStorage，以便钉钉跳转丢失 query 后仍能恢复；
 * URL 上没有时回退到 sessionStorage 中已记住的值。
 */
export function resolveOrgId(): string {
  const fromLocation = readOrgIdFromLocation()
  if (fromLocation) {
    persistOrgId(fromLocation)
    return fromLocation
  }
  return sanitizeOrgId(readOrgIdFromStorage())
}

// 记住 org_id（供跳转前显式持久化）
export function rememberOrgId(orgId: string | null | undefined): void {
  persistOrgId(sanitizeOrgId(orgId))
}

// 清理已记住的 org_id，避免登出后切换企业时复用旧企业
export function clearRememberedOrgId(): void {
  persistOrgId('')
}

// 生成携带 org_id 的请求参数对象，org_id 为空时返回空对象
export function orgIdParams(): Record<string, string> {
  const orgId = resolveOrgId()
  return orgId ? { org_id: orgId } : {}
}
