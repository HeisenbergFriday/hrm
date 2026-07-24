import React, { useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Descriptions,
  Drawer,
  Empty,
  Image,
  Input,
  Result,
  Space,
  Table,
  Tag,
  Tooltip,
} from 'antd'
import { ClearOutlined, EyeOutlined, FileSearchOutlined, ReloadOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import { approvalAPI } from '../services/api'
import { useAuthStore } from '../store/authStore'
import { maskEmail, maskMobile } from '../utils/maskPii'
import { hasMenuPermission, hasPermission } from '../utils/permission'

dayjs.extend(utc)

type OAApprovalRecord = {
  key: string
  fields: Record<string, unknown>
}

const display = (value: unknown) => {
  if (value === null || value === undefined || value === '') return '—'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

const field = (record: OAApprovalRecord, ...names: string[]) => {
  for (const name of names) {
    const value = record.fields[name]
    if (value !== undefined && value !== null && value !== '') return value
  }
  return undefined
}

const formatDate = (value: unknown) => {
  if (value === null || value === undefined || value === '') return '—'
  const raw = String(value).trim()
  const hasExplicitOffset = /(?:Z|[+-]\d{2}:?\d{2})$/i.test(raw)
  const parsed = hasExplicitOffset ? dayjs(raw).utcOffset(8) : dayjs.utc(raw).utcOffset(8, true)
  return parsed.isValid() ? parsed.format('YYYY-MM-DD HH:mm:ss') : display(value)
}

const labelMap: Record<string, string> = {
  corp_name: '公司主体',
  business_id: '业务 ID',
  process_name: '流程名称',
  process_code: '流程唯一编码',
  approval_title: '审批标题',
  originator_user_id: '发起人用户 ID',
  originator_user_name: '发起人',
  originator_dept_id: '发起人部门 ID',
  originator_dept_name: '发起人部门',
  originator_position: '发起人职位',
  originator_email: '发起人邮箱',
  originator_mobile: '发起人手机',
  originator_avatar_url: '发起人头像',
  user_name: '发起人',
  status: '审批状态',
  approval_status: '审批状态',
  approval_result: '审批结果',
  approval_create_time: '审批创建时间',
  approval_finish_time: '审批完成时间',
  approval_dept_name: '审批部门',
  approval_type: '审批类型',
  attendance_date: '考勤日期',
  attendance_start_time: '考勤开始时间',
  attendance_end_time: '考勤结束时间',
  attendance_reason: '考勤事由',
  attendance_sub_type: '考勤子类型',
  attendance_type: '考勤类型',
  current_activity_ids: '当前节点',
  current_approver_count: '当前审批人数量',
  current_approver_names: '当前审批人',
  current_approver_user_ids: '当前审批人 ID',
  current_task_create_time: '当前任务创建时间',
  current_task_ids: '当前任务 ID',
  create_time: '创建时间',
  gmt_create: '创建时间',
  finish_time: '完成时间',
  gmt_finished: '完成时间',
  update_time: '更新时间',
  db_update_time: '数据更新时间',
  duration: '时长',
  duration_unit: '时长单位',
  first_level_department_id: '一级部门 ID',
  first_level_department_name: '一级部门',
  second_level_department_id: '二级部门 ID',
  second_level_department_name: '二级部门',
  form_component_values: '审批内容',
  biz_action: '业务动作',
  attachments: '附件',
  photo_url: '图片',
  attachment_url: '附件',
  reason: '事由',
  remark: '备注',
  title: '标题',
  content: '内容',
  leave_type: '请假类型',
  overtime_type: '加班类型',
  amount: '金额',
  total_amount: '总金额',
  is_effective: '是否有效',
  is_operation_staff: '是否运营人员',
  proof_type: '证明类型',
  proof_url: '证明附件',
  related_process_list_json: '关联流程',
  repair_plan_id: '补卡计划 ID',
  repair_plan_text: '补卡说明',
  source_db_update_time: '数据源更新时间',
  user_dept_match_flag: '用户部门匹配',
  user_title: '员工职位',
  witness_user_id: '证明人用户 ID',
  witness_user_name: '证明人',
  witness_user_names: '证明人',
  work_place: '工作地点',
  includes_holiday: '是否含法定节假日',
  holiday_dates: '法定节假日日期',
  is_holiday: '是否节假日',
}

const unitMap: Record<string, string> = {
  hour: '小时',
  hours: '小时',
  day: '天',
  days: '天',
  half_day: '半天',
  minute: '分钟',
  minutes: '分钟',
}

const translateUnit = (value: unknown) => {
  if (value === null || value === undefined || value === '') return '—'
  if (typeof value === 'string' && unitMap[value]) return unitMap[value]
  return display(value)
}

const tryParseJSON = (value: unknown): unknown => {
  if (typeof value !== 'string') return value
  const trimmed = value.trim()
  if (!trimmed.startsWith('[') && !trimmed.startsWith('{')) return value
  try {
    return JSON.parse(trimmed)
  } catch {
    return value
  }
}

const isImageUrl = (v: unknown): v is string => {
  if (typeof v !== 'string') return false
  const s = v.trim()
  if (!/^https?:\/\//i.test(s)) return false
  return /\.(jpe?g|png|gif|webp|bmp|svg)(\?.*)?$/i.test(s) || /static\.dingtalk\.com\/media\//i.test(s)
}

const isHttpUrl = (v: unknown): v is string => typeof v === 'string' && /^https?:\/\//i.test(v.trim())

const sensitiveFieldType = (fieldName?: string): 'email' | 'mobile' | null => {
  const name = String(fieldName || '').trim().toLowerCase()
  if (!name) return null
  if (/(^|[_\s-])(e_?mail|mailbox)([_\s-]|$)|邮箱|電子郵件|电子邮件/.test(name)) return 'email'
  if (/(^|[_\s-])(mobile|phone|telephone|tel)([_\s-]|$)|手机号|手機號|手机|聯繫電話|联系电话|电话|電話/.test(name)) return 'mobile'
  return null
}

const maskSensitiveValue = (fieldName: string | undefined, value: unknown): unknown => {
  const type = sensitiveFieldType(fieldName)
  if (type && (typeof value === 'string' || typeof value === 'number')) {
    return type === 'email' ? maskEmail(String(value)) : maskMobile(String(value))
  }
  if (Array.isArray(value)) return value.map((item) => maskSensitiveValue(fieldName, item))
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>).map(([name, item]) => [name, maskSensitiveValue(name, item)]),
    )
  }
  return value
}

