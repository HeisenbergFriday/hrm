import React from 'react'
import { Descriptions, Drawer, Empty, Space, Spin, Tag, Timeline, Typography } from 'antd'
import type { PerformanceParticipant, PerformanceReviewVersion } from '../services/api'
import { formatDateTime } from '../utils/format'

const { Text } = Typography

type SelfReviewHistoryDrawerProps = {
  open: boolean
  target: PerformanceParticipant | null
  versions: PerformanceReviewVersion[]
  loading: boolean
  onClose: () => void
}

function getVersionMeta(version: PerformanceReviewVersion) {
  return (version.operation_meta || {}) as Record<string, unknown>
}

function metaText(meta: Record<string, unknown>, key: string) {
  const value = meta[key]
  if (value === undefined || value === null) return ''
  return String(value)
}

function firstPresentValue(...values: unknown[]) {
  return values.find(value => value !== undefined && value !== null && value !== '')
}

function formatRecordText(value?: unknown) {
  if (value === undefined || value === null || value === '') return '-'
  return String(value)
}

function formatReviewScore(value?: unknown) {
  if (value === undefined || value === null || value === '') return '-'
  const score = Number(value)
  return Number.isFinite(score) ? score.toFixed(1) : String(value)
}

function getSelfReviewSnapshotLines(params: {
  score?: unknown
  good?: string
  improvement?: string
  summary?: string
}) {
  const lines = [
    { label: '自评分', value: formatReviewScore(params.score) },
    { label: '做得好的地方', value: params.good || '' },
    { label: '需要改进的地方', value: params.improvement || '' },
    { label: '自评摘要', value: params.summary || '' },
  ]
  return lines.filter(line => line.value && line.value !== '-')
}

function getDepartmentScoreAfter(meta: Record<string, unknown>) {
  return firstPresentValue(meta.new_department_score, meta.baseline_score)
}

function getDepartmentSnapshotLines(meta: Record<string, unknown>, version: PerformanceReviewVersion, type: 'before' | 'after') {
  if (type === 'before') {
    return [
      { label: '最终等级', value: formatRecordText(firstPresentValue(meta.old_final_level, meta.baseline_level)) },
      { label: '最终分', value: formatReviewScore(firstPresentValue(meta.old_department_score, meta.old_adjusted_score, meta.baseline_score)) },
      { label: '主管评分', value: formatReviewScore(firstPresentValue(meta.old_total_manager_score, meta.old_manager_score, version.manager_score)) },
    ]
  }
  return [
    { label: '部门最终等级', value: formatRecordText(firstPresentValue(meta.new_final_level, version.final_level)) },
    { label: '部门最终分', value: formatReviewScore(getDepartmentScoreAfter(meta)) },
  ]
}

function renderRecordSnapshot(title: string, lines: { label: string; value: string }[], emptyText: string) {
  return (
    <div style={{ border: '1px solid var(--color-border-light)', borderRadius: 'var(--radius-sm)', padding: 10, background: 'var(--color-bg-hover)' }}>
      <Text strong>{title}</Text>
      {lines.length === 0 ? (
        <div style={{ marginTop: 6 }}>
          <Text type="secondary">{emptyText}</Text>
        </div>
      ) : (
        <Space direction="vertical" size={4} style={{ width: '100%', marginTop: 6 }}>
          {lines.map(line => (
            <div key={line.label}>
              <Text type="secondary">{line.label}：</Text>
              <Text>{line.value}</Text>
            </div>
          ))}
        </Space>
      )}
    </div>
  )
}

