import React from 'react'
import dayjs from 'dayjs'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Modal } from 'antd'
import WeekSchedule, { buildUpcomingSaturdayNotice } from './WeekSchedule'

const mocks = vi.hoisted(() => ({
  permissions: [] as string[],
  getUsers: vi.fn(),
  getDepartments: vi.fn(),
  getShifts: vi.fn(),
  getRules: vi.fn(),
  getCalendar: vi.fn(),
  getHolidays: vi.fn(),
  getSyncLogs: vi.fn(),
  getGroupTargets: vi.fn(),
  pushPersonalSchedule: vi.fn(),
  pushGroupSchedule: vi.fn(),
  listShiftConfigs: vi.fn(),
  messageSuccess: vi.fn(),
  messageError: vi.fn(),
  messageWarning: vi.fn(),
  messageInfo: vi.fn(),
}))

vi.mock('../store/authStore', () => ({
  useAuthStore: (selector: (state: { permissions: string[] }) => unknown) =>
    selector({ permissions: mocks.permissions }),
}))

vi.mock('../services/api', () => ({
  userAPI: {
    getUsers: (...args: unknown[]) => mocks.getUsers(...args),
  },
  departmentAPI: {
    getDepartments: (...args: unknown[]) => mocks.getDepartments(...args),
  },
  shiftConfigAPI: {
    list: (...args: unknown[]) => mocks.listShiftConfigs(...args),
  },
  weekScheduleAPI: {
    getShifts: (...args: unknown[]) => mocks.getShifts(...args),
    getRules: (...args: unknown[]) => mocks.getRules(...args),
    getCalendar: (...args: unknown[]) => mocks.getCalendar(...args),
    getHolidays: (...args: unknown[]) => mocks.getHolidays(...args),
    getSyncLogs: (...args: unknown[]) => mocks.getSyncLogs(...args),
    getGroupTargets: (...args: unknown[]) => mocks.getGroupTargets(...args),
    pushPersonalSchedule: (...args: unknown[]) => mocks.pushPersonalSchedule(...args),
    pushGroupSchedule: (...args: unknown[]) => mocks.pushGroupSchedule(...args),
  },
}))

vi.mock('antd', async () => {
  const actual = await vi.importActual<typeof import('antd')>('antd')
  return {
    ...actual,
    message: {
      success: mocks.messageSuccess,
      error: mocks.messageError,
      warning: mocks.messageWarning,
      info: mocks.messageInfo,
      destroy: vi.fn(),
    },
  }
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <WeekSchedule />
    </QueryClientProvider>,
  )
}

function mockBaseData() {
  mocks.getUsers.mockResolvedValue({
    data: {
      items: [{ id: 1, user_id: 'u-1', name: '张三', department_id: 'd-1' }],
      total: 1,
    },
  })
  mocks.getDepartments.mockResolvedValue({ data: { departments: [{ id: 1, department_id: 'd-1', name: '研发部' }] } })
  mocks.getShifts.mockResolvedValue({ data: { items: [] } })
  mocks.getRules.mockResolvedValue({ data: { items: [] } })
  mocks.getHolidays.mockResolvedValue({ data: { items: [] } })
  mocks.getSyncLogs.mockResolvedValue({ data: { items: [] } })
  mocks.listShiftConfigs.mockResolvedValue({ data: { items: [] } })
  mocks.getCalendar.mockResolvedValue({
    data: {
      items: [{
        week_start: '2026-07-27',
        week_end: '2026-08-02',
        week_type: 'big',
        is_override: false,
        saturday_work: true,
        holidays: [],
      }],
    },
  })
  mocks.getGroupTargets.mockResolvedValue({
    data: {
      items: [{ id: 7, group_name: '研发群', status: 'active', bound_by_user_name: '张三', bound_at: '2026-07-29T09:00:00Z' }],
    },
  })
}

beforeEach(() => {
  Modal.destroyAll()
  document.querySelectorAll('.ant-modal-root').forEach((item) => item.remove())
  vi.clearAllMocks()
  vi.setSystemTime(new Date('2026-07-29T09:00:00+08:00'))
  mockBaseData()
  Object.defineProperty(HTMLCanvasElement.prototype, 'getContext', {
    configurable: true,
    value: vi.fn(() => ({
      scale: vi.fn(),
      fillRect: vi.fn(),
      strokeRect: vi.fn(),
      fillText: vi.fn(),
      fillStyle: '',
      strokeStyle: '',
      font: '',
      textAlign: '',
      textBaseline: '',
      globalAlpha: 1,
    })),
  })
  Object.defineProperty(HTMLCanvasElement.prototype, 'toBlob', {
    configurable: true,
    value: vi.fn((callback: BlobCallback) => callback(new Blob(['png'], { type: 'image/png' }))),
  })
})

