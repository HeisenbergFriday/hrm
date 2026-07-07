import React, { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Col,
  Empty,
  Form,
  Input,
  Progress,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  AuditOutlined,
  BarChartOutlined,
  CheckCircleOutlined,
  DownloadOutlined,
  FileExcelOutlined,
  FlagOutlined,
  LineChartOutlined,
  ReloadOutlined,
  RiseOutlined,
  SearchOutlined,
  TeamOutlined,
  TrophyOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import {
  PerformanceActivity,
  PerformanceChartItem,
  PerformanceContentReportRow,
  PerformanceProgressReportRow,
  PerformanceReport,
  PerformanceReportFilters,
  PerformanceResultReportRow,
  departmentAPI,
  performanceAPI,
} from '../services/api'
import './PerformanceReports.css'

const { Text } = Typography

type ReportTabKey = 'progress' | 'content' | 'result'

interface DepartmentOption {
  department_id: string
  name: string
  parent_id?: string
}

interface ReportFormValues extends PerformanceReportFilters {
  activity_id?: number
}

const PARTICIPANT_STATUS_LABELS: Record<string, { label: string; color: string }> = {
  pending: { label: '待开始', color: 'default' },
  target_pending_approval: { label: '目标待审核', color: 'warning' },
  target_rejected: { label: '目标驳回', color: 'error' },
  target_set: { label: '目标已定', color: 'processing' },
  self_submitted: { label: '自评已交', color: 'processing' },
  manager_submitted: { label: '上级已评', color: 'processing' },
  manager_recheck: { label: '待上级复核', color: 'warning' },
  employee_confirmed: { label: '员工已确认', color: 'success' },
  manager_confirmed: { label: '主管已确认', color: 'success' },
  hr_confirmed: { label: 'HR已确认', color: 'success' },
  result_confirmed: { label: '结果已确认', color: 'success' },
  locked: { label: '已锁定', color: 'success' },
}

const REPORT_TABLE_PAGINATION = {
  pageSize: 20,
  showSizeChanger: true,
  locale: { items_per_page: '条/页' },
  showTotal: (total: number) => `共 ${total} 条`,
}

const STATUS_OPTIONS = Object.entries(PARTICIPANT_STATUS_LABELS).map(([value, meta]) => ({
  value,
  label: meta.label,
}))

const LEVEL_OPTIONS = ['S', 'A', 'B', 'C', 'D'].map(level => ({ value: level, label: level }))

function unwrapData<T>(res: any, fallback: T): T {
  return (res?.data || res || fallback) as T
}

function unwrapList<T>(res: any, key: string): T[] {
  const data = unwrapData<any>(res, {})
  return data?.[key] || data?.items || []
}

function compactFilters(values: ReportFormValues): PerformanceReportFilters {
  return {
    company_id: values.company_id || undefined,
    department_id: values.department_id || undefined,
    status: values.status || undefined,
    level: values.level || undefined,
    employee_keyword: values.employee_keyword?.trim() || undefined,
  }
}

function statusTag(status: string) {
  const meta = PARTICIPANT_STATUS_LABELS[status] || { label: status || '-', color: 'default' }
  return <StatusTag color={meta.color}>{meta.label}</StatusTag>
}

function chartItemName(name: string) {
  return PARTICIPANT_STATUS_LABELS[name]?.label || name || '-'
}

function yesNoTag(value: boolean) {
  return value ? <Tag color="success">是</Tag> : <Tag>否</Tag>
}

function levelTag(level?: string) {
  if (!level) return '-'
  const colorMap: Record<string, string> = { S: 'purple', A: 'green', B: 'blue', C: 'orange', D: 'red' }
  return <Tag color={colorMap[level] || 'default'}>{level}</Tag>
}

function formatPercent(value?: number) {
  return `${Number(value || 0).toFixed(2)}%`
}

function clampPercent(value?: number) {
  const percent = Number(value || 0)
  if (!Number.isFinite(percent)) return 0
  return Math.max(0, Math.min(100, percent))
}

function calcRate(value: number, total: number) {
  if (!total) return 0
  return (value / total) * 100
}

function pendingCount(total: number, done?: number) {
  return Math.max(total - Number(done || 0), 0)
}

function formatNumber(value?: number | null) {
  if (value === null || value === undefined) return '-'
  return Number(value).toFixed(2)
}

function renderScore(value: number, visible = true) {
  if (!visible) return '-'
  return formatNumber(value)
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

type ReportTone = 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'purple'

interface MetricCardProps {
  title: string
  value: number | string
  suffix?: string
  icon?: React.ReactNode
  tone?: ReportTone
  description?: React.ReactNode
  progress?: number
}

interface InsightItem {
  label: string
  value: number | string
  description?: React.ReactNode
  tone?: ReportTone
}

interface StageMetric {
  label: string
  value: number
  total: number
}

interface FocusItem {
  title: React.ReactNode
  meta?: React.ReactNode
  value: React.ReactNode
  tone?: ReportTone
}

const ACTIVITY_STATUS_LABELS: Record<string, { label: string; color: string }> = {
  draft: { label: '草稿', color: 'default' },
  target_setting: { label: '目标拟定', color: 'processing' },
  target_approval: { label: '目标审核', color: 'warning' },
  self_evaluation: { label: '自评中', color: 'processing' },
  manager_evaluation: { label: '上级评估', color: 'processing' },
  department_evaluation: { label: '部门/中心评估', color: 'processing' },
  hr_review: { label: 'HR审核', color: 'warning' },
  hr_confirmation: { label: 'HR确认', color: 'warning' },
  result_publish: { label: '结果公布', color: 'success' },
  locked: { label: '已锁定', color: 'success' },
  archived: { label: '已归档', color: 'success' },
}

function activityStatusTag(status?: string) {
  const meta = ACTIVITY_STATUS_LABELS[status || ''] || { label: status || '-', color: 'default' }
  return <StatusTag color={meta.color}>{meta.label}</StatusTag>
}

function activityKindLabel(activity?: PerformanceActivity | null, isNewFlow?: boolean) {
  if (!activity) return '-'
  if (activity.activity_kind === 'goal_setting') return '目标设定活动'
  if (activity.activity_kind === 'review_scoring') return '评分活动'
  return isNewFlow || activity.flow_type === 'new' ? '沐腾新流程' : '旧流程/兼容流程'
}

function distributionColor(name: string) {
  if (name === 'S') return 'var(--color-primary)'
  if (name === 'A') return 'var(--color-success)'
  if (name === 'B') return 'var(--color-info)'
  if (name === 'C') return 'var(--color-warning)'
  if (name === 'D') return 'var(--color-error)'
  if (PARTICIPANT_STATUS_LABELS[name]?.color === 'success') return 'var(--color-success)'
  if (PARTICIPANT_STATUS_LABELS[name]?.color === 'warning') return 'var(--color-warning)'
  if (PARTICIPANT_STATUS_LABELS[name]?.color === 'error') return 'var(--color-error)'
  return 'var(--color-info)'
}

function sortedChartItems(items?: PerformanceChartItem[]) {
  return [...(items || [])].sort((left, right) => Number(right.value || 0) - Number(left.value || 0))
}

const MetricCard: React.FC<MetricCardProps> = ({
  title,
  value,
  suffix,
  icon,
  tone = 'default',
  description,
  progress,
}) => (
  <div className={`performanceReportMetric performanceReportMetric-${tone}`}>
    <div className="performanceReportMetricHeader">
      {icon && <span className="performanceReportMetricIcon">{icon}</span>}
      <Statistic title={title} value={value} suffix={suffix} />
    </div>
    {description && <Text type="secondary" className="performanceReportMetricDescription">{description}</Text>}
    {progress !== undefined && <Progress percent={clampPercent(progress)} showInfo={false} size="small" />}
  </div>
)

const DistributionBars: React.FC<{ title: string; items: PerformanceChartItem[]; emptyText?: string }> = ({
  title,
  items,
  emptyText = '暂无分布数据',
}) => (
  <section className="performanceReportPanel">
    <div className="performanceReportPanelHeader">
      <Text strong>{title}</Text>
      {items.length > 0 && <Text type="secondary">共 {items.reduce((sum, item) => sum + Number(item.value || 0), 0)} 条</Text>}
    </div>
    <div className="performanceReportDistribution">
      {items.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyText} />
      ) : (
        <Space direction="vertical" className="performanceReportFullWidth" size={10}>
          {items.map(item => (
            <div key={item.name} className="performanceReportDistributionItem">
              <div className="performanceReportDistributionMeta">
                <Text ellipsis className="performanceReportDistributionName">{chartItemName(item.name)}</Text>
                <Text type="secondary">{item.value} / {formatPercent(item.rate)}</Text>
              </div>
              <Progress
                percent={clampPercent(item.rate)}
                showInfo={false}
                size="small"
                strokeColor={distributionColor(item.name)}
              />
            </div>
          ))}
        </Space>
      )}
    </div>
  </section>
)

