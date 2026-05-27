import axios from 'axios'
import { useAuthStore } from '../store/authStore'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  },
})

api.interceptors.request.use(
  (config) => {
    const token = useAuthStore.getState().token
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error),
)

// 刷新菜单权限（通过 api 实例自动带 token，并用锁避免重复刷新）
let isRefreshingMenuKeys = false
export function refreshMenuKeys() {
  if (isRefreshingMenuKeys) return
  isRefreshingMenuKeys = true

  api.get('/auth/me')
    .then((res: any) => {
      const keys = res?.data?.user?.menu_keys
      if (Array.isArray(keys)) useAuthStore.getState().setMenuKeys(keys)
    })
    .catch(() => {})
    .finally(() => {
      isRefreshingMenuKeys = false
    })
}

api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      useAuthStore.getState().logout()
      window.location.href = '/login'
    }
    if (error.response?.status === 403) {
      refreshMenuKeys()
    }
    return Promise.reject(error)
  },
)

export const authAPI = {
  login: (data: { username: string; password: string }) => api.post('/auth/login', data),
  dingtalkLogin: (data: { code: string }) => api.post('/auth/dingtalk/in-app', data),
  logout: () => api.post('/auth/logout'),
  getCurrentUser: () => api.get('/auth/me'),
}

export const userAPI = {
  getUsers: (params: { page: number; page_size: number }) => api.get('/users', { params }),
  getUser: (id: string) => api.get(`/users/${id}`),
  updateUser: (id: string, data: { extension: any }) => api.put(`/users/${id}`, data),
}

export const departmentAPI = {
  getDepartments: () => api.get('/departments'),
  getDepartment: (id: string) => api.get(`/departments/${id}`),
}

export const syncAPI = {
  syncDepartments: () => api.post('/sync/departments'),
  syncUsers: () => api.post('/sync/users'),
  getSyncStatus: () => api.get('/sync/status'),
}

export const orgAPI = {
  getOverview: (params?: { department_id?: string }) => api.get('/org/overview', { params }),
  getDepartmentTree: () => api.get('/org/departments/tree'),
  getDepartmentHistory: (id: string, params?: { limit?: number }) => api.get(`/org/departments/${id}/history`, { params }),
  getEmployees: (params: { page?: number; page_size?: number; department_id?: string; search?: string; status?: string }) =>
    api.get('/org/employees', { params }),
  getEmployee: (id: string) => api.get(`/org/employees/${id}`),
  syncOrg: () => api.post('/org/sync'),
}

export const attendanceAPI = {
  getRecords: (params: {
    page?: number
    page_size?: number
    user_id?: string
    department_id?: string
    start_date?: string
    end_date?: string
  }) => api.get('/attendance/records', { params }),

  getStats: (params: {
    start_date?: string
    end_date?: string
    department_id?: string
  }) => api.get('/attendance/stats', { params }),

  sync: (data?: { start_date?: string; end_date?: string }) => api.post('/attendance/sync', data),

  export: (data: {
    start_date: string
    end_date: string
    user_id?: string
    department_id?: string
  }) => api.post('/attendance/export', data),

  getExports: (params: { page?: number; page_size?: number }) => api.get('/attendance/exports', { params }),
  getLastSyncTime: () => api.get('/attendance/last-sync'),
}

export const approvalAPI = {
  getTemplates: () => api.get('/approvals/templates'),
  getInstances: (params: {
    page?: number
    page_size?: number
    status?: string
    template_id?: string
    applicant_id?: string
    start_date?: string
    end_date?: string
  }) => api.get('/approvals/instances', { params }),
  getApproval: (id: string) => api.get(`/approvals/${id}`),
  sync: (data: { process_code: string; start_date?: string; end_date?: string }) => api.post('/approvals/sync', data),
}

export const permissionAPI = {
  getRoles: () => api.get('/permission/roles'),
  createRole: (data: { name: string; description: string }) => api.post('/permission/roles', data),
  updateRole: (id: number, data: { name: string; description: string }) => api.put(`/permission/roles/${id}`, data),
  getPermissions: () => api.get('/permission/permissions'),
  getUserRoles: (userId: string) => api.get(`/permission/users/${userId}/roles`),
  assignUserRole: (data: { user_id: string; role_id: number }) => api.post('/permission/users/roles/assign', data),
  removeUserRole: (data: { user_id: string; role_id: number }) => api.post('/permission/users/roles/remove', data),
  getUserPermissions: (userId: string) => api.get(`/permission/users/${userId}/permissions`),
  getRoleUsers: (roleId: number) => api.get(`/permission/roles/${roleId}/users`),
  getMenuPermission: (roleId: number) => api.get(`/permission/roles/${roleId}/menu`),
  saveMenuPermission: (roleId: number, menuKeys: string[]) => api.post(`/permission/roles/${roleId}/menu`, { menu_keys: JSON.stringify(menuKeys) }),
  getDataPermission: (roleId: number) => api.get(`/permission/roles/${roleId}/data`),
  saveDataPermission: (roleId: number, scope: string, departmentKeys: string[]) => api.post(`/permission/roles/${roleId}/data`, { scope, department_keys: JSON.stringify(departmentKeys) }),
}

