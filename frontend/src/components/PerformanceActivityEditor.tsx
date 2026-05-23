import React from 'react'
import {
  Button,
  Col,
  DatePicker,
  Form,
  Input,
  Progress,
  Row,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
} from 'antd'
import type { FormInstance } from 'antd/es/form'
import { SaveOutlined } from '@ant-design/icons'

const { Text } = Typography
const { TextArea } = Input
const { RangePicker } = DatePicker

type SelectOption = {
  label: React.ReactNode
  value: string | number
}

interface PerformanceActivityEditorProps {
  visible: boolean
  editing: boolean
  form: FormInstance
  saving?: boolean
  indicatorLibraries: any[]
  indicatorLibrariesLoading: boolean
  departmentOptions: SelectOption[]
  userOptions: SelectOption[]
  scopeOptionsLoading: boolean
  onSave: () => void
  onCancel: () => void
}

const activitySections = [
  { id: 'activity-basic-section', label: '基础信息' },
  { id: 'activity-period-section', label: '周期设置' },
  { id: 'activity-review-section', label: '评审流程' },
  { id: 'activity-scope-section', label: '参与范围' },
  { id: 'activity-advanced-section', label: '高级设置' },
]

function isRangeFilled(value: unknown) {
  return Array.isArray(value) && Boolean(value[0] && value[1])
}

function scrollToSection(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

const sectionStyle: React.CSSProperties = {
  padding: '24px 28px 12px',
  borderBottom: '1px solid #f0f0f0',
  scrollMarginTop: 110,
  background: '#fff',
}

const sectionTitleStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 10,
  marginBottom: 20,
}

const cycleLabels: Record<string, string> = {
  monthly: '月度',
  quarterly: '季度',
  annual: '年度',
}

function normalizeCycleType(value?: string) {
  return String(value || '').trim()
}

function getCycleLabel(value?: string) {
  const normalized = normalizeCycleType(value)
  return cycleLabels[normalized] || normalized || '未知周期'
}

