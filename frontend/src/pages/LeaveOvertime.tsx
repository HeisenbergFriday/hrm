import React, { useState } from 'react'
import {
  Alert,
  Button,
  Col,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Steps,
  Table,
  Tabs,
  Tooltip,
  Typography,
  message,
} from 'antd'
import { ClockCircleOutlined, DeleteOutlined, GiftOutlined, MinusCircleOutlined, ReloadOutlined, SearchOutlined, SyncOutlined, ThunderboltOutlined, CalendarOutlined } from '@ant-design/icons'
import { useMutation, useQuery } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { leaveAPI, orgAPI, overtimeAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import { formatDateTime } from '../utils/format'

const { Title, Text } = Typography

const formatWorkingYears = (value?: number) =>
  Number.isFinite(value) ? Number(value).toFixed(1) : '0.0'

const formatDays = (value?: number) =>
  `${Number.isFinite(value) ? Number(value) : 0} 天`

const EmployeeSelect: React.FC<{
  value?: string
  onChange?: (userId: string) => void
  style?: React.CSSProperties
}> = ({ value, onChange, style }) => {
  const { data } = useQuery({
    queryKey: ['employees-all'],
    queryFn: () => orgAPI.getEmployees({ page: 1, page_size: 500 }),
    staleTime: 60_000,
  })

  const employees: any[] = (data as any)?.data?.items ?? []

  return (
    <Select
      showSearch
      allowClear
      placeholder="输入姓名搜索"
      value={value || undefined}
      onChange={onChange}
      filterOption={(input, opt) =>
        ((opt?.label as string) ?? '').toLowerCase().includes(input.toLowerCase())
      }
      options={employees
        .filter((employee: any) => employee?.user_id && employee.user_id !== 'admin')
        .map((employee: any) => ({
          value: employee.user_id,
          label: employee.name,
        }))}
      style={{ width: 160, ...style }}
    />
  )
}

const EligibilityTab: React.FC = () => {
  const [userID, setUserID] = useState('')
  const [year, setYear] = useState(dayjs().year())
  const [queryKey, setQueryKey] = useState<{ user_id: string; year: number } | null>(null)

  const { data, isFetching } = useQuery({
    queryKey: ['leave-eligibility', queryKey],
    queryFn: () => leaveAPI.getEligibility(queryKey!),
    enabled: !!queryKey,
  })

  const recalcMutation = useMutation({
    mutationFn: () => leaveAPI.recalculateEligibility({ user_id: userID, year }),
    onSuccess: () => {
      message.success('资格重算完成')
      setQueryKey({ user_id: userID, year })
    },
    onError: () => message.error('资格重算失败'),
  })

  const columns = [
    { title: '季度', dataIndex: 'quarter', key: 'quarter', render: (q: number) => `Q${q}` },
    {
      title: '是否有资格', dataIndex: 'is_eligible', key: 'is_eligible',
      render: (value: boolean) => (
        <StatusTag color={value ? 'success' : 'error'}>
          {value ? '有资格' : '无资格'}
        </StatusTag>
      ),
    },
    { title: '入职日期', dataIndex: 'entry_date', key: 'entry_date' },
    { title: '转正日期', dataIndex: 'confirmation_date', key: 'confirmation_date' },
    {
      title: '追溯来源季度', dataIndex: 'retroactive_source_quarter', key: 'retroactive_source_quarter',
      render: (value: number) => (value ? `Q${value}` : '-'),
    },
    { title: '计算原因', dataIndex: 'calc_reason', key: 'calc_reason' },
  ]

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <EmployeeSelect value={userID} onChange={(next) => setUserID(next ?? '')} />
        <InputNumber
          value={year}
          onChange={(next) => setYear(next ?? dayjs().year())}
          min={2020}
          max={2030}
          style={{ width: 100 }}
        />
        <Button type="primary" icon={<SearchOutlined />} onClick={() => setQueryKey({ user_id: userID, year })} disabled={!userID}>
          查询
        </Button>
        <Button icon={<SyncOutlined />} onClick={() => recalcMutation.mutate()} loading={recalcMutation.isPending} disabled={!userID}>
          重算资格
        </Button>
      </Space>
      <Table columns={columns} dataSource={(data as any)?.data || []} rowKey="quarter" loading={isFetching} pagination={false} />
    </div>
  )
}

