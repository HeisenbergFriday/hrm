import React, { useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Alert, Card, Col, Row, Space, Table, Tag, Typography, Button, Modal, Form, Input, InputNumber,
  Select, message, Spin, Drawer, Tooltip, Divider, Descriptions, Steps
} from 'antd'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import type { ColumnsType } from 'antd/es/table'
import type { Dayjs } from 'dayjs'
import dayjs from 'dayjs'
import {
  departmentAPI,
  performanceAPI,
  PerformanceActivity,
  PerformanceParticipant,
  PerformanceDistributionRule,
  PerformanceHRDeadlineStatus,
  PerformanceIndicatorLibrary,
  userAPI,
} from '../services/api'
import PerformanceActivityEditor from '../components/PerformanceActivityEditor'
import { BarChartOutlined, PlusOutlined } from '@ant-design/icons'
import { getCycleLabel, formatDateTime } from '../utils/format'

const { Text, Paragraph } = Typography
const { TextArea } = Input

type RejectGoalFormValues = {
  comment: string
}

function normalizeIDArray(value?: string[] | string): string[] {
  if (Array.isArray(value)) return value.filter(Boolean)
  if (!value) return []
  return String(value).split(',').map(item => item.trim()).filter(Boolean)
}

function getListFromResponse(res: any, keys: string[]): any[] {
  const data = res?.data || res
  if (Array.isArray(data)) return data
  for (const key of keys) {
    if (Array.isArray(data?.[key])) return data[key]
  }
  return []
}

function getDepartmentOption(department: any) {
  const value = String(department.department_id || department.id || '')
  const name = department.name || department.department_name || value
  return value ? { value, label: `${name}（${value}）` } : null
}

function getUserOption(user: any) {
  const value = String(user.user_id || user.employee_id || user.id || '')
  const name = user.name || user.user_name || user.employee_name || value
  const departmentName = user.department_name ? ` - ${user.department_name}` : ''
  return value ? { value, label: `${name}（${value}）${departmentName}` } : null
}

function formatRangeStart(range?: [Dayjs, Dayjs]) {
  return range?.[0]?.format('YYYY-MM-DD') || ''
}

function formatRangeEnd(range?: [Dayjs, Dayjs]) {
  return range?.[1]?.format('YYYY-MM-DD') || ''
}

// 状态映射
const STATUS_MAP: Record<string, { label: string; color: string }> = {
  draft: { label: '草稿', color: 'default' },
  target_setting: { label: '目标设定', color: 'cyan' },
  self_evaluation: { label: '自评中', color: 'processing' },
  manager_evaluation: { label: '主管评分', color: 'warning' },
  employee_confirmation: { label: '员工确认', color: 'blue' },
  manager_confirmation: { label: '主管确认', color: 'orange' },
  hr_confirmation: { label: 'HR确认', color: 'purple' },
  locked: { label: '已锁定', color: 'error' },
  result_confirmed: { label: '已确认', color: 'success' },
  archived: { label: '已归档', color: 'default' },
}

const STATUS_OPTIONS = Object.entries(STATUS_MAP).map(([value, { label }]) => ({ value, label }))

// 参与人状态映射
const PARTICIPANT_STATUS_MAP: Record<string, { label: string; color: string }> = {
  pending: { label: '待目标', color: 'default' },
  target_pending_approval: { label: '目标待审', color: 'cyan' },
  target_rejected: { label: '目标驳回', color: 'red' },
  target_set: { label: '目标已定', color: 'cyan' },
  self_submitted: { label: '已自评', color: 'processing' },
  manager_submitted: { label: '已评分', color: 'warning' },
  employee_confirmed: { label: '已员工确认', color: 'blue' },
  manager_confirmed: { label: '已主管确认', color: 'orange' },
  hr_confirmed: { label: '已HR确认', color: 'purple' },
  locked: { label: '已冻结', color: 'orange' },
  result_confirmed: { label: '已确认', color: 'success' },
  inactive: { label: '已离职', color: 'error' },
  removed_from_scope: { label: '已移除', color: 'error' },
}

const ACTIVITY_FLOW = [
  { status: 'target_setting', label: '目标设定' },
  { status: 'self_evaluation', label: '自评' },
  { status: 'manager_evaluation', label: '评分' },
  { status: 'employee_confirmation', label: '员工确认' },
  { status: 'manager_confirmation', label: '主管确认' },
  { status: 'hr_confirmation', label: 'HR确认' },
  { status: 'archived', label: '归档' },
]

function formatDateRange(start?: string, end?: string) {
  if (!start && !end) return '-'
  return `${start || '-'} ~ ${end || '-'}`
}

function getActivityStepIndex(status?: string) {
  if (status === 'locked') return ACTIVITY_FLOW.length - 1 // archived step
  if (status === 'draft') return 0
  const index = ACTIVITY_FLOW.findIndex(item => item.status === status)
  return index >= 0 ? index : 0
}

function getStatusMeta(status?: string) {
  return STATUS_MAP[status || ''] || { label: status || '-', color: 'default' }
}

function getParticipantStatusMeta(status?: string) {
  return PARTICIPANT_STATUS_MAP[status || ''] || { label: status || '-', color: 'default' }
}

