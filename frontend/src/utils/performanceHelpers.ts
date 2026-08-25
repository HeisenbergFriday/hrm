import type { Dayjs } from 'dayjs'
import type { PerformanceActivityManagerAssignment, AssessmentManagerCandidate, AssessmentManagerSource } from '../services/api'

// 状态映射
export const STATUS_MAP: Record<string, { label: string; color: string }> = {
  draft: { label: '草稿', color: 'default' },
  target_setting: { label: '目标设定', color: 'cyan' },
  self_evaluation: { label: '自评中', color: 'processing' },
  manager_evaluation: { label: '主管评分', color: 'warning' },
  employee_confirmation: { label: '员工确认', color: 'blue' },
  manager_confirmation: { label: '主管确认', color: 'orange' },
  hr_confirmation: { label: 'HR确认', color: 'purple' },
  locked: { label: '已锁定', color: 'error' },
  result_confirmed: { label: '已确认', color: 'success' },
  archived: { label: '已归档', color: 'default' },
}

// 参与人状态映射
export const PARTICIPANT_STATUS_MAP: Record<string, { label: string; color: string }> = {
  pending: { label: '待目标', color: 'default' },
  target_pending_approval: { label: '目标待审', color: 'cyan' },
  target_rejected: { label: '目标驳回', color: 'red' },
  target_set: { label: '目标已定', color: 'cyan' },
  self_submitted: { label: '已自评', color: 'processing' },
  manager_submitted: { label: '已评分', color: 'warning' },
  employee_confirmed: { label: '已员工确认', color: 'blue' },
  manager_confirmed: { label: '已主管确认', color: 'orange' },
  hr_confirmed: { label: '已HR确认', color: 'purple' },
  locked: { label: '已冻结', color: 'orange' },
  result_confirmed: { label: '已确认', color: 'success' },
  inactive: { label: '已离职', color: 'error' },
  removed_from_scope: { label: '已移除', color: 'error' },
}

// 活动流程
export const ACTIVITY_FLOW = [
  { status: 'target_setting', label: '目标设定' },
  { status: 'self_evaluation', label: '自评' },
  { status: 'manager_evaluation', label: '评分' },
  { status: 'employee_confirmation', label: '员工确认' },
  { status: 'manager_confirmation', label: '主管确认' },
  { status: 'hr_confirmation', label: 'HR确认' },
  { status: 'archived', label: '归档' },
]

export function normalizeIDArray(value?: string[] | string): string[] {
  if (Array.isArray(value)) return value.filter(Boolean)
  if (!value) return []
  return String(value).split(',').map(item => item.trim()).filter(Boolean)
}

export function getListFromResponse(res: any, keys: string[]): any[] {
  const data = res?.data || res
  if (Array.isArray(data)) return data
  for (const key of keys) {
    if (Array.isArray(data?.[key])) return data[key]
  }
  return []
}

export function getDepartmentOption(department: any) {
  const value = String(department.department_id || department.id || '').trim()
  const name = department.name || department.department_name || value
  return value ? { value, label: `${name}（${value}）` } : null
}

export function getUserOption(user: any) {
  // 参与范围 Select value 必须是 User.UserID，禁止 fallback 到 employee_id / 数据库自增 id。
  const value = String(user?.user_id || '').trim()
  if (!value) return null
  const name = user.name || user.user_name || user.employee_name || value
  const departmentName = user.department_name ? ` - ${user.department_name}` : ''
  return { value, label: `${name}（${value}）${departmentName}` }
}

export function getImportedUserOption(user: any) {
  // 导入结果若已转换为 UserID，则只认 user_id；不再 fallback employee_id / id。
  const value = String(user?.user_id || '').trim()
  if (!value) return null
  const employeeID = String(user?.employee_id || '').trim()
  const name = String(user?.name || user?.user_name || user?.employee_name || value).trim()
  const employeeIDText = employeeID && employeeID !== value ? ` / ${employeeID}` : ''
  const departmentName = user?.department_name ? ` - ${user.department_name}` : ''
  return { value, label: `${name}（${value}${employeeIDText}）${departmentName}` }
}

export function mergeSelectOptions<T extends { value: string | number }>(baseOptions: T[], extraOptions: T[]): T[] {
  const merged = [...baseOptions]
  const seen = new Set(baseOptions.map(option => String(option.value)))

  extraOptions.forEach(option => {
    const key = String(option.value)
    if (!key || seen.has(key)) return
    seen.add(key)
    merged.push(option)
  })

  return merged
}

export function normalizeImportedManagerAssignments(
  assignments?: PerformanceActivityManagerAssignment[] | null,
  employeeIDs?: string[],
): PerformanceActivityManagerAssignment[] {
  const allowedEmployeeIDs = employeeIDs ? new Set(normalizeIDArray(employeeIDs)) : null
  const byUserID = new Map<string, PerformanceActivityManagerAssignment>()
  ;(assignments || []).forEach(assignment => {
    const userID = String(assignment.user_id || '').trim()
    const managerUserID = String(assignment.assessment_manager_user_id || '').trim()
    if (!userID || !managerUserID) return
    if (allowedEmployeeIDs && !allowedEmployeeIDs.has(userID)) return
    byUserID.set(userID, {
      ...assignment,
      user_id: userID,
      employee_id: String(assignment.employee_id || '').trim() || undefined,
      assessment_manager_user_id: managerUserID,
      assessment_manager_employee_id: String(assignment.assessment_manager_employee_id || '').trim() || undefined,
      assessment_manager_name: String(assignment.assessment_manager_name || '').trim(),
      assessment_manager_source: assignment.assessment_manager_source || 'IMPORT',
      manager_override_reason: String(assignment.manager_override_reason || '').trim() || undefined,
    })
  })
  return Array.from(byUserID.values())
}

