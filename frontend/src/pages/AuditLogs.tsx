import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, DatePicker, Input, Select } from 'antd'
import { HistoryOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { auditAPI, userAPI } from '../services/api'
import dayjs from 'dayjs'

const { Text } = Typography
const { RangePicker } = DatePicker

interface AuditLog {
  id: string
  user_id: string
  user_name: string
  operation: string
  resource: string
  ip: string
  details: any
  created_at: string
}

const AuditLogs: React.FC = () => {
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null]>([null, null])
  const [searchText, setSearchText] = useState('')
  const [userID, setUserID] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)

  const queryParams = {
    page,
    page_size: pageSize,
    start_date: dateRange[0]?.format('YYYY-MM-DD'),
    end_date: dateRange[1]?.format('YYYY-MM-DD'),
    user_id: userID || undefined,
  }

  const { data: logsData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['audit-logs', queryParams],
    queryFn: () => auditAPI.getLogs(queryParams),
  })

  const { data: usersData } = useQuery({
    queryKey: ['users-list-audit'],
    queryFn: async () => {
      const res = await userAPI.getUsers({ page: 1, page_size: 200 })
      return res.data?.data?.users || []
    },
  })

  const columns = [
    {
      title: '操作人',
      dataIndex: 'user_name',
      key: 'user_name',
      render: (text: string, record: AuditLog) => (
        <div>
          <Text strong>{text}</Text>
          <div style={{ fontSize: 12, color: '#999' }}>{record.user_id}</div>
        </div>
      ),
    },
    {
      title: '操作',
      dataIndex: 'operation',
      key: 'operation',
    },
    {
      title: '资源',
      dataIndex: 'resource',
      key: 'resource',
    },
    {
      title: 'IP地址',
      dataIndex: 'ip',
      key: 'ip',
    },
    {
      title: '操作时间',
      dataIndex: 'created_at',
      key: 'created_at',
    },
    {
      title: '详情',
      key: 'details',
      render: (_: any, record: AuditLog) => (
        <Text type="secondary" ellipsis>
          {JSON.stringify(record.details)}
        </Text>
      ),
    },
  ]

  return (
    <div style={{ padding: '20px 28px', background: '#e4e8ee', minHeight: '100vh' }}>
      <h2 style={{ margin: '0 0 4px', fontSize: 22, fontWeight: 700, color: '#111827' }}>
        <HistoryOutlined style={{ color: '#4338ca', marginRight: 8 }} />审计日志
      </h2>
      <Text style={{ color: '#6b7280', fontSize: 13.5 }}>查看系统操作审计记录</Text>
      <Card style={{ marginTop: 16, borderRadius: 14, border: '1px solid #e5e7eb', boxShadow: '0 2px 10px rgba(0,0,0,0.05)' }}>
        <div style={{ marginBottom: 16, display: 'flex', gap: 16, alignItems: 'center', flexWrap: 'wrap' }}>
          <RangePicker onChange={setDateRange} />
          <Input
            placeholder="搜索操作"
            style={{ width: 200 }}
            prefix={<SearchOutlined />}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
          />
          <Select
            placeholder="操作人"
            style={{ width: 150 }}
            allowClear
            showSearch
            optionFilterProp="label"
            onChange={setUserID}
            options={(usersData || []).map((u: any) => ({ label: u.name || u.username, value: u.id }))}
          />
          <Button type="primary" onClick={() => refetch()} style={{ borderRadius: 8, fontWeight: 600 }}>
            查询
          </Button>
          <Button icon={<ReloadOutlined />} onClick={() => refetch()} loading={isLoading} style={{ borderRadius: 8, fontWeight: 600 }}>
            刷新
          </Button>
        </div>

        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <div style={{ padding: '20px' }}>
            <Alert
              message="加载失败"
              description={(error as Error)?.message || '获取审计日志失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()} style={{ borderRadius: 8, fontWeight: 600 }}>
                  重试
                </Button>
              }
            />
          </div>
        ) : logsData?.data?.items?.length ? (
          <Table
            columns={columns}
            dataSource={logsData.data.items as AuditLog[]}
            rowKey="id"
            pagination={{
              current: page,
              pageSize: pageSize,
              total: logsData.data.total,
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (v: number) => <span style={{ color: '#6b7280' }}>共 {v} 条</span>,
              onChange: (newPage, newPageSize) => {
                setPage(newPage)
                setPageSize(newPageSize)
              },
            }}
          />
        ) : (
          <Empty description="暂无审计日志" imageStyle={{ height: 80 }} />
        )}
      </Card>
    </div>
  )
}

export default AuditLogs
