import { beforeEach, describe, expect, it, vi } from 'vitest'
import { message } from 'antd'
import { attendanceAPI, orgAPI, syncAPI, type OrgSyncTaskAPIResponse } from '../services/api'
import { completeJobRun, runRealJob, type Job } from './SyncJobs'

function job(overrides: Partial<Job>): Job {
  return {
    id: '1',
    name: '同步用户数据',
    description: '',
    type: 'sync_users',
    status: 'idle',
    last_run_time: '',
    next_run_time: '',
    ...overrides,
  }
}

function taskResponse(status: 'success' | 'partial_failed' = 'success'): OrgSyncTaskAPIResponse {
  return {
    code: status === 'success' ? 200 : 207,
    message: status,
    data: {
      status,
      success_count: status === 'success' ? 10 : 8,
      fail_count: status === 'success' ? 0 : 2,
      error: status === 'success' ? '' : '同步部分完成，请前往同步日志查看失败项',
      error_code: status === 'success' ? undefined : 'DINGTALK_PERMISSION_DENIED',
      request_id: 'sync-jobs-test',
      duration_ms: 10,
    },
  }
}

beforeEach(() => {
  vi.restoreAllMocks()
})

describe('SyncJobs real task routing', () => {
  it('routes user and department tasks to their dedicated APIs', async () => {
    const userSpy = vi.spyOn(syncAPI, 'syncUsers').mockResolvedValue(taskResponse())
    const departmentSpy = vi.spyOn(syncAPI, 'syncDepartments').mockResolvedValue(taskResponse())

    await runRealJob(job({ id: '1', type: 'sync_users' }))
    await runRealJob(job({ id: '2', type: 'sync_departments' }))

    expect(userSpy).toHaveBeenCalledTimes(1)
    expect(departmentSpy).toHaveBeenCalledTimes(1)
  })

  it('rejects unsupported tasks without falling back to full organization sync', async () => {
    const userSpy = vi.spyOn(syncAPI, 'syncUsers').mockResolvedValue(taskResponse())
    const departmentSpy = vi.spyOn(syncAPI, 'syncDepartments').mockResolvedValue(taskResponse())
    const attendanceSpy = vi.spyOn(attendanceAPI, 'sync').mockResolvedValue({} as never)
    const fullOrgSpy = vi.spyOn(orgAPI, 'syncOrg').mockResolvedValue({} as never)

    await expect(runRealJob(job({ id: '1', type: 'unknown_task' })))
      .rejects.toThrow('不支持的同步任务类型')

    expect(userSpy).not.toHaveBeenCalled()
    expect(departmentSpy).not.toHaveBeenCalled()
    expect(attendanceSpy).not.toHaveBeenCalled()
    expect(fullOrgSpy).not.toHaveBeenCalled()
  })

  it('refreshes after a valid HTTP 207 partial result', async () => {
    const warningSpy = vi.spyOn(message, 'warning').mockImplementation(() => undefined as never)
    const onCompleted = vi.fn()

    await completeJobRun({ kind: 'org_sync', response: taskResponse('partial_failed') }, onCompleted)

    expect(onCompleted).toHaveBeenCalledTimes(1)
    expect(warningSpy).toHaveBeenCalledWith(expect.stringContaining('钉钉通讯录权限不足'))
    expect(warningSpy).toHaveBeenCalledWith(expect.stringContaining('请求编号：sync-jobs-test'))
  })
})
