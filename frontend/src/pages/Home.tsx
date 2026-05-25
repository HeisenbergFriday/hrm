import React from 'react'
import { Row, Col, Typography, Spin, Alert, Button } from 'antd'
import { UserOutlined, TeamOutlined, ClockCircleOutlined, FileOutlined, DashboardOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { userAPI, departmentAPI, attendanceAPI, approvalAPI } from '../services/api'
import PageContainer from '../components/PageContainer'

const { Text } = Typography

const statCards = [
  { key: 'users', title: '员工总数', icon: <UserOutlined />, gradient: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)', shadow: 'rgba(102,126,234,0.35)' },
  { key: 'departments', title: '部门总数', icon: <TeamOutlined />, gradient: 'linear-gradient(135deg, #43e97b 0%, #38f9d7 100%)', shadow: 'rgba(67,233,123,0.3)' },
  { key: 'attendance', title: '考勤率', icon: <ClockCircleOutlined />, gradient: 'linear-gradient(135deg, #fa709a 0%, #fee140 100%)', shadow: 'rgba(250,112,154,0.3)' },
  { key: 'approvals', title: '审批数量', icon: <FileOutlined />, gradient: 'linear-gradient(135deg, #a18cd1 0%, #fbc2eb 100%)', shadow: 'rgba(161,140,209,0.3)' },
] as const

const Home: React.FC = () => {
  const navigate = useNavigate()
  const { data: usersData, isLoading: usersLoading, isError: usersError } = useQuery({
    queryKey: ['users'],
    queryFn: () => userAPI.getUsers({ page: 1, page_size: 1 })
  })

  const { data: departmentsData, isLoading: deptsLoading, isError: deptsError } = useQuery({
    queryKey: ['departments'],
    queryFn: departmentAPI.getDepartments
  })

  const { data: attendanceData, isLoading: attendanceLoading, isError: attendanceError } = useQuery({
    queryKey: ['attendanceStats'],
    queryFn: () => attendanceAPI.getStats({})
  })

  const { data: approvalsData, isLoading: approvalsLoading, isError: approvalsError } = useQuery({
    queryKey: ['approvals'],
    queryFn: () => approvalAPI.getInstances({ page: 1, page_size: 1 })
  })

  const isLoading = usersLoading || deptsLoading || attendanceLoading || approvalsLoading
  const isError = usersError || deptsError || attendanceError || approvalsError

  if (isLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 400 }}>
        <Spin size="large" />
      </div>
    )
  }

  if (isError) {
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

  const userCount = usersData?.data?.total || 0
  const departmentCount = departmentsData?.data?.departments?.length || 0
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
        background: 'linear-gradient(135deg, #4338ca 0%, #6366f1 50%, #818cf8 100%)',
        borderRadius: 'var(--radius-2xl)',
        padding: '28px 32px',
        marginBottom: 'var(--space-6)',
        color: '#fff',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        boxShadow: '0 4px 20px rgba(67,56,202,0.3)',
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
            { label: '组织架构', icon: <TeamOutlined />, color: '#4338ca', bg: '#eef2ff', path: '/department-tree' },
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
