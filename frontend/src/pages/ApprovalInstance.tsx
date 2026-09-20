import React, { useState, useEffect, useMemo, useRef } from 'react'
import { Typography, Table, Spin, Empty, Alert, Button, Select, DatePicker, Space, Input, Tooltip } from 'antd'
import type { TableProps } from 'antd'
import { FileTextOutlined, SyncOutlined, SearchOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { useLocation, useNavigate } from 'react-router-dom'
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
const APPROVAL_SORT_DIRECTIONS: Array<'ascend' | 'descend'> = ['ascend', 'descend', 'ascend']

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
  business_start_time?: string
  business_end_time?: string
  extension: any
}

type ApprovalInstanceSortOrder = 'ascend' | 'descend'
type ApprovalInstanceSortField = 'create_time' | 'finish_time' | 'business_start_time' | 'business_end_time'

interface ApprovalInstanceListState {
  status: string
  templateID: string
  category: string
  startDate: string
  endDate: string
  searchText: string
  page: number
  pageSize: number
  sortField: ApprovalInstanceSortField
  sortOrder: ApprovalInstanceSortOrder
}

const parsePositiveInteger = (value: string | null, fallback: number) => {
  const parsed = Number(value)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback
}

const readListState = (search: string): ApprovalInstanceListState => {
  const params = new URLSearchParams(search)
  const requestedSortField = params.get('sort_field')
  const sortField: ApprovalInstanceSortField = requestedSortField === 'finish_time'
    || requestedSortField === 'business_start_time'
    || requestedSortField === 'business_end_time'
    ? requestedSortField
    : 'create_time'
  return {
    status: params.get('status') || '',
    templateID: params.get('template_id') || '',
    category: params.get('category') || '',
    startDate: params.get('start_date') || '',
    endDate: params.get('end_date') || '',
    searchText: params.get('title') || '',
    page: parsePositiveInteger(params.get('page'), 1),
    pageSize: parsePositiveInteger(params.get('page_size'), 10),
    sortField,
    sortOrder: params.get('sort_order') === 'asc' ? 'ascend' : 'descend',
  }
}

const parseDate = (value: string) => {
  if (!value) return null
  const parsed = dayjs(value)
  return parsed.isValid() ? parsed : null
}

const formatBusinessTime = (value?: string | null) => {
  if (!value) return '-'
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return dayjs(value).format('YYYY年M月D日')
  }
  return formatDateTime(value)
}

const buildListSearch = (state: ApprovalInstanceListState) => {
  const params = new URLSearchParams()
  if (state.status) params.set('status', state.status)
  if (state.templateID) params.set('template_id', state.templateID)
  if (state.category) params.set('category', state.category)
  if (state.startDate) params.set('start_date', state.startDate)
  if (state.endDate) params.set('end_date', state.endDate)
  if (state.searchText.trim()) params.set('title', state.searchText.trim())
  if (state.page !== 1) params.set('page', String(state.page))
  if (state.pageSize !== 10) params.set('page_size', String(state.pageSize))
  if (state.sortField !== 'create_time') params.set('sort_field', state.sortField)
  if (state.sortOrder !== 'descend') params.set('sort_order', 'asc')
  const search = params.toString()
  return search ? `?${search}` : ''
}

