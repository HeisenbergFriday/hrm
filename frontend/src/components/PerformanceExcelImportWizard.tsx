import React, { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Col,
  DatePicker,
  Divider,
  Empty,
  Input,
  Modal,
  Result,
  Row,
  Select,
  Space,
  Steps,
  Table,
  Tag,
  Typography,
  Upload,
  message,
} from 'antd'
import type { UploadFile } from 'antd/es/upload/interface'
import { FileExcelOutlined, InboxOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import {
  performanceAPI,
  type PerformanceActivityImportBatch,
  type PerformanceActivityImportCommitDraft,
  type PerformanceActivityImportDraft,
  type PerformanceActivityImportIssue,
} from '../services/api'

const { Text, Paragraph } = Typography
const { Dragger } = Upload

interface PerformanceExcelImportWizardProps {
  open: boolean
  onCancel: () => void
  onCommitted: () => void
}

interface EmployeeOption {
  user_id: string
  name: string
  department_name?: string
  position?: string
}

const stepItems = [
  { title: '上传 Excel' },
  { title: '识别结果' },
  { title: '补充确认' },
  { title: '创建完成' },
]

const cycleOptions = [
  { label: '月度', value: 'monthly' },
  { label: '季度', value: 'quarterly' },
  { label: '年度', value: 'annual' },
]

const unwrapData = <T,>(response: any): T => (response?.data ?? response) as T

const errorMessage = (error: any, fallback: string) =>
  error?.response?.data?.message || error?.message || fallback

const issueAlertType = (issue: PerformanceActivityImportIssue) => {
  if (issue.level === 'error') return 'error' as const
  if (issue.level === 'info') return 'info' as const
  return 'warning' as const
}

const matchTag = (draft: PerformanceActivityImportDraft) => {
  if (draft.employee_match === 'matched') return <Tag color="success">已匹配员工</Tag>
  if (draft.employee_match === 'ambiguous') return <Tag color="warning">同名员工，需确认</Tag>
  return <Tag>未匹配员工</Tag>
}

const PerformanceExcelImportWizard: React.FC<PerformanceExcelImportWizardProps> = ({
  open,
  onCancel,
  onCommitted,
}) => {
  const [currentStep, setCurrentStep] = useState(0)
  const [fileList, setFileList] = useState<UploadFile[]>([])
  const [analyzing, setAnalyzing] = useState(false)
  const [committing, setCommitting] = useState(false)
  const [batch, setBatch] = useState<PerformanceActivityImportBatch>()
  const [drafts, setDrafts] = useState<PerformanceActivityImportDraft[]>([])
  const [employees, setEmployees] = useState<EmployeeOption[]>([])
  const [employeesLoading, setEmployeesLoading] = useState(false)

  const selectedDrafts = useMemo(() => drafts.filter(draft => draft.selected), [drafts])

  const reset = () => {
    setCurrentStep(0)
    setFileList([])
    setBatch(undefined)
    setDrafts([])
    setEmployees([])
  }

  useEffect(() => {
    if (!open && !analyzing && !committing) reset()
  }, [open, analyzing, committing])

  const updateDraft = (draftKey: string, patch: Partial<PerformanceActivityImportDraft>) => {
    setDrafts(current => current.map(draft => (
      draft.draft_key === draftKey ? { ...draft, ...patch } : draft
    )))
  }

  const loadEmployees = async () => {
    if (employees.length || employeesLoading) return
    setEmployeesLoading(true)
    try {
      const response: any = await performanceAPI.getScopeOptions({ page: 1, page_size: 2000 })
      const data: any = unwrapData(response)
      setEmployees(Array.isArray(data?.employees) ? data.employees : [])
    } catch {
      setEmployees([])
      message.warning('员工选项加载失败，已匹配员工仍可直接提交')
    } finally {
      setEmployeesLoading(false)
    }
  }

  const analyze = async () => {
    const file = fileList[0]?.originFileObj
    if (!file) {
      message.warning('请先选择 Excel 文件')
      return
    }
    setAnalyzing(true)
    try {
      const response: any = await performanceAPI.analyzeActivityImport(file)
      const nextBatch = unwrapData<PerformanceActivityImportBatch>(response)
      if (!nextBatch?.preview) throw new Error('服务器未返回识别预览')
      setBatch(nextBatch)
      setDrafts(nextBatch.preview.drafts.map(draft => ({ ...draft })))
      setCurrentStep(1)
      message.success(`已识别为${nextBatch.preview.source_label}模板`)
    } catch (error: any) {
      message.error(errorMessage(error, 'Excel 识别失败'))
    } finally {
      setAnalyzing(false)
    }
  }

  const goToConfirm = async () => {
    setCurrentStep(2)
    await loadEmployees()
  }

  const commit = async () => {
    if (!batch) return
    if (!selectedDrafts.length) {
      message.warning('请至少选择一个绩效活动')
      return
    }
    const incomplete = selectedDrafts.find(draft => (
      !draft.template_name.trim() || !draft.activity_name.trim() || !draft.start_date || !draft.end_date
    ))
    if (incomplete) {
      message.warning(`请补全“${incomplete.template_name || incomplete.source_sheet}”的名称和日期`)
      return
    }
    const payload: PerformanceActivityImportCommitDraft[] = drafts.map(draft => ({
      draft_key: draft.draft_key,
      selected: draft.selected,
      template_name: draft.template_name,
      activity_name: draft.activity_name,
      cycle_type: draft.cycle_type,
      start_date: draft.start_date,
      end_date: draft.end_date,
      employee_user_id: draft.employee_user_id || undefined,
    }))
    setCommitting(true)
    try {
      const response: any = await performanceAPI.commitActivityImport(batch.batch_id, payload)
      const result = unwrapData<any>(response)
      setBatch(current => current ? { ...current, status: 'committed', result } : current)
      setCurrentStep(3)
      message.success('绩效模板和草稿活动已创建')
      onCommitted()
    } catch (error: any) {
      message.error(errorMessage(error, '创建绩效活动失败'))
    } finally {
      setCommitting(false)
    }
  }

  const close = () => {
    if (analyzing || committing) return
    reset()
    onCancel()
  }

  const uploadContent = (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Alert
        type="info"
        showIcon
        message="支持小铁文娱、沐腾科技绩效模板"
        description="这里只做识别和预览，不会直接创建活动。确认提交时才会整批创建，任意一步失败都会全部回滚。"
      />
      <Dragger
        accept=".xlsx"
        maxCount={1}
        fileList={fileList}
        beforeUpload={(file) => {
          if (!file.name.toLowerCase().endsWith('.xlsx')) {
            message.error('仅支持 .xlsx 文件')
            return Upload.LIST_IGNORE
          }
          if (file.size > 10 * 1024 * 1024) {
            message.error('Excel 文件不能超过 10MB')
            return Upload.LIST_IGNORE
          }
          setFileList([{ ...file, originFileObj: file } as UploadFile])
          return false
        }}
        onRemove={() => {
          setFileList([])
          return true
        }}
      >
        <p className="ant-upload-drag-icon"><InboxOutlined /></p>
        <p className="ant-upload-text">点击或拖入绩效 Excel</p>
        <p className="ant-upload-hint">系统会自动判断是小铁文娱还是沐腾科技模板</p>
      </Dragger>
    </Space>
  )

  const previewContent = batch?.preview ? (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Alert
        type="success"
        showIcon
        message={`已识别：${batch.preview.source_label}`}
        description={`文件：${batch.preview.file_name}；共识别 ${batch.preview.drafts.length} 个可导入内容。`}
      />
      {batch.preview.issues.map((issue, index) => (
        <Alert
          key={`${issue.code}-${issue.draft_key || ''}-${index}`}
          type={issueAlertType(issue)}
          showIcon
          message={issue.message}
          description={[issue.sheet && `Sheet：${issue.sheet}`, issue.row && `第 ${issue.row} 行`].filter(Boolean).join('；') || undefined}
        />
      ))}
      <Row gutter={[12, 12]}>
        {drafts.map(draft => (
          <Col xs={24} lg={12} key={draft.draft_key}>
            <Card size="small" title={draft.template_name} extra={draft.selected ? <Tag color="blue">默认导入</Tag> : <Tag>默认不导入</Tag>}>
              <Space direction="vertical" size={8} style={{ width: '100%' }}>
                <Text type="secondary">来源 Sheet：{draft.source_sheet}</Text>
                <Space wrap>
                  <Tag>{draft.flow_type === 'new' ? '沐腾新流程' : '小铁原流程'}</Tag>
                  <Tag>{draft.cycle_type === 'monthly' ? '月度' : draft.cycle_type === 'quarterly' ? '季度' : '年度'}</Tag>
                  {draft.enable_bonus_score && <Tag color="purple">含附加分</Tag>}
                  {matchTag(draft)}
                </Space>
                <Text>维度：{draft.sections.map(section => `${section.name} ${section.weight}%`).join('、')}</Text>
                <Text>指标：{draft.goals.length} 项；源权重合计：{draft.source_weight_total}%</Text>
              </Space>
            </Card>
          </Col>
        ))}
      </Row>
    </Space>
  ) : <Empty description="暂无识别结果" />

  const confirmContent = (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Alert
        type="warning"
        showIcon
        message="请确认活动周期和日期"
        description="特别是沐腾模板，文件名、Q1/Q2 和“下季度”可能冲突，系统不会替 HR 猜年份。"
      />
      {drafts.map(draft => (
        <Card
          key={draft.draft_key}
          title={
            <Checkbox checked={draft.selected} onChange={event => updateDraft(draft.draft_key, { selected: event.target.checked })}>
              {draft.source_sheet}
            </Checkbox>
          }
          size="small"
          styles={{ body: { opacity: draft.selected ? 1 : 0.55 } }}
        >
          <Row gutter={[12, 12]}>
            <Col xs={24} md={12}>
              <Text type="secondary">模板名称</Text>
              <Input disabled={!draft.selected} value={draft.template_name} onChange={event => updateDraft(draft.draft_key, { template_name: event.target.value })} />
            </Col>
            <Col xs={24} md={12}>
              <Text type="secondary">活动名称</Text>
              <Input disabled={!draft.selected} value={draft.activity_name} onChange={event => updateDraft(draft.draft_key, { activity_name: event.target.value })} />
            </Col>
            <Col xs={24} md={8}>
              <Text type="secondary">周期类型</Text>
              <Select disabled={!draft.selected} style={{ width: '100%' }} value={draft.cycle_type} options={cycleOptions} onChange={value => updateDraft(draft.draft_key, { cycle_type: value })} />
            </Col>
            <Col xs={24} md={8}>
              <Text type="secondary">开始和结束日期</Text>
              <DatePicker.RangePicker
                allowEmpty={[true, true]}
                disabled={!draft.selected}
                style={{ width: '100%' }}
                value={draft.start_date && draft.end_date ? [dayjs(draft.start_date), dayjs(draft.end_date)] : null}
                onChange={dates => updateDraft(draft.draft_key, {
                  start_date: dates?.[0]?.format('YYYY-MM-DD') || '',
                  end_date: dates?.[1]?.format('YYYY-MM-DD') || '',
                })}
              />
            </Col>
            <Col xs={24} md={8}>
              <Text type="secondary">参与员工（可空）</Text>
              <Select
                allowClear
                showSearch
                disabled={!draft.selected}
                loading={employeesLoading}
                style={{ width: '100%' }}
                value={draft.employee_user_id || undefined}
                placeholder={draft.employee_name ? `识别姓名：${draft.employee_name}` : '未识别姓名，可选择员工'}
                optionFilterProp="label"
                options={employees.map(employee => ({
                  value: employee.user_id,
                  label: `${employee.name}${employee.department_name ? ` · ${employee.department_name}` : ''}`,
                }))}
                onChange={value => updateDraft(draft.draft_key, { employee_user_id: value || '' })}
              />
            </Col>
          </Row>
          <Divider style={{ marginBlock: 12 }} />
          <Table
            size="small"
            pagination={false}
            rowKey={record => `${record.section_type}-${record.sort_order}-${record.item_name}`}
            dataSource={draft.goals}
            columns={[
              { title: '维度', dataIndex: 'section_type', width: 120 },
              { title: '指标/目标', dataIndex: 'item_name' },
              { title: '权重', dataIndex: 'weight', width: 90, render: value => `${value}%` },
              { title: '类型', dataIndex: 'goal_type', width: 90 },
            ]}
          />
        </Card>
      ))}
    </Space>
  )

  const completedContent = batch?.result ? (
    <Result
      status="success"
      title="绩效模板和草稿活动已创建"
      subTitle={`批次 ${batch.result.batch_id}，共创建 ${batch.result.created.length} 个活动。重复提交该批次不会重复创建。`}
      extra={batch.result.created.map(item => (
        <Card key={item.draft_key} size="small" style={{ textAlign: 'left', marginBottom: 8 }}>
          <Space wrap>
            <FileExcelOutlined />
            <Text strong>{item.activity_name}</Text>
            <Tag color="blue">活动ID {item.activity_id}</Tag>
            <Tag>{item.template_reused ? '复用已有模板' : '新建模板'}</Tag>
            {item.goal_count > 0 && <Tag color="green">目标 {item.goal_count} 项</Tag>}
          </Space>
        </Card>
      ))}
    />
  ) : <Empty description="暂无创建结果" />

  const body = [uploadContent, previewContent, confirmContent, completedContent][currentStep]
  const footer = currentStep === 0 ? [
    <Button key="cancel" onClick={close}>取消</Button>,
    <Button key="analyze" type="primary" loading={analyzing} onClick={analyze}>开始识别</Button>,
  ] : currentStep === 1 ? [
    <Button key="reupload" onClick={() => { setCurrentStep(0); setBatch(undefined); setDrafts([]) }}>重新上传</Button>,
    <Button key="confirm" type="primary" onClick={goToConfirm}>去补充确认</Button>,
  ] : currentStep === 2 ? [
    <Button key="back" onClick={() => setCurrentStep(1)}>上一步</Button>,
    <Button key="commit" type="primary" loading={committing} onClick={commit}>确认创建（{selectedDrafts.length}）</Button>,
  ] : [
    <Button key="done" type="primary" onClick={close}>完成</Button>,
  ]

  return (
    <Modal
      open={open}
      title="Excel 导入绩效活动"
      width={1040}
      onCancel={close}
      maskClosable={false}
      destroyOnHidden
      footer={footer}
    >
      <Steps current={currentStep} items={stepItems} size="small" style={{ marginBottom: 20 }} />
      <div style={{ maxHeight: '68vh', overflowY: 'auto', paddingRight: 4 }}>
        {body}
      </div>
      {currentStep < 3 && (
        <Paragraph type="secondary" style={{ marginTop: 16, marginBottom: 0 }}>
          分析阶段只保存导入预览；点击“确认创建”后，模板、活动、参与人和目标才会在同一个事务中写入。
        </Paragraph>
      )}
    </Modal>
  )
}

export default PerformanceExcelImportWizard