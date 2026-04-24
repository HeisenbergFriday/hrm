import React, { useMemo, useState } from 'react'
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
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Segmented,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import type { TableColumnsType } from 'antd'
import { CalendarOutlined, DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined, SyncOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { departmentAPI, shiftConfigAPI, userAPI, weekScheduleAPI } from '../services/api'

const { Title, Text, Paragraph } = Typography
const { TextArea } = Input

type ScopeType = 'company' | 'department' | 'user'
type WeekType = 'big' | 'small'
type HolidayType = 'holiday' | 'workday'
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
      color: '#fa8c16',
      tagColor: 'orange',
      background: '#fff7e6',
      borderColor: '#ffd591',
    }
  }

  return {
    label: '大周',
    restLabel: '双休',
    color: '#1677ff',
    tagColor: 'blue',
    background: '#e6f4ff',
    borderColor: '#91caff',
  }
}

function getStatusTag(status: string) {
  if (status === 'active') return <Tag color="success">生效中</Tag>
  if (status === 'inactive') return <Tag>已停用</Tag>
  if (status === 'success') return <Tag color="success">成功</Tag>
  if (status === 'partial') return <Tag color="warning">部分成功</Tag>
  if (status === 'failed') return <Tag color="error">失败</Tag>
  return <Tag>{status}</Tag>
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

function getCellStyle(state: CalendarCellState) {
  if (state === 'outside') return { background: '#fafafa', color: '#bfbfbf' }
  if (state === 'holiday') return { background: '#ff4d4f', color: '#fff' }
  if (state === 'rest') return { background: '#fff', color: '#1f1f1f' }
  return { background: '#fff566', color: '#1f1f1f' }
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

export default function WeekSchedule() {
  const queryClient = useQueryClient()
  const [calendarScopeType, setCalendarScopeType] = useState<ScopeType>('company')
  const [selectedDepartmentId, setSelectedDepartmentId] = useState('')
  const [selectedUserId, setSelectedUserId] = useState('')
  const [syncWeeks, setSyncWeeks] = useState(4)
  const [holidayYear, setHolidayYear] = useState(dayjs().year())

  const [ruleModalOpen, setRuleModalOpen] = useState(false)
  const [overrideModalOpen, setOverrideModalOpen] = useState(false)
  const [holidayModalOpen, setHolidayModalOpen] = useState(false)
  const [holidayImportModalOpen, setHolidayImportModalOpen] = useState(false)
  const [shiftModalOpen, setShiftModalOpen] = useState(false)

  const [editingRule, setEditingRule] = useState<WeekScheduleRule | null>(null)
  const [selectedWeek, setSelectedWeek] = useState<WeekCalendarItem | null>(null)
  const [holidayImportText, setHolidayImportText] = useState('')

  const [ruleForm] = Form.useForm<RuleFormValues>()
  const [overrideForm] = Form.useForm<OverrideFormValues>()
  const [holidayForm] = Form.useForm<HolidayFormValues>()
  const [shiftForm] = Form.useForm<ShiftFormValues>()

  const usersQuery = useQuery({
    queryKey: ['week-schedule', 'users'],
    queryFn: () => userAPI.getUsers({ page: 1, page_size: 500 }),
    retry: false,
  })

  const departmentsQuery = useQuery({
    queryKey: ['week-schedule', 'departments'],
    queryFn: () => departmentAPI.getDepartments(),
    retry: false,
  })

  const shiftsQuery = useQuery({
    queryKey: ['week-schedule', 'shifts'],
    queryFn: () => weekScheduleAPI.getShifts(),
    retry: false,
  })

  const rulesQuery = useQuery({
    queryKey: ['week-schedule', 'rules'],
    queryFn: () => weekScheduleAPI.getRules(),
    retry: false,
  })

  const logsQuery = useQuery({
    queryKey: ['week-schedule', 'logs'],
    queryFn: () => weekScheduleAPI.getSyncLogs({ page: 1, page_size: 20 }),
    retry: false,
  })

  const users = getItems<UserOption>(usersQuery.data)
  const departments = unwrapData<{ departments: DepartmentOption[] }>(departmentsQuery.data, { departments: [] }).departments ?? []
  const shifts = getItems<ShiftOption>(shiftsQuery.data)
  const rules = getItems<WeekScheduleRule>(rulesQuery.data)
  const syncLogs = getItems<SyncLogRecord>(logsQuery.data)

  const selectedDepartment = departments.find((item) => item.department_id === selectedDepartmentId) ?? null
  const selectedUser = users.find((item) => item.user_id === selectedUserId) ?? null

  const calendarParams = useMemo(() => {
    if (calendarScopeType === 'department') {
      return { weeks: 8, department_id: selectedDepartmentId }
    }
    if (calendarScopeType === 'user') {
      return { weeks: 8, user_id: selectedUserId, department_id: selectedUser?.department_id || '' }
    }
    return { weeks: 8 }
  }, [calendarScopeType, selectedDepartmentId, selectedUserId, selectedUser?.department_id])

  const canQueryCalendar =
    calendarScopeType === 'company' ||
    (calendarScopeType === 'department' && Boolean(selectedDepartmentId)) ||
    (calendarScopeType === 'user' && Boolean(selectedUserId))

  const calendarQuery = useQuery({
    queryKey: ['week-schedule', 'calendar', calendarScopeType, selectedDepartmentId, selectedUserId],
    queryFn: () => weekScheduleAPI.getCalendar(calendarParams),
    enabled: canQueryCalendar,
    retry: false,
  })

  const holidaysQuery = useQuery({
    queryKey: ['week-schedule', 'holidays', holidayYear],
    queryFn: () => weekScheduleAPI.getHolidays({ year: holidayYear }),
    retry: false,
  })

  const calendarItems = getItems<WeekCalendarItem>(calendarQuery.data)
  const holidayRecords = getItems<HolidayRecord>(holidaysQuery.data)
  const monthCalendarSections = useMemo(() => buildMonthCalendarSections(calendarItems), [calendarItems])

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

  const syncToMutation = useMutation({
    mutationFn: () => weekScheduleAPI.syncToDingtalk({ weeks: syncWeeks }),
    onSuccess: async (response) => {
      const result = unwrapData<{ message?: string }>(response, {})
      message.success(result.message || '已推送到钉钉')
      await invalidateAll()
    },
    onError: (error) => message.error(getErrorMessage(error, '推送到钉钉失败')),
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
    setSelectedWeek(week)
    overrideForm.setFieldsValue({
      week_type: week.week_type,
      reason: '',
    })
    setOverrideModalOpen(true)
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
            <Tag color="blue">{getScopeLabel(record.scope_type)}</Tag>
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
      render: (value) => dayjs(value).format('YYYY-MM-DD HH:mm'),
    },
    {
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
    },
  ]

  const holidayColumns: TableColumnsType<HolidayRecord> = [
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
      render: (value: HolidayType) => <Tag color={value === 'holiday' ? 'red' : 'gold'}>{value === 'holiday' ? '放假' : '调休上班'}</Tag>,
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_, record) => (
        <Popconfirm title="确定删除这条节假日记录？" onConfirm={() => deleteHolidayMutation.mutate(record.id)}>
          <Button size="small" danger icon={<DeleteOutlined />}>
            删除
          </Button>
        </Popconfirm>
      ),
    },
  ]

  const logColumns: TableColumnsType<SyncLogRecord> = [
    {
      title: '时间',
      dataIndex: 'created_at',
      width: 180,
      render: (value) => dayjs(value).format('YYYY-MM-DD HH:mm'),
    },
    {
      title: '同步方向',
      dataIndex: 'sync_type',
      width: 120,
      render: (value) => getSyncTypeLabel(value),
    },
    {
      title: '影响人数',
      dataIndex: 'user_count',
      width: 100,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (value) => getStatusTag(value),
    },
    {
      title: '说明',
      dataIndex: 'message',
    },
  ]

  const ruleScope = Form.useWatch('scope_type', ruleForm) ?? 'company'

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card>
        <Space direction="vertical" size={8}>
          <Title level={3} style={{ margin: 0 }}>
            大小周与节假日管理
          </Title>
          <Paragraph type="secondary" style={{ marginBottom: 0 }}>
            日历已改成按月表格展示。工作日、休息日、节假日和大小周状态都会在同一张月度表里展示，点击任意周行可以手动覆盖该周为大周或小周。
          </Paragraph>
        </Space>
      </Card>

      <Card
        title="查询与同步"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={() => invalidateAll()}>
              刷新
            </Button>
            <Button loading={syncHolidayMutation.isPending} onClick={() => syncHolidayMutation.mutate()}>
              同步节假日
            </Button>
            <Button loading={syncFromMutation.isPending} onClick={() => syncFromMutation.mutate()}>
              从钉钉拉取
            </Button>
            <InputNumber min={1} max={12} value={syncWeeks} onChange={(value) => setSyncWeeks(Number(value) || 4)} />
            <Button type="primary" icon={<SyncOutlined />} loading={syncToMutation.isPending} onClick={() => syncToMutation.mutate()}>
              推送到钉钉
            </Button>
          </Space>
        }
      >
        <Row gutter={[16, 16]}>
          <Col xs={24} lg={8}>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              <Text strong>查看范围</Text>
              <Segmented
                block
                value={calendarScopeType}
                onChange={(value) => setCalendarScopeType(value as ScopeType)}
                options={[
                  { label: '全公司', value: 'company' },
                  { label: '部门', value: 'department' },
                  { label: '个人', value: 'user' },
                ]}
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
        title="大小周日历"
        extra={<Text type="secondary">当前范围：{currentScopeName || '未选择'}</Text>}
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
          <Empty description="暂无可展示的周次安排" />
        ) : (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Alert
              type="info"
              showIcon
              message="颜色说明"
              description="黄色表示工作日，白色表示休息日，红色表示法定节假日或特殊休假，灰色表示非当前月份日期。右侧“大小周”列会同时展示单双休、周六状态和本周节假日摘要。"
            />

            {monthCalendarSections.map((section) => (
              <Card
                key={section.month}
                title={`${dayjs(`${section.month}-01`).format('YYYY年M月')}作息时间表`}
                bodyStyle={{ padding: 0, overflowX: 'auto' }}
              >
                <table
                  style={{
                    width: '100%',
                    minWidth: 980,
                    tableLayout: 'fixed',
                    borderCollapse: 'collapse',
                  }}
                >
                  <thead>
                    <tr>
                      {['周数', '周一', '周二', '周三', '周四', '周五', '周六', '周日', '大小周'].map((label) => (
                        <th
                          key={label}
                          style={{
                            border: '1px solid #d9d9d9',
                            padding: '12px 8px',
                            background: '#fafafa',
                            textAlign: 'center',
                            fontWeight: 700,
                            fontSize: 18,
                          }}
                        >
                          {label}
                        </th>
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

                      return (
                        <tr key={row.week.week_start} onClick={() => openOverrideModal(row.week)} style={{ cursor: 'pointer' }}>
                          <td
                            style={{
                              border: '1px solid #d9d9d9',
                              padding: 12,
                              background: '#fff',
                              textAlign: 'center',
                              verticalAlign: 'middle',
                              fontWeight: 700,
                            }}
                          >
                            <div>第{index + 1}周</div>
                            <div style={{ marginTop: 6, fontSize: 12, color: '#8c8c8c', fontWeight: 400 }}>
                              {dayjs(row.week.week_start).format('MM/DD')} - {dayjs(row.week.week_end).format('MM/DD')}
                            </div>
                          </td>

                          {row.cells.map((cell) => {
                            const cellStyle = getCellStyle(cell.state)
                            return (
                              <td
                                key={cell.date}
                                style={{
                                  border: '1px solid #d9d9d9',
                                  padding: 8,
                                  textAlign: 'center',
                                  verticalAlign: 'top',
                                  ...cellStyle,
                                }}
                              >
                                <div
                                  style={{
                                    fontSize: 18,
                                    fontWeight: 700,
                                    lineHeight: 1.2,
                                    opacity: cell.inCurrentMonth ? 1 : 0.35,
                                  }}
                                >
                                  {cell.dayNumber}
                                </div>
                                <div style={{ marginTop: 8, minHeight: 36, fontSize: 12, lineHeight: 1.5 }}>
                                  {cell.holidayLabel ? (
                                    cell.holidayLabel
                                  ) : cell.state === 'rest' ? (
                                    '休息'
                                  ) : cell.state === 'saturday-work' ? (
                                    <>
                                      <div>周六上班</div>
                                      {selectedUserEndTime && (
                                        <div style={{ color: '#1677ff', marginTop: 2 }}>下班 {selectedUserEndTime}</div>
                                      )}
                                    </>
                                  ) : cell.state === 'work' && selectedUserEndTime ? (
                                    <div style={{ color: '#1677ff' }}>下班 {selectedUserEndTime}</div>
                                  ) : null}
                                </div>
                              </td>
                            )
                          })}

                          <td
                            style={{
                              border: '1px solid #d9d9d9',
                              padding: 12,
                              background: weekMeta.background,
                              verticalAlign: 'middle',
                            }}
                          >
                            <div style={{ fontSize: 16, fontWeight: 700, color: weekMeta.color }}>{weekMeta.restLabel}</div>
                            <div style={{ marginTop: 8, fontSize: 12, color: '#595959', lineHeight: 1.6 }}>
                              {row.week.is_override ? '已手动覆盖' : '按规则生成'}
                              <br />
                              {row.week.saturday_work ? '周六上班' : '周六休息'}
                            </div>
                            <div style={{ marginTop: 8, fontSize: 12, color: '#8c8c8c', lineHeight: 1.6 }}>{holidaySummary}</div>
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

      <Row gutter={[16, 16]}>
        <Col xs={24} xl={14}>
          <Card
            title="规则管理"
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

        <Col xs={24} xl={10}>
          <Card
            title="钉钉同步"
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
          dataSource={holidayRecords}
          pagination={{ pageSize: 10, hideOnSinglePage: true }}
        />
      </Card>

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
    </Space>
  )
}
