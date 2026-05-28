import React, { useState } from 'react'
import { Typography, DatePicker, Spin, Empty, Alert, Button, Row, Col, Statistic, Table, Tag, Select, message } from 'antd'
import { BarChartOutlined, SyncOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { approvalAPI } from '../services/api'
import { hasPermission } from '../utils/permission'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import dayjs from 'dayjs'

const { Title, Text } = Typography
const { RangePicker } = DatePicker
const { Option } = Select

interface TemplateStat {
  template_id: string
  template_name: string
  total: number
  completed: number
  rejected: number
  in_progress: number
  approval_rate: string
}

interface StatusStat {
  status: string
  count: number
  percentage: string
}

const ApprovalStats: React.FC = () => {
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null]>([null, null])
  const [templateID, setTemplateID] = useState<string>('')

  // 模拟审批统计数据
  const mockStatsData = {
    summary: {
      total: 100,
      completed: 85,
      rejected: 10,
      in_progress: 5,
      approval_rate: '85.00%',
    },
    template_stats: [
      {
        template_id: 'template123',
        template_name: '请假审批',
        total: 45,
        completed: 40,
        rejected: 3,
        in_progress: 2,
        approval_rate: '88.89%',
      },
      {
        template_id: 'template456',
        template_name: '报销审批',
        total: 30,
        completed: 25,
        rejected: 4,
        in_progress: 1,
        approval_rate: '83.33%',
      },
      {
        template_id: 'template789',
        template_name: '加班审批',
        total: 25,
        completed: 20,
        rejected: 3,
        in_progress: 2,
        approval_rate: '80.00%',
      },
    ],
    status_stats: [
      { status: '已完成', count: 85, percentage: '85.00%' },
      { status: '已拒绝', count: 10, percentage: '10.00%' },
      { status: '处理中', count: 5, percentage: '5.00%' },
    ],
  }

  const { data: templatesData } = useQuery({
    queryKey: ['approval-templates'],
    queryFn: () => approvalAPI.getTemplates(),
  })

  const syncMutation = useMutation({
    mutationFn: () => approvalAPI.sync({
      process_code: templateID,
      start_date: dateRange[0]?.format('YYYY-MM-DD'),
      end_date: dateRange[1]?.format('YYYY-MM-DD'),
    }),
  })

  const handleSync = () => {
    if (!templateID) {
      message.warning('请先选择审批模板/流程代码')
      return
    }
    syncMutation.mutate()
  }

  const columns = [
    {
      title: '审批模板',
      dataIndex: 'template_name',
      key: 'template_name',
    },
    {
      title: '总审批数',
      dataIndex: 'total',
      key: 'total',
    },
    {
      title: '已通过',
      dataIndex: 'completed',
      key: 'completed',
      render: (count: number) => <StatusTag color="green">{count}</StatusTag>,
    },
    {
      title: '已拒绝',
      dataIndex: 'rejected',
      key: 'rejected',
      render: (count: number) => <StatusTag color="red">{count}</StatusTag>,
    },
    {
      title: '处理中',
      dataIndex: 'in_progress',
      key: 'in_progress',
      render: (count: number) => <StatusTag color="blue">{count}</StatusTag>,
    },
    {
      title: '通过率',
      dataIndex: 'approval_rate',
      key: 'approval_rate',
      render: (rate: string) => (
        <span style={{ color: parseFloat(rate) >= 80 ? 'var(--color-success)' : 'var(--color-error)' }}>
          {rate}
        </span>
      ),
    },
  ]

  return (
    <PageContainer
      title="审批统计"
      icon={<BarChartOutlined />}
    >
      <PageCard>
        <div style={{ marginBottom: 'var(--space-4)', display: 'flex', gap: 'var(--space-4)', alignItems: 'center', flexWrap: 'wrap' }}>
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
          <Button
            type="primary"
            icon={<BarChartOutlined />}
          >
            统计
          </Button>
          <Button
            icon={<SyncOutlined />}
            onClick={handleSync}
            loading={syncMutation.isPending}
            disabled={!hasPermission('approval:sync')}
          >
            同步数据
          </Button>
        </div>

        <Row gutter={16} style={{ marginBottom: 'var(--space-6)' }}>
          <Col span={6}>
            <Statistic
              title="总审批数"
              value={mockStatsData.summary.total}
              prefix={<BarChartOutlined />}
            />
          </Col>
          <Col span={6}>
            <Statistic
              title="已完成"
              value={mockStatsData.summary.completed}
              valueStyle={{ color: 'var(--color-success)' }}
            />
          </Col>
          <Col span={6}>
            <Statistic
              title="已拒绝"
              value={mockStatsData.summary.rejected}
              valueStyle={{ color: 'var(--color-error)' }}
            />
          </Col>
          <Col span={6}>
            <Statistic
              title="通过率"
              value={mockStatsData.summary.approval_rate}
              valueStyle={{ color: parseFloat(mockStatsData.summary.approval_rate) >= 80 ? 'var(--color-success)' : 'var(--color-error)' }}
            />
          </Col>
        </Row>

        <Title level={5}>状态分布</Title>
        <div style={{ marginBottom: 'var(--space-6)' }}>
          <Row gutter={16}>
            {mockStatsData.status_stats.map((stat, index) => (
              <Col key={index} span={8}>
                <PageCard>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Text strong>{stat.status}</Text>
                    <Text>{stat.count}</Text>
                  </div>
                  <div style={{ marginTop: 'var(--space-2)' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 'var(--space-1)' }}>
                      <Text type="secondary">占比</Text>
                      <Text>{stat.percentage}</Text>
                    </div>
                    <div style={{ height: 8, backgroundColor: 'var(--color-border-light)', borderRadius: 'var(--radius-xs)' }}>
                      <div
                        style={{
                          height: '100%',
                          backgroundColor: stat.status === '已完成' ? 'var(--color-success)' : stat.status === '已拒绝' ? 'var(--color-error)' : 'var(--color-primary)',
                          borderRadius: 'var(--radius-xs)',
                          width: stat.percentage,
                        }}
                      />
                    </div>
                  </div>
                </PageCard>
              </Col>
            ))}
          </Row>
        </div>

        <Title level={5}>模板统计</Title>
        <Table
          columns={columns}
          dataSource={mockStatsData.template_stats}
          rowKey="template_id"
          pagination={false}
        />
      </PageCard>
    </PageContainer>
  )
}

export default ApprovalStats
