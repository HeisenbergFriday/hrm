/** 登录错误码/短词白名单 → 中文文案；未知内容不原样展示（防钓鱼） */
const LOGIN_ERROR_MAP: Record<string, string> = {
  access_denied: '登录被拒绝，请重试或联系管理员',
  invalid_state: '登录状态已失效，请重新发起登录',
  invalid_code: '授权码无效或已过期，请重新登录',
  missing_org: '请选择要登录的企业后再试',
  org_not_found: '企业不存在或已停用',
  org_inactive: '企业已停用，无法登录',
  user_inactive: '账号已停用，请联系管理员',
  user_not_found: '账号不存在，请联系管理员开通',
  token_missing_org_id: '登录凭证缺少组织信息，请重新登录',
  session_expired: '登录已过期，请重新登录',
  unauthorized: '未授权，请重新登录',
  forbidden: '没有权限登录该企业',
  server_error: '服务暂时不可用，请稍后重试',
  dingtalk_failed: '钉钉授权失败，请重试',
  login_failed: '登录失败，请重试',
}

const SAFE_LITERALS = new Set([
  '登录失败，请重试',
  '登录失败',
  '请重新登录',
  '会话已失效',
])

/**
 * 将 URL ?error= 或后端短码映射为安全提示。
 * 未知长文本统一降级，避免钓鱼文案以系统错误样式展示。
 */
export function safeLoginErrorMessage(raw?: string | null, fallback = '登录失败，请重试'): string {
  if (!raw) return fallback
  let decoded = raw
  try {
    decoded = decodeURIComponent(raw)
  } catch {
    decoded = raw
  }
  const trimmed = decoded.trim()
  if (!trimmed) return fallback
  if (SAFE_LITERALS.has(trimmed)) return trimmed

  const key = trimmed.toLowerCase().replace(/\s+/g, '_')
  if (LOGIN_ERROR_MAP[key]) return LOGIN_ERROR_MAP[key]
  if (LOGIN_ERROR_MAP[trimmed]) return LOGIN_ERROR_MAP[trimmed]

  // 短码（无空格、长度有限）可展示；长句/URL/诱导文案降级
  if (/^[a-zA-Z0-9_.:-]{1,64}$/.test(trimmed) && LOGIN_ERROR_MAP[key] === undefined) {
    // 未收录短码仍不直接透出英文技术细节，统一友好文案
    return fallback
  }
  return fallback
}
