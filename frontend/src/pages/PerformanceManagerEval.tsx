import React, { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  Typography, Form, Input, InputNumber, Button, Space,
  message, Spin, Row, Col, Table, Select, Progress, Tag, Modal, Badge, Image
} from 'antd'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import { ArrowLeftOutlined, CheckCircleOutlined, PaperClipOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { performanceAPI, PerformanceGoalRecord, PerformanceParticipant, TeamQuotaStatus } from '../services/api'
import { withFileAccessToken } from '../utils/authFileUrl'

const { Title, Text } = Typography
const { TextArea } = Input

const LEVEL_OPTIONS = [
  { value: 'S', label: 'S - 杰出', color: '#f50' },
  { value: 'A', label: 'A - 优秀', color: '#2db7f5' },
  { value: 'B', label: 'B - 良好', color: '#87d068' },
  { value: 'C', label: 'C - 待改进', color: '#faad14' },
  { value: 'D', label: 'D - 不合格', color: '#ff4d4f' },
]

const calcPerformanceLevel = (score: number) => {
  if (score >= 100) return 'S'
  if (score >= 90) return 'A'
  if (score >= 80) return 'B'
  if (score >= 60) return 'C'
  return 'D'
}

const PerformanceManagerEval: React.FC = () => {
  const { activityId, participantId } = useParams<{ activityId: string; participantId: string }>()
  const navigate = useNavigate()
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [totalManagerScore, setTotalManagerScore] = useState(0)
  const [quotaData, setQuotaData] = useState<TeamQuotaStatus[]>([])
  const [participant, setParticipant] = useState<PerformanceParticipant | null>(null)
  const [bonusItems, setBonusItems] = useState<PerformanceGoalRecord[]>([])
  const [previewAttachments, setPreviewAttachments] = useState<{ visible: boolean; attachments: string[] }>({ visible: false, attachments: [] })
  const [autoScoring, setAutoScoring] = useState(false)

  const loadData = useCallback(async () => {
    if (!participantId || !activityId) return
    setLoading(true)
    try {
      const [recordsRes, quotaRes, participantRes] = await Promise.all([
        performanceAPI.getGoalRecords(Number(participantId)),
        performanceAPI.getRealtimeDistributionCheck(Number(activityId)),
        performanceAPI.getParticipant(Number(participantId))
      ])

      const allItems: PerformanceGoalRecord[] = recordsRes.data?.items || []
      const items: PerformanceGoalRecord[] = allItems.filter(
        (item: PerformanceGoalRecord) => item.section_type !== 'bonus_penalty'
      )
      const bonus: PerformanceGoalRecord[] = allItems.filter(
        (item: PerformanceGoalRecord) => item.section_type === 'bonus_penalty'
      )
      const currentParticipant = participantRes.data?.participant || participantRes.data
      setQuotaData(quotaRes.data?.teams || [])
      setParticipant(currentParticipant)
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
      calcTotal(formItems)
    } catch {
      message.error('加载数据失败')
    } finally {
      setLoading(false)
    }
  }, [participantId, activityId, form])

  useEffect(() => { loadData() }, [loadData])

  const calcTotal = (items: any[]) => {
    const total = items.reduce((sum, i) => sum + (i.manager_score || 0) * (i.weight || 0), 0)
    const roundedTotal = Math.round(total * 100) / 100
    setTotalManagerScore(roundedTotal)
    if (!levelManuallySetRef.current) {
      const level = calcPerformanceLevel(roundedTotal)
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
          return { ...i, manager_score: result.score }
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

  const getQuotaForLevel = (level: string) => {
    const team = currentTeamQuota()
    if (!team) return null
    const key = levelQuotaKey(level)
    const quota = team.levels[key]
    if (!quota) return null

    const currentLevelKey = levelQuotaKey(participant?.final_level || participant?.suggested_level)
    return {
      ...quota,
      current: currentLevelKey === key ? Math.max(0, quota.current - 1) : quota.current
    }
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
    const level = form.getFieldValue('suggested_level')
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
  }, [totalManagerScore])

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
      message.success('评分提交成功')
      navigate(-1)
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '提交失败')
    } finally {
      setSaving(false)
    }
  }

  const renderQuotaPanel = () => {
    const team = currentTeamQuota()
    if (!team) return null
    return (
      <PageCard title="配额进度" size="small" style={{ marginBottom: 16 }}>
        <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>
          团队：{team.manager_name || '未分组'}（共 {team.total} 人）
        </Text>
        {['S', 'A', 'B', 'CD'].map(level => {
          const q = team.levels[level]
          if (!q) return null
          const percent = q.max > 0 ? Math.round((q.current / q.max) * 100) : 0
          const isFull = q.current >= q.max
          return (
            <div key={level} style={{ marginBottom: 8 }}>
              <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                <StatusTag color={isFull ? 'red' : 'blue'}>{level}</StatusTag>
                <Text>{q.current} / {q.max}（{q.percent}%）</Text>
              </Space>
              <Progress percent={percent} size="small" status={isFull ? 'exception' : 'active'}
                strokeColor={isFull ? '#ff4d4f' : undefined} />
            </div>
          )
        })}
      </PageCard>
    )
  }

  const columns = [
    {
      title: '指标名称',
      dataIndex: 'item_name',
      key: 'item_name',
      width: 150,
      render: (val: string, _: any, idx: number) => {
        const items = form.getFieldValue('items') || []
        const item = items[idx]
        const isQuant = item?.section_type === 'quantitative'
        return (
          <>
            <Form.Item name={['items', idx, 'record_id']} hidden><Input /></Form.Item>
            <div>
              <StatusTag color={isQuant ? 'blue' : 'green'} style={{ marginBottom: 4 }}>
                {isQuant ? '量化指标' : '关键行动'}
              </StatusTag>
              <Text strong>{val}</Text>
            </div>
          </>
        )
      }
    },
    {
      title: '目标/评分规则',
      key: 'target_rule',
      width: 180,
      render: (_: any, __: any, idx: number) => {
        const items = form.getFieldValue('items') || []
        const item = items[idx]
        if (item?.section_type === 'quantitative') {
          return (
            <div style={{ fontSize: 'var(--font-size-xs)' }}>
              {item.red_line_value && <div style={{ color: 'var(--color-error)' }}>红线: {item.red_line_value}</div>}
              {item.target_value && <div style={{ color: 'var(--color-info)' }}>目标: {item.target_value}</div>}
              {item.challenge_value && <div style={{ color: 'var(--color-success)' }}>挑战: {item.challenge_value}</div>}
              {item.scoring_rule && <div style={{ color: 'var(--color-text-tertiary)' }}>考核: {item.scoring_rule}</div>}
              {!item.red_line_value && !item.target_value && !item.challenge_value && <Text type="secondary">-</Text>}
            </div>
          )
        }
        return (
          <Text type="secondary" style={{ fontSize: 12 }}>
            {(item?.target_value || item?.scoring_rule)
              ? ((item.target_value || item.scoring_rule).length > 50
                ? (item.target_value || item.scoring_rule).substring(0, 50) + '...'
                : (item.target_value || item.scoring_rule))
              : '-'}
          </Text>
        )
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
      width: 150,
      render: (val: string) => <Text style={{ fontSize: 'var(--font-size-xs)' }}>{val || '-'}</Text>
    },
    {
      title: '附件',
      key: 'attachments',
      width: 100,
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
              onClick={() => setPreviewAttachments({ visible: true, attachments })}
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
      width: 80,
      render: (val: number) => <Text>{val}</Text>
    },
    {
      title: '上级评分',
      key: 'manager_score',
      width: 120,
      render: (_: any, __: any, idx: number) => (
        <Form.Item name={['items', idx, 'manager_score']} style={{ margin: 0 }}
          rules={[{ required: true, message: '请评分' }]}>
          <InputNumber min={0} max={120} style={{ width: '100%' }} />
        </Form.Item>
      )
    }
  ]

  if (loading) return <div style={{ textAlign: 'center', padding: 100 }}><Spin size="large" /></div>

  return (
    <PageContainer title="上级绩效评分">
      <Row gutter={24}>
        <Col span={18}>
          <Space style={{ marginBottom: 16 }}>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(-1)}>返回</Button>
            <Title level={4} style={{ margin: 0 }}>上级绩效评分</Title>
          </Space>

          <Form form={form} onValuesChange={handleValuesChange} layout="vertical">
            <PageCard title="指标评分" extra={
              !['locked', 'hr_confirmed', 'manager_confirmed'].includes(participant?.status || '') ? (
                <Button
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
                      title: '主管评分',
                      key: 'manager_score',
                      width: 120,
                      render: (_: any, record: any) => (
                        <InputNumber
                          min={0}
                          max={100}
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
                                <Image
                                  key={idx}
                                  src={withFileAccessToken(url)}
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

            <PageCard title="总分与等级" style={{ marginTop: 16 }}>
              <Row gutter={24}>
                <Col span={6}>
                  <Text>上级评分总分：</Text>
                  <Text strong style={{ fontSize: 24, color: 'var(--color-info)' }}>{totalManagerScore}</Text>
                </Col>
                <Col span={8}>
                  <Form.Item name="suggested_level" label="绩效等级" rules={[{ required: true, message: '请填写等级' }]}>
                    <Select placeholder="根据上级评分总分自动生成">
                      {LEVEL_OPTIONS.map(l => (
                        <Select.Option key={l.value} value={l.value}>
                          <StatusTag color={l.color}>{l.label}</StatusTag>
                        </Select.Option>
                      ))}
                    </Select>
                  </Form.Item>
                  <Text type="secondary">
                    {levelManuallySetRef.current
                      ? '已手动调整等级'
                      : '根据上级评分总分自动生成，可手动调整'}
                  </Text>
                </Col>
              </Row>
            </PageCard>

            <PageCard title="上级总体评价" style={{ marginTop: 16 }}>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item name="evaluation_good" label="做得好的地方">
                    <TextArea rows={4} placeholder="请描述员工做得好的地方" />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="evaluation_improvement" label="需要改进的地方">
                    <TextArea rows={4} placeholder="请描述需要改进的地方" />
                  </Form.Item>
                </Col>
              </Row>
            </PageCard>

            <div style={{ textAlign: 'center', marginTop: 24 }}>
              <Button type="primary" icon={<CheckCircleOutlined />} loading={saving} onClick={handleSubmit} size="large">
                提交评分
              </Button>
            </div>
          </Form>
        </Col>

        <Col span={6}>
          {renderQuotaPanel()}
        </Col>
      </Row>

      <Modal
        title="附件列表"
        open={previewAttachments.visible}
        onCancel={() => setPreviewAttachments({ visible: false, attachments: [] })}
        footer={null}
      >
        <div style={{ maxHeight: 400, overflow: 'auto' }}>
          {previewAttachments.attachments.map((url, idx) => (
            <div key={idx} style={{ marginBottom: 8 }}>
              <a href={withFileAccessToken(url)} target="_blank" rel="noopener noreferrer">
                <PaperClipOutlined /> 附件 {idx + 1}
              </a>
            </div>
          ))}
        </div>
      </Modal>
    </PageContainer>
  )
}

export default PerformanceManagerEval