const syncStatusColor: Record<string, string> = {
  success: 'green', failed: 'red', skipped: 'default', pending: 'orange',
}
const syncStatusLabel: Record<string, string> = {
  success: '已同步', failed: '失败', skipped: '未同步', pending: '待同步',
}

const GrantTab: React.FC = () => {
  const [userID, setUserID] = useState('')
  const [year, setYear] = useState(dayjs().year())
  const [queryKey, setQueryKey] = useState<{ user_id: string; year: number } | null>(null)
  const [batchModalOpen, setBatchModalOpen] = useState(false)
  const [batchForm] = Form.useForm()

  const { data, isFetching, refetch } = useQuery({
    queryKey: ['leave-grants', queryKey],
    queryFn: () => leaveAPI.getGrants(queryKey!),
    enabled: !!queryKey,
  })

  const runQuarterMutation = useMutation({
    mutationFn: (values: { year: number; quarter: number }) => leaveAPI.runQuarterGrant(values),
    onSuccess: (res: any) => {
      message.success(res?.message || '季度发放完成')
      setBatchModalOpen(false)
      refetch()
    },
    onError: () => message.error('发放失败'),
  })

  const regrantMutation = useMutation({
    mutationFn: () => leaveAPI.regrant({ user_id: userID, year }),
    onSuccess: (res: any) => {
      message.success(res?.message || '追溯补发完成')
      refetch()
    },
    onError: () => message.error('追溯补发失败'),
  })

  const syncMutation = useMutation({
    mutationFn: () => leaveAPI.syncToDingTalk(),
    onSuccess: (res: any) => {
      const d = (res as any)?.data
      message.success(
        `同步完成：成功 ${d?.dingtalk_synced_count ?? 0} 条，失败 ${d?.dingtalk_failed_count ?? 0} 条，共 ${d?.total_days ?? 0} 天`
      )
      if (queryKey) refetch()
    },
    onError: (err: any) => message.error(err?.response?.data?.error || '同步失败'),
  })

  const handleSyncToDingTalk = () => {
    Modal.confirm({
      title: '同步年假余额到钉钉',
      content: '将把所有未同步的发放记录写入钉钉假期余额（增量叠加）。请确认这些记录之前未曾同步过，否则余额会重复计入。确认执行？',
      okText: '确认同步',
      cancelText: '取消',
      onOk: () => syncMutation.mutateAsync(),
    })
  }

  const typeColor: Record<string, string> = { normal: 'blue', retroactive: 'orange', adjustment: 'purple' }
  const typeLabel: Record<string, string> = { normal: '正常发放', retroactive: '追溯补发', adjustment: '调整' }

  const columns = [
    { title: '季度', dataIndex: 'quarter', key: 'quarter', render: (q: number) => `Q${q}` },
    {
      title: '类型', dataIndex: 'grant_type', key: 'grant_type',
      render: (value: string) => <StatusTag color={typeColor[value] || 'default'}>{typeLabel[value] || value}</StatusTag>,
    },
    { title: '工龄(年)', dataIndex: 'working_years', key: 'working_years', render: (value?: number) => formatWorkingYears(value) },
    { title: '基础天数', dataIndex: 'base_days', key: 'base_days', render: (value?: number) => formatDays(value) },
    { title: '本次发放', dataIndex: 'granted_days', key: 'granted_days', render: (value?: number) => formatDays(value) },
    { title: '已用', dataIndex: 'used_days', key: 'used_days', render: (value?: number) => formatDays(value) },
    { title: '剩余', dataIndex: 'remaining_days', key: 'remaining_days', render: (value?: number) => formatDays(value) },
    { title: '备注', dataIndex: 'remark', key: 'remark' },
    {
      title: '钉钉同步', dataIndex: 'dingtalk_sync_status', key: 'dingtalk_sync_status',
      render: (value: string) => (
        <StatusTag color={syncStatusColor[value] || 'default'}>{syncStatusLabel[value] || value || '-'}</StatusTag>
      ),
    },
  ]

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <EmployeeSelect value={userID} onChange={(next) => setUserID(next ?? '')} />
        <InputNumber value={year} onChange={(next) => setYear(next ?? dayjs().year())} min={2020} max={2030} style={{ width: 100 }} />
        <Button type="primary" icon={<SearchOutlined />} onClick={() => setQueryKey({ user_id: userID, year })} disabled={!userID}>
          查询
        </Button>
        <Button icon={<GiftOutlined />} onClick={() => setBatchModalOpen(true)}>手动发放季度年假</Button>
        <Button icon={<SyncOutlined />} onClick={() => regrantMutation.mutate()} loading={regrantMutation.isPending} disabled={!userID}>
          追溯补发
        </Button>
        <Button icon={<SyncOutlined />} onClick={handleSyncToDingTalk} loading={syncMutation.isPending}>
          同步到钉钉
        </Button>
      </Space>
      <Table columns={columns} dataSource={(data as any)?.data || []} rowKey="id" loading={isFetching} pagination={false} />
      <Modal
        title="手动发放季度年假"
        open={batchModalOpen}
        onCancel={() => setBatchModalOpen(false)}
        onOk={() => batchForm.submit()}
        confirmLoading={runQuarterMutation.isPending}
      >
        <Form
          form={batchForm}
          layout="vertical"
          initialValues={{ year: dayjs().year(), quarter: Math.ceil((dayjs().month() + 1) / 3) }}
          onFinish={(values) => runQuarterMutation.mutate(values)}
        >
          <Form.Item name="year" label="年份" rules={[{ required: true }]}>
            <InputNumber min={2020} max={2030} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="quarter" label="季度" rules={[{ required: true }]}>
            <Select>
              {[1, 2, 3, 4].map((quarter) => (
                <Select.Option key={quarter} value={quarter}>Q{quarter}</Select.Option>
              ))}
            </Select>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

const OvertimeTab: React.FC = () => {
  const [userID, setUserID] = useState('')
  const [selectedMonth, setSelectedMonth] = useState(dayjs().startOf('month'))
  const [queryKey, setQueryKey] = useState<{ user_id: string; start_date: string; end_date: string } | null>(null)
  const [suppModalOpen, setSuppModalOpen] = useState(false)
  const [suppMatchRecord, setSuppMatchRecord] = useState<any>(null)
  const [suppForm] = Form.useForm()

  const buildOvertimeQuery = () => ({
    user_id: userID,
    start_date: selectedMonth.startOf('month').format('YYYY-MM-DD'),
    end_date: selectedMonth.endOf('month').format('YYYY-MM-DD'),
  })

  const { data, isFetching, refetch } = useQuery({
    queryKey: ['overtime-matches', queryKey],
    queryFn: () => overtimeAPI.getMatches(queryKey!),
    enabled: !!queryKey,
  })

  const refreshOvertimeMatches = () => {
    const next = buildOvertimeQuery()
    if (
      queryKey &&
      queryKey.user_id === next.user_id &&
      queryKey.start_date === next.start_date &&
      queryKey.end_date === next.end_date
    ) {
      void refetch()
      return
    }
    setQueryKey(next)
  }

  const clearRematchMutation = useMutation({
    mutationFn: () =>
      overtimeAPI.clearAndRematch({
        user_id: userID || undefined,
        start_date: selectedMonth.startOf('month').format('YYYY-MM-DD'),
        end_date: selectedMonth.endOf('month').format('YYYY-MM-DD'),
      }),
    onSuccess: () => {
      message.success('清空并重新匹配完成')
      if (userID) refreshOvertimeMatches()
    },
    onError: (err: any) => message.error(err?.response?.data?.error || '清空重匹配失败'),
  })

  const deleteMatchesMutation = useMutation({
    mutationFn: () =>
      overtimeAPI.deleteMatches({
        user_id: userID || undefined,
        start_date: selectedMonth.startOf('month').format('YYYY-MM-DD'),
        end_date: selectedMonth.endOf('month').format('YYYY-MM-DD'),
      }),
    onSuccess: (res: any) => {
      message.success(res?.message || '删除完成')
      if (userID) refreshOvertimeMatches()
    },
    onError: (err: any) => message.error(err?.response?.data?.error || '删除失败'),
  })

  const submitSuppMutation = useMutation({
    mutationFn: (data: { match_result_id: number; clock_in: string; clock_out: string; reason?: string }) =>
      overtimeAPI.submitSupplementary(data),
    onSuccess: (res: any) => {
      message.success(res?.message || '补卡申请已提交')
      setSuppModalOpen(false)
      suppForm.resetFields()
      refreshOvertimeMatches()
    },
    onError: (err: any) => message.error(err?.response?.data?.error || '提交补卡申请失败'),
  })

  const handleOpenSuppModal = (record: any) => {
    setSuppMatchRecord(record)
    suppForm.resetFields()
    setSuppModalOpen(true)
  }

  const handleSubmitSupp = async () => {
    try {
      const values = await suppForm.validateFields()
      await submitSuppMutation.mutateAsync({
        match_result_id: suppMatchRecord.id,
        clock_in: values.clock_in.format('YYYY-MM-DD HH:mm'),
        clock_out: values.clock_out.format('YYYY-MM-DD HH:mm'),
        reason: values.reason,
      })
    } catch {}
  }

  const handleClearRematch = () => {
    Modal.confirm({
      title: '清空并重新匹配',
      content: `将删除所选月份内该员工的匹配记录，然后重新执行匹配。确认？`,
      okText: '确认',
      cancelText: '取消',
      onOk: () => clearRematchMutation.mutateAsync(),
    })
  }

  const handleDeleteMatches = () => {
    Modal.confirm({
      title: '删除匹配记录',
      content: `将删除所选月份内该员工的匹配记录（不重新匹配）。确认？`,
      okText: '确认删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: () => deleteMatchesMutation.mutateAsync(),
    })
  }

  const runMatchMutation = useMutation({
    mutationFn: () =>
      overtimeAPI.runMatch({
        user_id: userID || undefined,
        start_date: selectedMonth.startOf('month').format('YYYY-MM-DD'),
        end_date: selectedMonth.endOf('month').format('YYYY-MM-DD'),
      }),
    onSuccess: () => {
      message.success('加班匹配完成')
      if (userID) refreshOvertimeMatches()
    },
    onError: () => message.error('匹配失败'),
  })

  // ---- ManualLeave 同步向导 ----
  const [wizardOpen, setWizardOpen] = useState(false)
  const [wizardStep, setWizardStep] = useState(0)
  const [previewReset, setPreviewReset] = useState<{ count: number; users: { user_id: string; name: string }[] } | null>(null)
  const [resetResult, setResetResult] = useState<{ success: number; failed: number; errors: string[] } | null>(null)
  const [previewResync, setPreviewResync] = useState<{ count: number; records: any[] } | null>(null)
  const [resyncResult, setResyncResult] = useState<{ success: number; failed: number; errors: string[] } | null>(null)

  const openWizard = () => {
    setWizardStep(0)
    setPreviewReset(null)
    setResetResult(null)
    setPreviewResync(null)
    setResyncResult(null)
    setWizardOpen(true)
  }

  const loadPreviewResetMut = useMutation({
    mutationFn: () => overtimeAPI.resetManualLeave({ dry_run: true }),
    onSuccess: (res: any) => setPreviewReset(res),
    onError: (err: any) => message.error(err?.response?.data?.error || '预览失败'),
  })

  const execResetMut = useMutation({
    mutationFn: () => overtimeAPI.resetManualLeave({ dry_run: false }),
    onSuccess: (res: any) => {
      setResetResult(res)
      setWizardStep(2)
      loadPreviewResyncMut.mutate()
    },
    onError: (err: any) => message.error(err?.response?.data?.error || '重置失败'),
  })

  const loadPreviewResyncMut = useMutation({
    mutationFn: () => overtimeAPI.resyncOvertimeToDingTalk({ dry_run: true }),
    onSuccess: (res: any) => setPreviewResync(res),
    onError: (err: any) => message.error(err?.response?.data?.error || '预览失败'),
  })

  const execResyncMut = useMutation({
    mutationFn: () => overtimeAPI.resyncOvertimeToDingTalk({ dry_run: false }),
    onSuccess: (res: any) => {
      setResyncResult(res)
      setWizardStep(3)
    },
    onError: (err: any) => message.error(err?.response?.data?.error || '重放失败'),
  })

  const handleWizardOpen = () => {
    openWizard()
    loadPreviewResetMut.mutate()
  }

  const wizardFooter = () => {
    if (wizardStep === 0) {
      return [
        <Button key="cancel" onClick={() => setWizardOpen(false)}>取消</Button>,
        <Button
          key="confirm"
          type="primary"
          danger
          loading={execResetMut.isPending}
          disabled={!previewReset || loadPreviewResetMut.isPending}
          onClick={() => { setWizardStep(1); execResetMut.mutate() }}
        >
          确认重置 {previewReset ? previewReset.count : '…'} 名员工余额
        </Button>,
      ]
    }
    if (wizardStep === 1) return []
    if (wizardStep === 2) {
      return [
        <Button key="cancel" onClick={() => setWizardOpen(false)}>取消</Button>,
        <Button
          key="confirm"
          type="primary"
          loading={execResyncMut.isPending}
          disabled={!previewResync || loadPreviewResyncMut.isPending}
          onClick={() => { setWizardStep(3); execResyncMut.mutate() }}
        >
          确认重放 {previewResync ? previewResync.count : '…'} 条记录
        </Button>,
      ]
    }
    return [
      <Button key="done" type="primary" onClick={() => setWizardOpen(false)}>完成</Button>,
    ]
  }

  const previewResetColumns = [
    { title: '员工ID', dataIndex: 'user_id', key: 'user_id' },
    { title: '姓名', dataIndex: 'name', key: 'name' },
  ]
  const previewResyncColumns = [
    { title: '员工ID', dataIndex: 'user_id', key: 'user_id' },
    { title: '加班日期', dataIndex: 'work_date', key: 'work_date' },
    { title: '有效加班(分钟)', dataIndex: 'minutes', key: 'minutes' },
    { title: '当前同步状态', dataIndex: 'status', key: 'status' },
  ]

  const statusColor: Record<string, string> = {
    matched: 'green', synced: 'blue', no_clock_record: 'red', insufficient_clock_record: 'orange',
    invalid_clock_time: 'red', zero_overtime: 'default', local_balance_failed: 'red',
    dingtalk_sync_failed: 'volcano', rolled_back: 'default',
  }
  const statusLabel: Record<string, string> = {
    matched: '已匹配', synced: '已同步', no_clock_record: '无打卡', insufficient_clock_record: '打卡不足',
    invalid_clock_time: '打卡异常', zero_overtime: '无有效调休', local_balance_failed: '本地余额失败',
    dingtalk_sync_failed: '钉钉同步失败', rolled_back: '已回滚',
  }

  const localBalanceLabel: Record<string, string> = { success: '已入账', failed: '入账失败', skipped: '未启用', pending: '待处理' }
  const localBalanceColor: Record<string, string> = { success: 'green', failed: 'red', skipped: 'default', pending: 'orange' }
  const dingtalkSyncLabel: Record<string, string> = { success: '已同步', failed: '同步失败', skipped: '未启用', pending: '待同步' }
  const dingtalkSyncColor: Record<string, string> = { success: 'blue', failed: 'red', skipped: 'default', pending: 'orange' }

  const columns = [
    { title: '员工', dataIndex: 'user_name', key: 'user_name', render: (value: string, record: any) => value || record.user_id },
    { title: '加班日期', dataIndex: 'work_date', key: 'work_date' },
    { title: '审批ID', dataIndex: 'approval_id', key: 'approval_id' },
    {
      title: '状态', dataIndex: 'match_status', key: 'match_status',
      render: (value: string, record: any) => (
        <Space size={4}>
          <StatusTag color={statusColor[value] || 'default'}>{statusLabel[value] || value}</StatusTag>
          {(value === 'no_clock_record' || value === 'insufficient_clock_record') && (
            <Button size="small" type="link" onClick={() => handleOpenSuppModal(record)}>补卡</Button>
          )}
        </Space>
      ),
    },
    { title: '提交时间', dataIndex: 'approval_start_time', key: 'approval_start_time', render: formatDateTime },
    { title: '通过时间', dataIndex: 'approval_end_time', key: 'approval_end_time', render: formatDateTime },
    { title: '审批耗时(分钟)', dataIndex: 'approval_duration_minutes', key: 'approval_duration_minutes' },
    { title: '预计加班开始', dataIndex: 'overtime_start_time', key: 'overtime_start_time', render: formatDateTime },
    { title: '预计加班结束', dataIndex: 'overtime_end_time', key: 'overtime_end_time', render: formatDateTime },
    { title: '预计时长(分钟)', dataIndex: 'overtime_duration_minutes', key: 'overtime_duration_minutes' },
    { title: '首次实际打卡', dataIndex: 'actual_first_clock_time', key: 'actual_first_clock_time', render: formatDateTime },
    { title: '末次实际打卡', dataIndex: 'actual_last_clock_time', key: 'actual_last_clock_time', render: formatDateTime },
    { title: '打卡跨度(分钟)', dataIndex: 'actual_clock_span_minutes', key: 'actual_clock_span_minutes' },
    { title: '休息扣除(分钟)', dataIndex: 'break_deduct_minutes', key: 'break_deduct_minutes' },
    { title: '最终调休(分钟)', dataIndex: 'effective_overtime_minutes', key: 'effective_overtime_minutes' },
    {
      title: '本地余额', dataIndex: 'local_balance_status', key: 'local_balance_status',
      render: (value: string) => value ? <StatusTag color={localBalanceColor[value] || 'default'}>{localBalanceLabel[value] || value}</StatusTag> : '-',
    },
    {
      title: '钉钉同步', dataIndex: 'dingtalk_sync_status', key: 'dingtalk_sync_status',
      render: (value: string) => value ? <StatusTag color={dingtalkSyncColor[value] || 'default'}>{dingtalkSyncLabel[value] || value}</StatusTag> : '-',
    },
    {
      title: '匹配说明', dataIndex: 'match_reason', key: 'match_reason',
      render: (value: string) => value ? <Tooltip title={value}><span style={{ cursor: 'help' }}>{value.length > 30 ? value.slice(0, 30) + '…' : value}</span></Tooltip> : '-',
    },
  ]

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <EmployeeSelect value={userID} onChange={(next) => setUserID(next ?? '')} />
        <DatePicker picker="month" value={selectedMonth} allowClear={false} onChange={(next) => next && setSelectedMonth(next.startOf('month'))} />
        <Button type="primary" icon={<SearchOutlined />} onClick={refreshOvertimeMatches} disabled={!userID}>查询</Button>
        <Button icon={<SyncOutlined />} onClick={() => runMatchMutation.mutate()} loading={runMatchMutation.isPending} disabled={!userID}>执行加班匹配</Button>
        <Button icon={<ThunderboltOutlined />} onClick={handleWizardOpen}>手动调休同步</Button>
        <Button icon={<ReloadOutlined />} onClick={handleClearRematch} loading={clearRematchMutation.isPending} disabled={!userID} danger>清空重匹配</Button>
        <Button icon={<DeleteOutlined />} onClick={handleDeleteMatches} loading={deleteMatchesMutation.isPending} disabled={!userID} danger>删除记录</Button>
      </Space>
      <Table columns={columns} dataSource={(data as any)?.data || []} rowKey="id" loading={isFetching} scroll={{ x: 1600 }} pagination={{ pageSize: 20, showSizeChanger: false }} />

      <Modal
        title="ManualLeave 同步向导"
        open={wizardOpen}
        onCancel={() => setWizardOpen(false)}
        footer={wizardFooter()}
        width={720}
        maskClosable={false}
      >
        <Steps
          current={wizardStep}
          items={[{ title: '预览员工' }, { title: '重置余额' }, { title: '预览记录' }, { title: '重放到钉钉' }]}
          style={{ marginBottom: 24 }}
        />
        {wizardStep === 0 && (
          <Spin spinning={loadPreviewResetMut.isPending}>
            {previewReset && (
              <>
                <Alert type="warning" message={`将把以下 ${previewReset.count} 名员工的 ManualLeave 余额重置为 0`} style={{ marginBottom: 12 }} />
                <Table size="small" columns={previewResetColumns} dataSource={previewReset.users} rowKey="user_id" pagination={{ pageSize: 8 }} />
              </>
            )}
          </Spin>
        )}
        {wizardStep === 1 && (
          <div style={{ textAlign: 'center', padding: '40px 0' }}>
            <Spin spinning={execResetMut.isPending} tip="正在重置余额，请稍候…" />
          </div>
        )}
        {wizardStep === 2 && (
          <Spin spinning={loadPreviewResyncMut.isPending}>
            {resetResult && (
              <Alert
                type={resetResult.failed > 0 ? 'warning' : 'success'}
                message={`重置完成：成功 ${resetResult.success}，失败 ${resetResult.failed}`}
                description={resetResult.errors.length > 0 ? resetResult.errors.join('；') : undefined}
                style={{ marginBottom: 12 }}
              />
            )}
            {previewResync && (
              <>
                <Alert type="info" message={`将把以下 ${previewResync.count} 条有效加班记录重放到钉钉`} style={{ marginBottom: 12 }} />
                <Table size="small" columns={previewResyncColumns} dataSource={previewResync.records} rowKey={(r: any) => `${r.user_id}-${r.work_date}`} pagination={{ pageSize: 8 }} />
              </>
            )}
          </Spin>
        )}
        {wizardStep === 3 && (
          resyncResult ? (
            <Alert
              type={resyncResult.failed > 0 ? 'warning' : 'success'}
              message={`重放完成：成功 ${resyncResult.success}，失败 ${resyncResult.failed}，合计 ${resyncResult.success + resyncResult.failed}`}
              description={resyncResult.errors.length > 0 ? resyncResult.errors.join('；') : undefined}
            />
          ) : (
            <div style={{ textAlign: 'center', padding: '40px 0' }}>
              <Spin spinning tip="正在同步到钉钉，请稍候…" />
            </div>
          )
        )}
      </Modal>

      <Modal
        title="提交补卡申请"
        open={suppModalOpen}
        onCancel={() => setSuppModalOpen(false)}
        onOk={handleSubmitSupp}
        confirmLoading={submitSuppMutation.isPending}
        okText="提交"
        cancelText="取消"
      >
        {suppMatchRecord && (
          <Alert
            type="info"
            message={`${suppMatchRecord.user_name || suppMatchRecord.user_id} - ${suppMatchRecord.work_date} 加班审批${suppMatchRecord.approval_id}`}
            style={{ marginBottom: 16 }}
          />
        )}
        <Form form={suppForm} layout="vertical">
          <Form.Item name="clock_in" label="补卡上班时间" rules={[{ required: true, message: '请选择补卡上班时间' }]}>
            <DatePicker showTime format="YYYY-MM-DD HH:mm" style={{ width: '100%' }} placeholder="选择上班打卡时间" />
          </Form.Item>
          <Form.Item name="clock_out" label="补卡下班时间" rules={[{ required: true, message: '请选择补卡下班时间' }]}>
            <DatePicker showTime format="YYYY-MM-DD HH:mm" style={{ width: '100%' }} placeholder="选择下班打卡时间" />
          </Form.Item>
          <Form.Item name="reason" label="补卡原因">
            <Input.TextArea rows={3} placeholder="请输入补卡原因（选填）" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

const CompBalanceTab: React.FC = () => {
  const [userID, setUserID] = useState('')
  const [queryUserID, setQueryUserID] = useState('')

  const { data, isFetching } = useQuery({
    queryKey: ['comp-balance', queryUserID],
    queryFn: () => overtimeAPI.getCompBalance({ user_id: queryUserID }),
    enabled: !!queryUserID,
  })

  const balance = (data as any)?.data

  return (
    <div>
      <Space style={{ marginBottom: 24 }}>
        <EmployeeSelect value={userID} onChange={(next) => setUserID(next ?? '')} />
        <Button type="primary" icon={<SearchOutlined />} onClick={() => setQueryUserID(userID)} disabled={!userID} loading={isFetching}>
          查询
        </Button>
      </Space>
      {balance && (
        <Row gutter={[32, 20]}>
          <Col>
            <div style={{ background: '#f0fdf4', borderRadius: 'var(--radius-lg)', padding: '18px 24px', border: '1px solid #bbf7d0' }}>
              <Text style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-sm)', fontWeight: 'var(--font-weight-medium)' }}>累计调休</Text>
              <div style={{ fontSize: 28, fontWeight: 700, color: '#15803d', marginTop: 4 }}>
                {balance.total_credit_minutes ?? 0} <span style={{ fontSize: 'var(--font-size-base)', fontWeight: 'var(--font-weight-medium)' }}>分钟</span>
              </div>
            </div>
          </Col>
          <Col>
            <div style={{ background: '#fef2f2', borderRadius: 'var(--radius-lg)', padding: '18px 24px', border: '1px solid #fecaca' }}>
              <Text style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-sm)', fontWeight: 'var(--font-weight-medium)' }}>已用调休</Text>
              <div style={{ fontSize: 28, fontWeight: 700, color: '#dc2626', marginTop: 4 }}>
                {balance.total_debit_minutes ?? 0} <span style={{ fontSize: 'var(--font-size-base)', fontWeight: 'var(--font-weight-medium)' }}>分钟</span>
              </div>
            </div>
          </Col>
          <Col>
            <div style={{ background: '#eef2ff', borderRadius: 'var(--radius-lg)', padding: '18px 24px', border: '1px solid #c7d2fe' }}>
              <Text style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-sm)', fontWeight: 'var(--font-weight-medium)' }}>剩余调休</Text>
              <div style={{ fontSize: 28, fontWeight: 700, color: 'var(--color-primary)', marginTop: 4 }}>
                {balance.balance_minutes ?? 0} <span style={{ fontSize: 'var(--font-size-base)', fontWeight: 'var(--font-weight-medium)' }}>分钟（约 {((balance.balance_minutes ?? 0) / 60).toFixed(1)} 小时）</span>
              </div>
            </div>
          </Col>
        </Row>
      )}
    </div>
  )
}

