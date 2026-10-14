import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, attendanceToolboxAPI } from './api'

describe('attendanceToolboxAPI page sync contract', () => {
  afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })

  it('starts a JSON job, polls, and returns the structured run with short request timeouts', async () => {
    vi.useFakeTimers()
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      code: 202, data: { request_id: 'toolbox-job', status: 'running' },
    })
    const result = { code: 200, data: { run_id: 'toolbox-run', module: 'dingtalk_sync', files: [] } }
    const get = vi.spyOn(api, 'get')
      .mockResolvedValueOnce({ code: 200, data: { request_id: 'toolbox-job', status: 'running' } })
      .mockResolvedValueOnce({ code: 200, data: { request_id: 'toolbox-job', status: 'success', run_id: 'toolbox-run' } })
      .mockResolvedValueOnce(result)
    const request = { start_date: '2026-08-01', end_date: '2026-09-30', flow_keys: ['leave'], padding_days: 31 }
    const pending = attendanceToolboxAPI.runDingtalkSyncStructured(request)
    await vi.advanceTimersByTimeAsync(4000)
    await expect(pending).resolves.toEqual(result)
    expect(post).toHaveBeenCalledExactlyOnceWith('/attendance/toolbox/dingtalk-sync/start', request)
    expect(get.mock.calls).toEqual([
      ['/attendance/toolbox/dingtalk-sync/jobs/toolbox-job'],
      ['/attendance/toolbox/dingtalk-sync/jobs/toolbox-job'],
      ['/attendance/toolbox/runs/toolbox-run'],
    ])
    expect(api.defaults.timeout).toBe(10000)
  })

  it('does not turn a missing async start route into a legacy gateway timeout', async () => {
    const post = vi.spyOn(api, 'post').mockRejectedValue({ response: { status: 404 } })
    await expect(attendanceToolboxAPI.runDingtalkSyncStructured({
      start_date: '2026-08-01', end_date: '2026-09-30', flow_keys: ['leave'],
    })).rejects.toThrow('服务器尚未部署异步钉钉拉取接口')
    expect(post).toHaveBeenCalledExactlyOnceWith('/attendance/toolbox/dingtalk-sync/start', expect.anything())
  })
})
