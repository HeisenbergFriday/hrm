import React, { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  Typography, Form, Input, InputNumber, Button, Space,
  message, Spin, Row, Col, Table, Select, Progress, Modal, Badge, Image, Alert, Tooltip
} from 'antd'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import AuthorizedFileFrame from '../components/AuthorizedFileFrame'
import AuthorizedImage from '../components/AuthorizedImage'
import { ArrowLeftOutlined, CheckCircleOutlined, PaperClipOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { performanceAPI, PerformanceActivity, PerformanceGoalRecord, PerformanceParticipant, TeamQuotaStatus } from '../services/api'
import { downloadAuthorizedFile } from '../utils/authFileUrl'

const { Text } = Typography
const { TextArea } = Input

function isReviewGoalRecord(activity: PerformanceActivity | null, record: PerformanceGoalRecord) {
  if (activity?.flow_type !== 'new') return true
  return String(record.goal_phase || 'review').trim() !== 'plan'
}

const LEVEL_OPTIONS = [
  { value: 'S', label: 'S - 杰出', color: '#f50' },
  { value: 'A', label: 'A - 优秀', color: '#2db7f5' },
  { value: 'B', label: 'B - 良好', color: '#87d068' },
  { value: 'C', label: 'C - 待改进', color: '#faad14' },
  { value: 'D', label: 'D - 不合格', color: '#ff4d4f' },
]

type LevelRule = {
  level: string
  min_score?: number
  max_score?: number
  min_inclusive?: boolean
  max_inclusive?: boolean
  sort_order?: number
}

const defaultLevelRules = (flowType?: string): LevelRule[] => {
  if (flowType === 'new') {
    return [
      { level: 'S', min_score: 10, min_inclusive: false, sort_order: 1 },
      { level: 'A', min_score: 9, max_score: 10, min_inclusive: true, max_inclusive: true, sort_order: 2 },
      { level: 'B', min_score: 7.5, max_score: 9, min_inclusive: true, max_inclusive: false, sort_order: 3 },
      { level: 'C', min_score: 6, max_score: 7.5, min_inclusive: true, max_inclusive: false, sort_order: 4 },
      { level: 'D', max_score: 6, max_inclusive: false, sort_order: 5 },
    ]
  }
  return [
    { level: 'S', min_score: 100, min_inclusive: true, sort_order: 1 },
    { level: 'A', min_score: 90, max_score: 100, min_inclusive: true, max_inclusive: false, sort_order: 2 },
    { level: 'B', min_score: 80, max_score: 90, min_inclusive: true, max_inclusive: false, sort_order: 3 },
    { level: 'C', min_score: 60, max_score: 80, min_inclusive: true, max_inclusive: false, sort_order: 4 },
    { level: 'D', max_score: 60, max_inclusive: false, sort_order: 5 },
  ]
}

const scoreMatchesRule = (score: number, rule: LevelRule) => {
  if (typeof rule.min_score === 'number') {
    if (rule.min_inclusive === false ? score <= rule.min_score : score < rule.min_score) return false
  }
  if (typeof rule.max_score === 'number') {
    if (rule.max_inclusive ? score > rule.max_score : score >= rule.max_score) return false
  }
  return true
}

const calcPerformanceLevel = (score: number, activity?: PerformanceActivity | null) => {
  const configuredRules = (activity?.level_rule_config?.rules as LevelRule[] | undefined) || []
  const rules = (configuredRules.length ? configuredRules : defaultLevelRules(activity?.flow_type))
    .slice()
    .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0))
  const matched = rules.find(rule => scoreMatchesRule(score, rule))
  if (matched?.level) return matched.level
  return 'D'
}

const normalizeAutoManagerScore = (score: number, isNewFlow: boolean) => {
  if (!Number.isFinite(score)) return 0
  const scaledScore = isNewFlow ? score / 10 : score
  const maxScore = isNewFlow ? 10 : 120
  const clampedScore = Math.max(0, Math.min(maxScore, scaledScore))
  return isNewFlow
    ? Math.round(clampedScore * 10) / 10
    : Math.round(clampedScore * 100) / 100
}

