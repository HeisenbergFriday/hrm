import React, { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  Typography, Form, Input, InputNumber, Button, Space,
  message, Spin, Row, Col, Table, Alert
} from 'antd'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import { ArrowLeftOutlined, CheckCircleOutlined } from '@ant-design/icons'
import { performanceAPI, PerformanceActivity, PerformanceGoalRecord, PerformanceParticipant } from '../services/api'
import AttachmentUpload from '../components/AttachmentUpload'

const { Title, Text } = Typography
const { TextArea } = Input

function isReviewGoalRecord(activity: PerformanceActivity | null, record: PerformanceGoalRecord) {
  if (activity?.flow_type !== 'new') return true
  return String(record.goal_phase || 'review').trim() !== 'plan'
}

function getSelfEvalDeadlineTime(endAt?: string) {
  const value = String(endAt || '').trim()
  if (!value) return null
  const normalized = /^\d{4}-\d{2}-\d{2}$/.test(value) ? `${value}T23:59:59` : value
  const time = new Date(normalized).getTime()
  return Number.isFinite(time) ? time : null
}

function formatCountdown(remainingMs: number) {
  if (remainingMs <= 0) return '已截止'
  const totalMinutes = Math.max(1, Math.ceil(remainingMs / 60000))
  const days = Math.floor(totalMinutes / 1440)
  const hours = Math.floor((totalMinutes % 1440) / 60)
  const minutes = totalMinutes % 60
  if (days > 0) return `${days}天 ${hours}小时 ${minutes}分钟`
  if (hours > 0) return `${hours}小时 ${minutes}分钟`
  return `${minutes}分钟`
}

function isSelfEvalSubmitted(status?: string) {
  return [
    'self_submitted',
    'manager_submitted',
    'result_confirmed',
    'employee_confirmed',
    'manager_recheck',
    'manager_confirmed',
    'hr_confirmed',
    'locked',
  ].includes(String(status || '').trim())
}

