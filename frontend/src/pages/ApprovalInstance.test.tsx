import React from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ApprovalInstance from './ApprovalInstance'

const mockGetInstances = vi.fn()
const mockGetTemplates = vi.fn()

vi.mock('../services/api', () => ({
  approvalAPI: {
    getInstances: (...args: unknown[]) => mockGetInstances(...args),
    getTemplates: (...args: unknown[]) => mockGetTemplates(...args),
    sync: vi.fn(),
  },
}))

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('../utils/permission', () => ({
  hasPermission: () => true,
}))

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ApprovalInstance />
    </QueryClientProvider>,
  )
}

describe('ApprovalInstance 标题搜索', () => {
  beforeEach(() => {
    mockGetInstances.mockReset()
    mockGetTemplates.mockReset()
    mockGetInstances.mockResolvedValue({ data: { items: [], total: 0 } })
    mockGetTemplates.mockResolvedValue({ data: { items: [] } })
  })

  it('输入停顿后以 title 关键词触发查询', async () => {
    renderPage()

    // 初始查询不带 title
    await waitFor(() => {
      expect(mockGetInstances).toHaveBeenCalled()
      const lastCall = mockGetInstances.mock.calls.at(-1)![0] as Record<string, unknown>
      expect(lastCall.title).toBeUndefined()
    })

    const input = screen.getByPlaceholderText('搜索标题')
    fireEvent.change(input, { target: { value: '请假' } })

    // 防抖 300ms 后应以 title 触发查询
    await waitFor(() => {
      const lastCall = mockGetInstances.mock.calls.at(-1)![0] as Record<string, unknown>
      expect(lastCall.title).toBe('请假')
    })
  })

  it('清空输入后 title 回到 undefined，不再过滤', async () => {
    renderPage()

    const input = screen.getByPlaceholderText('搜索标题')
    fireEvent.change(input, { target: { value: '加班' } })
    await waitFor(() => {
      const lastCall = mockGetInstances.mock.calls.at(-1)![0] as Record<string, unknown>
      expect(lastCall.title).toBe('加班')
    })

    fireEvent.change(input, { target: { value: '' } })
    await waitFor(() => {
      const lastCall = mockGetInstances.mock.calls.at(-1)![0] as Record<string, unknown>
      expect(lastCall.title).toBeUndefined()
    })
  })
})
