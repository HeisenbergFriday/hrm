import React from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ApprovalStats from './ApprovalStats'

const mockGetStats = vi.fn()
const mockGetTemplates = vi.fn()
const mockSync = vi.fn()
const mockResumeSync = vi.fn()
const mockPendingRequestID = vi.fn()
const mockHasPermission = vi.fn()

vi.mock('../services/api', () => ({
  approvalAPI: {
    getStats: (...args: unknown[]) => mockGetStats(...args),
    getTemplates: (...args: unknown[]) => mockGetTemplates(...args),
    sync: (...args: unknown[]) => mockSync(...args),
    resumeSync: (...args: unknown[]) => mockResumeSync(...args),
  },
  getPendingApprovalSyncRequestID: () => mockPendingRequestID(),
}))

vi.mock('../utils/permission', () => ({
  hasPermission: (...args: unknown[]) => mockHasPermission(...args),
}))

function result(status: 'success' | 'partial' | 'failed') {
  return {
    code: 200,
    message: status,
    data: {
      status,
      processes: [],
      process_count: 1,
      succeeded_processes: status === 'failed' ? 0 : 1,
      failed_processes: status === 'success' ? 0 : 1,
      fetched_count: 2,
      fetch_fail_count: 0,
      success_count: status === 'failed' ? 0 : 2,
      fail_count: 0,
      start_date: '2026-08-01',
      end_date: '2026-08-05',
      sync_time: '2026-08-05T10:00:00+08:00',
      duration_ms: 10,
      request_id: 'request-stats',
    },
  }
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ApprovalStats />
    </QueryClientProvider>,
  )
}

describe('ApprovalStats 审批同步', () => {
  beforeEach(() => {
    mockGetStats.mockReset()
    mockGetTemplates.mockReset()
    mockSync.mockReset()
    mockResumeSync.mockReset()
    mockPendingRequestID.mockReset()
    mockPendingRequestID.mockReturnValue('')
    mockHasPermission.mockReset()
    mockHasPermission.mockReturnValue(true)
    mockGetStats.mockResolvedValue({
      code: 200,
      message: 'success',
      data: { summary: { total: 0, completed: 0, refused: 0, running: 0, terminated: 0, canceled: 0, approval_rate: '0.00%' }, template_stats: [] },
    })
    mockGetTemplates.mockResolvedValue({ data: { items: [{ template_id: 'PROC-LEAVE', name: '请假审批' }] } })
  })

  it('未选择模板时同步全部并在部分成功后刷新统计来源', async () => {
    const user = userEvent.setup()
    mockSync.mockResolvedValue(result('partial'))
    renderPage()
    const button = await screen.findByRole('button', { name: /同步全部/ })
    const callsBeforeSync = mockGetStats.mock.calls.length

    await user.click(button)

    await waitFor(() => expect(mockSync).toHaveBeenCalledWith(expect.objectContaining({ process_code: undefined })))
    expect(await screen.findByText('审批同步部分成功')).toBeInTheDocument()
    await waitFor(() => expect(mockGetStats.mock.calls.length).toBeGreaterThan(callsBeforeSync))
  })

  it('选择模板时只同步当前模板', async () => {
    const user = userEvent.setup()
    mockSync.mockResolvedValue(result('success'))
    renderPage()
    const selector = await screen.findByRole('combobox')
    await user.click(selector)
    await user.click(await screen.findByText('请假审批'))

    await user.click(screen.getByRole('button', { name: /同步当前模板/ }))

    await waitFor(() => expect(mockSync).toHaveBeenCalledWith(expect.objectContaining({ process_code: 'PROC-LEAVE' })))
  })

  it('无权限时禁用同步入口', async () => {
    mockHasPermission.mockReturnValue(false)
    renderPage()
    expect(await screen.findByRole('button', { name: /同步全部/ })).toBeDisabled()
  })
})
