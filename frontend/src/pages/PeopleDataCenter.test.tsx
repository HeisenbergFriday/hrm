import React from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter, useLocation } from 'react-router-dom'
import PeopleDataCenter from './PeopleDataCenter'

const mockSummary = vi.fn()
const mockEmployees = vi.fn()
const mockBusiness = vi.fn()
const mockInboundLinks = vi.fn()
const mockStartSync = vi.fn()
const mockGetSyncRun = vi.fn()

vi.mock('../services/api', () => ({
  peopleDataCenterAPI: {
    getSummary: (...args: unknown[]) => mockSummary(...args),
    getEmployees: (...args: unknown[]) => mockEmployees(...args),
    getBusiness: (...args: unknown[]) => mockBusiness(...args),
    getInboundLinks: (...args: unknown[]) => mockInboundLinks(...args),
    startSync: (...args: unknown[]) => mockStartSync(...args),
    getSyncRun: (...args: unknown[]) => mockGetSyncRun(...args),
  },
}))

vi.mock('../utils/permission', () => ({ hasPermission: () => true }))

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const LocationProbe = () => <span data-testid="location">{useLocation().pathname}</span>
  return render(<MemoryRouter><QueryClientProvider client={client}><PeopleDataCenter /><LocationProbe /></QueryClientProvider></MemoryRouter>)
}

describe('PeopleDataCenter 人事数据中心', () => {
  beforeEach(() => {
    mockSummary.mockReset().mockResolvedValue({ data: { employee_total: 3, active_employee_total: 2, local_employee_total: 2, mirror_employee_total: 1, business_total: 4, local_business_total: 3, mirror_business_total: 1, sync_failure_count: 0 } })
    mockEmployees.mockReset().mockResolvedValue({ data: { items: [
      { id: 1, source_org_id: 'muteng', source_user_id: 'u1', employee_id: 'MT001', name: '张三', department_name: '人事部', position: 'HR', status: 'active', last_synced_at: '2026-09-21T10:00:00+08:00', is_mirror: false },
      { id: 2, source_org_id: 'xiaotie', source_user_id: 'u2', employee_id: 'WY001', name: '李四', department_name: '内容部', position: '编导', status: 'active', last_synced_at: '2026-09-21T10:00:00+08:00', is_mirror: true },
    ], total: 2, page: 1, page_size: 20 } })
    mockBusiness.mockReset().mockResolvedValue({ data: { items: [{ id: 7, source_org_id: 'xiaotie', entity_type: 'approval', source_key: 'proc-7', source_user_id: 'u2', payload: { title: '请假申请', applicant_name: '李四', status: 'COMPLETED', create_time: '2026-09-21T10:00:00+08:00' }, last_synced_at: '2026-09-21T10:00:00+08:00', sync_status: 'success', is_mirror: true }], total: 1, page: 1, page_size: 20 } })
    mockInboundLinks.mockReset().mockResolvedValue({ data: { items: [{ source_org_id: 'xiaotie', target_org_id: 'muteng', status: 'active', employee_sync: true, business_sync: true, business_scopes: 'attendance,approval,annual_leave_grant,overtime_match,performance_activity,performance_participant' }] } })
    mockStartSync.mockReset().mockResolvedValue({ data: { request_id: 'run-1', status: 'running', source_org_id: 'xiaotie', target_org_id: 'muteng' } })
    mockGetSyncRun.mockReset().mockResolvedValue({ data: { request_id: 'run-1', status: 'success', employee_count: 1, business_count: 6, failure_count: 0 } })
  })

  it('展示沐腾与文娱汇总卡片及员工来源', async () => {
    renderPage()
    expect(await screen.findByText('员工总数')).toBeInTheDocument()
    expect((await screen.findAllByText('文娱员工')).length).toBeGreaterThanOrEqual(1)
    expect(await screen.findByText('张三')).toBeInTheDocument()
    expect(screen.getByText('沐腾员工')).toBeInTheDocument()
    expect(screen.getAllByText('文娱员工').length).toBeGreaterThanOrEqual(1)
  })

  it('按员工来源筛选并区分本地跳转和文娱只读详情', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: '张三' }))
    expect(screen.getByTestId('location')).toHaveTextContent('/employees/1')

    await user.click(screen.getByRole('combobox', { name: '员工来源' }))
    await user.click(await screen.findByText('文娱员工', { selector: '.ant-select-item-option-content' }))
    await waitFor(() => expect(mockEmployees).toHaveBeenLastCalledWith(expect.objectContaining({ source: 'mirror' })))

    await user.click(await screen.findByRole('button', { name: '李四' }))
    expect(await screen.findByText('文娱员工镜像，仅供查看')).toBeInTheDocument()
  })

  it('可切换到业务数据页签并查看文娱业务镜像明细', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: '业务数据' }))
    expect(await screen.findByText('请假申请 · 李四')).toBeInTheDocument()
    await user.click(screen.getByRole('combobox', { name: '业务来源' }))
    await user.click(await screen.findByText('文娱业务镜像', { selector: '.ant-select-item-option-content' }))
    await waitFor(() => expect(mockBusiness).toHaveBeenLastCalledWith(expect.objectContaining({ source: 'mirror' })))
    await user.click(screen.getByRole('button', { name: '查看详情' }))
    expect(await screen.findByText('文娱业务镜像，仅供查看')).toBeInTheDocument()
    expect(screen.getByText('业务内容')).toBeInTheDocument()
  })

  it('显示同步按钮并启动文娱到沐腾任务', async () => {
    const user = userEvent.setup()
    renderPage()
    const button = await screen.findByRole('button', { name: '立即同步' })
    await user.click(button)
    await waitFor(() => expect(mockStartSync).toHaveBeenCalledWith('xiaotie'))
  })
})
