import React, { useState, useCallback, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Alert, Card, Col, Row, Space, Table, Tag, Typography, Button, Modal, Form, Input, InputNumber,
  Select, message, Spin, Drawer, Tooltip, Divider, Descriptions, Steps, Segmented, Empty
} from 'antd'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import SelfReviewHistoryDrawer from '../components/SelfReviewHistoryDrawer'
import PerformanceParticipantTable from '../components/PerformanceParticipantTable'
import PerformanceActivityDetailDrawer from '../components/PerformanceActivityDetailDrawer'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import {
  performanceAPI,
  PerformanceActivity,
  PerformanceParticipant,
  PerformanceParticipantStatus,
  PerformanceParticipantImportResult,
  PerformanceActivityManagerAssignment,
  PerformanceDistributionRule,
  PerformanceHRDeadlineStatus,
  PerformanceHRConfirmReminderResult,
  PerformanceManagerEvalReminderResult,
  PerformanceSelfEvalReminderResult,
  PerformanceReminderRecipientDetail,
  PerformanceReviewVersion,
  PerformanceIndicatorLibrary,
  PerformanceDistributionCheck,
  PerformanceResultSummary,
  PerformanceTemplate,
  AssessmentManagerCandidate,
  AssessmentManagerCandidateSourceGroup,
  AssessmentManagerSource,
} from '../services/api'
import PerformanceActivityEditor from '../components/PerformanceActivityEditor'
import PerformanceExcelImportWizard from '../components/PerformanceExcelImportWizard'
import {
  BarChartOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  FileExcelOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { getCycleLabel, formatDateTime } from '../utils/format'
import { hasPermission } from '../utils/permission'
import { useAuthStore } from '../store/authStore'
import {
  MANAGER_CONFIG_STATUS_LABELS,
  MANAGER_SOURCE_LABELS,
  formatDateRange,
  formatRangeEnd,
  formatRangeStart,
  getDepartmentOption,
  getImportedUserOption,
  getListFromResponse,
  getUserOption,
  mergeSelectOptions,
  normalizeIDArray,
  normalizeImportedManagerAssignments,
} from '../utils/performanceHelpers'

const { Text, Paragraph } = Typography
const { TextArea } = Input

const activityActionButtonStyle: React.CSSProperties = {
  height: 22,
  paddingInline: 0,
  fontSize: 'var(--font-size-sm)',
  fontWeight: 600,
  lineHeight: '22px',
  whiteSpace: 'nowrap',
}

const activityActionListStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  alignItems: 'center',
  columnGap: 14,
  rowGap: 2,
}

const participantActionColumnWidth = 220
const participantTableScrollX = 1320
const participantActionListStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  alignItems: 'center',
  columnGap: 0,
  rowGap: 0,
  maxWidth: '100%',
}
const participantActionButtonStyle: React.CSSProperties = {
  fontSize: 'var(--font-size-sm)',
  padding: '0 2px',
}

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

const PERFORMANCE_VIEW_STORAGE_KEY = 'peopleops.performance.activeView'

const PERFORMANCE_VIEW_META: Record<PerformanceView, { title: string; description: string; detailTitle: string; icon: React.ReactNode; accent: string; softBg: string }> = {
  employee: {
    title: '我的绩效',
    description: '聚焦我参与的活动、当前待办和结果确认',
    detailTitle: '我的事项',
    icon: <UserOutlined />,
    accent: 'var(--color-primary)',
    softBg: 'var(--color-primary-bg)',
  },
  manager: {
    title: '团队绩效',
    description: '聚焦团队待处理事项、评分与复核进度',
    detailTitle: '团队处理',
    icon: <TeamOutlined />,
    accent: 'var(--color-info)',
    softBg: '#ecfeff',
  },
  hr: {
    title: 'HR 管理',
    description: '维护活动配置、阶段推进和全员进度',
    detailTitle: 'HR 管控',
    icon: <SafetyCertificateOutlined />,
    accent: 'var(--color-primary-active)',
    softBg: 'var(--color-primary-bg)',
  },
}

function readStoredPerformanceView(): PerformanceView | null {
  try {
    const stored = window.localStorage.getItem(PERFORMANCE_VIEW_STORAGE_KEY)
    if (stored === 'employee' || stored === 'manager' || stored === 'hr') return stored
  } catch {
    // ignore storage access errors (private mode / SSR-like env)
  }
  return null
}

function writeStoredPerformanceView(view: PerformanceView) {
  try {
    window.localStorage.setItem(PERFORMANCE_VIEW_STORAGE_KEY, view)
  } catch {
    // ignore storage access errors
  }
}

/** Prefer last choice; otherwise HR → manager → employee by available capability. */
function resolvePreferredPerformanceView(canUseHRView: boolean, canUseManagerView: boolean): PerformanceView {
  const stored = readStoredPerformanceView()
  if (stored === 'hr' && canUseHRView) return 'hr'
  if (stored === 'manager' && canUseManagerView) return 'manager'
  if (stored === 'employee') return 'employee'
  if (canUseHRView) return 'hr'
  if (canUseManagerView) return 'manager'
  return 'employee'
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
      tooltip: `流程模板：${templateDisplayName}`,
    }
  }

  const fallbackTemplateName = getTemplateDisplayName('', flowType)

  if (templateId) {
    return {
      label: fallbackTemplateName || `模板 #${templateId}`,
      color: fallbackTemplateName ? (flowType === 'new' ? 'purple' : 'default') : 'warning',
      tooltip: '未找到模板详情，已按流程模板默认值展示',
    }
  }

  return {
    label: fallbackTemplateName || '未配置流程模板',
    color: flowType === 'new' ? 'purple' : 'default',
    tooltip: '历史活动未关联绩效模板',
  }
}

