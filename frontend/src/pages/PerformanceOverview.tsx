import React, { useState, useCallback, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Alert, Card, Col, Row, Space, Table, Tag, Typography, Button, Modal, Form, Input, InputNumber,
  Select, message, Spin, Drawer, Tooltip, Divider, Descriptions, Steps, Segmented, Progress
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
  PerformanceParticipantImportResult,
  PerformanceActivityManagerAssignment,
  PerformanceDistributionRule,
  PerformanceHRDeadlineStatus,
  PerformanceIndicatorLibrary,
  PerformanceTemplate,
  AssessmentManagerCandidate,
  AssessmentManagerCandidateSourceGroup,
  AssessmentManagerSource,
  userAPI,
} from '../services/api'
import PerformanceActivityEditor from '../components/PerformanceActivityEditor'
import {
  BarChartOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { getCycleLabel, formatDateTime } from '../utils/format'
import { hasPermission } from '../utils/permission'
import { useAuthStore } from '../store/authStore'

const { Text, Paragraph } = Typography
const { TextArea } = Input

type RejectGoalFormValues = {
  comment: string
}

type SelectOption = {
  label: React.ReactNode
  value: string | number
}

type AssessmentManagerSelectOption = SelectOption & {
  searchText: string
}

type PerformanceView = 'employee' | 'manager' | 'hr'

const PERFORMANCE_VIEW_OPTIONS: Array<{ label: string; value: PerformanceView }> = [
  { label: '我的绩效', value: 'employee' },
  { label: '团队绩效', value: 'manager' },
  { label: 'HR 管理', value: 'hr' },
]

const PERFORMANCE_VIEW_META: Record<PerformanceView, { title: string; description: string; detailTitle: string; icon: React.ReactNode; accent: string; softBg: string }> = {
  employee: {
    title: '我的绩效',
    description: '聚焦我参与的活动、当前待办和结果确认',
    detailTitle: '我的事项',
    icon: <UserOutlined />,
    accent: '#1677ff',
    softBg: '#eff6ff',
  },
  manager: {
    title: '团队绩效',
    description: '聚焦团队待处理事项、评分与复核进度',
    detailTitle: '团队处理',
    icon: <TeamOutlined />,
    accent: '#0891b2',
    softBg: '#ecfeff',
  },
  hr: {
    title: 'HR 管理',
    description: '维护活动配置、阶段推进和全员进度',
    detailTitle: 'HR 管控',
    icon: <SafetyCertificateOutlined />,
    accent: '#4f46e5',
    softBg: '#eef2ff',
  },
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

const FLOW_TYPE_LABELS: Record<string, string> = {
  old: '旧流程',
  new: '新流程',
}

const FLOW_TEMPLATE_LABELS: Record<string, string> = {
  old: '小铁文娱流程模版',
  new: '沐腾科技流程模版',
}

const BUILT_IN_FLOW_TEMPLATE_NAMES = new Set([
  '旧绩效流程模板',
  '新绩效流程模板',
  '旧绩效流程模版',
  '新绩效流程模版',
  '旧流程模板',
  '新流程模板',
  '旧流程模版',
  '新流程模版',
  '旧流程',
  '新流程',
])

function getFlowTypeLabel(flowType?: string) {
  return FLOW_TYPE_LABELS[String(flowType || '').trim()] || '未配置流程'
}

function getTemplateDisplayName(templateName?: string, flowType?: string) {
  const normalizedName = String(templateName || '').trim()
  const normalizedFlowType = String(flowType || '').trim()
  if (normalizedName && !BUILT_IN_FLOW_TEMPLATE_NAMES.has(normalizedName)) return normalizedName
  return FLOW_TEMPLATE_LABELS[normalizedFlowType] || normalizedName
}

function getActivityTemplateDisplay(
  activity: PerformanceActivity,
  templateById: Map<string, PerformanceTemplate>,
) {
  const templateId = activity.template_id == null ? '' : String(activity.template_id)
  const inlineTemplate = (activity as any).template
  const inlineTemplateName = String((activity as any).template_name || inlineTemplate?.name || '').trim()
  const matchedTemplate = templateId ? templateById.get(templateId) : undefined
  const templateName = inlineTemplateName || String(matchedTemplate?.name || '').trim()
  const flowType = String(inlineTemplate?.flow_type || matchedTemplate?.flow_type || activity.flow_type || '').trim()
  const templateDisplayName = getTemplateDisplayName(templateName, flowType)

  if (templateDisplayName) {
    return {
      label: templateDisplayName,
      color: flowType === 'new' ? 'purple' : 'default',
      tooltip: `流程类型：${getFlowTypeLabel(flowType)}`,
    }
  }

  if (templateId) {
    return {
      label: `模板 #${templateId}`,
      color: 'warning',
      tooltip: `未找到模板详情，流程类型：${getFlowTypeLabel(flowType)}`,
    }
  }

  return {
    label: getTemplateDisplayName('', flowType) || getFlowTypeLabel(flowType),
    color: flowType === 'new' ? 'purple' : 'default',
    tooltip: '历史活动未关联绩效模板',
  }
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

function getImportedUserOption(user: any) {
  const value = String(user?.user_id || user?.employee_id || user?.id || '').trim()
  if (!value) return null
  const employeeID = String(user?.employee_id || '').trim()
  const name = String(user?.name || user?.user_name || user?.employee_name || value).trim()
  const employeeIDText = employeeID && employeeID !== value ? ` / ${employeeID}` : ''
  const departmentName = user?.department_name ? ` - ${user.department_name}` : ''
  return { value, label: `${name}（${value}${employeeIDText}）${departmentName}` }
}

function mergeSelectOptions(baseOptions: SelectOption[], extraOptions: SelectOption[]) {
  const merged = [...baseOptions]
  const seen = new Set(baseOptions.map(option => String(option.value)))

  extraOptions.forEach(option => {
    const key = String(option.value)
    if (!key || seen.has(key)) return
    seen.add(key)
    merged.push(option)
  })

  return merged
}

function getImportedUserOptions(
  result: PerformanceParticipantImportResult | null | undefined,
  employeeIDs: string[],
) {
  const detailOptions = (result?.employees || []).flatMap(employee => {
    const option = getImportedUserOption(employee)
    return option ? [option] : []
  })
  const fallbackOptions = employeeIDs.flatMap(employeeID => {
    const option = getImportedUserOption({ user_id: employeeID })
    return option ? [option] : []
  })
  return mergeSelectOptions(detailOptions, fallbackOptions)
}

function normalizeImportedManagerAssignments(
  assignments?: PerformanceActivityManagerAssignment[] | null,
  employeeIDs?: string[],
): PerformanceActivityManagerAssignment[] {
  const allowedEmployeeIDs = employeeIDs ? new Set(normalizeIDArray(employeeIDs)) : null
  const byUserID = new Map<string, PerformanceActivityManagerAssignment>()
  ;(assignments || []).forEach(assignment => {
    const userID = String(assignment.user_id || '').trim()
    const managerUserID = String(assignment.assessment_manager_user_id || '').trim()
    if (!userID || !managerUserID) return
    if (allowedEmployeeIDs && !allowedEmployeeIDs.has(userID)) return
    byUserID.set(userID, {
      ...assignment,
      user_id: userID,
      employee_id: String(assignment.employee_id || '').trim() || undefined,
      assessment_manager_user_id: managerUserID,
      assessment_manager_employee_id: String(assignment.assessment_manager_employee_id || '').trim() || undefined,
      assessment_manager_name: String(assignment.assessment_manager_name || '').trim(),
      assessment_manager_source: assignment.assessment_manager_source || 'IMPORT',
      manager_override_reason: String(assignment.manager_override_reason || '').trim() || undefined,
    })
  })
  return Array.from(byUserID.values())
}

function getAssessmentCandidateOption(candidate: AssessmentManagerCandidate): AssessmentManagerSelectOption | null {
  const value = String(candidate.user_id || '').trim()
  if (!value) return null
  const name = String(candidate.name || value).trim()
  const employeeNo = String(candidate.employee_no || '').trim()
  const departmentName = String(candidate.department_name || '').trim()
  const sourceLabel = candidate.candidate_source_label || MANAGER_SOURCE_LABELS[candidate.candidate_source] || '候选'
  const sourceTag = candidate.is_self_final_candidate ? '自评即终评' : sourceLabel
  return {
    value,
    searchText: [name, value, employeeNo, departmentName].filter(Boolean).join(' '),
    label: (
      <Space size={6}>
        <Text>{name}</Text>
        <Text type="secondary">{employeeNo || value}</Text>
        {departmentName && <Text type="secondary">{departmentName}</Text>}
        <Tag color={candidate.is_self_final_candidate ? 'purple' : 'blue'}>{sourceTag}</Tag>
      </Space>
    ),
  }
}

function getAssessmentUserOption(user: any): AssessmentManagerSelectOption | null {
  if (String(user?.status || 'active').trim() !== 'active') return null
  const value = String(user?.user_id || user?.employee_id || user?.id || '').trim()
  if (!value) return null
  const name = String(user?.name || user?.user_name || user?.employee_name || value).trim()
  const employeeNo = String(user?.employee_id || user?.employee_no || '').trim()
  const departmentName = String(user?.department_name || '').trim()
  return {
    value,
    searchText: [name, value, employeeNo, departmentName, user?.mobile].filter(Boolean).join(' '),
    label: (
      <Space size={6}>
        <Text>{name}</Text>
        <Text type="secondary">{employeeNo || value}</Text>
        {departmentName && <Text type="secondary">{departmentName}</Text>}
        <Tag>手动指定</Tag>
      </Space>
    ),
  }
}

function normalizedIdentityValues(values: unknown[]) {
  return values.map(value => String(value ?? '').trim()).filter(Boolean)
}

function identitiesIntersect(leftValues: unknown[], rightValues: unknown[]) {
  const right = new Set(normalizedIdentityValues(rightValues))
  return normalizedIdentityValues(leftValues).some(value => right.has(value))
}

function participantIdentityValues(participant?: PerformanceParticipant | null) {
  if (!participant) return []
  const extra = participant as any
  return normalizedIdentityValues([
    participant.employee_id,
    extra.user_id,
    extra.employee_no,
  ])
}

function candidateIdentityValues(candidate: AssessmentManagerCandidate) {
  return normalizedIdentityValues([
    candidate.user_id,
    candidate.employee_no,
  ])
}

function userIdentityValues(user: any) {
  return normalizedIdentityValues([
    user?.user_id,
    user?.employee_id,
    user?.employee_no,
    user?.id,
  ])
}

function currentUserIdentityValues(user: any) {
  return normalizedIdentityValues([
    user?.user_id,
    user?.employee_id,
    user?.employee_no,
    user?.id,
    user?.dingtalk_user_id,
  ])
}

function participantMatchesCurrentUser(participant: PerformanceParticipant | null | undefined, user: any) {
  return identitiesIntersect(participantIdentityValues(participant), currentUserIdentityValues(user))
}

function participantManagedByCurrentUser(participant: PerformanceParticipant | null | undefined, user: any) {
  if (!participant) return false
  return identitiesIntersect([participant.manager_id, (participant as any).manager_user_id], currentUserIdentityValues(user))
}

function valueMatchesParticipant(value: unknown, participant?: PerformanceParticipant | null) {
  return identitiesIntersect([value], participantIdentityValues(participant))
}

function valueMatchesAnyParticipant(value: unknown, participants: Array<PerformanceParticipant | null | undefined>) {
  return participants.some(participant => valueMatchesParticipant(value, participant))
}

function candidateMatchesAnyParticipant(candidate: AssessmentManagerCandidate, participants: Array<PerformanceParticipant | null | undefined>) {
  return participants.some(participant => identitiesIntersect(candidateIdentityValues(candidate), participantIdentityValues(participant)))
}

function selfFinalCandidateMatchesValue(candidate: AssessmentManagerCandidate, value: unknown) {
  return Boolean(candidate.is_self_final_candidate) && identitiesIntersect(candidateIdentityValues(candidate), [value])
}

function valueIsAllowedSelfFinalCandidate(value: unknown, candidates: AssessmentManagerCandidate[]) {
  return candidates.some(candidate => selfFinalCandidateMatchesValue(candidate, value))
}

function userMatchesAnyParticipant(user: any, participants: Array<PerformanceParticipant | null | undefined>) {
  return participants.some(participant => identitiesIntersect(userIdentityValues(user), participantIdentityValues(participant)))
}

function isSelfFinalAssessmentRecord(record: PerformanceParticipant) {
  return record.manager_source === 'MANUAL' && valueMatchesParticipant(record.manager_id, record)
}

function getEffectiveManagerConfigStatus(record: PerformanceParticipant) {
  if (record.manager_config_status === 'INVALID' && isSelfFinalAssessmentRecord(record)) {
    return 'CONFIGURED'
  }
  return String(record.manager_config_status || '').trim()
}

function isAssessmentManagerConfigured(record: PerformanceParticipant) {
  const managerID = String(record.manager_id || '').trim()
  const configStatus = getEffectiveManagerConfigStatus(record)
  return Boolean(managerID) && configStatus !== 'PENDING' && configStatus !== 'INVALID'
}

function getManagerEvaluationBlockedReason(record: PerformanceParticipant) {
  const configStatus = getEffectiveManagerConfigStatus(record)
  if (!String(record.manager_id || '').trim()) return '请先配置考核上级'
  if (configStatus === 'PENDING') return '请先配置考核上级'
  if (configStatus === 'INVALID') return '考核上级不可用，请先调整'
  return ''
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

const ACTIVITY_STATUS_FILTER_IN_PROGRESS = '__in_progress__'
const ACTIVITY_STATUS_FILTER_CONFIRMED = '__confirmed__'
const IN_PROGRESS_ACTIVITY_STATUSES = [
  'target_setting',
  'self_evaluation',
  'manager_evaluation',
  'employee_confirmation',
  'manager_confirmation',
  'hr_confirmation',
]
const CONFIRMED_ACTIVITY_STATUSES = ['locked', 'result_confirmed']
const PERSONAL_ENTRY_ACTIVITY_STATUSES = [
  'target_setting',
  'self_evaluation',
  'manager_evaluation',
  'employee_confirmation',
  'manager_confirmation',
  'hr_confirmation',
  'locked',
  'result_confirmed',
]
const PERSONAL_TARGET_STATUSES = ['pending', 'target_pending_approval', 'target_rejected', 'target_set']
const PERSONAL_SELF_EVAL_STATUSES = ['target_set', 'self_submitted']
const PERSONAL_RESULT_STATUSES = ['manager_submitted', 'employee_confirmed', 'manager_recheck', 'manager_confirmed', 'hr_confirmed', 'locked', 'result_confirmed']
const SELF_EVAL_EDITABLE_ACTIVITY_STATUSES = ['self_evaluation', 'manager_evaluation', 'employee_confirmation', 'manager_confirmation', 'hr_confirmation', 'result_confirmed']
const SELF_EVAL_EDITABLE_PARTICIPANT_STATUSES = ['target_set', 'self_submitted', 'manager_submitted', 'employee_confirmed', 'manager_recheck', 'manager_confirmed']
const ACTIVITY_STATUS_FILTER_GROUPS: Record<string, string[]> = {
  [ACTIVITY_STATUS_FILTER_IN_PROGRESS]: IN_PROGRESS_ACTIVITY_STATUSES,
  [ACTIVITY_STATUS_FILTER_CONFIRMED]: CONFIRMED_ACTIVITY_STATUSES,
}
const ACTIVITY_STATUS_FILTER_OPTIONS = [
  { value: ACTIVITY_STATUS_FILTER_IN_PROGRESS, label: '进行中活动' },
  { value: ACTIVITY_STATUS_FILTER_CONFIRMED, label: '已确认结果' },
  ...STATUS_OPTIONS,
]

function resolveActivityStatusFilter(statusFilter?: string) {
  if (!statusFilter) return undefined
  return ACTIVITY_STATUS_FILTER_GROUPS[statusFilter] || [statusFilter]
}

const MANAGER_SOURCE_LABELS: Record<AssessmentManagerSource, string> = {
  DIRECT_MANAGER: '直属主管',
  DEPARTMENT_HEAD: '部门负责人',
  CENTER_HEAD: '中心负责人',
  MANUAL: '手动指定',
  IMPORT: '导入指定',
  EMPTY: '暂未配置',
  SYSTEM: '系统兼容',
}

const MANAGER_CONFIG_STATUS_LABELS: Record<string, { label: string; color: string }> = {
  CONFIGURED: { label: '已配置', color: 'green' },
  PENDING: { label: '待配置考核上级', color: 'orange' },
  INVALID: { label: '考核上级不可用', color: 'red' },
}

const ADJUSTABLE_MANAGER_SOURCES: AssessmentManagerSource[] = [
  'DIRECT_MANAGER',
  'DEPARTMENT_HEAD',
  'CENTER_HEAD',
  'MANUAL',
]

const getAdjustableManagerSource = (source?: AssessmentManagerSource): AssessmentManagerSource =>
  source && ADJUSTABLE_MANAGER_SOURCES.includes(source) ? source : 'MANUAL'

const MANAGER_SOURCE_OPTIONS = ADJUSTABLE_MANAGER_SOURCES.map(value => ({
  value,
  label: MANAGER_SOURCE_LABELS[value],
}))

// 参与人状态映射
const PARTICIPANT_STATUS_MAP: Record<string, { label: string; color: string }> = {
  pending: { label: '待目标', color: 'default' },
  target_pending_approval: { label: '目标待审', color: 'cyan' },
  target_rejected: { label: '目标驳回', color: 'red' },
  target_set: { label: '目标已定', color: 'cyan' },
  self_submitted: { label: '已自评', color: 'processing' },
  manager_submitted: { label: '已评分', color: 'warning' },
  employee_confirmed: { label: '已员工确认', color: 'blue' },
  manager_recheck: { label: '待领导复核', color: 'warning' },
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

const PERFORMANCE_PERMISSION_LABELS: Record<string, string> = {
  'performance:activity:manage': '绩效活动管理',
  'performance:distribution:manage': '绩效分布规则',
  'performance:goal:manage': '绩效目标管理',
  'performance:self_eval:submit': '绩效自评提交',
  'performance:manager_eval:submit': '绩效主管评分',
  'performance:result:view': '绩效结果查看',
  'performance:hr_confirm:submit': '绩效HR确认',
  'performance:assessment_manager:update': '考核上级调整',
  'performance:assessment_manager:batch_update': '批量考核上级调整',
}

const PerformanceOverview: React.FC = () => {
  const navigate = useNavigate()
  const currentUser = useAuthStore(state => state.user)
  const [activeView, setActiveView] = useState<PerformanceView>('employee')
  const activityListRef = React.useRef<HTMLDivElement | null>(null)
  const [, forceRender] = React.useState(0)
  const forceUpdate = () => forceRender(n => n + 1)
  const [activities, setActivities] = useState<PerformanceActivity[]>([])
  const [activitiesLoading, setActivitiesLoading] = useState(false)
  const [activitiesTotal, setActivitiesTotal] = useState(0)
  const [activityModalVisible, setActivityModalVisible] = useState(false)
  const [activitySaving, setActivitySaving] = useState(false)
  const [participantImporting, setParticipantImporting] = useState(false)
  const [editingActivity, setEditingActivity] = useState<PerformanceActivity | null>(null)
  const [form] = Form.useForm()
  const [departments, setDepartments] = useState<any[]>([])
  const [users, setUsers] = useState<any[]>([])
  const [importedUserOptions, setImportedUserOptions] = useState<SelectOption[]>([])
  const [importedManagerAssignments, setImportedManagerAssignments] = useState<PerformanceActivityManagerAssignment[]>([])
  const [scopeOptionsLoading, setScopeOptionsLoading] = useState(false)
  const [indicatorLibraries, setIndicatorLibraries] = useState<PerformanceIndicatorLibrary[]>([])
  const [indicatorLibrariesLoading, setIndicatorLibrariesLoading] = useState(false)
  const [performanceTemplates, setPerformanceTemplates] = useState<PerformanceTemplate[]>([])
  const [performanceTemplatesLoading, setPerformanceTemplatesLoading] = useState(false)
  const performanceTemplateById = React.useMemo(
    () => new Map(performanceTemplates.map(template => [String(template.id), template])),
    [performanceTemplates],
  )
  const [previousActivityOptions, setPreviousActivityOptions] = useState<SelectOption[]>([])
  const [previousActivityLoading, setPreviousActivityLoading] = useState(false)

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
  // 注意：以下 Modal 相关的 Form 实例在 Modal 关闭时会产生 antd 的 "not connected" warning
  // 这是因为 useForm() 在组件挂载时创建实例，但 Modal 内的 <Form> 组件在 Modal 打开时才渲染
  // 这个 warning 不影响功能，是 antd Form 设计模式的固有特性，已在 E2E 测试中验证功能正常
  const [distributionForm] = Form.useForm()
  const [rejectGoalModalVisible, setRejectGoalModalVisible] = useState(false)
  const [rejectGoalTarget, setRejectGoalTarget] = useState<PerformanceParticipant | null>(null)
  const [rejectGoalForm] = Form.useForm<RejectGoalFormValues>()

  // 批量评分相关
  const [batchEvalModalVisible, setBatchEvalModalVisible] = useState(false)
  const [batchEvalSelected, setBatchEvalSelected] = useState<number[]>([])
  const [batchEvalLoading, setBatchEvalLoading] = useState(false)
  const [batchEvalForm] = Form.useForm()
  const [batchEvalScore, setBatchEvalScore] = useState<number>(0)
  const [selectedParticipantIds, setSelectedParticipantIds] = useState<React.Key[]>([])
  const [managerModalVisible, setManagerModalVisible] = useState(false)
  const [managerModalMode, setManagerModalMode] = useState<'single' | 'batch'>('single')
  const [managerTargetParticipant, setManagerTargetParticipant] = useState<PerformanceParticipant | null>(null)
  const [managerCandidates, setManagerCandidates] = useState<AssessmentManagerCandidate[]>([])
  const [managerCandidateSources, setManagerCandidateSources] = useState<AssessmentManagerCandidateSourceGroup[]>([])
  const [managerCandidateLoading, setManagerCandidateLoading] = useState(false)
  const [managerUpdating, setManagerUpdating] = useState(false)
  const [managerForm] = Form.useForm()
  const [selectedManagerSource, setSelectedManagerSource] = useState<AssessmentManagerSource | undefined>(undefined)

  // 活动列表筛选
  const [activitySearchText, setActivitySearchText] = useState('')
  const [activityStatusFilter, setActivityStatusFilter] = useState<string | undefined>(undefined)

  const canUseManagerView = hasPermission('performance:manager_eval:submit') ||
    hasPermission('performance:goal:manage') ||
    hasPermission('performance:manager_confirm:submit')
  const canUseHRView = hasPermission('performance:activity:manage') ||
    hasPermission('performance:distribution:manage') ||
    hasPermission('performance:hr_confirm:submit') ||
    hasPermission('performance:assessment_manager:batch_update')
  const performanceViewOptions = React.useMemo(
    () => PERFORMANCE_VIEW_OPTIONS.filter(option => {
      if (option.value === 'manager') return canUseManagerView
      if (option.value === 'hr') return canUseHRView
      return true
    }),
    [canUseHRView, canUseManagerView],
  )

  useEffect(() => {
    if (performanceViewOptions.some(option => option.value === activeView)) return
    setActiveView(performanceViewOptions[0]?.value || 'employee')
    setActivityStatusFilter(undefined)
    setSelectedParticipantIds([])
  }, [activeView, performanceViewOptions])

  const departmentOptions = React.useMemo(
    () => departments.flatMap(department => {
      const option = getDepartmentOption(department)
      return option ? [option] : []
    }),
    [departments],
  )

  const baseUserOptions = React.useMemo(
    () => users.flatMap(user => {
      const option = getUserOption(user)
      return option ? [option] : []
    }),
    [users],
  )

  const userOptions = React.useMemo(
    () => mergeSelectOptions(baseUserOptions, importedUserOptions),
    [baseUserOptions, importedUserOptions],
  )
  const managerTargetParticipants = React.useMemo(() => {
    if (managerModalMode === 'single') {
      return managerTargetParticipant ? [managerTargetParticipant] : []
    }
    const selected = new Set(selectedParticipantIds.map(id => String(id)))
    return participants.filter(participant => selected.has(String(participant.id)))
  }, [managerModalMode, managerTargetParticipant, participants, selectedParticipantIds])

  const managerSelectOptions = React.useMemo(() => {
    const options: AssessmentManagerSelectOption[] = []
    const seen = new Set<string>()
    const addOption = (option: AssessmentManagerSelectOption | null) => {
      if (!option) return
      const key = String(option.value)
      if (!key || seen.has(key)) return
      seen.add(key)
      options.push(option)
    }

    managerCandidates.forEach(candidate => {
      if (candidate.is_self_final_candidate || !candidateMatchesAnyParticipant(candidate, managerTargetParticipants)) {
        addOption(getAssessmentCandidateOption(candidate))
      }
    })
    if (selectedManagerSource === 'MANUAL') {
      users.forEach(user => {
        if (!userMatchesAnyParticipant(user, managerTargetParticipants)) {
          addOption(getAssessmentUserOption(user))
        }
      })
    }
    return options
  }, [managerCandidates, managerTargetParticipants, selectedManagerSource, users])

  const selectedManagerSourceGroup = React.useMemo(
    () => managerCandidateSources.find(item => item.source === selectedManagerSource),
    [managerCandidateSources, selectedManagerSource],
  )

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

  const loadPerformanceTemplates = useCallback(async () => {
    setPerformanceTemplatesLoading(true)
    try {
      const res: any = await performanceAPI.getTemplates({
        page: 1,
        page_size: 1000,
        status: 'active',
      })
      setPerformanceTemplates(getListFromResponse(res, ['items', 'templates']))
    } catch {
      setPerformanceTemplates([])
      message.error('流程模板选项加载失败')
    } finally {
      setPerformanceTemplatesLoading(false)
    }
  }, [])

  const loadPreviousActivities = useCallback(async (excludeId?: number) => {
    setPreviousActivityLoading(true)
    try {
      const res: any = await performanceAPI.getActivities({ page: 1, page_size: 200 })
      const list = getListFromResponse(res, ['items', 'activities']) as PerformanceActivity[]
      const options: SelectOption[] = list
        .filter(a => a.flow_type === 'new' && (excludeId ? a.id !== excludeId : true))
        .sort((a, b) => (a.start_date > b.start_date ? -1 : 1))
        .map(a => ({ value: a.id, label: a.name }))
      setPreviousActivityOptions(options)
    } catch {
      setPreviousActivityOptions([])
    } finally {
      setPreviousActivityLoading(false)
    }
  }, [])

  // 首次加载活动列表
  React.useEffect(() => {
    loadActivities()
    loadScopeOptions()
    loadPerformanceTemplates()
  }, [loadActivities, loadScopeOptions, loadPerformanceTemplates])

  // 加载活动详情
  const loadActivityDetail = async (activity: PerformanceActivity) => {
    setCurrentActivity(activity)
    setDetailDrawerVisible(true)
    setParticipantsLoading(true)
    setSummaryLoading(activeView === 'hr')
    setDistributionCheckLoading(activeView === 'hr')
    setHrDeadlineStatus(null)

    try {
      const res: any = await performanceAPI.getParticipants(activity.id, { page: 1, page_size: 200 })
      const pData = res?.data || res
      setParticipants(pData?.items || [])
      setSelectedParticipantIds([])
    } catch {
      setParticipants([])
      setSelectedParticipantIds([])
    } finally {
      setParticipantsLoading(false)
    }

    if (activeView !== 'hr') {
      setSummary(null)
      setDistributionCheck(null)
      setDistributionRules([])
      setSummaryLoading(false)
      setDistributionCheckLoading(false)
      return
    }

    const results = await Promise.allSettled([
      performanceAPI.getResultSummary(activity.id),
      performanceAPI.getDistributionCheck(activity.id),
      performanceAPI.getDistributionRules(activity.id),
      performanceAPI.getHRConfirmDeadlineStatus(activity.id),
    ])

    if (results[0].status === 'fulfilled') {
      const res = results[0].value as any
      setSummary(res?.data || null)
    } else setSummary(null)
    setSummaryLoading(false)

    if (results[1].status === 'fulfilled') {
      const res = results[1].value as any
      setDistributionCheck((res?.data || res) || null)
    } else setDistributionCheck(null)
    setDistributionCheckLoading(false)

    if (results[2].status === 'fulfilled') {
      const res = results[2].value as any
      setDistributionRules((res?.data || res)?.rules || [])
    } else setDistributionRules([])

    if (results[3].status === 'fulfilled') {
      const res = results[3].value as any
      setHrDeadlineStatus((res?.data || res) as PerformanceHRDeadlineStatus)
    } else setHrDeadlineStatus(null)
  }

  const reloadParticipants = async (activityId: number) => {
    setParticipantsLoading(true)
    try {
      const res: any = await performanceAPI.getParticipants(activityId, { page: 1, page_size: 200 })
      const pData = res?.data || res
      setParticipants(pData?.items || [])
      setSelectedParticipantIds([])
    } catch {
      setParticipants([])
      setSelectedParticipantIds([])
    } finally {
      setParticipantsLoading(false)
    }
  }

  const closeActivityEditor = () => {
    setActivityModalVisible(false)
    setEditingActivity(null)
    setImportedUserOptions([])
    setImportedManagerAssignments([])
    form.resetFields()
  }

  // 导入参与人名单
  const handleImportParticipants = async (file: File) => {
    if (participantImporting) return
    setParticipantImporting(true)
    try {
      const res: any = await performanceAPI.importParticipants(file)
      const result = (res?.data?.result || res?.result || res?.data || res) as PerformanceParticipantImportResult
      const employeeIDs = normalizeIDArray(result.employee_ids)
      const managerAssignments = normalizeImportedManagerAssignments(result.manager_assignments, employeeIDs)
      setImportedManagerAssignments(managerAssignments)
      setImportedUserOptions(previous => mergeSelectOptions(previous, getImportedUserOptions(result, employeeIDs)))
      if (employeeIDs.length > 0) {
        const nextValues: Record<string, any> = { target_employee_ids: employeeIDs }
        if (result.activity_name && !String(form.getFieldValue('name') || '').trim()) {
          nextValues.name = result.activity_name
        }
        form.setFieldsValue(nextValues)
        forceUpdate()
      }

      const notes: string[] = []
      if (result.duplicate_count) notes.push(`重复 ${result.duplicate_count}`)
      if (result.missing_employee_ids?.length) notes.push(`未匹配 ${result.missing_employee_ids.length}`)
      if (result.inactive_employee_ids?.length) notes.push(`非在职 ${result.inactive_employee_ids.length}`)
      if (result.skipped_rows?.length) notes.push(`跳过 ${result.skipped_rows.length} 行`)
      if (result.manager_assignment_skipped_rows?.length) notes.push(`上级跳过 ${result.manager_assignment_skipped_rows.length} 行`)
      if (managerAssignments.length) notes.push(`上级 ${managerAssignments.length}`)
      const suffix = notes.length ? `（${notes.join('，')}）` : ''
      if (employeeIDs.length > 0) {
        message.success(`已导入 ${employeeIDs.length} 名指定员工${suffix}`)
      } else {
        message.warning(`未导入有效员工${suffix}`)
      }
      result.warnings?.slice(0, 2).forEach(warning => message.warning(warning))
    } catch (err: any) {
      message.error(err?.response?.data?.message || '导入失败，请检查 Excel 模板')
    } finally {
      setParticipantImporting(false)
    }
  }

  // 创建/编辑活动
  const handleSaveActivity = async () => {
    if (activitySaving) return
    setActivitySaving(true)
    try {
      const values = await form.validateFields()
      const isCreating = !editingActivity
      const targetEmployeeIDs = normalizeIDArray(values.target_employee_ids)
      const managerAssignments = normalizeImportedManagerAssignments(
        importedManagerAssignments,
        targetEmployeeIDs.length ? targetEmployeeIDs : undefined,
      )
      const selectedTemplate = performanceTemplates.find(template => String(template.id) === String(values.template_id))
      const flowType = selectedTemplate?.flow_type || values.flow_type || 'old'
      const data = {
        name: values.name,
        cycle_type: values.cycle_type,
        start_date: values.date_range[0].format('YYYY-MM-DD'),
        end_date: values.date_range[1].format('YYYY-MM-DD'),
        template_id: values.template_id,
        flow_type: flowType,
        organization_id: values.organization_id || '',
        applicable_org_scope: normalizeIDArray(values.applicable_org_scope),
        target_set_start_at: formatRangeStart(values.target_set_range),
        target_set_end_at: formatRangeEnd(values.target_set_range),
        snapshot_as_of_date: values.snapshot_as_of_date?.format('YYYY-MM-DD') || '',
        snapshot_source: values.snapshot_as_of_date ? 'assessment_period' : 'current_user',
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
        target_employee_ids: targetEmployeeIDs,
        manager_assignments: managerAssignments,
        default_assessment_manager_source: values.default_assessment_manager_source || 'DIRECT_MANAGER',
        indicator_library_id: values.indicator_library_id,
        description: values.description,
        enable_bonus_score: values.enable_bonus_score || false,
        strict_time_mode: values.strict_time_mode || false,
        previous_review_activity_id: flowType === 'new' ? values.previous_review_activity_id : undefined,
      }
      if (editingActivity) {
        await performanceAPI.updateActivity(editingActivity.id, data)
        message.success('更新成功')
      } else {
        const res: any = await performanceAPI.createActivity(data)
        const resData = res?.data || res
        const createdActivity = (resData?.activity || resData) as PerformanceActivity | undefined
        message.success('创建成功')
        closeActivityEditor()
        await loadActivities()
        if (isCreating && createdActivity?.id) {
          const detailRes: any = await performanceAPI.getActivity(createdActivity.id)
          const latestActivity = detailRes?.data?.activity || detailRes?.data || detailRes || createdActivity
          await loadActivityDetail(latestActivity)
        }
        return
      }
      closeActivityEditor()
      await loadActivities()
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
    setRejectGoalTarget(record)
    setRejectGoalModalVisible(true)
  }

  const renderPermissionButton = (
    permissionCode: string,
    button: React.ReactElement,
    _permissionName = PERFORMANCE_PERMISSION_LABELS[permissionCode] || permissionCode,
  ) => {
    if (hasPermission(permissionCode)) return button
    return null
  }

  const renderActivityManageButton = (button: React.ReactElement) =>
    renderPermissionButton('performance:activity:manage', button)

  const renderDistributionButton = (button: React.ReactElement) =>
    renderPermissionButton('performance:distribution:manage', button)

  const renderManagerEvalButton = (button: React.ReactElement) =>
    renderPermissionButton('performance:manager_eval:submit', button)

  const loadAssessmentManagerCandidates = async (keyword = '', participantId?: number, source?: AssessmentManagerSource) => {
    if (!currentActivity) return
    setManagerCandidateLoading(true)
    try {
      const managerSource = source || managerForm.getFieldValue('manager_source')
      const res: any = await performanceAPI.getAssessmentManagerCandidates(currentActivity.id, {
        participant_id: participantId,
        source: managerSource,
        keyword,
        limit: 30,
      })
      const data = res?.data?.data || res?.data || res
      setManagerCandidates(data?.items || [])
      setManagerCandidateSources(data?.sources || [])
    } catch (err: any) {
      setManagerCandidates([])
      setManagerCandidateSources([])
      message.error(err?.response?.data?.message || '考核上级候选人加载失败')
    } finally {
      setManagerCandidateLoading(false)
    }
  }

  const openAssessmentManagerModal = (record?: PerformanceParticipant) => {
    if (!currentActivity) return
    if (!record && selectedParticipantIds.length === 0) {
      message.warning('请先选择参与人')
      return
    }
    setManagerModalMode(record ? 'single' : 'batch')
    setManagerTargetParticipant(record || null)
    if (!users.length) {
      loadScopeOptions()
    }
    const managerSource = getAdjustableManagerSource(record?.manager_source)
    const currentManagerID = record?.manager_id || undefined
    const currentManagerIsSelfFinal = Boolean(
      record &&
      managerSource === 'MANUAL' &&
      valueMatchesParticipant(currentManagerID, record),
    )
    managerForm.setFieldsValue({
      manager_user_id: valueMatchesParticipant(currentManagerID, record) && !currentManagerIsSelfFinal ? undefined : currentManagerID,
      manager_source: managerSource,
      reason: '',
    })
    setSelectedManagerSource(managerSource)
    setManagerModalVisible(true)
    loadAssessmentManagerCandidates('', record?.id, managerSource)
  }

  const handleAssessmentManagerSearch = (keyword: string) => {
    loadAssessmentManagerCandidates(keyword, managerTargetParticipant?.id)
  }

  const handleAssessmentManagerSourceChange = (source: AssessmentManagerSource) => {
    setSelectedManagerSource(source)
    managerForm.setFieldsValue({ manager_user_id: undefined })
    setManagerCandidates([])
    setManagerCandidateSources([])
    loadAssessmentManagerCandidates('', managerTargetParticipant?.id, source)
  }

  const handleSaveAssessmentManager = async () => {
    if (!currentActivity) return
    try {
      const values = await managerForm.validateFields()
      if (valueMatchesAnyParticipant(values.manager_user_id, managerTargetParticipants)) {
        const currentManagerIsSelfFinal = Boolean(
          managerModalMode === 'single' &&
          managerTargetParticipant &&
          getAdjustableManagerSource(managerTargetParticipant.manager_source) === 'MANUAL' &&
          valueMatchesParticipant(managerTargetParticipant.manager_id, managerTargetParticipant) &&
          valueMatchesParticipant(values.manager_user_id, managerTargetParticipant),
        )
        const isAllowedSelfFinal =
          managerModalMode === 'single' &&
          values.manager_source === 'MANUAL' &&
          (valueIsAllowedSelfFinalCandidate(values.manager_user_id, managerCandidates) || currentManagerIsSelfFinal)
        if (!isAllowedSelfFinal) {
          const names = managerTargetParticipants
            .filter(participant => valueMatchesParticipant(values.manager_user_id, participant))
            .map(participant => participant?.employee_name)
            .filter(Boolean)
            .join('、')
          message.error(names ? `只有最高级或无可用组织上级人员可选择本人自评即终评：${names}` : '只有最高级或无可用组织上级人员可选择本人自评即终评')
          return
        }
      }
      setManagerUpdating(true)
      if (managerModalMode === 'single' && managerTargetParticipant) {
        await performanceAPI.updateAssessmentManager(managerTargetParticipant.id, {
          manager_user_id: values.manager_user_id,
          manager_source: values.manager_source,
          reason: values.reason,
        })
        message.success('考核上级已更新')
      } else {
        const items = selectedParticipantIds.map(id => ({
          participant_id: Number(id),
          manager_user_id: values.manager_user_id,
          manager_source: values.manager_source,
          reason: values.reason,
        }))
        const res: any = await performanceAPI.batchUpdateAssessmentManagers(currentActivity.id, items)
        const data = res?.data?.data || res?.data || res
        const results = data?.results || []
        const successCount = results.filter((item: any) => item.success).length
        const failedCount = results.length - successCount
        if (failedCount > 0) {
          message.warning(`已更新 ${successCount} 人，${failedCount} 人失败`)
        } else {
          message.success(`已更新 ${successCount} 人`)
        }
      }
      setManagerModalVisible(false)
      setSelectedParticipantIds([])
      managerForm.resetFields()
      reloadParticipants(currentActivity.id)
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '考核上级更新失败')
    } finally {
      setManagerUpdating(false)
    }
  }

  const renderDetailActionButtons = (activity: PerformanceActivity) => {
    const actions: React.ReactNode[] = []
    const activityManage = (button: React.ReactElement) => renderActivityManageButton(button)

    if (!['locked', 'archived'].includes(activity.status)) {
      actions.push(
        renderPermissionButton(
          'performance:assessment_manager:batch_update',
          <Button key="batch-manager" size="small" onClick={() => openAssessmentManagerModal()}>批量调整考核上级</Button>,
        ),
      )
    }

    if (activity.status === 'draft') {
      actions.push(
        activityManage(<Button key="edit-participants" size="small" onClick={() => { setDetailDrawerVisible(false); openActivityModal(activity) }}>编辑参与人</Button>),
        activityManage(<Button key="open-target-setting" type="primary" size="small" onClick={() => handleActivityAction('open-target-setting', activity)}>开启目标设定</Button>),
        activityManage(<Button key="publish" size="small" onClick={() => handleActivityAction('publish', activity)}>直接开启自评</Button>),
      )
    }
    if (activity.status === 'target_setting') {
      actions.push(activityManage(<Button key="open-self-evaluation" type="primary" size="small" onClick={() => handleActivityAction('open-self-evaluation', activity)}>开启自评</Button>))
    }
    if (activity.status === 'self_evaluation') {
      actions.push(
        activityManage(<Button key="open-manager-evaluation" type="primary" size="small" onClick={() => handleActivityAction('open-manager-evaluation', activity)}>开启主管评分</Button>),
        activityManage(<Button key="send-self-reminder" size="small" onClick={async () => { try { await performanceAPI.sendSelfEvalReminder(activity.id); message.success('已发送自评提醒') } catch (err: any) { message.error(err?.response?.data?.message || '发送提醒失败') } }}>提醒自评</Button>),
      )
    }
    if (activity.status === 'manager_evaluation') {
      actions.push(
        activityManage(<Button key="open-employee-confirmation" type="primary" size="small" onClick={() => handleActivityAction('open-employee-confirmation', activity)}>开启员工确认</Button>),
        renderDistributionButton(<Button key="distribution" size="small" onClick={() => setDistributionModalVisible(true)}>强制分布</Button>),
        renderManagerEvalButton(<Button key="batch-eval" size="small" onClick={() => { const selectable = participants.filter(p => (p.status === 'self_submitted' || p.status === 'manager_submitted') && isAssessmentManagerConfigured(p)); setBatchEvalSelected(selectable.map(p => p.id)); setBatchEvalModalVisible(true) }}>批量评分</Button>),
        activityManage(<Button key="send-manager-reminder" size="small" onClick={async () => { try { await performanceAPI.sendManagerEvalReminder(activity.id); message.success('已发送评分提醒') } catch (err: any) { message.error(err?.response?.data?.message || '发送提醒失败') } }}>提醒评分</Button>),
      )
    }
    if (activity.status === 'employee_confirmation') {
      actions.push(activityManage(<Button key="open-manager-confirmation" type="primary" size="small" onClick={() => handleActivityAction('open-manager-confirmation', activity)}>开启主管确认</Button>))
    }
    if (activity.status === 'manager_confirmation') {
      actions.push(activityManage(<Button key="open-hr-confirmation" type="primary" size="small" onClick={() => handleActivityAction('open-hr-confirmation', activity)}>开启HR确认</Button>))
    }
    if (activity.status === 'hr_confirmation') {
      actions.push(activityManage(<Button key="send-hr-reminder" size="small" onClick={async () => { try { await performanceAPI.sendHRConfirmReminder(activity.id); message.success('已发送HR确认提醒') } catch (err: any) { message.error(err?.response?.data?.message || '发送提醒失败') } }}>提醒HR确认</Button>))
      if (hrDeadlineStatus?.can_force_lock) {
        actions.push(activityManage(<Button key="force-lock-overdue" danger size="small" onClick={() => handleForceLockOverdueHR(activity)}>逾期强制锁定</Button>))
      }
      actions.push(activityManage(<Button key="lock" type="primary" danger size="small" onClick={() => handleActivityAction('lock', activity)}>锁定活动</Button>))
    }
    if (activity.status === 'locked' || activity.status === 'result_confirmed') {
      actions.push(activityManage(<Button key="archive" size="small" onClick={() => handleActivityAction('archive', activity)}>归档活动</Button>))
    }
    return actions
  }

  // 打开活动表单
  const openActivityModal = (activity?: PerformanceActivity) => {
    setEditingActivity(activity || null)
    loadScopeOptions()
    loadIndicatorLibraries()
    loadPerformanceTemplates()
    loadPreviousActivities(activity?.id)
    if (activity) {
      const targetEmployeeIDs = normalizeIDArray(activity.target_employee_ids)
      setImportedManagerAssignments(normalizeImportedManagerAssignments(
        activity.manager_assignments,
        targetEmployeeIDs.length ? targetEmployeeIDs : undefined,
      ))
      setImportedUserOptions(getImportedUserOptions(null, targetEmployeeIDs))
      form.setFieldsValue({
        name: activity.name,
        cycle_type: activity.cycle_type,
        template_id: activity.template_id,
        flow_type: activity.flow_type || 'old',
        organization_id: activity.organization_id || '',
        applicable_org_scope: normalizeIDArray(activity.applicable_org_scope),
        date_range: [dayjs(activity.start_date), dayjs(activity.end_date)],
        target_set_range: activity.target_set_start_at && activity.target_set_end_at ? [dayjs(activity.target_set_start_at), dayjs(activity.target_set_end_at)] : undefined,
        snapshot_as_of_date: activity.snapshot_as_of_date ? dayjs(activity.snapshot_as_of_date) : undefined,
        self_eval_range: [dayjs(activity.self_eval_start_at), dayjs(activity.self_eval_end_at)],
        manager_eval_range: [dayjs(activity.manager_eval_start_at), dayjs(activity.manager_eval_end_at)],
        result_confirm_range: [dayjs(activity.result_confirm_start_at), dayjs(activity.result_confirm_end_at)],
        employee_confirm_range: activity.employee_confirm_start_at && activity.employee_confirm_end_at ? [dayjs(activity.employee_confirm_start_at), dayjs(activity.employee_confirm_end_at)] : undefined,
        manager_confirm_range: activity.manager_confirm_start_at && activity.manager_confirm_end_at ? [dayjs(activity.manager_confirm_start_at), dayjs(activity.manager_confirm_end_at)] : undefined,
        hr_confirm_range: activity.hr_confirm_start_at && activity.hr_confirm_end_at ? [dayjs(activity.hr_confirm_start_at), dayjs(activity.hr_confirm_end_at)] : undefined,
        hr_confirm_deadline: activity.hr_confirm_deadline ? dayjs(activity.hr_confirm_deadline) : undefined,
        target_department_ids: normalizeIDArray(activity.target_department_ids),
        target_employee_ids: targetEmployeeIDs,
        default_assessment_manager_source: activity.default_assessment_manager_source || 'DIRECT_MANAGER',
        indicator_library_id: activity.indicator_library_id,
        description: activity.description,
        enable_bonus_score: activity.enable_bonus_score || false,
        strict_time_mode: activity.strict_time_mode || false,
        previous_review_activity_id: activity.previous_review_activity_id,
      })
    } else {
      setImportedUserOptions([])
      setImportedManagerAssignments([])
      form.resetFields()
      form.setFieldsValue({ default_assessment_manager_source: 'DIRECT_MANAGER', flow_type: 'old' })
    }
    setActivityModalVisible(true)
    window.requestAnimationFrame(() => {
      document.getElementById('performance-activity-editor')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  }

  const renderMyParticipantActionButton = (activity: PerformanceActivity): React.ReactNode => {
    const participant = activity.my_participant
    if (!participant) return null

    const linkStyle: React.CSSProperties = { paddingInline: 0, fontWeight: 600 }
    const activityId = activity.id
    const participantId = participant.id

    if (activity.status === 'target_setting' && PERSONAL_TARGET_STATUSES.includes(participant.status)) {
      const targetLabel = ['pending', 'target_rejected'].includes(participant.status) ? '填写目标' : '我的目标'
      return renderPermissionButton(
        'performance:goal:manage',
        <Button
          key="my-target"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-my-target-${activity.id}`}
          onClick={() => navigate(`/performance-goal-setting/${activityId}/${participantId}`)}
        >
          {targetLabel}
        </Button>,
        '绩效目标填写',
      )
    }

    const canEditSelfEvaluation = SELF_EVAL_EDITABLE_ACTIVITY_STATUSES.includes(activity.status) &&
      SELF_EVAL_EDITABLE_PARTICIPANT_STATUSES.includes(participant.status)

    if (canEditSelfEvaluation && (activity.status === 'self_evaluation' || participant.status === 'manager_recheck' || participant.status === 'manager_confirmed')) {
      return renderPermissionButton(
        'performance:self_eval:submit',
        <Button
          key="my-self-eval"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-my-self-${activity.id}`}
          onClick={() => navigate(`/performance-self-eval/${activityId}/${participantId}`)}
        >
          {PERSONAL_SELF_EVAL_STATUSES.includes(participant.status) ? '填写自评' : '修改自评'}
        </Button>,
        '绩效自评提交',
      )
    }

    if (PERSONAL_RESULT_STATUSES.includes(participant.status)) {
      const resultLabel = activity.status === 'employee_confirmation' && participant.status === 'manager_submitted'
        ? '确认结果'
        : '查看结果'
      return renderPermissionButton(
        'performance:result:view',
        <Button
          key="my-result"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-my-result-${activity.id}`}
          onClick={() => navigate(`/performance-result/${activityId}/${participantId}`)}
        >
          {resultLabel}
        </Button>,
        '绩效结果查看',
      )
    }

    return null
  }

  // 活动列表操作按钮
  const getActionButtons = (record: PerformanceActivity) => {
    const buttons: React.ReactNode[] = []
    const status = record.status
    const personalActionButton = renderMyParticipantActionButton(record)

    if (personalActionButton) {
      buttons.push(personalActionButton)
    }

    buttons.push(
        <Button size="small" type="link" data-testid={`performance-activity-view-${record.id}`} onClick={() => loadActivityDetail(record)} key="view">详情</Button>
    )

    if (status === 'draft') {
      buttons.push(
        <Button size="small" type="link" data-testid={`performance-activity-edit-${record.id}`} onClick={() => openActivityModal(record)} key="edit">编辑参与人</Button>,
        <Button size="small" type="link" data-testid={`performance-activity-open-target-${record.id}`} onClick={() => handleActivityAction('open-target-setting', record)} key="start">开启目标</Button>
      )
    } else if (status === 'target_setting') {
      buttons.push(
        <Button size="small" type="link" data-testid={`performance-activity-open-self-${record.id}`} onClick={() => handleActivityAction('open-self-evaluation', record)} key="open-self">开启自评</Button>
      )
    } else if (status === 'self_evaluation') {
      buttons.push(
        <Button size="small" type="link" data-testid={`performance-activity-notify-self-${record.id}`} onClick={() => handleActivityAction('notify-self-eval', record)} key="notify-self">提醒自评</Button>,
        <Button size="small" type="link" data-testid={`performance-activity-open-manager-${record.id}`} onClick={() => handleActivityAction('open-manager-evaluation', record)} key="open-mgr">开启主管评分</Button>
      )
    } else if (status === 'manager_evaluation') {
      buttons.push(
        <Button size="small" type="link" data-testid={`performance-activity-open-employee-confirm-${record.id}`} onClick={() => handleActivityAction('open-employee-confirmation', record)} key="confirm">员工确认</Button>
      )
    } else if (status === 'employee_confirmation') {
      buttons.push(
        <Button size="small" type="link" data-testid={`performance-activity-open-manager-confirm-${record.id}`} onClick={() => handleActivityAction('open-manager-confirmation', record)} key="manager-confirm">主管确认</Button>
      )
    } else if (status === 'manager_confirmation') {
      buttons.push(
        <Button size="small" type="link" data-testid={`performance-activity-open-hr-confirm-${record.id}`} onClick={() => handleActivityAction('open-hr-confirmation', record)} key="hr-confirm">HR确认</Button>
      )
    } else if (status === 'hr_confirmation') {
      buttons.push(
        <Button size="small" type="link" danger data-testid={`performance-activity-lock-${record.id}`} onClick={() => handleActivityAction('lock', record)} key="lock">锁定</Button>
      )
    } else if (status === 'locked') {
      buttons.push(
        <Button size="small" type="link" data-testid={`performance-activity-archive-${record.id}`} onClick={() => handleActivityAction('archive', record)} key="archive">归档</Button>
      )
    } else if (status === 'result_confirmed') {
      buttons.push(
        <Button size="small" type="link" data-testid={`performance-activity-archive-${record.id}`} onClick={() => handleActivityAction('archive', record)} key="archive">归档</Button>
      )
    }

    return buttons.map(button => (
      React.isValidElement(button) && button.key !== 'view' && !String(button.key || '').startsWith('my-')
        ? renderActivityManageButton(button)
        : button
    ))
  }

  // 活动列表 columns
  const getViewActionButtons = (record: PerformanceActivity) => {
    if (activeView === 'employee') {
      const personalActionButton = renderMyParticipantActionButton(record)
      return (
        <Space size={2} wrap>
          {personalActionButton}
          <Button size="small" type="link" data-testid={`performance-activity-view-${record.id}`} onClick={() => loadActivityDetail(record)}>详情</Button>
        </Space>
      )
    }

    if (activeView === 'manager') {
      return (
        <Space size={2} wrap>
          <Button size="small" type="link" data-testid={`performance-activity-team-${record.id}`} onClick={() => loadActivityDetail(record)}>查看团队</Button>
        </Space>
      )
    }

    return <Space size={2} wrap>{getActionButtons(record)}</Space>
  }

  const hrActivityColumns: ColumnsType<PerformanceActivity> = [
    { title: '活动名称', dataIndex: 'name', key: 'name', width: 180, ellipsis: true },
    { title: '周期', dataIndex: 'cycle_type', key: 'cycle_type', width: 80, render: (v: string) => getCycleLabel(v) },
    {
      title: '模板',
      dataIndex: 'template_id',
      key: 'template_id',
      width: 140,
      render: (_: number | undefined, record: PerformanceActivity) => {
        const templateDisplay = getActivityTemplateDisplay(record, performanceTemplateById)
        return (
          <Tooltip title={templateDisplay.tooltip}>
            <Tag color={templateDisplay.color} style={{ maxWidth: 120, overflow: 'hidden', textOverflow: 'ellipsis', verticalAlign: 'middle' }}>
              {templateDisplay.label}
            </Tag>
          </Tooltip>
        )
      }
    },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (status: string) => {
        const s = STATUS_MAP[status] || { label: status, color: 'default' }
        return <StatusTag color={s.color}>{s.label}</StatusTag>
      }
    },
    { title: '自评时间', key: 'self_eval', width: 200, render: (_, r) => `${formatDateTime(r.self_eval_start_at)} ~ ${formatDateTime(r.self_eval_end_at)}` },
    { title: '主管评分时间', key: 'mgr_eval', width: 200, render: (_, r) => `${formatDateTime(r.manager_eval_start_at)} ~ ${formatDateTime(r.manager_eval_end_at)}` },
    { title: '操作', key: 'actions', fixed: 'right', width: 280, render: (_, record) => (
      <Space size={2} wrap>{getActionButtons(record)}</Space>
    )},
  ]

  // 参与人 columns
  const employeeActivityColumns: ColumnsType<PerformanceActivity> = [
    { title: '绩效活动', dataIndex: 'name', key: 'name', width: 220, ellipsis: true },
    {
      title: '当前阶段', dataIndex: 'status', key: 'status', width: 120,
      render: (status: string) => {
        const s = STATUS_MAP[status] || { label: status, color: 'default' }
        return <StatusTag color={s.color}>{s.label}</StatusTag>
      }
    },
    {
      title: '我的状态',
      key: 'my_status',
      width: 130,
      render: (_, record) => {
        const participant = record.my_participant
        if (!participant) return <Text type="secondary">未参与</Text>
        const s = PARTICIPANT_STATUS_MAP[participant.status] || { label: participant.status, color: 'default' }
        return <StatusTag color={s.color}>{s.label}</StatusTag>
      },
    },
    {
      title: '关键时间',
      key: 'deadline',
      width: 220,
      render: (_, record) => {
        if (record.status === 'target_setting') return `${formatDateTime(record.target_set_start_at)} ~ ${formatDateTime(record.target_set_end_at)}`
        if (record.status === 'self_evaluation') return `${formatDateTime(record.self_eval_start_at)} ~ ${formatDateTime(record.self_eval_end_at)}`
        if (record.status === 'employee_confirmation') return `${formatDateTime(record.employee_confirm_start_at)} ~ ${formatDateTime(record.employee_confirm_end_at)}`
        return `${formatDateTime(record.start_date)} ~ ${formatDateTime(record.end_date)}`
      },
    },
    { title: '操作', key: 'actions', fixed: 'right', width: 160, render: (_, record) => getViewActionButtons(record) },
  ]

  const managerActivityColumns: ColumnsType<PerformanceActivity> = [
    { title: '绩效活动', dataIndex: 'name', key: 'name', width: 220, ellipsis: true },
    {
      title: '阶段', dataIndex: 'status', key: 'status', width: 120,
      render: (status: string) => {
        const s = STATUS_MAP[status] || { label: status, color: 'default' }
        return <StatusTag color={s.color}>{s.label}</StatusTag>
      }
    },
    {
      title: '团队处理重点',
      key: 'manager_focus',
      width: 180,
      render: (_, record) => {
        const focusMap: Record<string, { label: string; color: string }> = {
          target_setting: { label: '目标审核/跟进', color: 'cyan' },
          manager_evaluation: { label: '主管评分', color: 'orange' },
          manager_confirmation: { label: '主管确认/复核', color: 'gold' },
          employee_confirmation: { label: '等待员工确认', color: 'blue' },
        }
        const item = focusMap[record.status] || { label: '查看团队进度', color: 'default' }
        return <Tag color={item.color}>{item.label}</Tag>
      },
    },
    { title: '评分时间', key: 'mgr_eval', width: 220, render: (_, r) => `${formatDateTime(r.manager_eval_start_at)} ~ ${formatDateTime(r.manager_eval_end_at)}` },
    { title: '操作', key: 'actions', fixed: 'right', width: 150, render: (_, record) => getViewActionButtons(record) },
  ]

  const activityColumns =
    activeView === 'employee' ? employeeActivityColumns :
    activeView === 'manager' ? managerActivityColumns :
    hrActivityColumns

  const renderEmployeeParticipantActions = (record: PerformanceParticipant) => {
    if (!currentActivity || !participantMatchesCurrentUser(record, currentUser)) return null
    return renderMyParticipantActionButton({
      ...currentActivity,
      my_participant: record,
    })
  }

  const renderManagerParticipantActions = (record: PerformanceParticipant) => {
    const activityId = currentActivity?.id
    const activityStatus = currentActivity?.status
    if (!activityId || !participantManagedByCurrentUser(record, currentUser)) return null

    const links: React.ReactNode[] = []
    const linkStyle = { fontSize: 'var(--font-size-sm)', padding: '0 2px' }

    if (activityStatus === 'target_setting' && ['pending', 'target_pending_approval', 'target_rejected', 'target_set'].includes(record.status)) {
      links.push(renderPermissionButton(
        'performance:goal:manage',
        <Button key="target" size="small" type="link" style={linkStyle} onClick={() => navigate(`/performance-goal-setting/${activityId}/${record.id}`)}>目标</Button>,
      ))
    }

    if (record.status === 'target_pending_approval') {
      links.push(renderPermissionButton(
        'performance:goal:manage',
        <Button key="approve" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-info)' }} onClick={async () => {
          try {
            await performanceAPI.approveGoalRecords(record.id)
            message.success('目标已通过')
            if (currentActivity) loadActivityDetail(currentActivity)
          } catch (err: any) {
            message.error(err?.response?.data?.message || '审批失败')
          }
        }}>通过</Button>,
      ))
      links.push(renderPermissionButton(
        'performance:goal:manage',
        <Button key="reject" size="small" type="link" danger style={linkStyle} onClick={() => handleRejectGoalRecords(record)}>驳回</Button>,
      ))
    }

    if (activityStatus === 'manager_evaluation' && ['self_submitted', 'manager_submitted'].includes(record.status)) {
      const managerEvalBlockedReason = getManagerEvaluationBlockedReason(record)
      links.push(renderPermissionButton(
        'performance:manager_eval:submit',
        managerEvalBlockedReason ? (
          <Tooltip key="mgr" title={managerEvalBlockedReason}>
            <span>
              <Button size="small" type="link" disabled style={linkStyle}>评分</Button>
            </span>
          </Tooltip>
        ) : (
          <Button key="mgr" size="small" type="link" style={linkStyle} onClick={() => navigate(`/performance-manager-eval/${activityId}/${record.id}`)}>评分</Button>
        ),
      ))
    }

    if (['manager_confirmation', 'hr_confirmation'].includes(activityStatus || '') && record.status === 'manager_recheck') {
      const managerEvalBlockedReason = getManagerEvaluationBlockedReason(record)
      links.push(renderPermissionButton(
        'performance:manager_confirm:submit',
        managerEvalBlockedReason ? (
          <Tooltip key="manager-recheck" title={managerEvalBlockedReason}>
            <span>
              <Button size="small" type="link" disabled style={linkStyle}>复核</Button>
            </span>
          </Tooltip>
        ) : (
          <Button key="manager-recheck" size="small" type="link" style={linkStyle} onClick={() => navigate(`/performance-manager-eval/${activityId}/${record.id}`)}>复核</Button>
        ),
      ))
    }

    if (PERSONAL_RESULT_STATUSES.includes(record.status)) {
      links.push(renderPermissionButton(
        'performance:result:view',
        <Button key="result" size="small" type="link" style={linkStyle} onClick={() => navigate(`/performance-result/${activityId}/${record.id}`)}>结果</Button>,
      ))
    }

    return links.length ? <Space size={0}>{links}</Space> : <Text type="secondary">暂无操作</Text>
  }

  const hrParticipantColumns: ColumnsType<PerformanceParticipant> = [
    { title: '员工', dataIndex: 'employee_name', key: 'employee_name', width: 80 },
    { title: '部门', dataIndex: 'department_name', key: 'department_name', width: 110, ellipsis: true },
    { title: '岗位', dataIndex: 'position', key: 'position', width: 90, ellipsis: true },
    {
      title: '考核上级', dataIndex: 'manager_name', key: 'manager_name', width: 100,
      render: (name: string, record: PerformanceParticipant) => {
        if (!name && (record.manager_id === null || record.manager_id === undefined || record.manager_id === '')) {
          return (
            <Tooltip title="该参与人未设置考核上级，经理侧评分与确认将不可操作">
              <StatusTag color="warning" style={{ cursor: 'default' }}>待配置考核上级</StatusTag>
            </Tooltip>
          )
        }
        return name || '-'
      }
    },
    { title: '当期直属主管', dataIndex: 'direct_manager_name_snapshot', key: 'direct_manager_name_snapshot', width: 110, render: (name: string) => name || '-' },
    {
      title: '来源', dataIndex: 'manager_source', key: 'manager_source', width: 110,
      render: (source: AssessmentManagerSource, record: PerformanceParticipant) => (
        <Space size={4} wrap>
          <Tag color={source === 'EMPTY' ? 'default' : record.manager_overridden ? 'orange' : 'blue'}>
            {MANAGER_SOURCE_LABELS[source] || source || '-'}
          </Tag>
          {record.manager_overridden && <Tag color="gold">已调整</Tag>}
        </Space>
      )
    },
    {
      title: '配置状态', dataIndex: 'manager_config_status', key: 'manager_config_status', width: 120,
      render: (status: string, record: PerformanceParticipant) => {
        const normalized = getEffectiveManagerConfigStatus(record) || (record.manager_id ? 'CONFIGURED' : 'PENDING')
        const config = MANAGER_CONFIG_STATUS_LABELS[normalized] || { label: normalized, color: 'default' }
        return <Tag color={config.color}>{config.label}</Tag>
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

        if (isArchived && !hasPermission('performance:result:view')) {
          return renderPermissionButton(
            'performance:result:view',
            <Button size="small" type="link" style={{ fontSize: 'var(--font-size-sm)' }}>查看</Button>
          )
        }

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

        links.push(renderPermissionButton(
          'performance:assessment_manager:update',
          <Button key="manager" size="small" type="link" style={linkStyle} onClick={() => openAssessmentManagerModal(record)}>调上级</Button>,
        ))

        if (activityStatus === 'target_setting' && ['pending', 'target_pending_approval', 'target_rejected', 'target_set'].includes(record.status) && !hasPermission('performance:goal:manage')) {
          links.push(renderPermissionButton('performance:goal:manage', <Button key="target-disabled" size="small" type="link" style={linkStyle}>目标</Button>))
        }
        const canSelfEvaluateRecord = SELF_EVAL_EDITABLE_ACTIVITY_STATUSES.includes(activityStatus || '') &&
          SELF_EVAL_EDITABLE_PARTICIPANT_STATUSES.includes(record.status)
        if (canSelfEvaluateRecord && !hasPermission('performance:self_eval:submit')) {
          links.push(renderPermissionButton('performance:self_eval:submit', <Button key="self-disabled" size="small" type="link" style={linkStyle}>自评</Button>))
        }
        if (activityStatus === 'manager_evaluation' && ['self_submitted', 'manager_submitted'].includes(record.status) && !hasPermission('performance:manager_eval:submit')) {
          links.push(renderPermissionButton('performance:manager_eval:submit', <Button key="mgr-disabled" size="small" type="link" style={linkStyle}>评分</Button>))
        }
        if (['manager_submitted', 'employee_confirmed', 'manager_recheck', 'manager_confirmed', 'hr_confirmed', 'locked', 'result_confirmed'].includes(record.status) && !hasPermission('performance:result:view')) {
          links.push(renderPermissionButton('performance:result:view', <Button key="result-disabled" size="small" type="link" style={linkStyle}>结果</Button>))
        }
        if (currentActivity?.status === 'hr_confirmation' && record.status === 'manager_confirmed' && !hasPermission('performance:hr_confirm:submit')) {
          links.push(renderPermissionButton('performance:hr_confirm:submit', <Button key="hr-confirm-disabled" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-primary)' }}>HR确认</Button>))
        }
        if (record.status === 'target_pending_approval' && !hasPermission('performance:goal:manage')) {
          links.push(renderPermissionButton('performance:goal:manage', <Button key="approve-disabled" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-info)' }}>通过</Button>))
          links.push(renderPermissionButton('performance:goal:manage', <Button key="reject-disabled" size="small" type="link" danger style={linkStyle}>驳回</Button>))
        }

        // 目标设定：活动必须处于 target_setting 状态，且参与人状态允许
        if (activityStatus === 'target_setting' && ['pending', 'target_pending_approval', 'target_rejected', 'target_set'].includes(record.status) && hasPermission('performance:goal:manage')) {
          links.push(
            <Button key="target" size="small" type="link" style={linkStyle} data-testid={`performance-participant-target-${record.id}`}
              onClick={() => navigate(`/performance-goal-setting/${activityId}/${record.id}`)}
            >目标</Button>
          )
        }
        // 新流程"补录"按钮：允许 HR 在活动进入自评/评分/确认后仍补录上一期 review 目标
        // 只要活动是新流程、参与人尚未进入锁定/归档终态，且有目标管理权限，就一直显示
        if (
          currentActivity?.flow_type === 'new'
          && !['locked', 'result_confirmed', 'archived'].includes(record.status)
          && hasPermission('performance:goal:manage')
        ) {
          links.push(
            <Button key="review-supplement" size="small" type="link" style={linkStyle} data-testid={`performance-participant-review-supplement-${record.id}`}
              onClick={() => navigate(`/performance-goal-setting/${activityId}/${record.id}?phase=review`)}
            >补录</Button>
          )
        }
        // 自评：HR确认前可修改自评，主管确认后提交会进入待领导复核
        if (canSelfEvaluateRecord && hasPermission('performance:self_eval:submit')) {
          links.push(
            <Button key="self" size="small" type="link" style={linkStyle} data-testid={`performance-participant-self-${record.id}`}
              onClick={() => navigate(`/performance-self-eval/${activityId}/${record.id}`)}
            >{PERSONAL_SELF_EVAL_STATUSES.includes(record.status) ? '自评' : '改自评'}</Button>
          )
        }
        // 主管评分：活动必须处于 manager_evaluation 状态，且参与人状态允许
        if (activityStatus === 'manager_evaluation' && ['self_submitted', 'manager_submitted'].includes(record.status) && hasPermission('performance:manager_eval:submit')) {
          const managerEvalBlockedReason = getManagerEvaluationBlockedReason(record)
          links.push(
            managerEvalBlockedReason ? (
              <Tooltip key="mgr" title={managerEvalBlockedReason}>
                <span>
                  <Button size="small" type="link" disabled style={linkStyle} data-testid={`performance-participant-manager-${record.id}`}>评分</Button>
                </span>
              </Tooltip>
            ) : (
              <Button key="mgr" size="small" type="link" style={linkStyle} data-testid={`performance-participant-manager-${record.id}`}
                onClick={() => navigate(`/performance-manager-eval/${activityId}/${record.id}`)}
              >评分</Button>
            )
          )
        }
        if (['manager_confirmation', 'hr_confirmation'].includes(activityStatus || '') && record.status === 'manager_recheck') {
          if (!hasPermission('performance:manager_confirm:submit')) {
            links.push(renderPermissionButton('performance:manager_confirm:submit', <Button key="manager-recheck-disabled" size="small" type="link" style={linkStyle}>复核</Button>))
          } else {
            const managerEvalBlockedReason = getManagerEvaluationBlockedReason(record)
            links.push(
              managerEvalBlockedReason ? (
                <Tooltip key="manager-recheck" title={managerEvalBlockedReason}>
                  <span>
                    <Button size="small" type="link" disabled style={linkStyle} data-testid={`performance-participant-manager-recheck-${record.id}`}>复核</Button>
                  </span>
                </Tooltip>
              ) : (
                <Button key="manager-recheck" size="small" type="link" style={linkStyle} data-testid={`performance-participant-manager-recheck-${record.id}`}
                  onClick={() => navigate(`/performance-manager-eval/${activityId}/${record.id}`)}
                >复核</Button>
              )
            )
          }
        }
        if (['manager_submitted', 'employee_confirmed', 'manager_recheck', 'manager_confirmed', 'hr_confirmed', 'locked', 'result_confirmed'].includes(record.status) && hasPermission('performance:result:view')) {
          links.push(
            <Button key="result" size="small" type="link" style={linkStyle} data-testid={`performance-participant-result-${record.id}`}
              onClick={() => navigate(`/performance-result/${activityId}/${record.id}`)}
            >结果</Button>
          )
        }
        if (currentActivity?.status === 'hr_confirmation' && record.status === 'manager_confirmed' && hasPermission('performance:hr_confirm:submit')) {
          links.push(
            <Button key="hr-confirm" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-primary)' }} data-testid={`performance-participant-hr-confirm-${record.id}`}
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
        if (record.status === 'target_pending_approval' && hasPermission('performance:goal:manage')) {
          links.push(
            <Button key="approve" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-info)' }} data-testid={`performance-participant-approve-${record.id}`}
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
            <Button key="reject" size="small" type="link" danger style={linkStyle} data-testid={`performance-participant-reject-${record.id}`}
              onClick={() => handleRejectGoalRecords(record)}
            >驳回</Button>
          )
        }

        return <Space size={0}>{links}</Space>
      }
    },
  ]

  // 统计数据
  const employeeParticipantColumns: ColumnsType<PerformanceParticipant> = [
    { title: '员工', dataIndex: 'employee_name', key: 'employee_name', width: 120 },
    {
      title: '我的状态',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (status: string) => {
        const s = PARTICIPANT_STATUS_MAP[status] || { label: status, color: 'default' }
        return <StatusTag color={s.color}>{s.label}</StatusTag>
      },
    },
    {
      title: '自评分',
      dataIndex: 'self_score',
      key: 'self_score',
      width: 90,
      render: (score: any) => score === null || score === undefined || score === '' ? <Text type="secondary">-</Text> : <Text strong>{String(score)}</Text>,
    },
    {
      title: '主管分',
      dataIndex: 'manager_score',
      key: 'manager_score',
      width: 90,
      render: (score: any) => score === null || score === undefined || score === '' ? <Text type="secondary">-</Text> : <Text strong>{String(score)}</Text>,
    },
    {
      title: '等级',
      dataIndex: 'final_level',
      key: 'final_level',
      width: 80,
      render: (v: string) => v ? <StatusTag color={({ S: '#f50', A: '#1677ff', B: '#52c41a', C: '#faad14', D: '#ff4d4f' } as Record<string, string>)[v] || 'default'}>{v}</StatusTag> : <Text type="secondary">-</Text>,
    },
    { title: '操作', key: 'actions', fixed: 'right', width: 130, render: (_, record) => renderEmployeeParticipantActions(record) || <Text type="secondary">暂无操作</Text> },
  ]

  const managerParticipantColumns: ColumnsType<PerformanceParticipant> = [
    { title: '员工', dataIndex: 'employee_name', key: 'employee_name', width: 100 },
    { title: '部门', dataIndex: 'department_name', key: 'department_name', width: 130, ellipsis: true },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (status: string) => {
        const s = PARTICIPANT_STATUS_MAP[status] || { label: status, color: 'default' }
        return <StatusTag color={s.color}>{s.label}</StatusTag>
      },
    },
    {
      title: '处理重点',
      key: 'manager_focus',
      width: 130,
      render: (_, record) => {
        if (record.status === 'target_pending_approval') return <Tag color="cyan">目标审核</Tag>
        if (['self_submitted', 'manager_submitted'].includes(record.status)) return <Tag color="orange">评分</Tag>
        if (record.status === 'manager_recheck') return <Tag color="gold">复核</Tag>
        if (PERSONAL_RESULT_STATUSES.includes(record.status)) return <Tag color="blue">结果</Tag>
        return <Tag>跟进</Tag>
      },
    },
    {
      title: '主管分',
      dataIndex: 'manager_score',
      key: 'manager_score',
      width: 90,
      render: (score: any) => score === null || score === undefined || score === '' ? <Text type="secondary">-</Text> : <Text strong>{String(score)}</Text>,
    },
    {
      title: '等级',
      dataIndex: 'final_level',
      key: 'final_level',
      width: 80,
      render: (v: string) => v ? <StatusTag color={({ S: '#f50', A: '#1677ff', B: '#52c41a', C: '#faad14', D: '#ff4d4f' } as Record<string, string>)[v] || 'default'}>{v}</StatusTag> : <Text type="secondary">-</Text>,
    },
    { title: '操作', key: 'actions', fixed: 'right', width: 160, render: (_, record) => renderManagerParticipantActions(record) },
  ]

  const participantColumns =
    activeView === 'employee' ? employeeParticipantColumns :
    activeView === 'manager' ? managerParticipantColumns :
    hrParticipantColumns

  const selectedActivityStatuses = React.useMemo(
    () => resolveActivityStatusFilter(activityStatusFilter),
    [activityStatusFilter],
  )
  const viewActivities = React.useMemo(
    () => activities.filter(item => {
      if (activeView === 'employee') return Boolean(item.my_participant)
      if (activeView === 'manager') {
        return canUseManagerView
      }
      return canUseHRView
    }),
    [activities, activeView, canUseHRView, canUseManagerView],
  )
  const filteredActivities = React.useMemo(
    () => viewActivities.filter(item => {
      const matchName = !activitySearchText || item.name?.toLowerCase().includes(activitySearchText.toLowerCase())
      const matchStatus = !selectedActivityStatuses || selectedActivityStatuses.includes(item.status)
      return matchName && matchStatus
    }),
    [viewActivities, activitySearchText, selectedActivityStatuses],
  )
  const handleActivityStatClick = useCallback((statusFilter?: string) => {
    setActivityStatusFilter(statusFilter)
    window.requestAnimationFrame(() => {
      activityListRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  }, [])
  const inProgressCount = viewActivities.filter(a => IN_PROGRESS_ACTIVITY_STATUSES.includes(a.status)).length
  const confirmedCount = viewActivities.filter(a => CONFIRMED_ACTIVITY_STATUSES.includes(a.status)).length
  const archivedCount = viewActivities.filter(a => a.status === 'archived').length
  const employeeTodoCount = viewActivities.filter(a => Boolean(a.my_participant && renderMyParticipantActionButton(a))).length
  const managerTodoCount = viewActivities.filter(a => ['target_setting', 'manager_evaluation', 'manager_confirmation'].includes(a.status)).length
  const activityStatCards = [
    { title: '绩效活动总数', value: activitiesTotal, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)', filter: undefined },
    { title: '进行中活动', value: inProgressCount, color: '#0369a1', bg: '#e0f2fe', filter: ACTIVITY_STATUS_FILTER_IN_PROGRESS },
    { title: '已确认结果', value: confirmedCount, color: 'var(--color-success)', bg: '#dcfce7', filter: ACTIVITY_STATUS_FILTER_CONFIRMED },
    { title: '已归档活动', value: archivedCount, color: 'var(--color-text-secondary)', bg: 'var(--color-bg-hover)', filter: 'archived' },
  ]
  const roleActivityStatCards = activeView === 'employee' ? [
    { title: '我的绩效活动', value: viewActivities.length, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)', filter: undefined },
    { title: '待我处理', value: employeeTodoCount, color: '#b45309', bg: '#fef3c7', filter: undefined },
    { title: '进行中', value: inProgressCount, color: '#0369a1', bg: '#e0f2fe', filter: ACTIVITY_STATUS_FILTER_IN_PROGRESS },
    { title: '已完成', value: confirmedCount, color: 'var(--color-success)', bg: '#dcfce7', filter: ACTIVITY_STATUS_FILTER_CONFIRMED },
  ] : activeView === 'manager' ? [
    { title: '团队绩效活动', value: viewActivities.length, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)', filter: undefined },
    { title: '待团队处理', value: managerTodoCount, color: '#b45309', bg: '#fef3c7', filter: undefined },
    { title: '评分阶段', value: viewActivities.filter(a => a.status === 'manager_evaluation').length, color: '#0369a1', bg: '#e0f2fe', filter: 'manager_evaluation' },
    { title: '已完成', value: confirmedCount, color: 'var(--color-success)', bg: '#dcfce7', filter: ACTIVITY_STATUS_FILTER_CONFIRMED },
  ] : [
    { title: '绩效活动总数', value: activitiesTotal, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)', filter: undefined },
    { title: '进行中活动', value: inProgressCount, color: '#0369a1', bg: '#e0f2fe', filter: ACTIVITY_STATUS_FILTER_IN_PROGRESS },
    { title: '已确认结果', value: confirmedCount, color: 'var(--color-success)', bg: '#dcfce7', filter: ACTIVITY_STATUS_FILTER_CONFIRMED },
    { title: '已归档活动', value: archivedCount, color: 'var(--color-text-secondary)', bg: 'var(--color-bg-hover)', filter: 'archived' },
  ]
  const viewMeta = PERFORMANCE_VIEW_META[activeView]
  const primaryTodoCount = activeView === 'employee'
    ? employeeTodoCount
    : activeView === 'manager'
      ? managerTodoCount
      : inProgressCount
  const primaryTodoLabel = activeView === 'employee'
    ? '待我处理'
    : activeView === 'manager'
      ? '待团队处理'
      : '进行中活动'
  const latestActivity = filteredActivities[0] || viewActivities[0]
  const completionPercent = viewActivities.length > 0 ? Math.round((confirmedCount / viewActivities.length) * 100) : 0
  const workbenchInsights = [
    {
      label: primaryTodoLabel,
      value: primaryTodoCount,
      hint: activeView === 'hr' ? '需要关注阶段推进' : '优先处理这些事项',
      icon: <ClockCircleOutlined />,
      color: '#b45309',
      bg: '#fffbeb',
    },
    {
      label: '最近活动',
      value: latestActivity?.name || '暂无活动',
      hint: latestActivity ? STATUS_MAP[latestActivity.status]?.label || latestActivity.status : '当前视角暂无数据',
      icon: <BarChartOutlined />,
      color: viewMeta.accent,
      bg: viewMeta.softBg,
    },
    {
      label: '完成率',
      value: `${completionPercent}%`,
      hint: `${confirmedCount}/${viewActivities.length || 0} 已完成`,
      icon: <CheckCircleOutlined />,
      color: 'var(--color-success)',
      bg: '#f0fdf4',
    },
  ]
  const detailParticipants = React.useMemo(() => {
    if (activeView === 'employee') {
      const mine = participants.filter(participant => participantMatchesCurrentUser(participant, currentUser))
      if (mine.length > 0) return mine
      return currentActivity?.my_participant ? [currentActivity.my_participant] : []
    }
    if (activeView === 'manager') {
      return participants
    }
    return participants
  }, [activeView, currentActivity, currentUser, participants])
  const detailSummaryCards = React.useMemo(() => {
    if (activeView === 'hr' && summary) {
      return [
        { title: '参与人数', value: summary.total_participants, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)' },
        { title: '已自评', value: summary.self_submitted_count, color: '#0369a1', bg: '#e0f2fe' },
        {
          title: '已评分',
          value: summary.manager_submitted_count + participants.filter(record =>
            record.status === 'self_submitted' && isSelfFinalAssessmentRecord(record)
          ).length,
          color: '#b45309',
          bg: '#fef3c7',
        },
        { title: '已确认', value: summary.result_confirmed_count, color: 'var(--color-success)', bg: '#dcfce7' },
      ]
    }

    return [
      { title: activeView === 'employee' ? '我的记录' : '负责员工', value: detailParticipants.length, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)' },
      { title: '待目标', value: detailParticipants.filter(record => ['pending', 'target_pending_approval', 'target_rejected', 'target_set'].includes(record.status)).length, color: '#0891b2', bg: '#cffafe' },
      { title: '待评分', value: detailParticipants.filter(record => ['self_submitted', 'manager_submitted', 'manager_recheck'].includes(record.status)).length, color: '#b45309', bg: '#fef3c7' },
      { title: '已确认', value: detailParticipants.filter(record => ['employee_confirmed', 'manager_confirmed', 'hr_confirmed', 'locked', 'result_confirmed'].includes(record.status)).length, color: 'var(--color-success)', bg: '#dcfce7' },
    ]
  }, [activeView, detailParticipants, participants, summary])
  const detailActions = activeView === 'hr' && currentActivity
    ? renderDetailActionButtons(currentActivity)
    : null

  const activityListActions = (
    <Space>
      {activeView === 'hr' && hasPermission('performance:activity:manage') && (
        <Button type="primary" data-testid="performance-create-activity" icon={<PlusOutlined />} onClick={() => openActivityModal()}>新建活动</Button>
      )}
      <Button data-testid="performance-refresh-activities" icon={<ReloadOutlined />} onClick={() => loadActivities()} disabled={activitiesLoading}>刷新</Button>
    </Space>
  )

  return (
    <PageContainer
      data-testid="performance-overview-page"
      title="绩效管理"
      icon={<BarChartOutlined />}
      subtitle="绩效活动管理与评分工作台"
    >

      <Card
        data-testid="performance-overview-card"
        style={{ borderRadius: 'var(--radius-xl)', border: '1px solid var(--color-border)', boxShadow: 'var(--shadow-card)' }}
        styles={{ header: { background: 'var(--color-bg-card-header)', borderBottom: '1px solid var(--color-border-light)' } }}
      >
        <div
          style={{
            margin: '-8px -8px 18px',
            padding: '22px 24px',
            borderRadius: 'var(--radius-lg)',
            background: `linear-gradient(135deg, ${viewMeta.softBg} 0%, #ffffff 58%, #f8fafc 100%)`,
            border: '1px solid var(--color-border-light)',
          }}
        >
          <div
            style={{
              display: 'flex',
              alignItems: 'flex-start',
              justifyContent: 'space-between',
              gap: 16,
              flexWrap: 'wrap',
              marginBottom: 18,
            }}
          >
            <Space align="start" size={12}>
              <div
                style={{
                  width: 46,
                  height: 46,
                  borderRadius: 'var(--radius-md)',
                  background: viewMeta.accent,
                  color: '#fff',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 22,
                  flexShrink: 0,
                }}
              >
                {viewMeta.icon}
              </div>
              <div>
                <Text strong style={{ fontSize: 22, color: 'var(--color-text-title)' }}>
                  {viewMeta.title}工作台
                </Text>
                <div style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-sm)', marginTop: 6 }}>
                  {viewMeta.description}
                </div>
              </div>
            </Space>
            <Segmented
              value={activeView}
              options={performanceViewOptions}
              onChange={(value) => {
                setActiveView(value as PerformanceView)
                setActivityStatusFilter(undefined)
                setSelectedParticipantIds([])
              }}
            />
          </div>

          <Row gutter={[12, 12]}>
            {workbenchInsights.map((item) => (
              <Col xs={24} md={8} key={item.label}>
                <div
                  style={{
                    minHeight: 92,
                    padding: '14px 16px',
                    background: '#fff',
                    border: '1px solid var(--color-border-light)',
                    borderRadius: 'var(--radius-md)',
                    display: 'flex',
                    alignItems: 'center',
                    gap: 12,
                  }}
                >
                  <div
                    style={{
                      width: 38,
                      height: 38,
                      borderRadius: 'var(--radius-md)',
                      background: item.bg,
                      color: item.color,
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontSize: 18,
                      flexShrink: 0,
                    }}
                  >
                    {item.icon}
                  </div>
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-xs)' }}>{item.label}</div>
                    <div style={{ color: 'var(--color-text-title)', fontSize: 20, fontWeight: 'var(--font-weight-bold)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {item.value}
                    </div>
                    <div style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)', marginTop: 2 }}>{item.hint}</div>
                  </div>
                  {item.label === '完成率' && (
                    <Progress type="circle" percent={completionPercent} size={46} strokeColor={String(item.color)} />
                  )}
                </div>
              </Col>
            ))}
          </Row>
        </div>

        <Row gutter={[12, 12]} style={{ marginBottom: 18 }}>
          {roleActivityStatCards.map((item) => {
            const active = item.filter !== undefined && activityStatusFilter === item.filter
            return (
              <Col xs={24} sm={12} lg={6} key={item.title}>
                <button
                  type="button"
                  aria-label={`查看${item.title}`}
                  onClick={() => handleActivityStatClick(item.filter)}
                  style={{
                    background: active ? item.bg : 'var(--color-bg-card)',
                    borderRadius: 'var(--radius-md)',
                    padding: '14px 16px',
                    border: active ? `1px solid ${item.color}` : '1px solid var(--color-border-light)',
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'flex-start',
                    gap: 8,
                    width: '100%',
                    minHeight: 104,
                    font: 'inherit',
                    textAlign: 'left',
                    cursor: 'pointer',
                    transition: 'border-color 0.2s, background 0.2s, transform 0.2s',
                  }}
                >
                  <div style={{
                    width: 34, height: 34, borderRadius: 'var(--radius-md)', background: item.bg,
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    fontSize: 18, color: item.color, fontWeight: 'var(--font-weight-bold)', flexShrink: 0,
                  }}>
                    {item.value}
                  </div>
                  <Text style={{ color: 'var(--color-text)', fontSize: 'var(--font-size-sm)', fontWeight: 'var(--font-weight-medium)' }}>{item.title}</Text>
                </button>
              </Col>
            )
          })}
        </Row>

          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              flexWrap: 'wrap',
              gap: 12,
              marginBottom: 16,
            }}
          >
            <Text strong style={{ fontSize: 16, color: 'var(--color-text-title)' }}>
              待办与活动
            </Text>
            {activityListActions}
          </div>

          <PerformanceActivityEditor
            visible={activityModalVisible}
            editing={Boolean(editingActivity)}
            form={form}
            saving={activitySaving}
            performanceTemplates={performanceTemplates}
            performanceTemplatesLoading={performanceTemplatesLoading}
            indicatorLibraries={indicatorLibraries}
            indicatorLibrariesLoading={indicatorLibrariesLoading}
            departmentOptions={departmentOptions}
            userOptions={userOptions}
            scopeOptionsLoading={scopeOptionsLoading}
            importingParticipants={participantImporting}
            previousActivityOptions={previousActivityOptions}
            previousActivityLoading={previousActivityLoading}
            onImportParticipants={handleImportParticipants}
            onSave={handleSaveActivity}
            onCancel={closeActivityEditor}
          />

          {/* 活动列表 */}
          <div ref={activityListRef}>
          <PageCard
            data-testid="performance-activity-list"
            style={{ marginBottom: 16 }}
          >
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: 12,
                flexWrap: 'wrap',
                padding: '10px 12px',
                marginBottom: 14,
                borderRadius: 'var(--radius-md)',
                background: '#f8fafc',
                border: '1px solid var(--color-border-light)',
              }}
            >
              <div>
                <Text strong style={{ color: 'var(--color-text-title)' }}>{viewMeta.title}列表</Text>
                <div style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-xs)', marginTop: 2 }}>
                  共 {filteredActivities.length} 条，按当前角色显示最相关字段
                </div>
              </div>
              <Space wrap>
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
                  options={ACTIVITY_STATUS_FILTER_OPTIONS}
                />
              </Space>
            </div>
            <Spin spinning={activitiesLoading}>
              <Table
                columns={activityColumns}
                dataSource={filteredActivities}
                rowKey="id"
                pagination={{ pageSize: 10 }}
                size="small"
                scroll={{ x: 900 }}
              />
            </Spin>
          </PageCard>
          </div>

      </Card>

      {/* 活动详情抽屉 */}
      <Drawer
        title={`活动详情：${currentActivity?.name || ''}`}
        placement="right"
        width={1000}
        open={detailDrawerVisible}
        onClose={() => { setDetailDrawerVisible(false); setCurrentActivity(null); setParticipants([]); setSelectedParticipantIds([]); setManagerModalVisible(false); setSummary(null); setDistributionCheck(null); setDistributionRules([]); setHrDeadlineStatus(null); }}
        styles={{ footer: { paddingTop: 12 } }}
      >
        {currentActivity && (
          <div data-testid="performance-detail-content">
            <div
              style={{
                padding: '16px 18px',
                marginBottom: 16,
                borderRadius: 'var(--radius-lg)',
                background: viewMeta.softBg,
                border: '1px solid var(--color-border-light)',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
                <Space size={12}>
                  <div
                    style={{
                      width: 42,
                      height: 42,
                      borderRadius: 'var(--radius-md)',
                      background: viewMeta.accent,
                      color: '#fff',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontSize: 20,
                    }}
                  >
                    {viewMeta.icon}
                  </div>
                  <div>
                    <Text strong style={{ fontSize: 18, color: 'var(--color-text-title)' }}>{currentActivity.name}</Text>
                    <div style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-sm)', marginTop: 4 }}>
                      {viewMeta.detailTitle} · {formatDateTime(currentActivity.start_date)} ~ {formatDateTime(currentActivity.end_date)}
                    </div>
                  </div>
                </Space>
                <Space>
                  <StatusTag color={STATUS_MAP[currentActivity.status]?.color}>{STATUS_MAP[currentActivity.status]?.label}</StatusTag>
                  <Tag color={currentActivity.flow_type === 'new' ? 'purple' : 'default'}>
                    {currentActivity.flow_type === 'new' ? '新流程' : '旧流程'}
                  </Tag>
                </Space>
              </div>
            </div>
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
              <Descriptions.Item label="流程类型">
                <Tag color={currentActivity.flow_type === 'new' ? 'purple' : 'default'}>
                  {currentActivity.flow_type === 'new' ? '新流程' : '旧流程'}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="绩效周期">{formatDateTime(currentActivity.start_date)} ~ {formatDateTime(currentActivity.end_date)}</Descriptions.Item>
              <Descriptions.Item label="自评时间">{formatDateTime(currentActivity.self_eval_start_at)} ~ {formatDateTime(currentActivity.self_eval_end_at)}</Descriptions.Item>
              <Descriptions.Item label="主管评分">{formatDateTime(currentActivity.manager_eval_start_at)} ~ {formatDateTime(currentActivity.manager_eval_end_at)}</Descriptions.Item>
              <Descriptions.Item label="结果确认">{formatDateTime(currentActivity.result_confirm_start_at)} ~ {formatDateTime(currentActivity.result_confirm_end_at)}</Descriptions.Item>
            </Descriptions>

            {/* 操作按钮 - 紧凑布局 */}
            {detailActions && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 16 }}>
              {detailActions}
              {false && (
                <>
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
                    <Button size="small" onClick={() => { const selectable = participants.filter(p => (p.status === 'self_submitted' || p.status === 'manager_submitted') && isAssessmentManagerConfigured(p)); setBatchEvalSelected(selectable.map(p => p.id)); setBatchEvalModalVisible(true) }}>批量评分</Button>
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
                </>
              )}
            </div>
            )}

            <Divider style={{ margin: '8px 0 10px' }} orientationMargin={0}>统计摘要</Divider>
            {activeView === 'hr' && currentActivity.status === 'hr_confirmation' && hrDeadlineStatus && (
              <Alert
                type={hrDeadlineStatus.overdue ? 'warning' : 'info'}
                showIcon
                style={{ marginBottom: 12 }}
                message={`HR确认截止：${hrDeadlineStatus.deadline || '未设置'}，待确认 ${hrDeadlineStatus.pending_count || 0} 人${hrDeadlineStatus.overdue ? '，已逾期' : ''}`}
              />
            )}

            <Spin spinning={summaryLoading}>
              {detailSummaryCards.length > 0 ? (
                <Row gutter={[10, 10]} style={{ marginBottom: 10 }}>
                  {detailSummaryCards.map((item, idx) => (
                    <Col xs={12} md={6} key={item.title}>
                      <div style={{
                        minHeight: 78,
                        padding: '12px 14px',
                        borderRadius: 'var(--radius-md)',
                        background: item.bg,
                        border: `1px solid ${idx === 0 ? viewMeta.accent : 'var(--color-border-light)'}`,
                      }}>
                        <div style={{ fontSize: 24, fontWeight: 'var(--font-weight-bold)', color: item.color, lineHeight: 1.2 }}>{item.value}</div>
                        <div style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-secondary)', marginTop: 6 }}>{item.title}</div>
                      </div>
                    </Col>
                  ))}
                </Row>
              ) : <Text type="secondary">暂无数据</Text>}
            </Spin>

            {activeView === 'hr' && distributionCheck && (
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

            <Divider style={{ margin: '12px 0' }}>{PERFORMANCE_VIEW_META[activeView].detailTitle}</Divider>
            <Spin spinning={participantsLoading}>
              <Table
                columns={participantColumns}
                dataSource={detailParticipants}
                rowKey="id"
                rowSelection={activeView === 'hr' && hasPermission('performance:assessment_manager:batch_update') ? {
                  selectedRowKeys: selectedParticipantIds,
                  onChange: setSelectedParticipantIds,
                } : undefined}
                pagination={{ pageSize: 10, size: 'small' }}
                size="small"
                scroll={{ x: activeView === 'hr' ? 1250 : 720 }}
              />
            </Spin>
          </div>
        )}
      </Drawer>

      <Modal
        title="驳回目标"
        open={rejectGoalModalVisible}
        okText="确认驳回"
        okButtonProps={{ danger: true }}
        cancelText="取消"
        onCancel={() => {
          setRejectGoalModalVisible(false)
          setRejectGoalTarget(null)
          rejectGoalForm.resetFields()
        }}
        onOk={async () => {
          if (!rejectGoalTarget) return
          const values = await rejectGoalForm.validateFields()
          try {
            await performanceAPI.rejectGoalRecords(rejectGoalTarget.id, { comment: values.comment })
            message.success('目标已驳回')
            setRejectGoalModalVisible(false)
            setRejectGoalTarget(null)
            rejectGoalForm.resetFields()
            if (currentActivity) loadActivityDetail(currentActivity)
          } catch (err: any) {
            message.error(err?.response?.data?.message || '驳回失败')
            throw err
          }
        }}
      >
        <Form form={rejectGoalForm} layout="vertical" preserve={false}>
          <Form.Item
            name="comment"
            label="驳回原因"
            rules={[{ required: true, message: '请输入驳回原因' }]}
          >
            <TextArea rows={3} placeholder="请说明需要员工调整的内容" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={managerModalMode === 'single' ? '调整考核上级' : '批量调整考核上级'}
        open={managerModalVisible}
        onOk={handleSaveAssessmentManager}
        onCancel={() => { setManagerModalVisible(false); managerForm.resetFields(); setManagerCandidates([]); setManagerCandidateSources([]) }}
        confirmLoading={managerUpdating}
        width={560}
      >
        <Alert
          showIcon
          type="info"
          style={{ marginBottom: 16 }}
          message={managerModalMode === 'single'
            ? `调整对象：${managerTargetParticipant?.employee_name || '-'}`
            : `已选择 ${selectedParticipantIds.length} 名参与人`}
          description={managerModalMode === 'single'
            ? '普通员工不能选择本人；最高级或无可用组织上级人员可在“手动指定”下选择本人，按自评即终评处理。'
            : '批量调整不能把任一已选参与人设置为自己的考核上级；自评即终评请单独调整。'}
        />
        <Form form={managerForm} layout="vertical">
          <Form.Item name="manager_user_id" label="考核上级" rules={[{ required: true, message: '请选择考核上级' }]}>
            <Select
              showSearch
              filterOption={(input, option) => {
                const searchText = String((option as AssessmentManagerSelectOption | undefined)?.searchText || option?.value || '')
                return searchText.toLowerCase().includes(input.trim().toLowerCase())
              }}
              onSearch={handleAssessmentManagerSearch}
              loading={managerCandidateLoading}
              placeholder="搜索姓名、UserID 或工号"
              notFoundContent={managerCandidateLoading ? <Spin size="small" /> : null}
              options={managerSelectOptions}
            />
          </Form.Item>
          <Form.Item name="manager_source" label="来源" rules={[{ required: true, message: '请选择来源' }]}>
            <Select options={MANAGER_SOURCE_OPTIONS} onChange={handleAssessmentManagerSourceChange} />
          </Form.Item>
          {selectedManagerSourceGroup?.reason ? (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
              message={selectedManagerSourceGroup.reason}
            />
          ) : selectedManagerSourceGroup ? (
            <Alert
              type="success"
              showIcon
              style={{ marginBottom: 16 }}
              message={`${selectedManagerSourceGroup.source_label}候选人 ${selectedManagerSourceGroup.items.length} 人`}
            />
          ) : null}
          <Form.Item name="reason" label="调整原因">
            <TextArea rows={3} maxLength={500} showCount placeholder="记录本次调整原因" />
          </Form.Item>
        </Form>
      </Modal>

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
            reloadParticipants(currentActivity.id)
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
