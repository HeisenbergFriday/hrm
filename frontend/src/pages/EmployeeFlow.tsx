import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, Modal, Form, Input, Select, DatePicker, message, Tabs, Descriptions, Tag, Divider } from 'antd'
import { UserAddOutlined, SwapOutlined, UserDeleteOutlined, ReloadOutlined, PlusOutlined, EditOutlined, CheckOutlined, CloseOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { employeeAPI } from '../services/api'
import dayjs from 'dayjs'

const { Title, Text } = Typography
const { Option } = Select

interface Transfer {
  id: string
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
  reason: string
  status: string
  approver_id: string
  approver_name: string
  approval_time: string
  approval_comment: string
  created_at: string
  updated_at: string
}

interface Resignation {
  id: string
  resignation_id: string
  user_id: string
  user_name: string
  department_id: string
  department_name: string
  position: string
  resign_date: string
  last_working_day: string
  resign_reason: string
  status: string
  approver_id: string
  approver_name: string
  approval_time: string
  approval_comment: string
  exit_process: any
  created_at: string
  updated_at: string
}

interface Onboarding {
  id: string
  onboarding_id: string
  employee_id: string
  name: string
  gender: string
  birth_date: string
  id_card_number: string
  mobile: string
  email: string
  department_id: string
  department_name: string
  position: string
  entry_date: string
  employment_type: string
  probation_end_date: string
  emergency_contact: string
  emergency_phone: string
  education: string
  graduate_school: string
  major: string
  onboarding_process: any
  status: string
  created_at: string
  updated_at: string
}

const EmployeeFlow: React.FC = () => {
  const [activeTab, setActiveTab] = useState('transfer')
  const [modalVisible, setModalVisible] = useState(false)
  const [form] = Form.useForm()
  const [currentItem, setCurrentItem] = useState<any>(null)

  // 转岗
  const { data: transfersData, isLoading: transfersLoading, refetch: refetchTransfers } = useQuery({
    queryKey: ['employee-transfers'],
    queryFn: () => employeeAPI.getTransfers(),
  })

  const createTransferMutation = useMutation({
    mutationFn: (data: any) => employeeAPI.createTransfer(data),
    onSuccess: () => {
      message.success('转岗申请创建成功')
      setModalVisible(false)
      form.resetFields()
      refetchTransfers()
    },
    onError: () => {
      message.error('转岗申请创建失败')
    },
  })

  // 离职
  const { data: resignationsData, isLoading: resignationsLoading, refetch: refetchResignations } = useQuery({
    queryKey: ['employee-resignations'],
    queryFn: () => employeeAPI.getResignations(),
  })

  const createResignationMutation = useMutation({
    mutationFn: (data: any) => employeeAPI.createResignation(data),
    onSuccess: () => {
      message.success('离职申请创建成功')
      setModalVisible(false)
      form.resetFields()
      refetchResignations()
    },
    onError: () => {
      message.error('离职申请创建失败')
    },
  })

  // 入职
  const { data: onboardingData, isLoading: onboardingLoading, refetch: refetchOnboarding } = useQuery({
    queryKey: ['employee-onboardings'],
    queryFn: () => employeeAPI.getOnboardings(),
  })

  const createOnboardingMutation = useMutation({
    mutationFn: (data: any) => employeeAPI.createOnboarding(data),
    onSuccess: () => {
      message.success('入职申请创建成功')
      setModalVisible(false)
      form.resetFields()
      refetchOnboarding()
    },
    onError: () => {
      message.error('入职申请创建失败')
    },
  })

  const handleCreate = () => {
    form.validateFields().then((values) => {
      if (activeTab === 'transfer') {
        createTransferMutation.mutate(values)
      } else if (activeTab === 'resignation') {
        createResignationMutation.mutate(values)
      } else if (activeTab === 'onboarding') {
        createOnboardingMutation.mutate(values)
      }
    })
  }

  const handleView = (item: any) => {
    setCurrentItem(item)
  }

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'pending':
        return <Tag color="blue">待审批</Tag>
      case 'approved':
        return <Tag color="green">已批准</Tag>
      case 'rejected':
        return <Tag color="red">已拒绝</Tag>
      case 'processing':
        return <Tag color="orange">处理中</Tag>
      case 'completed':
        return <Tag color="green">已完成</Tag>
      default:
        return <Tag>{status}</Tag>
    }
  }

  const transferColumns = [
    {
      title: '转岗编号',
      dataIndex: 'transfer_id',
      key: 'transfer_id',
    },
    {
      title: '员工姓名',
      dataIndex: 'user_name',
      key: 'user_name',
    },
    {
      title: '原部门/职位',
      key: 'old_info',
      render: (_, record: Transfer) => (
        <div>
          <div>{record.old_department_name}</div>
          <div style={{ fontSize: 12, color: '#666' }}>{record.old_position}</div>
        </div>
      ),
    },
    {
      title: '新部门/职位',
      key: 'new_info',
      render: (_, record: Transfer) => (
        <div>
          <div>{record.new_department_name}</div>
          <div style={{ fontSize: 12, color: '#666' }}>{record.new_position}</div>
        </div>
      ),
    },
    {
      title: '转岗日期',
      dataIndex: 'transfer_date',
      key: 'transfer_date',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => getStatusTag(status),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Transfer) => (
        <Button type="link" onClick={() => handleView(record)}>
          查看
        </Button>
      ),
    },
  ]

  const resignationColumns = [
    {
      title: '离职编号',
      dataIndex: 'resignation_id',
      key: 'resignation_id',
    },
    {
      title: '员工姓名',
      dataIndex: 'user_name',
      key: 'user_name',
    },
    {
      title: '部门/职位',
      key: 'dept_info',
      render: (_, record: Resignation) => (
        <div>
          <div>{record.department_name}</div>
          <div style={{ fontSize: 12, color: '#666' }}>{record.position}</div>
        </div>
      ),
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
      render: (status: string) => getStatusTag(status),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Resignation) => (
        <Button type="link" onClick={() => handleView(record)}>
          查看
        </Button>
      ),
    },
  ]

  const onboardingColumns = [
    {
      title: '入职编号',
      dataIndex: 'onboarding_id',
      key: 'onboarding_id',
    },
    {
      title: '员工姓名',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '工号',
      dataIndex: 'employee_id',
      key: 'employee_id',
    },
    {
      title: '部门/职位',
      key: 'dept_info',
      render: (_, record: Onboarding) => (
        <div>
          <div>{record.department_name}</div>
          <div style={{ fontSize: 12, color: '#666' }}>{record.position}</div>
        </div>
      ),
    },
    {
      title: '入职日期',
      dataIndex: 'entry_date',
      key: 'entry_date',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => getStatusTag(status),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Onboarding) => (
        <Button type="link" onClick={() => handleView(record)}>
          查看
        </Button>
      ),
    },
  ]

  const renderDetail = () => {
    if (!currentItem) return null

    if (activeTab === 'transfer') {
      const item = currentItem as Transfer
      return (
        <Card title="转岗详情">
          <Descriptions column={2} bordered>
            <Descriptions.Item label="转岗编号" span={1}>{item.transfer_id}</Descriptions.Item>
            <Descriptions.Item label="员工姓名" span={1}>{item.user_name}</Descriptions.Item>
            <Descriptions.Item label="原部门" span={1}>{item.old_department_name}</Descriptions.Item>
            <Descriptions.Item label="原职位" span={1}>{item.old_position}</Descriptions.Item>
            <Descriptions.Item label="新部门" span={1}>{item.new_department_name}</Descriptions.Item>
            <Descriptions.Item label="新职位" span={1}>{item.new_position}</Descriptions.Item>
            <Descriptions.Item label="转岗日期" span={1}>{item.transfer_date}</Descriptions.Item>
            <Descriptions.Item label="状态" span={1}>{getStatusTag(item.status)}</Descriptions.Item>
            <Descriptions.Item label="转岗原因" span={2}>{item.reason}</Descriptions.Item>
            {item.status !== 'pending' && (
              <>
                <Descriptions.Item label="审批人" span={1}>{item.approver_name}</Descriptions.Item>
                <Descriptions.Item label="审批时间" span={1}>{item.approval_time}</Descriptions.Item>
                <Descriptions.Item label="审批意见" span={2}>{item.approval_comment}</Descriptions.Item>
              </>
            )}
          </Descriptions>
        </Card>
      )
    } else if (activeTab === 'resignation') {
      const item = currentItem as Resignation
      return (
        <Card title="离职详情">
          <Descriptions column={2} bordered>
            <Descriptions.Item label="离职编号" span={1}>{item.resignation_id}</Descriptions.Item>
            <Descriptions.Item label="员工姓名" span={1}>{item.user_name}</Descriptions.Item>
            <Descriptions.Item label="部门" span={1}>{item.department_name}</Descriptions.Item>
            <Descriptions.Item label="职位" span={1}>{item.position}</Descriptions.Item>
            <Descriptions.Item label="离职日期" span={1}>{item.resign_date}</Descriptions.Item>
            <Descriptions.Item label="最后工作日" span={1}>{item.last_working_day}</Descriptions.Item>
            <Descriptions.Item label="状态" span={1}>{getStatusTag(item.status)}</Descriptions.Item>
            <Descriptions.Item label="离职原因" span={2}>{item.resign_reason}</Descriptions.Item>
            {item.status !== 'pending' && (
              <>
                <Descriptions.Item label="审批人" span={1}>{item.approver_name}</Descriptions.Item>
                <Descriptions.Item label="审批时间" span={1}>{item.approval_time}</Descriptions.Item>
                <Descriptions.Item label="审批意见" span={2}>{item.approval_comment}</Descriptions.Item>
              </>
            )}
          </Descriptions>
          <Divider orientation="left">离职手续</Divider>
          {item.exit_process?.items?.map((process: any, index: number) => (
            <div key={index} style={{ marginBottom: 8 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text>{process.name}</Text>
                <Tag color={process.status === 'completed' ? 'green' : 'blue'}>
                  {process.status === 'completed' ? '已完成' : '待处理'}
                </Tag>
              </div>
            </div>
          ))}
        </Card>
      )
    } else if (activeTab === 'onboarding') {
      const item = currentItem as Onboarding
      return (
        <Card title="入职详情">
          <Descriptions column={2} bordered>
            <Descriptions.Item label="入职编号" span={1}>{item.onboarding_id}</Descriptions.Item>
            <Descriptions.Item label="员工姓名" span={1}>{item.name}</Descriptions.Item>
            <Descriptions.Item label="工号" span={1}>{item.employee_id}</Descriptions.Item>
            <Descriptions.Item label="性别" span={1}>{item.gender}</Descriptions.Item>
            <Descriptions.Item label="部门" span={1}>{item.department_name}</Descriptions.Item>
            <Descriptions.Item label="职位" span={1}>{item.position}</Descriptions.Item>
            <Descriptions.Item label="入职日期" span={1}>{item.entry_date}</Descriptions.Item>
            <Descriptions.Item label="雇佣类型" span={1}>{item.employment_type}</Descriptions.Item>
            <Descriptions.Item label="试用期结束日期" span={1}>{item.probation_end_date}</Descriptions.Item>
            <Descriptions.Item label="状态" span={1}>{getStatusTag(item.status)}</Descriptions.Item>
            <Descriptions.Item label="联系电话" span={1}>{item.mobile}</Descriptions.Item>
            <Descriptions.Item label="邮箱" span={1}>{item.email}</Descriptions.Item>
            <Descriptions.Item label="紧急联系人" span={1}>{item.emergency_contact}</Descriptions.Item>
            <Descriptions.Item label="紧急联系电话" span={1}>{item.emergency_phone}</Descriptions.Item>
            <Descriptions.Item label="学历" span={1}>{item.education}</Descriptions.Item>
            <Descriptions.Item label="毕业院校" span={1}>{item.graduate_school}</Descriptions.Item>
            <Descriptions.Item label="专业" span={2}>{item.major}</Descriptions.Item>
          </Descriptions>
          <Divider orientation="left">入职流程</Divider>
          {item.onboarding_process?.items?.map((process: any, index: number) => (
            <div key={index} style={{ marginBottom: 8 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text>{process.name}</Text>
                <Tag color={
                  process.status === 'completed' ? 'green' : 
                  process.status === 'in_progress' ? 'orange' : 'blue'
                }>
                  {process.status === 'completed' ? '已完成' : 
                   process.status === 'in_progress' ? '处理中' : '待处理'}
                </Tag>
              </div>
            </div>
          ))}
        </Card>
      )
    }
    return null
  }

  return (
    <div>
      <Title level={4}>入转调离管理</Title>
      <Tabs activeKey={activeTab} onChange={setActiveTab}>
        <Tabs.TabPane tab="转岗管理" key="transfer" icon={<SwapOutlined />}>
          <Card
            extra={
              <div style={{ display: 'flex', gap: 8 }}>
                <Button icon={<ReloadOutlined />} onClick={() => refetchTransfers()} loading={transfersLoading}>
                  刷新
                </Button>
                <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalVisible(true)}>
                  新建转岗申请
                </Button>
              </div>
            }
          >
            {transfersLoading ? (
              <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
                <Spin size="large" />
              </div>
            ) : transfersData?.data?.items?.length ? (
              <Table
                columns={transferColumns}
                dataSource={transfersData.data.items as Transfer[]}
                rowKey="id"
                pagination={{
                  showTotal: (total: number) => `共 ${total} 条转岗记录`,
                }}
              />
            ) : (
              <Empty description="暂无转岗记录" />
            )}
          </Card>
        </Tabs.TabPane>
        <Tabs.TabPane tab="离职管理" key="resignation" icon={<UserDeleteOutlined />}>
          <Card
            extra={
              <div style={{ display: 'flex', gap: 8 }}>
                <Button icon={<ReloadOutlined />} onClick={() => refetchResignations()} loading={resignationsLoading}>
                  刷新
                </Button>
                <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalVisible(true)}>
                  新建离职申请
                </Button>
              </div>
            }
          >
            {resignationsLoading ? (
              <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
                <Spin size="large" />
              </div>
            ) : resignationsData?.data?.items?.length ? (
              <Table
                columns={resignationColumns}
                dataSource={resignationsData.data.items as Resignation[]}
                rowKey="id"
                pagination={{
                  showTotal: (total: number) => `共 ${total} 条离职记录`,
                }}
              />
            ) : (
              <Empty description="暂无离职记录" />
            )}
          </Card>
        </Tabs.TabPane>
        <Tabs.TabPane tab="入职管理" key="onboarding" icon={<UserAddOutlined />}>
          <Card
            extra={
              <div style={{ display: 'flex', gap: 8 }}>
                <Button icon={<ReloadOutlined />} onClick={() => refetchOnboarding()} loading={onboardingLoading}>
                  刷新
                </Button>
                <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalVisible(true)}>
                  新建入职申请
                </Button>
              </div>
            }
          >
            {onboardingLoading ? (
              <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
                <Spin size="large" />
              </div>
            ) : onboardingData?.data?.items?.length ? (
              <Table
                columns={onboardingColumns}
                dataSource={onboardingData.data.items as Onboarding[]}
                rowKey="id"
                pagination={{
                  showTotal: (total: number) => `共 ${total} 条入职记录`,
                }}
              />
            ) : (
              <Empty description="暂无入职记录" />
            )}
          </Card>
        </Tabs.TabPane>
      </Tabs>

      {currentItem && renderDetail()}

      <Modal
        title={
          activeTab === 'transfer' ? '新建转岗申请' : 
          activeTab === 'resignation' ? '新建离职申请' : '新建入职申请'
        }
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setModalVisible(false)}>
            取消
          </Button>,
          <Button
            key="submit"
            type="primary"
            onClick={handleCreate}
            loading={
              activeTab === 'transfer' ? createTransferMutation.isPending :
              activeTab === 'resignation' ? createResignationMutation.isPending :
              createOnboardingMutation.isPending
            }
          >
            确认
          </Button>,
        ]}
        width={800}
      >
        <Form form={form} layout="vertical">
          {activeTab === 'transfer' && (
            <>
              <Form.Item
                name="user_id"
                label="员工ID"
                rules={[{ required: true, message: '请输入员工ID' }]}
              >
                <Input placeholder="请输入员工ID" />
              </Form.Item>
              <Form.Item
                name="user_name"
                label="员工姓名"
                rules={[{ required: true, message: '请输入员工姓名' }]}
              >
                <Input placeholder="请输入员工姓名" />
              </Form.Item>
              <Form.Item
                name="old_department_id"
                label="原部门ID"
                rules={[{ required: true, message: '请输入原部门ID' }]}
              >
                <Input placeholder="请输入原部门ID" />
              </Form.Item>
              <Form.Item
                name="old_department_name"
                label="原部门名称"
                rules={[{ required: true, message: '请输入原部门名称' }]}
              >
                <Input placeholder="请输入原部门名称" />
              </Form.Item>
              <Form.Item
                name="old_position"
                label="原职位"
                rules={[{ required: true, message: '请输入原职位' }]}
              >
                <Input placeholder="请输入原职位" />
              </Form.Item>
              <Form.Item
                name="new_department_id"
                label="新部门ID"
                rules={[{ required: true, message: '请输入新部门ID' }]}
              >
                <Input placeholder="请输入新部门ID" />
              </Form.Item>
              <Form.Item
                name="new_department_name"
                label="新部门名称"
                rules={[{ required: true, message: '请输入新部门名称' }]}
              >
                <Input placeholder="请输入新部门名称" />
              </Form.Item>
              <Form.Item
                name="new_position"
                label="新职位"
                rules={[{ required: true, message: '请输入新职位' }]}
              >
                <Input placeholder="请输入新职位" />
              </Form.Item>
              <Form.Item
                name="transfer_date"
                label="转岗日期"
                rules={[{ required: true, message: '请选择转岗日期' }]}
              >
                <DatePicker style={{ width: '100%' }} placeholder="选择日期" />
              </Form.Item>
              <Form.Item
                name="reason"
                label="转岗原因"
                rules={[{ required: true, message: '请输入转岗原因' }]}
              >
                <Input.TextArea placeholder="请输入转岗原因" rows={4} />
              </Form.Item>
            </>
          )}

          {activeTab === 'resignation' && (
            <>
              <Form.Item
                name="user_id"
                label="员工ID"
                rules={[{ required: true, message: '请输入员工ID' }]}
              >
                <Input placeholder="请输入员工ID" />
              </Form.Item>
              <Form.Item
                name="user_name"
                label="员工姓名"
                rules={[{ required: true, message: '请输入员工姓名' }]}
              >
                <Input placeholder="请输入员工姓名" />
              </Form.Item>
              <Form.Item
                name="department_id"
                label="部门ID"
                rules={[{ required: true, message: '请输入部门ID' }]}
              >
                <Input placeholder="请输入部门ID" />
              </Form.Item>
              <Form.Item
                name="department_name"
                label="部门名称"
                rules={[{ required: true, message: '请输入部门名称' }]}
              >
                <Input placeholder="请输入部门名称" />
              </Form.Item>
              <Form.Item
                name="position"
                label="职位"
                rules={[{ required: true, message: '请输入职位' }]}
              >
                <Input placeholder="请输入职位" />
              </Form.Item>
              <Form.Item
                name="resign_date"
                label="离职日期"
                rules={[{ required: true, message: '请选择离职日期' }]}
              >
                <DatePicker style={{ width: '100%' }} placeholder="选择日期" />
              </Form.Item>
              <Form.Item
                name="last_working_day"
                label="最后工作日"
                rules={[{ required: true, message: '请选择最后工作日' }]}
              >
                <DatePicker style={{ width: '100%' }} placeholder="选择日期" />
              </Form.Item>
              <Form.Item
                name="resign_reason"
                label="离职原因"
                rules={[{ required: true, message: '请输入离职原因' }]}
              >
                <Input.TextArea placeholder="请输入离职原因" rows={4} />
              </Form.Item>
            </>
          )}

          {activeTab === 'onboarding' && (
            <>
              <Form.Item
                name="employee_id"
                label="员工工号"
                rules={[{ required: true, message: '请输入员工工号' }]}
              >
                <Input placeholder="请输入员工工号" />
              </Form.Item>
              <Form.Item
                name="name"
                label="姓名"
                rules={[{ required: true, message: '请输入姓名' }]}
              >
                <Input placeholder="请输入姓名" />
              </Form.Item>
              <Form.Item
                name="gender"
                label="性别"
              >
                <Select placeholder="请选择性别">
                  <Option value="男">男</Option>
                  <Option value="女">女</Option>
                </Select>
              </Form.Item>
              <Form.Item
                name="birth_date"
                label="出生日期"
              >
                <DatePicker style={{ width: '100%' }} placeholder="选择日期" />
              </Form.Item>
              <Form.Item
                name="id_card_number"
                label="身份证号"
              >
                <Input placeholder="请输入身份证号" />
              </Form.Item>
              <Form.Item
                name="mobile"
                label="联系电话"
              >
                <Input placeholder="请输入联系电话" />
              </Form.Item>
              <Form.Item
                name="email"
                label="邮箱"
              >
                <Input placeholder="请输入邮箱" />
              </Form.Item>
              <Form.Item
                name="department_id"
                label="部门ID"
                rules={[{ required: true, message: '请输入部门ID' }]}
              >
                <Input placeholder="请输入部门ID" />
              </Form.Item>
              <Form.Item
                name="department_name"
                label="部门名称"
                rules={[{ required: true, message: '请输入部门名称' }]}
              >
                <Input placeholder="请输入部门名称" />
              </Form.Item>
              <Form.Item
                name="position"
                label="职位"
                rules={[{ required: true, message: '请输入职位' }]}
              >
                <Input placeholder="请输入职位" />
              </Form.Item>
              <Form.Item
                name="entry_date"
                label="入职日期"
                rules={[{ required: true, message: '请选择入职日期' }]}
              >
                <DatePicker style={{ width: '100%' }} placeholder="选择日期" />
              </Form.Item>
              <Form.Item
                name="employment_type"
                label="雇佣类型"
                rules={[{ required: true, message: '请选择雇佣类型' }]}
              >
                <Select placeholder="请选择雇佣类型">
                  <Option value="全职">全职</Option>
                  <Option value="兼职">兼职</Option>
                  <Option value="实习">实习</Option>
                </Select>
              </Form.Item>
              <Form.Item
                name="probation_end_date"
                label="试用期结束日期"
              >
                <DatePicker style={{ width: '100%' }} placeholder="选择日期" />
              </Form.Item>
              <Form.Item
                name="emergency_contact"
                label="紧急联系人"
              >
                <Input placeholder="请输入紧急联系人" />
              </Form.Item>
              <Form.Item
                name="emergency_phone"
                label="紧急联系电话"
              >
                <Input placeholder="请输入紧急联系电话" />
              </Form.Item>
              <Form.Item
                name="education"
                label="学历"
              >
                <Select placeholder="请选择学历">
                  <Option value="高中">高中</Option>
                  <Option value="大专">大专</Option>
                  <Option value="本科">本科</Option>
                  <Option value="硕士">硕士</Option>
                  <Option value="博士">博士</Option>
                </Select>
              </Form.Item>
              <Form.Item
                name="graduate_school"
                label="毕业院校"
              >
                <Input placeholder="请输入毕业院校" />
              </Form.Item>
              <Form.Item
                name="major"
                label="专业"
              >
                <Input placeholder="请输入专业" />
              </Form.Item>
            </>
          )}
        </Form>
      </Modal>
    </div>
  )
}

export default EmployeeFlow
