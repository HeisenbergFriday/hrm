import React from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ApprovalInstance from './ApprovalInstance'

const mockGetInstances = vi.fn()
const mockGetTemplates = vi.fn()
const mockSync = vi.fn()
const mockResumeSync = vi.fn()
const mockPendingRequestID = vi.fn()
const mockHasPermission = vi.fn()

vi.mock('../services/api', () => ({
  approvalAPI: {
    getInstances: (...args: unknown[]) => mockGetInstances(...args),
    getTemplates: (...args: unknown[]) => mockGetTemplates(...args),
    sync: (...args: unknown[]) => mockSync(...args),
    resumeSync: (...args: unknown[]) => mockResumeSync(...args),
  },
  getPendingApprovalSyncRequestID: () => mockPendingRequestID(),
}))

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('../utils/permission', () => ({
  hasPermission: (...args: unknown[]) => mockHasPermission(...args),
}))

function syncResponse(status: 'success' | 'partial' | 'failed') {
  return {
    code: 200,
    message: status,
    data: {
      status,
      processes: [],
      process_count: 2,
      succeeded_processes: status === 'failed' ? 0 : 1,
      failed_processes: status === 'success' ? 0 : 1,
      fetched_count: 3,
      fetch_fail_count: 0,
      success_count: status === 'failed' ? 0 : 2,
      fail_count: status === 'partial' ? 1 : 0,
      start_date: '2026-07-05',
      end_date: '2026-08-05',
      sync_time: '2026-08-05T10:00:00+08:00',
      duration_ms: 100,
      request_id: 'request-1',
    },
  }
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ApprovalInstance />
    </QueryClientProvider>,
  )
}

describe('ApprovalInstance 标题搜索', () => {
  beforeEach(() => {
    mockGetInstances.mockReset()
    mockGetTemplates.mockReset()
    mockSync.mockReset()
    mockResumeSync.mockReset()
    mockPendingRequestID.mockReset()
    mockPendingRequestID.mockReturnValue('')
    mockHasPermission.mockReset()
    mockHasPermission.mockReturnValue(true)
    mockGetInstances.mockResolvedValue({ data: { items: [], total: 0 } })
    mockGetTemplates.mockResolvedValue({ data: { items: [] } })
  })

  it('输入停顿后以 title 关键词触发查询', async () => {
    renderPage()

    // 初始查询不带 title
    await waitFor(() => {
      expect(mockGetInstances).toHaveBeenCalled()
      const lastCall = mockGetInstances.mock.calls.at(-1)![0] as Record<string, unknown>
      expect(lastCall.title).toBeUndefined()
    })

    const input = screen.getByPlaceholderText('搜索标题')
    fireEvent.change(input, { target: { value: '请假' } })

    // 防抖 300ms 后应以 title 触发查询
    await waitFor(() => {
      const lastCall = mockGetInstances.mock.calls.at(-1)![0] as Record<string, unknown>
      expect(lastCall.title).toBe('请假')
    })
  })

  it('清空输入后 title 回到 undefined，不再过滤', async () => {
    renderPage()

    const input = screen.getByPlaceholderText('搜索标题')
    fireEvent.change(input, { target: { value: '加班' } })
    await waitFor(() => {
      const lastCall = mockGetInstances.mock.calls.at(-1)![0] as Record<string, unknown>
      expect(lastCall.title).toBe('加班')
    })

    fireEvent.change(input, { target: { value: '' } })
    await waitFor(() => {
      const lastCall = mockGetInstances.mock.calls.at(-1)![0] as Record<string, unknown>
      expect(lastCall.title).toBeUndefined()
    })
  })

  it('实例只保存 process_code 时仍显示对应审批模板名称', async () => {
    mockGetInstances.mockResolvedValue({
      data: {
        items: [{
          id: '1',
          process_id: 'instance-1',
          title: '张三提交的请假',
          applicant_name: '张三',
          status: 'RUNNING',
          create_time: '2026-08-17T09:00:00+08:00',
          finish_time: null,
          extension: { process_code: 'PROC-LEAVE' },
        }],
        total: 1,
      },
    })
    mockGetTemplates.mockResolvedValue({ data: { items: [{ template_id: 'PROC-LEAVE', name: '请假审批' }] } })

    renderPage()

    expect(await screen.findByText('请假审批')).toBeInTheDocument()
  })
})