export const auditAPI = {
  getLogs: (params: {
    page?: number
    page_size?: number
    start_date?: string
    end_date?: string
    user_id?: string
    operation?: string
    resource?: string
  }) => api.get('/audit/logs', { params }),
}

export const jobAPI = {
  getJobs: () => api.get('/jobs'),
  runJob: (id: string) => api.post(`/jobs/${id}/run`),
}

export const employeeAPI = {
  getProfiles: (params?: { page?: number; page_size?: number; department_id?: string; status?: string }) =>
    api.get('/employee/profiles', { params }),
  getProfile: (id: string) => api.get(`/employee/profiles/${id}`),
  createProfile: (data: any) => api.post('/employee/profiles', data),
  updateProfile: (id: string, data: any) => api.put(`/employee/profiles/${id}`, data),
  getLifecycleLedger: (params?: { page?: number; page_size?: number; department_id?: string; status?: string; keyword?: string }) =>
    api.get('/employee/ledger', { params }),

  getTransfers: (params?: { page?: number; page_size?: number; status?: string }) =>
    api.get('/employee/transfers', { params }),
  createTransfer: (data: any) => api.post('/employee/transfers', data),

  getResignations: (params?: { page?: number; page_size?: number; status?: string }) =>
    api.get('/employee/resignations', { params }),
  createResignation: (data: any) => api.post('/employee/resignations', data),

  getOnboardings: (params?: { page?: number; page_size?: number; status?: string }) =>
    api.get('/employee/onboardings', { params }),
  createOnboarding: (data: any) => api.post('/employee/onboardings', data),
}

export const talentAPI = {
  getAnalysis: (params?: { page?: number; page_size?: number; department_id?: string }) =>
    api.get('/talent/analysis', { params }),
  getAnalysisDetail: (id: string) => api.get(`/talent/analysis/${id}`),
  createAnalysis: (data: any) => api.post('/talent/analysis', data),
}

export const weekScheduleAPI = {
  getRules: () => api.get('/week-schedule/rules'),
  createRule: (data: Record<string, unknown>) => api.post('/week-schedule/rules', data),
  updateRule: (id: number | string, data: Record<string, unknown>) => api.put(`/week-schedule/rules/${id}`, data),
  deleteRule: (id: number | string) => api.delete(`/week-schedule/rules/${id}`),
  batchSetRules: (data: { user_ids: string[]; base_date: string; pattern: string; shift_id?: number; conflict_mode: string; dry_run: boolean }) =>
    api.post('/week-schedule/rules/batch', data),

  getShifts: () => api.get('/week-schedule/shifts'),
  createShift: (data: { name: string; check_in_time: string; check_out_time: string }) =>
    api.post('/week-schedule/shifts', data),

  getCalendar: (params: { weeks?: number; user_id?: string; department_id?: string; start_date?: string }) =>
    api.get('/week-schedule/calendar', { params }),

  setOverride: (data: Record<string, unknown>) => api.post('/week-schedule/overrides', data),
  deleteOverride: (id: number | string) => api.delete(`/week-schedule/overrides/${id}`),

  getHolidays: (params: { year: number }) => api.get('/week-schedule/holidays', { params }),
  createHoliday: (data: Record<string, unknown>) => api.post('/week-schedule/holidays', data),
  batchCreateHolidays: (data: { holidays: Array<{ date: string; name: string; type: string }> }) =>
    api.post('/week-schedule/holidays/batch', data),
  deleteHoliday: (id: number | string) => api.delete(`/week-schedule/holidays/${id}`),

  syncToDingtalk: (data: { weeks: number }) => api.post('/week-schedule/sync/to-dingtalk', data),
  syncFromDingtalk: () => api.post('/week-schedule/sync/from-dingtalk'),
  syncHolidaysFromJuhe: () => api.post('/week-schedule/holidays/sync/from-juhe'),
  getSyncLogs: (params: { page?: number; page_size?: number }) => api.get('/week-schedule/sync/logs', { params }),
}

