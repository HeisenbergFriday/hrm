import React from 'react'
import { Row, Col, Typography, Alert, Button, Result, Skeleton } from 'antd'
import {
  UserOutlined,
  TeamOutlined,
  ClockCircleOutlined,
  FileOutlined,
  DashboardOutlined,
  LockOutlined,
} from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { attendanceAPI, approvalAPI, orgAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import { useAuthStore } from '../store/authStore'
import { hasMenuPermission } from '../utils/permission'

const { Text } = Typography

const isForbiddenError = (error: unknown) => {
  const status = (error as { response?: { status?: number } })?.response?.status
  return status === 403
}

const statCards = [
  {
    key: 'users',
    title: '员工总数',
    icon: <UserOutlined />,
    gradient: 'linear-gradient(135deg, #2563eb 0%, #0891b2 100%)',
    shadow: 'rgba(37,99,235,0.22)',
    path: '/employees',
    menuKey: 'menu:employees',
  },
  {
    key: 'departments',
    title: '部门总数',
    icon: <TeamOutlined />,
    gradient: 'linear-gradient(135deg, #10b981 0%, #22c55e 100%)',
    shadow: 'rgba(16,185,129,0.2)',
    path: '/department-tree',
    menuKey: 'menu:department-tree',
  },
  {
    key: 'attendance',
    title: '考勤率',
    icon: <ClockCircleOutlined />,
    gradient: 'linear-gradient(135deg, #f59e0b 0%, #fb7185 100%)',
    shadow: 'rgba(245,158,11,0.2)',
    path: '/attendance',
    menuKey: 'menu:attendance',
  },
  {
    key: 'approvals',
    title: '审批数量',
    icon: <FileOutlined />,
    gradient: 'linear-gradient(135deg, #8b5cf6 0%, #06b6d4 100%)',
    shadow: 'rgba(139,92,246,0.2)',
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
    queryKey: ['attendanceStats'],
    queryFn: () => attendanceAPI.getStats({}),
    enabled: canLoadAttendance,
  })

  const { data: approvalsData, isLoading: approvalsLoading, isError: approvalsError, error: approvalsQueryError, refetch: refetchApprovals } = useQuery({
    queryKey: ['approvals'],
    queryFn: () => approvalAPI.getInstances({ page: 1, page_size: 1 }),
    enabled: canLoadApprovals,
  })

  if (menuKeys.length === 0) {
    return (
      <PageContainer>
        <div style={{
          background: 'linear-gradient(135deg, #2563eb 0%, #0891b2 55%, #10b981 100%)',
          borderRadius: 'var(--radius-2xl)',
          padding: '20px 24px',
          marginBottom: 'var(--space-6)',
          color: '#fff',
          boxShadow: '0 16px 36px rgba(37,99,235,0.16)',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 6 }}>
            <DashboardOutlined style={{ fontSize: 22 }} />
            <span style={{ margin: 0, color: '#fff', fontWeight: 'var(--font-weight-bold)', fontSize: 'var(--font-size-lg)' }}>
              系统概览
            </span>
          </div>
          <Text style={{ color: 'rgba(255,255,255,0.85)', fontSize: 'var(--font-size-base)' }}>
            欢迎使用人事管理系统
          </Text>
        </div>
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
  const attendanceRate: number | string = !canLoadAttendance
    ? '—'
    : attendanceLoading
      ? '…'
      : attendanceData?.data?.summary?.normal_rate
        ? parseFloat(attendanceData.data.summary.normal_rate)
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
    { label: '组织架构', icon: <TeamOutlined />, color: '#2563eb', bg: '#eaf2ff', path: '/department-tree', menuKey: 'menu:department-tree' },
    { label: '考勤管理', icon: <ClockCircleOutlined />, color: '#0369a1', bg: '#e0f2fe', path: '/attendance', menuKey: 'menu:attendance' },
    { label: '审批管理', icon: <FileOutlined />, color: '#b45309', bg: '#fef3c7', path: '/approval-instances', menuKey: 'menu:approval-instances' },
    { label: '绩效管理', icon: <DashboardOutlined />, color: '#15803d', bg: '#dcfce7', path: '/performance-overview', menuKey: 'menu:performance-overview' },
  ].filter((item) => hasMenuPermission(item.menuKey))

  const displayName = (user?.name || '').trim() || '同事'

  const handleRetryStats = () => {
    if (canLoadOverview && overviewError && !isForbiddenError(overviewQueryError)) void refetchOverview()
    if (canLoadAttendance && attendanceError && !isForbiddenError(attendanceQueryError)) void refetchAttendance()
    if (canLoadApprovals && approvalsError && !isForbiddenError(approvalsQueryError)) void refetchApprovals()
  }

  return (
    <PageContainer>
      {hasPermissionLimitedError ? (
        <Alert
          message="部分首页数据未展示"
          description="当前账号已登录成功，但部分统计接口返回了 403，说明是权限不足，不是网络问题。"
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
        />
      ) : null}
      {isError && hasBlockingError ? (
        <Alert
          message="部分首页数据加载失败"
          description="请检查网络后重试相关统计，不影响下方快捷入口使用。"
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
          action={
            <Button size="small" onClick={handleRetryStats}>
              重试
            </Button>
          }
        />
      ) : null}

      {/* 欢迎区：单行信息 + CTA 风格，避免厚重展示块 */}
      <div style={{
        background: 'linear-gradient(135deg, #2563eb 0%, #0891b2 55%, #10b981 100%)',
        borderRadius: 'var(--radius-2xl)',
        padding: '18px 24px',
        marginBottom: 'var(--space-6)',
        color: '#fff',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 16,
        flexWrap: 'wrap',
        boxShadow: '0 16px 36px rgba(37,99,235,0.16)',
      }}>
        <div style={{ minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 4 }}>
            <DashboardOutlined style={{ fontSize: 22 }} />
            <span style={{
              margin: 0,
              color: '#fff',
              fontWeight: 'var(--font-weight-bold)',
              fontSize: 'var(--font-size-lg)',
            }}>
              你好，{displayName}
            </span>
          </div>
          <Text style={{ color: 'rgba(255,255,255,0.85)', fontSize: 'var(--font-size-sm)' }}>
            人事管理系统 · 从下方统计与快捷入口进入工作
          </Text>
        </div>
        {shortcuts[0] ? (
          <Button
            type="default"
            onClick={() => navigate(shortcuts[0].path)}
            style={{
              fontWeight: 600,
              border: 'none',
              color: 'var(--color-primary-active)',
              background: 'rgba(255,255,255,0.95)',
            }}
          >
            进入{shortcuts[0].label}
          </Button>
        ) : null}
      </div>

      <Row gutter={[20, 20]}>
        {statCards.map((card) => {
          const canOpen = hasMenuPermission(card.menuKey)
          return (
            <Col xs={24} sm={12} lg={6} key={card.key}>
              <div
                role={canOpen ? 'button' : undefined}
                tabIndex={canOpen ? 0 : undefined}
                aria-label={canOpen ? `查看${card.title}` : card.title}
                style={{
                  background: 'var(--color-bg-card)',
                  borderRadius: 'var(--radius-xl)',
                  padding: '22px 24px',
                  boxShadow: '0 2px 12px rgba(0,0,0,0.06)',
                  border: '1px solid var(--color-border)',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 16,
                  transition: 'var(--transition-normal)',
                  cursor: canOpen ? 'pointer' : 'default',
                }}
                onClick={() => {
                  if (canOpen) navigate(card.path)
                }}
                onKeyDown={(e) => {
                  if (!canOpen) return
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    navigate(card.path)
                  }
                }}
                onMouseEnter={(e) => {
                  if (!canOpen) return
                  e.currentTarget.style.boxShadow = `0 4px 20px ${card.shadow}`
                  e.currentTarget.style.transform = 'translateY(-2px)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.boxShadow = '0 2px 12px rgba(0,0,0,0.06)'
                  e.currentTarget.style.transform = 'translateY(0)'
                }}
              >
                <div
                  style={{
                    width: 52,
                    height: 52,
                    borderRadius: 'var(--radius-xl)',
                    background: card.gradient,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: 24,
                    color: '#fff',
                    flexShrink: 0,
                    boxShadow: `0 4px 12px ${card.shadow}`,
                  }}
                >
                  {card.icon}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <Text
                    style={{
                      color: 'var(--color-text-secondary)',
                      fontSize: 'var(--font-size-sm)',
                      fontWeight: 'var(--font-weight-medium)',
                    }}
                  >
                    {card.title}
                  </Text>
                  <div
                    style={{
                      fontSize: 28,
                      fontWeight: 'var(--font-weight-bold)',
                      color: 'var(--color-text-title)',
                      lineHeight: 1.2,
                      marginTop: 4,
                      minHeight: 34,
                    }}
                  >
                    {cardLoading[card.key] ? (
                      <Skeleton.Input active size="small" style={{ width: 72, height: 28 }} />
                    ) : (
                      formatStatValue(card.key, values[card.key])
                    )}
                  </div>
                </div>
              </div>
            </Col>
          )
        })}
      </Row>

      {shortcuts.length > 0 ? (
        <div style={{ marginTop: 'var(--space-6)' }}>
          <span
            style={{
              color: 'var(--color-text-heading)',
              fontWeight: 'var(--font-weight-bold)',
              marginBottom: 'var(--space-4)',
              display: 'block',
              fontSize: 'var(--font-size-md)',
            }}
          >
            快捷入口
          </span>
          <Row gutter={[16, 16]}>
            {shortcuts.map((item) => (
              <Col xs={12} sm={6} key={item.label}>
                <div
                  role="button"
                  tabIndex={0}
                  style={{
                    background: 'var(--color-bg-card)',
                    borderRadius: 'var(--radius-lg)',
                    padding: '20px 16px',
                    textAlign: 'center',
                    boxShadow: 'var(--shadow-sm)',
                    border: '1px solid var(--color-border-light)',
                    cursor: 'pointer',
                    transition: 'var(--transition-normal)',
                  }}
                  onClick={() => navigate(item.path)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      navigate(item.path)
                    }
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.boxShadow = `0 4px 16px ${item.color}22`
                    e.currentTarget.style.transform = 'translateY(-2px)'
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.boxShadow = 'var(--shadow-sm)'
                    e.currentTarget.style.transform = 'translateY(0)'
                  }}
                >
                  <div
                    style={{
                      width: 48,
                      height: 48,
                      borderRadius: 'var(--radius-lg)',
                      background: item.bg,
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontSize: 'var(--font-size-xl)',
                      color: item.color,
                      marginBottom: 10,
                    }}
                  >
                    {item.icon}
                  </div>
                  <div
                    style={{
                      fontWeight: 'var(--font-weight-semibold)',
                      fontSize: 'var(--font-size-base)',
                      color: 'var(--color-text-primary)',
                    }}
                  >
                    {item.label}
                  </div>
                </div>
              </Col>
            ))}
          </Row>
        </div>
      ) : null}
    </PageContainer>
  )
}

export default Home
