import React, { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Alert,
  Button,
  Card,
  Col,
  DatePicker,
  Descriptions,
  Divider,
  Drawer,
  Dropdown,
  Empty,
  Grid,
  Pagination,
  Row,
  Segmented,
  Select,
  Space,
  Spin,
  Statistic,
  Table,
  Tag,
  Timeline,
  Tooltip,
  Typography,
  message,
  Modal,
} from 'antd'
import {
  CalendarOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  DownOutlined,
  EllipsisOutlined,
  FileDoneOutlined,
  ReloadOutlined,
  SearchOutlined,
  SyncOutlined,
  TeamOutlined,
  UndoOutlined,
  UpOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { attendanceAPI, departmentAPI, userAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import { hasMenuPermission, hasPermission } from '../utils/permission'
import { resolveMobileLayout, useMobileRuntime } from '../utils/responsive'
import './Attendance.css'

const { Text } = Typography
const { RangePicker } = DatePicker

export interface AttendanceDailyPunch {
  check_type: string
  check_time: string
  time_result?: string
  location_result?: string
  user_address?: string
  source_type?: string
}

export interface AttendanceDailyStatus {
  code: string
  label: string
  level: 'error' | 'warning' | 'processing' | 'success' | 'default'
  category: 'attendance' | 'approval'
}

export interface AttendanceDailyApproval {
  proc_inst_id: string
  tag_name: string
  sub_type?: string
  label: string
  begin_time?: string
  end_time?: string
  duration?: string
  duration_unit?: string
}

export interface AttendanceDailyResult {
  key: string
  work_date: string
  user_id: string
  external_user_id: string
  user_name: string
  department_id?: string
  department_name?: string
  on_duty_time?: string
  off_duty_time?: string
  punches: AttendanceDailyPunch[]
  statuses: AttendanceDailyStatus[]
  approvals: AttendanceDailyApproval[]
  has_exception: boolean
  source_updated_at: string
}

interface AttendanceDailySummary {
  total: number
  normal: number
  exception: number
  with_approval: number
}

type QuickRange = 'today' | 'yesterday' | '7days' | '30days' | 'custom'
type StatusFilter =
  | 'all'
  | 'normal'
  | 'exception'
  | 'approval'
  | 'late'
  | 'not_signed'
  | 'absenteeism'
  | 'leave'

interface AttendanceUserOption {
  value: string
  label: string
}

const defaultRange = (): [dayjs.Dayjs, dayjs.Dayjs] => [
  dayjs().subtract(6, 'day').startOf('day'),
  dayjs().endOf('day'),
]

const statusColor = (level: AttendanceDailyStatus['level']) => {
  switch (level) {
    case 'error':
      return 'error'
    case 'warning':
      return 'warning'
    case 'processing':
      return 'processing'
    case 'success':
      return 'success'
    default:
      return 'default'
  }
}

const formatTime = (value?: string) => {
  if (!value) return '-'
  const parsed = dayjs(value)
  return parsed.isValid() ? parsed.format('HH:mm') : '-'
}

const formatDateTime = (value?: string) => {
  if (!value) return '-'
  const parsed = dayjs(value)
  return parsed.isValid() ? parsed.format('YYYY-MM-DD HH:mm:ss') : '-'
}

const locationLabel = (punch: AttendanceDailyPunch) => {
  if (punch.user_address) return punch.user_address
  if (punch.location_result === 'Normal') return '正常范围'
  return punch.location_result || '-'
}

const statusOptions = [
  { value: 'all', label: '全部状态' },
  { value: 'normal', label: '正常' },
  { value: 'exception', label: '仅异常' },
  { value: 'approval', label: '有审批' },
  { value: 'late', label: '迟到' },
  { value: 'not_signed', label: '缺卡' },
  { value: 'absenteeism', label: '旷工' },
  { value: 'leave', label: '请假' },
]

export const DailyStatusTags: React.FC<{ statuses?: AttendanceDailyStatus[]; limit?: number }> = ({
  statuses = [],
  limit,
}) => {
  const visible = limit ? statuses.slice(0, limit) : statuses
  const hidden = limit ? Math.max(0, statuses.length - limit) : 0
  return (
    <Space size={[4, 4]} wrap>
      {visible.map((status) => (
        <StatusTag key={`${status.code}-${status.label}`} color={statusColor(status.level)}>
          {status.label}
        </StatusTag>
      ))}
      {hidden > 0 ? <Tag>+{hidden}</Tag> : null}
    </Space>
  )
}

const SummaryCard: React.FC<{
  title: string
  value: number
  icon: React.ReactNode
  tone: 'blue' | 'green' | 'orange' | 'purple'
  active: boolean
  onClick: () => void
}> = ({ title, value, icon, tone, active, onClick }) => (
  <button
    type="button"
    className={`attendance-summary-button attendance-summary-${tone}${active ? ' is-active' : ''}`}
    onClick={onClick}
    aria-pressed={active}
  >
    <Card className="attendance-summary-card" variant="borderless">
      <Statistic title={title} value={value} prefix={icon} />
    </Card>
  </button>
)

const Attendance: React.FC = () => {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const screens = Grid.useBreakpoint()
  const mobileRuntime = useMobileRuntime()
  const isMobile = resolveMobileLayout(screens.md, mobileRuntime)
  const canOpenExternalSync =
    hasPermission('attendance_manage') || hasMenuPermission('menu:attendance-external-sync')
  const canOpenExport = hasMenuPermission('menu:attendance-export')
  const canManage = hasPermission('attendance_manage')

  const [user, setUser] = useState('')
  const [department, setDepartment] = useState('')
  const [status, setStatus] = useState<StatusFilter>('all')
  const [quickRange, setQuickRange] = useState<QuickRange>('7days')
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs]>(defaultRange())
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [selectedRecord, setSelectedRecord] = useState<AttendanceDailyResult | null>(null)
  const [employeeKeyword, setEmployeeKeyword] = useState('')
  const [debouncedEmployeeKeyword, setDebouncedEmployeeKeyword] = useState('')
  const [selectedEmployeeOption, setSelectedEmployeeOption] = useState<AttendanceUserOption | null>(null)

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedEmployeeKeyword(employeeKeyword.trim())
    }, 300)
    return () => window.clearTimeout(timer)
  }, [employeeKeyword])

  const queryParams = useMemo(
    () => ({
      page,
      page_size: pageSize,
      user_id: user || undefined,
      department_id: department || undefined,
      status,
      start_date: dateRange[0].format('YYYY-MM-DD'),
      end_date: dateRange[1].format('YYYY-MM-DD'),
    }),
    [dateRange, department, page, pageSize, status, user],
  )

  const {
    data: attendanceData,
    isLoading,
    isFetching,
    isError,
    refetch,
    error,
  } = useQuery({
    queryKey: ['external-attendance-daily-results', queryParams],
    queryFn: () => attendanceAPI.externalSync.getDailyResults(queryParams),
  })

  const { data: syncStatusData } = useQuery({
    queryKey: ['external-attendance-sync-status'],
    queryFn: () => attendanceAPI.externalSync.getStatus(),
    refetchInterval: 30_000,
  })

  const { data: usersData, isFetching: isFetchingUsers } = useQuery({
    queryKey: ['users', 'attendance-filter', debouncedEmployeeKeyword],
    queryFn: () => userAPI.getUsers({
      page: 1,
      page_size: 50,
      search: debouncedEmployeeKeyword || undefined,
    }),
  })

  const { data: departmentsData } = useQuery({
    queryKey: ['departments', 'attendance-filter'],
    queryFn: () => departmentAPI.getDepartments(),
  })

  const users: Array<{ user_id: string; name: string }> = usersData?.data?.items || []
  const employeeOptions = useMemo(() => {
    const options = users.map((item) => ({ value: item.user_id, label: item.name }))
    if (selectedEmployeeOption && !options.some((item) => item.value === selectedEmployeeOption.value)) {
      return [selectedEmployeeOption, ...options]
    }
    return options
  }, [selectedEmployeeOption, users])
  const departments = departmentsData?.data?.departments || []
  const records: AttendanceDailyResult[] = attendanceData?.data?.items || []
  const total = attendanceData?.data?.total || 0
  const summary: AttendanceDailySummary = attendanceData?.data?.summary || {
    total: 0,
    normal: 0,
    exception: 0,
    with_approval: 0,
  }
  const latestExternalUpdate = syncStatusData?.data?.external_last_attendance_update

  const syncMutation = useMutation({
    mutationFn: () => attendanceAPI.externalSync.run({ source: 'attendance' }),
    onSuccess: (resp: any) => {
      const job = resp?.data
      if (job?.status === 'failed') {
        message.error(job.error_summary || '外部考勤同步失败')
      } else if (job?.status === 'partial') {
        message.warning(job.error_summary || '外部考勤同步部分成功')
      } else {
        message.success('外部考勤同步完成')
      }
      queryClient.invalidateQueries({ queryKey: ['external-attendance-daily-results'] })
      queryClient.invalidateQueries({ queryKey: ['external-attendance-sync-status'] })
    },
    onError: (err: any) => {
      message.error(err?.response?.data?.message || err?.message || '外部考勤同步失败')
    },
  })

  const handleImmediateSync = () => {
    if (!canManage) return
    Modal.confirm({
      title: '确认同步外部考勤数据？',
      content: '本次将从当前游标开始增量同步考勤明细，可能耗时较长。确认开始同步？',
      okText: '确认同步',
      cancelText: '取消',
      onOk: () => syncMutation.mutateAsync(),
    })
  }

  const changeQuickRange = (value: QuickRange) => {
    const now = dayjs()
    let nextRange: [dayjs.Dayjs, dayjs.Dayjs]
    switch (value) {
      case 'today':
        nextRange = [now.startOf('day'), now.endOf('day')]
        break
      case 'yesterday': {
        const yesterday = now.subtract(1, 'day')
        nextRange = [yesterday.startOf('day'), yesterday.endOf('day')]
        break
      }
      case '30days':
        nextRange = [now.subtract(29, 'day').startOf('day'), now.endOf('day')]
        break
      default:
        nextRange = defaultRange()
        break
    }
    setQuickRange(value)
    setDateRange(nextRange)
    setPage(1)
  }

  const resetFilters = () => {
    setUser('')
    setEmployeeKeyword('')
    setSelectedEmployeeOption(null)
    setDepartment('')
    setStatus('all')
    setQuickRange('7days')
    setDateRange(defaultRange())
    setPage(1)
  }

  const openDetails = (record: AttendanceDailyResult) => setSelectedRecord(record)

  const columns = [
    {
      title: '日期 / 员工',
      width: 250,
      render: (_: unknown, record: AttendanceDailyResult) => (
        <div className="attendance-person-cell">
          <Text className="attendance-date-text">{record.work_date}</Text>
          <Text strong className="attendance-person-name">{record.user_name}</Text>
          <Text type="secondary" className="attendance-department-text">
            {record.department_name || record.department_id || '未匹配部门'}
          </Text>
        </div>
      ),
    },
    {
      title: '打卡概览',
      width: 230,
      render: (_: unknown, record: AttendanceDailyResult) => (
        <div className="attendance-punch-overview">
          <div>
            <span>上班</span>
            <strong>{formatTime(record.on_duty_time)}</strong>
          </div>
          <div>
            <span>下班</span>
            <strong>{formatTime(record.off_duty_time)}</strong>
          </div>
          <Text type="secondary">共 {record.punches.length} 次打卡</Text>
        </div>
      ),
    },
    {
      title: '考勤状态',
      dataIndex: 'statuses',
      width: 300,
      render: (statuses: AttendanceDailyStatus[]) => <DailyStatusTags statuses={statuses} limit={4} />,
    },
    {
      title: '审批状态',
      dataIndex: 'approvals',
      width: 230,
      render: (approvals: AttendanceDailyApproval[]) =>
        approvals.length ? (
          <Space size={[4, 4]} wrap>
            {approvals.slice(0, 2).map((approval) => (
              <Tag color="blue" key={`${approval.proc_inst_id}-${approval.label}`}>
                {approval.label}
              </Tag>
            ))}
            {approvals.length > 2 ? <Tag>+{approvals.length - 2}</Tag> : null}
          </Space>
        ) : (
          <Text type="secondary">无审批</Text>
        ),
    },
    {
      title: '操作',
      width: 90,
      render: (_: unknown, record: AttendanceDailyResult) => (
        <Button
          type="link"
          onClick={(event) => {
            event.stopPropagation()
            openDetails(record)
          }}
        >
          查看详情
        </Button>
      ),
    },
  ]

  const extraMenu = {
    items: [
      {
        key: 'export',
        label: canOpenExport
          ? '导出考勤'
          : (
            <Tooltip title="你缺少 menu:attendance-export 权限，需要联系管理员添加">
              <span>导出考勤</span>
            </Tooltip>
          ),
        disabled: !canOpenExport,
        onClick: () => {
          if (!canOpenExport) return
          navigate('/attendance-export')
        },
      },
      {
        key: 'sync-center',
        label: canOpenExternalSync
          ? '外部同步中心'
          : (
            <Tooltip title="你缺少外部同步权限，需要联系管理员添加">
              <span>外部同步中心</span>
            </Tooltip>
          ),
        disabled: !canOpenExternalSync,
        onClick: () => {
          if (!canOpenExternalSync) return
          navigate('/attendance/external-sync')
        },
      },
    ],
  }

  const pageExtra = (
    <Space wrap className="attendance-page-actions">
      {!isMobile && latestExternalUpdate ? (
        <Text type="secondary">更新于 {formatDateTime(latestExternalUpdate)}</Text>
      ) : null}
      <Tooltip title="刷新当前结果">
        <Button icon={<ReloadOutlined />} aria-label="刷新考勤结果" onClick={() => refetch()} />
      </Tooltip>
      <Tooltip title={canManage ? undefined : '你缺少 attendance_manage 权限，需要联系管理员添加'}>
        <span>
          <Button
            type="primary"
            icon={<SyncOutlined />}
            loading={syncMutation.isPending}
            disabled={!canManage}
            onClick={handleImmediateSync}
          >
            立即同步
          </Button>
        </span>
      </Tooltip>
      <Dropdown menu={extraMenu} placement="bottomRight">
        <Button icon={<EllipsisOutlined />} aria-label="更多考勤操作" />
      </Dropdown>
    </Space>
  )

  return (
    <PageContainer
      className="attendance-page"
      title="考勤查询"
      icon={<ClockCircleOutlined />}
      subtitle="按员工与日期查看打卡、异常和审批状态"
      extra={pageExtra}
    >
      <Row className="attendance-summary-grid" gutter={[16, 16]}>
        <Col xs={12} lg={6}>
          <SummaryCard
            title="考勤人次"
            value={summary.total}
            icon={<TeamOutlined />}
            tone="blue"
            active={status === 'all'}
            onClick={() => {
              setStatus('all')
              setPage(1)
            }}
          />
        </Col>
        <Col xs={12} lg={6}>
          <SummaryCard
            title="正常"
            value={summary.normal}
            icon={<CheckCircleOutlined />}
            tone="green"
            active={status === 'normal'}
            onClick={() => {
              setStatus('normal')
              setPage(1)
            }}
          />
        </Col>
        <Col xs={12} lg={6}>
          <SummaryCard
            title="异常"
            value={summary.exception}
            icon={<WarningOutlined />}
            tone="orange"
            active={status === 'exception'}
            onClick={() => {
              setStatus('exception')
              setPage(1)
            }}
          />
        </Col>
        <Col xs={12} lg={6}>
          <SummaryCard
            title="请假 / 出差"
            value={summary.with_approval}
            icon={<FileDoneOutlined />}
            tone="purple"
            active={status === 'approval'}
            onClick={() => {
              setStatus('approval')
              setPage(1)
            }}
          />
        </Col>
      </Row>

      <PageCard className="attendance-filter-card">
        <div className="attendance-filter-primary">
          <Segmented
            value={quickRange}
            options={[
              { value: 'today', label: '今天' },
              { value: 'yesterday', label: '昨天' },
              { value: '7days', label: '近7天' },
              { value: '30days', label: '近30天' },
            ]}
            onChange={(value) => changeQuickRange(value as QuickRange)}
          />
          <RangePicker
            value={dateRange}
            allowClear={false}
            onChange={(dates) => {
              if (dates?.[0] && dates?.[1]) {
                setDateRange([dates[0], dates[1]])
                setQuickRange('custom')
                setPage(1)
              }
            }}
          />
          <Select
            className="attendance-status-select"
            value={status}
            options={statusOptions}
            onChange={(value) => {
              setStatus(value as StatusFilter)
              setPage(1)
            }}
          />
          <Button type="primary" icon={<SearchOutlined />} onClick={() => refetch()}>
            查询
          </Button>
          <Button icon={<UndoOutlined />} onClick={resetFilters}>
            重置
          </Button>
          <Button
            type="text"
            icon={advancedOpen ? <UpOutlined /> : <DownOutlined />}
            onClick={() => setAdvancedOpen((current) => !current)}
          >
            更多筛选
          </Button>
        </div>

        {advancedOpen ? (
          <div className="attendance-filter-advanced">
            <Select
              aria-label="搜索员工"
              placeholder="输入姓名搜索员工"
              allowClear
              showSearch
              filterOption={false}
              loading={isFetchingUsers}
              searchValue={employeeKeyword}
              value={user || undefined}
              options={employeeOptions}
              notFoundContent={isFetchingUsers ? <Spin size="small" /> : employeeKeyword.trim() ? '未找到员工' : '输入姓名搜索'}
              onSearch={setEmployeeKeyword}
              onChange={(value) => {
                setUser(value || '')
                setSelectedEmployeeOption(
                  value ? employeeOptions.find((item) => item.value === value) || null : null,
                )
                setEmployeeKeyword('')
                setPage(1)
              }}
            />
            <Select
              placeholder="选择部门"
              allowClear
              showSearch
              optionFilterProp="label"
              value={department || undefined}
              options={departments.map((item: any) => ({ value: item.department_id, label: item.name }))}
              onChange={(value) => {
                setDepartment(value || '')
                setPage(1)
              }}
            />
            <Text type="secondary">筛选条件改变后会自动刷新结果</Text>
          </div>
        ) : null}
      </PageCard>

      <PageCard className="attendance-result-card">
        <div className="attendance-result-header">
          <div>
            <Text strong>每日考勤结果</Text>
            <Text type="secondary">共 {total} 人次</Text>
          </div>
          {isFetching && !isLoading ? <Spin size="small" /> : null}
        </div>

        {isLoading ? (
          <div className="attendance-loading">
            <Spin size="large" />
          </div>
        ) : isError ? (
          <Alert
            message="考勤结果加载失败"
            description={(error as Error)?.message || '请稍后重试'}
            type="error"
            showIcon
            action={<Button size="small" onClick={() => refetch()}>重试</Button>}
          />
        ) : records.length ? (
          isMobile ? (
            <div className="attendance-mobile-list">
              {records.map((record) => (
                <button
                  type="button"
                  className={`attendance-mobile-item${record.has_exception ? ' is-exception' : ''}`}
                  key={record.key}
                  onClick={() => openDetails(record)}
                >
                  <div className="attendance-mobile-item-header">
                    <div>
                      <strong>{record.user_name}</strong>
                      <span>{record.department_name || record.department_id || '未匹配部门'}</span>
                    </div>
                    <Text>{record.work_date}</Text>
                  </div>
                  <DailyStatusTags statuses={record.statuses} limit={3} />
                  <div className="attendance-mobile-punches">
                    <div><span>上班</span><strong>{formatTime(record.on_duty_time)}</strong></div>
                    <div><span>下班</span><strong>{formatTime(record.off_duty_time)}</strong></div>
                    <div><span>打卡</span><strong>{record.punches.length} 次</strong></div>
                  </div>
                </button>
              ))}
              {total > 0 ? (
                <div style={{ display: 'flex', justifyContent: 'center', padding: '12px 0 4px' }}>
                  <Pagination
                    simple
                    current={page}
                    pageSize={pageSize}
                    total={total}
                    showTotal={(value) => `共 ${value} 人次`}
                    onChange={(nextPage, nextPageSize) => {
                      setPage(nextPageSize !== pageSize ? 1 : nextPage)
                      if (nextPageSize) setPageSize(nextPageSize)
                    }}
                  />
                </div>
              ) : null}
            </div>
          ) : (
            <Table
              rowKey="key"
              columns={columns}
              dataSource={records}
              size="middle"
              sticky
              rowClassName={(record) => (record.has_exception ? 'attendance-row-exception' : '')}
              onRow={(record) => ({
                onClick: () => openDetails(record),
                onKeyDown: (event) => {
                  if (event.key === 'Enter' || event.key === ' ') openDetails(record)
                },
                tabIndex: 0,
                'aria-label': `查看 ${record.user_name} ${record.work_date} 考勤详情`,
              })}
              pagination={{
                current: page,
                pageSize,
                total,
                showSizeChanger: true,
                pageSizeOptions: [10, 20, 50, 100],
                showTotal: (value) => `共 ${value} 人次`,
                onChange: (nextPage, nextPageSize) => {
                  setPage(nextPageSize !== pageSize ? 1 : nextPage)
                  setPageSize(nextPageSize)
                },
              }}
            />
          )
        ) : (
          <Empty description="当前筛选范围暂无考勤数据">
            <Button onClick={resetFilters}>清除筛选</Button>
          </Empty>
        )}
      </PageCard>

      <Drawer
        className="attendance-detail-drawer"
        title={selectedRecord ? `${selectedRecord.user_name} · ${selectedRecord.work_date}` : '考勤详情'}
        placement="right"
        width={isMobile ? '100%' : 640}
        open={Boolean(selectedRecord)}
        onClose={() => setSelectedRecord(null)}
      >
        {selectedRecord ? (
          <Space direction="vertical" size={18} className="attendance-detail-content">
            <div className="attendance-detail-status">
              <DailyStatusTags statuses={selectedRecord.statuses} />
            </div>
            <Descriptions bordered column={1} size="small">
              <Descriptions.Item label="员工">{selectedRecord.user_name}</Descriptions.Item>
              <Descriptions.Item label="部门">
                {selectedRecord.department_name || selectedRecord.department_id || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="日期">{selectedRecord.work_date}</Descriptions.Item>
              <Descriptions.Item label="打卡概览">
                上班 {formatTime(selectedRecord.on_duty_time)} · 下班 {formatTime(selectedRecord.off_duty_time)} · 共 {selectedRecord.punches.length} 次
              </Descriptions.Item>
            </Descriptions>

            <Divider orientation="left">打卡时间轴</Divider>
            {selectedRecord.punches.length ? (
              <Timeline
                items={selectedRecord.punches.map((punch) => ({
                  color: punch.time_result && punch.time_result !== 'Normal' ? 'orange' : 'green',
                  children: (
                    <Space direction="vertical" size={2}>
                      <Text strong>{punch.check_type} · {formatDateTime(punch.check_time)}</Text>
                      <Text type="secondary">地点：{locationLabel(punch)}</Text>
                      <Text type="secondary">结果：{punch.time_result || 'Normal'} / {punch.location_result || 'Normal'}</Text>
                    </Space>
                  ),
                }))}
              />
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当天无打卡记录" />
            )}

            <Divider orientation="left">审批记录</Divider>
            {selectedRecord.approvals.length ? (
              <div className="attendance-approval-list">
                {selectedRecord.approvals.map((approval) => (
                  <Alert
                    key={`${approval.proc_inst_id}-${approval.label}`}
                    type="info"
                    showIcon
                    message={approval.label}
                    description={approval.begin_time || approval.end_time
                      ? `${formatDateTime(approval.begin_time)} 至 ${formatDateTime(approval.end_time)}`
                      : '审批时间段未提供'}
                  />
                ))}
              </div>
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当天无请假、出差等审批" />
            )}
          </Space>
        ) : null}
      </Drawer>
    </PageContainer>
  )
}

export default Attendance
