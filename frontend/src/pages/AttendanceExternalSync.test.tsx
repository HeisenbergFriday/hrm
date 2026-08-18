import React from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AttendanceExternalSync, { externalSyncJobPollingInterval } from './AttendanceExternalSync'

const mockGetStatus = vi.fn()
const mockGetJobs = vi.fn()
const mockGetJob = vi.fn()
const mockRun = vi.fn()

vi.mock('../services/api', () => ({
  attendanceAPI: {
    externalSync: {
      getStatus: (...args: unknown[]) => mockGetStatus(...args),
      getJobs: (...args: unknown[]) => mockGetJobs(...args),
      getJob: (...args: unknown[]) => mockGetJob(...args),
      run: (...args: unknown[]) => mockRun(...args),
    },
  },
}))

vi.mock('../store/authStore', () => ({
  useAuthStore: (selector: (state: { orgId: string }) => unknown) => selector({ orgId: 'xiaotie' }),
}))

vi.mock('../utils/permission', () => ({
  hasPermission: () => true,
  hasMenuPermission: () => true,
}))

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <AttendanceExternalSync />
    </QueryClientProvider>,
  )
}

describe('AttendanceExternalSync', () => {
  beforeEach(() => {
    mockGetStatus.mockReset()
    mockGetJobs.mockReset()
    mockGetJob.mockReset()
    mockRun.mockReset()
    mockGetStatus.mockResolvedValue({
      data: {
        org_id: 'xiaotie',
        org_name: '测试企业',
        enabled: true,
        source_healthy: true,
        source_latency_ms: 12,
        external_last_attendance_update: '2026-07-16T09:34:41+08:00',
        external_last_department_update: '2026-07-16T09:00:06+08:00',
        cursors: [],
        last_job: null,
      },
    })
    mockGetJobs.mockResolvedValue({ data: { list: [], total: 0 } })
    mockGetJob.mockResolvedValue({ data: { status: 'success', id: 1 } })
    mockRun.mockResolvedValue({ data: { status: 'success' } })
  })

  it('shows source health and confirms an incremental sync', async () => {
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText(/健康 · 12ms/)).toBeInTheDocument()
    expect(screen.getByText('暂无同步任务')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /立即同步/ }))
    expect((await screen.findAllByText('确认同步外部考勤数据？')).length).toBeGreaterThan(0)
    expect(screen.getByText(/不会执行部门关系失活/)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '确认同步' }))
    await waitFor(() => {
      expect(mockRun).toHaveBeenCalledWith({
        source: 'all',
        full_department_snapshot: false,
      })
    })
  })

  it('disables immediate sync while an existing task is running', async () => {
    const user = userEvent.setup()
    mockGetStatus.mockResolvedValue({
      data: {
        org_id: 'xiaotie',
        org_name: '测试企业',
        enabled: true,
        source_healthy: true,
        active_job: { id: 853, status: 'running' },
        last_job: { id: 853, status: 'running' },
      },
    })
    mockGetJobs.mockResolvedValue({ data: { list: [{ id: 853, status: 'running' }], total: 1 } })
    renderPage()

    const runButton = await screen.findByRole('button', { name: /立即同步/ })
    await waitFor(() => expect(runButton).toBeDisabled())
    await user.hover(runButton)
    expect(await screen.findByText('同步任务 #853 正在运行，请等待任务完成')).toBeInTheDocument()
    expect(mockRun).not.toHaveBeenCalled()
  })

  it('stops polling after a terminal result and refreshes the page state', async () => {
    mockGetStatus
      .mockResolvedValueOnce({
        data: {
          org_id: 'xiaotie',
          org_name: '测试企业',
          enabled: true,
          source_healthy: true,
          active_job: { id: 854, status: 'running' },
          last_job: { id: 854, status: 'running' },
        },
      })
      .mockResolvedValue({
        data: {
          org_id: 'xiaotie',
          org_name: '测试企业',
          enabled: true,
          source_healthy: true,
          active_job: null,
          last_job: { id: 854, status: 'success', inserted: 2, updated: 1, skipped: 0, failed: 0 },
        },
      })
    mockGetJobs
      .mockResolvedValueOnce({ data: { list: [{ id: 854, status: 'running' }], total: 1 } })
      .mockResolvedValue({ data: { list: [{ id: 854, status: 'success' }], total: 1 } })
    mockGetJob.mockResolvedValue({ data: { id: 854, status: 'success', inserted: 2, updated: 1, skipped: 0, failed: 0 } })
    renderPage()

    await waitFor(() => expect(mockGetJob).toHaveBeenCalledWith(854))
    await waitFor(() => expect(screen.getByRole('button', { name: /立即同步/ })).not.toBeDisabled())
    expect(externalSyncJobPollingInterval('running')).toBe(2000)
    expect(externalSyncJobPollingInterval()).toBe(2000)
    expect(externalSyncJobPollingInterval('success')).toBe(false)
    expect(mockGetStatus.mock.calls.length).toBeGreaterThanOrEqual(2)
    expect(mockGetJobs.mock.calls.length).toBeGreaterThanOrEqual(2)
  })
})
