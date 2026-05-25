import React, { useEffect, useMemo, useState } from 'react'
import {
  Button,
  Col,
  Empty,
  Input,
  Row,
  Select,
  Space,
  Spin,
  Table,
  Typography,
  message,
} from 'antd'
import { ReloadOutlined, SearchOutlined, SyncOutlined, TeamOutlined, UserOutlined, WarningOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { departmentAPI, orgAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'

const { Text } = Typography
const { Search } = Input

interface Department {
  department_id: string
  name: string
  parent_id: string
}

interface EmployeeItem {
  id: number
  user_id: string
  name: string
  email: string
  mobile: string
  department_id: string
  position: string
  status: string
  created_at?: string
}

interface DistributionItem {
  key: string
  label: string
  count: number
}

interface OverviewSummary {
  active_employees: number
  probation_employee_count: number
  planned_regularization_count: number
}

interface ScopeInfo {
  mode: string
  department_names?: string[]
}

interface OverviewData {
  scope: ScopeInfo
  summary: OverviewSummary
  employee_type_distribution: DistributionItem[]
  job_level_distribution: DistributionItem[]
  job_family_distribution: DistributionItem[]
}

const emptySummary: OverviewSummary = {
  active_employees: 0,
  probation_employee_count: 0,
  planned_regularization_count: 0,
}

const renderDistributionItems = (items: DistributionItem[]) => {
  if (!items.length) {
    return <Empty description="暂无分布数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
  }

  return (
    <Row gutter={[10, 10]}>
      {items.map((item) => (
        <Col xs={12} md={8} key={item.key}>
          <div style={{
            background: 'var(--color-bg-container)',
            borderRadius: 'var(--radius-sm)',
            padding: '12px 14px',
            border: '1px solid var(--color-border-subtle)',
          }}>
            <Text style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-xs)', fontWeight: 'var(--font-weight-medium)', display: 'block', marginBottom: 4 }}>
              {item.label}
            </Text>
            <span style={{ fontSize: 'var(--font-size-xl)', fontWeight: 'var(--font-weight-bold)', color: 'var(--color-text-heading)' }}>{item.count}</span>
          </div>
        </Col>
      ))}
    </Row>
  )
}

const statConfig = [
  { key: 'active', title: '在职人数', icon: <UserOutlined />, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)' },
  { key: 'probation', title: '试用期人数', icon: <TeamOutlined />, color: '#0369a1', bg: '#e0f2fe' },
  { key: 'warning', title: '计划转正预警', icon: <WarningOutlined />, color: '#b45309', bg: '#fef3c7' },
] as const

const EmployeeList: React.FC = () => {
  const navigate = useNavigate()
  const [employees, setEmployees] = useState<EmployeeItem[]>([])
  const [departments, setDepartments] = useState<Department[]>([])
  const [overview, setOverview] = useState<OverviewData | null>(null)
  const [loading, setLoading] = useState(true)
  const [syncing, setSyncing] = useState(false)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [search, setSearch] = useState('')
  const [departmentID, setDepartmentID] = useState<string>()
  const [status, setStatus] = useState<string>()

  const departmentNameMap = useMemo(() => {
    const entries = departments.map((department) => [department.department_id, department.name])
    return Object.fromEntries(entries) as Record<string, string>
  }, [departments])

  const scopeLabel = useMemo(() => {
    if (!overview?.scope) {
      return '正在加载数据范围...'
    }
    if (overview.scope.mode === 'all') {
      return '全组织'
    }
    if (overview.scope.department_names?.length) {
      return overview.scope.department_names.join(' / ')
    }
    return '部门范围'
  }, [overview])

  const loadData = async (showLoading = true) => {
    if (showLoading) setLoading(true)
    try {
      const [departmentRes, overviewRes, employeeRes] = await Promise.all([
        departmentAPI.getDepartments(),
        orgAPI.getOverview(departmentID ? { department_id: departmentID } : undefined),
        orgAPI.getEmployees({
          page,
          page_size: pageSize,
          department_id: departmentID,
          search,
          status,
        }),
      ])
      setDepartments(departmentRes.data.departments || [])
      setOverview(overviewRes.data.overview || null)
      setEmployees(employeeRes.data.items || [])
      setTotal(employeeRes.data.total || 0)
    } catch {
      message.error('获取组织数据失败')
    } finally {
      if (showLoading) setLoading(false)
    }
  }

  useEffect(() => { void loadData() }, [page, pageSize, search, departmentID, status])

  const handleSync = async () => {
    setSyncing(true)
    try {
      await orgAPI.syncOrg()
      message.success('组织数据同步成功')
      await loadData(false)
    } catch {
      message.error('组织数据同步失败')
    } finally {
      setSyncing(false)
    }
  }

  const summaryValues = [
    overview?.summary.active_employees ?? emptySummary.active_employees,
    overview?.summary.probation_employee_count ?? emptySummary.probation_employee_count,
    overview?.summary.planned_regularization_count ?? emptySummary.planned_regularization_count,
  ]

  const columns = [
    {
      title: '员工', dataIndex: 'name', key: 'name',
      render: (_: string, record: EmployeeItem) => (
        <div>
          <a
            onClick={() => navigate(`/employees/${record.id}`)}
            style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-primary)', fontSize: 'var(--font-size-base)' }}
          >
            {record.name}
          </a>
          <div style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)', marginTop: 2 }}>{record.user_id}</div>
        </div>
      ),
    },
    {
      title: '部门', dataIndex: 'department_id', key: 'department_id',
      render: (value: string) => (
        <span style={{ color: '#374151' }}>{departmentNameMap[value] || value || '-'}</span>
      ),
    },
    {
      title: '岗位', dataIndex: 'position', key: 'position',
      render: (value: string) => <span style={{ color: '#374151' }}>{value || '-'}</span>,
    },
    {
      title: '联系方式', key: 'contact',
      render: (_: unknown, record: EmployeeItem) => (
        <div>
          <div style={{ color: '#374151' }}>{record.email || '-'}</div>
          <div style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)' }}>{record.mobile || '-'}</div>
        </div>
      ),
    },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (value: string) => (
        <StatusTag color={value === 'active' ? 'success' : 'default'}>
          {value === 'active' ? '在职' : value === 'inactive' ? '离职' : value}
        </StatusTag>
      ),
    },
  ]

  return (
    <PageContainer
      title="组织花名册"
      icon={<TeamOutlined />}
      subtitle={<>数据范围：<span style={{ color: 'var(--color-primary)', fontWeight: 'var(--font-weight-semibold)' }}>{scopeLabel}</span></>}
      extra={
        <Space>
          <Button icon={<ReloadOutlined />} onClick={() => void loadData()} loading={loading}>刷新</Button>
          <Button type="primary" icon={<SyncOutlined />} onClick={() => void handleSync()} loading={syncing}>同步组织数据</Button>
        </Space>
      }
    >
      {/* 统计卡片 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 'var(--space-5)' }}>
        {statConfig.map((item, idx) => (
          <Col xs={24} md={8} key={item.key}>
            <div style={{
              background: 'var(--color-bg-card)',
              borderRadius: 'var(--radius-xl)',
              padding: '20px 22px',
              boxShadow: 'var(--shadow-card)',
              border: '1px solid var(--color-border)',
              display: 'flex',
              alignItems: 'center',
              gap: 14,
            }}>
              <div style={{
                width: 48,
                height: 48,
                borderRadius: 'var(--radius-lg)',
                background: item.bg,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: 'var(--font-size-xl)',
                color: item.color,
                flexShrink: 0,
              }}>
                {item.icon}
              </div>
              <div>
                <Text style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-sm)', fontWeight: 'var(--font-weight-medium)' }}>{item.title}</Text>
                <div style={{ fontSize: 'var(--font-size-2xl)', fontWeight: 'var(--font-weight-bold)', color: 'var(--color-text-title)', lineHeight: 1.2, marginTop: 2 }}>
                  {summaryValues[idx]}
                </div>
              </div>
            </div>
          </Col>
        ))}
      </Row>

      {/* 分布卡片 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 'var(--space-5)' }}>
        {[
          { title: '员工类型分布', data: overview?.employee_type_distribution || [] },
          { title: '职级分布', data: overview?.job_level_distribution || [] },
          { title: '岗位序列分布', data: overview?.job_family_distribution || [] },
        ].map((section) => (
          <Col xs={24} lg={8} key={section.title}>
            <PageCard
              title={<span style={{ fontWeight: 'var(--font-weight-semibold)', fontSize: 'var(--font-size-base)', color: 'var(--color-text-heading)' }}>{section.title}</span>}
            >
              {renderDistributionItems(section.data)}
            </PageCard>
          </Col>
        ))}
      </Row>

      {/* 花名册表格 */}
      <PageCard title={<span style={{ fontWeight: 'var(--font-weight-bold)', fontSize: 'var(--font-size-md)', color: 'var(--color-text-title)' }}>花名册</span>}>
        <Space wrap style={{ marginBottom: 18 }}>
          <Search
            allowClear
            enterButton={<SearchOutlined />}
            placeholder="搜索姓名、工号、邮箱、手机号、岗位"
            onSearch={(value) => { setPage(1); setSearch(value.trim()) }}
            style={{ width: 320 }}
          />
          <Select
            allowClear
            placeholder="按部门筛选"
            style={{ width: 220 }}
            value={departmentID}
            onChange={(value) => { setPage(1); setDepartmentID(value) }}
            options={departments.map((d) => ({ label: d.name, value: d.department_id }))}
          />
          <Select
            allowClear
            placeholder="按状态筛选"
            style={{ width: 160 }}
            value={status}
            onChange={(value) => { setPage(1); setStatus(value) }}
            options={[
              { label: '在职', value: 'active' },
              { label: '离职/停用', value: 'inactive' },
            ]}
          />
        </Space>

        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}>
            <Spin size="large" />
          </div>
        ) : (
          <Table
            rowKey="id"
            columns={columns}
            dataSource={employees}
            pagination={{
              current: page,
              pageSize,
              total,
              showSizeChanger: false,
              showTotal: (value) => <span style={{ color: 'var(--color-text-secondary)' }}>共 {value} 人</span>,
              onChange: (nextPage, nextPageSize) => { setPage(nextPage); setPageSize(nextPageSize) },
            }}
          />
        )}
      </PageCard>
    </PageContainer>
  )
}

export default EmployeeList
