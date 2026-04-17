import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, Tag, Select, DatePicker, Space, Input } from 'antd'
import { FileTextOutlined, SyncOutlined, SearchOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { approvalAPI } from '../services/api'
import dayjs from 'dayjs'

const { Title, Text } = Typography
const { Option } = Select
const { RangePicker } = DatePicker

interface ApprovalInstance {
  id: string
  process_id: string
  template_id: string
  template_name: string
  title: string
  applicant_id: string
  applicant_name: string
  status: string
  create_time: string
  finish_time: string | null
  extension: any
}

const ApprovalInstance: React.FC = () => {
  const navigate = useNavigate()
  const [status, setStatus] = useState<string>('')
  const [templateID, setTemplateID] = useState<string>('')
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null]>([null, null])
  const [searchText, setSearchText] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)

  const queryParams = {
    page,
    page_size: pageSize,
    status: status || undefined,
    template_id: templateID || undefined,
    start_date: dateRange[0]?.format('YYYY-MM-DD') || undefined,
    end_date: dateRange[1]?.format('YYYY-MM-DD') || undefined,
  }

  const { data: instancesData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['approval-instances', queryParams],
    queryFn: () => approvalAPI.getInstances(queryParams),
  })

  const { data: templatesData } = useQuery({
    queryKey: ['approval-templates'],
    queryFn: () => approvalAPI.getTemplates(),
  })

  const syncMutation = useMutation({
    mutationFn: () => approvalAPI.sync({
      start_date: dateRange[0]?.format('YYYY-MM-DD'),
      end_date: dateRange[1]?.format('YYYY-MM-DD'),
    }),
    onSuccess: () => {
      refetch()
    },
  })

  const handleViewDetail = (id: string) => {
    navigate(`/approval-detail/${id}`)
  }

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'completed':
        return <Tag color="green">已完成</Tag>
      case 'in_progress':
        return <Tag color="blue">处理中</Tag>
      case 'rejected':
        return <Tag color="red">已拒绝</Tag>
      case 'pending':
        return <Tag color="orange">待处理</Tag>
      default:
        return <Tag>{status}</Tag>
    }
  }

  const columns = [
    {
      title: '审批标题',
      dataIndex: 'title',
      key: 'title',
      render: (text: string, record: ApprovalInstance) => (
        <Text strong onClick={() => handleViewDetail(record.id)} style={{ cursor: 'pointer', color: '#1890ff' }}>
          {text}
        </Text>
      ),
    },
    {
      title: '审批模板',
      dataIndex: 'template_name',
      key: 'template_name',
    },
    {
      title: '发起人',
      dataIndex: 'applicant_name',
      key: 'applicant_name',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => getStatusTag(status),
    },
    {
      title: '发起时间',
      dataIndex: 'create_time',
      key: 'create_time',
    },
    {
      title: '结束时间',
      dataIndex: 'finish_time',
      key: 'finish_time',
      render: (finishTime: string | null) => finishTime || '-',
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: ApprovalInstance) => (
        <Button
          type="link"
          onClick={() => handleViewDetail(record.id)}
        >
          查看详情
        </Button>
      ),
    },
  ]

  return (
    <div>
      <Title level={4}>审批实例</Title>
      <Card>
        <div style={{ marginBottom: 16, display: 'flex', gap: 16, alignItems: 'center', flexWrap: 'wrap' }}>
          <Select
            placeholder="状态"
            style={{ width: 120 }}
            allowClear
            onChange={setStatus}
          >
            <Option value="completed">已完成</Option>
            <Option value="in_progress">处理中</Option>
            <Option value="rejected">已拒绝</Option>
            <Option value="pending">待处理</Option>
          </Select>
          <Select
            placeholder="审批模板"
            style={{ width: 150 }}
            allowClear
            onChange={setTemplateID}
          >
            {templatesData?.data?.items?.map((template: any) => (
              <Option key={template.template_id} value={template.template_id}>
                {template.name}
              </Option>
            ))}
          </Select>
          <RangePicker onChange={setDateRange} />
          <Input
            placeholder="搜索标题"
            style={{ width: 200 }}
            prefix={<SearchOutlined />}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
          />
          <Space>
            <Button type="primary" onClick={() => refetch()}>
              查询
            </Button>
            <Button
              icon={<SyncOutlined />}
              onClick={() => syncMutation.mutate()}
              loading={syncMutation.isPending}
            >
              同步数据
            </Button>
          </Space>
        </div>

        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <div style={{ padding: '20px' }}>
            <Alert
              message="加载失败"
              description={(error as Error)?.message || '获取审批实例失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()}>
                  重试
                </Button>
              }
            />
          </div>
        ) : instancesData?.data?.items?.length ? (
          <Table
            columns={columns}
            dataSource={instancesData.data.items as ApprovalInstance[]}
            rowKey="id"
            pagination={{
              current: page,
              pageSize: pageSize,
              total: instancesData.data.total,
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
          <Empty description="暂无审批实例" />
        )}
      </Card>
    </div>
  )
}

export default ApprovalInstance