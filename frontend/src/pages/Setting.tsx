import React, { useState } from 'react'
import { Typography, Form, Input, Button, Spin, Empty, Alert, message, Row, Col } from 'antd'
import { SettingOutlined, SyncOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { orgAPI, syncAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import { formatDateTime } from '../utils/format'

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

  // 多租户：普通设置页只能同步当前登录组织，跨组织同步须走受控运维入口。
  const handleSyncCurrentOrg = async () => {
    setSyncing(true)
    try {
      await orgAPI.syncOrg()
      message.success('当前组织花名册同步成功')
      await refetchSyncStatus()
    } catch (error) {
      message.error('当前组织花名册同步失败')
    } finally {
      setSyncing(false)
    }
  }

  const onFinish = () => {
    message.success('配置保存成功')
  }

  return (
    <PageContainer
      title="系统设置"
      icon={<SettingOutlined />}
      subtitle="管理系统配置与同步设置"
    >
      <Row gutter="var(--space-4)" style={{ marginTop: 'var(--space-4)' }}>
        <Col span={12}>
          <PageCard title="系统配置">
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
          </PageCard>
        </Col>
        <Col span={12}>
          <PageCard title="同步设置">
            {isLoading ? (
              <div className="loading-container">
                <Spin size="small" />
              </div>
            ) : isError ? (
              <div className="error-container">
                <Alert message="加载失败" type="error" showIcon />
                <Button className="retry-button" onClick={() => refetchSyncStatus()}>重试</Button>
              </div>
            ) : syncStatus ? (
              <div>
                <div style={{ marginBottom: 'var(--space-4)' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 'var(--space-2)' }}>
                    <span>部门同步状态</span>
                    <span>{syncStatus.departments.status === 'success' ? '成功' : '失败'}</span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <span>上次同步时间</span>
                    <span>{formatDateTime(syncStatus.departments.last_sync_time)}</span>
                  </div>
                </div>
                <div style={{ marginBottom: 'var(--space-4)' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 'var(--space-2)' }}>
                    <span>用户同步状态</span>
                    <span>{syncStatus.users.status === 'success' ? '成功' : '失败'}</span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <span>上次同步时间</span>
                    <span>{formatDateTime(syncStatus.users.last_sync_time)}</span>
                  </div>
                </div>
                <Alert
                  style={{ marginBottom: 'var(--space-4)' }}
                  type="info"
                  showIcon
                  message="花名册同步只作用于当前登录的组织；跨组织同步请通过运维入口执行。"
                />
                <div style={{ display: 'flex', gap: 'var(--space-3)', flexWrap: 'wrap' }}>
                  <Button
                    type="primary"
                    icon={<SyncOutlined />}
                    loading={syncing}
                    onClick={handleSyncCurrentOrg}
                  >
                    同步当前组织花名册
                  </Button>
                </div>
              </div>
            ) : (
              <div className="empty-container">
                <Empty description="暂无同步状态" imageStyle={{ height: 80 }} />
              </div>
            )}
          </PageCard>
        </Col>
      </Row>
    </PageContainer>
  )
}

export default Setting
