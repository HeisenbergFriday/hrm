import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, approvalAPI, type ApprovalSyncAPIResponse } from './api'

function terminal(status: 'success' | 'partial' | 'failed'): ApprovalSyncAPIResponse {
  return {
    code: 200,
    message: status,
    data: {
      status,
      processes: [],
      process_count: 1,
      succeeded_processes: status === 'failed' ? 0 : 1,
      failed_processes: status === 'success' ? 0 : 1,
      fetched_count: 1,
      fetch_fail_count: 0,
      success_count: status === 'failed' ? 0 : 1,
      fail_count: 0,
      start_date: '2026-08-01',
      end_date: '2026-08-05',
      sync_time: '2026-08-05T10:00:00+08:00',
      duration_ms: 10,
      request_id: 'approval-request-1',
    },
  }
}

describe('approvalAPI async sync', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    sessionStorage.clear()
  })

  it.each(['success', 'partial', 'failed'] as const)('polls running then returns %s', async (status) => {
    vi.useFakeTimers()
    vi.spyOn(api, 'post').mockResolvedValue({
      code: 202,
      message: 'running',
      data: { status: 'running', request_id: 'approval-request-1' },
    })
    const get = vi.spyOn(api, 'get')
      .mockResolvedValueOnce({ code: 202, message: 'running', data: { status: 'running', request_id: 'approval-request-1' } })
      .mockResolvedValueOnce(terminal(status))

    const pending = approvalAPI.sync({})
    await vi.advanceTimersByTimeAsync(4_000)

    await expect(pending).resolves.toEqual(terminal(status))
    expect(get).toHaveBeenCalledTimes(2)
    expect(get).toHaveBeenCalledWith('/approvals/sync/approval-request-1', expect.objectContaining({ timeout: 10_000 }))
  })

  it('reports an uncertain outcome when polling transport fails after start', async () => {
    vi.useFakeTimers()
    vi.spyOn(api, 'post').mockResolvedValue({
      code: 202,
      message: 'running',
      data: { status: 'running', request_id: 'approval-request-network' },
    })
    vi.spyOn(api, 'get').mockRejectedValue(new Error('network down'))

    const pending = approvalAPI.sync({})
    const assertion = expect(pending).rejects.toThrow(/后台任务可能仍在执行.*approval-request-network/)
    await vi.advanceTimersByTimeAsync(12_000)
    await assertion
  })

  it('retries a transient polling network failure and then succeeds', async () => {
    vi.useFakeTimers()
    vi.spyOn(api, 'post').mockResolvedValue({
      code: 202,
      message: 'running',
      data: { status: 'running', request_id: 'approval-request-retry' },
    })
    vi.spyOn(api, 'get')
      .mockRejectedValueOnce(new Error('temporary network failure'))
      .mockResolvedValueOnce(terminal('success'))

    const pending = approvalAPI.sync({})
    await vi.advanceTimersByTimeAsync(5_000)
    await expect(pending).resolves.toEqual(terminal('success'))
  })

  it('takes over the real running request returned by 409', async () => {
    vi.useFakeTimers()
    vi.spyOn(api, 'post').mockRejectedValue({
      response: { status: 409, data: { data: { status: 'running', request_id: 'existing-request' } } },
    })
    const get = vi.spyOn(api, 'get').mockResolvedValue(terminal('partial'))

    const pending = approvalAPI.sync({})
    await vi.advanceTimersByTimeAsync(2_000)
    await expect(pending).resolves.toEqual(terminal('partial'))
    expect(get).toHaveBeenCalledWith('/approvals/sync/existing-request', expect.anything())
  })

  it('reports an uncertain outcome when the start response is lost', async () => {
    vi.spyOn(api, 'post').mockRejectedValue(new Error('network down'))

    await expect(approvalAPI.sync({})).rejects.toThrow(/启动响应未确认.*可能已启动/)
  })
})