const InsightList: React.FC<{ title: string; items: InsightItem[] }> = ({ title, items }) => (
  <section className="performanceReportPanel performanceReportInsightPanel">
    <div className="performanceReportPanelHeader">
      <Text strong>{title}</Text>
    </div>
    <div className="performanceReportInsightList">
      {items.map(item => (
        <div key={item.label} className={`performanceReportInsight performanceReportInsight-${item.tone || 'default'}`}>
          <div>
            <Text type="secondary">{item.label}</Text>
            {item.description && <div className="performanceReportInsightDescription">{item.description}</div>}
          </div>
          <Text strong className="performanceReportInsightValue">{item.value}</Text>
        </div>
      ))}
    </div>
  </section>
)

const StageFunnel: React.FC<{ title: string; stages: StageMetric[] }> = ({ title, stages }) => (
  <section className="performanceReportPanel">
    <div className="performanceReportPanelHeader">
      <Text strong>{title}</Text>
      <Text type="secondary">节点完成率</Text>
    </div>
    <div className="performanceReportStageList">
      {stages.map(stage => {
        const rate = calcRate(stage.value, stage.total)
        return (
          <div key={stage.label} className="performanceReportStageItem">
            <div className="performanceReportStageMeta">
              <Text>{stage.label}</Text>
              <Text type="secondary">{stage.value} / {stage.total || 0}</Text>
            </div>
            <Progress percent={clampPercent(rate)} size="small" />
          </div>
        )
      })}
    </div>
  </section>
)