const attachmentExt = (url: string) => {
  const cleanUrl = url.split('?')[0].split('#')[0]
  const filename = cleanUrl.split('/').pop() || ''
  const dotIndex = filename.lastIndexOf('.')
  return dotIndex >= 0 ? filename.slice(dotIndex).toLowerCase() : ''
}

const attachmentName = (url: string, index: number) => {
  const cleanUrl = url.split('?')[0].split('#')[0]
  return decodeURIComponent(cleanUrl.split('/').pop() || `附件 ${index + 1}`)
}

const isImageAttachment = (url: string) => ['.jpg', '.jpeg', '.png', '.gif', '.webp'].includes(attachmentExt(url))
const isFramePreviewAttachment = (url: string) => ['.pdf', '.txt', '.csv', '.md'].includes(attachmentExt(url))

const PerformanceManagerEval: React.FC = () => {
  const { activityId, participantId } = useParams<{ activityId: string; participantId: string }>()
  const navigate = useNavigate()
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [totalManagerScore, setTotalManagerScore] = useState(0)
  const [quotaData, setQuotaData] = useState<TeamQuotaStatus[]>([])
  const [participant, setParticipant] = useState<PerformanceParticipant | null>(null)
  const [activity, setActivity] = useState<PerformanceActivity | null>(null)
  const [bonusItems, setBonusItems] = useState<PerformanceGoalRecord[]>([])
  const [previewAttachments, setPreviewAttachments] = useState<{ visible: boolean; attachments: string[]; currentIndex: number }>({ visible: false, attachments: [], currentIndex: 0 })
  const [autoScoring, setAutoScoring] = useState(false)
  const isNewFlow = activity?.flow_type === 'new'
  const scoreMax = isNewFlow ? 10 : 120
  const scoreStep = isNewFlow ? 0.1 : 1
  const isManagerRecheck = participant?.status === 'manager_recheck'
  const selectedLevel = Form.useWatch('suggested_level', form) as string | undefined

  const loadData = useCallback(async () => {
    if (!participantId || !activityId) return
    setLoading(true)
    try {
      const [recordsRes, quotaRes, participantRes] = await Promise.all([
        performanceAPI.getGoalRecords(Number(participantId)),
        performanceAPI.getRealtimeDistributionCheck(Number(activityId)),
        performanceAPI.getParticipant(Number(participantId))
      ])

      const currentParticipant = participantRes.data?.participant || participantRes.data
      const currentActivity = participantRes.data?.activity || null
      const allItems: PerformanceGoalRecord[] = recordsRes.data?.items || []
      const items: PerformanceGoalRecord[] = allItems.filter(
        (item: PerformanceGoalRecord) => item.section_type !== 'bonus_penalty' && isReviewGoalRecord(currentActivity, item)
      )
      const bonus: PerformanceGoalRecord[] = allItems.filter(
        (item: PerformanceGoalRecord) => item.section_type === 'bonus_penalty'
      )
      setQuotaData(quotaRes.data?.teams || [])
      setParticipant(currentParticipant)
      setActivity(currentActivity)
      setBonusItems(bonus)

      const formItems = items.map(i => ({
        record_id: i.id,
        item_name: i.item_name,
        section_type: i.section_type,
        weight: i.weight,
        weight_percent: (i.weight * 100).toFixed(0),
        actual_result: i.actual_result,
        self_score: i.self_score,
        manager_score: i.manager_score || 0,
        red_line_value: i.red_line_value,
        target_value: i.target_value,
        challenge_value: i.challenge_value,
        scoring_rule: i.scoring_rule,
        attachments: i.attachments || []
      }))
      form.setFieldsValue({
        items: formItems,
        suggested_level: currentParticipant?.suggested_level || currentParticipant?.final_level || undefined,
        evaluation_good: currentParticipant?.manager_evaluation_good || '',
        evaluation_improvement: currentParticipant?.manager_evaluation_improvement || '',
      })
      if (currentParticipant?.suggested_level || currentParticipant?.final_level) {
        levelManuallySetRef.current = true
      }
      calcTotal(formItems, currentActivity)
    } catch {
      message.error('加载数据失败')
    } finally {
      setLoading(false)
    }
  }, [participantId, activityId, form])

  useEffect(() => { loadData() }, [loadData])

  const calcTotal = (items: any[], currentActivity: PerformanceActivity | null = activity) => {
    const total = items.reduce((sum, i) => sum + (i.manager_score || 0) * (i.weight || 0), 0)
    const roundedTotal = Math.round(total * 100) / 100
    setTotalManagerScore(roundedTotal)
    if (!levelManuallySetRef.current) {
      const level = calcPerformanceLevel(roundedTotal, currentActivity)
      form.setFieldsValue({ suggested_level: level })
    }
  }

  const handleAutoScore = async () => {
    const allItems = form.getFieldValue('items') || []
    const quantitativeItems = allItems.filter((i: any) => i.section_type === 'quantitative')
    if (quantitativeItems.length === 0) {
      message.info('没有可自动评分的量化指标')
      return
    }
    setAutoScoring(true)
    try {
      const res = await performanceAPI.autoScoreGoalRecords(
        quantitativeItems.map((i: any) => ({
          record_id: i.record_id,
          section_type: i.section_type,
          weight: i.weight,
          red_line_value: i.red_line_value || '',
          target_value: i.target_value || '',
          challenge_value: i.challenge_value || '',
          scoring_rule: i.scoring_rule || '',
          actual_result: i.actual_result || '',
        }))
      )
      // axios 拦截器返回 response.data = {code, message, data}
      const scoredItems = ((res as any)?.data?.items || []) as { record_id: number; score: number; breakdown: string; auto_scored: boolean }[]
      const scoreMap = new Map<number, { score: number; breakdown: string; auto_scored: boolean }>()
      for (const item of scoredItems) {
        scoreMap.set(item.record_id, item)
      }
      const updatedItems = allItems.map((i: any) => {
        const result = scoreMap.get(i.record_id)
        if (result && result.auto_scored) {
          return { ...i, manager_score: normalizeAutoManagerScore(result.score, isNewFlow) }
        }
        return i
      })
      form.setFieldsValue({ items: updatedItems })
      calcTotal(updatedItems)
      const autoCount = scoredItems.filter(i => i.auto_scored).length
      const skipCount = scoredItems.filter(i => !i.auto_scored).length
      let msg = `已自动评分 ${autoCount} 项`
      if (skipCount > 0) msg += `，${skipCount} 项需手动评分`
      message.success(msg)
    } catch {
      message.error('自动评分失败')
    } finally {
      setAutoScoring(false)
    }
  }

  const currentTeamQuota = () => {
    const managerId = participant?.manager_id || ''
    return quotaData.find(team => team.manager_id === managerId) || null
  }

  const levelQuotaKey = (level?: string) => {
    if (level === 'C' || level === 'D') return 'CD'
    return level || ''
  }

  const currentParticipantQuotaKey = () => levelQuotaKey(participant?.final_level || participant?.suggested_level)

  const getQuotaForLevel = (level: string) => {
    const team = currentTeamQuota()
    if (!team) return null
    const key = levelQuotaKey(level)
    const quota = team.levels[key]
    if (!quota) return null

    const currentLevelKey = currentParticipantQuotaKey()
    return {
      ...quota,
      current: currentLevelKey === key ? Math.max(0, quota.current - 1) : quota.current
    }
  }

  const previewQuotaForLevel = (level: string) => {
    const team = currentTeamQuota()
    if (!team) return null
    const key = levelQuotaKey(level)
    const quota = team.levels[key]
    if (!quota) return null

    const currentLevelKey = currentParticipantQuotaKey()
    const selectedLevelKey = levelQuotaKey(selectedLevel)
    let current = quota.current
    if (currentLevelKey === key) current = Math.max(0, current - 1)
    if (selectedLevelKey === key) current += 1
    return { ...quota, current }
  }

  const prevLevelRef = React.useRef<string | undefined>(undefined)
  const levelManuallySetRef = React.useRef(false)

  const handleValuesChange = (_changed: any, _allValues: any) => {
    if (_changed.items !== undefined) {
      const items = form.getFieldValue('items') || []
      calcTotal(items)
    }
    if (_changed.suggested_level !== undefined) {
      levelManuallySetRef.current = true
    }
  }

  React.useEffect(() => {
    const level = selectedLevel
    if (level && prevLevelRef.current !== undefined && prevLevelRef.current !== level) {
      const quota = getQuotaForLevel(level)
      if (quota && quota.current >= quota.max) {
        Modal.warning({
          title: '配额超限警告',
          content: `当前团队 ${level} 等级配额已用完（${quota.current}/${quota.max}），请调整评分或确认配额。`
        })
      }
    }
    prevLevelRef.current = level
  }, [selectedLevel, totalManagerScore])

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()

      const level = values.suggested_level
      const quota = getQuotaForLevel(level)
      if (quota && quota.current >= quota.max) {
        Modal.error({
          title: '无法提交',
          content: `${level} 等级配额已用完（${quota.current}/${quota.max}），请调整等级后再提交。`
        })
        return
      }

      const items = values.items.map((i: any) => ({
        record_id: i.record_id,
        manager_score: i.manager_score
      }))

      const bonusItemsPayload = bonusItems.map(item => ({
        record_id: item.id,
        manager_score: item.manager_score || 0
      }))

      setSaving(true)
      await performanceAPI.submitGoalManagerEvaluation(Number(participantId), {
        items,
        bonus_items: bonusItemsPayload,
        suggested_level: values.suggested_level,
        evaluation_good: values.evaluation_good || '',
        evaluation_improvement: values.evaluation_improvement || ''
      })
      message.success(isManagerRecheck ? '复核意见已提交' : '评分提交成功')
      navigate(-1)
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '提交失败')
    } finally {
      setSaving(false)
    }
  }

  const handleConfirmRecheck = async () => {
    if (!participantId) return
    setSaving(true)
    try {
      await performanceAPI.confirmManagerResult(Number(participantId))
      message.success('已确认查看')
      navigate(-1)
    } catch (err: any) {
      message.error(err?.response?.data?.message || '确认失败')
    } finally {
      setSaving(false)
    }
  }

  const openAttachmentPreview = (attachments: string[]) => {
    setPreviewAttachments({ visible: true, attachments, currentIndex: 0 })
  }

  const closeAttachmentPreview = () => {
    setPreviewAttachments({ visible: false, attachments: [], currentIndex: 0 })
  }

  const renderAttachmentPreview = () => {
    const currentUrl = previewAttachments.attachments[previewAttachments.currentIndex]
    if (!currentUrl) {
      return <Text type="secondary">暂无附件</Text>
    }
    const fileName = attachmentName(currentUrl, previewAttachments.currentIndex)
    if (isImageAttachment(currentUrl)) {
      return (
        <AuthorizedImage
          data-testid="performance-manager-attachment-preview-image"
          src={currentUrl}
          alt={fileName}
          wrapperStyle={{ width: '100%', display: 'flex', justifyContent: 'center' }}
          style={{ maxWidth: '100%', maxHeight: 560, objectFit: 'contain' }}
        />
      )
    }
    if (isFramePreviewAttachment(currentUrl)) {
      return (
        <AuthorizedFileFrame
          data-testid="performance-manager-attachment-preview-frame"
          title={fileName}
          src={currentUrl}
          style={{
            width: '100%',
            height: 560,
            border: '1px solid var(--color-border-light)',
            borderRadius: 'var(--radius-md)',
            background: 'var(--color-bg-card)',
          }}
        />
      )
    }
    return (
      <div style={{
        minHeight: 240,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 12,
        border: '1px dashed var(--color-border)',
        borderRadius: 'var(--radius-md)',
        background: 'var(--color-bg-page)',
      }}>
        <PaperClipOutlined style={{ fontSize: 28, color: 'var(--color-text-secondary)' }} />
        <Text
          strong
          title={fileName}
          style={{ maxWidth: '100%', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
        >
          {fileName}
        </Text>
        <Text type="secondary">该文件类型暂不支持在线预览，请下载后查看。</Text>
        <Button
          onClick={() => {
            void downloadAuthorizedFile(currentUrl, fileName).catch(() => message.error('附件下载失败'))
          }}
        >
          下载附件
        </Button>
      </div>
    )
  }

  const renderQuotaPanel = () => {
    const team = currentTeamQuota()
    if (!team) return null
    return (
      <PageCard
        title="配额进度"
        size="small"
        styles={{ body: { padding: 12 } }}
      >
        <Text type="secondary" style={{ display: 'block', marginBottom: 6 }}>
          考核上级团队：{team.manager_name || '未分组'}（共 {team.total} 人）
        </Text>
        {['S', 'A', 'B', 'CD'].map(level => {
          const q = previewQuotaForLevel(level)
          if (!q) return null
          const percent = q.max > 0 ? Math.round((q.current / q.max) * 100) : 0
          const isFull = q.current >= q.max
          return (
            <div key={level} data-testid={`performance-quota-${level}`} style={{ marginBottom: 6 }}>
              <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                <StatusTag color={isFull ? 'red' : 'blue'}>{level}</StatusTag>
                <Text>{q.current} / {q.max}（{q.percent}%）</Text>
              </Space>
              <Progress percent={percent} size="small" showInfo={false} status={isFull ? 'exception' : 'active'}
                strokeColor={isFull ? '#ff4d4f' : undefined} />
            </div>
          )
        })}
      </PageCard>
    )
  }

  const renderClampedText = (value?: string, lines = 2) => {
    const text = String(value || '').trim()
    if (!text) return <Text type="secondary">-</Text>
    return (
      <Tooltip title={text}>
        <Text
          style={{
            display: '-webkit-box',
            WebkitLineClamp: lines,
            WebkitBoxOrient: 'vertical',
            overflow: 'hidden',
            whiteSpace: 'normal',
            fontSize: 'var(--font-size-xs)',
            lineHeight: '20px',
          }}
        >
          {text}
        </Text>
      </Tooltip>
    )
  }

  const renderRuleText = (label: string, value: string | undefined, color?: string) => {
    if (!value) return null
    return (
      <Tooltip title={`${label}: ${value}`}>
        <Text
          style={{
            display: 'block',
            color,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            fontSize: 'var(--font-size-xs)',
            lineHeight: '20px',
          }}
        >
          {label}: {value}
        </Text>
      </Tooltip>
    )
  }

  const columns = [
    {
      title: '指标名称',
      dataIndex: 'item_name',
      key: 'item_name',
      width: 220,
      render: (val: string, _: any, idx: number) => {
        const items = form.getFieldValue('items') || []
        const item = items[idx]
        const isQuant = item?.section_type === 'quantitative'
        return (
          <>
            <Form.Item name={['items', idx, 'record_id']} hidden><Input /></Form.Item>
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              <StatusTag color={isQuant ? 'blue' : 'green'} style={{ marginBottom: 4 }}>
                {isQuant ? '量化指标' : '关键行动'}
              </StatusTag>
              <Tooltip title={val}>
                <Text
                  strong
                  style={{
                    display: '-webkit-box',
                    WebkitLineClamp: 2,
                    WebkitBoxOrient: 'vertical',
                    overflow: 'hidden',
                    whiteSpace: 'normal',
                  }}
                >
                  {val}
                </Text>
              </Tooltip>
            </Space>
          </>
        )
      }
    },
    {
      title: '目标/评分规则',
      key: 'target_rule',
      width: 260,
      render: (_: any, __: any, idx: number) => {
        const items = form.getFieldValue('items') || []
        const item = items[idx]
        if (item?.section_type === 'quantitative') {
          return (
            <div>
              {renderRuleText('红线', item.red_line_value, 'var(--color-error)')}
              {renderRuleText('目标', item.target_value, 'var(--color-info)')}
              {renderRuleText('挑战', item.challenge_value, 'var(--color-success)')}
              {item.scoring_rule && renderClampedText(`考核：${item.scoring_rule}`, 2)}
              {!item.red_line_value && !item.target_value && !item.challenge_value && !item.scoring_rule && <Text type="secondary">-</Text>}
            </div>
          )
        }
        return renderClampedText(item?.target_value || item?.scoring_rule, 2)
      }
    },
    {
      title: '权重',
      dataIndex: 'weight_percent',
      key: 'weight',
      width: 70
    },
    {
      title: '实际达成',
      dataIndex: 'actual_result',
      key: 'actual_result',
      width: 220,
      render: (val: string) => renderClampedText(val, 2)
    },
    {
      title: '附件',
      key: 'attachments',
      width: 86,
      render: (_: any, __: any, idx: number) => {
        const items = form.getFieldValue('items') || []
        const item = items[idx]
        const attachments = item?.attachments || []
        if (attachments.length === 0) return <Text type="secondary">-</Text>
        return (
          <Badge count={attachments.length} size="small">
            <Button
              type="link"
              size="small"
              icon={<PaperClipOutlined />}
              onClick={() => openAttachmentPreview(attachments)}
            >
              查看
            </Button>
          </Badge>
        )
      }
    },
    {
      title: '自评得分',
      dataIndex: 'self_score',
      key: 'self_score',
      width: 88,
      render: (val: number) => <Text>{val}</Text>
    },
    {
      title: '上级评分',
      key: 'manager_score',
      width: 120,
      render: (_: any, __: any, idx: number) => (
        <Form.Item name={['items', idx, 'manager_score']} style={{ margin: 0 }}
          rules={[{ required: true, message: '请评分' }]}>
          <InputNumber data-testid={`performance-manager-score-${idx}`} min={0} max={scoreMax} step={scoreStep} style={{ width: '100%' }} />
        </Form.Item>
      )
    }
  ]

  if (loading) {
    return (
      <>
        <Form form={form} component={false} />
        <div style={{ textAlign: 'center', padding: 100 }}><Spin size="large" /></div>
      </>
    )
  }

  const currentPreviewUrl = previewAttachments.attachments[previewAttachments.currentIndex] || ''
  const currentPreviewName = attachmentName(currentPreviewUrl, previewAttachments.currentIndex)
  const quotaPanel = renderQuotaPanel()

  return (
    <PageContainer
      data-testid="performance-manager-eval-page"
      title="上级绩效评分"
      extra={<Button icon={<ArrowLeftOutlined />} onClick={() => navigate(-1)}>返回</Button>}
    >
      {isManagerRecheck && (
        <Alert
          data-testid="performance-manager-recheck-notice"
          type="warning"
          showIcon
          message="员工已修改自评，待领导复核"
          description="可直接确认查看，也可以调整上级评价后提交复核意见。"
          action={
            <Button size="small" type="primary" loading={saving} onClick={handleConfirmRecheck}>
              确认查看
            </Button>
          }
          style={{ marginBottom: 16 }}
        />
      )}

      <Form form={form} onValuesChange={handleValuesChange} layout="vertical">
        <Row gutter={[16, 16]} align="stretch" style={{ marginBottom: 16 }}>
          <Col xs={24} lg={quotaPanel ? 14 : 24} xl={quotaPanel ? 16 : 24}>
            <PageCard title="评分概览" size="small" styles={{ body: { padding: 16 } }}>
              <Row gutter={[16, 12]} align="middle">
                <Col xs={24} sm={8} md={6}>
                  <Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>上级评分总分：</Text>
                  <Text strong style={{ fontSize: 28, lineHeight: '34px', color: 'var(--color-info)' }}>
                    {totalManagerScore}
                  </Text>
                </Col>
                <Col xs={24} sm={16} md={10}>
                  <Form.Item
                    name="suggested_level"
                    label="绩效等级"
                    rules={[{ required: true, message: '请填写等级' }]}
                    style={{ marginBottom: 4 }}
                  >
                    <Select placeholder="根据上级评分总分自动生成">
                      {LEVEL_OPTIONS.map(l => (
                        <Select.Option key={l.value} value={l.value}>
                          <StatusTag color={l.color}>{l.label}</StatusTag>
                        </Select.Option>
                      ))}
                    </Select>
                  </Form.Item>
                  <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>
                    {levelManuallySetRef.current ? '已手动调整等级' : '根据上级评分总分自动生成，可手动调整'}
                  </Text>
                </Col>
                <Col xs={24} md={8}>
                  <Text type="secondary" style={{ display: 'block', lineHeight: '22px' }}>
                    {isNewFlow
                      ? '沐腾科技流程模版采用 0-10 分制，最终分按上级评分 × 权重求和。'
                      : '小铁文娱流程模版沿用历史评分规则。'}
                  </Text>
                </Col>
              </Row>
            </PageCard>
          </Col>
          {quotaPanel && (
            <Col xs={24} lg={10} xl={8}>
              {quotaPanel}
            </Col>
          )}
        </Row>

            <PageCard title="指标评分" extra={
              !['locked', 'hr_confirmed', 'manager_confirmed'].includes(participant?.status || '') ? (
                <Button
                  data-testid="performance-manager-auto-score"
                  type="primary"
                  icon={<ThunderboltOutlined />}
                  loading={autoScoring}
                  onClick={handleAutoScore}
                >
                  一键评分
                </Button>
              ) : null
            }>
              <Table
                dataSource={form.getFieldValue('items') || []}
                columns={columns}
                rowKey="record_id"
                pagination={false}
                size="small"
                bordered
                tableLayout="fixed"
                scroll={{ x: 1064 }}
              />
            </PageCard>

            {bonusItems.length > 0 && (
              <PageCard title="附加考核项" style={{ marginTop: 16 }}>
                <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>
                  附加分仅作为参考或激励依据，不计入总分
                </Text>
                <Table
                  dataSource={bonusItems}
                  rowKey="id"
                  pagination={false}
                  size="small"
                  bordered
                  columns={[
                    {
                      title: '指标名称',
                      dataIndex: 'item_name',
                      key: 'item_name',
                      width: 200
                    },
                    {
                      title: '权重',
                      dataIndex: 'weight',
                      key: 'weight',
                      width: 80,
                      render: (val: number) => `${(val * 100).toFixed(0)}%`
                    },
                    {
                      title: '员工自评',
                      dataIndex: 'self_score',
                      key: 'self_score',
                      width: 100,
                      render: (val: number) => val || '-'
                    },
                    {
                      title: '附加分',
                      dataIndex: 'bonus_score',
                      key: 'bonus_score',
                      width: 100,
                      render: (val: number) => val || '-'
                    },
                    {
                      title: '考核上级评分',
                      key: 'manager_score',
                      width: 120,
                      render: (_: any, record: any) => (
                        <InputNumber
                          min={0}
                          max={scoreMax}
                          step={scoreStep}
                          style={{ width: '100%' }}
                          value={record.manager_score || 0}
                          onChange={(val) => {
                            const updated = bonusItems.map(item =>
                              item.id === record.id ? { ...item, manager_score: val || 0 } : item
                            )
                            setBonusItems(updated)
                          }}
                        />
                      )
                    },
                    {
                      title: '附件',
                      dataIndex: 'attachments',
                      key: 'attachments',
                      width: 200,
                      render: (val: any) => {
                        const attachments = Array.isArray(val) ? val : []
                        if (attachments.length === 0) return '-'
                        return (
                          <Image.PreviewGroup>
                            <Space wrap size={4}>
                              {attachments.map((url: string, idx: number) => (
                                <AuthorizedImage
                                  key={idx}
                                  src={url}
                                  width={48}
                                  height={48}
                                  style={{ objectFit: 'cover', borderRadius: 4 }}
                                  preview={{ mask: '查看' }}
                                />
                              ))}
                            </Space>
                          </Image.PreviewGroup>
                        )
                      }
                    }
                  ]}
                />
              </PageCard>
            )}

            <PageCard title="上级总体评价" style={{ marginTop: 16 }}>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item name="evaluation_good" label="做得好的地方">
                    <TextArea data-testid="performance-manager-good" rows={4} placeholder="请描述员工做得好的地方" />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="evaluation_improvement" label="需要改进的地方">
                    <TextArea data-testid="performance-manager-improvement" rows={4} placeholder="请描述需要改进的地方" />
                  </Form.Item>
                </Col>
              </Row>
            </PageCard>

            <div style={{ textAlign: 'center', marginTop: 24 }}>
              <Button data-testid="performance-manager-submit" type="primary" icon={<CheckCircleOutlined />} loading={saving} onClick={handleSubmit} size="large">
                {isManagerRecheck ? '提交复核意见' : '提交评分'}
              </Button>
            </div>
      </Form>

      <Modal
        title="附件预览"
        open={previewAttachments.visible}
        onCancel={closeAttachmentPreview}
        footer={null}
        width={960}
      >
        <div style={{ display: 'grid', gridTemplateColumns: '220px minmax(0, 1fr)', gap: 16, minWidth: 0 }}>
          <div style={{ borderRight: '1px solid var(--color-border-light)', paddingRight: 12, minWidth: 0, overflow: 'hidden' }}>
            <Space direction="vertical" size={8} style={{ width: '100%', minWidth: 0 }}>
              {previewAttachments.attachments.map((url, idx) => {
                const active = idx === previewAttachments.currentIndex
                const fileName = attachmentName(url, idx)
                return (
                  <Button
                    key={`${url}-${idx}`}
                    data-testid={`performance-manager-attachment-item-${idx}`}
                    type={active ? 'primary' : 'default'}
                    block
                    icon={<PaperClipOutlined />}
                    title={fileName}
                    onClick={() => setPreviewAttachments(prev => ({ ...prev, currentIndex: idx }))}
                    style={{ justifyContent: 'flex-start', overflow: 'hidden' }}
                  >
                    <span style={{ display: 'block', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {fileName}
                    </span>
                  </Button>
                )
              })}
            </Space>
          </div>
          <div style={{ minWidth: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12, minWidth: 0 }}>
              <Text
                strong
                data-testid="performance-manager-attachment-current-name"
                title={currentPreviewName}
                style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
              >
                {currentPreviewName}
              </Text>
              {currentPreviewUrl && (
                <Button
                  size="small"
                  onClick={() => {
                    void downloadAuthorizedFile(currentPreviewUrl, currentPreviewName).catch(() => message.error('附件下载失败'))
                  }}
                  style={{ flex: 'none' }}
                >
                  下载
                </Button>
              )}
            </div>
            {renderAttachmentPreview()}
          </div>
        </div>
      </Modal>
    </PageContainer>
  )
}

export default PerformanceManagerEval
