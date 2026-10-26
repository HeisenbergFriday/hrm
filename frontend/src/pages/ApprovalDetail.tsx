import React from 'react'
import { Typography, Descriptions, Timeline, Button, Spin, Alert, Empty, Image, Card, Row, Col, Space } from 'antd'
import { ArrowLeftOutlined, CheckCircleOutlined, CloseCircleOutlined, FileSearchOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { approvalAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import ApprovalStatusTag from '../components/ApprovalStatusTag'
import ApprovalSyncStatus from '../components/ApprovalSyncStatus'
import { useApprovalSync } from '../hooks/useApprovalSync'
import { formatDateTime } from '../utils/format'
import './approval.css'

const { Title, Text, Paragraph } = Typography

interface FlowNode {
  node_name: string
  approver_id: string
  approver_name: string
  action: string
  comment: string
  time: string
}

const DISPLAYABLE_OBJECT_KEYS = new Set([
  'label', 'value', 'content', 'text', 'remark', 'comment', 'children', 'rowValue',
  'rowValues', 'details', 'detailList', 'items', 'name', 'type', 'duration', 'unit',
  'startTime', 'endTime', 'start_time', 'end_time', 'from', 'to', 'date', 'amount',
])

const HIDDEN_FORM_KEYS = new Set([
  'componentName', 'componentType', 'bizAlias', 'id', 'required', 'disabled', 'hidden',
  'hiddenInApprovalDetail', 'notPrint', 'holidayOptions', 'options', 'placeholder',
  'format', 'useCalendar', 'featureMap', 'props',
])

const AUXILIARY_TEXT_PATTERNS = [
  /温馨提示/,
  /自动计算/,
  /组件配置/,
  /填写说明/,
  /请填写/,
  /系统自动/,
  /仅供参考/,
  /无需填写/,
]

const OBJECT_KEY_LABELS: Record<string, string> = {
  label: '字段',
  value: '值',
  content: '内容',
  text: '说明',
  remark: '备注',
  comment: '备注',
  children: '明细',
  rowValue: '明细',
  rowValues: '明细',
  detailList: '明细',
  startTime: '开始时间',
  endTime: '结束时间',
  start_time: '开始时间',
  end_time: '结束时间',
  duration: '时长',
  unit: '单位',
}

const isRecord = (value: unknown): value is Record<string, unknown> => (
  typeof value === 'object' && value !== null && !Array.isArray(value)
)

const parseNestedJSON = (value: unknown): unknown => {
  let parsed = value
  for (let depth = 0; depth < 4 && typeof parsed === 'string'; depth += 1) {
    const trimmed = parsed.trim()
    if ((!trimmed.startsWith('{') && !trimmed.startsWith('[')) || trimmed.length < 2) break
    try {
      parsed = JSON.parse(trimmed)
    } catch {
      break
    }
  }
  return parsed
}

const isPresentationOnlyRecord = (value: Record<string, unknown>): boolean => {
  const componentNames = [value.componentName, value.componentType]
    .filter((item): item is string => typeof item === 'string')
    .map(item => item.toLowerCase().replace(/[-_\s]/g, ''))
  return componentNames.some(name => name === 'textnote' || name === 'description')
}

const isAuxiliaryText = (value: string): boolean => AUXILIARY_TEXT_PATTERNS.some(pattern => pattern.test(value))

const hasBusinessValue = (value: unknown): boolean => {
  const parsed = parseNestedJSON(value)
  if (parsed !== value) return hasBusinessValue(parsed)
  if (value === null || value === undefined) return false
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase()
    return normalized !== '' && normalized !== 'null' && normalized !== 'undefined' && !isAuxiliaryText(value)
  }
  if (Array.isArray(value)) return value.some(hasBusinessValue)
  if (isRecord(value)) {
    if (isPresentationOnlyRecord(value)) return false
    const entries = Object.entries(value).filter(([key]) => !HIDDEN_FORM_KEYS.has(key) || key === 'props')
    if (entries.length === 0) return false
    return entries.some(([, item]) => hasBusinessValue(item))
  }
  return true
}

const objectEntriesForDisplay = (value: Record<string, unknown>): Array<[string, unknown]> => {
  const parsedProps = parseNestedJSON(value.props)
  const source = isRecord(parsedProps) ? parsedProps : value
  return Object.entries(source).filter(([key, item]) => {
    if (HIDDEN_FORM_KEYS.has(key)) return false
    if (key === 'label' || key === 'value') return false
    if (!hasBusinessValue(item)) return false
    // Keep unknown business fields, while hiding only known DingTalk schema metadata.
    return DISPLAYABLE_OBJECT_KEYS.has(key)
      || source !== value
      || !Object.prototype.hasOwnProperty.call(value, 'componentName')
  })
}

const getBusinessField = (value: Record<string, unknown>): { label: unknown; value: unknown } | null => {
  const parsedProps = parseNestedJSON(value.props)
  const source = isRecord(parsedProps) ? parsedProps : value
  const label = source.label
  if (!hasBusinessValue(label)) return null

  const candidateKeys = ['value', 'children', 'content', 'text']
  const valueKey = candidateKeys.find(key => source[key] !== undefined && source[key] !== null && hasBusinessValue(source[key]))
    || candidateKeys.find(key => source[key] !== undefined && source[key] !== null)
  if (!valueKey) return null
  return { label, value: source[valueKey] }
}

const displayObjectKey = (key: string): string => OBJECT_KEY_LABELS[key] || key

const ApprovalDetail: React.FC = () => {
  const navigate = useNavigate()
  const location = useLocation()
  const { id } = useParams<{ id: string }>()
  const returnTo = typeof location.state?.from === 'string' && location.state.from.startsWith('/approval-instances')
    ? location.state.from
    : '/approval-instances'

  const { data: approvalData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['approval-detail', id],
    queryFn: () => approvalAPI.getApproval(id!),
  })

  const approval = approvalData?.data?.approval
  const processCode = approval?.extension?.process_code || approval?.template_id
  const { canSync, isSyncing, notice: syncNotice, startManualSync } = useApprovalSync({
    processCode,
    onCompleted: () => { void refetch() },
  })

  const unitMap: Record<string, string> = {
    hour: '小时', hours: '小时', day: '天', days: '天',
    half_day: '半天', minute: '分钟', minutes: '分钟',
  }

  const stringifyCell = (v: unknown): string => {
    if (v === null || v === undefined || v === '') return '—'
    if (typeof v === 'string') return unitMap[v] || v
    if (typeof v === 'object') return '—'
    return String(v)
  }

  const formatBusinessTime = (value?: string | null): string => {
    if (!value) return '—'
    return /^\d{4}-\d{2}-\d{2}$/.test(value) ? value : formatDateTime(value)
  }

  const isImageUrl = (v: unknown): v is string => {
    if (typeof v !== 'string') return false
    const s = v.trim()
    if (!/^https?:\/\//i.test(s)) return false
    return /\.(jpe?g|png|gif|webp|bmp|svg)(\?.*)?$/i.test(s) || /static\.dingtalk\.com\/media\//i.test(s)
  }

  const isHttpUrl = (v: unknown): v is string => typeof v === 'string' && /^https?:\/\//i.test(v.trim())

  const renderCell = (v: unknown): React.ReactNode => {
    if (isImageUrl(v)) {
      return <Image src={v} alt="附件图片" width={120} style={{ borderRadius: 4 }} />
    }
    if (isHttpUrl(v)) {
      return <a href={v} target="_blank" rel="noreferrer noopener">{v}</a>
    }
    return <Text style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{stringifyCell(v)}</Text>
  }

  const renderStructuredValue = (rawValue: unknown, depth = 0): React.ReactNode => {
    const value = parseNestedJSON(rawValue)
    if (depth > 8) return renderCell(value)

    if (Array.isArray(value)) {
      const businessItems = value.filter(hasBusinessValue)
      if (businessItems.length === 0) return null
      if (businessItems.some(isImageUrl)) {
        return (
          <Image.PreviewGroup>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
              {businessItems.map((item, index) => <React.Fragment key={index}>{renderCell(item)}</React.Fragment>)}
            </div>
          </Image.PreviewGroup>
        )
      }
      return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, width: '100%' }}>
          {businessItems.map((item, index) => (
            <div
              key={index}
              style={{
                padding: isRecord(parseNestedJSON(item)) || Array.isArray(parseNestedJSON(item)) ? '6px 8px' : 0,
                border: isRecord(parseNestedJSON(item)) || Array.isArray(parseNestedJSON(item)) ? '1px solid var(--color-border)' : undefined,
                borderRadius: 'var(--radius-sm)',
                background: isRecord(parseNestedJSON(item)) || Array.isArray(parseNestedJSON(item)) ? 'var(--color-bg-layout)' : undefined,
              }}
            >
              {renderStructuredValue(item, depth + 1)}
            </div>
          ))}
        </div>
      )
    }

    if (isRecord(value)) {
      if (isPresentationOnlyRecord(value)) return null
      const businessField = getBusinessField(value)
      if (businessField) {
        return (
          <div style={{ display: 'flex', gap: 8, alignItems: 'flex-start' }}>
            <Text strong>{stringifyCell(businessField.label)}</Text>
            <div style={{ minWidth: 0, flex: 1 }}>{renderStructuredValue(businessField.value, depth + 1)}</div>
          </div>
        )
      }
      const entries = objectEntriesForDisplay(value)
      if (entries.length === 0) return null
      return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, width: '100%' }}>
          {entries.map(([key, item]) => (
            <div key={key} style={{ display: 'grid', gridTemplateColumns: 'minmax(72px, 120px) minmax(0, 1fr)', gap: 8, alignItems: 'start' }}>
              <Text type="secondary">{displayObjectKey(key)}</Text>
              <div style={{ minWidth: 0 }}>{renderStructuredValue(item, depth + 1)}</div>
            </div>
          ))}
        </div>
      )
    }

    return renderCell(value)
  }

  const renderContentValue = (rawKey: string, rawValue: unknown): React.ReactNode => {
    const parsedKey = parseNestedJSON(rawKey)
    const parsedValue = parseNestedJSON(rawValue)

    if (Array.isArray(parsedKey) && Array.isArray(parsedValue)) {
      return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          {parsedKey.map((k: unknown, i: number) => (
            <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <Text strong>{stringifyCell(k)}：</Text>
              {renderStructuredValue(parsedValue[i])}
            </div>
          ))}
        </div>
      )
    }
    return renderStructuredValue(parsedValue)
  }

  const getActionIcon = (action: string) => {
    if (action === 'approved') {
      return <CheckCircleOutlined style={{ color: 'var(--color-success)' }} />
    } else if (action === 'rejected') {
      return <CloseCircleOutlined style={{ color: 'var(--color-error)' }} />
    }
    return null
  }

  const getActionText = (action: string) => {
    switch (action) {
      case 'approved':
        return '已通过'
      case 'rejected':
        return '已拒绝'
      case 'pending':
        return '待处理'
      default:
        return action
    }
  }

  return (
    <PageContainer
      title="审批详情"
      subtitle="集中查看审批内容、业务时间和完整流转轨迹"
      icon={<FileSearchOutlined />}
      extra={
        <Button
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate(returnTo, { replace: true })}
        >
          返回列表
        </Button>
      }
    >
      <PageCard>
        {isLoading ? (
          <div className="approval-loading">
            <Spin size="large" />
          </div>
        ) : isError ? (
          <div style={{ padding: 'var(--space-5)' }}>
            <Alert
              message="加载失败"
              description={(error as Error)?.message || '获取审批详情失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()}>
                  重试
                </Button>
              }
            />
          </div>
        ) : approvalData?.data?.approval ? (
          <div>
            {(() => {
              const approval = approvalData.data.approval
              const contentEntries = Object.entries(approval.content || {})
                .filter(([key, value]) => hasBusinessValue(key) && hasBusinessValue(value))
              const flowHistory = approval.flow_history || []
              const templateLabel = approval.template_name
                || approval.extension?.process_code
                || approval.template_id
                || '—'

              return (
                <div className="approval-page-stack">
                  <ApprovalSyncStatus
                    notice={syncNotice}
                    isSyncing={isSyncing}
                    canSync={canSync}
                    processCode={processCode}
                    disabledReason={processCode ? undefined : '当前审批缺少流程编码，无法补拉'}
                    onManualSync={startManualSync}
                  />

                  <div className="approval-detail-summary">
                    <div className="approval-detail-summary-head">
                      <div className="approval-detail-summary-title">
                        <Space direction="vertical" size={4}>
                          <Title level={4}>{approval.title || '审批详情'}</Title>
                          <Text type="secondary">审批模板：{templateLabel}</Text>
                        </Space>
                      </div>
                      <ApprovalStatusTag
                        status={approval.status}
                        result={approval.extension?.result}
                        emptyLabel=""
                      />
                    </div>
                    <Descriptions
                      column={{ xs: 1, sm: 2, md: 3 }}
                      size="small"
                    >
                      <Descriptions.Item label="申请人">{approval.applicant_name || '—'}</Descriptions.Item>
                      <Descriptions.Item label="发起时间">{formatDateTime(approval.create_time)}</Descriptions.Item>
                      <Descriptions.Item label="审批完成时间">
                        {approval.finish_time ? formatDateTime(approval.finish_time) : '—'}
                      </Descriptions.Item>
                    </Descriptions>
                  </div>

                  <Row className="approval-detail-grid" gutter={[16, 16]}>
                    <Col xs={24} xl={16}>
                      <Card className="approval-detail-card" title="审批内容" size="small">
                        {contentEntries.length > 0 ? (
                          <Descriptions bordered column={1} size="small">
                            {contentEntries.map(([key, value]) => {
                              const parsedKey = parseNestedJSON(key)
                              const labelText = Array.isArray(parsedKey)
                                ? parsedKey.map(stringifyCell).join(' / ')
                                : String(key)
                              return (
                                <Descriptions.Item key={key} label={labelText}>
                                  {renderContentValue(key, value)}
                                </Descriptions.Item>
                              )
                            })}
                          </Descriptions>
                        ) : (
                          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无有效审批内容" />
                        )}
                      </Card>
                    </Col>
                    <Col xs={24} xl={8}>
                      <div className="approval-page-stack">
                        <Card className="approval-detail-card" title="业务时间" size="small">
                          <Descriptions column={1} size="small">
                            <Descriptions.Item label="业务开始时间">
                              {formatBusinessTime(approval.business_start_time)}
                            </Descriptions.Item>
                            <Descriptions.Item label="业务结束时间">
                              {formatBusinessTime(approval.business_end_time)}
                            </Descriptions.Item>
                          </Descriptions>
                        </Card>

                        <Card className="approval-detail-card approval-detail-flow" title="审批流程" size="small">
                          {flowHistory.length > 0 ? (
                            <Timeline
                              items={flowHistory.map((node: FlowNode) => ({
                                color: node.action === 'approved' ? 'green' : node.action === 'rejected' ? 'red' : 'blue',
                                children: (
                                  <div>
                                    <Space size={8} wrap>
                                      <Text strong>{node.node_name || '审批节点'}</Text>
                                      {node.approver_name && <Text>{node.approver_name}</Text>}
                                      {getActionIcon(node.action)}
                                      <Text type={node.action === 'approved' ? 'success' : node.action === 'rejected' ? 'danger' : 'secondary'}>
                                        {getActionText(node.action)}
                                      </Text>
                                    </Space>
                                    {node.comment && (
                                      <Paragraph style={{ marginTop: 'var(--space-2)', marginBottom: 0 }}>
                                        备注：{node.comment}
                                      </Paragraph>
                                    )}
                                    {node.time && (
                                      <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>
                                        {formatDateTime(node.time)}
                                      </Text>
                                    )}
                                  </div>
                                ),
                              }))}
                            />
                          ) : (
                            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无审批流程记录" />
                          )}
                        </Card>
                      </div>
                    </Col>
                  </Row>
                </div>
              )
            })()}
          </div>
        ) : (
          <Empty description="审批详情不存在" />
        )}
      </PageCard>
    </PageContainer>
  )
}

export default ApprovalDetail