const PerformanceActivityEditor: React.FC<PerformanceActivityEditorProps> = ({
  visible,
  editing,
  form,
  saving = false,
  indicatorLibraries,
  indicatorLibrariesLoading,
  departmentOptions,
  userOptions,
  scopeOptionsLoading,
  onSave,
  onCancel,
}) => {
  const [, forceFormRerender] = React.useState(0)
  const values = form.getFieldsValue(true)
  const cycleType = Form.useWatch('cycle_type', form) as string | undefined
  const selectedIndicatorLibraryId = Form.useWatch('indicator_library_id', form) as number | string | undefined
  const normalizedCycleType = normalizeCycleType(cycleType)
  const selectedIndicatorLibraryIdKey = selectedIndicatorLibraryId == null ? '' : String(selectedIndicatorLibraryId)
  const selectedIndicatorLibrary = React.useMemo(
    () => indicatorLibraries.find(lib => String(lib.id) === selectedIndicatorLibraryIdKey) || null,
    [indicatorLibraries, selectedIndicatorLibraryIdKey],
  )
  const indicatorLibraryCycleMismatch = Boolean(
    normalizedCycleType
      && selectedIndicatorLibrary
      && normalizeCycleType(selectedIndicatorLibrary.default_cycle) !== normalizedCycleType,
  )
  const visibleIndicatorLibraries = React.useMemo(() => {
    const cycleFilteredLibraries = indicatorLibraries.filter(lib => {
      if (!normalizedCycleType) return true
      return normalizeCycleType(lib.default_cycle) === normalizedCycleType
    })

    if (!selectedIndicatorLibrary || !indicatorLibraryCycleMismatch) {
      return cycleFilteredLibraries
    }

    if (cycleFilteredLibraries.some(lib => String(lib.id) === String(selectedIndicatorLibrary.id))) {
      return cycleFilteredLibraries
    }

    return [...cycleFilteredLibraries, selectedIndicatorLibrary]
  }, [indicatorLibraries, normalizedCycleType, indicatorLibraryCycleMismatch, selectedIndicatorLibrary])
  const requiredChecks = [
    { id: 'activity-basic-section', label: '基础信息', done: Boolean(values.name && values.cycle_type) },
    { id: 'activity-period-section', label: '周期设置', done: isRangeFilled(values.date_range) },
    {
      id: 'activity-review-section',
      label: '评审流程',
      done: isRangeFilled(values.self_eval_range)
        && isRangeFilled(values.manager_eval_range)
        && isRangeFilled(values.result_confirm_range),
    },
  ]
  const doneCount = requiredChecks.filter(item => item.done).length
  const progress = Math.round((doneCount / requiredChecks.length) * 100)
  const getRequiredCheck = (sectionId: string) => requiredChecks.find(item => item.id === sectionId)

  if (!visible) return null

  return (
    <div
      id="performance-activity-editor"
      style={{
        background: '#fff',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 12,
          padding: '14px 24px',
          borderBottom: '1px solid #e5e7eb',
          background: '#f8fafc',
        }}
      >
        <Space size={16} align="center">
          <Progress percent={progress} size="small" showInfo={false} style={{ width: 120 }} />
          <Text type="secondary" style={{ fontSize: 13, background: '#e2e8f0', padding: '2px 10px', borderRadius: 10 }}>
            {doneCount}/{requiredChecks.length} 必填项已完成
          </Text>
        </Space>
        <Space>
          <Button onClick={onCancel} disabled={saving}>
            取消
          </Button>
          <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={onSave} style={{ background: '#4338ca', borderColor: '#4338ca' }}>
            {editing ? '保存修改' : '保存活动'}
          </Button>
        </Space>
      </div>

      <div
        style={{
          display: 'flex',
          gap: 8,
          padding: '12px 24px',
          overflowX: 'auto',
          borderBottom: '1px solid #e5e7eb',
          background: '#f8fafc',
        }}
      >
        {activitySections.map((section, idx) => {
          const check = getRequiredCheck(section.id)
          return (
            <Button
              key={section.id}
              size="small"
              type="text"
              onClick={() => scrollToSection(section.id)}
              style={{
                flex: '0 0 auto',
                height: 32,
                paddingInline: 14,
                borderRadius: 6,
                background: check?.done ? '#ecfdf5' : undefined,
                color: check?.done ? '#047857' : '#4b5563',
                fontWeight: 500,
                fontSize: 13,
                transition: 'all 0.2s',
              }}
            >
              <span style={{ marginRight: 6, color: '#9ca3af', fontSize: 12 }}>{idx + 1}.</span>
              {section.label}
            </Button>
          )
        })}
      </div>

      <Form form={form} layout="vertical" onValuesChange={() => forceFormRerender(version => version + 1)}>
            <section id="activity-basic-section" style={sectionStyle}>
              <div style={sectionTitleStyle}>
                <Text strong style={{ fontSize: 15 }}>基础信息</Text>
                <Tag color={requiredChecks[0].done ? 'success' : 'warning'} style={{ marginInlineEnd: 0 }}>
                  必填
                </Tag>
              </div>
              <Row gutter={16}>
                <Col xs={24} md={8}>
                  <Form.Item name="name" label="活动名称" rules={[{ required: true, message: '请输入活动名称' }]}>
                    <Input placeholder="如：2026 Q2 绩效评估" />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item name="cycle_type" label="周期类型" rules={[{ required: true, message: '请选择周期类型' }]}>
                    <Select
                      placeholder="选择周期类型"
                      options={[
                        { value: 'monthly', label: '月度' },
                        { value: 'quarterly', label: '季度' },
                        { value: 'annual', label: '年度' },
                      ]}
                    />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item
                    name="indicator_library_id"
                    label="关联指标库"
                    rules={[
                      {
                        validator: (_, value) => {
                          if (!value || !normalizedCycleType) return Promise.resolve()

                          const library = indicatorLibraries.find(lib => String(lib.id) === String(value))
                          if (!library) return Promise.resolve()

                          const libraryCycle = normalizeCycleType(library.default_cycle)
                          if (libraryCycle === normalizedCycleType) return Promise.resolve()

                          return Promise.reject(new Error(`请选择${getCycleLabel(normalizedCycleType)}指标库`))
                        },
                      },
                    ]}
                    extra={
                      normalizedCycleType
                        ? indicatorLibraryCycleMismatch && selectedIndicatorLibrary
                          ? (
                            <Text type="warning">
                              当前已选指标库默认周期为 {getCycleLabel(selectedIndicatorLibrary.default_cycle)}，与活动周期 {getCycleLabel(normalizedCycleType)} 不一致，请更换。
                            </Text>
                          )
                          : (
                            <Text type="secondary">
                              仅显示默认周期为 {getCycleLabel(normalizedCycleType)} 的指标库。
                            </Text>
                          )
                        : (
                          <Text type="secondary">
                            请先选择周期类型，指标库会按周期自动过滤。
                          </Text>
                        )
                    }
                  >
                    <Select
                      placeholder={normalizedCycleType ? `请选择${getCycleLabel(normalizedCycleType)}指标库（可选）` : '请先选择周期类型'}
                      allowClear
                      showSearch
                      disabled={!normalizedCycleType}
                      loading={indicatorLibrariesLoading}
                      optionFilterProp="label"
                      options={visibleIndicatorLibraries.map(lib => ({
                        value: lib.id,
                        label: `${lib.name}${lib.default_cycle ? `（${getCycleLabel(lib.default_cycle)}）` : ''}`,
                      }))}
                    />
                  </Form.Item>
                </Col>
              </Row>
            </section>

            <section id="activity-period-section" style={sectionStyle}>
              <div style={sectionTitleStyle}>
                <Text strong style={{ fontSize: 15 }}>周期设置</Text>
                <Tag color={requiredChecks[1].done ? 'success' : 'warning'} style={{ marginInlineEnd: 0 }}>
                  必填
                </Tag>
              </div>
              <Row gutter={16}>
                <Col xs={24} md={12}>
                  <Form.Item name="date_range" label="绩效周期" rules={[{ required: true, message: '请选择绩效周期' }]}>
                    <RangePicker placeholder={['开始日期', '结束日期']} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item name="target_set_range" label="目标设定时间">
                    <RangePicker placeholder={['开始日期', '结束日期']} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>
            </section>

            <section id="activity-review-section" style={sectionStyle}>
              <div style={sectionTitleStyle}>
                <Text strong style={{ fontSize: 15 }}>评审流程</Text>
                <Tag color={requiredChecks[2].done ? 'success' : 'warning'} style={{ marginInlineEnd: 0 }}>
                  必填
                </Tag>
              </div>
              <Row gutter={16}>
                <Col xs={24} lg={8}>
                  <Form.Item name="self_eval_range" label="自评时间" rules={[{ required: true, message: '请选择自评时间' }]}>
                    <RangePicker placeholder={['开始日期', '结束日期']} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} lg={8}>
                  <Form.Item name="manager_eval_range" label="主管评分时间" rules={[{ required: true, message: '请选择主管评分时间' }]}>
                    <RangePicker placeholder={['开始日期', '结束日期']} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} lg={8}>
                  <Form.Item name="result_confirm_range" label="结果确认时间" rules={[{ required: true, message: '请选择结果确认时间' }]}>
                    <RangePicker placeholder={['开始日期', '结束日期']} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>
            </section>

            <section id="activity-scope-section" style={sectionStyle}>
              <div style={sectionTitleStyle}>
                <Text strong style={{ fontSize: 15 }}>参与范围</Text>
                <Tag style={{ marginInlineEnd: 0 }}>可选</Tag>
              </div>
              <Row gutter={16}>
                <Col xs={24} md={12}>
                  <Form.Item name="target_department_ids" label="参与部门">
                    <Select
                      mode="multiple"
                      allowClear
                      showSearch
                      loading={scopeOptionsLoading}
                      optionFilterProp="label"
                      placeholder="请选择参与部门"
                      options={departmentOptions}
                    />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item name="target_employee_ids" label="指定员工">
                    <Select
                      mode="multiple"
                      allowClear
                      showSearch
                      loading={scopeOptionsLoading}
                      optionFilterProp="label"
                      placeholder="请选择指定员工"
                      options={userOptions}
                    />
                  </Form.Item>
                </Col>
              </Row>
            </section>

            <section id="activity-advanced-section" style={{ ...sectionStyle, borderBottom: 'none', paddingBottom: 24 }}>
              <div style={sectionTitleStyle}>
                <Text strong style={{ fontSize: 15 }}>高级设置</Text>
                <Tag style={{ marginInlineEnd: 0 }}>可选</Tag>
              </div>
              <Row gutter={16}>
                <Col xs={24} md={12}>
                  <Form.Item
                    name="enable_bonus_score"
                    label="启用附加分"
                    valuePropName="checked"
                    extra="启用后员工和主管须评估附加考核项，附加分将计入总分并影响绩效等级"
                  >
                    <Switch checkedChildren="启用" unCheckedChildren="关闭" />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item name="hr_confirm_deadline" label="HR确认截止日">
                    <DatePicker placeholder="请选择截止日" style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} lg={8}>
                  <Form.Item name="employee_confirm_range" label="员工确认时间">
                    <RangePicker placeholder={['开始日期', '结束日期']} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} lg={8}>
                  <Form.Item name="manager_confirm_range" label="主管确认时间">
                    <RangePicker placeholder={['开始日期', '结束日期']} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24} lg={8}>
                  <Form.Item name="hr_confirm_range" label="HR确认时间">
                    <RangePicker placeholder={['开始日期', '结束日期']} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col xs={24}>
                  <Form.Item name="description" label="描述">
                    <TextArea rows={3} placeholder="补充活动说明" />
                  </Form.Item>
                </Col>
              </Row>
            </section>
      </Form>
    </div>
  )
}

export default PerformanceActivityEditor
