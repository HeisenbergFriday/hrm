import { Modal, message } from 'antd'
import axios from 'axios'
import { orgAPI, type OrgSyncAPIResponse, type OrgSyncResponse, type OrgSyncTaskResponse } from '../services/api'
import { hasPermission } from './permission'

export const ORG_SYNC_PERMISSION = 'attendance_manage'

export function canSyncOrgData(): boolean {
  return hasPermission(ORG_SYNC_PERMISSION)
}

export const missingOrgSyncPermissionTip = '你缺少 attendance_manage 权限，需要联系管理员添加'

/** 是否正在同步中（防重复点击） */
let syncInProgress = false

const safeClientErrorMessages = new Set([
  '组织同步启动响应异常，请稍后重试',
  '组织同步页面等待超时，后台任务可能仍在执行，请前往同步日志确认结果，暂勿重复点击',
])

export function getRequestID(data: unknown): string | undefined {
  if (!data || typeof data !== 'object') return undefined
  const outer = data as { request_id?: unknown; data?: { request_id?: unknown } }
  const requestID = outer.request_id ?? outer.data?.request_id
  return typeof requestID === 'string' && /^[A-Za-z0-9._:-]{1,128}$/.test(requestID) ? requestID : undefined
}

const syncErrorCodeMessages: Record<string, string> = {
  DINGTALK_CONFIG_MISSING: '钉钉组织配置缺失',
  DINGTALK_TOKEN_FAILED: '获取钉钉访问凭证失败',
  DINGTALK_PERMISSION_DENIED: '钉钉通讯录权限不足',
  DINGTALK_NETWORK_FAILED: '连接钉钉服务失败',
  DINGTALK_RESPONSE_INVALID: '钉钉返回的部门数据格式异常',
	DINGTALK_DEPARTMENT_EMPTY: '钉钉返回的部门数据为空',
	DINGTALK_USER_SOURCE_INCOMPLETE: '钉钉员工源数据不完整，已停止同步以避免误停用历史员工',
  DEPARTMENT_PERSIST_FAILED: '部门数据写入失败，原有组织数据未被覆盖',
  EMPLOYEE_SYNC_SKIPPED: '部门同步未完成，已跳过员工同步',
	EMPLOYEE_SYNC_FAILED: '员工同步失败',
	EMPLOYEE_DEACTIVATION_FAILED: '历史员工安全停用未完成',
}

function safeSyncReason(errorCode: unknown, _serverMessage: unknown, fallback: string): string {
  if (typeof errorCode === 'string' && syncErrorCodeMessages[errorCode]) {
    return syncErrorCodeMessages[errorCode]
  }
  return fallback
}

function withRequestID(messageText: string, requestID?: string): string {
  const safeRequestID = getRequestID({ request_id: requestID })
  return safeRequestID ? `${messageText}（请求编号：${safeRequestID}）` : messageText
}

function formatHRMFieldSummary(employees: OrgSyncResponse['employees']): string | undefined {
  if (employees.hrm_field_status === 'failed') {
    return '智能人事字段同步失败，请检查钉钉应用的智能人事花名册权限'
  }

  if (employees.hrm_field_status === 'success_no_fields') {
    return employees.hrm_field_error || '智能人事接口调用成功，但未获取到员工类型、职级、岗位序列字段，请检查钉钉应用的花名册字段权限或字段代码配置'
  }

  const missing: string[] = []
  if ((employees.employment_type_missing_count ?? 0) > 0) {
    missing.push(`员工类型 ${employees.employment_type_missing_count} 人`)
  }
  if ((employees.job_level_missing_count ?? 0) > 0) {
    missing.push(`职级 ${employees.job_level_missing_count} 人`)
  }
  if ((employees.job_family_missing_count ?? 0) > 0) {
    missing.push(`岗位序列 ${employees.job_family_missing_count} 人`)
  }
  if ((employees.regularization_date_missing_count ?? 0) > 0) {
    missing.push(`转正日期 ${employees.regularization_date_missing_count} 人`)
  }
  return missing.length > 0 ? `仍有字段未填写：${missing.join('、')}` : undefined
}

function logSyncError(err: unknown): void {
  if (!import.meta.env.DEV) return
  if (axios.isAxiosError(err)) {
    console.error('[org-sync]', {
      error_type: err.name || 'AxiosError',
      code: err.code,
      http_status: err.response?.status,
      request_id: getRequestID(err.response?.data),
    })
    return
  }
  console.error('[org-sync]', { error_type: err instanceof Error ? err.name : typeof err })
}