const ConsumeTab: React.FC = () => {
  const [logUserID, setLogUserID] = useState('')
  const [logQueryKey, setLogQueryKey] = useState('')
  const [form] = Form.useForm()

  const { data: logData, isFetching: logFetching, refetch: refetchLog } = useQuery({
    queryKey: ['consume-log', logQueryKey],
    queryFn: () => leaveAPI.getConsumeLog({ user_id: logQueryKey }),
    enabled: !!logQueryKey,
  })

  const consumeMutation = useMutation({
    mutationFn: (values: any) => leaveAPI.consume(values),
    onSuccess: () => {
      message.success('消费记录成功')
      form.resetFields()
      if (logQueryKey) refetchLog()
    },
    onError: (err: any) => message.error(err?.response?.data?.error || '消费失败'),
  })

  const logColumns = [
    { title: '发放记录ID', dataIndex: 'grant_id', key: 'grant_id' },
    { title: '消费天数', dataIndex: 'days', key: 'days', render: (v: number) => formatDays(v) },
    { title: '审批单号', dataIndex: 'approval_ref', key: 'approval_ref', render: (v: string) => v || '-' },
    { title: '备注', dataIndex: 'remark', key: 'remark' },
    { title: '时间', dataIndex: 'created_at', key: 'created_at', render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm') },
  ]

  return (
    <div>
      <Row gutter={24}>
        <Col span={10}>
          <PageCard
            size="small"
            title={<span style={{ fontWeight: 'var(--font-weight-semibold)', fontSize: 'var(--font-size-base)', color: 'var(--color-text-heading)' }}>手动录入年假消费</span>}
          >
            <Form form={form} layout="vertical" onFinish={(values) => consumeMutation.mutate(values)}>
              <Form.Item name="user_id" label="员工" rules={[{ required: true, message: '请选择员工' }]}>
                <EmployeeSelect />
              </Form.Item>
              <Form.Item name="days" label="消费天数" rules={[{ required: true, message: '请输入天数' }]}>
                <InputNumber min={0.5} step={0.5} style={{ width: '100%' }} placeholder="如 1 或 0.5" />
              </Form.Item>
              <Form.Item name="approval_ref" label="审批单号（可选，填入可防重复）">
                <Input placeholder="钉钉审批流程ID" />
              </Form.Item>
              <Form.Item name="remark" label="备注">
                <Input placeholder="如：2025年春节年假" />
              </Form.Item>
              <Button type="primary" htmlType="submit" icon={<MinusCircleOutlined />} loading={consumeMutation.isPending} block>
                确认消费
              </Button>
            </Form>
          </PageCard>
        </Col>
        <Col span={14}>
          <PageCard
            size="small"
            title={<span style={{ fontWeight: 'var(--font-weight-semibold)', fontSize: 'var(--font-size-base)', color: 'var(--color-text-heading)' }}>消费记录查询</span>}
          >
            <Space style={{ marginBottom: 12 }}>
              <EmployeeSelect value={logUserID} onChange={(v) => setLogUserID(v ?? '')} />
              <Button type="primary" icon={<SearchOutlined />} onClick={() => setLogQueryKey(logUserID)} disabled={!logUserID}>
                查询
              </Button>
            </Space>
            <Table columns={logColumns} dataSource={(logData as any)?.data || []} rowKey="id" loading={logFetching} pagination={{ pageSize: 10, showSizeChanger: false }} size="small" />
          </PageCard>
        </Col>
      </Row>
    </div>
  )
}

const LeaveOvertime: React.FC = () => {
  const tabs = [
    { key: 'eligibility', label: '年假资格', children: <EligibilityTab /> },
    { key: 'grants', label: '年假发放', children: <GrantTab /> },
    { key: 'consume', label: '年假消费', children: <ConsumeTab /> },
    { key: 'overtime', label: '加班匹配', children: <OvertimeTab /> },
    { key: 'comp', label: '调休余额', children: <CompBalanceTab /> },
  ]

  return (
    <PageContainer title="年假与调休" icon={<CalendarOutlined />} subtitle="管理年假资格、发放、消费及加班调休匹配">
      <PageCard>
        <Tabs items={tabs} />
      </PageCard>
    </PageContainer>
  )
}

export default LeaveOvertime
