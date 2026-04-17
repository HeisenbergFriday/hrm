import axios from 'axios'
import { useAuthStore } from '../store/authStore'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// 请求拦截器
api.interceptors.request.use(
  (config) => {
    const token = useAuthStore.getState().token
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// 响应拦截器
api.interceptors.response.use(
  (response) => {
    return response.data
  },
  (error) => {
    if (error.response?.status === 401) {
      useAuthStore.getState().logout()
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

// 认证相关
export const authAPI = {
  login: (data: { username: string; password: string }) => api.post('/auth/login', data),
  dingtalkLogin: (data: { code: string }) => api.post('/auth/dingtalk', data),
  logout: () => api.post('/auth/logout'),
  getCurrentUser: () => api.get('/auth/me')
}

// 用户相关
export const userAPI = {
  getUsers: (params: { page: number; page_size: number }) => api.get('/users', { params }),
  getUser: (id: string) => api.get(`/users/${id}`),
  updateUser: (id: string, data: { extension: any }) => api.put(`/users/${id}`, data)
}

// 部门相关
export const departmentAPI = {
  getDepartments: () => api.get('/departments'),
  getDepartment: (id: string) => api.get(`/departments/${id}`)
}

// 同步相关
export const syncAPI = {
  syncDepartments: () => api.post('/sync/departments'),
  syncUsers: () => api.post('/sync/users'),
  getSyncStatus: () => api.get('/sync/status')
}

// 组织与员工模块
export const orgAPI = {
  // 部门树
  getDepartmentTree: () => api.get('/org/departments/tree'),

  // 员工相关
  getEmployees: (params: { page?: number; page_size?: number; department_id?: string }) => api.get('/org/employees', { params }),
  getEmployee: (id: string) => api.get(`/org/employees/${id}`),

  // 同步
  syncOrg: () => api.post('/org/sync')
}

// 考勤模块
export const attendanceAPI = {
  // 考勤记录
  getRecords: (params: {
    page?: number
    page_size?: number
    user_id?: string
    department_id?: string
    start_date?: string
    end_date?: string
  }) => api.get('/attendance/records', { params }),

  // 考勤统计
  getStats: (params: {
    start_date?: string
    end_date?: string
    department_id?: string
  }) => api.get('/attendance/stats', { params }),

  // 同步考勤数据
  sync: (data?: { start_date?: string; end_date?: string }) => api.post('/attendance/sync', data),

  // 导出考勤数据
  export: (data: {
    start_date: string
    end_date: string
    user_id?: string
    department_id?: string
  }) => api.post('/attendance/export', data),

  // 获取导出记录
  getExports: (params: { page?: number; page_size?: number }) => api.get('/attendance/exports', { params }),

  // 获取最近同步时间
  getLastSyncTime: () => api.get('/attendance/last-sync')
}

// 审批模块
export const approvalAPI = {
  // 获取审批模板列表
  getTemplates: () => api.get('/approvals/templates'),

  // 获取审批实例列表
  getInstances: (params: {
    page?: number
    page_size?: number
    status?: string
    template_id?: string
    applicant_id?: string
    start_date?: string
    end_date?: string
  }) => api.get('/approvals/instances', { params }),

  // 获取审批详情
  getApproval: (id: string) => api.get(`/approvals/${id}`),

  // 同步审批数据
  sync: (data?: { start_date?: string; end_date?: string }) => api.post('/approvals/sync', data)
}

// 权限管理模块
export const permissionAPI = {
  // 获取角色列表
  getRoles: () => api.get('/permission/roles'),
  // 创建角色
  createRole: (data: { name: string; description: string }) => api.post('/permission/roles', data),
  // 获取权限列表
  getPermissions: () => api.get('/permission/permissions'),
}

// 审计日志模块
export const auditAPI = {
  // 获取审计日志
  getLogs: (params: {
    page?: number
    page_size?: number
    start_date?: string
    end_date?: string
    user_id?: string
  }) => api.get('/audit/logs', { params }),
}

// 任务中心模块
export const jobAPI = {
  // 获取任务列表
  getJobs: () => api.get('/jobs'),
  // 运行任务
  runJob: (id: string) => api.post(`/jobs/${id}/run`),
}

// 员工档案中心模块
export const employeeAPI = {
  // 员工档案
  getProfiles: (params?: { page?: number; page_size?: number; department_id?: string; status?: string }) => 
    api.get('/employee/profiles', { params }),
  getProfile: (id: string) => api.get(`/employee/profiles/${id}`),
  createProfile: (data: any) => api.post('/employee/profiles', data),
  updateProfile: (id: string, data: any) => api.put(`/employee/profiles/${id}`, data),
  
  // 转岗
  getTransfers: (params?: { page?: number; page_size?: number; status?: string }) => 
    api.get('/employee/transfers', { params }),
  createTransfer: (data: any) => api.post('/employee/transfers', data),
  
  // 离职
  getResignations: (params?: { page?: number; page_size?: number; status?: string }) => 
    api.get('/employee/resignations', { params }),
  createResignation: (data: any) => api.post('/employee/resignations', data),
  
  // 入职
  getOnboardings: (params?: { page?: number; page_size?: number; status?: string }) => 
    api.get('/employee/onboardings', { params }),
  createOnboarding: (data: any) => api.post('/employee/onboardings', data),
}

// 人才分析模块
export const talentAPI = {
  getAnalysis: (params?: { page?: number; page_size?: number; department_id?: string }) => 
    api.get('/talent/analysis', { params }),
  getAnalysisDetail: (id: string) => api.get(`/talent/analysis/${id}`),
  createAnalysis: (data: any) => api.post('/talent/analysis', data),
}

export default api