export const shiftConfigAPI = {
  list: () => api.get('/shift-config/list'),
  catalogs: () => api.get('/shift-config/catalogs'),
  preview: (data: {
    user_ids: string[]
    shift_id?: number
    end_time?: string
    name?: string
    check_in?: string
    check_out?: string
    start_date: string
    end_date: string
  }) => api.post('/shift-config/preview', data),
  set: (data: { user_ids: string[]; shift_id: number; end_time: string; note?: string }) =>
    api.post('/shift-config/set', data),
  apply: (data: {
    user_ids: string[]
    shift_id?: number
    end_time?: string
    note?: string
    name?: string
    check_in?: string
    check_out?: string
    start_date: string
    end_date: string
  }) => api.post('/shift-config/apply', data),
  remove: (userId: string) => api.delete(`/shift-config/${userId}`),
  getOrCreateShift: (data: { name: string; check_in: string; check_out: string }) =>
    api.post('/shift-config/get-or-create-shift', data),
}

export const leaveAPI = {
  getEligibility: (params: { user_id: string; year: number }) =>
    api.get('/leave/eligibility', { params }),
  recalculateEligibility: (data: { user_id: string; year: number }) =>
    api.post('/leave/eligibility/recalculate', data),
  getGrants: (params: { user_id: string; year: number }) =>
    api.get('/leave/grants', { params }),
  runQuarterGrant: (data: { year: number; quarter: number }) =>
    api.post('/leave/grants/run-quarter', data),
  regrant: (data: { user_id: string; year: number }) =>
    api.post('/leave/grants/regrant', data),
  syncToDingTalk: () => api.post('/leave/grants/sync-to-dingtalk', { confirm: true }),
  consume: (data: { user_id: string; days: number; approval_ref?: string; remark?: string }) =>
    api.post('/leave/consume', data),
  getConsumeLog: (params: { user_id: string }) =>
    api.get('/leave/consume-log', { params }),
}

export const overtimeAPI = {
  getMatches: (params: { user_id: string; start_date: string; end_date: string }) =>
    api.get('/overtime/matches', { params }),
  runMatch: (data: { user_id?: string; start_date: string; end_date: string }) =>
    api.post('/overtime/matches/run', data),
  syncAndMatch: (data: { start_date: string; end_date: string }) =>
    api.post('/overtime/sync-and-match', data),
  clearAndRematch: (data: { user_id?: string; start_date: string; end_date: string }) =>
    api.post('/overtime/matches/clear-rematch', data),
  deleteMatches: (data: { user_id?: string; start_date: string; end_date: string }) =>
    api.post('/overtime/matches/delete', data),
  getCompBalance: (params: { user_id: string }) =>
    api.get('/comp-time/balance', { params }),
  resetManualLeave: (data: { dry_run: boolean }) =>
    api.post('/overtime/reset-manual-leave', data, { timeout: 300_000 }),
  resyncOvertimeToDingTalk: (data: { dry_run: boolean; user_id?: string; start_date?: string; end_date?: string }) =>
    api.post('/overtime/resync-overtime', data, { timeout: 300_000 }),
  submitSupplementary: (data: { match_result_id: number; clock_in: string; clock_out: string; reason?: string }) =>
    api.post('/overtime/supplementary/submit', data),
  approveSupplementary: (data: { request_id: number; approved: boolean; rejected_reason?: string }) =>
    api.post('/overtime/supplementary/approve', data),
  getSupplementaryList: (params: { user_id?: string; start_date?: string; end_date?: string }) =>
    api.get('/overtime/supplementary/list', { params }),
}

// ============= 绩效模块 API =============
// 注意：后端已提供模板 CRUD；以下接口直接对接后端绩效模板与指标库能力

export type PerformanceActivityStatus = 'draft' | 'target_setting' | 'self_evaluation' | 'manager_evaluation' | 'employee_confirmation' | 'manager_confirmation' | 'hr_confirmation' | 'locked' | 'result_confirmed' | 'archived'

