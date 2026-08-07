import React, { useEffect, useMemo, useState } from 'react'
import type { Dayjs } from 'dayjs'
import dayjs from 'dayjs'
import {
  Alert,
  Button,
  Card,
  Col,
  DatePicker,
  Empty,
  Form,
  Input,
  List,
  Modal,
  Popconfirm,
  Row,
  Segmented,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import type { TableColumnsType } from 'antd'
import { CalendarOutlined, DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined, SyncOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { departmentAPI, shiftConfigAPI, userAPI, weekScheduleAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import { useAuthStore } from '../store/authStore'
import { formatDateTime } from '../utils/format'

const { Title, Text, Paragraph } = Typography
const { TextArea } = Input

const USERS_PAGE_SIZE = 200
const USERS_MAX_PAGES = 50 // 最多拉取 200*50=10000 人，覆盖全员选人

type ScopeType = 'company' | 'department' | 'user'
type WeekType = 'big' | 'small'
type HolidayType = 'holiday' | 'workday'
type PushTargetType = 'personal' | 'group'
type CalendarCellState = 'work' | 'rest' | 'holiday' | 'workday' | 'saturday-work' | 'outside'

interface UserOption {
  id: number
  user_id: string
  name: string
  department_id: string
}

interface DepartmentOption {
  id: number
  department_id: string
  name: string
}

interface ShiftOption {
  id: number
  name: string
}

interface WeekScheduleRule {
  id: number
  scope_type: ScopeType
  scope_id: string
  scope_name: string
  base_date: string
  pattern: 'big_first' | 'small_first'
  shift_id: number
  status: 'active' | 'inactive'
  created_at: string
  updated_at: string
}

interface WeekHolidayInfo {
  date: string
  name: string
  type: HolidayType
}

interface WeekCalendarItem {
  week_start: string
  week_end: string
  week_type: WeekType
  is_override: boolean
  saturday_work: boolean
  holidays: WeekHolidayInfo[] | null
}

interface HolidayRecord {
  id: number
  date: string
  name: string
  type: HolidayType
  year: number
  created_at: string
}

interface SyncLogRecord {
  id: number
  sync_type: 'to_dingtalk' | 'from_dingtalk' | string
  target_date?: string
  user_count: number
  status: 'success' | 'failed' | 'partial' | string
  message: string
  created_at: string
}

interface WeekScheduleGroupTarget {
  id: number
  group_name: string
  status: 'active' | 'unbound' | string
  bound_by_user_name: string
  bound_at: string
}

interface RuleFormValues {
  scope_type: ScopeType
  scope_target_id?: string
  base_date: Dayjs
  pattern: 'big_first' | 'small_first'
  shift_id?: number
  status: 'active' | 'inactive'
}

interface OverrideFormValues {
  week_type: WeekType
  reason?: string
}

interface HolidayFormValues {
  date: Dayjs
  name: string
  type: HolidayType
}

interface ShiftFormValues {
  name: string
  check_in_time: string
  check_out_time: string
}

interface MonthCalendarCell {
  date: string
  dayNumber: number
  inCurrentMonth: boolean
  state: CalendarCellState
  holidayLabel?: string
}

interface MonthCalendarRow {
  week: WeekCalendarItem
  cells: MonthCalendarCell[]
}

interface MonthCalendarSection {
  month: string
  rows: MonthCalendarRow[]
}

function unwrapData<T>(response: any, fallback: T): T {
  return (response?.data as T) ?? fallback
}

function getItems<T>(response: any): T[] {
  return Array.isArray(response?.data?.items) ? (response.data.items as T[]) : []
}

function getPagedTotal(response: any): number {
  const total = Number(response?.data?.total)
  return Number.isFinite(total) && total >= 0 ? total : 0
}

/** 分页拉全量用户，避免 page_size 被后端截断后只能看到前 100 人 */
async function fetchAllUsersForPicker(): Promise<UserOption[]> {
  const first = await userAPI.getUsers({ page: 1, page_size: USERS_PAGE_SIZE, status: 'active' })
  const items = getItems<UserOption>(first)
  const total = getPagedTotal(first)
  const pageSize = Math.max(1, Math.min(USERS_PAGE_SIZE, items.length || USERS_PAGE_SIZE))
  const totalPages = Math.min(USERS_MAX_PAGES, Math.max(1, Math.ceil((total || items.length) / pageSize)))

  const byID = new Map<string, UserOption>()
  const add = (list: UserOption[]) => {
    list.forEach((u) => {
      if (u?.user_id) byID.set(u.user_id, u)
    })
  }
  add(items)

  if (totalPages > 1) {
    const rest = await Promise.all(
      Array.from({ length: totalPages - 1 }, (_, i) =>
        userAPI.getUsers({ page: i + 2, page_size: USERS_PAGE_SIZE, status: 'active' }),
      ),
    )
    rest.forEach((res) => add(getItems<UserOption>(res)))
  }

  return Array.from(byID.values()).sort((a, b) => (a.name || '').localeCompare(b.name || '', 'zh-CN'))
}

function getScopeLabel(scopeType: ScopeType) {
  if (scopeType === 'company') return '全公司'
  if (scopeType === 'department') return '部门'
  return '个人'
}

function getPatternLabel(pattern: WeekScheduleRule['pattern']) {
  return pattern === 'big_first' ? '基准周为大周' : '基准周为小周'
}

function getWeekTypeMeta(weekType: WeekType) {
  if (weekType === 'small') {
    return {
      label: '小周',
      restLabel: '单休',
      color: 'var(--color-warning-dark)',
      tagColor: 'orange' as const,
      background: '#fff7e6',
      borderColor: '#ffd591',
      // canvas 不能用 CSS 变量，保留实色
      solidColor: '#b45309',
      solidBackground: '#fff7e6',
    }
  }

  return {
    label: '大周',
    restLabel: '双休',
    color: 'var(--color-primary)',
    tagColor: 'blue' as const,
    background: 'var(--color-primary-bg)',
    borderColor: '#bfdbfe',
    solidColor: '#2563eb',
    solidBackground: '#eaf2ff',
  }
}

function getStatusTag(status: string) {
  if (status === 'active') return <Tag color="success" style={{ borderRadius: 6, fontWeight: 600, margin: 0 }}>生效中</Tag>
  if (status === 'inactive') return <Tag style={{ borderRadius: 6, fontWeight: 600, margin: 0 }}>已停用</Tag>
  if (status === 'success') return <Tag color="success" style={{ borderRadius: 6, fontWeight: 600, margin: 0 }}>成功</Tag>
  if (status === 'partial') return <Tag color="warning" style={{ borderRadius: 6, fontWeight: 600, margin: 0 }}>部分成功</Tag>
  if (status === 'failed') return <Tag color="error" style={{ borderRadius: 6, fontWeight: 600, margin: 0 }}>失败</Tag>
  return <Tag style={{ borderRadius: 6, fontWeight: 600, margin: 0 }}>{status}</Tag>
}

function getSyncTypeLabel(syncType: string) {
  return syncType === 'from_dingtalk' ? '从钉钉拉取' : '推送到钉钉'
}

function getErrorMessage(error: unknown, fallback: string) {
  if (typeof error === 'object' && error && 'response' in error) {
    const maybeResponse = (error as { response?: { data?: { message?: string } } }).response
    if (maybeResponse?.data?.message) {
      return maybeResponse.data.message
    }
  }
  if (error instanceof Error && error.message) {
    return error.message
  }
  return fallback
}

function getDayState(week: WeekCalendarItem, date: Dayjs): { state: CalendarCellState; holidayLabel?: string } {
  const dateStr = date.format('YYYY-MM-DD')
  const holiday = week.holidays?.find((item) => item.date === dateStr)
  if (holiday) {
    return {
      state: holiday.type === 'holiday' ? 'holiday' : 'workday',
      holidayLabel: holiday.name,
    }
  }

  if (date.day() === 0) {
    return { state: 'rest' }
  }

  if (date.day() === 6) {
    return { state: week.saturday_work ? 'saturday-work' : 'rest' }
  }

  return { state: 'work' }
}

function getCellStyle(state: CalendarCellState): React.CSSProperties {
  if (state === 'outside') {
    return { background: '#f3f5f8', color: '#b0b8c4' }
  }
  if (state === 'holiday') {
    return { background: '#ff4d4f', color: '#ffffff' }
  }
  if (state === 'rest') {
    return { background: '#ffffff', color: '#1f2937' }
  }
  // work / workday / saturday-work — warmer yellow for clearer contrast
  return { background: '#fff566', color: '#1f2937' }
}

function getCellCanvasColors(state: CalendarCellState): { background: string; color: string } {
  if (state === 'outside') return { background: '#f3f4f6', color: '#9ca3af' }
  if (state === 'holiday') return { background: '#ff4d4f', color: '#ffffff' }
  if (state === 'rest') return { background: '#ffffff', color: '#1f2937' }
  return { background: '#fff566', color: '#1f2937' }
}

function buildMonthCalendarSections(calendarItems: WeekCalendarItem[]): MonthCalendarSection[] {
  const monthMap = new Map<string, MonthCalendarSection>()

  calendarItems.forEach((week) => {
    const weekStart = dayjs(week.week_start)
    const weekDays = Array.from({ length: 7 }, (_, offset) => weekStart.add(offset, 'day'))
    const months = Array.from(new Set(weekDays.map((day) => day.format('YYYY-MM'))))

    months.forEach((month) => {
      if (!monthMap.has(month)) {
        monthMap.set(month, { month, rows: [] })
      }

      const cells = weekDays.map((day) => {
        const inCurrentMonth = day.format('YYYY-MM') === month
        const info = inCurrentMonth ? getDayState(week, day) : { state: 'outside' as CalendarCellState }
        return {
          date: day.format('YYYY-MM-DD'),
          dayNumber: day.date(),
          inCurrentMonth,
          state: info.state,
          holidayLabel: info.holidayLabel,
        }
      })

      monthMap.get(month)?.rows.push({ week, cells })
    })
  })

  return Array.from(monthMap.values()).sort((a, b) => a.month.localeCompare(b.month))
}

function getMobileCalendarNote(cell: MonthCalendarCell, selectedUserEndTime: string | null) {
  if (!cell.inCurrentMonth) return ''
  if (cell.holidayLabel) return cell.holidayLabel
  if (cell.state === 'holiday') return '放假'
  if (cell.state === 'workday') return '调班'
  if (cell.state === 'saturday-work') return '上班'
  if (cell.state === 'rest') return '休息'
  return selectedUserEndTime || '工作'
}

/** 根据当前月份日历数据生成 PNG（与网页同风格：标题条 + 第N周 + 单休/双休） */
async function renderMonthSchedulePng(section: MonthCalendarSection, title: string): Promise<Blob> {
  const colCount = 9
  const colWidth = 96
  const headerH = 44
  const titleH = 64
  const rowH = 72
  const pad = 24
  const width = pad * 2 + colCount * colWidth
  const height = pad * 2 + titleH + headerH + section.rows.length * rowH + 12

  const canvas = document.createElement('canvas')
  const scale = 2
  canvas.width = width * scale
  canvas.height = height * scale
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    throw new Error('当前浏览器不支持 Canvas，无法生成作息表图片')
  }
  ctx.scale(scale, scale)

  ctx.fillStyle = '#ffffff'
  ctx.fillRect(0, 0, width, height)

  ctx.fillStyle = '#172033'
  ctx.font = 'bold 26px "Microsoft YaHei", "PingFang SC", sans-serif'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(title, width / 2, pad + titleH / 2)

  const tableTop = pad + titleH
  const tableLeft = pad
  const headers = ['周数', '周一', '周二', '周三', '周四', '周五', '周六', '周日', '大小周']

  headers.forEach((label, i) => {
    const x = tableLeft + i * colWidth
    ctx.fillStyle = '#f8fafc'
    ctx.fillRect(x, tableTop, colWidth, headerH)
    ctx.strokeStyle = '#e5e7eb'
    ctx.strokeRect(x, tableTop, colWidth, headerH)
    ctx.fillStyle = '#374151'
    ctx.font = 'bold 13px "Microsoft YaHei", "PingFang SC", sans-serif'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(label, x + colWidth / 2, tableTop + headerH / 2)
  })

  section.rows.forEach((row, rowIndex) => {
    const y = tableTop + headerH + rowIndex * rowH
    const weekMeta = getWeekTypeMeta(row.week.week_type)
    const weekIndexLabel = `第${rowIndex + 1}周`
    const rangeLabel = `${dayjs(row.week.week_start).format('MM/DD')}-${dayjs(row.week.week_end).format('MM/DD')}`

    ctx.fillStyle = '#ffffff'
    ctx.fillRect(tableLeft, y, colWidth, rowH)
    ctx.strokeStyle = '#e5e7eb'
    ctx.strokeRect(tableLeft, y, colWidth, rowH)
    ctx.fillStyle = '#111827'
    ctx.font = 'bold 14px "Microsoft YaHei", "PingFang SC", sans-serif'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(weekIndexLabel, tableLeft + colWidth / 2, y + rowH / 2 - 10)
    ctx.fillStyle = '#9ca3af'
    ctx.font = '12px "Microsoft YaHei", "PingFang SC", sans-serif'
    ctx.fillText(rangeLabel, tableLeft + colWidth / 2, y + rowH / 2 + 12)

    row.cells.forEach((cell, cellIndex) => {
      const x = tableLeft + (cellIndex + 1) * colWidth
      const colors = getCellCanvasColors(cell.state)
      ctx.fillStyle = colors.background
      ctx.fillRect(x, y, colWidth, rowH)
      ctx.strokeStyle = '#e5e7eb'
      ctx.strokeRect(x, y, colWidth, rowH)
      ctx.fillStyle = colors.color
      ctx.font = 'bold 18px "Microsoft YaHei", "PingFang SC", sans-serif'
      ctx.textAlign = 'center'
      ctx.textBaseline = 'middle'
      ctx.globalAlpha = cell.inCurrentMonth ? 1 : 0.4
      ctx.fillText(String(cell.dayNumber), x + colWidth / 2, y + rowH / 2)
      ctx.globalAlpha = 1
    })

    const summaryX = tableLeft + 8 * colWidth
    ctx.fillStyle = weekMeta.solidBackground
    ctx.fillRect(summaryX, y, colWidth, rowH)
    ctx.strokeStyle = '#e5e7eb'
    ctx.strokeRect(summaryX, y, colWidth, rowH)
    ctx.fillStyle = weekMeta.solidColor
    ctx.font = 'bold 18px "Microsoft YaHei", "PingFang SC", sans-serif'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(weekMeta.restLabel, summaryX + colWidth / 2, y + rowH / 2)
  })

  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error('生成 PNG 失败'))
        return
      }
      resolve(blob)
    }, 'image/png')
  })
}

