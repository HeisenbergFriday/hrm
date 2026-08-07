import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Checkbox,
  Col,
  Collapse,
  DatePicker,
  Divider,
  Empty,
  Input,
  InputNumber,
  Popover,
  Radio,
  Row,
  Space,
  Steps,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography,
  Upload,
  message,
} from 'antd'
import type { UploadFile, UploadProps } from 'antd'
import type { RcFile } from 'antd/es/upload/interface'
import type { RangePickerProps } from 'antd/es/date-picker'
import {
  CalculatorOutlined,
  CheckCircleOutlined,
  CheckOutlined,
  CloseCircleOutlined,
  CloseOutlined,
  CloudDownloadOutlined,
  CopyOutlined,
  DownloadOutlined,
  ExclamationCircleOutlined,
  FileExcelOutlined,
  FileProtectOutlined,
  InfoCircleOutlined,
  PlusOutlined,
  SyncOutlined,
  ToolOutlined,
  UploadOutlined,
} from '@ant-design/icons'
import axios from 'axios'
import dayjs from 'dayjs'
import PageContainer from '../components/PageContainer'
import OvertimeRulesEditor, { AppliedRulesState } from './OvertimeRulesEditor'
import { attendanceToolboxAPI, AttendanceToolboxRunResponse } from '../services/api'
import { useAuthStore } from '../store/authStore'
import { shouldFallbackToLegacyToolboxAPI } from '../utils/toolboxFallback'

const { Text, Title, Paragraph } = Typography
const { Dragger } = Upload
const { RangePicker } = DatePicker

type ToolboxModuleKey = 'leave' | 'overtime' | 'subsidy' | 'final' | 'parttime' | 'dingtalk_sync'
type ToolboxDefaults = Record<string, string[]>
type SyncMode = 'auto' | 'manual'
type SyncState = {
  loading: boolean
  lastSyncAt?: string
  error?: string
}
type DingtalkSyncSelectionRequest = {
  start_date: string
  end_date: string
  flow_keys: string[]
  max_instances?: number
  padding_days?: number
}

interface FileField {
  name: string
  label: string
  required?: boolean
  multiple?: boolean
  templateId?: string
}

interface TemplateMetaItem {
  id: string
  file_name: string
}

interface TextField {
  name: string
  label: string
  placeholder: string
}

interface ModuleConfig {
  key: ToolboxModuleKey
  title: string
  description: string
  outputName: string
  zipOutputName?: string
  fileFields: FileField[]
  textFields?: TextField[]
}

interface MaternityLeaveOverride {
  key: string
  employee_no: string
  name: string
  start_date: string
  end_date: string
}

export const getAttendanceToolboxDownloadableFiles = (run: AttendanceToolboxRunResponse | null | undefined) =>
  (run?.files || []).filter((file) => file.kind !== 'meta')

export const getDingtalkSyncExportFile = (
  run: AttendanceToolboxRunResponse | null | undefined,
  flowKey: string,
) => {
  const files = run?.files || []
  // 严格匹配 kind=export 且 flow_key 精确等于目标 flow_key，禁止回退到第一个 export。
  return files.find((file) => file.kind === 'export' && file.flow_key === flowKey)
}

export interface SubsidyAuditMeta {
  targetMonth: string
  holidaySource: 'custom_rules' | 'schedule'
  holidayConflictCount: number
  missingAttendanceCount: number
  missingAttendanceNames: string[]
}

export const getSubsidyAuditMeta = (
  run: AttendanceToolboxRunResponse | null | undefined,
): SubsidyAuditMeta | null => {
  const raw = run?.meta?.subsidy_audit
  if (!raw || typeof raw !== 'object') return null
  const value = raw as Record<string, unknown>
  return {
    targetMonth: String(value.target_month || ''),
    holidaySource: value.holiday_source === 'custom_rules' ? 'custom_rules' : 'schedule',
    holidayConflictCount: Number(value.holiday_conflict_count || 0),
    missingAttendanceCount: Number(value.missing_attendance_count || 0),
    missingAttendanceNames: Array.isArray(value.missing_attendance_names)
      ? value.missing_attendance_names.map((item) => String(item)).filter(Boolean)
      : [],
  }
}

export const buildAttendanceToolboxWorkflowOptionFields = (
  moduleKey: ToolboxModuleKey,
  textValues: Record<string, string>,
  appliedRules: AppliedRulesState,
): Record<string, string> => {
  const result: Record<string, string> = {}
  if (moduleKey === 'overtime') {
    const month = (textValues.overtime_target_month || '').trim()
    if (month) result.overtime_target_month = month
  }
  if (moduleKey === 'subsidy') {
    const month = (textValues.subsidy_target_month || '').trim()
    if (month) result.subsidy_target_month = month
  }
  if (
    (moduleKey === 'overtime' || moduleKey === 'subsidy')
    && appliedRules.source === 'custom'
    && appliedRules.rulesJson
  ) {
    result.rules_json = appliedRules.rulesJson
  }
  return result
}

export const getPreviousCalendarMonthRange = (
  reference: dayjs.Dayjs = dayjs(),
): [dayjs.Dayjs, dayjs.Dayjs] => {
  const previousMonth = reference.subtract(1, 'month')
  return [previousMonth.startOf('month'), previousMonth.endOf('month')]
}

const MATERNITY_LEAVE_STORAGE_KEY = 'attendance-toolbox-maternity-leave-overrides'

const loadMaternityLeaveOverrides = (): MaternityLeaveOverride[] => {
  if (typeof window === 'undefined') return []
  try {
    const value = JSON.parse(window.localStorage.getItem(MATERNITY_LEAVE_STORAGE_KEY) || '[]')
    if (!Array.isArray(value)) return []
    return value.filter((item) => item && typeof item === 'object').map((item, index) => ({
      key: String(item.key || `maternity-${Date.now()}-${index}`),
      employee_no: String(item.employee_no || ''),
      name: String(item.name || ''),
      start_date: String(item.start_date || ''),
      end_date: String(item.end_date || ''),
    }))
  } catch {
    return []
  }
}

/** Recommended month-end order (UI guide only; tabs stay free to switch). */
const MONTH_END_WIZARD_STEPS: Array<{ key: ToolboxModuleKey; title: string; hint: string }> = [
  { key: 'dingtalk_sync', title: '钉钉同步', hint: '拉审批中间表' },
  { key: 'leave', title: '请假明细', hint: '生成请假表' },
  { key: 'overtime', title: '加班明细', hint: '生成加班回填' },
  { key: 'subsidy', title: '补贴扣款', hint: '生成核对表' },
  { key: 'final', title: '最终汇总', hint: '生成最终表' },
]

const modules: ModuleConfig[] = [
  {
    key: 'leave',
    title: '请假明细',
    description: '从钉钉拉取或上传请假系统导出表，并配合作息表生成请假明细表。',
    outputName: '请假明细表.xlsx',
    fileFields: [
      { name: 'leave_src', label: '请假系统导出表', required: true, templateId: 'leave_export' },
      { name: 'leave_schedule', label: '作息表', required: true, templateId: 'schedule' },
      { name: 'leave_offsite_duration', label: '月度汇总表（异地外勤，可选）', templateId: 'offsite_duration' },
    ],
    textFields: [
      { name: 'leave_special_names', label: '特殊名单', placeholder: '不填则使用原工具默认特殊名单，多个姓名用逗号分隔' },
      { name: 'chengdu_schedule_names', label: '成都作息名单', placeholder: '不填则使用原工具默认成都作息名单，多个姓名用逗号分隔' },
    ],
  },
  {
    key: 'overtime',
    title: '加班明细',
    description: '上传加班导出表，可附加排班、考勤、作息、花名册，生成回填表。',
    outputName: '加班明细_回填.xlsx',
    fileFields: [
      { name: 'overtime_src', label: '加班系统导出表', required: true, templateId: 'overtime_export' },
      { name: 'overtime_schedules', label: '排班表（可多选）', multiple: true, templateId: 'overtime_schedule' },
      { name: 'overtime_attendance', label: '考勤打卡明细表', templateId: 'overtime_attendance' },
      { name: 'overtime_calendar', label: '作息表', templateId: 'schedule' },
      { name: 'overtime_roster', label: '花名册/员工信息表', templateId: 'roster' },
    ],
    textFields: [
      { name: 'chengdu_schedule_names', label: '成都作息名单', placeholder: '不填则使用原工具默认成都作息名单，多个姓名用逗号分隔' },
    ],
  },
  {
    key: 'subsidy',
    title: '补贴扣款',
    description: '数据来源：钉钉考勤打卡 → 考勤统计 → 报表管理 → 月度汇总表（补贴及扣款）。请在钉钉后台导出对应月份的Excel后上传，配合作息、签到和考勤数据生成核对表。',
    outputName: '补贴扣款核对表.xlsx',
    zipOutputName: '补贴扣款结果.zip',
    fileFields: [
      { name: 'subsidy_src', label: '钉钉月度汇总表（补贴及扣款）', required: true, templateId: 'subsidy_source' },
      { name: 'subsidy_checkin', label: '签到表', required: true, templateId: 'activity_checkin' },
      { name: 'subsidy_schedule', label: '作息表', required: true, templateId: 'schedule' },
      { name: 'subsidy_attendance', label: '考勤表', templateId: 'subsidy_attendance' },
      { name: 'subsidy_attendance_result', label: '考勤结果表', templateId: 'attendance_result' },
    ],
    textFields: [
      { name: 'sub_dept_keywords', label: '产研部门关键字', placeholder: '不填则使用原工具默认部门关键字，多个用逗号分隔' },
      { name: 'sub_late22_names', label: '晚走补贴人员', placeholder: '不填则使用原工具默认人员名单，多个姓名用逗号分隔' },
    ],
  },
  {
    key: 'final',
    title: '最终汇总',
    description: '汇总花名册、作息、请假、加班、补贴扣款结果，生成最终考勤表。',
    outputName: '最终表.xlsx',
    fileFields: [
      { name: 'final_active', label: '在职花名册', required: true, templateId: 'roster' },
      { name: 'final_resign', label: '离职花名册', templateId: 'roster' },
      { name: 'final_transfer', label: '异动流程表', templateId: 'transfer' },
      { name: 'final_schedule', label: '作息表', required: true, templateId: 'schedule' },
      { name: 'final_leave', label: '请假明细表', required: true, templateId: 'final_leave_detail' },
      { name: 'final_overtime', label: '加班明细表', required: true, templateId: 'final_overtime_detail' },
      { name: 'final_subsidy', label: '补贴扣款表', required: true, templateId: 'subsidy_source' },
    ],
    textFields: [
      { name: 'chengdu_schedule_names', label: '成都作息名单', placeholder: '不填则使用原工具默认成都作息名单，多个姓名用逗号分隔' },
    ],
  },
  {
    key: 'parttime',
    title: '兼职汇总',
    description: '上传兼职月度汇总、排班和默认作息，生成兼职汇总表。',
    outputName: '兼职汇总.xlsx',
    fileFields: [
      { name: 'parttime_default_schedule', label: '默认作息表', required: true, templateId: 'schedule' },
      { name: 'parttime_attendance_detail', label: '考勤明细', templateId: 'parttime_monthly_summary' },
      { name: 'parttime_monthly', label: '腾小宝月度汇总（可多选）', multiple: true, templateId: 'parttime_monthly_summary' },
      { name: 'parttime_schedules', label: '排班表（可多选）', multiple: true, templateId: 'parttime_schedule' },
    ],
    textFields: [
      { name: 'part_special_names', label: '特殊人员名单', placeholder: '不填则使用原工具默认特殊人员名单，多个姓名用逗号分隔' },
    ],
  },
  {
    key: 'dingtalk_sync',
    title: '钉钉同步',
    description: '从钉钉同步审批数据，生成中间表用于后续考勤计算。',
    outputName: '钉钉同步结果.xlsx',
    zipOutputName: '钉钉同步结果.zip',
    fileFields: [],
  },
]

const templateDownloadItems: Array<{ key: string; label: string; templateId: string }> = [
  { key: 'leave_export', label: '请假系统导出表', templateId: 'leave_export' },
  { key: 'schedule', label: '作息表', templateId: 'schedule' },
  { key: 'overtime_export', label: '加班系统导出表', templateId: 'overtime_export' },
  { key: 'overtime_schedule', label: '加班排班表', templateId: 'overtime_schedule' },
  { key: 'overtime_attendance', label: '加班考勤打卡明细表', templateId: 'overtime_attendance' },
  { key: 'subsidy_source', label: '补贴扣款表', templateId: 'subsidy_source' },
  { key: 'activity_checkin', label: '活动签到表', templateId: 'activity_checkin' },
  { key: 'subsidy_attendance', label: '考勤表', templateId: 'subsidy_attendance' },
  { key: 'attendance_result', label: '考勤结果表', templateId: 'attendance_result' },
  { key: 'roster', label: '花名册', templateId: 'roster' },
  { key: 'transfer', label: '异动流程表', templateId: 'transfer' },
  { key: 'final_leave_detail', label: '请假明细表', templateId: 'final_leave_detail' },
  { key: 'final_overtime_detail', label: '加班明细表', templateId: 'final_overtime_detail' },
  { key: 'offsite_duration', label: '异地不打卡人员表', templateId: 'offsite_duration' },
  { key: 'parttime_monthly_summary', label: '腾小宝月度汇总', templateId: 'parttime_monthly_summary' },
  { key: 'parttime_schedule', label: '兼职排班表', templateId: 'parttime_schedule' },
]

