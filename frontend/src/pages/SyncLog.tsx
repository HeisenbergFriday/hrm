import React, { useEffect, useState } from 'react'
import { Card, Table, Button, message, Spin, Typography, DatePicker } from 'antd'
import { SyncOutlined, ReloadOutlined } from '@ant-design/icons'
import { orgAPI, syncAPI } from '../services/api'

// 格式化时间函数
const formatDateTime = (dateString: string): string => {
  const date = new Date(dateString)
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  const seconds = String(date.getSeconds()).padStart(2, '0')
  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}

const { Title } = Typography
const { RangePicker } = DatePicker

const SyncLog: React.FC = () => {
  const [logs, setLogs] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [syncStatus, setSyncStatus] = useState<any>(null)

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
      const generatedLogs = []
      
      if (status.departments) {
        generatedLogs.push({
          id: 1,
          type: 'departments',
          status: status.departments.status,
          count: 0, // 从消息中提取数量
          sync_time: status.departments.last_sync_time,
          duration: '未知',
        })
      }
      
      if (status.users) {
        generatedLogs.push({
          id: 2,
          type: 'employees',
          status: status.users.status,
          count: 0, // 从消息中提取数量
          sync_time: status.users.last_sync_time,
          duration: '未知',
        })
      }
      
      setLogs(generatedLogs)
    } catch (error) {
      message.error('获取同步日志失败')
    } finally {
      setLoading(false)
    }
  }

  const handleSync = async () => {
    setLoading(true)
    try {
      await orgAPI.syncOrg()
      message.success('同步成功')
      fetchSyncStatus()
      fetchSyncLogs()
    } catch (error) {
      message.error('同步失败')
    } finally {
      setLoading(false)
    }
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
      render: (status: string) => {
        return status === 'success' ? (
          <span style={{ color: 'green' }}>成功</span>
        ) : (
          <span style={{ color: 'red' }}>失败</span>
        )
      },
    },
    {
      title: '同步数量',
      dataIndex: 'count',
      key: 'count',
    },
    {
      title: '同步时间',
      dataIndex: 'sync_time',
      key: 'sync_time',
      render: (syncTime: string) => formatDateTime(syncTime),
    },
    {
      title: '耗时',
      dataIndex: 'duration',
      key: 'duration',
    },
  ]

  return (
    <div>
      <Card 
        title={<Title level={4}>同步日志</Title>} 
        extra={
          <Button 
            type="primary" 
            icon={<SyncOutlined />} 
            onClick={handleSync}
            loading={loading}
          >
            手动同步
          </Button>
        }
      >
        <div style={{ marginBottom: '24px' }}>
          <h3>同步状态</h3>
          {syncStatus && (
            <div style={{ display: 'flex', gap: '24px', marginTop: '8px' }}>
              <div>
                <p>部门同步状态: {syncStatus.departments.status}</p>
                <p>最后同步时间: {formatDateTime(syncStatus.departments.last_sync_time)}</p>
              </div>
              <div>
                <p>员工同步状态: {syncStatus.users.status}</p>
                <p>最后同步时间: {formatDateTime(syncStatus.users.last_sync_time)}</p>
              </div>
            </div>
          )}
        </div>
        <div style={{ marginBottom: '16px', display: 'flex', gap: '16px', alignItems: 'center' }}>
          <RangePicker style={{ width: 300 }} />
          <Button 
            icon={<ReloadOutlined />} 
            onClick={fetchSyncLogs}
          >
            刷新
          </Button>
        </div>
        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
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
      </Card>
    </div>
  )
}

export default SyncLog