export function getAssessmentCandidateOption(candidate: AssessmentManagerCandidate): any | null {
  const value = String(candidate.user_id || '').trim()
  if (!value) return null
  const name = String(candidate.name || value).trim()
  const employeeNo = String(candidate.employee_no || '').trim()
  const departmentName = String(candidate.department_name || '').trim()
  const sourceLabel = candidate.candidate_source_label || MANAGER_SOURCE_LABELS[candidate.candidate_source] || '候选'
  return {
    value,
    searchText: [name, value, employeeNo, departmentName].filter(Boolean).join(' '),
    label: `${name} ${employeeNo || value} ${departmentName} ${sourceLabel}`.trim(),
  }
}

export function getAssessmentUserOption(user: any): any | null {
  if (String(user?.status || 'active').trim() !== 'active') return null
  const value = String(user?.user_id || user?.employee_id || user?.id || '').trim()
  if (!value) return null
  const name = String(user?.name || user?.user_name || user?.employee_name || value).trim()
  const employeeNo = String(user?.employee_id || user?.employee_no || '').trim()
  const departmentName = String(user?.department_name || '').trim()
  return {
    value,
    searchText: [name, value, employeeNo, departmentName, user?.mobile].filter(Boolean).join(' '),
    label: `${name} ${employeeNo || value} ${departmentName} 手动指定`.trim(),
  }
}

export function formatRangeStart(range?: [Dayjs, Dayjs]) {
  return range?.[0]?.format('YYYY-MM-DD') || ''
}

export function formatRangeEnd(range?: [Dayjs, Dayjs]) {
  return range?.[1]?.format('YYYY-MM-DD') || ''
}

export function formatDateRange(start?: string, end?: string) {
  if (!start && !end) return '-'
  return `${start || '-'} ~ ${end || '-'}`
}

export function getActivityStepIndex(status?: string) {
  if (status === 'locked') return ACTIVITY_FLOW.length - 1
  if (status === 'draft') return 0
  const index = ACTIVITY_FLOW.findIndex(item => item.status === status)
  return index >= 0 ? index : 0
}

export function getStatusMeta(status?: string) {
  return STATUS_MAP[status || ''] || { label: status || '-', color: 'default' }
}

export function getParticipantStatusMeta(status?: string) {
  return PARTICIPANT_STATUS_MAP[status || ''] || { label: status || '-', color: 'default' }
}

export const ACTIVITY_STATUS_FILTER_IN_PROGRESS = '__in_progress__'
export const ACTIVITY_STATUS_FILTER_CONFIRMED = '__confirmed__'
export const IN_PROGRESS_ACTIVITY_STATUSES = [
  'target_setting',
  'self_evaluation',
  'manager_evaluation',
  'employee_confirmation',
  'manager_confirmation',
  'hr_confirmation',
]
export const CONFIRMED_ACTIVITY_STATUSES = ['locked', 'result_confirmed']
export const ACTIVITY_STATUS_FILTER_GROUPS: Record<string, string[]> = {
  [ACTIVITY_STATUS_FILTER_IN_PROGRESS]: IN_PROGRESS_ACTIVITY_STATUSES,
  [ACTIVITY_STATUS_FILTER_CONFIRMED]: CONFIRMED_ACTIVITY_STATUSES,
}

export function resolveActivityStatusFilter(statusFilter?: string) {
  if (!statusFilter) return undefined
  return ACTIVITY_STATUS_FILTER_GROUPS[statusFilter] || [statusFilter]
}

export const MANAGER_SOURCE_LABELS: Record<AssessmentManagerSource, string> = {
  DIRECT_MANAGER: '直属主管',
  DEPARTMENT_HEAD: '部门负责人',
  CENTER_HEAD: '中心负责人',
  MANUAL: '手动指定',
  IMPORT: '导入指定',
  EMPTY: '暂未配置',
  SYSTEM: '系统兼容',
}

export const MANAGER_CONFIG_STATUS_LABELS: Record<string, { label: string; color: string }> = {
  CONFIGURED: { label: '已配置', color: 'green' },
  PENDING: { label: '待配置考核上级', color: 'orange' },
  INVALID: { label: '考核上级不可用', color: 'red' },
}

export const PERFORMANCE_PERMISSION_LABELS: Record<string, string> = {
  'performance:activity:manage': '绩效活动管理',
  'performance:distribution:manage': '绩效分布规则',
  'performance:goal:manage': '绩效目标管理',
  'performance:self_eval:submit': '绩效自评提交',
  'performance:manager_eval:submit': '绩效主管评分',
  'performance:result:view': '绩效结果查看',
  'performance:hr_confirm:submit': '绩效HR确认',
  'performance:department_eval:submit': '绩效部门/中心评估',
  'performance:hr_review:submit': '绩效HR审核',
  'performance:result_publish:manage': '绩效结果公布',
  'performance:appeal:manage': '绩效申诉处理',
  'performance:assessment_manager:update': '考核上级调整',
  'performance:assessment_manager:batch_update': '批量考核上级调整',
}
