import React from 'react'
import { Row, Col, Typography, Spin, Alert, Button, Result } from 'antd'
import { UserOutlined, TeamOutlined, ClockCircleOutlined, FileOutlined, DashboardOutlined, LockOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { attendanceAPI, approvalAPI, orgAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import { useAuthStore } from '../store/authStore'

const { Text } = Typography

const isForbiddenError = (error: unknown) => {
  const status = (error as { response?: { status?: number } })?.response?.status
  return status === 403
}

const statCards = [
  { key: 'users', title: '员工总数', icon: <UserOutlined />, gradient: 'linear-gradient(135deg, #2563eb 0%, #0891b2 100%)', shadow: 'rgba(37,99,235,0.22)' },
  { key: 'departments', title: '部门总数', icon: <TeamOutlined />, gradient: 'linear-gradient(135deg, #10b981 0%, #22c55e 100%)', shadow: 'rgba(16,185,129,0.2)' },
  { key: 'attendance', title: '考勤率', icon: <ClockCircleOutlined />, gradient: 'linear-gradient(135deg, #f59e0b 0%, #fb7185 100%)', shadow: 'rgba(245,158,11,0.2)' },
  { key: 'approvals', title: '审批数量', icon: <FileOutlined />, gradient: 'linear-gradient(135deg, #8b5cf6 0%, #06b6d4 100%)', shadow: 'rgba(139,92,246,0.2)' },
] as const

const Home: React.FC = () => {
  const navigate = useNavigate()
  const { menuKeys } = useAuthStore()

  const { data: overviewData, isLoading: overviewLoading, isError: overviewError, error: overviewQueryError } = useQuery({
    queryKey: ['orgOverview', 'home'],
    queryFn: () => orgAPI.getOverview(),
    enabled: menuKeys.length > 0
  })

  const { data: attendanceData, isLoading: attendanceLoading, isError: attendanceError, error: attendanceQueryError } = useQuery({
    queryKey: ['attendanceStats'],
    queryFn: () => attendanceAPI.getStats({}),
    enabled: menuKeys.length > 0
  })

  const { data: approvalsData, isLoading: approvalsLoading, isError: approvalsError, error: approvalsQueryError } = useQuery({
    queryKey: ['approvals'],
    queryFn: () => approvalAPI.getInstances({ page: 1, page_size: 1 }),
    enabled: menuKeys.length > 0
  })

  // 未分配角色的用户不显示任何数据
  if (menuKeys.length === 0) {
    return (
      <PageContainer>
        <div style={{
          background: 'linear-gradient(135deg, #2563eb 0%, #0891b2 55%, #10b981 100%)',
          borderRadius: 'var(--radius-2xl)',
          padding: '28px 32px',
          marginBottom: 'var(--space-6)',
          color: '#fff',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          boxShadow: '0 16px 36px rgba(37,99,235,0.16)',
        }}>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 8 }}>
              <DashboardOutlined style={{ fontSize: 28 }} />
              <span style={{ margin: 0, color: '#fff', fontWeight: 'var(--font-weight-bold)', fontSize: 'var(--font-size-xl)' }}>系统概览</span>
            </div>
            <Text style={{ color: 'rgba(255,255,255,0.8)', fontSize: 'var(--font-size-base)' }}>
              欢迎使用人事管理系统
            </Text>
          </div>
          <div style={{
            width: 64,
            height: 64,
            borderRadius: 'var(--radius-2xl)',
            background: 'rgba(255,255,255,0.15)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            backdropFilter: 'blur(10px)',
          }}>
            <DashboardOutlined style={{ fontSize: 32, color: '#fff' }} />
          </div>
        </div>
        <Result
          icon={<LockOutlined style={{ color: 'var(--color-primary)' }} />}
          title="暂无数据权限"
          subTitle="您尚未被分配任何角色，请联系管理员配置权限后再使用系统功能。"
        />
      </PageContainer>
    )
  }

  const isLoading = overviewLoading || attendanceLoading || approvalsLoading
  const hasUnexpectedError =
    (overviewError && !isForbiddenError(overviewQueryError)) ||
    (attendanceError && !isForbiddenError(attendanceQueryError)) ||
    (approvalsError && !isForbiddenError(approvalsQueryError))

  if (isLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 400 }}>
        <Spin size="large" />
      </div>
    )
  }

  if (hasUnexpectedError) {
    return (
      <PageContainer>
        <Alert
          message="数据加载失败"
          description="请检查网络连接后重试"
          type="error"
          showIcon
          action={<Button size="small" onClick={() => window.location.reload()}>重试</Button>}
        />
      </PageContainer>
    )
  }

  const overviewSummary = overviewData?.data?.overview?.summary
  const userCount = overviewSummary?.total_employees || 0
  const departmentCount = overviewSummary?.department_count || 0
  const attendanceRate = attendanceData?.data?.summary?.normal_rate ? parseFloat(attendanceData.data.summary.normal_rate) : 0
  const approvalCount = approvalsData?.data?.total || 0

  const values: Record<string, number | string> = {
    users: userCount,
    departments: departmentCount,
    attendance: attendanceRate,
    approvals: approvalCount,
  }

  return (
    <PageContainer>
      {/* 欢迎区 */}
      <div style={{
        background: 'linear-gradient(135deg, #2563eb 0%, #0891b2 55%, #10b981 100%)',
        borderRadius: 'var(--radius-2xl)',
        padding: '28px 32px',
        marginBottom: 'var(--space-6)',
        color: '#fff',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        boxShadow: '0 16px 36px rgba(37,99,235,0.16)',
      }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 8 }}>
            <DashboardOutlined style={{ fontSize: 28 }} />
            <span style={{ margin: 0, color: '#fff', fontWeight: 'var(--font-weight-bold)', fontSize: 'var(--font-size-xl)' }}>系统概览</span>
          </div>
          <Text style={{ color: 'rgba(255,255,255,0.8)', fontSize: 'var(--font-size-base)' }}>
            欢迎使用人事管理系统，以下是当前系统核心数据概况
          </Text>
        </div>
        <div style={{
          width: 64,
          height: 64,
          borderRadius: 'var(--radius-2xl)',
          background: 'rgba(255,255,255,0.15)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          backdropFilter: 'blur(10px)',
        }}>
          <DashboardOutlined style={{ fontSize: 32, color: '#fff' }} />
        </div>
      </div>

      {/* 统计卡片 */}
      <Row gutter={[20, 20]}>
        {statCards.map((card) => (
          <Col xs={24} sm={12} lg={6} key={card.key}>
            <div style={{
              background: 'var(--color-bg-card)',
              borderRadius: 'var(--radius-xl)',
              padding: '22px 24px',
              boxShadow: '0 2px 12px rgba(0,0,0,0.06)',
              border: '1px solid var(--color-border)',
              display: 'flex',
              alignItems: 'center',
              gap: 16,
              transition: 'var(--transition-normal)',
              cursor: 'default',
            }}
              onMouseEnter={(e) => {
                e.currentTarget.style.boxShadow = `0 4px 20px ${card.shadow}`
                e.currentTarget.style.transform = 'translateY(-2px)'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.boxShadow = '0 2px 12px rgba(0,0,0,0.06)'
                e.currentTarget.style.transform = 'translateY(0)'
              }}
            >
              <div style={{
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
              }}>
                {card.icon}
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <Text style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-sm)', fontWeight: 'var(--font-weight-medium)' }}>{card.title}</Text>
                <div style={{
                  fontSize: 28,
                  fontWeight: 'var(--font-weight-bold)',
                  color: 'var(--color-text-title)',
                  lineHeight: 1.2,
                  marginTop: 4,
                }}>
                  {card.key === 'attendance'
                    ? `${values[card.key]}%`
                    : values[card.key]
                  }
                </div>
              </div>
            </div>
          </Col>
        ))}
      </Row>

      {/* 快捷入口 */}
      <div style={{ marginTop: 'var(--space-6)' }}>
        <span style={{ color: '#374151', fontWeight: 'var(--font-weight-bold)', marginBottom: 'var(--space-4)', display: 'block', fontSize: 'var(--font-size-md)' }}>快捷入口</span>
        <Row gutter={[16, 16]}>
          {[
            { label: '组织架构', icon: <TeamOutlined />, color: '#2563eb', bg: '#eaf2ff', path: '/department-tree' },
            { label: '考勤管理', icon: <ClockCircleOutlined />, color: '#0369a1', bg: '#e0f2fe', path: '/attendance' },
            { label: '审批管理', icon: <FileOutlined />, color: '#b45309', bg: '#fef3c7', path: '/approval-instances' },
            { label: '绩效管理', icon: <DashboardOutlined />, color: '#15803d', bg: '#dcfce7', path: '/performance-overview' },
          ].map((item) => (
            <Col xs={12} sm={6} key={item.label}>
              <div style={{
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
                onMouseEnter={(e) => {
                  e.currentTarget.style.boxShadow = `0 4px 16px ${item.color}22`
                  e.currentTarget.style.transform = 'translateY(-2px)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.boxShadow = 'var(--shadow-sm)'
                  e.currentTarget.style.transform = 'translateY(0)'
                }}
              >
                <div style={{
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
                }}>
                  {item.icon}
                </div>
                <div style={{ fontWeight: 'var(--font-weight-semibold)', fontSize: 'var(--font-size-base)', color: '#1f2937' }}>{item.label}</div>
              </div>
            </Col>
          ))}
        </Row>
      </div>
    </PageContainer>
  )
}

export default Home
