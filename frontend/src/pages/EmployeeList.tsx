import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Alert,
  Avatar,
  Button,
  Card,
  Col,
  Collapse,
  Empty,
  Input,
  Row,
  Select,
  Skeleton,
  Space,
  Segmented,
  Spin,
  Tag,
  Table,
  Tooltip,
  Typography,
  message,
} from 'antd'
import { CloseCircleOutlined, EyeOutlined, FilterOutlined, ReloadOutlined, SearchOutlined, SyncOutlined, TeamOutlined, UserOutlined, WarningOutlined } from '@ant-design/icons'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { departmentAPI, orgAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import { hasPermission } from '../utils/permission'
import { maskEmail, maskMobile } from '../utils/maskPii'
import { confirmOrgSync } from '../utils/orgSyncAction'
import './organization.css'

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
  avatar?: string
  status: string
  extension?: Record<string, any>
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

const renderDistributionItems = (items: DistributionItem[], loading?: boolean) => {
  if (loading) {
    return <Skeleton active paragraph={{ rows: 2 }} title={false} />
  }
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
  { key: 'active', title: '在职人数', icon: <UserOutlined />, color: 'var(--color-primary)', bg: 'var(--color-primary-bg)', filterType: undefined },
  { key: 'probation', title: '试用期人数', icon: <TeamOutlined />, color: 'var(--color-info)', bg: '#e0f2fe', filterType: 'probation' },
  { key: 'warning', title: '计划转正预警', icon: <WarningOutlined />, color: 'var(--color-warning-dark)', bg: '#fef3c7', filterType: 'regularization_warning' },
] as const

const getPositionDiagnosticText = (record: EmployeeItem) => {
  const diagnostic = record.extension?.dingtalk_position_sync
  if (!diagnostic) return '岗位信息尚未同步，请先同步组织数据。'
  return '钉钉未返回岗位信息，请重新同步组织数据；如仍未显示，请联系管理员。'
}

const FILTER_TYPE_LABELS: Record<string, string> = {
  probation: '试用期员工',
  regularization_warning: '计划转正预警',
}

const EmployeeList: React.FC = () => {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  const [employees, setEmployees] = useState<EmployeeItem[]>([])
  const [departments, setDepartments] = useState<Department[]>([])
  const [overview, setOverview] = useState<OverviewData | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [filtersOpen, setFiltersOpen] = useState(true)
  const [viewMode, setViewMode] = useState<'table' | 'cards'>('table')
  const [total, setTotal] = useState(0)
  const page = Math.max(1, Number(searchParams.get('page')) || 1)
  const pageSize = Math.max(1, Number(searchParams.get('page_size')) || 10)
  const search = searchParams.get('search')?.trim() || ''
  const departmentID = searchParams.get('department_id') || undefined
  const status = searchParams.get('status') || undefined
  const rawFilterType = searchParams.get('filter_type') || ''
  const filterType = FILTER_TYPE_LABELS[rawFilterType] ? rawFilterType : undefined
  const [searchInput, setSearchInput] = useState(search)
  const loadSeqRef = useRef(0)

  const departmentNameMap = useMemo(() => {
    const entries = departments.map((department) => [department.department_id, department.name])
    return Object.fromEntries(entries) as Record<string, string>
  }, [departments])

  const hasActiveFilters = Boolean(search || departmentID || status || filterType)

  const updateSearchParams = useCallback((updates: Record<string, string | number | undefined>, resetPage = false) => {
    const next = new URLSearchParams(searchParams)
    Object.entries(updates).forEach(([key, value]) => {
      const normalized = String(value ?? '').trim()
      if (normalized && !(key === 'page' && normalized === '1') && !(key === 'page_size' && normalized === '10')) {
        next.set(key, normalized)
      } else {
        next.delete(key)
      }
    })
    if (resetPage) next.delete('page')
    setSearchParams(next)
  }, [searchParams, setSearchParams])

  useEffect(() => {
    setSearchInput(search)
  }, [search])

  const scopeLabel = useMemo(() => {
    if (loading && !overview?.scope) {
      return '加载中…'
    }
    if (!overview?.scope) {
      return '—'
    }
    if (overview.scope.mode === 'all') {
      return '全组织'
    }
    if (overview.scope.department_names?.length) {
      return overview.scope.department_names.join(' / ')
    }
    return '部门范围'
  }, [overview, loading])

  const loadData = async (showLoading = true) => {
    const seq = ++loadSeqRef.current
    if (showLoading) setLoading(true)
    setLoadError(false)
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
          filter_type: filterType,
        }),
      ])
      // 忽略过期响应，避免筛选/翻页竞态覆盖新结果
      if (seq !== loadSeqRef.current) return
      setDepartments(departmentRes.data.departments || [])
      setOverview(overviewRes.data.overview || null)
      setEmployees(employeeRes.data.items || [])
      setTotal(employeeRes.data.total || 0)
    } catch {
      if (seq !== loadSeqRef.current) return
      setLoadError(true)
      message.error('获取组织数据失败')
    } finally {
      if (seq === loadSeqRef.current && showLoading) setLoading(false)
    }
  }

  useEffect(() => { void loadData() }, [page, pageSize, search, departmentID, status, filterType])

  const clearFilters = () => {
    setSearchParams(new URLSearchParams())
  }

  const canSyncOrg = hasPermission('attendance_manage')

  const handleSync = () => {
    if (!canSyncOrg) return
    confirmOrgSync({
      onStart: () => setSyncing(true),
      onSettled: () => setSyncing(false),
      onCompleted: async () => {
        await loadData(false)
      },
    })
  }

  const overviewLoading = loading && !overview
  const summaryValues: Array<number | string> = overviewLoading
    ? ['…', '…', '…']
    : [
      overview?.summary.active_employees ?? emptySummary.active_employees,
      overview?.summary.probation_employee_count ?? emptySummary.probation_employee_count,
      overview?.summary.planned_regularization_count ?? emptySummary.planned_regularization_count,
    ]

  const columns = [
    {
      title: '员工', dataIndex: 'name', key: 'name',
      render: (_: string, record: EmployeeItem) => (
        <Space size={10}>
          <Avatar size={34} src={record.avatar} icon={<UserOutlined />} />
          <div>
          <a
            href={`/employees/${record.id}`}
            onClick={(e) => {
              e.preventDefault()
              navigate(`/employees/${record.id}`)
            }}
            style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-primary)', fontSize: 'var(--font-size-base)' }}
          >
            {record.name}
          </a>
            <div style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)', marginTop: 2 }}>{record.user_id}</div>
          </div>
        </Space>
      ),
    },
    {
      title: '部门', dataIndex: 'department_id', key: 'department_id',
      render: (value: string) => (
        <span style={{ color: 'var(--color-text-primary)' }}>{departmentNameMap[value] || value || '-'}</span>
      ),
    },
    {
      title: '岗位', dataIndex: 'position', key: 'position',
      render: (value: string, record: EmployeeItem) => value ? (
        <span style={{ color: 'var(--color-text-primary)' }}>{value}</span>
      ) : (
        <Tooltip title={<span style={{ whiteSpace: 'pre-line' }}>{getPositionDiagnosticText(record)}</span>}>
          <Tag color="orange">未同步岗位</Tag>
        </Tooltip>
      ),
    },
    {
      title: '联系方式', key: 'contact',
      render: (_: unknown, record: EmployeeItem) => (
        <div>
          <div style={{ color: 'var(--color-text-primary)' }}>{maskEmail(record.email)}</div>
          <div style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)' }}>{maskMobile(record.mobile)}</div>
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
    {
      title: '操作', key: 'actions', fixed: 'right' as const, width: 112,
      render: (_: unknown, record: EmployeeItem) => (
        <Button type="link" icon={<EyeOutlined />} onClick={() => navigate(`/employees/${record.id}`)}>
          查看档案
        </Button>
      ),
    },
  ]

  const renderEmployeeCard = (record: EmployeeItem) => (
    <Card className="organization-employee-card" key={record.id} size="small">
      <div className="organization-employee-card-heading">
        <Space size={10}>
          <Avatar size={40} src={record.avatar} icon={<UserOutlined />} />
          <div>
            <Button type="link" className="organization-employee-card-name" onClick={() => navigate(`/employees/${record.id}`)}>
              {record.name}
            </Button>
            <Text className="organization-employee-card-id">{record.user_id}</Text>
          </div>
        </Space>
        <StatusTag color={record.status === 'active' ? 'success' : 'default'}>
          {record.status === 'active' ? '在职' : record.status === 'inactive' ? '离职' : record.status}
        </StatusTag>
      </div>
      <div className="organization-employee-card-details">
        <div><Text type="secondary">部门</Text><span>{departmentNameMap[record.department_id] || record.department_id || '未分配部门'}</span></div>
        <div><Text type="secondary">岗位</Text><span>{record.position || '未同步岗位'}</span></div>
        <div><Text type="secondary">联系方式</Text><span>{maskMobile(record.mobile) || maskEmail(record.email) || '未填写'}</span></div>
      </div>
      <Button type="link" icon={<EyeOutlined />} onClick={() => navigate(`/employees/${record.id}`)}>
        查看人事档案
      </Button>
    </Card>
  )

  return (
    <PageContainer
      className="organization-roster-page"
      title="员工中心"
      icon={<TeamOutlined />}
      subtitle={
        loading && !overview?.scope
          ? <span style={{ color: 'var(--color-text-secondary)' }}>数据范围加载中…</span>
          : <>组织范围：<span style={{ color: 'var(--color-primary)', fontWeight: 'var(--font-weight-semibold)' }}>{scopeLabel}</span>；点击员工可查看人事档案</>
      }
      extra={
        <Space>
          <Button icon={<ReloadOutlined />} onClick={() => void loadData()} loading={loading}>刷新</Button>
          <Tooltip title={canSyncOrg ? undefined : '你缺少 attendance_manage 权限，需要联系管理员添加'}>
            <span>
              <Button
                type="primary"
                icon={<SyncOutlined />}
                onClick={handleSync}
                loading={syncing}
                disabled={!canSyncOrg}
              >
                同步组织数据
              </Button>
            </span>
          </Tooltip>
        </Space>
      }
    >
      <section className="organization-roster-section" aria-labelledby="employee-overview-title">
        <div className="organization-section-heading">
          <div>
            <h2 id="employee-overview-title">员工概览</h2>
            <Text>快速查看当前组织的人员状态</Text>
          </div>
          <span className="organization-section-kicker">实时数据</span>
        </div>
      <Row gutter={[16, 16]} className="organization-metric-grid">
        {statConfig.map((item, idx) => {
          const isActive = item.filterType ? filterType === item.filterType : !filterType
          const isClickable = !!item.filterType
          const handleCardClick = () => {
            if (!isClickable) return
            updateSearchParams({ filter_type: filterType === item.filterType ? undefined : item.filterType }, true)
          }
          const handleCardKeyDown = (e: React.KeyboardEvent) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              handleCardClick()
            }
          }
          return (
            <Col xs={24} md={8} key={item.key}>
              <button
                type="button"
                className={`organization-metric-card${isActive ? ' organization-metric-card-active' : ''}`}
                aria-label={isClickable ? `筛选${item.title}` : undefined}
                aria-pressed={isClickable ? isActive : undefined}
                disabled={!isClickable}
                onClick={handleCardClick}
                onKeyDown={handleCardKeyDown}
                style={{
                  '--metric-accent': item.color,
                  '--metric-background': item.bg,
                } as React.CSSProperties}
              >
                <span className="organization-metric-icon" aria-hidden="true">
                  {item.icon}
                </span>
                <span className="organization-metric-copy">
                  <span className="organization-metric-title">{item.title}</span>
                  <span className="organization-metric-value">
                    {overviewLoading ? (
                      <Skeleton.Input active size="small" />
                    ) : (
                      summaryValues[idx]
                    )}
                  </span>
                </span>
              </button>
            </Col>
          )
        })}
      </Row>
      </section>

      <section className="organization-roster-section" aria-labelledby="employee-distribution-title">
        <div className="organization-section-heading">
          <div>
            <h2 id="employee-distribution-title">人员分布</h2>
            <Text>从员工类型、职级和岗位序列了解人员结构</Text>
          </div>
        </div>
      <Row gutter={[16, 16]} className="organization-distribution-grid">
        {[
          { title: '员工类型分布', data: overview?.employee_type_distribution || [] },
          { title: '职级分布', data: overview?.job_level_distribution || [] },
          { title: '岗位序列分布', data: overview?.job_family_distribution || [] },
        ].map((section) => (
          <Col xs={24} lg={8} key={section.title}>
            <PageCard title={section.title} className="organization-distribution-card">
              {renderDistributionItems(section.data, overviewLoading)}
            </PageCard>
          </Col>
        ))}
      </Row>
      </section>

      <section className="organization-roster-section" aria-labelledby="employee-roster-title">
        <div className="organization-section-heading">
          <div>
            <h2 id="employee-roster-title">花名册</h2>
            <Text>搜索、筛选并进入员工人事档案</Text>
          </div>
          <Text type="secondary">{loading ? '正在加载…' : `共 ${total} 人`}</Text>
        </div>
      <PageCard className="organization-roster-card" title="员工列表">
        <div className="organization-roster-toolbar">
          <div>
            <Text strong>人员名册</Text>
            <Text type="secondary">按条件筛选后，可切换表格或卡片视图</Text>
          </div>
          <Space wrap>
            <Button icon={<FilterOutlined />} onClick={() => setFiltersOpen((open) => !open)} aria-expanded={filtersOpen}>
              {filtersOpen ? '收起筛选' : '展开筛选'}
            </Button>
            <Segmented
              value={viewMode}
              onChange={(value) => setViewMode(value as 'table' | 'cards')}
              options={[{ label: '表格', value: 'table' }, { label: '卡片', value: 'cards' }]}
            />
          </Space>
        </div>

        {filtersOpen ? <div className="organization-filter-bar" aria-label="员工筛选条件">
        <Space wrap className="organization-filter-controls">
          <Search
            allowClear
            enterButton={<SearchOutlined />}
            placeholder="搜索姓名、工号、邮箱、手机号、岗位"
            value={searchInput}
            onChange={(event) => {
              const value = event.target.value
              setSearchInput(value)
              if (value === '' && search !== '') updateSearchParams({ search: undefined }, true)
            }}
            onSearch={(value) => updateSearchParams({ search: value.trim() }, true)}
            className="organization-search-input"
          />
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder="按部门筛选"
            className="organization-department-select"
            value={departmentID}
            onChange={(value) => updateSearchParams({ department_id: value }, true)}
            options={departments.map((d) => ({ label: d.name, value: d.department_id }))}
          />
          <Select
            allowClear
            placeholder="按状态筛选"
            className="organization-status-select"
            value={status}
            onChange={(value) => updateSearchParams({ status: value }, true)}
            options={[
              { label: '在职', value: 'active' },
              { label: '离职/停用', value: 'inactive' },
            ]}
          />
          {hasActiveFilters ? (
            <Button type="link" aria-label="清除筛选" onClick={clearFilters} icon={<CloseCircleOutlined />}>清除筛选</Button>
          ) : null}
        </Space>
        <div className="organization-filter-meta">
          <Text type="secondary">{loading ? '正在加载员工数据…' : `当前结果 ${total} 人`}</Text>
          {hasActiveFilters ? <Text type="secondary">已应用筛选</Text> : <Text type="secondary">支持按姓名、工号、联系方式和岗位搜索</Text>}
        </div>
        </div> : (
          <div className="organization-filter-collapsed">
            {hasActiveFilters ? <Text type="secondary">已应用筛选条件，点击“展开筛选”可调整</Text> : <Text type="secondary">未设置筛选条件，当前显示全部可见员工</Text>}
          </div>
        )}

        {filterType && FILTER_TYPE_LABELS[filterType] ? (
          <div className="organization-roster-summary" style={{ marginTop: 16 }}>
            <Tag
              color={filterType === 'probation' ? 'blue' : 'orange'}
              closable
              closeIcon={<CloseCircleOutlined aria-label="关闭业务筛选" />}
              onClose={() => updateSearchParams({ filter_type: undefined }, true)}
            >
              {FILTER_TYPE_LABELS[filterType]}
            </Tag>
            <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>
              当前筛选：{FILTER_TYPE_LABELS[filterType]}
              {!overviewLoading ? `（${total} 人）` : ''}
            </Text>
          </div>
        ) : null}

        {loadError ? (
          <Alert
            type="error"
            showIcon
            message="花名册数据加载失败"
            action={<Button size="small" onClick={() => void loadData()}>重试</Button>}
            style={{ marginBottom: 16 }}
          />
        ) : null}

        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}>
            <Spin size="large" />
          </div>
        ) : viewMode === 'cards' ? (
          employees.length > 0 ? (
            <div className="organization-employee-card-grid">
              {employees.map(renderEmployeeCard)}
            </div>
          ) : (
            <Empty description={hasActiveFilters ? '当前筛选条件下无员工' : '暂无员工数据'}>
              {hasActiveFilters ? <Button type="link" onClick={clearFilters}>清除筛选</Button> : null}
            </Empty>
          )
        ) : (
          <Table
            rowKey="id"
            columns={columns}
            dataSource={employees}
            scroll={{ x: 760 }}
            pagination={{
              current: page,
              pageSize,
              total,
              showSizeChanger: true,
              pageSizeOptions: ['10', '20', '50'],
              showTotal: (value) => <span style={{ color: 'var(--color-text-secondary)' }}>共 {value} 人</span>,
              onChange: (nextPage, nextPageSize) => updateSearchParams({ page: nextPage, page_size: nextPageSize }),
            }}
            locale={{
              emptyText: (
                <Empty
                  description={
                    loadError
                      ? '加载失败'
                      : hasActiveFilters
                        ? '当前筛选条件下无员工'
                        : '暂无员工数据'
                  }
                >
                  {hasActiveFilters && !loadError ? (
                    <Button type="link" onClick={clearFilters}>清除筛选</Button>
                  ) : null}
                </Empty>
              ),
            }}
          />
        )}
      </PageCard>
      </section>
    </PageContainer>
  )
}

export default EmployeeList