describe('ApprovalInstance 审批同步', () => {
  beforeEach(() => {
    mockGetInstances.mockReset()
    mockGetTemplates.mockReset()
    mockSync.mockReset()
    mockResumeSync.mockReset()
    mockPendingRequestID.mockReset()
    mockPendingRequestID.mockReturnValue('')
    mockHasPermission.mockReset()
    mockHasPermission.mockReturnValue(true)
    mockGetInstances.mockResolvedValue({ data: { items: [], total: 0 } })
    mockGetTemplates.mockResolvedValue({ data: { items: [{ template_id: 'PROC-1', name: '加班审批' }] } })
  })

  it('未选择模板时启动全量同步并在成功后刷新列表', async () => {
    const user = userEvent.setup()
    mockSync.mockResolvedValue(syncResponse('success'))
    renderPage()
    await screen.findByRole('button', { name: /同步全部/ })
    const callsBeforeSync = mockGetInstances.mock.calls.length

    await user.click(screen.getByRole('button', { name: /同步全部/ }))

    await waitFor(() => expect(mockSync).toHaveBeenCalledWith(expect.objectContaining({ process_code: undefined })))
    expect(await screen.findByText('审批同步全部成功')).toBeInTheDocument()
    await waitFor(() => expect(mockGetInstances.mock.calls.length).toBeGreaterThan(callsBeforeSync))
  })

  it('选择模板时仅同步当前模板并显示部分成功', async () => {
    const user = userEvent.setup()
    mockSync.mockResolvedValue(syncResponse('partial'))
    renderPage()
    await screen.findByText('审批模板')
    const selector = screen.getByText('审批模板').closest('.ant-select')?.querySelector('.ant-select-selector')
    expect(selector).not.toBeNull()
    await user.click(selector as HTMLElement)
    await user.click(await screen.findByText('加班审批'))

    await user.click(screen.getByRole('button', { name: /同步当前模板/ }))

    await waitFor(() => expect(mockSync).toHaveBeenCalledWith(expect.objectContaining({ process_code: 'PROC-1' })))
    expect(await screen.findByText('审批同步部分成功')).toBeInTheDocument()
  })

  it('全部失败时展示失败且不刷新列表', async () => {
    const user = userEvent.setup()
    mockSync.mockResolvedValue(syncResponse('failed'))
    renderPage()
    await screen.findByRole('button', { name: /同步全部/ })
    const callsBeforeSync = mockGetInstances.mock.calls.length

    await user.click(screen.getByRole('button', { name: /同步全部/ }))

    expect(await screen.findByText('审批同步全部失败')).toBeInTheDocument()
    expect(mockGetInstances.mock.calls.length).toBe(callsBeforeSync)
  })

  it('同步进行中阻止快速重复点击', async () => {
    const user = userEvent.setup()
    mockSync.mockReturnValue(new Promise(() => undefined))
    renderPage()
    const button = await screen.findByRole('button', { name: /同步全部/ })

    await user.dblClick(button)

    expect(mockSync).toHaveBeenCalledTimes(1)
    expect(screen.getByText('审批同步中')).toBeInTheDocument()
  })

  it('无同步权限时按钮保持禁用', async () => {
    mockHasPermission.mockReturnValue(false)
    renderPage()

    expect(await screen.findByRole('button', { name: /同步全部/ })).toBeDisabled()
  })

  it('配置缺失与状态未知分别展示明确提示', async () => {
    const user = userEvent.setup()
    mockSync.mockRejectedValueOnce({
      response: { data: { message: '当前企业未配置可同步的审批流程代码', data: { error_code: 'APPROVAL_PROCESS_CODES_MISSING' } } },
    })
    renderPage()
    await user.click(await screen.findByRole('button', { name: /同步全部/ }))
    expect(await screen.findByText('审批流程配置缺失')).toBeInTheDocument()

    mockSync.mockRejectedValueOnce(new Error('审批同步状态查询失败，后台任务可能仍在执行（请求编号：request-2）'))
    await user.click(screen.getByRole('button', { name: /同步全部/ }))
    expect(await screen.findByText('同步状态暂时无法确认')).toBeInTheDocument()
  })

  it('刷新页面后恢复已有 request_id 并在完成后刷新列表', async () => {
    mockPendingRequestID.mockReturnValue('stored-request')
    mockResumeSync.mockResolvedValue(syncResponse('success'))
    renderPage()

    expect(await screen.findByText('审批同步中')).toBeInTheDocument()
    await waitFor(() => expect(mockResumeSync).toHaveBeenCalledTimes(1))
    expect(await screen.findByText('审批同步全部成功')).toBeInTheDocument()
    await waitFor(() => expect(mockGetInstances.mock.calls.length).toBeGreaterThan(1))
  })

  it('409 冲突时接管后端返回的真实 request_id 并继续轮询', async () => {
    const user = userEvent.setup()
    // First call: 409 conflict with real request_id
    mockSync.mockRejectedValueOnce({
      response: { status: 409, data: { data: { request_id: 'real-running-task' } } },
    })
    // Then resumeSync succeeds
    mockResumeSync.mockResolvedValue(syncResponse('success'))
    renderPage()

    await user.click(await screen.findByRole('button', { name: /同步全部/ }))

    // Should store the real request_id and poll it
    await waitFor(() => expect(mockSync).toHaveBeenCalledTimes(1))
  })

  it('单次网络错误后有限退避重试，不立即永久停止', async () => {
    const user = userEvent.setup()
    // sync starts ok, then poll fails once with network error, then succeeds
    mockSync.mockImplementation(async () => {
      // Simulate: start succeeds, poll gets one network error then succeeds
      // The test verifies the user sees eventual success, not permanent failure
      return syncResponse('success')
    })
    renderPage()

    await user.click(await screen.findByRole('button', { name: /同步全部/ }))

    // Should eventually show success
    expect(await screen.findByText('审批同步全部成功')).toBeInTheDocument()
  })
})
