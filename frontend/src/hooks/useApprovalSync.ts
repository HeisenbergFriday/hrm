import { useCallback, useEffect, useRef, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import {
  approvalAPI,
  getPendingApprovalSyncRequestID,
  type ApprovalSyncAPIResponse,
  type ApprovalSyncResult,
} from '../services/api'
import { hasPermission } from '../utils/permission'
import {
  approvalSyncErrorNotice,
  approvalSyncResultNotice,
  approvalSyncRunningNotice,
  type ApprovalSyncNotice,
} from '../utils/approvalSync'

type UseApprovalSyncOptions = {
  processCode?: string
  startDate?: string
  endDate?: string
  onCompleted?: (result: ApprovalSyncResult) => void
}

export function useApprovalSync({ processCode, startDate, endDate, onCompleted }: UseApprovalSyncOptions = {}) {
  const [notice, setNotice] = useState<ApprovalSyncNotice | null>(null)
  const syncInFlightRef = useRef(false)
  const onCompletedRef = useRef(onCompleted)
  onCompletedRef.current = onCompleted
  const canSync = hasPermission('approval:sync')

  const syncMutation = useMutation<ApprovalSyncAPIResponse, Error, boolean>({
    mutationFn: (resume) => resume
      ? approvalAPI.resumeSync()
      : approvalAPI.sync({
        process_code: processCode || undefined,
        start_date: startDate || undefined,
        end_date: endDate || undefined,
      }),
    onSuccess: (response) => {
      setNotice(approvalSyncResultNotice(response.data))
      if (response.data.status === 'success' || response.data.status === 'partial') {
        onCompletedRef.current?.(response.data)
      }
    },
    onError: (error) => {
      setNotice(approvalSyncErrorNotice(error))
    },
    onSettled: () => {
      syncInFlightRef.current = false
    },
  })

  const resumeSync = syncMutation.mutate
  useEffect(() => {
    if (!canSync || !getPendingApprovalSyncRequestID() || syncInFlightRef.current) return
    syncInFlightRef.current = true
    setNotice(approvalSyncRunningNotice())
    resumeSync(true)
  }, [canSync, resumeSync])

  const startManualSync = useCallback(() => {
    if (!canSync || syncInFlightRef.current) return
    syncInFlightRef.current = true
    setNotice(approvalSyncRunningNotice(processCode))
    syncMutation.mutate(false)
  }, [canSync, processCode, syncMutation])

  return {
    canSync,
    isSyncing: syncMutation.isPending,
    notice,
    startManualSync,
  }
}
