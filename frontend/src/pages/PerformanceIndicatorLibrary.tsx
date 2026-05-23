import { useState, useEffect, useCallback } from 'react'
import {
  Table, Button, Modal, Form, Input, Select, Tag, Space, Card, message,
  Popconfirm, Empty, InputNumber, Row, Col
} from 'antd'
import { PlusOutlined, ArrowLeftOutlined, DeleteOutlined, DatabaseOutlined, AppstoreOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { performanceAPI, departmentAPI, type PerformanceIndicatorLibrary as ILibrary, type PerformanceIndicatorItem } from '../services/api'

const { TextArea } = Input

type DraftIndicatorItem = {
  name: string
  description: string
  weight: number
  red_line_value?: string
  target_value?: string
  challenge_value?: string
  scoring_rule?: string
}

type DepartmentOption = {
  department_id: string
  name: string
}

const isValidWeight = (weight: number) => Number.isFinite(weight) && weight >= 10 && weight % 5 === 0

const newQuantItem = (): DraftIndicatorItem => ({
  name: '',
  description: '',
  weight: 10,
  red_line_value: '',
  target_value: '',
  challenge_value: '',
  scoring_rule: '',
})

const newActionItem = (): DraftIndicatorItem => ({
  name: '',
  description: '',
  weight: 10,
  target_value: '',
})

export default function PerformanceIndicatorLibrary() {
  const navigate = useNavigate()
  const [libraries, setLibraries] = useState<ILibrary[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [selectedLib, setSelectedLib] = useState<ILibrary | null>(null)
  const [items, setItems] = useState<PerformanceIndicatorItem[]>([])
  const [itemsLoading, setItemsLoading] = useState(false)
  const [departments, setDepartments] = useState<DepartmentOption[]>([])
  const [departmentsLoading, setDepartmentsLoading] = useState(false)
  const [form] = Form.useForm()
  const [quantItems, setQuantItems] = useState<DraftIndicatorItem[]>([newQuantItem()])
  const [actionItems, setActionItems] = useState<DraftIndicatorItem[]>([newActionItem()])

  const fetchLibraries = useCallback(async () => {
    setLoading(true)
    try {
      const res: any = await performanceAPI.getIndicatorLibraries({ page, page_size: pageSize })
      const data = res.data || res
      setLibraries(data?.items || [])
      setTotal(data?.total || 0)
    } catch {
      message.error('获取指标库列表失败')
    } finally {
      setLoading(false)
    }
  }, [page, pageSize])

  useEffect(() => { fetchLibraries() }, [fetchLibraries])

  const fetchDepartments = useCallback(async () => {
    setDepartmentsLoading(true)
    try {
      const res: any = await departmentAPI.getDepartments()
      const data = res.data || res
      setDepartments(data?.departments || [])
    } catch {
      message.error('获取部门列表失败')
    } finally {
      setDepartmentsLoading(false)
    }
  }, [])

  useEffect(() => { fetchDepartments() }, [fetchDepartments])

  const resetCreateState = () => {
    form.resetFields()
    setQuantItems([newQuantItem()])
    setActionItems([newActionItem()])
  }

  const fetchItems = async (libraryId: number) => {
    setItemsLoading(true)
    try {
      const res: any = await performanceAPI.getIndicatorItems(libraryId)
      const data = res.data || res
      setItems(data?.items || [])
    } catch {
      message.error('获取指标项失败')
    } finally {
      setItemsLoading(false)
    }
  }

  const handleCreate = async (values: any) => {
    const totalWeight = [...quantItems, ...actionItems].reduce((sum, item) => sum + (item.weight || 0), 0)
    const quantWeight = quantItems.reduce((sum, item) => sum + (item.weight || 0), 0)
    const actionWeight = actionItems.reduce((sum, item) => sum + (item.weight || 0), 0)

    if ([...quantItems, ...actionItems].some(item => !isValidWeight(item.weight))) {
      message.error('权重必须为 5% 的倍数，且单项不低于 10%')
      return
    }
    if (Math.abs(totalWeight - 100) > 0.001) {
      message.error(`权重合计必须为 100%，当前为 ${totalWeight}%`)
      return
    }
    if (Math.abs(quantWeight - 70) > 0.001) {
      message.error(`量化指标权重必须为 70%，当前为 ${quantWeight}%`)
      return
    }
    if (Math.abs(actionWeight - 30) > 0.001) {
      message.error(`关键行动权重必须为 30%，当前为 ${actionWeight}%`)
      return
    }
    if (quantItems.some(item => !item.name.trim() || !item.description.trim())) {
      message.warning('请填写量化指标名称和指标定义及口径说明')
      return
    }
    if (actionItems.some(item => !item.name.trim() || !item.description.trim() || !item.target_value?.trim())) {
      message.warning('请填写关键行动名称、指标定义及口径说明和定性目标')
      return
    }
    if (quantItems.some(item =>
      !item.red_line_value?.trim() ||
      !item.target_value?.trim() ||
      !item.challenge_value?.trim() ||
      !item.scoring_rule?.trim()
    )) {
      message.warning('请填写量化指标的红线值、目标值、挑战值和考核标准')
      return
    }

    const payload = {
      ...values,
      items: [
        ...quantItems.map((item, idx) => ({
          section_type: 'quantitative',
          name: item.name,
          description: item.description,
          weight: item.weight,
          red_line_value: item.red_line_value,
          target_value: item.target_value,
          challenge_value: item.challenge_value,
          scoring_rule: item.scoring_rule,
          is_default: true,
          sort_order: idx + 1,
        })),
        ...actionItems.map((item, idx) => ({
          section_type: 'key_action',
          name: item.name,
          description: item.description,
          weight: item.weight,
          target_value: item.target_value,
          is_default: true,
          sort_order: quantItems.length + idx + 1,
        })),
      ],
    }

    setCreating(true)
    try {
      const res: any = await performanceAPI.createIndicatorLibrary(payload)
      const data = res.data || res
      const library = data?.library
      message.success('创建成功')
      setCreateOpen(false)
      resetCreateState()
      await fetchLibraries()
      if (library?.id) {
        setSelectedLib(library)
        fetchItems(library.id)
      }
    } catch (err: any) {
      message.error(err.response?.data?.message || '创建失败')
    } finally {
      setCreating(false)
    }
  }

  const handleArchive = async (id: number) => {
    try {
      await performanceAPI.archiveIndicatorLibrary(id)
      message.success('归档成功')
      fetchLibraries()
      if (selectedLib?.id === id) setSelectedLib(null)
    } catch {
      message.error('归档失败')
    }
  }

  const handleSelectLib = (record: ILibrary) => {
    setSelectedLib(record)
    fetchItems(record.id)
  }

  const updateQuantItem = (index: number, patch: Partial<DraftIndicatorItem>) => {
    setQuantItems(prev => prev.map((item, idx) => idx === index ? { ...item, ...patch } : item))
  }

  const updateActionItem = (index: number, patch: Partial<DraftIndicatorItem>) => {
    setActionItems(prev => prev.map((item, idx) => idx === index ? { ...item, ...patch } : item))
  }

  const handleDepartmentChange = (departmentId: string) => {
    const department = departments.find(item => item.department_id === departmentId)
    form.setFieldsValue({
      department_id: departmentId,
      department_name: department?.name || '',
    })
  }

  const columns = [
    {
      title: '指标库名称', dataIndex: 'name', key: 'name',
      render: (text: string, record: ILibrary) => (
        <a
          onClick={() => handleSelectLib(record)}
          style={{ fontWeight: 600, color: selectedLib?.id === record.id ? '#4338ca' : '#1e1b4b', fontSize: 14 }}
        >
          {text}
        </a>
      ),
    },
    { title: '所属部门', dataIndex: 'department_name', key: 'department_name' },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (status: string) => (
        <Tag
          color={status === 'active' ? 'success' : 'default'}
          style={{ borderRadius: 6, fontWeight: 600, margin: 0, fontSize: 12.5 }}
        >
          {status === 'active' ? '启用' : '已归档'}
        </Tag>
      )
    },
    {
      title: '来源', key: 'source', width: 80,
      render: (_: any, record: ILibrary) => (
        <Tag style={{ borderRadius: 6, margin: 0, background: record.parent_library_id ? '#fff7e6' : '#f0fdf4', color: record.parent_library_id ? '#d48806' : '#15803d', border: 'none', fontWeight: 600, fontSize: 12.5 }}>
          {record.parent_library_id ? '继承' : '自建'}
        </Tag>
      )
    },
    {
      title: '操作', key: 'action', width: 120,
      render: (_: any, record: ILibrary) => (
        <Space size={14}>
          <a onClick={() => handleSelectLib(record)} style={{ fontWeight: 600, color: '#4338ca' }}>查看</a>
          {record.status === 'active' && (
            <Popconfirm title="确认归档该指标库？" onConfirm={() => handleArchive(record.id)}>
              <a style={{ color: '#dc2626', fontWeight: 600 }}>归档</a>
            </Popconfirm>
          )}
        </Space>
      )
    }
  ]

  const itemColumns = [
    { title: '指标名称/重点计划', dataIndex: 'name', key: 'name' },
    {
      title: '类别',
      key: 'section_type',
      width: 90,
      render: (_: any, record: PerformanceIndicatorItem) => (
        <span>{
          ({
            quantitative: '量化指标',
            key_action: '关键行动',
            bonus_penalty: '附加项',
          } as Record<string, string>)[record.section_type || record.indicator_type] || record.section_type || record.indicator_type || '-'
        }</span>
      )
    },
    {
      title: '权重',
      key: 'weight',
      width: 80,
      render: (_: any, record: PerformanceIndicatorItem) => {
        const value = record.weight ?? record.default_weight ?? 0
        return `${value}%`
      }
    },
    {
      title: '目标',
      key: 'target',
      render: (_: any, record: PerformanceIndicatorItem) => {
        if (record.section_type === 'quantitative') {
          return [record.red_line_value, record.target_value, record.challenge_value].filter(Boolean).join(' / ') || '-'
        }
        return record.target_value || '-'
      }
    },
  ]

  return (
    <div style={{ padding: '20px 28px', background: '#e4e8ee', minHeight: '100vh' }}>
      <Button
        type="text"
        icon={<ArrowLeftOutlined />}
        onClick={() => navigate(-1)}
        style={{
          paddingLeft: 0,
          color: '#6b7280',
          fontWeight: 500,
          marginBottom: 4,
          fontSize: 14,
        }}
      >
        返回
      </Button>
      <div style={{ marginBottom: 20 }}>
        <h2 style={{
          margin: '4px 0 6px',
          fontSize: 24,
          fontWeight: 700,
          color: '#111827',
          letterSpacing: 0.5,
        }}>
          <DatabaseOutlined style={{ marginRight: 10, color: '#4338ca' }} />
          指标库管理
        </h2>
        <p style={{ color: '#6b7280', marginBottom: 0, fontSize: 14 }}>
          创建时一次性配置指标项，创建后的指标库仅支持查看与继承
        </p>
      </div>

      <div style={{
        background: 'linear-gradient(135deg, #eef2ff 0%, #e0e7ff 100%)',
        border: '1px solid #c7d2fe',
        borderRadius: 12,
        padding: '16px 20px',
        marginBottom: 20,
        display: 'flex',
        alignItems: 'center',
        gap: 14,
      }}>
        <div style={{
          width: 40,
          height: 40,
          borderRadius: 10,
          background: 'linear-gradient(135deg, #4338ca, #6366f1)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          flexShrink: 0,
          boxShadow: '0 2px 8px rgba(99,102,241,0.3)',
        }}>
          <AppstoreOutlined style={{ color: '#fff', fontSize: 18 }} />
        </div>
        <div>
          <strong style={{ color: '#1e1b4b', fontSize: 14 }}>创建规则</strong>
          <p style={{ margin: '2px 0 0', color: '#4338ca', fontSize: 13.5, fontWeight: 500 }}>
            量化指标权重合计 70% ，关键行动权重合计 30% ，总权重必须为 100% 。
          </p>
        </div>
      </div>

      <div style={{ display: 'flex', gap: 20 }}>
        <Card
          title={
            <span style={{ fontWeight: 700, fontSize: 15, color: '#111827' }}>指标库列表</span>
          }
          extra={
            <Space>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}
                style={{ borderRadius: 8, fontWeight: 600, height: 36, boxShadow: '0 2px 6px rgba(67,56,202,0.3)' }}
              >
                创建
              </Button>
              <Button style={{ borderRadius: 8, height: 36 }}>继承</Button>
            </Space>
          }
          style={{
            flex: 1,
            borderRadius: 14,
            boxShadow: '0 2px 12px rgba(0,0,0,0.08)',
            border: '1px solid #e5e7eb',
          }}
          styles={{ header: { borderBottom: '1px solid #f0f0f0', background: '#fafbfc' } }}
        >
          <Table
            dataSource={libraries}
            columns={columns}
            rowKey="id"
            loading={loading}
            pagination={{ current: page, pageSize, total, onChange: (p, ps) => { setPage(p); setPageSize(ps) }, showSizeChanger: false }}
            locale={{ emptyText: <Empty description="暂无指标库" imageStyle={{ height: 60 }} /> }}
          />
        </Card>

        <Card
          title={
            <span style={{ fontWeight: 700, fontSize: 15, color: '#111827' }}>指标项</span>
          }
          style={{
            width: 520,
            borderRadius: 14,
            boxShadow: '0 2px 12px rgba(0,0,0,0.08)',
            border: '1px solid #e5e7eb',
          }}
          styles={{ header: { borderBottom: '1px solid #f0f0f0', background: '#fafbfc' } }}
        >
          {selectedLib ? (
            <div>
              <div style={{
                background: 'linear-gradient(135deg, #eef2ff, #e0e7ff)',
                borderRadius: 10,
                padding: '12px 16px',
                marginBottom: 16,
                border: '1px solid #c7d2fe',
              }}>
                <span style={{ color: '#6366f1', fontSize: 12, fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.5 }}>当前查看</span>
                <div style={{ marginTop: 4, display: 'flex', alignItems: 'center', gap: 8 }}>
                  <strong style={{ color: '#1e1b4b', fontSize: 15 }}>{selectedLib.name}</strong>
                  <Tag color="blue" style={{ borderRadius: 6, fontWeight: 500, margin: 0 }}>{selectedLib.department_name}</Tag>
                </div>
              </div>
              <Table
                dataSource={items}
                columns={itemColumns}
                rowKey="id"
                loading={itemsLoading}
                pagination={false}
                size="small"
                locale={{
                  emptyText: selectedLib
                    ? '该指标库当前没有指标项，请先创建或从其他库继承'
                    : '请先选择一个指标库',
                }}
              />
            </div>
          ) : (
            <Empty description="请先选择一个指标库" imageStyle={{ height: 80 }} />
          )}
        </Card>
      </div>

      <Modal
        title={
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <div style={{
              width: 36,
              height: 36,
              borderRadius: 10,
              background: 'linear-gradient(135deg, #4338ca, #6366f1)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              boxShadow: '0 2px 8px rgba(99,102,241,0.35)',
            }}>
              <PlusOutlined style={{ color: '#fff', fontSize: 16 }} />
            </div>
            <span style={{ fontWeight: 700, fontSize: 17, color: '#111827' }}>创建指标库</span>
          </div>
        }
        open={createOpen}
        onCancel={() => { setCreateOpen(false); resetCreateState() }}
        onOk={() => form.submit()}
        confirmLoading={creating}
        width={1280}
        styles={{
          header: {
            background: 'linear-gradient(180deg, #f8f9ff 0%, #fff 100%)',
            borderBottom: '2px solid #eef2ff',
            paddingBottom: 16,
            marginBottom: 0,
            borderRadius: '14px 14px 0 0',
          },
          body: { paddingTop: 24 },
          mask: { backdropFilter: 'blur(4px)' },
        }}
      >
        <Form form={form} layout="vertical" onFinish={handleCreate} style={{ marginTop: 4 }}>
          <div style={{
            background: '#f8f9fc',
            borderRadius: 12,
            padding: '22px 22px 10px',
            marginBottom: 22,
            border: '1px solid #e2e5f0',
            boxShadow: 'inset 0 1px 3px rgba(0,0,0,0.03)',
          }}>
            <div style={{ marginBottom: 14, display: 'flex', alignItems: 'center', gap: 8 }}>
              <div style={{ width: 4, height: 18, borderRadius: 2, background: 'linear-gradient(180deg, #4338ca, #6366f1)' }} />
              <span style={{ fontWeight: 700, fontSize: 14, color: '#1e1b4b' }}>基本信息</span>
            </div>
            <Row gutter={20}>
              <Col span={8}>
                <Form.Item name="department_id" label={<span style={{ fontWeight: 600, color: '#374151' }}>所属部门</span>} rules={[{ required: true, message: '请选择所属部门' }]}>
                  <Select
                    showSearch
                    placeholder="请选择所属部门"
                    loading={departmentsLoading}
                    optionFilterProp="label"
                    options={departments.map(item => ({
                      label: item.name,
                      value: item.department_id,
                    }))}
                    onChange={handleDepartmentChange}
                    style={{ borderRadius: 8 }}
                  />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item name="department_name" hidden>
                  <Input />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item name="name" label={<span style={{ fontWeight: 600, color: '#374151' }}>指标库名称</span>} rules={[{ required: true, message: '请输入指标库名称' }]}>
                  <Input style={{ borderRadius: 8 }} />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={20}>
              <Col span={16}>
                <Form.Item name="description" label={<span style={{ fontWeight: 600, color: '#374151' }}>描述</span>}>
                  <TextArea rows={2} style={{ borderRadius: 8 }} />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item name="default_cycle" label={<span style={{ fontWeight: 600, color: '#374151' }}>默认周期</span>} initialValue="monthly">
                  <Select
                    options={[{ value: 'monthly', label: '月度' }, { value: 'quarterly', label: '季度' }, { value: 'annual', label: '年度' }]}
                    style={{ borderRadius: 8 }}
                  />
                </Form.Item>
              </Col>
            </Row>
          </div>
        </Form>

        <Card
          size="small"
          title={
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <div style={{ width: 4, height: 18, borderRadius: 2, background: 'linear-gradient(180deg, #4338ca, #6366f1)' }} />
              <span style={{ fontWeight: 700, fontSize: 14, color: '#1e1b4b' }}>量化指标</span>
              <span style={{
                background: 'linear-gradient(135deg, #eef2ff, #dbeafe)',
                color: '#4338ca',
                fontSize: 12,
                fontWeight: 700,
                padding: '4px 12px',
                borderRadius: 14,
                border: '1px solid #c7d2fe',
                letterSpacing: 0.3,
              }}>
                权重合计 70%
              </span>
            </div>
          }
          style={{
            marginTop: 10,
            borderRadius: 12,
            border: '1px solid #e0e3ed',
            boxShadow: '0 1px 6px rgba(0,0,0,0.05)',
          }}
          styles={{ header: { background: '#f5f6fb', borderBottom: '1px solid #eef0f6' } }}
        >
          <Table
            dataSource={quantItems}
            rowKey={(_, idx) => `q-${idx}`}
            pagination={false}
            size="small"
            bordered
            columns={[
              {
                title: '指标名称',
                width: 150,
                render: (_: any, __: any, idx: number) => (
                  <Input value={quantItems[idx].name} onChange={e => updateQuantItem(idx, { name: e.target.value })} />
                )
              },
              {
                title: '指标定义及口径说明',
                width: 260,
                render: (_: any, __: any, idx: number) => (
                  <TextArea rows={2} value={quantItems[idx].description} onChange={e => updateQuantItem(idx, { description: e.target.value })} placeholder="明确指标范围和计算公式" />
                )
              },
              {
                title: '权重',
                width: 90,
                render: (_: any, __: any, idx: number) => (
                  <InputNumber min={10} max={100} step={5} value={quantItems[idx].weight} onChange={value => updateQuantItem(idx, { weight: value || 0 })} addonAfter="%" style={{ width: '100%' }} />
                )
              },
              {
                title: '红线值',
                width: 110,
                render: (_: any, __: any, idx: number) => (
                  <Input value={quantItems[idx].red_line_value} onChange={e => updateQuantItem(idx, { red_line_value: e.target.value })} />
                )
              },
              {
                title: '目标值',
                width: 110,
                render: (_: any, __: any, idx: number) => (
                  <Input value={quantItems[idx].target_value} onChange={e => updateQuantItem(idx, { target_value: e.target.value })} />
                )
              },
              {
                title: '挑战值',
                width: 110,
                render: (_: any, __: any, idx: number) => (
                  <Input value={quantItems[idx].challenge_value} onChange={e => updateQuantItem(idx, { challenge_value: e.target.value })} />
                )
              },
              {
                title: '考核标准',
                width: 240,
                render: (_: any, __: any, idx: number) => (
                  <TextArea rows={2} value={quantItems[idx].scoring_rule} onChange={e => updateQuantItem(idx, { scoring_rule: e.target.value })} placeholder="定量按区间/上限设置" />
                )
              },
              {
                title: '',
                width: 48,
                render: (_: any, __: any, idx: number) => (
                  <Button type="text" danger icon={<DeleteOutlined />} disabled={quantItems.length <= 1} onClick={() => setQuantItems(prev => prev.filter((_, i) => i !== idx))} />
                )
              }
            ]}
          />
          <Button
            type="dashed"
            icon={<PlusOutlined />}
            onClick={() => setQuantItems(prev => [...prev, newQuantItem()])}
            style={{
              marginTop: 12,
              width: '100%',
              borderRadius: 8,
              height: 40,
              fontWeight: 600,
              color: '#4338ca',
              borderColor: '#a5b4fc',
              fontSize: 13.5,
            }}
          >
            添加量化指标
          </Button>
        </Card>

        <Card
          size="small"
          title={
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <div style={{ width: 4, height: 18, borderRadius: 2, background: 'linear-gradient(180deg, #15803d, #22c55e)' }} />
              <span style={{ fontWeight: 700, fontSize: 14, color: '#14532d' }}>关键行动</span>
              <span style={{
                background: 'linear-gradient(135deg, #f0fdf4, #dcfce7)',
                color: '#15803d',
                fontSize: 12,
                fontWeight: 700,
                padding: '4px 12px',
                borderRadius: 14,
                border: '1px solid #bbf7d0',
                letterSpacing: 0.3,
              }}>
                权重合计 30%
              </span>
            </div>
          }
          style={{
            marginTop: 16,
            borderRadius: 12,
            border: '1px solid #e0e3ed',
            boxShadow: '0 1px 6px rgba(0,0,0,0.05)',
          }}
          styles={{ header: { background: '#f5f6fb', borderBottom: '1px solid #eef0f6' } }}
        >
          <Table
            dataSource={actionItems}
            rowKey={(_, idx) => `a-${idx}`}
            pagination={false}
            size="small"
            bordered
            columns={[
              {
                title: '重点计划',
                width: 180,
                render: (_: any, __: any, idx: number) => (
                  <Input value={actionItems[idx].name} onChange={e => updateActionItem(idx, { name: e.target.value })} />
                )
              },
              {
                title: '指标定义及口径说明',
                width: 320,
                render: (_: any, __: any, idx: number) => (
                  <TextArea rows={2} value={actionItems[idx].description} onChange={e => updateActionItem(idx, { description: e.target.value })} placeholder="明确行动范围和完成口径" />
                )
              },
              {
                title: '权重',
                width: 100,
                render: (_: any, __: any, idx: number) => (
                  <InputNumber min={10} max={100} step={5} value={actionItems[idx].weight} onChange={value => updateActionItem(idx, { weight: value || 0 })} addonAfter="%" style={{ width: '100%' }} />
                )
              },
              {
                title: '定性目标',
                render: (_: any, __: any, idx: number) => (
                  <TextArea rows={3} value={actionItems[idx].target_value} onChange={e => updateActionItem(idx, { target_value: e.target.value })} placeholder="描述关键结果、交付物或完成标准" />
                )
              },
              {
                title: '',
                width: 48,
                render: (_: any, __: any, idx: number) => (
                  <Button type="text" danger icon={<DeleteOutlined />} disabled={actionItems.length <= 1} onClick={() => setActionItems(prev => prev.filter((_, i) => i !== idx))} />
                )
              }
            ]}
          />
          <Button
            type="dashed"
            icon={<PlusOutlined />}
            onClick={() => setActionItems(prev => [...prev, newActionItem()])}
            style={{
              marginTop: 12,
              width: '100%',
              borderRadius: 8,
              height: 40,
              fontWeight: 600,
              color: '#15803d',
              borderColor: '#86efac',
              fontSize: 13.5,
            }}
          >
            添加关键行动
          </Button>
        </Card>
      </Modal>
    </div>
  )
}