const renderScalar = (v: unknown, fieldName?: string): React.ReactNode => {
  if (v === null || v === undefined || v === '') return '—'
  const safeValue = maskSensitiveValue(fieldName, v)
  if (isImageUrl(v)) {
    return <Image src={v as string} alt="附件图片" width={120} style={{ borderRadius: 4 }} />
  }
  if (isHttpUrl(v)) {
    return <a href={v as string} target="_blank" rel="noreferrer noopener">{v as string}</a>
  }
  return display(safeValue)
}

type TableRowCell = { label?: string; bizAlias?: string; value?: unknown }
type TableRow = { rowValue?: TableRowCell[] }
type FormComponent = {
  componentType?: string
  name?: string
  bizAlias?: string
  value?: unknown
}

const renderComponentValue = (rawValue: unknown, componentType?: string): React.ReactNode => {
  if (rawValue === null || rawValue === undefined || rawValue === '') return '—'
  const parsed = tryParseJSON(rawValue)

  if (componentType === 'TableField' && Array.isArray(parsed)) {
    const rows = parsed as TableRow[]
    if (rows.length === 0) return '—'
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        {rows.map((row, idx) => {
          const cells = Array.isArray(row?.rowValue) ? row.rowValue : []
          return (
            <div key={idx} style={{ background: 'var(--color-bg-page)', padding: '4px 8px', borderRadius: 6 }}>
              <span style={{ color: 'var(--color-text-secondary)', marginRight: 6 }}>第 {idx + 1} 行</span>
              {cells.map((cell, i) => (
                <span key={i} style={{ marginRight: 12, display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                  <span style={{ color: 'var(--color-text-secondary)' }}>{cell?.label || cell?.bizAlias || ''}：</span>
                  {renderScalar(cell?.value, cell?.bizAlias || cell?.label)}
                </span>
              ))}
            </div>
          )
        })}
      </div>
    )
  }

  if (Array.isArray(parsed)) {
    const hasImage = parsed.some(isImageUrl)
    if (hasImage) {
      return (
        <Image.PreviewGroup>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            {parsed.map((item, i) => <React.Fragment key={i}>{renderScalar(item)}</React.Fragment>)}
          </div>
        </Image.PreviewGroup>
      )
    }
    return parsed.map((item) => (typeof item === 'string' ? unitMap[item] || item : String(item))).join('、')
  }
  if (parsed && typeof parsed === 'object') return display(parsed)
  if (typeof parsed === 'string') {
    if (isImageUrl(parsed)) return renderScalar(parsed)
    if (isHttpUrl(parsed)) return renderScalar(parsed)
    return unitMap[parsed] || parsed
  }
  return display(parsed)
}

const renderFormComponents = (value: unknown): React.ReactNode => {
  const parsed = tryParseJSON(value)
  if (!Array.isArray(parsed) || parsed.length === 0) return display(value)
  const items = parsed as FormComponent[]
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      {items.map((item, idx) => (
        <div
          key={idx}
          style={{
            display: 'flex',
            alignItems: 'flex-start',
            padding: '4px 0',
            borderBottom: idx < items.length - 1 ? '1px dashed var(--color-border)' : 'none',
          }}
        >
          <div style={{ minWidth: 120, color: 'var(--color-text-secondary)' }}>{item?.name || item?.bizAlias || '—'}</div>
          <div style={{ flex: 1, wordBreak: 'break-word' }}>
            {sensitiveFieldType(item?.bizAlias || item?.name)
              ? renderScalar(item?.value, item?.bizAlias || item?.name)
              : renderComponentValue(item?.value, item?.componentType)}
          </div>
        </div>
      ))}
    </div>
  )
}

