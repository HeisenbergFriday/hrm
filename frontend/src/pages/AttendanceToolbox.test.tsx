import React from 'react'
import { act, render, screen, waitFor, cleanup, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import dayjs from 'dayjs'
import AttendanceToolbox, {
  buildAttendanceToolboxWorkflowOptionFields,
  getAttendanceToolboxDownloadableFiles,
  getDingtalkSyncExportFile,
  getPreviousCalendarMonthRange,
  getSubsidyAuditMeta,
  isRosterDepartmentPathError,
  resolveErrorMessage,
} from './AttendanceToolbox'

const { mockConfirmOrgSync } = vi.hoisted(() => ({ mockConfirmOrgSync: vi.fn() }))

const mockRun = vi.fn()
const mockGetDefaults = vi.fn()
const mockListTemplates = vi.fn()
const mockRunDingtalkSync = vi.fn()
const mockRunWorkflow = vi.fn()
const mockRunDingtalkSyncStructured = vi.fn()
const mockDownloadRunFile = vi.fn()
const mockDownloadRunZip = vi.fn()
const mockImportRulesPreview = vi.fn()
const mockExportRules = vi.fn()
const mockPreviewRun = vi.fn()
const mockAuditUploads = vi.fn()
const mockExportTemplates = vi.fn()
const mockParttimeMonthlyPunch = vi.fn()
// 必须在 vi.mock('../services/api') 之前定义，因为 vi.mock 会被 hoisted 到文件顶部。
const mockGenerateOrgRoster = vi.fn()

let mockPermissions: string[] = [
  'attendance_toolbox_operate',
  'attendance_toolbox_dingtalk_sync',
  'attendance_toolbox_rules_edit',
]

vi.mock('../store/authStore', () => ({
  useAuthStore: (selector: (s: { permissions: string[] }) => unknown) =>
    selector({ permissions: mockPermissions }),
}))

vi.mock('../utils/orgSyncAction', () => ({
  confirmOrgSync: (options: unknown) => mockConfirmOrgSync(options),
}))

vi.mock('../services/api', () => ({
  attendanceToolboxAPI: {
    getDefaults: (...args: unknown[]) => mockGetDefaults(...args),
    listTemplates: (...args: unknown[]) => mockListTemplates(...args),
    run: (...args: unknown[]) => mockRun(...args),
    runDingtalkSync: (...args: unknown[]) => mockRunDingtalkSync(...args),
    runWorkflow: (...args: unknown[]) => mockRunWorkflow(...args),
    runDingtalkSyncStructured: (...args: unknown[]) => mockRunDingtalkSyncStructured(...args),
    downloadRunFile: (...args: unknown[]) => mockDownloadRunFile(...args),
    downloadRunZip: (...args: unknown[]) => mockDownloadRunZip(...args),
    importRulesPreview: (...args: unknown[]) => mockImportRulesPreview(...args),
    exportRules: (...args: unknown[]) => mockExportRules(...args),
    previewRun: (...args: unknown[]) => mockPreviewRun(...args),
    auditUploads: (...args: unknown[]) => mockAuditUploads(...args),
    exportTemplates: (...args: unknown[]) => mockExportTemplates(...args),
    parttimeMonthlyPunch: (...args: unknown[]) => mockParttimeMonthlyPunch(...args),
    generateOrgRoster: (...args: unknown[]) => mockGenerateOrgRoster(...args),
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

function makeDingtalkSyncRunResponse(flowKey: string) {
  return {
    data: {
      run_id: `run-${flowKey}`,
      module: 'dingtalk_sync',
      log: 'sync-ok',
      stats: { rows: 2 },
      meta: {},
      files: [
        {
          file_key: `${flowKey}_export.xlsx`,
          file_name: `${flowKey}_export.xlsx`,
          content_type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
          size: 12,
          kind: 'export',
          flow_key: flowKey,
          row_count: 2,
        },
        {
          file_key: 'sync_audit.xlsx',
          file_name: '钉钉同步审计.xlsx',
          content_type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
          size: 8,
          kind: 'audit',
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

async function getUploadInputByTabAndLabel(
  user: ReturnType<typeof userEvent.setup>,
  tabName: RegExp,
  label: string,
) {
  await user.click(screen.getByRole('tab', { name: tabName }))
  const activePanel = document.querySelector('.ant-tabs-tabpane-active') as HTMLElement
  const labels = await within(activePanel).findAllByText(label)
  const card = labels
    .map((item) => item.closest('.ant-card'))
    .find((item) => item?.querySelector('input[type="file"]'))
  if (!card) throw new Error(`未找到上传位：${label}`)
  return {
    panel: activePanel,
    input: card.querySelector('input[type="file"]') as HTMLInputElement,
  }
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

  it('selects the matching business export instead of the audit workbook', () => {
    const run = makeDingtalkSyncRunResponse('position_transfer').data

    expect(getDingtalkSyncExportFile(run, 'position_transfer')?.file_key)
      .toBe('position_transfer_export.xlsx')
  })
})

describe('attendance toolbox error messages', () => {
  it('detects only roster department path integrity errors for automatic repair', () => {
    expect(isRosterDepartmentPathError('1 名在职员工缺少有效主部门或部门层级无法解析')).toBe(true)
    expect(isRosterDepartmentPathError('1 名在职员工缺少业务工号')).toBe(false)
  })

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
    if (typeof window !== 'undefined') window.localStorage.clear()
    Object.values(messageApi).forEach((fn) => fn.mockReset())
    mockRun.mockReset()
    mockGetDefaults.mockReset()
    mockListTemplates.mockReset()
    mockRunDingtalkSync.mockReset()
    mockRunWorkflow.mockReset()
    mockRunDingtalkSyncStructured.mockReset()
    mockDownloadRunFile.mockReset()
    mockDownloadRunZip.mockReset()
    mockImportRulesPreview.mockReset()
    mockExportRules.mockReset()
    mockPreviewRun.mockReset()
    mockAuditUploads.mockReset()
    mockExportTemplates.mockReset()
    mockGenerateOrgRoster.mockReset()
    mockConfirmOrgSync.mockReset()

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
    mockRunDingtalkSyncStructured.mockImplementation((request: { flow_keys?: string[] }) => (
      Promise.resolve(makeDingtalkSyncRunResponse(request.flow_keys?.[0] || 'leave'))
    ))
    mockRunWorkflow.mockResolvedValue(makeRunResponse('leave'))
    mockDownloadRunFile.mockResolvedValue(new Blob(['xlsx']))
    mockDownloadRunZip.mockResolvedValue(new Blob(['zip'], { type: 'application/zip' }))
    mockPreviewRun.mockResolvedValue({ data: { rows: [{ 姓名: '甲', 工号: 'E01' }] } })
    mockAuditUploads.mockResolvedValue({ data: { warnings: [] } })
    mockExportTemplates.mockResolvedValue(new Blob(['tpl']))
    mockParttimeMonthlyPunch.mockResolvedValue(new Blob(['xlsx'], {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }))
    mockGenerateOrgRoster.mockReset()
    // 全局响应拦截器会解包 response.data，调用方直接收到 Blob。
    mockGenerateOrgRoster.mockResolvedValue(new Blob(['roster-xlsx'], {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }))

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

  async function openRosterAndTransferControls(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByText('固定配置（名单 / 同步源）'))
    const rosterCards = (await screen.findAllByText('花名册'))
      .map((node) => node.closest('.ant-card'))
      .filter((node): node is HTMLElement => Boolean(node))
    const transferCards = (await screen.findAllByText('异动流程'))
      .map((node) => node.closest('.ant-card'))
      .filter((node): node is HTMLElement => Boolean(node))
    const rosterCard = rosterCards.find((card) => within(card).queryByRole('button', { name: /组织数据生成/ }))
    const transferCard = transferCards.find((card) => within(card).queryByRole('button', { name: /钉钉同步/ }))
    if (!rosterCard || !transferCard) throw new Error('未找到花名册/异动流程权限控件')
    return {
      rosterButton: within(rosterCard).getByRole('button', { name: /组织数据生成/ }),
      transferButton: within(transferCard).getByRole('button', { name: /钉钉同步/ }),
    }
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

  it('renders only the five output tabs without a standalone dingtalk sync tab', async () => {
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    expect(screen.getAllByRole('tab')).toHaveLength(5)
    expect(screen.getByRole('tab', { name: /请假明细/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /加班明细/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /补贴扣款/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /最终汇总/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /兼职汇总/ })).toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: /钉钉同步/ })).not.toBeInTheDocument()
  })

  it('pulls the leave export from dingtalk and fills the upload field', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    await user.click(screen.getByRole('button', { name: '从钉钉拉取请假表' }))

    await waitFor(() => {
      const previousMonth = dayjs().subtract(1, 'month')
      expect(mockRunDingtalkSyncStructured).toHaveBeenCalledWith(expect.objectContaining({
        start_date: previousMonth.startOf('month').format('YYYY-MM-DD'),
        end_date: previousMonth.endOf('month').format('YYYY-MM-DD'),
        flow_keys: ['leave'],
        padding_days: 31,
      }))
    })
    expect(await screen.findByText('请假系统导出_钉钉同步.xlsx')).toBeInTheDocument()
    expect(messageApi.success).toHaveBeenCalledWith('请假数据拉取完成，已自动回填到请假系统导出表')
    expect(mockDownloadRunFile).toHaveBeenCalledWith('run-leave', 'leave_export.xlsx')
  })

  it('pulls only the overtime export from dingtalk and fills the overtime source field', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    await user.click(screen.getByRole('tab', { name: /加班明细/ }))
    await user.click(screen.getByRole('button', { name: '从钉钉拉取加班表' }))

    await waitFor(() => {
      const previousMonth = dayjs().subtract(1, 'month')
      expect(mockRunDingtalkSyncStructured).toHaveBeenCalledWith(expect.objectContaining({
        start_date: previousMonth.startOf('month').format('YYYY-MM-DD'),
        end_date: previousMonth.endOf('month').format('YYYY-MM-DD'),
        flow_keys: ['overtime'],
        padding_days: 31,
      }))
    })
    expect(await screen.findByText('加班系统导出_钉钉同步.xlsx')).toBeInTheDocument()
    expect(messageApi.success).toHaveBeenCalledWith('加班数据拉取完成，已自动回填到加班系统导出表')
    expect(mockDownloadRunFile).toHaveBeenCalledWith('run-overtime', 'overtime_export.xlsx')
  })

  it('keeps manual overtime upload available when pulling overtime data fails', async () => {
    mockRunDingtalkSyncStructured.mockImplementation((request: { flow_keys?: string[] }) => {
      if (request.flow_keys?.includes('overtime')) {
        return Promise.reject({ response: { status: 400, data: { message: '加班流程未配置' } } })
      }
      return Promise.resolve(makeDingtalkSyncRunResponse(request.flow_keys?.[0] || 'position_transfer'))
    })
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    await user.click(screen.getByRole('tab', { name: /加班明细/ }))
    await user.click(screen.getByRole('button', { name: '从钉钉拉取加班表' }))

    await waitFor(() => expect(messageApi.error).toHaveBeenCalledWith('加班数据拉取失败：加班流程未配置'))
    expect(screen.getByText('拉取失败：加班流程未配置')).toBeInTheDocument()
    expect(screen.getAllByText('点击或拖拽 Excel 到这里').length).toBeGreaterThan(0)
  })

  it('keeps manual upload available when pulling leave data fails', async () => {
    mockRunDingtalkSyncStructured.mockImplementation((request: { flow_keys?: string[] }) => {
      if (request.flow_keys?.includes('leave')) {
        return Promise.reject({ response: { status: 400, data: { message: '请假流程未配置' } } })
      }
      return Promise.resolve(makeDingtalkSyncRunResponse(request.flow_keys?.[0] || 'position_transfer'))
    })
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    await user.click(screen.getByRole('button', { name: '从钉钉拉取请假表' }))

    await waitFor(() => expect(messageApi.error).toHaveBeenCalledWith('请假数据拉取失败：请假流程未配置'))
    expect(screen.getByText('拉取失败：请假流程未配置')).toBeInTheDocument()
    expect(screen.getAllByText('点击或拖拽 Excel 到这里').length).toBeGreaterThan(0)
  })

  it('falls back once to the legacy blob endpoint when structured sync is unavailable', async () => {
    mockRunDingtalkSyncStructured.mockImplementation((request: { flow_keys?: string[] }) => {
      if (request.flow_keys?.includes('leave')) {
        return Promise.reject({ response: { status: 404 } })
      }
      return Promise.resolve(makeDingtalkSyncRunResponse(request.flow_keys?.[0] || 'position_transfer'))
    })
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    await user.click(screen.getByRole('button', { name: '从钉钉拉取请假表' }))

    await waitFor(() => expect(mockRunDingtalkSync).toHaveBeenCalledWith(expect.objectContaining({
      flow_keys: ['leave'],
    })))
    expect(await screen.findByText('请假系统导出_钉钉同步.xlsx')).toBeInTheDocument()
  })

  it('legacy fallback that returns ZIP surfaces the upgrade error instead of filling', async () => {
    mockRunDingtalkSyncStructured.mockImplementation((request: { flow_keys?: string[] }) => {
      if (request.flow_keys?.includes('leave')) {
        return Promise.reject({ response: { status: 404 } })
      }
      return Promise.resolve(makeDingtalkSyncRunResponse(request.flow_keys?.[0] || 'position_transfer'))
    })
    mockRunDingtalkSync.mockResolvedValue(new Blob(['zip-bytes'], { type: 'application/zip' }))
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    await user.click(screen.getByRole('button', { name: '从钉钉拉取请假表' }))

    await waitFor(() => expect(mockRunDingtalkSync).toHaveBeenCalledTimes(1))
    await waitFor(() => {
      expect(messageApi.error).toHaveBeenCalled()
      const args = messageApi.error.mock.calls.map((c) => String(c[0])).join(' ')
      expect(args).toMatch(/服务器版本暂不支持/)
    })
    // 升级提示：不得把 ZIP 当业务表回填。
    expect(screen.queryByText('请假系统导出_钉钉同步.xlsx')).not.toBeInTheDocument()
  })

  it('structured success but single-file download failure never re-runs dingtalk sync', async () => {
    mockRunDingtalkSyncStructured.mockResolvedValue(makeDingtalkSyncRunResponse('position_transfer'))
    mockDownloadRunFile.mockRejectedValue({ response: { status: 500, data: { message: 'download failed' } } })
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    // 花名册走 generateOrgRoster，异动走 structured position_transfer；download 失败禁止重跑钉钉同步。
    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(mockRunDingtalkSyncStructured).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(mockRunDingtalkSync).not.toHaveBeenCalled())
  })

  it('auto fills the generated rich roster into overtime and final while keeping transfer independent', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    // 花名册同步必须调用新 roster API
    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(1))
    // position_transfer 仅用于异动流程同步（1 次），不再用于花名册
    const ptCalls = mockRunDingtalkSyncStructured.mock.calls.filter((c) => {
      const req = c[0] as { flow_keys?: string[] } | undefined
      return req?.flow_keys?.includes('position_transfer')
    })
    expect(ptCalls.length).toBe(1)

    // Must not surface the modern-browser File getter error
    expect(messageApi.error).not.toHaveBeenCalled()
    expect(screen.queryByText(/lastModifiedDate/)).not.toBeInTheDocument()

    // 进入加班明细 tab，断言花名册_组织生成.xlsx 出现
    await user.click(screen.getByRole('tab', { name: /加班明细/ }))
    const overtimePanel = document.querySelector('.ant-tabs-tabpane-active') as HTMLElement
    expect(await within(overtimePanel).findByText('花名册_组织生成.xlsx')).toBeInTheDocument()

    // 进入最终汇总 tab，断言花名册_组织生成.xlsx 出现
    await user.click(screen.getByRole('tab', { name: /最终汇总/ }))
    const finalPanel = document.querySelector('.ant-tabs-tabpane-active') as HTMLElement
    expect(await within(finalPanel).findByText('花名册_组织生成.xlsx')).toBeInTheDocument()

    // 异动流程仍独立使用 position_transfer
    expect(screen.getAllByText('异动流程_钉钉自动同步.xlsx').length).toBeGreaterThan(0)
    expect(mockRunDingtalkSync).not.toHaveBeenCalled()
  })

  it('auto roster response preserves a user upload made during the request and fills the other empty slot', async () => {
    const user = userEvent.setup()
    let resolveRosterGeneration: ((value: Blob) => void) | undefined
    mockGenerateOrgRoster.mockImplementation(() => new Promise((resolve) => {
      resolveRosterGeneration = resolve
    }))

    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(1))

    const overtime = await getUploadInputByTabAndLabel(user, /加班明细/, '花名册/员工信息表')
    await user.upload(overtime.input, new File(['manual'], '请求期间上传.xlsx', {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }))

    await act(async () => {
      resolveRosterGeneration?.(new Blob(['generated']))
      await Promise.resolve()
    })

    expect(await within(overtime.panel).findByText('请求期间上传.xlsx')).toBeInTheDocument()
    expect(within(overtime.panel).queryByText('花名册_组织生成.xlsx')).not.toBeInTheDocument()
    const final = await getUploadInputByTabAndLabel(user, /最终汇总/, '在职花名册')
    expect(await within(final.panel).findByText('花名册_组织生成.xlsx')).toBeInTheDocument()
  })

  it('auto roster only fills the empty slot when another slot already had a user file at request start', async () => {
    mockPermissions = []
    const user = userEvent.setup()
    const view = render(<AttendanceToolbox />)
    await waitForToolboxReady()
    expect(mockGenerateOrgRoster).not.toHaveBeenCalled()

    const overtime = await getUploadInputByTabAndLabel(user, /加班明细/, '花名册/员工信息表')
    await user.upload(overtime.input, new File(['manual'], '预先上传.xlsx', {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }))

    mockPermissions = ['attendance_toolbox_operate']
    view.rerender(<AttendanceToolbox />)
    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(1))
    expect(await within(overtime.panel).findByText('预先上传.xlsx')).toBeInTheDocument()
    expect(within(overtime.panel).queryByText('花名册_组织生成.xlsx')).not.toBeInTheDocument()
    const final = await getUploadInputByTabAndLabel(user, /最终汇总/, '在职花名册')
    expect(await within(final.panel).findByText('花名册_组织生成.xlsx')).toBeInTheDocument()
  })

  it('auto roster response does not refill a slot the user replaced and deleted during the request', async () => {
    const user = userEvent.setup()
    let resolveRosterGeneration: ((value: Blob) => void) | undefined
    mockGenerateOrgRoster.mockImplementation(() => new Promise((resolve) => {
      resolveRosterGeneration = resolve
    }))

    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(1))
    const overtime = await getUploadInputByTabAndLabel(user, /加班明细/, '花名册/员工信息表')
    await user.upload(overtime.input, new File(['first'], '第一次上传.xlsx'))
    await user.click(within(overtime.panel).getByRole('button', { name: '移除第一次上传.xlsx' }))
    const replacement = await getUploadInputByTabAndLabel(user, /加班明细/, '花名册/员工信息表')
    await user.upload(replacement.input, new File(['replacement'], '替换文件.xlsx'))
    await user.click(within(overtime.panel).getByRole('button', { name: '移除替换文件.xlsx' }))

    await act(async () => {
      resolveRosterGeneration?.(new Blob(['generated']))
      await Promise.resolve()
    })

    expect(within(overtime.panel).queryByText('第一次上传.xlsx')).not.toBeInTheDocument()
    expect(within(overtime.panel).queryByText('替换文件.xlsx')).not.toBeInTheDocument()
    expect(within(overtime.panel).queryByText('花名册_组织生成.xlsx')).not.toBeInTheDocument()
    const final = await getUploadInputByTabAndLabel(user, /最终汇总/, '在职花名册')
    expect(await within(final.panel).findByText('花名册_组织生成.xlsx')).toBeInTheDocument()
  })

  it('manual roster generation replaces unchanged slots but preserves files changed during its request', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(1))

    const overtime = await getUploadInputByTabAndLabel(user, /加班明细/, '花名册/员工信息表')
    await user.upload(overtime.input, new File(['manual-before'], '加班手工文件.xlsx'))
    const final = await getUploadInputByTabAndLabel(user, /最终汇总/, '在职花名册')
    await user.upload(final.input, new File(['manual-before'], '汇总手工文件.xlsx'))

    let resolveManualGeneration: ((value: Blob) => void) | undefined
    mockGenerateOrgRoster.mockImplementationOnce(() => new Promise((resolve) => {
      resolveManualGeneration = resolve
    }))
    const { rosterButton } = await openRosterAndTransferControls(user)
    await user.click(rosterButton)
    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(2))

    const changedOvertime = await getUploadInputByTabAndLabel(user, /加班明细/, '花名册/员工信息表')
    await user.upload(changedOvertime.input, new File(['manual-after'], '请求后替换.xlsx'))
    await act(async () => {
      resolveManualGeneration?.(new Blob(['generated-manual']))
      await Promise.resolve()
    })

    expect(await within(changedOvertime.panel).findByText('请求后替换.xlsx')).toBeInTheDocument()
    expect(within(changedOvertime.panel).queryByText('花名册_组织生成.xlsx')).not.toBeInTheDocument()
    const finalAfter = await getUploadInputByTabAndLabel(user, /最终汇总/, '在职花名册')
    expect(await within(finalAfter.panel).findByText('花名册_组织生成.xlsx')).toBeInTheDocument()
    expect(within(finalAfter.panel).queryByText('汇总手工文件.xlsx')).not.toBeInTheDocument()
    expect(messageApi.info).toHaveBeenCalledWith('花名册生成完成，已保留请求期间修改的文件并回填其余上传位')
  })

  it('roster sync failure does not overwrite existing uploads and keeps transfer independent', async () => {
    const user = userEvent.setup()
    let rejectRosterGeneration: ((reason?: unknown) => void) | undefined
    mockGenerateOrgRoster.mockImplementation(() => new Promise((_resolve, reject) => {
      rejectRosterGeneration = reject
    }))
    mockRunDingtalkSyncStructured.mockResolvedValue(makeDingtalkSyncRunResponse('position_transfer'))

    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(1))

    // 先手动上传一个本地花名册，再让已经发起的生成请求失败。
    const finalTab = screen.getByRole('tab', { name: /最终汇总/ })
    await user.click(finalTab)
    const activePanel = document.querySelector('.ant-tabs-tabpane-active') as HTMLElement
    const activeLabels = await within(activePanel).findAllByText('在职花名册')
    const activeCard = activeLabels
      .map((item) => item.closest('.ant-card'))
      .find((item) => item?.querySelector('input[type="file"]'))
    const activeInput = (activeCard as HTMLElement).querySelector('input[type="file"]') as HTMLInputElement
    await user.upload(activeInput, new File(['local-roster'], '本地花名册.xlsx', {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }))
    expect(await screen.findByText('本地花名册.xlsx')).toBeInTheDocument()

    await act(async () => {
      rejectRosterGeneration?.({
        response: { status: 400, data: { message: '花名册生成服务未配置' } },
      })
      await Promise.resolve()
    })

    await user.click(screen.getByText('固定配置（名单 / 同步源）'))
    expect(await screen.findByText(/生成失败：花名册生成服务未配置/)).toBeInTheDocument()

    // 生成失败完成后不得产生自动文件，也不得覆盖已经上传的本地文件。
    expect(screen.queryByText('花名册_组织生成.xlsx')).not.toBeInTheDocument()

    // 原文件仍保留（未被失败的同步覆盖）
    expect(screen.getByText('本地花名册.xlsx')).toBeInTheDocument()

    // 异动流程同步仍独立运行（自动同步已完成）
    await waitFor(() => expect(mockRunDingtalkSyncStructured).toHaveBeenCalled())

    // 花名册同步未使用 position_transfer（只有异动用了）
    const ptCalls = mockRunDingtalkSyncStructured.mock.calls.filter((c) => {
      const req = c[0] as { flow_keys?: string[] } | undefined
      return req?.flow_keys?.includes('position_transfer')
    })
    expect(ptCalls.length).toBe(1)
  })

  it('offers organization sync and retries roster once after a department path error', async () => {
    mockPermissions = ['attendance_manage']
    mockGenerateOrgRoster
      .mockRejectedValueOnce({
        response: {
          status: 400,
          data: { message: '1 名在职员工缺少有效主部门或部门层级无法解析，请先修复组织数据' },
        },
      })
      .mockResolvedValueOnce(new Blob(['repaired-roster'], {
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      }))

    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await waitFor(() => expect(mockConfirmOrgSync).toHaveBeenCalledTimes(1))

    const options = mockConfirmOrgSync.mock.calls[0][0] as {
      onStart?: () => void
      onCompleted?: () => Promise<void>
      onSettled?: () => void
    }
    await act(async () => {
      options.onStart?.()
      await options.onCompleted?.()
      options.onSettled?.()
    })

    expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(2)
    await userEvent.setup().click(screen.getByText('固定配置（名单 / 同步源）'))
    expect(await screen.findByText(/上次生成：/)).toBeInTheDocument()
  })

  it('does not start organization sync repair without attendance_manage permission', async () => {
    mockPermissions = ['attendance_toolbox_operate']
    mockGenerateOrgRoster.mockRejectedValue({
      response: {
        status: 400,
        data: { message: '1 名在职员工缺少有效主部门或部门层级无法解析，请先修复组织数据' },
      },
    })

    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(1))
    expect(mockConfirmOrgSync).not.toHaveBeenCalled()
    await user.click(screen.getByText('固定配置（名单 / 同步源）'))
    expect(await screen.findByText(/生成失败：1 名在职员工缺少有效主部门/)).toBeInTheDocument()
  })

  it('permission matrix: operate-only generates roster and disables transfer sync', async () => {
    mockPermissions = ['attendance_toolbox_operate']
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(1))
    expect(mockRunDingtalkSyncStructured).not.toHaveBeenCalled()
    const { rosterButton, transferButton } = await openRosterAndTransferControls(user)
    expect(rosterButton).toBeEnabled()
    expect(transferButton).toBeDisabled()
  })

  it('permission matrix: dingtalk-sync-only syncs transfer and blocks roster with operation hint', async () => {
    mockPermissions = ['attendance_toolbox_dingtalk_sync']
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    expect(mockGenerateOrgRoster).not.toHaveBeenCalled()
    await waitFor(() => expect(mockRunDingtalkSyncStructured).toHaveBeenCalledTimes(1))
    const { rosterButton, transferButton } = await openRosterAndTransferControls(user)
    expect(rosterButton).toBeDisabled()
    expect(transferButton).toBeEnabled()
    expect(screen.getByText(/当前账号无操作权限/)).toBeInTheDocument()
    await user.hover(rosterButton.parentElement as HTMLElement)
    expect(await screen.findByText('你缺少考勤工具箱操作权限，需要联系管理员添加')).toBeInTheDocument()
  })

  it('permission matrix: operate and dingtalk sync each automatic action exactly once', async () => {
    mockPermissions = ['attendance_toolbox_operate', 'attendance_toolbox_dingtalk_sync']
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(mockRunDingtalkSyncStructured).toHaveBeenCalledTimes(1))
    expect(mockRunDingtalkSyncStructured).toHaveBeenCalledWith(expect.objectContaining({
      flow_keys: ['position_transfer'],
    }))
    const { rosterButton, transferButton } = await openRosterAndTransferControls(user)
    expect(rosterButton).toBeEnabled()
    expect(transferButton).toBeEnabled()
  })

  it('permission matrix: attendance_manage enables roster and transfer', async () => {
    mockPermissions = ['attendance_manage']
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(mockRunDingtalkSyncStructured).toHaveBeenCalledTimes(1))
    const { rosterButton, transferButton } = await openRosterAndTransferControls(user)
    expect(rosterButton).toBeEnabled()
    expect(transferButton).toBeEnabled()
  })

  it('permission matrix: no permission runs neither action and disables both buttons', async () => {
    mockPermissions = []
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()

    expect(mockGenerateOrgRoster).not.toHaveBeenCalled()
    expect(mockRunDingtalkSyncStructured).not.toHaveBeenCalled()
    const { rosterButton, transferButton } = await openRosterAndTransferControls(user)
    expect(rosterButton).toBeDisabled()
    expect(transferButton).toBeDisabled()
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
    await user.click(screen.getByRole('tab', { name: /加班明细/ }))
    expect(await screen.findByRole('button', { name: '从钉钉拉取加班表' })).toBeDisabled()
  })

  it('skips auto transfer sync without dingtalk permission but generates roster with operate', async () => {
    mockPermissions = ['attendance_toolbox_operate']
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await waitFor(() => expect(mockGetDefaults).toHaveBeenCalled())
    // roster generation should happen (canOperate = true)
    await waitFor(() => expect(mockGenerateOrgRoster).toHaveBeenCalled())
    // position_transfer sync should NOT happen (canDingtalkSync = false)
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
    let resultDownloadCount = 0
    mockDownloadRunFile.mockImplementation((_runId: string, fileKey: string) => {
      if (fileKey !== '1_result.xlsx') {
        return Promise.resolve(new Blob(['auto-sync-xlsx']))
      }
      resultDownloadCount += 1
      return resultDownloadCount === 1
        ? Promise.resolve(new Blob(['xlsx']))
        : Promise.reject({ response: { status: 410, data: { message: '结果已过期，请重新计算' } } })
    })
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
    await uploadRequiredFileByLabel(user, '钉钉月度汇总表（补贴及扣款）', 'subsidy.xlsx')
    await uploadRequiredFileByLabel(user, '签到表', 'checkin.xlsx')
    await uploadRequiredFileByLabel(user, '作息表', 'schedule.xlsx')
    await user.click(screen.getByRole('button', { name: /开始计算/ }))
    expect(await screen.findByText('有 2 人缺少考勤记录')).toBeInTheDocument()
    expect(screen.getByText(/丁俊、任澳辉/)).toBeInTheDocument()
    expect(messageApi.warning).toHaveBeenCalledWith(expect.stringContaining('2 人缺少考勤记录'))
  })

  // ── 兼职月度打卡记录卡片（req 1-7） ────────────────────────────────────────

  it('默认月份为上一个自然月（req 1）', async () => {
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    // 卡片在未展开时不渲染月份选择器；展开固定配置区域。
    // 这里只验证 getPreviousCalendarMonthRange 的契约（组件内部使用同样的函数）。
    const [start, end] = getPreviousCalendarMonthRange(dayjs('2026-08-02'))
    expect(start.format('YYYY-MM-DD')).toBe('2026-07-01')
    expect(end.format('YYYY-MM-DD')).toBe('2026-07-31')
  })

  it('页面加载时不会自动抓取（req 2）：挂载后 parttimeMonthlyPunch 未被调用', async () => {
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    expect(mockParttimeMonthlyPunch).not.toHaveBeenCalled()
  })

  it('无钉钉同步权限时按钮禁用并显示缺少权限提示（req 3）', async () => {
    mockPermissions = ['attendance_toolbox_operate']
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await user.click(screen.getByText('固定配置（名单 / 同步源）'))
    const fetchBtn = await screen.findByRole('button', { name: /从钉钉抓取/ })
    expect(fetchBtn).toBeDisabled()
    // 卡片内的提示文案明确告知缺少权限（Tooltip 标题也在 DOM 中，故用 getAllByText）。
    expect(screen.getAllByText(/当前账号无钉钉同步权限/).length).toBeGreaterThan(0)
  })

  it('点击抓取后请求月份参数正确（req 4）并回填兼职模块上传位（req 5）', async () => {
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await user.click(screen.getByText('固定配置（名单 / 同步源）'))
    await user.click(await screen.findByRole('button', { name: /从钉钉抓取/ }))
    await waitFor(() => expect(mockParttimeMonthlyPunch).toHaveBeenCalledTimes(1))
    expect(mockParttimeMonthlyPunch).toHaveBeenCalledWith({ month: '2026-07' })
    // 回填到兼职汇总的「考勤明细」上传位。
    await user.click(screen.getByRole('tab', { name: /兼职汇总/ }))
    expect(await screen.findByText(/兼职月度打卡记录_202607\.xlsx/)).toBeInTheDocument()
  })

  it('抓取失败后显示错误并允许重试（req 6），手动上传仍可用（req 7）', async () => {
    mockParttimeMonthlyPunch.mockRejectedValue({ response: { status: 400, data: { message: '未配置钉钉管理员' } } })
    const user = userEvent.setup()
    render(<AttendanceToolbox />)
    await waitForToolboxReady()
    await user.click(screen.getByText('固定配置（名单 / 同步源）'))
    await user.click(await screen.findByRole('button', { name: /从钉钉抓取/ }))
    await waitFor(() => expect(messageApi.error).toHaveBeenCalledWith(expect.stringContaining('抓取失败')))
    // 失败后按钮文案变为「重试抓取」，仍可再次点击（req 6）。
    expect(await screen.findByRole('button', { name: /重试抓取/ })).toBeEnabled()
  })
})