const PerformanceSelfEval: React.FC = () => {
  const { participantId } = useParams<{ activityId: string; participantId: string }>()
  const navigate = useNavigate()
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [activity, setActivity] = useState<PerformanceActivity | null>(null)
  const [participant, setParticipant] = useState<PerformanceParticipant | null>(null)
  const [records, setRecords] = useState<PerformanceGoalRecord[]>([])
  const [bonusRecords, setBonusRecords] = useState<PerformanceGoalRecord[]>([])
  const [formItems, setFormItems] = useState<any[]>([])
  const [formBonusItems, setFormBonusItems] = useState<any[]>([])
  const [totalSelfScore, setTotalSelfScore] = useState(0)
  const [now, setNow] = useState(() => Date.now())
  const isNewFlow = activity?.flow_type === 'new'
  const scoreMax = isNewFlow ? 10 : 120
  const scoreStep = isNewFlow ? 0.1 : 1
  const missingReviewRecords = isNewFlow && !loading && formItems.length === 0
  const participantStatus = String(participant?.status || '').trim()
  const isHRFinalized = Boolean(participant?.hr_confirmed_at) || ['hr_confirmed', 'locked'].includes(participantStatus)
  const isAfterManagerConfirmEdit = !isHRFinalized && (participantStatus === 'manager_confirmed' || participantStatus === 'manager_recheck' || Boolean(participant?.manager_confirmed_at))
  const selfEvalDeadlineTime = getSelfEvalDeadlineTime(activity?.self_eval_end_at)
  const showSelfEvalCountdown = Boolean(selfEvalDeadlineTime && !isSelfEvalSubmitted(participant?.status))
  const selfEvalRemainingMs = selfEvalDeadlineTime ? selfEvalDeadlineTime - now : 0
  const selfEvalCountdownType = selfEvalRemainingMs <= 0 ? 'error' : selfEvalRemainingMs <= 24 * 60 * 60 * 1000 ? 'warning' : 'info'
  const selfEvalCountdownMessage = selfEvalRemainingMs <= 0
    ? '自评已到截止时间'
    : `自评倒计时：还剩 ${formatCountdown(selfEvalRemainingMs)}`

  useEffect(() => {
    if (!showSelfEvalCountdown) return undefined
    setNow(Date.now())
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [showSelfEvalCountdown, selfEvalDeadlineTime])

  const loadData = useCallback(async () => {
    if (!participantId) return
    setLoading(true)
    try {
      const [recordsRes, participantRes] = await Promise.all([
        performanceAPI.getGoalRecords(Number(participantId)),
        performanceAPI.getParticipant(Number(participantId))
      ])
      const allItems: PerformanceGoalRecord[] = recordsRes.data?.items || []
      const participant: PerformanceParticipant = participantRes.data?.participant || participantRes.data
      const currentActivity: PerformanceActivity | null = participantRes.data?.activity || null
      const items = allItems.filter((item: PerformanceGoalRecord) =>
        item.section_type !== 'bonus_penalty' && isReviewGoalRecord(currentActivity, item)
      )
      const bonusItems = allItems.filter((item: PerformanceGoalRecord) => item.section_type === 'bonus_penalty')
      setRecords(items)
      setBonusRecords(bonusItems)
      setActivity(currentActivity)
      setParticipant(participant)

      const itemsData = items.map(i => ({
        record_id: i.id,
        item_name: i.item_name,
        section_type: i.section_type,
        weight: i.weight,
        weight_percent: (i.weight * 100).toFixed(0),
        red_line_value: i.red_line_value,
        target_value: i.target_value,
        challenge_value: i.challenge_value,
        scoring_rule: i.scoring_rule,
        actual_result: i.actual_result || '',
        attachments: i.attachments || [],
        self_score: i.self_score || 0
      }))

      const bonusData = bonusItems.map(i => ({
        record_id: i.id,
        item_name: i.item_name,
        self_score: i.self_score || 0
      }))

      setFormItems(itemsData)
      setFormBonusItems(bonusData)

      form.setFieldsValue({
        items: itemsData,
        bonus_items: bonusData,
        evaluation_good: participant?.self_evaluation_good || '',
        evaluation_improvement: participant?.self_evaluation_improvement || ''
      })
      calcTotal(itemsData)
    } catch {
      message.error('加载目标指标失败')
    } finally {
      setLoading(false)
    }
  }, [participantId, form])

  useEffect(() => { loadData() }, [loadData])

  const toNumber = (value: any) => {
    const numberValue = Number(value)
    return Number.isFinite(numberValue) ? numberValue : 0
  }

  const calcTotal = (items?: any[]) => {
    const data = items || form.getFieldsValue().items || []
    const total = (data || []).reduce((sum: number, i: any, idx: number) => {
      const weight = toNumber(i?.weight ?? formItems[idx]?.weight ?? records[idx]?.weight)
      return sum + toNumber(i?.self_score) * weight
    }, 0)
    setTotalSelfScore(Math.round(total * 100) / 100)
  }

  const handleValuesChange = (changedValues: any, allValues: any) => {
    if (changedValues.items !== undefined || allValues.items) {
      calcTotal(form.getFieldValue('items') || allValues.items)
    }
    if (allValues.bonus_items) {
      setFormBonusItems(allValues.bonus_items)
    }
  }

  const handleSubmit = async () => {
    if (missingReviewRecords) {
      message.warning('缺少上一季度绩效考核指标，无法提交自评')
      return
    }
    if (isHRFinalized) {
      message.warning('HR已确认，不能修改自评')
      return
    }
    try {
      const values = await form.validateFields()
      const items = (values.items || []).map((i: any) => ({
        record_id: i.record_id,
        actual_result: i.actual_result,
        attachments: i.attachments || [],
        self_score: i.self_score
      }))

      const bonusItems = (values.bonus_items || []).map((i: any) => ({
        record_id: i.record_id,
        self_score: i.self_score || 0
      }))

      setSaving(true)
      await performanceAPI.submitGoalSelfEvaluation(Number(participantId), {
        items,
        bonus_items: bonusItems,
        evaluation_good: values.evaluation_good || '',
        evaluation_improvement: values.evaluation_improvement || ''
      })
      message.success(isAfterManagerConfirmEdit ? '自评修改已提交，已通知主管复核' : '自评提交成功')
      navigate(-1)
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '提交失败')
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    {
      title: '指标名称',
      dataIndex: 'item_name',
      key: 'item_name',
      width: 150,
      render: (val: string, record: any, idx: number) => {
        const prev = idx > 0 ? formItems[idx - 1] : null
        const showDivider = idx === 0 || (prev && prev.section_type !== record?.section_type)
        const isQuant = record?.section_type === 'quantitative'
        return (
          <>
            <Form.Item name={['items', idx, 'record_id']} hidden><Input /></Form.Item>
            <Form.Item name={['items', idx, 'weight']} hidden><Input /></Form.Item>
            <div>
              {showDivider && (
                <StatusTag color={isQuant ? 'blue' : 'green'} style={{ marginBottom: 4 }}>
                  {isQuant ? '量化指标' : '关键行动'}
                </StatusTag>
              )}
              <Text strong>{val}</Text>
            </div>
          </>
        )
      }
    },
    {
      title: '权重',
      dataIndex: 'weight_percent',
      key: 'weight',
      width: 70,
      render: (val: string) => <Text>{val}%</Text>
    },
    {
      title: '目标',
      key: 'target',
      width: 180,
      render: (_: any, record: any) => {
        if (record.section_type === 'quantitative') {
          return (
            <div style={{ fontSize: 'var(--font-size-xs)' }}>
              {record.red_line_value && <div>红线: {record.red_line_value}</div>}
              {record.target_value && <div>目标: {record.target_value}</div>}
              {record.challenge_value && <div>挑战: {record.challenge_value}</div>}
              {record.scoring_rule && <div>考核: {record.scoring_rule}</div>}
            </div>
          )
        }
        const qualitativeTarget = record.target_value || record.scoring_rule
        return (
          <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>
            {qualitativeTarget ? (qualitativeTarget.length > 50 ? qualitativeTarget.substring(0, 50) + '...' : qualitativeTarget) : '-'}
          </Text>
        )
      }
    },
    {
      title: '实际达成结果',
      key: 'actual_result',
      width: 250,
      render: (_: any, __: any, idx: number) => (
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          <Form.Item name={['items', idx, 'actual_result']} style={{ margin: 0 }}
            rules={[{ required: true, message: '请填写达成结果' }]}>
            <TextArea data-testid={`performance-self-actual-${idx}`} rows={2} placeholder="描述实际完成情况" />
          </Form.Item>
          <Form.Item name={['items', idx, 'attachments']} style={{ margin: 0 }}>
            <AttachmentUpload maxCount={5} />
          </Form.Item>
        </Space>
      )
    },
    {
      title: '自评得分',
      key: 'self_score',
      width: 100,
      render: (_: any, __: any, idx: number) => (
        <Form.Item name={['items', idx, 'self_score']} style={{ margin: 0 }}
          rules={[{ required: true, message: '请评分' }]}>
          <InputNumber data-testid={`performance-self-score-${idx}`} min={0} max={scoreMax} step={scoreStep} style={{ width: '100%' }} />
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

  return (
    <PageContainer data-testid="performance-self-eval-page" title="绩效自评">
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(-1)}>返回</Button>
        <Title level={4} style={{ margin: 0 }}>绩效自评</Title>
        <Text type="secondary">{isNewFlow ? '0-10 分制' : '0-120 分制'}</Text>
        <Text>自评总分：</Text>
        <Text data-testid="performance-self-total-score-inline" strong style={{ fontSize: 20, color: 'var(--color-info)' }}>
          {totalSelfScore}
        </Text>
      </Space>

      {showSelfEvalCountdown && (
        <Alert
          data-testid="performance-self-deadline-countdown"
          type={selfEvalCountdownType}
          showIcon
          message={selfEvalCountdownMessage}
          description={`截止时间：${activity?.self_eval_end_at}。请在截止前提交，自评完成后将不再展示倒计时提示。`}
          style={{ marginBottom: 16 }}
        />
      )}

      {isAfterManagerConfirmEdit && (
        <Alert
          data-testid="performance-self-manager-recheck-notice"
          type="warning"
          showIcon
          message="主管确认后修改将进入待领导复核"
          description="提交后会通知直属领导查看最新完成情况并复核；1小时内多次修改只提醒一次。"
          style={{ marginBottom: 16 }}
        />
      )}

      {isHRFinalized && (
        <Alert
          data-testid="performance-self-hr-locked-notice"
          type="info"
          showIcon
          message="HR已确认，不能修改"
          description="该绩效结果已完成HR确认，如需调整请联系HR处理。"
          style={{ marginBottom: 16 }}
        />
      )}

      <Form form={form} onValuesChange={handleValuesChange} layout="vertical">
        <PageCard
          title="指标评分"
          extra={
            <Space>
              <Text type="secondary">实时总分</Text>
              <Text strong style={{ color: 'var(--color-info)', fontSize: 18 }}>{totalSelfScore}</Text>
            </Space>
          }
        >
          <Text type="secondary" style={{ display: 'block', marginBottom: 12 }}>
            自评总分按每项自评得分 × 权重自动汇总，附加考核项不计入总分。
          </Text>
          {missingReviewRecords && (
            <Alert
              type="warning"
              showIcon
              message="缺少上一季度绩效考核指标"
              description={
                <div>
                  <div>新流程自评需要先承接上一期目标计划作为本期考核指标。</div>
                  <div style={{ marginTop: 4 }}>
                    请联系 HR 在【绩效总览 → 参与人列表】点击本活动的<Text strong>「补录」</Text>为你手动录入本期考核指标；
                    或让 HR 在活动编辑中选择「承接上一期活动」，系统会自动把上一期设定的下季度目标同步为本期考核指标。
                  </div>
                </div>
              }
              style={{ marginBottom: 12 }}
            />
          )}
          <Table
            dataSource={formItems}
            columns={columns}
            rowKey="record_id"
            pagination={false}
            size="small"
            bordered
            locale={{ emptyText: missingReviewRecords ? '缺少上一季度绩效考核指标' : undefined }}
          />
        </PageCard>

        {bonusRecords.length > 0 && (
          <PageCard title="附加考核项" style={{ marginTop: 16 }}>
            <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>
              附加分仅作为参考或激励依据，不计入总分
            </Text>
            <Table
              dataSource={formBonusItems}
              rowKey="record_id"
              pagination={false}
              size="small"
              bordered
              columns={[
                {
                  title: '指标名称',
                  dataIndex: 'item_name',
                  key: 'item_name',
                  width: 300,
                  render: (val: string, _: any, idx: number) => (
                    <>
                      <Form.Item name={['bonus_items', idx, 'record_id']} hidden><Input /></Form.Item>
                      <Text>{val}</Text>
                    </>
                  )
                },
                {
                  title: '自评得分',
              key: 'self_score',
              width: 150,
              render: (_: any, __: any, idx: number) => (
                <Form.Item name={['bonus_items', idx, 'self_score']} style={{ margin: 0 }}>
                      <InputNumber min={0} max={scoreMax} step={scoreStep} style={{ width: '100%' }} placeholder={`0-${scoreMax}`} />
                    </Form.Item>
              )
            }
              ]}
            />
          </PageCard>
        )}

        <PageCard title="系统自动计算" style={{ marginTop: 16 }}>
          <Row gutter={16}>
            <Col span={8}>
              <Text>自评总分：</Text>
              <Text data-testid="performance-self-total-score" strong style={{ fontSize: 24, color: 'var(--color-info)' }}>{totalSelfScore}</Text>
              <Text type="secondary" style={{ marginLeft: 8 }}>
                {isNewFlow ? '满分 10' : '满分 120'}
              </Text>
            </Col>
          </Row>
        </PageCard>

        <PageCard title="员工自我评价" style={{ marginTop: 16 }}>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="evaluation_good" label="做得好的地方">
                <TextArea data-testid="performance-self-good" rows={4} placeholder="请描述本周期做得好的地方" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="evaluation_improvement" label="需要改进的地方">
                <TextArea data-testid="performance-self-improvement" rows={4} placeholder="请描述需要改进的地方" />
              </Form.Item>
            </Col>
          </Row>
        </PageCard>

        <div style={{ textAlign: 'center', marginTop: 24 }}>
          <Button data-testid="performance-self-submit" type="primary" icon={<CheckCircleOutlined />} loading={saving} disabled={missingReviewRecords || isHRFinalized} onClick={handleSubmit} size="large">
            {isAfterManagerConfirmEdit ? '提交修改并通知主管' : '提交自评'}
          </Button>
        </div>
      </Form>
    </PageContainer>
  )
}

export default PerformanceSelfEval
