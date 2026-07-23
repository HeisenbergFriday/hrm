import React from 'react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Attendance from './Attendance'

const getDailyResults = vi.fn()
const getStatus = vi.fn()
const runSync = vi.fn()
const getUsers = vi.fn()

vi.mock('../services/api', () => ({
  attendanceAPI: {
    externalSync: {
      getDailyResults: (...args: unknown[]) => getDailyResults(...args),
      getStatus: (...args: unknown[]) => getStatus(...args),
      run: (...args: unknown[]) => runSync(...args),
    },
  },
  userAPI: {
    getUsers: (...args: unknown[]) => getUsers(...args),
  },
  departmentAPI: {
    getDepartments: vi.fn().mockResolvedValue({ data: { departments: [] } }),
  },
}))

vi.mock('../utils/permission', () => ({
  hasPermission: () => true,
  hasMenuPermission: () => true,
}))

vi.mock('../utils/responsive', () => ({
  resolveMobileLayout: () => false,
  useMobileRuntime: () => false,
}))

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <MemoryRouter>
      <QueryClientProvider client={client}>
        <Attendance />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('Attendance daily result view', () => {
  beforeEach(() => {
    getDailyResults.mockReset()
    getStatus.mockReset()
    runSync.mockReset()
    getUsers.mockReset()
    getUsers.mockImplementation((params: { search?: string }) => Promise.resolve({
      data: {
        items: params.search === '测试员工'
          ? [{ user_id: 'xiaotie:test-employee', name: '测试员工' }]
          : [],
      },
    }))
    getStatus.mockResolvedValue({ data: { external_last_attendance_update: '2026-07-16T09:00:00+08:00' } })
    getDailyResults.mockResolvedValue({
      data: {
        total: 1,
        summary: { total: 1, normal: 0, exception: 1, with_approval: 1 },
        items: [
          {
            key: 'u1|2026-07-16',
            work_date: '2026-07-16',
            user_id: 'xiaotie:u1',
            external_user_id: 'u1',
            user_name: '张三',
            department_name: '研发部',
            on_duty_time: '2026-07-16T09:12:00+08:00',
            off_duty_time: '2026-07-16T18:05:00+08:00',
            punches: [
              {
                check_type: '上班',
                check_time: '2026-07-16T09:12:00+08:00',
                time_result: 'Late',
                location_result: 'Normal',
                user_address: '深圳办公室',
              },
              {
                check_type: '下班',
                check_time: '2026-07-16T18:05:00+08:00',
                time_result: 'Normal',
                location_result: 'Normal',
              },
            ],
            statuses: [
              { code: 'late', label: '迟到', level: 'warning', category: 'attendance' },
              { code: 'leave:年假', label: '年假 2小时', level: 'processing', category: 'approval' },
            ],
            approvals: [
              {
                proc_inst_id: 'proc-1',
                tag_name: '请假',
                sub_type: '年假',
                label: '年假 2小时',
                begin_time: '2026-07-16T14:00:00+08:00',
                end_time: '2026-07-16T16:00:00+08:00',
              },
            ],
            has_exception: true,
            source_updated_at: '2026-07-16T18:05:00+08:00',
          },
        ],
      },
    })
  })

  it('shows punch times and keeps late plus leave statuses', async () => {
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('张三')).toBeInTheDocument()
    expect(screen.getByText('09:12')).toBeInTheDocument()
    expect(screen.getByText('18:05')).toBeInTheDocument()
    expect(screen.getByText('迟到')).toBeInTheDocument()
    expect(screen.getAllByText('年假 2小时').length).toBeGreaterThan(0)

    await user.click(screen.getByRole('button', { name: /异常/ }))
    await waitFor(() => {
      expect(getDailyResults).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'exception' }))
    })

    await user.click(screen.getByRole('button', { name: '查看详情' }))
    expect(await screen.findByText(/深圳办公室/)).toBeInTheDocument()
    expect(screen.getByText(/2026-07-16 14:00:00 至 2026-07-16 16:00:00/)).toBeInTheDocument()
  })

  it('searches employees remotely instead of limiting results to the first page', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByRole('button', { name: /更多筛选/ }))
    const employeeSelect = await screen.findByRole('combobox', { name: '搜索员工' })
    await user.click(employeeSelect)
    await user.type(employeeSelect, '测试员工')

    await waitFor(() => {
      expect(getUsers).toHaveBeenCalledWith({
        page: 1,
        page_size: 50,
        search: '测试员工',
      })
    })

    await user.click(await screen.findByText('测试员工'))
    await waitFor(() => {
      expect(getDailyResults).toHaveBeenLastCalledWith(
        expect.objectContaining({ user_id: 'xiaotie:zhengfengyi' }),
      )
    })
  })
})
