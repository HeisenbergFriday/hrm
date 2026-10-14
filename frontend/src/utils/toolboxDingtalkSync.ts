import type { AttendanceToolboxRunResponse } from '../services/api'

export type ToolboxDingtalkSyncRequest = {
  start_date: string
  end_date: string
  flow_keys?: string[]
  max_instances?: number
  padding_days?: number
}

export type ToolboxDingtalkSyncJob = {
  request_id: string
  status: 'running' | 'success' | 'failed'
  message: string
  run_id?: string
}

type SyncDependencies = {
  start: (request: ToolboxDingtalkSyncRequest) => Promise<{ data: ToolboxDingtalkSyncJob }>
  getJob: (requestId: string) => Promise<{ data: ToolboxDingtalkSyncJob }>
  getRun: (runId: string) => Promise<{ data: AttendanceToolboxRunResponse }>
  scope: () => string
  pause?: () => Promise<void>
  now?: () => number
  timeoutMs?: number
}

function responseStatus(error: unknown): number | undefined {
  return (error as { response?: { status?: number } })?.response?.status
}

function startUnsupportedError(status: number): Error {
  const error = new Error(`服务器尚未部署异步钉钉拉取接口（HTTP ${status}），请先更新后端服务`)
  error.name = 'ToolboxDingtalkSyncStartUnsupported'
  return error
}

// Retain accepted tasks after transport failures so an explicit retry queries
// the same task instead of submitting another expensive DingTalk pull.
export function createToolboxDingtalkSyncClient(deps: SyncDependencies) {
  const pending = new Map<string, ToolboxDingtalkSyncJob>()
  const now = deps.now || Date.now
  const pause = deps.pause || (() => new Promise<void>((resolve) => setTimeout(resolve, 2000)))

  return async (request: ToolboxDingtalkSyncRequest): Promise<{ data: AttendanceToolboxRunResponse }> => {
    const scope = deps.scope()
    const key = JSON.stringify([scope, request])
    let job = pending.get(key)
    if (!job) {
      try {
        job = (await deps.start(request)).data
      } catch (error) {
        const status = responseStatus(error)
        // A page pull must never fall back to the old long request when the
        // async endpoint is missing: that path is what produces the gateway
        // timeout shown to users and can duplicate an already accepted pull.
        if (status === 404 || status === 405 || status === 501) throw startUnsupportedError(status)
        throw new Error('拉取任务受理状态未确认，请稍后重试；已有任务执行时系统会阻止重复启动')
      }
      if (!job?.request_id) throw new Error('拉取任务受理状态未确认，请稍后查询')
      pending.set(key, job)
    }

    const deadline = now() + (deps.timeoutMs ?? 11 * 60 * 1000)
    const assertScope = () => {
      if (deps.scope() !== scope) throw new Error('登录用户或组织已变更，请在当前组织重新拉取')
    }
    while (job.status === 'running') {
      assertScope()
      if (now() >= deadline) {
        throw new Error(`拉取任务仍在处理中，请稍后重试查询（任务编号：${job.request_id}）`)
      }
      await pause()
      assertScope()
      try {
        job = (await deps.getJob(job.request_id)).data
        pending.set(key, job)
      } catch (error) {
        if ([403, 404, 410].includes(responseStatus(error) || 0)) pending.delete(key)
        // An accepted task must never fall back to a synchronous pull, even on 404.
        throw new Error(`拉取任务状态查询失败，请稍后重试查询（任务编号：${job.request_id}）`)
      }
    }
    assertScope()
    if (job.status === 'failed') {
      pending.delete(key)
      throw new Error(job.message || '钉钉数据拉取失败，请稍后重试')
    }
    if (job.status !== 'success' || !job.run_id) throw new Error('拉取任务未返回有效结果，请稍后查询')
    try {
      const result = await deps.getRun(job.run_id)
      assertScope()
      pending.delete(key)
      return result
    } catch {
      throw new Error(`拉取已完成，但结果读取失败，请稍后重试查询（任务编号：${job.request_id}）`)
    }
  }
}