/**
 * 解析同步失败的具体原因，返回用户友好的中文提示。
 * 不泄露 Token、密钥、数据库错误或完整内部堆栈。
 */
export function resolveSyncErrorMessage(err: unknown): string {
  if (axios.isAxiosError(err)) {
    // 请求超时（Axios 内部超时，非 HTTP 408）
    if (err.code === 'ECONNABORTED' || err.code === 'ETIMEDOUT' || /timeout/i.test(err.message || '')) {
      return '请求等待超时，同步可能仍在后台执行，请前往同步日志查看，暂勿重复点击'
    }
    // 网络错误（断网、DNS 失败、连接被拒）
    if (err.code === 'ERR_NETWORK' || !err.response) {
      return '网络异常，请检查网络连接后重试'
    }
    const status = err.response.status
    // 认证失败
    if (status === 401) {
      return '登录已过期，请重新登录'
    }
    // 权限不足
    if (status === 403) {
      return '权限不足，无法执行同步操作'
    }
    // 并发冲突
    if (status === 409) {
      return '该组织正在同步中，请勿重复提交'
    }
    // 查询不到通常表示任务状态已丢失、服务重启或 request_id 已失效。
    if (status === 404) {
      return '同步任务不存在或服务可能已重启，请前往同步日志确认结果，暂勿重复点击'
    }
    if (status === 502 || status === 503 || status === 504) {
      return '同步服务暂时不可用，请稍后重试，并前往同步日志确认结果'
    }
    // 服务端返回了结构化的同步结果
    const data = err.response.data as {
      message?: string
      data?: OrgSyncResponse | { request_id?: string; error_code?: string; error?: string }
    } | undefined
    if (data?.data && 'overall_status' in data.data && data.data.overall_status) {
      return formatSyncResult(data.data as OrgSyncResponse)
    }
    const requestID = getRequestID(data)
    const taskData = data?.data as { error_code?: string; error?: string } | undefined
    if (taskData?.error_code || taskData?.error) {
      return withRequestID(`同步失败：${safeSyncReason(taskData.error_code, taskData.error, '同步服务处理失败')}`, requestID)
    }
    // 未知响应不回显后端、第三方或网关原文，只保留状态码便于用户反馈。
    return status >= 500
      ? '同步失败，请前往同步日志查看详情'
      : `同步请求未能完成（HTTP ${status}）`
  }
  // 非 Axios 错误
  if (err instanceof Error && safeClientErrorMessages.has(err.message)) {
    return err.message
  }
  return '同步失败，请稍后重试'
}

/**
 * 将后端返回的同步结果格式化为用户可读消息。
 */
export function formatSyncResult(data: OrgSyncResponse): string {
  const deptOk = data.departments.status === 'success'
  const userOk = data.employees.status === 'success'
  const hrmSummary = formatHRMFieldSummary(data.employees)

  if (deptOk && userOk) {
    const base = `同步成功：${data.departments.success_count} 个部门、${data.employees.success_count} 名员工`
    return withRequestID(hrmSummary ? `${base}；${hrmSummary}` : base, data.request_id)
  }
  if (data.overall_status === 'failed') {
    if (!deptOk) {
      const reason = safeSyncReason(data.departments.error_code, data.departments.error, '部门数据未能同步')
      return withRequestID(`部门同步失败：${reason}`, data.request_id)
    }
    const reason = safeSyncReason(data.employees.error_code, data.employees.error, '员工数据未能同步')
    return withRequestID(`员工同步失败：${reason}`, data.request_id)
  }
  // 部分失败
  const parts: string[] = []
  if (deptOk) {
    parts.push(`部门同步成功（${data.departments.success_count} 个）`)
  } else {
    const reason = safeSyncReason(data.departments.error_code, data.departments.error, '原因未知')
    parts.push(reason ? `部门同步失败（${reason}）` : '部门同步失败')
  }
  if (userOk) {
    parts.push(`员工同步成功（${data.employees.success_count} 名）`)
    if (hrmSummary) parts.push(hrmSummary)
  } else if (data.employees.hrm_field_status === 'failed' && data.employees.fail_count === 0) {
    parts.push(`员工基础资料同步成功（${data.employees.success_count} 名）`)
    if (hrmSummary) parts.push(hrmSummary)
  } else if (data.employees.status === 'skipped') {
    parts.push('员工同步已跳过')
  } else {
    const outcome = data.employees.success_count > 0 ? '员工同步部分失败' : '员工同步失败'
    parts.push(`${outcome}（成功 ${data.employees.success_count}，失败 ${data.employees.fail_count}）`)
  }
  return withRequestID(`同步部分完成：${parts.join('；')}`, data.request_id)
}

