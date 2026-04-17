import React, { useEffect, useState } from 'react'
import { Card, Descriptions, Avatar, Button, message, Spin, Typography, Tag, Divider } from 'antd'
import { ArrowLeftOutlined, EditOutlined, SyncOutlined } from '@ant-design/icons'
import { useNavigate, useParams } from 'react-router-dom'
import { orgAPI } from '../services/api'

const { Title } = Typography

const EmployeeDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>()
  const [employee, setEmployee] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  useEffect(() => {
    if (id) {
      fetchEmployeeDetail()
    }
  }, [id])

  const fetchEmployeeDetail = async () => {
    setLoading(true)
    try {
      const response = await orgAPI.getEmployee(id!)
      setEmployee(response.data.employee)
    } catch (error) {
      message.error('获取员工详情失败')
    } finally {
      setLoading(false)
    }
  }

  const handleBack = () => {
    navigate('/employees')
  }

  const handleSync = async () => {
    setLoading(true)
    try {
      await orgAPI.syncOrg()
      message.success('同步成功')
      fetchEmployeeDetail()
    } catch (error) {
      message.error('同步失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div>
      <Button 
        icon={<ArrowLeftOutlined />} 
        onClick={handleBack}
        style={{ marginBottom: '16px' }}
      >
        返回员工列表
      </Button>
      <Card 
        title={<Title level={4}>员工详情</Title>} 
        extra={
          <Button 
            type="primary" 
            icon={<SyncOutlined />} 
            onClick={handleSync}
            loading={loading}
          >
            同步数据
          </Button>
        }
      >
        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : employee ? (
          <div>
            <div style={{ display: 'flex', alignItems: 'center', marginBottom: '24px' }}>
              <Avatar size={80} src={employee.avatar} />
              <div style={{ marginLeft: '24px' }}>
                <h2 style={{ margin: 0 }}>{employee.name}</h2>
                <p style={{ margin: '8px 0' }}>{employee.position}</p>
                <div style={{ display: 'flex', gap: '8px' }}>
                  {employee.extension?.tags?.map((tag: string, index: number) => (
                    <Tag key={index}>{tag}</Tag>
                  ))}
                </div>
              </div>
            </div>
            <Divider />
            <Descriptions column={2}>
              <Descriptions.Item label="员工ID">{employee.user_id}</Descriptions.Item>
              <Descriptions.Item label="邮箱">{employee.email}</Descriptions.Item>
              <Descriptions.Item label="手机号">{employee.mobile}</Descriptions.Item>
              <Descriptions.Item label="部门">{employee.department_id}</Descriptions.Item>
              <Descriptions.Item label="状态">{employee.status}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{employee.created_at}</Descriptions.Item>
              <Descriptions.Item label="更新时间" span={2}>{employee.updated_at}</Descriptions.Item>
              <Descriptions.Item label="备注" span={2}>{employee.extension?.remarks || '-'}</Descriptions.Item>
              <Descriptions.Item label="自定义字段" span={2}>{employee.extension?.custom || '-'}</Descriptions.Item>
            </Descriptions>
          </div>
        ) : (
          <div>员工不存在</div>
        )}
      </Card>
    </div>
  )
}

export default EmployeeDetail