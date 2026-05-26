import dayjs from 'dayjs'

/** 周期类型中文映射 */
export const CYCLE_TYPE_MAP: Record<string, string> = {
  monthly: '月度',
  quarterly: '季度',
  annual: '年度',
  weekly: '周度',
}

/** 获取周期中文标签 */
export const getCycleLabel = (cycle?: string): string =>
  cycle ? (CYCLE_TYPE_MAP[cycle] || cycle) : '-'

/** 格式化时间为 "YYYY年M月D日 HH:mm:ss" */
export const formatDateTime = (value?: string): string => {
  if (!value) return '-'
  const d = dayjs(value)
  return d.isValid() && !value.startsWith('0001-01-01')
    ? d.format('YYYY年M月D日 HH:mm:ss')
    : '-'
}