const PerformanceOverview: React.FC = () => {
  const navigate = useNavigate()
  const [, forceRender] = React.useState(0)
  const forceUpdate = () => forceRender(n => n + 1)
  const [activities, setActivities] = useState<PerformanceActivity[]>([])
  const [activitiesLoading, setActivitiesLoading] = useState(false)
  const [activitiesTotal, setActivitiesTotal] = useState(0)
  const [activityModalVisible, setActivityModalVisible] = useState(false)
  const [activitySaving, setActivitySaving] = useState(false)
  const [editingActivity, setEditingActivity] = useState<PerformanceActivity | null>(null)
  const [form] = Form.useForm()
  const [departments, setDepartments] = useState<any[]>([])
  const [users, setUsers] = useState<any[]>([])
  const [scopeOptionsLoading, setScopeOptionsLoading] = useState(false)
  const [indicatorLibraries, setIndicatorLibraries] = useState<PerformanceIndicatorLibrary[]>([])
  const [indicatorLibrariesLoading, setIndicatorLibrariesLoading] = useState(false)

  // 活动详情抽屉
  const [detailDrawerVisible, setDetailDrawerVisible] = useState(false)
  const [currentActivity, setCurrentActivity] = useState<PerformanceActivity | null>(null)
  const [participants, setParticipants] = useState<PerformanceParticipant[]>([])
  const [participantsLoading, setParticipantsLoading] = useState(false)
  const [summaryLoading, setSummaryLoading] = useState(false)
  const [distributionCheckLoading, setDistributionCheckLoading] = useState(false)
  const [summary, setSummary] = useState<any>(null)
  const [distributionCheck, setDistributionCheck] = useState<any>(null)
  const [distributionRules, setDistributionRules] = useState<PerformanceDistributionRule[]>([])
  const [hrDeadlineStatus, setHrDeadlineStatus] = useState<PerformanceHRDeadlineStatus | null>(null)

  // 评分弹窗
  // 强制分布弹窗
  const [distributionModalVisible, setDistributionModalVisible] = useState(false)
  const [distributionForm] = Form.useForm()
  const [rejectGoalForm] = Form.useForm<RejectGoalFormValues>()

  // 批量评分相关
  const [batchEvalModalVisible, setBatchEvalModalVisible] = useState(false)
  const [batchEvalSelected, setBatchEvalSelected] = useState<number[]>([])
  const [batchEvalLoading, setBatchEvalLoading] = useState(false)
  const [batchEvalForm] = Form.useForm()
  const [batchEvalScore, setBatchEvalScore] = useState<number>(0)

  // 活动列表筛选
  const [activitySearchText, setActivitySearchText] = useState('')
  const [activityStatusFilter, setActivityStatusFilter] = useState<string | undefined>(undefined)

  // 加载活动列表
  const loadActivities = useCallback(async () => {
    setActivitiesLoading(true)
    try {
      const res: any = await performanceAPI.getActivities({ page: 1, page_size: 100 })
      const data = res.data || res
      setActivities(data.items || [])
      setActivitiesTotal(data.total || 0)
    } catch (err: any) {
      message.error(err?.response?.data?.message || '加载活动列表失败')
    } finally {
      setActivitiesLoading(false)
    }
  }, [])

  // 加载活动适用范围选项
  const loadScopeOptions = useCallback(async () => {
    setScopeOptionsLoading(true)
    try {
      const [departmentResult, userResult] = await Promise.allSettled([
        departmentAPI.getDepartments(),
        userAPI.getUsers({ page: 1, page_size: 2000 }),
      ])

      const failed: string[] = []
      if (departmentResult.status === 'fulfilled') {
        setDepartments(getListFromResponse(departmentResult.value, ['departments', 'items']))
      } else {
        failed.push('部门')
      }

      if (userResult.status === 'fulfilled') {
        setUsers(getListFromResponse(userResult.value, ['items', 'users', 'employees']))
      } else {
        failed.push('员工')
      }

      if (failed.length) {
        message.error(`${failed.join('、')}选项加载失败`)
      }
    } finally {
      setScopeOptionsLoading(false)
    }
  }, [])

  const loadIndicatorLibraries = useCallback(async () => {
    setIndicatorLibrariesLoading(true)
    try {
      const res: any = await performanceAPI.getIndicatorLibraries({
        page: 1,
        page_size: 1000,
        status: 'active',
      })
      setIndicatorLibraries(getListFromResponse(res, ['items', 'libraries']))
    } catch {
      setIndicatorLibraries([])
      message.error('指标库选项加载失败')
    } finally {
      setIndicatorLibrariesLoading(false)
    }
  }, [])

  // 首次加载活动列表
  React.useEffect(() => {
    loadActivities()
  }, [loadActivities])

  // 加载活动详情
  const loadActivityDetail = async (activity: PerformanceActivity) => {
    setCurrentActivity(activity)
    setDetailDrawerVisible(true)
    setParticipantsLoading(true)
    setSummaryLoading(true)
    setDistributionCheckLoading(true)
    setHrDeadlineStatus(null)

    // 使用 Promise.allSettled 避免单个接口失败阻塞整个流程
    const results = await Promise.allSettled([
      performanceAPI.getParticipants(activity.id, { page: 1, page_size: 200 }),
      performanceAPI.getResultSummary(activity.id),
      performanceAPI.getDistributionCheck(activity.id),
      performanceAPI.getDistributionRules(activity.id),
      performanceAPI.getHRConfirmDeadlineStatus(activity.id),
    ])

    // 处理参与人
    const participantsResult = results[0]
    if (participantsResult.status === 'fulfilled') {
      const res = participantsResult.value as any
      const pData = res?.data || res
      setParticipants(pData?.items || [])
    } else {
      setParticipants([])
    }
    setParticipantsLoading(false)

    // 处理统计摘要
    const summaryResult = results[1]
    if (summaryResult.status === 'fulfilled') {
      const res = summaryResult.value as any
      setSummary(res?.data || null)
    } else {
      setSummary(null)
    }
    setSummaryLoading(false)

    // 处理强制分布检查
    const distributionCheckResult = results[2]
    if (distributionCheckResult.status === 'fulfilled') {
      const res = distributionCheckResult.value as any
      const dcData = res?.data || res
      setDistributionCheck(dcData || null)
    } else {
      setDistributionCheck(null)
    }
    setDistributionCheckLoading(false)

    // 处理分布规则
    const rulesResult = results[3]
    if (rulesResult.status === 'fulfilled') {
      const res = rulesResult.value as any
      const rData = res?.data || res
      setDistributionRules(rData?.rules || [])
    } else {
      setDistributionRules([])
    }

    const hrDeadlineResult = results[4]
    if (hrDeadlineResult.status === 'fulfilled') {
      const res = hrDeadlineResult.value as any
      setHrDeadlineStatus((res?.data || res) as PerformanceHRDeadlineStatus)
    } else {
      setHrDeadlineStatus(null)
    }
  }

  const refreshParticipants = async (activityId: number) => {
    setParticipantsLoading(true)
    try {
      const res: any = await performanceAPI.getParticipants(activityId, { page: 1, page_size: 200 })
      const pData = res?.data || res
      setParticipants(pData?.items || [])
    } catch {
      setParticipants([])
    } finally {
      setParticipantsLoading(false)
    }
  }

  const closeActivityEditor = () => {
    setActivityModalVisible(false)
    setEditingActivity(null)
    form.resetFields()
  }

  // 创建/编辑活动
  const handleSaveActivity = async () => {
    if (activitySaving) return
    setActivitySaving(true)
    try {
      const values = await form.validateFields()
      const data = {
        name: values.name,
        cycle_type: values.cycle_type,
        start_date: values.date_range[0].format('YYYY-MM-DD'),
        end_date: values.date_range[1].format('YYYY-MM-DD'),
        target_set_start_at: formatRangeStart(values.target_set_range),
        target_set_end_at: formatRangeEnd(values.target_set_range),
        self_eval_start_at: values.self_eval_range[0].format('YYYY-MM-DD'),
        self_eval_end_at: values.self_eval_range[1].format('YYYY-MM-DD'),
        manager_eval_start_at: values.manager_eval_range[0].format('YYYY-MM-DD'),
        manager_eval_end_at: values.manager_eval_range[1].format('YYYY-MM-DD'),
        result_confirm_start_at: values.result_confirm_range[0].format('YYYY-MM-DD'),
        result_confirm_end_at: values.result_confirm_range[1].format('YYYY-MM-DD'),
        employee_confirm_start_at: formatRangeStart(values.employee_confirm_range),
        employee_confirm_end_at: formatRangeEnd(values.employee_confirm_range),
        manager_confirm_start_at: formatRangeStart(values.manager_confirm_range),
        manager_confirm_end_at: formatRangeEnd(values.manager_confirm_range),
        hr_confirm_start_at: formatRangeStart(values.hr_confirm_range),
        hr_confirm_end_at: formatRangeEnd(values.hr_confirm_range),
        hr_confirm_deadline: values.hr_confirm_deadline?.format('YYYY-MM-DD') || '',
        status: editingActivity?.status || 'draft',
        target_department_ids: normalizeIDArray(values.target_department_ids),
        target_employee_ids: normalizeIDArray(values.target_employee_ids),
        indicator_library_id: values.indicator_library_id,
        description: values.description,
        enable_bonus_score: values.enable_bonus_score || false,
        strict_time_mode: values.strict_time_mode || false,
      }
      if (editingActivity) {
        await performanceAPI.updateActivity(editingActivity.id, data)
        message.success('更新成功')
      } else {
        await performanceAPI.createActivity(data)
        message.success('创建成功')
      }
      closeActivityEditor()
      loadActivities()
    } catch (err: any) {
      if (err.errorFields) {
        const firstField = err.errorFields[0]?.name
        if (firstField) {
          form.scrollToField(firstField, { behavior: 'smooth', block: 'center' })
        }
        message.warning('请补充必填信息')
        return
      }
      message.error(err?.response?.data?.message || '操作失败')
    } finally {
      setActivitySaving(false)
    }
  }

  // 活动状态操作
  const handleActivityAction = async (action: string, activity: PerformanceActivity) => {
    try {
      const apiMap: Record<string, (id: number) => Promise<any>> = {
        start: performanceAPI.startActivity,
        'open-self-evaluation': performanceAPI.openSelfEvaluation,
        'open-manager-evaluation': performanceAPI.openManagerEvaluation,
        'confirm-results': performanceAPI.confirmResults,
        archive: performanceAPI.archiveActivity,
        publish: performanceAPI.publishActivity,
        close: performanceAPI.closeActivity,
        refresh: performanceAPI.refreshParticipants,
        'open-target-setting': performanceAPI.openTargetSetting,
        'open-employee-confirmation': performanceAPI.openEmployeeConfirmation,
        'open-manager-confirmation': performanceAPI.openManagerConfirmation,
        'open-hr-confirmation': performanceAPI.openHRConfirmation,
        lock: performanceAPI.lockActivity,
        'force-lock-overdue-hr': performanceAPI.forceLockOverdueHR,
        'notify-self-eval': performanceAPI.sendSelfEvalReminder,
      }
      const apiFn = apiMap[action]
      if (!apiFn) return
      await apiFn(activity.id)
      message.success('操作成功')
      loadActivities()
      if (detailDrawerVisible && currentActivity?.id === activity.id) {
        const detailRes: any = await performanceAPI.getActivity(activity.id)
        const updated = detailRes.data?.activity || detailRes.data || detailRes
        await loadActivityDetail(updated)
      } else if (action === 'refresh') {
        const detailRes: any = await performanceAPI.getActivity(activity.id)
        const updated = detailRes.data?.activity || detailRes.data || detailRes
        await loadActivityDetail(updated)
      }
    } catch (err: any) {
      message.error(err?.response?.data?.message || '操作失败')
    }
  }

  // 保存强制分布规则
  const handleForceLockOverdueHR = (activity: PerformanceActivity) => {
    Modal.confirm({
      title: '逾期强制锁定',
      content: '将把已完成主管确认但未完成 HR 确认的参与人标记为逾期强制锁定，并锁定活动。此操作会冻结绩效结果。',
      okText: '确认强制锁定',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: () => handleActivityAction('force-lock-overdue-hr', activity),
    })
  }

  const handleSaveDistribution = async () => {
    if (!currentActivity) return
    try {
      const values = await distributionForm.validateFields()
      const rules = ['S', 'A', 'B', 'C', 'D'].map(level => ({
        level,
        distribution_percent: values[`percent_${level}`] || 0,
        description: values[`desc_${level}`] || '',
      }))
      const total = Object.values(values).reduce((sum: number, v: any) => sum + (Number(v) || 0), 0)
      if (total !== 100) {
        message.warning(`比例总和 ${total}%，需等于 100%`)
        return
      }
      await performanceAPI.putDistributionRules(currentActivity.id, rules)
      message.success('强制分布规则已保存')
      setDistributionModalVisible(false)
      if (currentActivity) loadActivityDetail(currentActivity)
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '保存失败')
    }
  }

  const handleRejectGoalRecords = (record: PerformanceParticipant) => {
    rejectGoalForm.resetFields()
    Modal.confirm({
      title: '驳回目标',
      content: (
        <Form form={rejectGoalForm} layout="vertical" preserve={false}>
          <Form.Item
            name="comment"
            label="驳回原因"
            rules={[{ required: true, message: '请输入驳回原因' }]}
          >
            <TextArea rows={3} placeholder="请说明需要员工调整的内容" />
          </Form.Item>
        </Form>
      ),
      okText: '确认驳回',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        const values = await rejectGoalForm.validateFields()
        try {
          await performanceAPI.rejectGoalRecords(record.id, { comment: values.comment })
          message.success('目标已驳回')
          if (currentActivity) loadActivityDetail(currentActivity)
        } catch (err: any) {
          message.error(err?.response?.data?.message || '驳回失败')
          throw err
        }
      },
    })
  }

  // 打开活动表单
  const openActivityModal = (activity?: PerformanceActivity) => {
    setEditingActivity(activity || null)
    loadScopeOptions()
    loadIndicatorLibraries()
    if (activity) {
      form.setFieldsValue({
        name: activity.name,
        cycle_type: activity.cycle_type,
        date_range: [dayjs(activity.start_date), dayjs(activity.end_date)],
        target_set_range: activity.target_set_start_at && activity.target_set_end_at ? [dayjs(activity.target_set_start_at), dayjs(activity.target_set_end_at)] : undefined,
        self_eval_range: [dayjs(activity.self_eval_start_at), dayjs(activity.self_eval_end_at)],
        manager_eval_range: [dayjs(activity.manager_eval_start_at), dayjs(activity.manager_eval_end_at)],
        result_confirm_range: [dayjs(activity.result_confirm_start_at), dayjs(activity.result_confirm_end_at)],
        employee_confirm_range: activity.employee_confirm_start_at && activity.employee_confirm_end_at ? [dayjs(activity.employee_confirm_start_at), dayjs(activity.employee_confirm_end_at)] : undefined,
        manager_confirm_range: activity.manager_confirm_start_at && activity.manager_confirm_end_at ? [dayjs(activity.manager_confirm_start_at), dayjs(activity.manager_confirm_end_at)] : undefined,
        hr_confirm_range: activity.hr_confirm_start_at && activity.hr_confirm_end_at ? [dayjs(activity.hr_confirm_start_at), dayjs(activity.hr_confirm_end_at)] : undefined,
        hr_confirm_deadline: activity.hr_confirm_deadline ? dayjs(activity.hr_confirm_deadline) : undefined,
        target_department_ids: normalizeIDArray(activity.target_department_ids),
        target_employee_ids: normalizeIDArray(activity.target_employee_ids),
        indicator_library_id: activity.indicator_library_id,
        description: activity.description,
        enable_bonus_score: activity.enable_bonus_score || false,
        strict_time_mode: activity.strict_time_mode || false,
      })
    } else {
      form.resetFields()
    }
    setActivityModalVisible(true)
    window.requestAnimationFrame(() => {
      document.getElementById('performance-activity-editor')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  }

  // 活动列表操作按钮
  const getActionButtons = (record: PerformanceActivity) => {
    const buttons: React.ReactNode[] = []
    const status = record.status

    buttons.push(
      <Button size="small" type="link" onClick={() => loadActivityDetail(record)} key="view">详情</Button>
    )

    if (status === 'draft') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('open-target-setting', record)} key="start">开启目标</Button>
      )
    } else if (status === 'target_setting') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('open-self-evaluation', record)} key="open-self">开启自评</Button>
      )
    } else if (status === 'self_evaluation') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('notify-self-eval', record)} key="notify-self">提醒自评</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('open-manager-evaluation', record)} key="open-mgr">开启主管评分</Button>
      )
    } else if (status === 'manager_evaluation') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('open-employee-confirmation', record)} key="confirm">员工确认</Button>
      )
    } else if (status === 'employee_confirmation') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('open-manager-confirmation', record)} key="manager-confirm">主管确认</Button>
      )
    } else if (status === 'manager_confirmation') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('open-hr-confirmation', record)} key="hr-confirm">HR确认</Button>
      )
    } else if (status === 'hr_confirmation') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" danger onClick={() => handleActivityAction('lock', record)} key="lock">锁定</Button>
      )
    } else if (status === 'locked') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('archive', record)} key="archive">归档</Button>
      )
    } else if (status === 'result_confirmed') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('archive', record)} key="archive">归档</Button>
      )
    }

    return buttons
  }

  // 活动列表 columns
  const activityColumns: ColumnsType<PerformanceActivity> = [
    { title: '活动名称', dataIndex: 'name', key: 'name', width: 180, ellipsis: true },
    { title: '周期', dataIndex: 'cycle_type', key: 'cycle_type', width: 80, render: (v: string) => getCycleLabel(v) },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (status: string) => {
        const s = STATUS_MAP[status] || { label: status, color: 'default' }
        return <StatusTag color={s.color}>{s.label}</StatusTag>
      }
    },
    { title: '自评时间', key: 'self_eval', width: 200, render: (_, r) => `${formatDateTime(r.self_eval_start_at)} ~ ${formatDateTime(r.self_eval_end_at)}` },
    { title: '主管评分时间', key: 'mgr_eval', width: 200, render: (_, r) => `${formatDateTime(r.manager_eval_start_at)} ~ ${formatDateTime(r.manager_eval_end_at)}` },
    { title: '操作', key: 'actions', fixed: 'right', width: 220, render: (_, record) => (
      <Space size={2} wrap>{getActionButtons(record)}</Space>
    )},
  ]

  // 参与人 columns
  const participantColumns: ColumnsType<PerformanceParticipant> = [
    { title: '员工', dataIndex: 'employee_name', key: 'employee_name', width: 80 },
    { title: '部门', dataIndex: 'department_name', key: 'department_name', width: 110, ellipsis: true },
    { title: '岗位', dataIndex: 'position', key: 'position', width: 90, ellipsis: true },
    {
      title: '直属主管', dataIndex: 'manager_name', key: 'manager_name', width: 90,
      render: (name: string, record: PerformanceParticipant) => {
        if (!name && (record.manager_id === null || record.manager_id === undefined || record.manager_id === '')) {
          return (
            <Tooltip title="该员工未设置直属主管，无法进入绩效流程">
              <StatusTag color="error" style={{ cursor: 'default' }}>未设置</StatusTag>
            </Tooltip>
          )
        }
        return name || '-'
      }
    },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 110,
      render: (status: string) => {
        const s = PARTICIPANT_STATUS_MAP[status] || { label: status, color: 'default' }
        return <StatusTag color={s.color}>{s.label}</StatusTag>
      }
    },
    {
      title: '自评分', dataIndex: 'self_score', key: 'self_score', width: 70,
      render: (score: any) => {
        if (score === null || score === undefined || score === '') return <Text type="secondary">-</Text>
        const text = String(score)
        const match = text.match(/^(\d+(?:\.\d+)?)(.*)$/)
        if (!match) return <Text>{score}</Text>
        const num = match[1]
        const suffix = match[2]
        return (
          <Tooltip title={suffix ? `分数 ${num}（${suffix.trim()}）` : undefined}>
            <span>
              <Text strong>{num}</Text>
              {suffix && <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)', marginLeft: 2 }}>{suffix}</Text>}
            </span>
          </Tooltip>
        )
      }
    },
    {
      title: '主管分', dataIndex: 'manager_score', key: 'manager_score', width: 70,
      render: (score: any) => {
        if (score === null || score === undefined || score === '') return <Text type="secondary">-</Text>
        return <Text strong>{String(score)}</Text>
      }
    },
    {
      title: '等级', dataIndex: 'final_level', key: 'final_level', width: 50,
      render: (v: string) => {
        if (!v) return <Text type="secondary">-</Text>
        const colorMap: Record<string, string> = { S: '#f50', A: '#1677ff', B: '#52c41a', C: '#faad14', D: '#ff4d4f' }
        return <StatusTag color={colorMap[v] || 'default'}>{v}</StatusTag>
      }
    },
    {
      title: '操作', key: 'actions', fixed: 'right', width: 150,
      render: (_, record: PerformanceParticipant) => {
        const activityId = currentActivity?.id
        const isArchived = ['archived', 'locked'].includes(currentActivity?.status || '')
        if (!activityId) return null

        if (isArchived) {
          return (
            <Button size="small" type="link" style={{ fontSize: 'var(--font-size-sm)' }}
              onClick={() => navigate(`/performance-result/${activityId}/${record.id}`)}
            >查看</Button>
          )
        }

        const links: React.ReactNode[] = []
        const linkStyle = { fontSize: 'var(--font-size-sm)', padding: '0 2px' }
        const activityStatus = currentActivity?.status

        // 目标设定：活动必须处于 target_setting 状态，且参与人状态允许
        if (activityStatus === 'target_setting' && ['pending', 'target_pending_approval', 'target_rejected', 'target_set'].includes(record.status)) {
          links.push(
            <Button key="target" size="small" type="link" style={linkStyle}
              onClick={() => navigate(`/performance-goal-setting/${activityId}/${record.id}`)}
            >目标</Button>
          )
        }
        // 自评：活动必须处于 self_evaluation 状态，且参与人状态允许
        if (activityStatus === 'self_evaluation' && ['target_set', 'self_submitted'].includes(record.status)) {
          links.push(
            <Button key="self" size="small" type="link" style={linkStyle}
              onClick={() => navigate(`/performance-self-eval/${activityId}/${record.id}`)}
            >自评</Button>
          )
        }
        // 主管评分：活动必须处于 manager_evaluation 状态，且参与人状态允许
        if (activityStatus === 'manager_evaluation' && ['self_submitted', 'manager_submitted'].includes(record.status)) {
          links.push(
            <Button key="mgr" size="small" type="link" style={linkStyle}
              onClick={() => navigate(`/performance-manager-eval/${activityId}/${record.id}`)}
            >评分</Button>
          )
        }
        if (['manager_submitted', 'employee_confirmed', 'manager_confirmed', 'hr_confirmed', 'locked', 'result_confirmed'].includes(record.status)) {
          links.push(
            <Button key="result" size="small" type="link" style={linkStyle}
              onClick={() => navigate(`/performance-result/${activityId}/${record.id}`)}
            >结果</Button>
          )
        }
        if (currentActivity?.status === 'hr_confirmation' && record.status === 'manager_confirmed') {
          links.push(
            <Button key="hr-confirm" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-primary)' }}
              onClick={async () => {
                try {
                  await performanceAPI.confirmHRResult(record.id)
                  message.success('HR确认成功')
                  if (currentActivity) loadActivityDetail(currentActivity)
                } catch (err: any) {
                  message.error(err?.response?.data?.message || 'HR确认失败')
                }
              }}
            >HR确认</Button>
          )
        }
        if (record.status === 'target_pending_approval') {
          links.push(
            <Button key="approve" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-info)' }}
              onClick={async () => {
                try {
                  await performanceAPI.approveGoalRecords(record.id)
                  message.success('目标已通过')
                  if (currentActivity) loadActivityDetail(currentActivity)
                } catch (err: any) {
                  message.error(err?.response?.data?.message || '审批失败')
                }
              }}
            >通过</Button>
          )
          links.push(
            <Button key="reject" size="small" type="link" danger style={linkStyle}
              onClick={() => handleRejectGoalRecords(record)}
            >驳回</Button>
          )
        }

        return <Space size={0}>{links}</Space>
      }
    },
  ]

  // 统计数据
  const inProgressCount = activities.filter(a => ['target_setting', 'self_evaluation', 'manager_evaluation', 'employee_confirmation', 'manager_confirmation', 'hr_confirmation'].includes(a.status)).length
  const confirmedCount = activities.filter(a => ['locked', 'result_confirmed'].includes(a.status)).length

  return (
    <PageContainer
      title="绩效管理"
      icon={<BarChartOutlined />}
      subtitle="绩效活动管理与评分工作台"
    >

      <Card
        style={{ borderRadius: 'var(--radius-xl)', border: '1px solid var(--color-border)', boxShadow: 'var(--shadow-card)' }}
        styles={{ header: { background: 'var(--color-bg-card-header)', borderBottom: '1px solid var(--color-border-light)' } }}
      >
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            {[
              { title: '绩效活动总数', value: activitiesTotal, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)' },
              { title: '进行中活动', value: inProgressCount, color: '#0369a1', bg: '#e0f2fe' },
              { title: '已确认结果', value: confirmedCount, color: 'var(--color-success)', bg: '#dcfce7' },
              { title: '已归档活动', value: activities.filter(a => a.status === 'archived').length, color: 'var(--color-text-secondary)', bg: 'var(--color-bg-hover)' },
            ].map((item) => (
              <Col xs={24} sm={12} lg={6} key={item.title}>
                <div style={{
                  background: 'var(--color-bg-card)',
                  borderRadius: 'var(--radius-md)',
                  padding: '18px 20px',
                  boxShadow: 'var(--shadow-card)',
                  border: '1px solid var(--color-border)',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 14,
                }}>
                  <div style={{
                    width: 44, height: 44, borderRadius: 'var(--radius-md)', background: item.bg,
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    fontSize: 22, color: item.color, fontWeight: 'var(--font-weight-bold)', flexShrink: 0,
                  }}>
                    {item.value}
                  </div>
                  <Text style={{ color: 'var(--color-text)', fontSize: 'var(--font-size-sm)', fontWeight: 'var(--font-weight-medium)' }}>{item.title}</Text>
                </div>
              </Col>
            ))}
          </Row>

          <PerformanceActivityEditor
            visible={activityModalVisible}
            editing={Boolean(editingActivity)}
            form={form}
            saving={activitySaving}
            indicatorLibraries={indicatorLibraries}
            indicatorLibrariesLoading={indicatorLibrariesLoading}
            departmentOptions={departments.flatMap(department => {
              const option = getDepartmentOption(department)
              return option ? [option] : []
            })}
            userOptions={users.flatMap(user => {
              const option = getUserOption(user)
              return option ? [option] : []
            })}
            scopeOptionsLoading={scopeOptionsLoading}
            onSave={handleSaveActivity}
            onCancel={closeActivityEditor}
          />

          {/* 活动列表 */}
          <PageCard
            title="绩效活动"
            style={{ marginBottom: 16 }}
            extra={
              <Space>
                <Button type="primary" icon={<PlusOutlined />} onClick={() => openActivityModal()}>新建活动</Button>
                <Button onClick={() => loadActivities()} disabled={activitiesLoading}>刷新</Button>
              </Space>
            }
          >
            <Space style={{ marginBottom: 16 }} wrap>
              <Input.Search
                placeholder="搜索活动名称"
                allowClear
                onSearch={setActivitySearchText}
                onChange={e => { if (!e.target.value) setActivitySearchText('') }}
                style={{ width: 220 }}
              />
              <Select
                placeholder="筛选状态"
                allowClear
                style={{ width: 140 }}
                value={activityStatusFilter}
                onChange={setActivityStatusFilter}
                options={STATUS_OPTIONS}
              />
            </Space>
            <Spin spinning={activitiesLoading}>
              <Table
                columns={activityColumns}
                dataSource={activities.filter(item => {
                  const matchName = !activitySearchText || item.name?.toLowerCase().includes(activitySearchText.toLowerCase())
                  const matchStatus = !activityStatusFilter || item.status === activityStatusFilter
                  return matchName && matchStatus
                })}
                rowKey="id"
                pagination={{ pageSize: 10 }}
                size="small"
                scroll={{ x: 900 }}
              />
            </Spin>
          </PageCard>

      </Card>

      {/* 活动详情抽屉 */}
      <Drawer
        title={`活动详情：${currentActivity?.name || ''}`}
        placement="right"
        width={1000}
        open={detailDrawerVisible}
        onClose={() => { setDetailDrawerVisible(false); setCurrentActivity(null); setParticipants([]); setSummary(null); setDistributionCheck(null); setDistributionRules([]); setHrDeadlineStatus(null); }}
        styles={{ footer: { paddingTop: 12 } }}
      >
        {currentActivity && (
          <>
            <Steps
              current={getActivityStepIndex(currentActivity.status)}
              items={ACTIVITY_FLOW.map(item => ({
                title: item.label,
                status: item.status === currentActivity.status ? 'process'
                  : getActivityStepIndex(currentActivity.status) > ACTIVITY_FLOW.findIndex(f => f.status === item.status) ? 'finish' : 'wait'
              }))}
              style={{ marginBottom: 20 }}
              size="small"
            />
            <Descriptions column={3} size="small" style={{ marginBottom: 16 }} bordered>
              <Descriptions.Item label="状态">
                <StatusTag color={STATUS_MAP[currentActivity.status]?.color}>{STATUS_MAP[currentActivity.status]?.label}</StatusTag>
              </Descriptions.Item>
              <Descriptions.Item label="周期类型">{getCycleLabel(currentActivity.cycle_type)}</Descriptions.Item>
              <Descriptions.Item label="绩效周期">{formatDateTime(currentActivity.start_date)} ~ {formatDateTime(currentActivity.end_date)}</Descriptions.Item>
              <Descriptions.Item label="自评时间">{formatDateTime(currentActivity.self_eval_start_at)} ~ {formatDateTime(currentActivity.self_eval_end_at)}</Descriptions.Item>
              <Descriptions.Item label="主管评分">{formatDateTime(currentActivity.manager_eval_start_at)} ~ {formatDateTime(currentActivity.manager_eval_end_at)}</Descriptions.Item>
              <Descriptions.Item label="结果确认">{formatDateTime(currentActivity.result_confirm_start_at)} ~ {formatDateTime(currentActivity.result_confirm_end_at)}</Descriptions.Item>
            </Descriptions>

            {/* 操作按钮 - 紧凑布局 */}
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 16 }}>
              {currentActivity.status === 'draft' && (
                <>
                  <Button type="primary" size="small" onClick={() => handleActivityAction('open-target-setting', currentActivity)}>开启目标设定</Button>
                  <Button size="small" onClick={() => handleActivityAction('publish', currentActivity)}>直接开启自评</Button>
                </>
              )}
              {currentActivity.status === 'target_setting' && (
                <Button type="primary" size="small" onClick={() => handleActivityAction('open-self-evaluation', currentActivity)}>开启自评</Button>
              )}
              {currentActivity.status === 'self_evaluation' && (
                <>
                  <Button type="primary" size="small" onClick={() => handleActivityAction('open-manager-evaluation', currentActivity)}>开启主管评分</Button>
                  <Button size="small" onClick={async () => { try { await performanceAPI.sendSelfEvalReminder(currentActivity.id); message.success('已发送自评提醒') } catch (err: any) { message.error(err?.response?.data?.message || '发送提醒失败') } }}>提醒自评</Button>
                </>
              )}
              {currentActivity.status === 'manager_evaluation' && (
                <>
                  <Button type="primary" size="small" onClick={() => handleActivityAction('open-employee-confirmation', currentActivity)}>开启员工确认</Button>
                  <Button size="small" onClick={() => setDistributionModalVisible(true)}>强制分布</Button>
                  <Button size="small" onClick={() => { const selectable = participants.filter(p => p.status === 'self_submitted' || p.status === 'manager_submitted'); setBatchEvalSelected(selectable.map(p => p.id)); setBatchEvalModalVisible(true) }}>批量评分</Button>
                  <Button size="small" onClick={async () => { try { await performanceAPI.sendManagerEvalReminder(currentActivity.id); message.success('已发送评分提醒') } catch (err: any) { message.error(err?.response?.data?.message || '发送提醒失败') } }}>提醒评分</Button>
                </>
              )}
              {currentActivity.status === 'employee_confirmation' && (
                <Button type="primary" size="small" onClick={() => handleActivityAction('open-manager-confirmation', currentActivity)}>开启主管确认</Button>
              )}
              {currentActivity.status === 'manager_confirmation' && (
                <Button type="primary" size="small" onClick={() => handleActivityAction('open-hr-confirmation', currentActivity)}>开启HR确认</Button>
              )}
              {currentActivity.status === 'hr_confirmation' && (
                <>
                  <Button size="small" onClick={async () => { try { await performanceAPI.sendHRConfirmReminder(currentActivity.id); message.success('已发送HR确认提醒') } catch (err: any) { message.error(err?.response?.data?.message || '发送提醒失败') } }}>提醒HR确认</Button>
                  {hrDeadlineStatus?.can_force_lock && (
                    <Button danger size="small" onClick={() => handleForceLockOverdueHR(currentActivity)}>逾期强制锁定</Button>
                  )}
                  <Button type="primary" danger size="small" onClick={() => handleActivityAction('lock', currentActivity)}>锁定活动</Button>
                </>
              )}
              {currentActivity.status === 'locked' && (
                <Button size="small" onClick={() => handleActivityAction('archive', currentActivity)}>归档活动</Button>
              )}
              {currentActivity.status === 'result_confirmed' && (
                <Button size="small" onClick={() => handleActivityAction('archive', currentActivity)}>归档活动</Button>
              )}
              {['draft', 'target_setting', 'self_evaluation', 'manager_evaluation'].includes(currentActivity.status) && (
                <Button size="small" onClick={() => handleActivityAction('refresh', currentActivity)}>刷新参与人</Button>
              )}
            </div>

            <Divider style={{ margin: '8px 0 10px' }} orientationMargin={0}>统计摘要</Divider>
            {currentActivity.status === 'hr_confirmation' && hrDeadlineStatus && (
              <Alert
                type={hrDeadlineStatus.overdue ? 'warning' : 'info'}
                showIcon
                style={{ marginBottom: 12 }}
                message={`HR确认截止：${hrDeadlineStatus.deadline || '未设置'}，待确认 ${hrDeadlineStatus.pending_count || 0} 人${hrDeadlineStatus.overdue ? '，已逾期' : ''}`}
              />
            )}

            <Spin spinning={summaryLoading}>
              {summary ? (
                <div style={{ display: 'flex', gap: 0, marginBottom: 10, borderRadius: 'var(--radius-md)', border: '1px solid var(--color-border)', overflow: 'hidden' }}>
                  {[
                    { title: '参与人数', value: summary.total_participants, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)' },
                    { title: '已自评', value: summary.self_submitted_count, color: '#0369a1', bg: '#e0f2fe' },
                    { title: '已评分', value: summary.manager_submitted_count, color: '#b45309', bg: '#fef3c7' },
                    { title: '已确认', value: summary.result_confirmed_count, color: 'var(--color-success)', bg: '#dcfce7' },
                  ].map((item, idx) => (
                    <div key={item.title} style={{
                      flex: 1, padding: '10px 14px', textAlign: 'center',
                      background: item.bg, borderRight: idx < 3 ? '1px solid var(--color-border)' : 'none',
                    }}>
                      <div style={{ fontSize: 22, fontWeight: 'var(--font-weight-bold)', color: item.color, lineHeight: 1.2 }}>{item.value}</div>
                      <div style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-secondary)', marginTop: 2 }}>{item.title}</div>
                    </div>
                  ))}
                </div>
              ) : <Text type="secondary">暂无数据</Text>}
            </Spin>

            {distributionCheck && (
              <Card size="small" style={{ marginBottom: 10 }}>
                <Row gutter={[6, 6]}>
                  {['S', 'A', 'B', 'C', 'D'].map(level => {
                    const dist = distributionCheck.distribution?.[level]
                    if (!dist) return null
                    const statusColor = dist.status === 'exceeded' ? 'exception' : dist.status === 'warning' ? 'normal' : 'success'
                    const bg = dist.status === 'exceeded' ? '#fff2f0' : dist.status === 'warning' ? '#fffbe6' : '#f6ffed'
                    const barColor = dist.status === 'exceeded' ? '#ff4d4f' : dist.status === 'warning' ? '#faad14' : '#52c41a'
                    return (
                      <Col span={4} key={level} style={{ minWidth: 0 }}>
                        <div style={{
                          textAlign: 'center', padding: '8px 4px', borderRadius: 'var(--radius-md)',
                          background: bg, border: `1px solid ${barColor}20`,
                        }}>
                          <div style={{
                            fontSize: 18, fontWeight: 'var(--font-weight-bold)', color: barColor, lineHeight: 1,
                          }}>{level}</div>
                          <div style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text)', margin: '4px 0 2px' }}>
                            {dist.actual_count}/{dist.expected_count}人
                          </div>
                          <div style={{
                            height: 4, borderRadius: 2, background: 'var(--color-border)',
                            overflow: 'hidden', margin: '0 8px',
                          }}>
                            <div style={{
                              height: '100%', borderRadius: 2, background: barColor,
                              width: `${Math.min(dist.progress, 100)}%`,
                            }} />
                          </div>
                          <div style={{ fontSize: 10, color: 'var(--color-text-tertiary)', marginTop: 3 }}>
                            期望 {dist.expected_percent}%
                          </div>
                        </div>
                      </Col>
                    )
                  })}
                </Row>
                {!distributionCheck.passed && distributionCheck.warnings?.length > 0 && (
                  <Alert
                    type="warning"
                    showIcon
                    message="配额超限"
                    description={distributionCheck.warnings.join('；')}
                    style={{ marginTop: 6 }}
                    closable
                  />
                )}
              </Card>
            )}

            <Divider style={{ margin: '12px 0' }}>参与人列表</Divider>
            <Spin spinning={participantsLoading}>
              <Table
                columns={participantColumns}
                dataSource={participants}
                rowKey="id"
                pagination={{ pageSize: 10, size: 'small' }}
                size="small"
                scroll={{ x: 900 }}
              />
            </Spin>
          </>
        )}
      </Drawer>

      {/* 强制分布规则弹窗 */}
      <Modal
        title="强制分布规则"
        open={distributionModalVisible}
        onOk={handleSaveDistribution}
        onCancel={() => setDistributionModalVisible(false)}
        width={560}
      >
        <Alert showIcon type="info" style={{ marginBottom: 16 }} message="前端校验比例总和需等于 100%，但以后端校验为准。" />

        {/* 可视化分布预览 */}
        {(() => {
          const formVals = distributionForm.getFieldsValue()
          const levels = ['S', 'A', 'B', 'C', 'D']
          const colors: Record<string, string> = { S: '#f50', A: '#1677ff', B: '#52c41a', C: '#faad14', D: '#ff4d4f' }
          const total = levels.reduce((sum, l) => sum + (Number(formVals[`percent_${l}`]) || 0), 0)
          return (
            <div style={{ marginBottom: 16 }}>
              <Text strong style={{ fontSize: 'var(--font-size-sm)', color: 'var(--color-text)', marginBottom: 8, display: 'block' }}>分布预览</Text>
              <div style={{
                display: 'flex', height: 32, borderRadius: 'var(--radius-sm)', overflow: 'hidden', border: '1px solid var(--color-border)', background: '#f5f5f5'
              }}>
                {levels.map(level => {
                  const val = Number(formVals[`percent_${level}`]) || 0
                  if (val <= 0) return null
                  return (
                    <Tooltip key={level} title={`${level}: ${val}%`}>
                      <div style={{
                        width: `${val}%`,
                        background: colors[level],
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        color: '#fff', fontWeight: 700, fontSize: 13,
                        transition: 'width 0.3s',
                      }}>
                        {val >= 8 ? `${level} ${val}%` : val >= 4 ? level : ''}
                      </div>
                    </Tooltip>
                  )
                })}
              </div>
              <div style={{ marginTop: 4, display: 'flex', justifyContent: 'space-between' }}>
                <Text type={total === 100 ? 'success' : 'danger'} style={{ fontSize: 'var(--font-size-xs)' }}>
                  合计：{total}%{total !== 100 ? '（需等于 100%）' : ' ✓'}
                </Text>
                <div style={{ display: 'flex', gap: 12 }}>
                  {levels.map(level => {
                    const val = Number(formVals[`percent_${level}`]) || 0
                    return val > 0 ? (
                      <Text key={level} style={{ fontSize: 11, color: colors[level] }}>
                        <span style={{ display: 'inline-block', width: 8, height: 8, borderRadius: 'var(--radius-xs)', background: colors[level], marginRight: 3 }} />
                        {level} {val}%
                      </Text>
                    ) : null
                  })}
                </div>
              </div>
            </div>
          )
        })()}

        <Form form={distributionForm} layout="vertical" onValuesChange={() => forceUpdate()}>
          {['S', 'A', 'B', 'C', 'D'].map(level => (
            <Card key={level} size="small" style={{ marginBottom: 8 }}>
              <Space wrap>
                <Text strong style={{ width: 40 }}>等级 {level}：</Text>
                <Form.Item name={`percent_${level}`} label="比例%" initialValue={distributionRules.find(r => r.level === level)?.distribution_percent || 0} style={{ marginBottom: 0 }}>
                  <InputNumber min={0} max={100} />
                </Form.Item>
                <Form.Item name={`desc_${level}`} label="说明" initialValue={distributionRules.find(r => r.level === level)?.description || ''} style={{ marginBottom: 0 }}>
                  <Input placeholder="如：杰出贡献" style={{ width: 120 }} />
                </Form.Item>
              </Space>
            </Card>
          ))}
          <Text type="secondary">示例：S: 10%, A: 20%, B: 40%, C: 20%, D: 10%</Text>
        </Form>
      </Modal>

      {/* 批量主管评分弹窗 */}
      <Modal
        title="批量主管评分"
        open={batchEvalModalVisible}
        onOk={async () => {
          if (!currentActivity || batchEvalSelected.length === 0) return
          try {
            const values = await batchEvalForm.validateFields()
            setBatchEvalLoading(true)
            const score = values.batch_score || 0
            const level = score >= 100 ? 'S' : score >= 90 ? 'A' : score >= 80 ? 'B' : score >= 60 ? 'C' : 'D'
            const evaluations = batchEvalSelected.map(pid => ({
              participant_id: pid,
              manager_score: score,
              suggested_level: level,
              manager_comment: values.batch_comment || '',
            }))
            await performanceAPI.batchSubmitManagerEvaluations(currentActivity.id, evaluations)
            message.success(`已为 ${batchEvalSelected.length} 名员工提交评分`)
            setBatchEvalModalVisible(false)
            batchEvalForm.resetFields()
            setBatchEvalScore(0)
            refreshParticipants(currentActivity.id)
          } catch (err: any) {
            if (err.errorFields) return
            message.error(err?.response?.data?.message || '批量评分失败')
          } finally {
            setBatchEvalLoading(false)
          }
        }}
        onCancel={() => { setBatchEvalModalVisible(false); batchEvalForm.resetFields(); setBatchEvalScore(0) }}
        confirmLoading={batchEvalLoading}
        width={520}
      >
        <Alert type="info" message={`已选择 ${batchEvalSelected.length} 名员工，将统一应用相同的评分和评语`} style={{ marginBottom: 16 }} />
        <Form form={batchEvalForm} layout="vertical" onValuesChange={(_, all) => setBatchEvalScore(all.batch_score || 0)}>
          <Form.Item name="batch_score" label="上级评分" rules={[{ required: true, message: '请输入评分' }]}>
            <InputNumber min={0} max={120} style={{ width: '100%' }} placeholder="0-120" />
          </Form.Item>
          <Form.Item label="绩效等级">
            <StatusTag color={
              batchEvalScore >= 100 ? '#f50' :
              batchEvalScore >= 90 ? '#2db7f5' :
              batchEvalScore >= 80 ? '#87d068' :
              batchEvalScore >= 60 ? '#faad14' : '#ff4d4f'
            } style={{ fontSize: 14, padding: '4px 12px' }}>
              {batchEvalScore >= 100 ? 'S - 杰出' :
               batchEvalScore >= 90 ? 'A - 优秀' :
               batchEvalScore >= 80 ? 'B - 良好' :
               batchEvalScore >= 60 ? 'C - 待改进' : 'D - 不合格'}
            </StatusTag>
            <Text type="secondary" style={{ marginLeft: 8 }}>根据评分自动生成</Text>
          </Form.Item>
          <Form.Item name="batch_comment" label="上级评语">
            <TextArea rows={3} placeholder="请输入统一评语（可选）" />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  )
}

export default PerformanceOverview
