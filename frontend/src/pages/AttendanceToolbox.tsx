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
  Input,
  InputNumber,
  Popover,
  Radio,
  Row,
  Space,
  Tabs,
  Tag,
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
import OvertimeRulesEditor from './OvertimeRulesEditor'
import { attendanceToolboxAPI } from '../services/api'

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

const modules: ModuleConfig[] = [
  {
    key: 'leave',
    title: '请假明细',
    description: '上传请假系统导出表和作息表，生成请假明细表。',
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
    description: '上传补贴扣款、签到、作息和考勤数据，生成核对表；存在差异时会返回压缩包。',
    outputName: '补贴扣款核对表.xlsx',
    zipOutputName: '补贴扣款结果.zip',
    fileFields: [
      { name: 'subsidy_src', label: '补贴扣款表', required: true, templateId: 'subsidy_source' },
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

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function getDownloadName(config: ModuleConfig, blob: Blob) {
  if (blob.type === 'application/zip' && config.zipOutputName) {
    return config.zipOutputName
  }
  return config.outputName
}

async function resolveErrorMessage(error: unknown) {
  if (axios.isAxiosError(error)) {
    const data = error.response?.data
    if (data instanceof Blob) {
      const text = await data.text()
      try {
        const parsed = JSON.parse(text)
        return parsed?.message || parsed?.error || '计算失败'
      } catch {
        return text || '计算失败'
      }
    }
    if (typeof data?.message === 'string') return data.message
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
      { label: '工号', required: true }, { label: '姓名', required: true },
      { label: '合同主体' }, { label: '一级部门' }, { label: '二级部门' }, { label: '三级部门' },
      { label: '岗位' }, { label: '员工类型' }, { label: '入职日期' }, { label: '离职日期' },
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
    { label: '补贴扣款表', badges: [
      { label: '姓名', required: true }, { label: '工号', required: true }, { label: '旷工天数', required: true },
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
  roster: ['overtime_roster', 'final_active'],
  transfer: ['final_transfer'],
} as const

// ── Main component ──

const AttendanceToolbox: React.FC = () => {
  const [fileLists, setFileLists] = useState<Record<string, UploadFile[]>>({})
  const [textValues, setTextValues] = useState<Record<string, string>>({})
  const [defaultsLoading, setDefaultsLoading] = useState(false)
  const [runningModule, setRunningModule] = useState<ToolboxModuleKey | null>(null)
  const [runLog, setRunLog] = useState<string>('')
  const [uploadWarnings, setUploadWarnings] = useState<Record<string, string[]>>({})
  const [auditWarnings, setAuditWarnings] = useState<string[]>([])
  const [templateMeta, setTemplateMeta] = useState<TemplateMetaItem[]>([])
  const [downloadingTemplate, setDownloadingTemplate] = useState<string | null>(null)
  const [requirementView, setRequirementView] = useState<Record<string, string>>({})
  const [rosterSync, setRosterSync] = useState<SyncState>({ loading: false })
  const [transferSync, setTransferSync] = useState<SyncState>({ loading: false })
  const autoSyncStartedRef = useRef(false)

  // DingTalk sync specific state
  const [dingtalkDateRange, setDingtalkDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null] | null>(null)
  const [dingtalkFlowKeys, setDingtalkFlowKeys] = useState<string[]>(['leave', 'overtime', 'attendance_correction', 'position_transfer'])
  const [dingtalkUnlimited, setDingtalkUnlimited] = useState(true)
  const [dingtalkMaxInstances, setDingtalkMaxInstances] = useState<number>(100)
  const [dingtalkPaddingDays, setDingtalkPaddingDays] = useState<number>(31)

  const moduleMap = useMemo(() => new Map(modules.map((item) => [item.key, item])), [])

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
          message.warning('默认名单加载失败，计算时仍会使用内置规则')
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
    setFileLists((prev) => ({ ...prev, [fieldName]: list }))
  }, [])

  const applySyncedFile = useCallback((fieldNames: string[], fileName: string, blob: Blob) => {
    const syncedAt = Date.now()
    const file = new File([blob], fileName, {
      type: blob.type || 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    })
    const uploadFile = Object.assign(file, {
      uid: `auto-sync-file-${syncedAt}`,
      lastModifiedDate: new Date(file.lastModified),
    }) as RcFile

    setFileLists((prev) => {
      const next = { ...prev }
      fieldNames.forEach((fieldName, index) => {
        next[fieldName] = [{
          uid: `auto-sync-${fieldName}-${syncedAt}-${index}`,
          name: file.name,
          status: 'done',
          percent: 100,
          size: file.size,
          type: file.type,
          originFileObj: uploadFile,
        }]
      })
      return next
    })

    setUploadWarnings((prev) => {
      const next = { ...prev }
      fieldNames.forEach((fieldName) => {
        delete next[fieldName]
      })
      return next
    })
  }, [])

  const syncRosterFromDingtalk = useCallback(async (mode: SyncMode = 'manual') => {
    setRosterSync({ loading: true })
    try {
      const today = dayjs()
      const start = today.startOf('month').format('YYYY-MM-DD')
      const end = today.endOf('month').format('YYYY-MM-DD')
      const blob = await attendanceToolboxAPI.runDingtalkSync({
        start_date: start,
        end_date: end,
        flow_keys: ['position_transfer'],
        padding_days: 90,
      }) as unknown as Blob

      if (blob.type === 'application/zip') {
        throw new Error('自动同步返回了多个文件，请改用手动上传')
      }

      applySyncedFile([...AUTO_SYNC_UPLOADS.roster], '花名册_钉钉自动同步.xlsx', blob)
      setRosterSync({ loading: false, lastSyncAt: dayjs().format('YYYY-MM-DD HH:mm') })
      if (mode === 'manual') {
        message.success('花名册同步完成，已自动回填到上传位')
      }
    } catch (error) {
      const errorMessage = await resolveErrorMessage(error)
      setRosterSync({ loading: false, error: errorMessage })
      if (mode === 'manual') {
        message.error(`花名册同步失败：${errorMessage}`)
      }
    }
  }, [applySyncedFile])

  const syncTransferFromDingtalk = useCallback(async (mode: SyncMode = 'manual') => {
    setTransferSync({ loading: true })
    try {
      const today = dayjs()
      const start = today.subtract(3, 'month').startOf('month').format('YYYY-MM-DD')
      const end = today.endOf('month').format('YYYY-MM-DD')
      const blob = await attendanceToolboxAPI.runDingtalkSync({
        start_date: start,
        end_date: end,
        flow_keys: ['position_transfer'],
        padding_days: 90,
      }) as unknown as Blob

      if (blob.type === 'application/zip') {
        throw new Error('自动同步返回了多个文件，请改用手动上传')
      }

      applySyncedFile([...AUTO_SYNC_UPLOADS.transfer], '异动流程_钉钉自动同步.xlsx', blob)
      setTransferSync({ loading: false, lastSyncAt: dayjs().format('YYYY-MM-DD HH:mm') })
      if (mode === 'manual') {
        message.success('异动流程同步完成，已自动回填到上传位')
      }
    } catch (error) {
      const errorMessage = await resolveErrorMessage(error)
      setTransferSync({ loading: false, error: errorMessage })
      if (mode === 'manual') {
        message.error(`异动流程同步失败：${errorMessage}`)
      }
    }
  }, [applySyncedFile])

  useEffect(() => {
    if (autoSyncStartedRef.current) return
    autoSyncStartedRef.current = true
    void syncRosterFromDingtalk('auto')
    void syncTransferFromDingtalk('auto')
  }, [syncRosterFromDingtalk, syncTransferFromDingtalk])

  const removeFieldFile = (fieldName: string, fileUid: string) => {
    setFileLists((prev) => ({
      ...prev,
      [fieldName]: (prev[fieldName] || []).filter((file) => file.uid !== fileUid),
    }))
    setUploadWarnings((prev) => {
      const next = { ...prev }
      delete next[fieldName]
      return next
    })
  }

  const runAuditForFiles = useCallback(async (config: ModuleConfig) => {
    const items = config.fileFields.flatMap((field) => (fileLists[field.name] || []).map((file) => ({
      file,
      field,
    })))
    if (items.length === 0) {
      setAuditWarnings([])
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
      setAuditWarnings(warnings)
      if (warnings.length > 0) {
        warnings.forEach((warning) => message.warning(warning))
      } else {
        message.success('文件检测通过')
      }
    } catch (error) {
      message.warning(`文件检测失败：${error instanceof Error ? error.message : '未知错误'}`)
      setAuditWarnings([])
    }
  }, [fileLists])

  const downloadTemplate = useCallback(async (templateId: string) => {
    setDownloadingTemplate(templateId)
    try {
      const blob = await attendanceToolboxAPI.exportTemplates(templateId) as unknown as Blob
      downloadBlob(blob, getTemplateDownloadFileName(templateId))
    } catch (error) {
      message.error(await resolveErrorMessage(error))
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
      message.error(await resolveErrorMessage(error))
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

  const handleRun = async (moduleKey: ToolboxModuleKey) => {
    const config = moduleMap.get(moduleKey)
    if (!config) return

    if (config.fileFields.length > 0) {
      await runAuditForFiles(config)
    }

    if (moduleKey === 'dingtalk_sync') {
      if (!dingtalkDateRange || !dingtalkDateRange[0] || !dingtalkDateRange[1]) {
        message.warning('请选择同步日期范围')
        return
      }
      if (dingtalkFlowKeys.length === 0) {
        message.warning('请至少选择一个同步流程')
        return
      }

      setRunningModule(moduleKey)
      try {
        const blob = await attendanceToolboxAPI.runDingtalkSync({
          start_date: dingtalkDateRange[0].format('YYYY-MM-DD'),
          end_date: dingtalkDateRange[1].format('YYYY-MM-DD'),
          flow_keys: dingtalkFlowKeys,
          max_instances: dingtalkUnlimited ? undefined : dingtalkMaxInstances,
          padding_days: dingtalkPaddingDays,
        }) as unknown as Blob
        downloadBlob(blob, getDownloadName(config, blob))
        message.success('同步完成，结果已下载')
      } catch (error) {
        message.error(await resolveErrorMessage(error))
      } finally {
        setRunningModule(null)
      }
      return
    }

    for (const field of config.fileFields) {
      if (field.required && !(fileLists[field.name] || []).length) {
        message.warning(`请上传${field.label}`)
        return
      }
    }
    if (moduleKey === 'parttime') {
      const hasSource = ['parttime_attendance_detail', 'parttime_monthly', 'parttime_schedules']
        .some((key) => (fileLists[key] || []).length > 0)
      if (!hasSource) {
        message.warning('请至少上传考勤明细、月度汇总或排班表中的一类')
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

    setRunningModule(moduleKey)
    try {
      const blob = await attendanceToolboxAPI.run(moduleKey, formData) as unknown as Blob
      downloadBlob(blob, getDownloadName(config, blob))
      message.success('计算完成，结果已下载')
    } catch (error) {
      message.error(await resolveErrorMessage(error))
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

    return (
      <Card
        size="small"
        style={{
          height: '100%',
          borderRadius: 'var(--radius-lg)',
          border: hasFile
            ? '1px solid var(--color-success)'
            : field.required
              ? '1px solid var(--color-error)'
              : '1px solid var(--color-border)',
          borderLeft: field.required ? '3px solid var(--color-error)' : undefined,
          transition: 'border-color 0.2s ease',
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
    )
  }

  const renderModule = (config: ModuleConfig) => {
    if (config.key === 'dingtalk_sync') {
      return renderDingtalkSync(config)
    }

    const requiredFields = config.fileFields.filter((f) => f.required)
    const optionalFields = config.fileFields.filter((f) => !f.required)

    return (
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        {renderUploadRequirements(config.key)}

        <Alert
          type="info"
          showIcon
          message={config.description}
          description="文件会上传到人事系统后端，由内置 Python 计算引擎生成结果。"
        />

        {auditWarnings.length > 0 && (
          <Alert
            type="warning"
            showIcon
            message={`文件检测发现 ${auditWarnings.length} 条警告`}
            description={
              <ul style={{ margin: 0, paddingLeft: 18 }}>
                {auditWarnings.map((warning, idx) => (
                  <li key={idx}>{warning}</li>
                ))}
              </ul>
            }
          />
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

        {config.key === 'overtime' && (
          <Collapse
            items={[{
              key: 'rules',
              label: <Space><ToolOutlined />加班规则配置</Space>,
              children: <OvertimeRulesEditor />,
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
            <Col>
              <Space>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  <InfoCircleOutlined style={{ marginRight: 4 }} />
                  计算结果将自动下载
                </Text>
              </Space>
            </Col>
            <Col>
              <Button
                type="primary"
                size="large"
                icon={<CalculatorOutlined />}
                loading={runningModule === config.key}
                onClick={() => handleRun(config.key)}
              >
                开始计算
              </Button>
            </Col>
          </Row>
        </Card>

        {runLog && (
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
                  {runLog}
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

    return (
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message={config.description}
          description="同步过程会按所选流程逐页拉取钉钉审批数据，流程越多耗时越久。"
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
                />
              </Space>
            </Col>
            <Col xs={24} md={12}>
              <Space direction="vertical" size={8} style={{ width: '100%' }}>
                <Text strong>审批发起时间前后扩展天数</Text>
                <InputNumber
                  style={{ width: '100%' }}
                  min={0}
                  max={90}
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
            <Button
              type="primary"
              size="large"
              icon={<SyncOutlined />}
              loading={runningModule === config.key}
              onClick={() => handleRun(config.key)}
            >
              从钉钉同步并生成中间表
            </Button>
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
    <Card
      size="small"
      style={{
        marginBottom: 16,
        borderRadius: 'var(--radius-lg)',
        background: 'var(--color-bg-container)',
        border: '1px solid var(--color-border)',
      }}
      styles={{ body: { padding: '14px 16px' } }}
    >
      <Row align="middle" gutter={[12, 8]} justify="space-between">
        <Col>
          <Space>
            <CloudDownloadOutlined style={{ color: 'var(--color-primary)', fontSize: 16 }} />
            <Text strong>模板下载中心</Text>
            <Text type="secondary">下载空白模板后填写上传</Text>
          </Space>
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
    </Card>
  )

  const renderFixedConfig = () => (
    <Card
      size="small"
      style={{
        marginBottom: 16,
        borderRadius: 'var(--radius-lg)',
        border: '1px solid var(--color-border)',
      }}
      styles={{ body: { padding: '14px 16px' } }}
    >
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <Space>
          <FileProtectOutlined style={{ color: 'var(--color-primary)', fontSize: 16 }} />
          <Text strong>固定配置（不随月份变化）</Text>
          <Text type="secondary">这些值已内置默认值，仅在人员/规则发生变化时修改</Text>
        </Space>

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
                  <Tag color="blue" style={{ fontSize: 11 }}>可同步</Tag>
                  {rosterSync.loading
                    ? <SyncOutlined spin />
                    : (
                      <Button
                        type="link"
                        size="small"
                        icon={<SyncOutlined />}
                        onClick={() => void syncRosterFromDingtalk()}
                        style={{ fontSize: 11, height: 22, padding: '0 4px' }}
                      >
                        {rosterSync.error ? '重试同步' : '从钉钉同步'}
                      </Button>
                    )}
                </Space>
                <Text type="secondary" style={{ fontSize: 11 }}>
                  页面加载时会自动尝试同步；失败后可手动重试，也可以继续上传本地花名册
                </Text>
                {rosterSync.lastSyncAt && (
                  <Text type="success" style={{ fontSize: 11 }}>
                    上次同步：{rosterSync.lastSyncAt}
                  </Text>
                )}
                {!rosterSync.loading && rosterSync.lastSyncAt && (
                  <Text type="secondary" style={{ fontSize: 11 }}>
                    已自动回填到“加班明细 &gt; 花名册/员工信息表”和“最终汇总 &gt; 在职花名册”
                  </Text>
                )}
                {rosterSync.error && (
                  <Text type="danger" style={{ fontSize: 11 }}>
                    同步失败：{rosterSync.error}
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
                      <Button
                        type="link"
                        size="small"
                        icon={<SyncOutlined />}
                        onClick={() => void syncTransferFromDingtalk()}
                        style={{ fontSize: 11, height: 22, padding: '0 4px' }}
                      >
                        {transferSync.error ? '重试同步' : '从钉钉同步'}
                      </Button>
                    )}
                </Space>
                <Text type="secondary" style={{ fontSize: 11 }}>
                  页面加载时会自动尝试同步；失败后可手动重试，也可以继续上传本地异动流程表
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
                  <OvertimeRulesEditor />
                </div>
              </Space>
            </Card>
          </Col>
        </Row>
      </Space>
    </Card>
  )

  return (
    <PageContainer
      title="考勤工具箱"
      icon={<ToolOutlined />}
      subtitle="在系统内上传 Excel、调用内置计算引擎，并下载生成结果。"
    >
      {renderHero()}
      {renderFixedConfig()}
      {renderToolbar()}
      <Tabs
        size="large"
        items={modules.map((item) => ({
          key: item.key,
          label: (
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
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
