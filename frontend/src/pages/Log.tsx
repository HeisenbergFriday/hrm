import React, { useState } from 'react'
import { Card, Typography, DatePicker, Table, Spin, Empty, Alert, Button, Select } from 'antd'
import { HistoryOutlined, CalendarOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { auditAPI } from '../services/api'
import dayjs from 'dayjs'

const { Title } = Typography
const { RangePicker } = DatePicker
const { Option } = Select



const Log: React.FC = () => {
  const [user, setUser] = useState<string>('')
  const [operation, setOperation] = useState<string>('')
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null]>([null, null])

  const { data: logs, isLoading, isError, refetch } = useQuery({
    queryKey: ['logs', user, operation, dateRange],
    queryFn: () => auditAPI.getLogs({
      user_id: user || undefined,
      operation: operation || undefined,
      start_date: dateRange[0]?.format('YYYY-MM-DD') || undefined,
      end_date: dateRange[1]?.format('YYYY-MM-DD') || undefined,
      page: 1,
      page_size: 10
    })
  })

  const handleDateChange = (dates: any) => {
    setDateRange(dates)
  }

  const columns = [
    { title: '操作用户', dataIndex: 'user_name', key: 'user_name' },
    { title: '操作类型', dataIndex: 'operation', key: 'operation' },
    { title: '操作资源', dataIndex: 'resource', key: 'resource' },
    { title: 'IP地址', dataIndex: 'ip', key: 'ip' },
    { title: '操作时间', dataIndex: 'created_at', key: 'created_at' },
  ]

  return (
    <div>
      <Title level={4}>操作日志</Title>
      <Card>
        <div style={{ marginBottom: 16, display: 'flex', gap: 16, alignItems: 'center' }}>
          <Select
            placeholder="选择用户"
            style={{ width: 200 }}
            onChange={setUser}
          >
            <Option value="">全部用户</Option>
            <Option value="1">管理员</Option>
            <Option value="2">张三</Option>
            <Option value="3">李四</Option>
          </Select>
          <Select
            placeholder="选择操作类型"
            style={{ width: 200 }}
            onChange={setOperation}
          >
            <Option value="">全部操作</Option>
            <Option value="登录">登录</Option>
            <Option value="查看">查看</Option>
            <Option value="编辑">编辑</Option>
            <Option value="删除">删除</Option>
            <Option value="同步">同步</Option>
          </Select>
          <RangePicker onChange={handleDateChange} />
          <Button type="primary" onClick={() => refetch()} icon={<CalendarOutlined />}>
            查询
          </Button>
        </div>

        {isLoading ? (
          <div className="loading-container">
            <Spin size="small" />
          </div>
        ) : isError ? (
          <div className="error-container">
            <Alert message="加载失败" type="error" showIcon />
            <Button className="retry-button" onClick={() => refetch()}>重试</Button>
          </div>
        ) : logs?.data?.items?.length ? (
          <Table
            columns={columns}
            dataSource={logs.data.items}
            rowKey="id"
            pagination={{ total: logs.data.total, pageSize: 10 }}
          />
        ) : (
          <div className="empty-container">
            <Empty description="暂无操作日志" />
          </div>
        )}
      </Card>
    </div>
  )
}

export default Log
