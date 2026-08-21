import React, { useState, useEffect, useMemo, useRef } from 'react'
import { Typography, Table, Spin, Empty, Alert, Button, Select, DatePicker, Space, Input, Tooltip } from 'antd'
import { FileTextOutlined, SyncOutlined, SearchOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { approvalAPI, getPendingApprovalSyncRequestID, type ApprovalSyncAPIResponse } from '../services/api'
import { hasPermission } from '../utils/permission'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import ApprovalStatusTag from '../components/ApprovalStatusTag'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import datePickerZhCN from 'antd/es/date-picker/locale/zh_CN'
import { formatDateTime } from '../utils/format'
import {
  approvalSyncErrorNotice,
  approvalSyncResultNotice,
  approvalSyncRunningNotice,
  missingApprovalSyncPermissionTip,
  type ApprovalSyncNotice,
} from '../utils/approvalSync'

dayjs.locale('zh-cn')

const { Title, Text } = Typography
const { Option } = Select
const { RangePicker } = DatePicker

interface ApprovalInstance {
  id: string
  process_id: string
  template_id: string
  template_name: string
  title: string
  applicant_id: string
  applicant_name: string
  status: string
  create_time: string
  finish_time: string | null
  extension: any
}

const ApprovalInstance: React.FC = () => {
  const navigate = useNavigate()
  const [status, setStatus] = useState<string>('')
  const [templateID, setTemplateID] = useState<string>('')
  const [category, setCategory] = useState<string>('')
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null]>([null, null])
  const [searchText, setSearchText] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [syncNotice, setSyncNotice] = useState<ApprovalSyncNotice | null>(null)
  const syncInFlightRef = useRef(false)

  const CATEGORY_OPTIONS: { value: string; label: string }[] = [
    { value: 'leave', label: '请假' },
    { value: 'overtime', label: '加班' },
    { value: 'punch_fix', label: '补卡' },
    { value: 'expense', label: '报销' },
    { value: 'business_trip', label: '出差' },
    { value: 'outing', label: '外出' },
    { value: 'other', label: '其他' },
  ]

  // 防抖搜索：输入停顿 300ms 后触发查询，并把分页重置到第一页
  const [debouncedSearch, setDebouncedSearch] = useState('')
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedSearch(searchText.trim())
      setPage(1)
    }, 300)
    return () => window.clearTimeout(timer)
  }, [searchText])

  const queryParams = {
    page,
    page_size: pageSize,
    status: status || undefined,
    template_id: templateID || undefined,
    category: templateID ? undefined : (category || undefined),
    title: debouncedSearch || undefined,
    start_date: dateRange[0]?.format('YYYY-MM-DD') || undefined,
    end_date: dateRange[1]?.format('YYYY-MM-DD') || undefined,
  }

  const { data: instancesData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['approval-instances', queryParams],
    queryFn: () => approvalAPI.getInstances(queryParams),
  })

  const { data: templatesData } = useQuery({
    queryKey: ['approval-templates'],
    queryFn: () => approvalAPI.getTemplates(),
  })

  const templateNameByID = useMemo(() => new Map<string, string>(
    (templatesData?.data?.items || []).map((template: { template_id: string; name: string }) => [template.template_id, template.name]),
  ), [templatesData?.data?.items])

  const syncMutation = useMutation<ApprovalSyncAPIResponse, Error, boolean>({
    mutationFn: (resume) => resume
      ? approvalAPI.resumeSync()
      : approvalAPI.sync({
        process_code: templateID || undefined,
        start_date: dateRange[0]?.format('YYYY-MM-DD'),
        end_date: dateRange[1]?.format('YYYY-MM-DD'),
      }),
    onSuccess: (response) => {
      setSyncNotice(approvalSyncResultNotice(response.data))
      if (response.data.status === 'success' || response.data.status === 'partial') {
        refetch()
      }
    },
    onError: (error) => {
      setSyncNotice(approvalSyncErrorNotice(error))
    },
    onSettled: () => {
      syncInFlightRef.current = false
    },
  })

  const resumeSync = syncMutation.mutate
  useEffect(() => {
    if (!hasPermission('approval:sync') || !getPendingApprovalSyncRequestID() || syncInFlightRef.current) return
    syncInFlightRef.current = true
    setSyncNotice(approvalSyncRunningNotice())
    resumeSync(true)
  }, [resumeSync])

  const handleViewDetail = (id: string) => {
    navigate(`/approval-detail/${id}`)
  }

  const handleSync = () => {
    if (syncInFlightRef.current) {
      return
    }
    syncInFlightRef.current = true
    setSyncNotice(approvalSyncRunningNotice(templateID))
    syncMutation.mutate(false)
  }

  const columns = [
    {
      title: '审批标题',
      dataIndex: 'title',
      key: 'title',
      render: (text: string, record: ApprovalInstance) => (
        <Text strong onClick={() => handleViewDetail(record.id)} style={{ cursor: 'pointer', color: 'var(--color-primary)' }}>
          {text}
        </Text>
      ),
    },
    {
      title: '审批模板',
      dataIndex: 'template_name',
      key: 'template_name',
      render: (templateName: string, record: ApprovalInstance) => {
        const resolvedTemplateID = record.template_id || record.extension?.process_code || record.extension?.template_id
        return templateName || templateNameByID.get(resolvedTemplateID) || resolvedTemplateID || '—'
      },
    },
    {
      title: '发起人',
      dataIndex: 'applicant_name',
      key: 'applicant_name',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => <ApprovalStatusTag status={status} />,
    },
    {
      title: '发起时间',
      dataIndex: 'create_time',
      key: 'create_time',
      render: (v: string) => formatDateTime(v),
    },
    {
      title: '结束时间',
      dataIndex: 'finish_time',
      key: 'finish_time',
      render: (finishTime: string | null) => finishTime ? formatDateTime(finishTime) : '-',
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: ApprovalInstance) => (
        <Button
          type="link"
          onClick={() => handleViewDetail(record.id)}
        >
          查看详情
        </Button>
      ),
    },
  ]

  return (
    <PageContainer
      title="审批实例"
      icon={<FileTextOutlined />}
    >
      <PageCard>
        <div style={{ marginBottom: 'var(--space-4)', display: 'flex', gap: 'var(--space-4)', alignItems: 'center', flexWrap: 'wrap' }}>
          <Select
            placeholder="状态"
            style={{ width: 120 }}
            allowClear
            onChange={setStatus}
          >
            <Option value="completed">已完成</Option>
            <Option value="in_progress">处理中</Option>
            <Option value="rejected">已拒绝</Option>
            <Option value="pending">待处理</Option>
          </Select>
          <Select
            placeholder="流程分类"
            style={{ width: 140 }}
            allowClear
            value={category || undefined}
            onChange={(v) => { setCategory(v || ''); setPage(1) }}
            disabled={!!templateID}
          >
            {CATEGORY_OPTIONS.map((opt) => (
              <Option key={opt.value} value={opt.value}>{opt.label}</Option>
            ))}
          </Select>
          <Select
            placeholder="审批模板"
            style={{ width: 150 }}
            allowClear
            value={templateID || undefined}
            onChange={(v) => { setTemplateID(v || ''); setPage(1) }}
          >
            {templatesData?.data?.items?.map((template: any) => (
              <Option key={template.template_id} value={template.template_id}>
                {template.name}
              </Option>
            ))}
          </Select>
          <RangePicker
            onChange={setDateRange}
            placeholder={['开始日期', '结束日期']}
            format="YYYY-MM-DD"
            locale={datePickerZhCN}
          />
          <Input
            placeholder="搜索标题"
            style={{ width: 200 }}
            prefix={<SearchOutlined />}
            allowClear
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
          />
          <Space>
            <Button type="primary" onClick={() => refetch()}>
              查询
            </Button>
            <Tooltip title={hasPermission('approval:sync') ? undefined : missingApprovalSyncPermissionTip}>
              <span>
                <Button
                  icon={<SyncOutlined />}
                  onClick={handleSync}
                  loading={syncMutation.isPending}
                  disabled={!hasPermission('approval:sync') || syncMutation.isPending}
                >
                  {syncMutation.isPending ? '同步中' : (templateID ? '同步当前模板' : '同步全部')}
                </Button>
              </span>
            </Tooltip>
          </Space>
        </div>

        {syncNotice && (
          <Alert
            style={{ marginBottom: 'var(--space-4)' }}
            type={syncNotice.type}
            message={syncNotice.message}
            description={syncNotice.description}
            showIcon
          />
        )}

        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <div style={{ padding: 'var(--space-5)' }}>
            <Alert
              message="加载失败"
              description={(error as Error)?.message || '获取审批实例失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()}>
                  重试
                </Button>
              }
            />
          </div>
        ) : instancesData?.data?.items?.length ? (
          <Table
            columns={columns}
            dataSource={instancesData.data.items as ApprovalInstance[]}
            rowKey="id"
            pagination={{
              current: page,
              pageSize: pageSize,
              total: instancesData.data.total,
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
          <Empty description="暂无审批实例" />
        )}
      </PageCard>
    </PageContainer>
  )
}

export default ApprovalInstance