// 绩效活动
export interface PerformanceActivity {
  id: number
  name: string
  cycle_type: string
  start_date: string
  end_date: string
  indicator_library_id?: number
  target_set_start_at?: string
  target_set_end_at?: string
  self_eval_start_at: string
  self_eval_end_at: string
  manager_eval_start_at: string
  manager_eval_end_at: string
  result_confirm_start_at: string
  result_confirm_end_at: string
  employee_confirm_start_at?: string
  employee_confirm_end_at?: string
  manager_confirm_start_at?: string
  manager_confirm_end_at?: string
  hr_confirm_start_at?: string
  hr_confirm_end_at?: string
  hr_confirm_deadline?: string
  status: PerformanceActivityStatus
  description?: string
  target_department_ids?: string[]
  target_employee_ids?: string[]
  enable_bonus_score?: boolean
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

// 绩效参与人状态
export type PerformanceParticipantStatus = 'pending' | 'target_pending_approval' | 'target_rejected' | 'target_set' | 'self_submitted' | 'manager_submitted' | 'result_confirmed' | 'inactive' | 'removed_from_scope' | 'employee_confirmed' | 'manager_confirmed' | 'hr_confirmed' | 'locked'

// 绩效参与人
export interface PerformanceParticipant {
  id: number
  activity_id: number
  employee_id: string
  employee_name: string
  department_id: string
  department_name: string
  position: string
  level: string
  employee_status: string
  manager_id?: string
  manager_name?: string
  status: PerformanceParticipantStatus
  self_score: number
  self_level: string
  self_summary: string
  manager_score: number
  manager_comment: string
  suggested_level: string
  final_level: string
  adjust_reason: string
  // 评价文本
  self_evaluation_comment?: string
  manager_evaluation_comment?: string
  // 拆分评价字段
  self_evaluation_good?: string
  self_evaluation_improvement?: string
  manager_evaluation_good?: string
  manager_evaluation_improvement?: string
  // 系统计算总分
  total_self_score?: number
  total_manager_score?: number
  // 附加项
  bonus_score?: number
  penalty_score?: number
  adjusted_score?: number
  // 收支系数
  revenue_coefficient?: number
  // 三级确认
  employee_confirmed_at?: string
  employee_confirmed_by?: string
  manager_confirmed_at?: string
  manager_confirmed_by?: string
  hr_confirmed_at?: string
  hr_confirmed_by?: string
  employee_target_confirmed_at?: string
  employee_target_confirmed_by?: string
  manager_target_confirmed_at?: string
  manager_target_confirmed_by?: string
  hr_target_confirmed_at?: string
  hr_target_confirmed_by?: string
  // 锁定
  is_locked?: boolean
  locked_at?: string
  locked_by?: string
  force_locked?: boolean
  force_locked_reason?: string
  // 兼容旧接口
  confirmed_at?: string
  confirmed_by: string
  created_at: string
  updated_at: string
  updated_by?: string
}

// 绩效活动列表响应
export interface PerformanceActivityListResponse {
  items: PerformanceActivity[]
  total: number
}

// 绩效参与人列表响应
export interface PerformanceParticipantListResponse {
  items: PerformanceParticipant[]
  total: number
}

// 强制分布规则
export interface PerformanceDistributionRule {
  id: number
  activity_id: string
  level: string
  distribution_percent: number
  description: string
}

// 绩效统计摘要
export interface PerformanceResultSummary {
  total_participants: number
  self_submitted_count: number
  manager_submitted_count: number
  result_confirmed_count: number
  level_distribution: Record<string, number>
}

// 强制分布检查结果
export interface PerformanceDistributionCheck {
  passed: boolean
  total_count: number
  exceeded_levels: { level: string; expected: number; actual: number; excess: number }[]
  distribution: Record<string, {
    expected_count: number
    actual_count: number
    expected_percent: number
    actual_percent: number
    progress: number
    status: string
  }>
  warnings: string[]
}

// 绩效指标库
export interface PerformanceIndicatorLibrary {
  id: number
  department_id: string
  department_name: string
  parent_library_id?: number
  name: string
  description: string
  default_cycle: string
  status: string
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
}

// 绩效指标项
export interface PerformanceIndicatorItem {
  id: number
  library_id: number
  parent_indicator_id?: number
  section_type?: string
  name: string
  description: string
  indicator_type: string
  keywords?: string[]
  cycle: string
  default_weight: number
  red_line_value: string
  target_value: string
  challenge_value: string
  scoring_rule?: string
  weight?: number
  is_default?: boolean
  is_inherited: boolean
  is_customized: boolean
  sort_order: number
  created_at: string
  updated_at: string
}

// 绩效目标记录
export interface PerformanceGoalRecord {
  id: number
  activity_id: string
  participant_id: number
  indicator_item_id?: number
  section_type: 'quantitative' | 'key_action' | 'bonus_penalty'
  item_name: string
  item_definition: string
  weight: number
  red_line_value: string
  target_value: string
  challenge_value: string
  scoring_rule: string
  actual_result: string
  attachments: string[]
  self_score: number
  manager_score: number
  bonus_score: number
  is_from_superior: boolean
  approval_status: string
  visibility_scope: string
  sort_order: number
  created_at: string
  updated_at: string
}

// 团队配额状态
export interface TeamQuotaStatus {
  manager_id: string
  manager_name: string
  total: number
  levels: Record<string, {
    current: number
    max: number
    percent: number
  }>
}

// 刷新参与人结果
export interface RefreshParticipantsResult {
  added_count: number
  updated_count: number
  inactive_count: number
}

// 自评提交请求
export interface SubmitSelfEvaluationRequest {
  self_score: number
  self_level: string
  self_summary: string
  self_attachments?: string[]
}

export interface SubmitReviewSelfEvaluationRequest {
  self_content_json: {
    content: string
  }
}

// 主管评分项
export interface EvaluationItem {
  item_key: string
  item_score: number
  item_value: string
}

// 主管评分提交请求
export interface SubmitManagerEvaluationRequest {
  manager_score: number
  suggested_level: string
  manager_comment: string
  evaluation_items?: EvaluationItem[]
}

export interface SubmitReviewManagerEvaluationRequest {
  manager_score_json?: Record<string, number>
  manager_comment: string
  final_level: string
  final_level_reason?: string
  bonus_score?: number
}

// 批量主管评分
export interface BatchManagerEvaluationItem {
  participant_id: number
  manager_score: number
  suggested_level: string
  manager_comment: string
  evaluation_items?: EvaluationItem[]
}

// 绩效版本记录
export interface PerformanceReviewVersion {
  id: number
  participant_id: number
  activity_id: string
  review_type: 'self' | 'manager' | 'adjust' | 'confirm'
  created_by: string
  self_score: number
  self_level: string
  self_summary: string
  self_attachments: string[]
  manager_score: number
  suggested_level: string
  manager_comment: string
  evaluation_items: EvaluationItem[]
  final_level: string
  adjust_reason: string
  confirm_comment: string
  confirmed_at: string
  created_at: string
  updated_at: string
}

// 创建绩效活动请求
export interface CreatePerformanceActivityRequest {
  name: string
  cycle_type: string
  start_date: string
  end_date: string
  target_set_start_at?: string
  target_set_end_at?: string
  self_eval_start_at: string
  self_eval_end_at: string
  manager_eval_start_at: string
  manager_eval_end_at: string
  result_confirm_start_at: string
  result_confirm_end_at: string
  employee_confirm_start_at?: string
  employee_confirm_end_at?: string
  manager_confirm_start_at?: string
  manager_confirm_end_at?: string
  hr_confirm_start_at?: string
  hr_confirm_end_at?: string
  hr_confirm_deadline?: string
  status: PerformanceActivityStatus
  target_department_ids?: string[]
  target_employee_ids?: string[]
  indicator_library_id?: number
  description?: string
  enable_bonus_score?: boolean
}

// 关系变更日志
export interface RelationshipChangeLog {
  id: number
  activity_id: string
  participant_id: number
  change_type: string
  field_name: string
  old_value: string
  new_value: string
  changed_at: string
  source: string
  created_by: string
}

export interface PerformanceCompanyFinance {
  id: number
  activity_id: string
  revenue_sign: 'revenue_gt_expense' | 'expense_gt_revenue' | 'equal' | string
  description?: string
  remark?: string
  set_by?: string
  set_at?: string
  created_at?: string
  updated_at?: string
}

export interface PerformanceHRDeadlineStatus {
  deadline?: string
  pending_count: number
  overdue: boolean
  can_force_lock?: boolean
}

export interface PerformanceHRForceLockResult {
  force_locked_count: number
  locked_count: number
  already_locked_count: number
  total_count: number
}

export interface PerformanceTemplatePayload {
  name: string
  description?: string
  status?: string
  sections?: {
    name: string
    section_type: string
    weight: number
    sort_order?: number
    is_score_required?: boolean
    is_comment_required?: boolean
    items: {
      name: string
      description?: string
      max_score: number
      weight: number
      sort_order?: number
    }[]
  }[]
}

export const performanceAPI = {
  // ===== 绩效活动 =====
  getActivities: (params?: {
    page?: number
    page_size?: number
    status?: string
    keyword?: string
    start_date?: string
    end_date?: string
  }) => api.get('/performance/activities', { params }),

  createActivity: (data: {
    name: string
    cycle_type: string
    start_date: string
    end_date: string
    target_set_start_at?: string
    target_set_end_at?: string
    self_eval_start_at: string
    self_eval_end_at: string
    manager_eval_start_at: string
    manager_eval_end_at: string
    result_confirm_start_at: string
    result_confirm_end_at: string
    employee_confirm_start_at?: string
    employee_confirm_end_at?: string
    manager_confirm_start_at?: string
    manager_confirm_end_at?: string
    hr_confirm_start_at?: string
    hr_confirm_end_at?: string
    hr_confirm_deadline?: string
    status: string
    target_department_ids?: string[]
    target_employee_ids?: string[]
    indicator_library_id?: number
    description?: string
    enable_bonus_score?: boolean
  }) => api.post('/performance/activities', data),

  getActivity: (activityId: number) =>
    api.get(`/performance/activities/${activityId}`),

  updateActivity: (activityId: number, data: {
    name: string
    cycle_type: string
    start_date: string
    end_date: string
    target_set_start_at?: string
    target_set_end_at?: string
    self_eval_start_at: string
    self_eval_end_at: string
    manager_eval_start_at: string
    manager_eval_end_at: string
    result_confirm_start_at: string
    result_confirm_end_at: string
    employee_confirm_start_at?: string
    employee_confirm_end_at?: string
    manager_confirm_start_at?: string
    manager_confirm_end_at?: string
    hr_confirm_start_at?: string
    hr_confirm_end_at?: string
    hr_confirm_deadline?: string
    status: string
    target_department_ids?: string[]
    target_employee_ids?: string[]
    indicator_library_id?: number
    description?: string
    enable_bonus_score?: boolean
  }) => api.put(`/performance/activities/${activityId}`, data),

  // 活动状态流转
  startActivity: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/start`),

  openSelfEvaluation: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-self-evaluation`),