function extractErrorMessage(err: any, fallback: string) {
  return err?.response?.data?.message || err?.response?.data?.error || err?.message || fallback
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

function getParticipantDisplayLevel(record: PerformanceParticipant, activity?: PerformanceActivity | null) {
  const finalLevel = String(record.final_level || '').trim()
  const suggestedLevel = String(record.suggested_level || '').trim()
  if (
    String(activity?.status || '').trim() === 'manager_evaluation' &&
    !record.department_adjusted &&
    !String(record.department_final_level || '').trim() &&
    suggestedLevel
  ) {
    return suggestedLevel
  }
  return finalLevel || suggestedLevel
}

function getManagerEvaluationBlockedReason(record: PerformanceParticipant) {
  const configStatus = getEffectiveManagerConfigStatus(record)
  if (!String(record.manager_id || '').trim()) return '请先配置考核上级'
  if (configStatus === 'PENDING') return '请先配置考核上级'
  if (configStatus === 'INVALID') return '考核上级不可用，请先调整'
  return ''
}

// 状态映射
const STATUS_MAP: Record<string, { label: string; color: string }> = {
  draft: { label: '草稿', color: 'default' },
  target_setting: { label: '目标设定', color: 'cyan' },
  target_approval: { label: '目标审核', color: 'geekblue' },
  self_evaluation: { label: '自评中', color: 'processing' },
  manager_evaluation: { label: '主管评分', color: 'warning' },
  department_evaluation: { label: '部门评分', color: 'orange' },
  hr_review: { label: 'HR审核', color: 'purple' },
  result_publish: { label: '结果公布', color: 'blue' },
  interview: { label: '绩效面谈', color: 'gold' },
  appeal: { label: '绩效申诉', color: 'volcano' },
  employee_confirmation: { label: '员工确认', color: 'blue' },
  manager_confirmation: { label: '主管确认', color: 'orange' },
  hr_confirmation: { label: 'HR确认', color: 'purple' },
  locked: { label: '已锁定', color: 'error' },
  result_confirmed: { label: '已确认', color: 'success' },
  archived: { label: '已归档', color: 'default' },
}

const ACTIVITY_STATUS_FILTER_IN_PROGRESS = '__in_progress__'
const ACTIVITY_STATUS_FILTER_CONFIRMED = '__confirmed__'
const ACTIVITY_STATUS_FILTER_TODO = '__todo__'
const ACTIVITY_LIST_PAGE_SIZE = 100
const ACTIVITY_LIST_MAX_PAGES = 50
const IN_PROGRESS_ACTIVITY_STATUSES = [
  'target_setting',
  'target_approval',
  'self_evaluation',
  'manager_evaluation',
  'department_evaluation',
  'hr_review',
  'employee_confirmation',
  'manager_confirmation',
  'hr_confirmation',
]
const CONFIRMED_ACTIVITY_STATUSES = ['result_publish', 'interview', 'appeal', 'locked', 'result_confirmed']
const DISTRIBUTION_DETAIL_STATUSES = [
  'manager_evaluation',
  'department_evaluation',
  'hr_review',
  'result_publish',
  'interview',
  'appeal',
  'employee_confirmation',
  'manager_confirmation',
  'hr_confirmation',
  'locked',
  'result_confirmed',
  'archived',
]
const PERSONAL_ENTRY_ACTIVITY_STATUSES = [
  'target_setting',
  'target_approval',
  'self_evaluation',
  'manager_evaluation',
  'department_evaluation',
  'hr_review',
  'result_publish',
  'interview',
  'appeal',
  'employee_confirmation',
  'manager_confirmation',
  'hr_confirmation',
  'locked',
  'result_confirmed',
]
const PERSONAL_TARGET_STATUSES = ['pending', 'target_pending_approval', 'target_rejected', 'target_set']
const PERSONAL_SELF_EVAL_STATUSES = ['target_set', 'self_submitted']
const PERSONAL_RESULT_STATUSES = ['manager_submitted', 'employee_confirmed', 'manager_recheck', 'manager_confirmed', 'hr_confirmed', 'locked', 'result_confirmed']
const SELF_EVAL_EDITABLE_ACTIVITY_STATUSES = ['self_evaluation', 'manager_evaluation', 'department_evaluation', 'hr_review', 'result_publish', 'interview', 'appeal', 'employee_confirmation', 'manager_confirmation', 'hr_confirmation', 'result_confirmed']
const SELF_EVAL_EDITABLE_PARTICIPANT_STATUSES = ['target_set', 'self_submitted', 'manager_submitted', 'employee_confirmed', 'manager_recheck', 'manager_confirmed']
const MUTENG_LATE_TARGET_ACTIVITY_STATUSES = ['target_approval', 'self_evaluation', 'manager_evaluation', 'department_evaluation', 'hr_review', 'result_publish', 'interview', 'appeal', 'hr_confirmation', 'employee_confirmation', 'locked', 'result_confirmed', 'archived']
const MUTENG_REVIEW_PIPELINE_ACTIVITY_STATUSES = ['self_evaluation', 'manager_evaluation', 'department_evaluation', 'hr_review', 'result_publish', 'interview', 'appeal', 'employee_confirmation', 'manager_confirmation', 'hr_confirmation', 'result_confirmed']
const ACTIVITY_STATUS_FILTER_GROUPS: Record<string, string[]> = {
  [ACTIVITY_STATUS_FILTER_IN_PROGRESS]: IN_PROGRESS_ACTIVITY_STATUSES,
  [ACTIVITY_STATUS_FILTER_CONFIRMED]: CONFIRMED_ACTIVITY_STATUSES,
}

function isSeparatedMutengActivity(activity?: PerformanceActivity | null) {
  return activity?.flow_type === 'new' && ['goal_setting', 'review_scoring'].includes(String(activity?.activity_kind || '').trim())
}

function isMutengGoalSettingActivity(activity?: PerformanceActivity | null) {
  return activity?.flow_type === 'new' && String(activity?.activity_kind || '').trim() === 'goal_setting'
}

function isSeparatedMutengReviewActivity(activity?: PerformanceActivity | null) {
  return activity?.flow_type === 'new' && String(activity?.activity_kind || '').trim() === 'review_scoring'
}

function isMutengReviewPipelineOpen(activity?: PerformanceActivity | null) {
  return activity?.flow_type === 'new' && !isMutengGoalSettingActivity(activity) && MUTENG_REVIEW_PIPELINE_ACTIVITY_STATUSES.includes(String(activity?.status || '').trim())
}

function isMutengTargetWorkflowOpen(activity?: PerformanceActivity | null) {
  return activity?.flow_type === 'new' && !['draft', 'locked', 'archived'].includes(String(activity?.status || '').trim())
}

function isMutengResultPublished(record?: PerformanceParticipant | null) {
  return ['hr_confirmed', 'locked', 'result_confirmed'].includes(String(record?.status || '').trim())
}

function canOpenTargetPlan(activity: PerformanceActivity | null | undefined, participant: PerformanceParticipant | null | undefined) {
  if (!activity || !participant || !PERSONAL_TARGET_STATUSES.includes(participant.status)) return false
  if (activity.status === 'target_setting') return true
  if (isMutengGoalSettingActivity(activity)) {
    return MUTENG_LATE_TARGET_ACTIVITY_STATUSES.includes(activity.status)
  }
  return activity.flow_type === 'new' && !['locked', 'archived'].includes(activity.status) && MUTENG_LATE_TARGET_ACTIVITY_STATUSES.includes(activity.status)
}
function resolveActivityStatusFilter(statusFilter?: string) {
  if (!statusFilter) return undefined
  return ACTIVITY_STATUS_FILTER_GROUPS[statusFilter] || [statusFilter]
}

function shouldLoadDistributionDetail(status?: string) {
  return DISTRIBUTION_DETAIL_STATUSES.includes(String(status || '').trim())
}

function shouldLoadHRDeadlineDetail(status?: string) {
  return String(status || '').trim() === 'hr_confirmation'
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

const MUTENG_PARTICIPANT_STATUS_MAP: Record<string, { label: string; color: string }> = {
  pending: { label: '待目标拟定', color: 'default' },
  target_pending_approval: { label: '目标待审核', color: 'cyan' },
  target_rejected: { label: '目标已驳回', color: 'red' },
  target_set: { label: '目标已审核', color: 'cyan' },
  self_submitted: { label: '自评已提交', color: 'processing' },
  manager_submitted: { label: '主管评分已完成', color: 'warning' },
  manager_confirmed: { label: '部门评分已完成', color: 'orange' },
  hr_confirmed: { label: '结果已公布', color: 'success' },
  employee_confirmed: { label: '结果已公布', color: 'success' },
  locked: { label: '已锁定', color: 'orange' },
  result_confirmed: { label: '结果已确认', color: 'success' },
  inactive: { label: '已离职', color: 'error' },
  removed_from_scope: { label: '已移除', color: 'error' },
}

const PARTICIPANT_PROGRESS_OPTIONS = Object.entries(PARTICIPANT_STATUS_MAP).map(([value, meta]) => ({
  value,
  label: meta.label,
}))

const MUTENG_PARTICIPANT_PROGRESS_STATUSES: PerformanceParticipantStatus[] = [
  'pending',
  'target_pending_approval',
  'target_rejected',
  'target_set',
  'self_submitted',
  'manager_submitted',
  'manager_confirmed',
  'hr_confirmed',
  'result_confirmed',
]

const MUTENG_PARTICIPANT_PROGRESS_STATUS_SET = new Set<string>(MUTENG_PARTICIPANT_PROGRESS_STATUSES)

const MUTENG_PARTICIPANT_PROGRESS_STATUS_ALIASES: Partial<Record<PerformanceParticipantStatus, PerformanceParticipantStatus>> = {
  employee_confirmed: 'hr_confirmed',
  manager_recheck: 'manager_submitted',
  locked: 'result_confirmed',
}

const PERFORMANCE_LEVEL_OPTIONS = ['S', 'A', 'B', 'C', 'D'].map(value => ({ value, label: value }))

function hasAnyPermission(...permissionCodes: string[]) {
  return permissionCodes.some(code => hasPermission(code))
}

const LEGACY_ACTIVITY_FLOW = [
  { status: 'target_setting', label: '目标设定' },
  { status: 'self_evaluation', label: '自评' },
  { status: 'manager_evaluation', label: '评分' },
  { status: 'employee_confirmation', label: '员工确认' },
  { status: 'manager_confirmation', label: '主管确认' },
  { status: 'hr_confirmation', label: 'HR确认' },
  { status: 'archived', label: '归档' },
]

const MUTENG_ACTIVITY_FLOW = [
  { status: 'target_setting', label: '目标拟定' },
  { status: 'target_approval', label: '目标审核' },
  { status: 'self_evaluation', label: '自评' },
  { status: 'manager_evaluation', label: '上级评估' },
  { status: 'department_evaluation', label: '部门/中心评估' },
  { status: 'hr_review', label: 'HR审核' },
  { status: 'result_publish', label: '结果公布' },
  { status: 'archived', label: '归档' },
]

const MUTENG_GOAL_SETTING_ACTIVITY_FLOW = [
  { status: 'target_setting', label: '目标拟定' },
  { status: 'target_approval', label: '目标审核' },
  { status: 'archived', label: '锁定/归档' },
]

const MUTENG_REVIEW_SCORING_ACTIVITY_FLOW = [
  { status: 'target_setting', label: '目标承接/补录' },
  { status: 'self_evaluation', label: '自评' },
  { status: 'manager_evaluation', label: '上级评估' },
  { status: 'department_evaluation', label: '部门/中心评估' },
  { status: 'hr_review', label: 'HR审核' },
  { status: 'result_publish', label: '结果公布' },
  { status: 'archived', label: '归档' },
]

function getActivityFlow(activity?: PerformanceActivity | null) {
  if (isMutengGoalSettingActivity(activity)) return MUTENG_GOAL_SETTING_ACTIVITY_FLOW
  if (isSeparatedMutengReviewActivity(activity)) return MUTENG_REVIEW_SCORING_ACTIVITY_FLOW
  return activity?.flow_type === 'new' ? MUTENG_ACTIVITY_FLOW : LEGACY_ACTIVITY_FLOW
}

function getActivityStepIndex(status?: string, activity?: PerformanceActivity | null) {
  const flow = getActivityFlow(activity)
  if (status === 'locked') return flow.length - 1 // archived step
  if (activity?.flow_type === 'new' && ['locked', 'result_confirmed', 'archived'].includes(status || '')) return flow.length - 1
  if (status === 'draft') return 0
  const index = flow.findIndex(item => item.status === status)
  return index >= 0 ? index : 0
}

function getStatusMeta(status?: string) {
  return STATUS_MAP[status || ''] || { label: status || '-', color: 'default' }
}

function getActivityStatusMeta(activity?: PerformanceActivity | null) {
  const meta = getStatusMeta(activity?.status)
  if (activity?.flow_type !== 'new') return meta
  if (activity.status === 'target_setting') {
    return { ...meta, label: isSeparatedMutengReviewActivity(activity) ? '目标承接/补录' : '目标拟定' }
  }
  if (activity.status === 'self_evaluation' && !isMutengGoalSettingActivity(activity)) {
    return { ...meta, label: '绩效考核进行中' }
  }
  if (activity.status === 'manager_evaluation') return { ...meta, label: '主管评分' }
  return meta
}

function getParticipantStatusMeta(status?: string, isMutengFlow = false) {
  const map = isMutengFlow ? MUTENG_PARTICIPANT_STATUS_MAP : PARTICIPANT_STATUS_MAP
  return map[status || ''] || { label: status || '-', color: 'default' }
}

function normalizeMutengParticipantProgressStatus(status?: string): PerformanceParticipantStatus | '' {
  const normalizedStatus = String(status || '').trim()
  if (!normalizedStatus) return ''
  if (MUTENG_PARTICIPANT_PROGRESS_STATUS_SET.has(normalizedStatus)) {
    return normalizedStatus as PerformanceParticipantStatus
  }
  return MUTENG_PARTICIPANT_PROGRESS_STATUS_ALIASES[normalizedStatus as PerformanceParticipantStatus] || ''
}

function getParticipantProgressOptions(isMutengFlow: boolean, currentStatus?: string) {
  if (!isMutengFlow) return PARTICIPANT_PROGRESS_OPTIONS

  const options = MUTENG_PARTICIPANT_PROGRESS_STATUSES.map(value => ({
    value,
    label: getParticipantStatusMeta(value, true).label,
  }))
  const normalizedCurrentStatus = normalizeMutengParticipantProgressStatus(currentStatus)
  if (normalizedCurrentStatus && !options.some(option => option.value === normalizedCurrentStatus)) {
    const current = getParticipantStatusMeta(normalizedCurrentStatus, true)
    return [{ value: normalizedCurrentStatus, label: current.label }, ...options]
  }
  return options
}

const PERFORMANCE_PERMISSION_LABELS: Record<string, string> = {
  'performance:activity:manage': '绩效活动管理',
  'performance:distribution:manage': '绩效分布规则',
  'performance:goal:manage': '绩效目标管理',
  'performance:self_eval:submit': '绩效自评提交',
  'performance:manager_eval:submit': '绩效主管评分',
  'performance:manager_confirm:submit': '绩效主管确认',
  'performance:result:view': '绩效结果查看',
  'performance:hr_confirm:submit': '绩效HR确认',
  'performance:department_eval:submit': '绩效部门评分',
  'performance:hr_review:submit': '绩效HR审核',
  'performance:result_publish:manage': '绩效结果公布',
  'performance:result_visibility:manage': '绩效结果屏蔽管理',
  'performance:hidden_result:view': '绩效屏蔽结果查看',
  'performance:appeal:manage': '绩效申诉处理',
  'performance:assessment_manager:update': '考核上级调整',
  'performance:assessment_manager:batch_update': '批量考核上级调整',
}

/** 绩效等级标签色：用 antd 语义色，避免旧主色 #1677ff 等硬编码 */
const PERFORMANCE_LEVEL_TAG_COLORS: Record<string, string> = {
  S: 'magenta',
  A: 'blue',
  B: 'green',
  C: 'gold',
  D: 'red',
}

/** 阶段推进/锁定类写操作二次确认文案（仅 UI 护栏，不改 API/状态机） */
const ACTIVITY_ACTION_CONFIRM: Record<string, { title: string; content: string; okText?: string; danger?: boolean }> = {
  'open-target-setting': {
    title: '开启目标设定',
    content: '确认开启目标设定阶段？开启后参与人可开始填写目标。',
    okText: '确认开启',
  },
  'open-self-evaluation': {
    title: '开启自评',
    content: '确认开启自评阶段？开启后参与人可提交自评。',
    okText: '确认开启',
  },
  'open-manager-evaluation': {
    title: '开启主管评分',
    content: '确认开启主管评分阶段？',
    okText: '确认开启',
  },
  'open-employee-confirmation': {
    title: '开启员工确认',
    content: '确认开启员工确认阶段？',
    okText: '确认开启',
  },
  'open-manager-confirmation': {
    title: '开启主管确认',
    content: '确认开启主管确认阶段？',
    okText: '确认开启',
  },
  'open-hr-confirmation': {
    title: '开启 HR 确认',
    content: '确认开启 HR 确认阶段？',
    okText: '确认开启',
  },
  'open-department-evaluation': {
    title: '开启部门评分',
    content: '确认开启部门评分阶段？',
    okText: '确认开启',
  },
  'open-hr-review': {
    title: '开启 HR 审核',
    content: '确认开启 HR 审核阶段？',
    okText: '确认开启',
  },
  'open-result-publish': {
    title: '开启结果公布',
    content: '确认进入结果公布阶段？公布后员工可按权限查看结果。',
    okText: '确认开启',
  },
  'open-performance-interview': {
    title: '开启绩效面谈',
    content: '确认进入绩效面谈阶段？',
    okText: '确认开启',
  },
  'open-performance-appeal': {
    title: '开启绩效申诉',
    content: '确认进入绩效申诉阶段？',
    okText: '确认开启',
  },
  'open-target-approval': {
    title: '开启目标审批',
    content: '确认开启目标审批阶段？',
    okText: '确认开启',
  },
  publish: {
    title: '直接开启自评',
    content: '确认跳过目标阶段，直接开启自评？此操作会改变活动阶段。',
    okText: '确认开启',
  },
  lock: {
    title: '锁定活动',
    content: '确认锁定活动？锁定后绩效结果将冻结，不可再修改。',
    okText: '确认锁定',
    danger: true,
  },
  archive: {
    title: '归档活动',
    content: '确认归档该绩效活动？归档后活动将进入归档列表。',
    okText: '确认归档',
  },
  close: {
    title: '关闭活动',
    content: '确认关闭该绩效活动？',
    okText: '确认关闭',
    danger: true,
  },
  'confirm-results': {
    title: '确认结果',
    content: '确认批量确认绩效结果？',
    okText: '确认',
  },
}

function confirmActivityWriteAction(
  action: string,
  activity: PerformanceActivity,
  run: (action: string, activity: PerformanceActivity) => void | Promise<void>,
) {
  const config = ACTIVITY_ACTION_CONFIRM[action]
  if (!config) {
    return run(action, activity)
  }
  Modal.confirm({
    title: config.title,
    content: config.content,
    okText: config.okText || '确认',
    cancelText: '取消',
    okButtonProps: config.danger ? { danger: true } : undefined,
    onOk: () => run(action, activity),
  })
}

function confirmParticipantWriteAction(options: {
  title: string
  content: string
  okText?: string
  danger?: boolean
  onOk: () => void | Promise<void>
}) {
  Modal.confirm({
    title: options.title,
    content: options.content,
    okText: options.okText || '确认',
    cancelText: '取消',
    okButtonProps: options.danger ? { danger: true } : undefined,
    onOk: options.onOk,
  })
}

function unwrapReminderResult<T>(res: any): T | undefined {
  let value = res
  for (let i = 0; i < 3; i++) {
    if (!value || typeof value !== 'object' || !('data' in value)) break
    value = value.data
  }
  return value as T | undefined
}

function unwrapSelfEvalReminderResult(res: any): PerformanceSelfEvalReminderResult | undefined {
  return unwrapReminderResult<PerformanceSelfEvalReminderResult>(res)
}

function unwrapHRConfirmReminderResult(res: any): PerformanceHRConfirmReminderResult | undefined {
  return unwrapReminderResult<PerformanceHRConfirmReminderResult>(res)
}

function unwrapManagerEvalReminderResult(res: any): PerformanceManagerEvalReminderResult | undefined {
  return unwrapReminderResult<PerformanceManagerEvalReminderResult>(res)
}

function formatReminderRecipient(recipient: PerformanceReminderRecipientDetail) {
  if (typeof recipient === 'string') return recipient || '-'
  const userID = String(recipient?.user_id || '').trim()
  const name = String(recipient?.name || '').trim()
  if (name && userID) return `${name}（${userID}）`
  return name || userID || '-'
}

function renderReminderRecipientList(title: string, recipients?: PerformanceReminderRecipientDetail[]) {
  if (!recipients?.length) return null
  return (
    <div>
      <Text strong>{title}</Text>
      <ul style={{ margin: '6px 0 0', paddingLeft: 18 }}>
        {recipients.map((recipient, index) => (
          <li key={`${title}-${index}`}>
            <Text>{formatReminderRecipient(recipient)}</Text>
          </li>
        ))}
      </ul>
    </div>
  )
}

function showReminderIssueDetails(
  title: string,
  result?: {
    skipped_recipients?: PerformanceReminderRecipientDetail[]
    failed_recipients?: PerformanceReminderRecipientDetail[]
    missing_id_participant_ids?: number[]
  },
) {
  const hasSkipped = Boolean(result?.skipped_recipients?.length)
  const hasFailed = Boolean(result?.failed_recipients?.length)
  const hasMissingID = Boolean(result?.missing_id_participant_ids?.length)
  if (!hasSkipped && !hasFailed && !hasMissingID) return

  Modal.warning({
    title,
    width: 560,
    content: (
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        {renderReminderRecipientList('发送失败', result?.failed_recipients)}
        {renderReminderRecipientList('已跳过（账号不可通知）', result?.skipped_recipients)}
        {hasMissingID && (
          <div>
            <Text strong>缺少员工ID的参与记录</Text>
            <Paragraph style={{ margin: '6px 0 0' }}>
              {result?.missing_id_participant_ids?.join('、')}
            </Paragraph>
          </div>
        )}
      </Space>
    ),
  })
}

function getReviewVersionsFromResponse(response: any): PerformanceReviewVersion[] {
  let payload = response
  for (let i = 0; i < 3; i += 1) {
    if (Array.isArray(payload?.versions)) return payload.versions
    payload = payload?.data
  }
  return []
}

function sortVersionsDesc(versions: PerformanceReviewVersion[]) {
  return [...versions].sort((a, b) => {
    const timeDiff = new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime()
    return timeDiff || b.id - a.id
  })
}

type ParticipantActionsProps = {
  activity: PerformanceActivity | null
  record: PerformanceParticipant
  canShowSelfReviewRecords: (record: PerformanceParticipant) => boolean
  navigateTo: (path: string) => void
  onOpenAssessmentManager: (record: PerformanceParticipant) => void
  onOpenSelfReviewRecords: (record: PerformanceParticipant) => void
  onRejectGoalRecords: (record: PerformanceParticipant) => void
  onReloadActivityDetail: (activity: PerformanceActivity) => void | Promise<void>
}

const ParticipantActions = React.memo(function ParticipantActions({
  activity,
  record,
  canShowSelfReviewRecords,
  navigateTo,
  onOpenAssessmentManager,
  onOpenSelfReviewRecords,
  onRejectGoalRecords,
  onReloadActivityDetail,
}: ParticipantActionsProps) {
  const activityId = activity?.id
  const activityStatus = activity?.status
  const isArchived = ['archived', 'locked'].includes(activityStatus || '')
  if (!activityId) return null

  const isMutengFlow = activity?.flow_type === 'new'
  /** 适用态下缺权：disabled + Tooltip，不隐藏 */
  const wrapPerm = (permissionCode: string, button: React.ReactElement, allowed = true) => {
    if (!allowed) {
      const permissionName = PERFORMANCE_PERMISSION_LABELS[permissionCode] || permissionCode
      return (
        <Tooltip key={button.key || permissionCode} title={`你缺少${permissionName}权限，需要联系管理员添加`}>
          <span>
            {React.cloneElement(button, {
              disabled: true,
              onClick: undefined,
            } as Partial<React.ComponentProps<typeof Button>>)}
          </span>
        </Tooltip>
      )
    }
    return button
  }
  const reloadActivityDetail = async () => {
    if (activity) await onReloadActivityDetail(activity)
  }
  const ensureReason = (reason: string, warning: string) => {
    if (reason.trim()) return true
    message.warning(warning)
    return false
  }
  const rejectKeepModalOpen = () => Promise.reject(new Error('reason required'))

  const openDepartmentEvaluationModal = () => {
    const baselineLevel = record.final_level || record.suggested_level || 'B'
    const baselineScore = record.adjusted_score || record.total_manager_score || record.manager_score || undefined
    let finalLevel = record.department_final_level || baselineLevel
    let finalScore: number | undefined = record.department_final_score ?? baselineScore
    let reason = ''
    const isAdjusted = () => {
      const levelChanged = String(finalLevel || '').trim() !== String(baselineLevel || '').trim()
      const scoreChanged = finalScore !== undefined && Number(finalScore) !== Number(baselineScore || 0)
      return levelChanged || scoreChanged
    }
    Modal.confirm({
      title: `部门评分：${record.employee_name}`,
      width: 480,
      okText: '确认',
      cancelText: '取消',
      content: (
        <Space direction="vertical" style={{ width: '100%', marginTop: 8 }} size={12}>
          <Alert
            showIcon
            type="info"
            message="可直接确认主管评分结果；如需调整最终等级或分数，请填写原因。"
          />
          <Select
            defaultValue={finalLevel}
            options={PERFORMANCE_LEVEL_OPTIONS}
            style={{ width: '100%' }}
            onChange={value => { finalLevel = value }}
          />
          <InputNumber
            min={0}
            max={120}
            precision={1}
            defaultValue={finalScore}
            style={{ width: '100%' }}
            placeholder="最终分，可选"
            onChange={value => { finalScore = value === null || value === undefined ? undefined : Number(value) }}
          />
          <TextArea rows={3} placeholder="调整时必填；不调整可填写确认说明" onChange={event => { reason = event.target.value }} />
        </Space>
      ),
      onOk: async () => {
        if (isAdjusted() && !ensureReason(reason, '请输入部门/中心调整原因')) return rejectKeepModalOpen()
        try {
          await performanceAPI.departmentEvaluateParticipantResult(record.id, {
            final_level: finalLevel,
            final_score: finalScore,
            reason: reason.trim() || '确认不调整',
          })
            message.success('部门评分已保存')
          await reloadActivityDetail()
        } catch (err: any) {
          message.error(err?.response?.data?.message || '保存失败')
          return Promise.reject(err)
        }
      },
    })
  }

  const openProgressModal = () => {
    let nextStatus: PerformanceParticipantStatus = isMutengFlow
      ? normalizeMutengParticipantProgressStatus(record.status) || 'pending'
      : record.status
    let reason = ''
    Modal.confirm({
      title: `调整进度：${record.employee_name}`,
      width: 460,
      okText: '保存',
      cancelText: '取消',
      content: (
        <Space direction="vertical" style={{ width: '100%', marginTop: 8 }} size={12}>
          <Select
            defaultValue={nextStatus}
            options={getParticipantProgressOptions(isMutengFlow, nextStatus)}
            style={{ width: '100%' }}
            onChange={value => { nextStatus = value as PerformanceParticipantStatus }}
          />
          <TextArea rows={3} placeholder="请输入调整原因" onChange={event => { reason = event.target.value }} />
        </Space>
      ),
      onOk: async () => {
        if (!ensureReason(reason, '请输入调整进度原因')) return rejectKeepModalOpen()
        try {
          await performanceAPI.adminAdjustParticipantProgress(record.id, nextStatus, reason.trim())
          message.success('进度已调整')
          await reloadActivityDetail()
        } catch (err: any) {
          message.error(err?.response?.data?.message || '调整失败')
          return Promise.reject(err)
        }
      },
    })
  }

  const openResultVisibilityModal = () => {
    const nextHidden = !record.result_hidden
    let reason = ''
    Modal.confirm({
      title: `${nextHidden ? '屏蔽' : '解除屏蔽'}结果：${record.employee_name}`,
      width: 460,
      okText: nextHidden ? '屏蔽' : '解除屏蔽',
      okButtonProps: nextHidden ? { danger: true } : undefined,
      cancelText: '取消',
      content: (
        <Space direction="vertical" style={{ width: '100%', marginTop: 8 }} size={12}>
          <Text type="secondary">{nextHidden ? '屏蔽后员工看不到任何绩效结果，也不会收到结果通知。' : '解除后员工可按权限查看绩效结果。'}</Text>
          <TextArea rows={3} placeholder="请输入操作原因" onChange={event => { reason = event.target.value }} />
        </Space>
      ),
      onOk: async () => {
        if (!ensureReason(reason, '请输入操作原因')) return rejectKeepModalOpen()
        try {
          await performanceAPI.setParticipantResultVisibility(record.id, nextHidden, reason.trim())
          message.success(nextHidden ? '已屏蔽结果' : '已解除屏蔽')
          await reloadActivityDetail()
        } catch (err: any) {
          message.error(err?.response?.data?.message || '操作失败')
          return Promise.reject(err)
        }
      },
    })
  }

  const openRemoveModal = () => {
    let reason = ''
    Modal.confirm({
      title: `移除参与人：${record.employee_name}`,
      width: 460,
      okText: '移除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      content: (
        <Space direction="vertical" style={{ width: '100%', marginTop: 8 }} size={12}>
          <Text type="secondary">移除后该员工的参与记录会软删除，历史操作仍保留。</Text>
          <TextArea rows={3} placeholder="请输入移除原因" onChange={event => { reason = event.target.value }} />
        </Space>
      ),
      onOk: async () => {
        if (!ensureReason(reason, '请输入移除原因')) return rejectKeepModalOpen()
        try {
          await performanceAPI.removePerformanceParticipant(record.id, reason.trim())
          message.success('参与人已移除')
          await reloadActivityDetail()
        } catch (err: any) {
          message.error(err?.response?.data?.message || '移除失败')
          return Promise.reject(err)
        }
      },
    })
  }

  const mutengAdminLinks = () => {
    if (!isMutengFlow) return null
    const items: React.ReactNode[] = []
    const canViewPrevious = hasAnyPermission('performance:result:view', 'performance:manager_eval:submit', 'performance:manager_confirm:submit', 'performance:department_eval:submit', 'performance:hr_review:submit', 'performance:result_publish:manage', 'performance:appeal:manage', 'performance:level_adjust:manage', 'performance:activity:manage')
    items.push(wrapPerm(
      'performance:result:view',
      <Button
        key="previous-result"
        size="small"
        type="link"
        style={participantActionButtonStyle}
        data-testid={`performance-participant-previous-result-${record.id}`}
        onClick={async () => {
          try {
            const res: any = await performanceAPI.getPreviousParticipantResult(record.id)
            const payload = res?.data ?? res
            const prevActivityId = payload?.activity?.id
            const prevParticipantId = payload?.participant?.id
            if (!prevActivityId || !prevParticipantId) {
              message.info('暂无上期绩效结果')
              return
            }
            navigateTo(`/performance-result/${prevActivityId}/${prevParticipantId}`)
          } catch (err: any) {
            message.error(err?.response?.data?.message || '获取上期结果失败')
          }
        }}
      >
        上期结果
      </Button>,
      canViewPrevious,
    ))
    if (
      isMutengReviewPipelineOpen(activity) &&
      ['manager_submitted', 'manager_confirmed'].includes(record.status)
    ) {
      const canDeptEval = hasAnyPermission('performance:department_eval:submit', 'performance:level_adjust:manage', 'performance:activity:manage')
      items.push(wrapPerm(
        'performance:department_eval:submit',
        <Button key="department-eval" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-department-eval-${record.id}`} onClick={openDepartmentEvaluationModal}>部门评分</Button>,
        canDeptEval,
      ))
    }
    if (isMutengResultPublished(record)) {
      items.push(wrapPerm(
        'performance:result_visibility:manage',
        <Button key="visibility" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-visibility-${record.id}`} onClick={openResultVisibilityModal}>
          {record.result_hidden ? '解除屏蔽' : '屏蔽'}
        </Button>,
        hasPermission('performance:result_visibility:manage'),
      ))
    }
    items.push(wrapPerm(
      'performance:activity:manage',
      <Button key="progress" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-progress-${record.id}`} onClick={openProgressModal}>调进度</Button>,
      hasPermission('performance:activity:manage'),
    ))
    items.push(wrapPerm(
      'performance:activity:manage',
      <Button key="remove" size="small" type="link" danger style={participantActionButtonStyle} data-testid={`performance-participant-remove-${record.id}`} onClick={openRemoveModal}>移除</Button>,
      hasPermission('performance:activity:manage'),
    ))
    return items
  }

  const archivedTargetButton = isMutengGoalSettingActivity(activity) && canOpenTargetPlan(activity, record)
    ? wrapPerm(
      'performance:goal:manage',
      <Button key="target" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-target-${record.id}`}
        onClick={() => navigateTo(`/performance-goal-setting/${activityId}/${record.id}`)}
      >目标</Button>,
      hasPermission('performance:goal:manage'),
    )
    : null

  if (isArchived) {
    const adminLinks = mutengAdminLinks()
    return (
      <div style={participantActionListStyle}>
        {archivedTargetButton}
        {!isMutengGoalSettingActivity(activity) && (
          wrapPerm(
            'performance:result:view',
            <Button size="small" type="link" style={participantActionButtonStyle}
              onClick={() => navigateTo(`/performance-result/${activityId}/${record.id}`)}
            >查看</Button>,
            hasPermission('performance:result:view'),
          )
        )}
        {canShowSelfReviewRecords(record) && (
          <Button size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-self-records-${record.id}`}
            onClick={() => onOpenSelfReviewRecords(record)}
          >记录</Button>
        )}
        {adminLinks}
      </div>
    )
  }

  const links: React.ReactNode[] = []

  links.push(wrapPerm(
    'performance:assessment_manager:update',
    <Button key="manager" size="small" type="link" style={participantActionButtonStyle} onClick={() => onOpenAssessmentManager(record)}>调上级</Button>,
    hasPermission('performance:assessment_manager:update'),
  ))

  const canSelfEvaluateRecord = (
    isMutengFlow
      ? isMutengReviewPipelineOpen(activity)
      : SELF_EVAL_EDITABLE_ACTIVITY_STATUSES.includes(activityStatus || '')
  ) && SELF_EVAL_EDITABLE_PARTICIPANT_STATUSES.includes(record.status)

  if (canOpenTargetPlan(activity, record)) {
    const isSeparatedReviewActivity = isSeparatedMutengReviewActivity(activity)
    const canGoal = hasPermission('performance:goal:manage')
    if (!isSeparatedReviewActivity) {
      links.push(wrapPerm(
        'performance:goal:manage',
        <Button key="target" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-target-${record.id}`}
          onClick={() => navigateTo(`/performance-goal-setting/${activityId}/${record.id}`)}
        >目标</Button>,
        canGoal,
      ))
    }
    if (activity?.flow_type === 'new' && ['target_setting', 'target_approval', 'self_evaluation'].includes(activityStatus || '') && (!isSeparatedMutengActivity(activity) || isSeparatedReviewActivity)) {
      links.push(wrapPerm(
        'performance:goal:manage',
        <Button key="review-supplement" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-review-supplement-${record.id}`}
          onClick={() => navigateTo(`/performance-goal-setting/${activityId}/${record.id}?phase=review`)}
        >补录</Button>,
        canGoal,
      ))
    }
  }

  if (canSelfEvaluateRecord) {
    links.push(wrapPerm(
      'performance:self_eval:submit',
      <Button key="self" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-self-${record.id}`}
        onClick={() => navigateTo(`/performance-self-eval/${activityId}/${record.id}`)}
      >{PERSONAL_SELF_EVAL_STATUSES.includes(record.status) ? '自评' : '改自评'}</Button>,
      hasPermission('performance:self_eval:submit'),
    ))
  }

  if (
    (isMutengFlow ? isMutengReviewPipelineOpen(activity) : activityStatus === 'manager_evaluation') &&
    ['self_submitted', 'manager_submitted', 'manager_recheck'].includes(record.status)
  ) {
    const managerEvalBlockedReason = getManagerEvaluationBlockedReason(record)
    const canMgr = hasPermission('performance:manager_eval:submit')
    if (!canMgr) {
      links.push(wrapPerm(
        'performance:manager_eval:submit',
        <Button key="mgr" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-manager-${record.id}`}>评分</Button>,
        false,
      ))
    } else if (managerEvalBlockedReason) {
      links.push(
        <Tooltip key="mgr" title={managerEvalBlockedReason}>
          <span>
            <Button size="small" type="link" disabled style={participantActionButtonStyle} data-testid={`performance-participant-manager-${record.id}`}>评分</Button>
          </span>
        </Tooltip>,
      )
    } else {
      links.push(
        <Button key="mgr" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-manager-${record.id}`}
          onClick={() => navigateTo(`/performance-manager-eval/${activityId}/${record.id}`)}
        >评分</Button>,
      )
    }
  }

  if (!isMutengFlow && ['manager_confirmation', 'hr_confirmation'].includes(activityStatus || '') && record.status === 'manager_recheck') {
    const managerEvalBlockedReason = getManagerEvaluationBlockedReason(record)
    const canConfirm = hasPermission('performance:manager_confirm:submit')
    if (!canConfirm) {
      links.push(wrapPerm(
        'performance:manager_confirm:submit',
        <Button key="manager-recheck" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-manager-recheck-${record.id}`}>复核</Button>,
        false,
      ))
    } else if (managerEvalBlockedReason) {
      links.push(
        <Tooltip key="manager-recheck" title={managerEvalBlockedReason}>
          <span>
            <Button size="small" type="link" disabled style={participantActionButtonStyle} data-testid={`performance-participant-manager-recheck-${record.id}`}>复核</Button>
          </span>
        </Tooltip>,
      )
    } else {
      links.push(
        <Button key="manager-recheck" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-manager-recheck-${record.id}`}
          onClick={() => navigateTo(`/performance-manager-eval/${activityId}/${record.id}`)}
        >复核</Button>,
      )
    }
  }

  if (
    isMutengFlow
      ? isMutengResultPublished(record)
      : ['manager_submitted', 'employee_confirmed', 'manager_recheck', 'manager_confirmed', 'hr_confirmed', 'locked', 'result_confirmed'].includes(record.status)
  ) {
    links.push(wrapPerm(
      'performance:result:view',
      <Button key="result" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-result-${record.id}`}
        onClick={() => navigateTo(`/performance-result/${activityId}/${record.id}`)}
      >结果</Button>,
      hasPermission('performance:result:view'),
    ))
  }

  if (canShowSelfReviewRecords(record)) {
    links.push(
      <Button key="self-records" size="small" type="link" style={participantActionButtonStyle} data-testid={`performance-participant-self-records-${record.id}`}
        onClick={() => onOpenSelfReviewRecords(record)}
      >记录</Button>,
    )
  }

  const stageCanReviewByHR = isMutengFlow && isMutengReviewPipelineOpen(activity) && record.status === 'manager_confirmed'
  const stageCanConfirmByHR = activityStatus === 'hr_confirmation' && record.status === 'manager_confirmed'
  if (stageCanReviewByHR || stageCanConfirmByHR) {
    const canReviewByHR = stageCanReviewByHR && hasAnyPermission('performance:hr_review:submit', 'performance:activity:manage')
    const canConfirmByHR = stageCanConfirmByHR && hasPermission('performance:hr_confirm:submit')
    const hrActionLabel = stageCanReviewByHR ? 'HR审核' : 'HR确认'
    const hrPermCode = stageCanReviewByHR ? 'performance:hr_review:submit' : 'performance:hr_confirm:submit'
    const allowed = canReviewByHR || canConfirmByHR
    links.push(wrapPerm(
      hrPermCode,
      <Button key="hr-confirm" size="small" type="link" style={{ ...participantActionButtonStyle, color: 'var(--color-primary)' }} data-testid={`performance-participant-hr-confirm-${record.id}`}
        onClick={() => {
          confirmParticipantWriteAction({
            title: `${hrActionLabel}：${record.employee_name}`,
            content: stageCanReviewByHR
              ? '确认完成 HR 审核？审核通过后结果将按流程公布，操作后不可轻易回退。'
              : '确认完成 HR 确认？确认后该参与人绩效结果将进入已确认状态。',
            okText: `确认${hrActionLabel}`,
            onOk: async () => {
              try {
                await performanceAPI.confirmHRResult(record.id)
                message.success(`${hrActionLabel}成功`)
                if (activity) await onReloadActivityDetail(activity)
              } catch (err: any) {
                message.error(err?.response?.data?.message || `${hrActionLabel}失败`)
                return Promise.reject(err)
              }
            },
          })
        }}
      >{hrActionLabel}</Button>,
      allowed,
    ))
  }

  if (
    (isMutengFlow ? isMutengTargetWorkflowOpen(activity) : activityStatus === 'target_approval') &&
    record.status === 'target_pending_approval'
  ) {
    const canApproveGoal = hasAnyPermission('performance:goal:manage', 'performance:hr_review:submit', 'performance:activity:manage')
    links.push(wrapPerm(
      'performance:goal:manage',
      <Button key="approve" size="small" type="link" style={{ ...participantActionButtonStyle, color: 'var(--color-info)' }} data-testid={`performance-participant-approve-${record.id}`}
        onClick={() => {
          confirmParticipantWriteAction({
            title: `通过目标：${record.employee_name}`,
            content: '确认通过该员工的目标设定？通过后目标将生效。',
            okText: '确认通过',
            onOk: async () => {
              try {
                await performanceAPI.approveGoalRecords(record.id)
                message.success('目标已通过')
                if (activity) await onReloadActivityDetail(activity)
              } catch (err: any) {
                message.error(err?.response?.data?.message || '审批失败')
                return Promise.reject(err)
              }
            },
          })
        }}
      >通过</Button>,
      canApproveGoal,
    ))
    links.push(wrapPerm(
      'performance:goal:manage',
      <Button key="reject" size="small" type="link" danger style={participantActionButtonStyle} data-testid={`performance-participant-reject-${record.id}`}
        onClick={() => onRejectGoalRecords(record)}
      >驳回</Button>,
      canApproveGoal,
    ))
  }

  const adminLinks = mutengAdminLinks()
  if (adminLinks) {
    links.push(...adminLinks)
  }

  return <div style={participantActionListStyle}>{links}</div>
})

const PerformanceOverview: React.FC = () => {
  const navigate = useNavigate()
  const currentUser = useAuthStore(state => state.user)
  const canUseManagerView = hasPermission('performance:manager_eval:submit') ||
    hasPermission('performance:goal:manage') ||
    hasPermission('performance:manager_confirm:submit')
  const canImportActivity = hasPermission('performance:activity:import') ||
    hasPermission('performance:activity:manage')
  const canUseHRView = canImportActivity ||
    hasPermission('performance:result:view') ||
    hasPermission('performance:distribution:manage') ||
    hasPermission('performance:hr_confirm:submit') ||
    hasPermission('performance:assessment_manager:batch_update') ||
    hasPermission('performance:hr_review:submit') ||
    hasPermission('performance:result_publish:manage') ||
    hasPermission('performance:result_visibility:manage') ||
    hasPermission('performance:appeal:manage') ||
    hasPermission('performance:department_eval:submit')
  const [activeView, setActiveView] = useState<PerformanceView>(() =>
    resolvePreferredPerformanceView(canUseHRView, canUseManagerView),
  )
  const activityListRef = React.useRef<HTMLDivElement | null>(null)
  const [, forceRender] = React.useState(0)
  const forceUpdate = () => forceRender(n => n + 1)
  const [activities, setActivities] = useState<PerformanceActivity[]>([])
  const [activitiesLoading, setActivitiesLoading] = useState(false)
  const [activitiesTotal, setActivitiesTotal] = useState(0)
  const [activityModalVisible, setActivityModalVisible] = useState(false)
  const [excelImportOpen, setExcelImportOpen] = useState(false)
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
  const [summary, setSummary] = useState<PerformanceResultSummary | null>(null)
  const [distributionCheck, setDistributionCheck] = useState<PerformanceDistributionCheck | null>(null)
  const [distributionRules, setDistributionRules] = useState<PerformanceDistributionRule[]>([])
  const [hrDeadlineStatus, setHrDeadlineStatus] = useState<PerformanceHRDeadlineStatus | null>(null)
  const [selfReviewDrawerVisible, setSelfReviewDrawerVisible] = useState(false)
  const [selfReviewTarget, setSelfReviewTarget] = useState<PerformanceParticipant | null>(null)
  const [selfReviewVersions, setSelfReviewVersions] = useState<PerformanceReviewVersion[]>([])
  const [selfReviewVersionsLoading, setSelfReviewVersionsLoading] = useState(false)

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
    const fallback = performanceViewOptions[0]?.value || 'employee'
    setActiveView(fallback)
    writeStoredPerformanceView(fallback)
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

  // 加载活动列表：分页拉全量，避免 page_size=100 截断导致芯片/列表不一致
  const loadActivities = useCallback(async (opts?: { silent?: boolean }) => {
    setActivitiesLoading(true)
    try {
      const allItems: PerformanceActivity[] = []
      let page = 1
      let total = 0
      while (page <= ACTIVITY_LIST_MAX_PAGES) {
        const res: any = await performanceAPI.getActivities({ page, page_size: ACTIVITY_LIST_PAGE_SIZE })
        const data = res?.data || res
        const pageItems = Array.isArray(data?.items)
          ? data.items
          : Array.isArray(data?.list)
            ? data.list
            : Array.isArray(data)
              ? data
              : []
        if (page === 1) {
          total = Number(data?.total ?? pageItems.length) || 0
        }
        allItems.push(...pageItems)
        if (pageItems.length < ACTIVITY_LIST_PAGE_SIZE || allItems.length >= total) {
          break
        }
        page += 1
      }
      setActivities(allItems)
      setActivitiesTotal(total || allItems.length)
      return { ok: true as const }
    } catch (err: any) {
      if (!opts?.silent) {
        message.error(extractErrorMessage(err, '加载活动列表失败'))
      }
      return { ok: false as const, error: extractErrorMessage(err, '加载活动列表失败') }
    } finally {
      setActivitiesLoading(false)
    }
  }, [])

  // 加载活动适用范围选项（绩效专用接口，不扩大通用 /users）
  const loadScopeOptions = useCallback(async (opts?: { silent?: boolean }) => {
    setScopeOptionsLoading(true)
    try {
      const res: any = await performanceAPI.getScopeOptions({ page: 1, page_size: 2000 })
      const data = res?.data || res
      setDepartments(Array.isArray(data?.departments) ? data.departments : [])
      setUsers(Array.isArray(data?.employees) ? data.employees : [])
      const warnings = Array.isArray(data?.warnings) ? data.warnings.filter(Boolean) : []
      if (!opts?.silent && warnings.length) {
        message.warning(warnings[0])
      }
      return { ok: true as const }
    } catch (err: any) {
      setDepartments([])
      setUsers([])
      if (!opts?.silent) {
        message.error(extractErrorMessage(err, '部门、员工选项加载失败'))
      }
      return { ok: false as const, error: extractErrorMessage(err, '部门、员工选项加载失败') }
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
    } catch (err: any) {
      setIndicatorLibraries([])
      message.error(extractErrorMessage(err, '指标库选项加载失败'))
    } finally {
      setIndicatorLibrariesLoading(false)
    }
  }, [])

  const loadPerformanceTemplates = useCallback(async (opts?: { silent?: boolean }) => {
    setPerformanceTemplatesLoading(true)
    try {
      const res: any = await performanceAPI.getTemplates({
        page: 1,
        page_size: 1000,
        status: 'active',
      })
      setPerformanceTemplates(getListFromResponse(res, ['items', 'templates']))
      return { ok: true as const }
    } catch (err: any) {
      setPerformanceTemplates([])
      if (!opts?.silent) {
        message.error(extractErrorMessage(err, '流程模板选项加载失败'))
      }
      return { ok: false as const, error: extractErrorMessage(err, '流程模板选项加载失败') }
    } finally {
      setPerformanceTemplatesLoading(false)
    }
  }, [])

  // 首次加载：合并错误提示，失败模块可独立重试
  const loadInitialPageData = useCallback(async () => {
    const results = await Promise.all([
      loadActivities({ silent: true }),
      loadScopeOptions({ silent: true }),
      loadPerformanceTemplates({ silent: true }),
    ])
    const failures = results
      .filter((item): item is { ok: false; error: string } => !item.ok)
      .map(item => item.error)
    if (failures.length === 1) {
      message.error(failures[0])
    } else if (failures.length > 1) {
      message.error(`页面初始化失败：${failures.join('；')}`)
    }
  }, [loadActivities, loadScopeOptions, loadPerformanceTemplates])

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
    loadInitialPageData()
  }, [loadInitialPageData])

  // 加载活动详情
  const loadActivityDetail = useCallback(async (activity: PerformanceActivity) => {
    const loadDistributionDetail = shouldLoadDistributionDetail(activity.status)
    const loadHRDeadline = shouldLoadHRDeadlineDetail(activity.status)
    const requests: Array<{
      key: 'participants' | 'summary' | 'distributionCheck' | 'distributionRules' | 'hrDeadline'
      request: Promise<any>
    }> = [
      { key: 'participants', request: performanceAPI.getParticipants(activity.id, { page: 1, page_size: 200 }) },
      { key: 'summary', request: performanceAPI.getResultSummary(activity.id) },
    ]
    if (loadDistributionDetail) {
      requests.push(
        { key: 'distributionCheck', request: performanceAPI.getDistributionCheck(activity.id) },
        { key: 'distributionRules', request: performanceAPI.getDistributionRules(activity.id) },
      )
    }
    if (loadHRDeadline) {
      requests.push({ key: 'hrDeadline', request: performanceAPI.getHRConfirmDeadlineStatus(activity.id) })
    }
    setCurrentActivity(activity)
    setDetailDrawerVisible(true)
    setParticipantsLoading(true)
    setSummaryLoading(true)
    setDistributionCheckLoading(loadDistributionDetail)
    setHrDeadlineStatus(null)
    if (!loadDistributionDetail) {
      setDistributionCheck(null)
      setDistributionRules([])
    }

    const results = await Promise.allSettled(requests.map(item => item.request))
    const resultByKey = new Map(requests.map((item, index) => [item.key, results[index]]))

    // 处理参与人
    const participantsResult = resultByKey.get('participants')
    if (participantsResult?.status === 'fulfilled') {
      const res = participantsResult.value as any
      const pData = res?.data || res
      setParticipants(pData?.items || [])
      setSelectedParticipantIds([])
    } else {
      setParticipants([])
      setSelectedParticipantIds([])
    }
    setParticipantsLoading(false)

    // 处理统计摘要
    const summaryResult = resultByKey.get('summary')
    if (summaryResult?.status === 'fulfilled') {
      const res = summaryResult.value as any
      setSummary(res?.data || null)
    } else setSummary(null)
    setSummaryLoading(false)

    const distributionCheckResult = resultByKey.get('distributionCheck')
    if (distributionCheckResult?.status === 'fulfilled') {
      const res = distributionCheckResult.value as any
      const dcData = res?.data || res
      setDistributionCheck(dcData || null)
    } else {
      setDistributionCheck(null)
    }
    setDistributionCheckLoading(false)

    const rulesResult = resultByKey.get('distributionRules')
    if (rulesResult?.status === 'fulfilled') {
      const res = rulesResult.value as any
      const rData = res?.data || res
      setDistributionRules(rData?.rules || [])
    } else {
      setDistributionRules([])
    }

    const hrDeadlineResult = resultByKey.get('hrDeadline')
    if (hrDeadlineResult?.status === 'fulfilled') {
      const res = hrDeadlineResult.value as any
      setHrDeadlineStatus((res?.data || res) as PerformanceHRDeadlineStatus)
    } else {
      setHrDeadlineStatus(null)
    }
  }, [])

  const reloadParticipants = useCallback(async (activityId: number) => {
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
  }, [])

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
      const activityKind = flowType === 'new'
        ? (values.activity_kind || editingActivity?.activity_kind || (editingActivity ? undefined : 'goal_setting'))
        : undefined
      const shouldSubmitReviewSchedule = flowType !== 'new' || activityKind !== 'goal_setting'
      const shouldSubmitResultConfirmSchedule = shouldSubmitReviewSchedule
        && !(flowType === 'new' && activityKind === 'review_scoring')
      const data = {
        name: values.name,
        cycle_type: values.cycle_type,
        start_date: values.date_range[0].format('YYYY-MM-DD'),
        end_date: values.date_range[1].format('YYYY-MM-DD'),
        template_id: values.template_id,
        flow_type: flowType,
        activity_kind: activityKind,
        organization_id: values.organization_id || '',
        applicable_org_scope: normalizeIDArray(values.applicable_org_scope),
        target_set_start_at: formatRangeStart(values.target_set_range),
        target_set_end_at: formatRangeEnd(values.target_set_range),
        snapshot_as_of_date: values.snapshot_as_of_date?.format('YYYY-MM-DD') || '',
        snapshot_source: values.snapshot_as_of_date ? 'assessment_period' : 'current_user',
        self_eval_start_at: shouldSubmitReviewSchedule ? formatRangeStart(values.self_eval_range) : '',
        self_eval_end_at: shouldSubmitReviewSchedule ? formatRangeEnd(values.self_eval_range) : '',
        manager_eval_start_at: shouldSubmitReviewSchedule ? formatRangeStart(values.manager_eval_range) : '',
        manager_eval_end_at: shouldSubmitReviewSchedule ? formatRangeEnd(values.manager_eval_range) : '',
        result_confirm_start_at: shouldSubmitResultConfirmSchedule ? formatRangeStart(values.result_confirm_range) : '',
        result_confirm_end_at: shouldSubmitResultConfirmSchedule ? formatRangeEnd(values.result_confirm_range) : '',
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
        previous_review_activity_id: activityKind === 'review_scoring' ? values.previous_review_activity_id : undefined,
        description: values.description,
        enable_bonus_score: values.enable_bonus_score || false,
        strict_time_mode: values.strict_time_mode || false,
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

  // 活动状态操作（执行体）；对外入口 handleActivityAction 会按需弹确认，不改 API/状态机
  const executeActivityAction = async (action: string, activity: PerformanceActivity) => {
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
        'open-target-approval': performanceAPI.openTargetApproval,
        'open-department-evaluation': performanceAPI.openDepartmentEvaluation,
        'open-hr-review': performanceAPI.openHRReview,
        'open-result-publish': performanceAPI.openResultPublish,
        'open-performance-interview': performanceAPI.openPerformanceInterviewStage,
        'open-performance-appeal': performanceAPI.openPerformanceAppeal,
        'open-employee-confirmation': performanceAPI.openEmployeeConfirmation,
        'open-manager-confirmation': performanceAPI.openManagerConfirmation,
        'open-hr-confirmation': performanceAPI.openHRConfirmation,
        lock: performanceAPI.lockActivity,
        'force-lock-overdue-hr': performanceAPI.forceLockOverdueHR,
        'notify-self-eval': performanceAPI.sendSelfEvalReminder,
      }
      const apiFn = apiMap[action]
      if (!apiFn) return
      const res: any = await apiFn(activity.id)
      message.success('操作成功')
      const warnings = res?.data?.warnings || res?.warnings
      if (Array.isArray(warnings) && warnings.length > 0) {
        message.warning(String(warnings[0]))
      }
      loadActivities()
      if (detailDrawerVisible && currentActivity?.id === activity.id) {
        const detailRes: any = await performanceAPI.getActivity(activity.id)
        const updated = detailRes.data?.activity || detailRes.data || detailRes
        await loadActivityDetail(updated)
      }
    } catch (err: any) {
      message.error(extractErrorMessage(err, '操作失败'))
      // 返回 rejected promise 给 Modal.confirm，失败时保持弹窗；调用方勿再额外 throw 未捕获错误
      return Promise.reject(err)
    }
  }

  const handleActivityAction = (action: string, activity: PerformanceActivity) => {
    confirmActivityWriteAction(action, activity, executeActivityAction)
  }

  const showSelfEvalReminderResult = (result?: PerformanceSelfEvalReminderResult) => {
    const pending = Number(result?.pending || 0)
    const candidates = Number(result?.candidates || 0)
    const sent = Number(result?.sent || 0)
    const skipped = Number(result?.skipped || 0)
    const failed = Number(result?.failed || 0)

    if (pending <= 0) {
      message.warning('当前没有待自评人员，无需发送提醒')
      return
    }
    if (sent > 0) {
      if (skipped > 0 || failed > 0) {
        message.warning(`已发送自评提醒 ${sent} 人，跳过 ${skipped} 人，失败 ${failed} 人`)
        showReminderIssueDetails('自评提醒发送明细', result)
        return
      }
      message.success(`已发送自评提醒 ${sent} 人`)
      return
    }
    if (candidates <= 0) {
      message.warning('没有找到可通知的待自评人员，请检查参与人配置')
      showReminderIssueDetails('自评提醒发送明细', result)
      return
    }
    if (failed > 0) {
      message.error(`自评提醒发送失败：${failed} 人失败`)
      showReminderIssueDetails('自评提醒发送明细', result)
      return
    }
    if (skipped > 0) {
      message.warning('自评提醒未发送：接收人不可通知，请检查钉钉账号状态')
      showReminderIssueDetails('自评提醒发送明细', result)
      return
    }
    message.warning('自评提醒未发送，请检查接收人配置')
    showReminderIssueDetails('自评提醒发送明细', result)
  }

  const handleSendSelfEvalReminder = (activity: PerformanceActivity) => {
    Modal.confirm({
      title: '发送自评提醒',
      content: `将向活动「${activity.name}」中待自评人员发送钉钉提醒。确认发送？`,
      okText: '确认发送',
      cancelText: '取消',
      onOk: async () => {
        try {
          const res: any = await performanceAPI.sendSelfEvalReminder(activity.id)
          showSelfEvalReminderResult(unwrapSelfEvalReminderResult(res))
        } catch (err: any) {
          message.error(err?.response?.data?.message || '发送提醒失败')
          return Promise.reject(err)
        }
      },
    })
  }

  const showManagerEvalReminderResult = (result?: PerformanceManagerEvalReminderResult) => {
    const pending = Number(result?.pending || 0)
    const candidates = Number(result?.candidates || 0)
    const sent = Number(result?.sent || 0)
    const skipped = Number(result?.skipped || 0)
    const failed = Number(result?.failed || 0)

    if (pending <= 0) {
      message.warning('当前没有待主管评分人员，无需发送提醒')
      return
    }
    if (sent > 0) {
      if (skipped > 0 || failed > 0) {
        message.warning(`已发送评分提醒 ${sent} 位主管，待评分 ${pending} 人，跳过 ${skipped} 位，失败 ${failed} 位`)
        showReminderIssueDetails('评分提醒发送明细', result)
        return
      }
      message.success(`已发送评分提醒 ${sent} 位主管，待评分 ${pending} 人`)
      return
    }
    if (candidates <= 0) {
      message.warning('没有找到可通知的主管，请检查考核上级配置')
      return
    }
    if (failed > 0) {
      message.error(`评分提醒发送失败：${failed} 位主管失败`)
      showReminderIssueDetails('评分提醒发送明细', result)
      return
    }
    if (skipped > 0) {
      message.warning('评分提醒未发送：主管账号不可通知，请检查钉钉账号状态')
      showReminderIssueDetails('评分提醒发送明细', result)
      return
    }
    message.warning('评分提醒未发送，请检查接收人配置')
  }

  const handleSendManagerEvalReminder = (activity: PerformanceActivity) => {
    Modal.confirm({
      title: '发送评分提醒',
      content: `将向活动「${activity.name}」中待评分主管发送钉钉提醒。确认发送？`,
      okText: '确认发送',
      cancelText: '取消',
      onOk: async () => {
        try {
          const res: any = await performanceAPI.sendManagerEvalReminder(activity.id)
          showManagerEvalReminderResult(unwrapManagerEvalReminderResult(res))
        } catch (err: any) {
          message.error(err?.response?.data?.message || '发送提醒失败')
          return Promise.reject(err)
        }
      },
    })
  }

  const showHRConfirmReminderResult = (result?: PerformanceHRConfirmReminderResult) => {
    const pending = Number(result?.pending || 0)
    const candidates = Number(result?.candidates || 0)
    const sent = Number(result?.sent || 0)
    const skipped = Number(result?.skipped || 0)
    const failed = Number(result?.failed || 0)

    if (pending <= 0) {
      message.warning('当前没有待HR确认人员，无需发送提醒')
      return
    }
    if (sent > 0) {
      if (skipped > 0 || failed > 0) {
        message.warning(`已发送HR确认提醒 ${sent} 人，跳过 ${skipped} 人，失败 ${failed} 人`)
        showReminderIssueDetails('HR确认提醒发送明细', result)
        return
      }
      message.success(`已发送HR确认提醒 ${sent} 人`)
      return
    }
    if (candidates <= 0) {
      message.warning('没有找到可通知的HR确认人，请检查HR确认权限配置')
      return
    }
    if (failed > 0) {
      message.error(`HR确认提醒发送失败：${failed} 人失败`)
      showReminderIssueDetails('HR确认提醒发送明细', result)
      return
    }
    if (skipped > 0) {
      message.warning('HR确认提醒未发送：接收人不可通知，请检查钉钉账号状态')
      showReminderIssueDetails('HR确认提醒发送明细', result)
      return
    }
    message.warning('HR确认提醒未发送，请检查接收人配置')
  }

  const handleSendHRConfirmReminder = (activity: PerformanceActivity) => {
    Modal.confirm({
      title: '发送 HR 确认提醒',
      content: `将向活动「${activity.name}」中待 HR 确认人员发送钉钉提醒。确认发送？`,
      okText: '确认发送',
      cancelText: '取消',
      onOk: async () => {
        try {
          const res: any = await performanceAPI.sendHRConfirmReminder(activity.id)
          showHRConfirmReminderResult(unwrapHRConfirmReminderResult(res))
        } catch (err: any) {
          message.error(err?.response?.data?.message || '发送提醒失败')
          return Promise.reject(err)
        }
      },
    })
  }

  const handleForceLockOverdueHR = (activity: PerformanceActivity) => {
    Modal.confirm({
      title: '逾期强制锁定',
      content: '将把已完成主管确认但未完成 HR 确认的参与人标记为逾期强制锁定，并锁定活动。此操作会冻结绩效结果。',
      okText: '确认强制锁定',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: () => executeActivityAction('force-lock-overdue-hr', activity),
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

  const handleRejectGoalRecords = useCallback((record: PerformanceParticipant) => {
    rejectGoalForm.resetFields()
    setRejectGoalTarget(record)
    setRejectGoalModalVisible(true)
  }, [rejectGoalForm])

  const renderPermissionButton = useCallback((
    permissionCode: string,
    button: React.ReactElement,
    permissionName = PERFORMANCE_PERMISSION_LABELS[permissionCode] || permissionCode,
  ) => {
    if (hasPermission(permissionCode)) return button
    return (
      <Tooltip key={button.key || permissionCode} title={`你缺少${permissionName}权限，需要联系管理员添加`}>
        <span>
          {React.cloneElement(button, {
            disabled: true,
            onClick: undefined,
          } as Partial<React.ComponentProps<typeof Button>>)}
        </span>
      </Tooltip>
    )
  }, [])

  const renderActivityManageButton = useCallback((button: React.ReactElement) =>
    renderPermissionButton('performance:activity:manage', button), [renderPermissionButton])

  // 详情操作：缺权时 disable + Tooltip，避免 sticky 操作栏出现空白/无说明
  const renderDetailActivityManageButton = useCallback((button: React.ReactElement) =>
    renderPermissionButton('performance:activity:manage', button), [renderPermissionButton])

  const renderDetailDistributionButton = useCallback((button: React.ReactElement) =>
    renderPermissionButton('performance:distribution:manage', button), [renderPermissionButton])

  const renderDetailManagerEvalButton = useCallback((button: React.ReactElement) =>
    renderPermissionButton('performance:manager_eval:submit', button), [renderPermissionButton])

  const withActivityActionStyle = useCallback((button: React.ReactElement) => {
    const props = button.props as { style?: React.CSSProperties }
    return React.cloneElement(button, {
      style: {
        ...activityActionButtonStyle,
        ...props.style,
      },
    } as Partial<React.ComponentProps<typeof Button>>)
  }, [])

  const loadAssessmentManagerCandidates = useCallback(async (keyword = '', participantId?: number, source?: AssessmentManagerSource) => {
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
  }, [currentActivity, managerForm])

  const openAssessmentManagerModal = useCallback((record?: PerformanceParticipant) => {
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
  }, [currentActivity, loadAssessmentManagerCandidates, loadScopeOptions, managerForm, selectedParticipantIds.length, users.length])

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

  const batchEvalSelectableParticipantIds = React.useMemo(
    () => participants
      .filter(p => (p.status === 'self_submitted' || p.status === 'manager_submitted') && isAssessmentManagerConfigured(p))
      .map(p => p.id),
    [participants],
  )

  const openBatchEvalModal = useCallback(() => {
    setBatchEvalSelected(batchEvalSelectableParticipantIds)
    setBatchEvalModalVisible(true)
  }, [batchEvalSelectableParticipantIds])

  const selfFinalSubmittedCount = React.useMemo(
    () => participants.reduce((count, record) => (
      record.status === 'self_submitted' && isSelfFinalAssessmentRecord(record) ? count + 1 : count
    ), 0),
    [participants],
  )

  const hrDetailSummaryCards = React.useMemo(() => {
    if (!summary) return []
    const isMutengFlow = currentActivity?.flow_type === 'new'
    if (isMutengGoalSettingActivity(currentActivity)) {
      const submittedCount = participants.filter(record => ['target_pending_approval', 'target_set', 'locked', 'result_confirmed'].includes(record.status)).length
      return [
        { title: '参与人数', value: summary.total_participants, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)' },
        { title: '已提交目标', value: submittedCount, color: '#0369a1', bg: '#e0f2fe' },
        { title: '已审核目标', value: summary.target_set_count || 0, color: '#16a34a', bg: '#dcfce7' },
        { title: '已锁定', value: summary.locked_count || 0, color: '#b45309', bg: '#fef3c7' },
      ]
    }
    if (isMutengFlow) {
      const countStatus = (...statuses: string[]) => participants.filter(record => statuses.includes(record.status)).length
      return [
        { title: '参与人数', value: summary.total_participants, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)' },
        { title: '待员工自评', value: countStatus('target_set'), color: '#64748b', bg: '#f1f5f9' },
        { title: '待上级评分', value: countStatus('self_submitted', 'manager_recheck'), color: '#0369a1', bg: '#e0f2fe' },
        { title: '待部门评分', value: countStatus('manager_submitted'), color: '#b45309', bg: '#fef3c7' },
        { title: '待HR审核', value: countStatus('manager_confirmed'), color: '#7e22ce', bg: '#f3e8ff' },
        { title: '结果已公布', value: countStatus('hr_confirmed', 'employee_confirmed', 'locked', 'result_confirmed'), color: 'var(--color-success)', bg: '#dcfce7' },
      ]
    }
    const employeeConfirmedCount = (summary.employee_confirmed_count || 0)
    return [
      { title: '参与人数', value: summary.total_participants, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)' },
      { title: '已自评', value: summary.self_submitted_count, color: '#0369a1', bg: '#e0f2fe' },
      {
        title: isMutengFlow ? '已主管评分' : '已评分',
        value: summary.manager_submitted_count + selfFinalSubmittedCount,
        color: '#b45309',
        bg: '#fef3c7',
      },
      {
        title: isMutengFlow ? '员工已确认' : '已确认',
        value: isMutengFlow ? employeeConfirmedCount : summary.result_confirmed_count,
        color: 'var(--color-success)',
        bg: '#dcfce7',
      },
    ]
  }, [currentActivity, participants, selfFinalSubmittedCount, summary])

  const currentActivityStatusConfig = React.useMemo(
    () => currentActivity ? getActivityStatusMeta(currentActivity) : undefined,
    [currentActivity],
  )

  const currentActivityTemplateDisplay = React.useMemo(
    () => currentActivity ? getActivityTemplateDisplay(currentActivity, performanceTemplateById) : null,
    [currentActivity, performanceTemplateById],
  )

  const currentActivityStepItems = React.useMemo(() => {
    if (!currentActivity) return []
    const flow = getActivityFlow(currentActivity)
    const currentStepIndex = getActivityStepIndex(currentActivity.status, currentActivity)
    return flow.map((item, index) => ({
      title: item.label,
      status: item.status === currentActivity.status
        ? 'process' as const
        : currentStepIndex > index ? 'finish' as const : 'wait' as const,
    }))
  }, [currentActivity])

  const closeDetailDrawer = useCallback(() => {
    setDetailDrawerVisible(false)
    setCurrentActivity(null)
    setParticipants([])
    setSelectedParticipantIds([])
    setManagerModalVisible(false)
    setSummary(null)
    setDistributionCheck(null)
    setDistributionRules([])
    setHrDeadlineStatus(null)
    setSelfReviewDrawerVisible(false)
    setSelfReviewTarget(null)
    setSelfReviewVersions([])
  }, [])

  const renderDetailActionButtons = (activity: PerformanceActivity) => {
    const actions: React.ReactNode[] = []
    const activityManage = (button: React.ReactElement) => renderDetailActivityManageButton(button)
    const isMutengFlow = activity.flow_type === 'new'

    if (!['locked', 'archived'].includes(activity.status)) {
      actions.push(
        renderPermissionButton(
          'performance:assessment_manager:batch_update',
          <Button key="batch-manager" size="small" data-testid={`performance-detail-batch-manager-${activity.id}`} onClick={() => openAssessmentManagerModal()}>批量调整考核上级</Button>,
        ),
      )
    }

    if (activity.status === 'draft') {
      actions.push(
        activityManage(<Button key="edit-participants" size="small" data-testid={`performance-detail-edit-participants-${activity.id}`} onClick={() => { setDetailDrawerVisible(false); openActivityModal(activity) }}>编辑参与人</Button>),
        activityManage(<Button key="open-target-setting" type="primary" size="small" data-testid={`performance-detail-open-target-${activity.id}`} onClick={() => handleActivityAction('open-target-setting', activity)}>{isSeparatedMutengReviewActivity(activity) ? '开启目标承接/补录' : '开启目标设定'}</Button>),
      )
      if (!isMutengFlow) {
        actions.push(activityManage(<Button key="publish" size="small" data-testid={`performance-detail-publish-${activity.id}`} onClick={() => handleActivityAction('publish', activity)}>直接开启自评</Button>))
      }
    }
    if (activity.status === 'target_setting') {
      if (!isMutengGoalSettingActivity(activity)) {
        actions.push(activityManage(
          <Button key="open-self-evaluation" type="primary" size="small" data-testid={`performance-detail-open-self-${activity.id}`} onClick={() => handleActivityAction('open-self-evaluation', activity)}>开启自评</Button>,
        ))
      }
    }
    if (isMutengFlow && !isMutengGoalSettingActivity(activity) && activity.status === 'target_approval') {
      actions.push(activityManage(<Button key="open-self-evaluation" type="primary" size="small" data-testid={`performance-detail-open-self-${activity.id}`} onClick={() => handleActivityAction('open-self-evaluation', activity)}>开启自评</Button>))
    }
    if (isMutengFlow && isMutengReviewPipelineOpen(activity)) {
      actions.push(
        activityManage(<Button key="send-self-reminder" size="small" data-testid={`performance-detail-remind-self-${activity.id}`} onClick={() => handleSendSelfEvalReminder(activity)}>提醒自评</Button>),
        renderDetailDistributionButton(<Button key="distribution" size="small" data-testid={`performance-detail-distribution-${activity.id}`} onClick={() => setDistributionModalVisible(true)}>强制分布</Button>),
        renderDetailManagerEvalButton(<Button key="batch-eval" size="small" data-testid={`performance-detail-batch-eval-${activity.id}`} onClick={openBatchEvalModal}>批量评分</Button>),
        activityManage(<Button key="send-manager-reminder" size="small" data-testid={`performance-detail-remind-manager-${activity.id}`} onClick={() => handleSendManagerEvalReminder(activity)}>提醒评分</Button>),
      )
    } else if (activity.status === 'self_evaluation') {
      actions.push(
        activityManage(<Button key="open-manager-evaluation" type="primary" size="small" data-testid={`performance-detail-open-manager-${activity.id}`} onClick={() => handleActivityAction('open-manager-evaluation', activity)}>开启主管评分</Button>),
        activityManage(<Button key="send-self-reminder" size="small" data-testid={`performance-detail-remind-self-${activity.id}`} onClick={() => handleSendSelfEvalReminder(activity)}>提醒自评</Button>),
      )
    }
    if (!isMutengFlow && activity.status === 'manager_evaluation') {
      actions.push(
        activityManage(<Button key="open-employee-confirmation" type="primary" size="small" data-testid={`performance-detail-open-employee-confirm-${activity.id}`} onClick={() => handleActivityAction('open-employee-confirmation', activity)}>开启员工确认</Button>),
        renderDetailDistributionButton(<Button key="distribution" size="small" data-testid={`performance-detail-distribution-${activity.id}`} onClick={() => setDistributionModalVisible(true)}>强制分布</Button>),
        renderDetailManagerEvalButton(<Button key="batch-eval" size="small" data-testid={`performance-detail-batch-eval-${activity.id}`} onClick={openBatchEvalModal}>批量评分</Button>),
        activityManage(<Button key="send-manager-reminder" size="small" data-testid={`performance-detail-remind-manager-${activity.id}`} onClick={() => handleSendManagerEvalReminder(activity)}>提醒评分</Button>),
      )
    }
    if (!isMutengFlow && activity.status === 'employee_confirmation') {
      actions.push(activityManage(<Button key="open-manager-confirmation" type="primary" size="small" data-testid={`performance-detail-open-manager-confirm-${activity.id}`} onClick={() => handleActivityAction('open-manager-confirmation', activity)}>开启主管确认</Button>))
    }
    if (!isMutengFlow && activity.status === 'manager_confirmation') {
      actions.push(activityManage(<Button key="open-hr-confirmation" type="primary" size="small" data-testid={`performance-detail-open-hr-confirm-${activity.id}`} onClick={() => handleActivityAction('open-hr-confirmation', activity)}>开启HR确认</Button>))
    }
    if (!isMutengFlow && activity.status === 'hr_confirmation') {
      actions.push(activityManage(<Button key="send-hr-reminder" size="small" data-testid={`performance-detail-remind-hr-${activity.id}`} onClick={() => handleSendHRConfirmReminder(activity)}>提醒HR确认</Button>))
      if (hrDeadlineStatus?.can_force_lock) {
        actions.push(activityManage(<Button key="force-lock-overdue" danger size="small" data-testid={`performance-detail-force-lock-${activity.id}`} onClick={() => handleForceLockOverdueHR(activity)}>逾期强制锁定</Button>))
      }
      actions.push(activityManage(<Button key="lock" type="primary" danger size="small" data-testid={`performance-detail-lock-${activity.id}`} onClick={() => handleActivityAction('lock', activity)}>锁定活动</Button>))
    }
    if (activity.status === 'locked' || activity.status === 'result_confirmed' || (isMutengFlow && ['result_publish', 'interview', 'appeal'].includes(activity.status))) {
      actions.push(activityManage(<Button key="archive" size="small" data-testid={`performance-detail-archive-${activity.id}`} onClick={() => handleActivityAction('archive', activity)}>归档活动</Button>))
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
        activity_kind: activity.activity_kind,
        organization_id: activity.organization_id || '',
        applicable_org_scope: normalizeIDArray(activity.applicable_org_scope),
        date_range: [dayjs(activity.start_date), dayjs(activity.end_date)],
        target_set_range: activity.target_set_start_at && activity.target_set_end_at ? [dayjs(activity.target_set_start_at), dayjs(activity.target_set_end_at)] : undefined,
        snapshot_as_of_date: activity.snapshot_as_of_date ? dayjs(activity.snapshot_as_of_date) : undefined,
        self_eval_range: activity.self_eval_start_at && activity.self_eval_end_at ? [dayjs(activity.self_eval_start_at), dayjs(activity.self_eval_end_at)] : undefined,
        manager_eval_range: activity.manager_eval_start_at && activity.manager_eval_end_at ? [dayjs(activity.manager_eval_start_at), dayjs(activity.manager_eval_end_at)] : undefined,
        result_confirm_range: activity.result_confirm_start_at && activity.result_confirm_end_at ? [dayjs(activity.result_confirm_start_at), dayjs(activity.result_confirm_end_at)] : undefined,
        employee_confirm_range: activity.employee_confirm_start_at && activity.employee_confirm_end_at ? [dayjs(activity.employee_confirm_start_at), dayjs(activity.employee_confirm_end_at)] : undefined,
        manager_confirm_range: activity.manager_confirm_start_at && activity.manager_confirm_end_at ? [dayjs(activity.manager_confirm_start_at), dayjs(activity.manager_confirm_end_at)] : undefined,
        hr_confirm_range: activity.hr_confirm_start_at && activity.hr_confirm_end_at ? [dayjs(activity.hr_confirm_start_at), dayjs(activity.hr_confirm_end_at)] : undefined,
        hr_confirm_deadline: activity.hr_confirm_deadline ? dayjs(activity.hr_confirm_deadline) : undefined,
        target_department_ids: normalizeIDArray(activity.target_department_ids),
        target_employee_ids: targetEmployeeIDs,
        default_assessment_manager_source: activity.default_assessment_manager_source || 'DIRECT_MANAGER',
        indicator_library_id: activity.indicator_library_id,
        previous_review_activity_id: activity.previous_review_activity_id,
        description: activity.description,
        enable_bonus_score: activity.enable_bonus_score || false,
        strict_time_mode: activity.strict_time_mode || false,
      })
    } else {
      setImportedUserOptions([])
      setImportedManagerAssignments([])
      form.resetFields()
      form.setFieldsValue({ default_assessment_manager_source: 'DIRECT_MANAGER', flow_type: 'old', activity_kind: undefined, previous_review_activity_id: undefined })
    }
    setActivityModalVisible(true)
    window.requestAnimationFrame(() => {
      document.getElementById('performance-activity-editor')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  }

  const renderMyParticipantActionButton = useCallback((activity: PerformanceActivity): React.ReactNode => {
    const participant = activity.my_participant
    if (!participant) return null

    const linkStyle: React.CSSProperties = activityActionButtonStyle
    const activityId = activity.id
    const participantId = participant.id

    if (canOpenTargetPlan(activity, participant)) {
      const isSeparatedReviewActivity = isSeparatedMutengReviewActivity(activity)
      const targetLabel = isSeparatedReviewActivity
        ? '补录目标'
        : (['pending', 'target_rejected'].includes(participant.status) ? '填写目标' : '我的目标')
      const targetUrl = isSeparatedReviewActivity
        ? `/performance-goal-setting/${activityId}/${participantId}?phase=review`
        : `/performance-goal-setting/${activityId}/${participantId}`
      return renderPermissionButton(
        'performance:goal:manage',
        <Button
          key="my-target"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-my-target-${activity.id}`}
          onClick={() => navigate(targetUrl)}
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

    const canViewNewFlowResult = activity.flow_type === 'new' &&
      !isMutengGoalSettingActivity(activity) &&
      isMutengResultPublished(participant)
    const canViewLegacyResult = activity.flow_type !== 'new' && PERSONAL_RESULT_STATUSES.includes(participant.status)

    if (participant.result_hidden && (canViewLegacyResult || canViewNewFlowResult)) {
      return <Tag color="default">绩效结果暂未公布</Tag>
    }

    if (canViewLegacyResult || canViewNewFlowResult) {
      const resultLabel = activity.flow_type !== 'new' && activity.status === 'employee_confirmation' && participant.status === 'manager_submitted'
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
  }, [navigate, renderPermissionButton])

  /** List-level primary stage action only (same handlers as detail drawer). 权限由 getActionButtons 统一 wrap。 */
  const getListStagePrimaryButton = useCallback((record: PerformanceActivity): React.ReactElement | null => {
    if (activeView !== 'hr') return null
    const isMutengFlow = record.flow_type === 'new'
    const linkStyle = activityActionButtonStyle

    if (record.status === 'draft') {
      return (
        <Button
          key="open-target"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-open-target-${record.id}`}
          onClick={() => handleActivityAction('open-target-setting', record)}
        >
          {isSeparatedMutengReviewActivity(record) ? '开启目标承接' : '开启目标设定'}
        </Button>
      )
    }
    if (record.status === 'target_setting' && !isMutengGoalSettingActivity(record)) {
      return (
        <Button
          key="open-self"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-open-self-${record.id}`}
          onClick={() => handleActivityAction('open-self-evaluation', record)}
        >
          开启自评
        </Button>
      )
    }
    if (isMutengFlow && !isMutengGoalSettingActivity(record) && record.status === 'target_approval') {
      return (
        <Button
          key="open-self"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-open-self-${record.id}`}
          onClick={() => handleActivityAction('open-self-evaluation', record)}
        >
          开启自评
        </Button>
      )
    }
    if (!isMutengFlow && record.status === 'self_evaluation') {
      return (
        <Button
          key="open-manager"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-open-manager-${record.id}`}
          onClick={() => handleActivityAction('open-manager-evaluation', record)}
        >
          开启主管评分
        </Button>
      )
    }
    if (!isMutengFlow && record.status === 'manager_evaluation') {
      return (
        <Button
          key="open-employee-confirm"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-open-employee-confirm-${record.id}`}
          onClick={() => handleActivityAction('open-employee-confirmation', record)}
        >
          开启员工确认
        </Button>
      )
    }
    if (!isMutengFlow && record.status === 'employee_confirmation') {
      return (
        <Button
          key="open-manager-confirm"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-open-manager-confirm-${record.id}`}
          onClick={() => handleActivityAction('open-manager-confirmation', record)}
        >
          开启主管确认
        </Button>
      )
    }
    if (!isMutengFlow && record.status === 'manager_confirmation') {
      return (
        <Button
          key="open-hr-confirm"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-open-hr-confirm-${record.id}`}
          onClick={() => handleActivityAction('open-hr-confirmation', record)}
        >
          开启HR确认
        </Button>
      )
    }
    if (!isMutengFlow && record.status === 'hr_confirmation') {
      return (
        <Button
          key="lock"
          size="small"
          type="link"
          danger
          style={linkStyle}
          data-testid={`performance-activity-lock-${record.id}`}
          onClick={() => handleActivityAction('lock', record)}
        >
          锁定活动
        </Button>
      )
    }
    if (record.status === 'locked' || record.status === 'result_confirmed' || (isMutengFlow && ['result_publish', 'interview', 'appeal'].includes(record.status))) {
      return (
        <Button
          key="archive"
          size="small"
          type="link"
          style={linkStyle}
          data-testid={`performance-activity-archive-${record.id}`}
          onClick={() => handleActivityAction('archive', record)}
        >
          归档
        </Button>
      )
    }
    return null
  }, [activeView, handleActivityAction])

  // 活动列表操作按钮
  const getActionButtons = useCallback((record: PerformanceActivity) => {
    const buttons: React.ReactNode[] = []
    const personalActionButton = renderMyParticipantActionButton(record)

    if (personalActionButton) {
      buttons.push(personalActionButton)
    }

    const stagePrimary = getListStagePrimaryButton(record)
    if (stagePrimary) {
      buttons.push(stagePrimary)
    }

    buttons.push(
        <Button size="small" type="link" data-testid={`performance-activity-view-${record.id}`} onClick={() => loadActivityDetail(record)} key="view">详情</Button>
    )

    return buttons.map(button => {
      if (!React.isValidElement(button)) return button

      const buttonKey = String(button.key || '')
      if (buttonKey.startsWith('my-')) return button

      const styledButton = withActivityActionStyle(button)
      // Stage primary + other manage actions go through permission wrapper; detail view is always free.
      if (buttonKey === 'view') return styledButton
      return renderActivityManageButton(styledButton)
    })
  }, [getListStagePrimaryButton, loadActivityDetail, renderActivityManageButton, renderMyParticipantActionButton, withActivityActionStyle])

  const renderActivityActions = useCallback((record: PerformanceActivity) => (
    <div style={activityActionListStyle}>
      {getActionButtons(record).map((button, index) => (
        <span key={React.isValidElement(button) ? button.key || index : index} style={{ display: 'inline-flex' }}>
          {button}
        </span>
      ))}
    </div>
  ), [getActionButtons])

  const canShowSelfReviewRecords = useCallback((record: PerformanceParticipant) => (
    hasPermission('performance:result:view') &&
    ['self_submitted', 'manager_submitted', 'employee_confirmed', 'manager_recheck', 'manager_confirmed', 'hr_confirmed', 'locked', 'result_confirmed'].includes(record.status)
  ), [])

  const handleOpenSelfReviewRecords = useCallback(async (record: PerformanceParticipant) => {
    setSelfReviewTarget(record)
    setSelfReviewDrawerVisible(true)
    setSelfReviewVersions([])
    setSelfReviewVersionsLoading(true)
    try {
      const res = await performanceAPI.getParticipantVersions(record.id)
      const canViewDepartmentEvaluationRecords = hasAnyPermission(
        'performance:activity:manage',
        'performance:department_eval:submit',
        'performance:hr_confirm:submit',
        'performance:hr_review:submit',
        'performance:result_publish:manage',
        'performance:level_adjust:manage',
      )
      const visibleReviewTypes = new Set(['self'])
      if (canViewDepartmentEvaluationRecords) {
        visibleReviewTypes.add('department_evaluation')
      }
      setSelfReviewVersions(sortVersionsDesc(getReviewVersionsFromResponse(res).filter(version => visibleReviewTypes.has(version.review_type))))
    } catch (err: any) {
      setSelfReviewVersions([])
      message.error(err?.response?.data?.message || '评审记录加载失败')
    } finally {
      setSelfReviewVersionsLoading(false)
    }
  }, [])

  const renderParticipantActions = useCallback((record: PerformanceParticipant) => (
    <ParticipantActions
      activity={currentActivity}
      record={record}
      canShowSelfReviewRecords={canShowSelfReviewRecords}
      navigateTo={navigate}
      onOpenAssessmentManager={openAssessmentManagerModal}
      onOpenSelfReviewRecords={handleOpenSelfReviewRecords}
      onRejectGoalRecords={handleRejectGoalRecords}
      onReloadActivityDetail={loadActivityDetail}
    />
  ), [
    canShowSelfReviewRecords,
    currentActivity,
    handleOpenSelfReviewRecords,
    handleRejectGoalRecords,
    loadActivityDetail,
    navigate,
    openAssessmentManagerModal,
  ])

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
      render: (_: string, record: PerformanceActivity) => {
        const s = getActivityStatusMeta(record)
        return <StatusTag color={s.color}>{s.label}</StatusTag>
      }
    },
    { title: '自评时间', key: 'self_eval', width: 200, render: (_, r) => `${formatDateTime(r.self_eval_start_at)} ~ ${formatDateTime(r.self_eval_end_at)}` },
    { title: '主管评分时间', key: 'mgr_eval', width: 200, render: (_, r) => `${formatDateTime(r.manager_eval_start_at)} ~ ${formatDateTime(r.manager_eval_end_at)}` },
    {
      title: '操作',
      key: 'actions',
      fixed: 'right',
      width: 220,
      onCell: () => ({ style: { verticalAlign: 'middle' } }),
      render: (_, record) => renderActivityActions(record),
    },
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

    if ((isMutengTargetWorkflowOpen(currentActivity) || activityStatus === 'target_setting') && ['pending', 'target_pending_approval', 'target_rejected', 'target_set'].includes(record.status)) {
      links.push(renderPermissionButton(
        'performance:goal:manage',
        <Button key="target" size="small" type="link" style={linkStyle} onClick={() => navigate(`/performance-goal-setting/${activityId}/${record.id}`)}>目标</Button>,
      ))
    }

    if (record.status === 'target_pending_approval') {
      links.push(renderPermissionButton(
        'performance:goal:manage',
        <Button key="approve" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-info)' }} onClick={() => {
          confirmParticipantWriteAction({
            title: `通过目标：${record.employee_name}`,
            content: '确认通过该员工的目标设定？通过后目标将生效。',
            okText: '确认通过',
            onOk: async () => {
              try {
                await performanceAPI.approveGoalRecords(record.id)
                message.success('目标已通过')
                if (currentActivity) loadActivityDetail(currentActivity)
              } catch (err: any) {
                message.error(err?.response?.data?.message || '审批失败')
                return Promise.reject(err)
              }
            },
          })
        }}>通过</Button>,
      ))
      links.push(renderPermissionButton(
        'performance:goal:manage',
        <Button key="reject" size="small" type="link" danger style={linkStyle} onClick={() => handleRejectGoalRecords(record)}>驳回</Button>,
      ))
    }

    if ((isMutengReviewPipelineOpen(currentActivity) || activityStatus === 'manager_evaluation') && ['self_submitted', 'manager_submitted', 'manager_recheck'].includes(record.status)) {
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
        const s = getParticipantStatusMeta(status, currentActivity?.flow_type === 'new')
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
      render: (_: string, record: PerformanceParticipant) => {
        const v = getParticipantDisplayLevel(record, currentActivity)
        if (!v) return <Text type="secondary">-</Text>
        return <StatusTag color={PERFORMANCE_LEVEL_TAG_COLORS[v] || 'default'}>{v}</StatusTag>
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

        if ((isMutengTargetWorkflowOpen(currentActivity) || activityStatus === 'target_setting') && ['pending', 'target_pending_approval', 'target_rejected', 'target_set'].includes(record.status) && !hasPermission('performance:goal:manage')) {
          links.push(renderPermissionButton('performance:goal:manage', <Button key="target-disabled" size="small" type="link" style={linkStyle}>目标</Button>))
        }
        const canSelfEvaluateRecord = SELF_EVAL_EDITABLE_ACTIVITY_STATUSES.includes(activityStatus || '') &&
          SELF_EVAL_EDITABLE_PARTICIPANT_STATUSES.includes(record.status)
        if (canSelfEvaluateRecord && !hasPermission('performance:self_eval:submit')) {
          links.push(renderPermissionButton('performance:self_eval:submit', <Button key="self-disabled" size="small" type="link" style={linkStyle}>自评</Button>))
        }
        if ((isMutengReviewPipelineOpen(currentActivity) || activityStatus === 'manager_evaluation') && ['self_submitted', 'manager_submitted', 'manager_recheck'].includes(record.status) && !hasPermission('performance:manager_eval:submit')) {
          links.push(renderPermissionButton('performance:manager_eval:submit', <Button key="mgr-disabled" size="small" type="link" style={linkStyle}>评分</Button>))
        }
        if ((currentActivity?.flow_type === 'new' ? isMutengResultPublished(record) : PERSONAL_RESULT_STATUSES.includes(record.status)) && !hasPermission('performance:result:view')) {
          links.push(renderPermissionButton('performance:result:view', <Button key="result-disabled" size="small" type="link" style={linkStyle}>结果</Button>))
        }
        if (currentActivity?.flow_type === 'new' && isMutengReviewPipelineOpen(currentActivity) && record.status === 'manager_confirmed' && !hasAnyPermission('performance:hr_review:submit', 'performance:activity:manage')) {
          links.push(renderPermissionButton('performance:hr_review:submit', <Button key="hr-review-disabled" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-primary)' }}>HR审核</Button>))
        } else if (currentActivity?.flow_type !== 'new' && currentActivity?.status === 'hr_confirmation' && record.status === 'manager_confirmed' && !hasPermission('performance:hr_confirm:submit')) {
          links.push(renderPermissionButton('performance:hr_confirm:submit', <Button key="hr-confirm-disabled" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-primary)' }}>HR确认</Button>))
        }
        if (record.status === 'target_pending_approval' && !hasPermission('performance:goal:manage')) {
          links.push(renderPermissionButton('performance:goal:manage', <Button key="approve-disabled" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-info)' }}>通过</Button>))
          links.push(renderPermissionButton('performance:goal:manage', <Button key="reject-disabled" size="small" type="link" danger style={linkStyle}>驳回</Button>))
        }

        // 目标设定：活动必须处于 target_setting 状态，且参与人状态允许
        if ((isMutengTargetWorkflowOpen(currentActivity) || activityStatus === 'target_setting') && ['pending', 'target_pending_approval', 'target_rejected', 'target_set'].includes(record.status) && hasPermission('performance:goal:manage')) {
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
        if ((isMutengReviewPipelineOpen(currentActivity) || activityStatus === 'manager_evaluation') && ['self_submitted', 'manager_submitted', 'manager_recheck'].includes(record.status) && hasPermission('performance:manager_eval:submit')) {
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
        if ((currentActivity?.flow_type === 'new' ? isMutengResultPublished(record) : PERSONAL_RESULT_STATUSES.includes(record.status)) && hasPermission('performance:result:view')) {
          links.push(
            <Button key="result" size="small" type="link" style={linkStyle} data-testid={`performance-participant-result-${record.id}`}
              onClick={() => navigate(`/performance-result/${activityId}/${record.id}`)}
            >结果</Button>
          )
        }
        if (currentActivity?.flow_type === 'new' && isMutengReviewPipelineOpen(currentActivity) && record.status === 'manager_confirmed' && hasAnyPermission('performance:hr_review:submit', 'performance:activity:manage')) {
          links.push(
            <Button key="hr-review" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-primary)' }} data-testid={`performance-participant-hr-confirm-${record.id}`}
              onClick={() => {
                confirmParticipantWriteAction({
                  title: `HR审核：${record.employee_name}`,
                  content: '确认完成 HR 审核？审核通过后结果将按流程公布，操作后不可轻易回退。',
                  okText: '确认HR审核',
                  onOk: async () => {
                    try {
                      await performanceAPI.confirmHRResult(record.id)
                      message.success('HR审核完成，结果已公布')
                      if (currentActivity) loadActivityDetail(currentActivity)
                    } catch (err: any) {
                      message.error(err?.response?.data?.message || 'HR审核失败')
                      return Promise.reject(err)
                    }
                  },
                })
              }}
            >HR审核</Button>
          )
        } else if (currentActivity?.flow_type !== 'new' && currentActivity?.status === 'hr_confirmation' && record.status === 'manager_confirmed' && hasPermission('performance:hr_confirm:submit')) {
          links.push(
            <Button key="hr-confirm" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-primary)' }} data-testid={`performance-participant-hr-confirm-${record.id}`}
              onClick={() => {
                confirmParticipantWriteAction({
                  title: `HR确认：${record.employee_name}`,
                  content: '确认完成 HR 确认？确认后该参与人绩效结果将进入已确认状态。',
                  okText: '确认HR确认',
                  onOk: async () => {
                    try {
                      await performanceAPI.confirmHRResult(record.id)
                      message.success('HR确认成功')
                      if (currentActivity) loadActivityDetail(currentActivity)
                    } catch (err: any) {
                      message.error(err?.response?.data?.message || 'HR确认失败')
                      return Promise.reject(err)
                    }
                  },
                })
              }}
            >HR确认</Button>
          )
        }
        if (record.status === 'target_pending_approval' && hasPermission('performance:goal:manage')) {
          links.push(
            <Button key="approve" size="small" type="link" style={{ ...linkStyle, color: 'var(--color-info)' }} data-testid={`performance-participant-approve-${record.id}`}
              onClick={() => {
                confirmParticipantWriteAction({
                  title: `通过目标：${record.employee_name}`,
                  content: '确认通过该员工的目标设定？通过后目标将生效。',
                  okText: '确认通过',
                  onOk: async () => {
                    try {
                      await performanceAPI.approveGoalRecords(record.id)
                      message.success('目标已通过')
                      if (currentActivity) loadActivityDetail(currentActivity)
                    } catch (err: any) {
                      message.error(err?.response?.data?.message || '审批失败')
                      return Promise.reject(err)
                    }
                  },
                })
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
      render: (v: string) => v ? <StatusTag color={PERFORMANCE_LEVEL_TAG_COLORS[v] || 'default'}>{v}</StatusTag> : <Text type="secondary">-</Text>,
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
      render: (v: string) => v ? <StatusTag color={PERFORMANCE_LEVEL_TAG_COLORS[v] || 'default'}>{v}</StatusTag> : <Text type="secondary">-</Text>,
    },
    { title: '操作', key: 'actions', fixed: 'right', width: 160, render: (_, record) => renderManagerParticipantActions(record) },
  ]

  const mergedHRParticipantColumns: ColumnsType<PerformanceParticipant> = hrParticipantColumns.map(column => (
    column.key === 'actions'
      ? { ...column, render: (_value: unknown, record: PerformanceParticipant) => renderParticipantActions(record) }
      : column
  ))
  const participantColumns =
    activeView === 'employee' ? employeeParticipantColumns :
    activeView === 'manager' ? managerParticipantColumns :
    mergedHRParticipantColumns

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
  const isEmployeeTodoActivity = useCallback((item: PerformanceActivity) => (
    Boolean(item.my_participant && renderMyParticipantActionButton(item))
  ), [renderMyParticipantActionButton])
  const isManagerTodoActivity = useCallback((item: PerformanceActivity) => (
    ['target_setting', 'manager_evaluation', 'manager_confirmation'].includes(item.status)
  ), [])
  const filteredActivities = React.useMemo(
    () => viewActivities.filter(item => {
      const matchName = !activitySearchText || item.name?.toLowerCase().includes(activitySearchText.toLowerCase())
      if (activityStatusFilter === ACTIVITY_STATUS_FILTER_TODO) {
        const matchTodo = activeView === 'employee'
          ? isEmployeeTodoActivity(item)
          : activeView === 'manager'
            ? isManagerTodoActivity(item)
            : true
        return matchName && matchTodo
      }
      const matchStatus = !selectedActivityStatuses || selectedActivityStatuses.includes(item.status)
      return matchName && matchStatus
    }),
    [
      viewActivities,
      activitySearchText,
      activityStatusFilter,
      selectedActivityStatuses,
      activeView,
      isEmployeeTodoActivity,
      isManagerTodoActivity,
    ],
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
  const employeeTodoCount = viewActivities.filter(isEmployeeTodoActivity).length
  const managerTodoCount = viewActivities.filter(isManagerTodoActivity).length
  // 芯片与列表同源：总数用已加载 viewActivities 长度，避免 total 与子集不一致
  const roleActivityStatCards = activeView === 'employee' ? [
    { title: '我的绩效活动', value: viewActivities.length, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)', filter: undefined as string | undefined },
    { title: '待我处理', value: employeeTodoCount, color: '#b45309', bg: '#fef3c7', filter: ACTIVITY_STATUS_FILTER_TODO },
    { title: '进行中', value: inProgressCount, color: '#0369a1', bg: '#e0f2fe', filter: ACTIVITY_STATUS_FILTER_IN_PROGRESS },
    { title: '已完成', value: confirmedCount, color: 'var(--color-success)', bg: '#dcfce7', filter: ACTIVITY_STATUS_FILTER_CONFIRMED },
  ] : activeView === 'manager' ? [
    { title: '团队绩效活动', value: viewActivities.length, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)', filter: undefined as string | undefined },
    { title: '待团队处理', value: managerTodoCount, color: '#b45309', bg: '#fef3c7', filter: ACTIVITY_STATUS_FILTER_TODO },
    { title: '评分阶段', value: viewActivities.filter(a => a.status === 'manager_evaluation').length, color: '#0369a1', bg: '#e0f2fe', filter: 'manager_evaluation' },
    { title: '已完成', value: confirmedCount, color: 'var(--color-success)', bg: '#dcfce7', filter: ACTIVITY_STATUS_FILTER_CONFIRMED },
  ] : [
    // 总数优先 activitiesTotal（服务端 total），与已加载列表取 max，避免截断时低估
    { title: '绩效活动总数', value: Math.max(activitiesTotal, viewActivities.length), color: 'var(--color-primary)', bg: 'var(--color-primary-bg)', filter: undefined as string | undefined },
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
      : '阶段待推进'
  const completionPercent = viewActivities.length > 0 ? Math.round((confirmedCount / viewActivities.length) * 100) : 0
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
    if (activeView === 'hr') return hrDetailSummaryCards

    return [
      { title: activeView === 'employee' ? '我的记录' : '负责员工', value: detailParticipants.length, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)' },
      { title: '待目标', value: detailParticipants.filter(record => ['pending', 'target_pending_approval', 'target_rejected', 'target_set'].includes(record.status)).length, color: '#0891b2', bg: '#cffafe' },
      { title: '待评分', value: detailParticipants.filter(record => ['self_submitted', 'manager_submitted', 'manager_recheck'].includes(record.status)).length, color: '#b45309', bg: '#fef3c7' },
      { title: '已确认', value: detailParticipants.filter(record => ['employee_confirmed', 'manager_confirmed', 'hr_confirmed', 'locked', 'result_confirmed'].includes(record.status)).length, color: 'var(--color-success)', bg: '#dcfce7' },
    ]
  }, [activeView, detailParticipants, hrDetailSummaryCards])
  const detailActions = activeView === 'hr' && currentActivity
    ? renderDetailActionButtons(currentActivity)
    : null

  const activityListActions = (
    <Space>
      {activeView === 'hr' && (
        <Tooltip title={hasPermission('performance:activity:manage') ? undefined : '你缺少绩效活动管理权限，需要联系管理员添加'}>
          <span>
            <Button
              type="primary"
              data-testid="performance-create-activity"
              icon={<PlusOutlined />}
              disabled={!hasPermission('performance:activity:manage')}
              onClick={() => openActivityModal()}
            >
              新建活动
            </Button>
          </span>
        </Tooltip>
      )}
      {activeView === 'hr' && (
        <Tooltip title={canImportActivity ? undefined : '你缺少绩效活动导入权限，需要联系管理员添加'}>
          <span>
            <Button
              data-testid="performance-import-excel"
              icon={<FileExcelOutlined />}
              disabled={!canImportActivity}
              onClick={() => setExcelImportOpen(true)}
            >
              Excel导入
            </Button>
          </span>
        </Tooltip>
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

      <div data-testid="performance-overview-card">
        <div
          style={{
            marginBottom: 16,
            padding: '16px 20px',
            borderRadius: 'var(--radius-lg)',
            background: `linear-gradient(135deg, ${viewMeta.softBg} 0%, #ffffff 58%, #f8fafc 100%)`,
            border: '1px solid var(--color-border-light)',
          }}
        >
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: 16,
              flexWrap: 'wrap',
            }}
          >
            <Space align="center" size={12} wrap>
              <div
                style={{
                  width: 40,
                  height: 40,
                  borderRadius: 'var(--radius-md)',
                  background: viewMeta.accent,
                  color: '#fff',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 20,
                  flexShrink: 0,
                }}
              >
                {viewMeta.icon}
              </div>
              <div>
                <Text strong style={{ fontSize: 18, color: 'var(--color-text-title)' }}>
                  {viewMeta.title}
                </Text>
                <div style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-xs)', marginTop: 2 }}>
                  {viewMeta.description}
                </div>
              </div>
              <Tag
                icon={<ClockCircleOutlined />}
                color={primaryTodoCount > 0 ? 'warning' : 'default'}
                style={{ borderRadius: 'var(--radius-pill)', marginInlineStart: 4 }}
              >
                {primaryTodoLabel} {primaryTodoCount}
              </Tag>
              <Tag
                icon={<CheckCircleOutlined />}
                color="success"
                style={{ borderRadius: 'var(--radius-pill)' }}
              >
                完成率 {completionPercent}%
              </Tag>
            </Space>
            <Space wrap>
              <Segmented
                value={activeView}
                options={performanceViewOptions}
                onChange={(value) => {
                  const next = value as PerformanceView
                  setActiveView(next)
                  writeStoredPerformanceView(next)
                  setActivityStatusFilter(undefined)
                  setSelectedParticipantIds([])
                }}
              />
              {activityListActions}
            </Space>
          </div>
        </div>

        {/* Compact filter chips (replaces heavy 4-card stat row) */}
        <div
          style={{
            display: 'flex',
            flexWrap: 'wrap',
            gap: 8,
            marginBottom: 16,
            alignItems: 'center',
          }}
          data-testid="performance-stat-filters"
        >
          {roleActivityStatCards.map((item) => {
            const isAllChip = item.filter === undefined
            const active = isAllChip
              ? activityStatusFilter === undefined
              : activityStatusFilter === item.filter
            return (
              <button
                key={item.title}
                type="button"
                aria-label={`查看${item.title}`}
                onClick={() => handleActivityStatClick(item.filter)}
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '6px 12px',
                  borderRadius: 'var(--radius-pill)',
                  border: active ? `1px solid ${item.color}` : '1px solid var(--color-border-light)',
                  background: active ? item.bg : 'var(--color-bg-card)',
                  color: 'var(--color-text)',
                  font: 'inherit',
                  cursor: 'pointer',
                  transition: 'border-color 0.2s, background 0.2s',
                }}
              >
                <span style={{
                  minWidth: 22,
                  height: 22,
                  borderRadius: 'var(--radius-pill)',
                  background: item.bg,
                  color: item.color,
                  display: 'inline-flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 12,
                  fontWeight: 700,
                  padding: '0 6px',
                }}>
                  {item.value}
                </span>
                <span style={{ fontSize: 'var(--font-size-sm)', fontWeight: 500 }}>{item.title}</span>
              </button>
            )
          })}
        </div>

          <PerformanceActivityEditor
            visible={activityModalVisible}
            editing={Boolean(editingActivity)}
            form={form}
            saving={activitySaving}
            performanceTemplates={performanceTemplates}
            performanceTemplatesLoading={performanceTemplatesLoading}
            activities={activities}
            currentActivityId={editingActivity?.id}
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
            title="绩效活动"
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
                background: 'var(--color-bg-subtle)',
                border: '1px solid var(--color-border-light)',
              }}
            >
              <div>
                <Text strong style={{ color: 'var(--color-text-title)' }}>{viewMeta.title}列表</Text>
                <div style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-xs)', marginTop: 2 }}>
                  共 {filteredActivities.length} 条 · 用上方芯片筛选状态，列表展示当前阶段主操作
                </div>
              </div>
              <Input.Search
                placeholder="搜索活动名称"
                allowClear
                onSearch={setActivitySearchText}
                onChange={e => { if (!e.target.value) setActivitySearchText('') }}
                style={{ width: 220 }}
              />
            </div>
            <Spin spinning={activitiesLoading}>
              <Table
                columns={activityColumns}
                dataSource={filteredActivities}
                rowKey="id"
                pagination={{ pageSize: 10 }}
                size="small"
                scroll={{ x: 1202 }}
                locale={{
                  emptyText: (
                    <Empty
                      description={
                        activitySearchText || activityStatusFilter
                          ? '当前筛选条件下无绩效活动'
                          : '暂无绩效活动'
                      }
                    >
                      {(activitySearchText || activityStatusFilter) ? (
                        <Button
                          type="link"
                          onClick={() => {
                            setActivitySearchText('')
                            setActivityStatusFilter(undefined)
                          }}
                        >
                          清除筛选
                        </Button>
                      ) : null}
                    </Empty>
                  ),
                }}
              />
            </Spin>
          </PageCard>
          </div>

      </div>

      <Drawer
        title={`活动详情：${currentActivity?.name || ''}`}
        placement="right"
        width="min(1000px, 100vw)"
        open={detailDrawerVisible}
        onClose={closeDetailDrawer}
        styles={{ body: { paddingTop: 12 }, footer: { paddingTop: 12 } }}
      >
        {currentActivity && (
          <div data-testid="performance-detail-content">
            <div
              style={{
                padding: '14px 16px',
                marginBottom: 12,
                borderRadius: 'var(--radius-lg)',
                background: viewMeta.softBg,
                border: '1px solid var(--color-border-light)',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
                <Space size={12}>
                  <div
                    style={{
                      width: 40,
                      height: 40,
                      borderRadius: 'var(--radius-md)',
                      background: viewMeta.accent,
                      color: '#fff',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontSize: 18,
                    }}
                  >
                    {viewMeta.icon}
                  </div>
                  <div>
                    <Text strong style={{ fontSize: 16, color: 'var(--color-text-title)' }}>{currentActivity.name}</Text>
                    <div style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-xs)', marginTop: 2 }}>
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

            {/* Sticky stage actions so HR can advance without scrolling */}
            {detailActions && (
              <div
                data-testid="performance-detail-actions"
                style={{
                  position: 'sticky',
                  top: 0,
                  zIndex: 5,
                  display: 'flex',
                  flexWrap: 'wrap',
                  gap: 8,
                  marginBottom: 12,
                  padding: '10px 12px',
                  borderRadius: 'var(--radius-md)',
                  background: 'var(--color-bg-card)',
                  border: '1px solid var(--color-border-light)',
                  boxShadow: 'var(--shadow-sm)',
                }}
              >
                {detailActions}
              </div>
            )}

            <Steps
              current={currentActivityStepItems.findIndex(item => item.status === 'process')}
              items={currentActivityStepItems}
              style={{ marginBottom: 16 }}
              size="small"
              responsive
            />
            <Descriptions column={2} size="small" style={{ marginBottom: 16 }} bordered>
              <Descriptions.Item label="周期类型">{getCycleLabel(currentActivity.cycle_type)}</Descriptions.Item>
              <Descriptions.Item label="绩效周期">{formatDateTime(currentActivity.start_date)} ~ {formatDateTime(currentActivity.end_date)}</Descriptions.Item>
              <Descriptions.Item label="自评时间">{formatDateTime(currentActivity.self_eval_start_at)} ~ {formatDateTime(currentActivity.self_eval_end_at)}</Descriptions.Item>
              <Descriptions.Item label="主管评分">{formatDateTime(currentActivity.manager_eval_start_at)} ~ {formatDateTime(currentActivity.manager_eval_end_at)}</Descriptions.Item>
              <Descriptions.Item label="结果确认" span={2}>{formatDateTime(currentActivity.result_confirm_start_at)} ~ {formatDateTime(currentActivity.result_confirm_end_at)}</Descriptions.Item>
            </Descriptions>

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
                    const bg = dist.status === 'exceeded' ? 'var(--color-error-bg)' : dist.status === 'warning' ? '#fffbe6' : 'var(--color-success-bg)'
                    const barColor = dist.status === 'exceeded' ? 'var(--color-error)' : dist.status === 'warning' ? 'var(--color-warning)' : 'var(--color-success)'
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
                locale={{ emptyText: '暂无参与人' }}
              />
            </Spin>
          </div>
        )}
      </Drawer>

      <SelfReviewHistoryDrawer
        open={selfReviewDrawerVisible}
        target={selfReviewTarget}
        versions={selfReviewVersions}
        loading={selfReviewVersionsLoading}
        onClose={() => {
          setSelfReviewDrawerVisible(false)
          setSelfReviewTarget(null)
          setSelfReviewVersions([])
        }}
      />

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
        <Alert showIcon type="info" style={{ marginBottom: 16 }} message="各等级比例总和必须等于 100%。" />

        {/* 可视化分布预览 */}
        {(() => {
          const formVals = distributionForm.getFieldsValue()
          const levels = ['S', 'A', 'B', 'C', 'D']
          const colors: Record<string, string> = {
            S: 'var(--color-error)',
            A: 'var(--color-primary)',
            B: 'var(--color-success)',
            C: 'var(--color-warning)',
            D: 'var(--color-error-dark)',
          }
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
      <PerformanceExcelImportWizard
        open={excelImportOpen}
        onCancel={() => setExcelImportOpen(false)}
        onCommitted={() => {
          void loadActivities()
        }}
      />

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
              batchEvalScore >= 100 ? 'magenta' :
              batchEvalScore >= 90 ? 'blue' :
              batchEvalScore >= 80 ? 'green' :
              batchEvalScore >= 60 ? 'gold' : 'red'
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
