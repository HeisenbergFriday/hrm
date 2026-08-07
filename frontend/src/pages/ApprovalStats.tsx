import React, { useEffect, useRef, useState } from 'react'
import { Typography, DatePicker, Alert, Button, Row, Col, Statistic, Table, Select, Tooltip } from 'antd'
import { BarChartOutlined, SyncOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { approvalAPI, getPendingApprovalSyncRequestID, type ApprovalStatsAPIResponse, type ApprovalSyncAPIResponse } from '../services/api'
import { hasPermission } from '../utils/permission'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import datePickerZhCN from 'antd/es/date-picker/locale/zh_CN'
import {
  approvalSyncErrorNotice,
  approvalSyncResultNotice,
  approvalSyncRunningNotice,
  missingApprovalSyncPermissionTip,
  type ApprovalSyncNotice,
} from '../utils/approvalSync'

dayjs.locale('zh-cn')

const { Title, Text } = Typography
const { RangePicker } = DatePicker
const { Option } = Select

interface TemplateStat {
  template_id: string
  template_name: string
  total: number
  completed: number
  refused: number
  running: number
  terminated: number
  canceled: number
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
  const [syncNotice, setSyncNotice] = useState<ApprovalSyncNotice | null>(null)
  const syncInFlightRef = useRef(false)

  const { data: templatesData } = useQuery({
    queryKey: ['approval-templates'],
    queryFn: () => approvalAPI.getTemplates(),
  })

  const statsParams = {
    template_id: templateID || undefined,
    start_date: dateRange[0]?.format('YYYY-MM-DD'),
    end_date: dateRange[1]?.format('YYYY-MM-DD'),
  }
  const { data: statsSource, refetch: refetchStats } = useQuery({
    queryKey: ['approval-stats-source', statsParams],
    queryFn: () => approvalAPI.getStats(statsParams),
  })

  const statsPayload = (statsSource as ApprovalStatsAPIResponse | undefined)?.data
  const summary = statsPayload?.summary || {
    total: 0, completed: 0, refused: 0, running: 0, terminated: 0, canceled: 0, approval_rate: '0.00%',
  }
  const percentage = (count: number) => `${summary.total ? ((count / summary.total) * 100).toFixed(2) : '0.00'}%`
  const statsData = {
    summary,
    template_stats: (statsPayload?.template_stats || []) as TemplateStat[],
    status_stats: [
      { status: '已通过', count: summary.completed, percentage: percentage(summary.completed) },
      { status: '已拒绝', count: summary.refused, percentage: percentage(summary.refused) },
      { status: '处理中', count: summary.running, percentage: percentage(summary.running) },
      { status: '已终止', count: summary.terminated, percentage: percentage(summary.terminated) },
      { status: '已取消', count: summary.canceled, percentage: percentage(summary.canceled) },
    ] as StatusStat[],
  }

  const syncMutation = useMutation<ApprovalSyncAPIResponse, Error, boolean>({
    mutationFn: (resume) => resume
      ? approvalAPI.resumeSync()
      : approvalAPI.sync({
        process_code: templateID || undefined,
        start_date: dateRange[0]?.format('YYYY-MM-DD'),
        end_date: dateRange[1]?.format('YYYY-MM-DD'),
      }),
    onSuccess: (response) => {
      setSyncNotice(approvalSyncResultNotice(response.data))
      if (response.data.status === 'success' || response.data.status === 'partial') {
        refetchStats()
      }
    },
    onError: (error) => setSyncNotice(approvalSyncErrorNotice(error)),
    onSettled: () => {
      syncInFlightRef.current = false
    },
  })

  const resumeSync = syncMutation.mutate
  useEffect(() => {
    if (!hasPermission('approval:sync') || !getPendingApprovalSyncRequestID() || syncInFlightRef.current) return
    syncInFlightRef.current = true
    setSyncNotice(approvalSyncRunningNotice())
    resumeSync(true)
  }, [resumeSync])

  const handleSync = () => {
    if (syncInFlightRef.current) {
      return
    }
    syncInFlightRef.current = true
    setSyncNotice(approvalSyncRunningNotice(templateID))
    syncMutation.mutate(false)
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
      dataIndex: 'refused',
      key: 'refused',
      render: (count: number) => <StatusTag color="red">{count}</StatusTag>,
    },
    {
      title: '处理中',
      dataIndex: 'running',
      key: 'running',
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
          <RangePicker
            onChange={setDateRange}
            placeholder={['开始日期', '结束日期']}
            format="YYYY-MM-DD"
            locale={datePickerZhCN}
          />
          <Button
            type="primary"
            icon={<BarChartOutlined />}
          >
            统计
          </Button>
          <Tooltip title={hasPermission('approval:sync') ? undefined : missingApprovalSyncPermissionTip}>
            <span>
              <Button
                icon={<SyncOutlined />}
                onClick={handleSync}
                loading={syncMutation.isPending}
                disabled={!hasPermission('approval:sync') || syncMutation.isPending}
              >
                {syncMutation.isPending ? '同步中' : (templateID ? '同步当前模板' : '同步全部')}
              </Button>
            </span>
          </Tooltip>
        </div>

        {syncNotice && (
          <Alert
            style={{ marginBottom: 'var(--space-4)' }}
            type={syncNotice.type}
            message={syncNotice.message}
            description={syncNotice.description}
            showIcon
          />
        )}

        <Row className="mobile-stat-grid" gutter={16} style={{ marginBottom: 'var(--space-6)' }}>
          <Col span={6}>
            <Statistic
              title="总审批数"
              value={statsData.summary.total}
              prefix={<BarChartOutlined />}
            />
          </Col>
          <Col span={6}>
            <Statistic
              title="已完成"
              value={statsData.summary.completed}
              valueStyle={{ color: 'var(--color-success)' }}
            />
          </Col>
          <Col span={6}>
            <Statistic
              title="已拒绝"
              value={statsData.summary.refused}
              valueStyle={{ color: 'var(--color-error)' }}
            />
          </Col>
          <Col span={6}>
            <Statistic
              title="通过率"
              value={statsData.summary.approval_rate}
              valueStyle={{ color: parseFloat(statsData.summary.approval_rate) >= 80 ? 'var(--color-success)' : 'var(--color-error)' }}
            />
          </Col>
        </Row>

        <Title level={5}>状态分布</Title>
        <div style={{ marginBottom: 'var(--space-6)' }}>
          <Row className="mobile-stat-grid" gutter={16}>
            {statsData.status_stats.map((stat, index) => (
              <Col key={index} flex="1 1 160px">
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
          dataSource={statsData.template_stats}
          rowKey="template_id"
          pagination={false}
        />
      </PageCard>
    </PageContainer>
  )
}

export default ApprovalStats
