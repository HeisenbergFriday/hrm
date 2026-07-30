import React, { useEffect, useState } from 'react'
import { Table, Button, message, Spin, DatePicker, Tooltip, Typography } from 'antd'
import { SyncOutlined, ReloadOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import datePickerZhCN from 'antd/es/date-picker/locale/zh_CN'
import { syncAPI, type OrgSyncStatusRecord } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import { formatDateTime } from '../utils/format'
import {
  canSyncOrgData,
  confirmOrgSync,
  missingOrgSyncPermissionTip,
} from '../utils/orgSyncAction'

dayjs.locale('zh-cn')

const { RangePicker } = DatePicker
const { Text } = Typography

type SyncLogRow = OrgSyncStatusRecord & {
  id: string
  type: 'departments' | 'employees'
}

const syncStatusLabel = (status: OrgSyncStatusRecord['status']) => {
  switch (status) {
    case 'success':
      return <StatusTag color="success">成功</StatusTag>
    case 'partial_failed':
      return <StatusTag color="warning">部分成功</StatusTag>
    case 'failed':
      return <StatusTag color="error">失败</StatusTag>
    case 'skipped':
      return <StatusTag color="default">已跳过</StatusTag>
    default:
      return <StatusTag color="default">未同步</StatusTag>
  }
}

const formatDuration = (durationMS?: number) => {
  if (durationMS === undefined || durationMS < 0) return '未知'
  if (durationMS < 1000) return `${durationMS} 毫秒`
  return `${(durationMS / 1000).toFixed(2)} 秒`
}

const SyncLog: React.FC = () => {
  const [logs, setLogs] = useState<SyncLogRow[]>([])
  const [loading, setLoading] = useState(true)
  const [syncing, setSyncing] = useState(false)
  const [syncStatus, setSyncStatus] = useState<Record<string, OrgSyncStatusRecord> | null>(null)

  useEffect(() => {
    fetchSyncStatus()
    fetchSyncLogs()
  }, [])

  const fetchSyncStatus = async () => {
    try {
      const response = await syncAPI.getSyncStatus()
      setSyncStatus(response.data.status)
    } catch (error) {
      message.error('获取同步状态失败')
    }
  }

  const fetchSyncLogs = async () => {
    setLoading(true)
    try {
      // 从同步状态生成日志数据
      const response = await syncAPI.getSyncStatus()
      const status = response.data.status
      
      // 转换为日志格式
      const generatedLogs: SyncLogRow[] = []
      
      if (status.departments) {
        generatedLogs.push({
          id: 'departments',
          type: 'departments',
          ...status.departments,
        })
      }
      
      if (status.users) {
        generatedLogs.push({
          id: 'employees',
          type: 'employees',
          ...status.users,
        })
      }
      
      setLogs(generatedLogs)
    } catch (error) {
      message.error('获取同步日志失败')
    } finally {
      setLoading(false)
    }
  }

  const canSync = canSyncOrgData()

  const handleSync = () => {
    if (!canSync) return
    confirmOrgSync({
      onStart: () => setSyncing(true),
      onSettled: () => setSyncing(false),
      onCompleted: async () => {
        setLoading(true)
        try {
          await fetchSyncStatus()
          await fetchSyncLogs()
        } finally {
          setLoading(false)
        }
      },
    })
  }

  const columns = [
    {
      title: '同步类型',
      dataIndex: 'type',
      key: 'type',
      render: (type: string) => {
        return type === 'departments' ? '部门' : '员工'
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: OrgSyncStatusRecord['status']) => syncStatusLabel(status),
    },
    {
      title: '同步数量',
      dataIndex: 'success_count',
      key: 'success_count',
      render: (count?: number) => count ?? 0,
    },
    {
      title: '同步时间',
      dataIndex: 'last_sync_time',
      key: 'last_sync_time',
      render: (syncTime: string | null) => syncTime ? formatDateTime(syncTime) : '-',
    },
    {
      title: '耗时',
      dataIndex: 'duration_ms',
      key: 'duration_ms',
      render: (durationMS?: number) => formatDuration(durationMS),
    },
    {
      title: '安全原因',
      dataIndex: 'message',
      key: 'message',
      render: (reason?: string) => reason || '-',
    },
    {
      title: '错误码',
      dataIndex: 'error_code',
      key: 'error_code',
      render: (errorCode?: string) => errorCode ? <Text code>{errorCode}</Text> : '-',
    },
    {
      title: '请求编号',
      dataIndex: 'request_id',
      key: 'request_id',
      render: (requestID?: string) => requestID ? <Text code copyable>{requestID}</Text> : '-',
    },
  ]

  return (
    <PageContainer
      title="同步日志"
      icon={<SyncOutlined />}
      extra={
        <Tooltip title={canSync ? undefined : missingOrgSyncPermissionTip}>
          <Button
            type="primary"
            icon={<SyncOutlined />}
            onClick={handleSync}
            loading={loading || syncing}
            disabled={!canSync}
          >
            手动同步
          </Button>
        </Tooltip>
      }
    >
      <PageCard>
        <div style={{ marginBottom: 'var(--space-6)' }}>
          <h3>同步状态</h3>
          {syncStatus && (
            <div style={{ display: 'flex', gap: 'var(--space-6)', marginTop: 'var(--space-2)' }}>
              <div>
                <p>部门同步状态: {syncStatusLabel(syncStatus.departments.status)}</p>
                <p>最后同步时间: {syncStatus.departments.last_sync_time ? formatDateTime(syncStatus.departments.last_sync_time) : '-'}</p>
              </div>
              <div>
                <p>员工同步状态: {syncStatusLabel(syncStatus.users.status)}</p>
                <p>最后同步时间: {syncStatus.users.last_sync_time ? formatDateTime(syncStatus.users.last_sync_time) : '-'}</p>
              </div>
            </div>
          )}
        </div>
        <div style={{ marginBottom: 'var(--space-4)', display: 'flex', gap: 'var(--space-4)', alignItems: 'center' }}>
          <RangePicker
            style={{ width: 300 }}
            placeholder={['开始日期', '结束日期']}
            format="YYYY-MM-DD"
            locale={datePickerZhCN}
          />
          <Button 
            icon={<ReloadOutlined />} 
            onClick={fetchSyncLogs}
          >
            刷新
          </Button>
        </div>
        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 'var(--space-10)' }}>
            <Spin size="large" />
          </div>
        ) : (
          <Table
            columns={columns}
            dataSource={logs}
            rowKey="id"
            pagination={{
              pageSize: 10,
            }}
          />
        )}
      </PageCard>
    </PageContainer>
  )
}

export default SyncLog