export function buildUpcomingSaturdayNotice(
  today: Dayjs,
  saturday: Dayjs,
  isWork: boolean,
  weekLabel: string,
): string[] {
  const dayDiff = saturday.startOf('day').diff(today.startOf('day'), 'day')
  let relativeLabel = '本周六'
  if (dayDiff === 0) {
    relativeLabel = '今天'
  } else if (dayDiff === 1) {
    relativeLabel = '明天'
  } else if (today.day() === 0) {
    relativeLabel = '下周六'
  }

  const statusLabel = isWork ? '需上班' : '休息'
  const dateLabel = saturday.format('YYYY年M月D日')
  const dateContext = relativeLabel.endsWith('周六') ? dateLabel : `${dateLabel}，周六`
  const lines = [
    `【${relativeLabel}${statusLabel}】`,
    `${relativeLabel}（${dateContext}）${isWork ? '需上班，请提前安排。' : '休息，无需上班。'}`,
  ]
  if (weekLabel) {
    lines.push(`${relativeLabel === '下周六' ? '下周' : '本周'}为${weekLabel}。`)
  }
  return lines
}

function buildSchedulePushContent(
  month: Dayjs,
  section: MonthCalendarSection | null,
): { title: string; content: string } {
  const title = `${month.format('YYYY年M月')}作息时间表`
  const lines: string[] = []

  const today = dayjs().startOf('day')
  if (section && today.isSame(month, 'month')) {
    const saturday = today.add((6 - today.day() + 7) % 7, 'day')
    const saturdayRow = section.rows.find((row) =>
      !saturday.isBefore(dayjs(row.week.week_start), 'day') &&
      !saturday.isAfter(dayjs(row.week.week_end), 'day'),
    )
    if (saturdayRow) {
      const saturdayState = getDayState(saturdayRow.week, saturday).state
      const isWork =
        saturdayState === 'work' || saturdayState === 'workday' || saturdayState === 'saturday-work'
      const weekLabel = saturdayRow.week.week_type === 'small' ? '小周' : '大周'
      lines.push(...buildUpcomingSaturdayNotice(today, saturday, isWork, weekLabel))
    }
  }
  lines.push(`${title}，请查收。`)

  return { title, content: lines.join('\n') }
}

