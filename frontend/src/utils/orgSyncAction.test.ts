import { beforeEach, describe, expect, it, vi } from 'vitest'
import axios from 'axios'
import { Modal, message } from 'antd'
import {
  ORG_SYNC_TIMEOUT_MS,
  ORG_SYNC_POLL_INTERVAL_MS,
  ORG_SYNC_REQUEST_CONFIG,
  ORG_SYNC_SHORT_REQUEST_CONFIG,
  ORG_SYNC_SHORT_REQUEST_TIMEOUT_MS,
  api,
  orgAPI,
  syncAPI,
  type OrgSyncAPIResponse,
  type OrgSyncResponse,
} from '../services/api'
import {
  confirmOrgSync,
  formatSyncResult,
  handleSyncResponse,
  resolveSyncErrorMessage,
} from './orgSyncAction'

vi.mock('./permission', () => ({ hasPermission: () => true }))

function syncResult(overrides?: Partial<OrgSyncResponse>): OrgSyncResponse {
  return {
    overall_status: 'success',
    departments: { status: 'success', success_count: 19, fail_count: 0, error: '' },
    employees: {
      status: 'success',
      success_count: 525,
      fail_count: 0,
      error: '',
      position_missing_count: 0,
      overwrite_empty: false,
      default_role_assigned_count: 0,
    },
    sync_time: '2026-07-27T10:00:00Z',
    duration_ms: 60_000,
    request_id: 'request-123',
    ...overrides,
  }
}

function apiResponse(data = syncResult()): OrgSyncAPIResponse {
  const code = data.overall_status === 'success' ? 200 : data.overall_status === 'partial_failed' ? 207 : 500
  return { code, message: data.overall_status, data }
}

const runningResponse = () => ({
  code: 202 as const,
  message: 'running' as const,
  data: { status: 'running' as const, request_id: 'request-123' },
})

function axiosHTTPError(status: number, data: unknown) {
  return new axios.AxiosError('request failed', 'ERR_BAD_RESPONSE', undefined, undefined, {
    status,
    statusText: 'Error',
    data,
    headers: {},
    config: {} as never,
  })
}

function captureConfirm() {
  const configs: Array<Parameters<typeof Modal.confirm>[0]> = []
  vi.spyOn(Modal, 'confirm').mockImplementation((config) => {
    configs.push(config)
    return { destroy: vi.fn(), update: vi.fn() }
  })
  return configs
}

function runConfirm(config: Parameters<typeof Modal.confirm>[0]): Promise<unknown> {
  return Promise.resolve((config.onOk as (() => unknown) | undefined)?.())
}

beforeEach(() => {
  vi.restoreAllMocks()
})

