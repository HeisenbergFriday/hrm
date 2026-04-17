import React from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, Tag, message } from 'antd'
import { SyncOutlined, ReloadOutlined, PlayCircleOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { jobAPI } from '../services/api'

const { Title, Text } = Typography

interface Job {
  id: string
  name: string
  description: string
  type: string
  status: string
  last_run_time: string
  next_run_time: string
}

const SyncJobs: React.FC = () => {
  const { data: jobsData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['jobs'],
    queryFn: () => jobAPI.getJobs(),
  })

  const runJobMutation = useMutation({
    mutationFn: (jobId: string) => jobAPI.runJob(jobId),
    onSuccess: () => {
      message.success('任务开始运行')
      refetch()
    },
    onError: () => {
      message.error('任务运行失败')
    },
  })

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'idle':
        return <Tag color="blue">空闲</Tag>
      case 'running':
        return <Tag color="green">运行中</Tag>
      case 'failed':
        return <Tag color="red">失败</Tag>
      case 'completed':
        return <Tag color="green">已完成</Tag>
      default:
        return <Tag>{status}</Tag>
    }
  }

  const columns = [
    {
      title: '任务名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string) => <Text strong>{text}</Text>,
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type: string) => {
        const typeMap: Record<string, string> = {
          sync_users: '同步用户',
          sync_departments: '同步部门',
          sync_attendance: '同步考勤',
        }
        return typeMap[type] || type
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => getStatusTag(status),
    },
    {
      title: '上次运行时间',
      dataIndex: 'last_run_time',
      key: 'last_run_time',
    },
    {
      title: '下次运行时间',
      dataIndex: 'next_run_time',
      key: 'next_run_time',
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Job) => (
        <Button
          type="primary"
          icon={<PlayCircleOutlined />}
          onClick={() => runJobMutation.mutate(record.id)}
          loading={runJobMutation.isPending && runJobMutation.variables === record.id}
          disabled={record.status === 'running'}
        >
          立即运行
        </Button>
      ),
    },
  ]

  return (
    <div>
      <Title level={4}>同步任务</Title>
      <Card
        extra={
          <Button icon={<ReloadOutlined />} onClick={() => refetch()} loading={isLoading}>
            刷新
          </Button>
        }
      >
        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <div style={{ padding: '20px' }}>
            <Alert
              message="加载失败"
              description={(error as Error)?.message || '获取任务列表失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()}>
                  重试
                </Button>
              }
            />
          </div>
        ) : jobsData?.data?.items?.length ? (
          <Table
            columns={columns}
            dataSource={jobsData.data.items as Job[]}
            rowKey="id"
            pagination={{
              showTotal: (total: number) => `共 ${total} 个任务`,
            }}
          />
        ) : (
          <Empty description="暂无任务" />
        )}
      </Card>
    </div>
  )
}

export default SyncJobs