export default function WeekSchedule() {
  const queryClient = useQueryClient()
  const permissions = useAuthStore((state) => state.permissions)
  const canManageAttendance = permissions.includes('attendance_manage')
  const canPushWeekScheduleGroup = canManageAttendance || permissions.includes('week_schedule_group_push')
  const canPushWeekSchedule = canManageAttendance || canPushWeekScheduleGroup
  const [calendarScopeType, setCalendarScopeType] = useState<ScopeType>(canManageAttendance ? 'company' : 'user')
  const [selectedDepartmentId, setSelectedDepartmentId] = useState('')
  const [selectedUserId, setSelectedUserId] = useState('')
  const [holidayYear, setHolidayYear] = useState(dayjs().year())
  const [selectedMonth, setSelectedMonth] = useState<Dayjs | null>(dayjs())

  const [ruleModalOpen, setRuleModalOpen] = useState(false)
  const [overrideModalOpen, setOverrideModalOpen] = useState(false)
  const [holidayModalOpen, setHolidayModalOpen] = useState(false)
  const [holidayImportModalOpen, setHolidayImportModalOpen] = useState(false)
  const [shiftModalOpen, setShiftModalOpen] = useState(false)
  const [pushModalOpen, setPushModalOpen] = useState(false)
  const [pushTargetType, setPushTargetType] = useState<PushTargetType>('personal')
  const [pushRecipientIds, setPushRecipientIds] = useState<string[]>([])
  const [pushGroupTargetId, setPushGroupTargetId] = useState<number | undefined>()
  const [pushUserSearch, setPushUserSearch] = useState('')
  const [pushUserOptions, setPushUserOptions] = useState<UserOption[]>([])
  const [pushUserSearching, setPushUserSearching] = useState(false)

  const [editingRule, setEditingRule] = useState<WeekScheduleRule | null>(null)
  const [selectedWeek, setSelectedWeek] = useState<WeekCalendarItem | null>(null)
  const [holidayImportText, setHolidayImportText] = useState('')

  const [ruleForm] = Form.useForm<RuleFormValues>()
  const [overrideForm] = Form.useForm<OverrideFormValues>()
  const [holidayForm] = Form.useForm<HolidayFormValues>()
  const [shiftForm] = Form.useForm<ShiftFormValues>()

  const usersQuery = useQuery({
    queryKey: ['week-schedule', 'users', 'all-active'],
    queryFn: fetchAllUsersForPicker,
    retry: false,
    staleTime: 5 * 60 * 1000,
  })

  const departmentsQuery = useQuery({
    queryKey: ['week-schedule', 'departments'],
    queryFn: () => departmentAPI.getDepartments(),
    retry: false,
  })

  const shiftsQuery = useQuery({
    queryKey: ['week-schedule', 'shifts'],
    queryFn: () => weekScheduleAPI.getShifts(),
    enabled: canManageAttendance,
    retry: false,
  })

  const rulesQuery = useQuery({
    queryKey: ['week-schedule', 'rules'],
    queryFn: () => weekScheduleAPI.getRules(),
    enabled: canManageAttendance,
    retry: false,
  })

  const logsQuery = useQuery({
    queryKey: ['week-schedule', 'logs'],
    queryFn: () => weekScheduleAPI.getSyncLogs({ page: 1, page_size: 20 }),
    enabled: canManageAttendance,
    retry: false,
  })

  const groupTargetsQuery = useQuery({
    queryKey: ['week-schedule', 'group-targets'],
    queryFn: () => weekScheduleAPI.getGroupTargets(),
    enabled: pushModalOpen && canPushWeekScheduleGroup,
    retry: false,
  })

  const users = usersQuery.data ?? []
  const departments = unwrapData<{ departments: DepartmentOption[] }>(departmentsQuery.data, { departments: [] }).departments ?? []
  const shifts = getItems<ShiftOption>(shiftsQuery.data)
  const rules = getItems<WeekScheduleRule>(rulesQuery.data)
  const syncLogs = getItems<SyncLogRecord>(logsQuery.data)
  const groupTargets = getItems<WeekScheduleGroupTarget>(groupTargetsQuery.data)

  const pushSelectOptions = useMemo(() => {
    const byID = new Map<string, UserOption>()
    users.forEach((u) => byID.set(u.user_id, u))
    pushUserOptions.forEach((u) => byID.set(u.user_id, u))
    // 已选但还不在列表里的，保留占位，避免 Select 只显示 id
    pushRecipientIds.forEach((id) => {
      if (!byID.has(id)) {
        byID.set(id, { id: 0, user_id: id, name: id, department_id: '' })
      }
    })
    return Array.from(byID.values())
      .sort((a, b) => (a.name || '').localeCompare(b.name || '', 'zh-CN'))
      .map((item) => ({ label: item.name || item.user_id, value: item.user_id }))
  }, [users, pushUserOptions, pushRecipientIds])

  useEffect(() => {
    if (!pushModalOpen) {
      setPushUserSearch('')
      setPushUserOptions([])
      return
    }
    const keyword = pushUserSearch.trim()
    if (!keyword) {
      setPushUserOptions([])
      return
    }
    // 本地全量列表已覆盖大多数场景；远端搜索补漏（如未在 active 首批、或后续新增）
    let cancelled = false
    const timer = window.setTimeout(async () => {
      setPushUserSearching(true)
      try {
        const res = await userAPI.getUsers({ page: 1, page_size: 100, search: keyword, status: 'active' })
        if (!cancelled) {
          setPushUserOptions(getItems<UserOption>(res))
        }
      } catch {
        if (!cancelled) {
          setPushUserOptions([])
        }
      } finally {
        if (!cancelled) {
          setPushUserSearching(false)
        }
      }
    }, 300)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [pushModalOpen, pushUserSearch])

  useEffect(() => {
    if (!canManageAttendance && calendarScopeType === 'company') {
      setCalendarScopeType('user')
    }
  }, [canManageAttendance, calendarScopeType])

  useEffect(() => {
    if (calendarScopeType !== 'user' || selectedUserId || users.length !== 1) {
      return
    }
    const [user] = users
    setSelectedUserId(user.user_id)
    if (!selectedDepartmentId && user.department_id) {
      setSelectedDepartmentId(user.department_id)
    }
  }, [calendarScopeType, selectedDepartmentId, selectedUserId, users])

  const selectedDepartment = departments.find((item) => item.department_id === selectedDepartmentId) ?? null
  const selectedUser = users.find((item) => item.user_id === selectedUserId) ?? null

  const calendarParams = useMemo(() => {
    const today = dayjs()
    const target = selectedMonth || today
    const monthStart = target.startOf('month')
    const startDate = monthStart.format('YYYY-MM-DD')
    const monthDiff = Math.max(0, target.diff(today, 'month'))
    const weeksNeeded = Math.max(8, monthDiff * 5 + 8)
    const params: Record<string, any> = { weeks: weeksNeeded, start_date: startDate }
    if (calendarScopeType === 'department') {
      params.department_id = selectedDepartmentId
    } else if (calendarScopeType === 'user') {
      params.user_id = selectedUserId
      params.department_id = selectedUser?.department_id || ''
    }
    return params
  }, [calendarScopeType, selectedDepartmentId, selectedUserId, selectedUser?.department_id, selectedMonth])

  const canQueryCalendar =
    (calendarScopeType === 'company' && canManageAttendance) ||
    (calendarScopeType === 'department' && Boolean(selectedDepartmentId)) ||
    (calendarScopeType === 'user' && Boolean(selectedUserId))

  const calendarQuery = useQuery({
    queryKey: ['week-schedule', 'calendar', calendarScopeType, selectedDepartmentId, selectedUserId, calendarParams.start_date, calendarParams.weeks],
    queryFn: () => weekScheduleAPI.getCalendar(calendarParams),
    enabled: canQueryCalendar,
    retry: false,
  })

  const holidaysQuery = useQuery({
    queryKey: ['week-schedule', 'holidays', holidayYear],
    queryFn: () => weekScheduleAPI.getHolidays({ year: holidayYear }),
    enabled: canManageAttendance,
    retry: false,
  })

  const calendarItems = getItems<WeekCalendarItem>(calendarQuery.data)
  const holidayRecords = getItems<HolidayRecord>(holidaysQuery.data)

  // 按名称分组合并连续日期
  const groupedHolidays = useMemo(() => {
    if (!holidayRecords.length) return []
    const sorted = [...holidayRecords].sort((a, b) => a.date.localeCompare(b.date))
    const groups: { name: string; startDate: string; endDate: string; type: HolidayType; ids: number[] }[] = []
    for (const r of sorted) {
      const last = groups[groups.length - 1]
      if (last && last.name === r.name && last.type === r.type) {
        last.endDate = r.date
        last.ids.push(r.id)
      } else {
        groups.push({ name: r.name, startDate: r.date, endDate: r.date, type: r.type, ids: [r.id] })
      }
    }
    return groups.map((g, i) => ({
      id: i,
      date: g.startDate === g.endDate ? g.startDate : `${g.startDate} ~ ${g.endDate}`,
      name: g.name,
      type: g.type,
      ids: g.ids,
    }))
  }, [holidayRecords])
  const monthCalendarSections = useMemo(() => buildMonthCalendarSections(calendarItems), [calendarItems])

  const filteredMonthSections = useMemo(() => {
    if (!selectedMonth) return monthCalendarSections
    const monthKey = selectedMonth.format('YYYY-MM')
    return monthCalendarSections.filter((section) => section.month === monthKey)
  }, [monthCalendarSections, selectedMonth])

  const shiftConfigQuery = useQuery({
    queryKey: ['week-schedule', 'user-shift-config', selectedUserId],
    queryFn: () => shiftConfigAPI.list(),
    enabled: calendarScopeType === 'user' && Boolean(selectedUserId),
    retry: false,
  })
  const shiftConfigItems = getItems<{ user_id: string; end_time: string }>(shiftConfigQuery.data)
  const selectedUserEndTime =
    calendarScopeType === 'user' && selectedUserId
      ? shiftConfigItems.find((item) => item.user_id === selectedUserId)?.end_time ?? null
      : null

  const currentScopeId =
    calendarScopeType === 'company'
      ? ''
      : calendarScopeType === 'department'
        ? selectedDepartmentId
        : selectedUserId

  const currentScopeName =
    calendarScopeType === 'company'
      ? '全公司'
      : calendarScopeType === 'department'
        ? selectedDepartment?.name || ''
        : selectedUser?.name || ''

  const calendarScopeOptions = useMemo(
    () => [
      ...(canManageAttendance ? [{ label: '全公司', value: 'company' as ScopeType }] : []),
      { label: '部门', value: 'department' as ScopeType },
      { label: '个人', value: 'user' as ScopeType },
    ],
    [canManageAttendance],
  )

  const invalidateAll = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['week-schedule', 'rules'] }),
      queryClient.invalidateQueries({ queryKey: ['week-schedule', 'calendar'] }),
      queryClient.invalidateQueries({ queryKey: ['week-schedule', 'holidays'] }),
      queryClient.invalidateQueries({ queryKey: ['week-schedule', 'logs'] }),
      queryClient.invalidateQueries({ queryKey: ['week-schedule', 'shifts'] }),
    ])
  }

  const createRuleMutation = useMutation({
    mutationFn: (payload: Record<string, unknown>) => weekScheduleAPI.createRule(payload),
    onSuccess: async () => {
      message.success('规则已创建')
      setRuleModalOpen(false)
      setEditingRule(null)
      ruleForm.resetFields()
      await invalidateAll()
    },
    onError: (error) => message.error(getErrorMessage(error, '创建规则失败')),
  })

  const updateRuleMutation = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: Record<string, unknown> }) => weekScheduleAPI.updateRule(id, payload),
    onSuccess: async () => {
      message.success('规则已更新')
      setRuleModalOpen(false)
      setEditingRule(null)
      ruleForm.resetFields()
      await invalidateAll()
    },
    onError: (error) => message.error(getErrorMessage(error, '更新规则失败')),
  })

  const deleteRuleMutation = useMutation({
    mutationFn: (id: number) => weekScheduleAPI.deleteRule(id),
    onSuccess: async () => {
      message.success('规则已删除')
      await invalidateAll()
    },
    onError: (error) => message.error(getErrorMessage(error, '删除规则失败')),
  })

  const overrideMutation = useMutation({
    mutationFn: (payload: Record<string, unknown>) => weekScheduleAPI.setOverride(payload),
    onSuccess: async () => {
      message.success('本周已手动覆盖')
      setOverrideModalOpen(false)
      overrideForm.resetFields()
      await invalidateAll()
    },
    onError: (error) => message.error(getErrorMessage(error, '设置覆盖失败')),
  })

  const pushPersonalMutation = useMutation({
    mutationFn: (formData: FormData) => weekScheduleAPI.pushPersonalSchedule(formData),
    onSuccess: async (response) => {
      const result = unwrapData<{
        status?: string
        message?: string
        success_count?: number
        failed_count?: number
        skipped_count?: number
        recipients?: Array<{ name?: string; user_id?: string; status?: string; message?: string }>
      }>(response, {})
      const status = result.status || 'success'
      const summary =
        result.message ||
        `作息表推送完成：成功 ${result.success_count ?? 0}，失败 ${result.failed_count ?? 0}，跳过 ${result.skipped_count ?? 0}`
      if (status === 'success') {
        message.success(summary)
      } else if (status === 'partial') {
        message.warning(summary)
      } else {
        message.error(summary)
      }
      if (Array.isArray(result.recipients) && result.recipients.length > 0) {
        const detail = result.recipients
          .map((item) => `${item.name || item.user_id || '-'}: ${item.status || '-'} ${item.message || ''}`.trim())
          .join('；')
        message.info(detail, 8)
      }
      setPushModalOpen(false)
    },
    onError: (error) => message.error(getErrorMessage(error, '作息表推送失败')),
  })

  const pushGroupMutation = useMutation({
    mutationFn: (formData: FormData) => weekScheduleAPI.pushGroupSchedule(formData),
    onSuccess: (response) => {
      const result = unwrapData<{ status?: string; message?: string }>(response, {})
      if (result.status === 'submitted') {
        message.success('已提交')
        setPushModalOpen(false)
        return
      }
      message.warning(result.message || '钉钉受理状态待确认')
    },
    onError: async (error) => {
      message.error(getErrorMessage(error, '群聊推送提交失败'))
      await queryClient.invalidateQueries({ queryKey: ['week-schedule', 'group-targets'] })
    },
  })

  const unbindGroupMutation = useMutation({
    mutationFn: (targetID: number) => weekScheduleAPI.unbindGroupTarget(targetID),
    onSuccess: async (_response, targetID) => {
      if (pushGroupTargetId === targetID) {
        setPushGroupTargetId(undefined)
      }
      message.success('群聊已解绑')
      await queryClient.invalidateQueries({ queryKey: ['week-schedule', 'group-targets'] })
    },
    onError: (error) => message.error(getErrorMessage(error, '解绑群聊失败')),
  })

  const syncFromMutation = useMutation({
    mutationFn: () => weekScheduleAPI.syncFromDingtalk(),
    onSuccess: async (response) => {
      const result = unwrapData<{ message?: string }>(response, {})
      message.success(result.message || '已从钉钉拉取')
      await invalidateAll()
    },
    onError: (error) => message.error(getErrorMessage(error, '从钉钉拉取失败')),
  })

  const syncHolidayMutation = useMutation({
    mutationFn: () => weekScheduleAPI.syncHolidaysFromJuhe(),
    onSuccess: async () => {
      message.success('节假日已同步')
      await invalidateAll()
    },
    onError: (error) => message.error(getErrorMessage(error, '同步节假日失败')),
  })

  const createHolidayMutation = useMutation({
    mutationFn: (payload: Record<string, unknown>) => weekScheduleAPI.createHoliday(payload),
    onSuccess: async () => {
      message.success('节假日已添加')
      setHolidayModalOpen(false)
      holidayForm.resetFields()
      await invalidateAll()
    },
    onError: (error) => message.error(getErrorMessage(error, '新增节假日失败')),
  })

  const batchHolidayMutation = useMutation({
    mutationFn: (payload: { holidays: Array<{ date: string; name: string; type: string }> }) => weekScheduleAPI.batchCreateHolidays(payload),
    onSuccess: async () => {
      message.success('节假日已批量导入')
      setHolidayImportModalOpen(false)
      setHolidayImportText('')
      await invalidateAll()
    },
    onError: (error) => message.error(getErrorMessage(error, '批量导入失败')),
  })

  const deleteHolidayMutation = useMutation({
    mutationFn: (id: number) => weekScheduleAPI.deleteHoliday(id),
    onSuccess: async () => {
      message.success('节假日已删除')
      await invalidateAll()
    },
    onError: (error) => message.error(getErrorMessage(error, '删除节假日失败')),
  })

  const createShiftMutation = useMutation({
    mutationFn: (payload: ShiftFormValues) => weekScheduleAPI.createShift(payload),
    onSuccess: async () => {
      message.success('班次已创建')
      setShiftModalOpen(false)
      shiftForm.resetFields()
      await invalidateAll()
    },
    onError: (error) => message.error(getErrorMessage(error, '创建班次失败')),
  })

  const openCreateRuleModal = () => {
    if (!canManageAttendance) return
    setEditingRule(null)
    ruleForm.setFieldsValue({
      scope_type: 'company',
      pattern: 'big_first',
      status: 'active',
      shift_id: 0,
      base_date: dayjs(),
    })
    setRuleModalOpen(true)
  }

  const openEditRuleModal = (rule: WeekScheduleRule) => {
    if (!canManageAttendance) return
    setEditingRule(rule)
    ruleForm.setFieldsValue({
      scope_type: rule.scope_type,
      scope_target_id: rule.scope_type === 'company' ? undefined : rule.scope_id,
      base_date: dayjs(rule.base_date),
      pattern: rule.pattern,
      shift_id: rule.shift_id,
      status: rule.status,
    })
    setRuleModalOpen(true)
  }

  const openOverrideModal = (week: WeekCalendarItem) => {
    if (!canManageAttendance) return
    setSelectedWeek(week)
    overrideForm.setFieldsValue({
      week_type: week.week_type,
      reason: '',
    })
    setOverrideModalOpen(true)
  }

  const openPushModal = () => {
    if (!canPushWeekSchedule) return
    if (!selectedMonth) {
      message.warning('请先选择月份')
      return
    }
    if (filteredMonthSections.length === 0) {
      message.warning('当前月份暂无作息表数据，请先加载日历')
      return
    }
    // 默认不选择任何员工或群聊，避免误推钉钉消息。
    setPushTargetType(canManageAttendance ? 'personal' : 'group')
    setPushRecipientIds([])
    setPushGroupTargetId(undefined)
    setPushModalOpen(true)
  }

  const handleConfirmPush = async () => {
    if (!selectedMonth) {
      message.warning('请先选择月份')
      return
    }
    if (pushTargetType === 'personal' && pushRecipientIds.length === 0) {
      message.warning('请至少选择一位收件人')
      return
    }
    if (pushTargetType === 'group' && !pushGroupTargetId) {
      message.warning('请选择已绑定群聊')
      return
    }
    const section = filteredMonthSections[0]
    if (!section) {
      message.warning('当前月份暂无作息表数据')
      return
    }

    try {
      const { title, content } = buildSchedulePushContent(selectedMonth, section)
      if (pushTargetType === 'group') {
        const target = groupTargets.find((item) => item.id === pushGroupTargetId)
        if (!target || target.status !== 'active') {
          message.warning('所选群聊已失效，请重新选择')
          return
        }
        Modal.confirm({
          title: '确认推送到群聊？',
          content: `将 ${selectedMonth.format('YYYY年M月')} 作息表推送到“${target.group_name}”。钉钉受理后页面仅显示“已提交”。`,
          okText: '确认推送',
          cancelText: '返回检查',
          async onOk() {
            const blob = await renderMonthSchedulePng(section, title)
            const formData = new FormData()
            formData.append('image', blob, `${selectedMonth.format('YYYY-MM')}-schedule.png`)
            formData.append('group_target_id', String(target.id))
            formData.append('title', title)
            formData.append('content', content)
            formData.append('month', selectedMonth.format('YYYY-MM'))
            await pushGroupMutation.mutateAsync(formData)
          },
        })
        return
      }

      const blob = await renderMonthSchedulePng(section, title)
      const formData = new FormData()
      formData.append('image', blob, `${selectedMonth.format('YYYY-MM')}-schedule.png`)
      formData.append('user_ids', JSON.stringify(pushRecipientIds))
      formData.append('title', title)
      formData.append('content', content)
      pushPersonalMutation.mutate(formData)
    } catch (error) {
      message.error(getErrorMessage(error, '生成作息表图片失败'))
    }
  }

  const confirmGroupUnbind = (target: WeekScheduleGroupTarget) => {
    Modal.confirm({
      title: '确认解绑群聊？',
      content: `解绑“${target.group_name}”后将无法选择该群推送作息表。重新在群内 @人事系统机器人发送任意内容可恢复绑定。`,
      okText: '确认解绑',
      cancelText: '取消',
      okType: 'danger',
      onOk: () => unbindGroupMutation.mutateAsync(target.id),
    })
  }

  const handleSubmitRule = async () => {
    const values = await ruleForm.validateFields()
    const scopeName =
      values.scope_type === 'company'
        ? '全公司'
        : values.scope_type === 'department'
          ? departments.find((item) => item.department_id === values.scope_target_id)?.name || ''
          : users.find((item) => item.user_id === values.scope_target_id)?.name || ''

    const payload = {
      scope_type: values.scope_type,
      scope_id: values.scope_type === 'company' ? '' : values.scope_target_id || '',
      scope_name: scopeName,
      base_date: values.base_date.format('YYYY-MM-DD'),
      pattern: values.pattern,
      shift_id: values.shift_id || 0,
      status: values.status,
    }

    if (editingRule) {
      updateRuleMutation.mutate({ id: editingRule.id, payload })
      return
    }

    createRuleMutation.mutate(payload)
  }

  const handleSubmitOverride = async () => {
    if (!selectedWeek) return
    const values = await overrideForm.validateFields()
    overrideMutation.mutate({
      scope_type: calendarScopeType,
      scope_id: currentScopeId,
      week_start_date: selectedWeek.week_start,
      week_type: values.week_type,
      reason: values.reason || '',
    })
  }

  const handleSubmitHoliday = async () => {
    const values = await holidayForm.validateFields()
    createHolidayMutation.mutate({
      date: values.date.format('YYYY-MM-DD'),
      name: values.name,
      type: values.type,
      year: values.date.year(),
    })
  }

  const handleImportHolidays = () => {
    try {
      const parsed = JSON.parse(holidayImportText)
      if (!Array.isArray(parsed)) {
        throw new Error('JSON 内容必须是数组')
      }
      batchHolidayMutation.mutate({
        holidays: parsed.map((item) => ({
          date: String(item.date),
          name: String(item.name),
          type: String(item.type),
        })),
      })
    } catch (error) {
      message.error(getErrorMessage(error, 'JSON 解析失败'))
    }
  }

  const handleSubmitShift = async () => {
    const values = await shiftForm.validateFields()
    createShiftMutation.mutate(values)
  }

  const ruleColumns: TableColumnsType<WeekScheduleRule> = [
    {
      title: '适用范围',
      key: 'scope',
      render: (_, record) => (
        <Space direction="vertical" size={2}>
          <Space>
            <Tag color="blue" style={{ borderRadius: 6, fontWeight: 600, margin: 0 }}>{getScopeLabel(record.scope_type)}</Tag>
            <Text strong>{record.scope_name || '未命名范围'}</Text>
          </Space>
          <Text type="secondary">{record.scope_id || '全公司'}</Text>
        </Space>
      ),
    },
    {
      title: '基准日期',
      dataIndex: 'base_date',
      width: 120,
    },
    {
      title: '轮换模式',
      dataIndex: 'pattern',
      width: 140,
      render: (value) => getPatternLabel(value),
    },
    {
      title: '工作班次',
      dataIndex: 'shift_id',
      width: 180,
      render: (shiftId) => shifts.find((item) => item.id === shiftId)?.name || '默认班次',
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      render: (value) => getStatusTag(value),
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      width: 180,
      render: formatDateTime,
    },
    ...(canManageAttendance ? [{
      title: '操作',
      key: 'actions',
      width: 140,
      render: (_, record) => (
        <Space>
          <Button size="small" icon={<EditOutlined />} onClick={() => openEditRuleModal(record)}>
            编辑
          </Button>
          <Popconfirm title="确定删除这条规则？" onConfirm={() => deleteRuleMutation.mutate(record.id)}>
            <Button size="small" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    }] : []),
  ]

  const holidayColumns = [
    {
      title: '日期',
      dataIndex: 'date',
      width: 120,
    },
    {
      title: '名称',
      dataIndex: 'name',
    },
    {
      title: '类型',
      dataIndex: 'type',
      width: 140,
      render: (value: HolidayType) => <Tag color={value === 'holiday' ? 'red' : 'gold'} style={{ borderRadius: 6, fontWeight: 600, margin: 0 }}>{value === 'holiday' ? '放假' : '调休上班'}</Tag>,
    },
    ...(canManageAttendance ? [{
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_, record) => (
        <Popconfirm title="确定删除这条节假日记录？" onConfirm={() => {
          const ids = (record as any).ids || [record.id]
          ids.forEach((id: number) => deleteHolidayMutation.mutate(id))
        }}>
          <Button size="small" danger icon={<DeleteOutlined />}>
            删除
          </Button>
        </Popconfirm>
      ),
    }] : []),
  ]

  const logColumns: TableColumnsType<SyncLogRecord> = [
    {
      title: '时间',
      dataIndex: 'created_at',
      width: 110,
      render: formatDateTime,
    },
    {
      title: '同步方向',
      dataIndex: 'sync_type',
      width: 120,
      render: (value) => getSyncTypeLabel(value),
    },
    {
      title: '人数',
      dataIndex: 'user_count',
      width: 50,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      render: (value) => getStatusTag(value),
    },
    {
      title: '说明',
      dataIndex: 'message',
      ellipsis: true,
    },
  ]

  const ruleScope = Form.useWatch('scope_type', ruleForm) ?? 'company'

  return (
    <PageContainer
      className="week-schedule-page"
      title="大小周与节假日管理"
      subtitle="按月查看大小周作息；黄/白/红分别表示工作、休息与法定节假日。支持手动覆盖周类型，并一键推送到个人钉钉。"
      icon={<CalendarOutlined />}
    >
      <Card
        className="ws-panel-card"
        title="查询范围"
      >
        <Row gutter={[16, 16]}>
          <Col xs={24} lg={8}>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              <Text strong>查看范围</Text>
              <Segmented
                block
                value={calendarScopeType}
                onChange={(value) => setCalendarScopeType(value as ScopeType)}
                options={calendarScopeOptions}
              />
            </Space>
          </Col>

          <Col xs={24} lg={8}>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              <Text strong>部门</Text>
              <Select
                allowClear
                placeholder="选择部门"
                disabled={calendarScopeType === 'company'}
                value={selectedDepartmentId || undefined}
                onChange={(value) => setSelectedDepartmentId(value || '')}
                options={departments.map((item) => ({ label: item.name, value: item.department_id }))}
              />
            </Space>
          </Col>

          <Col xs={24} lg={8}>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              <Text strong>员工</Text>
              <Select
                allowClear
                showSearch
                placeholder="选择员工"
                disabled={calendarScopeType !== 'user'}
                value={selectedUserId || undefined}
                onChange={(value) => setSelectedUserId(value || '')}
                options={users
                  .filter((item) => !selectedDepartmentId || item.department_id === selectedDepartmentId)
                  .map((item) => ({ label: item.name, value: item.user_id }))}
              />
            </Space>
          </Col>
        </Row>
      </Card>

      <Card
        className="ws-panel-card ws-calendar-card"
        title="大小周日历"
        extra={
          <Space wrap className="ws-calendar-toolbar">
            <Button icon={<ReloadOutlined />} onClick={() => invalidateAll()}>
              刷新
            </Button>
            {canManageAttendance && (
              <>
                <Button loading={syncHolidayMutation.isPending} onClick={() => syncHolidayMutation.mutate()}>
                  同步节假日
                </Button>
                <Button loading={syncFromMutation.isPending} onClick={() => syncFromMutation.mutate()}>
                  从钉钉拉取
                </Button>
              </>
            )}
            <Tooltip title={canPushWeekSchedule ? undefined : '你缺少作息表推送权限，需要联系管理员添加'}>
              <span>
                <Button
                  type="primary"
                  icon={<SyncOutlined />}
                  loading={pushPersonalMutation.isPending || pushGroupMutation.isPending}
                  onClick={openPushModal}
                  disabled={!canPushWeekSchedule}
                >
                  作息表推送
                </Button>
              </span>
            </Tooltip>
            <Text type="secondary" className="ws-scope-hint">当前：{currentScopeName || '未选择'}</Text>
            <DatePicker
              picker="month"
              value={selectedMonth}
              onChange={(date) => setSelectedMonth(date || dayjs())}
              allowClear={false}
              className="ws-month-picker"
            />
          </Space>
        }
      >
        {!canQueryCalendar ? (
          <Alert type="info" showIcon message={calendarScopeType === 'department' ? '请选择部门后再查看日历' : '请选择员工后再查看日历'} />
        ) : calendarQuery.isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '48px 0' }}>
            <Spin size="large" />
          </div>
        ) : calendarQuery.isError ? (
          <Alert type="error" showIcon message="日历加载失败" description={getErrorMessage(calendarQuery.error, '请稍后重试')} />
        ) : monthCalendarSections.length === 0 ? (
          <Empty description="暂无可展示的周次安排" imageStyle={{ height: 80 }} />
        ) : filteredMonthSections.length === 0 ? (
          <Empty description={`${selectedMonth?.format('YYYY年M月') || '该月份'}暂无周次安排`} imageStyle={{ height: 80 }} />
        ) : (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <div className="ws-legend" aria-label="颜色说明">
              <span className="ws-legend-item"><i className="ws-swatch ws-swatch-work" />工作日</span>
              <span className="ws-legend-item"><i className="ws-swatch ws-swatch-rest" />休息日</span>
              <span className="ws-legend-item"><i className="ws-swatch ws-swatch-holiday" />法定节假日</span>
              <span className="ws-legend-item"><i className="ws-swatch ws-swatch-outside" />非本月</span>
              <span className="ws-legend-note">点击周行可手动覆盖大小周</span>
            </div>

            {filteredMonthSections.map((section) => (
              <Card
                className="week-calendar-month-card"
                key={section.month}
                title={`${dayjs(`${section.month}-01`).format('YYYY年M月')}作息时间表`}
                styles={{ body: { padding: 0, overflowX: 'auto' } }}
              >
                <div className="week-calendar-mobile-panel">
                  <div className="week-calendar-mobile-month-title">
                    {dayjs(`${section.month}-01`).format('YYYY年M月')}
                  </div>
                  <div className="week-calendar-mobile-weekdays" aria-hidden="true">
                    {['一', '二', '三', '四', '五', '六', '日'].map((label) => (
                      <span key={label}>{label}</span>
                    ))}
                  </div>
                  <div className="week-calendar-mobile-grid">
                    {section.rows.flatMap((row) =>
                      row.cells.map((cell) => {
                        const isToday = dayjs(cell.date).isSame(dayjs(), 'day')
                        const note = getMobileCalendarNote(cell, selectedUserEndTime)
                        return (
                          <button
                            key={`${row.week.week_start}-${cell.date}`}
                            type="button"
                            className="week-calendar-mobile-day"
                            data-state={cell.state}
                            data-outside={!cell.inCurrentMonth ? 'true' : undefined}
                            data-today={isToday ? 'true' : undefined}
                            disabled={!canManageAttendance || !cell.inCurrentMonth}
                            onClick={() => openOverrideModal(row.week)}
                            aria-label={`${cell.date} ${note}`}
                          >
                            <span className="week-calendar-mobile-day-number">{cell.dayNumber}</span>
                            <span className="week-calendar-mobile-day-note">{note}</span>
                          </button>
                        )
                      }),
                    )}
                  </div>
                </div>
                <table className="week-calendar-table">
                  <thead>
                    <tr>
                      {['周数', '周一', '周二', '周三', '周四', '周五', '周六', '周日', '大小周'].map((label) => (
                        <th key={label}>{label}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {section.rows.map((row, index) => {
                      const weekMeta = getWeekTypeMeta(row.week.week_type)
                      const holidaySummary =
                        row.week.holidays && row.week.holidays.length > 0
                          ? row.week.holidays.map((holiday) => `${dayjs(holiday.date).format('M/D')} ${holiday.name}`).join('、')
                          : '无特殊日期'

                      const isTodayRow = row.cells.some((c) => dayjs(c.date).isSame(dayjs(), 'day') && c.inCurrentMonth)
                      return (
                        <tr
                          key={row.week.week_start}
                          className={`ws-week-row${isTodayRow ? ' is-current-week' : ''}`}
                          onClick={() => openOverrideModal(row.week)}
                          style={{ cursor: canManageAttendance ? 'pointer' : 'default' }}
                          title={canManageAttendance ? `点击覆盖本周（当前${weekMeta.label}）` : undefined}
                        >
                          <td className="ws-week-index">
                            <div className="ws-week-index-title">第{index + 1}周</div>
                            <div className="ws-week-index-range">
                              {dayjs(row.week.week_start).format('MM/DD')}-{dayjs(row.week.week_end).format('MM/DD')}
                            </div>
                          </td>

                          {row.cells.map((cell) => {
                            const isToday = dayjs(cell.date).isSame(dayjs(), 'day')
                            const showRestLabel = cell.inCurrentMonth && cell.state === 'rest' && !cell.holidayLabel
                            const showWorkExtra =
                              cell.inCurrentMonth &&
                              (cell.state === 'saturday-work' ||
                                cell.state === 'workday' ||
                                (cell.state === 'work' && selectedUserEndTime))
                            return (
                              <td
                                key={cell.date}
                                className={`ws-day-cell ws-day-${cell.state}${!cell.inCurrentMonth ? ' is-outside' : ''}${isToday ? ' is-today' : ''}`}
                                style={getCellStyle(cell.state)}
                              >
                                <div className="ws-day-number">{cell.dayNumber}</div>
                                {(cell.holidayLabel || showRestLabel || showWorkExtra) && (
                                  <div className="ws-day-note">
                                    {cell.holidayLabel ? (
                                      cell.holidayLabel
                                    ) : cell.state === 'rest' ? (
                                      '休息'
                                    ) : cell.state === 'saturday-work' ? (
                                      <>
                                        <div>周六上班</div>
                                        {selectedUserEndTime && <div className="ws-day-shift">下班 {selectedUserEndTime}</div>}
                                      </>
                                    ) : cell.state === 'workday' ? (
                                      '调休上班'
                                    ) : cell.state === 'work' && selectedUserEndTime ? (
                                      <div className="ws-day-shift">下班 {selectedUserEndTime}</div>
                                    ) : null}
                                  </div>
                                )}
                              </td>
                            )
                          })}

                          <td className="ws-size-cell" style={{ background: weekMeta.background }}>
                            <div className="ws-size-label" style={{ color: weekMeta.color }}>{weekMeta.restLabel}</div>
                            <div className="ws-size-meta">
                              {weekMeta.label}
                              {row.week.is_override ? ' · 已覆盖' : ''}
                              <br />
                              {row.week.saturday_work ? '周六上班' : '周六休息'}
                            </div>
                            {holidaySummary !== '无特殊日期' && (
                              <div className="ws-size-holidays">{holidaySummary}</div>
                            )}
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </Card>
            ))}
          </Space>
        )}
      </Card>

      {canManageAttendance && (
        <>
          <Row gutter={[24, 24]}>
            <Col xs={24} xxl={12}>
              <Card
                title="规则管理"
                className="ws-panel-card"
                extra={
                  <Space>
                    <Button onClick={() => shiftsQuery.refetch()}>刷新班次</Button>
                    <Button onClick={() => setShiftModalOpen(true)}>新增班次</Button>
                    <Button type="primary" icon={<PlusOutlined />} onClick={openCreateRuleModal}>
                      新增规则
                    </Button>
                  </Space>
                }
              >
                <Table
                  rowKey="id"
                  loading={rulesQuery.isLoading}
                  columns={ruleColumns}
                  dataSource={rules}
                  pagination={{ pageSize: 8, hideOnSinglePage: true }}
                />
              </Card>
            </Col>

            <Col xs={24} xxl={12}>
              <Card
                title="钉钉同步"
                className="ws-panel-card"
                extra={
                  <Space>
                    <Button icon={<ReloadOutlined />} onClick={() => logsQuery.refetch()}>
                      刷新
                    </Button>
                  </Space>
                }
              >
                <Table
                  rowKey="id"
                  size="small"
                  loading={logsQuery.isLoading}
                  columns={logColumns}
                  dataSource={syncLogs}
                  pagination={{ pageSize: 6, hideOnSinglePage: true }}
                />
              </Card>
            </Col>
          </Row>

          <Card
            title="节假日管理"
            className="ws-panel-card"
            extra={
              <Space>
                <DatePicker picker="year" value={dayjs(`${holidayYear}-01-01`)} onChange={(value) => setHolidayYear(value?.year() || dayjs().year())} />
                <Button onClick={() => setHolidayImportModalOpen(true)}>批量导入</Button>
                <Button type="primary" onClick={() => setHolidayModalOpen(true)}>
                  新增节假日
                </Button>
              </Space>
            }
          >
            <Table
              rowKey="id"
              loading={holidaysQuery.isLoading}
              columns={holidayColumns}
              dataSource={groupedHolidays}
              pagination={{ pageSize: 10, hideOnSinglePage: true }}
            />
          </Card>
        </>
      )}

      <Modal
        open={pushModalOpen}
        title="作息表推送"
        okText="确认推送"
        cancelText="取消"
        onCancel={() => {
          if (pushPersonalMutation.isPending || pushGroupMutation.isPending) return
          setPushModalOpen(false)
        }}
        onOk={handleConfirmPush}
        confirmLoading={pushPersonalMutation.isPending || pushGroupMutation.isPending}
        okButtonProps={{
          disabled: pushTargetType === 'personal' ? pushRecipientIds.length === 0 : !pushGroupTargetId,
        }}
        destroyOnClose
      >
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <div>
            <Text type="secondary">推送对象</Text>
            <Segmented<PushTargetType>
              block
              style={{ marginTop: 8 }}
              value={pushTargetType}
              options={[
                { label: '员工', value: 'personal', disabled: !canManageAttendance },
                { label: '群聊', value: 'group', disabled: !canPushWeekScheduleGroup },
              ]}
              onChange={(value) => {
                setPushTargetType(value)
                setPushRecipientIds([])
                setPushGroupTargetId(undefined)
              }}
            />
          </div>
          <Alert
            type="info"
            showIcon
            message={pushTargetType === 'personal'
              ? `将推送 ${selectedMonth?.format('YYYY年M月') || ''} 作息时间表图片与文字提醒到所选员工钉钉（不写入考勤排班）`
              : `将提交 ${selectedMonth?.format('YYYY年M月') || ''} 作息时间表图片与文字到已绑定钉钉群聊`}
          />
          <div>
            <Text type="secondary">推送月份</Text>
            <div style={{ marginTop: 4, fontWeight: 600 }}>{selectedMonth?.format('YYYY年M月') || '-'}</div>
          </div>
          {pushTargetType === 'personal' ? (
            <div>
              <Text type="secondary">
                收件人
                {usersQuery.isFetching || pushUserSearching ? '（加载中…）' : `（${users.length} 人可选）`}
              </Text>
              <Select
                mode="multiple"
                allowClear
                showSearch
                placeholder={usersQuery.isLoading ? '正在加载员工列表…' : '搜索姓名 / 选择收件人'}
                style={{ width: '100%', marginTop: 8 }}
                value={pushRecipientIds}
                onChange={(values) => setPushRecipientIds(values)}
                onSearch={(value) => setPushUserSearch(value)}
                filterOption={(input, option) => {
                  const label = String(option?.label ?? '')
                  const value = String(option?.value ?? '')
                  const q = input.trim().toLowerCase()
                  return label.toLowerCase().includes(q) || value.toLowerCase().includes(q)
                }}
                optionFilterProp="label"
                options={pushSelectOptions}
                maxTagCount="responsive"
                listHeight={320}
                virtual
                notFoundContent={usersQuery.isLoading || pushUserSearching ? <Spin size="small" /> : '未找到匹配员工'}
              />
              <div style={{ marginTop: 8 }}>
                <Space wrap size={8}>
                  <Button
                    size="small"
                    onClick={() => setPushRecipientIds(users.map((u) => u.user_id))}
                    disabled={users.length === 0}
                  >
                    全选当前列表
                  </Button>
                  <Button size="small" onClick={() => setPushRecipientIds([])}>
                    清空
                  </Button>
                </Space>
              </div>
            </div>
          ) : (
            <div>
              <Text type="secondary">已绑定群聊</Text>
              <Select<number>
                allowClear
                showSearch
                placeholder={groupTargetsQuery.isLoading ? '正在加载群聊…' : '请选择群聊'}
                style={{ width: '100%', marginTop: 8 }}
                value={pushGroupTargetId}
                onChange={(value) => setPushGroupTargetId(value)}
                optionFilterProp="label"
                options={groupTargets.map((item) => ({
                  label: item.status === 'active'
                    ? item.group_name
                    : `${item.group_name}（${item.status === 'inactive' ? '已停用' : '已解绑'}）`,
                  value: item.id,
                  disabled: item.status !== 'active',
                }))}
                notFoundContent={groupTargetsQuery.isLoading ? <Spin size="small" /> : '暂无已绑定群聊'}
              />
              {groupTargetsQuery.isError && (
                <Alert style={{ marginTop: 8 }} type="error" showIcon message="群聊加载失败" description="请稍后重试" />
              )}
              {!groupTargetsQuery.isLoading && !groupTargetsQuery.isError && groupTargets.every((item) => item.status !== 'active') && (
                <Alert
                  style={{ marginTop: 8 }}
                  type="info"
                  showIcon
                  message="暂无可用群聊"
                  description="首次使用时，在群内 @人事系统机器人发送任意内容即可绑定。"
                />
              )}
              {pushGroupTargetId && (
                <div style={{ marginTop: 8 }}>
                  <Text type="secondary">目标群聊：</Text>
                  <Text strong>{groupTargets.find((item) => item.id === pushGroupTargetId)?.group_name || '-'}</Text>
                </div>
              )}
              {!groupTargetsQuery.isLoading && !groupTargetsQuery.isError && groupTargets.length > 0 && (
                <List
                  size="small"
                  header={<Text type="secondary">群聊绑定管理</Text>}
                  dataSource={groupTargets}
                  renderItem={(item) => (
                    <List.Item
                      actions={item.status === 'active'
                        ? [
                            <Button
                              key="unbind"
                              type="link"
                              danger
                              size="small"
                              icon={<DeleteOutlined />}
                              loading={unbindGroupMutation.isPending && unbindGroupMutation.variables === item.id}
                              onClick={() => confirmGroupUnbind(item)}
                            >
                              解绑
                            </Button>,
                          ]
                        : undefined}
                    >
                      <Space size={8}>
                        <Text>{item.group_name}</Text>
                        <Tag color={item.status === 'active' ? 'success' : 'default'}>
                          {item.status === 'active' ? '可用' : item.status === 'inactive' ? '已停用' : '已解绑'}
                        </Tag>
                      </Space>
                    </List.Item>
                  )}
                />
              )}
            </div>
          )}
          {selectedMonth && filteredMonthSections[0] && (
            <div>
              <Text type="secondary">文字预览</Text>
              <div className="ws-push-preview" style={{ marginTop: 8 }}>
                {buildSchedulePushContent(selectedMonth, filteredMonthSections[0]).content}
              </div>
            </div>
          )}
        </Space>
      </Modal>

      <Modal
        open={ruleModalOpen}
        title={editingRule ? '编辑大小周规则' : '新增大小周规则'}
        onCancel={() => {
          setRuleModalOpen(false)
          setEditingRule(null)
          ruleForm.resetFields()
        }}
        onOk={handleSubmitRule}
        confirmLoading={createRuleMutation.isPending || updateRuleMutation.isPending}
      >
        <Form form={ruleForm} layout="vertical">
          <Form.Item<RuleFormValues> label="作用范围" name="scope_type" rules={[{ required: true, message: '请选择范围' }]}>
            <Segmented
              block
              options={[
                { label: '全公司', value: 'company' },
                { label: '部门', value: 'department' },
                { label: '个人', value: 'user' },
              ]}
            />
          </Form.Item>

          {ruleScope !== 'company' && (
            <Form.Item<RuleFormValues>
              label={ruleScope === 'department' ? '选择部门' : '选择员工'}
              name="scope_target_id"
              rules={[{ required: true, message: ruleScope === 'department' ? '请选择部门' : '请选择员工' }]}
            >
              <Select
                showSearch={ruleScope === 'user'}
                options={
                  ruleScope === 'department'
                    ? departments.map((item) => ({ label: item.name, value: item.department_id }))
                    : users.map((item) => ({ label: item.name, value: item.user_id }))
                }
              />
            </Form.Item>
          )}

          <Form.Item<RuleFormValues> label="基准日期" name="base_date" rules={[{ required: true, message: '请选择基准日期' }]}>
            <DatePicker style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item<RuleFormValues> label="轮换模式" name="pattern" rules={[{ required: true, message: '请选择轮换模式' }]}>
            <Select
              options={[
                { label: '基准周为大周', value: 'big_first' },
                { label: '基准周为小周', value: 'small_first' },
              ]}
            />
          </Form.Item>

          <Form.Item<RuleFormValues> label="工作班次" name="shift_id">
            <Select
              options={[
                { label: '默认班次', value: 0 },
                ...shifts.map((item) => ({ label: item.name, value: item.id })),
              ]}
            />
          </Form.Item>

          <Form.Item<RuleFormValues> label="状态" name="status" rules={[{ required: true, message: '请选择状态' }]}>
            <Select
              options={[
                { label: '生效中', value: 'active' },
                { label: '已停用', value: 'inactive' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        open={overrideModalOpen}
        title="手动覆盖本周"
        onCancel={() => {
          setOverrideModalOpen(false)
          setSelectedWeek(null)
          overrideForm.resetFields()
        }}
        onOk={handleSubmitOverride}
        confirmLoading={overrideMutation.isPending}
      >
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          {selectedWeek && (
            <Alert
              type="info"
              showIcon
              message={`周范围：${selectedWeek.week_start} 至 ${selectedWeek.week_end}`}
              description={`当前范围：${currentScopeName || '全公司'}`}
            />
          )}

          <Form form={overrideForm} layout="vertical">
            <Form.Item<OverrideFormValues> label="覆盖为" name="week_type" rules={[{ required: true, message: '请选择周类型' }]}>
              <Select
                options={[
                  { label: '大周（双休）', value: 'big' },
                  { label: '小周（单休）', value: 'small' },
                ]}
              />
            </Form.Item>

            <Form.Item<OverrideFormValues> label="说明" name="reason">
              <TextArea rows={4} maxLength={120} placeholder="例如：五一调休、项目上线保障" />
            </Form.Item>
          </Form>
        </Space>
      </Modal>

      <Modal
        open={holidayModalOpen}
        title="新增节假日"
        onCancel={() => {
          setHolidayModalOpen(false)
          holidayForm.resetFields()
        }}
        onOk={handleSubmitHoliday}
        confirmLoading={createHolidayMutation.isPending}
      >
        <Form form={holidayForm} layout="vertical">
          <Form.Item<HolidayFormValues> label="日期" name="date" rules={[{ required: true, message: '请选择日期' }]}>
            <DatePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item<HolidayFormValues> label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="例如：劳动节、国庆调休上班" />
          </Form.Item>
          <Form.Item<HolidayFormValues> label="类型" name="type" rules={[{ required: true, message: '请选择类型' }]}>
            <Select
              options={[
                { label: '放假', value: 'holiday' },
                { label: '调休上班', value: 'workday' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        open={holidayImportModalOpen}
        title="批量导入节假日"
        onCancel={() => {
          setHolidayImportModalOpen(false)
          setHolidayImportText('')
        }}
        onOk={handleImportHolidays}
        confirmLoading={batchHolidayMutation.isPending}
      >
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            message="支持 JSON 数组导入"
            description='示例：[{"date":"2026-05-01","name":"劳动节","type":"holiday"}]'
          />
          <TextArea rows={10} value={holidayImportText} onChange={(event) => setHolidayImportText(event.target.value)} />
        </Space>
      </Modal>

      <Modal
        open={shiftModalOpen}
        title="创建新班次"
        onCancel={() => {
          setShiftModalOpen(false)
          shiftForm.resetFields()
        }}
        onOk={handleSubmitShift}
        confirmLoading={createShiftMutation.isPending}
      >
        <Form form={shiftForm} layout="vertical">
          <Form.Item<ShiftFormValues> label="班次名称" name="name" rules={[{ required: true, message: '请输入班次名称' }]}>
            <Input placeholder="例如：17:30下班" />
          </Form.Item>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item<ShiftFormValues> label="上班时间" name="check_in_time" rules={[{ required: true, message: '请输入上班时间' }]}>
                <Input placeholder="09:00" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item<ShiftFormValues> label="下班时间" name="check_out_time" rules={[{ required: true, message: '请输入下班时间' }]}>
                <Input placeholder="17:30" />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
    </PageContainer>
  )
}
