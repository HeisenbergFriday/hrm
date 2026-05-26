import React, { useState } from 'react'
import { Typography, DatePicker, Table, Spin, Empty, Alert, Button, Select, Space, message, Row, Col, Statistic } from 'antd'
import { WarningOutlined, ClockCircleOutlined, UserOutlined, TeamOutlined, ExclamationCircleOutlined, SyncOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { attendanceAPI, departmentAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import dayjs from 'dayjs'

const { Title } = Typography
const { RangePicker } = DatePicker
const { Option } = Select

interface AbnormalUser {
  user_id: string
  user_name: string
  times: number
}

interface AbnormalDetail {
  type: string
  count: number
  users: AbnormalUser[]
}

interface DepartmentStat {
  department_id: string
  department_name: string
  total_users: number
  normal_count: number
  late_count: number
  leave_early_count: number
  absent_count: number
  normal_rate: string
}

interface StatsData {
  summary: {
    total_users: number
    normal_count: number
    late_count: number
    leave_early_count: number
    absent_count: number
    normal_rate: string
  }
  department_stats: DepartmentStat[]
  abnormal_details: AbnormalDetail[]
  start_date?: string
  end_date?: string
  department_id?: string
}

const AttendanceStats: React.FC = () => {
  const [department, setDepartment] = useState<string>('')
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null]>([null, null])
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [expandedRows, setExpandedRows] = useState<string[]>([])

  const queryParams = {
    start_date: dateRange[0]?.format('YYYY-MM-DD') || undefined,
    end_date: dateRange[1]?.format('YYYY-MM-DD') || undefined,
    department_id: department || undefined,
  }

  const { data: statsData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['attendance-stats', queryParams],
    queryFn: () => attendanceAPI.getStats(queryParams),
  })

  // 获取部门列表
  const { data: departmentsData } = useQuery({
    queryKey: ['departments'],
    queryFn: () => departmentAPI.getDepartments(),
  })

  const departments = departmentsData?.data?.departments || []

  const syncMutation = useMutation({
    mutationFn: (data?: { start_date?: string; end_date?: string }) => attendanceAPI.sync(data),
    onSuccess: () => {
      message.success('同步成功')
      refetch()
    },
    onError: () => {
      message.error('同步失败')
    },
  })

  const handleDateChange = (dates: [dayjs.Dayjs | null, dayjs.Dayjs | null] | null) => {
    setDateRange(dates || [null, null])
  }

  const handleSync = () => {
    syncMutation.mutate({
      start_date: dateRange[0]?.format('YYYY-MM-DD'),
      end_date: dateRange[1]?.format('YYYY-MM-DD'),
    })
  }

  const handleExpandRow = (type: string) => {
    if (expandedRows.includes(type)) {
      setExpandedRows(expandedRows.filter(t => t !== type))
    } else {
      setExpandedRows([...expandedRows, type])
    }
  }

  const abnormalColumns = [
    { title: '异常类型', dataIndex: 'type', key: 'type', render: (v: string) => {
      const map: Record<string, string> = { late: '迟到', leave_early: '早退', absent: '缺勤' }
      return map[v] || v
    } },
    {
      title: '人数',
      dataIndex: 'count',
      key: 'count',
      render: (count: number, record: AbnormalDetail) => (
        <StatusTag color={count > 0 ? 'error' : 'success'}>
          {count} 人
        </StatusTag>
      ),
    },
    {
      title: '详情',
      key: 'action',
      render: (_: any, record: AbnormalDetail) => (
        <Button
          type="link"
          onClick={() => handleExpandRow(record.type)}
        >
          {expandedRows.includes(record.type) ? '收起' : '查看明细'}
        </Button>
      ),
    },
  ]

  const expandedRowRender = (record: AbnormalDetail) => {
    const userColumns = [
      { title: '员工ID', dataIndex: 'user_id', key: 'user_id' },
      { title: '员工姓名', dataIndex: 'user_name', key: 'user_name' },
      { title: '异常次数', dataIndex: 'times', key: 'times' },
    ]

    return (
      <Table
        columns={userColumns}
        dataSource={record.users}
        rowKey="user_id"
        pagination={false}
        size="small"
      />
    )
  }

  const departmentColumns = [
    { title: '部门', dataIndex: 'department_name', key: 'department_name' },
    { title: '总人数', dataIndex: 'total_users', key: 'total_users' },
    {
      title: '正常',
      dataIndex: 'normal_count',
      key: 'normal_count',
      render: (count: number) => <StatusTag color="success">{count}</StatusTag>,
    },
    {
      title: '迟到',
      dataIndex: 'late_count',
      key: 'late_count',
      render: (count: number) => count > 0 ? <StatusTag color="warning">{count}</StatusTag> : <span>0</span>,
    },
    {
      title: '早退',
      dataIndex: 'leave_early_count',
      key: 'leave_early_count',
      render: (count: number) => count > 0 ? <StatusTag color="warning">{count}</StatusTag> : <span>0</span>,
    },
    {
      title: '缺勤',
      dataIndex: 'absent_count',
      key: 'absent_count',
      render: (count: number) => count > 0 ? <StatusTag color="error">{count}</StatusTag> : <span>0</span>,
    },
    {
      title: '正常率',
      dataIndex: 'normal_rate',
      key: 'normal_rate',
      render: (rate: string) => {
        const rateValue = parseFloat(rate)
        return (
          <span style={{ color: rateValue >= 90 ? 'var(--color-success)' : rateValue >= 80 ? 'var(--color-warning)' : 'var(--color-error)' }}>
            {rate}
          </span>
        )
      },
    },
  ]

  return (
    <PageContainer title="异常统计" icon={<WarningOutlined />} subtitle="考勤异常数据统计分析">
      <PageCard>
        <div style={{ marginBottom: 'var(--space-4)', display: 'flex', gap: 'var(--space-4)', alignItems: 'center', flexWrap: 'wrap' }}>
          <Select
            placeholder="选择部门"
            style={{ width: 150 }}
            allowClear
            onChange={setDepartment}
          >
            {departments.map(dept => (
              <Option key={dept.department_id} value={dept.department_id}>
                {dept.name}
              </Option>
            ))}
          </Select>
          <RangePicker onChange={handleDateChange} />
          <Space>
            <Button type="primary" icon={<ClockCircleOutlined />} onClick={() => refetch()}>
              查询
            </Button>
            <Button icon={<SyncOutlined />} onClick={handleSync} loading={syncMutation.isPending}>
              同步
            </Button>
          </Space>
        </div>

        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 'var(--space-10)' }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <div style={{ padding: 'var(--space-5)' }}>
            <Alert
              message="加载失败"
              description={(error as Error)?.message || '获取考勤统计失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()}>
                  重试
                </Button>
              }
            />
          </div>
        ) : statsData?.data ? (
          <>
            <Row gutter={16} style={{ marginBottom: 'var(--space-6)' }}>
              <Col span={4}>
                <Statistic
                  title="总人数"
                  value={statsData.data.summary?.total_users || 0}
                  prefix={<UserOutlined />}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="正常人数"
                  value={statsData.data.summary?.normal_count || 0}
                  valueStyle={{ color: 'var(--color-success)' }}
                  prefix={<TeamOutlined />}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="迟到人数"
                  value={statsData.data.summary?.late_count || 0}
                  valueStyle={{ color: 'var(--color-error)' }}
                  prefix={<WarningOutlined />}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="早退人数"
                  value={statsData.data.summary?.leave_early_count || 0}
                  valueStyle={{ color: 'var(--color-error)' }}
                  prefix={<WarningOutlined />}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="缺勤人数"
                  value={statsData.data.summary?.absent_count || 0}
                  valueStyle={{ color: 'var(--color-error)' }}
                  prefix={<ExclamationCircleOutlined />}
                />
              </Col>
              <Col span={4}>
                <Statistic
                  title="正常率"
                  value={statsData.data.summary?.normal_rate || '0%'}
                  valueStyle={{ color: parseFloat(statsData.data.summary?.normal_rate || '0') >= 90 ? 'var(--color-success)' : 'var(--color-error)' }}
                />
              </Col>
            </Row>

            <Title level={5}>异常明细</Title>
            <Table
              columns={abnormalColumns}
              dataSource={statsData.data.abnormal_details || []}
              rowKey="type"
              expandable={{
                expandedRowKeys: expandedRows,
                expandedRowRender,
              }}
              pagination={false}
              style={{ marginBottom: 'var(--space-6)' }}
            />

            <Title level={5}>部门统计</Title>
            <Table
              columns={departmentColumns}
              dataSource={statsData.data.department_stats || []}
              rowKey="department_id"
              pagination={{
                current: page,
                pageSize: pageSize,
                total: statsData.data.department_stats?.length || 0,
                onChange: (newPage, newPageSize) => {
                  setPage(newPage)
                  setPageSize(newPageSize)
                },
              }}
            />
          </>
        ) : (
          <Empty description="暂无统计数据" />
        )}
      </PageCard>
    </PageContainer>
  )
}

export default AttendanceStats
