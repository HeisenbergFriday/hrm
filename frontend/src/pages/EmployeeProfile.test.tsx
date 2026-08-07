import React from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EmployeeProfile from './EmployeeProfile'

const mockGetProfiles = vi.fn()
const mockGetEmployees = vi.fn()
const mockGetDepartments = vi.fn()

vi.mock('../services/api', () => ({
  employeeAPI: {
    getProfiles: (...args: unknown[]) => mockGetProfiles(...args),
    createProfile: vi.fn(),
    updateProfile: vi.fn(),
  },
  orgAPI: {
    getEmployees: (...args: unknown[]) => mockGetEmployees(...args),
  },
  departmentAPI: {
    getDepartments: (...args: unknown[]) => mockGetDepartments(...args),
  },
}))

vi.mock('../utils/permission', () => ({ hasPermission: () => true }))

const employees = [
  { id: 1, user_id: 'anonymous-user-a', name: '匿名中文姓名甲', department_id: 'dept-a', position: '质量工程师', status: 'active' },
  { id: 2, user_id: 'anonymous-user-b', name: '匿名中文姓名乙', department_id: 'dept-b', position: '产品专员', status: 'active' },
]

const profiles = [
  { id: 1, user_id: 'anonymous-user-a', employee_id: 'ANON-EMP-001', profile_status: 'active' },
  { id: 2, user_id: 'anonymous-user-b', employee_id: 'ANON-EMP-002', profile_status: 'active' },
]

const profileResponse = (items = profiles, total = items.length) => Promise.resolve({ data: { items, total } })

const installProfileSearchMock = (total = 2) => {
  mockGetProfiles.mockImplementation(({ keyword }: { keyword?: string }) => {
    if (!keyword) return profileResponse(profiles, total)
    if (keyword.includes('甲')) return profileResponse(profiles.slice(0, 1), 1)
    if (keyword.includes('乙')) return profileResponse(profiles.slice(1), 1)
    return profileResponse([], 0)
  })
}

const renderPage = (initialEntry = '/employee-profile') => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  const router = createMemoryRouter([
    { path: '/employee-profile', element: <EmployeeProfile /> },
  ], { initialEntries: [initialEntry] })
  const result = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { router, queryClient, ...result }
}

describe('EmployeeProfile search and pagination', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetEmployees.mockResolvedValue({ data: { items: employees, total: employees.length } })
    mockGetDepartments.mockResolvedValue({
      data: { departments: [{ department_id: 'dept-a', name: '研发部' }, { department_id: 'dept-b', name: '产品部' }] },
    })
    installProfileSearchMock()
  })

  it('shows the employee profile search box and backend total', async () => {
    renderPage()
    expect(await screen.findByPlaceholderText('搜索姓名、工号、邮箱、手机号、岗位')).toBeInTheDocument()
    expect(await screen.findByText('匿名中文姓名甲')).toBeInTheDocument()
    expect(screen.getByText('共 2 条')).toBeInTheDocument()
    expect(mockGetProfiles).toHaveBeenCalledWith({ page: 1, page_size: 20, keyword: undefined })
  })

  it.each([
    ['完整中文姓名', '匿名中文姓名甲'],
    ['部分中文姓名', '中文姓名甲'],
  ])('submits %s to the employee profile API', async (_label, value) => {
    const user = userEvent.setup()
    const { router } = renderPage('/employee-profile?page=3&page_size=10')
    const input = await screen.findByPlaceholderText('搜索姓名、工号、邮箱、手机号、岗位')

    await user.type(input, `${value}{Enter}`)

    await waitFor(() => expect(mockGetProfiles).toHaveBeenLastCalledWith({ page: 1, page_size: 10, keyword: value }))
    expect(router.state.location.search).toContain(`keyword=${encodeURIComponent(value)}`)
    expect(router.state.location.search).toContain('page=1')
    expect(await screen.findByText('匿名中文姓名甲')).toBeInTheDocument()
  })

  it('trims keywords, resets pagination and restores the full list when cleared', async () => {
    const user = userEvent.setup()
    const { router } = renderPage('/employee-profile?page=4&page_size=20')
    const input = await screen.findByPlaceholderText('搜索姓名、工号、邮箱、手机号、岗位')

    await user.type(input, '  中文姓名甲  {Enter}')
    await waitFor(() => expect(mockGetProfiles).toHaveBeenLastCalledWith({ page: 1, page_size: 20, keyword: '中文姓名甲' }))
    expect(input).toHaveValue('中文姓名甲')
    expect(router.state.location.search).toContain('page=1')

    await user.click(screen.getByLabelText('close-circle'))
    await waitFor(() => expect(mockGetProfiles).toHaveBeenLastCalledWith({ page: 1, page_size: 20, keyword: undefined }))
    expect(router.state.location.search).not.toContain('keyword=')
    expect(await screen.findByText('匿名中文姓名乙')).toBeInTheDocument()
  })

  it('restores URL state after refresh and browser back or forward navigation', async () => {
    const firstURL = '/employee-profile?keyword=%E4%B8%AD%E6%96%87%E5%A7%93%E5%90%8D%E4%B9%99&page=2&page_size=10'
    const first = renderPage(firstURL)
    expect(await screen.findByPlaceholderText('搜索姓名、工号、邮箱、手机号、岗位')).toHaveValue('中文姓名乙')
    await waitFor(() => expect(mockGetProfiles).toHaveBeenLastCalledWith({ page: 2, page_size: 10, keyword: '中文姓名乙' }))
    first.unmount()

    const refreshed = renderPage(firstURL)
    const user = userEvent.setup()
    const input = await screen.findByPlaceholderText('搜索姓名、工号、邮箱、手机号、岗位')
    expect(input).toHaveValue('中文姓名乙')
    await user.clear(input)
    await user.type(input, '中文姓名甲{Enter}')
    await waitFor(() => expect(refreshed.router.state.location.search).toContain('%E7%94%B2'))

    await act(async () => refreshed.router.navigate(-1))
    await waitFor(() => expect(input).toHaveValue('中文姓名乙'))
    await act(async () => refreshed.router.navigate(1))
    await waitFor(() => expect(input).toHaveValue('中文姓名甲'))
  })

  it('shows an empty search result', async () => {
    const user = userEvent.setup()
    renderPage()
    const input = await screen.findByPlaceholderText('搜索姓名、工号、邮箱、手机号、岗位')
    await user.type(input, '不存在{Enter}')
    expect(await screen.findByText('未找到匹配的员工档案')).toBeInTheDocument()
  })

  it('shows request failure and retries the employee profile API', async () => {
    mockGetProfiles.mockReset()
    mockGetProfiles
      .mockRejectedValueOnce(new Error('request failed'))
      .mockImplementation(() => profileResponse(profiles, 2))
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('员工档案加载失败')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /重\s*试/ }))
    expect(await screen.findByText('匿名中文姓名甲')).toBeInTheDocument()
    expect(mockGetProfiles).toHaveBeenCalledTimes(2)
  })

  it('uses API total and writes page and page size to the URL', async () => {
    installProfileSearchMock(42)
    const user = userEvent.setup()
    const { router } = renderPage('/employee-profile?page=1&page_size=10')

    expect(await screen.findByText('共 42 条')).toBeInTheDocument()
    await user.click(screen.getByTitle('2'))
    await waitFor(() => expect(router.state.location.search).toContain('page=2'))
    expect(router.state.location.search).toContain('page_size=10')
    await waitFor(() => expect(mockGetProfiles).toHaveBeenLastCalledWith({ page: 2, page_size: 10, keyword: undefined }))
  })
})
