import React, { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import {
  Typography, Input, InputNumber, Button, Space,
  message, Spin, Table, Tag, Modal, AutoComplete, Alert, Progress, Tooltip, Popconfirm
} from 'antd'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import {
  ArrowLeftOutlined, SaveOutlined, CheckCircleOutlined,
  PlusOutlined, DeleteOutlined, BulbOutlined
} from '@ant-design/icons'
import { performanceAPI, PerformanceActivity, PerformanceGoalApprovalLog, PerformanceGoalRecord, PerformanceParticipant } from '../services/api'
import { formatDateTime } from '../utils/format'

const { Title, Text } = Typography
const { TextArea } = Input

type NewFlowPlanRow = {
  key: string
  kind: 'fixed' | 'kpi' | 'okr'
  sourceIndex: number
  item: any
}

type GoalItemKind = 'quant' | 'action'

const getGoalRowKey = (record: any) => record.id ?? record.draft_key
const normalizePercentNumber = (value: number) => {
  if (!Number.isFinite(value)) return 0
  return Number(value.toFixed(2))
}
const normalizeWeightFraction = (value: number) => {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(1, Number(value.toFixed(4))))
}
const getWeightPercentValue = (weight: number) => normalizePercentNumber((Number(weight) || 0) * 100)
const getWeightFractionFromPercent = (percent: number | null) => normalizeWeightFraction((Number(percent) || 0) / 100)
const formatWeightPercent = (weight: number) => {
  const percent = getWeightPercentValue(weight)
  return `${Number.isInteger(percent) ? percent.toFixed(0) : percent}%`
}
const mapGoalRecordToQuantItem = (record: PerformanceGoalRecord, goalPhase: string) => ({
  id: record.id,
  item_name: record.item_name,
  item_definition: record.item_definition,
  weight: record.weight,
  weight_percent: String(getWeightPercentValue(record.weight || 0)),
  red_line_value: record.red_line_value,
  target_value: record.target_value,
  challenge_value: record.challenge_value,
  scoring_rule: record.scoring_rule,
  actual_result: record.actual_result,
  attachments: record.attachments || [],
  goal_phase: record.goal_phase || goalPhase,
  goal_type: record.goal_type || 'kpi',
  fixed_key: record.fixed_key || '',
  is_fixed: record.is_fixed || false,
  approval_status: record.approval_status,
  sort_order: record.sort_order
})

const mapGoalRecordToActionItem = (record: PerformanceGoalRecord, goalPhase: string) => ({
  id: record.id,
  item_name: record.item_name,
  item_definition: record.item_definition,
  weight: record.weight,
  weight_percent: String(getWeightPercentValue(record.weight || 0)),
  target_value: record.target_value || record.scoring_rule,
  actual_result: record.actual_result,
  attachments: record.attachments || [],
  goal_phase: record.goal_phase || goalPhase,
  goal_type: record.goal_type || (record.is_fixed ? 'fixed' : 'okr'),
  fixed_key: record.fixed_key || '',
  is_fixed: record.is_fixed || false,
  approval_status: record.approval_status,
  sort_order: record.sort_order
})

const mapGoalRecordsToGoalItems = (records: PerformanceGoalRecord[], goalPhase: string) => ({
  quant: records
    .filter(record => record.section_type === 'quantitative')
    .map(record => mapGoalRecordToQuantItem(record, goalPhase)),
  actions: records
    .filter(record => record.section_type === 'key_action')
    .map(record => mapGoalRecordToActionItem(record, goalPhase)),
})

const buildNewFlowPlanRows = (quantItems: any[], actionItems: any[], keyPrefix = ''): NewFlowPlanRow[] => {
  const prefix = keyPrefix ? `${keyPrefix}-` : ''
  return [
    ...actionItems
      .map((item, sourceIndex) => ({
        key: item.id ? `${prefix}fixed-${item.id}` : `${prefix}fixed-${item.fixed_key || item.draft_key}`,
        kind: item.is_fixed ? 'fixed' as const : 'okr' as const,
        sourceIndex,
        item,
      }))
      .filter(row => row.kind === 'fixed'),
    ...quantItems.map((item, sourceIndex) => ({
      key: item.id ? `${prefix}kpi-${item.id}` : `${prefix}kpi-${item.draft_key}`,
      kind: 'kpi' as const,
      sourceIndex,
      item,
    })),
    ...actionItems
      .map((item, sourceIndex) => ({
        key: item.id ? `${prefix}okr-${item.id}` : `${prefix}okr-${item.draft_key}`,
        kind: item.is_fixed ? 'fixed' as const : 'okr' as const,
        sourceIndex,
        item,
      }))
      .filter(row => row.kind === 'okr'),
  ]
}

const targetReadonlyParticipantStatuses = new Set([
  'target_set',
  'self_submitted',
  'manager_submitted',
  'result_confirmed',
  'employee_confirmed',
  'manager_recheck',
  'manager_confirmed',
  'hr_confirmed',
  'locked',
])