export function formatSyncTaskResult(data: OrgSyncTaskResponse): string {
  const requestID = data.request_id
  if (data.status === 'success') {
    return withRequestID(`同步成功：${data.success_count}`, requestID)
  }
  if (data.status === 'partial_failed') {
    const reason = safeSyncReason(data.error_code, data.error, '存在部分失败项')
    return withRequestID(`同步部分完成：成功 ${data.success_count}，失败 ${data.fail_count}；${reason}`, requestID)
  }
  if (data.status === 'skipped') {
    return withRequestID(safeSyncReason(data.error_code, data.error, '同步已跳过'), requestID)
  }
  return withRequestID(`同步失败：${safeSyncReason(data.error_code, data.error, '同步服务处理失败')}`, requestID)
}

/**
 * 检查后端返回的同步结果，返回最终用户提示。
 * Axios 响应拦截器已返回后端 Response，OrgSyncResponse 位于 response.data。
 */
export function handleSyncResponse(response: OrgSyncAPIResponse): {
  success: boolean
  message: string
} {
  const result = response.data
  if (!result || !result.overall_status) {
    return { success: false, message: '同步响应格式异常，请前往同步日志确认结果' }
  }

  switch (result.overall_status) {
    case 'success':
      return {
        success: true,
        message: formatSyncResult(result),
      }
    case 'partial_failed':
      return {
        success: false,
        message: formatSyncResult(result),
      }
    case 'failed':
      return {
        success: false,
        message: formatSyncResult(result),
      }
    default:
      return { success: false, message: `同步完成但状态未知（${result.overall_status}），请前往同步日志确认` }
  }
}

/**
 * 统一组织花名册同步：权限门闩 + Modal.confirm + orgAPI.syncOrg。
 * 不改变同步 API / 权限码语义，仅统一前端交互。
 */
export function confirmOrgSync(options?: {
  title?: string
  content?: string
  successMessage?: string
  errorMessage?: string
  onStart?: () => void
  onSettled?: () => void
  onCompleted?: (result: OrgSyncResponse) => void | Promise<void>
  /** @deprecated 仅在全部成功时执行；需要刷新有效的部分成功结果时使用 onCompleted。 */
  onSuccess?: () => void | Promise<void>
}): void {
  if (!canSyncOrgData()) {
    message.warning(missingOrgSyncPermissionTip)
    return
  }

  Modal.confirm({
    title: options?.title || '同步组织数据',
    content:
      options?.content ||
      '将从钉钉重新拉取部门与员工数据并写入本系统，可能耗时较长。确认开始同步？',
    okText: '确认同步',
    cancelText: '取消',
    okButtonProps: { disabled: syncInProgress },
    onOk: async () => {
      if (syncInProgress) {
        message.warning('正在同步中，请勿重复提交')
        return
      }
      syncInProgress = true
      try {
        options?.onStart?.()
        const response = await orgAPI.syncOrg()
        const { success, message: msg } = handleSyncResponse(response)
        if (success) {
          message.success(options?.successMessage || msg)
        } else if (response.data.overall_status === 'partial_failed') {
          message.warning(options?.errorMessage || msg)
        } else {
          message.error(options?.errorMessage || msg)
        }
        if (response.data.overall_status === 'success' || response.data.overall_status === 'partial_failed') {
          try {
            await options?.onCompleted?.(response.data)
            if (success) {
              await options?.onSuccess?.()
            }
          } catch (refreshError) {
            logSyncError(refreshError)
            message.warning(success
              ? '同步成功，但页面数据刷新失败，请手动刷新'
              : '同步部分完成，但页面数据刷新失败，请手动刷新')
          }
        }
      } catch (err) {
        logSyncError(err)
        const msg = options?.errorMessage || resolveSyncErrorMessage(err)
        message.error(msg)
        // 不 reject Promise，避免 Modal 内部 catch 行为混乱
      } finally {
        syncInProgress = false
        options?.onSettled?.()
      }
    },
  })
}
