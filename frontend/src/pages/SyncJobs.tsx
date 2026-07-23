import React from 'react'
import { Typography, Table, Spin, Empty, Alert, Button, message, Modal, Tooltip } from 'antd'
import { SyncOutlined, ReloadOutlined, PlayCircleOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { attendanceAPI, jobAPI, orgAPI, syncAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import { formatDateTime } from '../utils/format'
import { hasPermission } from '../utils/permission'

const { Text } = Typography

interface Job {
  id: string
  name: string
  description: string
  type: string
  status: string
  last_run_time: string
  next_run_time: string
}

/** 将列表中的任务 id/type 映射到真实同步调用（避免 RunJob 仅改状态的假成功） */
async function runRealJob(job: Job): Promise<void> {
  const key = `${job.id}|${job.type}`.toLowerCase()
  if (key.includes('user') || job.id === '1' || job.type === 'sync_users') {
    await syncAPI.syncUsers()
    return
  }
  if (key.includes('department') || job.id === '2' || job.type === 'sync_departments') {
    await syncAPI.syncDepartments()
    return
  }
  if (key.includes('attendance') || job.id === '3' || job.type === 'sync_attendance') {
    await attendanceAPI.sync()
    return
  }
  // 兜底：组织花名册全量同步（部门+用户）
  await orgAPI.syncOrg()
}

const SyncJobs: React.FC = () => {
  const canRun = hasPermission('attendance_manage')

  const { data: jobsData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['jobs'],
    queryFn: () => jobAPI.getJobs(),
  })

  const runJobMutation = useMutation({
    mutationFn: (job: Job) => runRealJob(job),
    onSuccess: () => {
      message.success('同步任务已执行')
      refetch()
    },
    onError: (err: any) => {
      message.error(err?.response?.data?.message || err?.response?.data?.error || '任务运行失败')
    },
  })

  const handleRun = (record: Job) => {
    if (!canRun) return
    Modal.confirm({
      title: '立即运行同步任务',
      content: `将立即执行「${record.name}」对应的真实同步（钉钉拉取/写入）。确认继续？`,
      okText: '确认运行',
      cancelText: '取消',
      onOk: () => runJobMutation.mutateAsync(record),
    })
  }

  const getStatusTag = (status: string) => {
    switch ((status || '').toLowerCase()) {
      case 'idle':
        return <StatusTag color="blue">空闲</StatusTag>
      case 'running':
        return <StatusTag color="green">运行中</StatusTag>
      case 'failed':
        return <StatusTag color="red">失败</StatusTag>
      case 'completed':
      case 'success':
        return <StatusTag color="green">成功</StatusTag>
      default:
        return <StatusTag>{status}</StatusTag>
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
      render: (v: string) => formatDateTime(v),
    },
    {
      title: '下次运行时间',
      dataIndex: 'next_run_time',
      key: 'next_run_time',
      render: (v: string) => formatDateTime(v),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Job) => (
        <Tooltip title={canRun ? undefined : '你缺少 attendance_manage 权限，需要联系管理员添加'}>
          <Button
            type="primary"
            icon={<PlayCircleOutlined />}
            onClick={() => handleRun(record)}
            loading={runJobMutation.isPending && (runJobMutation.variables as Job | undefined)?.id === record.id}
            disabled={!canRun || record.status === 'running'}
          >
            立即运行
          </Button>
        </Tooltip>
      ),
    },
  ]

  return (
    <PageContainer title="同步任务" icon={<SyncOutlined />} subtitle="管理系统数据同步任务（立即运行将触发真实同步）">
      <PageCard
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
          <div style={{ padding: 'var(--space-5)' }}>
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
              showTotal: (v: number) => <span style={{ color: 'var(--color-text-secondary)' }}>共 {v} 条</span>,
            }}
          />
        ) : (
          <Empty description="暂无任务" imageStyle={{ height: 80 }} />
        )}
      </PageCard>
    </PageContainer>
  )
}

export default SyncJobs
