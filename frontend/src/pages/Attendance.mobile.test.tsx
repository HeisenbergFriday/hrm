import React from 'react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Attendance from './Attendance'

vi.mock('../services/api', () => ({
  attendanceAPI: {
    externalSync: {
      getDailyResults: vi.fn().mockResolvedValue({
        data: {
          total: 1,
          summary: { total: 1, normal: 1, exception: 0, with_approval: 0 },
          items: [
            {
              key: 'u1|2026-07-16',
              work_date: '2026-07-16',
              user_id: 'u1',
              external_user_id: 'u1',
              user_name: '李四',
              department_name: '产品部',
              on_duty_time: '2026-07-16T08:58:00+08:00',
              off_duty_time: '2026-07-16T18:02:00+08:00',
              punches: [],
              statuses: [{ code: 'normal', label: '正常', level: 'success', category: 'attendance' }],
              approvals: [],
              has_exception: false,
              source_updated_at: '2026-07-16T18:02:00+08:00',
            },
          ],
        },
      }),
      getStatus: vi.fn().mockResolvedValue({ data: {} }),
      run: vi.fn(),
    },
  },
  userAPI: { getUsers: vi.fn().mockResolvedValue({ data: { items: [] } }) },
  departmentAPI: { getDepartments: vi.fn().mockResolvedValue({ data: { departments: [] } }) },
}))

vi.mock('../utils/permission', () => ({
  hasPermission: () => true,
  hasMenuPermission: () => true,
}))

vi.mock('../utils/responsive', () => ({
  resolveMobileLayout: () => true,
  useMobileRuntime: () => true,
}))

describe('Attendance mobile view', () => {
  it('uses a tappable card instead of a wide table', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { container } = render(
      <MemoryRouter>
        <QueryClientProvider client={client}>
          <Attendance />
        </QueryClientProvider>
      </MemoryRouter>,
    )

    expect(await screen.findByText('李四')).toBeInTheDocument()
    expect(container.querySelector('.attendance-mobile-item')).toBeInTheDocument()
    expect(container.querySelector('.ant-table')).not.toBeInTheDocument()

    fireEvent.click(container.querySelector('.attendance-mobile-item')!)
    expect(await screen.findByText(/李四 · 2026-07-16/)).toBeInTheDocument()
  })
})
