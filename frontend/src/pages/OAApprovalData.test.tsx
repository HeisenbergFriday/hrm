import React from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OAApprovalData from './OAApprovalData'

const mockGetOAData = vi.fn()

vi.mock('../services/api', () => ({
  approvalAPI: {
    getOAData: (...args: unknown[]) => mockGetOAData(...args),
  },
}))

vi.mock('../store/authStore', () => ({
  useAuthStore: (selector: (state: { orgId: string }) => unknown) => selector({ orgId: 'muteng' }),
}))

vi.mock('../utils/permission', () => ({
  hasPermission: () => true,
  hasMenuPermission: () => true,
}))

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <OAApprovalData />
    </QueryClientProvider>,
  )
}

describe('OAApprovalData', () => {
  beforeEach(() => {
    mockGetOAData.mockReset()
    mockGetOAData.mockResolvedValue({
      data: {
        items: [{
          key: 'proc-1',
          fields: {
            corp_name: '深圳市沐腾科技有限公司',
            process_instance_id: 'proc-1',
            process_name: '请假',
            approval_title: '张三提交的请假',
            originator_user_name: '张三',
            originator_mobile: '13812345678',
            originator_email: 'zhangsan@example.com',
            create_time: '2026-07-24T00:00:00Z',
            form_component_values: JSON.stringify([
              { componentType: 'TextField', name: '联系电话', value: '13987654321' },
            ]),
            custom_field: '自定义内容',
          },
        }],
        total: 1,
      },
    })
  })

  it('renders approval rows and all fields in the detail drawer', async () => {
    renderPage()

    expect(await screen.findByText('张三提交的请假')).toBeInTheDocument()
    expect(screen.getByText('2026-07-24 08:00:00')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '查看审批详情' }))
    expect(await screen.findByText('custom_field')).toBeInTheDocument()
    expect(screen.getByText('自定义内容')).toBeInTheDocument()
    expect(screen.getByText('138****5678')).toBeInTheDocument()
    expect(screen.getByText('z***@example.com')).toBeInTheDocument()
    expect(screen.getByText('139****4321')).toBeInTheDocument()
    expect(screen.queryByText('13812345678')).not.toBeInTheDocument()
    expect(screen.queryByText('zhangsan@example.com')).not.toBeInTheDocument()
    expect(screen.queryByText('13987654321')).not.toBeInTheDocument()
  })
})
