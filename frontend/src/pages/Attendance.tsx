import React, { useState } from 'react'
import { Card, Typography, DatePicker, Table, Spin, Empty, Alert, Button, Select, Space, Tag, message, Drawer } from 'antd'
import { ClockCircleOutlined, CalendarOutlined, SyncOutlined, ExportOutlined, ExclamationCircleOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { attendanceAPI, userAPI, departmentAPI } from '../services/api'
import dayjs from 'dayjs'

const { Title, Text } = Typography
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
    { title: '姓名', dataIndex: 'user_name', key: 'user_name', render: (v: string) => <span style={{ fontWeight: 600, color: '#1e1b4b' }}>{v}</span> },
    { title: '员工ID', dataIndex: 'user_id', key: 'user_id', render: (v: string) => <span style={{ color: '#6b7280' }}>{v}</span> },
    { title: '打卡时间', dataIndex: 'check_time', key: 'check_time' },
    { title: '打卡类型', dataIndex: 'check_type', key: 'check_type' },
    { title: '打卡地点', dataIndex: 'location', key: 'location', render: (v: string) => v || '-' },
    {
      title: '状态', dataIndex: 'is_abnormal', key: 'is_abnormal',
      render: (isAbnormal: boolean, record: AttendanceRecord) => (
        isAbnormal ? (
          <Tag color="error" icon={<ExclamationCircleOutlined />} style={{ borderRadius: 6, fontWeight: 600, margin: 0 }}>
            {record.abnormal_type || '异常'}
          </Tag>
        ) : (
          <Tag color="success" style={{ borderRadius: 6, fontWeight: 600, margin: 0 }}>正常</Tag>
        )
      ),
    },
    {
      title: '操作', key: 'action',
      render: (_: any, record: AttendanceRecord) => (
        <a onClick={() => handleViewDetail(record)} style={{ fontWeight: 600, color: '#4338ca' }}>查看详情</a>
      ),
    },
  ]

  return (
    <div style={{ padding: '20px 28px', background: '#e4e8ee', minHeight: '100vh' }}>
      <div style={{ marginBottom: 20 }}>
        <h2 style={{ margin: '0 0 4px', fontSize: 22, fontWeight: 700, color: '#111827' }}>
          <ClockCircleOutlined style={{ marginRight: 10, color: '#4338ca' }} />
          考勤查询
        </h2>
        <Text style={{ color: '#6b7280', fontSize: 13.5 }}>
          查询员工打卡记录，同步钉钉考勤数据
        </Text>
      </div>

      <Card
        style={{ borderRadius: 14, border: '1px solid #e5e7eb', boxShadow: '0 2px 10px rgba(0,0,0,0.05)' }}
        styles={{ header: { background: '#fafbfc', borderBottom: '1px solid #f0f0f0' } }}
      >
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
            <Button type="primary" icon={<CalendarOutlined />} onClick={() => refetch()}
              style={{ borderRadius: 8, fontWeight: 600 }}>查询</Button>
            <Button icon={<SyncOutlined />} onClick={handleSync} loading={syncMutation.isPending}
              style={{ borderRadius: 8 }}>同步</Button>
            <Button icon={<ExportOutlined />} onClick={() => window.location.href = '/attendance-export'}
              style={{ borderRadius: 8 }}>导出</Button>
          </Space>
        </div>

        {lastSyncData && (
          <div style={{
            marginBottom: 16,
            padding: '10px 14px',
            background: '#f8f9fc',
            borderRadius: 8,
            border: '1px solid #eef0f5',
            color: '#6b7280',
            fontSize: 13,
          }}>
            最近同步时间: <span style={{ color: '#1e1b4b', fontWeight: 500 }}>{lastSyncData.data?.attendance?.last_sync_time || '暂无'}</span>
            {lastSyncData.data?.attendance?.record_count !== undefined && (
              <span style={{ marginLeft: 20 }}>同步记录数: <span style={{ color: '#1e1b4b', fontWeight: 500 }}>{lastSyncData.data.attendance.record_count}</span></span>
            )}
          </div>
        )}

        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '48px' }}>
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
              showTotal: (total: number) => <span style={{ color: '#6b7280' }}>共 {total} 条记录</span>,
              onChange: (newPage, newPageSize) => { setPage(newPage); setPageSize(newPageSize) },
            }}
          />
        ) : (
          <Empty description="暂无考勤数据" imageStyle={{ height: 80 }} />
        )}
      </Card>

      <Drawer
        title={<span style={{ fontWeight: 700, color: '#111827' }}>考勤详情</span>}
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
              <div key={item.label} style={{ display: 'flex', justifyContent: 'space-between', padding: '10px 0', borderBottom: '1px solid #f0f0f0' }}>
                <Text style={{ color: '#6b7280', fontWeight: 500 }}>{item.label}</Text>
                <Text style={{ color: '#1e1b4b', fontWeight: 600 }}>{item.value}</Text>
              </div>
            ))}
            {selectedRecord.extension && Object.keys(selectedRecord.extension).length > 0 && (
              <div>
                <Text style={{ color: '#6b7280', fontWeight: 500, display: 'block', marginBottom: 8 }}>扩展信息</Text>
                <pre style={{
                  background: '#f8f9fc',
                  borderRadius: 8,
                  padding: 14,
                  border: '1px solid #eef0f5',
                  fontSize: 12.5,
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
    </div>
  )
}

export default Attendance
