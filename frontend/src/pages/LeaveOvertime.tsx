import React, { useState } from 'react'
import {
  Button,
  Card,
  Col,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd'
import { ClockCircleOutlined, GiftOutlined, MinusCircleOutlined, SearchOutlined, SyncOutlined } from '@ant-design/icons'
import { useMutation, useQuery } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { leaveAPI, orgAPI, overtimeAPI } from '../services/api'

const { Title } = Typography
const { RangePicker } = DatePicker

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
      title: '是否有资格',
      dataIndex: 'is_eligible',
      key: 'is_eligible',
      render: (value: boolean) => <Tag color={value ? 'green' : 'red'}>{value ? '有资格' : '无资格'}</Tag>,
    },
    { title: '入职日期', dataIndex: 'entry_date', key: 'entry_date' },
    { title: '转正日期', dataIndex: 'confirmation_date', key: 'confirmation_date' },
    {
      title: '追溯来源季度',
      dataIndex: 'retroactive_source_quarter',
      key: 'retroactive_source_quarter',
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
        <Button
          type="primary"
          icon={<SearchOutlined />}
          onClick={() => setQueryKey({ user_id: userID, year })}
          disabled={!userID}
        >
          查询
        </Button>
        <Button
          icon={<SyncOutlined />}
          onClick={() => recalcMutation.mutate()}
          loading={recalcMutation.isPending}
          disabled={!userID}
        >
          重算资格
        </Button>
      </Space>
      <Table
        columns={columns}
        dataSource={(data as any)?.data || []}
        rowKey="quarter"
        loading={isFetching}
        pagination={false}
      />
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
      title: '类型',
      dataIndex: 'grant_type',
      key: 'grant_type',
      render: (value: string) => <Tag color={typeColor[value] || 'default'}>{typeLabel[value] || value}</Tag>,
    },
    { title: '工龄(年)', dataIndex: 'working_years', key: 'working_years', render: (value?: number) => formatWorkingYears(value) },
    { title: '基础天数', dataIndex: 'base_days', key: 'base_days', render: (value?: number) => formatDays(value) },
    { title: '本次发放', dataIndex: 'granted_days', key: 'granted_days', render: (value?: number) => formatDays(value) },
    { title: '已用', dataIndex: 'used_days', key: 'used_days', render: (value?: number) => formatDays(value) },
    { title: '剩余', dataIndex: 'remaining_days', key: 'remaining_days', render: (value?: number) => formatDays(value) },
    { title: '备注', dataIndex: 'remark', key: 'remark' },
    {
      title: '钉钉同步',
      dataIndex: 'dingtalk_sync_status',
      key: 'dingtalk_sync_status',
      render: (value: string) => (
        <Tag color={syncStatusColor[value] || 'default'}>{syncStatusLabel[value] || value || '-'}</Tag>
      ),
    },
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
        <Button
          type="primary"
          icon={<SearchOutlined />}
          onClick={() => setQueryKey({ user_id: userID, year })}
          disabled={!userID}
        >
          查询
        </Button>
        <Button icon={<GiftOutlined />} onClick={() => setBatchModalOpen(true)}>
          手动发放季度年假
        </Button>
        <Button
          icon={<SyncOutlined />}
          onClick={() => regrantMutation.mutate()}
          loading={regrantMutation.isPending}
          disabled={!userID}
        >
          追溯补发
        </Button>
        <Button
          icon={<SyncOutlined />}
          onClick={handleSyncToDingTalk}
          loading={syncMutation.isPending}
        >
          同步到钉钉
        </Button>
      </Space>
      <Table
        columns={columns}
        dataSource={(data as any)?.data || []}
        rowKey="id"
        loading={isFetching}
        pagination={false}
      />
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
                <Select.Option key={quarter} value={quarter}>
                  Q{quarter}
                </Select.Option>
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
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs]>([
    dayjs().startOf('month'),
    dayjs().endOf('month'),
  ])
  const [queryKey, setQueryKey] = useState<{ user_id: string; start_date: string; end_date: string } | null>(null)

  const { data, isFetching } = useQuery({
    queryKey: ['overtime-matches', queryKey],
    queryFn: () => overtimeAPI.getMatches(queryKey!),
    enabled: !!queryKey,
  })

  const runMatchMutation = useMutation({
    mutationFn: () =>
      overtimeAPI.runMatch({
        start_date: dateRange[0].format('YYYY-MM-DD'),
        end_date: dateRange[1].format('YYYY-MM-DD'),
      }),
    onSuccess: () => {
      message.success('加班匹配完成')
      if (userID) {
        setQueryKey({
          user_id: userID,
          start_date: dateRange[0].format('YYYY-MM-DD'),
          end_date: dateRange[1].format('YYYY-MM-DD'),
        })
      }
    },
    onError: () => message.error('匹配失败'),
  })

  const statusColor: Record<string, string> = { matched: 'green', partial: 'orange', unmatched: 'red', rolled_back: 'default' }
  const statusLabel: Record<string, string> = { matched: '完全匹配', partial: '部分匹配', unmatched: '未匹配', rolled_back: '已回滚' }

  const columns = [
    { title: '审批ID', dataIndex: 'approval_id', key: 'approval_id' },
    {
      title: '状态',
      dataIndex: 'match_status',
      key: 'match_status',
      render: (value: string) => <Tag color={statusColor[value] || 'default'}>{statusLabel[value] || value}</Tag>,
    },
    { title: '审批开始', dataIndex: 'approval_start_time', key: 'approval_start_time' },
    { title: '审批结束', dataIndex: 'approval_end_time', key: 'approval_end_time' },
    { title: '打卡时长(分钟)', dataIndex: 'matched_minutes', key: 'matched_minutes' },
    { title: '有效加班(分钟)', dataIndex: 'qualified_minutes', key: 'qualified_minutes' },
    { title: '匹配说明', dataIndex: 'match_reason', key: 'match_reason' },
  ]

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <EmployeeSelect value={userID} onChange={(next) => setUserID(next ?? '')} />
        <RangePicker
          value={dateRange}
          onChange={(next) => next && setDateRange(next as [dayjs.Dayjs, dayjs.Dayjs])}
        />
        <Button
          type="primary"
          icon={<SearchOutlined />}
          onClick={() =>
            setQueryKey({
              user_id: userID,
              start_date: dateRange[0].format('YYYY-MM-DD'),
              end_date: dateRange[1].format('YYYY-MM-DD'),
            })
          }
          disabled={!userID}
        >
          查询
        </Button>
        <Button icon={<SyncOutlined />} onClick={() => runMatchMutation.mutate()} loading={runMatchMutation.isPending}>
          执行加班匹配
        </Button>
      </Space>
      <Table
        columns={columns}
        dataSource={(data as any)?.data || []}
        rowKey="id"
        loading={isFetching}
        pagination={{ pageSize: 20 }}
      />
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
        <Button
          type="primary"
          icon={<SearchOutlined />}
          onClick={() => setQueryUserID(userID)}
          disabled={!userID}
          loading={isFetching}
        >
          查询
        </Button>
      </Space>
      {balance && (
        <Row gutter={32}>
          <Col>
            <Statistic
              title="累计调休(分钟)"
              value={balance.total_credit_minutes ?? 0}
              prefix={<ClockCircleOutlined />}
              valueStyle={{ color: '#3f8600' }}
            />
          </Col>
          <Col>
            <Statistic
              title="已用调休(分钟)"
              value={balance.total_debit_minutes ?? 0}
              valueStyle={{ color: '#cf1322' }}
            />
          </Col>
          <Col>
            <Statistic
              title="剩余调休(分钟)"
              value={balance.balance_minutes ?? 0}
              suffix={`（约 ${((balance.balance_minutes ?? 0) / 60).toFixed(1)} 小时）`}
              valueStyle={{ color: '#1677ff' }}
            />
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
          <Card size="small" title="手动录入年假消费">
            <Form
              form={form}
              layout="vertical"
              onFinish={(values) => consumeMutation.mutate(values)}
            >
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
              <Button
                type="primary"
                htmlType="submit"
                icon={<MinusCircleOutlined />}
                loading={consumeMutation.isPending}
                block
              >
                确认消费
              </Button>
            </Form>
          </Card>
        </Col>
        <Col span={14}>
          <Card size="small" title="消费记录查询">
            <Space style={{ marginBottom: 12 }}>
              <EmployeeSelect value={logUserID} onChange={(v) => setLogUserID(v ?? '')} />
              <Button
                type="primary"
                icon={<SearchOutlined />}
                onClick={() => setLogQueryKey(logUserID)}
                disabled={!logUserID}
              >
                查询
              </Button>
            </Space>
            <Table
              columns={logColumns}
              dataSource={(logData as any)?.data || []}
              rowKey="id"
              loading={logFetching}
              pagination={{ pageSize: 10 }}
              size="small"
            />
          </Card>
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
    <div>
      <Title level={4}>年假与调休</Title>
      <Card>
        <Tabs items={tabs} />
      </Card>
    </div>
  )
}

export default LeaveOvertime
