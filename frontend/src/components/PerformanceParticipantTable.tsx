import React from 'react'
import { Spin, Table } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { PerformanceParticipant } from '../services/api'

type PerformanceParticipantTableProps = {
  columns: ColumnsType<PerformanceParticipant>
  participants: PerformanceParticipant[]
  loading: boolean
  selectedParticipantIds: React.Key[]
  onSelectionChange: (selectedRowKeys: React.Key[]) => void
  selectable: boolean
  scrollX: number
  scrollY?: number
  virtual?: boolean
}

const PerformanceParticipantTable: React.FC<PerformanceParticipantTableProps> = React.memo(({
  columns,
  participants,
  loading,
  selectedParticipantIds,
  onSelectionChange,
  selectable,
  scrollX,
  scrollY,
  virtual,
}) => (
  <Spin spinning={loading}>
    <Table
      columns={columns}
      dataSource={participants}
      rowKey="id"
      rowSelection={selectable ? {
        selectedRowKeys: selectedParticipantIds,
        onChange: onSelectionChange,
      } : undefined}
      pagination={{ pageSize: 10, size: 'small' }}
      size="small"
      scroll={{ x: scrollX, y: scrollY }}
      virtual={virtual}
    />
  </Spin>
))

export default PerformanceParticipantTable
