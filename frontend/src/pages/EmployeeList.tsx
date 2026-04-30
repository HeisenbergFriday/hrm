import React, { useEffect, useMemo, useState } from 'react'
import {
  Button,
  Card,
  Col,
  Empty,
  Input,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import { ReloadOutlined, SearchOutlined, SyncOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { departmentAPI, orgAPI } from '../services/api'

const { Title, Text } = Typography
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
    <Row gutter={[12, 12]}>
      {items.map((item) => (
        <Col xs={12} md={8} key={item.key}>
          <Card size="small">
            <Statistic title={item.label} value={item.count} />
          </Card>
        </Col>
      ))}
    </Row>
  )
}

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
      return '当前范围：全组织'
    }
    if (overview.scope.department_names?.length) {
      return `当前范围：${overview.scope.department_names.join(' / ')}`
    }
    return '当前范围：部门范围'
  }, [overview])

  const loadData = async (showLoading = true) => {
    if (showLoading) {
      setLoading(true)
    }

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
    } catch (error) {
      message.error('获取组织数据失败')
    } finally {
      if (showLoading) {
        setLoading(false)
      }
    }
  }

  useEffect(() => {
    void loadData()
  }, [page, pageSize, search, departmentID, status])

  const handleSync = async () => {
    setSyncing(true)
    try {
      await orgAPI.syncOrg()
      message.success('组织数据同步成功')
      await loadData(false)
    } catch (error) {
      message.error('组织数据同步失败')
    } finally {
      setSyncing(false)
    }
  }

  const columns = [
    {
      title: '员工',
      dataIndex: 'name',
      key: 'name',
      render: (_: string, record: EmployeeItem) => (
        <div>
          <Button type="link" style={{ padding: 0 }} onClick={() => navigate(`/employees/${record.id}`)}>
            {record.name}
          </Button>
          <div style={{ color: '#8c8c8c', fontSize: 12 }}>{record.user_id}</div>
        </div>
      ),
    },
    {
      title: '部门',
      dataIndex: 'department_id',
      key: 'department_id',
      render: (value: string) => departmentNameMap[value] || value || '-',
    },
    {
      title: '岗位',
      dataIndex: 'position',
      key: 'position',
      render: (value: string) => value || '-',
    },
    {
      title: '联系方式',
      key: 'contact',
      render: (_: unknown, record: EmployeeItem) => (
        <div>
          <div>{record.email || '-'}</div>
          <div style={{ color: '#8c8c8c', fontSize: 12 }}>{record.mobile || '-'}</div>
        </div>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (value: string) => (
        <Tag color={value === 'active' ? 'green' : 'default'}>
          {value === 'active' ? '在职' : value === 'inactive' ? '离职/停用' : value}
        </Tag>
      ),
    },
  ]

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div>
          <Title level={4} style={{ marginBottom: 4 }}>
            组织花名册
          </Title>
          <Text type="secondary">{scopeLabel}</Text>
        </div>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={() => void loadData()} loading={loading}>
            刷新
          </Button>
          <Button type="primary" icon={<SyncOutlined />} onClick={() => void handleSync()} loading={syncing}>
            同步组织数据
          </Button>
        </Space>
      </div>

      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} md={8}>
          <Card>
            <Statistic title="在职人数" value={overview?.summary.active_employees ?? emptySummary.active_employees} />
          </Card>
        </Col>
        <Col xs={24} md={8}>
          <Card>
            <Statistic
              title="试用期人数"
              value={overview?.summary.probation_employee_count ?? emptySummary.probation_employee_count}
            />
          </Card>
        </Col>
        <Col xs={24} md={8}>
          <Card>
            <Statistic
              title="计划转正预警"
              value={overview?.summary.planned_regularization_count ?? emptySummary.planned_regularization_count}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} lg={8}>
          <Card title="员工类型分布">{renderDistributionItems(overview?.employee_type_distribution || [])}</Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card title="职级分布">{renderDistributionItems(overview?.job_level_distribution || [])}</Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card title="岗位序列分布">{renderDistributionItems(overview?.job_family_distribution || [])}</Card>
        </Col>
      </Row>

      <Card title="花名册">
        <Space wrap style={{ marginBottom: 16 }}>
          <Search
            allowClear
            enterButton={<SearchOutlined />}
            placeholder="搜索姓名、工号、邮箱、手机号、岗位"
            onSearch={(value) => {
              setPage(1)
              setSearch(value.trim())
            }}
            style={{ width: 320 }}
          />
          <Select
            allowClear
            placeholder="按部门筛选"
            style={{ width: 220 }}
            value={departmentID}
            onChange={(value) => {
              setPage(1)
              setDepartmentID(value)
            }}
            options={departments.map((department) => ({
              label: department.name,
              value: department.department_id,
            }))}
          />
          <Select
            allowClear
            placeholder="按状态筛选"
            style={{ width: 160 }}
            value={status}
            onChange={(value) => {
              setPage(1)
              setStatus(value)
            }}
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
              showSizeChanger: true,
              showTotal: (value) => `共 ${value} 人`,
              onChange: (nextPage, nextPageSize) => {
                setPage(nextPage)
                setPageSize(nextPageSize)
              },
            }}
          />
        )}
      </Card>
    </div>
  )
}

export default EmployeeList
