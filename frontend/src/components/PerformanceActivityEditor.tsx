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
import { CloseOutlined, SaveOutlined } from '@ant-design/icons'

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
  padding: '20px 24px 8px',
  borderBottom: '1px solid #eef0f4',
  scrollMarginTop: 104,
}

const sectionTitleStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: 12,
  marginBottom: 16,
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
        marginBottom: 16,
        overflow: 'hidden',
        background: '#fff',
        border: '1px solid #dbe3f0',
        borderRadius: 8,
        boxShadow: '0 2px 10px rgba(15, 23, 42, 0.06)',
      }}
    >
      <div
        style={{
          position: 'sticky',
          top: 0,
          zIndex: 4,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 12,
          padding: '14px 20px',
          borderBottom: '1px solid #e5e7eb',
          background: 'rgba(248, 250, 252, 0.98)',
          backdropFilter: 'blur(8px)',
        }}
      >
        <Space size={12} wrap>
          <Text strong style={{ fontSize: 16 }}>{editing ? '编辑活动' : '新建活动'}</Text>
          <Progress percent={progress} size="small" showInfo={false} style={{ width: 150 }} />
          <Text type="secondary" style={{ fontSize: 13 }}>{doneCount}/{requiredChecks.length}</Text>
        </Space>
        <Space>
          <Button icon={<CloseOutlined />} onClick={onCancel} disabled={saving}>
            取消
          </Button>
          <Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={onSave}>
            {editing ? '保存修改' : '保存活动'}
          </Button>
        </Space>
      </div>

      <div
        style={{
          display: 'flex',
          gap: 8,
          padding: '10px 20px',
          overflowX: 'auto',
          borderBottom: '1px solid #eef0f4',
          background: '#fff',
        }}
      >
        {activitySections.map(section => {
          const check = getRequiredCheck(section.id)
          return (
            <Button
              key={section.id}
              size="small"
              type="text"
              onClick={() => scrollToSection(section.id)}
              style={{
                flex: '0 0 auto',
                height: 30,
                paddingInline: 10,
                background: check?.done ? '#f0fdf4' : undefined,
                color: check?.done ? '#166534' : undefined,
              }}
            >
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
                  <Form.Item name="indicator_library_id" label="关联指标库">
                    <Select
                      placeholder="请选择指标库（可选）"
                      allowClear
                      loading={indicatorLibrariesLoading}
                      options={indicatorLibraries.map(lib => ({ value: lib.id, label: lib.name }))}
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

            <section id="activity-advanced-section" style={{ ...sectionStyle, borderBottom: 'none' }}>
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
