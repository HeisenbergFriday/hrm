import React from 'react'
import { Alert, Button, Space, Tag, Tooltip, Typography } from 'antd'
import { CloudSyncOutlined, ToolOutlined } from '@ant-design/icons'
import { missingApprovalSyncPermissionTip, type ApprovalSyncNotice } from '../utils/approvalSync'

const { Text } = Typography

type ApprovalSyncStatusProps = {
  notice?: ApprovalSyncNotice | null
  isSyncing: boolean
  canSync: boolean
  processCode?: string
  disabledReason?: string
  onManualSync: () => void
}

const ApprovalSyncStatus: React.FC<ApprovalSyncStatusProps> = ({
  notice,
  isSyncing,
  canSync,
  processCode,
  disabledReason,
  onManualSync,
}) => (
  <div className="approval-sync-section">
    <div className="approval-sync-status">
      <div className="approval-sync-status-icon" aria-hidden="true">
        <CloudSyncOutlined />
      </div>
      <div className="approval-sync-status-copy">
        <Space size={8} wrap>
          <Text strong>审批数据自动同步</Text>
          <Tag color="processing">实时更新</Tag>
          <Tag>15 分钟补偿</Tag>
        </Space>
        <Text type="secondary">
          新增和状态变化会自动进入系统；只有发现遗漏或需要补拉历史数据时，才使用手动补拉。
        </Text>
      </div>
      <Tooltip title={disabledReason || (canSync ? '从钉钉重新拉取所选范围，适用于异常遗漏或历史数据补录' : missingApprovalSyncPermissionTip)}>
        <span>
          <Button
            icon={<ToolOutlined />}
            loading={isSyncing}
            disabled={!canSync || Boolean(disabledReason) || isSyncing}
            onClick={onManualSync}
          >
            {isSyncing ? '补拉中' : (processCode ? '补拉当前模板' : '手动补拉')}
          </Button>
        </span>
      </Tooltip>
    </div>
    {notice && (
      <Alert
        className="approval-sync-notice"
        type={notice.type}
        message={notice.message}
        description={notice.description}
        showIcon
      />
    )}
  </div>
)

export default ApprovalSyncStatus
