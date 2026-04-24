import React, { useEffect, useState } from 'react'
import {
  Card, Descriptions, Avatar, Button, message, Spin, Typography,
  Tag, Divider, Form, Input, DatePicker, Select, Modal
} from 'antd'
import { ArrowLeftOutlined, SyncOutlined, EditOutlined } from '@ant-design/icons'
import { useNavigate, useParams } from 'react-router-dom'
import { orgAPI, employeeAPI } from '../services/api'
import dayjs from 'dayjs'

const { Title } = Typography

const EmployeeDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>()
  const [employee, setEmployee] = useState<any>(null)
  const [profile, setProfile] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const [editOpen, setEditOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm()
  const navigate = useNavigate()

  useEffect(() => {
    if (id) fetchDetail()
  }, [id])

  const fetchDetail = async () => {
    setLoading(true)
    try {
      const res = await orgAPI.getEmployee(id!)
      setEmployee(res.data.employee)
      setProfile(res.data.profile || null)
    } catch {
      message.error('获取员工详情失败')
    } finally {
      setLoading(false)
    }
  }

  const handleSync = async () => {
    setLoading(true)
    try {
      await orgAPI.syncOrg()
      message.success('同步成功')
      fetchDetail()
    } catch {
      message.error('同步失败')
    } finally {
      setLoading(false)
    }
  }

  const openEdit = () => {
    form.setFieldsValue({
      entry_date: profile?.entry_date ? dayjs(profile.entry_date) : null,
      probation_end_date: profile?.probation_end_date ? dayjs(profile.probation_end_date) : null,
      contract_start_date: profile?.contract_start_date ? dayjs(profile.contract_start_date) : null,
      contract_end_date: profile?.contract_end_date ? dayjs(profile.contract_end_date) : null,
      employment_type: profile?.employment_type || '',
      gender: profile?.gender || '',
      birth_date: profile?.birth_date ? dayjs(profile.birth_date) : null,
      education: profile?.education || '',
      address: profile?.address || '',
      emergency_contact: profile?.emergency_contact || '',
      emergency_phone: profile?.emergency_phone || '',
    })
    setEditOpen(true)
  }

  const handleSave = async () => {
    const vals = await form.validateFields()
    setSaving(true)
    try {
      const payload: Record<string, string> = {}
      const dateFields = ['entry_date', 'probation_end_date', 'contract_start_date', 'contract_end_date', 'birth_date']
      for (const key of Object.keys(vals)) {
        if (dateFields.includes(key)) {
          payload[key] = vals[key] ? (vals[key] as dayjs.Dayjs).format('YYYY-MM-DD') : ''
        } else {
          payload[key] = vals[key] || ''
        }
      }
      if (profile?.id) {
        await employeeAPI.updateProfile(String(profile.id), payload)
      } else {
        await employeeAPI.createProfile({ ...payload, user_id: employee.user_id, employee_id: employee.user_id })
      }
      message.success('保存成功')
      setEditOpen(false)
      fetchDetail()
    } catch {
      message.error('保存失败')
    } finally {
      setSaving(false)
    }
  }

  const renderTag = (status: string) => {
    const map: Record<string, string> = { active: 'green', inactive: 'red' }
    return <Tag color={map[status] || 'default'}>{status === 'active' ? '在职' : status === 'inactive' ? '离职' : status}</Tag>
  }

  return (
    <div>
      <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/employees')} style={{ marginBottom: 16 }}>
        返回员工列表
      </Button>

      <Card
        title={<Title level={4}>员工详情</Title>}
        extra={
          <Button.Group>
            <Button icon={<EditOutlined />} onClick={openEdit} disabled={loading}>编辑档案</Button>
            <Button type="primary" icon={<SyncOutlined />} onClick={handleSync} loading={loading}>同步数据</Button>
          </Button.Group>
        }
      >
        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 40 }}>
            <Spin size="large" />
          </div>
        ) : employee ? (
          <>
            {/* 头部 */}
            <div style={{ display: 'flex', alignItems: 'center', marginBottom: 24 }}>
              <Avatar size={80} src={employee.avatar} />
              <div style={{ marginLeft: 24 }}>
                <h2 style={{ margin: 0 }}>{employee.name}</h2>
                <p style={{ margin: '4px 0', color: '#666' }}>{employee.position}</p>
                {renderTag(employee.status)}
              </div>
            </div>

            <Divider orientation="left">基本信息</Divider>
            <Descriptions column={2}>
              <Descriptions.Item label="员工ID">{employee.user_id}</Descriptions.Item>
              <Descriptions.Item label="部门ID">{employee.department_id}</Descriptions.Item>
              <Descriptions.Item label="邮箱">{employee.email}</Descriptions.Item>
              <Descriptions.Item label="手机号">{employee.mobile}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{employee.created_at?.slice(0, 10)}</Descriptions.Item>
              <Descriptions.Item label="更新时间">{employee.updated_at?.slice(0, 10)}</Descriptions.Item>
            </Descriptions>

            <Divider orientation="left">员工档案</Divider>
            {profile ? (
              <Descriptions column={2}>
                <Descriptions.Item label="入职日期">
                  {profile.entry_date || <span style={{ color: '#999' }}>未填写</span>}
                </Descriptions.Item>
                <Descriptions.Item label="转正日期">
                  {profile.probation_end_date || <span style={{ color: '#999' }}>未填写</span>}
                </Descriptions.Item>
                <Descriptions.Item label="合同开始">
                  {profile.contract_start_date || <span style={{ color: '#999' }}>未填写</span>}
                </Descriptions.Item>
                <Descriptions.Item label="合同结束">
                  {profile.contract_end_date || <span style={{ color: '#999' }}>未填写</span>}
                </Descriptions.Item>
                <Descriptions.Item label="雇佣类型">{profile.employment_type || '-'}</Descriptions.Item>
                <Descriptions.Item label="性别">{profile.gender || '-'}</Descriptions.Item>
                <Descriptions.Item label="出生日期">{profile.birth_date || '-'}</Descriptions.Item>
                <Descriptions.Item label="学历">{profile.education || '-'}</Descriptions.Item>
                <Descriptions.Item label="工作邮箱">{profile.work_email || '-'}</Descriptions.Item>
                <Descriptions.Item label="紧急联系人">{profile.emergency_contact || '-'}</Descriptions.Item>
                <Descriptions.Item label="紧急联系电话">{profile.emergency_phone || '-'}</Descriptions.Item>
                <Descriptions.Item label="地址" span={2}>{profile.address || '-'}</Descriptions.Item>
              </Descriptions>
            ) : (
              <div style={{ color: '#999', padding: '8px 0' }}>
                暂无档案，点击「编辑档案」创建
              </div>
            )}
          </>
        ) : (
          <div>员工不存在</div>
        )}
      </Card>

      {/* 编辑弹窗 */}
      <Modal
        title="编辑员工档案"
        open={editOpen}
        onCancel={() => setEditOpen(false)}
        onOk={handleSave}
        confirmLoading={saving}
        width={600}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="entry_date" label="入职日期">
            <DatePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="probation_end_date" label="转正日期（试用期结束）">
            <DatePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="contract_start_date" label="合同开始日期">
            <DatePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="contract_end_date" label="合同结束日期">
            <DatePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="employment_type" label="雇佣类型">
            <Select allowClear>
              <Select.Option value="全职">全职</Select.Option>
              <Select.Option value="兼职">兼职</Select.Option>
              <Select.Option value="实习">实习</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="gender" label="性别">
            <Select allowClear>
              <Select.Option value="男">男</Select.Option>
              <Select.Option value="女">女</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="birth_date" label="出生日期">
            <DatePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="education" label="学历">
            <Input />
          </Form.Item>
          <Form.Item name="emergency_contact" label="紧急联系人">
            <Input />
          </Form.Item>
          <Form.Item name="emergency_phone" label="紧急联系电话">
            <Input />
          </Form.Item>
          <Form.Item name="address" label="地址">
            <Input />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default EmployeeDetail
