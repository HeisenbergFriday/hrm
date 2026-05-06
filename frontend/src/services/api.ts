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

api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      useAuthStore.getState().logout()
      window.location.href = '/login'
    }
    return Promise.reject(error)
  },
)

export const authAPI = {
  login: (data: { username: string; password: string }) => api.post('/auth/login', data),
  dingtalkLogin: (data: { code: string }) => api.post('/auth/dingtalk', data),
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
  sync: (data?: { start_date?: string; end_date?: string }) => api.post('/approvals/sync', data),
}

export const permissionAPI = {
  getRoles: () => api.get('/permission/roles'),
  createRole: (data: { name: string; description: string }) => api.post('/permission/roles', data),
  getPermissions: () => api.get('/permission/permissions'),
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

  getCalendar: (params: { weeks?: number; user_id?: string; department_id?: string }) =>
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
  runMatch: (data: { start_date: string; end_date: string }) =>
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
}

export default api
