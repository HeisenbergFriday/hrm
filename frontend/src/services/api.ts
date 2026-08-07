import axios from 'axios'
import { useAuthStore } from '../store/authStore'
import { authRedirectTargetFromLocation, loginPathWithRedirect, rememberAuthRedirect } from '../utils/authRedirect'
import { csrfHeadersForMethod } from '../utils/csrf'

export const api = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
  withCredentials: true,
  headers: {
    'Content-Type': 'application/json',
  },
})

// 安全模型：token 存于 HttpOnly cookie，请求自动携带（withCredentials）。
// 写操作需附带 CSRF token（双提交 cookie 方案），从 cookie 读取后放入请求头。
api.interceptors.request.use(
  (config) => {
    for (const [key, value] of Object.entries(csrfHeadersForMethod(config.method))) {
      config.headers[key] = value
    }
    return config
  },
  (error) => Promise.reject(error),
)

// 刷新菜单权限（通过 api 实例自动带认证 Cookie，并用锁避免重复刷新）
let isRefreshingMenuKeys = false
export function refreshMenuKeys() {
  if (isRefreshingMenuKeys) return
  isRefreshingMenuKeys = true

  api.get('/auth/me')
    .then((res: any) => {
      const keys = res?.data?.user?.menu_keys
      const perms = res?.data?.user?.permissions
      if (Array.isArray(keys)) useAuthStore.getState().setMenuKeys(keys)
      if (Array.isArray(perms)) useAuthStore.getState().setPermissions(perms)
    })
    .catch((err) => console.error('[refreshMenuKeys] error:', err))
    .finally(() => {
      isRefreshingMenuKeys = false
    })
}

api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      useAuthStore.getState().logout()
      const redirectTarget = authRedirectTargetFromLocation(window.location)
      rememberAuthRedirect(redirectTarget)
      window.location.href = loginPathWithRedirect(redirectTarget)
    }
    if (error.response?.status === 403) {
      refreshMenuKeys()
    }
    return Promise.reject(error)
  },
)

export const authAPI = {
  dingtalkLogin: (data: { code: string }) => api.post('/auth/dingtalk/in-app', data),
  logout: () => api.post('/auth/logout'),
  getCurrentUser: () => api.get('/auth/me'),
}

export const userAPI = {
  getUsers: (params: { page: number; page_size: number; search?: string; department_id?: string; status?: string }) => api.get('/users', { params }),
  getUser: (id: string) => api.get(`/users/${id}`),
  updateUser: (id: string, data: { extension?: any; manager_user_id?: string; manager_name?: string }) => api.put(`/users/${id}`, data),
}

export const departmentAPI = {
  getDepartments: () => api.get('/departments'),
  getDepartment: (id: string) => api.get(`/departments/${id}`),
}

/** 组织全量同步响应（POST /org/sync） */
export type OrgSyncStageResult = {
  status: 'success' | 'partial_failed' | 'failed' | 'skipped'
  success_count: number
  fail_count: number
  error: string
  error_code?: string
}

export type OrgSyncResponse = {
  overall_status: 'success' | 'partial_failed' | 'failed'
  departments: OrgSyncStageResult
  employees: OrgSyncStageResult & {
    position_missing_count: number
    employment_type_missing_count?: number
    job_level_missing_count?: number
    job_family_missing_count?: number
    regularization_date_missing_count?: number
    hrm_field_status?: 'success' | 'failed' | 'success_no_fields'
    hrm_field_error?: string
    overwrite_empty: boolean
		default_role_assigned_count: number
		deactivated_missing_count?: number
		deactivation_status?: 'success' | 'failed'
		deactivation_error?: string
  }
  sync_time: string
  duration_ms: number
  request_id: string
}

export type OrgSyncAPIResponse = {
  code: 200 | 207 | 500
  message: 'success' | 'partial_failed' | 'failed'
  data: OrgSyncResponse
}

export const ORG_SYNC_TIMEOUT_MS = 10 * 60 * 1000
export const ORG_SYNC_REQUEST_CONFIG = Object.freeze({ timeout: ORG_SYNC_TIMEOUT_MS })
export const ORG_SYNC_POLL_INTERVAL_MS = 2 * 1000
export const ORG_SYNC_SHORT_REQUEST_TIMEOUT_MS = 10 * 1000
export const ORG_SYNC_SHORT_REQUEST_CONFIG = Object.freeze({ timeout: ORG_SYNC_SHORT_REQUEST_TIMEOUT_MS })

export type OrgSyncStartAPIResponse = {
  code: 202
  message: 'running'
  data: {
    status: 'running'
    request_id: string
  }
}

export type OrgSyncRunningAPIResponse = {
  code: 202
  message: 'running'
  data: {
    status: 'running'
    request_id: string
    duration_ms?: number
  }
}

export type OrgSyncTaskResponse = {
  status: 'success' | 'partial_failed' | 'failed' | 'skipped'
  success_count: number
  fail_count: number
  error: string
  error_code?: string
  request_id: string
  duration_ms: number
  position_missing_count?: number
  employment_type_missing_count?: number
  job_level_missing_count?: number
  job_family_missing_count?: number
  regularization_date_missing_count?: number
  hrm_field_status?: 'success' | 'failed' | 'success_no_fields'
  hrm_field_error?: string
  overwrite_empty?: boolean
	default_role_assigned_count?: number
	change_log_count?: number
	deactivated_missing_count?: number
	deactivation_status?: 'success' | 'failed'
	deactivation_error?: string
}

export type OrgSyncStatusRecord = {
  last_sync_time: string | null
  status: 'never' | 'running' | 'success' | 'partial_failed' | 'failed' | 'skipped'
  message?: string
  request_id?: string
  duration_ms?: number
  error_code?: string
  success_count?: number
  fail_count?: number
  details?: Record<string, unknown>
}

export type OrgSyncStatusAPIResponse = {
  code: number
  message: string
  data: {
    status: {
      departments: OrgSyncStatusRecord
      users: OrgSyncStatusRecord
      [key: string]: OrgSyncStatusRecord
    }
  }
}

export type OrgSyncTaskAPIResponse = {
  code: 200 | 207 | 500
  message: string
  data: OrgSyncTaskResponse
}

