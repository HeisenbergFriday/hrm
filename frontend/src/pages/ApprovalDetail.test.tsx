import React from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ApprovalDetail from './ApprovalDetail'

const mockGetApproval = vi.fn()
const mockSync = vi.fn()
const mockResumeSync = vi.fn()
const mockPendingRequestID = vi.fn()
const mockNavigate = vi.fn()

vi.mock('../services/api', () => ({
  approvalAPI: {
    getApproval: (...args: unknown[]) => mockGetApproval(...args),
    sync: (...args: unknown[]) => mockSync(...args),
    resumeSync: (...args: unknown[]) => mockResumeSync(...args),
  },
  getPendingApprovalSyncRequestID: () => mockPendingRequestID(),
}))

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useLocation: () => ({ pathname: '/approval-detail/approval-1', state: undefined }),
  useParams: () => ({ id: 'approval-1' }),
}))

vi.mock('../utils/permission', () => ({
  hasPermission: () => true,
}))

const renderPage = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ApprovalDetail />
    </QueryClientProvider>,
  )
}

describe('ApprovalDetail 审批内容展示', () => {
  beforeEach(() => {
    mockGetApproval.mockReset()
    mockSync.mockReset()
    mockResumeSync.mockReset()
    mockPendingRequestID.mockReset()
    mockPendingRequestID.mockReturnValue('')
    mockNavigate.mockReset()
  })

  it('递归展示加班审批中的嵌套表单，不渲染原始长 JSON', async () => {
    const rawFormValue = JSON.stringify({
      componentName: 'TableField',
      componentType: 'TableField',
      id: 'table-1',
      props: {
        bizAlias: 'overtimeDetail',
        label: '加班明细',
        children: [
          { label: '加班日期', value: '2026-10-24' },
          { label: '开始时间', value: '09:00' },
          { label: '结束时间', value: '18:30' },
        ],
      },
    })
    mockGetApproval.mockResolvedValue({
      data: {
        approval: {
          id: 'approval-1',
          title: '加班审批',
          status: 'COMPLETED',
          applicant_name: '张三',
          create_time: '2026-10-24 09:00:00',
          content: { 加班: rawFormValue },
          flow_history: [],
        },
      },
    })

    renderPage()

    expect(await screen.findByText('加班明细')).toBeInTheDocument()
    expect(screen.getByText('加班日期')).toBeInTheDocument()
    expect(screen.getByText('2026-10-24')).toBeInTheDocument()
    expect(screen.getByText('开始时间')).toBeInTheDocument()
    expect(screen.getByText('18:30')).toBeInTheDocument()
    expect(screen.queryByText(rawFormValue)).not.toBeInTheDocument()
  })

  it('保留普通对象和数组中的业务字段，并过滤表单元数据', async () => {
    mockGetApproval.mockResolvedValue({
      data: {
        approval: {
          id: 'approval-1',
          title: '其他审批',
          status: 'COMPLETED',
          applicant_name: '李四',
          create_time: '2026-10-24 09:00:00',
          content: {
            说明: { text: '需要采购办公用品', required: true },
            明细: JSON.stringify([{ name: '签字笔', amount: 10 }, { name: '文件夹', amount: 5 }]),
          },
          flow_history: [],
        },
      },
    })

    renderPage()

    await waitFor(() => expect(screen.getByText('需要采购办公用品')).toBeInTheDocument())
    expect(screen.getByText('签字笔')).toBeInTheDocument()
    expect(screen.getByText('文件夹')).toBeInTheDocument()
    expect(screen.queryByText('true')).not.toBeInTheDocument()
  })

  it('过滤辅助说明、空值和技术性中间层，并保留加班业务字段', async () => {
    mockGetApproval.mockResolvedValue({
      data: {
        approval: {
          id: 'approval-1',
          title: '加班申请',
          status: 'COMPLETED',
          applicant_name: '王五',
          create_time: '2026-10-24 09:00:00',
          content: {
            加班信息: JSON.stringify({
              componentName: 'TableField',
              props: {
                label: '加班明细',
                children: [
                  { label: '开始时间', value: '2026-10-24 18:00' },
                  { label: '结束时间', value: '2026-10-24 21:00' },
                  { label: '时长（小时）', value: 3 },
                  { label: '因何事加班', value: '版本发布' },
                  { label: '是否运维人员', value: '否' },
                  { label: '是否包含法定节假日', value: '否' },
                  { label: '附件', value: 'https://example.com/overtime.png' },
                  { componentName: 'TextNote', props: { content: '温馨提示：时长由系统自动计算' } },
                ],
              },
            }),
            关联审批单: 'null',
            说明: JSON.stringify({ componentName: 'TextNote', props: { content: '组件配置说明，仅供参考' } }),
          },
          flow_history: [],
        },
      },
    })

    renderPage()

    expect(await screen.findByText('加班明细')).toBeInTheDocument()
    expect(screen.getByText('因何事加班')).toBeInTheDocument()
    expect(screen.getByText('版本发布')).toBeInTheDocument()
    expect(screen.getByText('时长（小时）')).toBeInTheDocument()
    expect(screen.getByText('是否包含法定节假日')).toBeInTheDocument()
    expect(screen.getByText('附件')).toBeInTheDocument()
    expect(screen.queryByText('字段')).not.toBeInTheDocument()
    expect(screen.queryByText('值')).not.toBeInTheDocument()
    expect(screen.queryByText('温馨提示：时长由系统自动计算')).not.toBeInTheDocument()
    expect(screen.queryByText('组件配置说明，仅供参考')).not.toBeInTheDocument()
    expect(screen.queryByText('null')).not.toBeInTheDocument()
  })
})
