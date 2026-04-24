import React, { useState } from 'react'
import { Card, Typography, DatePicker, Table, Spin, Empty, Alert, Button, Select, Badge } from 'antd'
import { CalendarOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'

const { Title } = Typography
const { RangePicker } = DatePicker
const { Option } = Select

// 模拟API调用
const fetchApprovals = async (params: any) => {
  await new Promise(resolve => setTimeout(resolve, 1000))
  return {
    items: [
      { id: '1', title: '请假申请', applicant_name: '张三', status: 'approved', create_time: '2024-01-01 10:00:00', finish_time: '2024-01-01 11:00:00' },
      { id: '2', title: '报销申请', applicant_name: '李四', status: 'pending', create_time: '2024-01-01 12:00:00' },
      { id: '3', title: '加班申请', applicant_name: '王五', status: 'rejected', create_time: '2024-01-01 13:00:00', finish_time: '2024-01-01 14:00:00' },
    ],
    total: 3,
  }
}

const Approval: React.FC = () => {
  const [user, setUser] = useState<string>('')
  const [status, setStatus] = useState<string>('')
  const [dateRange, setDateRange] = useState<[any, any]>([null, null])

  const { data: approvals, isLoading, isError, refetch } = useQuery({
    queryKey: ['approvals', user, status, dateRange],
    queryFn: () => fetchApprovals({ user, status, dateRange })
  })

  const handleDateChange = (dates: any) => {
    setDateRange(dates)
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'approved': return 'success'
      case 'pending': return 'processing'
      case 'rejected': return 'error'
      default: return 'default'
    }
  }

  const getStatusText = (status: string) => {
    switch (status) {
      case 'approved': return '已通过'
      case 'pending': return '审批中'
      case 'rejected': return '已拒绝'
      default: return status
    }
  }

  const columns = [
    { title: '审批标题', dataIndex: 'title', key: 'title' },
    { title: '申请人', dataIndex: 'applicant_name', key: 'applicant_name' },
    { 
      title: '状态', 
      dataIndex: 'status', 
      key: 'status',
      render: (status: string) => (
        <Badge status={getStatusColor(status)} text={getStatusText(status)} />
      )
    },
    { title: '创建时间', dataIndex: 'create_time', key: 'create_time' },
    { title: '完成时间', dataIndex: 'finish_time', key: 'finish_time' },
  ]

  return (
    <div>
      <Title level={4}>审批管理</Title>
      <Card>
        <div style={{ marginBottom: 16, display: 'flex', gap: 16, alignItems: 'center' }}>
          <Select
            placeholder="选择申请人"
            style={{ width: 200 }}
            onChange={setUser}
          >
            <Option value="">全部申请人</Option>
            <Option value="1">张三</Option>
            <Option value="2">李四</Option>
            <Option value="3">王五</Option>
          </Select>
          <Select
            placeholder="选择状态"
            style={{ width: 200 }}
            onChange={setStatus}
          >
            <Option value="">全部状态</Option>
            <Option value="approved">已通过</Option>
            <Option value="pending">审批中</Option>
            <Option value="rejected">已拒绝</Option>
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
        ) : approvals?.items?.length ? (
          <Table
            columns={columns}
            dataSource={approvals.items}
            rowKey="id"
            pagination={{ total: approvals.total, pageSize: 10 }}
          />
        ) : (
          <div className="empty-container">
            <Empty description="暂无审批数据" />
          </div>
        )}
      </Card>
    </div>
  )
}

export default Approval