const PerformanceGoalSetting: React.FC = () => {
  const { activityId, participantId } = useParams<{ activityId: string; participantId: string }>()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const requestedGoalPhase = searchParams.get('phase') === 'review' ? 'review' : 'plan'
  const draftKeyRef = useRef(0)
  const nextDraftKey = (prefix: string) => `${prefix}-${draftKeyRef.current++}`
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [participant, setParticipant] = useState<PerformanceParticipant | null>(null)
  const [activity, setActivity] = useState<PerformanceActivity | null>(null)
  const [latestRejection, setLatestRejection] = useState<PerformanceGoalApprovalLog | null>(null)
  const [quantItems, setQuantItems] = useState<any[]>([])
  const [actionItems, setActionItems] = useState<any[]>([])
  const [reviewSupplementRows, setReviewSupplementRows] = useState<NewFlowPlanRow[]>([])
  const [suggestions, setSuggestions] = useState<any[]>([])
  const [showSuggestions, setShowSuggestions] = useState(false)
  const [validationVisible, setValidationVisible] = useState(false)
  const [quantSearchResults, setQuantSearchResults] = useState<Record<number, any[]>>({})
  const [actionSearchResults, setActionSearchResults] = useState<Record<number, any[]>>({})
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const isNewFlow = activity?.flow_type === 'new'
  const isReviewSupplementMode = isNewFlow && requestedGoalPhase === 'review'
  const currentGoalPhase = isReviewSupplementMode ? 'review' : (isNewFlow ? 'plan' : 'review')

  const loadData = useCallback(async () => {
    if (!participantId || !activityId) return
    setLoading(true)
    try {
      const [recordsRes, participantRes] = await Promise.all([
        performanceAPI.getGoalRecords(Number(participantId)),
        performanceAPI.getParticipant(Number(participantId))
      ])

      const recordsData = recordsRes.data || {}
      const allItems: PerformanceGoalRecord[] = recordsData.items || []
      setLatestRejection(recordsData.latest_rejection || null)
      const currentParticipant = participantRes.data?.participant || participantRes.data
      const currentActivity = participantRes.data?.activity || null
      setParticipant(currentParticipant)
      setActivity(currentActivity)
      const isNewFlow = currentActivity?.flow_type === 'new'
      const targetGoalPhase = isNewFlow && requestedGoalPhase === 'review' ? 'review' : (isNewFlow ? 'plan' : 'review')
      const targetItems = isNewFlow
        ? allItems.filter((i: PerformanceGoalRecord) => String(i.goal_phase || 'review').trim() === targetGoalPhase)
        : allItems
      const { quant, actions } = mapGoalRecordsToGoalItems(targetItems, targetGoalPhase)
      const reviewItems = isNewFlow && targetGoalPhase === 'plan'
        ? allItems.filter((i: PerformanceGoalRecord) => String(i.goal_phase || 'review').trim() === 'review')
        : []
      const reviewSupplementItems = mapGoalRecordsToGoalItems(reviewItems, 'review')

      setQuantItems(quant.length > 0 ? quant : [{ ...newQuantItem(targetGoalPhase), goal_phase: targetGoalPhase }])
      const nextActions = actions.length > 0 ? actions : [{ ...newActionItem(targetGoalPhase), goal_phase: targetGoalPhase }]
      setActionItems(isNewFlow ? ensureFixedActionItems(nextActions, targetGoalPhase) : nextActions)
      setReviewSupplementRows(
        reviewItems.length > 0
          ? buildNewFlowPlanRows(reviewSupplementItems.quant, reviewSupplementItems.actions, 'review')
          : []
      )
    } catch {
      message.error('加载数据失败')
    } finally {
      setLoading(false)
    }
  }, [participantId, activityId, requestedGoalPhase])

  useEffect(() => { loadData() }, [loadData])

  const focusGoalNameInput = (testId: string) => {
    window.setTimeout(() => {
      const target = document.querySelector<HTMLElement>(`[data-testid="${testId}"] input`) ||
        document.querySelector<HTMLElement>(`[data-testid="${testId}"]`)
      target?.focus()
    }, 0)
  }

  function newQuantItem(goalPhase = currentGoalPhase) {
    return {
      draft_key: nextDraftKey('quant'),
      id: undefined,
      item_name: '',
      item_definition: '',
      weight: 0,
      weight_percent: '0',
      red_line_value: '',
      target_value: '',
      challenge_value: '',
      scoring_rule: '',
      actual_result: '',
      attachments: [],
      goal_phase: goalPhase,
      goal_type: 'kpi',
      fixed_key: '',
      is_fixed: false,
      sort_order: 0
    }
  }

  function newActionItem(goalPhase = currentGoalPhase) {
    return {
      draft_key: nextDraftKey('action'),
      id: undefined,
      item_name: '',
      item_definition: '',
      weight: 0,
      weight_percent: '0',
      target_value: '',
      actual_result: '',
      attachments: [],
      goal_phase: goalPhase,
      goal_type: 'okr',
      fixed_key: '',
      is_fixed: false,
      sort_order: 0
    }
  }

  function newFixedActionItem(fixedKey: string, goalPhase = currentGoalPhase) {
    const fixedMap: Record<string, { item_name: string; item_definition: string }> = {
      manager_arrangement: {
        item_name: '上级安排事项完成情况',
        item_definition: '上级安排的所有事项需在规定时间内完成，工作结果得到领导认可',
      },
      values_discipline: {
        item_name: '价值观及工作纪律',
        item_definition: '拥抱公司价值观，不得违反公司管理制度、规范等',
      },
    }
    return {
      ...newActionItem(goalPhase),
      ...(fixedMap[fixedKey] || fixedMap.manager_arrangement),
      weight: 0.15,
      weight_percent: '15',
      goal_phase: goalPhase,
      goal_type: 'fixed',
      fixed_key: fixedKey,
      is_fixed: true,
    }
  }

  function ensureFixedActionItems(items: any[], goalPhase = currentGoalPhase) {
    const byKey = new Map(items.map(item => [String(item.fixed_key || ''), item]))
    const fixedKeys = ['manager_arrangement', 'values_discipline']
    const fixedItems = fixedKeys.map(key => {
      const base = newFixedActionItem(key, goalPhase)
      const existing = byKey.get(key) || {}
      return {
        ...base,
        ...existing,
        item_name: base.item_name,
        item_definition: base.item_definition,
        target_value: existing.target_value || base.item_definition,
        weight: 0.15,
        weight_percent: '15',
        goal_type: 'fixed',
        goal_phase: goalPhase,
        fixed_key: key,
        is_fixed: true,
      }
    })
    const variableItems = items.filter(item => !item.fixed_key)
    return [...fixedItems, ...variableItems]
  }

  const handleAddQuantItem = () => {
    if (targetSettingReadonly) {
      message.warning('目标设定已审批通过，无法修改')
      return
    }
    const nextIndex = quantItems.length
    setQuantItems([...quantItems, newQuantItem()])
    focusGoalNameInput(`performance-goal-quant-name-${nextIndex}`)
  }

  const handleRemoveQuantItem = (index: number) => {
    if (targetSettingReadonly) {
      message.warning('目标设定已审批通过，无法修改')
      return
    }
    if (quantItems.length <= 1) {
      message.warning('至少保留一个量化指标')
      return
    }
    setQuantItems(quantItems.filter((_, i) => i !== index))
  }

  const handleAddActionItem = () => {
    if (targetSettingReadonly) {
      message.warning('目标设定已审批通过，无法修改')
      return
    }
    const nextIndex = actionItems.length
    setActionItems([...actionItems, newActionItem()])
    focusGoalNameInput(`performance-goal-action-name-${nextIndex}`)
  }

  const handleRemoveActionItem = (index: number) => {
    if (targetSettingReadonly) {
      message.warning('目标设定已审批通过，无法修改')
      return
    }
    if (actionItems[index]?.is_fixed) {
      message.warning('固定项不可删除')
      return
    }
    if (actionItems.length <= 1) {
      message.warning('至少保留一个关键行动')
      return
    }
    setActionItems(actionItems.filter((_, i) => i !== index))
  }

  const handleQuantItemChange = (index: number, field: string, value: any) => {
    if (targetSettingReadonly) return
    const updated = [...quantItems]
    const nextValue = field === 'weight' ? normalizeWeightFraction(Number(value) || 0) : value
    updated[index] = {
      ...updated[index],
      [field]: nextValue,
      __touched: true,
      ...(field === 'weight' ? { weight_percent: String(getWeightPercentValue(nextValue)) } : {})
    }
    setQuantItems(updated)
  }

  const handleActionItemChange = (index: number, field: string, value: any) => {
    if (targetSettingReadonly) return
    if (actionItems[index]?.is_fixed && ['item_name', 'item_definition', 'weight'].includes(field)) {
      return
    }
    const updated = [...actionItems]
    const nextValue = field === 'weight' ? normalizeWeightFraction(Number(value) || 0) : value
    updated[index] = {
      ...updated[index],
      [field]: nextValue,
      __touched: true,
      ...(field === 'weight' ? { weight_percent: String(getWeightPercentValue(nextValue)) } : {})
    }
    setActionItems(updated)
  }

  const quantWeightTotal = quantItems.reduce((sum, i) => sum + (i.weight || 0), 0)
  const actionWeightTotal = actionItems.reduce((sum, i) => sum + (i.weight || 0), 0)
  const totalWeight = quantWeightTotal + actionWeightTotal
  const fixedWeightTotal = actionItems.filter(item => item.is_fixed).reduce((sum, i) => sum + (i.weight || 0), 0)
  const variableWeightTotal = totalWeight - fixedWeightTotal
  const remainingWeight = normalizeWeightFraction(Math.max(0, 1 - totalWeight))
  const formatPercent = formatWeightPercent
  const targetSettingReadonly =
    isReviewSupplementMode
      ? Boolean(participant && !['pending', 'target_pending_approval', 'target_rejected', 'target_set'].includes(participant.status)) ||
        Boolean(activity && !['target_setting', 'self_evaluation'].includes(activity.status))
      : Boolean(participant && targetReadonlyParticipantStatuses.has(participant.status)) ||
        [...quantItems, ...actionItems].some(item => item.approval_status === 'approved')

  const getGoalItemCompletion = (item: any, kind: GoalItemKind) => {
    if (item?.is_fixed) {
      return { label: '固定项', color: 'default', tooltip: '模板固定职责项，无需手动维护基础信息' }
    }

    const requiredFields = isNewFlow
      ? ['item_name', 'item_definition', 'target_value']
      : kind === 'quant'
      ? ['item_name', 'item_definition', 'target_value', 'red_line_value', 'challenge_value', 'scoring_rule']
      : ['item_name', 'item_definition', 'target_value']
    const filledCount = requiredFields.filter(field => String(item?.[field] || '').trim()).length
      + (Number(item?.weight || 0) > 0 ? 1 : 0)
    const totalCount = requiredFields.length + 1

    if (filledCount === totalCount) {
      return { label: '已完整', color: 'success', tooltip: '必填内容和权重已填写完整' }
    }
    if (filledCount === 0 || isBlankGoalItem(item)) {
      return { label: '未填写', color: 'default', tooltip: `需完成 ${totalCount} 项必填内容` }
    }
    return { label: `${filledCount}/${totalCount}`, color: 'warning', tooltip: `还有 ${totalCount - filledCount} 项待完善` }
  }

  const renderGoalItemStatus = (item: any, kind: GoalItemKind, idx: number) => {
    const status = getGoalItemCompletion(item, kind)
    return (
      <Tooltip title={status.tooltip}>
        <Space direction="vertical" size={2} align="center" style={{ width: '100%' }}>
          <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>#{idx + 1}</Text>
          <Tag color={status.color} style={{ marginInlineEnd: 0 }}>{status.label}</Tag>
        </Space>
      </Tooltip>
    )
  }

  const getBalancedItemWeight = (itemWeight: number) => normalizeWeightFraction(itemWeight + (1 - totalWeight))

  const handleBalanceItemWeight = (kind: GoalItemKind, idx: number) => {
    if (targetSettingReadonly) return
    const item = kind === 'quant' ? quantItems[idx] : actionItems[idx]
    if (!item || item.is_fixed) return
    if (weightBalanced) {
      message.info('当前权重已配平')
      return
    }
    const nextWeight = getBalancedItemWeight(Number(item.weight || 0))
    if (nextWeight <= 0 && totalWeight > 1) {
      message.warning('当前总权重超出较多，请先调低其他项目')
      return
    }
    if (kind === 'quant') {
      handleQuantItemChange(idx, 'weight', nextWeight)
    } else {
      handleActionItemChange(idx, 'weight', nextWeight)
    }
    message.success(`已将该行权重调整为 ${formatPercent(nextWeight)}`)
  }

  const renderWeightControl = (
    kind: GoalItemKind,
    idx: number,
    item: any,
    error: string,
    disabled = false,
  ) => {
    const nextWeight = getBalancedItemWeight(Number(item?.weight || 0))
    const balanceDisabled = disabled || weightBalanced || item?.is_fixed
    const balanceTooltip = weightBalanced
      ? '权重已配平'
      : `将该行调整为 ${formatPercent(nextWeight)}，使总权重达到 100%`
    return renderFieldWithFeedback(
      <Space.Compact style={{ width: '100%' }}>
        <InputNumber
          data-testid={kind === 'quant' ? `performance-goal-quant-weight-${idx}` : `performance-goal-action-weight-${idx}`}
          min={0}
          max={100}
          value={getWeightPercentValue(item?.weight || 0)}
          onChange={val => (kind === 'quant' ? handleQuantItemChange : handleActionItemChange)(idx, 'weight', getWeightFractionFromPercent(val))}
          status={error ? 'error' : undefined}
          style={{ width: '100%' }}
          disabled={disabled}
        />
        <Button disabled>%</Button>
        <Tooltip title={balanceTooltip}>
          <Button
            data-testid={`performance-goal-balance-weight-${kind}-${idx}`}
            aria-label="补齐权重"
            icon={<CheckCircleOutlined />}
            disabled={balanceDisabled}
            onClick={() => handleBalanceItemWeight(kind, idx)}
          />
        </Tooltip>
      </Space.Compact>,
      error
    )
  }

  const renderDeleteAction = (
    onDelete: () => void,
    options: { disabled?: boolean; disabledReason?: string; directReason?: string } = {},
  ) => {
    const button = (
      <Button
        aria-label="删除目标项"
        type="text"
        danger
        icon={<DeleteOutlined />}
        onClick={options.directReason ? onDelete : undefined}
        disabled={options.disabled}
      />
    )
    if (options.disabled) {
      return <Tooltip title={options.disabledReason || '不可删除'}>{button}</Tooltip>
    }
    if (options.directReason) {
      return <Tooltip title={options.directReason}>{button}</Tooltip>
    }
    return (
      <Popconfirm
        title="确认删除该目标项？"
        okText="删除"
        cancelText="取消"
        okButtonProps={{ danger: true }}
        onConfirm={onDelete}
      >
        <Tooltip title="删除该目标项">{button}</Tooltip>
      </Popconfirm>
    )
  }

  const renderGoalSectionTitle = (title: string, count: number, weight: number, hint: string) => (
    <Space size={8} wrap>
      <span>{title}</span>
      <Tag style={{ marginInlineEnd: 0 }}>{count} 项</Tag>
      <Tag color={Math.abs(totalWeight - 1) < 0.001 ? 'success' : 'warning'} style={{ marginInlineEnd: 0 }}>
        {formatPercent(weight)}
      </Tag>
      <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>{hint}</Text>
    </Space>
  )

  const loadSuggestions = async () => {
    if (targetSettingReadonly) {
      message.warning('目标设定已审批通过，无法修改')
      return
    }
    if (!participantId) return
    try {
      const res = await performanceAPI.getGoalSuggestions(Number(participantId))
      setSuggestions(res.data?.suggestions || [])
      setShowSuggestions(true)
    } catch {
      message.error('获取建议失败')
    }
  }

  const applySuggestion = (suggestion: any) => {
    if (targetSettingReadonly) {
      message.warning('目标设定已审批通过，无法修改')
      return
    }
    const newItem = {
      ...newQuantItem(),
      item_name: suggestion.name || suggestion.item_name,
      item_definition: suggestion.description || suggestion.item_definition,
      red_line_value: suggestion.red_line_value || '',
      target_value: suggestion.target_value || '',
      challenge_value: suggestion.challenge_value || '',
      scoring_rule: suggestion.scoring_rule || '',
      weight: suggestion.weight || 0,
      __touched: true,
    }
    if (suggestion.section_type === 'key_action') {
      setActionItems([...actionItems, {
        ...newActionItem(),
        item_name: newItem.item_name,
        item_definition: newItem.item_definition,
        target_value: suggestion.target_value || suggestion.scoring_rule || '',
        weight: newItem.weight,
        __touched: true,
      }])
    } else {
      setQuantItems([...quantItems, newItem])
    }
    setShowSuggestions(false)
    message.success('已应用建议')
  }

  const searchIndicators = useCallback((keyword: string, resultsSetter: React.Dispatch<React.SetStateAction<Record<number, any[]>>>, rowIndex: number, sectionType: string) => {
    if (targetSettingReadonly) return
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    const libraryId = activity?.indicator_library_id
    if (!libraryId) {
      resultsSetter(prev => ({ ...prev, [rowIndex]: [] }))
      return
    }
    if (!keyword || keyword.trim().length < 1) {
      resultsSetter(prev => ({ ...prev, [rowIndex]: [] }))
      return
    }
    searchTimerRef.current = setTimeout(async () => {
      try {
        const res: any = await performanceAPI.searchIndicatorItems({ keyword: keyword.trim(), library_ids: [libraryId], section_type: sectionType })
        const data = res.data || res
        const raw: any[] = data?.items || []
        resultsSetter(prev => ({ ...prev, [rowIndex]: raw }))
      } catch {
        resultsSetter(prev => ({ ...prev, [rowIndex]: [] }))
      }
    }, 300)
  }, [activity?.indicator_library_id, targetSettingReadonly])

  const getSearchOptions = (results: any[]) =>
    results.map((item: any) => ({
      value: item.name,
      label: `${item.name}${item.description ? ' — ' + item.description.slice(0, 40) : ''}`,
    }))

  const handleIndicatorSelect = (
    value: string,
    rowIndex: number,
    sourceItems: any[],
    setter: React.Dispatch<React.SetStateAction<any[]>>,
    allItems: any[],
    isQuant: boolean,
  ) => {
    const matched = sourceItems.find((item: any) => item.name === value)
    if (!matched) return
    const patch: Record<string, any> = {
      item_name: matched.name,
      item_definition: matched.description || '',
    }
    if (isQuant) {
      patch.red_line_value = matched.red_line_value || ''
      patch.target_value = matched.target_value || ''
      patch.challenge_value = matched.challenge_value || ''
      patch.scoring_rule = matched.scoring_rule || ''
    } else {
      patch.target_value = matched.target_value || matched.scoring_rule || ''
    }
    setter(allItems.map((item, idx) => idx === rowIndex ? { ...item, ...patch, __touched: true } : item))
  }

  const isBlankGoalItem = (item: any) =>
    !String(item.item_name || '').trim() &&
    !String(item.item_definition || '').trim() &&
    !String(item.target_value || '').trim() &&
    !String(item.red_line_value || '').trim() &&
    !String(item.challenge_value || '').trim() &&
    !String(item.scoring_rule || '').trim() &&
    !(item.weight || 0)

  const shouldValidateGoalItem = (item: any) => {
    if (targetSettingReadonly || item?.is_fixed) return false
    if (isNewFlow && isBlankGoalItem(item) && !item?.__touched) return false
    return validationVisible || Boolean(item?.__touched)
  }

  const getGoalFieldError = (item: any, field: string, kind: 'quant' | 'action') => {
    if (!shouldValidateGoalItem(item)) return ''
    if (field === 'item_name' && !String(item.item_name || '').trim()) {
      return kind === 'quant' ? '请填写指标名称' : '请填写重点计划'
    }
    if (field === 'item_definition' && !String(item.item_definition || '').trim()) {
      return kind === 'quant' ? '请填写指标定义' : '请填写计划说明'
    }
    if (field === 'target_value' && !String(item.target_value || '').trim()) {
      return kind === 'quant' ? '请填写目标值' : '请填写完成标准'
    }
    if (field === 'weight' && !(Number(item.weight) > 0)) {
      return '请填写权重'
    }
    if (field === 'red_line_value' && !String(item.red_line_value || '').trim()) {
      return '请填写红线值'
    }
    if (field === 'challenge_value' && !String(item.challenge_value || '').trim()) {
      return '请填写挑战值'
    }
    if (field === 'scoring_rule' && !String(item.scoring_rule || '').trim()) {
      return '请填写考核标准'
    }
    return ''
  }

  const renderFieldWithFeedback = (field: React.ReactNode, error: string) => (
    <div style={{ width: '100%' }}>
      {field}
      {error && (
        <Text type="danger" style={{ display: 'block', marginTop: 4, fontSize: 'var(--font-size-xs)' }}>
          {error}
        </Text>
      )}
    </div>
  )

  const buildPayload = () => {
    const quantSourceItems = isNewFlow
      ? quantItems.filter(item => !isBlankGoalItem(item))
      : quantItems
    const actionSourceItems = isNewFlow
      ? actionItems.filter(item => item.is_fixed || !isBlankGoalItem(item))
      : actionItems
    const items = [
      ...quantSourceItems.map((item, idx) => ({
        id: item.id,
        section_type: 'quantitative',
        goal_phase: currentGoalPhase,
        goal_type: item.goal_type || 'kpi',
        fixed_key: item.fixed_key || '',
        is_fixed: Boolean(item.is_fixed),
        item_name: item.item_name,
        item_definition: item.item_definition,
        weight: item.weight,
        red_line_value: item.red_line_value,
        target_value: item.target_value,
        challenge_value: item.challenge_value,
        scoring_rule: item.scoring_rule,
        actual_result: item.actual_result,
        attachments: item.attachments,
        sort_order: idx
      })),
      ...actionSourceItems.map((item, idx) => ({
        id: item.id,
        section_type: 'key_action',
        goal_phase: currentGoalPhase,
        goal_type: item.goal_type || (item.is_fixed ? 'fixed' : 'okr'),
        fixed_key: item.fixed_key || '',
        is_fixed: Boolean(item.is_fixed),
        item_name: item.item_name,
        item_definition: item.item_definition,
        weight: item.weight,
        target_value: item.is_fixed ? (item.target_value || item.item_definition) : item.target_value,
        actual_result: item.actual_result,
        attachments: item.attachments,
        sort_order: quantSourceItems.length + idx
      }))
    ]
    return items
  }

  const handleSaveDraft = async () => {
    if (!participantId) return
    if (targetSettingReadonly) {
      message.warning('目标设定已审批通过，无法修改')
      return
    }
    const items = buildPayload()
    if (items.some(i => !i.item_name)) {
      message.warning('请填写所有指标名称')
      return
    }
    setSaving(true)
    try {
      if (isReviewSupplementMode) {
        await performanceAPI.batchSaveReviewGoalRecords(Number(participantId), { items })
        message.success('补录保存成功')
      } else {
        await performanceAPI.batchSaveGoalRecords(Number(participantId), { items })
        message.success('草稿保存成功')
      }
    } catch (err: any) {
      message.error(err?.response?.data?.message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const validateRequiredFields = () => {
    if (isNewFlow) {
      const activeQuantItems = quantItems.filter(item => !isBlankGoalItem(item))
      const variableActions = actionItems.filter(item => !item.is_fixed && !isBlankGoalItem(item))
      if ([...activeQuantItems, ...variableActions].some(i => !String(i.item_name || '').trim())) {
        message.warning('请填写所有目标/关键职责事项名称')
        return false
      }
      if ([...activeQuantItems, ...variableActions].some(i => !String(i.item_definition || '').trim())) {
        message.warning('请填写目标/关键职责事项说明')
        return false
      }
      if (activeQuantItems.some(i => !String(i.target_value || '').trim())) {
        message.warning('请填写 KPI 目标值')
        return false
      }
      if (variableActions.some(i => !String(i.target_value || '').trim())) {
        message.warning('请填写 OKR / 重点计划完成标准')
        return false
      }
      if ([...activeQuantItems, ...variableActions].some(i => !(Number(i.weight) > 0))) {
        message.warning('请填写每项目标权重')
        return false
      }
      return true
    }

    if (quantItems.some(i => !String(i.item_name || '').trim())) {
      message.warning('请填写所有量化指标名称')
      return false
    }
    if (actionItems.some(i => !String(i.item_name || '').trim())) {
      message.warning('请填写所有关键行动名称')
      return false
    }
    if (quantItems.some(i => !String(i.item_definition || '').trim())) {
      message.warning('请填写量化指标定义及口径说明')
      return false
    }
    if (actionItems.some(i => !String(i.item_definition || '').trim())) {
      message.warning('请填写关键行动定义及口径说明')
      return false
    }
    if (quantItems.some(i =>
      !String(i.red_line_value || '').trim() ||
      !String(i.target_value || '').trim() ||
      !String(i.challenge_value || '').trim() ||
      !String(i.scoring_rule || '').trim()
    )) {
      message.warning('请填写量化指标的红线值、目标值、挑战值和考核标准')
      return false
    }
    if (actionItems.some(i => !String(i.target_value || '').trim())) {
      message.warning('请填写关键行动的定性目标')
      return false
    }
    if ([...quantItems, ...actionItems].some(i => !(Number(i.weight) > 0))) {
      message.warning('请填写每项目标权重')
      return false
    }
    return true
  }

  const handleSubmit = async () => {
    if (!participantId) return
    if (targetSettingReadonly) {
      message.warning('目标设定已审批通过，无法修改')
      return
    }
    setValidationVisible(true)
    if (Math.abs(totalWeight - 1) > 0.001) {
      message.error(`权重合计必须为100%，当前为 ${(totalWeight * 100).toFixed(0)}%`)
      return
    }
    if (!isNewFlow && (quantWeightTotal < 0.65 || quantWeightTotal > 0.75)) {
      message.error('量化指标权重需约70%（允许65%-75%）')
      return
    }
    if (!isNewFlow && (actionWeightTotal < 0.25 || actionWeightTotal > 0.35)) {
      message.error('关键行动权重需约30%（允许25%-35%）')
      return
    }

    const items = buildPayload()
    if (!validateRequiredFields()) {
      return
    }

    Modal.confirm({
      title: isReviewSupplementMode ? '确认完成补录' : '确认提交目标',
      content: isReviewSupplementMode ? '补录的上一季度考核指标将作为本期自评和评分依据，确认继续？' : '提交后将进入审批流程，确认继续？',
      onOk: async () => {
        setSubmitting(true)
        try {
          if (isReviewSupplementMode) {
            await performanceAPI.batchSaveReviewGoalRecords(Number(participantId), { items })
            message.success('上一季度考核指标已补录')
          } else {
            await performanceAPI.batchSaveGoalRecords(Number(participantId), { items })
            await performanceAPI.submitGoalApproval(Number(participantId))
            message.success('目标已提交')
          }
          navigate(-1)
        } catch (err: any) {
          message.error(err?.response?.data?.message || '提交失败')
        } finally {
          setSubmitting(false)
        }
      }
    })
  }

  const quantColumns = [
    {
      title: '状态',
      key: 'status',
      width: 92,
      align: 'center' as const,
      render: (_: any, __: any, idx: number) => renderGoalItemStatus(quantItems[idx], 'quant', idx),
    },
    {
      title: '指标名称',
      dataIndex: 'item_name',
      key: 'item_name',
      width: 220,
      render: (_: any, __: any, idx: number) => {
        const error = getGoalFieldError(quantItems[idx], 'item_name', 'quant')
        return renderFieldWithFeedback(
          <AutoComplete
            data-testid={`performance-goal-quant-name-${idx}`}
            value={quantItems[idx]?.item_name}
            options={getSearchOptions(quantSearchResults[idx] || [])}
            onSearch={(val) => searchIndicators(val, setQuantSearchResults, idx, 'quantitative')}
            onChange={(val) => handleQuantItemChange(idx, 'item_name', val)}
            onSelect={(val) => handleIndicatorSelect(val, idx, quantSearchResults[idx] || [], setQuantItems, quantItems, true)}
            placeholder="输入关键词搜索指标"
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly}
            style={{ width: '100%' }}
          />,
          error
        )
      }
    },
    {
      title: '指标定义',
      dataIndex: 'item_definition',
      key: 'item_definition',
      width: 260,
      render: (_: any, __: any, idx: number) => {
        const error = getGoalFieldError(quantItems[idx], 'item_definition', 'quant')
        return renderFieldWithFeedback(
          <TextArea
            data-testid={`performance-goal-quant-definition-${idx}`}
            value={quantItems[idx]?.item_definition}
            onChange={e => handleQuantItemChange(idx, 'item_definition', e.target.value)}
            rows={2}
            placeholder="明确指标范围和计算公式"
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly}
          />,
          error
        )
      }
    },
    {
      title: '权重%',
      key: 'weight',
      width: 168,
      render: (_: any, __: any, idx: number) => {
        const error = getGoalFieldError(quantItems[idx], 'weight', 'quant')
        return renderWeightControl('quant', idx, quantItems[idx], error, targetSettingReadonly)
      }
    },
    {
      title: '红线值',
      dataIndex: 'red_line_value',
      key: 'red_line_value',
      width: 140,
      render: (_: any, __: any, idx: number) => {
        const error = getGoalFieldError(quantItems[idx], 'red_line_value', 'quant')
        return renderFieldWithFeedback(
          <Input
            data-testid={`performance-goal-quant-red-line-${idx}`}
            value={quantItems[idx]?.red_line_value}
            onChange={e => handleQuantItemChange(idx, 'red_line_value', e.target.value)}
            placeholder="最低"
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly}
          />,
          error
        )
      }
    },
    {
      title: '目标值',
      dataIndex: 'target_value',
      key: 'target_value',
      width: 160,
      render: (_: any, __: any, idx: number) => {
        const error = getGoalFieldError(quantItems[idx], 'target_value', 'quant')
        return renderFieldWithFeedback(
          <Input
            data-testid={`performance-goal-quant-target-${idx}`}
            value={quantItems[idx]?.target_value}
            onChange={e => handleQuantItemChange(idx, 'target_value', e.target.value)}
            placeholder="目标"
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly}
          />,
          error
        )
      }
    },
    {
      title: '挑战值',
      dataIndex: 'challenge_value',
      key: 'challenge_value',
      width: 140,
      render: (_: any, __: any, idx: number) => {
        const error = getGoalFieldError(quantItems[idx], 'challenge_value', 'quant')
        return renderFieldWithFeedback(
          <Input
            data-testid={`performance-goal-quant-challenge-${idx}`}
            value={quantItems[idx]?.challenge_value}
            onChange={e => handleQuantItemChange(idx, 'challenge_value', e.target.value)}
            placeholder="挑战"
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly}
          />,
          error
        )
      }
    },
    {
      title: '考核标准',
      dataIndex: 'scoring_rule',
      key: 'scoring_rule',
      width: 260,
      render: (_: any, __: any, idx: number) => {
        const error = getGoalFieldError(quantItems[idx], 'scoring_rule', 'quant')
        return renderFieldWithFeedback(
          <TextArea
            data-testid={`performance-goal-quant-scoring-rule-${idx}`}
            value={quantItems[idx]?.scoring_rule}
            onChange={e => handleQuantItemChange(idx, 'scoring_rule', e.target.value)}
            rows={2}
            placeholder="定量按区间/上限设置"
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly}
          />,
          error
        )
      }
    },
    {
      title: '',
      key: 'action',
      width: 64,
      align: 'center' as const,
      render: (_: any, __: any, idx: number) => renderDeleteAction(
        () => handleRemoveQuantItem(idx),
        {
          disabled: targetSettingReadonly,
          disabledReason: readonlyLabel,
          directReason: quantItems.length <= 1 ? '至少保留一个量化指标' : undefined,
        }
      )
    }
  ]

  const actionColumns = [
    {
      title: '状态',
      key: 'status',
      width: 92,
      align: 'center' as const,
      render: (_: any, __: any, idx: number) => renderGoalItemStatus(actionItems[idx], 'action', idx),
    },
    {
      title: '重点计划',
      dataIndex: 'item_name',
      key: 'item_name',
      width: 180,
      render: (_: any, __: any, idx: number) => {
        const error = getGoalFieldError(actionItems[idx], 'item_name', 'action')
        return renderFieldWithFeedback(
          <AutoComplete
            data-testid={`performance-goal-action-name-${idx}`}
            value={actionItems[idx]?.item_name}
            options={getSearchOptions(actionSearchResults[idx] || [])}
            onSearch={(val) => searchIndicators(val, setActionSearchResults, idx, 'key_action')}
            onChange={(val) => handleActionItemChange(idx, 'item_name', val)}
            onSelect={(val) => handleIndicatorSelect(val, idx, actionSearchResults[idx] || [], setActionItems, actionItems, false)}
            placeholder="输入关键词搜索指标"
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly || actionItems[idx]?.is_fixed}
            style={{ width: '100%' }}
          />,
          error
        )
      }
    },
    {
      title: '指标定义及口径说明',
      dataIndex: 'item_definition',
      key: 'item_definition',
      width: '30%',
      render: (_: any, __: any, idx: number) => {
        const error = getGoalFieldError(actionItems[idx], 'item_definition', 'action')
        return renderFieldWithFeedback(
          <TextArea
            data-testid={`performance-goal-action-definition-${idx}`}
            value={actionItems[idx]?.item_definition}
            onChange={e => handleActionItemChange(idx, 'item_definition', e.target.value)}
            rows={2}
            placeholder="明确行动范围和完成口径"
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly || actionItems[idx]?.is_fixed}
          />,
          error
        )
      }
    },
    {
      title: '权重%',
      key: 'weight',
      width: 184,
      render: (_: any, __: any, idx: number) => {
        const error = getGoalFieldError(actionItems[idx], 'weight', 'action')
        return renderWeightControl('action', idx, actionItems[idx], error, targetSettingReadonly || actionItems[idx]?.is_fixed)
      }
    },
    {
      title: '定性目标',
      dataIndex: 'target_value',
      key: 'target_value',
      width: '30%',
      render: (_: any, __: any, idx: number) => {
        const error = getGoalFieldError(actionItems[idx], 'target_value', 'action')
        return renderFieldWithFeedback(
          <TextArea
            data-testid={`performance-goal-action-target-${idx}`}
            value={actionItems[idx]?.target_value}
            onChange={e => handleActionItemChange(idx, 'target_value', e.target.value)}
            rows={3}
            placeholder="描述关键结果、交付物或完成标准"
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly}
          />,
          error
        )
      }
    },
    {
      title: '',
      key: 'action',
      width: 64,
      align: 'center' as const,
      render: (_: any, __: any, idx: number) => renderDeleteAction(
        () => handleRemoveActionItem(idx),
        {
          disabled: targetSettingReadonly || actionItems[idx]?.is_fixed,
          disabledReason: actionItems[idx]?.is_fixed ? '固定项不可删除' : readonlyLabel,
          directReason: actionItems.length <= 1 ? '至少保留一个关键行动' : undefined,
        }
      )
    }
  ]

  const newFlowPlanRows = buildNewFlowPlanRows(quantItems, actionItems)

  const newFlowPlanColumns = [
    {
      title: '状态',
      key: 'index',
      width: 92,
      align: 'center' as const,
      render: (_: any, record: NewFlowPlanRow, idx: number) => {
        if (record.kind === 'fixed') return renderGoalItemStatus(record.item, 'action', idx)
        return renderGoalItemStatus(record.item, record.kind === 'kpi' ? 'quant' : 'action', idx)
      },
    },
    {
      title: '类型',
      key: 'kind',
      width: 86,
      render: (_: any, record: NewFlowPlanRow) => {
        if (record.kind === 'fixed') return <Tag color="default">固定</Tag>
        if (record.kind === 'kpi') return <Tag color="blue">KPI</Tag>
        return <Tag color="green">OKR</Tag>
      },
    },
    {
      title: '目标/关键职责事项',
      dataIndex: 'item_name',
      key: 'item_name',
      width: 220,
      render: (_: any, record: NewFlowPlanRow) => {
        if (record.kind === 'fixed') {
          return <Text strong>{record.item.item_name}</Text>
        }
        if (record.kind === 'kpi') {
          const idx = record.sourceIndex
          const error = getGoalFieldError(quantItems[idx], 'item_name', 'quant')
          return renderFieldWithFeedback(
            <AutoComplete
              data-testid={`performance-goal-quant-name-${idx}`}
              value={quantItems[idx]?.item_name}
              options={getSearchOptions(quantSearchResults[idx] || [])}
              onSearch={(val) => searchIndicators(val, setQuantSearchResults, idx, 'quantitative')}
              onChange={(val) => handleQuantItemChange(idx, 'item_name', val)}
              onSelect={(val) => handleIndicatorSelect(val, idx, quantSearchResults[idx] || [], setQuantItems, quantItems, true)}
              placeholder="输入目标或搜索指标"
              status={error ? 'error' : undefined}
              disabled={targetSettingReadonly}
              style={{ width: '100%' }}
            />,
            error
          )
        }
        const idx = record.sourceIndex
        const error = getGoalFieldError(actionItems[idx], 'item_name', 'action')
        return renderFieldWithFeedback(
          <AutoComplete
            data-testid={`performance-goal-action-name-${idx}`}
            value={actionItems[idx]?.item_name}
            options={getSearchOptions(actionSearchResults[idx] || [])}
            onSearch={(val) => searchIndicators(val, setActionSearchResults, idx, 'key_action')}
            onChange={(val) => handleActionItemChange(idx, 'item_name', val)}
            onSelect={(val) => handleIndicatorSelect(val, idx, actionSearchResults[idx] || [], setActionItems, actionItems, false)}
            placeholder="输入重点计划或搜索指标"
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly}
            style={{ width: '100%' }}
          />,
          error
        )
      },
    },
    {
      title: '权重',
      key: 'weight',
      width: 184,
      render: (_: any, record: NewFlowPlanRow) => {
        if (record.kind === 'fixed') {
          return <Text strong>{formatPercent(record.item.weight || 0)}</Text>
        }
        const idx = record.sourceIndex
        const isKpi = record.kind === 'kpi'
        const item = isKpi ? quantItems[idx] : actionItems[idx]
        const error = getGoalFieldError(item, 'weight', isKpi ? 'quant' : 'action')
        return renderWeightControl(isKpi ? 'quant' : 'action', idx, item, error, targetSettingReadonly)
      },
    },
    {
      title: '目标/关键职责事项说明',
      dataIndex: 'item_definition',
      key: 'item_definition',
      width: '28%',
      render: (_: any, record: NewFlowPlanRow) => {
        if (record.kind === 'fixed') {
          return <Text>{record.item.item_definition}</Text>
        }
        const idx = record.sourceIndex
        const isKpi = record.kind === 'kpi'
        const item = isKpi ? quantItems[idx] : actionItems[idx]
        const error = getGoalFieldError(item, 'item_definition', isKpi ? 'quant' : 'action')
        return renderFieldWithFeedback(
          <TextArea
            data-testid={isKpi ? `performance-goal-quant-definition-${idx}` : `performance-goal-action-definition-${idx}`}
            value={isKpi ? quantItems[idx]?.item_definition : actionItems[idx]?.item_definition}
            onChange={e => (isKpi ? handleQuantItemChange : handleActionItemChange)(idx, 'item_definition', e.target.value)}
            rows={2}
            placeholder={isKpi ? '填写指标定义、计算口径或范围' : '填写行动范围、完成口径或关键结果'}
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly}
          />,
          error
        )
      },
    },
    {
      title: '目标值/完成标准',
      dataIndex: 'target_value',
      key: 'target_value',
      width: '25%',
      render: (_: any, record: NewFlowPlanRow) => {
        if (record.kind === 'fixed') {
          return <Text type="secondary">按固定说明执行</Text>
        }
        const idx = record.sourceIndex
        if (record.kind === 'kpi') {
          const error = getGoalFieldError(quantItems[idx], 'target_value', 'quant')
          return renderFieldWithFeedback(
            <Input
              data-testid={`performance-goal-quant-target-${idx}`}
              value={quantItems[idx]?.target_value}
              onChange={e => handleQuantItemChange(idx, 'target_value', e.target.value)}
              placeholder="例如：Q3销售额达到100万"
              status={error ? 'error' : undefined}
              disabled={targetSettingReadonly}
            />,
            error
          )
        }
        const error = getGoalFieldError(actionItems[idx], 'target_value', 'action')
        return renderFieldWithFeedback(
          <TextArea
            data-testid={`performance-goal-action-target-${idx}`}
            value={actionItems[idx]?.target_value}
            onChange={e => handleActionItemChange(idx, 'target_value', e.target.value)}
            rows={2}
            placeholder="描述关键结果、交付物或完成标准"
            status={error ? 'error' : undefined}
            disabled={targetSettingReadonly}
          />,
          error
        )
      },
    },
    {
      title: '',
      key: 'action',
      width: 64,
      align: 'center' as const,
      render: (_: any, record: NewFlowPlanRow) => {
        if (record.kind === 'fixed') return null
        const idx = record.sourceIndex
        return renderDeleteAction(
          () => record.kind === 'kpi' ? handleRemoveQuantItem(idx) : handleRemoveActionItem(idx),
          {
            disabled: targetSettingReadonly,
            disabledReason: readonlyLabel,
            directReason: record.kind === 'kpi'
              ? (quantItems.length <= 1 ? '至少保留一个量化指标' : undefined)
              : (actionItems.filter(item => !item.is_fixed).length <= 1 ? '至少保留一个关键行动' : undefined),
          }
        )
      },
    },
  ]

  const reviewSupplementWeightTotal = reviewSupplementRows.reduce((sum, row) => sum + (row.item.weight || 0), 0)
  const showReviewSupplement = isNewFlow && !isReviewSupplementMode && reviewSupplementRows.length > 0
  const reviewSupplementColumns = [
    {
      title: '状态',
      key: 'status',
      width: 92,
      align: 'center' as const,
      render: (_: any, record: NewFlowPlanRow, idx: number) => (
        renderGoalItemStatus(record.item, record.kind === 'kpi' ? 'quant' : 'action', idx)
      ),
    },
    {
      title: '类型',
      key: 'kind',
      width: 86,
      render: (_: any, record: NewFlowPlanRow) => {
        if (record.kind === 'fixed') return <Tag color="default">固定</Tag>
        if (record.kind === 'kpi') return <Tag color="blue">KPI</Tag>
        return <Tag color="green">OKR</Tag>
      },
    },
    {
      title: '目标/关键职责事项',
      dataIndex: 'item_name',
      key: 'item_name',
      width: 220,
      render: (_: any, record: NewFlowPlanRow) => (
        <Text strong={record.kind === 'fixed'}>{record.item.item_name || '-'}</Text>
      ),
    },
    {
      title: '权重',
      key: 'weight',
      width: 120,
      render: (_: any, record: NewFlowPlanRow) => <Text strong>{formatPercent(record.item.weight || 0)}</Text>,
    },
    {
      title: '目标/关键职责事项说明',
      dataIndex: 'item_definition',
      key: 'item_definition',
      width: '30%',
      render: (_: any, record: NewFlowPlanRow) => (
        <Text style={{ whiteSpace: 'pre-wrap' }}>{record.item.item_definition || '-'}</Text>
      ),
    },
    {
      title: '目标值/完成标准',
      dataIndex: 'target_value',
      key: 'target_value',
      width: '25%',
      render: (_: any, record: NewFlowPlanRow) => (
        <Text type={record.kind === 'fixed' ? 'secondary' : undefined} style={{ whiteSpace: 'pre-wrap' }}>
          {record.kind === 'fixed' ? '按固定说明执行' : (record.item.target_value || '-')}
        </Text>
      ),
    },
  ]

  const pageTitle = isReviewSupplementMode ? '上一季度考核指标补录' : (isNewFlow ? '下季度目标计划' : '目标设定')
  const readonlyLabel = isReviewSupplementMode ? '当前阶段不可补录' : '目标已审批通过，不可修改'
  const saveButtonLabel = isReviewSupplementMode ? '保存补录' : '保存草稿'
  const submitButtonLabel = isReviewSupplementMode ? '完成补录' : '提交目标'
  const rejectionComment = String(latestRejection?.comment || '').trim()
  const rejectionTime = latestRejection?.created_at ? formatDateTime(latestRejection.created_at) : ''
  const showRejectionReason = participant?.status === 'target_rejected' && Boolean(rejectionComment)
  const participantStatusMeta: Record<string, { label: string; color: string }> = {
    pending: { label: '草稿', color: 'default' },
    target_pending_approval: { label: '待审批', color: 'processing' },
    target_rejected: { label: '已驳回', color: 'error' },
    target_set: { label: '已通过', color: 'success' },
    self_submitted: { label: '自评已提交', color: 'success' },
    manager_submitted: { label: '主管已评分', color: 'success' },
    result_confirmed: { label: '结果已确认', color: 'success' },
    employee_confirmed: { label: '员工已确认', color: 'success' },
    manager_recheck: { label: '待领导复核', color: 'warning' },
    manager_confirmed: { label: '主管已确认', color: 'success' },
    hr_confirmed: { label: 'HR已确认', color: 'success' },
    locked: { label: '已锁定', color: 'success' },
  }
  const defaultStatusMeta = participant?.status
    ? { label: participant.status, color: 'default' }
    : { label: '未开始', color: 'default' }
  const statusMeta = targetSettingReadonly
    ? { label: readonlyLabel, color: isReviewSupplementMode ? 'warning' : 'success' }
    : (participantStatusMeta[participant?.status || ''] || defaultStatusMeta)
  const weightBalanced = Math.abs(totalWeight - 1) < 0.001
  const weightOverLimit = totalWeight > 1
  const totalWeightPercent = Math.round(totalWeight * 100)
  const weightProgressPercent = Math.max(0, Math.min(totalWeightPercent, 100))
  const weightProgressColor = weightBalanced
    ? 'var(--color-success)'
    : (weightOverLimit ? 'var(--color-error)' : 'var(--color-warning)')
  const weightHint = weightBalanced
    ? '已配平'
    : (weightOverLimit ? `超出 ${formatPercent(totalWeight - 1)}` : `还差 ${formatPercent(1 - totalWeight)}`)
  const submitDisabled = targetSettingReadonly || !weightBalanced
  const submitDisabledReason = targetSettingReadonly
    ? readonlyLabel
    : (!weightBalanced ? `权重合计需为 100%，当前 ${formatPercent(totalWeight)}，${weightHint}` : '')

  if (loading) return <div style={{ textAlign: 'center', padding: 100 }}><Spin size="large" /></div>

  return (
    <PageContainer data-testid="performance-goal-setting-page" noPadding>
      <div style={{
        position: 'sticky', top: 0, zIndex: 10,
        background: 'var(--color-bg-card)', borderBottom: '1px solid var(--color-border-light)',
        padding: '12px var(--page-padding)',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap' }}>
          <Space size={12} align="center" wrap>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(-1)}>返回</Button>
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, flexWrap: 'wrap' }}>
              <Title level={4} style={{ margin: 0 }}>{pageTitle}</Title>
              {participant && (
                <Text type="secondary">
                  {participant.employee_name || participant.employee_id}
                </Text>
              )}
            </div>
            <StatusTag color={statusMeta.color}>{statusMeta.label}</StatusTag>
          </Space>
          <Space size={16} align="center" wrap style={{ justifyContent: 'flex-end', flex: '1 1 420px' }}>
            <div style={{ minWidth: 280, maxWidth: 360, flex: '1 1 280px' }}>
              <Space size={8} align="baseline" style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text style={{ fontSize: 'var(--font-size-sm)' }}>权重合计</Text>
                <Space size={6} align="baseline">
                  <Text strong style={{ color: weightProgressColor, fontSize: 'var(--font-size-md)' }}>{formatPercent(totalWeight)}</Text>
                  <Text style={{ color: weightProgressColor, fontSize: 'var(--font-size-xs)' }}>{weightHint}</Text>
                </Space>
              </Space>
              <Progress
                percent={weightProgressPercent}
                showInfo={false}
                size="small"
                strokeColor={weightProgressColor}
                status={weightOverLimit ? 'exception' : undefined}
              />
              <Text type="secondary" style={{ display: 'block', fontSize: 'var(--font-size-xs)' }}>
                {isNewFlow ? (
                  <>
                    固定 {formatPercent(fixedWeightTotal)} / 可变 {formatPercent(variableWeightTotal)}
                  </>
                ) : (
                  <>
                    量化 {formatPercent(quantWeightTotal)} / 关键 {formatPercent(actionWeightTotal)}
                  </>
                )}
              </Text>
            </div>
            <Space>
            <Button
              data-testid="performance-goal-save-draft"
              icon={<SaveOutlined />}
              loading={saving}
              onClick={handleSaveDraft}
              disabled={targetSettingReadonly}
            >
              {saveButtonLabel}
            </Button>
            <Tooltip title={submitDisabledReason}>
              <span>
                <Button
                  data-testid="performance-goal-submit"
                  type="primary"
                  icon={<CheckCircleOutlined />}
                  loading={submitting}
                  onClick={handleSubmit}
                  disabled={submitDisabled}
                >
                  {submitButtonLabel}
                </Button>
              </span>
            </Tooltip>
            </Space>
          </Space>
        </div>
      </div>

      <div style={{ padding: 'var(--page-padding)' }}>
      {showRejectionReason && (
          <Alert
            data-testid="performance-goal-rejection-reason"
            type="error"
            showIcon
            message="目标已被驳回，请根据驳回理由修改后重新提交"
            style={{ marginBottom: 16 }}
            description={
              <Space direction="vertical" size={4}>
                <Text>{rejectionComment}</Text>
                {(latestRejection?.approver_name || latestRejection?.created_at) && (
                  <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>
                    {latestRejection?.approver_name ? `驳回人：${latestRejection.approver_name}` : ''}
                    {latestRejection?.approver_name && latestRejection?.created_at ? ' / ' : ''}
                    {rejectionTime ? `驳回时间：${rejectionTime}` : ''}
                  </Text>
                )}
              </Space>
            }
          />
      )}

      {isNewFlow ? (
        <>
        {showReviewSupplement && (
          <PageCard
            title={
              <Space size={8} wrap>
                <span>上一季度考核指标补录</span>
                <Tag style={{ marginInlineEnd: 0 }}>{reviewSupplementRows.length} 项</Tag>
                <Tag
                  color={Math.abs(reviewSupplementWeightTotal - 1) < 0.001 ? 'success' : 'warning'}
                  style={{ marginInlineEnd: 0 }}
                >
                  {formatPercent(reviewSupplementWeightTotal)}
                </Tag>
                <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>
                  已补录，仅展示，作为本期自评和评分依据
                </Text>
              </Space>
            }
            style={{ marginBottom: 24 }}
            styles={{ body: { paddingTop: 16 } }}
          >
            <Table
              className="performance-goal-plan-table"
              dataSource={reviewSupplementRows}
              columns={reviewSupplementColumns}
              rowKey={record => record.key}
              rowClassName={record => record.kind === 'fixed' ? 'performance-goal-fixed-row' : ''}
              pagination={false}
              size="middle"
              bordered
            />
          </PageCard>
        )}
        <PageCard
          title={pageTitle}
          styles={{ body: { paddingTop: 16 } }}
        >
          <div style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(4, minmax(120px, 1fr))',
            gap: 12,
            marginBottom: 16,
          }}>
            {[
              { label: '固定两项', value: formatPercent(fixedWeightTotal), color: 'var(--color-text-primary)' },
              { label: '已填可变', value: formatPercent(variableWeightTotal), color: variableWeightTotal > 0 ? 'var(--color-primary)' : 'var(--color-text-secondary)' },
              { label: '剩余权重', value: formatPercent(remainingWeight), color: remainingWeight === 0 ? 'var(--color-success)' : 'var(--color-warning)' },
              { label: '合计', value: formatPercent(totalWeight), color: Math.abs(totalWeight - 1) < 0.001 ? 'var(--color-success)' : 'var(--color-error)' },
            ].map(item => (
              <div
                key={item.label}
                style={{
                  border: '1px solid var(--color-border)',
                  borderRadius: 8,
                  padding: '10px 12px',
                  background: 'var(--color-bg-container)',
                }}
              >
                <Text type="secondary" style={{ display: 'block', fontSize: 'var(--font-size-xs)' }}>{item.label}</Text>
                <Text strong style={{ color: item.color, fontSize: 'var(--font-size-lg)' }}>{item.value}</Text>
              </div>
            ))}
          </div>
          <Table
            className="performance-goal-plan-table"
            dataSource={newFlowPlanRows}
            columns={newFlowPlanColumns}
            rowKey={record => record.key}
            rowClassName={record => record.kind === 'fixed' ? 'performance-goal-fixed-row' : ''}
            pagination={false}
            size="middle"
            bordered
          />
          <div style={{
            display: 'grid',
            gridTemplateColumns: '1fr 1fr',
            gap: 12,
            marginTop: 12,
          }}>
            <Button
              data-testid="performance-goal-add-quant"
              type="dashed"
              icon={<PlusOutlined />}
              onClick={handleAddQuantItem}
              disabled={targetSettingReadonly}
            >
              添加 KPI 指标
            </Button>
            <Button
              data-testid="performance-goal-add-action"
              type="dashed"
              icon={<PlusOutlined />}
              onClick={handleAddActionItem}
              disabled={targetSettingReadonly}
            >
              添加 OKR / 重点计划
            </Button>
          </div>
        </PageCard>
        </>
      ) : (
          <>
            <PageCard title={renderGoalSectionTitle('量化指标', quantItems.length, quantWeightTotal, '建议约 70%，允许 65%-75%')}>
              <Table
                className="performance-goal-plan-table"
                dataSource={quantItems}
                columns={quantColumns}
                rowKey={getGoalRowKey}
                pagination={false}
                size="small"
                bordered
                scroll={{ x: 1328 }}
              />
            <Button
              data-testid="performance-goal-add-quant"
              type="dashed"
              icon={<PlusOutlined />}
              onClick={handleAddQuantItem}
              disabled={targetSettingReadonly}
              style={{ marginTop: 12, width: '100%' }}
            >
              添加量化指标
            </Button>
          </PageCard>

          <PageCard
            title={renderGoalSectionTitle('关键行动', actionItems.length, actionWeightTotal, '建议约 30%，允许 25%-35%')}
            style={{ marginTop: 24 }}
          >
            <Table
              className="performance-goal-plan-table"
              dataSource={actionItems}
              columns={actionColumns}
              rowKey={record => record.id ?? record.draft_key}
              pagination={false}
              size="small"
              bordered
            />
            <Button
              data-testid="performance-goal-add-action"
              type="dashed"
              icon={<PlusOutlined />}
              onClick={handleAddActionItem}
              disabled={targetSettingReadonly}
              style={{ marginTop: 12, width: '100%' }}
            >
              添加关键行动
            </Button>
          </PageCard>
        </>
      )}

      <PageCard title={
        <Space>
          <BulbOutlined />
          <span>指标库建议</span>
        </Space>
      } style={{ marginTop: 24 }}>
        <Button data-testid="performance-goal-load-suggestions" type="primary" icon={<BulbOutlined />} onClick={loadSuggestions} disabled={targetSettingReadonly} style={{ marginBottom: showSuggestions ? 12 : 0 }}>
          从指标库获取建议
        </Button>
        {showSuggestions && suggestions.length > 0 && (
          <div>
            <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>
              点击应用将添加到对应区域
            </Text>
            <Space wrap>
              {suggestions.map((s, idx) => (
                <Tag
                  key={idx}
                  color={s.section_type === 'key_action' ? 'green' : 'blue'}
                  style={{ cursor: targetSettingReadonly ? 'default' : 'pointer', padding: '4px 8px' }}
                  onClick={targetSettingReadonly ? undefined : () => applySuggestion(s)}
                >
                  {s.name || s.item_name}
                </Tag>
              ))}
            </Space>
          </div>
        )}
        {showSuggestions && suggestions.length === 0 && (
          <Text type="secondary" style={{ display: 'block', marginTop: 8 }}>暂无建议</Text>
        )}
      </PageCard>
      </div>
    </PageContainer>
  )
}

export default PerformanceGoalSetting
