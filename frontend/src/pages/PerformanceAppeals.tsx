import React, { useEffect, useMemo, useState } from 'react'
import {
  Button,
  Col,
  Empty,
  Form,
  Input,
  Modal,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  message,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { CheckCircleOutlined, CloseCircleOutlined, PlusOutlined, ReloadOutlined, SearchOutlined, WarningOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import {
  PerformanceActivity,
  PerformanceAppealRecord,
  PerformanceAppealStatus,
  PerformanceFollowupListResponse,
  PerformanceFollowupSummary,
  PerformanceParticipant,
  performanceAPI,
} from '../services/api'
import { useAuthStore } from '../store/authStore'

interface AppealFilterValues {
  activity_id?: number
  status?: string
  employee_keyword?: string
}

interface AppealSubmitValues {
  activity_id?: number
  participant_id: number
  appeal_reason: string
  desired_result?: string
}

interface AppealProcessValues {
  handle_comment?: string
}

const APPEAL_STATUS_META: Record<PerformanceAppealStatus, { label: string; color: string }> = {
  submitted: { label: '待受理', color: 'warning' },
  processing: { label: '处理中', color: 'processing' },
  resolved: { label: '已处理', color: 'success' },
  rejected: { label: '已驳回', color: 'error' },
  withdrawn: { label: '已撤回', color: 'default' },
}

const emptySummary: PerformanceFollowupSummary = {
  total: 0,
  status_map: {},
  pending: 0,
  processing: 0,
  completed: 0,
  closed: 0,
}

function unwrapData<T>(res: any, fallback: T): T {
  return (res?.data || res || fallback) as T
}

function unwrapItems<T>(res: any): T[] {
  const data = unwrapData<any>(res, {})
  return data?.items || []
}

function statusTag(status: PerformanceAppealStatus) {
  const meta = APPEAL_STATUS_META[status] || { label: status, color: 'default' }
  return <Tag color={meta.color}>{meta.label}</Tag>
}

function levelTag(level?: string) {
  if (!level) return '-'
  const colorMap: Record<string, string> = { S: 'purple', A: 'green', B: 'blue', C: 'orange', D: 'red' }
  return <Tag color={colorMap[level] || 'default'}>{level}</Tag>
}

function formatDateTime(value?: string) {
  if (!value) return '-'
  const parsed = dayjs(value)
  return parsed.isValid() ? parsed.format('YYYY-MM-DD HH:mm') : value
}

const PerformanceAppeals: React.FC = () => {
  const [filterForm] = Form.useForm<AppealFilterValues>()
  const [submitForm] = Form.useForm<AppealSubmitValues>()
  const [processForm] = Form.useForm<AppealProcessValues>()
  const { permissions, user } = useAuthStore()
  const [activities, setActivities] = useState<PerformanceActivity[]>([])
  const [participants, setParticipants] = useState<PerformanceParticipant[]>([])
  const [items, setItems] = useState<PerformanceAppealRecord[]>([])
  const [summary, setSummary] = useState<PerformanceFollowupSummary>(emptySummary)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [loading, setLoading] = useState(false)
  const [baseLoading, setBaseLoading] = useState(false)
  const [submitOpen, setSubmitOpen] = useState(false)
  const [processOpen, setProcessOpen] = useState(false)
  const [processingRecord, setProcessingRecord] = useState<PerformanceAppealRecord | null>(null)
  const [targetStatus, setTargetStatus] = useState<Exclude<PerformanceAppealStatus, 'submitted' | 'withdrawn'>>('resolved')
  const [submitting, setSubmitting] = useState(false)

  const canManage = useMemo(() => {
    return user?.user_id === 'admin' ||
      permissions.includes('performance:appeal:manage') ||
      permissions.includes('performance:activity:manage')
  }, [permissions, user?.user_id])

  const activityOptions = useMemo(() => activities.map(activity => ({
    value: activity.id,
    label: `${activity.name} (${activity.start_date || '-'} ~ ${activity.end_date || '-'})`,
  })), [activities])

  const participantOptions = useMemo(() => participants.map(participant => ({
    value: participant.id,
    label: `${participant.employee_name} / ${participant.department_name || '-'} / ${participant.final_level || participant.suggested_level || '-'}`,
  })), [participants])

  const loadActivities = async () => {
    try {
      setBaseLoading(true)
      const res = await performanceAPI.getActivities({ page: 1, page_size: 200 })
      setActivities(unwrapItems<PerformanceActivity>(res))
    } catch (error) {
      message.error('加载绩效活动失败')
    } finally {
      setBaseLoading(false)
    }
  }

  const loadParticipants = async (activityId?: number) => {
    if (!activityId) {
      setParticipants([])
      return
    }
    try {
      const res = await performanceAPI.getParticipants(activityId, { page: 1, page_size: 500 })
      setParticipants(unwrapItems<PerformanceParticipant>(res))
    } catch (error) {
      setParticipants([])
      message.error('加载参与人失败')
    }
  }

  const loadRecords = async (nextPage = page, nextPageSize = pageSize) => {
    const values = filterForm.getFieldsValue()
    try {
      setLoading(true)
      const res = await performanceAPI.getPerformanceAppeals({
        page: nextPage,
        page_size: nextPageSize,
        activity_id: values.activity_id,
        status: values.status,
        employee_keyword: values.employee_keyword?.trim(),
      })
      const data = unwrapData<PerformanceFollowupListResponse<PerformanceAppealRecord>>(res, {
        items: [],
        total: 0,
        summary: emptySummary,
      })
      setItems(data.items || [])
      setTotal(data.total || 0)
      setSummary(data.summary || emptySummary)
      setPage(nextPage)
      setPageSize(nextPageSize)
    } catch (error) {
      message.error('加载绩效申诉失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadActivities()
    loadRecords(1, pageSize)
  }, [])

  const openSubmitModal = () => {
    setParticipants([])
    submitForm.resetFields()
    setSubmitOpen(true)
  }

  const handleSubmitAppeal = async () => {
    try {
      const values = await submitForm.validateFields()
      setSubmitting(true)
      await performanceAPI.createPerformanceAppeal({
        participant_id: values.participant_id,
        appeal_reason: values.appeal_reason.trim(),
        desired_result: values.desired_result?.trim(),
      })
      message.success('提交绩效申诉成功')
      setSubmitOpen(false)
      await loadRecords(1, pageSize)
    } catch (error) {
      if ((error as any)?.errorFields) return
      message.error('提交绩效申诉失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleAccept = async (record: PerformanceAppealRecord) => {
    try {
      await performanceAPI.updatePerformanceAppeal(record.id, { status: 'processing' })
      message.success('已受理绩效申诉')
      await loadRecords(page, pageSize)
    } catch (error) {
      message.error('受理绩效申诉失败')
    }
  }

  const openProcessModal = (record: PerformanceAppealRecord, status: Exclude<PerformanceAppealStatus, 'submitted' | 'withdrawn'>) => {
    setProcessingRecord(record)
    setTargetStatus(status)
    processForm.resetFields()
    processForm.setFieldsValue({ handle_comment: record.handle_comment })
    setProcessOpen(true)
  }

  const handleProcessAppeal = async () => {
    if (!processingRecord) return
    try {
      const values = await processForm.validateFields()
      setSubmitting(true)
      await performanceAPI.updatePerformanceAppeal(processingRecord.id, {
        status: targetStatus,
        handle_comment: values.handle_comment?.trim(),
      })
      message.success('处理绩效申诉成功')
      setProcessOpen(false)
      await loadRecords(page, pageSize)
    } catch (error) {
      if ((error as any)?.errorFields) return
      message.error('处理绩效申诉失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleWithdraw = (record: PerformanceAppealRecord) => {
    Modal.confirm({
      title: '确认撤回申诉？',
      content: '撤回后该申诉将不再继续处理。',
      okText: '确认撤回',
      cancelText: '取消',
      onOk: async () => {
        try {
          await performanceAPI.withdrawPerformanceAppeal(record.id)
          message.success('已撤回绩效申诉')
          await loadRecords(page, pageSize)
        } catch (error) {
          message.error('撤回绩效申诉失败')
        }
      },
    })
  }

  const columns: ColumnsType<PerformanceAppealRecord> = [
    { title: '活动', dataIndex: 'activity_name', width: 180, ellipsis: true },
    { title: '员工', dataIndex: 'employee_name', width: 120 },
    { title: '部门', dataIndex: 'department_name', width: 140, ellipsis: true },
    { title: '岗位', dataIndex: 'position', width: 120, ellipsis: true },
    { title: '等级', dataIndex: 'final_level', width: 80, render: levelTag },
    { title: '状态', dataIndex: 'status', width: 100, render: statusTag },
    { title: '申诉原因', dataIndex: 'appeal_reason', width: 220, ellipsis: true },
    { title: '处理人', dataIndex: 'handler_name', width: 120, render: value => value || '-' },
    { title: '提交时间', dataIndex: 'created_at', width: 150, render: formatDateTime },
    { title: '处理时间', dataIndex: 'handled_at', width: 150, render: formatDateTime },
    {
      title: '操作',
      key: 'actions',
      width: 210,
      fixed: 'right',
      render: (_, record) => {
        const active = record.status === 'submitted' || record.status === 'processing'
        if (canManage) {
          return (
            <Space size={6}>
              {record.status === 'submitted' && <Button size="small" onClick={() => handleAccept(record)}>受理</Button>}
              {active && (
                <Button size="small" type="primary" icon={<CheckCircleOutlined />} onClick={() => openProcessModal(record, 'resolved')}>
                  完成
                </Button>
              )}
              {active && (
                <Button size="small" danger icon={<CloseCircleOutlined />} onClick={() => openProcessModal(record, 'rejected')}>
                  驳回
                </Button>
              )}
            </Space>
          )
        }
        return active ? <Button size="small" onClick={() => handleWithdraw(record)}>撤回</Button> : '-'
      },
    },
  ]

  return (
    <PageContainer
      title="绩效申诉"
      subtitle="结果公布后的独立申诉处理，不回退绩效活动主流程"
      icon={<WarningOutlined />}
      extra={<Button type="primary" icon={<PlusOutlined />} onClick={openSubmitModal}>提交申诉</Button>}
    >
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Row gutter={[16, 16]}>
          <Col xs={12} md={6}><PageCard><Statistic title="申诉总数" value={summary.total} /></PageCard></Col>
          <Col xs={12} md={6}><PageCard><Statistic title="待受理" value={summary.pending} /></PageCard></Col>
          <Col xs={12} md={6}><PageCard><Statistic title="处理中" value={summary.processing} /></PageCard></Col>
          <Col xs={12} md={6}><PageCard><Statistic title="已完结" value={summary.completed + summary.closed} /></PageCard></Col>
        </Row>

        <PageCard>
          <Form form={filterForm} layout="inline" onFinish={() => loadRecords(1, pageSize)}>
            <Form.Item name="activity_id" label="活动">
              <Select allowClear showSearch loading={baseLoading} style={{ width: 260 }} options={activityOptions} optionFilterProp="label" />
            </Form.Item>
            <Form.Item name="status" label="状态">
              <Select
                allowClear
                style={{ width: 140 }}
                options={Object.entries(APPEAL_STATUS_META).map(([value, meta]) => ({ value, label: meta.label }))}
              />
            </Form.Item>
            <Form.Item name="employee_keyword" label="员工">
              <Input allowClear placeholder="姓名/工号" style={{ width: 180 }} />
            </Form.Item>
            <Form.Item>
              <Space>
                <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
                <Button icon={<ReloadOutlined />} onClick={() => { filterForm.resetFields(); loadRecords(1, pageSize) }}>重置</Button>
              </Space>
            </Form.Item>
          </Form>
        </PageCard>

        <PageCard>
          <Table
            rowKey="id"
            columns={columns}
            dataSource={items}
            loading={loading}
            locale={{ emptyText: <Empty description="暂无绩效申诉记录" /> }}
            scroll={{ x: 1460 }}
            pagination={{
              current: page,
              pageSize,
              total,
              showSizeChanger: true,
              showTotal: value => `共 ${value} 条`,
              onChange: loadRecords,
            }}
          />
        </PageCard>
      </Space>

      <Modal
        title="提交绩效申诉"
        open={submitOpen}
        onCancel={() => setSubmitOpen(false)}
        onOk={handleSubmitAppeal}
        confirmLoading={submitting}
        width={720}
        destroyOnHidden
      >
        <Form form={submitForm} layout="vertical">
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="activity_id" label="绩效活动" rules={[{ required: true, message: '请选择绩效活动' }]}>
                <Select
                  showSearch
                  loading={baseLoading}
                  options={activityOptions}
                  optionFilterProp="label"
                  onChange={(value: number) => {
                    submitForm.setFieldValue('participant_id', undefined)
                    loadParticipants(value)
                  }}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="participant_id" label="员工" rules={[{ required: true, message: '请选择员工' }]}>
                <Select showSearch options={participantOptions} optionFilterProp="label" />
              </Form.Item>
            </Col>
            <Col span={24}>
              <Form.Item name="appeal_reason" label="申诉原因" rules={[{ required: true, message: '请填写申诉原因' }]}>
                <Input.TextArea rows={4} placeholder="说明对绩效结果存在异议的原因" />
              </Form.Item>
            </Col>
            <Col span={24}>
              <Form.Item name="desired_result" label="期望处理结果">
                <Input.TextArea rows={3} placeholder="可填写期望复核内容、补充事实或建议处理方式" />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>

      <Modal
        title={targetStatus === 'resolved' ? '处理完成' : '驳回申诉'}
        open={processOpen}
        onCancel={() => setProcessOpen(false)}
        onOk={handleProcessAppeal}
        confirmLoading={submitting}
        okText={targetStatus === 'resolved' ? '确认完成' : '确认驳回'}
        okButtonProps={{ danger: targetStatus === 'rejected' }}
        destroyOnHidden
      >
        <Form form={processForm} layout="vertical">
          <Form.Item name="handle_comment" label="处理说明" rules={[{ required: true, message: '请填写处理说明' }]}>
            <Input.TextArea rows={5} placeholder="填写复核结论、沟通结果或驳回原因" />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  )
}

export default PerformanceAppeals
