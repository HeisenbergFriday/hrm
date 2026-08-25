import React, { useEffect, useMemo, useState } from 'react'
import {
  Button,
  DatePicker,
  Empty,
  Form,
  Input,
  Modal,
  Row,
  Col,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  message,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { EditOutlined, FileTextOutlined, PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { unwrapApiData, unwrapApiItems } from '../utils/apiResponse'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import {
  PerformanceActivity,
  PerformanceFollowupListResponse,
  PerformanceFollowupSummary,
  PerformanceInterviewRecord,
  PerformanceInterviewStatus,
  PerformanceInterviewType,
  PerformanceParticipant,
  performanceAPI,
} from '../services/api'
import { useAuthStore } from '../store/authStore'

interface InterviewFormValues {
  activity_id?: number
  participant_id: number
  interview_type: PerformanceInterviewType
  status: PerformanceInterviewStatus
  interviewer_id?: string
  interviewer_name?: string
  scheduled_at?: dayjs.Dayjs
  location?: string
  summary?: string
  result?: string
  cancel_reason?: string
}

interface FilterValues {
  activity_id?: number
  status?: string
  employee_keyword?: string
}

const INTERVIEW_STATUS_META: Record<PerformanceInterviewStatus, { label: string; color: string }> = {
  pending: { label: '待安排', color: 'warning' },
  scheduled: { label: '待面谈', color: 'processing' },
  completed: { label: '已完成', color: 'success' },
  cancelled: { label: '已取消', color: 'default' },
}

const INTERVIEW_TYPE_LABELS: Record<PerformanceInterviewType, string> = {
  required: '必谈',
  optional: '选谈',
}

const emptySummary: PerformanceFollowupSummary = {
  total: 0,
  status_map: {},
  pending: 0,
  processing: 0,
  completed: 0,
  closed: 0,
}

function formatDateTime(value?: string) {
  if (!value) return '-'
  const parsed = dayjs(value)
  return parsed.isValid() ? parsed.format('YYYY-MM-DD HH:mm') : value
}

function statusTag(status: PerformanceInterviewStatus) {
  const meta = INTERVIEW_STATUS_META[status] || { label: status, color: 'default' }
  return <Tag color={meta.color}>{meta.label}</Tag>
}

function levelTag(level?: string) {
  if (!level) return '-'
  const colorMap: Record<string, string> = { S: 'purple', A: 'green', B: 'blue', C: 'orange', D: 'red' }
  return <Tag color={colorMap[level] || 'default'}>{level}</Tag>
}

const PerformanceInterviews: React.FC = () => {
  const [filterForm] = Form.useForm<FilterValues>()
  const [interviewForm] = Form.useForm<InterviewFormValues>()
  const { permissions, user } = useAuthStore()
  const [activities, setActivities] = useState<PerformanceActivity[]>([])
  const [participants, setParticipants] = useState<PerformanceParticipant[]>([])
  const [items, setItems] = useState<PerformanceInterviewRecord[]>([])
  const [summary, setSummary] = useState<PerformanceFollowupSummary>(emptySummary)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [loading, setLoading] = useState(false)
  const [baseLoading, setBaseLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingRecord, setEditingRecord] = useState<PerformanceInterviewRecord | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const canManage = useMemo(() => {
    return user?.user_id === 'admin' ||
      permissions.includes('performance:interview:manage') ||
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
      setActivities(unwrapApiItems<PerformanceActivity>(res))
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
      setParticipants(unwrapApiItems<PerformanceParticipant>(res))
    } catch (error) {
      setParticipants([])
      message.error('加载参与人失败')
    }
  }

  const loadRecords = async (nextPage = page, nextPageSize = pageSize) => {
    const values = filterForm.getFieldsValue()
    try {
      setLoading(true)
      const res = await performanceAPI.getPerformanceInterviews({
        page: nextPage,
        page_size: nextPageSize,
        activity_id: values.activity_id,
        status: values.status,
        employee_keyword: values.employee_keyword?.trim(),
      })
      const data = unwrapApiData<PerformanceFollowupListResponse<PerformanceInterviewRecord>>(res, {
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
      message.error('加载绩效面谈失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadActivities()
    loadRecords(1, pageSize)
  }, [])

  const openCreateModal = () => {
    setEditingRecord(null)
    setParticipants([])
    interviewForm.resetFields()
    interviewForm.setFieldsValue({ interview_type: 'required', status: 'scheduled' })
    setModalOpen(true)
  }

  const openEditModal = async (record: PerformanceInterviewRecord) => {
    setEditingRecord(record)
    const activityId = Number(record.activity_id)
    await loadParticipants(activityId)
    interviewForm.setFieldsValue({
      activity_id: activityId,
      participant_id: record.participant_id,
      interview_type: record.interview_type,
      status: record.status,
      interviewer_id: record.interviewer_id,
      interviewer_name: record.interviewer_name,
      scheduled_at: record.scheduled_at ? dayjs(record.scheduled_at) : undefined,
      location: record.location,
      summary: record.summary,
      result: record.result,
      cancel_reason: record.cancel_reason,
    })
    setModalOpen(true)
  }

  const handleSubmit = async () => {
    try {
      const values = await interviewForm.validateFields()
      setSubmitting(true)
      const payload = {
        participant_id: values.participant_id,
        interview_type: values.interview_type,
        status: values.status,
        interviewer_id: values.interviewer_id?.trim(),
        interviewer_name: values.interviewer_name?.trim(),
        scheduled_at: values.scheduled_at?.toISOString(),
        location: values.location?.trim(),
        summary: values.summary?.trim(),
        result: values.result?.trim(),
        cancel_reason: values.cancel_reason?.trim(),
      }
      if (editingRecord) {
        await performanceAPI.updatePerformanceInterview(editingRecord.id, payload)
      } else {
        await performanceAPI.createPerformanceInterview(payload)
      }
      message.success('保存绩效面谈成功')
      setModalOpen(false)
      await loadRecords(page, pageSize)
    } catch (error) {
      if ((error as any)?.errorFields) return
      message.error('保存绩效面谈失败')
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<PerformanceInterviewRecord> = [
    { title: '活动', dataIndex: 'activity_name', width: 180, ellipsis: true },
    { title: '员工', dataIndex: 'employee_name', width: 120 },
    { title: '部门', dataIndex: 'department_name', width: 140, ellipsis: true },
    { title: '岗位', dataIndex: 'position', width: 120, ellipsis: true },
    { title: '等级', dataIndex: 'final_level', width: 80, render: levelTag },
    { title: '类型', dataIndex: 'interview_type', width: 90, render: (value: PerformanceInterviewType) => INTERVIEW_TYPE_LABELS[value] || value },
    { title: '状态', dataIndex: 'status', width: 100, render: statusTag },
    { title: '面谈人', dataIndex: 'interviewer_name', width: 120, render: value => value || '-' },
    { title: '计划时间', dataIndex: 'scheduled_at', width: 150, render: formatDateTime },
    { title: '完成时间', dataIndex: 'completed_at', width: 150, render: formatDateTime },
    {
      title: '操作',
      key: 'actions',
      width: 110,
      fixed: 'right',
      render: (_, record) => canManage ? (
        <Button size="small" icon={<EditOutlined />} onClick={() => openEditModal(record)}>编辑</Button>
      ) : '-',
    },
  ]

  return (
    <PageContainer
      title="绩效面谈"
      subtitle="结果公布后的独立面谈记录，不影响绩效活动主流程状态"
      icon={<FileTextOutlined />}
      extra={canManage ? <Button type="primary" icon={<PlusOutlined />} onClick={openCreateModal}>安排面谈</Button> : null}
    >
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Row gutter={[16, 16]}>
          <Col xs={12} md={6}><PageCard><Statistic title="面谈总数" value={summary.total} /></PageCard></Col>
          <Col xs={12} md={6}><PageCard><Statistic title="待安排" value={summary.pending} /></PageCard></Col>
          <Col xs={12} md={6}><PageCard><Statistic title="待面谈" value={summary.processing} /></PageCard></Col>
          <Col xs={12} md={6}><PageCard><Statistic title="已完成" value={summary.completed} /></PageCard></Col>
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
                options={Object.entries(INTERVIEW_STATUS_META).map(([value, meta]) => ({ value, label: meta.label }))}
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
            locale={{ emptyText: <Empty description="暂无绩效面谈记录" /> }}
            scroll={{ x: 1320 }}
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
        title={editingRecord ? '编辑绩效面谈' : '安排绩效面谈'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={handleSubmit}
        confirmLoading={submitting}
        width={760}
        destroyOnHidden
      >
        <Form form={interviewForm} layout="vertical">
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="activity_id" label="绩效活动" rules={[{ required: true, message: '请选择绩效活动' }]}>
                <Select
                  disabled={Boolean(editingRecord)}
                  showSearch
                  options={activityOptions}
                  optionFilterProp="label"
                  onChange={(value: number) => {
                    interviewForm.setFieldValue('participant_id', undefined)
                    loadParticipants(value)
                  }}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="participant_id" label="员工" rules={[{ required: true, message: '请选择员工' }]}>
                <Select disabled={Boolean(editingRecord)} showSearch options={participantOptions} optionFilterProp="label" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="interview_type" label="面谈类型" rules={[{ required: true, message: '请选择面谈类型' }]}>
                <Select options={[
                  { value: 'required', label: '必谈' },
                  { value: 'optional', label: '选谈' },
                ]} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="status" label="状态" rules={[{ required: true, message: '请选择状态' }]}>
                <Select options={Object.entries(INTERVIEW_STATUS_META).map(([value, meta]) => ({ value, label: meta.label }))} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="interviewer_name" label="面谈人">
                <Input placeholder="填写面谈负责人" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="scheduled_at" label="计划时间">
                <DatePicker showTime style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={24}>
              <Form.Item name="location" label="面谈地点">
                <Input placeholder="会议室、线上会议链接等" />
              </Form.Item>
            </Col>
            <Col span={24}>
              <Form.Item name="summary" label="面谈安排/摘要">
                <Input.TextArea rows={3} placeholder="填写面谈重点、准备事项或过程摘要" />
              </Form.Item>
            </Col>
            <Col span={24}>
              <Form.Item name="result" label="面谈结果">
                <Input.TextArea rows={4} placeholder="状态为已完成时填写面谈结论、改进计划或共识" />
              </Form.Item>
            </Col>
            <Col span={24}>
              <Form.Item name="cancel_reason" label="取消原因">
                <Input.TextArea rows={2} placeholder="状态为已取消时填写" />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
    </PageContainer>
  )
}

export default PerformanceInterviews