const ApprovalInstance: React.FC = () => {
  const navigate = useNavigate()
  const location = useLocation()
  const initialListState = readListState(location.search)
  const [status, setStatus] = useState<string>(initialListState.status)
  const [templateID, setTemplateID] = useState<string>(initialListState.templateID)
  const [category, setCategory] = useState<string>(initialListState.category)
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null]>([
    parseDate(initialListState.startDate),
    parseDate(initialListState.endDate),
  ])
  const [searchText, setSearchText] = useState(initialListState.searchText)
  const [page, setPage] = useState(initialListState.page)
  const [pageSize, setPageSize] = useState(initialListState.pageSize)
  const [sortField, setSortField] = useState<ApprovalInstanceSortField>(initialListState.sortField)
  const [sortOrder, setSortOrder] = useState<ApprovalInstanceSortOrder>(initialListState.sortOrder)
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

  // 防抖搜索：输入停顿 300ms 后触发查询。
  const [debouncedSearch, setDebouncedSearch] = useState(initialListState.searchText)
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedSearch(searchText.trim())
    }, 300)
    return () => window.clearTimeout(timer)
  }, [searchText])

  const startDate = dateRange[0]?.format('YYYY-MM-DD') || ''
  const endDate = dateRange[1]?.format('YYYY-MM-DD') || ''

  useEffect(() => {
    const nextSearch = buildListSearch({
      status,
      templateID,
      category,
      startDate,
      endDate,
      searchText,
      page,
      pageSize,
      sortField,
      sortOrder,
    })
    if (location.search !== nextSearch) {
      navigate({ pathname: location.pathname, search: nextSearch }, { replace: true })
    }
  }, [category, endDate, location.pathname, location.search, navigate, page, pageSize, searchText, sortField, sortOrder, startDate, status, templateID])

  const queryParams = {
    page,
    page_size: pageSize,
    status: status || undefined,
    template_id: templateID || undefined,
    category: templateID ? undefined : (category || undefined),
    title: debouncedSearch || undefined,
    start_date: startDate || undefined,
    end_date: endDate || undefined,
    sort_field: sortField,
    sort_order: sortOrder === 'ascend' ? 'asc' as const : 'desc' as const,
  }

  const { data: instancesData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['approval-instances', queryParams],
    queryFn: () => approvalAPI.getInstances(queryParams),
  })

  const { data: templatesData } = useQuery({
    queryKey: ['approval-templates'],
    queryFn: () => approvalAPI.getTemplates(),
  })

  const restoredScrollKeyRef = useRef('')
  useEffect(() => {
    if (isLoading || !instancesData) return
    const listSearch = buildListSearch({
      status,
      templateID,
      category,
      startDate,
      endDate,
      searchText,
      page,
      pageSize,
      sortField,
      sortOrder,
    })
    const storageKey = `approval-instances-scroll:${location.pathname}${listSearch}`
    if (restoredScrollKeyRef.current === storageKey) return
    const storedScrollY = window.sessionStorage.getItem(storageKey)
    if (storedScrollY === null) {
      restoredScrollKeyRef.current = storageKey
      return
    }
    const scrollY = Number(storedScrollY)
    if (!Number.isFinite(scrollY) || scrollY < 0) return
    restoredScrollKeyRef.current = storageKey
    window.sessionStorage.removeItem(storageKey)
    window.requestAnimationFrame(() => window.scrollTo({ top: scrollY, behavior: 'auto' }))
  }, [category, endDate, instancesData, isLoading, location.pathname, page, pageSize, searchText, sortField, sortOrder, startDate, status, templateID])

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
    const returnTo = `${location.pathname}${buildListSearch({
      status,
      templateID,
      category,
      startDate,
      endDate,
      searchText,
      page,
      pageSize,
      sortField,
      sortOrder,
    })}`
    window.sessionStorage.setItem(`approval-instances-scroll:${returnTo}`, String(window.scrollY))
    navigate(`/approval-detail/${id}`, { state: { from: returnTo } })
  }

  const handlePaginationChange = (nextPage: number, nextPageSize: number) => {
    if (nextPageSize !== pageSize) {
      setPageSize(nextPageSize)
      setPage(1)
      return
    }
    setPage(nextPage)
  }

  const handleTableChange: TableProps<ApprovalInstance>['onChange'] = (_pagination, _filters, sorter, extra) => {
    if (extra.action !== 'sort') return
    const nextSorter = Array.isArray(sorter) ? sorter[0] : sorter
    const nextSorterField = nextSorter?.field ?? nextSorter?.columnKey
    if (nextSorterField === 'create_time' || nextSorterField === 'finish_time' || nextSorterField === 'business_start_time' || nextSorterField === 'business_end_time') {
      const nextSortField = nextSorterField as ApprovalInstanceSortField
      const requestedOrder = nextSorter.order === 'ascend' || nextSorter.order === 'descend'
        ? nextSorter.order
        : undefined
      const nextSortOrder = nextSortField === sortField
        ? (requestedOrder && requestedOrder !== sortOrder
          ? requestedOrder
          : (sortOrder === 'ascend' ? 'descend' : 'ascend'))
        : requestedOrder
      if (nextSortOrder) {
        if (nextSortField !== sortField || nextSortOrder !== sortOrder) {
          setSortField(nextSortField)
          setSortOrder(nextSortOrder)
          setPage(1)
        }
      }
    }
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
      sorter: true,
      sortDirections: APPROVAL_SORT_DIRECTIONS,
      sortOrder: sortField === 'create_time' ? sortOrder : undefined,
      render: (v: string) => formatDateTime(v),
    },
    {
      title: '审批完成时间',
      dataIndex: 'finish_time',
      key: 'finish_time',
      sorter: true,
      sortDirections: APPROVAL_SORT_DIRECTIONS,
      sortOrder: sortField === 'finish_time' ? sortOrder : undefined,
      render: (finishTime: string | null) => finishTime ? formatDateTime(finishTime) : '-',
    },
    {
      title: '业务开始时间',
      dataIndex: 'business_start_time',
      key: 'business_start_time',
      sorter: true,
      sortDirections: APPROVAL_SORT_DIRECTIONS,
      sortOrder: sortField === 'business_start_time' ? sortOrder : undefined,
      render: (value: string | null | undefined) => formatBusinessTime(value),
    },
    {
      title: '业务结束时间',
      dataIndex: 'business_end_time',
      key: 'business_end_time',
      sorter: true,
      sortDirections: APPROVAL_SORT_DIRECTIONS,
      sortOrder: sortField === 'business_end_time' ? sortOrder : undefined,
      render: (value: string | null | undefined) => formatBusinessTime(value),
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
            value={status || undefined}
            onChange={(v) => { setStatus(v || ''); setPage(1) }}
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
            value={dateRange}
            onChange={(range) => { setDateRange(range || [null, null]); setPage(1) }}
            placeholder={['业务开始日期', '业务结束日期']}
            format="YYYY-MM-DD"
            locale={datePickerZhCN}
          />
          <Input
            placeholder="搜索标题"
            style={{ width: 200 }}
            prefix={<SearchOutlined />}
            allowClear
            value={searchText}
            onChange={(e) => { setSearchText(e.target.value); setPage(1) }}
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
              onChange: handlePaginationChange,
            }}
            onChange={handleTableChange}
          />
        ) : (
          <Empty description="暂无审批实例" />
        )}
      </PageCard>
    </PageContainer>
  )
}

export default ApprovalInstance
