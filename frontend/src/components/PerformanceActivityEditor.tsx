import React from 'react'
import {
  Button,
  Col,
  DatePicker,
  Form,
  Input,
  Progress,
  Row,
  Segmented,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
  Upload,
} from 'antd'
import type { FormInstance } from 'antd/es/form'
import { CloseOutlined, SaveOutlined, UploadOutlined } from '@ant-design/icons'

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
  performanceTemplates?: any[]
  performanceTemplatesLoading?: boolean
  activities?: any[]
  currentActivityId?: number | string
  indicatorLibraries: any[]
  indicatorLibrariesLoading: boolean
  departmentOptions: SelectOption[]
  userOptions: SelectOption[]
  scopeOptionsLoading: boolean
  importingParticipants?: boolean
  previousActivityOptions?: SelectOption[]
  previousActivityLoading?: boolean
  onImportParticipants: (file: File) => Promise<void>
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
  semiannual: '半年度',
  annual: '年度',
  probation: '试用期',
}

const flowTypeLabels: Record<string, string> = {
  old: '小铁文娱流程模版',
  new: '沐腾科技流程模版',
}

const mutengActivityKindLabels: Record<string, string> = {
  goal_setting: '目标设定活动',
  review_scoring: '评分活动',
}

const completedActivityStatuses = new Set(['locked', 'result_confirmed', 'archived'])

