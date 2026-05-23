import React, { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  Card, Typography, Form, Input, InputNumber, Button, Space, Divider,
  message, Spin, Row, Col, Table, Alert, Tag
} from 'antd'
import { SaveOutlined, ArrowLeftOutlined, CheckCircleOutlined } from '@ant-design/icons'
import { performanceAPI, PerformanceGoalRecord, PerformanceParticipant } from '../services/api'

const { Title, Text, Paragraph } = Typography
const { TextArea } = Input

const PerformanceSelfEval: React.FC = () => {
  const { activityId, participantId } = useParams<{ activityId: string; participantId: string }>()
  const navigate = useNavigate()
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [records, setRecords] = useState<PerformanceGoalRecord[]>([])
  const [bonusRecords, setBonusRecords] = useState<PerformanceGoalRecord[]>([])
  const [formItems, setFormItems] = useState<any[]>([])
  const [formBonusItems, setFormBonusItems] = useState<any[]>([])
  const [totalSelfScore, setTotalSelfScore] = useState(0)

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
      const items = allItems.filter((item: PerformanceGoalRecord) => item.section_type !== 'bonus_penalty')
      const bonusItems = allItems.filter((item: PerformanceGoalRecord) => item.section_type === 'bonus_penalty')
      setRecords(items)
      setBonusRecords(bonusItems)

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

  const calcTotal = (items?: any[]) => {
    const data = items || form.getFieldsValue().items || []
    const total = (data || []).reduce((sum: number, i: any) => sum + (i.self_score || 0) * (i.weight || 0), 0)
    setTotalSelfScore(Math.round(total * 100) / 100)
  }

  const handleValuesChange = (_: any, allValues: any) => {
    if (allValues.items) {
      calcTotal(allValues.items)
    }
    if (allValues.bonus_items) {
      setFormBonusItems(allValues.bonus_items)
    }
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      const items = values.items.map((i: any) => ({
        record_id: i.record_id,
        actual_result: i.actual_result,
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
      message.success('自评提交成功')
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
            <div>
              {showDivider && (
                <Tag color={isQuant ? 'blue' : 'green'} style={{ marginBottom: 4 }}>
                  {isQuant ? '量化指标' : '关键行动'}
                </Tag>
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
            <div style={{ fontSize: 12 }}>
              {record.red_line_value && <div>红线: {record.red_line_value}</div>}
              {record.target_value && <div>目标: {record.target_value}</div>}
              {record.challenge_value && <div>挑战: {record.challenge_value}</div>}
              {record.scoring_rule && <div>考核: {record.scoring_rule}</div>}
            </div>
          )
        }
        const qualitativeTarget = record.target_value || record.scoring_rule
        return (
          <Text type="secondary" style={{ fontSize: 12 }}>
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
        <Form.Item name={['items', idx, 'actual_result']} style={{ margin: 0 }}
          rules={[{ required: true, message: '请填写达成结果' }]}>
          <TextArea rows={2} placeholder="描述实际完成情况" />
        </Form.Item>
      )
    },
    {
      title: '自评得分',
      key: 'self_score',
      width: 100,
      render: (_: any, __: any, idx: number) => (
        <Form.Item name={['items', idx, 'self_score']} style={{ margin: 0 }}
          rules={[{ required: true, message: '请评分' }]}>
          <InputNumber min={0} max={120} style={{ width: '100%' }} />
        </Form.Item>
      )
    }
  ]

  if (loading) return <div style={{ textAlign: 'center', padding: 100 }}><Spin size="large" /></div>

  return (
    <div style={{ padding: 24 }}>
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(-1)}>返回</Button>
        <Title level={4} style={{ margin: 0 }}>绩效自评</Title>
      </Space>

      <Form form={form} onValuesChange={handleValuesChange} layout="vertical">
        <Card title="指标评分">
          <Table
            dataSource={formItems}
            columns={columns}
            rowKey="record_id"
            pagination={false}
            size="small"
            bordered
          />
        </Card>

        {bonusRecords.length > 0 && (
          <Card title="附加考核项" style={{ marginTop: 16 }}>
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
                      <InputNumber min={0} max={100} style={{ width: '100%' }} placeholder="0-100" />
                    </Form.Item>
                  )
                }
              ]}
            />
          </Card>
        )}

        <Card title="系统自动计算" style={{ marginTop: 16 }}>
          <Row gutter={16}>
            <Col span={8}>
              <Text>自评总分：</Text>
              <Text strong style={{ fontSize: 24, color: '#1890ff' }}>{totalSelfScore}</Text>
            </Col>
          </Row>
        </Card>

        <Card title="员工自我评价" style={{ marginTop: 16 }}>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="evaluation_good" label="做得好的地方">
                <TextArea rows={4} placeholder="请描述本周期做得好的地方" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="evaluation_improvement" label="需要改进的地方">
                <TextArea rows={4} placeholder="请描述需要改进的地方" />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        <div style={{ textAlign: 'center', marginTop: 24 }}>
          <Button type="primary" icon={<CheckCircleOutlined />} loading={saving} onClick={handleSubmit} size="large">
            提交自评
          </Button>
        </div>
      </Form>
    </div>
  )
}

export default PerformanceSelfEval
