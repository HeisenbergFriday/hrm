import React, { useMemo, useState } from 'react'
import type { Dayjs } from 'dayjs'
import {
  Alert,
  Button,
  Col,
  DatePicker,
  Descriptions,
  Form,
  Input,
  Modal,
  Row,
  Select,
  Spin,
  Table,
  Tabs,
  Typography,
  message,
} from 'antd'
import { PlusOutlined, ReloadOutlined, SwapOutlined, UserAddOutlined, UserDeleteOutlined } from '@ant-design/icons'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import { useMutation, useQuery } from '@tanstack/react-query'
import { departmentAPI, employeeAPI } from '../services/api'

const { Title, Paragraph, Text } = Typography
const { TextArea } = Input

type FlowTabKey = 'onboarding' | 'transfer' | 'resignation'

interface DepartmentItem {
  department_id: string
  name: string
}

interface TransferRecord {
  id: number | string
  transfer_id: string
  user_id: string
  user_name: string
  old_department_id: string
  old_department_name: string
  old_position: string
  new_department_id: string
  new_department_name: string
  new_position: string
  transfer_date: string
  reason?: string
  status?: string
  approver_name?: string
  approval_time?: string
  approval_comment?: string
}

interface ResignationRecord {
  id: number | string
  resignation_id: string
  user_id: string
  user_name: string
  department_id: string
  department_name: string
  position: string
  resign_date: string
  last_working_day: string
  resign_reason?: string
  status?: string
  approver_name?: string
  approval_time?: string
  approval_comment?: string
}

interface OnboardingRecord {
  id: number | string
  onboarding_id: string
  employee_id: string
  name: string
  gender?: string
  birth_date?: string
  id_card_number?: string
  mobile?: string
  email?: string
  department_id: string
  department_name: string
  position: string
  entry_date: string
  employment_type: string
  probation_end_date?: string
  emergency_contact?: string
  emergency_phone?: string
  education?: string
  graduate_school?: string
  major?: string
  status?: string
}

interface DetailState {
  type: FlowTabKey
  record: TransferRecord | ResignationRecord | OnboardingRecord
}

interface TransferFormValues {
  user_id: string
  user_name: string
  old_department_id: string
  old_position: string
  new_department_id: string
  new_position: string
  transfer_date: Dayjs | null
  reason?: string
}

interface ResignationFormValues {
  user_id: string
  user_name: string
  department_id: string
  position: string
  resign_date: Dayjs | null
  last_working_day: Dayjs | null
  resign_reason?: string
}

interface OnboardingFormValues {
  employee_id: string
  name: string
  gender?: string
  birth_date?: Dayjs | null
  id_card_number?: string
  mobile?: string
  email?: string
  department_id: string
  position: string
  entry_date: Dayjs | null
  employment_type: string
  probation_end_date?: Dayjs | null
  emergency_contact?: string
  emergency_phone?: string
  education?: string
  graduate_school?: string
  major?: string
}

type FlowFormValues = TransferFormValues & ResignationFormValues & OnboardingFormValues

const trimText = (value?: string) => (typeof value === 'string' ? value.trim() : '')
const formatDate = (value?: Dayjs | null) => (value ? value.format('YYYY-MM-DD') : '')

const renderStatusTag = (value?: string) => {
  const statusMap: Record<string, { color: string; label: string }> = {
    pending: { color: 'blue', label: '待处理' },
    approved: { color: 'success', label: '已批准' },
    rejected: { color: 'error', label: '已驳回' },
    processing: { color: 'warning', label: '处理中' },
    completed: { color: 'success', label: '已完成' },
  }
  const matched = statusMap[value || '']
  if (!matched) {
    return <StatusTag>{value || '未设置'}</StatusTag>
  }
  return <StatusTag color={matched.color}>{matched.label}</StatusTag>
}

const employmentTypeOptions = ['全职', '兼职', '实习']
const educationOptions = ['高中', '大专', '本科', '硕士', '博士']

