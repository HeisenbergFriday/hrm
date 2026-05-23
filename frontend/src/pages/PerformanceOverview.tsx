import React, { useState, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Alert, Card, Col, Row, Space, Statistic, Table, Tag, Typography, Button, Modal, Form, Input, InputNumber,
  Select, message, Spin, Drawer, Popconfirm, Tooltip, Divider, Descriptions, Progress, Tabs
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { Dayjs } from 'dayjs'
import dayjs from 'dayjs'
import {
  departmentAPI,
  performanceAPI,
  PerformanceActivity,
  PerformanceParticipant,
  PerformanceDistributionRule,
  userAPI,
} from '../services/api'
import PerformanceActivityEditor from '../components/PerformanceActivityEditor'
import { BarChartOutlined, BellOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons'

const { Text, Paragraph } = Typography
const { TextArea } = Input

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
  locked: { label: '已冻结', color: 'error' },
  result_confirmed: { label: '已确认', color: 'success' },
  inactive: { label: '已离职', color: 'error' },
  removed_from_scope: { label: '已移除', color: 'error' },
}

const ACTIVITY_FLOW = [
  { status: 'draft', label: '草稿' },
  { status: 'target_setting', label: '目标设定' },
  { status: 'self_evaluation', label: '员工自评' },
  { status: 'manager_evaluation', label: '主管评分' },
  { status: 'employee_confirmation', label: '员工确认' },
  { status: 'manager_confirmation', label: '主管确认' },
  { status: 'hr_confirmation', label: 'HR确认' },
  { status: 'locked', label: '锁定' },
  { status: 'archived', label: '归档' },
]

function formatDateRange(start?: string, end?: string) {
  if (!start && !end) return '-'
  return `${start || '-'} ~ ${end || '-'}`
}

function getActivityStepIndex(status?: string) {
  const index = ACTIVITY_FLOW.findIndex(item => item.status === status)
  return index >= 0 ? index : 0
}

function getStatusMeta(status?: string) {
  return STATUS_MAP[status || ''] || { label: status || '-', color: 'default' }
}

function getParticipantStatusMeta(status?: string) {
  return PARTICIPANT_STATUS_MAP[status || ''] || { label: status || '-', color: 'default' }
}

const compactButtonStyle: React.CSSProperties = { marginRight: 0 }

const PerformanceOverview: React.FC = () => {
  const navigate = useNavigate()
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

  // 指标库管理
  const [indicatorLibraries, setIndicatorLibraries] = useState<any[]>([])
  const [indicatorLibrariesLoading, setIndicatorLibrariesLoading] = useState(false)
  const [indicatorLibrariesTotal, setIndicatorLibrariesTotal] = useState(0)
  const [indicatorLibraryModalVisible, setIndicatorLibraryModalVisible] = useState(false)
  const [editingIndicatorLibrary, setEditingIndicatorLibrary] = useState<any | null>(null)
  const [indicatorLibraryForm] = Form.useForm()

  // 指标项管理
  const [indicatorItems, setIndicatorItems] = useState<any[]>([])
  const [indicatorItemsLoading, setIndicatorItemsLoading] = useState(false)
  const [currentIndicatorLibrary, setCurrentIndicatorLibrary] = useState<any | null>(null)
  const [indicatorItemModalVisible, setIndicatorItemModalVisible] = useState(false)
  const [editingIndicatorItem, setEditingIndicatorItem] = useState<any | null>(null)
  const [indicatorItemForm] = Form.useForm()

  // 当前 Tab
  const [activeTab, setActiveTab] = useState('activities')

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

  // 评分弹窗
  // 强制分布弹窗
  const [distributionModalVisible, setDistributionModalVisible] = useState(false)
  const [distributionForm] = Form.useForm()

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

  // 加载指标库列表
  const loadIndicatorLibraries = useCallback(async () => {
    setIndicatorLibrariesLoading(true)
    try {
      const res: any = await performanceAPI.getIndicatorLibraries({ page: 1, page_size: 100 })
      const data = res.data || res
      setIndicatorLibraries(data.items || [])
      setIndicatorLibrariesTotal(data.total || 0)
    } catch (err: any) {
      message.error(err?.response?.data?.message || '加载指标库列表失败')
    } finally {
      setIndicatorLibrariesLoading(false)
    }
  }, [])

  // 加载指标项列表
  const loadIndicatorItems = useCallback(async (libraryId: number) => {
    setIndicatorItemsLoading(true)
    try {
      const res: any = await performanceAPI.getIndicatorItems(libraryId)
      const data = res.data || res
      setIndicatorItems(data.items || [])
    } catch (err: any) {
      message.error(err?.response?.data?.message || '加载指标项列表失败')
    } finally {
      setIndicatorItemsLoading(false)
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

  // Tab 切换时加载数据
  React.useEffect(() => {
    if (activeTab === 'activities') {
      loadActivities()
    } else if (activeTab === 'indicatorLibraries') {
      loadIndicatorLibraries()
    }
  }, [activeTab, loadActivities, loadIndicatorLibraries])

  // 指标库需要部门选项时加载
  React.useEffect(() => {
    if (activeTab === 'indicatorLibraries' && departments.length === 0) {
      departmentAPI.getDepartments().then(res => {
        setDepartments(getListFromResponse(res, ['departments', 'items']))
      }).catch(() => {})
    }
  }, [activeTab, departments.length])

  // 加载活动详情
  const loadActivityDetail = async (activity: PerformanceActivity) => {
    setCurrentActivity(activity)
    setDetailDrawerVisible(true)
    setParticipantsLoading(true)
    setSummaryLoading(true)
    setDistributionCheckLoading(true)

    // 使用 Promise.allSettled 避免单个接口失败阻塞整个流程
    const results = await Promise.allSettled([
      performanceAPI.getParticipants(activity.id, { page: 1, page_size: 200 }),
      performanceAPI.getResultSummary(activity.id),
      performanceAPI.getDistributionCheck(activity.id),
      performanceAPI.getDistributionRules(activity.id),
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
      })
    } else {
      form.resetFields()
    }
    setActivityModalVisible(true)
    window.requestAnimationFrame(() => {
      document.getElementById('performance-activity-editor')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  }

  // 打开指标库表单
  const openIndicatorLibraryModal = (library?: any) => {
    setEditingIndicatorLibrary(library || null)
    // 确保部门选项已加载
    if (departments.length === 0) {
      departmentAPI.getDepartments().then(res => {
        setDepartments(getListFromResponse(res, ['departments', 'items']))
      }).catch(() => {})
    }
    if (library) {
      indicatorLibraryForm.setFieldsValue({
        name: library.name,
        description: library.description,
        department_id: library.department_id,
        department_name: library.department_name,
        default_cycle: library.default_cycle,
      })
    } else {
      indicatorLibraryForm.resetFields()
    }
    setIndicatorLibraryModalVisible(true)
  }

  // 保存指标库
  const handleSaveIndicatorLibrary = async () => {
    try {
      const values = await indicatorLibraryForm.validateFields()
      // 自动填充部门名称
      if (values.department_id && !values.department_name) {
        const dept = departments.find(d => String(d.department_id || d.id) === String(values.department_id))
        if (dept) values.department_name = dept.name || dept.department_name || ''
      }
      if (editingIndicatorLibrary) {
        await performanceAPI.updateIndicatorLibrary(editingIndicatorLibrary.id, values)
        message.success('指标库更新成功')
      } else {
        await performanceAPI.createIndicatorLibrary(values)
        message.success('指标库创建成功')
      }
      setIndicatorLibraryModalVisible(false)
      loadIndicatorLibraries()
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '保存失败')
    }
  }

  // 打开指标项表单
  const openIndicatorItemModal = (item?: any, libraryId?: number) => {
    setEditingIndicatorItem(item || null)
    if (item) {
      indicatorItemForm.setFieldsValue({
        name: item.name,
        description: item.description,
        section_type: item.section_type,
        weight: item.weight,
        is_default: item.is_default,
        sort_order: item.sort_order,
      })
    } else {
      indicatorItemForm.resetFields()
      indicatorItemForm.setFieldsValue({
        library_id: libraryId,
        section_type: 'quantitative',
        weight: 0,
        is_default: false,
        sort_order: 0,
      })
    }
    setIndicatorItemModalVisible(true)
  }

  // 保存指标项
  const handleSaveIndicatorItem = async () => {
    try {
      const values = await indicatorItemForm.validateFields()
      if (editingIndicatorItem) {
        await performanceAPI.updateIndicatorItem(editingIndicatorItem.id, values)
        message.success('指标项更新成功')
      } else {
        await performanceAPI.createIndicatorItem({
          library_id: currentIndicatorLibrary?.id,
          ...values,
        })
        message.success('指标项创建成功')
      }
      setIndicatorItemModalVisible(false)
      if (currentIndicatorLibrary) {
        loadIndicatorItems(currentIndicatorLibrary.id)
      }
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '保存失败')
    }
  }

  // 删除指标项
  const handleDeleteIndicatorItem = async (itemId: number) => {
    try {
      await performanceAPI.deleteIndicatorItem(itemId)
      message.success('删除成功')
      if (currentIndicatorLibrary) {
        loadIndicatorItems(currentIndicatorLibrary.id)
      }
    } catch (err: any) {
      message.error(err?.response?.data?.message || '删除失败')
    }
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
        <Button size="small" type="link" onClick={() => handleActivityAction('open-self-evaluation', record)} key="notify-self">提醒自评</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('open-manager-evaluation', record)} key="open-mgr">开启主管评分</Button>
      )
    } else if (status === 'manager_evaluation') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('open-employee-confirmation', record)} key="confirm">员工确认</Button>
      )
    } else if (status === 'employee_confirmation') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('open-manager-confirmation', record)} key="manager-confirm">主管确认</Button>
      )
    } else if (status === 'manager_confirmation') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('open-hr-confirmation', record)} key="hr-confirm">HR确认</Button>
      )
    } else if (status === 'hr_confirmation') {
      buttons.push(
        <Button size="small" type="link" danger onClick={() => handleActivityAction('lock', record)} key="lock">锁定</Button>
      )
    } else if (status === 'locked') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('archive', record)} key="archive">归档</Button>
      )
    } else if (status === 'result_confirmed') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('archive', record)} key="archive">归档</Button>
      )
    }

    return buttons
  }

  // 活动列表 columns
  const activityColumns: ColumnsType<PerformanceActivity> = [
    { title: '活动名称', dataIndex: 'name', key: 'name', width: 180, ellipsis: true },
    { title: '周期', dataIndex: 'cycle_type', key: 'cycle_type', width: 80 },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (status: string) => {
        const s = STATUS_MAP[status] || { label: status, color: 'default' }
        return <Tag color={s.color}>{s.label}</Tag>
      }
    },
    { title: '自评时间', key: 'self_eval', width: 200, render: (_, r) => `${r.self_eval_start_at} ~ ${r.self_eval_end_at}` },
    { title: '主管评分时间', key: 'mgr_eval', width: 200, render: (_, r) => `${r.manager_eval_start_at} ~ ${r.manager_eval_end_at}` },
    { title: '操作', key: 'actions', fixed: 'right', width: 180, render: (_, record) => (
      <Space size={4}>{getActionButtons(record)}</Space>
    )},
  ]

  // 参与人 columns
  const participantColumns: ColumnsType<PerformanceParticipant> = [
    { title: '员工', dataIndex: 'employee_name', key: 'employee_name', width: 100 },
    { title: '部门', dataIndex: 'department_name', key: 'department_name', width: 120, ellipsis: true },
    { title: '岗位', dataIndex: 'position', key: 'position', width: 100, ellipsis: true },
    { title: '直属主管', dataIndex: 'manager_name', key: 'manager_name', width: 100 },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (status: string, record: PerformanceParticipant) => {
        const s = PARTICIPANT_STATUS_MAP[status] || { label: status, color: 'default' }
        return (
          <Space size={4}>
            <Tag color={s.color}>{s.label}</Tag>
            {record.manager_id === null || record.manager_id === undefined || record.manager_id === '' ? (
              <Tooltip title="缺少主管">
                <Tag color="error">无主管</Tag>
              </Tooltip>
            ) : null}
          </Space>
        )
      }
    },
    { title: '自评分', dataIndex: 'self_score', key: 'self_score', width: 70 },
    { title: '主管分', dataIndex: 'manager_score', key: 'manager_score', width: 70 },
    { title: '等级', dataIndex: 'final_level', key: 'final_level', width: 60, render: (v: string) => v || '-' },
    {
      title: '操作', key: 'actions', fixed: 'right', width: 160,
      render: (_, record: PerformanceParticipant) => {
        const activityId = currentActivity?.id
        const isArchived = ['archived', 'locked'].includes(currentActivity?.status || '')
        const canTarget = ['pending', 'target_pending_approval', 'target_rejected', 'target_set'].includes(record.status)
        const canSelfEval = ['target_set', 'self_submitted'].includes(record.status)
        const canMgrEval = ['self_submitted', 'manager_submitted'].includes(record.status)
        const canResultView = ['manager_submitted', 'employee_confirmed', 'manager_confirmed', 'hr_confirmed', 'locked', 'result_confirmed'].includes(record.status)

        return (
          <Space size={4}>
            <Button
              size="small" type="link" disabled={isArchived || !canTarget || !activityId}
              onClick={() => activityId && navigate(`/performance-goal-setting/${activityId}/${record.id}`)}
            >目标</Button>
            <Button
              size="small" type="link" disabled={isArchived || !canSelfEval || !activityId}
              onClick={() => activityId && navigate(`/performance-self-eval/${activityId}/${record.id}`)}
            >自评</Button>
            <Button
              size="small" type="link" disabled={isArchived || !canMgrEval || !activityId}
              onClick={() => activityId && navigate(`/performance-manager-eval/${activityId}/${record.id}`)}
            >评分</Button>
            <Button
              size="small" type="link" disabled={isArchived || !canResultView || !activityId}
              onClick={() => activityId && navigate(`/performance-result/${activityId}/${record.id}`)}
            >结果</Button>
            {record.status === 'target_pending_approval' && activityId && (
              <Button
                size="small"
                type="link"
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
            )}
          </Space>
        )
      }
    },
  ]

  // 统计数据
  const inProgressCount = activities.filter(a => ['target_setting', 'self_evaluation', 'manager_evaluation', 'employee_confirmation', 'manager_confirmation', 'hr_confirmation'].includes(a.status)).length
  const confirmedCount = activities.filter(a => ['locked', 'result_confirmed'].includes(a.status)).length

  return (
    <div style={{ padding: '20px 28px', background: '#e4e8ee', minHeight: '100vh' }}>
      <div style={{ marginBottom: 20 }}>
        <h2 style={{ margin: '0 0 4px', fontSize: 22, fontWeight: 700, color: '#111827' }}>
          <BarChartOutlined style={{ marginRight: 10, color: '#4338ca' }} />
          绩效管理
        </h2>
        <Paragraph type="secondary" style={{ marginBottom: 0, color: '#6b7280', fontSize: 13.5 }}>
          绩效活动管理与评分工作台
        </Paragraph>
      </div>

      <Card
        style={{ borderRadius: 14, border: '1px solid #e5e7eb', boxShadow: '0 2px 10px rgba(0,0,0,0.05)' }}
        styles={{ header: { background: '#fafbfc', borderBottom: '1px solid #f0f0f0' } }}
      >
      <Tabs activeKey={activeTab} onChange={setActiveTab}>
        <Tabs.TabPane tab="绩效活动" key="activities">
          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            {[
              { title: '绩效活动总数', value: activitiesTotal, color: '#4338ca', bg: '#eef2ff' },
              { title: '进行中活动', value: inProgressCount, color: '#0369a1', bg: '#e0f2fe' },
              { title: '已确认结果', value: confirmedCount, color: '#15803d', bg: '#dcfce7' },
              { title: '已归档活动', value: activities.filter(a => a.status === 'archived').length, color: '#6b7280', bg: '#f3f4f6' },
            ].map((item) => (
              <Col xs={24} sm={12} lg={6} key={item.title}>
                <div style={{
                  background: '#fff',
                  borderRadius: 14,
                  padding: '20px 22px',
                  boxShadow: '0 2px 10px rgba(0,0,0,0.05)',
                  border: '1px solid #e5e7eb',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 14,
                }}>
                  <div style={{
                    width: 48, height: 48, borderRadius: 12, background: item.bg,
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    fontSize: 20, color: item.color, fontWeight: 700, flexShrink: 0,
                  }}>
                    {item.value}
                  </div>
                  <Text style={{ color: '#6b7280', fontSize: 13, fontWeight: 500 }}>{item.title}</Text>
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
          <Card
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
          </Card>
        </Tabs.TabPane>

        <Tabs.TabPane tab="指标库管理" key="indicatorLibraries">
          <Card
            title="指标库列表"
            extra={
              <Space>
                <Button type="primary" onClick={() => openIndicatorLibraryModal()}>新建指标库</Button>
                <Button onClick={() => loadIndicatorLibraries()} disabled={indicatorLibrariesLoading}>刷新</Button>
              </Space>
            }
          >
            <Spin spinning={indicatorLibrariesLoading}>
              <Table
                dataSource={indicatorLibraries}
                rowKey="id"
                size="small"
                columns={[
                  { title: '指标库名称', dataIndex: 'name', key: 'name' },
                  { title: '所属部门', dataIndex: 'department_name', key: 'department_name' },
                  { title: '默认周期', dataIndex: 'default_cycle', key: 'default_cycle' },
                  { title: '状态', dataIndex: 'status', key: 'status', render: (status: string) => <Tag color={status === 'active' ? 'green' : 'default'}>{status === 'active' ? '启用' : '归档'}</Tag> },
                  {
                    title: '操作', key: 'actions', render: (_, record) => (
                      <Space size="small">
                        <Button size="small" onClick={() => { setCurrentIndicatorLibrary(record); loadIndicatorItems(record.id); }}>管理指标项</Button>
                        <Button size="small" onClick={() => openIndicatorLibraryModal(record)}>编辑</Button>
                        <Popconfirm title="确定归档此指标库？" onConfirm={async () => { await performanceAPI.archiveIndicatorLibrary(record.id); loadIndicatorLibraries(); }}>
                          <Button size="small" danger>归档</Button>
                        </Popconfirm>
                      </Space>
                    )
                  }
                ]}
                pagination={{ pageSize: 10, total: indicatorLibrariesTotal }}
              />
            </Spin>
          </Card>

          {/* 指标项管理 */}
          {currentIndicatorLibrary && (
            <Card
              title={`${currentIndicatorLibrary.name} - 指标项`}
              style={{ marginTop: 16 }}
              extra={
                <Space>
                  <Button type="primary" onClick={() => openIndicatorItemModal(undefined, currentIndicatorLibrary.id)}>添加指标项</Button>
                  <Button onClick={() => setCurrentIndicatorLibrary(null)}>关闭</Button>
                </Space>
              }
            >
              <Spin spinning={indicatorItemsLoading}>
                <Table
                  dataSource={indicatorItems}
                  rowKey="id"
                  size="small"
                  columns={[
                    { title: '指标名称', dataIndex: 'name', key: 'name' },
                    { title: '类型', dataIndex: 'section_type', key: 'section_type', render: (type: string) => ({ quantitative: '量化指标', key_action: '关键行动', bonus_penalty: '附加项' }[type] || type) },
                    { title: '权重', dataIndex: 'weight', key: 'weight', render: (w: number) => `${w}%` },
                    { title: '默认', dataIndex: 'is_default', key: 'is_default', render: (d: boolean) => d ? '是' : '否' },
                    {
                      title: '操作', key: 'actions', render: (_, record) => (
                        <Space size="small">
                          <Button size="small" onClick={() => openIndicatorItemModal(record)}>编辑</Button>
                          <Popconfirm title="确定删除此指标项？" onConfirm={() => handleDeleteIndicatorItem(record.id)}>
                            <Button size="small" danger>删除</Button>
                          </Popconfirm>
                        </Space>
                      )
                    }
                  ]}
                />
              </Spin>
            </Card>
          )}
        </Tabs.TabPane>
      </Tabs>
      </Card>

      {/* 活动详情抽屉 */}
      <Drawer
        title={`活动详情：${currentActivity?.name || ''}`}
        placement="right"
        width={1000}
        open={detailDrawerVisible}
        onClose={() => { setDetailDrawerVisible(false); setCurrentActivity(null); setParticipants([]); setSummary(null); setDistributionCheck(null); setDistributionRules([]); }}
      >
        {currentActivity && (
          <>
            <Descriptions column={3} size="small" style={{ marginBottom: 16 }} bordered>
              <Descriptions.Item label="状态">
                <Tag color={STATUS_MAP[currentActivity.status]?.color}>{STATUS_MAP[currentActivity.status]?.label}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="周期类型">{currentActivity.cycle_type}</Descriptions.Item>
              <Descriptions.Item label="绩效周期">{currentActivity.start_date} ~ {currentActivity.end_date}</Descriptions.Item>
              <Descriptions.Item label="自评时间">{currentActivity.self_eval_start_at} ~ {currentActivity.self_eval_end_at}</Descriptions.Item>
              <Descriptions.Item label="主管评分">{currentActivity.manager_eval_start_at} ~ {currentActivity.manager_eval_end_at}</Descriptions.Item>
              <Descriptions.Item label="结果确认">{currentActivity.result_confirm_start_at} ~ {currentActivity.result_confirm_end_at}</Descriptions.Item>
            </Descriptions>

            {/* 操作按钮 */}
            <Space style={{ marginBottom: 16 }} wrap>
              {currentActivity.status === 'draft' && (
                <>
                  <Button type="primary" onClick={() => handleActivityAction('open-target-setting', currentActivity)}>开启目标设定</Button>
                  <Button onClick={() => handleActivityAction('publish', currentActivity)}>直接开启自评（兼容）</Button>
                  <Button onClick={() => handleActivityAction('refresh', currentActivity)}>刷新参与人</Button>
                </>
              )}
              {currentActivity.status === 'target_setting' && (
                <>
                  <Button type="primary" onClick={() => handleActivityAction('open-self-evaluation', currentActivity)}>开启自评</Button>
                  <Button onClick={() => handleActivityAction('refresh', currentActivity)}>刷新参与人</Button>
                </>
              )}
              {currentActivity.status === 'self_evaluation' && (
                <>
                  <Button onClick={async () => {
                    try {
                      await performanceAPI.sendSelfEvalReminder(currentActivity.id)
                      message.success('已发送自评提醒')
                    } catch (err: any) {
                      message.error(err?.response?.data?.message || '发送提醒失败')
                    }
                  }}>提醒自评</Button>
                  <Button type="primary" onClick={() => handleActivityAction('open-manager-evaluation', currentActivity)}>开启主管评分</Button>
                  <Button onClick={() => handleActivityAction('refresh', currentActivity)}>刷新参与人</Button>
                </>
              )}
              {currentActivity.status === 'manager_evaluation' && (
                <>
                  <Button onClick={async () => {
                    try {
                      await performanceAPI.sendManagerEvalReminder(currentActivity.id)
                      message.success('已发送评分提醒')
                    } catch (err: any) {
                      message.error(err?.response?.data?.message || '发送提醒失败')
                    }
                  }}>提醒评分</Button>
                  <Button type="primary" onClick={() => handleActivityAction('open-employee-confirmation', currentActivity)}>开启员工确认</Button>
                  <Button onClick={() => handleActivityAction('refresh', currentActivity)}>刷新参与人</Button>
                  <Button onClick={() => setDistributionModalVisible(true)}>强制分布</Button>
                  <Divider type="vertical" />
                  <Button onClick={() => {
                    const selectable = participants.filter(p => p.status === 'self_submitted' || p.status === 'manager_submitted')
                    setBatchEvalSelected(selectable.map(p => p.id))
                    setBatchEvalModalVisible(true)
                  }}>批量评分</Button>
                </>
              )}
              {currentActivity.status === 'employee_confirmation' && (
                <Button type="primary" onClick={() => handleActivityAction('open-manager-confirmation', currentActivity)}>开启主管确认</Button>
              )}
              {currentActivity.status === 'manager_confirmation' && (
                <Button type="primary" onClick={() => handleActivityAction('open-hr-confirmation', currentActivity)}>开启HR确认</Button>
              )}
              {currentActivity.status === 'hr_confirmation' && (
                <Button type="primary" danger onClick={() => handleActivityAction('lock', currentActivity)}>锁定活动</Button>
              )}
              {currentActivity.status === 'locked' && (
                <Button onClick={() => handleActivityAction('archive', currentActivity)}>归档活动</Button>
              )}
              {currentActivity.status === 'result_confirmed' && (
                <Button onClick={() => handleActivityAction('archive', currentActivity)}>归档活动</Button>
              )}
            </Space>

            <Divider style={{ margin: '12px 0' }}>统计摘要</Divider>
            <Spin spinning={summaryLoading}>
              {summary ? (
                <Row gutter={[12, 8]} style={{ marginBottom: 12 }}>
                  <Col span={6}><Card size="small"><Statistic title="参与人数" value={summary.total_participants} /></Card></Col>
                  <Col span={6}><Card size="small"><Statistic title="已自评" value={summary.self_submitted_count} /></Card></Col>
                  <Col span={6}><Card size="small"><Statistic title="已评分" value={summary.manager_submitted_count} /></Card></Col>
                  <Col span={6}><Card size="small"><Statistic title="已确认" value={summary.result_confirmed_count} /></Card></Col>
                </Row>
              ) : <Text type="secondary">暂无数据</Text>}
            </Spin>

            {distributionCheck && (
              <Card size="small" style={{ marginBottom: 12 }}>
                <Row gutter={[8, 8]}>
                  {['S', 'A', 'B', 'C', 'D'].map(level => {
                    const dist = distributionCheck.distribution?.[level]
                    if (!dist) return null
                    const statusColor = dist.status === 'exceeded' ? 'exception' : dist.status === 'warning' ? 'normal' : 'success'
                    return (
                      <Col span={4} key={level}>
                        <Card size="small" style={{ textAlign: 'center', background: dist.status === 'exceeded' ? '#fff2f0' : dist.status === 'warning' ? '#fffbe6' : '#f6ffed' }}>
                          <Text strong style={{ fontSize: 16 }}>{level}</Text>
                          <br />
                          <Text type="secondary">{dist.actual_count}/{dist.expected_count} 人</Text>
                          <br />
                          <Progress percent={Math.min(dist.progress, 100)} size="small" status={statusColor} showInfo={false} strokeWidth={6} />
                          <Text type="secondary" style={{ fontSize: 10 }}>期望 {dist.expected_percent}%</Text>
                        </Card>
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
                    style={{ marginTop: 8 }}
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
      >
        <Alert showIcon type="info" style={{ marginBottom: 16 }} message="前端校验比例总和需等于 100%，但以后端校验为准。" />
        <Form form={distributionForm} layout="vertical">
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
            <Tag color={
              batchEvalScore >= 100 ? '#f50' :
              batchEvalScore >= 90 ? '#2db7f5' :
              batchEvalScore >= 80 ? '#87d068' :
              batchEvalScore >= 60 ? '#faad14' : '#ff4d4f'
            } style={{ fontSize: 14, padding: '4px 12px' }}>
              {batchEvalScore >= 100 ? 'S - 杰出' :
               batchEvalScore >= 90 ? 'A - 优秀' :
               batchEvalScore >= 80 ? 'B - 良好' :
               batchEvalScore >= 60 ? 'C - 待改进' : 'D - 不合格'}
            </Tag>
            <Text type="secondary" style={{ marginLeft: 8 }}>根据评分自动生成</Text>
          </Form.Item>
          <Form.Item name="batch_comment" label="上级评语">
            <TextArea rows={3} placeholder="请输入统一评语（可选）" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 指标库表单弹窗 */}
      <Modal
        title={editingIndicatorLibrary ? '编辑指标库' : '新建指标库'}
        open={indicatorLibraryModalVisible}
        onOk={handleSaveIndicatorLibrary}
        onCancel={() => setIndicatorLibraryModalVisible(false)}
      >
        <Form form={indicatorLibraryForm} layout="vertical">
          <Form.Item name="name" label="指标库名称" rules={[{ required: true, message: '请输入指标库名称' }]}>
            <Input placeholder="请输入指标库名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <TextArea rows={3} placeholder="请输入描述" />
          </Form.Item>
          <Form.Item name="department_id" label="所属部门" rules={[{ required: true, message: '请选择所属部门' }]}>
            <Select
              placeholder="请选择所属部门"
              showSearch
              optionFilterProp="label"
              options={departments.map(d => getDepartmentOption(d)).filter(Boolean) as any[]}
            />
          </Form.Item>
          <Form.Item name="default_cycle" label="默认周期">
            <Select placeholder="请选择默认周期">
              <Select.Option value="monthly">月度</Select.Option>
              <Select.Option value="quarterly">季度</Select.Option>
              <Select.Option value="annual">年度</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      {/* 指标项表单弹窗 */}
      <Modal
        title={editingIndicatorItem ? '编辑指标项' : '添加指标项'}
        open={indicatorItemModalVisible}
        onOk={handleSaveIndicatorItem}
        onCancel={() => setIndicatorItemModalVisible(false)}
      >
        <Form form={indicatorItemForm} layout="vertical">
          <Form.Item name="name" label="指标名称" rules={[{ required: true, message: '请输入指标名称' }]}>
            <Input placeholder="请输入指标名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <TextArea rows={3} placeholder="请输入描述" />
          </Form.Item>
          <Form.Item name="section_type" label="指标类型" rules={[{ required: true, message: '请选择指标类型' }]}>
            <Select placeholder="请选择指标类型">
              <Select.Option value="quantitative">量化指标</Select.Option>
              <Select.Option value="key_action">关键行动</Select.Option>
              <Select.Option value="bonus_penalty">附加项</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="weight" label="权重（%）">
            <InputNumber min={0} max={100} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="is_default" label="默认指标" valuePropName="checked">
            <Input type="checkbox">设为默认指标</Input>
          </Form.Item>
          <Form.Item name="sort_order" label="排序">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default PerformanceOverview
