import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, Space, Tag, message, DatePicker, Select, Modal } from 'antd'
import { DownloadOutlined, SyncOutlined, FileExcelOutlined, EyeOutlined, DeleteOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { attendanceAPI } from '../services/api'
import dayjs from 'dayjs'

const { Title } = Typography
const { RangePicker } = DatePicker
const { Option } = Select

interface ExportRecord {
  id: string
  user_id: string
  user_name: string
  file_name: string
  file_path: string
  record_count: number
  status: 'pending' | 'processing' | 'completed' | 'failed'
  error_msg?: string
  start_date: string
  end_date: string
  created_at: string
}

const AttendanceExport: React.FC = () => {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [exportModalVisible, setExportModalVisible] = useState(false)
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null]>([null, null])
  const [department, setDepartment] = useState<string>('')
  const [user, setUser] = useState<string>('')

  const { data: exportsData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['attendance-exports', page, pageSize],
    queryFn: () => attendanceAPI.getExports({ page, page_size: pageSize }),
  })

  const exportMutation = useMutation({
    mutationFn: (data: { start_date: string; end_date: string; department_id?: string; user_id?: string }) => attendanceAPI.export(data),
    onSuccess: () => {
      message.success('导出任务已创建')
      setExportModalVisible(false)
      refetch()
    },
    onError: () => {
      message.error('创建导出任务失败')
    },
  })

  const handleDateChange = (dates: [dayjs.Dayjs | null, dayjs.Dayjs | null] | null) => {
    setDateRange(dates || [null, null])
  }

  const handleExport = () => {
    if (!dateRange[0] || !dateRange[1]) {
      message.error('请选择导出日期范围')
      return
    }
    exportMutation.mutate({
      start_date: dateRange[0].format('YYYY-MM-DD'),
      end_date: dateRange[1].format('YYYY-MM-DD'),
      department_id: department || undefined,
      user_id: user || undefined,
    })
  }

  const handleDownload = (record: ExportRecord) => {
    if (record.status === 'completed' && record.file_path) {
      window.open(record.file_path, '_blank')
    } else {
      message.warning('文件暂不可用，请稍后再试')
    }
  }

  const handlePreview = (record: ExportRecord) => {
    Modal.info({
      title: '导出详情',
      content: (
        <div>
          <p><strong>文件名:</strong> {record.file_name}</p>
          <p><strong>导出记录数:</strong> {record.record_count}</p>
          <p><strong>日期范围:</strong> {record.start_date} 至 {record.end_date}</p>
          <p><strong>创建时间:</strong> {record.created_at}</p>
          {record.error_msg && (
            <p><strong>错误信息:</strong> {record.error_msg}</p>
          )}
        </div>
      ),
    })
  }

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'completed':
        return <Tag color="success" icon={<DownloadOutlined />}>已完成</Tag>
      case 'processing':
        return <Tag color="processing" icon={<SyncOutlined />}>处理中</Tag>
      case 'pending':
        return <Tag color="default" icon={<SyncOutlined />}>等待中</Tag>
      case 'failed':
        return <Tag color="error" icon={<DeleteOutlined />}>失败</Tag>
      default:
        return <Tag>{status}</Tag>
    }
  }

  const columns = [
    { title: '导出人', dataIndex: 'user_name', key: 'user_name' },
    { title: '文件名', dataIndex: 'file_name', key: 'file_name' },
    { title: '记录数', dataIndex: 'record_count', key: 'record_count' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => getStatusTag(status),
    },
    { title: '开始日期', dataIndex: 'start_date', key: 'start_date' },
    { title: '结束日期', dataIndex: 'end_date', key: 'end_date' },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at' },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: ExportRecord) => (
        <Space>
          <Button
            type="link"
            icon={<EyeOutlined />}
            onClick={() => handlePreview(record)}
          >
            详情
          </Button>
          <Button
            type="link"
            icon={<DownloadOutlined />}
            onClick={() => handleDownload(record)}
            disabled={record.status !== 'completed'}
          >
            下载
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Title level={4}>导出记录</Title>
      <Card
        extra={
          <Button
            type="primary"
            icon={<FileExcelOutlined />}
            onClick={() => setExportModalVisible(true)}
          >
            新建导出
          </Button>
        }
      >
        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <div style={{ padding: '20px' }}>
            <Alert
              message="加载失败"
              description={(error as Error)?.message || '获取导出记录失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()}>
                  重试
                </Button>
              }
            />
          </div>
        ) : exportsData?.data?.items?.length ? (
          <Table
            columns={columns}
            dataSource={exportsData.data.items}
            rowKey="id"
            pagination={{
              current: page,
              pageSize: pageSize,
              total: exportsData.data.total,
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
          <Empty description="暂无导出记录" />
        )}
      </Card>

      <Modal
        title="新建考勤导出"
        open={exportModalVisible}
        onCancel={() => setExportModalVisible(false)}
        onOk={handleExport}
        confirmLoading={exportMutation.isPending}
        okText="确定"
        cancelText="取消"
      >
        <div style={{ padding: '20px 0' }}>
          <div style={{ marginBottom: 16 }}>
            <label style={{ display: 'block', marginBottom: 8 }}>日期范围</label>
            <RangePicker
              style={{ width: '100%' }}
              onChange={handleDateChange}
            />
          </div>
          <div style={{ marginBottom: 16 }}>
            <label style={{ display: 'block', marginBottom: 8 }}>部门（可选）</label>
            <Select
              placeholder="选择部门"
              style={{ width: '100%' }}
              allowClear
              onChange={setDepartment}
            >
              <Option value="1">技术部</Option>
              <Option value="2">市场部</Option>
              <Option value="3">产品部</Option>
            </Select>
          </div>
          <div style={{ marginBottom: 16 }}>
            <label style={{ display: 'block', marginBottom: 8 }}>员工（可选）</label>
            <Select
              placeholder="选择员工"
              style={{ width: '100%' }}
              allowClear
              onChange={setUser}
            >
              <Option value="user123">张三</Option>
              <Option value="user456">李四</Option>
              <Option value="user789">王五</Option>
            </Select>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default AttendanceExport