const EmployeeFlow: React.FC = () => {
  const [activeTab, setActiveTab] = useState<FlowTabKey>('onboarding')
  const [createModalOpen, setCreateModalOpen] = useState(false)
  const [detailState, setDetailState] = useState<DetailState | null>(null)
  const [form] = Form.useForm<FlowFormValues>()

  const departmentsQuery = useQuery({
    queryKey: ['employee-flow-departments'],
    queryFn: () => departmentAPI.getDepartments(),
    staleTime: 60_000,
  })

  const transfersQuery = useQuery({
    queryKey: ['employee-flow-transfers'],
    queryFn: () => employeeAPI.getTransfers({ page: 1, page_size: 1000 }),
    enabled: activeTab === 'transfer',
  })

  const resignationsQuery = useQuery({
    queryKey: ['employee-flow-resignations'],
    queryFn: () => employeeAPI.getResignations({ page: 1, page_size: 1000 }),
    enabled: activeTab === 'resignation',
  })

  const onboardingsQuery = useQuery({
    queryKey: ['employee-flow-onboardings'],
    queryFn: () => employeeAPI.getOnboardings({ page: 1, page_size: 1000 }),
    enabled: activeTab === 'onboarding',
  })

  const departments = (departmentsQuery.data?.data?.departments ?? []) as DepartmentItem[]
  const departmentNameMap = useMemo(() => {
    const result: Record<string, string> = {}
    departments.forEach((department) => {
      result[department.department_id] = department.name
    })
    return result
  }, [departments])

  const departmentOptions = useMemo(
    () =>
      departments.map((department) => ({
        label: department.name,
        value: department.department_id,
      })),
    [departments],
  )

  const refetchCurrentList = () => {
    if (activeTab === 'transfer') {
      void transfersQuery.refetch()
      return
    }
    if (activeTab === 'resignation') {
      void resignationsQuery.refetch()
      return
    }
    void onboardingsQuery.refetch()
  }

  const resetModalState = () => {
    setCreateModalOpen(false)
    form.resetFields()
  }

  const createTransferMutation = useMutation({
    mutationFn: (payload: Record<string, string>) => employeeAPI.createTransfer(payload),
    onSuccess: () => {
      message.success('调岗记录创建成功')
      resetModalState()
      void transfersQuery.refetch()
    },
    onError: () => {
      message.error('调岗记录创建失败')
    },
  })

  const createResignationMutation = useMutation({
    mutationFn: (payload: Record<string, string>) => employeeAPI.createResignation(payload),
    onSuccess: () => {
      message.success('离职记录创建成功')
      resetModalState()
      void resignationsQuery.refetch()
    },
    onError: () => {
      message.error('离职记录创建失败')
    },
  })

  const createOnboardingMutation = useMutation({
    mutationFn: (payload: Record<string, string>) => employeeAPI.createOnboarding(payload),
    onSuccess: () => {
      message.success('入职记录创建成功')
      resetModalState()
      void onboardingsQuery.refetch()
    },
    onError: () => {
      message.error('入职记录创建失败')
    },
  })

  const buildTransferPayload = (values: TransferFormValues) => ({
    user_id: trimText(values.user_id),
    user_name: trimText(values.user_name),
    old_department_id: values.old_department_id,
    old_department_name: departmentNameMap[values.old_department_id] || values.old_department_id,
    old_position: trimText(values.old_position),
    new_department_id: values.new_department_id,
    new_department_name: departmentNameMap[values.new_department_id] || values.new_department_id,
    new_position: trimText(values.new_position),
    transfer_date: formatDate(values.transfer_date),
    reason: trimText(values.reason),
  })

  const buildResignationPayload = (values: ResignationFormValues) => ({
    user_id: trimText(values.user_id),
    user_name: trimText(values.user_name),
    department_id: values.department_id,
    department_name: departmentNameMap[values.department_id] || values.department_id,
    position: trimText(values.position),
    resign_date: formatDate(values.resign_date),
    last_working_day: formatDate(values.last_working_day),
    resign_reason: trimText(values.resign_reason),
  })

  const buildOnboardingPayload = (values: OnboardingFormValues) => ({
    employee_id: trimText(values.employee_id),
    name: trimText(values.name),
    gender: trimText(values.gender),
    birth_date: formatDate(values.birth_date),
    id_card_number: trimText(values.id_card_number),
    mobile: trimText(values.mobile),
    email: trimText(values.email),
    department_id: values.department_id,
    department_name: departmentNameMap[values.department_id] || values.department_id,
    position: trimText(values.position),
    entry_date: formatDate(values.entry_date),
    employment_type: trimText(values.employment_type),
    probation_end_date: formatDate(values.probation_end_date),
    emergency_contact: trimText(values.emergency_contact),
    emergency_phone: trimText(values.emergency_phone),
    education: trimText(values.education),
    graduate_school: trimText(values.graduate_school),
    major: trimText(values.major),
  })

  const handleCreate = async () => {
    const values = await form.validateFields()
    if (activeTab === 'transfer') {
      createTransferMutation.mutate(buildTransferPayload(values))
      return
    }
    if (activeTab === 'resignation') {
      createResignationMutation.mutate(buildResignationPayload(values))
      return
    }
    createOnboardingMutation.mutate(buildOnboardingPayload(values))
  }

  const handleTabChange = (key: string) => {
    setActiveTab(key as FlowTabKey)
    setDetailState(null)
    resetModalState()
  }

  const transferColumns = [
    {
      title: '调岗单号',
      dataIndex: 'transfer_id',
      key: 'transfer_id',
    },
    {
      title: '员工',
      key: 'employee',
      render: (_: unknown, record: TransferRecord) => (
        <div>
          <Text strong>{record.user_name}</Text>
          <div style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)' }}>{record.user_id}</div>
        </div>
      ),
    },
    {
      title: '原部门 / 岗位',
      key: 'old_info',
      render: (_: unknown, record: TransferRecord) => `${record.old_department_name || '-'} / ${record.old_position || '-'}`,
    },
    {
      title: '新部门 / 岗位',
      key: 'new_info',
      render: (_: unknown, record: TransferRecord) => `${record.new_department_name || '-'} / ${record.new_position || '-'}`,
    },
    {
      title: '调岗日期',
      dataIndex: 'transfer_date',
      key: 'transfer_date',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (value?: string) => renderStatusTag(value),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: TransferRecord) => (
        <Button type="link" onClick={() => setDetailState({ type: 'transfer', record })}>
          查看
        </Button>
      ),
    },
  ]

  const resignationColumns = [
    {
      title: '离职单号',
      dataIndex: 'resignation_id',
      key: 'resignation_id',
    },
    {
      title: '员工',
      key: 'employee',
      render: (_: unknown, record: ResignationRecord) => (
        <div>
          <Text strong>{record.user_name}</Text>
          <div style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)' }}>{record.user_id}</div>
        </div>
      ),
    },
    {
      title: '部门 / 岗位',
      key: 'department',
      render: (_: unknown, record: ResignationRecord) => `${record.department_name || '-'} / ${record.position || '-'}`,
    },
    {
      title: '离职日期',
      dataIndex: 'resign_date',
      key: 'resign_date',
    },
    {
      title: '最后工作日',
      dataIndex: 'last_working_day',
      key: 'last_working_day',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (value?: string) => renderStatusTag(value),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: ResignationRecord) => (
        <Button type="link" onClick={() => setDetailState({ type: 'resignation', record })}>
          查看
        </Button>
      ),
    },
  ]

  const onboardingColumns = [
    {
      title: '入职单号',
      dataIndex: 'onboarding_id',
      key: 'onboarding_id',
    },
    {
      title: '员工',
      key: 'employee',
      render: (_: unknown, record: OnboardingRecord) => (
        <div>
          <Text strong>{record.name}</Text>
          <div style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)' }}>{record.employee_id}</div>
        </div>
      ),
    },
    {
      title: '部门 / 岗位',
      key: 'department',
      render: (_: unknown, record: OnboardingRecord) => `${record.department_name || '-'} / ${record.position || '-'}`,
    },
    {
      title: '入职日期',
      dataIndex: 'entry_date',
      key: 'entry_date',
    },
    {
      title: '用工类型',
      dataIndex: 'employment_type',
      key: 'employment_type',
      render: (value?: string) => value || '-',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (value?: string) => renderStatusTag(value),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: OnboardingRecord) => (
        <Button type="link" onClick={() => setDetailState({ type: 'onboarding', record })}>
          查看
        </Button>
      ),
    },
  ]

  const renderListContent = () => {
    if (activeTab === 'transfer') {
      if (transfersQuery.isLoading) {
        return (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}>
            <Spin size="large" />
          </div>
        )
      }
      if (transfersQuery.isError) {
        return (
          <Alert
            type="error"
            showIcon
            message="调岗记录加载失败"
            action={
              <Button size="small" onClick={() => void transfersQuery.refetch()}>
                重试
              </Button>
            }
          />
        )
      }
      return (
        <Table<TransferRecord>
          rowKey="id"
          columns={transferColumns}
          dataSource={(transfersQuery.data?.data?.items ?? []) as TransferRecord[]}
          locale={{ emptyText: '暂无调岗记录' }}
          pagination={false}
        />
      )
    }

    if (activeTab === 'resignation') {
      if (resignationsQuery.isLoading) {
        return (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}>
            <Spin size="large" />
          </div>
        )
      }
      if (resignationsQuery.isError) {
        return (
          <Alert
            type="error"
            showIcon
            message="离职记录加载失败"
            action={
              <Button size="small" onClick={() => void resignationsQuery.refetch()}>
                重试
              </Button>
            }
          />
        )
      }
      return (
        <Table<ResignationRecord>
          rowKey="id"
          columns={resignationColumns}
          dataSource={(resignationsQuery.data?.data?.items ?? []) as ResignationRecord[]}
          locale={{ emptyText: '暂无离职记录' }}
          pagination={false}
        />
      )
    }

    if (onboardingsQuery.isLoading) {
      return (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}>
          <Spin size="large" />
        </div>
      )
    }
    if (onboardingsQuery.isError) {
      return (
        <Alert
          type="error"
          showIcon
          message="入职记录加载失败"
          action={
            <Button size="small" onClick={() => void onboardingsQuery.refetch()}>
              重试
            </Button>
          }
        />
      )
    }
    return (
      <Table<OnboardingRecord>
        rowKey="id"
        columns={onboardingColumns}
        dataSource={(onboardingsQuery.data?.data?.items ?? []) as OnboardingRecord[]}
        locale={{ emptyText: '暂无入职记录' }}
        pagination={false}
      />
    )
  }

  const renderDetailContent = () => {
    if (!detailState) {
      return null
    }

    if (detailState.type === 'transfer') {
      const record = detailState.record as TransferRecord
      return (
        <Descriptions column={2} bordered size="small">
          <Descriptions.Item label="调岗单号">{record.transfer_id}</Descriptions.Item>
          <Descriptions.Item label="状态">{renderStatusTag(record.status)}</Descriptions.Item>
          <Descriptions.Item label="员工姓名">{record.user_name}</Descriptions.Item>
          <Descriptions.Item label="员工 ID">{record.user_id}</Descriptions.Item>
          <Descriptions.Item label="原部门">{record.old_department_name || '-'}</Descriptions.Item>
          <Descriptions.Item label="原岗位">{record.old_position || '-'}</Descriptions.Item>
          <Descriptions.Item label="新部门">{record.new_department_name || '-'}</Descriptions.Item>
          <Descriptions.Item label="新岗位">{record.new_position || '-'}</Descriptions.Item>
          <Descriptions.Item label="调岗日期">{record.transfer_date || '-'}</Descriptions.Item>
          <Descriptions.Item label="审批人">{record.approver_name || '-'}</Descriptions.Item>
          <Descriptions.Item label="审批时间" span={2}>
            {record.approval_time || '-'}
          </Descriptions.Item>
          <Descriptions.Item label="原因" span={2}>
            {record.reason || '-'}
          </Descriptions.Item>
          <Descriptions.Item label="审批意见" span={2}>
            {record.approval_comment || '-'}
          </Descriptions.Item>
        </Descriptions>
      )
    }

    if (detailState.type === 'resignation') {
      const record = detailState.record as ResignationRecord
      return (
        <Descriptions column={2} bordered size="small">
          <Descriptions.Item label="离职单号">{record.resignation_id}</Descriptions.Item>
          <Descriptions.Item label="状态">{renderStatusTag(record.status)}</Descriptions.Item>
          <Descriptions.Item label="员工姓名">{record.user_name}</Descriptions.Item>
          <Descriptions.Item label="员工 ID">{record.user_id}</Descriptions.Item>
          <Descriptions.Item label="部门">{record.department_name || '-'}</Descriptions.Item>
          <Descriptions.Item label="岗位">{record.position || '-'}</Descriptions.Item>
          <Descriptions.Item label="离职日期">{record.resign_date || '-'}</Descriptions.Item>
          <Descriptions.Item label="最后工作日">{record.last_working_day || '-'}</Descriptions.Item>
          <Descriptions.Item label="审批人">{record.approver_name || '-'}</Descriptions.Item>
          <Descriptions.Item label="审批时间">{record.approval_time || '-'}</Descriptions.Item>
          <Descriptions.Item label="离职原因" span={2}>
            {record.resign_reason || '-'}
          </Descriptions.Item>
          <Descriptions.Item label="审批意见" span={2}>
            {record.approval_comment || '-'}
          </Descriptions.Item>
        </Descriptions>
      )
    }

    const record = detailState.record as OnboardingRecord
    return (
      <Descriptions column={2} bordered size="small">
        <Descriptions.Item label="入职单号">{record.onboarding_id}</Descriptions.Item>
        <Descriptions.Item label="状态">{renderStatusTag(record.status)}</Descriptions.Item>
        <Descriptions.Item label="员工姓名">{record.name}</Descriptions.Item>
        <Descriptions.Item label="档案工号">{record.employee_id}</Descriptions.Item>
        <Descriptions.Item label="部门">{record.department_name || '-'}</Descriptions.Item>
        <Descriptions.Item label="岗位">{record.position || '-'}</Descriptions.Item>
        <Descriptions.Item label="入职日期">{record.entry_date || '-'}</Descriptions.Item>
        <Descriptions.Item label="用工类型">{record.employment_type || '-'}</Descriptions.Item>
        <Descriptions.Item label="试用期结束日期">{record.probation_end_date || '-'}</Descriptions.Item>
        <Descriptions.Item label="性别">{record.gender || '-'}</Descriptions.Item>
        <Descriptions.Item label="手机号">{record.mobile || '-'}</Descriptions.Item>
        <Descriptions.Item label="邮箱">{record.email || '-'}</Descriptions.Item>
        <Descriptions.Item label="紧急联系人">{record.emergency_contact || '-'}</Descriptions.Item>
        <Descriptions.Item label="紧急联系电话">{record.emergency_phone || '-'}</Descriptions.Item>
        <Descriptions.Item label="学历">{record.education || '-'}</Descriptions.Item>
        <Descriptions.Item label="毕业院校">{record.graduate_school || '-'}</Descriptions.Item>
        <Descriptions.Item label="专业" span={2}>
          {record.major || '-'}
        </Descriptions.Item>
      </Descriptions>
    )
  }

  const createTitleMap: Record<FlowTabKey, string> = {
    onboarding: '新建入职记录',
    transfer: '新建调岗记录',
    resignation: '新建离职记录',
  }

  return (
    <PageContainer
      title="入转调离"
      subtitle="本页只保留查询与新建。状态、审批人、审批时间、审批意见以及流程结果字段仅用于展示，不会进入创建 payload。"
    >
      <Alert
        style={{ marginBottom: 16 }}
        type="info"
        showIcon
        message="入职 / 调岗 / 离职均不提供编辑和删除。创建 payload 只提交各自台账 struct 的字段；不会提交 status、approver_id、approver_name、approval_time、approval_comment、onboarding_process、exit_process。"
      />

      <Tabs activeKey={activeTab} onChange={handleTabChange}>
        <Tabs.TabPane tab="入职" key="onboarding" icon={<UserAddOutlined />}>
          <PageCard
            extra={
              <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                <Button icon={<ReloadOutlined />} onClick={refetchCurrentList} loading={onboardingsQuery.isFetching}>
                  刷新
                </Button>
                <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalOpen(true)}>
                  新建入职
                </Button>
              </div>
            }
          >
            {renderListContent()}
          </PageCard>
        </Tabs.TabPane>
        <Tabs.TabPane tab="调岗" key="transfer" icon={<SwapOutlined />}>
          <PageCard
            extra={
              <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                <Button icon={<ReloadOutlined />} onClick={refetchCurrentList} loading={transfersQuery.isFetching}>
                  刷新
                </Button>
                <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalOpen(true)}>
                  新建调岗
                </Button>
              </div>
            }
          >
            {renderListContent()}
          </PageCard>
        </Tabs.TabPane>
        <Tabs.TabPane tab="离职" key="resignation" icon={<UserDeleteOutlined />}>
          <PageCard
            extra={
              <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                <Button icon={<ReloadOutlined />} onClick={refetchCurrentList} loading={resignationsQuery.isFetching}>
                  刷新
                </Button>
                <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalOpen(true)}>
                  新建离职
                </Button>
              </div>
            }
          >
            {renderListContent()}
          </PageCard>
        </Tabs.TabPane>
      </Tabs>

      <Modal
        title={createTitleMap[activeTab]}
        open={createModalOpen}
        onCancel={resetModalState}
        onOk={() => void handleCreate()}
        okText="保存"
        cancelText="取消"
        width={820}
        destroyOnClose
        confirmLoading={
          createTransferMutation.isPending || createResignationMutation.isPending || createOnboardingMutation.isPending
        }
      >
        <Form form={form} layout="vertical">
          {activeTab === 'transfer' ? (
            <Row gutter={16}>
              <Col xs={24} md={12}>
                <Form.Item name="user_id" label="员工 ID" rules={[{ required: true, message: '请输入员工 ID' }]}>
                  <Input placeholder="请输入员工 ID" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="user_name" label="员工姓名" rules={[{ required: true, message: '请输入员工姓名' }]}>
                  <Input placeholder="请输入员工姓名" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="old_department_id" label="原部门" rules={[{ required: true, message: '请选择原部门' }]}>
                  <Select showSearch optionFilterProp="label" options={departmentOptions} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="old_position" label="原岗位" rules={[{ required: true, message: '请输入原岗位' }]}>
                  <Input placeholder="请输入原岗位" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="new_department_id" label="新部门" rules={[{ required: true, message: '请选择新部门' }]}>
                  <Select showSearch optionFilterProp="label" options={departmentOptions} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="new_position" label="新岗位" rules={[{ required: true, message: '请输入新岗位' }]}>
                  <Input placeholder="请输入新岗位" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="transfer_date" label="调岗日期" rules={[{ required: true, message: '请选择调岗日期' }]}>
                  <DatePicker style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col span={24}>
                <Form.Item name="reason" label="调岗原因" rules={[{ required: true, message: '请输入调岗原因' }]}>
                  <TextArea rows={4} placeholder="请输入调岗原因" />
                </Form.Item>
              </Col>
            </Row>
          ) : null}

          {activeTab === 'resignation' ? (
            <Row gutter={16}>
              <Col xs={24} md={12}>
                <Form.Item name="user_id" label="员工 ID" rules={[{ required: true, message: '请输入员工 ID' }]}>
                  <Input placeholder="请输入员工 ID" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="user_name" label="员工姓名" rules={[{ required: true, message: '请输入员工姓名' }]}>
                  <Input placeholder="请输入员工姓名" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="department_id" label="部门" rules={[{ required: true, message: '请选择部门' }]}>
                  <Select showSearch optionFilterProp="label" options={departmentOptions} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="position" label="岗位" rules={[{ required: true, message: '请输入岗位' }]}>
                  <Input placeholder="请输入岗位" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="resign_date" label="离职日期" rules={[{ required: true, message: '请选择离职日期' }]}>
                  <DatePicker style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item
                  name="last_working_day"
                  label="最后工作日"
                  rules={[{ required: true, message: '请选择最后工作日' }]}
                >
                  <DatePicker style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col span={24}>
                <Form.Item name="resign_reason" label="离职原因" rules={[{ required: true, message: '请输入离职原因' }]}>
                  <TextArea rows={4} placeholder="请输入离职原因" />
                </Form.Item>
              </Col>
            </Row>
          ) : null}

          {activeTab === 'onboarding' ? (
            <Row gutter={16}>
              <Col xs={24} md={12}>
                <Form.Item name="employee_id" label="档案工号" rules={[{ required: true, message: '请输入档案工号' }]}>
                  <Input placeholder="请输入档案工号" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="name" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}>
                  <Input placeholder="请输入姓名" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="gender" label="性别">
                  <Select
                    allowClear
                    options={[
                      { label: '男', value: '男' },
                      { label: '女', value: '女' },
                    ]}
                  />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="birth_date" label="出生日期">
                  <DatePicker style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="id_card_number" label="身份证号">
                  <Input placeholder="请输入身份证号" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="mobile" label="手机号">
                  <Input placeholder="请输入手机号" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="email" label="邮箱">
                  <Input placeholder="请输入邮箱" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="department_id" label="部门" rules={[{ required: true, message: '请选择部门' }]}>
                  <Select showSearch optionFilterProp="label" options={departmentOptions} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="position" label="岗位" rules={[{ required: true, message: '请输入岗位' }]}>
                  <Input placeholder="请输入岗位" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="entry_date" label="入职日期" rules={[{ required: true, message: '请选择入职日期' }]}>
                  <DatePicker style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item
                  name="employment_type"
                  label="用工类型"
                  rules={[{ required: true, message: '请选择用工类型' }]}
                >
                  <Select allowClear options={employmentTypeOptions.map((item) => ({ label: item, value: item }))} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="probation_end_date" label="试用期结束日期">
                  <DatePicker style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="emergency_contact" label="紧急联系人">
                  <Input placeholder="请输入紧急联系人" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="emergency_phone" label="紧急联系电话">
                  <Input placeholder="请输入紧急联系电话" />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="education" label="学历">
                  <Select allowClear options={educationOptions.map((item) => ({ label: item, value: item }))} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="graduate_school" label="毕业院校">
                  <Input placeholder="请输入毕业院校" />
                </Form.Item>
              </Col>
              <Col span={24}>
                <Form.Item name="major" label="专业">
                  <Input placeholder="请输入专业" />
                </Form.Item>
              </Col>
            </Row>
          ) : null}
        </Form>
      </Modal>

      <Modal
        title="台账详情"
        open={Boolean(detailState)}
        onCancel={() => setDetailState(null)}
        footer={[
          <Button key="close" onClick={() => setDetailState(null)}>
            关闭
          </Button>,
        ]}
        width={760}
      >
        {renderDetailContent()}
      </Modal>
    </PageContainer>
  )
}

export default EmployeeFlow
