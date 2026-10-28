import React, { useState } from 'react'
import { Typography, DatePicker, Alert, Button, Row, Col, Statistic, Table, Select, Progress, Spin, Empty, Segmented, Space, Tag } from 'antd'
import { BarChartOutlined, ReloadOutlined, CalendarOutlined } from '@ant-design/icons'
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
type QuickRange = 'all' | '7days' | '30days' | 'thisMonth' | 'lastMonth' | 'custom'

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
  const [quickRange, setQuickRange] = useState<QuickRange>('all')

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
  const selectedTemplateName = templateID
    ? templatesData?.data?.items?.find((template: { template_id: string }) => template.template_id === templateID)?.name || templateID
    : '全部模板'
  const rangeLabel = dateRange[0] && dateRange[1]
    ? `${dateRange[0].format('YYYY-MM-DD')} 至 ${dateRange[1].format('YYYY-MM-DD')}`
    : '全部业务日期'

  const changeQuickRange = (value: QuickRange) => {
    const now = dayjs()
    let nextRange: [dayjs.Dayjs | null, dayjs.Dayjs | null] = [null, null]
    if (value === '7days') nextRange = [now.subtract(6, 'day').startOf('day'), now.endOf('day')]
    if (value === '30days') nextRange = [now.subtract(29, 'day').startOf('day'), now.endOf('day')]
    if (value === 'thisMonth') nextRange = [now.startOf('month'), now.endOf('day')]
    if (value === 'lastMonth') {
      const lastMonth = now.subtract(1, 'month')
      nextRange = [lastMonth.startOf('month'), lastMonth.endOf('month')]
    }
    setQuickRange(value)
    setDateRange(nextRange)
  }

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
            <Segmented
              aria-label="快捷日期范围"
              value={quickRange}
              options={[
                { value: 'all', label: '全部' },
                { value: '7days', label: '近 7 天' },
                { value: '30days', label: '近 30 天' },
                { value: 'thisMonth', label: '本月' },
                { value: 'lastMonth', label: '上月' },
              ]}
              onChange={(value) => changeQuickRange(value as QuickRange)}
            />
            <RangePicker
              value={dateRange}
              onChange={(range) => {
                setDateRange(range || [null, null])
                setQuickRange(range?.[0] && range?.[1] ? 'custom' : 'all')
              }}
              placeholder={['业务开始日期', '业务结束日期']}
              format="YYYY-MM-DD"
              locale={datePickerZhCN}
            />
            <Button type="primary" icon={<BarChartOutlined />} onClick={() => refetchStats()}>
              刷新统计
            </Button>
          </div>
          <div className="approval-stats-context" role="status" aria-live="polite">
            <Space size={[8, 4]} wrap>
              <Text strong><CalendarOutlined /> 统计口径</Text>
              <Tag color="blue">模板：{selectedTemplateName}</Tag>
              <Tag>业务日期：{rangeLabel}</Tag>
              {statsPayload ? <Text type="secondary">共 {summary.total} 条审批</Text> : null}
            </Space>
            <Text type="secondary">快捷范围会立即刷新统计结果</Text>
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
