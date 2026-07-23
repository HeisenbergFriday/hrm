/** 手机号脱敏：保留前 3 后 4；短号只显示末 2 位 */
export function maskMobile(value?: string | null): string {
  const raw = String(value || '').trim()
  if (!raw) return '-'
  const digits = raw.replace(/\D/g, '')
  if (digits.length >= 7) {
    return `${digits.slice(0, 3)}****${digits.slice(-4)}`
  }
  if (digits.length >= 3) {
    return `**${digits.slice(-2)}`
  }
  return '****'
}

/** 邮箱脱敏：保留首字符与域名 */
export function maskEmail(value?: string | null): string {
  const raw = String(value || '').trim()
  if (!raw) return '-'
  const at = raw.indexOf('@')
  if (at <= 0) return '****'
  const name = raw.slice(0, at)
  const domain = raw.slice(at)
  if (name.length <= 1) return `*${domain}`
  return `${name[0]}***${domain}`
}

/** 身份证等证件号脱敏 */
export function maskIdNumber(value?: string | null): string {
  const raw = String(value || '').trim()
  if (!raw) return '-'
  if (raw.length <= 8) return '****'
  return `${raw.slice(0, 4)}**********${raw.slice(-4)}`
}

/** 银行卡脱敏 */
export function maskBankAccount(value?: string | null): string {
  const raw = String(value || '').replace(/\s/g, '')
  if (!raw) return '-'
  if (raw.length <= 8) return '****'
  return `**** **** **** ${raw.slice(-4)}`
}
