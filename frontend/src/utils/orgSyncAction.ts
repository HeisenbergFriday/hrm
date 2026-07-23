import { Modal, message } from 'antd'
import { orgAPI } from '../services/api'
import { hasPermission } from './permission'

export const ORG_SYNC_PERMISSION = 'attendance_manage'

export function canSyncOrgData(): boolean {
  return hasPermission(ORG_SYNC_PERMISSION)
}

export const missingOrgSyncPermissionTip = '你缺少 attendance_manage 权限，需要联系管理员添加'

/**
 * 统一组织花名册同步：权限门闩 + Modal.confirm + orgAPI.syncOrg。
 * 不改变同步 API / 权限码语义，仅统一前端交互。
 */
export function confirmOrgSync(options?: {
  title?: string
  content?: string
  successMessage?: string
  errorMessage?: string
  onSuccess?: () => void | Promise<void>
}): void {
  if (!canSyncOrgData()) {
    message.warning(missingOrgSyncPermissionTip)
    return
  }

  Modal.confirm({
    title: options?.title || '同步组织数据',
    content:
      options?.content ||
      '将从钉钉重新拉取部门与员工数据并写入本系统，可能耗时较长。确认开始同步？',
    okText: '确认同步',
    cancelText: '取消',
    onOk: async () => {
      try {
        await orgAPI.syncOrg()
        message.success(options?.successMessage || '组织数据同步成功')
        if (options?.onSuccess) {
          await options.onSuccess()
        }
      } catch {
        message.error(options?.errorMessage || '组织数据同步失败')
        return Promise.reject(new Error('org sync failed'))
      }
    },
  })
}
