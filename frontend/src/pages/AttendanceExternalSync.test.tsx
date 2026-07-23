import React from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AttendanceExternalSync from './AttendanceExternalSync'

const mockGetStatus = vi.fn()
const mockGetJobs = vi.fn()
const mockRun = vi.fn()

vi.mock('../services/api', () => ({
  attendanceAPI: {
    externalSync: {
      getStatus: (...args: unknown[]) => mockGetStatus(...args),
      getJobs: (...args: unknown[]) => mockGetJobs(...args),
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
})
