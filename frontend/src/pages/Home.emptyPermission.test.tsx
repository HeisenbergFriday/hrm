import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Home, { calculateExternalAttendanceRate } from './Home'
import { useAuthStore } from '../store/authStore'

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
  Link: ({ children, ...props }: any) => <a {...props}>{children}</a>,
}))

vi.mock('../services/api', () => ({
  orgAPI: { getOverview: vi.fn() },
  attendanceAPI: { externalSync: { getDailyResults: vi.fn() } },
  approvalAPI: { getInstances: vi.fn() },
}))

describe('Home empty permission', () => {
  beforeEach(() => {
    useAuthStore.setState({
      user: { name: '空权限用户', org_id: 'org-1' },
      isLoggedIn: true,
      menuKeys: [],
      permissions: [],
      orgId: 'org-1',
    })
  })

  it('menuKeys 为空时展示友好空态，不抛错、不请求统计', () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <Home />
      </QueryClientProvider>,
    )

    expect(screen.getByText('暂无数据权限')).toBeInTheDocument()
    expect(
      screen.getByText('您尚未被分配任何角色，请联系管理员配置权限后再使用系统功能。'),
    ).toBeInTheDocument()
    expect(screen.getByText('系统概览')).toBeInTheDocument()
  })
})

describe('calculateExternalAttendanceRate', () => {
  it('uses all daily results as the denominator, including approval days', () => {
    expect(calculateExternalAttendanceRate({ total: 10, normal: 7 })).toBe(70)
  })

  it('returns zero when no daily results exist', () => {
    expect(calculateExternalAttendanceRate({ total: 0, normal: 0 })).toBe(0)
  })

  it('rounds the rate to two decimal places', () => {
    expect(calculateExternalAttendanceRate({ total: 3, normal: 2 })).toBe(66.67)
  })
})
