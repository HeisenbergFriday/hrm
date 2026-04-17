import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, Modal, Form, Input, Select, DatePicker, message, Tabs, Divider, Descriptions, Collapse, Avatar } from 'antd'
import { UserOutlined, PlusOutlined, EditOutlined, ReloadOutlined, FileTextOutlined, IdcardOutlined, MailOutlined, PhoneOutlined, HomeOutlined, BriefcaseOutlined, GraduationCapOutlined, AwardOutlined, BankOutlined, EnvironmentOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { employeeAPI } from '../services/api'
import dayjs from 'dayjs'

const { Title, Text } = Typography
const { Option } = Select
const { Panel } = Collapse

interface EmployeeProfile {
  id: string
  user_id: string
  employee_id: string
  name: string
  gender: string
  birth_date: string
  nationality: string
  id_card_number: string
  employment_type: string
  entry_date: string
  probation_end_date: string
  contract_start_date: string
  contract_end_date: string
  work_email: string
  personal_email: string
  emergency_contact: string
  emergency_phone: string
  education: string
  graduate_school: string
  major: string
  graduation_date: string
  work_experience: any
  skills: any
  bank_account: string
  bank_name: string
  tax_number: string
  address: string
  profile_status: string
  created_at: string
  updated_at: string
}

const EmployeeProfile: React.FC = () => {
  const [modalVisible, setModalVisible] = useState(false)
  const [currentProfile, setCurrentProfile] = useState<EmployeeProfile | null>(null)
  const [form] = Form.useForm()
  const [activeTab, setActiveTab] = useState('list')

  const { data: profilesData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['employee-profiles'],
    queryFn: () => employeeAPI.getProfiles(),
  })

  const createProfileMutation = useMutation({
    mutationFn: (data: any) => employeeAPI.createProfile(data),
    onSuccess: () => {
      message.success('员工档案创建成功')
      setModalVisible(false)
      form.resetFields()
      refetch()
    },
    onError: () => {
      message.error('员工档案创建失败')
    },
  })

  const updateProfileMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: any }) => employeeAPI.updateProfile(id, data),
    onSuccess: () => {
      message.success('员工档案更新成功')
      setModalVisible(false)
      refetch()
    },
    onError: () => {
      message.error('员工档案更新失败')
    },
  })

  const handleCreateProfile = () => {
    form.validateFields().then((values) => {
      createProfileMutation.mutate(values)
    })
  }

  const handleUpdateProfile = () => {
    form.validateFields().then((values) => {
      if (currentProfile) {
        updateProfileMutation.mutate({ id: currentProfile.id, data: values })
      }
    })
  }

  const handleEditProfile = (profile: EmployeeProfile) => {
    setCurrentProfile(profile)
    form.setFieldsValue(profile)
    setModalVisible(true)
  }

  const handleViewProfile = (profile: EmployeeProfile) => {
    setCurrentProfile(profile)
    setActiveTab('detail')
  }

  const columns = [
    {
      title: '员工工号',
      dataIndex: 'employee_id',
      key: 'employee_id',
    },
    {
      title: '姓名',
      dataIndex: 'name',
      key: 'name',
      render: (text: string) => <Text strong>{text}</Text>,
    },
    {
      title: '部门',
      dataIndex: 'department_name',
      key: 'department_name',
      render: () => '技术部', // 模拟数据
    },
    {
      title: '职位',
      dataIndex: 'position',
      key: 'position',
      render: () => '工程师', // 模拟数据
    },
    {
      title: '入职日期',
      dataIndex: 'entry_date',
      key: 'entry_date',
    },
    {
      title: '状态',
      dataIndex: 'profile_status',
      key: 'profile_status',
      render: (status: string) => (
        <Text type={status === 'active' ? 'success' : 'warning'}>
          {status === 'active' ? '在职' : '离职'}
        </Text>
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: EmployeeProfile) => (
        <div style={{ display: 'flex', gap: 8 }}>
          <Button type="link" icon={<EditOutlined />} onClick={() => handleEditProfile(record)}>
            编辑
          </Button>
          <Button type="link" onClick={() => handleViewProfile(record)}>
            查看
          </Button>
        </div>
      ),
    },
  ]

  return (
    <div>
      <Title level={4}>员工档案中心</Title>
      {activeTab === 'list' ? (
        <Card
          extra={
            <div style={{ display: 'flex', gap: 8 }}>
              <Button icon={<ReloadOutlined />} onClick={() => refetch()} loading={isLoading}>
                刷新
              </Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalVisible(true)}>
                新建档案
              </Button>
            </div>
          }
        >
          {isLoading ? (
            <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
              <Spin size="large" />
            </div>
          ) : isError ? (
            <div style={{ padding: '20px' }}>
              <Alert
                message="加载失败"
                description={(error as Error)?.message || '获取员工档案失败，请稍后重试'}
                type="error"
                showIcon
                action={
                  <Button size="small" onClick={() => refetch()}>
                    重试
                  </Button>
                }
              />
            </div>
          ) : profilesData?.data?.items?.length ? (
            <Table
              columns={columns}
              dataSource={profilesData.data.items as EmployeeProfile[]}
              rowKey="id"
              pagination={{
                showTotal: (total: number) => `共 ${total} 个员工档案`,
              }}
            />
          ) : (
            <Empty description="暂无员工档案" />
          )}
        </Card>
      ) : (
        <Card
          title={
            <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
              <Avatar size={64} icon={<UserOutlined />} />
              <div>
                <Text strong style={{ fontSize: 18 }}>{currentProfile?.name}</Text>
                <div style={{ fontSize: 14, color: '#666' }}>{currentProfile?.employee_id}</div>
              </div>
              <Button type="primary" onClick={() => setActiveTab('list')} style={{ marginLeft: 'auto' }}>
                返回列表
              </Button>
            </div>
          }
        >
          <Tabs defaultActiveKey="basic" style={{ marginTop: 24 }}>
            <Tabs.TabPane tab="基本信息" key="basic">
              <Descriptions column={2} bordered>
                <Descriptions.Item label="姓名" span={1}>{currentProfile?.name}</Descriptions.Item>
                <Descriptions.Item label="工号" span={1}>{currentProfile?.employee_id}</Descriptions.Item>
                <Descriptions.Item label="性别" span={1}>{currentProfile?.gender}</Descriptions.Item>
                <Descriptions.Item label="出生日期" span={1}>{currentProfile?.birth_date}</Descriptions.Item>
                <Descriptions.Item label="国籍" span={1}>{currentProfile?.nationality}</Descriptions.Item>
                <Descriptions.Item label="身份证号" span={1}>{currentProfile?.id_card_number}</Descriptions.Item>
                <Descriptions.Item label="工作邮箱" span={1}>{currentProfile?.work_email}</Descriptions.Item>
                <Descriptions.Item label="个人邮箱" span={1}>{currentProfile?.personal_email}</Descriptions.Item>
                <Descriptions.Item label="紧急联系人" span={1}>{currentProfile?.emergency_contact}</Descriptions.Item>
                <Descriptions.Item label="紧急联系电话" span={1}>{currentProfile?.emergency_phone}</Descriptions.Item>
                <Descriptions.Item label="地址" span={2}>{currentProfile?.address}</Descriptions.Item>
              </Descriptions>
            </Tabs.TabPane>
            <Tabs.TabPane tab="工作信息" key="work">
              <Descriptions column={2} bordered>
                <Descriptions.Item label="雇佣类型" span={1}>{currentProfile?.employment_type}</Descriptions.Item>
                <Descriptions.Item label="入职日期" span={1}>{currentProfile?.entry_date}</Descriptions.Item>
                <Descriptions.Item label="试用期结束日期" span={1}>{currentProfile?.probation_end_date}</Descriptions.Item>
                <Descriptions.Item label="合同开始日期" span={1}>{currentProfile?.contract_start_date}</Descriptions.Item>
                <Descriptions.Item label="合同结束日期" span={1}>{currentProfile?.contract_end_date}</Descriptions.Item>
                <Descriptions.Item label="状态" span={1}>
                  <Text type={currentProfile?.profile_status === 'active' ? 'success' : 'warning'}>
                    {currentProfile?.profile_status === 'active' ? '在职' : '离职'}
                  </Text>
                </Descriptions.Item>
              </Descriptions>
              <Divider orientation="left">工作经历</Divider>
              {currentProfile?.work_experience?.experiences?.map((exp: any, index: number) => (
                <Card key={index} style={{ marginBottom: 12 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Text strong>{exp.company}</Text>
                    <Text type="secondary">{exp.start_date} - {exp.end_date}</Text>
                  </div>
                  <div style={{ marginTop: 8 }}>
                    <Text>职位：{exp.position}</Text>
                  </div>
                  <div style={{ marginTop: 4 }}>
                    <Text type="secondary">{exp.description}</Text>
                  </div>
                </Card>
              ))}
            </Tabs.TabPane>
            <Tabs.TabPane tab="教育背景" key="education">
              <Descriptions column={2} bordered>
                <Descriptions.Item label="学历" span={1}>{currentProfile?.education}</Descriptions.Item>
                <Descriptions.Item label="毕业院校" span={1}>{currentProfile?.graduate_school}</Descriptions.Item>
                <Descriptions.Item label="专业" span={1}>{currentProfile?.major}</Descriptions.Item>
                <Descriptions.Item label="毕业日期" span={1}>{currentProfile?.graduation_date}</Descriptions.Item>
              </Descriptions>
            </Tabs.TabPane>
            <Tabs.TabPane tab="技能证书" key="skills">
              {currentProfile?.skills?.certificates?.map((cert: any, index: number) => (
                <Card key={index} style={{ marginBottom: 12 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Text strong>{cert.name}</Text>
                    <Text type="secondary">颁发日期：{cert.issue_date}</Text>
                  </div>
                  <div style={{ marginTop: 4 }}>
                    <Text type="secondary">有效期至：{cert.expiry_date}</Text>
                  </div>
                </Card>
              ))}
            </Tabs.TabPane>
            <Tabs.TabPane tab="财务信息" key="finance">
              <Descriptions column={2} bordered>
                <Descriptions.Item label="银行账号" span={1}>{currentProfile?.bank_account}</Descriptions.Item>
                <Descriptions.Item label="银行名称" span={1}>{currentProfile?.bank_name}</Descriptions.Item>
                <Descriptions.Item label="税号" span={2}>{currentProfile?.tax_number}</Descriptions.Item>
              </Descriptions>
            </Tabs.TabPane>
          </Tabs>
        </Card>
      )}

      <Modal
        title={currentProfile ? '编辑员工档案' : '新建员工档案'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setModalVisible(false)}>
            取消
          </Button>,
          <Button
            key="submit"
            type="primary"
            onClick={currentProfile ? handleUpdateProfile : handleCreateProfile}
            loading={currentProfile ? updateProfileMutation.isPending : createProfileMutation.isPending}
          >
            确认
          </Button>,
        ]}
        width={800}
      >
        <Form form={form} layout="vertical">
          <Tabs defaultActiveKey="basic">
            <Tabs.TabPane tab="基本信息" key="basic">
              <Form.Item
                name="user_id"
                label="用户ID"
                rules={[{ required: true, message: '请输入用户ID' }]}
              >
                <Input placeholder="请输入用户ID" />
              </Form.Item>
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
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item
                name="nationality"
                label="国籍"
              >
                <Input placeholder="请输入国籍" />
              </Form.Item>
              <Form.Item
                name="id_card_number"
                label="身份证号"
              >
                <Input placeholder="请输入身份证号" />
              </Form.Item>
              <Form.Item
                name="work_email"
                label="工作邮箱"
              >
                <Input placeholder="请输入工作邮箱" />
              </Form.Item>
              <Form.Item
                name="personal_email"
                label="个人邮箱"
              >
                <Input placeholder="请输入个人邮箱" />
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
                name="address"
                label="地址"
              >
                <Input.TextArea placeholder="请输入地址" rows={3} />
              </Form.Item>
            </Tabs.TabPane>
            <Tabs.TabPane tab="工作信息" key="work">
              <Form.Item
                name="employment_type"
                label="雇佣类型"
              >
                <Select placeholder="请选择雇佣类型">
                  <Option value="全职">全职</Option>
                  <Option value="兼职">兼职</Option>
                  <Option value="实习">实习</Option>
                </Select>
              </Form.Item>
              <Form.Item
                name="entry_date"
                label="入职日期"
              >
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item
                name="probation_end_date"
                label="试用期结束日期"
              >
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item
                name="contract_start_date"
                label="合同开始日期"
              >
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item
                name="contract_end_date"
                label="合同结束日期"
              >
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Tabs.TabPane>
            <Tabs.TabPane tab="教育背景" key="education">
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
              <Form.Item
                name="graduation_date"
                label="毕业日期"
              >
                <DatePicker style={{ width: '100%' }} />
              </Form.Item>
            </Tabs.TabPane>
            <Tabs.TabPane tab="财务信息" key="finance">
              <Form.Item
                name="bank_account"
                label="银行账号"
              >
                <Input placeholder="请输入银行账号" />
              </Form.Item>
              <Form.Item
                name="bank_name"
                label="银行名称"
              >
                <Input placeholder="请输入银行名称" />
              </Form.Item>
              <Form.Item
                name="tax_number"
                label="税号"
              >
                <Input placeholder="请输入税号" />
              </Form.Item>
            </Tabs.TabPane>
          </Tabs>
        </Form>
      </Modal>
    </div>
  )
}

export default EmployeeProfile