export const syncAPI = {
  syncDepartments: () => api.post<OrgSyncTaskAPIResponse, OrgSyncTaskAPIResponse>(
    '/sync/departments',
    {},
    ORG_SYNC_REQUEST_CONFIG,
  ),
  syncUsers: () => api.post<OrgSyncTaskAPIResponse, OrgSyncTaskAPIResponse>(
    '/sync/users',
    {},
    ORG_SYNC_REQUEST_CONFIG,
  ),
  getSyncStatus: () => api.get<OrgSyncStatusAPIResponse, OrgSyncStatusAPIResponse>('/sync/status'),
}

export const orgAPI = {
  getOverview: (params?: { department_id?: string }) => api.get('/org/overview', { params }),
  getDepartmentTree: (params?: { all?: boolean }) => api.get('/org/departments/tree', { params }),
  getDepartmentHistory: (id: string, params?: { limit?: number }) => api.get(`/org/departments/${id}/history`, { params }),
  getEmployees: (params: { page?: number; page_size?: number; department_id?: string; search?: string; status?: string; filter_type?: string }) =>
    api.get('/org/employees', { params }),
  getEmployee: (id: string) => api.get(`/org/employees/${id}`),
  getEmployeePositionDiagnostic: (id: string) => api.get(`/org/employees/${id}/position-sync-diagnostic`),
  // 长任务先由短请求启动，再轮询短查询，避免网关等待单条连接超过 90 秒后返回 504。
  syncOrg: async (): Promise<OrgSyncAPIResponse> => {
    const startedAt = Date.now()
    const startResponse = await api.post<OrgSyncStartAPIResponse, OrgSyncStartAPIResponse>(
      '/org/sync/start',
      {},
      ORG_SYNC_SHORT_REQUEST_CONFIG,
    )
    const requestID = startResponse.data?.request_id
    if (startResponse.code !== 202 || startResponse.data?.status !== 'running' || !requestID) {
      throw new Error('组织同步启动响应异常，请稍后重试')
    }
    while (Date.now() - startedAt < ORG_SYNC_TIMEOUT_MS) {
      const remainingWaitMS = ORG_SYNC_TIMEOUT_MS - (Date.now() - startedAt)
      await new Promise((resolve) => setTimeout(resolve, Math.min(ORG_SYNC_POLL_INTERVAL_MS, remainingWaitMS)))
      if (Date.now() - startedAt >= ORG_SYNC_TIMEOUT_MS) break
      const response = await api.get<OrgSyncAPIResponse | OrgSyncRunningAPIResponse, OrgSyncAPIResponse | OrgSyncRunningAPIResponse>(
        `/org/sync/${encodeURIComponent(requestID)}`,
        ORG_SYNC_SHORT_REQUEST_CONFIG,
      )
      if (response.code === 202 || ('status' in response.data && response.data.status === 'running')) {
        continue
      }
      return response as OrgSyncAPIResponse
    }
    throw new Error('组织同步页面等待超时，后台任务可能仍在执行，请前往同步日志确认结果，暂勿重复点击')
  },
  getOrganizations: () => api.get('/auth/orgs'),
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

  sync: (data?: { start_date?: string; end_date?: string }) => api.post('/attendance/sync', data),

  export: (data: {
    start_date: string
    end_date: string
    user_id?: string
    department_id?: string
  }) => api.post('/attendance/export', data),

  getExports: (params: { page?: number; page_size?: number }) => api.get('/attendance/exports', { params }),
  getLastSyncTime: () => api.get('/attendance/last-sync'),

  // 外部 Doris 考勤同步中心
  externalSync: {
    getStatus: () => api.get('/attendance/external-sync/status'),
    getDailyResults: (params?: {
      page?: number
      page_size?: number
      user_id?: string
      department_id?: string
      start_date?: string
      end_date?: string
      status?: string
    }) => api.get('/attendance/external-sync/daily-results', { params }),
    // 同步可能运行数分钟；覆盖全局 10s timeout，与后端 Handler 上限对齐
    run: (data?: {
      source?: 'all' | 'attendance' | 'department'
      lookback_minutes?: number
      full_department_snapshot?: boolean
    }) => api.post('/attendance/external-sync/run', data || {}, { timeout: 10 * 60 * 1000 }),
    getJobs: (params?: { page?: number; page_size?: number }) =>
      api.get('/attendance/external-sync/jobs', { params }),
    getJob: (id: number | string) => api.get(`/attendance/external-sync/jobs/${id}`),
  },

  // 考勤数据处理
  processing: {
    leave: (formData: FormData) =>
      api.post('/attendance/processing/leave', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
        responseType: 'blob',
        timeout: 300000,
      }),
    overtime: (formData: FormData) =>
      api.post('/attendance/processing/overtime', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
        responseType: 'blob',
        timeout: 300000,
      }),
    subsidy: (formData: FormData) =>
      api.post('/attendance/processing/subsidy', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
        responseType: 'blob',
        timeout: 300000,
      }),
    final: (formData: FormData) =>
      api.post('/attendance/processing/final', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
        responseType: 'blob',
        timeout: 300000,
      }),
    parttime: (formData: FormData) =>
      api.post('/attendance/processing/parttime', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
        responseType: 'blob',
        timeout: 300000,
      }),
  },
}

export type AttendanceToolboxRunFile = {
  file_key: string
  file_name: string
  content_type: string
  size: number
  kind?: string
  flow_key?: string
  row_count?: number
}

export type AttendanceToolboxRunResponse = {
  run_id: string
  module: string
  log?: string
  stats?: Record<string, unknown>
  meta?: Record<string, unknown>
  files: AttendanceToolboxRunFile[]
  expires_at: string
}

// 后端工具箱默认 600 秒；客户端多等待 60 秒，以便接收后端超时响应。
export const ATTENDANCE_TOOLBOX_REQUEST_TIMEOUT_MS = 11 * 60 * 1000

