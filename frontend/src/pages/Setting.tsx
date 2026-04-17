import React, { useState } from 'react'
import { Card, Typography, Form, Input, Button, Spin, Empty, Alert, message, Row, Col } from 'antd'
import { SettingOutlined, SyncOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'

const { Title } = Typography

// 模拟API调用
const fetchSyncStatus = async () => {
  await new Promise(resolve => setTimeout(resolve, 1000))
  return {
    departments: {
      last_sync_time: '2024-01-01T00:00:00Z',
      status: 'success'
    },
    users: {
      last_sync_time: '2024-01-01T00:00:00Z',
      status: 'success'
    }
  }
}

const Setting: React.FC = () => {
  const [syncing, setSyncing] = useState(false)

  const { data: syncStatus, isLoading, isError, refetch: refetchSyncStatus } = useQuery({
    queryKey: ['syncStatus'],
    queryFn: fetchSyncStatus
  })

  const handleSync = async (type: string) => {
    setSyncing(true)
    try {
      // 模拟同步请求
      await new Promise(resolve => setTimeout(resolve, 2000))
      message.success(`${type === 'departments' ? '部门' : '用户'}同步成功`)
      refetchSyncStatus()
    } catch (error) {
      message.error(`${type === 'departments' ? '部门' : '用户'}同步失败`)
    } finally {
      setSyncing(false)
    }
  }

  const onFinish = (values: any) => {
    console.log('Form values:', values)
    message.success('配置保存成功')
  }

  return (
    <div>
      <Title level={4}>系统设置</Title>
      <Row gutter={16}>
        <Col span={12}>
          <Card title="系统配置">
            <Form
              layout="vertical"
              onFinish={onFinish}
            >
              <Form.Item label="钉钉App Key" name="appKey">
                <Input placeholder="请输入钉钉App Key" />
              </Form.Item>
              <Form.Item label="钉钉App Secret" name="appSecret">
                <Input.Password placeholder="请输入钉钉App Secret" />
              </Form.Item>
              <Form.Item label="JWT Secret" name="jwtSecret">
                <Input.Password placeholder="请输入JWT Secret" />
              </Form.Item>
              <Form.Item>
                <Button type="primary" htmlType="submit">
                  保存配置
                </Button>
              </Form.Item>
            </Form>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="同步设置">
            {isLoading ? (
              <div className="loading-container">
                <Spin size="small" />
              </div>
            ) : isError ? (
              <div className="error-container">
                <Alert message="加载失败" type="error" showIcon />
                <Button className="retry-button" onClick={refetchSyncStatus}>重试</Button>
              </div>
            ) : syncStatus ? (
              <div>
                <div style={{ marginBottom: 16 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                    <span>部门同步状态</span>
                    <span>{syncStatus.departments.status === 'success' ? '成功' : '失败'}</span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <span>上次同步时间</span>
                    <span>{new Date(syncStatus.departments.last_sync_time).toLocaleString()}</span>
                  </div>
                </div>
                <div style={{ marginBottom: 16 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                    <span>用户同步状态</span>
                    <span>{syncStatus.users.status === 'success' ? '成功' : '失败'}</span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <span>上次同步时间</span>
                    <span>{new Date(syncStatus.users.last_sync_time).toLocaleString()}</span>
                  </div>
                </div>
                <div style={{ display: 'flex', gap: 16 }}>
                  <Button 
                    type="primary" 
                    icon={<SyncOutlined />} 
                    loading={syncing}
                    onClick={() => handleSync('departments')}
                  >
                    同步部门
                  </Button>
                  <Button 
                    type="primary" 
                    icon={<SyncOutlined />} 
                    loading={syncing}
                    onClick={() => handleSync('users')}
                  >
                    同步用户
                  </Button>
                </div>
              </div>
            ) : (
              <div className="empty-container">
                <Empty description="暂无同步状态" />
              </div>
            )}
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default Setting