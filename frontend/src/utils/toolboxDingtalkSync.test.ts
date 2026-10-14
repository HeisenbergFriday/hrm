import { describe, expect, it, vi } from 'vitest'
import { createToolboxDingtalkSyncClient, type ToolboxDingtalkSyncJob } from './toolboxDingtalkSync'

const request = { start_date: '2026-08-01', end_date: '2026-09-30', flow_keys: ['leave'], padding_days: 31 }
const running: ToolboxDingtalkSyncJob = { request_id: 'job-1', status: 'running', message: '正在拉取' }
const success: ToolboxDingtalkSyncJob = { ...running, status: 'success', run_id: 'run-1' }
const run = { run_id: 'run-1', module: 'dingtalk_sync', files: [], expires_at: '2099-01-01' }

function fixture() {
  let clock = 0
  let scope = 'org-a:user-a'
  const deps = {
    start: vi.fn().mockResolvedValue({ data: running }),
    getJob: vi.fn().mockResolvedValue({ data: success }),
    getRun: vi.fn().mockResolvedValue({ data: run }),
    scope: () => scope,
    now: () => clock,
    pause: vi.fn(async () => { clock += 2000 }),
    timeoutMs: 660000,
  }
  return { deps, client: createToolboxDingtalkSyncClient(deps), changeScope: () => { scope = 'org-b:user-a' }, clock: () => clock }
}

describe('toolbox dingtalk async sync', () => {
  it('handles a pull longer than the gateway deadline with one start and short queries', async () => {
    const { deps, client, clock } = fixture()
    for (let i = 0; i < 35; i++) deps.getJob.mockResolvedValueOnce({ data: running })
    await expect(client(request)).resolves.toEqual({ data: run })
    expect(clock()).toBeGreaterThan(60000)
    expect(deps.start).toHaveBeenCalledExactlyOnceWith(request)
    expect(deps.getJob).toHaveBeenCalledTimes(36)
    expect(deps.getRun).toHaveBeenCalledExactlyOnceWith('run-1')
  })

  it('retains accepted task on query failure and resumes without another pull', async () => {
    const { deps, client } = fixture()
    deps.getJob.mockRejectedValueOnce({ response: { status: 504 } })
    await expect(client(request)).rejects.toThrow('状态查询失败')
    await expect(client(request)).resolves.toEqual({ data: run })
    expect(deps.start).toHaveBeenCalledTimes(1)
  })

  it('does not expose 404 from a query as an unsupported start for legacy fallback', async () => {
    const { deps, client } = fixture()
    deps.getJob.mockRejectedValueOnce({ response: { status: 404 } })
    try { await client(request); expect.fail('expected query error') } catch (error) {
      expect(error).toBeInstanceOf(Error)
      expect(error).not.toHaveProperty('response')
    }
    expect(deps.getRun).not.toHaveBeenCalled()
  })

  it('keeps failed jobs out of the result download path', async () => {
    const { deps, client } = fixture()
    deps.getJob.mockResolvedValue({ data: { ...running, status: 'failed', message: '钉钉流程未配置' } })
    await expect(client(request)).rejects.toThrow('钉钉流程未配置')
    expect(deps.getRun).not.toHaveBeenCalled()
  })

  it('reports an unsupported start endpoint without triggering a legacy pull', async () => {
    const { deps, client } = fixture()
    deps.start.mockRejectedValue({ response: { status: 404 } })
    await expect(client(request)).rejects.toThrow('服务器尚未部署异步钉钉拉取接口')
    expect(deps.getJob).not.toHaveBeenCalled()
  })

  it('labels start transport failures as unconfirmed without automatic resubmission', async () => {
    const { deps, client } = fixture()
    deps.start.mockRejectedValue({ response: { status: 504, data: '<html>gateway</html>' } })
    await expect(client(request)).rejects.toThrow('受理状态未确认')
    expect(deps.start).toHaveBeenCalledTimes(1)
    expect(deps.getJob).not.toHaveBeenCalled()
  })

  it('continues reading the completed result after a read failure instead of re-pulling', async () => {
    const { deps, client } = fixture()
    deps.getRun.mockRejectedValueOnce({ response: { status: 404 } })
    await expect(client(request)).rejects.toThrow('结果读取失败')
    await expect(client(request)).resolves.toEqual({ data: run })
    expect(deps.start).toHaveBeenCalledTimes(1)
    expect(deps.getJob).toHaveBeenCalledTimes(1)
  })

  it('stops at the polling deadline and retains the task for a later query', async () => {
    const { deps, client } = fixture()
    deps.getJob.mockResolvedValue({ data: running })
    await expect(client(request)).rejects.toThrow('仍在处理中')
    deps.getJob.mockResolvedValue({ data: success })
    await expect(client(request)).resolves.toEqual({ data: run })
    expect(deps.start).toHaveBeenCalledTimes(1)
  })

  it('stops polling after changing organization without returning stale results', async () => {
    const { deps, client, changeScope } = fixture()
    deps.pause.mockImplementation(async () => { changeScope() })
    await expect(client(request)).rejects.toThrow('登录用户或组织已变更')
    expect(deps.getJob).not.toHaveBeenCalled()
    expect(deps.getRun).not.toHaveBeenCalled()
  })
})
