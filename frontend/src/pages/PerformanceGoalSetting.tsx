import React, { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  Typography, Input, InputNumber, Button, Space,
  message, Spin, Table, Tag, Modal, AutoComplete
} from 'antd'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import {
  ArrowLeftOutlined, SaveOutlined, CheckCircleOutlined,
  PlusOutlined, DeleteOutlined, BulbOutlined, PaperClipOutlined
} from '@ant-design/icons'
import { performanceAPI, PerformanceGoalRecord, PerformanceParticipant } from '../services/api'
import AttachmentUpload from '../components/AttachmentUpload'

const { Title, Text } = Typography
const { TextArea } = Input

const PerformanceGoalSetting: React.FC = () => {
  const { activityId, participantId } = useParams<{ activityId: string; participantId: string }>()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [participant, setParticipant] = useState<PerformanceParticipant | null>(null)
  const [quantItems, setQuantItems] = useState<any[]>([])
  const [actionItems, setActionItems] = useState<any[]>([])
  const [suggestions, setSuggestions] = useState<any[]>([])
  const [showSuggestions, setShowSuggestions] = useState(false)
  const [quantSearchResults, setQuantSearchResults] = useState<Record<number, any[]>>({})
  const [actionSearchResults, setActionSearchResults] = useState<Record<number, any[]>>({})
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const loadData = useCallback(async () => {
    if (!participantId || !activityId) return
    setLoading(true)
    try {
      const [recordsRes, participantRes] = await Promise.all([
        performanceAPI.getGoalRecords(Number(participantId)),
        performanceAPI.getParticipant(Number(participantId))
      ])

      const allItems: PerformanceGoalRecord[] = recordsRes.data?.items || []
      const currentParticipant = participantRes.data?.participant || participantRes.data
      setParticipant(currentParticipant)

      const quant = allItems
        .filter((i: PerformanceGoalRecord) => i.section_type === 'quantitative')
        .map((i: PerformanceGoalRecord) => ({
          id: i.id,
          item_name: i.item_name,
          item_definition: i.item_definition,
          weight: i.weight,
          weight_percent: (i.weight * 100).toFixed(0),
          red_line_value: i.red_line_value,
          target_value: i.target_value,
          challenge_value: i.challenge_value,
          scoring_rule: i.scoring_rule,
          actual_result: i.actual_result,
          attachments: i.attachments || [],
          sort_order: i.sort_order
        }))

      const actions = allItems
        .filter((i: PerformanceGoalRecord) => i.section_type === 'key_action')
        .map((i: PerformanceGoalRecord) => ({
          id: i.id,
          item_name: i.item_name,
          item_definition: i.item_definition,
          weight: i.weight,
          weight_percent: (i.weight * 100).toFixed(0),
          target_value: i.target_value || i.scoring_rule,
          actual_result: i.actual_result,
          attachments: i.attachments || [],
          sort_order: i.sort_order
        }))

      setQuantItems(quant.length > 0 ? quant : [newQuantItem()])
      setActionItems(actions.length > 0 ? actions : [newActionItem()])
    } catch {
      message.error('加载数据失败')
    } finally {
      setLoading(false)
    }
  }, [participantId, activityId])

  useEffect(() => { loadData() }, [loadData])

  function newQuantItem() {
    return {
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
      sort_order: 0
    }
  }

  function newActionItem() {
    return {
      id: undefined,
      item_name: '',
      item_definition: '',
      weight: 0,
      weight_percent: '0',
      target_value: '',
      actual_result: '',
      attachments: [],
      sort_order: 0
    }
  }

  const handleAddQuantItem = () => {
    setQuantItems([...quantItems, newQuantItem()])
  }

  const handleRemoveQuantItem = (index: number) => {
    if (quantItems.length <= 1) {
      message.warning('至少保留一个量化指标')
      return
    }
    setQuantItems(quantItems.filter((_, i) => i !== index))
  }

  const handleAddActionItem = () => {
    setActionItems([...actionItems, newActionItem()])
  }

  const handleRemoveActionItem = (index: number) => {
    if (actionItems.length <= 1) {
      message.warning('至少保留一个关键行动')
      return
    }
    setActionItems(actionItems.filter((_, i) => i !== index))
  }

  const handleQuantItemChange = (index: number, field: string, value: any) => {
    const updated = [...quantItems]
    updated[index] = {
      ...updated[index],
      [field]: value,
      ...(field === 'weight' ? { weight_percent: (value * 100).toFixed(0) } : {})
    }
    setQuantItems(updated)
  }

  const handleActionItemChange = (index: number, field: string, value: any) => {
    const updated = [...actionItems]
    updated[index] = {
      ...updated[index],
      [field]: value,
      ...(field === 'weight' ? { weight_percent: (value * 100).toFixed(0) } : {})
    }
    setActionItems(updated)
  }

  const quantWeightTotal = quantItems.reduce((sum, i) => sum + (i.weight || 0), 0)
  const actionWeightTotal = actionItems.reduce((sum, i) => sum + (i.weight || 0), 0)
  const totalWeight = quantWeightTotal + actionWeightTotal

  const loadSuggestions = async () => {
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
    const newItem = {
      ...newQuantItem(),
      item_name: suggestion.name || suggestion.item_name,
      item_definition: suggestion.description || suggestion.item_definition,
      red_line_value: suggestion.red_line_value || '',
      target_value: suggestion.target_value || '',
      challenge_value: suggestion.challenge_value || '',
      scoring_rule: suggestion.scoring_rule || '',
      weight: suggestion.weight || 0
    }
    if (suggestion.section_type === 'key_action') {
      setActionItems([...actionItems, {
        ...newActionItem(),
        item_name: newItem.item_name,
        item_definition: newItem.item_definition,
        target_value: suggestion.target_value || suggestion.scoring_rule || '',
        weight: newItem.weight
      }])
    } else {
      setQuantItems([...quantItems, newItem])
    }
    setShowSuggestions(false)
    message.success('已应用建议')
  }

  const searchIndicators = useCallback((keyword: string, resultsSetter: React.Dispatch<React.SetStateAction<Record<number, any[]>>>, rowIndex: number, sectionType: string) => {
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    if (!keyword || keyword.trim().length < 1) {
      resultsSetter(prev => ({ ...prev, [rowIndex]: [] }))
      return
    }
    searchTimerRef.current = setTimeout(async () => {
      try {
        const res: any = await performanceAPI.searchIndicatorItems({ keyword: keyword.trim(), section_type: sectionType })
        const data = res.data || res
        const raw: any[] = data?.items || []
        resultsSetter(prev => ({ ...prev, [rowIndex]: raw }))
      } catch {
        resultsSetter(prev => ({ ...prev, [rowIndex]: [] }))
      }
    }, 300)
  }, [])

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
    setter(allItems.map((item, idx) => idx === rowIndex ? { ...item, ...patch } : item))
  }

  const buildPayload = () => {
    const items = [
      ...quantItems.map((item, idx) => ({
        id: item.id,
        section_type: 'quantitative',
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
      ...actionItems.map((item, idx) => ({
        id: item.id,
        section_type: 'key_action',
        item_name: item.item_name,
        item_definition: item.item_definition,
        weight: item.weight,
        target_value: item.target_value,
        actual_result: item.actual_result,
        attachments: item.attachments,
        sort_order: quantItems.length + idx
      }))
    ]
    return items
  }

  const handleSaveDraft = async () => {
    if (!participantId) return
    const items = buildPayload()
    if (items.some(i => !i.item_name)) {
      message.warning('请填写所有指标名称')
      return
    }
    setSaving(true)
    try {
      await performanceAPI.batchSaveGoalRecords(Number(participantId), { items })
      message.success('草稿保存成功')
    } catch (err: any) {
      message.error(err?.response?.data?.message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const validateRequiredFields = () => {
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
    return true
  }

  const handleSubmit = async () => {
    if (!participantId) return
    if (Math.abs(totalWeight - 1) > 0.001) {
      message.error(`权重合计必须为100%，当前为 ${(totalWeight * 100).toFixed(0)}%`)
      return
    }
    if (quantWeightTotal < 0.65 || quantWeightTotal > 0.75) {
      message.error('量化指标权重需约70%（允许65%-75%）')
      return
    }
    if (actionWeightTotal < 0.25 || actionWeightTotal > 0.35) {
      message.error('关键行动权重需约30%（允许25%-35%）')
      return
    }

    const items = buildPayload()
    if (!validateRequiredFields()) {
      return
    }

    Modal.confirm({
      title: '确认提交目标',
      content: '提交后将进入审批流程，确认继续？',
      onOk: async () => {
        setSubmitting(true)
        try {
          await performanceAPI.batchSaveGoalRecords(Number(participantId), { items })
          await performanceAPI.submitGoalApproval(Number(participantId))
          message.success('目标已提交')
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
      title: '指标名称',
      dataIndex: 'item_name',
      key: 'item_name',
      width: '40%',
      render: (_: any, __: any, idx: number) => (
        <AutoComplete
          value={quantItems[idx]?.item_name}
          options={getSearchOptions(quantSearchResults[idx] || [])}
          onSearch={(val) => searchIndicators(val, setQuantSearchResults, idx, 'quantitative')}
          onChange={(val) => handleQuantItemChange(idx, 'item_name', val)}
          onSelect={(val) => handleIndicatorSelect(val, idx, quantSearchResults[idx] || [], setQuantItems, quantItems, true)}
          placeholder="输入关键词搜索指标"
          style={{ width: '100%' }}
        />
      )
    },
    {
      title: '权重%',
      key: 'weight',
      width: 140,
      render: (_: any, __: any, idx: number) => (
        <InputNumber
          min={0}
          max={100}
          value={quantItems[idx]?.weight ? quantItems[idx].weight * 100 : 0}
          onChange={val => handleQuantItemChange(idx, 'weight', (val || 0) / 100)}
          style={{ width: '100%' }}
          addonAfter="%"
        />
      )
    },
    {
      title: '目标值',
      dataIndex: 'target_value',
      key: 'target_value',
      width: '35%',
      render: (_: any, __: any, idx: number) => (
        <Input
          value={quantItems[idx]?.target_value}
          onChange={e => handleQuantItemChange(idx, 'target_value', e.target.value)}
          placeholder="标准"
        />
      )
    },
    {
      title: '',
      key: 'action',
      width: 48,
      render: (_: any, __: any, idx: number) => (
        <Button
          type="text"
          danger
          icon={<DeleteOutlined />}
          onClick={() => handleRemoveQuantItem(idx)}
        />
      )
    }
  ]

  const quantExpandedRowRender = (record: any, idx: number) => (
    <div style={{ padding: '8px 0' }}>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', gap: 16 }}>
        <div>
          <Text style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-secondary)', fontWeight: 'var(--font-weight-medium)', marginBottom: 6, display: 'block' }}>指标定义</Text>
          <TextArea
            value={quantItems[idx]?.item_definition}
            onChange={e => handleQuantItemChange(idx, 'item_definition', e.target.value)}
            rows={2}
            placeholder="明确指标范围和计算公式"
          />
        </div>
        <div>
          <Text style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-secondary)', fontWeight: 'var(--font-weight-medium)', marginBottom: 6, display: 'block' }}>红线值</Text>
          <Input
            value={quantItems[idx]?.red_line_value}
            onChange={e => handleQuantItemChange(idx, 'red_line_value', e.target.value)}
            placeholder="最低"
          />
        </div>
        <div>
          <Text style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-secondary)', fontWeight: 'var(--font-weight-medium)', marginBottom: 6, display: 'block' }}>挑战值</Text>
          <Input
            value={quantItems[idx]?.challenge_value}
            onChange={e => handleQuantItemChange(idx, 'challenge_value', e.target.value)}
            placeholder="挑战"
          />
        </div>
        <div>
          <Text style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-secondary)', fontWeight: 'var(--font-weight-medium)', marginBottom: 6, display: 'block' }}>考核标准</Text>
          <TextArea
            value={quantItems[idx]?.scoring_rule}
            onChange={e => handleQuantItemChange(idx, 'scoring_rule', e.target.value)}
            rows={2}
            placeholder="定量按区间/上限设置"
          />
        </div>
      </div>
      <div style={{ marginTop: 12, borderTop: '1px solid var(--color-border-light)', paddingTop: 12 }}>
        <Text style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-secondary)', fontWeight: 'var(--font-weight-medium)', marginBottom: 6, display: 'block' }}>
          <PaperClipOutlined style={{ marginRight: 4 }} />附件
        </Text>
        <AttachmentUpload
          value={quantItems[idx]?.attachments || []}
          onChange={(urls) => handleQuantItemChange(idx, 'attachments', urls)}
          maxCount={5}
        />
      </div>
    </div>
  )

  const actionColumns = [
    {
      title: '重点计划',
      dataIndex: 'item_name',
      key: 'item_name',
      width: 180,
      render: (_: any, __: any, idx: number) => (
        <AutoComplete
          value={actionItems[idx]?.item_name}
          options={getSearchOptions(actionSearchResults[idx] || [])}
          onSearch={(val) => searchIndicators(val, setActionSearchResults, idx, 'key_action')}
          onChange={(val) => handleActionItemChange(idx, 'item_name', val)}
          onSelect={(val) => handleIndicatorSelect(val, idx, actionSearchResults[idx] || [], setActionItems, actionItems, false)}
          placeholder="输入关键词搜索指标"
          style={{ width: '100%' }}
        />
      )
    },
    {
      title: '指标定义及口径说明',
      dataIndex: 'item_definition',
      key: 'item_definition',
      width: '30%',
      render: (_: any, __: any, idx: number) => (
        <TextArea
          value={actionItems[idx]?.item_definition}
          onChange={e => handleActionItemChange(idx, 'item_definition', e.target.value)}
          rows={2}
          placeholder="明确行动范围和完成口径"
        />
      )
    },
    {
      title: '权重%',
      key: 'weight',
      width: 120,
      render: (_: any, __: any, idx: number) => (
        <InputNumber
          min={0}
          max={100}
          value={actionItems[idx]?.weight ? actionItems[idx].weight * 100 : 0}
          onChange={val => handleActionItemChange(idx, 'weight', (val || 0) / 100)}
          style={{ width: '100%' }}
          addonAfter="%"
        />
      )
    },
    {
      title: '定性目标',
      dataIndex: 'target_value',
      key: 'target_value',
      width: '30%',
      render: (_: any, __: any, idx: number) => (
        <TextArea
          value={actionItems[idx]?.target_value}
          onChange={e => handleActionItemChange(idx, 'target_value', e.target.value)}
          rows={3}
          placeholder="描述关键结果、交付物或完成标准"
        />
      )
    },
    {
      title: '附件',
      key: 'attachments',
      width: 120,
      render: (_: any, __: any, idx: number) => (
        <AttachmentUpload
          value={actionItems[idx]?.attachments || []}
          onChange={(urls) => handleActionItemChange(idx, 'attachments', urls)}
          maxCount={5}
        />
      )
    },
    {
      title: '',
      key: 'action',
      width: 48,
      render: (_: any, __: any, idx: number) => (
        <Button
          type="text"
          danger
          icon={<DeleteOutlined />}
          onClick={() => handleRemoveActionItem(idx)}
        />
      )
    }
  ]

  if (loading) return <div style={{ textAlign: 'center', padding: 100 }}><Spin size="large" /></div>

  return (
    <PageContainer noPadding title="目标设定" subtitle={participant ? (participant.employee_name || participant.employee_id) : undefined}>
      <div style={{
        position: 'sticky', top: 0, zIndex: 10,
        background: 'var(--color-bg-card)', borderBottom: '1px solid var(--color-border-light)',
        padding: '0 var(--page-padding)',
      }}>
        {/* Row 1: Title */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, paddingTop: 12 }}>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(-1)}>返回</Button>
          <Title level={4} style={{ margin: 0 }}>目标设定</Title>
          {participant && (
            <Text type="secondary">
              {participant.employee_name || participant.employee_id}
            </Text>
          )}
        </div>
        {/* Row 2: Weight + Actions */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', paddingTop: 8, paddingBottom: 12 }}>
          <Text style={{ fontSize: 'var(--font-size-sm)' }}>
            量化 <Text strong style={{ color: Math.abs(quantWeightTotal - 0.7) < 0.06 ? 'var(--color-success)' : 'var(--color-warning)' }}>{(quantWeightTotal * 100).toFixed(0)}%</Text>
            {' / '}
            关键 <Text strong style={{ color: Math.abs(actionWeightTotal - 0.3) < 0.06 ? 'var(--color-success)' : 'var(--color-warning)' }}>{(actionWeightTotal * 100).toFixed(0)}%</Text>
            {' / '}
            合计 <Text strong style={{ color: Math.abs(totalWeight - 1) < 0.001 ? 'var(--color-success)' : 'var(--color-error)', fontSize: 'var(--font-size-md)' }}>{(totalWeight * 100).toFixed(0)}%</Text>
            {Math.abs(totalWeight - 1) > 0.001 && (
              <Text type="danger" style={{ marginLeft: 4, fontSize: 'var(--font-size-xs)' }}>(需100%)</Text>
            )}
          </Text>
          <Space>
            <Button
              icon={<SaveOutlined />}
              loading={saving}
              onClick={handleSaveDraft}
            >
              保存草稿
            </Button>
            <Button
              type="primary"
              icon={<CheckCircleOutlined />}
              loading={submitting}
              onClick={handleSubmit}
              disabled={Math.abs(totalWeight - 1) > 0.001}
            >
              提交目标
            </Button>
          </Space>
        </div>
      </div>

      <PageCard title="量化指标">
        <Table
          dataSource={quantItems}
          columns={quantColumns}
          rowKey={(_, idx) => String(idx)}
          pagination={false}
          size="small"
          bordered
          expandable={{
            expandedRowRender: (_, idx) => quantExpandedRowRender(_, idx as number),
            rowExpandable: () => true,
            expandRowByClick: true,
          }}
        />
        <Button
          type="dashed"
          icon={<PlusOutlined />}
          onClick={handleAddQuantItem}
          style={{ marginTop: 12, width: '100%' }}
        >
          添加量化指标
        </Button>
      </PageCard>

      <PageCard
        title="关键行动"
        style={{ marginTop: 24 }}
      >
        <Table
          dataSource={actionItems}
          columns={actionColumns}
          rowKey={(_, idx) => String(idx)}
          pagination={false}
          size="small"
          bordered
        />
        <Button
          type="dashed"
          icon={<PlusOutlined />}
          onClick={handleAddActionItem}
          style={{ marginTop: 12, width: '100%' }}
        >
          添加关键行动
        </Button>
      </PageCard>

      <PageCard title={
        <Space>
          <BulbOutlined />
          <span>指标库建议</span>
        </Space>
      } style={{ marginTop: 24 }}>
        <Button type="primary" icon={<BulbOutlined />} onClick={loadSuggestions} style={{ marginBottom: showSuggestions ? 12 : 0 }}>
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
                  style={{ cursor: 'pointer', padding: '4px 8px' }}
                  onClick={() => applySuggestion(s)}
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
    </PageContainer>
  )
}

export default PerformanceGoalSetting