  openManagerEvaluation: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-manager-evaluation`),

  confirmResults: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/confirm-results`),

  archiveActivity: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/archive`),

  // 新增状态流转（9状态流）
  openTargetSetting: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-target-setting`),

  openEmployeeConfirmation: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-employee-confirmation`),

  openManagerConfirmation: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-manager-confirmation`),

  openHRConfirmation: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-hr-confirmation`),

  lockActivity: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/lock`),

  forceLockOverdueHR: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/force-lock-overdue-hr`),

  // 兼容旧接口
  publishActivity: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/publish`),
  closeActivity: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/close`),

  // ===== 绩效参与人 =====
  getParticipants: (activityId: number, params?: {
    page?: number
    page_size?: number
    department_id?: string
    manager_id?: string
    status?: string
    employee_keyword?: string
  }) => api.get(`/performance/activities/${activityId}/participants`, { params }),

  refreshParticipants: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/refresh-participants`),

  getParticipant: (participantId: number) =>
    api.get(`/performance/participants/${participantId}`),

  // ===== 自评 =====
  submitSelfEvaluation: (participantId: number, data: {
    self_score: number
    self_level: string
    self_summary: string
    self_attachments?: string[]
  }) => api.post(`/performance/participants/${participantId}/self-evaluation`, data),

  submitReviewSelfEvaluation: (participantId: number, data: SubmitReviewSelfEvaluationRequest) =>
    api.post(`/performance/reviews/${participantId}/self-evaluation`, data),

  // ===== 主管评分 =====
  submitManagerEvaluation: (participantId: number, data: {
    manager_score: number
    suggested_level: string
    manager_comment: string
    evaluation_items?: { item_key: string; item_score: number; item_value: string }[]
  }) => api.post(`/performance/participants/${participantId}/manager-evaluation`, data),

  submitReviewManagerEvaluation: (participantId: number, data: SubmitReviewManagerEvaluationRequest) =>
    api.post(`/performance/reviews/${participantId}/manager-evaluation`, data),

  batchSubmitManagerEvaluations: (activityId: number, evaluations: {
    participant_id: number
    manager_score: number
    suggested_level: string
    manager_comment: string
    evaluation_items?: { item_key: string; item_score: number; item_value: string }[]
  }[]) => api.post(`/performance/activities/${activityId}/batch-manager-evaluations`, { evaluations }),

  // ===== 批量确认结果 =====
  batchConfirmResults: (activityId: number, participantIds: number[]) =>
    api.post(`/performance/activities/${activityId}/batch-confirm-results`, { participant_ids: participantIds }),

  // ===== 钉钉待办/提醒 =====
  sendSelfEvalReminder: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/send-self-eval-reminder`),

  sendManagerEvalReminder: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/send-manager-eval-reminder`),

  sendHRConfirmReminder: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/send-hr-confirm-reminder`),

  // ===== HR 收支与确认管理 =====
  setCompanyFinance: (activityId: number, data: {
    revenue_sign: 'revenue_gt_expense' | 'expense_gt_revenue' | 'equal' | string
    description?: string
    remark?: string
  }) => api.put(`/performance/activities/${activityId}/finance`, data),

  getCompanyFinance: (activityId: number) =>
    api.get(`/performance/activities/${activityId}/finance`),

  getPendingHRConfirm: (activityId: number) =>
    api.get(`/performance/activities/${activityId}/pending-hr-confirm`),

  setHRConfirmDeadline: (activityId: number, deadline: string) =>
    api.put(`/performance/activities/${activityId}/hr-confirm-deadline`, { deadline }),

  getHRConfirmDeadlineStatus: (activityId: number) =>
    api.get(`/performance/activities/${activityId}/hr-confirm-deadline-status`),

  // ===== 绩效面谈 =====
  triggerPerformanceInterview: (participantId: number, interviewType: 'required' | 'optional') =>
    api.post(`/performance/participants/${participantId}/trigger-interview`, { interview_type: interviewType }),

  // ===== 调整最终等级 =====
  adjustFinalLevel: (participantId: number, finalLevel: string, reason: string) =>
    api.post(`/performance/participants/${participantId}/adjust-final-level`, { final_level: finalLevel, reason }),

  // ===== 确认结果 =====
  confirmResult: (participantId: number, confirmComment?: string) =>
    api.post(`/performance/participants/${participantId}/confirm-result`, { confirm_comment: confirmComment }),

  // ===== 版本记录 =====
  getParticipantVersions: (participantId: number) =>
    api.get(`/performance/participants/${participantId}/versions`),

  // ===== 目标记录 =====
  getGoalRecords: (participantId: number) =>
    api.get(`/performance/goal-records/${participantId}`),

  // ===== 新版评分（基于目标指标） =====
  submitGoalSelfEvaluation: (participantId: number, data: {
    items: { record_id: number; actual_result: string; self_score: number }[]
    bonus_items?: { record_id: number; self_score: number }[]
    evaluation_good: string
    evaluation_improvement: string
  }) => api.post(`/performance/goal-reviews/${participantId}/self-evaluation`, data),

  submitGoalManagerEvaluation: (participantId: number, data: {
    items: { record_id: number; manager_score: number }[]
    bonus_items?: { record_id: number; manager_score: number }[]
    suggested_level: string
    evaluation_good: string
    evaluation_improvement: string
  }) => api.post(`/performance/goal-reviews/${participantId}/manager-evaluation`, data),

  // ===== 自动评分 =====
  autoScoreGoalRecords: (items: {
    record_id: number
    section_type: string
    weight: number
    red_line_value: string
    target_value: string
    challenge_value: string
    scoring_rule: string
    actual_result: string
  }[]) => api.post('/performance/auto-score', { items }),

  // ===== 实时分布检查 =====
  getRealtimeDistributionCheck: (activityId: number) =>
    api.get(`/performance/activities/${activityId}/realtime-distribution-check`),

  // ===== 附加项设置 =====
  setBonusPenaltyScore: (participantId: number, bonusScore: number, penaltyScore: number) =>
    api.post(`/performance/participants/${participantId}/bonus-penalty`, { bonus_score: bonusScore, penalty_score: penaltyScore }),

  // ===== 三级确认 =====
  confirmEmployeeResult: (participantId: number) =>
    api.post(`/performance/participants/${participantId}/confirm-employee`),

  confirmManagerResult: (participantId: number) =>
    api.post(`/performance/participants/${participantId}/confirm-manager`),

  confirmHRResult: (participantId: number) =>
    api.post(`/performance/participants/${participantId}/confirm-hr`),

  // ===== 关系变更日志 =====
  getParticipantRelationshipChangeLogs: (participantId: number) =>
    api.get(`/performance/participants/${participantId}/relationship-change-logs`),

  getActivityRelationshipChangeLogs: (activityId: number) =>
    api.get(`/performance/activities/${activityId}/relationship-change-logs`),

  // ===== 强制分布规则 =====
  getDistributionRules: (activityId: number) =>
    api.get(`/performance/activities/${activityId}/distribution-rules`),

  putDistributionRules: (activityId: number, rules: { level: string; distribution_percent: number; description: string }[]) =>
    api.put(`/performance/activities/${activityId}/distribution-rules`, { rules }),

  // ===== 统计和强制分布 =====
  getResultSummary: (activityId: number) =>
    api.get(`/performance/activities/${activityId}/result-summary`),

  getDistributionCheck: (activityId: number) =>
    api.get(`/performance/activities/${activityId}/distribution-check`),

  // ===== 模板管理（兼容旧接口） =====
  getTemplates: (params?: { page?: number; page_size?: number; status?: string }) =>
    api.get('/performance/templates', { params }),

  createTemplate: (data: PerformanceTemplatePayload) =>
    api.post('/performance/templates', data),

  getTemplate: (templateId: number) =>
    api.get(`/performance/templates/${templateId}`),

  updateTemplate: (templateId: number, data: PerformanceTemplatePayload) =>
    api.put(`/performance/templates/${templateId}`, data),

  // ===== 指标库管理 =====
  getIndicatorLibraries: (params?: {
    page?: number
    page_size?: number
    department_id?: string
    keyword?: string
    status?: string
  }) => api.get('/performance/indicator-libraries', { params }),

  createIndicatorLibrary: (data: {
    department_id: string
    department_name: string
    name: string
    description?: string
    default_cycle?: string
    items?: {
      section_type: string
      name: string
      description?: string
      weight?: number
      red_line_value?: string
      target_value?: string
      challenge_value?: string
      scoring_rule?: string
      is_default?: boolean
      sort_order?: number
    }[]
  }) => api.post('/performance/indicator-libraries', data),

  getIndicatorLibrary: (libraryId: number) =>
    api.get(`/performance/indicator-libraries/${libraryId}`),

  updateIndicatorLibrary: (libraryId: number, data: {
    name?: string
    description?: string
    department_name?: string
    default_cycle?: string
  }) => api.put(`/performance/indicator-libraries/${libraryId}`, data),

  archiveIndicatorLibrary: (libraryId: number) =>
    api.post(`/performance/indicator-libraries/${libraryId}/archive`),

  getIndicatorLibrariesByDepartment: (departmentId: string) =>
    api.get(`/performance/indicator-libraries/department/${departmentId}`),

  inheritIndicatorLibrary: (data: {
    parent_library_id: number
    target_department_id: string
    target_department_name: string
    name?: string
    description?: string
  }) => api.post('/performance/indicator-libraries/inherit', data),

  // ===== 指标项管理 =====
  getIndicatorItems: (libraryId: number, sectionType?: string) =>
    api.get('/performance/indicator-items', { params: { library_id: libraryId, section_type: sectionType } }),

  createIndicatorItem: (data: {
    library_id: number
    section_type: string
    name: string
    description?: string
    indicator_type?: string
    keywords?: string[]
    calculation_method?: string
    data_source?: string
    cycle?: string
    default_weight?: number
    weight?: number
    red_line_value?: string
    target_value?: string
    challenge_value?: string
    scoring_rule?: string
    is_default?: boolean
    sort_order?: number
  }) => api.post('/performance/indicator-items', data),

  updateIndicatorItem: (itemId: number, data: {
    name?: string
    description?: string
    weight?: number
    red_line_value?: string
    target_value?: string
    challenge_value?: string
    scoring_rule?: string
    is_default?: boolean
    sort_order?: number
  }) => api.put(`/performance/indicator-items/${itemId}`, data),

  deleteIndicatorItem: (itemId: number) =>
    api.delete(`/performance/indicator-items/${itemId}`),

  searchIndicatorItems: (params: {
    keyword?: string
    library_ids?: number[]
    section_type?: string
  }) => api.get('/performance/indicator-items/search', { params }),

  // ===== 目标记录管理（目标设定阶段） =====
  batchSaveGoalRecords: (participantId: number, data: {
    items: {
      id?: number
      section_type: string
      item_name: string
      item_definition?: string
      weight: number
      red_line_value?: string
      target_value?: string
      challenge_value?: string
      scoring_rule?: string
      actual_result?: string
      self_score?: number
      manager_score?: number
      attachments?: string[]
      sort_order?: number
    }[]
  }) => api.post(`/performance/goal-records/${participantId}`, data),

  submitGoalApproval: (participantId: number, data?: { comment?: string }) =>
    api.post(`/performance/goal-records/${participantId}/submit`, data || {}),

  approveGoalRecords: (participantId: number, data?: { comment?: string }) =>
    api.post(`/performance/goal-records/${participantId}/approve`, data || {}),

  rejectGoalRecords: (participantId: number, data: { comment: string }) =>
    api.post(`/performance/goal-records/${participantId}/reject`, data),

  getManagerGoals: (participantId: number) =>
    api.get(`/performance/goal-records/${participantId}/manager-goals`),

  getGoalSuggestions: (participantId: number) =>
    api.get(`/performance/goal-records/${participantId}/suggestions`),

  batchAssignGoals: (activityId: number, data: {
    participant_ids: number[]
    items: {
      section_type: string
      item_name: string
      item_definition?: string
      weight: number
      red_line_value?: string
      target_value?: string
      challenge_value?: string
      scoring_rule?: string
    }[]
  }) => api.post(`/performance/activities/${activityId}/batch-assign-goals`, data),
}

export default api
