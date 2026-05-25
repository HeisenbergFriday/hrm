import React, { useState } from 'react'
import { Typography, DatePicker, Table, Spin, Empty, Alert, Button, Select, Space, Drawer, message } from 'antd'
import { ClockCircleOutlined, CalendarOutlined, SyncOutlined, ExportOutlined, ExclamationCircleOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { attendanceAPI, userAPI, departmentAPI } from '../services/api'
import dayjs from 'dayjs'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'

const { Text } = Typography
const { RangePicker } = DatePicker
const { Option } = Select

interface AttendanceRecord {
  id: string
  user_id: string
  user_name: string
  check_time: string
  check_type: string
  location: string
  is_abnormal: boolean
  abnormal_type?: string
  extension: Record<string, any>
}

const Attendance: React.FC = () => {
  const [user, setUser] = useState<string>('')
  const [department, setDepartment] = useState<string>('')
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null]>([null, null])
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [selectedRecord, setSelectedRecord] = useState<AttendanceRecord | null>(null)
  const [drawerVisible, setDrawerVisible] = useState(false)

  const queryParams = {
    page,
    page_size: pageSize,
    user_id: user || undefined,
    department_id: department || undefined,
    start_date: dateRange[0]?.format('YYYY-MM-DD') || undefined,
    end_date: dateRange[1]?.format('YYYY-MM-DD') || undefined,
  }

  const { data: attendanceData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['attendance-records', queryParams],
    queryFn: () => attendanceAPI.getRecords(queryParams),
  })

  const { data: lastSyncData } = useQuery({
    queryKey: ['attendance-last-sync'],
    queryFn: () => attendanceAPI.getLastSyncTime(),
  })

  const { data: usersData } = useQuery({
    queryKey: ['users'],
    queryFn: () => userAPI.getUsers({ page: 1, page_size: 100 }),
  })

  const { data: departmentsData } = useQuery({
    queryKey: ['departments'],
    queryFn: () => departmentAPI.getDepartments(),
  })

  const users = usersData?.data?.items || []
  const departments = departmentsData?.data?.departments || []

  const syncMutation = useMutation({
    mutationFn: (data?: { start_date?: string; end_date?: string }) => attendanceAPI.sync(data),
    onSuccess: () => { message.success('同步成功'); refetch() },
    onError: () => message.error('同步失败'),
  })

  const handleDateChange = (dates: [dayjs.Dayjs | null, dayjs.Dayjs | null] | null) => {
    setDateRange(dates || [null, null])
  }

  const handleViewDetail = (record: AttendanceRecord) => {
    setSelectedRecord(record)
    setDrawerVisible(true)
  }

  const handleSync = () => {
    syncMutation.mutate({
      start_date: dateRange[0]?.format('YYYY-MM-DD'),
      end_date: dateRange[1]?.format('YYYY-MM-DD'),
    })
  }

  const columns = [
    { title: '姓名', dataIndex: 'user_name', key: 'user_name', render: (v: string) => <span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text-heading)' }}>{v}</span> },
    { title: '员工ID', dataIndex: 'user_id', key: 'user_id', render: (v: string) => <span style={{ color: 'var(--color-text-secondary)' }}>{v}</span> },
    { title: '打卡时间', dataIndex: 'check_time', key: 'check_time' },
    { title: '打卡类型', dataIndex: 'check_type', key: 'check_type' },
    { title: '打卡地点', dataIndex: 'location', key: 'location', render: (v: string) => v || '-' },
    {
      title: '状态', dataIndex: 'is_abnormal', key: 'is_abnormal',
      render: (isAbnormal: boolean, record: AttendanceRecord) => (
        isAbnormal ? (
          <StatusTag color="error" icon={<ExclamationCircleOutlined />}>
            {record.abnormal_type || '异常'}
          </StatusTag>
        ) : (
          <StatusTag color="success">正常</StatusTag>
        )
      ),
    },
    {
      title: '操作', key: 'action',
      render: (_: any, record: AttendanceRecord) => (
        <a onClick={() => handleViewDetail(record)} style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-primary)' }}>查看详情</a>
      ),
    },
  ]

  return (
    <PageContainer title="考勤查询" icon={<ClockCircleOutlined />} subtitle="查询员工打卡记录，同步钉钉考勤数据">
      <PageCard>
        <div style={{ marginBottom: 18, display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <Select placeholder="选择员工" style={{ width: 150 }} allowClear onChange={setUser}>
            {users.map(u => (
              <Option key={u.user_id} value={u.user_id}>{u.name}</Option>
            ))}
          </Select>
          <Select placeholder="选择部门" style={{ width: 150 }} allowClear onChange={setDepartment}>
            {departments.map(dept => (
              <Option key={dept.department_id} value={dept.department_id}>{dept.name}</Option>
            ))}
          </Select>
          <RangePicker onChange={handleDateChange} />
          <Space>
            <Button type="primary" icon={<CalendarOutlined />} onClick={() => refetch()}>查询</Button>
            <Button icon={<SyncOutlined />} onClick={handleSync} loading={syncMutation.isPending}>同步</Button>
            <Button icon={<ExportOutlined />} onClick={() => window.location.href = '/attendance-export'}>导出</Button>
          </Space>
        </div>

        {lastSyncData && (
          <div style={{
            marginBottom: 'var(--space-4)',
            padding: '10px 14px',
            background: 'var(--color-bg-container)',
            borderRadius: 'var(--radius-md)',
            border: '1px solid var(--color-border-subtle)',
            color: 'var(--color-text-secondary)',
            fontSize: 'var(--font-size-sm)',
          }}>
            最近同步时间: <span style={{ color: 'var(--color-text-heading)', fontWeight: 'var(--font-weight-medium)' }}>{lastSyncData.data?.attendance?.last_sync_time || '暂无'}</span>
            {lastSyncData.data?.attendance?.record_count !== undefined && (
              <span style={{ marginLeft: 20 }}>同步记录数: <span style={{ color: 'var(--color-text-heading)', fontWeight: 'var(--font-weight-medium)' }}>{lastSyncData.data.attendance.record_count}</span></span>
            )}
          </div>
        )}

        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 'var(--space-10)' }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <Alert
            message="加载失败"
            description={(error as Error)?.message || '获取考勤记录失败，请稍后重试'}
            type="error"
            showIcon
            action={<Button size="small" onClick={() => refetch()}>重试</Button>}
          />
        ) : attendanceData?.data?.items?.length ? (
          <Table
            columns={columns}
            dataSource={attendanceData.data.items}
            rowKey="id"
            pagination={{
              current: page,
              pageSize,
              total: attendanceData.data.total,
              showSizeChanger: false,
              showTotal: (total: number) => <span style={{ color: 'var(--color-text-secondary)' }}>共 {total} 条记录</span>,
              onChange: (newPage, newPageSize) => { setPage(newPage); setPageSize(newPageSize) },
            }}
          />
        ) : (
          <Empty description="暂无考勤数据" imageStyle={{ height: 80 }} />
        )}
      </PageCard>

      <Drawer
        title={<span style={{ fontWeight: 'var(--font-weight-bold)', color: 'var(--color-text-title)' }}>考勤详情</span>}
        placement="right"
        width={420}
        onClose={() => setDrawerVisible(false)}
        open={drawerVisible}
      >
        {selectedRecord && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            {[
              { label: '员工姓名', value: selectedRecord.user_name },
              { label: '员工ID', value: selectedRecord.user_id },
              { label: '打卡时间', value: selectedRecord.check_time },
              { label: '打卡类型', value: selectedRecord.check_type },
              { label: '打卡地点', value: selectedRecord.location || '-' },
              { label: '状态', value: selectedRecord.is_abnormal ? selectedRecord.abnormal_type || '异常' : '正常' },
            ].map((item) => (
              <div key={item.label} style={{ display: 'flex', justifyContent: 'space-between', padding: '10px 0', borderBottom: '1px solid var(--color-border-light)' }}>
                <Text style={{ color: 'var(--color-text-secondary)', fontWeight: 'var(--font-weight-medium)' }}>{item.label}</Text>
                <Text style={{ color: 'var(--color-text-heading)', fontWeight: 'var(--font-weight-semibold)' }}>{item.value}</Text>
              </div>
            ))}
            {selectedRecord.extension && Object.keys(selectedRecord.extension).length > 0 && (
              <div>
                <Text style={{ color: 'var(--color-text-secondary)', fontWeight: 'var(--font-weight-medium)', display: 'block', marginBottom: 8 }}>扩展信息</Text>
                <pre style={{
                  background: 'var(--color-bg-container)',
                  borderRadius: 'var(--radius-md)',
                  padding: 14,
                  border: '1px solid var(--color-border-subtle)',
                  fontSize: 'var(--font-size-xs)',
                  color: '#374151',
                  overflow: 'auto',
                  maxHeight: 300,
                }}>
                  {JSON.stringify(selectedRecord.extension, null, 2)}
                </pre>
              </div>
            )}
          </div>
        )}
      </Drawer>
    </PageContainer>
  )
}

export default Attendance
