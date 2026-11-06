import React from 'react'
import { Typography, Alert, Button, Result, Skeleton, Empty } from 'antd'
import {
  AppstoreOutlined,
  ArrowRightOutlined,
  UserOutlined,
  TeamOutlined,
  ClockCircleOutlined,
  FileOutlined,
  DashboardOutlined,
  LockOutlined,
} from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import dayjs from 'dayjs'
import { attendanceAPI, approvalAPI, orgAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import { useAuthStore } from '../store/authStore'
import { hasMenuPermission } from '../utils/permission'
import './home.css'

const { Text } = Typography

const isForbiddenError = (error: unknown) => {
  const status = (error as { response?: { status?: number } })?.response?.status
  return status === 403
}

export const calculateExternalAttendanceRate = (summary?: { total: number; normal: number }) => {
  if (!summary || summary.total <= 0) return 0
  return Math.round((summary.normal / summary.total) * 10_000) / 100
}

const statCards = [
  {
    key: 'users',
    title: '员工总数',
    icon: <UserOutlined />,
    gradient: 'linear-gradient(135deg, #2563eb 0%, #0891b2 100%)',
    shadow: 'rgba(37,99,235,0.22)',
    description: '查看员工花名册和在职状态',
    path: '/employees',
    menuKey: 'menu:employees',
  },
  {
    key: 'departments',
    title: '部门总数',
    icon: <TeamOutlined />,
    gradient: 'linear-gradient(135deg, #10b981 0%, #22c55e 100%)',
    shadow: 'rgba(16,185,129,0.2)',
    description: '浏览组织层级和部门人数',
    path: '/department-tree',
    menuKey: 'menu:department-tree',
  },
  {
    key: 'attendance',
    title: '考勤率',
    icon: <ClockCircleOutlined />,
    gradient: 'linear-gradient(135deg, #f59e0b 0%, #fb7185 100%)',
    shadow: 'rgba(245,158,11,0.2)',
    description: '近 30 天同步考勤结果',
    path: '/attendance',
    menuKey: 'menu:attendance',
  },
  {
    key: 'approvals',
    title: '审批数量',
    icon: <FileOutlined />,
    gradient: 'linear-gradient(135deg, #8b5cf6 0%, #06b6d4 100%)',
    shadow: 'rgba(139,92,246,0.2)',
    description: '当前组织可见的审批实例',
    path: '/approval-instances',
    menuKey: 'menu:approval-instances',
  },
] as const

function formatStatValue(cardKey: string, value: number | string): string | number {
  // 权限不足 / 加载中：不与真实 0 混淆
  if (value === '--' || value === '—' || value === '…') return value === '…' ? '…' : '—'
  if (cardKey === 'attendance' && typeof value === 'number') {
    return `${value}%`
  }
  return value
}

const Home: React.FC = () => {
  const navigate = useNavigate()
  const { menuKeys, user } = useAuthStore()

  // 按菜单权限分别拉取，避免无权限接口 403 噪声，且不阻塞欢迎区/快捷入口
  const canLoadOverview = menuKeys.length > 0 && (
    hasMenuPermission('menu:employees') || hasMenuPermission('menu:department-tree') || hasMenuPermission('menu:organization-dashboard')
  )
  const canLoadAttendance = menuKeys.length > 0 && hasMenuPermission('menu:attendance')
  const canLoadApprovals = menuKeys.length > 0 && hasMenuPermission('menu:approval-instances')

  const { data: overviewData, isLoading: overviewLoading, isError: overviewError, error: overviewQueryError, refetch: refetchOverview } = useQuery({
    queryKey: ['orgOverview', 'home'],
    queryFn: () => orgAPI.getOverview(),
    enabled: canLoadOverview,
  })

  const { data: attendanceData, isLoading: attendanceLoading, isError: attendanceError, error: attendanceQueryError, refetch: refetchAttendance } = useQuery({
    queryKey: ['externalAttendanceSummary', 'home'],
    queryFn: () => attendanceAPI.externalSync.getDailyResults({
      page: 1,
      page_size: 1,
      start_date: dayjs().subtract(30, 'day').format('YYYY-MM-DD'),
      end_date: dayjs().format('YYYY-MM-DD'),
    }),
    enabled: canLoadAttendance,
  })

  const { data: approvalsData, isLoading: approvalsLoading, isError: approvalsError, error: approvalsQueryError, refetch: refetchApprovals } = useQuery({
    queryKey: ['approvals'],
    queryFn: () => approvalAPI.getInstances({ page: 1, page_size: 1 }),
    enabled: canLoadApprovals,
  })

  if (menuKeys.length === 0) {
    return (
      <PageContainer title="工作台" subtitle="人事管理系统" className="home-page">
        <section className="home-hero home-hero-empty" aria-labelledby="home-empty-title">
          <div className="home-hero-icon" aria-hidden="true"><DashboardOutlined /></div>
          <div>
            <div className="home-hero-eyebrow">欢迎使用</div>
            <h1 id="home-empty-title">系统概览</h1>
            <Text>配置角色权限后，即可查看组织、考勤和审批工作台。</Text>
          </div>
        </section>
        <Result
          icon={<LockOutlined style={{ color: 'var(--color-primary)' }} />}
          title="暂无数据权限"
          subTitle="您尚未被分配任何角色，请联系管理员配置权限后再使用系统功能。"
        />
      </PageContainer>
    )
  }

  const queryErrors = [
    overviewQueryError,
    attendanceQueryError,
    approvalsQueryError,
  ].filter(Boolean)
  const hasPermissionLimitedError = queryErrors.some(isForbiddenError)
  const hasBlockingError = queryErrors.some((error) => !isForbiddenError(error))
  const isError = overviewError || attendanceError || approvalsError

  const overviewSummary = overviewData?.data?.overview?.summary
  const userCount: number | string = !canLoadOverview
    ? '—'
    : overviewLoading
      ? '…'
      : overviewError && isForbiddenError(overviewQueryError)
        ? '--'
        : overviewError
          ? '—'
          : overviewSummary?.total_employees ?? 0
  const departmentCount: number | string = !canLoadOverview
    ? '—'
    : overviewLoading
      ? '…'
      : overviewError && isForbiddenError(overviewQueryError)
        ? '--'
        : overviewError
          ? '—'
          : overviewSummary?.department_count ?? 0
  const attendanceSummary = attendanceData?.data?.summary
  const attendanceRate: number | string = !canLoadAttendance
    ? '—'
    : attendanceLoading
      ? '…'
      : attendanceSummary && attendanceSummary.total > 0
        ? calculateExternalAttendanceRate(attendanceSummary)
        : attendanceError && isForbiddenError(attendanceQueryError)
          ? '--'
          : attendanceError
            ? '—'
            : 0
  const approvalCount: number | string = !canLoadApprovals
    ? '—'
    : approvalsLoading
      ? '…'
      : approvalsError && isForbiddenError(approvalsQueryError)
        ? '--'
        : approvalsError
          ? '—'
          : approvalsData?.data?.total ?? 0

  const values: Record<string, number | string> = {
    users: userCount,
    departments: departmentCount,
    attendance: attendanceRate,
    approvals: approvalCount,
  }

  const cardLoading: Record<string, boolean> = {
    users: canLoadOverview && overviewLoading,
    departments: canLoadOverview && overviewLoading,
    attendance: canLoadAttendance && attendanceLoading,
    approvals: canLoadApprovals && approvalsLoading,
  }

  // 仅展示当前账号已有菜单权限的入口，避免点进 RouteGuard 403
  const shortcuts = [
    { label: '组织架构', description: '查看部门层级和人员分布', icon: <TeamOutlined />, color: '#2563eb', bg: '#eaf2ff', path: '/department-tree', menuKey: 'menu:department-tree' },
    { label: '考勤管理', description: '查询打卡结果和异常记录', icon: <ClockCircleOutlined />, color: '#0369a1', bg: '#e0f2fe', path: '/attendance', menuKey: 'menu:attendance' },
    { label: '审批管理', description: '跟进审批实例和处理进度', icon: <FileOutlined />, color: '#b45309', bg: '#fef3c7', path: '/approval-instances', menuKey: 'menu:approval-instances' },
    { label: '绩效管理', description: '进入绩效活动和结果管理', icon: <DashboardOutlined />, color: '#15803d', bg: '#dcfce7', path: '/performance-overview', menuKey: 'menu:performance-overview' },
  ].filter((item) => hasMenuPermission(item.menuKey))

  const displayName = (user?.name || '').trim() || '同事'

  const handleRetryStats = () => {
    if (canLoadOverview && overviewError && !isForbiddenError(overviewQueryError)) void refetchOverview()
    if (canLoadAttendance && attendanceError && !isForbiddenError(attendanceQueryError)) void refetchAttendance()
    if (canLoadApprovals && approvalsError && !isForbiddenError(approvalsQueryError)) void refetchApprovals()
  }

  return (
    <PageContainer title="工作台" subtitle="聚合组织、人事、考勤与审批工作的日常入口" className="home-page">
      {hasPermissionLimitedError ? (
        <Alert
          message="部分首页数据未展示"
          description="当前账号无权查看部分统计数据，如有需要请联系管理员开通权限。"
          type="warning"
          showIcon
          className="home-alert"
        />
      ) : null}
      {isError && hasBlockingError ? (
        <Alert
          message="部分首页数据加载失败"
          description="请检查网络后重试相关统计，不影响下方快捷入口使用。"
          type="error"
          showIcon
          className="home-alert"
          action={
            <Button size="small" onClick={handleRetryStats}>
              重试
            </Button>
          }
        />
      ) : null}

      <section className="home-hero" aria-labelledby="home-welcome-title">
        <div className="home-hero-content">
          <div className="home-hero-icon" aria-hidden="true"><DashboardOutlined /></div>
          <div>
            <div className="home-hero-eyebrow">人事管理工作台</div>
            <h1 id="home-welcome-title">你好，{displayName}</h1>
            <Text>从核心指标和常用入口开始今天的工作</Text>
          </div>
        </div>
        {shortcuts[0] ? (
          <Button
            type="primary"
            onClick={() => navigate(shortcuts[0].path)}
            icon={<ArrowRightOutlined />}
            iconPosition="end"
            className="home-hero-action"
          >
            进入{shortcuts[0].label}
          </Button>
        ) : null}
      </section>

      <section className="home-section" aria-labelledby="home-metrics-title">
        <div className="home-section-heading">
          <div>
            <h2 id="home-metrics-title">核心指标</h2>
            <Text>快速了解当前组织的关键数据</Text>
          </div>
          <span className="home-section-kicker">实时概览</span>
        </div>
        <div className="home-metrics-grid">
        {statCards.map((card) => {
          const canOpen = hasMenuPermission(card.menuKey)
          return (
              <button
                type="button"
                className={`home-metric-card${canOpen ? '' : ' home-metric-card-disabled'}`}
                aria-label={canOpen ? `查看${card.title}` : `${card.title}无权限`}
                disabled={!canOpen}
                onClick={() => {
                  if (canOpen) navigate(card.path)
                }}
              >
                <span className="home-metric-icon" style={{ background: card.gradient, boxShadow: `0 4px 12px ${card.shadow}` }} aria-hidden="true">
                  {card.icon}
                </span>
                <span className="home-metric-content">
                  <span className="home-metric-title">{card.title}</span>
                  <span className="home-metric-value">
                    {cardLoading[card.key] ? (
                      <Skeleton.Input active size="small" />
                    ) : (
                      formatStatValue(card.key, values[card.key])
                    )}
                  </span>
                  <span className="home-metric-description">{card.description}</span>
                </span>
                {canOpen ? <ArrowRightOutlined className="home-metric-arrow" aria-hidden="true" /> : null}
              </button>
          )
        })}
        </div>
      </section>

      <div className="home-lower-grid">
        <section className="home-panel home-section" aria-labelledby="home-shortcuts-title">
          <div className="home-section-heading">
            <div>
              <h2 id="home-shortcuts-title">常用入口</h2>
              <Text>按当前账号权限展示可用工作区</Text>
            </div>
            <AppstoreOutlined className="home-section-icon" aria-hidden="true" />
          </div>
          {shortcuts.length > 0 ? (
            <div className="home-shortcuts-list">
              {shortcuts.map((item) => (
                <button type="button" className="home-shortcut" key={item.label} onClick={() => navigate(item.path)}>
                  <span className="home-shortcut-icon" style={{ color: item.color, background: item.bg }} aria-hidden="true">{item.icon}</span>
                  <span className="home-shortcut-copy">
                    <span className="home-shortcut-label">{item.label}</span>
                    <span className="home-shortcut-description">{item.description}</span>
                  </span>
                  <ArrowRightOutlined className="home-shortcut-arrow" aria-hidden="true" />
                </button>
              ))}
            </div>
          ) : (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无可用入口" />
          )}
        </section>

        <section className="home-panel home-section home-data-note" aria-labelledby="home-data-note-title">
          <div className="home-section-heading">
            <div>
              <h2 id="home-data-note-title">数据说明</h2>
              <Text>指标口径保持透明，便于快速判断数据范围</Text>
            </div>
          </div>
          <ul className="home-note-list">
            <li><strong>员工总数、部门总数</strong><span>来自当前组织概览数据。</span></li>
            <li><strong>考勤率</strong><span>按近 30 天同步考勤结果计算。</span></li>
            <li><strong>审批数量</strong><span>统计当前组织可见的审批实例。</span></li>
          </ul>
        </section>
      </div>
    </PageContainer>
  )
}

export default Home
