import React, { useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Col,
  Descriptions,
  Empty,
  Modal,
  Row,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Tooltip,
  message,
} from 'antd'
import {
  CloudSyncOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SyncOutlined,
} from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { attendanceAPI } from '../services/api'
import { useAuthStore } from '../store/authStore'
import { hasMenuPermission, hasPermission } from '../utils/permission'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'

const { Option } = Select

interface SyncCursor {
  source_table: string
  cursor_time?: string
  last_success_at?: string
  last_error_summary?: string
}

interface SyncJob {
  id: number
  org_id: string
  trigger: string
  source: string
  status: string
  started_at: string
  finished_at?: string
  cursor_from?: string
  cursor_to?: string
  inserted: number
  updated: number
  skipped: number
  failed: number
  error_summary?: string
  operator_user_id?: string
}

interface SyncStatus {
  org_id: string
  org_name: string
  enabled: boolean
  source_healthy: boolean
  source_latency_ms?: number
  source_error?: string
  external_last_attendance_update?: string
  external_last_department_update?: string
  cursors?: SyncCursor[]
  last_job?: SyncJob | null
}

const statusColor = (status?: string) => {
  switch (status) {
    case 'success':
      return 'success'
    case 'partial':
      return 'warning'
    case 'failed':
      return 'error'
    case 'running':
      return 'processing'
    default:
      return 'default'
  }
}

const formatTime = (value?: string) => {
  if (!value) return '—'
  const d = dayjs(value)
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : value
}