const statusTextMap: Record<string, string> = {
  COMPLETED: '已完成',
  TERMINATED: '已终止',
  RUNNING: '审批中',
  CANCELED: '已取消',
  CANCELLED: '已取消',
  agree: '同意',
  AGREE: '同意',
  refuse: '拒绝',
  REFUSE: '拒绝',
}

const translateStatus = (value: string) => statusTextMap[value] || value

const statusColor = (value: string): 'success' | 'error' | 'processing' | 'default' => {
  if (['已完成', '同意', '通过'].includes(value)) return 'success'
  if (['已终止', '拒绝', '已取消'].includes(value)) return 'error'
  if (['审批中', '处理中'].includes(value)) return 'processing'
  return 'default'
}

const isTimeKey = (name: string) => /_(time|date)$/.test(name) || name === 'gmt_create' || name === 'gmt_finished'
const isStatusKey = (name: string) => name === 'status' || name === 'approval_status' || name === 'approval_result' || name === 'result'
const isUnitKey = (name: string) => name === 'duration_unit' || name === 'unit'
const isFormComponentKey = (name: string) => name === 'form_component_values' || name === 'form_components' || name === 'components'
const HIDDEN_FIELDS = new Set(['process_instance_id'])

const OAApprovalData: React.FC = () => {
  const orgId = useAuthStore((state) => state.orgId)
  const canView = hasPermission('approval_manage') || hasMenuPermission('menu:oa-approval-data')
  const [keyword, setKeyword] = useState('')
  const [inputKeyword, setInputKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<OAApprovalRecord | null>(null)
  const pageSize = 20

  const query = useQuery({
    queryKey: ['oa-approval-data', orgId, keyword, page],
    queryFn: () => approvalAPI.getOAData({ page, page_size: pageSize, keyword: keyword || undefined }),
    enabled: canView && orgId.trim().toLowerCase() === 'muteng',
  })

  const items: OAApprovalRecord[] = query.data?.data?.items || []
  const total = Number(query.data?.data?.total || 0)

  const columns = useMemo(() => [
    { title: '公司主体', key: 'corp_name', width: 180, render: (_: unknown, record: OAApprovalRecord) => display(field(record, 'corp_name')) },
    { title: '审批标题', key: 'approval_title', width: 220, ellipsis: true, render: (_: unknown, record: OAApprovalRecord) => display(field(record, 'approval_title', 'process_name')) },
    { title: '流程名称', key: 'process_name', width: 160, ellipsis: true, render: (_: unknown, record: OAApprovalRecord) => display(field(record, 'process_name')) },
    { title: '发起人', key: 'originator_user_name', width: 120, render: (_: unknown, record: OAApprovalRecord) => display(field(record, 'originator_user_name', 'user_name', 'originator_user_id')) },
    { title: '创建时间', key: 'create_time', width: 180, render: (_: unknown, record: OAApprovalRecord) => formatDate(field(record, 'create_time', 'gmt_create', 'approval_create_time')) },
    { title: '状态', key: 'status', width: 110, render: (_: unknown, record: OAApprovalRecord) => {
      const raw = field(record, 'status', 'approval_status', 'result')
      if (raw === undefined) return '—'
      const value = translateStatus(String(raw))
      return <Tag color={statusColor(value)}>{value}</Tag>
    } },
    { title: '操作', key: 'action', fixed: 'right' as const, width: 72, render: (_: unknown, record: OAApprovalRecord) => (
      <Tooltip title="查看详情">
        <Button type="link" icon={<EyeOutlined />} aria-label="查看审批详情" onClick={() => setSelected(record)} />
      </Tooltip>
    ) },
  ], [])

  const runSearch = () => {
    setPage(1)
    setKeyword(inputKeyword.trim())
  }

  if (orgId.trim().toLowerCase() !== 'muteng') {
    return <Result status="403" title="无访问权限" subTitle="OA审批数据仅对沐腾组织开放。" />
  }

  if (!canView) {
    return <PageContainer title="OA审批数据" icon={<FileSearchOutlined />}><PageCard><Alert type="warning" showIcon message="无访问权限" description="你没有访问 OA 审批数据的权限。" /></PageCard></PageContainer>
  }

  return (
    <PageContainer title="OA审批数据" icon={<FileSearchOutlined />} subtitle="沐腾组织审批明细">
      <PageCard>
        <Space wrap style={{ marginBottom: 16 }}>
          <Input
            allowClear
            value={inputKeyword}
            style={{ width: 320 }}
            placeholder="搜索流程、标题、发起人或业务 ID"
            onChange={(event) => setInputKeyword(event.target.value)}
            onPressEnter={runSearch}
          />
          <Button type="primary" icon={<FileSearchOutlined />} onClick={runSearch}>查询</Button>
          <Button icon={<ClearOutlined />} onClick={() => { setInputKeyword(''); setKeyword(''); setPage(1) }}>重置</Button>
          <Button icon={<ReloadOutlined />} loading={query.isFetching} onClick={() => { void query.refetch() }}>刷新</Button>
        </Space>
        {query.isError ? (
          <Alert type="error" showIcon message="OA审批数据加载失败" description={(query.error as Error)?.message || '请稍后重试'} action={<Button size="small" onClick={() => { void query.refetch() }}>重试</Button>} />
        ) : (
          <Table
            rowKey="key"
            loading={query.isLoading}
            columns={columns}
            dataSource={items}
            scroll={{ x: 1250 }}
            locale={{ emptyText: <Empty description="暂无 OA 审批数据" /> }}
            pagination={{ current: page, pageSize, total, showSizeChanger: false, onChange: (next) => setPage(next), showTotal: (count) => `共 ${count} 条` }}
          />
        )}
      </PageCard>

      <Drawer title="OA审批详情" width={560} open={Boolean(selected)} onClose={() => setSelected(null)}>
        {selected ? (
          <Descriptions bordered column={1} size="small">
            {Object.entries(selected.fields)
              .filter(([name]) => !HIDDEN_FIELDS.has(name))
              .map(([name, value]) => {
                let content: React.ReactNode
                if (isFormComponentKey(name)) {
                  content = renderFormComponents(value)
                } else if (isTimeKey(name)) {
                  content = formatDate(value)
                } else if (isStatusKey(name) && value !== null && value !== undefined && value !== '') {
                  const text = translateStatus(String(value))
                  content = <Tag color={statusColor(text)}>{text}</Tag>
                } else if (isUnitKey(name)) {
                  content = translateUnit(value)
                } else {
                  content = renderScalar(value, name)
                }
                return (
                  <Descriptions.Item key={name} label={labelMap[name] || name}>{content}</Descriptions.Item>
                )
              })}
          </Descriptions>
        ) : null}
      </Drawer>
    </PageContainer>
  )
}

export default OAApprovalData