const acceptExcel = '.xls,.xlsx'

function normalizeDefaults(response: unknown): ToolboxDefaults {
  const payload = (response as { data?: unknown } | null)?.data ?? response
  if (!payload || typeof payload !== 'object') return {}

  return Object.entries(payload as Record<string, unknown>).reduce<ToolboxDefaults>((result, [key, value]) => {
    if (Array.isArray(value)) {
      result[key] = value.map((item) => String(item).trim()).filter(Boolean)
    }
    return result
  }, {})
}

function formatNameList(values: string[]) {
  return values.join('、')
}

/** Exported for unit tests to mock without triggering jsdom navigation. */
export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.rel = 'noopener'
  // Prefer programmatic download without relying on default navigation behavior.
  link.style.display = 'none'
  document.body.appendChild(link)
  try {
    link.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window }))
  } catch {
    // jsdom may not implement navigation; ignore.
  }
  link.remove()
  URL.revokeObjectURL(url)
}

function getDownloadName(config: ModuleConfig, blob: Blob) {
  if (blob.type === 'application/zip' && config.zipOutputName) {
    return config.zipOutputName
  }
  return config.outputName
}

export async function resolveErrorMessage(error: unknown) {
  const status =
    axios.isAxiosError(error)
      ? error.response?.status
      : (error as { response?: { status?: number } } | null)?.response?.status

  if (status === 504) {
    return '网关等待钉钉响应超时，请稍后重试；若持续出现，请联系管理员检查代理超时配置'
  }
  if (status === 502 || status === 503) {
    return '服务网关暂时不可用，请稍后重试'
  }
  if (status === 410) {
    return '结果已过期，请重新计算'
  }
  if (status === 403) {
    const data = axios.isAxiosError(error)
      ? error.response?.data
      : (error as { response?: { data?: unknown } })?.response?.data
    if (typeof data === 'object' && data && 'message' in data && typeof (data as { message?: string }).message === 'string') {
      return (data as { message: string }).message
    }
    return '权限不足，需要联系管理员添加'
  }

  if (axios.isAxiosError(error)) {
    if (error.code === 'ECONNABORTED' || error.code === 'ETIMEDOUT') {
      return '请求等待超时，请稍后重试；若持续出现，请联系管理员检查长任务超时配置'
    }
    const data = error.response?.data
    if (data instanceof Blob) {
      const text = await data.text()
      try {
        const parsed = JSON.parse(text)
        return parsed?.message || parsed?.error || '计算失败'
      } catch {
        if (/<!doctype\s+html|<html[\s>]/i.test(text)) {
          return '服务网关返回异常，请稍后重试'
        }
        return text.length > 500 ? `${text.slice(0, 500)}…` : (text || '计算失败')
      }
    }
    if (typeof data?.message === 'string') return data.message
  }

  const plain = error as { response?: { data?: { message?: string } }; message?: string } | null
  if (typeof plain?.response?.data?.message === 'string') {
    return plain.response.data.message
  }
  return error instanceof Error ? error.message : '计算失败'
}

// ── Field badge data per module (for the upload requirements collapse) ──

type FieldBadge = { label: string; required?: boolean }

const fieldRequirements: Record<string, { label: string; badges: FieldBadge[] }[]> = {
  leave: [
    { label: '请假系统导出表', badges: [
      { label: '审批状态', required: true }, { label: '审批结果', required: true },
      { label: '发起人工号', required: true }, { label: '发起人姓名', required: true },
      { label: '发起人部门', required: true }, { label: '请假类型', required: true },
      { label: '开始时间', required: true }, { label: '结束时间', required: true },
      { label: '时长', required: true }, { label: '审批编号', required: true },
      { label: '发起时间' }, { label: '完成时间' }, { label: '岗位名称' },
    ]},
    { label: '作息表', badges: [
      { label: '作息时间表', required: true }, { label: '周数', required: true },
      { label: '黄色工作日', required: true }, { label: '红色法定节假日', required: true },
      { label: '蓝色公司福利假', required: true },
    ]},
    { label: '特殊名单', badges: [{ label: '工号' }]},
    { label: '月度汇总表（异地外勤）', badges: [
      { label: '工号' }, { label: '姓名' }, { label: '请假时长' }, { label: '审批编号' },
    ]},
  ],
  overtime: [
    { label: '加班系统导出表', badges: [
      { label: '审批状态', required: true }, { label: '审批结果', required: true },
      { label: '发起人工号', required: true }, { label: '发起人姓名', required: true },
      { label: '发起人部门', required: true }, { label: '开始时间', required: true },
      { label: '结束时间', required: true }, { label: '时长', required: true },
      { label: '审批编号' }, { label: '明细' }, { label: '加班时间' },
    ]},
    { label: '排班表', badges: [
      { label: '日期行' }, { label: '星期行' }, { label: '姓名' }, { label: '排班内容' }, { label: 'OFF 或空白' },
    ]},
    { label: '考勤打卡明细表', badges: [
      { label: '姓名', required: true }, { label: '工号', required: true },
      { label: '考勤组' }, { label: '考勤结果' }, { label: '日期行' }, { label: '打卡时间' },
    ]},
    { label: '花名册/员工信息表', badges: [
      { label: '工号', required: true }, { label: '姓名', required: true },
      { label: '一级部门' }, { label: '二级部门' }, { label: '三级部门' }, { label: '部门路径' },
    ]},
  ],
  subsidy: [
    { label: '补贴扣款表', badges: [
      { label: '姓名', required: true }, { label: '工号', required: true },
      { label: '考勤组', required: true }, { label: '一级部门', required: true },
      { label: '二级部门', required: true }, { label: '三级部门', required: true },
      { label: '旷工天数', required: true }, { label: '缺卡次数', required: true },
      { label: '晚走补贴', required: true }, { label: '产研休息日加班补贴', required: true },
    ]},
    { label: '签到表', badges: [
      { label: 'sheet 含羽毛球或篮球', required: true }, { label: '姓名', required: true },
      { label: '活动日期', required: true }, { label: '已参加', required: true },
    ]},
    { label: '考勤表', badges: [
      { label: '姓名' }, { label: '工号' }, { label: '日期行' }, { label: '打卡时间' }, { label: '缺卡' },
    ]},
    { label: '考勤结果表', badges: [
      { label: '第 3 行日期' }, { label: '第 4 行起员工数据' }, { label: '姓名' }, { label: '批注：已补流程' },
    ]},
  ],
  final: [
    { label: '在职花名册', badges: [
      { label: '姓名', required: true },
    ]},
    { label: '作息表', badges: [
      { label: '作息时间表', required: true }, { label: '周数', required: true },
      { label: '黄色工作日', required: true }, { label: '红色法定节假日', required: true },
    ]},
    { label: '请假明细表', badges: [
      { label: '请假类型', required: true }, { label: '最终请假天数', required: true },
      { label: '发起人工号' }, { label: '发起人姓名' }, { label: '开始时间' }, { label: '结束时间' },
    ]},
    { label: '加班明细表', badges: [
      { label: '2倍加班小时' }, { label: '3倍加班小时' },
      { label: '2倍加班天数' }, { label: '3倍加班天数' },
      { label: '发起人工号' }, { label: '发起人姓名' },
    ]},
    { label: '钉钉月度汇总表（补贴及扣款）', badges: [
      { label: '姓名', required: true }, { label: '工号' }, { label: '考勤组' },
      { label: '部门' }, { label: '岗位' }, { label: '旷工天数', required: true },
    ]},
  ],
  parttime: [
    { label: '默认作息表', badges: [
      { label: '作息时间表', required: true }, { label: '周数', required: true },
      { label: '黄色工作日', required: true }, { label: '成都作息时间表' },
    ]},
    { label: '腾小宝月度汇总', badges: [
      { label: '姓名', required: true }, { label: '出勤天数', required: true },
      { label: '考勤结果', required: true }, { label: '工号' }, { label: '考勤组' }, { label: '部门' },
    ]},
    { label: '排班表', badges: [
      { label: '姓名' }, { label: '所属公司' }, { label: '日期列' }, { label: '班' }, { label: '休' },
    ]},
  ],
}

// ── Hero badge data ──

const heroBadges = [
  { label: 'Excel 导入', icon: <UploadOutlined /> },
  { label: '规则校验', icon: <FileProtectOutlined /> },
  { label: '结果导出', icon: <CloudDownloadOutlined /> },
]

const AUTO_SYNC_UPLOADS = {
  leave: ['leave_src'],
  roster: ['overtime_roster', 'final_active'],
  transfer: ['final_transfer'],
} as const

const LEAVE_SYNC_PADDING_DAYS = 31

// ── Main component ──

