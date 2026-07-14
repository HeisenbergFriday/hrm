import React from 'react'
import { Alert, Card, Col, Descriptions, Divider, Drawer, Row, Spin, Steps, Tag, Tooltip, Typography } from 'antd'
import type { PerformanceActivity, PerformanceDistributionCheck, PerformanceHRDeadlineStatus } from '../services/api'
import { formatDateTime, getCycleLabel } from '../utils/format'
import StatusTag from './StatusTag'

const { Text } = Typography

type ActivityTemplateDisplay = {
  label: string
  color: string
  tooltip: string
}

type DetailSummaryCard = {
  title: string
  value: React.ReactNode
  color: string
  bg: string
}

type PerformanceActivityDetailDrawerProps = {
  open: boolean
  activity: PerformanceActivity | null
  stepItems: NonNullable<React.ComponentProps<typeof Steps>['items']>
  statusLabel?: string
  statusColor?: string
  templateDisplay: ActivityTemplateDisplay | null
  actionButtons: React.ReactNode
  hrDeadlineStatus: PerformanceHRDeadlineStatus | null
  summaryLoading: boolean
  summaryCards: DetailSummaryCard[]
  distributionCheck: PerformanceDistributionCheck | null
  participantTable: React.ReactNode
  onClose: () => void
}

function getDistributionLevelStyle(status?: string) {
  if (status === 'exceeded') {
    return { bg: '#fff2f0', barColor: '#ff4d4f' }
  }
  if (status === 'warning') {
    return { bg: '#fffbe6', barColor: '#faad14' }
  }
  return { bg: '#f6ffed', barColor: '#52c41a' }
}

const PerformanceActivityDetailDrawer: React.FC<PerformanceActivityDetailDrawerProps> = React.memo(({
  open,
  activity,
  stepItems,
  statusLabel,
  statusColor,
  templateDisplay,
  actionButtons,
  hrDeadlineStatus,
  summaryLoading,
  summaryCards,
  distributionCheck,
  participantTable,
  onClose,
}) => (
  <Drawer
    title={`活动详情：${activity?.name || ''}`}
    placement="right"
    width={1000}
    open={open}
    onClose={onClose}
    styles={{ footer: { paddingTop: 12 } }}
  >
    {activity && (
      <div data-testid="performance-detail-content">
        <Steps
          current={stepItems.findIndex(item => item.status === 'process')}
          items={stepItems}
          style={{ marginBottom: 20 }}
          size="small"
        />
        <Descriptions column={3} size="small" style={{ marginBottom: 16 }} bordered>
          <Descriptions.Item label="状态">
            <StatusTag color={statusColor}>{statusLabel}</StatusTag>
          </Descriptions.Item>
          <Descriptions.Item label="周期类型">{getCycleLabel(activity.cycle_type)}</Descriptions.Item>
          <Descriptions.Item label="流程模板">
            {templateDisplay && (
              <Tooltip title={templateDisplay.tooltip}>
                <Tag color={templateDisplay.color}>
                  {templateDisplay.label}
                </Tag>
              </Tooltip>
            )}
          </Descriptions.Item>
          <Descriptions.Item label="绩效周期">{formatDateTime(activity.start_date)} ~ {formatDateTime(activity.end_date)}</Descriptions.Item>
          <Descriptions.Item label="自评时间">{formatDateTime(activity.self_eval_start_at)} ~ {formatDateTime(activity.self_eval_end_at)}</Descriptions.Item>
          <Descriptions.Item label="主管评分">{formatDateTime(activity.manager_eval_start_at)} ~ {formatDateTime(activity.manager_eval_end_at)}</Descriptions.Item>
          <Descriptions.Item label={activity.flow_type === 'new' ? '结果公布/后续处理' : '结果确认'}>{formatDateTime(activity.result_confirm_start_at)} ~ {formatDateTime(activity.result_confirm_end_at)}</Descriptions.Item>
        </Descriptions>

        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 16 }}>
          {actionButtons}
        </div>

        <Divider style={{ margin: '8px 0 10px' }} orientationMargin={0}>统计摘要</Divider>
        {activity.status === 'hr_confirmation' && hrDeadlineStatus && (
          <Alert
            type={hrDeadlineStatus.overdue ? 'warning' : 'info'}
            showIcon
            style={{ marginBottom: 12 }}
            message={`HR确认截止：${hrDeadlineStatus.deadline || '未设置'}，待确认 ${hrDeadlineStatus.pending_count || 0} 人${hrDeadlineStatus.overdue ? '，已逾期' : ''}`}
          />
        )}

        <Spin spinning={summaryLoading}>
          {summaryCards.length > 0 ? (
            <div style={{ display: 'flex', gap: 0, marginBottom: 10, borderRadius: 'var(--radius-md)', border: '1px solid var(--color-border)', overflow: 'hidden' }}>
              {summaryCards.map((item, idx) => (
                <div key={item.title} style={{
                  flex: 1, padding: '10px 14px', textAlign: 'center',
                  background: item.bg, borderRight: idx < 3 ? '1px solid var(--color-border)' : 'none',
                }}>
                  <div style={{ fontSize: 22, fontWeight: 'var(--font-weight-bold)', color: item.color, lineHeight: 1.2 }}>{item.value}</div>
                  <div style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-secondary)', marginTop: 2 }}>{item.title}</div>
                </div>
              ))}
            </div>
          ) : <Text type="secondary">暂无数据</Text>}
        </Spin>

        {distributionCheck && (
          <Card size="small" style={{ marginBottom: 10 }}>
            <Row gutter={[6, 6]}>
              {['S', 'A', 'B', 'C', 'D'].map(level => {
                const dist = distributionCheck.distribution?.[level]
                if (!dist) return null
                const { bg, barColor } = getDistributionLevelStyle(dist.status)
                return (
                  <Col span={4} key={level} style={{ minWidth: 0 }}>
                    <div style={{
                      textAlign: 'center', padding: '8px 4px', borderRadius: 'var(--radius-md)',
                      background: bg, border: `1px solid ${barColor}20`,
                    }}>
                      <div style={{
                        fontSize: 18, fontWeight: 'var(--font-weight-bold)', color: barColor, lineHeight: 1,
                      }}>{level}</div>
                      <div style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text)', margin: '4px 0 2px' }}>
                        {dist.actual_count}/{dist.expected_count}人
                      </div>
                      <div style={{
                        height: 4, borderRadius: 2, background: 'var(--color-border)',
                        overflow: 'hidden', margin: '0 8px',
                      }}>
                        <div style={{
                          height: '100%', borderRadius: 2, background: barColor,
                          width: `${Math.min(dist.progress, 100)}%`,
                        }} />
                      </div>
                      <div style={{ fontSize: 10, color: 'var(--color-text-tertiary)', marginTop: 3 }}>
                        期望 {dist.expected_percent}%
                      </div>
                    </div>
                  </Col>
                )
              })}
            </Row>
            {!distributionCheck.passed && distributionCheck.warnings?.length > 0 && (
              <Alert
                type="warning"
                showIcon
                message="配额超限"
                description={distributionCheck.warnings.join('；')}
                style={{ marginTop: 6 }}
                closable
              />
            )}
          </Card>
        )}

        <Divider style={{ margin: '12px 0' }}>参与人列表</Divider>
        {participantTable}
      </div>
    )}
  </Drawer>
))

export default PerformanceActivityDetailDrawer
