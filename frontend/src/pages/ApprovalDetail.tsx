import React from 'react'
import { Typography, Descriptions, Timeline, Button, Spin, Alert, Divider, Empty, Image, message } from 'antd'
import { ArrowLeftOutlined, CheckCircleOutlined, CloseCircleOutlined, SyncOutlined, FileSearchOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { approvalAPI } from '../services/api'
import { hasPermission } from '../utils/permission'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import ApprovalStatusTag from '../components/ApprovalStatusTag'
import { formatDateTime } from '../utils/format'

const { Title, Text, Paragraph } = Typography

interface FlowNode {
  node_name: string
  approver_id: string
  approver_name: string
  action: string
  comment: string
  time: string
}

const ApprovalDetail: React.FC = () => {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()

  const { data: approvalData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['approval-detail', id],
    queryFn: () => approvalAPI.getApproval(id!),
  })

  const syncMutation = useMutation({
    mutationFn: (processCode: string) => approvalAPI.sync({ process_code: processCode }),
    onSuccess: () => {
      refetch()
    },
  })

  const unitMap: Record<string, string> = {
    hour: '小时', hours: '小时', day: '天', days: '天',
    half_day: '半天', minute: '分钟', minutes: '分钟',
  }

  const tryParseJSON = (value: unknown): unknown => {
    if (typeof value !== 'string') return value
    const trimmed = value.trim()
    if (!trimmed.startsWith('[') && !trimmed.startsWith('{')) return value
    try { return JSON.parse(trimmed) } catch { return value }
  }

  const stringifyCell = (v: unknown): string => {
    if (v === null || v === undefined || v === '') return '—'
    if (typeof v === 'string') return unitMap[v] || v
    if (typeof v === 'object') return JSON.stringify(v)
    return String(v)
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
    return <Text>{stringifyCell(v)}</Text>
  }

  const renderContentValue = (rawKey: string, rawValue: unknown): React.ReactNode => {
    const parsedKey = tryParseJSON(rawKey)
    const parsedValue = tryParseJSON(rawValue)

    if (Array.isArray(parsedKey) && Array.isArray(parsedValue)) {
      return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          {parsedKey.map((k: unknown, i: number) => (
            <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <Text strong>{stringifyCell(k)}：</Text>
              {renderCell(parsedValue[i])}
            </div>
          ))}
        </div>
      )
    }
    if (Array.isArray(parsedValue)) {
      const hasImage = parsedValue.some(isImageUrl)
      if (hasImage) {
        return (
          <Image.PreviewGroup>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
              {parsedValue.map((item, i) => <React.Fragment key={i}>{renderCell(item)}</React.Fragment>)}
            </div>
          </Image.PreviewGroup>
        )
      }
      return <Text>{parsedValue.map(stringifyCell).join('、')}</Text>
    }
    return renderCell(parsedValue)
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

  const handleSync = () => {
    const approval = approvalData?.data?.approval
    const processCode = approval?.extension?.process_code || approval?.template_id
    if (!processCode) {
      message.warning('当前审批缺少 process_code，无法同步')
      return
    }
    syncMutation.mutate(processCode)
  }

  return (
    <PageContainer
      title="审批详情"
      icon={<FileSearchOutlined />}
      extra={
        <Button
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate('/approval-instances')}
        >
          返回列表
        </Button>
      }
    >
      <PageCard>
        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
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
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-6)' }}>
              <div>
                <Title level={5}>{approvalData.data.approval.title}</Title>
                <Text type="secondary">模板：{approvalData.data.approval.template_name}</Text>
              </div>
              <ApprovalStatusTag status={approvalData.data.approval.status} emptyLabel="" />
            </div>

            <Descriptions bordered column={1} style={{ marginBottom: 'var(--space-6)' }}>
              <Descriptions.Item label="发起人">{approvalData.data.approval.applicant_name}</Descriptions.Item>
              <Descriptions.Item label="发起时间">{formatDateTime(approvalData.data.approval.create_time)}</Descriptions.Item>
              {approvalData.data.approval.finish_time && (
                <Descriptions.Item label="结束时间">{formatDateTime(approvalData.data.approval.finish_time)}</Descriptions.Item>
              )}
            </Descriptions>

            <Title level={5}>审批内容</Title>
            <div style={{ border: '1px solid var(--color-border-light)', borderRadius: 'var(--radius-xs)', padding: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
              {Object.entries(approvalData.data.approval.content || {}).map(([key, value]) => {
                const parsedKey = tryParseJSON(key)
                const labelText = Array.isArray(parsedKey) ? parsedKey.map(stringifyCell).join(' / ') : String(key)
                return (
                  <div key={key} style={{ marginBottom: 'var(--space-3)' }}>
                    <Text strong>{labelText}：</Text>
                    {renderContentValue(key, value)}
                  </div>
                )
              })}
            </div>

            <Title level={5}>审批流程</Title>
            <Timeline
              items={approvalData.data.approval.flow_history?.map((node: FlowNode, index: number) => ({
                color: node.action === 'approved' ? 'green' : node.action === 'rejected' ? 'red' : 'blue',
                children: (
                  <div>
                    <div style={{ display: 'flex', alignItems: 'center' }}>
                      <Text strong>{node.node_name}</Text>
                      <Text style={{ marginLeft: 'var(--space-3)' }}>{node.approver_name}</Text>
                      {getActionIcon(node.action)}
                      <Text style={{ marginLeft: 'var(--space-2)', color: node.action === 'approved' ? 'var(--color-success)' : node.action === 'rejected' ? 'var(--color-error)' : 'var(--color-primary)' }}>
                        {getActionText(node.action)}
                      </Text>
                    </div>
                    {node.comment && (
                      <Paragraph style={{ marginTop: 'var(--space-2)', marginBottom: 0 }}>
                        备注：{node.comment}
                      </Paragraph>
                    )}
                    <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>{formatDateTime(node.time)}</Text>
                  </div>
                ),
              })) || []}
            />

            <Divider />
            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <Button
                icon={<SyncOutlined />}
                onClick={handleSync}
                loading={syncMutation.isPending}
                disabled={!hasPermission('approval:sync')}
              >
                同步数据
              </Button>
            </div>
          </div>
        ) : (
          <Empty description="审批详情不存在" />
        )}
      </PageCard>
    </PageContainer>
  )
}

export default ApprovalDetail
