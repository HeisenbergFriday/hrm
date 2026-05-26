import React, { useMemo, useState } from 'react'
import type { Dayjs } from 'dayjs'
import dayjs from 'dayjs'
import {
  Alert,
  Button,
  Col,
  DatePicker,
  Form,
  Input,
  Modal,
  Row,
  Select,
  Spin,
  Table,
  Typography,
  message,
} from 'antd'
import { EditOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import { useMutation, useQuery } from '@tanstack/react-query'
import { departmentAPI, employeeAPI, orgAPI } from '../services/api'

const { Title, Text, Paragraph } = Typography
const { TextArea } = Input

interface EmployeeProfileRecord {
  id: number | string
  user_id: string
  employee_id: string
  gender?: string
  birth_date?: string
  nationality?: string
  id_card_number?: string
  employment_type?: string
  entry_date?: string
  probation_end_date?: string
  planned_regular_date?: string
  actual_regular_date?: string
  job_level?: string
  job_family?: string
  contract_start_date?: string
  contract_end_date?: string
  work_email?: string
  personal_email?: string
  emergency_contact?: string
  emergency_phone?: string
  education?: string
  graduate_school?: string
  major?: string
  graduation_date?: string
  bank_account?: string
  bank_name?: string
  tax_number?: string
  address?: string
  profile_status?: string
}

interface EmployeeItem {
  id: number
  user_id: string
  name: string
  department_id: string
  position: string
  status: string
}

interface DepartmentItem {
  department_id: string
  name: string
}

interface ProfileFormValues {
  user_id: string
  employee_id: string
  gender?: string
  birth_date?: Dayjs | null
  nationality?: string
  id_card_number?: string
  employment_type?: string
  entry_date?: Dayjs | null
  probation_end_date?: Dayjs | null
  planned_regular_date?: Dayjs | null
  actual_regular_date?: Dayjs | null
  job_level?: string
  job_family?: string
  contract_start_date?: Dayjs | null
  contract_end_date?: Dayjs | null
  work_email?: string
  personal_email?: string
  emergency_contact?: string
  emergency_phone?: string
  education?: string
  graduate_school?: string
  major?: string
  graduation_date?: Dayjs | null
  bank_account?: string
  bank_name?: string
  tax_number?: string
  address?: string
}

const employmentTypeOptions = ['全职', '兼职', '实习', '劳务']
const educationOptions = ['高中', '大专', '本科', '硕士', '博士', '其他']
const jobFamilyOptions = ['管理', '专业', '技术']
const profileDateFields = [
  'birth_date',
  'entry_date',
  'probation_end_date',
  'planned_regular_date',
  'actual_regular_date',
  'contract_start_date',
  'contract_end_date',
  'graduation_date',
] as const

const trimText = (value?: string) => (typeof value === 'string' ? value.trim() : '')
const formatDate = (value?: Dayjs | null) => (value ? value.format('YYYY-MM-DD') : '')

const buildProfilePayload = (values: ProfileFormValues) => ({
  user_id: trimText(values.user_id),
  employee_id: trimText(values.employee_id),
  gender: trimText(values.gender),
  birth_date: formatDate(values.birth_date),
  nationality: trimText(values.nationality),
  id_card_number: trimText(values.id_card_number),
  employment_type: trimText(values.employment_type),
  entry_date: formatDate(values.entry_date),
  probation_end_date: formatDate(values.probation_end_date),
  planned_regular_date: formatDate(values.planned_regular_date),
  actual_regular_date: formatDate(values.actual_regular_date),
  job_level: trimText(values.job_level),
  job_family: trimText(values.job_family),
  contract_start_date: formatDate(values.contract_start_date),
  contract_end_date: formatDate(values.contract_end_date),
  work_email: trimText(values.work_email),
  personal_email: trimText(values.personal_email),
  emergency_contact: trimText(values.emergency_contact),
  emergency_phone: trimText(values.emergency_phone),
  education: trimText(values.education),
  graduate_school: trimText(values.graduate_school),
  major: trimText(values.major),
  graduation_date: formatDate(values.graduation_date),
  bank_account: trimText(values.bank_account),
  bank_name: trimText(values.bank_name),
  tax_number: trimText(values.tax_number),
  address: trimText(values.address),
})

const toFormValues = (profile: EmployeeProfileRecord): ProfileFormValues => {
  const next = { ...profile } as Record<string, unknown>
  profileDateFields.forEach((field) => {
    const value = profile[field]
    next[field] = value ? dayjs(value) : null
  })
  return next as unknown as ProfileFormValues
}

const EmployeeProfilePage: React.FC = () => {
  const [modalOpen, setModalOpen] = useState(false)
  const [editingProfile, setEditingProfile] = useState<EmployeeProfileRecord | null>(null)
  const [form] = Form.useForm<ProfileFormValues>()
  const selectedUserID = Form.useWatch('user_id', form)

  const profilesQuery = useQuery({
    queryKey: ['employee-profiles-page'],
    queryFn: () => employeeAPI.getProfiles({ page: 1, page_size: 1000 }),
  })

  const employeesQuery = useQuery({
    queryKey: ['employee-profile-org-employees'],
    queryFn: () => orgAPI.getEmployees({ page: 1, page_size: 2000 }),
    staleTime: 60_000,
  })

  const departmentsQuery = useQuery({
    queryKey: ['employee-profile-departments'],
    queryFn: () => departmentAPI.getDepartments(),
    staleTime: 60_000,
  })

  const employees = (employeesQuery.data?.data?.items ?? []) as EmployeeItem[]
  const departments = (departmentsQuery.data?.data?.departments ?? []) as DepartmentItem[]
  const profiles = (profilesQuery.data?.data?.items ?? []) as EmployeeProfileRecord[]

  const employeeByUserID = useMemo(() => {
    const result: Record<string, EmployeeItem> = {}
    employees.forEach((item) => {
      result[item.user_id] = item
    })
    return result
  }, [employees])

  const departmentNameMap = useMemo(() => {
    const result: Record<string, string> = {}
    departments.forEach((item) => {
      result[item.department_id] = item.name
    })
    return result
  }, [departments])

  const employeeOptions = useMemo(
    () =>
      [...employees]
        .sort((left, right) => left.name.localeCompare(right.name) || left.user_id.localeCompare(right.user_id))
        .map((employee) => ({
          label: `${employee.name} (${employee.user_id})`,
          value: employee.user_id,
        })),
    [employees],
  )

  const linkedEmployee = selectedUserID ? employeeByUserID[selectedUserID] : undefined

  const createMutation = useMutation({
    mutationFn: (payload: ReturnType<typeof buildProfilePayload>) => employeeAPI.createProfile(payload),
    onSuccess: () => {
      message.success('员工档案创建成功')
      setModalOpen(false)
      setEditingProfile(null)
      form.resetFields()
      void profilesQuery.refetch()
    },
    onError: () => {
      message.error('员工档案创建失败')
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, payload }: { id: number | string; payload: ReturnType<typeof buildProfilePayload> }) =>
      employeeAPI.updateProfile(String(id), payload),
    onSuccess: () => {
      message.success('员工档案更新成功')
      setModalOpen(false)
      setEditingProfile(null)
      void profilesQuery.refetch()
    },
    onError: () => {
      message.error('员工档案更新失败')
    },
  })

  const openCreateModal = () => {
    setEditingProfile(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEditModal = (profile: EmployeeProfileRecord) => {
    setEditingProfile(profile)
    form.setFieldsValue(toFormValues(profile))
    setModalOpen(true)
  }

  const closeModal = () => {
    setModalOpen(false)
    setEditingProfile(null)
    form.resetFields()
  }

  const handleSubmit = async () => {
    const values = await form.validateFields()
    const payload = buildProfilePayload(values)
    if (editingProfile) {
      updateMutation.mutate({ id: editingProfile.id, payload })
      return
    }
    createMutation.mutate(payload)
  }

  const columns = [
    {
      title: '关联员工',
      key: 'employee',
      render: (_: unknown, record: EmployeeProfileRecord) => {
        const employee = employeeByUserID[record.user_id]
        return (
          <div>
            <Text strong>{employee?.name || record.user_id}</Text>
            <div style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)' }}>{record.user_id}</div>
          </div>
        )
      },
    },
    {
      title: '档案工号',
      dataIndex: 'employee_id',
      key: 'employee_id',
    },
    {
      title: '部门',
      key: 'department',
      render: (_: unknown, record: EmployeeProfileRecord) => {
        const employee = employeeByUserID[record.user_id]
        return departmentNameMap[employee?.department_id || ''] || employee?.department_id || '-'
      },
    },
    {
      title: '岗位',
      key: 'position',
      render: (_: unknown, record: EmployeeProfileRecord) => employeeByUserID[record.user_id]?.position || '-',
    },
    {
      title: '入职日期',
      dataIndex: 'entry_date',
      key: 'entry_date',
      render: (value?: string) => value || '-',
    },
    {
      title: '档案状态',
      dataIndex: 'profile_status',
      key: 'profile_status',
      render: (value?: string) => {
        const map: Record<string, string> = { active: '在职', inactive: '离职/停用' }
        return <StatusTag color={value === 'active' ? 'success' : 'default'}>{map[value || ''] || value || '未设置'}</StatusTag>
      },
    },
    {
      title: '操作',
      key: 'action',
      render: (_: unknown, record: EmployeeProfileRecord) => (
        <Button type="link" icon={<EditOutlined />} onClick={() => openEditModal(record)}>
          编辑
        </Button>
      ),
    },
  ]

  return (
    <PageContainer
      title="员工档案"
      subtitle="仅维护 EmployeeProfile 档案字段。User.name / email / mobile / department_id / position / avatar / status 在这里保持只读，不会进入创建或编辑 payload。"
      extra={
        <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
          <Button icon={<ReloadOutlined />} onClick={() => void profilesQuery.refetch()} loading={profilesQuery.isFetching}>
            刷新
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreateModal}>
            新建档案
          </Button>
        </div>
      }
    >

      <Alert
        style={{ marginBottom: 16 }}
        type="info"
        showIcon
        message="本页不提供删除能力。user_id 只作为档案关联字段；档案编辑不会更新钉钉同步主数据，也不会提交 profile_status。"
      />

      <PageCard>
        {profilesQuery.isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}>
            <Spin size="large" />
          </div>
        ) : profilesQuery.isError ? (
          <Alert
            type="error"
            showIcon
            message="员工档案加载失败"
            action={
              <Button size="small" onClick={() => void profilesQuery.refetch()}>
                重试
              </Button>
            }
          />
        ) : (
          <Table<EmployeeProfileRecord>
            rowKey="id"
            columns={columns}
            dataSource={profiles}
            locale={{ emptyText: '暂无员工档案' }}
            pagination={false}
          />
        )}
      </PageCard>

      <Modal
        title={editingProfile ? '编辑员工档案' : '新建员工档案'}
        open={modalOpen}
        onCancel={closeModal}
        onOk={() => void handleSubmit()}
        okText="保存"
        cancelText="取消"
        width={880}
        confirmLoading={createMutation.isPending || updateMutation.isPending}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Alert
            style={{ marginBottom: 16 }}
            type="info"
            showIcon
            message={
              linkedEmployee
                ? `当前关联员工：${linkedEmployee.name} / ${departmentNameMap[linkedEmployee.department_id] || linkedEmployee.department_id || '未分配部门'} / ${linkedEmployee.position || '未设置岗位'}`
                : '请选择一个已同步员工作为档案关联对象。姓名、部门、岗位等主数据仅用于展示，不会通过本表单更新。'
            }
          />

          <Row gutter={16}>
            <Col xs={24} md={12}>
              <Form.Item
                name="user_id"
                label="关联员工"
                rules={[{ required: true, message: '请选择关联员工' }]}
              >
                <Select
                  showSearch
                  placeholder="请选择员工"
                  optionFilterProp="label"
                  options={employeeOptions}
                  loading={employeesQuery.isLoading}
                />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item
                name="employee_id"
                label="档案工号"
                rules={[{ required: true, message: '请输入档案工号' }]}
              >
                <Input placeholder="请输入档案工号" />
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
              <Form.Item name="nationality" label="国籍">
                <Input placeholder="请输入国籍" />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="id_card_number" label="身份证号">
                <Input placeholder="请输入身份证号" />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="work_email" label="工作邮箱">
                <Input placeholder="请输入工作邮箱" />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="personal_email" label="个人邮箱">
                <Input placeholder="请输入个人邮箱" />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="employment_type" label="用工类型">
                <Select allowClear options={employmentTypeOptions.map((item) => ({ label: item, value: item }))} />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="job_level" label="职级">
                <Input placeholder="请输入职级" />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="job_family" label="岗位序列">
                <Select allowClear options={jobFamilyOptions.map((item) => ({ label: item, value: item }))} />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="entry_date" label="入职日期">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item name="probation_end_date" label="试用期结束日期">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item name="planned_regular_date" label="计划转正日期">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item name="actual_regular_date" label="实际转正日期">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="contract_start_date" label="合同开始日期">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="contract_end_date" label="合同结束日期">
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
            <Col xs={24} md={8}>
              <Form.Item name="education" label="学历">
                <Select allowClear options={educationOptions.map((item) => ({ label: item, value: item }))} />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item name="graduate_school" label="毕业院校">
                <Input placeholder="请输入毕业院校" />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item name="major" label="专业">
                <Input placeholder="请输入专业" />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="graduation_date" label="毕业日期">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="bank_account" label="银行卡号">
                <Input placeholder="请输入银行卡号" />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="bank_name" label="开户行">
                <Input placeholder="请输入开户行" />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="tax_number" label="税号">
                <Input placeholder="请输入税号" />
              </Form.Item>
            </Col>
            <Col span={24}>
              <Form.Item name="address" label="联系地址">
                <TextArea rows={3} placeholder="请输入联系地址" />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
    </PageContainer>
  )
}

export default EmployeeProfilePage
