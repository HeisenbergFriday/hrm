import React, { useState, useEffect, useMemo, useRef } from 'react'
import { Typography, Table, Spin, Empty, Alert, Button, Select, DatePicker, Space, Input } from 'antd'
import type { TableProps } from 'antd'
import { FileTextOutlined, SearchOutlined, ReloadOutlined, UndoOutlined, EyeOutlined, ArrowUpOutlined, ArrowDownOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { useLocation, useNavigate } from 'react-router-dom'
import { approvalAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import ApprovalStatusTag from '../components/ApprovalStatusTag'
import ApprovalSyncStatus from '../components/ApprovalSyncStatus'
import { useApprovalSync } from '../hooks/useApprovalSync'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import datePickerZhCN from 'antd/es/date-picker/locale/zh_CN'
import { formatDateTime } from '../utils/format'
import './approval.css'

dayjs.locale('zh-cn')

const { Text } = Typography
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

  const { canSync, isSyncing, notice: syncNotice, startManualSync } = useApprovalSync({
    processCode: templateID || undefined,
    startDate: startDate || undefined,
    endDate: endDate || undefined,
    onCompleted: () => { void refetch() },
  })

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
    const nextSortField = nextSorterField === 'approval_time'
      ? 'create_time'
      : nextSorterField === 'business_time'
        ? 'business_start_time'
        : nextSorterField
    if (nextSortField === 'create_time' || nextSortField === 'finish_time' || nextSortField === 'business_start_time' || nextSortField === 'business_end_time') {
      const normalizedSortField = nextSortField as ApprovalInstanceSortField
      const requestedOrder = nextSorter.order === 'ascend' || nextSorter.order === 'descend'
        ? nextSorter.order
        : undefined
      const nextSortOrder = normalizedSortField === sortField
        ? (requestedOrder && requestedOrder !== sortOrder
          ? requestedOrder
          : (sortOrder === 'ascend' ? 'descend' : 'ascend'))
        : requestedOrder
      if (nextSortOrder) {
        if (normalizedSortField !== sortField || nextSortOrder !== sortOrder) {
          setSortField(normalizedSortField)
          setSortOrder(nextSortOrder)
          setPage(1)
        }
      }
    }
  }

  const resetFilters = () => {
    setStatus('')
    setTemplateID('')
    setCategory('')
    setDateRange([null, null])
    setSearchText('')
    setPage(1)
    setSortField('create_time')
    setSortOrder('descend')
  }

  const approvalItems = (instancesData?.data?.items || []) as ApprovalInstance[]
  const sortOptions = [
    { value: 'create_time', label: '发起时间' },
    { value: 'finish_time', label: '完成时间' },
    { value: 'business_start_time', label: '业务开始时间' },
    { value: 'business_end_time', label: '业务结束时间' },
  ]

  const statusLabels: Record<string, string> = {
    completed: '已完成（含通过/拒绝）',
    running: '审批中',
    terminated: '已终止',
    canceled: '已取消',
  }
  const selectedTemplateName = templateID
    ? templateNameByID.get(templateID) || templateID
    : ''
  const filterSummary = [
    status ? `状态：${statusLabels[status] || status}` : '',
    selectedTemplateName ? `模板：${selectedTemplateName}` : '',
    category ? `类型：${CATEGORY_OPTIONS.find((item) => item.value === category)?.label || category}` : '',
    startDate || endDate ? `业务日期：${startDate || '不限'} 至 ${endDate || '不限'}` : '',
    debouncedSearch ? `标题：${debouncedSearch}` : '',
  ].filter(Boolean)

  const handleSortFieldChange = (nextField: ApprovalInstanceSortField) => {
    setSortField(nextField)
    setPage(1)
  }

  const toggleSortOrder = () => {
    setSortOrder((current) => current === 'ascend' ? 'descend' : 'ascend')
    setPage(1)
  }

  const columns = [
    {
      title: '审批标题',
      dataIndex: 'title',
      key: 'title',
      width: 260,
      render: (text: string, record: ApprovalInstance) => (
        <div className="approval-title-link">
          <Button
            className="approval-title-link-button"
            type="link"
            onClick={() => handleViewDetail(record.id)}
          >
            {text || '未命名审批'}
          </Button>
          <Text type="secondary">
            模板：{templateNameByID.get(record.template_id || record.extension?.process_code || record.extension?.template_id)
              || record.template_name
              || record.template_id
              || record.extension?.process_code
              || '未关联模板'}
          </Text>
          <Text type="secondary">实例编号：{record.process_id || record.id}</Text>
        </div>
      ),
    },
    {
      title: '发起人',
      dataIndex: 'applicant_name',
      key: 'applicant_name',
      width: 110,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string, record: ApprovalInstance) => (
        <ApprovalStatusTag status={status} result={record.extension?.result} />
      ),
    },
    {
      title: '审批时间',
      key: 'approval_time',
      width: 190,
      sorter: true,
      sortDirections: APPROVAL_SORT_DIRECTIONS,
      sortOrder: sortField === 'create_time' ? sortOrder : undefined,
      render: (_: unknown, record: ApprovalInstance) => (
        <div className="approval-time-cell">
          <Text>{formatDateTime(record.create_time)}</Text>
          <Text type="secondary">完成：{record.finish_time ? formatDateTime(record.finish_time) : '进行中'}</Text>
        </div>
      ),
    },
    {
      title: '业务时间',
      key: 'business_time',
      width: 180,
      sorter: true,
      sortDirections: APPROVAL_SORT_DIRECTIONS,
      sortOrder: sortField === 'business_start_time' ? sortOrder : undefined,
      render: (_: unknown, record: ApprovalInstance) => (
        <div className="approval-time-cell">
          <Text>{formatBusinessTime(record.business_start_time)}</Text>
          <Text type="secondary">至 {formatBusinessTime(record.business_end_time)}</Text>
        </div>
      ),
    },
    {
      title: '操作',
      key: 'action',
      fixed: 'right' as const,
      width: 100,
      render: (_: any, record: ApprovalInstance) => (
        <Button
          type="link"
          icon={<EyeOutlined />}
          aria-label="查看详情"
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
      subtitle="有新审批会自动更新，系统每 15 分钟自动查漏补缺"
      icon={<FileTextOutlined />}
      extra={(
        <Button icon={<ReloadOutlined />} onClick={() => refetch()} loading={isLoading}>
          刷新
        </Button>
      )}
    >
      <div className="approval-page-stack">
        <ApprovalSyncStatus
          notice={syncNotice}
          isSyncing={isSyncing}
          canSync={canSync}
          processCode={templateID || undefined}
          onManualSync={startManualSync}
        />

        <PageCard className="approval-filter-card">
          <div className="approval-filter-grid">
          <Select
            placeholder="状态"
            style={{ width: 120 }}
            allowClear
            value={status || undefined}
            onChange={(v) => { setStatus(v || ''); setPage(1) }}
          >
            <Option value="completed">已完成（含通过/拒绝）</Option>
            <Option value="running">审批中</Option>
            <Option value="terminated">已终止</Option>
            <Option value="canceled">已取消</Option>
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
            <Button type="primary" icon={<SearchOutlined />} onClick={() => refetch()}>
              查询结果
            </Button>
            <Button icon={<UndoOutlined />} onClick={resetFilters}>
              重置
            </Button>
          </Space>
          </div>
        </PageCard>

        <PageCard className="approval-table-card">
        {isLoading ? (
          <div className="approval-loading">
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
            size="middle"
            scroll={{ x: 980 }}
            title={() => (
              <div className="approval-table-title">
                <Space size={8} wrap>
                  <Text strong>审批记录</Text>
                  <Text type="secondary">共 {instancesData.data.total} 条</Text>
                </Space>
                <Text type="secondary" className="approval-filter-summary">
                  {filterSummary.length > 0 ? `当前筛选：${filterSummary.join(' · ')}` : '当前显示全部审批记录'}
                </Text>
                <Space size={8}>
                  <Text type="secondary">排序</Text>
                  <Select
                    size="small"
                    value={sortField}
                    options={sortOptions}
                    onChange={handleSortFieldChange}
                    aria-label="排序字段"
                  />
                  <Button
                    size="small"
                    type="text"
                    icon={sortOrder === 'ascend' ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
                    aria-label={sortOrder === 'ascend' ? '升序' : '降序'}
                    onClick={toggleSortOrder}
                  />
                </Space>
              </div>
            )}
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
      </div>
    </PageContainer>
  )
}

export default ApprovalInstance