const FocusList: React.FC<{ title: string; items: FocusItem[]; emptyText: string }> = ({ title, items, emptyText }) => (
  <section className="performanceReportPanel">
    <div className="performanceReportPanelHeader">
      <Text strong>{title}</Text>
    </div>
    {items.length === 0 ? (
      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyText} />
    ) : (
      <div className="performanceReportFocusList">
        {items.map((item, index) => (
          <div key={`${index}-${String(item.title)}`} className={`performanceReportFocusItem performanceReportFocusItem-${item.tone || 'default'}`}>
            <div className="performanceReportFocusIndex">{index + 1}</div>
            <div className="performanceReportFocusBody">
              <Text strong ellipsis>{item.title}</Text>
              {item.meta && <Text type="secondary" className="performanceReportFocusMeta">{item.meta}</Text>}
            </div>
            <div className="performanceReportFocusValue">{item.value}</div>
          </div>
        ))}
      </div>
    )}
  </section>
)

const PerformanceReports: React.FC = () => {
  const [form] = Form.useForm<ReportFormValues>()
  const [activities, setActivities] = useState<PerformanceActivity[]>([])
  const [departments, setDepartments] = useState<DepartmentOption[]>([])
  const [report, setReport] = useState<PerformanceReport | null>(null)
  const [baseLoading, setBaseLoading] = useState(false)
  const [reportLoading, setReportLoading] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [activeTab, setActiveTab] = useState<ReportTabKey>('progress')
  const selectedActivityID = Form.useWatch('activity_id', form)

  const companyOptions = useMemo(() => {
    return departments
      .filter(department => !department.parent_id || department.parent_id === '0')
      .map(department => ({ value: department.department_id, label: department.name }))
  }, [departments])

  const departmentOptions = useMemo(() => {
    return departments.map(department => ({
      value: department.department_id,
      label: `${department.name}（${department.department_id}）`,
    }))
  }, [departments])

  const activityOptions = useMemo(() => {
    return activities.map(activity => ({
      value: activity.id,
      label: `${activity.name}（${activity.start_date} 至 ${activity.end_date}）`,
    }))
  }, [activities])

  const selectedActivity = useMemo(() => {
    if (report?.activity && report.activity.id === selectedActivityID) return report.activity
    return activities.find(activity => activity.id === Number(selectedActivityID)) || report?.activity || null
  }, [activities, report?.activity, selectedActivityID])

  const reportOverview = useMemo(() => {
    const progressSummary = report?.progress.summary
    const resultSummary = report?.result.summary
    const total = Number(progressSummary?.total_participants || resultSummary?.total_participants || 0)
    const lockedCount = Number(resultSummary?.locked_count || progressSummary?.locked_count || 0)
    const hiddenCount = Number(resultSummary?.hidden_count || 0)
    const averageScore = Number(resultSummary?.average_score || 0)

    return {
      total,
      lockedCount,
      hiddenCount,
      averageScore,
      targetSubmitted: Number(progressSummary?.target_submitted_count || 0),
      selfSubmitted: Number(progressSummary?.self_submitted_count || 0),
      managerSubmitted: Number(progressSummary?.manager_submitted_count || 0),
      departmentReviewed: Number(progressSummary?.department_reviewed_count || 0),
      hrConfirmed: Number(progressSummary?.hr_confirmed_count || 0),
      completionRate: Number(progressSummary?.completion_rate || calcRate(lockedCount, total)),
    }
  }, [report])

  const overviewInsights = useMemo<InsightItem[]>(() => {
    const total = reportOverview.total
    const pendingSelf = pendingCount(total, reportOverview.selfSubmitted)
    const pendingManager = pendingCount(total, reportOverview.managerSubmitted)
    const pendingHR = pendingCount(total, reportOverview.hrConfirmed)
    const pendingLocked = pendingCount(total, reportOverview.lockedCount)

    return [
      {
        label: '待自评',
        value: pendingSelf,
        description: `完成率 ${formatPercent(calcRate(reportOverview.selfSubmitted, total))}`,
        tone: pendingSelf > 0 ? 'warning' : 'success',
      },
      {
        label: '待上级评分',
        value: pendingManager,
        description: `完成率 ${formatPercent(calcRate(reportOverview.managerSubmitted, total))}`,
        tone: pendingManager > 0 ? 'warning' : 'success',
      },
      {
        label: '待HR确认',
        value: pendingHR,
        description: `完成率 ${formatPercent(calcRate(reportOverview.hrConfirmed, total))}`,
        tone: pendingHR > 0 ? 'warning' : 'success',
      },
      {
        label: '未锁定',
        value: pendingLocked,
        description: `锁定率 ${formatPercent(calcRate(reportOverview.lockedCount, total))}`,
        tone: pendingLocked > 0 ? 'danger' : 'success',
      },
    ]
  }, [reportOverview])

  const overviewStages = useMemo<StageMetric[]>(() => {
    const total = reportOverview.total
    return [
      { label: '目标提交', value: reportOverview.targetSubmitted, total },
      { label: '自评提交', value: reportOverview.selfSubmitted, total },
      { label: '上级评分', value: reportOverview.managerSubmitted, total },
      { label: '部门/中心评估', value: reportOverview.departmentReviewed, total },
      { label: report?.is_new_flow ? 'HR审核' : 'HR确认', value: reportOverview.hrConfirmed, total },
      { label: '结果锁定', value: reportOverview.lockedCount, total },
    ]
  }, [report?.is_new_flow, reportOverview])

  const loadReport = async (activityId: number, filters: PerformanceReportFilters) => {
    setReportLoading(true)
    try {
      const res = await performanceAPI.getReport(activityId, filters)
      setReport(unwrapData<PerformanceReport | null>(res, null))
    } catch (err: any) {
      message.error(err?.response?.data?.message || '获取绩效报表失败')
      setReport(null)
    } finally {
      setReportLoading(false)
    }
  }

  const loadBaseData = async () => {
    setBaseLoading(true)
    try {
      const [activityRes, departmentRes] = await Promise.all([
        performanceAPI.getActivities({ page: 1, page_size: 100 }),
        departmentAPI.getDepartments(),
      ])
      const nextActivities = unwrapList<PerformanceActivity>(activityRes, 'items')
      const nextDepartments = unwrapList<DepartmentOption>(departmentRes, 'departments')
      setActivities(nextActivities)
      setDepartments(nextDepartments)
      const firstActivityID = nextActivities[0]?.id
      if (firstActivityID) {
        form.setFieldsValue({ activity_id: firstActivityID })
        await loadReport(firstActivityID, {})
      }
    } catch (err: any) {
      message.error(err?.response?.data?.message || '加载绩效报表基础数据失败')
    } finally {
      setBaseLoading(false)
    }
  }

  useEffect(() => {
    void loadBaseData()
  }, [])

  const handleSearch = async () => {
    const values = await form.validateFields()
    if (!values.activity_id) {
      message.warning('请先选择绩效活动')
      return
    }
    await loadReport(values.activity_id, compactFilters(values))
  }

  const handleReset = async () => {
    const activityID = form.getFieldValue('activity_id') || activities[0]?.id
    form.resetFields()
    if (activityID) {
      form.setFieldsValue({ activity_id: activityID })
      await loadReport(activityID, {})
    }
  }

  const handleExport = async () => {
    const values = await form.validateFields()
    if (!values.activity_id) {
      message.warning('请先选择绩效活动')
      return
    }
    setExporting(true)
    try {
      const data = await performanceAPI.exportReport(values.activity_id, {
        ...compactFilters(values),
        report_type: activeTab,
      })
      const blob = data instanceof Blob
        ? data
        : new Blob([data as any], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
      downloadBlob(blob, `performance_report_${values.activity_id}_${activeTab}.xlsx`)
      message.success('导出成功')
    } catch (err: any) {
      message.error(err?.response?.data?.message || '导出绩效报表失败')
    } finally {
      setExporting(false)
    }
  }

  const progressColumns: ColumnsType<PerformanceProgressReportRow> = [
    { title: '员工', dataIndex: 'employee_name', key: 'employee_name', width: 120, render: (name, row) => <span>{name}<br /><Text type="secondary">{row.employee_id}</Text></span> },
    { title: '部门', dataIndex: 'department_name', key: 'department_name', width: 140, ellipsis: true },
    { title: '岗位', dataIndex: 'position', key: 'position', width: 120, ellipsis: true },
    { title: '考核上级', dataIndex: 'manager_name', key: 'manager_name', width: 120, render: value => value || '-' },
    { title: '当前状态', dataIndex: 'status', key: 'status', width: 120, render: statusTag },
    { title: '目标', dataIndex: 'target_submitted', key: 'target_submitted', width: 80, render: yesNoTag },
    { title: '自评', dataIndex: 'self_submitted', key: 'self_submitted', width: 80, render: yesNoTag },
    { title: '上级评分', dataIndex: 'manager_submitted', key: 'manager_submitted', width: 90, render: yesNoTag },
    { title: '部门/中心', dataIndex: 'department_reviewed', key: 'department_reviewed', width: 100, render: yesNoTag },
    { title: 'HR确认', dataIndex: 'hr_confirmed', key: 'hr_confirmed', width: 90, render: yesNoTag },
    { title: '锁定', dataIndex: 'locked', key: 'locked', width: 80, render: yesNoTag },
    {
      title: '进度',
      dataIndex: 'progress_rate',
      key: 'progress_rate',
      width: 140,
      render: value => <Progress percent={clampPercent(value)} size="small" />,
    },
  ]

  const contentColumns: ColumnsType<PerformanceContentReportRow> = [
    { title: '员工', dataIndex: 'employee_name', key: 'employee_name', width: 120, render: (name, row) => <span>{name}<br /><Text type="secondary">{row.employee_id}</Text></span> },
    { title: '部门', dataIndex: 'department_name', key: 'department_name', width: 130, ellipsis: true },
    { title: '阶段', dataIndex: 'goal_phase_label', key: 'goal_phase_label', width: 130, render: value => <Tag color={value?.includes('下季度') ? 'blue' : 'green'}>{value}</Tag> },
    { title: '目标类型', dataIndex: 'goal_type', key: 'goal_type', width: 90, render: value => value || '-' },
    { title: '类别', dataIndex: 'section_type', key: 'section_type', width: 110 },
    { title: '目标名称', dataIndex: 'item_name', key: 'item_name', width: 220, ellipsis: true },
    { title: '权重', dataIndex: 'weight', key: 'weight', width: 80, render: value => formatNumber(value) },
    { title: '目标值/计划', dataIndex: 'target_value', key: 'target_value', width: 180, ellipsis: true, render: value => value || '-' },
    { title: '完成情况', dataIndex: 'actual_result', key: 'actual_result', width: 220, ellipsis: true, render: value => value || '-' },
    { title: '完成率', dataIndex: 'completion_rate', key: 'completion_rate', width: 90, render: value => formatPercent(value) },
    { title: '自评分', dataIndex: 'self_score', key: 'self_score', width: 80, render: value => formatNumber(value) },
    { title: '主管评分', dataIndex: 'manager_score', key: 'manager_score', width: 90, render: value => formatNumber(value) },
    { title: '附件', dataIndex: 'attachments_count', key: 'attachments_count', width: 70 },
  ]

  const resultColumns: ColumnsType<PerformanceResultReportRow> = [
    { title: '员工', dataIndex: 'employee_name', key: 'employee_name', width: 120, render: (name, row) => <span>{name}<br /><Text type="secondary">{row.employee_id}</Text></span> },
    { title: '部门', dataIndex: 'department_name', key: 'department_name', width: 130, ellipsis: true },
    { title: '岗位', dataIndex: 'position', key: 'position', width: 110, ellipsis: true },
    { title: '考核上级', dataIndex: 'manager_name', key: 'manager_name', width: 120, render: value => value || '-' },
    { title: '状态', dataIndex: 'status', key: 'status', width: 120, render: statusTag },
    { title: '自评分', dataIndex: 'self_score', key: 'self_score', width: 80, render: (value, row) => renderScore(value, row.result_visible) },
    { title: '主管评分', dataIndex: 'manager_score', key: 'manager_score', width: 90, render: (value, row) => renderScore(value, row.result_visible) },
    { title: '调整后分数', dataIndex: 'adjusted_score', key: 'adjusted_score', width: 100, render: (value, row) => renderScore(value, row.result_visible) },
    { title: '建议等级', dataIndex: 'suggested_level', key: 'suggested_level', width: 90, render: (_value, row) => row.result_visible ? levelTag(row.suggested_level) : '-' },
    { title: '最终等级', dataIndex: 'effective_final_level', key: 'effective_final_level', width: 90, render: (_value, row) => row.result_visible ? levelTag(row.effective_final_level) : '-' },
    { title: '部门最终分', dataIndex: 'department_final_score', key: 'department_final_score', width: 100, render: (value, row) => renderScore(value, row.result_visible) },
    { title: 'HR确认', dataIndex: 'hr_confirmed', key: 'hr_confirmed', width: 90, render: yesNoTag },
    { title: '锁定', dataIndex: 'locked', key: 'locked', width: 80, render: yesNoTag },
    { title: '屏蔽', dataIndex: 'result_hidden', key: 'result_hidden', width: 80, render: value => value ? <Tag color="warning">是</Tag> : <Tag>否</Tag> },
  ]

  const renderOverview = () => {
    const hasReport = Boolean(report)
    return (
      <PageCard className="performanceReportOverviewCard">
        <div className="performanceReportOverview">
          <div className="performanceReportActivity">
            <div className="performanceReportActivityHeader">
              <div>
                <Text type="secondary">当前活动</Text>
                <h3>{selectedActivity?.name || '未选择绩效活动'}</h3>
              </div>
              <Space wrap>
                {activityStatusTag(selectedActivity?.status)}
                <Tag color={report?.is_new_flow || selectedActivity?.flow_type === 'new' ? 'blue' : 'default'}>
                  {activityKindLabel(selectedActivity, report?.is_new_flow)}
                </Tag>
              </Space>
            </div>
            <div className="performanceReportActivityMeta">
              <span>{selectedActivity?.cycle_type || '-'} 周期</span>
              <span>{selectedActivity ? `${selectedActivity.start_date} 至 ${selectedActivity.end_date}` : '-'}</span>
              <span>快照：{selectedActivity?.snapshot_as_of_date || '未设置'}</span>
            </div>
            <div className="performanceReportActivitySummary">
              <MetricCard
                title="参与人数"
                value={reportOverview.total}
                icon={<TeamOutlined />}
                tone="primary"
                description={hasReport ? '当前筛选范围内人数' : '暂无报表数据'}
              />
              <MetricCard
                title="整体完成率"
                value={formatNumber(reportOverview.completionRate)}
                suffix="%"
                icon={<CheckCircleOutlined />}
                tone={reportOverview.completionRate >= 100 ? 'success' : 'warning'}
                progress={reportOverview.completionRate}
              />
              <MetricCard
                title="平均分"
                value={formatNumber(reportOverview.averageScore)}
                icon={<TrophyOutlined />}
                tone="purple"
                description="按可见结果计算"
              />
              <MetricCard
                title="屏蔽结果"
                value={reportOverview.hiddenCount}
                icon={<WarningOutlined />}
                tone={reportOverview.hiddenCount > 0 ? 'danger' : 'default'}
                description="遵循结果屏蔽权限"
              />
            </div>
          </div>
          <InsightList title="待处理重点" items={overviewInsights} />
        </div>
        <StageFunnel title="流程节点漏斗" stages={overviewStages} />
      </PageCard>
    )
  }

  const renderProgressTab = () => {
    const progress = report?.progress
    const summary = progress?.summary
    const total = Number(summary?.total_participants || 0)
    const bottleneckItems: InsightItem[] = [
      {
        label: '目标未提交',
        value: pendingCount(total, summary?.target_submitted_count),
        description: `目标提交率 ${formatPercent(calcRate(Number(summary?.target_submitted_count || 0), total))}`,
        tone: pendingCount(total, summary?.target_submitted_count) > 0 ? 'warning' : 'success',
      },
      {
        label: '自评未提交',
        value: pendingCount(total, summary?.self_submitted_count),
        description: `自评提交率 ${formatPercent(calcRate(Number(summary?.self_submitted_count || 0), total))}`,
        tone: pendingCount(total, summary?.self_submitted_count) > 0 ? 'warning' : 'success',
      },
      {
        label: '上级未评分',
        value: pendingCount(total, summary?.manager_submitted_count),
        description: `上级评分率 ${formatPercent(calcRate(Number(summary?.manager_submitted_count || 0), total))}`,
        tone: pendingCount(total, summary?.manager_submitted_count) > 0 ? 'danger' : 'success',
      },
    ]
    const slowProgressRows: FocusItem[] = [...(progress?.rows || [])]
      .filter(row => Number(row.progress_rate || 0) < 100)
      .sort((left, right) => Number(left.progress_rate || 0) - Number(right.progress_rate || 0))
      .slice(0, 5)
      .map(row => ({
        title: row.employee_name,
        meta: `${row.department_name || '-'} / ${row.manager_name || '无考核上级'}`,
        value: <Progress percent={clampPercent(row.progress_rate)} size="small" className="performanceReportFocusProgress" />,
        tone: Number(row.progress_rate || 0) < 50 ? 'danger' : 'warning',
      }))

    return (
      <Space direction="vertical" size={16} className="performanceReportFullWidth">
        <Row gutter={[16, 16]}>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard title="参与人数" value={total} icon={<TeamOutlined />} tone="primary" />
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard
              title="自评完成"
              value={summary?.self_submitted_count || 0}
              icon={<AuditOutlined />}
              tone="success"
              progress={calcRate(Number(summary?.self_submitted_count || 0), total)}
            />
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard
              title="上级评分完成"
              value={summary?.manager_submitted_count || 0}
              icon={<FlagOutlined />}
              tone="warning"
              progress={calcRate(Number(summary?.manager_submitted_count || 0), total)}
            />
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard
              title="锁定完成率"
              value={formatNumber(summary?.completion_rate || 0)}
              suffix="%"
              icon={<CheckCircleOutlined />}
              tone="purple"
              progress={summary?.completion_rate || 0}
            />
          </Col>
        </Row>
        <Row gutter={[16, 16]}>
          <Col xs={24} xl={8}><InsightList title="流程卡点" items={bottleneckItems} /></Col>
          <Col xs={24} xl={8}><DistributionBars title="状态分布" items={progress?.status_distribution || []} /></Col>
          <Col xs={24} xl={8}><FocusList title="低进度员工" items={slowProgressRows} emptyText="当前筛选范围内暂无低进度员工" /></Col>
        </Row>
        <Table
          rowKey="participant_id"
          columns={progressColumns}
          dataSource={progress?.rows || []}
          loading={reportLoading}
          scroll={{ x: 1320 }}
          pagination={REPORT_TABLE_PAGINATION}
        />
      </Space>
    )
  }

  const renderContentTab = () => {
    const content = report?.content
    const summary = content?.summary
    const contentRows = content?.rows || []
    const lowCompletionRows: FocusItem[] = contentRows
      .filter(row => row.goal_phase !== 'plan' && Number(row.completion_rate || 0) < 100)
      .sort((left, right) => Number(left.completion_rate || 0) - Number(right.completion_rate || 0))
      .slice(0, 5)
      .map(row => ({
        title: row.item_name || row.employee_name,
        meta: `${row.employee_name} / ${row.department_name || '-'} / 权重 ${formatNumber(row.weight)}`,
        value: <Text strong>{formatPercent(row.completion_rate)}</Text>,
        tone: Number(row.completion_rate || 0) < 60 ? 'danger' : 'warning',
      }))
    const highWeightPlanRows: FocusItem[] = contentRows
      .filter(row => row.goal_phase === 'plan')
      .sort((left, right) => Number(right.weight || 0) - Number(left.weight || 0))
      .slice(0, 5)
      .map(row => ({
        title: row.item_name || row.employee_name,
        meta: `${row.employee_name} / ${row.department_name || '-'} / ${row.goal_type || '目标'}`,
        value: <Text strong>{formatNumber(row.weight)}</Text>,
        tone: 'primary',
      }))

    return (
      <Space direction="vertical" size={16} className="performanceReportFullWidth">
        {report?.is_new_flow && (
          <Alert
            type="info"
            showIcon
            message="沐腾科技流程报表会同时展示 review 上季度完成情况和 plan 下季度目标计划。"
          />
        )}
        <Row gutter={[16, 16]}>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard title="上季度目标数" value={summary?.review_item_count || 0} icon={<AuditOutlined />} tone="success" />
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard title="下季度计划数" value={summary?.plan_item_count || 0} icon={<FlagOutlined />} tone="primary" />
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard
              title="有完成记录人数"
              value={summary?.participants_with_review || 0}
              icon={<TeamOutlined />}
              tone="warning"
              description={`有计划人数 ${summary?.participants_with_plan || 0}`}
            />
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard
              title="平均完成率"
              value={formatNumber(summary?.average_completion_rate || 0)}
              suffix="%"
              icon={<LineChartOutlined />}
              tone="purple"
              progress={summary?.average_completion_rate || 0}
            />
          </Col>
        </Row>
        <Row gutter={[16, 16]}>
          <Col xs={24} xl={8}><DistributionBars title="目标阶段分布" items={content?.phases || []} /></Col>
          <Col xs={24} xl={8}><FocusList title="低完成率目标" items={lowCompletionRows} emptyText="当前筛选范围内暂无低完成率目标" /></Col>
          <Col xs={24} xl={8}><FocusList title="下季度重点目标" items={highWeightPlanRows} emptyText="当前筛选范围内暂无下季度计划目标" /></Col>
        </Row>
        <Table
          rowKey="id"
          columns={contentColumns}
          dataSource={content?.rows || []}
          loading={reportLoading}
          scroll={{ x: 1500 }}
          pagination={REPORT_TABLE_PAGINATION}
        />
      </Space>
    )
  }

  const renderResultTab = () => {
    const result = report?.result
    const summary = result?.summary
    const resultRows = result?.rows || []
    const total = Number(summary?.total_participants || 0)
    const visibleScoreRows: FocusItem[] = resultRows
      .filter(row => row.result_visible)
      .sort((left, right) => Number((right.department_final_score ?? right.adjusted_score) || 0) - Number((left.department_final_score ?? left.adjusted_score) || 0))
      .slice(0, 5)
      .map(row => ({
        title: row.employee_name,
        meta: `${row.department_name || '-'} / ${row.effective_final_level || '未定级'}`,
        value: <Text strong>{formatNumber(row.department_final_score ?? row.adjusted_score)}</Text>,
        tone: 'success',
      }))
    const resultInsights: InsightItem[] = [
      {
        label: '未锁定',
        value: pendingCount(total, summary?.locked_count),
        description: `锁定率 ${formatPercent(calcRate(Number(summary?.locked_count || 0), total))}`,
        tone: pendingCount(total, summary?.locked_count) > 0 ? 'warning' : 'success',
      },
      {
        label: '结果屏蔽',
        value: summary?.hidden_count || 0,
        description: `占比 ${formatPercent(calcRate(Number(summary?.hidden_count || 0), total))}`,
        tone: Number(summary?.hidden_count || 0) > 0 ? 'danger' : 'default',
      },
      {
        label: '平均分',
        value: formatNumber(summary?.average_score || 0),
        description: '按可见结果计算',
        tone: 'purple',
      },
    ]

    return (
      <Space direction="vertical" size={16} className="performanceReportFullWidth">
        <Row gutter={[16, 16]}>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard title="结果人数" value={total} icon={<TeamOutlined />} tone="primary" />
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard
              title="已锁定"
              value={summary?.locked_count || 0}
              icon={<CheckCircleOutlined />}
              tone="success"
              progress={calcRate(Number(summary?.locked_count || 0), total)}
            />
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard
              title="屏蔽结果"
              value={summary?.hidden_count || 0}
              icon={<WarningOutlined />}
              tone={Number(summary?.hidden_count || 0) > 0 ? 'danger' : 'default'}
            />
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <MetricCard title="平均分" value={formatNumber(summary?.average_score || 0)} icon={<RiseOutlined />} tone="purple" />
          </Col>
        </Row>
        <Row gutter={[16, 16]}>
          <Col xs={24} xl={8}><InsightList title="结果关注" items={resultInsights} /></Col>
          <Col xs={24} xl={8}><DistributionBars title="等级分布" items={sortedChartItems(result?.level_distribution)} /></Col>
          <Col xs={24} xl={8}><FocusList title="高分结果" items={visibleScoreRows} emptyText="当前筛选范围内暂无可见结果" /></Col>
        </Row>
        <Row gutter={[16, 16]}>
          <Col xs={24}><DistributionBars title="部门人数分布" items={sortedChartItems(result?.department_distribution)} /></Col>
        </Row>
        <Table
          rowKey="participant_id"
          columns={resultColumns}
          dataSource={result?.rows || []}
          loading={reportLoading}
          scroll={{ x: 1480 }}
          pagination={REPORT_TABLE_PAGINATION}
        />
      </Space>
    )
  }

  return (
    <PageContainer
      title="绩效报表"
      icon={<BarChartOutlined />}
      subtitle="按活动、公司、部门、员工、流程状态和等级筛选绩效进度、考核内容与结果。"
      extra={
        <Button icon={<ReloadOutlined />} onClick={() => void loadBaseData()} loading={baseLoading || reportLoading}>
          刷新
        </Button>
      }
    >
      <Space direction="vertical" size={16} className="performanceReportFullWidth">
        <PageCard className="performanceReportFilterCard">
          <Form form={form} layout="inline" onFinish={() => void handleSearch()} className="performanceReportFilters">
            <Form.Item name="activity_id" label="绩效活动" rules={[{ required: true, message: '请选择绩效活动' }]}>
              <Select
                showSearch
                style={{ width: 320 }}
                placeholder="请选择绩效活动"
                options={activityOptions}
                optionFilterProp="label"
              />
            </Form.Item>
            <Form.Item name="company_id" label="公司">
              <Select allowClear showSearch style={{ width: 180 }} placeholder="全部公司" options={companyOptions} optionFilterProp="label" />
            </Form.Item>
            <Form.Item name="department_id" label="部门">
              <Select allowClear showSearch style={{ width: 220 }} placeholder="全部部门" options={departmentOptions} optionFilterProp="label" />
            </Form.Item>
            <Form.Item name="employee_keyword" label="员工">
              <Input allowClear style={{ width: 180 }} placeholder="姓名/工号" />
            </Form.Item>
            <Form.Item name="status" label="流程状态">
              <Select allowClear style={{ width: 150 }} placeholder="全部状态" options={STATUS_OPTIONS} />
            </Form.Item>
            <Form.Item name="level" label="等级">
              <Select allowClear style={{ width: 110 }} placeholder="全部" options={LEVEL_OPTIONS} />
            </Form.Item>
            <Form.Item>
              <Space>
                <Button type="primary" htmlType="submit" icon={<SearchOutlined />} loading={reportLoading}>查询</Button>
                <Button onClick={() => void handleReset()}>重置</Button>
                <Button icon={<FileExcelOutlined />} onClick={() => void handleExport()} loading={exporting} disabled={!report}>
                  导出当前报表
                </Button>
              </Space>
            </Form.Item>
          </Form>
        </PageCard>

        {(activities.length > 0 || report) && renderOverview()}

        <PageCard className="performanceReportTabsCard">
          {activities.length === 0 && !baseLoading ? (
            <Empty description="暂无可查看的绩效活动" />
          ) : (
            <Tabs
              activeKey={activeTab}
              onChange={key => setActiveTab(key as ReportTabKey)}
              tabBarExtraContent={
                <Button icon={<DownloadOutlined />} onClick={() => void handleExport()} loading={exporting} disabled={!report}>
                  导出
                </Button>
              }
              items={[
                { key: 'progress', label: '考核进度报表', children: renderProgressTab() },
                { key: 'content', label: '绩效考核内容报表', children: renderContentTab() },
                { key: 'result', label: '考核结果报表', children: renderResultTab() },
              ]}
            />
          )}
        </PageCard>
      </Space>
    </PageContainer>
  )
}

export default PerformanceReports