const builtInFlowTemplateNames = new Set([
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

function normalizeCycleType(value?: string) {
  return String(value || '').trim()
}

function getCycleLabel(value?: string) {
  const normalized = normalizeCycleType(value)
  return cycleLabels[normalized] || normalized || '未知周期'
}

function getFlowTypeLabel(value?: string) {
  const normalized = String(value || '').trim()
  return flowTypeLabels[normalized] || '选择模板后自动带出'
}

function getMutengActivityKindLabel(value?: string) {
  const normalized = String(value || '').trim()
  return mutengActivityKindLabels[normalized] || '历史混合活动'
}

function getTemplateDisplayName(template: any) {
  const flowType = String(template?.flow_type || '').trim()
  const templateName = String(template?.name || '').trim()
  if (templateName && !builtInFlowTemplateNames.has(templateName)) return templateName
  return flowTypeLabels[flowType] || templateName || '未命名流程模版'
}

function getFlowTemplateOptionLabel(template: any) {
  return getTemplateDisplayName(template)
}

function getPreviousActivityOptionLabel(activity: any) {
  const period = [activity?.start_date, activity?.end_date].filter(Boolean).join(' ~ ')
  return `${activity?.name || `活动 #${activity?.id}`}${period ? `（${period}）` : ''}`
}

function normalizeEditorIDArray(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(item => String(item).trim()).filter(Boolean)
  if (!value) return []
  return String(value).split(',').map(item => item.trim()).filter(Boolean)
}

function toSearchText(value: unknown): string {
  if (typeof value === 'string' || typeof value === 'number') return String(value)
  if (Array.isArray(value)) return value.map(toSearchText).join(' ')
  return ''
}

function filterSelectOption(input: string, option?: { label?: unknown; value?: unknown }) {
  const keyword = input.trim().toLowerCase()
  if (!keyword) return true
  return `${toSearchText(option?.label)} ${toSearchText(option?.value)}`.toLowerCase().includes(keyword)
}

const fieldHeaderStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: 8,
  minHeight: 24,
  marginBottom: 8,
}

type PerformanceActivityEditorContentProps = Omit<PerformanceActivityEditorProps, 'visible'>

const PerformanceActivityEditorContent: React.FC<PerformanceActivityEditorContentProps> = ({
  editing,
  form,
  saving = false,
  performanceTemplates = [],
  performanceTemplatesLoading = false,
  activities = [],
  currentActivityId,
  indicatorLibraries,
  indicatorLibrariesLoading,
  departmentOptions,
  userOptions,
  scopeOptionsLoading,
  importingParticipants = false,
  onImportParticipants,
  onSave,
  onCancel,
}) => {
  const [, forceFormRerender] = React.useState(0)

  const values = form.getFieldsValue(true)
  const cycleType = values.cycle_type as string | undefined
  const targetEmployeeIDs = normalizeEditorIDArray(values.target_employee_ids)
  const selectedIndicatorLibraryId = values.indicator_library_id as number | string | undefined
  const selectedTemplateId = values.template_id as number | string | undefined
  const selectedPreviousReviewActivityId = values.previous_review_activity_id as number | string | undefined
  const normalizedCycleType = normalizeCycleType(cycleType)
  const selectedIndicatorLibraryIdKey = selectedIndicatorLibraryId == null ? '' : String(selectedIndicatorLibraryId)
  const selectedTemplateIdKey = selectedTemplateId == null ? '' : String(selectedTemplateId)
  const visiblePerformanceTemplates = React.useMemo(() => {
    const seen = new Set<string>()
    return performanceTemplates.filter(template => {
      const key = `${String(template.flow_type || '').trim()}::${getFlowTemplateOptionLabel(template)}`
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
  }, [performanceTemplates])
  const currentActivityIdKey = currentActivityId == null ? '' : String(currentActivityId)
  const selectedTemplate = React.useMemo(
    () => performanceTemplates.find(template => String(template.id) === selectedTemplateIdKey) || null,
    [performanceTemplates, selectedTemplateIdKey],
  )
  const templateSelectOptions = React.useMemo(() => {
    const options = visiblePerformanceTemplates.map(template => ({
      value: template.id,
      label: getFlowTemplateOptionLabel(template),
    }))
    if (selectedTemplate && !options.some(option => String(option.value) === String(selectedTemplate.id))) {
      options.push({ value: selectedTemplate.id, label: getFlowTemplateOptionLabel(selectedTemplate) })
    }
    return options
  }, [selectedTemplate, visiblePerformanceTemplates])
  const selectedFlowType = String(selectedTemplate?.flow_type || values.flow_type || '').trim()
  const isMutengTemplate = selectedFlowType === 'new'
  const selectedActivityKind = String(values.activity_kind || '').trim()
  const isHistoricalMutengActivity = editing && isMutengTemplate && !selectedActivityKind
  const isMutengReviewScoringActivity = isMutengTemplate && selectedActivityKind === 'review_scoring'
  const requiresReviewSchedule = !isMutengTemplate || isMutengReviewScoringActivity || isHistoricalMutengActivity
  const requiresResultConfirmSchedule = requiresReviewSchedule && !isMutengReviewScoringActivity
  React.useEffect(() => {
    if (selectedTemplateIdKey || performanceTemplatesLoading || visiblePerformanceTemplates.length === 0) return
    const currentFlowType = String(form.getFieldValue('flow_type') || 'old').trim()
    const defaultTemplate = visiblePerformanceTemplates.find(template => String(template.flow_type || '').trim() === currentFlowType)
      || visiblePerformanceTemplates[0]
    if (!defaultTemplate) return
    form.setFieldsValue({
      template_id: defaultTemplate.id,
      flow_type: defaultTemplate.flow_type || currentFlowType,
      activity_kind: String(defaultTemplate.flow_type || currentFlowType).trim() === 'new' ? 'goal_setting' : undefined,
    })
    forceFormRerender(version => version + 1)
  }, [form, performanceTemplatesLoading, selectedTemplateIdKey, visiblePerformanceTemplates])
  React.useEffect(() => {
    if (isMutengTemplate) return
    if (form.getFieldValue('previous_review_activity_id')) {
      form.setFieldValue('previous_review_activity_id', undefined)
    }
    if (form.getFieldValue('activity_kind')) {
      form.setFieldValue('activity_kind', undefined)
    }
  }, [form, isMutengTemplate])
  const selectedIndicatorLibrary = React.useMemo(
    () => indicatorLibraries.find(lib => String(lib.id) === selectedIndicatorLibraryIdKey) || null,
    [indicatorLibraries, selectedIndicatorLibraryIdKey],
  )
  const indicatorLibraryCycleMismatch = Boolean(
    normalizedCycleType
      && selectedIndicatorLibrary
      && normalizeCycleType(selectedIndicatorLibrary.default_cycle) !== normalizedCycleType,
  )
  const indicatorLibraryTemplateMismatch = Boolean(
    selectedTemplateIdKey
      && selectedIndicatorLibrary
      && String(selectedIndicatorLibrary.template_id || '') !== selectedTemplateIdKey,
  )
  const visibleIndicatorLibraries = React.useMemo(() => {
    const filteredLibraries = indicatorLibraries.filter(lib => {
      if (selectedTemplateIdKey && String(lib.template_id || '') !== selectedTemplateIdKey) return false
      if (!normalizedCycleType) return true
      return normalizeCycleType(lib.default_cycle) === normalizedCycleType
    })

    if (!selectedIndicatorLibrary || (!indicatorLibraryCycleMismatch && !indicatorLibraryTemplateMismatch)) {
      return filteredLibraries
    }

    if (filteredLibraries.some(lib => String(lib.id) === String(selectedIndicatorLibrary.id))) {
      return filteredLibraries
    }

    return [...filteredLibraries, selectedIndicatorLibrary]
  }, [indicatorLibraries, normalizedCycleType, selectedTemplateIdKey, indicatorLibraryCycleMismatch, indicatorLibraryTemplateMismatch, selectedIndicatorLibrary])
  const previousReviewActivityOptions = React.useMemo(() => {
    if (!isMutengReviewScoringActivity) return []
    return activities
      .filter(activity => {
        const activityId = activity?.id == null ? '' : String(activity.id)
        if (!activityId || activityId === currentActivityIdKey) return false
        if (String(activity.flow_type || '').trim() !== 'new') return false
        const kind = String(activity.activity_kind || '').trim()
        if (kind && kind !== 'goal_setting') return false
        if (!completedActivityStatuses.has(String(activity.status || '').trim())) return false
        if (normalizedCycleType && normalizeCycleType(activity.cycle_type) !== normalizedCycleType) return false
        return true
      })
      .sort((a, b) => {
        const endDiff = String(b.end_date || '').localeCompare(String(a.end_date || ''))
        return endDiff || Number(b.id || 0) - Number(a.id || 0)
      })
      .map(activity => ({
        value: activity.id,
        label: getPreviousActivityOptionLabel(activity),
      }))
  }, [activities, currentActivityIdKey, isMutengReviewScoringActivity, normalizedCycleType])
  const requiredChecks = [
    { id: 'activity-basic-section', label: '基础信息', done: Boolean(values.name && values.cycle_type && values.template_id) },
    { id: 'activity-period-section', label: '周期设置', done: isRangeFilled(values.date_range) },
    {
      id: 'activity-review-section',
      label: '评审流程',
      done: !requiresReviewSchedule || (
        isRangeFilled(values.self_eval_range)
        && isRangeFilled(values.manager_eval_range)
        && (!requiresResultConfirmSchedule || isRangeFilled(values.result_confirm_range))
      ),
    },
  ]
  const doneCount = requiredChecks.filter(item => item.done).length
  const progress = Math.round((doneCount / requiredChecks.length) * 100)
  const getRequiredCheck = (sectionId: string) => requiredChecks.find(item => item.id === sectionId)

  const saveActions = (
    <Space wrap>
      <Button data-testid="performance-editor-cancel" icon={<CloseOutlined />} onClick={onCancel} disabled={saving}>
        取消
      </Button>
      <Button data-testid="performance-editor-save" type="primary" icon={<SaveOutlined />} loading={saving} onClick={onSave} style={{ background: '#4338ca', borderColor: '#4338ca' }}>
        {editing ? '保存修改' : '保存活动'}
      </Button>
    </Space>
  )

  return (
    <div
      id="performance-activity-editor"
      data-testid="performance-activity-editor"
      style={{
        background: '#fff',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        border: '1px solid #e5e7eb',
        borderRadius: 8,
        overflow: 'hidden',
        marginBottom: 16,
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: 16,
          padding: '18px 24px',
          borderBottom: '1px solid #e5e7eb',
          background: '#fff',
        }}
      >
        <Space size={10} align="center">
          <Text strong style={{ fontSize: 16, color: '#111827' }}>
            {editing ? '编辑绩效活动' : '创建绩效活动'}
          </Text>
          <Tag color={progress === 100 ? 'success' : 'processing'} style={{ marginInlineEnd: 0 }}>
            {doneCount}/{requiredChecks.length} 必填项已完成
          </Tag>
        </Space>
        <Space size={12} align="center">
          <Progress percent={progress} size="small" showInfo={false} style={{ width: 160 }} />
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
              <Row gutter={[16, 12]}>
                <Col xs={24} md={8}>
                  <Form.Item name="name" label="活动名称" rules={[{ required: true, message: '请输入活动名称' }]}>
                    <Input data-testid="performance-editor-activity-name" placeholder="如：2026 Q2 绩效评估" />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item name="cycle_type" label="周期类型" rules={[{ required: true, message: '请选择周期类型' }]}>
                    <Select
                      placeholder="选择周期类型"
                      onChange={() => {
                        form.setFieldValue('indicator_library_id', undefined)
                        form.setFieldValue('previous_review_activity_id', undefined)
                        forceFormRerender(version => version + 1)
                      }}
                      options={[
                        { value: 'monthly', label: '月度' },
                        { value: 'quarterly', label: '季度' },
                        { value: 'semiannual', label: '半年度' },
                        { value: 'annual', label: '年度' },
                        { value: 'probation', label: '试用期' },
                      ]}
                    />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item
                    name="template_id"
                    label="流程模板"
                    rules={[{ required: true, message: '请选择流程模板' }]}
                  >
                    <Select
                      placeholder="请选择绩效流程模板"
                      showSearch
                      loading={performanceTemplatesLoading}
                      optionFilterProp="label"
                      filterOption={filterSelectOption}
                      options={templateSelectOptions}
                      onChange={(value) => {
                        const template = performanceTemplates.find(item => String(item.id) === String(value))
                        if (template?.flow_type) {
                          form.setFieldValue('flow_type', template.flow_type)
                          form.setFieldValue('activity_kind', String(template.flow_type).trim() === 'new' ? 'goal_setting' : undefined)
                        } else {
                          form.setFieldValue('flow_type', undefined)
                          form.setFieldValue('activity_kind', undefined)
                        }
                        form.setFieldValue('indicator_library_id', undefined)
                        form.setFieldValue('previous_review_activity_id', undefined)
                        forceFormRerender(version => version + 1)
                      }}
                    />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item name="flow_type" hidden>
                    <Input />
                  </Form.Item>
                  <Form.Item label="流程类型">
                    <Space size={8} wrap>
                      <Tag color={selectedFlowType === 'new' ? 'processing' : 'default'} style={{ marginInlineEnd: 0 }}>
                        {getFlowTypeLabel(selectedFlowType)}
                      </Tag>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        由流程模板自动决定
                      </Text>
                    </Space>
                  </Form.Item>
                  {selectedTemplate?.description && (
                    <Text type="secondary" style={{ display: 'block', marginTop: -12, marginBottom: 12, fontSize: 12 }}>
                      {selectedTemplate.description}
                    </Text>
                  )}
                </Col>
                {isMutengTemplate && (
                  <Col xs={24} md={8}>
                    {isHistoricalMutengActivity ? (
                      <Form.Item label="活动类型">
                        <Space size={8} wrap>
                          <Tag color="default" style={{ marginInlineEnd: 0 }}>{getMutengActivityKindLabel(selectedActivityKind)}</Tag>
                          <Text type="secondary" style={{ fontSize: 12 }}>历史已有活动不迁移</Text>
                        </Space>
                      </Form.Item>
                    ) : (
                      <Form.Item
                        name="activity_kind"
                        label="活动类型"
                        rules={[{ required: true, message: '请选择活动类型' }]}
                        extra={
                          <Text type="secondary">
                            目标设定活动只填写下季度目标；评分活动承接上一目标后进入自评评分。
                          </Text>
                        }
                      >
                        <Segmented
                          block
                          options={[
                            { label: '目标设定', value: 'goal_setting' },
                            { label: '评分', value: 'review_scoring' },
                          ]}
                          onChange={(value) => {
                            if (value !== 'review_scoring') {
                              form.setFieldValue('previous_review_activity_id', undefined)
                            }
                            forceFormRerender(version => version + 1)
                          }}
                        />
                      </Form.Item>
                    )}
                  </Col>
                )}
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

                          if (selectedTemplateIdKey && String(library.template_id || '') !== selectedTemplateIdKey) {
                            return Promise.reject(new Error('请选择当前流程模板下的指标库'))
                          }

                          const libraryCycle = normalizeCycleType(library.default_cycle)
                          if (libraryCycle === normalizedCycleType) return Promise.resolve()

                          return Promise.reject(new Error(`请选择${getCycleLabel(normalizedCycleType)}指标库`))
                        },
                      },
                    ]}
                    extra={
                      selectedTemplateIdKey && normalizedCycleType
                        ? indicatorLibraryTemplateMismatch && selectedIndicatorLibrary
                          ? (
                            <Text type="warning">
                              当前已选指标库不属于所选流程模板，请更换。
                            </Text>
                          )
                          : indicatorLibraryCycleMismatch && selectedIndicatorLibrary
                          ? (
                            <Text type="warning">
                              当前已选指标库默认周期为 {getCycleLabel(selectedIndicatorLibrary.default_cycle)}，与活动周期 {getCycleLabel(normalizedCycleType)} 不一致，请更换。
                            </Text>
                          )
                          : (
                            <Text type="secondary">
                              仅显示当前流程模板下，且默认周期为 {getCycleLabel(normalizedCycleType)} 的指标库。
                            </Text>
                          )
                        : (
                          <Text type="secondary">
                            请先选择流程模板和周期类型，指标库会自动过滤。
                          </Text>
                        )
                    }
                  >
                    <Select
                      placeholder={selectedTemplateIdKey && normalizedCycleType ? `请选择${getCycleLabel(normalizedCycleType)}指标库（可选）` : '请先选择流程模板和周期类型'}
                      allowClear
                      showSearch
                      disabled={!selectedTemplateIdKey || !normalizedCycleType}
                      loading={indicatorLibrariesLoading}
                      optionFilterProp="label"
                      filterOption={filterSelectOption}
                      options={visibleIndicatorLibraries.map(lib => ({
                        value: lib.id,
                        label: `${lib.name}${lib.default_cycle ? `（${getCycleLabel(lib.default_cycle)}）` : ''}`,
                      }))}
                    />
                  </Form.Item>
                </Col>
                {isMutengReviewScoringActivity && (
                  <Col xs={24} md={8}>
                    <Form.Item
                      name="previous_review_activity_id"
                      label="评分活动承接目标活动"
                      rules={[
                        {
                          validator: (_, value) => {
                            if (!value) return Promise.reject(new Error('请选择上一目标设定活动'))
                            if (previousReviewActivityOptions.some(option => String(option.value) === String(value))) {
                              return Promise.resolve()
                            }
                            return Promise.reject(new Error('请选择已完成的沐腾科技目标设定活动'))
                          },
                        },
                      ]}
                      extra={
                        <Text type="secondary">
                          创建评分活动时选择上一期目标设定活动；只创建下季度目标设定活动时可留空。
                        </Text>
                      }
                    >
                      <Select
                        data-testid="performance-editor-previous-review-activity"
                        placeholder="选择上一目标设定活动（可选）"
                        allowClear
                        showSearch
                        optionFilterProp="label"
                        filterOption={filterSelectOption}
                        options={previousReviewActivityOptions}
                        disabled={!normalizedCycleType}
                        notFoundContent={normalizedCycleType ? '暂无已完成的沐腾科技目标设定活动' : '请先选择周期类型'}
                        onChange={() => forceFormRerender(version => version + 1)}
                      />
                    </Form.Item>
                    {selectedPreviousReviewActivityId && previousReviewActivityOptions.length === 0 && (
                      <Text type="warning" style={{ display: 'block', marginTop: -12, marginBottom: 12, fontSize: 12 }}>
                        当前承接活动不符合已完成沐腾科技流程条件，请重新选择。
                      </Text>
                    )}
                  </Col>
                )}
              </Row>
            </section>

            <section id="activity-period-section" style={sectionStyle}>
              <div style={sectionTitleStyle}>
                <Text strong style={{ fontSize: 15 }}>周期设置</Text>
                <Tag color={requiredChecks[1].done ? 'success' : 'warning'} style={{ marginInlineEnd: 0 }}>
                  必填
                </Tag>
              </div>
              <Row gutter={[16, 12]}>
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
                <Col xs={24} md={12}>
                  <Form.Item name="snapshot_as_of_date" label="组织快照日期" extra="为空时按创建参与人时的当前组织归属生成快照">
                    <DatePicker placeholder="选择考核期归属日期" style={{ width: '100%' }} />
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
              <Row gutter={[16, 12]}>
                {requiresReviewSchedule ? (
                  <>
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
                    {requiresResultConfirmSchedule && (
                      <Col xs={24} lg={8}>
                        <Form.Item
                          name="result_confirm_range"
                          label={isMutengTemplate ? 'HR/员工确认时间' : '结果确认时间'}
                          rules={[{ required: true, message: isMutengTemplate ? '请选择HR/员工确认时间' : '请选择结果确认时间' }]}
                        >
                          <RangePicker placeholder={['开始日期', '结束日期']} style={{ width: '100%' }} />
                        </Form.Item>
                      </Col>
                    )}
                  </>
                ) : (
                  <Col span={24}>
                    <Text type="secondary">目标设定活动只包含目标拟定、目标审核和锁定归档，不需要配置自评、主管评分或结果确认时间。</Text>
                  </Col>
                )}
              </Row>
            </section>

            <section id="activity-scope-section" style={sectionStyle}>
              <div style={sectionTitleStyle}>
                <Text strong style={{ fontSize: 15 }}>参与范围</Text>
                <Tag style={{ marginInlineEnd: 0 }}>可选</Tag>
              </div>
              <Row gutter={[16, 12]} align="top">
                <Col xs={24} md={12}>
                  <div style={fieldHeaderStyle}>
                    <Text>参与部门</Text>
                  </div>
                  <Form.Item name="target_department_ids" style={{ marginBottom: 0 }}>
                    <Select
                      mode="multiple"
                      allowClear
                      showSearch
                      loading={scopeOptionsLoading}
                      optionFilterProp="label"
                      filterOption={filterSelectOption}
                      maxTagCount="responsive"
                      placeholder="请选择参与部门"
                      options={departmentOptions}
                      style={{ width: '100%' }}
                    />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <div style={fieldHeaderStyle}>
                    <Text>指定员工</Text>
                    <Upload
                      accept=".xlsx"
                      showUploadList={false}
                      beforeUpload={(file) => {
                        void onImportParticipants(file as File)
                        return false
                      }}
                    >
                      <Button data-testid="performance-import-participants" size="small" type="text" icon={<UploadOutlined />} loading={importingParticipants}>
                        导入 Excel
                      </Button>
                    </Upload>
                  </div>
                  <Form.Item name="target_employee_ids" style={{ marginBottom: 0 }}>
                    <Select
                      mode="multiple"
                      allowClear
                      showSearch
                      loading={scopeOptionsLoading}
                      optionFilterProp="label"
                      filterOption={filterSelectOption}
                      maxTagCount="responsive"
                      placeholder="请选择指定员工"
                      options={userOptions}
                      style={{ width: '100%' }}
                    />
                  </Form.Item>
                  {targetEmployeeIDs.length > 0 && (
                    <Text type="secondary" style={{ display: 'block', marginTop: 6, fontSize: 12 }}>
                      已选择 {targetEmployeeIDs.length} 人
                    </Text>
                  )}
                </Col>
              </Row>
            </section>

            <section id="activity-advanced-section" style={{ ...sectionStyle, borderBottom: 'none', paddingBottom: 24 }}>
              <div style={sectionTitleStyle}>
                <Text strong style={{ fontSize: 15 }}>高级设置</Text>
                <Tag style={{ marginInlineEnd: 0 }}>可选</Tag>
              </div>
              <Row gutter={[16, 12]}>
                <Col xs={24} md={12}>
                  <Form.Item
                    name="default_assessment_manager_source"
                    label="默认考核上级规则"
                    initialValue="DIRECT_MANAGER"
                  >
                    <Select
                      options={[
                        { label: '直属主管', value: 'DIRECT_MANAGER' },
                        { label: '部门负责人', value: 'DEPARTMENT_HEAD' },
                        { label: '中心负责人', value: 'CENTER_HEAD' },
                        { label: '暂不设置', value: 'EMPTY' },
                      ]}
                    />
                  </Form.Item>
                </Col>
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
                  <Form.Item
                    name="strict_time_mode"
                    label="严格时间模式"
                    valuePropName="checked"
                    extra="开启后超过截止时间将禁止提交自评/评分"
                  >
                    <Switch checkedChildren="开启" unCheckedChildren="关闭" />
                  </Form.Item>
                </Col>
                {!isMutengTemplate && (
                  <>
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
                  </>
                )}
                <Col xs={24}>
                  <Form.Item name="description" label="描述">
                    <TextArea rows={3} placeholder="补充活动说明" />
                  </Form.Item>
                </Col>
              </Row>
            </section>
      </Form>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: 16,
          padding: '14px 28px',
          borderTop: '1px solid #e5e7eb',
          background: '#fff',
          position: 'sticky',
          bottom: 0,
          zIndex: 2,
        }}
      >
        <Text type="secondary" style={{ fontSize: 12 }}>
          {targetEmployeeIDs.length > 0 ? `已指定 ${targetEmployeeIDs.length} 名员工` : '未指定员工时可按部门生成参与范围'}
        </Text>
        {saveActions}
      </div>
    </div>
  )
}

const PerformanceActivityEditor: React.FC<PerformanceActivityEditorProps> = ({ visible, form, ...props }) => {
  return (
    <div style={{ display: visible ? 'block' : 'none' }}>
      <PerformanceActivityEditorContent form={form} {...props} />
    </div>
  )
}

export default PerformanceActivityEditor