describe('organization sync request', () => {
  it('uses an independent ten minute timeout', () => {
    expect(ORG_SYNC_TIMEOUT_MS).toBe(600_000)
    expect(ORG_SYNC_REQUEST_CONFIG).toEqual({ timeout: 600_000 })
    expect(ORG_SYNC_SHORT_REQUEST_TIMEOUT_MS).toBe(10_000)
    expect(ORG_SYNC_SHORT_REQUEST_CONFIG).toEqual({ timeout: 10_000 })
  })

  it('starts full sync with a short request and polls its result', async () => {
    vi.useFakeTimers()
    try {
      const postSpy = vi.spyOn(api, 'post')
        .mockResolvedValueOnce(apiResponse())
        .mockResolvedValueOnce(apiResponse())
        .mockResolvedValueOnce(runningResponse())
      const getSpy = vi.spyOn(api, 'get').mockResolvedValue(apiResponse())
      await syncAPI.syncUsers()
      await syncAPI.syncDepartments()
      const syncPromise = orgAPI.syncOrg()
      await vi.advanceTimersByTimeAsync(ORG_SYNC_POLL_INTERVAL_MS)
      await expect(syncPromise).resolves.toEqual(apiResponse())
      expect(postSpy).toHaveBeenNthCalledWith(1, '/sync/users', {}, ORG_SYNC_REQUEST_CONFIG)
      expect(postSpy).toHaveBeenNthCalledWith(2, '/sync/departments', {}, ORG_SYNC_REQUEST_CONFIG)
      expect(postSpy).toHaveBeenNthCalledWith(3, '/org/sync/start', {}, ORG_SYNC_SHORT_REQUEST_CONFIG)
      expect(getSpy).toHaveBeenCalledWith('/org/sync/request-123', ORG_SYNC_SHORT_REQUEST_CONFIG)
    } finally {
      vi.useRealTimers()
    }
  })

  it('keeps polling through multiple running responses before success', async () => {
    vi.useFakeTimers()
    try {
      vi.spyOn(api, 'post').mockResolvedValue(runningResponse())
      const getSpy = vi.spyOn(api, 'get')
        .mockResolvedValueOnce(runningResponse())
        .mockResolvedValueOnce(runningResponse())
        .mockResolvedValueOnce(apiResponse())

      const syncPromise = orgAPI.syncOrg()
      await vi.advanceTimersByTimeAsync(ORG_SYNC_POLL_INTERVAL_MS * 3)

      await expect(syncPromise).resolves.toEqual(apiResponse())
      expect(getSpy).toHaveBeenCalledTimes(3)
    } finally {
      vi.useRealTimers()
    }
  })

  it('stops with a safe service-restart hint when the status query returns 404', async () => {
    vi.useFakeTimers()
    try {
      vi.spyOn(api, 'post').mockResolvedValue(runningResponse())
      const getSpy = vi.spyOn(api, 'get').mockRejectedValue(
        axiosHTTPError(404, { message: 'task not found after process restart' }),
      )

      const syncError = orgAPI.syncOrg().catch((error) => error as unknown)
      await vi.advanceTimersByTimeAsync(ORG_SYNC_POLL_INTERVAL_MS)
      const error = await syncError

      expect(getSpy).toHaveBeenCalledTimes(1)
      expect(resolveSyncErrorMessage(error))
        .toBe('同步任务不存在或服务可能已重启，请前往同步日志确认结果，暂勿重复点击')
    } finally {
      vi.useRealTimers()
    }
  })

  it('stops with a safe message when the status query loses the network', async () => {
    vi.useFakeTimers()
    try {
      vi.spyOn(api, 'post').mockResolvedValue(runningResponse())
      const getSpy = vi.spyOn(api, 'get').mockRejectedValue(
        new axios.AxiosError('Network Error', 'ERR_NETWORK'),
      )

      const syncError = orgAPI.syncOrg().catch((error) => error as unknown)
      await vi.advanceTimersByTimeAsync(ORG_SYNC_POLL_INTERVAL_MS)
      const error = await syncError

      expect(getSpy).toHaveBeenCalledTimes(1)
      expect(resolveSyncErrorMessage(error)).toBe('网络异常，请检查网络连接后重试')
    } finally {
      vi.useRealTimers()
    }
  })

  it('stops polling at the page wait limit without cancelling the background task', async () => {
    vi.useFakeTimers()
    try {
      vi.spyOn(api, 'post').mockResolvedValue(runningResponse())
      const getSpy = vi.spyOn(api, 'get').mockResolvedValue(runningResponse())

      const syncPromise = orgAPI.syncOrg()
      const rejection = expect(syncPromise).rejects.toThrow(
        '组织同步页面等待超时，后台任务可能仍在执行，请前往同步日志确认结果，暂勿重复点击',
      )
      await vi.advanceTimersByTimeAsync(ORG_SYNC_TIMEOUT_MS)

      await rejection
      expect(getSpy).toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('handles a successful response from the Axios interceptor shape', () => {
    const result = handleSyncResponse(apiResponse())
    expect(result.success).toBe(true)
    expect(result.message).toContain('19 个部门')
    expect(result.message).toContain('525 名员工')
  })

  it('reports a department failure with a safe error code, skipped employees, and request id', () => {
    const data = syncResult({
      overall_status: 'failed',
      departments: {
        status: 'failed',
        success_count: 0,
        fail_count: 1,
        error: 'internal database detail that must not be shown',
        error_code: 'DINGTALK_PERMISSION_DENIED',
      },
      employees: {
        status: 'skipped',
        success_count: 0,
        fail_count: 0,
        error: '部门同步未完成，已跳过员工同步',
        error_code: 'EMPLOYEE_SYNC_SKIPPED',
        position_missing_count: 0,
        overwrite_empty: false,
        default_role_assigned_count: 0,
      },
    })
    const output = formatSyncResult(data)
    expect(output).toContain('部门同步失败：钉钉通讯录权限不足')
    expect(output).toContain('请求编号：request-123')
    expect(output).not.toContain('internal database detail')
    expect(handleSyncResponse(apiResponse(data)).success).toBe(false)
  })

  it('reports an employee partial failure with counts', () => {
    const data = syncResult({
      overall_status: 'partial_failed',
      employees: {
        status: 'partial_failed',
        success_count: 500,
        fail_count: 25,
        error: '员工同步失败，请前往同步日志查看或联系管理员',
        position_missing_count: 0,
        overwrite_empty: false,
        default_role_assigned_count: 0,
      },
    })
    const result = handleSyncResponse(apiResponse(data))
    expect(result.success).toBe(false)
    expect(result.message).toContain('员工同步部分失败')
    expect(result.message).toContain('成功 500，失败 25')
  })

  it('reads a structured non-2xx failure without exposing internal errors', () => {
    const data = syncResult({
      overall_status: 'failed',
      departments: { status: 'failed', success_count: 0, fail_count: 1, error: '数据库 password=secret' },
      employees: {
        status: 'failed',
        success_count: 0,
        fail_count: 1,
        error: '员工同步失败，请前往同步日志查看或联系管理员',
        position_missing_count: 0,
        overwrite_empty: false,
        default_role_assigned_count: 0,
      },
    })
    const output = resolveSyncErrorMessage(axiosHTTPError(500, apiResponse(data)))
    expect(output).toContain('同步失败')
    expect(output).not.toContain('password')
    expect(output).not.toContain('secret')
  })

  it('distinguishes timeout, network, authorization, conflict, missing task, and gateway failures', () => {
    const timeout = new axios.AxiosError('timeout of 600000ms exceeded', 'ECONNABORTED')
    const network = new axios.AxiosError('Network Error', 'ERR_NETWORK')
    expect(resolveSyncErrorMessage(timeout)).toBe(
      '请求等待超时，同步可能仍在后台执行，请前往同步日志查看，暂勿重复点击',
    )
    expect(resolveSyncErrorMessage(network)).toContain('网络异常')
    expect(resolveSyncErrorMessage(axiosHTTPError(401, {}))).toContain('登录已过期')
    expect(resolveSyncErrorMessage(axiosHTTPError(403, {}))).toContain('权限不足')
    expect(resolveSyncErrorMessage(axiosHTTPError(409, {}))).toContain('正在同步中')
    expect(resolveSyncErrorMessage(axiosHTTPError(404, { message: 'task not found after restart' })))
      .toBe('同步任务不存在或服务可能已重启，请前往同步日志确认结果，暂勿重复点击')
    expect(resolveSyncErrorMessage(axiosHTTPError(504, '<html>nginx gateway timeout</html>')))
      .toBe('同步服务暂时不可用，请稍后重试，并前往同步日志确认结果')
    expect(resolveSyncErrorMessage(axiosHTTPError(500, { message: 'password=secret SQL SELECT * FROM users' })))
      .toBe('同步失败，请前往同步日志查看详情')
    expect(resolveSyncErrorMessage(new Error('<html>nginx third-party raw error</html>')))
      .toBe('同步失败，请稍后重试')
  })
})

describe('confirmOrgSync', () => {
  it('prevents a duplicate submission while the first request is running', async () => {
    const configs = captureConfirm()
    let resolveFirst!: (value: OrgSyncAPIResponse) => void
    const firstRequest = new Promise<OrgSyncAPIResponse>((resolve) => { resolveFirst = resolve })
    const syncSpy = vi.spyOn(orgAPI, 'syncOrg').mockReturnValue(firstRequest)
    const warningSpy = vi.spyOn(message, 'warning').mockImplementation(() => undefined as never)

    confirmOrgSync()
    const firstRun = runConfirm(configs[0])
    confirmOrgSync()
    await runConfirm(configs[1])

    expect(syncSpy).toHaveBeenCalledTimes(1)
    expect(warningSpy).toHaveBeenCalledWith('正在同步中，请勿重复提交')

    resolveFirst(apiResponse())
    await firstRun
  })

  it('resolves the modal action after a timeout and shows the explicit warning', async () => {
    const configs = captureConfirm()
    vi.spyOn(orgAPI, 'syncOrg').mockRejectedValue(
      new axios.AxiosError('timeout of 600000ms exceeded', 'ECONNABORTED'),
    )
    const errorSpy = vi.spyOn(message, 'error').mockImplementation(() => undefined as never)

    confirmOrgSync()
    await expect(runConfirm(configs[0])).resolves.toBeUndefined()
    expect(errorSpy).toHaveBeenCalledWith(
      '请求等待超时，同步可能仍在后台执行，请前往同步日志查看，暂勿重复点击',
    )
  })

  it('runs onCompleted after a valid HTTP 207 partial result', async () => {
    const configs = captureConfirm()
    const partial = syncResult({
      overall_status: 'partial_failed',
      employees: {
        status: 'partial_failed',
        success_count: 500,
        fail_count: 25,
        error: '员工同步失败，请前往同步日志查看或联系管理员',
        position_missing_count: 0,
        overwrite_empty: false,
        default_role_assigned_count: 0,
      },
    })
    vi.spyOn(orgAPI, 'syncOrg').mockResolvedValue(apiResponse(partial))
    vi.spyOn(message, 'warning').mockImplementation(() => undefined as never)
    const onCompleted = vi.fn()
    const onSuccess = vi.fn()

    confirmOrgSync({ onCompleted, onSuccess })
    await runConfirm(configs[0])

    expect(onCompleted).toHaveBeenCalledWith(partial)
    expect(onSuccess).not.toHaveBeenCalled()
  })

  it('shows a failed result and does not refresh after HTTP 500', async () => {
    const configs = captureConfirm()
    const failed = syncResult({
      overall_status: 'failed',
      departments: {
        status: 'failed',
        success_count: 0,
        fail_count: 1,
        error: 'third-party raw error',
        error_code: 'DINGTALK_NETWORK_FAILED',
      },
      employees: {
        status: 'skipped',
        success_count: 0,
        fail_count: 0,
        error: '',
        position_missing_count: 0,
        overwrite_empty: false,
        default_role_assigned_count: 0,
      },
    })
    vi.spyOn(orgAPI, 'syncOrg').mockRejectedValue(axiosHTTPError(500, apiResponse(failed)))
    const errorSpy = vi.spyOn(message, 'error').mockImplementation(() => undefined as never)
    const onCompleted = vi.fn()

    confirmOrgSync({ onCompleted })
    await runConfirm(configs[0])

    expect(errorSpy).toHaveBeenCalledWith(expect.stringContaining('部门同步失败：连接钉钉服务失败'))
    expect(errorSpy.mock.calls[0][0]).not.toContain('third-party raw error')
    expect(onCompleted).not.toHaveBeenCalled()
  })

  it('keeps the completed result when the page refresh callback fails', async () => {
    const configs = captureConfirm()
    vi.spyOn(orgAPI, 'syncOrg').mockResolvedValue(apiResponse())
    vi.spyOn(message, 'success').mockImplementation(() => undefined as never)
    const warningSpy = vi.spyOn(message, 'warning').mockImplementation(() => undefined as never)
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const onCompleted = vi.fn().mockRejectedValue(new Error('refresh failed'))

    confirmOrgSync({ onCompleted })
    await runConfirm(configs[0])

    expect(onCompleted).toHaveBeenCalledWith(apiResponse().data)
    expect(warningSpy).toHaveBeenCalledWith('同步成功，但页面数据刷新失败，请手动刷新')
  })
})
