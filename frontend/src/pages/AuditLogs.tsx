import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, DatePicker, Input, Select } from 'antd'
import { HistoryOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { auditAPI } from '../services/api'
import dayjs from 'dayjs'

const { Title, Text } = Typography
const { RangePicker } = DatePicker
const { Option } = Select

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
    <div>
      <Title level={4}>审计日志</Title>
      <Card>
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
            onChange={setUserID}
          >
            <Option value="user123">张三</Option>
            <Option value="user456">李四</Option>
            <Option value="user789">王五</Option>
          </Select>
          <Button type="primary" onClick={() => refetch()}>
            查询
          </Button>
          <Button icon={<ReloadOutlined />} onClick={() => refetch()} loading={isLoading}>
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
                <Button size="small" onClick={() => refetch()}>
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
              showTotal: (total: number) => `共 ${total} 条记录`,
              onChange: (newPage, newPageSize) => {
                setPage(newPage)
                setPageSize(newPageSize)
              },
            }}
          />
        ) : (
          <Empty description="暂无审计日志" />
        )}
      </Card>
    </div>
  )
}

export default AuditLogs