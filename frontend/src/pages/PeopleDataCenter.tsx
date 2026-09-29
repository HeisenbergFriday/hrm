import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Button, Card, Col, Descriptions, Drawer, Empty, Input, Row, Select, Spin, Space, Statistic, Table, Tabs, Tag, Tooltip, Typography, message } from 'antd'
import { DatabaseOutlined, ReloadOutlined, TeamOutlined, UserOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import { peopleDataCenterAPI, PeopleDataCenterBusiness, PeopleDataCenterEmployee } from '../services/api'
import { hasPermission } from '../utils/permission'

const { Text } = Typography

const statusLabels: Record<string, string> = { active: '在职', inactive: '停用', resigned: '离职', pending: '待处理' }
const syncStatusLabels: Record<string, string> = { success: '已同步', partial: '部分成功', failed: '同步失败', running: '同步中', local: '本地数据' }
const formatSyncStatus = (value?: string) => value ? (syncStatusLabels[value] || '其他') : '未同步'
const businessLabels: Record<string, string> = {
  attendance: '考勤', approval: '审批', annual_leave_grant: '年假发放', overtime_match: '加班匹配',
  performance_activity: '绩效活动', performance_participant: '绩效参与人',
}
const businessDescriptions: Record<string, string> = {
  attendance: '打卡与出勤记录', approval: '请假、加班等审批实例', annual_leave_grant: '年假额度发放记录',
  overtime_match: '加班与调休匹配结果', performance_activity: '绩效活动及周期信息', performance_participant: '绩效活动参与人员信息',
}
const formatDate = (value?: string) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
const sourceLabel = (isMirror: boolean) => isMirror ? '文娱员工' : '沐腾员工'
const businessSourceLabel = (isMirror: boolean) => isMirror ? '文娱业务镜像' : '沐腾业务'

const payloadValue = (payload: Record<string, unknown>, ...keys: string[]) => {
  for (const key of keys) {
    const direct = payload[key]
    if (direct !== undefined && direct !== null && direct !== '') return direct
    for (const containerKey of ['extension', 'content']) {
      const container = payload[containerKey]
      if (container && typeof container === 'object' && !Array.isArray(container)) {
        const nested = (container as Record<string, unknown>)[key]
        if (nested !== undefined && nested !== null && nested !== '') return nested
      }
    }
  }
  return undefined
}

const displayValue = (value: unknown) => {
  if (value === undefined || value === null || value === '') return '—'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

const businessSummary = (record: PeopleDataCenterBusiness) => {
  const payload = record.payload || {}
  switch (record.entity_type) {
    case 'attendance': return `${displayValue(payloadValue(payload, 'user_name', 'user_id', 'applicant_name'))} · ${displayValue(payloadValue(payload, 'check_type'))}`
    case 'approval': return `${displayValue(payloadValue(payload, 'title'))} · ${displayValue(payloadValue(payload, 'applicant_name', 'applicant_id'))}`
    case 'annual_leave_grant': return `${displayValue(payloadValue(payload, 'year'))} 年第 ${displayValue(payloadValue(payload, 'quarter'))} 季度 · ${displayValue(payloadValue(payload, 'user_id'))}`
    case 'overtime_match': return `${displayValue(payloadValue(payload, 'user_name', 'user_id'))} · ${displayValue(payloadValue(payload, 'work_date'))}`
    case 'performance_activity': return displayValue(payloadValue(payload, 'name'))
    case 'performance_participant': return `${displayValue(payloadValue(payload, 'employee_name', 'employee_id'))} · ${displayValue(payloadValue(payload, 'activity_id'))}`
    default: return record.source_key || '—'
  }
}

const businessTime = (record: PeopleDataCenterBusiness) => {
  const payload = record.payload || {}
  const value = payloadValue(payload,
    record.entity_type === 'attendance' ? 'check_time' : '',
    record.entity_type === 'approval' ? 'create_time' : '',
    record.entity_type === 'annual_leave_grant' ? 'updated_at' : '',
    record.entity_type === 'overtime_match' ? 'work_date' : '',
    record.entity_type === 'performance_activity' ? 'start_date' : '',
    record.entity_type === 'performance_participant' ? 'snapshot_as_of_date' : '',
  )
  return displayValue(value || record.source_updated_at)
}

const businessStatus = (record: PeopleDataCenterBusiness) => {
  const payload = record.payload || {}
  return displayValue(payloadValue(payload, 'status', 'approval_status', 'match_status', 'employee_status'))
}

const businessDetailFields: Record<string, Array<[string, string]>> = {
  attendance: [['员工', 'user_name'], ['员工 ID', 'user_id'], ['打卡时间', 'check_time'], ['打卡类型', 'check_type'], ['地点', 'location']],
  approval: [['审批标题', 'title'], ['申请人', 'applicant_name'], ['申请人 ID', 'applicant_id'], ['状态', 'status'], ['发起时间', 'create_time'], ['完成时间', 'finish_time'], ['业务开始时间', 'business_start_time'], ['业务结束时间', 'business_end_time']],
  annual_leave_grant: [['员工 ID', 'user_id'], ['年份', 'year'], ['季度', 'quarter'], ['工作年限', 'working_years'], ['发放天数', 'granted_days'], ['已用天数', 'used_days'], ['剩余天数', 'remaining_days'], ['发放类型', 'grant_type'], ['备注', 'remark']],
  overtime_match: [['员工', 'user_name'], ['员工 ID', 'user_id'], ['工作日期', 'work_date'], ['加班时长（分钟）', 'overtime_duration_minutes'], ['有效加班（分钟）', 'effective_overtime_minutes'], ['匹配状态', 'match_status'], ['匹配说明', 'match_reason']],
  performance_activity: [['活动名称', 'name'], ['周期类型', 'cycle_type'], ['开始日期', 'start_date'], ['结束日期', 'end_date'], ['状态', 'status'], ['活动类型', 'activity_kind']],
  performance_participant: [['员工', 'employee_name'], ['员工 ID', 'employee_id'], ['部门', 'department_name'], ['岗位', 'position'], ['绩效活动 ID', 'activity_id'], ['状态', 'status'], ['考核经理', 'manager_name']],
}

const PeopleDataCenter: React.FC = () => {
  const navigate = useNavigate()
  const [activeTab, setActiveTab] = useState('employees')
  const [employeePage, setEmployeePage] = useState(1)
  const [businessPage, setBusinessPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState('')
  const [employeeSource, setEmployeeSource] = useState<'local' | 'mirror' | ''>('')
  const [entityType, setEntityType] = useState('')
  const [businessSource, setBusinessSource] = useState<'local' | 'mirror' | ''>('')
  const [selected, setSelected] = useState<PeopleDataCenterEmployee | PeopleDataCenterBusiness | null>(null)
  const [syncRequestID, setSyncRequestID] = useState('')
  const [messageApi, messageContextHolder] = message.useMessage()
  const queryClient = useQueryClient()
  const handledRunRef = useRef('')
  const canSync = hasPermission('permission_manage')

  const summaryQuery = useQuery({ queryKey: ['people-data-center-summary'], queryFn: peopleDataCenterAPI.getSummary })
  const inboundLinksQuery = useQuery({ queryKey: ['people-data-center-inbound-links'], queryFn: peopleDataCenterAPI.getInboundLinks })
  const syncMutation = useMutation({
    mutationFn: () => {
      const links = inboundLinksQuery.data?.data?.items || []
      const activeLinks = links.filter((link) => link.status === 'active')
      return peopleDataCenterAPI.startSync(activeLinks.length === 1 ? activeLinks[0].source_org_id : undefined)
    },
    onSuccess: (response) => {
      setSyncRequestID(response.data.request_id)
      messageApi.info('同步任务已启动，正在等待结果')
    },
    onError: (error: any) => {
      const status = error?.response?.status
      messageApi.error(status === 404 ? '尚未配置文娱到沐腾的同步关系' : status === 409 ? '存在多个同步来源或已有任务正在执行' : '启动同步失败，请稍后重试')
    },
  })
  const syncRunQuery = useQuery({
    queryKey: ['people-data-center-sync-run', syncRequestID],
    queryFn: () => peopleDataCenterAPI.getSyncRun(syncRequestID),
    enabled: Boolean(syncRequestID),
    refetchInterval: (query) => query.state.data?.data?.status === 'running' ? 2000 : false,
  })
  const syncRun = syncRunQuery.data?.data
  useEffect(() => {
    if (!syncRun || syncRun.status === 'running' || handledRunRef.current === syncRun.request_id) return
    handledRunRef.current = syncRun.request_id
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ['people-data-center-summary'] }),
      queryClient.invalidateQueries({ queryKey: ['people-data-center-employees'] }),
      queryClient.invalidateQueries({ queryKey: ['people-data-center-business'] }),
    ])
    messageApi[syncRun.status === 'success' ? 'success' : syncRun.status === 'partial' ? 'warning' : 'error'](
      syncRun.status === 'success' ? `同步完成：员工 ${syncRun.employee_count} 条，业务 ${syncRun.business_count} 条` : syncRun.status === 'partial' ? `同步部分完成：失败 ${syncRun.failure_count} 条` : '同步失败，请查看同步状态',
    )
  }, [messageApi, queryClient, syncRun])
  const employeesQuery = useQuery({
    queryKey: ['people-data-center-employees', employeePage, keyword, status, employeeSource],
    queryFn: () => peopleDataCenterAPI.getEmployees({ page: employeePage, page_size: 20, keyword: keyword || undefined, status: status || undefined, source: employeeSource || undefined }),
  })
  const businessQuery = useQuery({
    queryKey: ['people-data-center-business', businessPage, entityType, businessSource],
    queryFn: () => peopleDataCenterAPI.getBusiness({ page: businessPage, page_size: 20, entity_type: entityType || undefined, source: businessSource || undefined }),
  })

  const summary = summaryQuery.data?.data
  const employeePageData = employeesQuery.data?.data
  const businessPageData = businessQuery.data?.data
  const selectedBusiness = selected && 'entity_type' in selected ? selected as PeopleDataCenterBusiness : null
  const selectedEmployee = selected && !('entity_type' in selected) ? selected as PeopleDataCenterEmployee : null
  const employeeColumns = useMemo(() => [
    { title: '姓名', dataIndex: 'name', key: 'name', render: (value: string, row: PeopleDataCenterEmployee) => <Button type="link" style={{ padding: 0 }} onClick={() => row.is_mirror ? setSelected(row) : navigate(`/employees/${row.id}`)}>{value || '—'}</Button> },
    { title: '工号', dataIndex: 'employee_id', key: 'employee_id', render: (value: string) => value || '—' },
    { title: '部门', dataIndex: 'department_name', key: 'department_name', render: (value: string) => value || '—' },
    { title: '岗位', dataIndex: 'position', key: 'position', render: (value: string) => value || '—' },
    { title: '状态', dataIndex: 'status', key: 'status', render: (value: string) => statusLabels[value] || (value ? '其他' : '—') },
    { title: '来源组织', key: 'source', render: (_: unknown, row: PeopleDataCenterEmployee) => <Tag color={row.is_mirror ? 'blue' : 'green'}>{sourceLabel(row.is_mirror)}</Tag> },
    { title: '最后同步', dataIndex: 'last_synced_at', key: 'last_synced_at', render: formatDate },
  ], [navigate])
  const businessColumns = useMemo(() => [
    { title: '业务类型', dataIndex: 'entity_type', key: 'entity_type', render: (value: string) => businessLabels[value] || '其他' },
    { title: '业务摘要', key: 'summary', render: (_: unknown, row: PeopleDataCenterBusiness) => businessSummary(row) },
    { title: '业务时间', key: 'business_time', render: (_: unknown, row: PeopleDataCenterBusiness) => businessTime(row) },
    { title: '业务状态', key: 'business_status', render: (_: unknown, row: PeopleDataCenterBusiness) => businessStatus(row) },
    { title: '来源', key: 'source', render: (_: unknown, row: PeopleDataCenterBusiness) => <Tag color={row.is_mirror ? 'blue' : 'green'}>{businessSourceLabel(row.is_mirror)}</Tag> },
    { title: '同步状态', dataIndex: 'sync_status', key: 'sync_status', render: (value: string, row: PeopleDataCenterBusiness) => row.is_mirror ? (syncStatusLabels[value] || '其他') : '本地数据' },
    { title: '操作', key: 'action', render: (_: unknown, row: PeopleDataCenterBusiness) => <Button type="link" onClick={() => setSelected(row)}>查看详情</Button> },
  ], [])

  const renderQueryState = (query: { isLoading: boolean; isError: boolean; refetch: () => unknown }, children: React.ReactNode) => query.isLoading ? <div style={{ textAlign: 'center', padding: 48 }}><Spin /></div> : query.isError ? <Alert type="error" showIcon message="数据加载失败" action={<Button size="small" icon={<ReloadOutlined />} onClick={() => void query.refetch()}>重试</Button>} /> : children

  return (
    <PageContainer title="人事数据中心" icon={<DatabaseOutlined />} subtitle="员工资料与文娱人事业务同步汇总（只读）" extra={<><Tooltip title={canSync ? '同步文娱员工资料及六类核心业务镜像' : '需要同步管理权限'}><Button type="primary" loading={syncMutation.isPending || syncRun?.status === 'running'} disabled={!canSync || syncRun?.status === 'running'} onClick={() => syncMutation.mutate()}>立即同步</Button></Tooltip><Button icon={<ReloadOutlined />} onClick={() => { void summaryQuery.refetch(); void inboundLinksQuery.refetch(); void employeesQuery.refetch(); void businessQuery.refetch() }}>刷新</Button></>}>
      {messageContextHolder}
      {syncRunQuery.isError && <Alert type="error" showIcon message="同步任务状态读取失败" description="任务可能仍在后台执行，请稍后刷新页面确认结果。" />}
      <Alert type="info" showIcon message="同步范围：员工资料 + 考勤、审批、年假发放、加班匹配、绩效活动、绩效参与人" style={{ marginBottom: 16 }} />
      {summaryQuery.isLoading ? <div style={{ textAlign: 'center', padding: 24 }}><Spin /></div> : summaryQuery.isError ? <Alert type="error" showIcon message="汇总数据加载失败" action={<Button size="small" onClick={() => void summaryQuery.refetch()}>重试</Button>} /> : (
        <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
          <Col xs={12} sm={8} xl={4}><PageCard><Statistic title="员工总数" value={summary?.employee_total ?? 0} prefix={<UserOutlined />} /></PageCard></Col>
          <Col xs={12} sm={8} xl={4}><PageCard><Statistic title="在职人数" value={summary?.active_employee_total ?? 0} prefix={<TeamOutlined />} /></PageCard></Col>
          <Col xs={12} sm={8} xl={4}><PageCard><Statistic title="本地员工" value={summary?.local_employee_total ?? 0} /></PageCard></Col>
          <Col xs={12} sm={8} xl={4}><PageCard><Statistic title="文娱员工" value={summary?.mirror_employee_total ?? 0} /></PageCard></Col>
          <Col xs={12} sm={8} xl={4}><PageCard><Statistic title="业务记录" value={summary?.business_total ?? 0} /></PageCard></Col>
          <Col xs={12} sm={8} xl={4}><PageCard><Statistic title="同步失败" value={summary?.sync_failure_count ?? 0} /></PageCard></Col>
          <Col xs={24} sm={16} xl={8}><PageCard><Statistic title="最近同步" value={formatSyncStatus(summary?.latest_run?.status)} suffix={<Text type="secondary" style={{ fontSize: 12 }}>{summary?.latest_run?.started_at ? formatDate(summary.latest_run.started_at) : '暂无记录'}</Text>} /></PageCard></Col>
        </Row>
      )}
      <PageCard>
        <Tabs activeKey={activeTab} onChange={setActiveTab} items={[
          { key: 'employees', label: '员工资料', children: <>
            <Row gutter={12} style={{ marginBottom: 16 }}><Col xs={24} sm={12} md={8}><Input allowClear placeholder="搜索姓名、工号、部门或岗位" value={keyword} onChange={(e) => { setKeyword(e.target.value); setEmployeePage(1) }} /></Col><Col xs={24} sm={12} md={5}><Select aria-label="员工状态" style={{ width: '100%' }} allowClear placeholder="全部状态" value={status || undefined} onChange={(value) => { setStatus(value || ''); setEmployeePage(1) }} options={[{ value: 'active', label: '在职' }, { value: 'inactive', label: '停用' }, { value: 'resigned', label: '离职' }]} /></Col><Col xs={24} sm={12} md={5}><Select aria-label="员工来源" style={{ width: '100%' }} allowClear placeholder="全部员工" value={employeeSource || undefined} onChange={(value) => { setEmployeeSource((value || '') as 'local' | 'mirror' | ''); setEmployeePage(1) }} options={[{ value: 'local', label: '沐腾员工' }, { value: 'mirror', label: '文娱员工' }]} /></Col></Row>
            {renderQueryState(employeesQuery, employeePageData?.items?.length ? <Table rowKey={(row) => `${row.source_org_id}:${row.source_user_id}`} columns={employeeColumns} dataSource={employeePageData.items} pagination={{ current: employeePage, pageSize: 20, total: employeePageData.total, showSizeChanger: false, onChange: setEmployeePage }} scroll={{ x: 900 }} /> : <Empty description="暂无员工资料" />)}
          </> },
          { key: 'business', label: '业务数据', children: <>
            <Alert type="info" showIcon style={{ marginBottom: 16 }} message="业务数据包括六类核心业务，均为只读汇总" description="考勤、审批、年假发放、加班匹配、绩效活动、绩效参与人。来源列区分沐腾本地业务与文娱同步镜像，镜像不会在沐腾侧执行或反向修改。" />
            <Space wrap style={{ marginBottom: 16 }}><Select aria-label="业务类型" style={{ width: 220 }} allowClear placeholder="全部业务类型" value={entityType || undefined} onChange={(value) => { setEntityType(value || ''); setBusinessPage(1) }} options={Object.entries(businessLabels).map(([value, label]) => ({ value, label, title: businessDescriptions[value] }))} /><Select aria-label="业务来源" style={{ width: 180 }} allowClear placeholder="全部业务来源" value={businessSource || undefined} onChange={(value) => { setBusinessSource((value || '') as 'local' | 'mirror' | ''); setBusinessPage(1) }} options={[{ value: 'local', label: '沐腾业务' }, { value: 'mirror', label: '文娱业务镜像' }]} /></Space>
            {renderQueryState(businessQuery, businessPageData?.items?.length ? <Table rowKey={(row) => `${row.source_org_id}:${row.entity_type}:${row.source_key}`} columns={businessColumns} dataSource={businessPageData.items} pagination={{ current: businessPage, pageSize: 20, total: businessPageData.total, showSizeChanger: false, onChange: setBusinessPage }} scroll={{ x: 1100 }} onRow={(row) => ({ onClick: () => setSelected(row) })} /> : <Empty description="暂无业务数据" />)}
          </> },
        ]} />
      </PageCard>
      <Drawer title={selectedBusiness ? `${businessLabels[selectedBusiness.entity_type] || '业务'}详情` : selectedEmployee ? `${sourceLabel(selectedEmployee.is_mirror)}详情` : '只读详情'} open={Boolean(selected)} onClose={() => setSelected(null)} width={520}>
        {selectedBusiness ? <>
          {selectedBusiness.is_mirror && <Alert type="info" showIcon style={{ marginBottom: 16 }} message="文娱业务镜像，仅供查看" description="此记录来自文娱组织，只能查看，不能在沐腾侧审批、修改或回写。" />}
          <Card size="small" title={businessLabels[selectedBusiness.entity_type] || '业务详情'} style={{ marginBottom: 16 }}>
            <Descriptions column={1} size="small">
              <Descriptions.Item label="来源">{businessSourceLabel(selectedBusiness.is_mirror)}</Descriptions.Item>
              <Descriptions.Item label="源记录 ID">{selectedBusiness.source_key || '—'}</Descriptions.Item>
              <Descriptions.Item label="同步状态">{selectedBusiness.is_mirror ? (syncStatusLabels[selectedBusiness.sync_status] || '其他') : '本地数据'}</Descriptions.Item>
              <Descriptions.Item label="同步时间">{formatDate(selectedBusiness.last_synced_at)}</Descriptions.Item>
            </Descriptions>
          </Card>
          <Card size="small" title="业务内容" style={{ marginBottom: 16 }}>
            <Descriptions bordered column={1} size="small">
              {(businessDetailFields[selectedBusiness.entity_type] || []).map(([label, key]) => <Descriptions.Item key={key} label={label}>{displayValue(payloadValue(selectedBusiness.payload || {}, key))}</Descriptions.Item>)}
              {!businessDetailFields[selectedBusiness.entity_type] && <Descriptions.Item label="业务摘要">{businessSummary(selectedBusiness)}</Descriptions.Item>}
            </Descriptions>
          </Card>
          <Text type="secondary">原始字段仅用于核对，不代表沐腾侧可执行的业务数据。</Text>
        </> : selectedEmployee && <><Text strong>{selectedEmployee.name}</Text>{selectedEmployee.is_mirror && <Alert type="info" showIcon style={{ margin: '12px 0' }} message="文娱员工镜像，仅供查看" description="如需修改员工资料，请在文娱组织处理；本页不会回写文娱或钉钉。" />}<p>员工来源：{sourceLabel(selectedEmployee.is_mirror)}</p><p>工号：{selectedEmployee.employee_id || '—'}</p><p>邮箱：{selectedEmployee.email || '—'}</p><p>手机：{selectedEmployee.mobile || '—'}</p><p>部门：{selectedEmployee.department_name || '—'}</p><p>岗位：{selectedEmployee.position || '—'}</p><p>状态：{statusLabels[selectedEmployee.status] || (selectedEmployee.status ? '其他' : '—')}</p></>}
      </Drawer>
    </PageContainer>
  )
}

export default PeopleDataCenter
