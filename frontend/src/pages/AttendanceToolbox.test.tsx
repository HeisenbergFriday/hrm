import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { message } from 'antd'
import AttendanceToolbox from './AttendanceToolbox'

const mockRun = vi.fn()
const mockGetDefaults = vi.fn()
const mockListTemplates = vi.fn()
const mockRunDingtalkSync = vi.fn()

vi.mock('../services/api', () => ({
  attendanceToolboxAPI: {
    getDefaults: (...args: unknown[]) => mockGetDefaults(...args),
    listTemplates: (...args: unknown[]) => mockListTemplates(...args),
    run: (...args: unknown[]) => mockRun(...args),
    runDingtalkSync: (...args: unknown[]) => mockRunDingtalkSync(...args),
  },
}))

describe('AttendanceToolbox', () => {
  beforeEach(() => {
    mockRun.mockReset()
    mockGetDefaults.mockReset()
    mockListTemplates.mockReset()
    mockRunDingtalkSync.mockReset()
    mockGetDefaults.mockResolvedValue({
      data: {
        leave_special_names: ['梁伯林', '陈秋宇'],
        chengdu_schedule_names: ['费婷玉'],
        sub_dept_keywords: ['产品中心', '研发中心'],
        sub_late22_names: ['崔利华'],
        part_special_names: ['王心英'],
      },
    })
    mockListTemplates.mockResolvedValue({ data: { templates: [] } })
    mockRunDingtalkSync.mockResolvedValue(new Blob(['excel'], {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }))
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders hero, toolbar, tabs, and default text values', async () => {
    render(<AttendanceToolbox />)

    // Hero
    expect(screen.getByText('考勤数据处理工具')).toBeInTheDocument()
    expect(screen.getByText('Excel 导入')).toBeInTheDocument()

    // Toolbar
    expect(screen.getByText('模板下载中心')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /下载全部模板/ })).toBeInTheDocument()

    // Tabs
    expect(screen.getByText('请假明细')).toBeInTheDocument()
    expect(screen.getByText('加班明细')).toBeInTheDocument()
    expect(screen.getByText('补贴扣款')).toBeInTheDocument()
    expect(screen.getByText('最终汇总')).toBeInTheDocument()

    // Default-loaded text areas
    expect(await screen.findByDisplayValue('梁伯林、陈秋宇')).toBeInTheDocument()
    expect(screen.getByDisplayValue('费婷玉')).toBeInTheDocument()

    expect(mockGetDefaults).toHaveBeenCalledTimes(1)
  })

  it('blocks leave calculation when required files are missing', async () => {
    const user = userEvent.setup()
    const warningSpy = vi.spyOn(message, 'warning').mockImplementation(() => null as never)

    render(<AttendanceToolbox />)
    await screen.findByDisplayValue('梁伯林、陈秋宇')

    await user.click(screen.getByRole('button', { name: /开始计算/ }))

    expect(warningSpy).toHaveBeenCalledWith('请上传请假系统导出表')
    expect(mockRun).not.toHaveBeenCalled()
  })

  it('shows uploaded files inside the upload card', async () => {
    const user = userEvent.setup()
    const { container } = render(<AttendanceToolbox />)
    await screen.findByDisplayValue('梁伯林、陈秋宇')

    const input = container.querySelector('input[type="file"]') as HTMLInputElement | null
    expect(input).not.toBeNull()

    const file = new File(['excel'], '请假导出表.xlsx', {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    })
    await user.upload(input as HTMLInputElement, file)

    expect(await screen.findByText('请假导出表.xlsx')).toBeInTheDocument()
  })

  it('blocks parttime calculation when the default schedule is missing', async () => {
    const user = userEvent.setup()
    const warningSpy = vi.spyOn(message, 'warning').mockImplementation(() => null as never)

    render(<AttendanceToolbox />)
    await screen.findByDisplayValue('梁伯林、陈秋宇')

    await user.click(screen.getByRole('tab', { name: /兼职汇总/ }))
    await user.click(screen.getByRole('button', { name: /开始计算/ }))

    expect(warningSpy).toHaveBeenCalledWith('请上传默认作息表')
    expect(mockRun).not.toHaveBeenCalled()
  })

  it('renders the upload requirements collapse with field badges', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await screen.findByDisplayValue('梁伯林、陈秋宇')

    // Collapse is collapsed by default; expand it
    await user.click(screen.getByText('上传文件要求和模板'))

    // Field badges should be visible
    expect(await screen.findByText('审批状态')).toBeInTheDocument()
    expect(screen.getByText('开始时间')).toBeInTheDocument()
  })

  it('renders dingtalk sync tab with its action button', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await screen.findByDisplayValue('梁伯林、陈秋宇')

    await user.click(screen.getByRole('tab', { name: /钉钉同步/ }))

    expect(await screen.getByRole('button', { name: /从钉钉同步并生成中间表/ })).toBeInTheDocument()
  })

  it('auto syncs roster and transfer files while keeping manual upload flows available', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)

    await screen.findByDisplayValue('梁伯林、陈秋宇')
    await waitFor(() => expect(mockRunDingtalkSync).toHaveBeenCalledTimes(2))

    expect(screen.getAllByRole('button', { name: /从钉钉同步/ }).length).toBeGreaterThan(0)

    await user.click(screen.getByRole('tab', { name: /加班明细/ }))
    expect(await screen.findByText('花名册_钉钉自动同步.xlsx')).toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: /最终汇总/ }))
    expect(await screen.findByText('异动流程_钉钉自动同步.xlsx')).toBeInTheDocument()
  })
})
