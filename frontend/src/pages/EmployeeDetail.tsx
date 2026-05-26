import React, { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Avatar,
  Button,
  Col,
  DatePicker,
  Descriptions,
  Form,
  Input,
  List,
  Modal,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Tag,
  Timeline,
  Typography,
  message,
} from 'antd'
import { ArrowLeftOutlined, EditOutlined, SwapRightOutlined, SyncOutlined, UserOutlined } from '@ant-design/icons'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import { useNavigate, useParams } from 'react-router-dom'
import dayjs from 'dayjs'
import { employeeAPI, orgAPI } from '../services/api'

const { Title, Text } = Typography

const employmentTypeOptions = ['正式', '试用', '实习', '劳务', '兼职']
const educationOptions = ['高中', '大专', '本科', '硕士', '博士', '其他']
const jobFamilyOptions = ['管理', '专业', '技术']

interface Employee {
  id: number
  user_id: string
  name: string
  email: string
  mobile: string
  department_id: string
  position: string
  avatar: string
  status: string
}

interface Profile {
  id: number
  user_id: string
  employee_id: string
  gender?: string
  birth_date?: string
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
  address?: string
  profile_status?: string
}

interface DepartmentPathItem {
  id: string
  name: string
}

interface DepartmentInfo {
  id: string
  name: string
  path: DepartmentPathItem[]
}

interface MemberRef {
  id: string
  user_id: string
  name: string
  department_id: string
  department_name: string
  position: string
}

interface WarningItem {
  type: string
  title: string
  description: string
  due_date?: string
  days_left?: number
}

interface TimelineEndpoint {
  department_id?: string
  department_name?: string
  position?: string
}

interface TimelineItem {
  type: string
  title: string
  description: string
  date: string
  status?: string
  operator_name?: string
  from?: TimelineEndpoint
  to?: TimelineEndpoint
  reason?: string
}

interface ScopeInfo {
  mode: string
  department_names?: string[]
}

interface DetailData {
  employee: Employee
  profile?: Profile
  scope?: ScopeInfo
  department: DepartmentInfo
  org_relation: {
    manager?: MemberRef
    direct_reports: MemberRef[]
    same_department_count: number
  }
  timeline: TimelineItem[]
  warnings: WarningItem[]
}

const EmployeeDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [detail, setDetail] = useState<DetailData | null>(null)
  const [loading, setLoading] = useState(true)
  const [syncing, setSyncing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  const [form] = Form.useForm()

  const scopeLabel = useMemo(() => {
    if (!detail?.scope) {
      return '正在加载数据范围...'
    }
    if (detail.scope.mode === 'all') {
      return '当前范围：全组织'
    }
    if (detail.scope.department_names?.length) {
      return `当前范围：${detail.scope.department_names.join(' / ')}`
    }
    return '当前范围：部门范围'
  }, [detail])

  const loadDetail = async (showLoading = true) => {
    if (!id) {
      return
    }
    if (showLoading) {
      setLoading(true)
    }
    try {
      const response = await orgAPI.getEmployee(id)
      setDetail(response.data.detail || null)
    } catch (error) {
      message.error('获取员工详情失败')
    } finally {
      if (showLoading) {
        setLoading(false)
      }
    }
  }

  useEffect(() => {
    void loadDetail()
  }, [id])

  const openEdit = () => {
    const profile = detail?.profile
    form.setFieldsValue({
      gender: profile?.gender || undefined,
      birth_date: profile?.birth_date ? dayjs(profile.birth_date) : null,
      employment_type: profile?.employment_type || undefined,
      entry_date: profile?.entry_date ? dayjs(profile.entry_date) : null,
      probation_end_date: profile?.probation_end_date ? dayjs(profile.probation_end_date) : null,
      planned_regular_date: profile?.planned_regular_date ? dayjs(profile.planned_regular_date) : null,
      actual_regular_date: profile?.actual_regular_date ? dayjs(profile.actual_regular_date) : null,
      job_level: profile?.job_level || '',
      job_family: profile?.job_family || undefined,
      contract_start_date: profile?.contract_start_date ? dayjs(profile.contract_start_date) : null,
      contract_end_date: profile?.contract_end_date ? dayjs(profile.contract_end_date) : null,
      education: profile?.education || undefined,
      work_email: profile?.work_email || '',
      personal_email: profile?.personal_email || '',
      emergency_contact: profile?.emergency_contact || '',
      emergency_phone: profile?.emergency_phone || '',
      address: profile?.address || '',
      profile_status: profile?.profile_status || 'active',
    })
    setEditOpen(true)
  }

  const handleSave = async () => {
    if (!detail?.employee) {
      return
    }

    const values = await form.validateFields()
    const payload: Record<string, string> = {}
    const dateFields = [
      'birth_date',
      'entry_date',
      'probation_end_date',
      'planned_regular_date',
      'actual_regular_date',
      'contract_start_date',
      'contract_end_date',
    ]

    Object.keys(values).forEach((key) => {
      if (dateFields.includes(key)) {
        payload[key] = values[key] ? values[key].format('YYYY-MM-DD') : ''
      } else {
        payload[key] = values[key] || ''
      }
    })

    setSaving(true)
    try {
      if (detail.profile?.id) {
        await employeeAPI.updateProfile(String(detail.profile.id), payload)
      } else {
        await employeeAPI.createProfile({
          ...payload,
          user_id: detail.employee.user_id,
          employee_id: detail.employee.user_id,
          profile_status: payload.profile_status || 'active',
        })
      }
      message.success('员工档案已保存')
      setEditOpen(false)
      await loadDetail(false)
    } catch (error) {
      message.error('保存员工档案失败')
    } finally {
      setSaving(false)
    }
  }

  const handleSync = async () => {
    setSyncing(true)
    try {
      await orgAPI.syncOrg()
      message.success('组织数据同步成功')
      await loadDetail(false)
    } catch (error) {
      message.error('组织数据同步失败')
    } finally {
      setSyncing(false)
    }
  }

  const statusTag = (value?: string) => (
    <StatusTag color={value === 'active' ? 'success' : 'default'}>
      {value === 'active' ? '在职' : value === 'inactive' ? '离职/停用' : value || '未设置'}
    </StatusTag>
  )

  const formatFlowStatus = (value?: string) => {
    const statusMap: Record<string, string> = {
      pending: '待处理',
      processing: '处理中',
      approved: '已通过',
      rejected: '已拒绝',
      completed: '已完成',
      planned: '计划中',
    }
    return value ? statusMap[value] || value : ''
  }

  const renderTimelineEndpoint = (endpoint: TimelineEndpoint | undefined, label: string) => (
    <div
      style={{
        flex: '1 1 180px',
        minWidth: 0,
        padding: '8px 10px',
        border: '1px solid var(--color-border-light)',
        borderRadius: 'var(--radius-md)',
        background: 'var(--color-bg-light)',
      }}
    >
      <div style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)', marginBottom: 2 }}>{label}</div>
      <Text strong style={{ display: 'block' }}>
        {endpoint?.department_name || endpoint?.department_id || '未记录部门'}
      </Text>
      <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>
        {endpoint?.position || '未记录岗位'}
      </Text>
    </div>
  )

  const renderTransferChange = (item: TimelineItem) => {
    if (item.type !== 'transfer' || (!item.from && !item.to)) {
      return null
    }

    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', marginTop: 8 }}>
        {renderTimelineEndpoint(item.from, '从')}
        <SwapRightOutlined style={{ color: 'var(--color-warning)' }} />
        {renderTimelineEndpoint(item.to, '到')}
      </div>
    )
  }

  return (
    <PageContainer
      title={detail?.employee?.name || '员工详情'}
      subtitle={scopeLabel}
      icon={<UserOutlined />}
      extra={
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/employees')}>
            返回花名册
          </Button>
          <Button icon={<EditOutlined />} onClick={openEdit}>
            编辑档案
          </Button>
          <Button type="primary" icon={<SyncOutlined />} onClick={() => void handleSync()} loading={syncing}>
            同步组织数据
          </Button>
        </Space>
      }
    >
      {loading ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}>
          <Spin size="large" />
        </div>
      ) : detail ? (
        <>
          <PageCard style={{ marginBottom: 16 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 20, flexWrap: 'wrap' }}>
              <Avatar size={80} src={detail.employee.avatar} icon={<UserOutlined />} />
              <div style={{ flex: 1 }}>
                <div style={{ marginBottom: 8 }}>
                  <Text>{detail.employee.position || '未设置岗位'}</Text>
                  <Text type="secondary"> / {detail.department.name || detail.employee.department_id}</Text>
                </div>
                <Space wrap>
                  {statusTag(detail.employee.status)}
                  <StatusTag>{detail.employee.user_id}</StatusTag>
                </Space>
              </div>
            </div>
          </PageCard>

          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            <Col xs={24} sm={12} lg={6}>
              <PageCard>
                <Statistic title="当前预警" value={detail.warnings.length} />
              </PageCard>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <PageCard>
                <Statistic title="同部门成员" value={detail.org_relation.same_department_count} />
              </PageCard>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <PageCard>
                <Statistic title="直属下属" value={detail.org_relation.direct_reports.length} />
              </PageCard>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <PageCard>
                <div style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-tertiary)', marginBottom: 8 }}>档案状态</div>
                {statusTag(detail.profile?.profile_status || detail.employee.status)}
              </PageCard>
            </Col>
          </Row>

          <Row gutter={[16, 16]}>
            <Col xs={24} lg={14}>
              <PageCard title="档案聚合">
                <Descriptions column={2} bordered size="small">
                  <Descriptions.Item label="员工工号">{detail.profile?.employee_id || detail.employee.user_id}</Descriptions.Item>
                  <Descriptions.Item label="员工状态">{statusTag(detail.employee.status)}</Descriptions.Item>
                  <Descriptions.Item label="组织路径" span={2}>
                    {detail.department.path.length
                      ? detail.department.path.map((item) => item.name).join(' / ')
                      : detail.department.name || '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="公司邮箱">{detail.profile?.work_email || detail.employee.email || '-'}</Descriptions.Item>
                  <Descriptions.Item label="手机">{detail.employee.mobile || '-'}</Descriptions.Item>
                  <Descriptions.Item label="雇佣类型">{detail.profile?.employment_type || '-'}</Descriptions.Item>
                  <Descriptions.Item label="学历">{detail.profile?.education || '-'}</Descriptions.Item>
                  <Descriptions.Item label="职级">{detail.profile?.job_level || '-'}</Descriptions.Item>
                  <Descriptions.Item label="岗位序列">{detail.profile?.job_family || '-'}</Descriptions.Item>
                  <Descriptions.Item label="入职日期">{detail.profile?.entry_date || '-'}</Descriptions.Item>
                  <Descriptions.Item label="试用期结束">{detail.profile?.probation_end_date || '-'}</Descriptions.Item>
                  <Descriptions.Item label="计划转正">{detail.profile?.planned_regular_date || '-'}</Descriptions.Item>
                  <Descriptions.Item label="实际转正">{detail.profile?.actual_regular_date || '-'}</Descriptions.Item>
                  <Descriptions.Item label="合同开始">{detail.profile?.contract_start_date || '-'}</Descriptions.Item>
                  <Descriptions.Item label="合同结束">{detail.profile?.contract_end_date || '-'}</Descriptions.Item>
                  <Descriptions.Item label="紧急联系人">{detail.profile?.emergency_contact || '-'}</Descriptions.Item>
                  <Descriptions.Item label="紧急联系电话">{detail.profile?.emergency_phone || '-'}</Descriptions.Item>
                  <Descriptions.Item label="联系地址" span={2}>
                    {detail.profile?.address || '-'}
                  </Descriptions.Item>
                </Descriptions>
              </PageCard>

              <PageCard title="生命周期时间轴" style={{ marginTop: 16 }}>
                {detail.timeline.length ? (
                  <Timeline
                    items={detail.timeline.map((item) => ({
                      color:
                        item.type === 'resignation'
                          ? 'red'
                          : item.type === 'transfer'
                            ? 'gold'
                            : item.type === 'audit'
                              ? 'blue'
                              : 'green',
                      children: (
                        <div>
                          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                            <Text strong>{item.title}</Text>
                            <Text type="secondary">{item.date}</Text>
                          </div>
                          {renderTransferChange(item)}
                          {item.description ? <div style={{ marginTop: 4 }}>{item.description}</div> : null}
                          {item.status || item.operator_name ? (
                            <div style={{ marginTop: 4, color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)' }}>
                              {item.status ? `状态：${formatFlowStatus(item.status)}` : ''}
                              {item.status && item.operator_name ? ' / ' : ''}
                              {item.operator_name ? `操作人：${item.operator_name}` : ''}
                            </div>
                          ) : null}
                        </div>
                      ),
                    }))}
                  />
                ) : (
                  <Alert type="info" showIcon message="当前员工还没有可展示的生命周期记录" />
                )}
              </PageCard>
            </Col>

            <Col xs={24} lg={10}>
              <PageCard title="汇报关系">
                <Descriptions column={1} bordered size="small">
                  <Descriptions.Item label="直属上级">
                    {detail.org_relation.manager
                      ? `${detail.org_relation.manager.name} / ${detail.org_relation.manager.position || '未设置岗位'}`
                      : '未配置'}
                  </Descriptions.Item>
                  <Descriptions.Item label="直属下属数量">
                    {detail.org_relation.direct_reports.length}
                  </Descriptions.Item>
                  <Descriptions.Item label="同部门成员数">
                    {detail.org_relation.same_department_count}
                  </Descriptions.Item>
                </Descriptions>

                <div style={{ marginTop: 16 }}>
                  <Title level={5}>直属下属</Title>
                  <List
                    locale={{ emptyText: '当前没有直属下属' }}
                    dataSource={detail.org_relation.direct_reports}
                    renderItem={(item) => (
                      <List.Item>
                        <div>
                          <Text strong>{item.name}</Text>
                          <div style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)' }}>
                            {item.position || '未设置岗位'} / {item.department_name || item.department_id}
                          </div>
                        </div>
                      </List.Item>
                    )}
                  />
                </div>
              </PageCard>

              <PageCard title="当前预警" style={{ marginTop: 16 }}>
                {detail.warnings.length ? (
                  <List
                    dataSource={detail.warnings}
                    renderItem={(item) => (
                      <List.Item>
                        <div style={{ width: '100%' }}>
                          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                            <Text strong>{item.title}</Text>
                            {item.due_date ? <Text type="secondary">{item.due_date}</Text> : null}
                          </div>
                          <div style={{ marginTop: 4, color: 'var(--color-text-tertiary)' }}>{item.description}</div>
                        </div>
                      </List.Item>
                    )}
                  />
                ) : (
                  <Alert type="success" showIcon message="当前没有组织预警" />
                )}
              </PageCard>
            </Col>
          </Row>
        </>
      ) : (
        <Alert type="warning" showIcon message="没有找到该员工" />
      )}

      <Modal
        title="编辑员工档案"
        open={editOpen}
        onCancel={() => setEditOpen(false)}
        onOk={() => void handleSave()}
        confirmLoading={saving}
        width={760}
      >
        <Form form={form} layout="vertical">
          <Row gutter={16}>
            <Col span={12}>
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
            <Col span={12}>
              <Form.Item name="birth_date" label="出生日期">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="employment_type" label="雇佣类型">
                <Select
                  allowClear
                  options={employmentTypeOptions.map((item) => ({ label: item, value: item }))}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="profile_status" label="档案状态">
                <Select
                  options={[
                    { label: '在职', value: 'active' },
                    { label: '非在职', value: 'inactive' },
                  ]}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="entry_date" label="入职日期">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="probation_end_date" label="试用期结束">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="planned_regular_date" label="计划转正">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="actual_regular_date" label="实际转正">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="job_level" label="职级">
                <Input />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="job_family" label="岗位序列">
                <Select
                  allowClear
                  options={jobFamilyOptions.map((item) => ({ label: item, value: item }))}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="contract_start_date" label="合同开始">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="contract_end_date" label="合同结束">
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="work_email" label="公司邮箱">
                <Input />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="personal_email" label="个人邮箱">
                <Input />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="education" label="学历">
                <Select
                  allowClear
                  options={educationOptions.map((item) => ({ label: item, value: item }))}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="emergency_contact" label="紧急联系人">
                <Input />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="emergency_phone" label="紧急联系电话">
                <Input />
              </Form.Item>
            </Col>
            <Col span={24}>
              <Form.Item name="address" label="联系地址">
                <Input.TextArea rows={3} />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
    </PageContainer>
  )
}

export default EmployeeDetail
