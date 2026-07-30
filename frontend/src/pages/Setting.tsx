import React, { useState } from 'react'
import { Typography, Button, Spin, Empty, Alert, Row, Col, Tooltip } from 'antd'
import { SettingOutlined, SyncOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { syncAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import { formatDateTime } from '../utils/format'
import {
  canSyncOrgData,
  confirmOrgSync,
  missingOrgSyncPermissionTip,
} from '../utils/orgSyncAction'

const { Text } = Typography

const Setting: React.FC = () => {
  const [syncing, setSyncing] = useState(false)
  const canSync = canSyncOrgData()

  const { data: syncStatus, isLoading, isError, refetch: refetchSyncStatus } = useQuery({
    queryKey: ['syncStatus'],
    queryFn: async () => {
      const res = await syncAPI.getSyncStatus()
      return res.data.status
    },
  })

  // 多租户：普通设置页只能同步当前登录组织，跨组织同步须走受控运维入口。
  const handleSyncCurrentOrg = () => {
    if (!canSync) return
    confirmOrgSync({
      title: '同步当前组织花名册',
      content: '将从钉钉重新拉取当前登录组织的部门与员工数据并写入本系统，可能耗时较长。确认开始同步？',
      onStart: () => setSyncing(true),
      onSettled: () => setSyncing(false),
      onCompleted: async () => {
        await refetchSyncStatus()
      },
    })
  }

  return (
    <PageContainer
      title="系统设置"
      icon={<SettingOutlined />}
      subtitle="查看同步状态；密钥与运行参数仅由服务端环境变量/运维入口配置"
    >
      <Row gutter="var(--space-4)" style={{ marginTop: 'var(--space-4)' }}>
        <Col span={12}>
          <PageCard title="系统配置">
            <Alert
              type="info"
              showIcon
              message="密钥不在此页配置"
              description={
                <div>
                  <p style={{ marginBottom: 8 }}>
                    钉钉 AppKey/AppSecret、JWT Secret 等敏感配置仅通过服务端环境变量或受控运维入口维护，本页不提供保存能力，避免假成功与密钥误录入。
                  </p>
                  <Text type="secondary">如需变更请联系运维，勿在浏览器表单中填写真实密钥。</Text>
                </div>
              }
            />
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
                    <span>{syncStatus.departments?.status === 'success' ? '成功' : (syncStatus.departments?.status || '未知')}</span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <span>上次同步时间</span>
                    <span>{formatDateTime(syncStatus.departments?.last_sync_time)}</span>
                  </div>
                </div>
                <div style={{ marginBottom: 'var(--space-4)' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 'var(--space-2)' }}>
                    <span>用户同步状态</span>
                    <span>{syncStatus.users?.status === 'success' ? '成功' : (syncStatus.users?.status || '未知')}</span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <span>上次同步时间</span>
                    <span>{formatDateTime(syncStatus.users?.last_sync_time)}</span>
                  </div>
                </div>
                <Alert
                  style={{ marginBottom: 'var(--space-4)' }}
                  type="info"
                  showIcon
                  message="花名册同步只作用于当前登录的组织；跨组织同步请通过运维入口执行。"
                />
                <div style={{ display: 'flex', gap: 'var(--space-3)', flexWrap: 'wrap' }}>
                  <Tooltip title={canSync ? undefined : missingOrgSyncPermissionTip}>
                    <Button
                      type="primary"
                      icon={<SyncOutlined />}
                      loading={syncing}
                      disabled={!canSync}
                      onClick={handleSyncCurrentOrg}
                    >
                      同步当前组织花名册
                    </Button>
                  </Tooltip>
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