export const attendanceToolboxAPI = {
  getDefaults: () => api.get('/attendance/toolbox/defaults'),
  run: (module: string, data: FormData) => api.post(`/attendance/toolbox/${module}/run`, data, {
    responseType: 'blob',
    timeout: ATTENDANCE_TOOLBOX_REQUEST_TIMEOUT_MS,
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  }),
  /** Structured workflow: returns run_id + file metadata instead of raw blob. */
  runWorkflow: (module: string, data: FormData) => api.post(`/attendance/toolbox/workflows/${module}`, data, {
    timeout: ATTENDANCE_TOOLBOX_REQUEST_TIMEOUT_MS,
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  }),
  runQuickWorkflow: (data: FormData) => api.post('/attendance/toolbox/workflows/quick', data, {
    timeout: ATTENDANCE_TOOLBOX_REQUEST_TIMEOUT_MS,
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  }),
  getRun: (runId: string) => api.get(`/attendance/toolbox/runs/${runId}`),
  downloadRunFile: (runId: string, fileKey: string) => api.get(`/attendance/toolbox/runs/${runId}/files/${encodeURIComponent(fileKey)}`, {
    responseType: 'blob',
    timeout: ATTENDANCE_TOOLBOX_REQUEST_TIMEOUT_MS,
  }),
  downloadRunZip: (runId: string) => api.get(`/attendance/toolbox/runs/${runId}/zip`, {
    responseType: 'blob',
    timeout: ATTENDANCE_TOOLBOX_REQUEST_TIMEOUT_MS,
  }),
  runDingtalkSync: (data: {
    start_date: string
    end_date: string
    flow_keys?: string[]
    max_instances?: number
    padding_days?: number
    process_leave?: string
    process_overtime?: string
    process_attendance_correction?: string
    process_position_transfer?: string
  }) => api.post('/attendance/toolbox/dingtalk-sync', data, {
    responseType: 'blob',
    timeout: ATTENDANCE_TOOLBOX_REQUEST_TIMEOUT_MS,
  }),
  /** Structured dingtalk sync via workflow API. */
  runDingtalkSyncStructured: (data: FormData | {
    start_date: string
    end_date: string
    flow_keys?: string[]
    max_instances?: number
    padding_days?: number
  }) => {
    if (data instanceof FormData) {
      return api.post('/attendance/toolbox/workflows/dingtalk_sync', data, {
        timeout: ATTENDANCE_TOOLBOX_REQUEST_TIMEOUT_MS,
        headers: { 'Content-Type': 'multipart/form-data' },
      })
    }
    const form = new FormData()
    form.append('dingtalk_sync_start_date', data.start_date)
    form.append('dingtalk_sync_end_date', data.end_date)
    if (data.flow_keys?.length) form.append('dingtalk_sync_flow_keys', data.flow_keys.join(','))
    if (data.max_instances != null) form.append('dingtalk_sync_max_instances', String(data.max_instances))
    if (data.padding_days != null) form.append('dingtalk_sync_padding_days', String(data.padding_days))
    return api.post('/attendance/toolbox/workflows/dingtalk_sync', form, {
      timeout: ATTENDANCE_TOOLBOX_REQUEST_TIMEOUT_MS,
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },
  /** Export default rules, or current session rules when rulesJson is provided. */
  exportRules: (rulesJson?: string) => {
    if (rulesJson && rulesJson.trim()) {
      const form = new FormData()
      form.append('rules_json', rulesJson)
      return api.post('/attendance/toolbox/rules/export', form, {
        responseType: 'blob',
        timeout: 60 * 1000,
        headers: { 'Content-Type': 'multipart/form-data' },
      })
    }
    return api.post('/attendance/toolbox/rules/export', {}, {
      responseType: 'blob',
      timeout: 60 * 1000,
    })
  },
  /** Preview first 200 rows of a stored run result (does not re-run calculation). */
  previewRun: (runId: string, fileKey?: string) => api.get(`/attendance/toolbox/runs/${runId}/preview`, {
    params: fileKey ? { file_key: fileKey } : undefined,
    timeout: 60 * 1000,
  }),
  importRulesPreview: (data: FormData) => api.post('/attendance/toolbox/rules/import-preview', data, {
    timeout: 60 * 1000,
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  }),
  validate: (module: string, data: FormData) => api.post(`/attendance/toolbox/${module}/validate`, data, {
    timeout: 60 * 1000,
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  }),
  exportTemplates: (templateId?: string) => api.post('/attendance/toolbox/templates', { template_id: templateId }, {
    responseType: 'blob',
    timeout: 60 * 1000,
  }),
  listTemplates: () => api.post('/attendance/toolbox/templates', {}, {
    timeout: 60 * 1000,
  }),
  auditUploads: (data: FormData) => api.post('/attendance/toolbox/audit', data, {
    timeout: 60 * 1000,
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  }),
  /** Fetch the DingTalk monthly punch records and return a blob for the part-time module. */
  parttimeMonthlyPunch: (data: { month: string }) => api.post('/attendance/toolbox/parttime-monthly-punch', data, {
    responseType: 'blob',
    timeout: ATTENDANCE_TOOLBOX_REQUEST_TIMEOUT_MS,
  }),
  /** 按当前组织生成标准在职花名册 xlsx（后端读取本组织 active 用户与档案）。 */
  generateOrgRoster: () => api.post('/attendance/toolbox/roster/generate', {}, {
    responseType: 'blob',
    timeout: ATTENDANCE_TOOLBOX_REQUEST_TIMEOUT_MS,
  }),
}

export const APPROVAL_SYNC_TIMEOUT_MS = 15 * 60 * 1000
export const APPROVAL_SYNC_POLL_INTERVAL_MS = 2 * 1000
export const APPROVAL_SYNC_SHORT_REQUEST_TIMEOUT_MS = 10 * 1000
export const APPROVAL_SYNC_SHORT_REQUEST_CONFIG = Object.freeze({ timeout: APPROVAL_SYNC_SHORT_REQUEST_TIMEOUT_MS })
export const APPROVAL_SYNC_STORAGE_KEY = 'peopleops:approval-sync:request-id'
export const APPROVAL_SYNC_NETWORK_RETRY_DELAYS_MS = [500, 1_000, 2_000] as const

export type ApprovalSyncProcessResult = {
  process_code: string
  status: 'success' | 'partial' | 'failed'
  fetched_count: number
  fetch_fail_count: number
  success_count: number
  fail_count: number
  error_code?: string
  error?: string
}

export type ApprovalSyncResult = {
  status: 'success' | 'partial' | 'failed'
  processes: ApprovalSyncProcessResult[]
  process_count: number
  succeeded_processes: number
  failed_processes: number
  fetched_count: number
  fetch_fail_count: number
  success_count: number
  fail_count: number
  start_date: string
  end_date: string
  sync_time: string
  duration_ms: number
  request_id: string
  discovery_error_code?: string
  discovery_error?: string
}

export type ApprovalSyncStartAPIResponse = {
  code: 202
  message: 'running'
  data: { status: 'running'; request_id: string }
}

export type ApprovalSyncRunningAPIResponse = {
  code: 202
  message: 'running'
  data: { status: 'running'; request_id: string; duration_ms?: number }
}

export type ApprovalSyncAPIResponse = {
  code: 200
  message: ApprovalSyncResult['status']
  data: ApprovalSyncResult
}

export type ApprovalStatsAPIResponse = {
  code: 200
  message: 'success'
  data: {
    summary: {
      total: number
      completed: number
      refused: number
      running: number
      terminated: number
      canceled: number
      approval_rate: string
    }
    template_stats: Array<{
      template_id: string
      template_name: string
      total: number
      completed: number
      refused: number
      running: number
      terminated: number
      canceled: number
      approval_rate: string
    }>
  }
}

function approvalSyncStorage(): Storage | undefined {
  return typeof window === 'undefined' ? undefined : window.sessionStorage
}

export function getPendingApprovalSyncRequestID(): string {
  return approvalSyncStorage()?.getItem(APPROVAL_SYNC_STORAGE_KEY)?.trim() || ''
}

function setPendingApprovalSyncRequestID(requestID: string) {
  approvalSyncStorage()?.setItem(APPROVAL_SYNC_STORAGE_KEY, requestID)
}

function clearPendingApprovalSyncRequestID(requestID?: string) {
  const storage = approvalSyncStorage()
  if (!storage) return
  if (!requestID || storage.getItem(APPROVAL_SYNC_STORAGE_KEY) === requestID) {
    storage.removeItem(APPROVAL_SYNC_STORAGE_KEY)
  }
}

async function pollApprovalSync(requestID: string, startedAt = Date.now()): Promise<ApprovalSyncAPIResponse> {
  let networkFailures = 0
  while (Date.now() - startedAt < APPROVAL_SYNC_TIMEOUT_MS) {
    const remainingWaitMS = APPROVAL_SYNC_TIMEOUT_MS - (Date.now() - startedAt)
    await new Promise((resolve) => setTimeout(resolve, Math.min(APPROVAL_SYNC_POLL_INTERVAL_MS, remainingWaitMS)))
    if (Date.now() - startedAt >= APPROVAL_SYNC_TIMEOUT_MS) break
    try {
      const response = await api.get<ApprovalSyncAPIResponse | ApprovalSyncRunningAPIResponse, ApprovalSyncAPIResponse | ApprovalSyncRunningAPIResponse>(
        `/approvals/sync/${encodeURIComponent(requestID)}`,
        APPROVAL_SYNC_SHORT_REQUEST_CONFIG,
      )
      networkFailures = 0
      if (response.code === 202) continue
      clearPendingApprovalSyncRequestID(requestID)
      return response as ApprovalSyncAPIResponse
    } catch (error) {
      const candidate = error as { response?: { status?: number } }
      if (!candidate?.response && networkFailures < APPROVAL_SYNC_NETWORK_RETRY_DELAYS_MS.length) {
        const delay = APPROVAL_SYNC_NETWORK_RETRY_DELAYS_MS[networkFailures++]
        await new Promise((resolve) => setTimeout(resolve, delay))
        continue
      }
      if (candidate?.response?.status === 404) clearPendingApprovalSyncRequestID(requestID)
      throw new Error(`审批同步状态查询失败，后台任务可能仍在执行（请求编号：${requestID}），请稍后刷新页面确认，暂勿重复点击`)
    }
  }
  throw new Error(`审批同步页面等待超时，后台任务可能仍在执行（请求编号：${requestID}），请稍后刷新页面，暂勿重复点击`)
}

export const approvalAPI = {
  getTemplates: () => api.get('/approvals/templates'),
  getInstances: (params: {
    page?: number
    page_size?: number
    status?: string
    template_id?: string
    category?: string
    applicant_id?: string
    title?: string
    start_date?: string
    end_date?: string
  }) => api.get('/approvals/instances', { params }),
  getApproval: (id: string) => api.get(`/approvals/${id}`),
  getStats: (params: { template_id?: string; start_date?: string; end_date?: string }) =>
    api.get<ApprovalStatsAPIResponse, ApprovalStatsAPIResponse>('/approvals/stats', { params }),
  resumeSync: async (): Promise<ApprovalSyncAPIResponse> => {
    const requestID = getPendingApprovalSyncRequestID()
    if (!requestID) throw new Error('没有可恢复的审批同步任务')
    return pollApprovalSync(requestID)
  },
  sync: async (data: { process_code?: string; start_date?: string; end_date?: string }): Promise<ApprovalSyncAPIResponse> => {
    const startedAt = Date.now()
    let requestID = ''
    try {
      const startResponse = await api.post<ApprovalSyncStartAPIResponse, ApprovalSyncStartAPIResponse>(
        '/approvals/sync/start',
        data,
        APPROVAL_SYNC_SHORT_REQUEST_CONFIG,
      )
      requestID = startResponse.data?.request_id
      if (startResponse.code !== 202 || startResponse.data?.status !== 'running' || !requestID) {
        throw new Error('审批同步启动响应异常，请稍后重试')
      }
    } catch (error) {
      const candidate = error as {
        response?: { status?: number; data?: { data?: { request_id?: string } } }
      }
      if (candidate?.response?.status === 409 && candidate.response.data?.data?.request_id) {
        requestID = candidate.response.data.data.request_id
      } else if (candidate?.response) {
        throw error
      } else {
        throw new Error('审批同步启动响应未确认，后台任务可能已启动，请稍后刷新页面确认，暂勿重复点击')
      }
    }
    setPendingApprovalSyncRequestID(requestID)
    return pollApprovalSync(requestID, startedAt)
  },
  getOAData: (params?: { page?: number; page_size?: number; keyword?: string }) =>
    api.get('/approvals/oa-data', { params }),
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
  getRolePermissions: (roleId: number) => api.get(`/permission/roles/${roleId}/permissions`),
  saveRolePermissions: (roleId: number, permissionIds: number[]) => api.post(`/permission/roles/${roleId}/permissions`, { permission_ids: permissionIds }),
  getMenuPermission: (roleId: number) => api.get(`/permission/roles/${roleId}/menu`),
  saveMenuPermission: (roleId: number, menuKeys: string[]) => api.post(`/permission/roles/${roleId}/menu`, { menu_keys: menuKeys }),
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
  getProfiles: (params?: { page?: number; page_size?: number; keyword?: string; department_id?: string; status?: string }) =>
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
  /** 作息表个人推送：multipart(image, user_ids, title, content)，不写考勤排班 */
  pushPersonalSchedule: (formData: FormData) =>
    api.post('/week-schedule/push/personal', formData, {
      timeout: 120000,
      headers: { 'Content-Type': 'multipart/form-data' },
    }),
  /** 查询当前组织群聊绑定记录；响应不包含 openConversationId 或钉钉凭据。 */
  getGroupTargets: () => api.get('/week-schedule/group-targets'),
  /** 手动解绑当前组织的本地群目标。 */
  unbindGroupTarget: (id: number | string) => api.delete(`/week-schedule/group-targets/${id}`),
  /** 作息表群聊推送：前端仅提交本地 group_target_id。 */
  pushGroupSchedule: (formData: FormData) =>
    api.post('/week-schedule/push/group', formData, {
      timeout: 120000,
      headers: { 'Content-Type': 'multipart/form-data' },
    }),
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

export type PerformanceActivityStatus = 'draft' | 'target_setting' | 'target_approval' | 'self_evaluation' | 'manager_evaluation' | 'department_evaluation' | 'hr_review' | 'result_publish' | 'interview' | 'appeal' | 'employee_confirmation' | 'manager_confirmation' | 'hr_confirmation' | 'locked' | 'result_confirmed' | 'archived'

// 绩效活动
export interface PerformanceActivity {
  id: number
  name: string
  cycle_type: string
  start_date: string
  end_date: string
  indicator_library_id?: number
  template_id?: number
  flow_type?: 'old' | 'new' | string
  activity_kind?: 'goal_setting' | 'review_scoring' | string
  organization_id?: string
  applicable_org_scope?: string[]
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
  manager_assignments?: PerformanceActivityManagerAssignment[]
  default_assessment_manager_source?: AssessmentManagerSource
  snapshot_as_of_date?: string
  snapshot_source?: string
  target_plan_activity_id?: number
  previous_review_activity_id?: number
  publish_mode?: 'manual' | 'auto' | string
  publish_at?: string
  reminder_config?: Record<string, unknown>
  workflow_config?: Record<string, unknown>
  form_config?: Record<string, unknown>
  level_rule_config?: Record<string, unknown>
  distribution_config?: Record<string, unknown>
  permission_config?: Record<string, unknown>
  publish_config?: Record<string, unknown>
  enable_bonus_score?: boolean
  strict_time_mode?: boolean
  created_at: string
  updated_at: string
  created_by: string
  updated_by: string
  my_participant?: PerformanceParticipant | null
}

// 绩效参与人状态
export type PerformanceParticipantStatus = 'pending' | 'target_pending_approval' | 'target_rejected' | 'target_set' | 'self_submitted' | 'manager_submitted' | 'manager_recheck' | 'result_confirmed' | 'inactive' | 'removed_from_scope' | 'employee_confirmed' | 'manager_confirmed' | 'hr_confirmed' | 'locked'

export type AssessmentManagerSource = 'DIRECT_MANAGER' | 'DEPARTMENT_HEAD' | 'CENTER_HEAD' | 'MANUAL' | 'IMPORT' | 'EMPTY' | 'SYSTEM'
export type AssessmentManagerConfigStatus = 'CONFIGURED' | 'PENDING' | 'INVALID'

export interface PerformanceActivityManagerAssignment {
  row?: number
  user_id: string
  employee_id?: string
  assessment_manager_user_id: string
  assessment_manager_employee_id?: string
  assessment_manager_name: string
  assessment_manager_source: AssessmentManagerSource
  manager_override_reason?: string
}

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
  snapshot_source?: string
  snapshot_as_of_date?: string
  manager_id?: string
  manager_name?: string
  direct_manager_id_snapshot?: string
  direct_manager_name_snapshot?: string
  manager_source?: AssessmentManagerSource
  manager_overridden?: boolean
  manager_override_reason?: string
  manager_config_status?: AssessmentManagerConfigStatus
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
  department_adjusted?: boolean
  department_final_score?: number | null
  department_final_level?: string
  department_adjust_reason?: string
  department_adjusted_at?: string
  department_adjusted_by?: string
  result_hidden?: boolean
  result_hidden_reason?: string
  result_hidden_at?: string
  result_hidden_by?: string
  removed_reason?: string
  removed_at?: string
  removed_by?: string
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
  target_set_count?: number
  self_submitted_count: number
  manager_submitted_count: number
  employee_confirmed_count?: number
  manager_confirmed_count?: number
  hr_confirmed_count?: number
  locked_count?: number
  result_confirmed_count: number
  level_distribution: Record<string, number>
}

export type PerformanceInterviewStatus = 'pending' | 'scheduled' | 'completed' | 'cancelled'
export type PerformanceInterviewType = 'required' | 'optional'

export interface PerformanceInterviewRecord {
  id: number
  activity_id: string
  activity_name: string
  participant_id: number
  employee_id: string
  employee_name: string
  department_id: string
  department_name: string
  position: string
  final_level: string
  interview_type: PerformanceInterviewType
  status: PerformanceInterviewStatus
  interviewer_id?: string
  interviewer_name?: string
  scheduled_at?: string
  completed_at?: string
  location?: string
  summary?: string
  result?: string
  cancel_reason?: string
  created_at: string
  updated_at: string
  created_by?: string
  updated_by?: string
}

export type PerformanceAppealStatus = 'submitted' | 'processing' | 'resolved' | 'rejected' | 'withdrawn'

export interface PerformanceAppealRecord {
  id: number
  activity_id: string
  activity_name: string
  participant_id: number
  employee_id: string
  employee_name: string
  department_id: string
  department_name: string
  position: string
  final_level: string
  status: PerformanceAppealStatus
  appeal_reason: string
  desired_result?: string
  handler_id?: string
  handler_name?: string
  handle_comment?: string
  handled_at?: string
  withdraw_reason?: string
  created_at: string
  updated_at: string
  created_by?: string
  updated_by?: string
}

export interface PerformanceFollowupSummary {
  total: number
  status_map: Record<string, number>
  pending: number
  processing: number
  completed: number
  closed: number
}

export interface PerformanceFollowupListResponse<T> {
  items: T[]
  total: number
  summary: PerformanceFollowupSummary
}

export interface PerformanceReportFilters {
  company_id?: string
  department_id?: string
  status?: string
  level?: string
  employee_keyword?: string
}

export interface PerformanceChartItem {
  name: string
  value: number
  rate: number
}

export interface PerformanceProgressReportRow {
  participant_id: number
  employee_id: string
  employee_name: string
  department_id: string
  department_name: string
  position: string
  manager_id: string
  manager_name: string
  status: string
  target_submitted: boolean
  self_submitted: boolean
  manager_submitted: boolean
  department_reviewed: boolean
  hr_confirmed: boolean
  locked: boolean
  progress_rate: number
  result_hidden: boolean
}

export interface PerformanceProgressReport {
  summary: {
    total_participants: number
    target_submitted_count: number
    self_submitted_count: number
    manager_submitted_count: number
    department_reviewed_count: number
    hr_confirmed_count: number
    locked_count: number
    completion_rate: number
  }
  rows: PerformanceProgressReportRow[]
  status_distribution: PerformanceChartItem[]
}

export interface PerformanceContentReportRow {
  id: number
  participant_id: number
  employee_id: string
  employee_name: string
  department_id: string
  department_name: string
  position: string
  manager_name: string
  goal_phase: string
  goal_phase_label: string
  section_type: string
  goal_type: string
  item_name: string
  item_definition: string
  weight: number
  target_value: string
  challenge_value: string
  metric_unit: string
  completion_rate: number
  actual_result: string
  self_score: number
  manager_score: number
  bonus_score: number
  attachments_count: number
  approval_status: string
}

export interface PerformanceContentReport {
  summary: {
    review_item_count: number
    plan_item_count: number
    participants_with_review: number
    participants_with_plan: number
    average_completion_rate: number
  }
  rows: PerformanceContentReportRow[]
  phases: PerformanceChartItem[]
}

export interface PerformanceResultReportRow {
  participant_id: number
  employee_id: string
  employee_name: string
  department_id: string
  department_name: string
  position: string
  manager_name: string
  status: string
  self_score: number
  manager_score: number
  total_self_score: number
  total_manager_score: number
  bonus_score: number
  penalty_score: number
  adjusted_score: number
  suggested_level: string
  final_level: string
  effective_final_level: string
  adjust_reason: string
  department_final_score?: number | null
  department_final_level: string
  department_adjust_reason: string
  hr_confirmed: boolean
  locked: boolean
  result_hidden: boolean
  result_visible: boolean
}

export interface PerformanceResultReport {
  summary: { total_participants: number; locked_count: number; hidden_count: number; average_score: number }
  rows: PerformanceResultReportRow[]
  level_distribution: PerformanceChartItem[]
  department_distribution: PerformanceChartItem[]
}

export interface PerformanceReport {
  activity: PerformanceActivity
  is_new_flow: boolean
  progress: PerformanceProgressReport
  content: PerformanceContentReport
  result: PerformanceResultReport
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
  template_id?: number
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
  goal_phase?: 'review' | 'plan' | string
  goal_type?: 'okr' | 'kpi' | 'fixed' | string
  fixed_key?: string
  is_fixed?: boolean
  item_name: string
  item_definition: string
  weight: number
  red_line_value: string
  target_value: string
  challenge_value: string
  metric_unit?: string
  completion_rate?: number
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

export interface PerformanceGoalApprovalLog {
  id: number
  participant_id: number
  activity_id: string
  goal_record_id?: number
  action: 'submit' | 'approve' | 'reject' | string
  comment: string
  approver_id?: string
  approver_name?: string
  version?: number
  created_by?: string
  created_at: string
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

export interface PerformanceActivityImportIssue {
  level: 'info' | 'warning' | 'error' | string
  code: string
  message: string
  draft_key?: string
  sheet?: string
  row?: number
}

export interface PerformanceActivityImportItemDraft {
  name: string
  description?: string
  weight: number
  max_score: number
}

export interface PerformanceActivityImportSectionDraft {
  name: string
  section_type: string
  weight: number
  is_score_required: boolean
  is_comment_required: boolean
  items: PerformanceActivityImportItemDraft[]
}

export interface PerformanceActivityImportGoalDraft {
  section_type: string
  goal_type: string
  fixed_key?: string
  is_fixed: boolean
  item_name: string
  item_definition?: string
  weight: number
  red_line_value?: string
  target_value?: string
  challenge_value?: string
  scoring_rule?: string
  sort_order: number
}

export interface PerformanceActivityImportDraft {
  draft_key: string
  selected: boolean
  source_sheet: string
  template_name: string
  activity_name: string
  flow_type: 'old' | 'new' | string
  activity_kind?: 'goal_setting' | 'review_scoring' | string
  cycle_type: 'monthly' | 'quarterly' | 'annual' | string
  start_date: string
  end_date: string
  enable_bonus_score: boolean
  employee_name?: string
  employee_user_id?: string
  employee_match: 'matched' | 'unmatched' | 'ambiguous' | string
  sections: PerformanceActivityImportSectionDraft[]
  goals: PerformanceActivityImportGoalDraft[]
  source_weight_total: number
}

export interface PerformanceActivityImportPreview {
  source_type: 'xiaotie' | 'muteng' | string
  source_label: string
  file_name: string
  file_sha256: string
  drafts: PerformanceActivityImportDraft[]
  issues: PerformanceActivityImportIssue[]
  requires_review: boolean
}

export interface PerformanceActivityImportCreatedResult {
  draft_key: string
  template_id: number
  template_reused: boolean
  activity_id: number
  activity_name: string
  participant_id?: number
  employee_user_id?: string
  goal_count: number
}

export interface PerformanceActivityImportCommitResult {
  batch_id: string
  created: PerformanceActivityImportCreatedResult[]
  warnings?: string[]
}

export interface PerformanceActivityImportBatch {
  batch_id: string
  status: string
  preview?: PerformanceActivityImportPreview
  result?: PerformanceActivityImportCommitResult
  failure_message?: string
  expires_at?: string
  created_at: string
  committed_at?: string
}

export interface PerformanceActivityImportCommitDraft {
  draft_key: string
  selected: boolean
  template_name: string
  activity_name: string
  cycle_type: string
  start_date: string
  end_date: string
  employee_user_id?: string
}
export interface PerformanceParticipantImportResult {
  activity_name: string
  employee_ids: string[]
  employees?: {
    user_id: string
    employee_id?: string
    name?: string
    department_id?: string
    department_name?: string
    assessment_manager_user_id?: string
    assessment_manager_employee_id?: string
    assessment_manager_name?: string
    assessment_manager_source?: AssessmentManagerSource
    manager_override_reason?: string
  }[]
  manager_assignments?: PerformanceActivityManagerAssignment[]
  manager_assignment_skipped_rows?: { row: number; reason: string }[]
  parsed_count: number
  imported_count: number
  duplicate_count: number
  missing_employee_ids: string[]
  inactive_employee_ids: string[]
  skipped_rows: { row: number; reason: string }[]
  warnings: string[]
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
  review_type: 'self' | 'manager' | 'adjust' | 'confirm' | 'department_evaluation' | string
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
  operation_meta?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface PreviousPerformanceResult {
  activity?: PerformanceActivity | null
  participant?: PerformanceParticipant | null
  goal_records?: PerformanceGoalRecord[]
  versions?: PerformanceReviewVersion[]
}

// 创建绩效活动请求
export interface CreatePerformanceActivityRequest {
  name: string
  cycle_type: string
  start_date: string
  end_date: string
  template_id?: number
  flow_type?: 'old' | 'new' | string
  activity_kind?: 'goal_setting' | 'review_scoring' | string
  organization_id?: string
  applicable_org_scope?: string[]
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
  snapshot_as_of_date?: string
  snapshot_source?: string
  target_plan_activity_id?: number
  previous_review_activity_id?: number
  publish_mode?: 'manual' | 'auto' | string
  publish_at?: string
  reminder_config?: Record<string, unknown>
  enable_bonus_score?: boolean
  strict_time_mode?: boolean
}

// 关系变更日志
export interface RelationshipChangeLog {
  id: number
  activity_id: string
  participant_id: number
  user_id?: string
  change_type: string
  field_name: string
  old_value: string
  new_value: string
  changed_at: string
  source: string
  created_by: string
  old_manager_id?: string
  old_manager_name?: string
  new_manager_id?: string
  new_manager_name?: string
  old_manager_source?: AssessmentManagerSource
  new_manager_source?: AssessmentManagerSource
  reason?: string
  operator_id?: string
  operator_name?: string
}

export interface AssessmentManagerCandidate {
  user_id: string
  name: string
  employee_no?: string
  department_name?: string
  candidate_source: AssessmentManagerSource
  candidate_source_label: string
  is_self_final_candidate?: boolean
}

export interface AssessmentManagerCandidateSourceGroup {
  source: AssessmentManagerSource
  source_label: string
  items: AssessmentManagerCandidate[]
  reason?: string
}

export interface AssessmentManagerUpdatePayload {
  manager_user_id: string
  manager_source: AssessmentManagerSource
  reason?: string
}

export interface AssessmentManagerBatchItem extends AssessmentManagerUpdatePayload {
  participant_id: number
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

export interface PerformanceReminderRecipient {
  user_id: string
  name?: string
}

export type PerformanceReminderRecipientDetail = string | PerformanceReminderRecipient

export interface PerformanceSelfEvalReminderResult {
  pending: number
  candidates: number
  sent: number
  skipped: number
  already_sent?: number
  failed: number
  sent_recipients?: PerformanceReminderRecipientDetail[]
  skipped_recipients?: PerformanceReminderRecipientDetail[]
  already_sent_recipients?: PerformanceReminderRecipientDetail[]
  failed_recipients?: PerformanceReminderRecipientDetail[]
  missing_id_participant_ids?: number[]
}

export interface PerformanceHRConfirmReminderResult {
  pending: number
  candidates: number
  sent: number
  skipped: number
  failed: number
  sent_recipients?: PerformanceReminderRecipientDetail[]
  skipped_recipients?: PerformanceReminderRecipientDetail[]
  failed_recipients?: PerformanceReminderRecipientDetail[]
}

export interface PerformanceManagerEvalReminderResult extends PerformanceHRConfirmReminderResult {}

export interface PerformanceHRForceLockResult {
  force_locked_count: number
  locked_count: number
  already_locked_count: number
  total_count: number
}

export interface PerformanceTemplatePayload {
  name: string
  code?: string
  description?: string
  flow_type?: 'old' | 'new' | string
  organization_id?: string
  organization_scope?: string[]
  status?: string
  cycle_types?: string[]
  workflow_config?: Record<string, unknown>
  form_config?: Record<string, unknown>
  level_rule_config?: Record<string, unknown>
  distribution_config?: Record<string, unknown>
  permission_config?: Record<string, unknown>
  publish_config?: Record<string, unknown>
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

export interface PerformanceTemplate extends PerformanceTemplatePayload {
  id: number
  code: string
  flow_type: 'old' | 'new' | string
  status: string
  created_at?: string
  updated_at?: string
  created_by?: string
  updated_by?: string
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
    template_id?: number
    flow_type?: 'old' | 'new' | string
    activity_kind?: 'goal_setting' | 'review_scoring' | string
    organization_id?: string
    applicable_org_scope?: string[]
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
    manager_assignments?: PerformanceActivityManagerAssignment[]
    default_assessment_manager_source?: AssessmentManagerSource
    indicator_library_id?: number
    description?: string
    snapshot_as_of_date?: string
    snapshot_source?: string
    target_plan_activity_id?: number
    previous_review_activity_id?: number
    publish_mode?: 'manual' | 'auto' | string
    publish_at?: string
    reminder_config?: Record<string, unknown>
    enable_bonus_score?: boolean
    strict_time_mode?: boolean
  }) => api.post('/performance/activities', data),

  getActivity: (activityId: number) =>
    api.get(`/performance/activities/${activityId}`),

  updateActivity: (activityId: number, data: {
    name: string
    cycle_type: string
    start_date: string
    end_date: string
    template_id?: number
    flow_type?: 'old' | 'new' | string
    activity_kind?: 'goal_setting' | 'review_scoring' | string
    organization_id?: string
    applicable_org_scope?: string[]
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
    manager_assignments?: PerformanceActivityManagerAssignment[]
    default_assessment_manager_source?: AssessmentManagerSource
    indicator_library_id?: number
    description?: string
    snapshot_as_of_date?: string
    snapshot_source?: string
    target_plan_activity_id?: number
    previous_review_activity_id?: number
    publish_mode?: 'manual' | 'auto' | string
    publish_at?: string
    reminder_config?: Record<string, unknown>
    enable_bonus_score?: boolean
    strict_time_mode?: boolean
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

  // 绩效活动参与范围选项（精简字段，不走通用 /users）
  getScopeOptions: (params?: { page?: number; page_size?: number; keyword?: string }) =>
    api.get('/performance/scope-options', { params }),

  // 新增状态流转（9状态流）
  openTargetSetting: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-target-setting`),

  openTargetApproval: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-target-approval`),

  openDepartmentEvaluation: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-department-evaluation`),

  openHRReview: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-hr-review`),

  openResultPublish: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-result-publish`),

  openPerformanceInterviewStage: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-performance-interview`),

  openPerformanceAppeal: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/open-performance-appeal`),

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

  getMyParticipants: (activityIds: number[]) =>
    api.get('/performance/participants/my', {
      params: { activity_ids: activityIds.join(',') },
    }),

  refreshParticipants: (activityId: number) =>
    api.post(`/performance/activities/${activityId}/refresh-participants`),

  importParticipants: (file: File) => {
    const formData = new FormData()
    formData.append('file', file)
    return api.post('/performance/participants/import', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 30000,
    })
  },

  analyzeActivityImport: (file: File) => {
    const formData = new FormData()
    formData.append('file', file)
    return api.post('/performance/imports/analyze', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 60000,
    })
  },

  getActivityImport: (batchId: string) =>
    api.get(`/performance/imports/${batchId}`),

  commitActivityImport: (batchId: string, drafts: PerformanceActivityImportCommitDraft[]) =>
    api.post(`/performance/imports/${batchId}/commit`, { drafts }),
  getParticipant: (participantId: number) =>
    api.get(`/performance/participants/${participantId}`),

  updateAssessmentManager: (participantId: number, data: AssessmentManagerUpdatePayload) =>
    api.put(`/performance/participants/${participantId}/assessment-manager`, data),

  batchUpdateAssessmentManagers: (activityId: number, items: AssessmentManagerBatchItem[]) =>
    api.post(`/performance/activities/${activityId}/assessment-managers/batch`, { items }),

  getAssessmentManagerCandidates: (activityId: number, params?: {
    participant_id?: number
    source?: AssessmentManagerSource
    keyword?: string
    limit?: number
  }) => api.get(`/performance/activities/${activityId}/assessment-manager-candidates`, { params }),

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
  getPerformanceInterviews: (params?: {
    page?: number
    page_size?: number
    activity_id?: number | string
    status?: PerformanceInterviewStatus | string
    employee_keyword?: string
  }) => api.get('/performance/interviews', { params }),

  createPerformanceInterview: (data: {
    participant_id: number
    interview_type?: PerformanceInterviewType
    status?: PerformanceInterviewStatus
    interviewer_id?: string
    interviewer_name?: string
    scheduled_at?: string
    location?: string
    summary?: string
    result?: string
    cancel_reason?: string
  }) => api.post('/performance/interviews', data),

  updatePerformanceInterview: (id: number, data: {
    interview_type?: PerformanceInterviewType
    status?: PerformanceInterviewStatus
    interviewer_id?: string
    interviewer_name?: string
    scheduled_at?: string
    location?: string
    summary?: string
    result?: string
    cancel_reason?: string
  }) => api.put(`/performance/interviews/${id}`, data),

  triggerPerformanceInterview: (participantId: number, interviewType: 'required' | 'optional') =>
    api.post(`/performance/participants/${participantId}/trigger-interview`, { interview_type: interviewType }),

  getPerformanceAppeals: (params?: {
    page?: number
    page_size?: number
    activity_id?: number | string
    status?: PerformanceAppealStatus | string
    employee_keyword?: string
  }) => api.get('/performance/appeals', { params }),

  createPerformanceAppeal: (data: {
    participant_id: number
    appeal_reason: string
    desired_result?: string
  }) => api.post('/performance/appeals', data),

  updatePerformanceAppeal: (id: number, data: {
    status: Exclude<PerformanceAppealStatus, 'submitted' | 'withdrawn'>
    handler_id?: string
    handler_name?: string
    handle_comment?: string
  }) => api.put(`/performance/appeals/${id}`, data),

  withdrawPerformanceAppeal: (id: number, reason?: string) =>
    api.post(`/performance/appeals/${id}/withdraw`, { reason }),

  adminAdjustParticipantProgress: (participantId: number, status: PerformanceParticipantStatus, reason: string) =>
    api.post(`/performance/participants/${participantId}/admin-progress`, { status, reason }),

  removePerformanceParticipant: (participantId: number, reason: string) =>
    api.post(`/performance/participants/${participantId}/remove`, { reason }),

  departmentEvaluateParticipantResult: (participantId: number, data: {
    final_level: string
    final_score?: number
    reason: string
  }) => api.post(`/performance/participants/${participantId}/department-evaluation`, data),

  setParticipantResultVisibility: (participantId: number, hidden: boolean, reason: string) =>
    api.put(`/performance/participants/${participantId}/result-visibility`, { hidden, reason }),

  // ===== 调整最终等级 =====
  adjustFinalLevel: (participantId: number, finalLevel: string, reason: string) =>
    api.post(`/performance/participants/${participantId}/adjust-final-level`, { final_level: finalLevel, reason }),

  // ===== 确认结果 =====
  confirmResult: (participantId: number, confirmComment?: string) =>
    api.post(`/performance/participants/${participantId}/confirm-result`, { confirm_comment: confirmComment }),

  // ===== 版本记录 =====
  getParticipantVersions: (participantId: number) =>
    api.get(`/performance/participants/${participantId}/versions`),

  getPreviousParticipantResult: (participantId: number) =>
    api.get(`/performance/participants/${participantId}/previous-result`),

  // ===== 目标记录 =====
  getGoalRecords: (participantId: number) =>
    api.get(`/performance/goal-records/${participantId}`),

  // ===== 新版评分（基于目标指标） =====
  submitGoalSelfEvaluation: (participantId: number, data: {
    items: { record_id: number; actual_result: string; attachments?: string[]; self_score: number }[]
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

  getReport: (activityId: number, params?: PerformanceReportFilters) =>
    api.get(`/performance/activities/${activityId}/report`, { params }),

  exportReport: (activityId: number, params?: PerformanceReportFilters & { report_type?: 'all' | 'progress' | 'content' | 'result' }) =>
    api.get(`/performance/activities/${activityId}/report/export`, { params, responseType: 'blob', timeout: 60000 }),

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
    template_id?: number
    keyword?: string
    status?: string
  }) => api.get('/performance/indicator-libraries', { params }),

  createIndicatorLibrary: (data: {
    department_id: string
    department_name: string
    template_id: number
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
    template_id?: number
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
      goal_phase?: string
      goal_type?: string
      fixed_key?: string
      is_fixed?: boolean
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

  batchSaveReviewGoalRecords: (participantId: number, data: {
    items: {
      id?: number
      section_type: string
      goal_phase?: string
      goal_type?: string
      fixed_key?: string
      is_fixed?: boolean
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
  }) => api.post(`/performance/goal-records/${participantId}/review-supplement`, data),

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
