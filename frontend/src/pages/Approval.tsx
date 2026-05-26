import React, { useState } from 'react'
import { Typography, DatePicker, Table, Spin, Empty, Alert, Button, Select, Badge } from 'antd'
import { CalendarOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import { formatDateTime } from '../utils/format'

const { Text } = Typography
const { RangePicker } = DatePicker
const { Option } = Select

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
    switch (status.toLowerCase()) {
      case 'approved': return '已通过'
      case 'pending': return '审批中'
      case 'rejected': return '已拒绝'
      default: return status
    }
  }

  const columns = [
    {
      title: '审批标题', dataIndex: 'title', key: 'title',
      render: (v: string) => <span style={{ fontWeight: 600, color: 'var(--color-text-heading)' }}>{v}</span>,
    },
    {
      title: '申请人', dataIndex: 'applicant_name', key: 'applicant_name',
      render: (v: string) => <span style={{ color: 'var(--color-primary)', fontWeight: 500 }}>{v}</span>,
    },
    {
      title: '状态', dataIndex: 'status', key: 'status',
      render: (status: string) => (
        <Badge status={getStatusColor(status)} text={<span style={{ fontWeight: 600 }}>{getStatusText(status)}</span>} />
      )
    },
    { title: '创建时间', dataIndex: 'create_time', key: 'create_time', render: (v: string) => formatDateTime(v) },
    { title: '完成时间', dataIndex: 'finish_time', key: 'finish_time', render: (v: string) => v ? formatDateTime(v) : '-' },
  ]

  return (
    <PageContainer title="审批管理" icon={<CalendarOutlined />} subtitle="查询审批记录与审批状态">
      <PageCard>
        <div style={{ marginBottom: 18, display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <Select placeholder="选择申请人" style={{ width: 200 }} allowClear onChange={setUser}>
            <Option value="">全部申请人</Option>
            <Option value="1">张三</Option>
            <Option value="2">李四</Option>
            <Option value="3">王五</Option>
          </Select>
          <Select placeholder="选择状态" style={{ width: 200 }} allowClear onChange={setStatus}>
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
          <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <Alert
            message="加载失败"
            type="error"
            showIcon
            action={<Button size="small" onClick={() => refetch()}>重试</Button>}
          />
        ) : approvals?.items?.length ? (
          <Table columns={columns} dataSource={approvals.items} rowKey="id"
            pagination={{ total: approvals.total, pageSize: 10, showSizeChanger: false, showTotal: (v) => <span style={{ color: 'var(--color-text-secondary)' }}>共 {v} 条</span> }} />
        ) : (
          <Empty description="暂无审批数据" imageStyle={{ height: 80 }} />
        )}
      </PageCard>
    </PageContainer>
  )
}

export default Approval
