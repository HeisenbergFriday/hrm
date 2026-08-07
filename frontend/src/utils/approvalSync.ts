import type { ApprovalSyncResult } from '../services/api'

export type ApprovalSyncNotice = {
  type: 'info' | 'success' | 'warning' | 'error'
  message: string
  description: string
}

export const missingApprovalSyncPermissionTip = '你缺少 approval:sync 权限，需要联系管理员添加'

export function approvalSyncRunningNotice(processCode?: string): ApprovalSyncNotice {
  return {
    type: 'info',
    message: '审批同步中',
    description: processCode ? '正在同步当前模板，请稍候。' : '正在发现并同步当前企业可访问的全部审批流程，请稍候。',
  }
}

export function approvalSyncResultNotice(result: ApprovalSyncResult): ApprovalSyncNotice {
  const fetchFailure = result.fetch_fail_count > 0 ? `，拉取失败 ${result.fetch_fail_count} 条` : ''
  const counts = `拉取 ${result.fetched_count} 条${fetchFailure}，写入 ${result.success_count} 条，写入失败 ${result.fail_count} 条。`
  if (result.status === 'success') {
    return { type: 'success', message: '审批同步全部成功', description: counts }
  }
  if (result.status === 'partial') {
    return {
      type: 'warning',
      message: '审批同步部分成功',
      description: `${counts} ${result.failed_processes} 个流程未完全成功。${result.discovery_error ? ` ${result.discovery_error}` : ''}`,
    }
  }
  return {
    type: 'error',
    message: '审批同步全部失败',
    description: result.discovery_error || result.processes.find((process) => process.error)?.error || '请检查企业流程配置、钉钉应用权限或稍后重试。',
  }
}

export function approvalSyncErrorNotice(error: unknown): ApprovalSyncNotice {
  const candidate = error as {
    message?: string
    response?: { data?: { message?: string; data?: { error_code?: string } } }
  }
  const payload = candidate?.response?.data
  if (payload?.data?.error_code === 'APPROVAL_PROCESS_CODES_MISSING') {
    return {
      type: 'error',
      message: '审批流程配置缺失',
      description: payload.message || '当前企业未配置可同步的审批流程代码。',
    }
  }
  const description = payload?.message || candidate?.message || '网络异常，后台任务可能仍在执行，请稍后刷新页面确认。'
  const uncertain = description.includes('可能仍在执行') || description.includes('等待超时')
  return {
    type: uncertain ? 'warning' : 'error',
    message: uncertain ? '同步状态暂时无法确认' : '审批同步启动失败',
    description,
  }
}