const SelfReviewHistoryDrawer: React.FC<SelfReviewHistoryDrawerProps> = React.memo(({
  open,
  target,
  versions,
  loading,
  onClose,
}) => {
  const timelineItems = React.useMemo(() => {
    const selfVersions = versions.filter(version => version.review_type === 'self')
    const initialSelfReviewVersionId = selfVersions[selfVersions.length - 1]?.id
    return versions.map(version => {
      const meta = getVersionMeta(version)
      if (version.review_type === 'department_evaluation') {
        const departmentAdjusted = meta.department_adjusted === true
        const reason = version.adjust_reason || metaText(meta, 'reason')

        return {
          color: departmentAdjusted ? 'orange' : 'green',
          children: (
            <div data-testid="performance-department-review-version">
              <Space size={8} wrap>
                <Text strong>部门/中心评分</Text>
                <Tag color={departmentAdjusted ? 'orange' : 'green'}>
                  {departmentAdjusted ? '已调整' : '确认不调整'}
                </Tag>
              </Space>
              <Descriptions
                size="small"
                column={1}
                style={{ marginTop: 8 }}
                items={[
                  { key: 'operator', label: '操作人', children: version.created_by || '-' },
                  { key: 'changed_at', label: '记录时间', children: formatDateTime(version.created_at) },
                  { key: 'reason', label: '调整原因', children: reason || '-' },
                ]}
              />
              <Space direction="vertical" size={8} style={{ width: '100%', marginTop: 8 }}>
                {renderRecordSnapshot('评分前', getDepartmentSnapshotLines(meta, version, 'before'), '暂无评分前快照')}
                {renderRecordSnapshot('评分后', getDepartmentSnapshotLines(meta, version, 'after'), '暂无评分后快照')}
              </Space>
            </div>
          ),
        }
      }

      const isManagerRecheck = meta.edit_after_manager_confirm === true
      const isInitial = version.id === initialSelfReviewVersionId
      const evaluationGood = metaText(meta, 'evaluation_good')
      const evaluationImprovement = metaText(meta, 'evaluation_improvement')
      const beforeLines = isInitial ? [] : getSelfReviewSnapshotLines({
        score: firstPresentValue(meta.previous_total_self_score, meta.previous_self_score),
        good: metaText(meta, 'previous_evaluation_good'),
        improvement: metaText(meta, 'previous_evaluation_improvement'),
        summary: metaText(meta, 'previous_self_summary'),
      })
      const afterLines = getSelfReviewSnapshotLines({
        score: version.self_score,
        good: evaluationGood,
        improvement: evaluationImprovement,
        summary: version.self_summary,
      })
      const title = isInitial
        ? '提交自评'
        : isManagerRecheck
          ? '修改自评（主管确认后）'
          : '修改自评'

      return {
        color: isManagerRecheck ? 'orange' : (isInitial ? 'green' : 'blue'),
        children: (
          <div data-testid="performance-self-review-version">
            <Space size={8} wrap>
              <Text strong>{title}</Text>
              <Tag color={isManagerRecheck ? 'orange' : 'default'}>
                主管确认后：{isManagerRecheck ? '是' : '否'}
              </Tag>
            </Space>
            <Descriptions
              size="small"
              column={1}
              style={{ marginTop: 8 }}
              items={[
                { key: 'operator', label: '修改人', children: target?.employee_name || version.created_by || '-' },
                { key: 'changed_at', label: '修改时间', children: formatDateTime(version.created_at) },
                { key: 'after_manager_confirm', label: '主管确认后修改', children: isManagerRecheck ? '是' : '否' },
              ]}
            />
            <Space direction="vertical" size={8} style={{ width: '100%', marginTop: 8 }}>
              {renderRecordSnapshot('修改前', beforeLines, isInitial ? '首次提交，无修改前内容' : '暂无修改前快照')}
              {renderRecordSnapshot(isInitial ? '提交内容' : '修改后', afterLines, '暂无修改后快照')}
            </Space>
          </div>
        ),
      }
    })
  }, [target?.employee_name, versions])

  return (
    <Drawer
      title={`评审记录：${target?.employee_name || '-'}`}
      placement="right"
      width={680}
      open={open}
      onClose={onClose}
    >
      <Spin spinning={loading}>
        {versions.length === 0 && !loading ? (
          <Empty description="暂无评审记录" />
        ) : (
          <Timeline items={timelineItems} />
        )}
      </Spin>
    </Drawer>
  )
})

export default SelfReviewHistoryDrawer
