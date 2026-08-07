import React from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EmployeeList from './EmployeeList'

const mockGetDepartments = vi.fn()
const mockGetOverview = vi.fn()
const mockGetEmployees = vi.fn()

vi.mock('../services/api', () => ({
  departmentAPI: {
    getDepartments: (...args: unknown[]) => mockGetDepartments(...args),
  },
  orgAPI: {
    getOverview: (...args: unknown[]) => mockGetOverview(...args),
    getEmployees: (...args: unknown[]) => mockGetEmployees(...args),
  },
}))

vi.mock('../utils/permission', () => ({ hasPermission: () => true }))
vi.mock('../utils/orgSyncAction', () => ({ confirmOrgSync: vi.fn() }))

const overview = {
  scope: { mode: 'all' },
  summary: {
    active_employees: 8,
    probation_employee_count: 2,
    planned_regularization_count: 1,
  },
  employee_type_distribution: [{ key: '1', label: '后端可信名称', count: 8 }],
  job_level_distribution: [],
  job_family_distribution: [],
}

const employees = [
  { id: 1, user_id: 'masked-1', name: '测试员工甲', email: '', mobile: '', department_id: 'dept-a', position: '工程师', status: 'active' },
  { id: 2, user_id: 'masked-2', name: '测试员工乙', email: '', mobile: '', department_id: 'dept-a', position: '工程师', status: 'active' },
]

const renderPage = (initialEntry = '/employees') => {
  const router = createMemoryRouter([
    { path: '/employees', element: <EmployeeList /> },
  ], { initialEntries: [initialEntry] })
  const result = render(<RouterProvider router={router} />)
  return { router, ...result }
}

describe('EmployeeList URL filters', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetDepartments.mockResolvedValue({ data: { departments: [{ department_id: 'dept-a', name: '研发部', parent_id: 'root' }] } })
    mockGetOverview.mockResolvedValue({ data: { overview } })
    mockGetEmployees.mockImplementation(({ filter_type: filterType }) => Promise.resolve({
      data: {
        items: filterType === 'regularization_warning' ? employees.slice(0, 1) : employees,
        total: filterType === 'regularization_warning' ? 1 : 2,
      },
    }))
  })

  it('pushes card filters and restores them with back and forward', async () => {
    const user = userEvent.setup()
    const { router } = renderPage()
    await screen.findByText('测试员工甲')

    const probationCard = screen.getByRole('button', { name: '筛选试用期人数' })
    expect(within(probationCard).getByText('2')).toBeInTheDocument()
    await user.click(probationCard)
    await waitFor(() => expect(router.state.location.search).toBe('?filter_type=probation'))
    await waitFor(() => expect(mockGetEmployees).toHaveBeenLastCalledWith(expect.objectContaining({ filter_type: 'probation' })))
    expect(screen.getByText('当前筛选：试用期员工（2 人）')).toBeInTheDocument()

    await router.navigate(-1)
    await waitFor(() => expect(router.state.location.search).toBe(''))
    expect(screen.queryByText(/当前筛选：试用期员工/)).not.toBeInTheDocument()

    await router.navigate(1)
    await waitFor(() => expect(router.state.location.search).toBe('?filter_type=probation'))
    expect(await screen.findByText('当前筛选：试用期员工（2 人）')).toBeInTheDocument()
  })

  it('supports Enter and Space and closes the active tag', async () => {
    const user = userEvent.setup()
    const { router } = renderPage()
    await screen.findByText('测试员工甲')

    const warningCard = screen.getByRole('button', { name: '筛选计划转正预警' })
    warningCard.focus()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(router.state.location.search).toBe('?filter_type=regularization_warning'))
    expect(await screen.findByText('当前筛选：计划转正预警（1 人）')).toBeInTheDocument()

    await user.click(screen.getByLabelText('关闭业务筛选'))
    await waitFor(() => expect(router.state.location.search).toBe(''))

    warningCard.focus()
    await user.keyboard(' ')
    await waitFor(() => expect(router.state.location.search).toBe('?filter_type=regularization_warning'))
  })

  it('controls search from the URL, trims it, resets page and clears filters', async () => {
    const user = userEvent.setup()
    const { router } = renderPage('/employees?search=%E6%97%A7%E5%85%B3%E9%94%AE%E8%AF%8D&department_id=dept-a&status=active&page=3')
    const input = await screen.findByPlaceholderText('搜索姓名、工号、邮箱、手机号、岗位')
    expect(input).toHaveValue('旧关键词')
    await waitFor(() => expect(mockGetEmployees).toHaveBeenLastCalledWith(expect.objectContaining({
      search: '旧关键词', department_id: 'dept-a', status: 'active', page: 3,
    })))

    await user.clear(input)
    await user.type(input, '  中文姓名  {Enter}')
    await waitFor(() => {
      expect(router.state.location.search).toContain('search=%E4%B8%AD%E6%96%87%E5%A7%93%E5%90%8D')
      expect(router.state.location.search).not.toContain('page=')
    })

    await user.click(screen.getByRole('button', { name: '清除筛选' }))
    await waitFor(() => expect(router.state.location.search).toBe(''))
    expect(input).toHaveValue('')
  })

  it('restores copied or refreshed URLs and never remaps backend employee labels', async () => {
    const url = '/employees?search=%E5%A7%93%E5%90%8D&filter_type=probation&page=2'
    const first = renderPage(url)
    const input = await screen.findByPlaceholderText('搜索姓名、工号、邮箱、手机号、岗位')
    expect(input).toHaveValue('姓名')
    expect(screen.getByText('后端可信名称')).toBeInTheDocument()
    expect(first.router.state.location.search).toContain('filter_type=probation')

    first.unmount()
    const refreshed = renderPage(url)
    expect(await screen.findByPlaceholderText('搜索姓名、工号、邮箱、手机号、岗位')).toHaveValue('姓名')
    expect(refreshed.router.state.location.search).toContain('page=2')
  })
})
