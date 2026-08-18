import React from 'react'
import {
  Alert,
  Button,
  Card,
  Col,
  Row,
  Spin,
  Statistic,
  Tabs,
  Tooltip,
  Typography,
} from 'antd'
import {
  ApartmentOutlined,
  ProfileOutlined,
  SwapOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { orgAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'

const { Title, Text, Paragraph } = Typography

interface ScopeInfo {
  mode: string
  department_names?: string[]
}

interface OrgOverviewSummary {
  total_employees: number
  active_employees: number
  department_count: number
  probation_employee_count: number
  planned_regularization_count: number
}

interface OrgOverviewData {
  scope?: ScopeInfo | null
  summary: OrgOverviewSummary
}

const connectedEntries = [
  {
    key: 'employee-profile',
    title: '员工档案',
    description: '查询、新建和维护员工档案信息。',
    path: '/employee-profile',
    icon: <ProfileOutlined />,
  },
  {
    key: 'employee-flow',
    title: '入转调离',
    description: '查询和新建员工入职、调岗、离职记录。',
    path: '/employee-flow',
    icon: <SwapOutlined />,
  },
  {
    key: 'employees',
    title: '组织花名册',
    description: '查看组织同步后的员工信息。',
    path: '/employees',
    icon: <UserOutlined />,
  },
  {
    key: 'department-tree',
    title: '组织架构',
    description: '查看部门结构和成员分布。',
    path: '/department-tree',
    icon: <ApartmentOutlined />,
  },
]

const formatScopeLabel = (scope?: ScopeInfo | null) => {
  if (!scope) {
    return '当前范围：本地组织数据'
  }
  if (scope.mode === 'all') {
    return '当前范围：全组织'
  }
  if (scope.department_names?.length) {
    return `当前范围：${scope.department_names.join(' / ')}`
  }
  return '当前范围：部门范围'
}

const Organization: React.FC = () => {
  const navigate = useNavigate()

  const overviewQuery = useQuery({
    queryKey: ['organization-overview-entry'],
    queryFn: () => orgAPI.getOverview(),
  })

  const overview = overviewQuery.data?.data?.overview as OrgOverviewData | undefined

  return (
    <PageContainer
      title="人才管理驾驶舱"
      icon={<TeamOutlined />}
      subtitle={formatScopeLabel(overview?.scope)}
      extra={
        <Button onClick={() => void overviewQuery.refetch()} loading={overviewQuery.isFetching}>
          刷新
        </Button>
      }
    >
      <Tabs
        defaultActiveKey="dashboard"
        items={[
          {
            key: 'dashboard',
            label: '人才管理驾驶舱',
            children: overviewQuery.isLoading ? (
              <div style={{ display: 'flex', justifyContent: 'center', padding: 'var(--space-10)' }}>
                <Spin size="large" />
              </div>
            ) : overviewQuery.isError ? (
              <Alert
                type="error"
                showIcon
                message="组织概览加载失败"
                action={
                  <Button size="small" onClick={() => void overviewQuery.refetch()}>
                    重试
                  </Button>
                }
              />
            ) : (
              <>
                <Row gutter={[16, 16]}>
                  <Col xs={24} sm={12} xl={4}>
                    <PageCard>
                      <Tooltip title="当前组织内有效在职+停用员工的唯一人数（已排除软删除和管理员）">
                        <Statistic title="员工总数" value={overview?.summary.total_employees ?? 0} prefix={<UserOutlined />} suffix={<span style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-tertiary)' }}>含停用</span>} />
                      </Tooltip>
                    </PageCard>
                  </Col>
                  <Col xs={24} sm={12} xl={4}>
                    <PageCard>
                      <Statistic title="在职人数" value={overview?.summary.active_employees ?? 0} prefix={<TeamOutlined />} />
                    </PageCard>
                  </Col>
                  <Col xs={24} sm={12} xl={4}>
                    <PageCard>
                      <Statistic title="部门数" value={overview?.summary.department_count ?? 0} prefix={<ApartmentOutlined />} />
                    </PageCard>
                  </Col>
                  <Col xs={24} sm={12} xl={6}>
                    <PageCard>
                      <Statistic title="试用期人数" value={overview?.summary.probation_employee_count ?? 0} />
                    </PageCard>
                  </Col>
                  <Col xs={24} sm={12} xl={6}>
                    <PageCard>
                      <Statistic title="转正预警" value={overview?.summary.planned_regularization_count ?? 0} />
                    </PageCard>
                  </Col>
                </Row>

                <Row gutter={[16, 16]} style={{ marginTop: 'var(--space-2)' }}>
                  {connectedEntries.map((entry) => (
                    <Col xs={24} md={12} key={entry.key}>
                      <PageCard>
                        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 'var(--space-4)', alignItems: 'flex-start' }}>
                          <div>
                            <Title level={5} style={{ marginTop: 0, marginBottom: 'var(--space-2)' }}>
                              {entry.icon} <span style={{ marginLeft: 'var(--space-2)' }}>{entry.title}</span>
                            </Title>
                            <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                              {entry.description}
                            </Paragraph>
                          </div>
                          <Button type="primary" onClick={() => navigate(entry.path)}>
                            进入
                          </Button>
                        </div>
                      </PageCard>
                    </Col>
                  ))}
                </Row>
              </>
            ),
          },
        ]}
      />
    </PageContainer>
  )
}

export default Organization
