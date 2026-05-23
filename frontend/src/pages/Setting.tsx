import React, { useState } from 'react'
import { Card, Typography, Form, Input, Button, Spin, Empty, Alert, message, Row, Col } from 'antd'
import { SettingOutlined, SyncOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { syncAPI } from '../services/api'

const { Text } = Typography

const Setting: React.FC = () => {
  const [syncing, setSyncing] = useState(false)

  const { data: syncStatus, isLoading, isError, refetch: refetchSyncStatus } = useQuery({
    queryKey: ['syncStatus'],
    queryFn: async () => {
      const res = await syncAPI.getSyncStatus()
      return res.data?.data?.status || res.data?.data
    }
  })

  const handleSync = async (type: string) => {
    setSyncing(true)
    try {
      const fn = type === 'departments' ? syncAPI.syncDepartments : syncAPI.syncUsers
      await fn()
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
    <div style={{ padding: '20px 28px', background: '#e4e8ee', minHeight: '100vh' }}>
      <h2 style={{ margin: '0 0 4px', fontSize: 22, fontWeight: 700, color: '#111827' }}>
        <SettingOutlined style={{ color: '#4338ca', marginRight: 8 }} />系统设置
      </h2>
      <Text style={{ color: '#6b7280', fontSize: 13.5 }}>管理系统配置与同步设置</Text>
      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title="系统配置" style={{ borderRadius: 14, border: '1px solid #e5e7eb', boxShadow: '0 2px 10px rgba(0,0,0,0.05)' }} styles={{ header: { background: '#fafbfc', borderBottom: '1px solid #f0f0f0' } }}>
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
                <Button type="primary" htmlType="submit" style={{ borderRadius: 8, fontWeight: 600 }}>
                  保存配置
                </Button>
              </Form.Item>
            </Form>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="同步设置" style={{ borderRadius: 14, border: '1px solid #e5e7eb', boxShadow: '0 2px 10px rgba(0,0,0,0.05)' }} styles={{ header: { background: '#fafbfc', borderBottom: '1px solid #f0f0f0' } }}>
            {isLoading ? (
              <div className="loading-container">
                <Spin size="small" />
              </div>
            ) : isError ? (
              <div className="error-container">
                <Alert message="加载失败" type="error" showIcon />
                <Button className="retry-button" onClick={() => refetchSyncStatus()} style={{ borderRadius: 8, fontWeight: 600 }}>重试</Button>
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
                    style={{ borderRadius: 8, fontWeight: 600 }}
                  >
                    同步部门
                  </Button>
                  <Button
                    type="primary"
                    icon={<SyncOutlined />}
                    loading={syncing}
                    onClick={() => handleSync('users')}
                    style={{ borderRadius: 8, fontWeight: 600 }}
                  >
                    同步用户
                  </Button>
                </div>
              </div>
            ) : (
              <div className="empty-container">
                <Empty description="暂无同步状态" imageStyle={{ height: 80 }} />
              </div>
            )}
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default Setting