const AttendanceToolbox: React.FC = () => {
  const [messageApi, messageContextHolder] = message.useMessage()
  const permissions = useAuthStore((s) => s.permissions) || []
  const hasPerm = useCallback((code: string) => {
    if (permissions.includes(code)) return true
    // Compat: legacy attendance_manage still enables toolbox actions.
    if (code.startsWith('attendance_toolbox_') && permissions.includes('attendance_manage')) return true
    return false
  }, [permissions])
  const canOperate = hasPerm('attendance_toolbox_operate') || hasPerm('attendance_manage')
  const canDingtalkSync = hasPerm('attendance_toolbox_dingtalk_sync') || hasPerm('attendance_manage')
  const canEditRules = hasPerm('attendance_toolbox_rules_edit') || hasPerm('attendance_manage')
  const canQuick = canOperate && canDingtalkSync

  const [activeModule, setActiveModule] = useState<ToolboxModuleKey>('leave')
  const [completedModules, setCompletedModules] = useState<Partial<Record<ToolboxModuleKey | 'quick', boolean>>>({})
  const [missingFieldName, setMissingFieldName] = useState<string | null>(null)
  const fieldCardRefs = useRef<Record<string, HTMLDivElement | null>>({})
  const [fileLists, setFileLists] = useState<Record<string, UploadFile[]>>({})
  const fileListsRef = useRef<Record<string, UploadFile[]>>({})
  const userFileChangeVersionsRef = useRef<Record<string, number>>({})
  const [textValues, setTextValues] = useState<Record<string, string>>({})
  const [defaultsLoading, setDefaultsLoading] = useState(false)
  const [runningModule, setRunningModule] = useState<ToolboxModuleKey | 'quick' | null>(null)
  // 按模块隔离，避免请假 audit/log 泄漏到加班/汇总 Tab
  const [runLogByModule, setRunLogByModule] = useState<Partial<Record<ToolboxModuleKey | 'quick', string>>>({})
  const [lastRun, setLastRun] = useState<AttendanceToolboxRunResponse | null>(null)
  const [previewRows, setPreviewRows] = useState<Record<string, string>[]>([])
  const [previewLoading, setPreviewLoading] = useState(false)
  const [appliedRules, setAppliedRules] = useState<AppliedRulesState>({
    source: 'default',
    rules: null,
    rulesJson: null,
  })
  const [quickRunLeave, setQuickRunLeave] = useState(true)
  const [quickRunOvertime, setQuickRunOvertime] = useState(true)
  const [maternityLeaveOverrides, setMaternityLeaveOverrides] = useState<MaternityLeaveOverride[]>(loadMaternityLeaveOverrides)
  const [uploadWarnings, setUploadWarnings] = useState<Record<string, string[]>>({})
  const [auditWarningsByModule, setAuditWarningsByModule] = useState<Partial<Record<ToolboxModuleKey, string[]>>>({})
  const [templateMeta, setTemplateMeta] = useState<TemplateMetaItem[]>([])
  const [downloadingTemplate, setDownloadingTemplate] = useState<string | null>(null)
  const [requirementView, setRequirementView] = useState<Record<string, string>>({})
  const [leaveSourceSync, setLeaveSourceSync] = useState<SyncState>({ loading: false })
  const [leaveSourceDateRange, setLeaveSourceDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs]>(
    getPreviousCalendarMonthRange,
  )
  const [rosterSync, setRosterSync] = useState<SyncState>({ loading: false })
  const [transferSync, setTransferSync] = useState<SyncState>({ loading: false })
  const [parttimePunchSync, setParttimePunchSync] = useState<SyncState>({ loading: false })
  // 默认上一个自然月（需求：默认上一个自然月）。
  const [parttimePunchMonth, setParttimePunchMonth] = useState<dayjs.Dayjs | null>(() => dayjs().subtract(1, 'month'))
  const autoSyncStartedRef = useRef(false)
  const autoRosterSyncStartedRef = useRef(false)
  const autoTransferSyncStartedRef = useRef(false)

  // DingTalk sync specific state
  const [dingtalkDateRange, setDingtalkDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null] | null>(
    getPreviousCalendarMonthRange,
  )
  const [dingtalkFlowKeys, setDingtalkFlowKeys] = useState<string[]>(['leave', 'overtime', 'attendance_correction', 'position_transfer'])
  const [dingtalkUnlimited, setDingtalkUnlimited] = useState(true)
  const [dingtalkMaxInstances, setDingtalkMaxInstances] = useState<number>(100)
  const [dingtalkPaddingDays, setDingtalkPaddingDays] = useState<number>(31)

  const moduleMap = useMemo(() => new Map(modules.map((item) => [item.key, item])), [])

  useEffect(() => {
    window.localStorage.setItem(MATERNITY_LEAVE_STORAGE_KEY, JSON.stringify(maternityLeaveOverrides))
  }, [maternityLeaveOverrides])

  useEffect(() => {
    let mounted = true
    setDefaultsLoading(true)

    attendanceToolboxAPI.getDefaults()
      .then((response) => {
        if (!mounted) return
        const defaults = normalizeDefaults(response)
        setTextValues((prev) => {
          const next = { ...prev }
          for (const [key, values] of Object.entries(defaults)) {
            if (next[key] === undefined) {
              next[key] = formatNameList(values)
            }
          }
          return next
        })
      })
      .catch(() => {
        if (mounted) {
          messageApi.warning('默认名单加载失败，计算时仍会使用内置规则')
        }
      })
      .finally(() => {
        if (mounted) setDefaultsLoading(false)
      })

    return () => {
      mounted = false
    }
  }, [])

  const refreshTemplateMeta = useCallback(async () => {
    try {
      const response = await attendanceToolboxAPI.listTemplates()
      const data = (response as unknown as { data?: { templates?: TemplateMetaItem[] } })?.data
      if (Array.isArray(data?.templates)) {
        setTemplateMeta(data.templates)
      }
    } catch {
      // Template catalogue is non-critical; ignore failures.
    }
  }, [])

  useEffect(() => {
    refreshTemplateMeta()
  }, [refreshTemplateMeta])

  const setFieldFiles = useCallback((fieldName: string, list: UploadFile[]) => {
    const next = { ...fileListsRef.current, [fieldName]: list }
    fileListsRef.current = next
    userFileChangeVersionsRef.current[fieldName] = (userFileChangeVersionsRef.current[fieldName] || 0) + 1
    setFileLists(next)
  }, [])

  const applySyncedFile = useCallback((
    fieldNames: string[],
    fileName: string,
    blob: Blob,
    canApply?: (fieldName: string, currentFiles: UploadFile[]) => boolean,
  ) => {
    const syncedAt = Date.now()
    // lastModifiedDate is a legacy read-only File getter in modern browsers;
    // assigning it via Object.assign throws:
    // "Cannot set property lastModifiedDate of #<File> which has only a getter"
    const uploadFile = new File([blob], fileName, {
      type: blob.type || 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      lastModified: syncedAt,
    }) as RcFile
    uploadFile.uid = `auto-sync-file-${syncedAt}`

    const appliedFieldNames: string[] = []
    const next = { ...fileListsRef.current }
    fieldNames.forEach((fieldName, index) => {
      const currentFiles = next[fieldName] || []
      if (canApply && !canApply(fieldName, currentFiles)) return
      next[fieldName] = [{
        uid: `auto-sync-${fieldName}-${syncedAt}-${index}`,
        name: uploadFile.name,
        status: 'done',
        percent: 100,
        size: uploadFile.size,
        type: uploadFile.type,
        originFileObj: uploadFile,
      }]
      appliedFieldNames.push(fieldName)
    })
    fileListsRef.current = next
    setFileLists(next)

    if (appliedFieldNames.length > 0) {
      setUploadWarnings((prev) => {
        const nextWarnings = { ...prev }
        appliedFieldNames.forEach((fieldName) => {
          delete nextWarnings[fieldName]
        })
        return nextWarnings
      })
    }
    return appliedFieldNames
  }, [])

  const fetchDingtalkExport = useCallback(async (
    request: DingtalkSyncSelectionRequest,
    flowKey: string,
  ): Promise<Blob> => {
    let response: unknown
    try {
      response = await attendanceToolboxAPI.runDingtalkSyncStructured(request)
    } catch (structuredError) {
      if (!shouldFallbackToLegacyToolboxAPI(structuredError)) {
        throw structuredError
      }
      // Compatibility for servers without the structured workflow endpoint.
      // Never enter this branch after a successful structured sync.
      const legacyBlob = await attendanceToolboxAPI.runDingtalkSync(request) as unknown as Blob
      if (legacyBlob.type === 'application/zip' || legacyBlob.type === 'application/x-zip-compressed') {
        throw new Error('服务器版本暂不支持从多文件结果中自动选择业务表，请联系管理员升级')
      }
      return legacyBlob
    }

    const run = (response as { data?: AttendanceToolboxRunResponse })?.data
      || (response as AttendanceToolboxRunResponse)
    if (!run?.run_id) {
      throw new Error('同步完成，但未返回结果编号')
    }
    const exportFile = getDingtalkSyncExportFile(run, flowKey)
    if (!exportFile) {
      throw new Error('同步完成，但未生成可用于自动回填的业务表')
    }
    try {
      return await attendanceToolboxAPI.downloadRunFile(run.run_id, exportFile.file_key) as unknown as Blob
    } catch (downloadError) {
      // 结构化同步已经成功；下载失败只提示，禁止重新触发钉钉同步。
      throw downloadError
    }
  }, [])

  const syncLeaveSourceFromDingtalk = useCallback(async () => {
    if (runningModule) {
      messageApi.warning('当前有任务正在执行，请稍候')
      return
    }

    setLeaveSourceSync({ loading: true })
    try {
      const blob = await fetchDingtalkExport({
        start_date: leaveSourceDateRange[0].format('YYYY-MM-DD'),
        end_date: leaveSourceDateRange[1].format('YYYY-MM-DD'),
        flow_keys: ['leave'],
        padding_days: LEAVE_SYNC_PADDING_DAYS,
      }, 'leave')

      applySyncedFile([...AUTO_SYNC_UPLOADS.leave], '请假系统导出_钉钉同步.xlsx', blob)
      setLeaveSourceSync({ loading: false, lastSyncAt: dayjs().format('YYYY-MM-DD HH:mm') })
      messageApi.success('请假数据拉取完成，已自动回填到请假系统导出表')
    } catch (error) {
      const errorMessage = await resolveErrorMessage(error)
      setLeaveSourceSync({ loading: false, error: errorMessage })
      messageApi.error(`请假数据拉取失败：${errorMessage}`)
    }
  }, [applySyncedFile, fetchDingtalkExport, leaveSourceDateRange, runningModule])

  // 花名册同步：必须走组织花名册生成接口，禁止再使用 position_transfer 导出表充当在职花名册。
  // 数据来自本地数据库的 active 用户、EmployeeProfile 权威工号与真实部门路径；
  // 同一富花名册可供加班部门映射与最终汇总使用。
  const generateRosterFromOrgData = useCallback(async (mode: SyncMode = 'manual') => {
    const requestSnapshot = new Map<string, { userChangeVersion: number; wasEmpty: boolean }>(
      AUTO_SYNC_UPLOADS.roster.map((fieldName) => [fieldName, {
        userChangeVersion: userFileChangeVersionsRef.current[fieldName] || 0,
        wasEmpty: (fileListsRef.current[fieldName] || []).length === 0,
      }]),
    )
    setRosterSync({ loading: true })
    try {
      // generateOrgRoster 配置了 responseType: 'blob'，axios 返回的 response.data 即为 Blob
      const response = await attendanceToolboxAPI.generateOrgRoster()
      const blob = response.data as Blob

      const appliedFieldNames = applySyncedFile(
        [...AUTO_SYNC_UPLOADS.roster],
        '花名册_组织生成.xlsx',
        blob,
        (fieldName, currentFiles) => {
          const snapshot = requestSnapshot.get(fieldName)
          if (!snapshot || (userFileChangeVersionsRef.current[fieldName] || 0) !== snapshot.userChangeVersion) {
            return false
          }
          return mode === 'manual' || (snapshot.wasEmpty && currentFiles.length === 0)
        },
      )
      setRosterSync({ loading: false, lastSyncAt: dayjs().format('YYYY-MM-DD HH:mm') })
      if (mode === 'manual') {
        if (appliedFieldNames.length === AUTO_SYNC_UPLOADS.roster.length) {
          messageApi.success('花名册生成完成，已自动回填到加班与最终汇总上传位')
        } else if (appliedFieldNames.length > 0) {
          messageApi.info('花名册生成完成，已保留请求期间修改的文件并回填其余上传位')
        } else {
          messageApi.info('花名册生成完成，已保留请求期间修改的上传文件')
        }
      }
    } catch (error) {
      const errorMessage = await resolveErrorMessage(error)
      setRosterSync({ loading: false, error: errorMessage })
      if (mode === 'manual') {
        messageApi.error(`花名册生成失败：${errorMessage}`)
      }
    }
  }, [applySyncedFile])

  // 异动流程同步：独立使用 position_transfer，只回填异动流程位置，不得与花名册混用。
  const syncTransferFromDingtalk = useCallback(async (mode: SyncMode = 'manual') => {
    setTransferSync({ loading: true })
    try {
      const today = dayjs()
      const start = today.subtract(3, 'month').startOf('month').format('YYYY-MM-DD')
      const end = today.endOf('month').format('YYYY-MM-DD')
      const blob = await fetchDingtalkExport({
        start_date: start,
        end_date: end,
        flow_keys: ['position_transfer'],
        padding_days: 90,
      }, 'position_transfer')

      applySyncedFile([...AUTO_SYNC_UPLOADS.transfer], '异动流程_钉钉自动同步.xlsx', blob)
      setTransferSync({ loading: false, lastSyncAt: dayjs().format('YYYY-MM-DD HH:mm') })
      if (mode === 'manual') {
        messageApi.success('异动流程同步完成，已自动回填到上传位')
      }
    } catch (error) {
      const errorMessage = await resolveErrorMessage(error)
      setTransferSync({ loading: false, error: errorMessage })
      if (mode === 'manual') {
        messageApi.error(`异动流程同步失败：${errorMessage}`)
      }
    }
  }, [applySyncedFile, fetchDingtalkExport])

  // 兼职月度打卡记录：只能由用户点击执行，不允许页面打开时自动抓取。
  const syncParttimeMonthlyPunch = useCallback(async () => {
    if (!parttimePunchMonth) {
      messageApi.warning('请先选择月份')
      return
    }
    setParttimePunchSync({ loading: true, error: undefined })
    try {
      const month = parttimePunchMonth.format('YYYY-MM')
      const blob = (await attendanceToolboxAPI.parttimeMonthlyPunch({ month })) as unknown as Blob
      // 回填到兼职汇总现有的「考勤明细」上传位（parttime_attendance_detail）。
      applySyncedFile(['parttime_attendance_detail'], `兼职月度打卡记录_${month.replace('-', '')}.xlsx`, blob)
      setParttimePunchSync({ loading: false, lastSyncAt: dayjs().format('YYYY-MM-DD HH:mm') })
      messageApi.success('兼职月度打卡记录抓取完成，已自动回填到兼职汇总的「考勤明细」上传位')
    } catch (error) {
      const errorMessage = await resolveErrorMessage(error)
      setParttimePunchSync({ loading: false, error: errorMessage })
      messageApi.error(`兼职月度打卡记录抓取失败：${errorMessage}，您仍可手动上传本地 Excel`)
    }
  }, [applySyncedFile, parttimePunchMonth])

  // 花名册生成由 canOperate 控制（数据来自本地组织数据库）
  useEffect(() => {
    if (!canOperate) return
    if (autoRosterSyncStartedRef.current) return
    autoRosterSyncStartedRef.current = true
    void generateRosterFromOrgData('auto')
  }, [canOperate, generateRosterFromOrgData])

  // 岗位异动同步由 canDingtalkSync 控制（数据来自钉钉审批流程）
  useEffect(() => {
    if (!canDingtalkSync) return
    if (autoTransferSyncStartedRef.current) return
    autoTransferSyncStartedRef.current = true
    void syncTransferFromDingtalk('auto')
  }, [canDingtalkSync, syncTransferFromDingtalk])

  const removeFieldFile = (fieldName: string, fileUid: string) => {
    const next = {
      ...fileListsRef.current,
      [fieldName]: (fileListsRef.current[fieldName] || []).filter((file) => file.uid !== fileUid),
    }
    fileListsRef.current = next
    userFileChangeVersionsRef.current[fieldName] = (userFileChangeVersionsRef.current[fieldName] || 0) + 1
    setFileLists(next)
    setUploadWarnings((prev) => {
      const next = { ...prev }
      delete next[fieldName]
      return next
    })
  }

  const runAuditForFiles = useCallback(async (config: ModuleConfig) => {
    const moduleKey = config.key
    const items = config.fileFields.flatMap((field) => (fileLists[field.name] || []).map((file) => ({
      file,
      field,
    })))
    if (items.length === 0) {
      setAuditWarningsByModule((prev) => ({ ...prev, [moduleKey]: [] }))
      return
    }
    const formData = new FormData()
    items.forEach(({ file, field }) => {
      if (file.originFileObj) formData.append(field.name, file.originFileObj)
    })
    try {
      const response = await attendanceToolboxAPI.auditUploads(formData) as unknown as {
        data?: { warnings?: string[] }
      }
      const warnings = response?.data?.warnings || []
      setAuditWarningsByModule((prev) => ({ ...prev, [moduleKey]: warnings }))
      if (warnings.length > 0) {
        warnings.forEach((warning) => messageApi.warning(warning))
      } else {
        messageApi.success('文件检测通过')
      }
    } catch (error) {
      messageApi.warning(`文件检测失败：${error instanceof Error ? error.message : '未知错误'}`)
      setAuditWarningsByModule((prev) => ({ ...prev, [moduleKey]: [] }))
    }
  }, [fileLists, messageApi])

  const downloadTemplate = useCallback(async (templateId: string) => {
    setDownloadingTemplate(templateId)
    try {
      const blob = await attendanceToolboxAPI.exportTemplates(templateId) as unknown as Blob
      downloadBlob(blob, getTemplateDownloadFileName(templateId))
    } catch (error) {
      messageApi.error(await resolveErrorMessage(error))
    } finally {
      setDownloadingTemplate(null)
    }
  }, [])

  const downloadAllTemplates = useCallback(async () => {
    setDownloadingTemplate('__all__')
    try {
      const blob = await attendanceToolboxAPI.exportTemplates() as unknown as Blob
      downloadBlob(blob, '考勤工具箱模板.zip')
    } catch (error) {
      messageApi.error(await resolveErrorMessage(error))
    } finally {
      setDownloadingTemplate(null)
    }
  }, [])

  const templateNameById = useMemo(() => {
    const map = new Map<string, string>(templateDownloadItems.map((item) => [item.templateId, item.label]))
    templateMeta.forEach((item) => {
      if (!map.has(item.id)) {
        map.set(item.id, item.file_name)
      }
    })
    return map
  }, [templateMeta])

  const getTemplateDownloadFileName = useCallback((templateId: string) => {
    const metaItem = templateMeta.find((item) => item.id === templateId)
    if (metaItem?.file_name) return metaItem.file_name
    return `${templateNameById.get(templateId) || templateId}.xlsx`
  }, [templateMeta, templateNameById])

  const checkFileWarnings = useCallback((fieldName: string, fileList: UploadFile[]) => {
    const warnings: string[] = []
    for (const file of fileList) {
      const sizeMB = (file.size || 0) / (1024 * 1024)
      if (sizeMB > 50) {
        warnings.push(`${file.name} 超过 50MB（${sizeMB.toFixed(1)}MB），上传可能较慢`)
      }
    }
    setUploadWarnings((prev) => {
      if (warnings.length === 0) {
        const next = { ...prev }
        delete next[fieldName]
        return next
      }
      return { ...prev, [fieldName]: warnings }
    })
  }, [])

  const uploadProps = (field: FileField): UploadProps => ({
    accept: acceptExcel,
    multiple: field.multiple,
    maxCount: field.multiple ? undefined : 1,
    beforeUpload: () => false,
    fileList: fileLists[field.name] || [],
    showUploadList: false,
    onChange: ({ fileList }) => {
      setFieldFiles(field.name, field.multiple ? fileList : fileList.slice(-1))
      checkFileWarnings(field.name, field.multiple ? fileList : fileList.slice(-1))
    },
  })

  const loadOvertimePreview = async (run: AttendanceToolboxRunResponse) => {
    if (run.module !== 'overtime') {
      setPreviewRows([])
      return
    }
    const exportFile = (run.files || []).find((f) => f.kind !== 'meta' && (f.file_name || '').endsWith('.xlsx'))
      || (run.files || []).find((f) => f.kind !== 'meta')
    if (!exportFile) {
      setPreviewRows([])
      return
    }
    setPreviewLoading(true)
    try {
      const response = await attendanceToolboxAPI.previewRun(run.run_id, exportFile.file_key) as {
        data?: { rows?: Record<string, string>[] }
      }
      const rows = response?.data?.rows || []
      setPreviewRows(Array.isArray(rows) ? rows : [])
    } catch {
      // Preview failure must not block download.
      setPreviewRows([])
    } finally {
      setPreviewLoading(false)
    }
  }

  const formatRunStats = (stats: Record<string, unknown> | undefined | null) => {
    if (!stats || typeof stats !== 'object') return []
    return Object.entries(stats).flatMap(([key, value]) => {
      if (value == null) return []
      if (typeof value === 'object' && !Array.isArray(value)) {
        return Object.entries(value as Record<string, unknown>).map(([subKey, subValue]) => ({
          label: `${key}.${subKey}`,
          text: String(subValue),
        }))
      }
      return [{ label: key, text: Array.isArray(value) ? value.join('、') : String(value) }]
    })
  }

  const focusMissingField = (fieldName: string) => {
    setMissingFieldName(fieldName)
    window.requestAnimationFrame(() => {
      const el = fieldCardRefs.current[fieldName]
      el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    })
    window.setTimeout(() => {
      setMissingFieldName((current) => (current === fieldName ? null : current))
    }, 2600)
  }

  /** @returns true 仅当生成了可下载结果；空结果不标记完成、不假装成功下载 */
  const applyRunResponse = async (payload: unknown, successText: string): Promise<boolean> => {
    const data = (payload as { data?: AttendanceToolboxRunResponse })?.data
      || (payload as AttendanceToolboxRunResponse)
    if (!data?.run_id) {
      throw new Error('未返回 run_id')
    }
    setLastRun(data)
    if (data.module) {
      setRunLogByModule((prev) => ({ ...prev, [data.module as ToolboxModuleKey | 'quick']: data.log || '' }))
    }
    // Prefer zip when multiple files; otherwise single file. Meta files are technical details, not user results.
    const downloadables = getAttendanceToolboxDownloadableFiles(data)
    if (downloadables.length === 0) {
      const isSyncModule = data.module === 'dingtalk_sync' || data.module === 'quick'
      messageApi.warning(
        isSyncModule
          ? '本次同步未生成可下载结果，请检查同步日志和钉钉流程码配置'
          : '本次计算未生成可下载结果，请检查运行日志与输入文件',
      )
      return false
    }

    let downloadOk = true
    try {
      if (downloadables.length > 1) {
        const zip = await attendanceToolboxAPI.downloadRunZip(data.run_id) as unknown as Blob
        downloadBlob(zip, '考勤工具箱结果.zip')
      } else if (downloadables.length === 1) {
        const one = downloadables[0]
        const blob = await attendanceToolboxAPI.downloadRunFile(data.run_id, one.file_key) as unknown as Blob
        downloadBlob(blob, one.file_name || '结果.xlsx')
      }
    } catch (downloadError) {
      downloadOk = false
      const detail = await resolveErrorMessage(downloadError)
      // 410 等过期场景沿用明确文案；其它下载失败保留「计算已完成」以免误导重跑
      if (detail.includes('过期')) {
        messageApi.error(detail)
      } else {
        messageApi.warning(`计算已完成，但自动下载失败：${detail}。可在下方结果区手动下载。`)
      }
    }

    if (data.module) {
      setCompletedModules((prev) => ({ ...prev, [data.module as ToolboxModuleKey]: true }))
    }
    // Best-effort 200-row preview for overtime only; never re-runs calculation.
    void loadOvertimePreview(data)
    const subsidyAudit = getSubsidyAuditMeta(data)
    if (subsidyAudit?.missingAttendanceCount) {
      const preview = subsidyAudit.missingAttendanceNames.slice(0, 5).join('、')
      const suffix = subsidyAudit.missingAttendanceCount > 5 ? '等' : ''
      messageApi.warning(
        `计算完成，但有 ${subsidyAudit.missingAttendanceCount} 人缺少考勤记录：${preview}${suffix}；详细名单已写入“异常审计”。`,
      )
    } else if (downloadOk) {
      messageApi.success(successText)
    }
    return true
  }

  const handleDownloadRunFile = async (fileKey: string, fileName: string) => {
    if (!lastRun?.run_id) return
    try {
      const blob = await attendanceToolboxAPI.downloadRunFile(lastRun.run_id, fileKey) as unknown as Blob
      downloadBlob(blob, fileName)
    } catch (error) {
      messageApi.error(await resolveErrorMessage(error))
    }
  }

  const handleDownloadRunZip = async () => {
    if (!lastRun?.run_id) return
    try {
      const blob = await attendanceToolboxAPI.downloadRunZip(lastRun.run_id) as unknown as Blob
      downloadBlob(blob, '考勤工具箱结果.zip')
    } catch (error) {
      messageApi.error(await resolveErrorMessage(error))
    }
  }

  const addMaternityLeaveOverride = () => {
    setMaternityLeaveOverrides((prev) => [
      ...prev,
      {
        key: `maternity-${Date.now()}-${prev.length}`,
        employee_no: '',
        name: '',
        start_date: '',
        end_date: '',
      },
    ])
  }

  const updateMaternityLeaveOverride = (
    key: string,
    field: keyof Omit<MaternityLeaveOverride, 'key'>,
    value: string,
  ) => {
    setMaternityLeaveOverrides((prev) => prev.map((item) => (
      item.key === key ? { ...item, [field]: value } : item
    )))
  }

  const removeMaternityLeaveOverride = (key: string) => {
    setMaternityLeaveOverrides((prev) => prev.filter((item) => item.key !== key))
  }

  const renderMaternityLeaveOverrides = () => (
    <Card
      size="small"
      title="长期产假人员（自动抓不到时兜底）"
      style={{ borderRadius: 'var(--radius-lg)' }}
      extra={
        <Button size="small" icon={<PlusOutlined />} onClick={addMaternityLeaveOverride}>
          添加人员
        </Button>
      }
    >
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="这里只用于补录自动抓不到的长期产假"
          description="普通产假仍会自动抓取；如果钉钉已有同一员工的重叠产假，系统会自动跳过手动记录，避免重复计算。配置会保存在当前浏览器。"
        />
        {maternityLeaveOverrides.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂未添加长期产假人员" />
        ) : (
          <Space direction="vertical" size={10} style={{ width: '100%' }}>
            {maternityLeaveOverrides.map((item, index) => (
              <Card
                key={item.key}
                size="small"
                title={`长期产假人员 ${index + 1}`}
                extra={
                  <Button
                    type="text"
                    danger
                    size="small"
                    icon={<CloseOutlined />}
                    aria-label={`删除长期产假人员${index + 1}`}
                    onClick={() => removeMaternityLeaveOverride(item.key)}
                  >
                    删除
                  </Button>
                }
              >
                <Row gutter={[12, 12]}>
                  <Col xs={24} md={6}>
                    <Space direction="vertical" size={4} style={{ width: '100%' }}>
                      <Text strong>工号（可不填）</Text>
                      <Input
                        value={item.employee_no}
                        placeholder="请输入员工工号"
                        onChange={(event) => updateMaternityLeaveOverride(item.key, 'employee_no', event.target.value)}
                      />
                    </Space>
                  </Col>
                  <Col xs={24} md={6}>
                    <Space direction="vertical" size={4} style={{ width: '100%' }}>
                      <Text strong>姓名 *</Text>
                      <Input
                        value={item.name}
                        placeholder="请输入员工姓名"
                        onChange={(event) => updateMaternityLeaveOverride(item.key, 'name', event.target.value)}
                      />
                    </Space>
                  </Col>
                  <Col xs={24} md={6}>
                    <Space direction="vertical" size={4} style={{ width: '100%' }}>
                      <Text strong>产假开始日期 *</Text>
                      <DatePicker
                        value={item.start_date ? dayjs(item.start_date) : null}
                        placeholder="请选择开始日期"
                        style={{ width: '100%' }}
                        onChange={(value) => updateMaternityLeaveOverride(
                          item.key,
                          'start_date',
                          value ? value.format('YYYY-MM-DD') : '',
                        )}
                      />
                    </Space>
                  </Col>
                  <Col xs={24} md={6}>
                    <Space direction="vertical" size={4} style={{ width: '100%' }}>
                      <Text strong>产假结束日期 *</Text>
                      <DatePicker
                        value={item.end_date ? dayjs(item.end_date) : null}
                        placeholder="请选择结束日期"
                        style={{ width: '100%' }}
                        onChange={(value) => updateMaternityLeaveOverride(
                          item.key,
                          'end_date',
                          value ? value.format('YYYY-MM-DD') : '',
                        )}
                      />
                    </Space>
                  </Col>
                </Row>
              </Card>
            ))}
          </Space>
        )}
      </Space>
    </Card>
  )

  const getMaternityLeaveOverridesForSubmit = () => {
    const configured = maternityLeaveOverrides.filter((item) => (
      item.employee_no.trim() || item.name.trim() || item.start_date || item.end_date
    ))
    const incomplete = configured.find((item) => !item.name.trim() || !item.start_date || !item.end_date)
    if (incomplete) {
      messageApi.warning('长期产假人员请填写完整的姓名、开始日期和结束日期；工号可以不填')
      return null
    }
    return configured.map(({ employee_no, name, start_date, end_date }) => ({
      employee_no: employee_no.trim(),
      name: name.trim(),
      start_date,
      end_date,
    }))
  }

  const handleQuickWorkflow = async () => {
    if (runningModule) {
      messageApi.warning('当前有任务正在执行，请稍候')
      return
    }
    if (!canQuick) {
      messageApi.warning('你缺少一键联动所需权限（操作 + 钉钉同步），需要联系管理员添加')
      return
    }
    if (!dingtalkDateRange || !dingtalkDateRange[0] || !dingtalkDateRange[1]) {
      messageApi.warning('请选择同步日期范围')
      return
    }
    if (!quickRunLeave && !quickRunOvertime) {
      messageApi.warning('一键联动至少选择请假或加班其中一项')
      return
    }
    const maternityOverrides = quickRunLeave ? getMaternityLeaveOverridesForSubmit() : []
    if (maternityOverrides === null) return

    const scheduleFiles = fileLists.leave_schedule || fileLists.overtime_calendar || []
    if (!scheduleFiles.length) {
      messageApi.warning('一键联动必须上传作息表（可在请假或加班页签上传）')
      return
    }

    const formData = new FormData()
    formData.append('dingtalk_sync_start_date', dingtalkDateRange[0].format('YYYY-MM-DD'))
    formData.append('dingtalk_sync_end_date', dingtalkDateRange[1].format('YYYY-MM-DD'))
    formData.append('dingtalk_sync_flow_keys', dingtalkFlowKeys.join(','))
    formData.append('dingtalk_sync_padding_days', String(dingtalkPaddingDays))
    if (!dingtalkUnlimited) {
      formData.append('dingtalk_sync_max_instances', String(dingtalkMaxInstances))
    }
    formData.append('run_leave', quickRunLeave ? 'true' : 'false')
    formData.append('run_overtime', quickRunOvertime ? 'true' : 'false')
    for (const file of scheduleFiles) {
      if (file.originFileObj) {
        formData.append('leave_schedule', file.originFileObj)
        formData.append('overtime_calendar', file.originFileObj)
      }
    }
    // Optional helpers from existing uploads
    for (const key of ['leave_offsite_duration', 'overtime_attendance', 'overtime_roster', 'overtime_schedules'] as const) {
      for (const file of fileLists[key] || []) {
        if (file.originFileObj) formData.append(key, file.originFileObj)
      }
    }
    if (Object.prototype.hasOwnProperty.call(textValues, 'leave_special_names')) {
      formData.append('leave_special_names', (textValues.leave_special_names || '').trim())
    }
    if (Object.prototype.hasOwnProperty.call(textValues, 'chengdu_schedule_names')) {
      formData.append('chengdu_schedule_names', (textValues.chengdu_schedule_names || '').trim())
    }
    if (quickRunLeave) {
      formData.append('maternity_leave_overrides', JSON.stringify(maternityOverrides))
    }
    if (appliedRules.source === 'custom' && appliedRules.rulesJson) {
      formData.append('rules_json', appliedRules.rulesJson)
    }
    const quickMonth = (textValues.overtime_target_month || '').trim()
    if (quickRunOvertime && quickMonth) {
      formData.append('overtime_target_month', quickMonth)
    }

    setRunningModule('quick')
    try {
      const response = await attendanceToolboxAPI.runQuickWorkflow(formData)
      const ok = await applyRunResponse(response, '一键联动完成，结果已下载')
      if (ok) {
        setCompletedModules((prev) => ({
          ...prev,
          dingtalk_sync: true,
          ...(quickRunLeave ? { leave: true } : {}),
          ...(quickRunOvertime ? { overtime: true } : {}),
        }))
      }
    } catch (error) {
      messageApi.error(await resolveErrorMessage(error))
    } finally {
      setRunningModule(null)
    }
  }

  const handleRun = async (moduleKey: ToolboxModuleKey) => {
    const config = moduleMap.get(moduleKey)
    if (!config) return

    if (runningModule) {
      messageApi.warning('当前有任务正在执行，请稍候')
      return
    }

    if (moduleKey === 'dingtalk_sync' && !canDingtalkSync) {
      messageApi.warning('你缺少考勤工具箱钉钉同步权限，需要联系管理员添加')
      return
    }
    if (moduleKey !== 'dingtalk_sync' && !canOperate) {
      messageApi.warning('你缺少考勤工具箱操作权限，需要联系管理员添加')
      return
    }

    // 尽早占位，避免 audit 窗口双提交 / 跨 Tab 并发
    setRunningModule(moduleKey)
    try {
      if (config.fileFields.length > 0) {
        await runAuditForFiles(config)
      }
    } catch {
      setRunningModule(null)
      return
    }

    if (moduleKey === 'dingtalk_sync') {
      if (!dingtalkDateRange || !dingtalkDateRange[0] || !dingtalkDateRange[1]) {
        messageApi.warning('请选择同步日期范围')
        setRunningModule(null)
        return
      }
      if (dingtalkFlowKeys.length === 0) {
        messageApi.warning('请至少选择一个同步流程')
        setRunningModule(null)
        return
      }
      try {
        // Prefer structured result; only fall back when server lacks the new endpoint.
        try {
          const formData = new FormData()
          formData.append('dingtalk_sync_start_date', dingtalkDateRange[0].format('YYYY-MM-DD'))
          formData.append('dingtalk_sync_end_date', dingtalkDateRange[1].format('YYYY-MM-DD'))
          formData.append('dingtalk_sync_flow_keys', dingtalkFlowKeys.join(','))
          formData.append('dingtalk_sync_padding_days', String(dingtalkPaddingDays))
          if (!dingtalkUnlimited) {
            formData.append('dingtalk_sync_max_instances', String(dingtalkMaxInstances))
          }
          const response = await attendanceToolboxAPI.runDingtalkSyncStructured(formData)
          const ok = await applyRunResponse(response, '同步完成，结果已下载')
          if (ok) {
            setCompletedModules((prev) => ({ ...prev, dingtalk_sync: true }))
          }
        } catch (structuredError) {
          if (!shouldFallbackToLegacyToolboxAPI(structuredError)) {
            throw structuredError
          }
          // Version mismatch only — do not re-run after successful structured sync.
          const blob = await attendanceToolboxAPI.runDingtalkSync({
            start_date: dingtalkDateRange[0].format('YYYY-MM-DD'),
            end_date: dingtalkDateRange[1].format('YYYY-MM-DD'),
            flow_keys: dingtalkFlowKeys,
            max_instances: dingtalkUnlimited ? undefined : dingtalkMaxInstances,
            padding_days: dingtalkPaddingDays,
          }) as unknown as Blob
          downloadBlob(blob, getDownloadName(config, blob))
          setCompletedModules((prev) => ({ ...prev, dingtalk_sync: true }))
          messageApi.success('同步完成，结果已下载')
        }
      } catch (error) {
        messageApi.error(await resolveErrorMessage(error))
      } finally {
        setRunningModule(null)
      }
      return
    }

    for (const field of config.fileFields) {
      if (field.required && !(fileLists[field.name] || []).length) {
        messageApi.warning(`请上传${field.label}`)
        focusMissingField(field.name)
        setRunningModule(null)
        return
      }
    }
    const maternityOverrides = moduleKey === 'leave' ? getMaternityLeaveOverridesForSubmit() : []
    if (maternityOverrides === null) {
      setRunningModule(null)
      return
    }

    if (moduleKey === 'parttime') {
      const parttimeSourceFields = ['parttime_attendance_detail', 'parttime_monthly', 'parttime_schedules']
      const hasSource = parttimeSourceFields.some((key) => (fileLists[key] || []).length > 0)
      if (!hasSource) {
        messageApi.warning('请至少上传考勤明细、月度汇总或排班表中的一类')
        focusMissingField(parttimeSourceFields[0])
        setRunningModule(null)
        return
      }
    }

    const formData = new FormData()
    for (const field of config.fileFields) {
      for (const file of fileLists[field.name] || []) {
        const rawFile = file.originFileObj
        if (rawFile) formData.append(field.name, rawFile)
      }
    }
    for (const field of config.textFields || []) {
      const value = (textValues[field.name] || '').trim()
      if (Object.prototype.hasOwnProperty.call(textValues, field.name)) {
        formData.append(field.name, value)
      }
    }
    if (moduleKey === 'leave') {
      formData.append('maternity_leave_overrides', JSON.stringify(maternityOverrides))
    }
    for (const [field, value] of Object.entries(
      buildAttendanceToolboxWorkflowOptionFields(moduleKey, textValues, appliedRules),
    )) {
      formData.append(field, value)
    }

    try {
      try {
        const response = await attendanceToolboxAPI.runWorkflow(moduleKey, formData)
        await applyRunResponse(
          response,
          moduleKey === 'overtime' && appliedRules.source === 'custom'
            ? '计算完成（已使用自定义规则），结果已下载'
            : '计算完成，结果已下载',
        )
      } catch (structuredError) {
        // Only fall back when the server does not expose structured workflows.
        // 403/400/410/5xx/timeout/network must not re-run via the legacy blob API.
        if (!shouldFallbackToLegacyToolboxAPI(structuredError)) {
          throw structuredError
        }
        const blob = await attendanceToolboxAPI.run(moduleKey, formData) as unknown as Blob
        downloadBlob(blob, getDownloadName(config, blob))
        messageApi.success('计算完成，结果已下载')
      }
    } catch (error) {
      messageApi.error(await resolveErrorMessage(error))
    } finally {
      setRunningModule(null)
    }
  }

  // ── Render helpers ──

  const renderFieldBadges = (badges: FieldBadge[]) => (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 4 }}>
      {badges.map((b) => (
        <Tag
          key={b.label}
          color={b.required ? 'error' : 'default'}
          style={{ borderRadius: 4, fontSize: 12, fontWeight: 500 }}
        >
          {b.required && <span style={{ marginRight: 4 }}>*</span>}
          {b.label}
        </Tag>
      ))}
    </div>
  )

  const renderUploadRequirements = (moduleKey: ToolboxModuleKey) => {
    const reqs = fieldRequirements[moduleKey]
    if (!reqs) return null

    const activeView = requirementView[moduleKey] || reqs[0].label

    return (
      <Collapse
        style={{ marginBottom: 16 }}
        items={[{
          key: 'req',
          label: (
            <Space>
              <InfoCircleOutlined style={{ color: 'var(--color-primary)' }} />
              <Text strong>上传文件要求和模板</Text>
              <Text type="secondary">点击展开查看各文件的必填字段和模板下载</Text>
            </Space>
          ),
          children: (
            <Space direction="vertical" size={16} style={{ width: '100%' }}>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                {templateDownloadItems
                  .filter((item) => {
                    const config = moduleMap.get(moduleKey)
                    if (!config) return false
                    return config.fileFields.some((f) => f.templateId === item.templateId)
                  })
                  .map((item) => (
                    <Button
                      key={item.key}
                      icon={<FileExcelOutlined />}
                      size="small"
                      loading={downloadingTemplate === item.templateId}
                      disabled={downloadingTemplate === '__all__'}
                      onClick={() => downloadTemplate(item.templateId)}
                    >
                      下载 {item.label} 模板
                    </Button>
                  ))}
              </div>

              <Radio.Group
                value={activeView}
                onChange={(e) => setRequirementView((prev) => ({ ...prev, [moduleKey]: e.target.value }))}
                buttonStyle="solid"
                size="small"
              >
                {reqs.map((r) => (
                  <Radio.Button key={r.label} value={r.label}>{r.label}</Radio.Button>
                ))}
              </Radio.Group>

              {reqs.filter((r) => r.label === activeView).map((r) => (
                <div key={r.label}>
                  <Text strong>{r.label}</Text>
                  {renderFieldBadges(r.badges)}
                </div>
              ))}
            </Space>
          ),
        }]}
      />
    )
  }

  const renderSelectedFiles = (field: FileField) => {
    const files = fileLists[field.name] || []
    if (files.length === 0) return null

    return (
      <Space direction="vertical" size={6} style={{ width: '100%', marginTop: 12 }}>
        {files.map((file) => (
          <div
            key={file.uid}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              minHeight: 32,
              padding: '4px 10px',
              border: '1px solid var(--color-success)',
              borderRadius: 6,
              background: 'var(--color-success-bg)',
            }}
          >
            <CheckCircleOutlined style={{ color: 'var(--color-success)', flex: '0 0 auto' }} />
            <Text ellipsis={{ tooltip: file.name }} style={{ flex: 1, minWidth: 0, fontSize: 13 }}>
              {file.name}
            </Text>
            <Button
              type="text"
              size="small"
              icon={<CloseOutlined />}
              aria-label={`移除${file.name}`}
              onClick={() => removeFieldFile(field.name, file.uid)}
            />
          </div>
        ))}
      </Space>
    )
  }

  const renderFileUploadCard = (field: FileField) => {
    const files = fileLists[field.name] || []
    const hasFile = files.length > 0
    const isMissingHighlight = missingFieldName === field.name

    return (
      <div
        ref={(node) => {
          fieldCardRefs.current[field.name] = node
        }}
      >
      <Card
        size="small"
        style={{
          height: '100%',
          borderRadius: 'var(--radius-lg)',
          borderWidth: field.required || isMissingHighlight ? '1px 1px 1px 3px' : 1,
          borderStyle: 'solid',
          borderColor: isMissingHighlight
            ? 'var(--color-error)'
            : hasFile
              ? 'var(--color-success)'
              : field.required
                ? 'var(--color-error)'
                : 'var(--color-border)',
          boxShadow: isMissingHighlight ? '0 0 0 3px rgba(255, 77, 79, 0.18)' : undefined,
          transition: 'border-color 0.2s ease, box-shadow 0.2s ease',
        }}
        styles={{ body: { padding: '16px' } }}
      >
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {hasFile ? (
              <Badge count={<CheckOutlined style={{ color: '#fff', fontSize: 10 }} />} offset={[-4, 4]}>
                <Text strong>{field.label}</Text>
              </Badge>
            ) : (
              <Text strong>{field.label}</Text>
            )}
            {field.required && <Tag color="error" style={{ fontSize: 11, lineHeight: '18px', height: 18 }}>必填</Tag>}
            {field.multiple && <Tag color="blue" style={{ fontSize: 11, lineHeight: '18px', height: 18 }}>可多选</Tag>}
          </div>

          <Dragger
            {...uploadProps(field)}
            style={{
              background: hasFile ? 'var(--color-success-bg)' : 'var(--color-bg-container)',
              borderRadius: 8,
              border: '1px dashed var(--color-border)',
              minHeight: 100,
            }}
          >
            <p className="ant-upload-drag-icon" style={{ marginBottom: 4 }}>
              {hasFile
                ? <CheckCircleOutlined style={{ color: 'var(--color-success)', fontSize: 28 }} />
                : <FileExcelOutlined style={{ color: 'var(--color-primary)', fontSize: 28 }} />}
            </p>
            <p className="ant-upload-text" style={{ fontSize: 13, margin: 0 }}>
              {hasFile ? '已上传，点击或拖拽可替换' : '点击或拖拽 Excel 到这里'}
            </p>
            <p className="ant-upload-hint" style={{ fontSize: 12 }}>
              {field.multiple ? '支持多文件 · ' : '支持单文件 · '}.xls / .xlsx
            </p>
          </Dragger>

          {renderSelectedFiles(field)}

          {uploadWarnings[field.name]?.map((warning, idx) => (
            <Alert key={idx} type="warning" message={warning} showIcon banner style={{ fontSize: 12 }} />
          ))}
        </Space>
      </Card>
      </div>
    )
  }

  const renderModule = (config: ModuleConfig) => {
    if (config.key === 'dingtalk_sync') {
      return renderDingtalkSync(config)
    }

    const requiredFields = config.fileFields.filter((f) => f.required)
    const optionalFields = config.fileFields.filter((f) => !f.required)
    const requiredReadyCount = requiredFields.filter((f) => (fileLists[f.name] || []).length > 0).length
    const monthField = config.key === 'overtime'
      ? 'overtime_target_month'
      : config.key === 'subsidy'
        ? 'subsidy_target_month'
        : null
    const lockedMonth = monthField ? (textValues[monthField] || '') : ''
    const subsidyAudit = config.key === 'subsidy' ? getSubsidyAuditMeta(lastRun) : null
    const customHolidayCount = appliedRules.rules?.legal_holidays_override?.length || 0

    return (
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        {renderUploadRequirements(config.key)}

        <Collapse
          size="small"
          ghost
          items={[{
            key: 'module-help',
            label: <Text type="secondary" style={{ fontSize: 13 }}>{config.description}</Text>,
            children: (
              <Alert
                type="info"
                showIcon
                message="使用说明"
                description="文件会上传到人事系统后端，由内置 Python 计算引擎生成结果。必填文件齐备后可在底部一键计算并下载。"
              />
            ),
          }]}
        />

        {monthField && (
          <Card size="small" title="处理月份" style={{ borderRadius: 'var(--radius-lg)' }}>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              <DatePicker
                picker="month"
                format="YYYY-MM"
                allowClear
                value={lockedMonth ? dayjs(`${lockedMonth}-01`) : null}
                placeholder="自动识别月份"
                aria-label={config.key === 'overtime' ? '加班处理月份' : '补贴考勤月份'}
                onChange={(value) => {
                  setTextValues((prev) => ({
                    ...prev,
                    [monthField]: value ? value.format('YYYY-MM') : '',
                  }))
                }}
              />
              <Text type="secondary" style={{ fontSize: 12 }}>
                默认自动识别；文件跨月或标题不规范时可手动锁定。锁定月份与作息表不一致时会停止计算。
              </Text>
              {config.key === 'subsidy' && (
                <Alert
                  type="info"
                  showIcon
                  message={customHolidayCount > 0
                    ? `法定节假日使用当前加班规则配置（${customHolidayCount}天）`
                    : '法定节假日使用作息表识别结果'}
                  description="如加班规则配置与作息表不一致，计算仍以加班规则配置为准，并把差异写入异常审计。"
                />
              )}
            </Space>
          </Card>
        )}

        {(auditWarningsByModule[config.key] || []).length > 0 && (
          <Alert
            type="warning"
            showIcon
            message={`文件检测发现 ${(auditWarningsByModule[config.key] || []).length} 条警告`}
            description={
              <ul style={{ margin: 0, paddingLeft: 18 }}>
                {(auditWarningsByModule[config.key] || []).map((warning, idx) => (
                  <li key={idx}>{warning}</li>
                ))}
              </ul>
            }
          />
        )}

        {config.key === 'leave' && (
          <Card size="small" title="钉钉请假数据" style={{ borderRadius: 'var(--radius-lg)' }}>
            <Row gutter={[16, 12]} align="middle">
              <Col xs={24} md={12}>
                <Space direction="vertical" size={4} style={{ width: '100%' }}>
                  <Text strong>请假日期范围</Text>
                  <RangePicker
                    style={{ width: '100%' }}
                    value={leaveSourceDateRange as RangePickerProps['value']}
                    onChange={(dates) => {
                      if (dates?.[0] && dates[1]) {
                        setLeaveSourceDateRange([dates[0], dates[1]])
                      }
                    }}
                    allowClear={false}
                    placeholder={['开始日期', '结束日期']}
                    aria-label="钉钉请假拉取日期范围"
                  />
                </Space>
              </Col>
              <Col xs={24} md={12}>
                <Space direction="vertical" size={4} style={{ width: '100%' }}>
                  <Tooltip
                    title={!canDingtalkSync
                      ? '你缺少考勤工具箱钉钉同步权限，需要联系管理员添加'
                      : undefined}
                  >
                    <span style={{ display: 'inline-block' }}>
                      <Button
                        type="primary"
                        icon={<SyncOutlined />}
                        aria-label="从钉钉拉取请假表"
                        loading={leaveSourceSync.loading}
                        disabled={!canDingtalkSync || !!runningModule}
                        onClick={() => void syncLeaveSourceFromDingtalk()}
                      >
                        从钉钉拉取请假表
                      </Button>
                    </span>
                  </Tooltip>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    按请假审批流程直接生成源表并回填；会自动扩展前后 {LEAVE_SYNC_PADDING_DAYS} 天查询，手动上传仍可作为兜底。
                  </Text>
                  {leaveSourceSync.lastSyncAt && (
                    <Text type="success" style={{ fontSize: 12 }}>
                      上次拉取：{leaveSourceSync.lastSyncAt}
                    </Text>
                  )}
                  {leaveSourceSync.error && (
                    <Text type="danger" style={{ fontSize: 12 }}>
                      拉取失败：{leaveSourceSync.error}
                    </Text>
                  )}
                </Space>
              </Col>
            </Row>
          </Card>
        )}

        {requiredFields.length > 0 && (
          <>
            <Title level={5} style={{ margin: 0 }}>
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                <span style={{
                  width: 3,
                  height: 18,
                  background: 'var(--color-error)',
                  borderRadius: 2,
                  display: 'inline-block',
                }} />
                必填文件
              </span>
            </Title>
            <Row gutter={[16, 16]}>
              {requiredFields.map((field) => (
                <Col xs={24} md={12} xl={field.multiple ? 12 : 8} key={field.name}>
                  {renderFileUploadCard(field)}
                </Col>
              ))}
            </Row>
          </>
        )}

        {optionalFields.length > 0 && (
          <>
            <Title level={5} style={{ margin: 0 }}>
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                <span style={{
                  width: 3,
                  height: 18,
                  background: 'var(--color-text-tertiary)',
                  borderRadius: 2,
                  display: 'inline-block',
                }} />
                可选文件
              </span>
            </Title>
            <Row gutter={[16, 16]}>
              {optionalFields.map((field) => (
                <Col xs={24} md={12} xl={8} key={field.name}>
                  {renderFileUploadCard(field)}
                </Col>
              ))}
            </Row>
          </>
        )}

        {(config.textFields || []).length > 0 && (
          <Card size="small" style={{ borderRadius: 'var(--radius-lg)' }}>
            <Row gutter={[16, 16]}>
              {(config.textFields || []).map((field) => {
                const hasValue = Boolean((textValues[field.name] || '').trim())
                return (
                  <Col xs={24} md={12} key={field.name}>
                    <Space direction="vertical" size={4} style={{ width: '100%' }}>
                      <Space size={4}>
                        <Text strong>{field.label}</Text>
                        {hasValue
                          ? <Tag color="success" style={{ fontSize: 11 }}>已自定义</Tag>
                          : <Tag style={{ fontSize: 11 }}>使用默认值</Tag>}
                      </Space>
                      <Input.TextArea
                        autoSize={{ minRows: 2, maxRows: 6 }}
                        value={textValues[field.name] || ''}
                        placeholder={defaultsLoading ? '正在加载默认名单...' : field.placeholder}
                        onChange={(event) => setTextValues((prev) => ({ ...prev, [field.name]: event.target.value }))}
                      />
                    </Space>
                  </Col>
                )
              })}
            </Row>
          </Card>
        )}

        {config.key === 'leave' && renderMaternityLeaveOverrides()}

        {config.key === 'overtime' && (
          <Collapse
            items={[{
              key: 'rules',
              label: (
                <Space>
                  <ToolOutlined />
                  加班规则配置
                  <Tag color={appliedRules.source === 'custom' ? 'success' : 'default'}>
                    {appliedRules.source === 'custom' ? '自定义规则' : '默认规则'}
                  </Tag>
                </Space>
              ),
              children: (
                <OvertimeRulesEditor
                  value={appliedRules}
                  onChange={setAppliedRules}
                  canEdit={canEditRules}
                  disabledReason="你缺少考勤工具箱规则编辑权限，需要联系管理员添加"
                />
              ),
            }]}
          />
        )}

        <Card
          size="small"
          style={{
            position: 'sticky',
            bottom: 0,
            zIndex: 10,
            boxShadow: '0 -4px 16px rgba(0,0,0,0.06)',
            borderRadius: 'var(--radius-lg)',
            background: 'var(--color-bg-card)',
          }}
          styles={{ body: { padding: '12px 16px' } }}
        >
          <Row justify="space-between" align="middle" gutter={[12, 12]}>
            <Col flex="1 1 auto" style={{ minWidth: 0 }}>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  <InfoCircleOutlined style={{ marginRight: 4 }} />
                  {config.key === 'overtime'
                    ? `当前计算使用${appliedRules.source === 'custom' ? '自定义' : '默认'}规则；结果可单独或 ZIP 下载`
                    : config.key === 'subsidy'
                      ? `法定节假日使用${customHolidayCount > 0 ? '当前加班规则配置' : '作息表'}；异常人员写入审计工作表`
                    : '计算结果将自动下载；多文件时提供 ZIP'}
                </Text>
                {requiredFields.length > 0 && (
                  <Space wrap size={[6, 6]}>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      必填 {requiredReadyCount}/{requiredFields.length}
                    </Text>
                    {requiredFields.map((field) => {
                      const ready = (fileLists[field.name] || []).length > 0
                      return (
                        <Tag
                          key={field.name}
                          color={ready ? 'success' : missingFieldName === field.name ? 'error' : 'default'}
                          style={{ margin: 0, cursor: ready ? 'default' : 'pointer' }}
                          onClick={() => {
                            if (!ready) focusMissingField(field.name)
                          }}
                        >
                          {ready ? <CheckOutlined style={{ marginRight: 4 }} /> : null}
                          {field.label}
                        </Tag>
                      )
                    })}
                  </Space>
                )}
              </Space>
            </Col>
            <Col>
              <Tooltip title={!canOperate ? '你缺少考勤工具箱操作权限，需要联系管理员添加' : undefined}>
                <span style={{ display: 'inline-block' }}>
                  <Button
                    type="primary"
                    size="large"
                    icon={<CalculatorOutlined />}
                    loading={runningModule === config.key}
                    disabled={!canOperate || !!runningModule}
                    onClick={() => handleRun(config.key)}
                  >
                    开始计算
                  </Button>
                </span>
              </Tooltip>
            </Col>
          </Row>
        </Card>

        {lastRun && lastRun.module === config.key && (
          <Card size="small" title="最近结果" style={{ borderRadius: 'var(--radius-lg)' }}>
            <Space direction="vertical" style={{ width: '100%' }} size={8}>
              <Space wrap>
                <Tag color="success" icon={<CheckCircleOutlined />}>计算成功</Tag>
                {lastRun.expires_at && (
                  <Text type="secondary" style={{ fontSize: 12 }}>结果有效至 {lastRun.expires_at}</Text>
                )}
              </Space>
              {subsidyAudit?.missingAttendanceCount ? (
                <Alert
                  type="warning"
                  showIcon
                  message={`有 ${subsidyAudit.missingAttendanceCount} 人缺少考勤记录`}
                  description={`${subsidyAudit.missingAttendanceNames.join('、')}。相关补贴字段保持空值，详细说明见结果文件“异常审计”工作表。`}
                />
              ) : null}
              {subsidyAudit?.holidayConflictCount ? (
                <Alert
                  type="warning"
                  showIcon
                  message={`法定节假日口径存在 ${subsidyAudit.holidayConflictCount} 个日期差异`}
                  description="本次已优先使用加班规则配置，差异日期已写入结果文件“异常审计”工作表。"
                />
              ) : null}
              {formatRunStats(lastRun.stats as Record<string, unknown> | undefined).length > 0 && (
                <Space wrap size={[6, 6]}>
                  {formatRunStats(lastRun.stats as Record<string, unknown> | undefined).map((item) => (
                    <Tag key={`${item.label}-${item.text}`} style={{ margin: 0 }}>
                      {item.label}: {item.text}
                    </Tag>
                  ))}
                </Space>
              )}
              <Collapse
                size="small"
                items={[{
                  key: 'tech',
                  label: '技术信息',
                  children: <Text type="secondary" style={{ fontSize: 12 }}>run_id: {lastRun.run_id}</Text>,
                }]}
              />
              <Space wrap>
                {(lastRun.files || []).filter((f) => f.kind !== 'meta').map((f) => (
                  <Button
                    key={f.file_key}
                    size="small"
                    icon={<DownloadOutlined />}
                    onClick={() => handleDownloadRunFile(f.file_key, f.file_name)}
                  >
                    {f.file_name}
                    {typeof f.row_count === 'number' ? ` (${f.row_count}行)` : ''}
                  </Button>
                ))}
                <Button size="small" type="primary" icon={<CloudDownloadOutlined />} onClick={handleDownloadRunZip}>
                  全部 ZIP 下载
                </Button>
              </Space>
              {config.key === 'overtime' && (
                <Card
                  size="small"
                  type="inner"
                  title="结果预览（前 200 行）"
                  loading={previewLoading}
                  style={{ marginTop: 8 }}
                >
                  {previewRows.length === 0 ? (
                    <Empty description={previewLoading ? '加载预览中…' : '暂无预览（不影响下载）'} />
                  ) : (
                    <Table
                      size="small"
                      pagination={false}
                      scroll={{ x: true, y: 360 }}
                      rowKey={(row) => JSON.stringify(row)}
                      dataSource={previewRows}
                      columns={Object.keys(previewRows[0] || {}).map((key) => ({
                        title: key,
                        dataIndex: key,
                        key,
                        ellipsis: true,
                        width: 140,
                      }))}
                    />
                  )}
                </Card>
              )}
            </Space>
          </Card>
        )}

        {(runLogByModule[config.key] || '') && (
          <Collapse
            items={[{
              key: 'log',
              label: <Space><InfoCircleOutlined />运行日志</Space>,
              children: (
                <pre style={{
                  margin: 0,
                  padding: 12,
                  background: 'var(--color-bg-layout)',
                  borderRadius: 6,
                  fontSize: 12,
                  lineHeight: 1.6,
                  maxHeight: 300,
                  overflow: 'auto',
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-all',
                }}>
                  {runLogByModule[config.key]}
                </pre>
              ),
            }]}
          />
        )}
      </Space>
    )
  }

  const renderDingtalkSync = (config: ModuleConfig) => {
    const flowOptions = [
      { label: '请假', value: 'leave' },
      { label: '加班', value: 'overtime' },
      { label: '补卡', value: 'attendance_correction' },
      { label: '岗位异动', value: 'position_transfer' },
    ]

    const downloadableFiles = getAttendanceToolboxDownloadableFiles(lastRun)
    const hasDownloadableResult = downloadableFiles.length > 0

    return (
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message={config.description}
          description="同步过程会按所选流程逐页拉取钉钉审批数据，流程越多耗时越久。下方一键联动可在同一次请求内直接生成请假/加班明细。"
        />

        <Card size="small" title="同步参数" style={{ borderRadius: 'var(--radius-lg)' }}>
          <Row gutter={[16, 16]}>
            <Col xs={24} md={12}>
              <Space direction="vertical" size={8} style={{ width: '100%' }}>
                <Text strong>同步日期范围 *</Text>
                <RangePicker
                  style={{ width: '100%' }}
                  value={dingtalkDateRange as RangePickerProps['value']}
                  onChange={(dates) => setDingtalkDateRange(dates as [dayjs.Dayjs | null, dayjs.Dayjs | null] | null)}
                  placeholder={['开始日期', '结束日期']}
                  aria-label="钉钉同步日期范围"
                />
              </Space>
            </Col>
            <Col xs={24} md={12}>
              <Space direction="vertical" size={8} style={{ width: '100%' }}>
                <Text strong>审批发起时间前后扩展天数</Text>
                <InputNumber
                  style={{ width: '100%' }}
                  min={0}
                  max={365}
                  value={dingtalkPaddingDays}
                  onChange={(value) => setDingtalkPaddingDays(value ?? 31)}
                />
                <Text type="secondary">钉钉审批列表只能按发起时间查。为避免提前申请的跨区间请假/加班漏掉，会在所选日期前后扩展查询。</Text>
              </Space>
            </Col>
          </Row>
        </Card>

        <Card size="small" title="同步流程" style={{ borderRadius: 'var(--radius-lg)' }}>
          <Checkbox.Group
            options={flowOptions}
            value={dingtalkFlowKeys}
            onChange={(values) => setDingtalkFlowKeys(values as string[])}
          />
        </Card>

        <Card size="small" title="拉取限制" style={{ borderRadius: 'var(--radius-lg)' }}>
          <Row gutter={[16, 16]}>
            <Col xs={24} md={12}>
              <Space direction="vertical" size={8} style={{ width: '100%' }}>
                <Checkbox
                  checked={dingtalkUnlimited}
                  onChange={(e) => setDingtalkUnlimited(e.target.checked)}
                >
                  全量拉取，不限制条数
                </Checkbox>
                <Text type="secondary">开启后会一直分页拉到钉钉返回没有下一页，避免因为条数上限漏数据。</Text>
              </Space>
            </Col>
            <Col xs={24} md={12}>
              <Space direction="vertical" size={8} style={{ width: '100%' }}>
                <Text strong>每类流程最多拉取审批数</Text>
                <InputNumber
                  style={{ width: '100%' }}
                  min={1}
                  max={10000}
                  value={dingtalkMaxInstances}
                  disabled={dingtalkUnlimited}
                  onChange={(value) => setDingtalkMaxInstances(value ?? 100)}
                />
                <Text type="secondary">仅在关闭全量拉取时生效，用于排查字段或小样本测试。</Text>
              </Space>
            </Col>
          </Row>
        </Card>

        <Card size="small" title="一键同步并生成请假/加班" style={{ borderRadius: 'var(--radius-lg)' }}>
          <Space direction="vertical" size={12} style={{ width: '100%' }}>
            <Text type="secondary">
              钉钉同步后，在同一次请求内直接用中间表计算请假/加班（需作息表；可选排班/考勤/花名册）。
              当前加班规则：{appliedRules.source === 'custom' ? '自定义规则' : '默认规则'}。
            </Text>
            <Space wrap>
              <Checkbox checked={quickRunLeave} onChange={(e) => setQuickRunLeave(e.target.checked)}>
                生成请假明细
              </Checkbox>
              <Checkbox checked={quickRunOvertime} onChange={(e) => setQuickRunOvertime(e.target.checked)}>
                生成加班明细
              </Checkbox>
            </Space>
            <Tooltip
              title={
                !canQuick
                  ? '你缺少一键联动所需权限（操作 + 钉钉同步），需要联系管理员添加'
                  : undefined
              }
            >
              <span style={{ display: 'inline-block' }}>
                <Button
                  type="default"
                  icon={<CalculatorOutlined />}
                  loading={runningModule === 'quick'}
                  disabled={!canQuick || !!runningModule || (!quickRunLeave && !quickRunOvertime)}
                  onClick={handleQuickWorkflow}
                >
                  一键同步并生成请假/加班
                </Button>
              </span>
            </Tooltip>
          </Space>
        </Card>

        {lastRun && (lastRun.module === 'dingtalk_sync' || lastRun.module === 'quick') && (
          <Card size="small" title="同步/联动结果" style={{ borderRadius: 'var(--radius-lg)' }}>
            <Space direction="vertical" style={{ width: '100%' }} size={8}>
              <Space wrap>
                {hasDownloadableResult ? (
                  <Tag color="success" icon={<CheckCircleOutlined />}>同步完成</Tag>
                ) : (
                  <Tag color="warning" icon={<ExclamationCircleOutlined />}>未生成结果</Tag>
                )}
                {lastRun.expires_at && (
                  <Text type="secondary" style={{ fontSize: 12 }}>结果有效至 {lastRun.expires_at}</Text>
                )}
              </Space>
              {formatRunStats(lastRun.stats as Record<string, unknown> | undefined).length > 0 && (
                <Space wrap size={[6, 6]}>
                  {formatRunStats(lastRun.stats as Record<string, unknown> | undefined).map((item) => (
                    <Tag key={`${item.label}-${item.text}`} style={{ margin: 0 }}>
                      {item.label}: {item.text}
                    </Tag>
                  ))}
                </Space>
              )}
              <Collapse
                size="small"
                items={[{
                  key: 'tech',
                  label: '技术信息',
                  children: <Text type="secondary" style={{ fontSize: 12 }}>run_id: {lastRun.run_id}</Text>,
                }]}
              />
              {!hasDownloadableResult && (
                <Alert
                  type="warning"
                  showIcon
                  message="本次没有生成可下载文件"
                  description="请查看下方同步日志；如果提示“未配置流程码”，需要补齐服务器钉钉流程码配置后重新同步。"
                />
              )}
              {hasDownloadableResult && (
                <Space wrap>
                  {downloadableFiles.map((f) => (
                    <Button
                      key={f.file_key}
                      size="small"
                      icon={<DownloadOutlined />}
                      onClick={() => handleDownloadRunFile(f.file_key, f.file_name)}
                    >
                      {f.file_name}
                      {typeof f.row_count === 'number' ? ` (${f.row_count}行)` : ''}
                    </Button>
                  ))}
                  <Button size="small" type="primary" icon={<CloudDownloadOutlined />} onClick={handleDownloadRunZip}>
                    全部 ZIP 下载
                  </Button>
                </Space>
              )}
              {(runLogByModule[config.key] || runLogByModule.quick || '') && (
                <Collapse
                  items={[{
                    key: 'sync-log',
                    label: '同步日志',
                    children: (
                      <pre style={{
                        margin: 0,
                        padding: 12,
                        background: 'var(--color-bg-layout)',
                        borderRadius: 6,
                        fontSize: 12,
                        maxHeight: 240,
                        overflow: 'auto',
                        whiteSpace: 'pre-wrap',
                      }}
                      >
                        {runLogByModule[config.key] || runLogByModule.quick}
                      </pre>
                    ),
                  }]}
                />
              )}
            </Space>
          </Card>
        )}

        <Card
          size="small"
          style={{
            position: 'sticky',
            bottom: 0,
            zIndex: 10,
            boxShadow: '0 -4px 16px rgba(0,0,0,0.06)',
            borderRadius: 'var(--radius-lg)',
          }}
          styles={{ body: { padding: '12px 16px' } }}
        >
          <Row justify="end" align="middle">
            <Tooltip title={!canDingtalkSync ? '你缺少考勤工具箱钉钉同步权限，需要联系管理员添加' : undefined}>
              <span style={{ display: 'inline-block' }}>
                <Button
                  type="primary"
                  size="large"
                  icon={<SyncOutlined />}
                  loading={runningModule === config.key}
                  disabled={!canDingtalkSync || !!runningModule}
                  onClick={() => handleRun(config.key)}
                >
                  从钉钉同步并生成中间表
                </Button>
              </span>
            </Tooltip>
          </Row>
        </Card>
      </Space>
    )
  }

  const renderHero = () => (
    <Card
      style={{
        marginBottom: 16,
        borderRadius: 'var(--radius-xl)',
        border: '1px solid var(--color-border)',
        background: 'linear-gradient(135deg, rgba(255,255,255,0.96), rgba(238,242,255,0.92))',
        boxShadow: '0 4px 20px rgba(67, 56, 202, 0.06)',
      }}
      styles={{ body: { padding: '20px 24px' } }}
    >
      <Row align="middle" gutter={[20, 16]}>
        <Col flex="1 1 auto" style={{ minWidth: 0 }}>
          <Space align="start" size={16}>
            <div style={{
              width: 52,
              height: 52,
              borderRadius: 12,
              background: 'linear-gradient(135deg, #eef2ff, #ecfeff)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 28,
              flex: '0 0 52px',
              boxShadow: 'inset 0 0 0 1px rgba(148, 163, 184, 0.22)',
            }}>
              📊
            </div>
            <div>
              <Title level={3} style={{ margin: 0, fontWeight: 700 }}>
                考勤数据处理工具
              </Title>
              <Paragraph style={{ margin: '4px 0 0', color: 'var(--color-text-secondary)', fontSize: 13 }}>
                集中处理请假、加班、补贴扣款、最终表和兼职汇总，减少月末重复整理。
              </Paragraph>
            </div>
          </Space>
        </Col>
        <Col flex="0 0 auto">
          <Space size={8} wrap>
            {heroBadges.map((b) => (
              <Tag
                key={b.label}
                icon={b.icon}
                style={{
                  borderRadius: 99,
                  padding: '4px 12px',
                  fontSize: 13,
                  fontWeight: 600,
                  background: '#fff',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-text-secondary)',
                }}
              >
                {b.label}
              </Tag>
            ))}
          </Space>
        </Col>
      </Row>
    </Card>
  )

  const renderToolbar = () => (
    <Collapse
      style={{ marginBottom: 16 }}
      items={[{
        key: 'templates',
        label: (
          <Space>
            <CloudDownloadOutlined style={{ color: 'var(--color-primary)' }} />
            <Text strong>模板下载中心</Text>
            <Text type="secondary">默认折叠，需要空白模板时再展开下载</Text>
          </Space>
        ),
        children: (
          <Row align="middle" gutter={[12, 8]} justify="space-between">
            <Col>
              <Text type="secondary">下载空白模板后填写上传</Text>
            </Col>
            <Col>
              <Button
                type="primary"
                size="small"
                icon={<CloudDownloadOutlined />}
                loading={downloadingTemplate === '__all__'}
                disabled={!!downloadingTemplate && downloadingTemplate !== '__all__'}
                onClick={downloadAllTemplates}
              >
                下载全部模板（zip）
              </Button>
            </Col>
          </Row>
        ),
      }]}
    />
  )

  const renderFixedConfig = () => (
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          这些值已内置默认值，仅在人员/规则发生变化时修改
        </Text>

        <Row gutter={[16, 16]}>
          <Col xs={24} md={8}>
            <Card size="small" style={{ background: 'var(--color-bg-layout)', borderRadius: 8 }}>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Text strong style={{ fontSize: 12 }}>特殊名单（实习生）</Text>
                <Text type="secondary" style={{ fontSize: 11 }}>命中后写入"实习生请假明细"子表</Text>
                <div style={{ marginTop: 4 }}>
                  {(textValues.leave_special_names || '').split('、').filter(Boolean).map((name) => (
                    <Tag key={name} color="blue" style={{ fontSize: 11, margin: '2px' }}>{name}</Tag>
                  ))}
                </div>
              </Space>
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card size="small" style={{ background: 'var(--color-bg-layout)', borderRadius: 8 }}>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Text strong style={{ fontSize: 12 }}>成都作息名单</Text>
                <Text type="secondary" style={{ fontSize: 11 }}>部门名称不含"成都"但按成都作息计算的人员</Text>
                <div style={{ marginTop: 4 }}>
                  {(textValues.chengdu_schedule_names || '').split('、').filter(Boolean).map((name) => (
                    <Tag key={name} color="green" style={{ fontSize: 11, margin: '2px' }}>{name}</Tag>
                  ))}
                </div>
              </Space>
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card size="small" style={{ background: 'var(--color-bg-layout)', borderRadius: 8 }}>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Text strong style={{ fontSize: 12 }}>产研部门关键字</Text>
                <Text type="secondary" style={{ fontSize: 11 }}>用于识别产研休息日加班补贴</Text>
                <div style={{ marginTop: 4 }}>
                  {(textValues.sub_dept_keywords || '').split('、').filter(Boolean).map((name) => (
                    <Tag key={name} color="orange" style={{ fontSize: 11, margin: '2px' }}>{name}</Tag>
                  ))}
                </div>
              </Space>
            </Card>
          </Col>
        </Row>

        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <Card size="small" style={{ background: 'var(--color-bg-layout)', borderRadius: 8 }}>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Space size={4}>
                  <Text strong style={{ fontSize: 12 }}>花名册</Text>
                  <Tag color="blue" style={{ fontSize: 11 }}>可生成</Tag>
                  {rosterSync.loading
                    ? <SyncOutlined spin />
                    : (
                      <Tooltip title={!canOperate ? '你缺少考勤工具箱操作权限，需要联系管理员添加' : undefined}>
                        <span style={{ display: 'inline-block' }}>
                          <Button
                            type="link"
                            size="small"
                            icon={<SyncOutlined />}
                            disabled={!canOperate}
                            onClick={() => void generateRosterFromOrgData()}
                            style={{ fontSize: 11, height: 22, padding: '0 4px' }}
                          >
                            {rosterSync.error ? '重试生成' : '从组织数据生成'}
                          </Button>
                        </span>
                      </Tooltip>
                    )}
                </Space>
                <Text type="secondary" style={{ fontSize: 11 }}>
                  {canOperate
                    ? '页面加载时会自动从本地组织数据生成花名册（使用最近一次组织同步数据）；失败后可手动重试，也可以继续上传本地花名册'
                    : '当前账号无操作权限，请上传本地花名册，或联系管理员开通权限'}
                </Text>
                {rosterSync.lastSyncAt && (
                  <Text type="success" style={{ fontSize: 11 }}>
                    上次生成：{rosterSync.lastSyncAt}
                  </Text>
                )}
                {!rosterSync.loading && rosterSync.lastSyncAt && (
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    已自动回填到“加班明细 &gt; 花名册/员工信息表”和“最终汇总 &gt; 在职花名册”
                  </Text>
                )}
                {rosterSync.error && (
                  <Text type="danger" style={{ fontSize: 11 }}>
                    生成失败：{rosterSync.error}
                  </Text>
                )}
              </Space>
            </Card>
          </Col>
          <Col xs={24} md={12}>
            <Card size="small" style={{ background: 'var(--color-bg-layout)', borderRadius: 8 }}>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Space size={4}>
                  <Text strong style={{ fontSize: 12 }}>异动流程</Text>
                  <Tag color="blue" style={{ fontSize: 11 }}>可同步</Tag>
                  {transferSync.loading
                    ? <SyncOutlined spin />
                    : (
                      <Tooltip title={!canDingtalkSync ? '你缺少考勤工具箱钉钉同步权限，需要联系管理员添加' : undefined}>
                        <span style={{ display: 'inline-block' }}>
                          <Button
                            type="link"
                            size="small"
                            icon={<SyncOutlined />}
                            disabled={!canDingtalkSync}
                            onClick={() => void syncTransferFromDingtalk()}
                            style={{ fontSize: 11, height: 22, padding: '0 4px' }}
                          >
                            {transferSync.error ? '重试同步' : '从钉钉同步'}
                          </Button>
                        </span>
                      </Tooltip>
                    )}
                </Space>
                <Text type="secondary" style={{ fontSize: 11 }}>
                  {canDingtalkSync
                    ? '页面加载时会自动尝试同步；失败后可手动重试，也可以继续上传本地异动流程表'
                    : '当前账号无钉钉同步权限，请上传本地异动流程表，或联系管理员开通权限'}
                </Text>
                {transferSync.lastSyncAt && (
                  <Text type="success" style={{ fontSize: 11 }}>
                    上次同步：{transferSync.lastSyncAt}
                  </Text>
                )}
                {!transferSync.loading && transferSync.lastSyncAt && (
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    已自动回填到“最终汇总 &gt; 异动流程表”
                  </Text>
                )}
                {transferSync.error && (
                  <Text type="danger" style={{ fontSize: 11 }}>
                    同步失败：{transferSync.error}
                  </Text>
                )}
              </Space>
            </Card>
          </Col>
          <Col xs={24} md={12}>
            <Card size="small" style={{ background: 'var(--color-bg-layout)', borderRadius: 8 }}>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Space size={4}>
                  <Text strong style={{ fontSize: 12 }}>兼职月度打卡记录</Text>
                  <Tag color="blue" style={{ fontSize: 11 }}>可同步</Tag>
                  {parttimePunchSync.loading
                    ? <SyncOutlined spin />
                    : (
                      <Tooltip title={!canDingtalkSync ? '你缺少考勤工具箱钉钉同步权限，需要联系管理员添加' : '仅按用户点击执行，不会自动抓取'}>
                        <span style={{ display: 'inline-block' }}>
                          <Button
                            type="link"
                            size="small"
                            icon={<SyncOutlined />}
                            disabled={!canDingtalkSync}
                            onClick={() => void syncParttimeMonthlyPunch()}
                            style={{ fontSize: 11, height: 22, padding: '0 4px' }}
                          >
                            {parttimePunchSync.error ? '重试抓取' : '从钉钉抓取'}
                          </Button>
                        </span>
                      </Tooltip>
                    )}
                </Space>
                <Space size={8} align="start" style={{ width: '100%' }}>
                  <Text type="secondary" style={{ fontSize: 11, whiteSpace: 'nowrap' }}>月份</Text>
                  <DatePicker
                    picker="month"
                    value={parttimePunchMonth}
                    onChange={(value) => setParttimePunchMonth(value)}
                    format="YYYY-MM"
                    allowClear={false}
                    style={{ width: 120 }}
                    placeholder="选择月份"
                  />
                </Space>
                <Text type="secondary" style={{ fontSize: 11 }}>
                  {canDingtalkSync
                    ? '点击「从钉钉抓取」拉取该月打卡记录并匹配兼职花名册；失败后可手动上传本地 Excel，再次抓取会替换上一次结果'
                    : '当前账号无钉钉同步权限，请上传本地月度打卡记录 Excel，或联系管理员开通权限'}
                </Text>
                {parttimePunchSync.lastSyncAt && (
                  <Text type="success" style={{ fontSize: 11 }}>
                    上次抓取：{parttimePunchSync.lastSyncAt}
                  </Text>
                )}
                {!parttimePunchSync.loading && parttimePunchSync.lastSyncAt && (
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    已自动回填到「兼职汇总 &gt; 考勤明细」上传位
                  </Text>
                )}
                {parttimePunchSync.error && (
                  <Text type="danger" style={{ fontSize: 11 }}>
                    抓取失败：{parttimePunchSync.error}
                  </Text>
                )}
              </Space>
            </Card>
          </Col>
        </Row>

        <Row gutter={[16, 16]}>
          <Col xs={24} md={8}>
            <Card size="small" style={{ background: 'var(--color-bg-layout)', borderRadius: 8 }}>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Text strong style={{ fontSize: 12 }}>晚走补贴人员</Text>
                <Text type="secondary" style={{ fontSize: 11 }}>即使在排除部门中也允许按22点后打卡计算</Text>
                <div style={{ marginTop: 4 }}>
                  {(textValues.sub_late22_names || '').split('、').filter(Boolean).map((name) => (
                    <Tag key={name} color="purple" style={{ fontSize: 11, margin: '2px' }}>{name}</Tag>
                  ))}
                </div>
              </Space>
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card size="small" style={{ background: 'var(--color-bg-layout)', borderRadius: 8 }}>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Text strong style={{ fontSize: 12 }}>兼职特殊人员</Text>
                <Text type="secondary" style={{ fontSize: 11 }}>无请假按满勤处理</Text>
                <div style={{ marginTop: 4 }}>
                  {(textValues.part_special_names || '').split('、').filter(Boolean).map((name) => (
                    <Tag key={name} color="cyan" style={{ fontSize: 11, margin: '2px' }}>{name}</Tag>
                  ))}
                </div>
              </Space>
            </Card>
          </Col>
          <Col xs={24} md={8}>
            <Card size="small" style={{ background: 'var(--color-bg-layout)', borderRadius: 8 }}>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Text strong style={{ fontSize: 12 }}>加班规则配置</Text>
                <Text type="secondary" style={{ fontSize: 11 }}>倍数规则、部门匹配、排除名单</Text>
                <div style={{ marginTop: 4 }}>
                  <OvertimeRulesEditor
                    value={appliedRules}
                    onChange={setAppliedRules}
                    canEdit={canEditRules}
                    disabledReason="你缺少考勤工具箱规则编辑权限，需要联系管理员添加"
                  />
                </div>
              </Space>
            </Card>
          </Col>
        </Row>
      </Space>
  )

  const wizardCurrent = Math.max(
    0,
    MONTH_END_WIZARD_STEPS.findIndex((step) => step.key === activeModule),
  )

  return (
    <PageContainer
      title="考勤工具箱"
      icon={<ToolOutlined />}
      subtitle="在系统内上传 Excel、调用内置计算引擎，并下载生成结果。"
    >
      {messageContextHolder}
      {renderHero()}

      <Card
        size="small"
        data-testid="attendance-toolbox-wizard"
        style={{
          marginBottom: 16,
          borderRadius: 'var(--radius-lg)',
          border: '1px solid var(--color-border)',
        }}
        styles={{ body: { padding: '14px 16px' } }}
      >
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Space wrap>
            <Text strong>月末结账向导</Text>
            <Text type="secondary" style={{ fontSize: 12 }}>
              推荐顺序；点击步骤可切换模块，不强制锁步
            </Text>
          </Space>
          <Steps
            size="small"
            current={wizardCurrent >= 0 ? wizardCurrent : 0}
            onChange={(index) => {
              const step = MONTH_END_WIZARD_STEPS[index]
              if (step) setActiveModule(step.key)
            }}
            items={MONTH_END_WIZARD_STEPS.map((step) => ({
              title: step.title,
              description: step.hint,
              status: completedModules[step.key]
                ? 'finish'
                : step.key === activeModule
                  ? 'process'
                  : 'wait',
              icon: completedModules[step.key] ? <CheckCircleOutlined /> : undefined,
            }))}
          />
        </Space>
      </Card>

      <Collapse
        style={{ marginBottom: 16 }}
        items={[{
          key: 'fixed-config',
          label: (
            <Space>
              <FileProtectOutlined style={{ color: 'var(--color-primary)' }} />
              <Text strong>固定配置（名单 / 同步源）</Text>
              <Text type="secondary">默认折叠，仅在人员或规则变化时展开</Text>
            </Space>
          ),
          children: renderFixedConfig(),
        }]}
      />
      {renderToolbar()}
      <Tabs
        size="large"
        activeKey={activeModule}
        onChange={(key) => setActiveModule(key as ToolboxModuleKey)}
        items={modules.map((item) => ({
          key: item.key,
          label: (
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
              {completedModules[item.key] ? <CheckCircleOutlined style={{ color: 'var(--color-success)' }} /> : null}
              {item.key === 'final'
                ? <CloudDownloadOutlined />
                : item.key === 'dingtalk_sync'
                  ? <SyncOutlined />
                  : <FileExcelOutlined />}
              {item.title}
            </span>
          ),
          children: (
            <div style={{ paddingTop: 8 }}>
              {renderModule(item)}
            </div>
          ),
        }))}
      />
    </PageContainer>
  )
}

export default AttendanceToolbox
