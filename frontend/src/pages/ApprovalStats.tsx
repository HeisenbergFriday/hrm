import React, { useState } from 'react'
import { Typography, DatePicker, Alert, Button, Row, Col, Statistic, Table, Select, Progress, Spin, Empty } from 'antd'
import { BarChartOutlined, ReloadOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { approvalAPI, type ApprovalStatsAPIResponse } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import ApprovalSyncStatus from '../components/ApprovalSyncStatus'
import { useApprovalSync } from '../hooks/useApprovalSync'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import datePickerZhCN from 'antd/es/date-picker/locale/zh_CN'
import './approval.css'

dayjs.locale('zh-cn')

const { Text } = Typography
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
  color: string
}

const ApprovalStats: React.FC = () => {
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null]>([null, null])
  const [templateID, setTemplateID] = useState<string>('')

  const { data: templatesData } = useQuery({
    queryKey: ['approval-templates'],
    queryFn: () => approvalAPI.getTemplates(),
  })

  const startDate = dateRange[0]?.format('YYYY-MM-DD')
  const endDate = dateRange[1]?.format('YYYY-MM-DD')
  const statsParams = {
    template_id: templateID || undefined,
    start_date: startDate,
    end_date: endDate,
  }
  const {
    data: statsSource,
    isLoading,
    isError,
    error,
    refetch: refetchStats,
  } = useQuery({
    queryKey: ['approval-stats-source', statsParams],
    queryFn: () => approvalAPI.getStats(statsParams),
  })

  const { canSync, isSyncing, notice: syncNotice, startManualSync } = useApprovalSync({
    processCode: templateID || undefined,
    startDate,
    endDate,
    onCompleted: () => { void refetchStats() },
  })

  const statsPayload = (statsSource as ApprovalStatsAPIResponse | undefined)?.data
  const summary = statsPayload?.summary || {
    total: 0, completed: 0, refused: 0, running: 0, terminated: 0, canceled: 0, approval_rate: '0.00%',
  }
  const percentage = (count: number) => `${summary.total ? ((count / summary.total) * 100).toFixed(2) : '0.00'}%`
  const statusStats: StatusStat[] = [
    { status: '已通过', count: summary.completed, percentage: percentage(summary.completed), color: 'var(--color-success)' },
    { status: '已拒绝', count: summary.refused, percentage: percentage(summary.refused), color: 'var(--color-error)' },
    { status: '处理中', count: summary.running, percentage: percentage(summary.running), color: 'var(--color-primary)' },
    { status: '已终止', count: summary.terminated, percentage: percentage(summary.terminated), color: 'var(--color-warning)' },
    { status: '已取消', count: summary.canceled, percentage: percentage(summary.canceled), color: 'var(--color-text-tertiary)' },
  ]
  const templateStats = (statsPayload?.template_stats || []) as TemplateStat[]

  const columns = [
    {
      title: '审批模板',
      dataIndex: 'template_name',
      key: 'template_name',
      render: (name: string, record: TemplateStat) => name || record.template_id || '—',
    },
    { title: '总审批数', dataIndex: 'total', key: 'total' },
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
        <Text style={{ color: parseFloat(rate) >= 80 ? 'var(--color-success)' : 'var(--color-error)' }}>
          {rate}
        </Text>
      ),
    },
  ]

  return (
    <PageContainer
      title="审批统计"
      subtitle="按模板和业务日期查看审批量、处理状态与通过率"
      icon={<BarChartOutlined />}
      extra={(
        <Button icon={<ReloadOutlined />} onClick={() => refetchStats()} loading={isLoading}>
          刷新
        </Button>
      )}
    >
      <div className="approval-page-stack">
        <ApprovalSyncStatus
          notice={syncNotice}
          isSyncing={isSyncing}
          canSync={canSync}
          processCode={templateID || undefined}
          onManualSync={startManualSync}
        />

        <PageCard className="approval-filter-card">
          <div className="approval-stats-filter">
            <Select
              placeholder="审批模板"
              allowClear
              value={templateID || undefined}
              onChange={(value) => setTemplateID(value || '')}
            >
              {templatesData?.data?.items?.map((template: { template_id: string; name: string }) => (
                <Option key={template.template_id} value={template.template_id}>
                  {template.name}
                </Option>
              ))}
            </Select>
            <RangePicker
              value={dateRange}
              onChange={(range) => setDateRange(range || [null, null])}
              placeholder={['业务开始日期', '业务结束日期']}
              format="YYYY-MM-DD"
              locale={datePickerZhCN}
            />
            <Button type="primary" icon={<BarChartOutlined />} onClick={() => refetchStats()}>
              刷新统计
            </Button>
          </div>
        </PageCard>

        {isLoading ? (
          <PageCard><div className="approval-loading"><Spin size="large" /></div></PageCard>
        ) : isError ? (
          <PageCard>
            <Alert
              type="error"
              showIcon
              message="统计数据加载失败"
              description={(error as Error)?.message || '请稍后重试'}
              action={<Button size="small" aria-label="重试" onClick={() => refetchStats()}>重试</Button>}
            />
          </PageCard>
        ) : (
          <>
            <Row gutter={[16, 16]}>
              <Col xs={12} md={6}>
                <PageCard className="approval-metric-card">
                  <Statistic title="总审批数" value={summary.total} prefix={<BarChartOutlined />} />
                </PageCard>
              </Col>
              <Col xs={12} md={6}>
                <PageCard className="approval-metric-card">
                  <Statistic title="已通过" value={summary.completed} valueStyle={{ color: 'var(--color-success)' }} />
                </PageCard>
              </Col>
              <Col xs={12} md={6}>
                <PageCard className="approval-metric-card">
                  <Statistic title="处理中" value={summary.running} valueStyle={{ color: 'var(--color-primary)' }} />
                </PageCard>
              </Col>
              <Col xs={12} md={6}>
                <PageCard className="approval-metric-card">
                  <Statistic
                    title="通过率"
                    value={summary.approval_rate}
                    valueStyle={{ color: parseFloat(summary.approval_rate) >= 80 ? 'var(--color-success)' : 'var(--color-error)' }}
                  />
                </PageCard>
              </Col>
            </Row>

            <PageCard title="状态分布">
              <div className="approval-status-distribution">
                {statusStats.map((stat) => (
                  <div className="approval-status-item" key={stat.status}>
                    <div className="approval-status-item-head">
                      <Text strong>{stat.status}</Text>
                      <Text>{stat.count} 条</Text>
                    </div>
                    <Progress
                      percent={Number.parseFloat(stat.percentage)}
                      strokeColor={stat.color}
                      size="small"
                    />
                  </div>
                ))}
              </div>
            </PageCard>

            <PageCard title="模板统计" className="approval-table-card">
              {templateStats.length > 0 ? (
                <Table
                  columns={columns}
                  dataSource={templateStats}
                  rowKey="template_id"
                  pagination={false}
                  size="middle"
                  scroll={{ x: 760 }}
                />
              ) : (
                <Empty description="当前筛选范围暂无审批统计" />
              )}
            </PageCard>
          </>
        )}
      </div>
    </PageContainer>
  )
}

export default ApprovalStats