describe('buildUpcomingSaturdayNotice', () => {
  it.each([
    {
      name: '周三提示本周六需上班',
      today: '2026-07-29',
      saturday: '2026-08-01',
      isWork: true,
      weekLabel: '小周',
      expected: [
        '【本周六需上班】',
        '本周六（2026年8月1日）需上班，请提前安排。',
        '本周为小周。',
      ],
    },
    {
      name: '周五提示明天需上班',
      today: '2026-07-31',
      saturday: '2026-08-01',
      isWork: true,
      weekLabel: '小周',
      expected: [
        '【明天需上班】',
        '明天（2026年8月1日，周六）需上班，请提前安排。',
        '本周为小周。',
      ],
    },
    {
      name: '周六提示今天休息',
      today: '2026-08-01',
      saturday: '2026-08-01',
      isWork: false,
      weekLabel: '大周',
      expected: [
        '【今天休息】',
        '今天（2026年8月1日，周六）休息，无需上班。',
        '本周为大周。',
      ],
    },
    {
      name: '周日提示下周六休息',
      today: '2026-08-02',
      saturday: '2026-08-08',
      isWork: false,
      weekLabel: '大周',
      expected: [
        '【下周六休息】',
        '下周六（2026年8月8日）休息，无需上班。',
        '下周为大周。',
      ],
    },
  ])('$name', ({ today, saturday, isWork, weekLabel, expected }) => {
    expect(buildUpcomingSaturdayNotice(dayjs(today), dayjs(saturday), isWork, weekLabel)).toEqual(expected)
  })
})

describe('WeekSchedule push dialog', () => {
  it('keeps the write button disabled with a permission tooltip', async () => {
    mocks.permissions = []
    const user = userEvent.setup()
    renderPage()

    const button = (await screen.findByText('作息表推送')).closest('button') as HTMLButtonElement
    expect(button).toBeDisabled()
    await user.hover(button.parentElement as HTMLElement)
    expect(await screen.findByText('你缺少作息表推送权限，需要联系管理员添加')).toBeInTheDocument()
  })

  it('switches from employees to groups, requires a target, confirms twice, and only submits local target id', async () => {
    mocks.permissions = ['attendance_manage']
    mocks.pushGroupSchedule.mockResolvedValue({ data: { status: 'submitted', message: '群消息已提交钉钉处理' } })
    const user = userEvent.setup()
    renderPage()

    await screen.findAllByText('第1周')
    await user.click((await screen.findByText('作息表推送')).closest('button') as HTMLButtonElement)
    const dialog = await screen.findByRole('dialog', { name: '作息表推送' })
    const submitButton = within(dialog).getByRole('button', { name: '确认推送' })
    expect(submitButton).toBeDisabled()

    await user.click(within(dialog).getByText('群聊'))
    await waitFor(() => expect(mocks.getGroupTargets).toHaveBeenCalledTimes(1))
    const groupSelect = within(dialog).getByRole('combobox')
    await user.click(groupSelect)
    const groupOption = (await screen.findAllByText('研发群')).find((item) => item.classList.contains('ant-select-item-option-content'))
    await user.click(groupOption as HTMLElement)
    await waitFor(() => expect(submitButton).toBeEnabled())
    expect(within(dialog).getByText(/目标群聊/)).toBeInTheDocument()

    await user.click(submitButton)
    const confirmTitle = (await screen.findAllByText('确认推送到群聊？'))
      .find((item) => item.classList.contains('ant-modal-confirm-title')) as HTMLElement
    const confirmDialog = confirmTitle.closest('.ant-modal') as HTMLElement
    expect(mocks.pushGroupSchedule).not.toHaveBeenCalled()
    await user.click(within(confirmDialog).getByRole('button', { name: '确认推送' }))

    await waitFor(() => expect(mocks.pushGroupSchedule).toHaveBeenCalledTimes(1))
    const formData = mocks.pushGroupSchedule.mock.calls[0][0] as FormData
    expect(formData.get('group_target_id')).toBe('7')
    expect(formData.get('month')).toBe('2026-07')
    expect(formData.get('content')).toContain('【本周六需上班】')
    expect(formData.get('content')).toContain('本周六（2026年8月1日）需上班，请提前安排。')
    expect(formData.get('openConversationId')).toBeNull()
    expect(formData.get('robotCode')).toBeNull()
    expect(mocks.messageSuccess).toHaveBeenCalledWith('已提交')
  })

  it('preserves the personal push flow', async () => {
    mocks.permissions = ['attendance_manage']
    mocks.pushPersonalSchedule.mockResolvedValue({ data: { status: 'success', message: '个人推送完成' } })
    const user = userEvent.setup()
    renderPage()

    await screen.findAllByText('第1周')
    await user.click((await screen.findByText('作息表推送')).closest('button') as HTMLButtonElement)
    const dialog = await screen.findByRole('dialog', { name: '作息表推送' })
    const recipientSelect = within(dialog).getByRole('combobox')
    await user.click(recipientSelect)
    const userOption = (await screen.findAllByText('张三')).find((item) => item.classList.contains('ant-select-item-option-content'))
    await user.click(userOption as HTMLElement)
    await waitFor(() => expect(within(dialog).getByRole('button', { name: '确认推送' })).toBeEnabled())
    await user.click(within(dialog).getByRole('button', { name: '确认推送' }))

    await waitFor(() => expect(mocks.pushPersonalSchedule).toHaveBeenCalledTimes(1))
    const formData = mocks.pushPersonalSchedule.mock.calls[0][0] as FormData
    expect(formData.get('user_ids')).toBe('["u-1"]')
    expect(formData.get('group_target_id')).toBeNull()
  })
})
