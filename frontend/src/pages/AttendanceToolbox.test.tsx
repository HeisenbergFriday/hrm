import React from 'react'
import { render, screen, waitFor, cleanup, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import dayjs from 'dayjs'
import AttendanceToolbox, {
  buildAttendanceToolboxWorkflowOptionFields,
  getAttendanceToolboxDownloadableFiles,
  getPreviousCalendarMonthRange,
  getSubsidyAuditMeta,
  resolveErrorMessage,
} from './AttendanceToolbox'

const mockRun = vi.fn()
const mockGetDefaults = vi.fn()
const mockListTemplates = vi.fn()
const mockRunDingtalkSync = vi.fn()
const mockRunWorkflow = vi.fn()
const mockRunQuickWorkflow = vi.fn()
const mockRunDingtalkSyncStructured = vi.fn()
const mockDownloadRunFile = vi.fn()
const mockDownloadRunZip = vi.fn()
const mockImportRulesPreview = vi.fn()
const mockExportRules = vi.fn()
const mockPreviewRun = vi.fn()
const mockAuditUploads = vi.fn()
const mockExportTemplates = vi.fn()

let mockPermissions: string[] = [
  'attendance_toolbox_operate',
  'attendance_toolbox_dingtalk_sync',
  'attendance_toolbox_rules_edit',
]

vi.mock('../store/authStore', () => ({
  useAuthStore: (selector: (s: { permissions: string[] }) => unknown) =>
    selector({ permissions: mockPermissions }),
}))

vi.mock('../services/api', () => ({
  attendanceToolboxAPI: {
    getDefaults: (...args: unknown[]) => mockGetDefaults(...args),
    listTemplates: (...args: unknown[]) => mockListTemplates(...args),
    run: (...args: unknown[]) => mockRun(...args),
    runDingtalkSync: (...args: unknown[]) => mockRunDingtalkSync(...args),
    runWorkflow: (...args: unknown[]) => mockRunWorkflow(...args),
    runQuickWorkflow: (...args: unknown[]) => mockRunQuickWorkflow(...args),
    runDingtalkSyncStructured: (...args: unknown[]) => mockRunDingtalkSyncStructured(...args),
    downloadRunFile: (...args: unknown[]) => mockDownloadRunFile(...args),
    downloadRunZip: (...args: unknown[]) => mockDownloadRunZip(...args),
    importRulesPreview: (...args: unknown[]) => mockImportRulesPreview(...args),
    exportRules: (...args: unknown[]) => mockExportRules(...args),
    previewRun: (...args: unknown[]) => mockPreviewRun(...args),
    auditUploads: (...args: unknown[]) => mockAuditUploads(...args),
    exportTemplates: (...args: unknown[]) => mockExportTemplates(...args),
  },
}))

// Silence antd message async mount (source of act warnings).
// Factory is hoisted — only vi.hoisted values may be referenced inside.
const { messageApi, messageModule } = vi.hoisted(() => {
  const api = {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
    loading: vi.fn(),
    open: vi.fn(),
    destroy: vi.fn(),
  }
  return {
    messageApi: api,
    messageModule: {
      ...api,
      useMessage: () => [api, null] as const,
    },
  }
})
vi.mock('antd', async () => {
  const actual = await vi.importActual<typeof import('antd')>('antd')
  return {
    ...actual,
    message: messageModule,
  }
})

function makeRunResponse(module = 'leave') {
  return {
    data: {
      run_id: 'run-1',
      module,
      log: 'ok-log',
      stats: { rows: 2 },
      meta: {},
      files: [
        {
          file_key: '1_result.xlsx',
          file_name: '结果.xlsx',
          content_type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
          size: 12,
          kind: 'export',
          row_count: 2,
        },
      ],
      expires_at: '2099-01-01T00:00:00Z',
    },
  }
}

async function uploadLeaveRequiredFiles(user: ReturnType<typeof userEvent.setup>) {
  const exportLabels = await screen.findAllByText('请假系统导出表')
  const exportCard = exportLabels
    .map((item) => item.closest('.ant-card'))
    .find((item) => item?.querySelector('input[type="file"]'))
  const exportInput = (exportCard as HTMLElement).querySelector('input[type="file"]') as HTMLInputElement
  await user.upload(exportInput, new File(['x'], 'leave.xlsx', {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  }))

  const scheduleLabels = screen.getAllByText('作息表')
  const scheduleCard = scheduleLabels
    .map((item) => item.closest('.ant-card'))
    .find((item) => item?.querySelector('input[type="file"]'))
  const scheduleInput = (scheduleCard as HTMLElement).querySelector('input[type="file"]') as HTMLInputElement
  await user.upload(scheduleInput, new File(['y'], 'schedule.xlsx', {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  }))
}

async function uploadRequiredFileByLabel(
  user: ReturnType<typeof userEvent.setup>,
  label: string,
  fileName: string,
) {
  const activePanel = document.querySelector('.ant-tabs-tabpane-active') as HTMLElement
  const labels = await within(activePanel).findAllByText(label)
  const card = labels
    .map((item) => item.closest('.ant-card'))
    .find((item) => item?.querySelector('input[type="file"]'))
  const input = (card as HTMLElement).querySelector('input[type="file"]') as HTMLInputElement
  await user.upload(input, new File(['excel'], fileName, {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  }))
}

describe('getAttendanceToolboxDownloadableFiles', () => {
  it('does not treat meta-only files as user-downloadable results', () => {
    const run = makeRunResponse('dingtalk_sync').data
    run.files = [{
      file_key: 'meta.json',
      file_name: 'dingtalk_sync_meta.json',
      content_type: 'application/json',
      size: 12,
      kind: 'meta',
    }]

    expect(getAttendanceToolboxDownloadableFiles(run)).toEqual([])
  })

  it('keeps real export files while ignoring meta files', () => {
    const run = makeRunResponse('dingtalk_sync').data
    run.files.push({
      file_key: 'meta.json',
      file_name: 'dingtalk_sync_meta.json',
      content_type: 'application/json',
      size: 12,
      kind: 'meta',
    })

    expect(getAttendanceToolboxDownloadableFiles(run).map((file) => file.file_key)).toEqual(['1_result.xlsx'])
  })
})

describe('attendance toolbox error messages', () => {
  it('hides nginx HTML and returns a friendly message for gateway timeouts', async () => {
    const message = await resolveErrorMessage({
      response: {
        status: 504,
        data: new Blob(['<html><head><title>504 Gateway Time-out</title></head></html>']),
      },
    })

    expect(message).toBe('网关等待钉钉响应超时，请稍后重试；若持续出现，请联系管理员检查代理超时配置')
    expect(message).not.toContain('<html>')
  })
})

describe('attendance toolbox workflow options', () => {
  it('uses the previous complete calendar month as the month-end default', () => {
    const [start, end] = getPreviousCalendarMonthRange(dayjs('2026-07-03'))
    expect([start.format('YYYY-MM-DD'), end.format('YYYY-MM-DD')]).toEqual([
      '2026-06-01',
      '2026-06-30',
    ])
  })

  it('passes the month lock and custom rules to subsidy', () => {
    expect(buildAttendanceToolboxWorkflowOptionFields(
      'subsidy',
      { subsidy_target_month: '2026-07' },
      { source: 'custom', rules: null, rulesJson: '{"legal_holidays_override":["2026-07-01"]}' },
    )).toEqual({
      subsidy_target_month: '2026-07',
      rules_json: '{"legal_holidays_override":["2026-07-01"]}',
    })
  })

  it('normalizes subsidy audit metadata', () => {
    const run = makeRunResponse('subsidy').data
    run.meta = {
      subsidy_audit: {
        target_month: '2026-07',
        holiday_source: 'custom_rules',
        holiday_conflict_count: 2,
        missing_attendance_count: 2,
        missing_attendance_names: ['丁俊', '任澳辉'],
      },
    }
    expect(getSubsidyAuditMeta(run)).toEqual({
      targetMonth: '2026-07',
      holidaySource: 'custom_rules',
      holidayConflictCount: 2,
      missingAttendanceCount: 2,
      missingAttendanceNames: ['丁俊', '任澳辉'],
    })
  })
})

describe('AttendanceToolbox', () => {
  beforeEach(() => {
    mockPermissions = [
      'attendance_toolbox_operate',
      'attendance_toolbox_dingtalk_sync',
      'attendance_toolbox_rules_edit',
    ]
    window.localStorage.clear()
    Object.values(messageApi).forEach((fn) => fn.mockReset())
    mockRun.mockReset()
    mockGetDefaults.mockReset()
    mockListTemplates.mockReset()
    mockRunDingtalkSync.mockReset()
    mockRunWorkflow.mockReset()
    mockRunQuickWorkflow.mockReset()
    mockRunDingtalkSyncStructured.mockReset()
    mockDownloadRunFile.mockReset()
    mockDownloadRunZip.mockReset()
    mockImportRulesPreview.mockReset()
    mockExportRules.mockReset()
    mockPreviewRun.mockReset()
    mockAuditUploads.mockReset()
    mockExportTemplates.mockReset()

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
    mockRunWorkflow.mockResolvedValue(makeRunResponse('leave'))
    mockDownloadRunFile.mockResolvedValue(new Blob(['xlsx']))
    mockDownloadRunZip.mockResolvedValue(new Blob(['zip'], { type: 'application/zip' }))
    mockPreviewRun.mockResolvedValue({ data: { rows: [{ 姓名: '甲', 工号: 'E01' }] } })
    mockAuditUploads.mockResolvedValue({ data: { warnings: [] } })
    mockExportTemplates.mockResolvedValue(new Blob(['tpl']))

    vi.stubGlobal('URL', {
      createObjectURL: () => 'blob:mock',
      revokeObjectURL: () => undefined,
    })
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
    vi.clearAllTimers()
  })

  async function waitForToolboxReady() {
    await waitFor(() => expect(mockGetDefaults).toHaveBeenCalled())
    expect((await screen.findAllByText('请假系统导出表')).length).toBeGreaterThan(0)
  }

  it('renders hero, toolbar, tabs, and default text values', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    expect(screen.getByText('考勤数据处理工具')).toBeInTheDocument()
    await waitForToolboxReady()
    // Fixed config is collapsed by default; expand to verify default name lists.
    await user.click(screen.getByText('固定配置（名单 / 同步源）'))
    expect(await screen.findByText('梁伯林')).toBeInTheDocument()
    expect(screen.getByText('陈秋宇')).toBeInTheDocument()
    expect(screen.getAllByText('费婷玉').length).toBeGreaterThan(0)
  })

  it('blocks leave calculation when required files are missing', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await user.click(screen.getByRole('button', { name: /开始计算/ }))
    expect(messageApi.warning).toHaveBeenCalledWith('请上传请假系统导出表')
    expect(mockRunWorkflow).not.toHaveBeenCalled()
  })

  it('shows uploaded files inside the upload card by field label', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    const labels = await screen.findAllByText('请假系统导出表')
    const card = labels
      .map((item) => item.closest('.ant-card'))
      .find((item) => item?.querySelector('input[type="file"]'))
    const input = (card as HTMLElement).querySelector('input[type="file"]') as HTMLInputElement
    await user.upload(input, new File(['excel'], '请假导出表.xlsx', {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }))
    expect(await screen.findByText('请假导出表.xlsx')).toBeInTheDocument()
  })

  it('blocks parttime calculation when the default schedule is missing', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await user.click(screen.getByRole('tab', { name: /兼职汇总/ }))
    await user.click(screen.getByRole('button', { name: /开始计算/ }))
    expect(messageApi.warning).toHaveBeenCalledWith('请上传默认作息表')
  })

  it('renders the upload requirements collapse with field badges', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await user.click(screen.getByText('上传文件要求和模板'))
    expect(await screen.findByText('审批状态')).toBeInTheDocument()
  })

  it('renders dingtalk sync tab with its action button', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await user.click(screen.getByRole('tab', { name: /钉钉同步/ }))
    expect(await screen.findByRole('button', { name: /从钉钉同步并生成中间表/ })).toBeInTheDocument()
    const dateInputs = await screen.findAllByLabelText('钉钉同步日期范围')
    const previousMonth = dayjs().subtract(1, 'month')
    expect(dateInputs.map((input) => (input as HTMLInputElement).value)).toEqual([
      previousMonth.startOf('month').format('YYYY-MM-DD'),
      previousMonth.endOf('month').format('YYYY-MM-DD'),
    ])
  })

  it('pulls the leave export from dingtalk and fills the upload field', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    await user.click(screen.getByRole('button', { name: '从钉钉拉取请假表' }))

    await waitFor(() => {
      const previousMonth = dayjs().subtract(1, 'month')
      expect(mockRunDingtalkSync).toHaveBeenCalledWith(expect.objectContaining({
        start_date: previousMonth.startOf('month').format('YYYY-MM-DD'),
        end_date: previousMonth.endOf('month').format('YYYY-MM-DD'),
        flow_keys: ['leave'],
        padding_days: 31,
      }))
    })
    expect(await screen.findByText('请假系统导出_钉钉同步.xlsx')).toBeInTheDocument()
    expect(messageApi.success).toHaveBeenCalledWith('请假数据拉取完成，已自动回填到请假系统导出表')
  })

  it('keeps manual upload available when pulling leave data fails', async () => {
    mockRunDingtalkSync.mockImplementation((request: { flow_keys?: string[] }) => {
      if (request.flow_keys?.includes('leave')) {
        return Promise.reject({ response: { status: 400, data: { message: '请假流程未配置' } } })
      }
      return Promise.resolve(new Blob(['excel'], {
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      }))
    })
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    await user.click(screen.getByRole('button', { name: '从钉钉拉取请假表' }))

    await waitFor(() => expect(messageApi.error).toHaveBeenCalledWith('请假数据拉取失败：请假流程未配置'))
    expect(screen.getByText('拉取失败：请假流程未配置')).toBeInTheDocument()
    expect(screen.getAllByText('点击或拖拽 Excel 到这里').length).toBeGreaterThan(0)
  })

  it('auto syncs roster and transfer files while keeping manual upload flows available', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await waitFor(() => expect(mockRunDingtalkSync).toHaveBeenCalledTimes(2))
    // Must not surface the modern-browser File getter error:
    // "Cannot set property lastModifiedDate of #<File> which has only a getter"
    expect(messageApi.error).not.toHaveBeenCalled()
    expect(screen.queryByText(/lastModifiedDate/)).not.toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: /加班明细/ }))
    expect(await screen.findByText('花名册_钉钉自动同步.xlsx')).toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: /最终汇总/ }))
    expect(await screen.findByText('异动流程_钉钉自动同步.xlsx')).toBeInTheDocument()
  })

  it('disables calculate button without operate permission', async () => {
    mockPermissions = ['attendance_toolbox_dingtalk_sync']
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    expect(screen.getByRole('button', { name: /开始计算/ })).toBeDisabled()
  })

  it('disables dingtalk sync without sync permission', async () => {
    mockPermissions = ['attendance_toolbox_operate']
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    // 无钉钉同步权限时不应自动拉花名册/异动
    expect(mockRunDingtalkSync).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: '从钉钉拉取请假表' })).toBeDisabled()
    await user.click(screen.getByRole('tab', { name: /钉钉同步/ }))
    expect(await screen.findByRole('button', { name: /从钉钉同步并生成中间表/ })).toBeDisabled()
  })

  it('skips auto roster/transfer sync without dingtalk permission', async () => {
    mockPermissions = ['attendance_toolbox_operate']
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await waitFor(() => expect(mockGetDefaults).toHaveBeenCalled())
    expect(mockRunDingtalkSync).not.toHaveBeenCalled()
    expect(messageApi.error).not.toHaveBeenCalled()
  })

  it('does not fallback to legacy on 403', async () => {
    mockRunWorkflow.mockRejectedValue({ response: { status: 403, data: { message: 'permission denied' } } })
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await uploadLeaveRequiredFiles(user)
    await user.click(screen.getByRole('button', { name: /开始计算/ }))
    await waitFor(() => expect(mockRunWorkflow).toHaveBeenCalled())
    expect(mockRun).not.toHaveBeenCalled()
    await waitFor(() => expect(messageApi.error).toHaveBeenCalled())
  })

  it('falls back to legacy only on 404', async () => {
    mockRunWorkflow.mockRejectedValue({ response: { status: 404 } })
    mockRun.mockResolvedValue(new Blob(['legacy'], {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }))
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await uploadLeaveRequiredFiles(user)
    await user.click(screen.getByRole('button', { name: /开始计算/ }))
    await waitFor(() => expect(mockRunWorkflow).toHaveBeenCalled())
    await waitFor(() => expect(mockRun).toHaveBeenCalled())
  })

  it('structured success shows stats and download buttons', async () => {
    mockRunWorkflow.mockResolvedValue(makeRunResponse('leave'))
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await uploadLeaveRequiredFiles(user)
    await user.click(screen.getByRole('button', { name: /开始计算/ }))
    await waitFor(() => expect(mockDownloadRunFile).toHaveBeenCalled())
    expect(await screen.findByText('计算成功')).toBeInTheDocument()
    expect(screen.getByText((content) => content.includes('rows') && content.includes('2'))).toBeInTheDocument()
    expect(screen.getByText('技术信息')).toBeInTheDocument()
  })

  it('410 expired download shows explicit message', async () => {
    mockRunWorkflow.mockResolvedValue(makeRunResponse('leave'))
    mockDownloadRunFile
      .mockResolvedValueOnce(new Blob(['xlsx']))
      .mockRejectedValueOnce({ response: { status: 410, data: { message: '结果已过期，请重新计算' } } })
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await uploadLeaveRequiredFiles(user)
    await user.click(screen.getByRole('button', { name: /开始计算/ }))
    await waitFor(() => expect(screen.getByText('计算成功')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: /结果\.xlsx/ }))
    await waitFor(() => {
      expect(messageApi.error).toHaveBeenCalled()
      const args = messageApi.error.mock.calls.map((c) => String(c[0])).join(' ')
      expect(args).toMatch(/过期/)
    })
  })

  it('renders the manual long maternity leave entry', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    expect(screen.getByText('长期产假人员（自动抓不到时兜底）')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /添加人员/ }))
    expect(await screen.findByText('长期产假人员 1')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('请输入员工姓名')).toBeInTheDocument()
  })

  it('submits configured long maternity leave with leave calculation', async () => {
    window.localStorage.setItem('attendance-toolbox-maternity-leave-overrides', JSON.stringify([{
      key: 'maternity-test',
      employee_no: '',
      name: '长期产假员工',
      start_date: '2026-05-20',
      end_date: '2026-07-10',
    }]))
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await uploadLeaveRequiredFiles(user)
    await user.click(screen.getByRole('button', { name: /开始计算/ }))

    await waitFor(() => expect(mockRunWorkflow).toHaveBeenCalled())
    const [moduleKey, formData] = mockRunWorkflow.mock.calls[0] as [string, FormData]
    expect(moduleKey).toBe('leave')
    expect(JSON.parse(String(formData.get('maternity_leave_overrides')))).toEqual([{
      employee_no: '',
      name: '长期产假员工',
      start_date: '2026-05-20',
      end_date: '2026-07-10',
    }])
  })

  it('renders month-end wizard steps', async () => {
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    expect(screen.getByTestId('attendance-toolbox-wizard')).toBeInTheDocument()
    expect(screen.getByText('月末结账向导')).toBeInTheDocument()
  })

  it('renders optional month locks for overtime and subsidy', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await user.click(screen.getByRole('tab', { name: /加班明细/ }))
    expect(await screen.findByLabelText('加班处理月份')).toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: /补贴扣款/ }))
    expect(await screen.findByLabelText('补贴考勤月份')).toBeInTheDocument()
  })

  it('shows missing attendance warning from subsidy audit metadata', async () => {
    const response = makeRunResponse('subsidy')
    response.data.meta = {
      subsidy_audit: {
        target_month: '2026-07',
        holiday_source: 'schedule',
        holiday_conflict_count: 0,
        missing_attendance_count: 2,
        missing_attendance_names: ['丁俊', '任澳辉'],
      },
    }
    mockRunWorkflow.mockResolvedValue(response)
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await user.click(screen.getByRole('tab', { name: /补贴扣款/ }))
    await uploadRequiredFileByLabel(user, '补贴扣款表', 'subsidy.xlsx')
    await uploadRequiredFileByLabel(user, '签到表', 'checkin.xlsx')
    await uploadRequiredFileByLabel(user, '作息表', 'schedule.xlsx')
    await user.click(screen.getByRole('button', { name: /开始计算/ }))
    expect(await screen.findByText('有 2 人缺少考勤记录')).toBeInTheDocument()
    expect(screen.getByText(/丁俊、任澳辉/)).toBeInTheDocument()
    expect(messageApi.warning).toHaveBeenCalledWith(expect.stringContaining('2 人缺少考勤记录'))
  })
})
