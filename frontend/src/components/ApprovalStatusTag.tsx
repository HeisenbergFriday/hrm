import React from 'react'
import StatusTag from './StatusTag'

type ApprovalStatusTagProps = {
  status: string
  emptyLabel?: React.ReactNode
}

const APPROVAL_STATUS_META: Record<string, { color?: string; label: string }> = {
  completed: { color: 'green', label: '已完成' },
  approved: { color: 'green', label: '已完成' },
  agree: { color: 'green', label: '已完成' },
  in_progress: { color: 'blue', label: '审批中' },
  running: { color: 'blue', label: '审批中' },
  rejected: { color: 'red', label: '已拒绝' },
  refuse: { color: 'red', label: '已拒绝' },
  terminated: { color: 'red', label: '已终止' },
  canceled: { color: 'red', label: '已取消' },
  cancelled: { color: 'red', label: '已取消' },
  pending: { color: 'orange', label: '待处理' },
}

const ApprovalStatusTag: React.FC<ApprovalStatusTagProps> = ({ status, emptyLabel = '—' }) => {
  const normalizedStatus = String(status || '').toLowerCase()
  const meta = APPROVAL_STATUS_META[normalizedStatus]
  if (meta) {
    return <StatusTag color={meta.color}>{meta.label}</StatusTag>
  }
  return <StatusTag>{status || emptyLabel}</StatusTag>
}

export default ApprovalStatusTag