const AttendanceExternalSync: React.FC = () => {
  const queryClient = useQueryClient()
  const orgId = useAuthStore((s) => s.orgId)

  const canManage = hasPermission('attendance_manage')
  const canView =
    canManage ||
    hasMenuPermission('menu:attendance') ||
    hasMenuPermission('menu:attendance-external-sync')

  const [source, setSource] = useState<'all' | 'attendance' | 'department'>('all')
  const [page, setPage] = useState(1)
  const pageSize = 10

  const {
    data: statusResp,
    isLoading: statusLoading,
    isError: statusError,
    error: statusErr,
    refetch: refetchStatus,
  } = useQuery({
    queryKey: ['external-attendance-sync-status', orgId],
    queryFn: () => attendanceAPI.externalSync.getStatus(),
    enabled: canView,
    refetchInterval: 30_000,
  })

  const {
    data: jobsResp,
    isLoading: jobsLoading,
    isError: jobsError,
    error: jobsErr,
    refetch: refetchJobs,
  } = useQuery({
    queryKey: ['external-attendance-sync-jobs', orgId, page],
    queryFn: () => attendanceAPI.externalSync.getJobs({ page, page_size: pageSize }),
    enabled: canView,
  })

  const runMutation = useMutation({
    mutationFn: () =>
      attendanceAPI.externalSync.run({
        source,
        // 仅“仅部门关系”做完整快照后失活缺失关系；all/attendance 走增量，禁止误删
        full_department_snapshot: source === 'department',
      }),
    onSuccess: (resp: any) => {
      const job = resp?.data
      if (job?.status === 'failed') {
        message.error(job.error_summary || '同步失败')
      } else if (job?.status === 'partial') {
        message.warning(job.error_summary || '同步部分成功')
      } else {
        message.success('同步任务已完成')
      }
      queryClient.invalidateQueries({ queryKey: ['external-attendance-sync-status'] })
      queryClient.invalidateQueries({ queryKey: ['external-attendance-sync-jobs'] })
    },
    onError: (err: any) => {
      message.error(err?.response?.data?.message || err?.message || '同步失败')
    },
  })

  const status: SyncStatus | undefined = statusResp?.data
  const jobs: SyncJob[] = jobsResp?.data?.list || []
  const total: number = jobsResp?.data?.total || 0
  const missingManageTip = '你缺少 attendance_manage 权限，需要联系管理员添加'

  const confirmRun = () => {
    const isDepartmentSnapshot = source === 'department'
    Modal.confirm({
      title: '确认同步外部考勤数据？',
      content: isDepartmentSnapshot
        ? '本次将同步部门人员关系完整快照。仅当同步阶段全部成功时，系统才会失活源表中已缺失的部门关系。'
        : source === 'attendance'
          ? '本次将从当前游标开始增量同步考勤明细。'
          : '本次将增量同步考勤明细和部门人员关系，不会执行部门关系失活。',
      okText: '确认同步',
      cancelText: '取消',
      onOk: () => runMutation.mutateAsync(),
    })
  }

  const columns = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 70 },
      { title: '来源', dataIndex: 'source', width: 110, render: (v: string) => v || '—' },
      { title: '触发', dataIndex: 'trigger', width: 90 },
      {
        title: '状态',
        dataIndex: 'status',
        width: 100,
        render: (v: string) => <Tag color={statusColor(v)}>{v || '—'}</Tag>,
      },
      { title: '开始时间', dataIndex: 'started_at', render: (v: string) => formatTime(v) },
      { title: '结束时间', dataIndex: 'finished_at', render: (v: string) => formatTime(v) },
      { title: '新增', dataIndex: 'inserted', width: 70 },
      { title: '更新', dataIndex: 'updated', width: 70 },
      { title: '跳过', dataIndex: 'skipped', width: 70 },
      { title: '失败', dataIndex: 'failed', width: 70 },
      {
        title: '错误摘要',
        dataIndex: 'error_summary',
        ellipsis: true,
        render: (v: string) => v || '—',
      },
    ],
    [],
  )

  if (!canView) {
    return (
      <PageContainer title="外部考勤同步中心" icon={<CloudSyncOutlined />} subtitle="从 Doris 只读接入钉钉考勤与部门关系">
        <PageCard>
          <Alert
            type="warning"
            showIcon
            message="无访问权限"
            description="你缺少考勤查看权限，需要联系管理员添加 menu:attendance 或 attendance_manage。"
          />
        </PageCard>
      </PageContainer>
    )
  }

  return (
    <PageContainer
      title="外部考勤同步中心"
      icon={<CloudSyncOutlined />}
      subtitle="从 Doris 只读接入钉钉考勤与部门关系，按当前组织隔离增量同步"
      extra={
        <Tag icon={<SafetyCertificateOutlined />} color="blue">
          当前组织：{status?.org_name || orgId || '—'}
        </Tag>
      }
    >
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <PageCard>
          <Space wrap>
            <Select value={source} style={{ width: 180 }} onChange={(v) => setSource(v)}>
              <Option value="all">考勤 + 部门关系</Option>
              <Option value="attendance">仅考勤</Option>
              <Option value="department">仅部门关系</Option>
            </Select>
            <Tooltip title={canManage ? undefined : missingManageTip}>
              <Button
                type="primary"
                icon={<SyncOutlined />}
                loading={runMutation.isPending}
                disabled={!canManage}
                onClick={confirmRun}
              >
                立即同步
              </Button>
            </Tooltip>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => {
                refetchStatus()
                refetchJobs()
              }}
            >
              刷新
            </Button>
          </Space>
        </PageCard>

        <Row gutter={[16, 16]}>
          <Col xs={24} lg={12}>
            <Card title="数据源状态">
              {statusLoading ? (
                <div style={{ textAlign: 'center', padding: 24 }}>
                  <Spin />
                </div>
              ) : statusError ? (
                <Alert
                  type="error"
                  showIcon
                  message="状态加载失败"
                  description={(statusErr as Error)?.message || '请稍后重试'}
                />
              ) : !status ? (
                <Empty description="暂无状态数据" />
              ) : (
                <Descriptions column={1} size="small">
                  <Descriptions.Item label="同步开关">
                    {status.enabled ? <Tag color="success">已启用</Tag> : <Tag>未启用</Tag>}
                  </Descriptions.Item>
                  <Descriptions.Item label="源健康">
                    {status.source_healthy ? (
                      <Tag color="success">
                        健康{status.source_latency_ms != null ? ` · ${status.source_latency_ms}ms` : ''}
                      </Tag>
                    ) : (
                      <Tag color="error">异常</Tag>
                    )}
                  </Descriptions.Item>
                  {status.source_error ? (
                    <Descriptions.Item label="源错误">{status.source_error}</Descriptions.Item>
                  ) : null}
                  <Descriptions.Item label="外部考勤最新更新">
                    {formatTime(status.external_last_attendance_update)}
                  </Descriptions.Item>
                  <Descriptions.Item label="外部部门最新更新">
                    {formatTime(status.external_last_department_update)}
                  </Descriptions.Item>
                </Descriptions>
              )}
            </Card>
          </Col>
          <Col xs={24} lg={12}>
            <Card title="游标 / 最近任务">
              {statusLoading ? (
                <div style={{ textAlign: 'center', padding: 24 }}>
                  <Spin />
                </div>
              ) : (
                <>
                  {(status?.cursors || []).length ? (
                    <Descriptions column={1} size="small" style={{ marginBottom: 12 }}>
                      {status!.cursors!.map((c) => (
                        <Descriptions.Item key={c.source_table} label={c.source_table}>
                          游标 {formatTime(c.cursor_time)} · 成功 {formatTime(c.last_success_at)}
                        </Descriptions.Item>
                      ))}
                    </Descriptions>
                  ) : (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无游标" />
                  )}
                  {status?.last_job ? (
                    <Alert
                      type={
                        status.last_job.status === 'success'
                          ? 'success'
                          : status.last_job.status === 'partial'
                            ? 'warning'
                            : 'info'
                      }
                      showIcon
                      message={`最近任务 #${status.last_job.id} · ${status.last_job.status}`}
                      description={`新增 ${status.last_job.inserted} / 更新 ${status.last_job.updated} / 跳过 ${status.last_job.skipped} / 失败 ${status.last_job.failed}${
                        status.last_job.error_summary ? ` · ${status.last_job.error_summary}` : ''
                      }`}
                    />
                  ) : null}
                </>
              )}
            </Card>
          </Col>
        </Row>

        <PageCard title="同步任务记录">
          {jobsLoading ? (
            <div style={{ textAlign: 'center', padding: 32 }}>
              <Spin />
            </div>
          ) : jobsError ? (
            <Alert
              type="error"
              showIcon
              message="任务列表加载失败"
              description={(jobsErr as Error)?.message || '请稍后重试'}
              action={
                <Button size="small" onClick={() => refetchJobs()}>
                  重试
                </Button>
              }
            />
          ) : jobs.length ? (
            <Table
              rowKey="id"
              columns={columns}
              dataSource={jobs}
              pagination={{
                current: page,
                pageSize,
                total,
                showSizeChanger: false,
                onChange: (p) => setPage(p),
                showTotal: (t) => `共 ${t} 条`,
              }}
              scroll={{ x: 1100 }}
            />
          ) : (
            <Empty description="暂无同步任务" />
          )}
        </PageCard>
      </Space>
    </PageContainer>
  )
}

export default AttendanceExternalSync
