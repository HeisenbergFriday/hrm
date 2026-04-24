import React, { useState } from 'react'
import { Card, Typography, DatePicker, Table, Spin, Empty, Alert, Button, Select, Space, Tag, message, Drawer } from 'antd'
import { ClockCircleOutlined, CalendarOutlined, SyncOutlined, ExportOutlined, ExclamationCircleOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { attendanceAPI, userAPI, departmentAPI } from '../services/api'
import dayjs from 'dayjs'

const { Title } = Typography
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

  // 获取员工列表
  const { data: usersData } = useQuery({
    queryKey: ['users'],
    queryFn: () => userAPI.getUsers({ page: 1, page_size: 100 }),
  })

  // 获取部门列表
  const { data: departmentsData } = useQuery({
    queryKey: ['departments'],
    queryFn: () => departmentAPI.getDepartments(),
  })

  const users = usersData?.data?.items || []
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
    { title: '姓名', dataIndex: 'user_name', key: 'user_name' },
    { title: '员工ID', dataIndex: 'user_id', key: 'user_id' },
    { title: '打卡时间', dataIndex: 'check_time', key: 'check_time' },
    { title: '打卡类型', dataIndex: 'check_type', key: 'check_type' },
    { title: '打卡地点', dataIndex: 'location', key: 'location' },
    {
      title: '状态',
      dataIndex: 'is_abnormal',
      key: 'is_abnormal',
      render: (isAbnormal: boolean, record: AttendanceRecord) => (
        isAbnormal ? (
          <Tag color="red" icon={<ExclamationCircleOutlined />}>
            {record.abnormal_type || '异常'}
          </Tag>
        ) : (
          <Tag color="green">正常</Tag>
        )
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: AttendanceRecord) => (
        <Button type="link" onClick={() => handleViewDetail(record)}>
          查看详情
        </Button>
      ),
    },
  ]

  return (
    <div>
      <Title level={4}>考勤查询</Title>
      <Card>
        <div style={{ marginBottom: 16, display: 'flex', gap: 16, alignItems: 'center', flexWrap: 'wrap' }}>
          <Select
            placeholder="选择员工"
            style={{ width: 150 }}
            allowClear
            onChange={setUser}
          >
            {users.map(user => (
              <Option key={user.user_id} value={user.user_id}>
                {user.name}
              </Option>
            ))}
          </Select>
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
            <Button type="primary" icon={<CalendarOutlined />} onClick={() => refetch()}>
              查询
            </Button>
            <Button icon={<SyncOutlined />} onClick={handleSync} loading={syncMutation.isPending}>
              同步
            </Button>
            <Button icon={<ExportOutlined />} onClick={() => window.location.href = '/attendance-export'}>
              导出
            </Button>
          </Space>
        </div>

        {lastSyncData && (
          <div style={{ marginBottom: 16, color: '#666', fontSize: 12 }}>
            最近同步时间: {lastSyncData.data?.attendance?.last_sync_time || '暂无'}
            {lastSyncData.data?.attendance?.record_count !== undefined && (
              <span style={{ marginLeft: 16 }}>同步记录数: {lastSyncData.data.attendance.record_count}</span>
            )}
          </div>
        )}

        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <div style={{ padding: '20px' }}>
            <Alert
              message="加载失败"
              description={(error as Error)?.message || '获取考勤记录失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()}>
                  重试
                </Button>
              }
            />
          </div>
        ) : attendanceData?.data?.items?.length ? (
          <Table
            columns={columns}
            dataSource={attendanceData.data.items}
            rowKey="id"
            pagination={{
              current: page,
              pageSize: pageSize,
              total: attendanceData.data.total,
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (total: number) => `共 ${total} 条记录`,
              onChange: (newPage, newPageSize) => {
                setPage(newPage)
                setPageSize(newPageSize)
              },
            }}
          />
        ) : (
          <Empty description="暂无考勤数据" />
        )}
      </Card>

      <Drawer
        title="考勤详情"
        placement="right"
        width={400}
        onClose={() => setDrawerVisible(false)}
        open={drawerVisible}
      >
        {selectedRecord && (
          <div>
            <p><strong>员工姓名:</strong> {selectedRecord.user_name}</p>
            <p><strong>员工ID:</strong> {selectedRecord.user_id}</p>
            <p><strong>打卡时间:</strong> {selectedRecord.check_time}</p>
            <p><strong>打卡类型:</strong> {selectedRecord.check_type}</p>
            <p><strong>打卡地点:</strong> {selectedRecord.location}</p>
            <p><strong>状态:</strong> {selectedRecord.is_abnormal ? selectedRecord.abnormal_type || '异常' : '正常'}</p>
            {selectedRecord.extension && Object.keys(selectedRecord.extension).length > 0 && (
              <>
                <p><strong>扩展信息:</strong></p>
                <pre>{JSON.stringify(selectedRecord.extension, null, 2)}</pre>
              </>
            )}
          </div>
        )}
      </Drawer>
    </div>
  )
}

export default Attendance
