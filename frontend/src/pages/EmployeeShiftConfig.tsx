import { useMemo, useState } from 'react'
import dayjs, { type Dayjs } from 'dayjs'
import {
  Alert,
  Button,
  DatePicker,
  Input,
  Modal,
  Radio,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ClockCircleOutlined, DeleteOutlined, SettingOutlined, TeamOutlined } from '@ant-design/icons'
import { shiftConfigAPI } from '../services/api'

const { RangePicker } = DatePicker
const { Text } = Typography

interface EmployeeShiftItem {
  user_id: string
  user_name: string
  department_id: string
  department_name: string
  end_time: string
  shift_id: number
  note: string
  has_custom: boolean
  config_id: number
}

interface ShiftCatalogItem {
  id: number
  name: string
  shift_id: number
  check_in: string
  check_out: string
  attached_to_group?: boolean
  group_id?: number
  group_name?: string
}

interface ShiftPreviewItem {
  user_id: string
  user_name: string
  work_date: string
  shift_id: number
  shift_name: string
  is_rest: boolean
  reason: string
  week_type?: string
  holiday_name?: string
  holiday_type?: string
  will_sync: boolean
}

type ShiftMode = 'existing' | 'create'

interface BatchFormState {
  mode: ShiftMode
  shift_id: number
  name: string
  check_in: string
  check_out: string
  note: string
  date_range: [Dayjs, Dayjs]
}

function unwrapEnvelope<T>(response: any): T {
  if (response && typeof response === 'object' && 'code' in response && 'data' in response) {
    return response.data as T
  }
  if (response && typeof response === 'object' && 'data' in response) {
    return response.data as T
  }
  return response as T
}

function makeDefaultBatchForm(hasCatalogs: boolean): BatchFormState {
  return {
    mode: hasCatalogs ? 'existing' : 'create',
    shift_id: 0,
    name: '17:30下班',
    check_in: '09:00',
    check_out: '17:30',
    note: '',
    date_range: [dayjs(), dayjs().add(30, 'day')],
  }
}

function previewReasonLabel(item: ShiftPreviewItem): string {
  switch (item.reason) {
    case 'holiday':
      return item.holiday_name ? `${item.holiday_name}放假` : '节假日休息'
    case 'workday_adjustment':
      return item.holiday_name ? `${item.holiday_name}调休上班` : '调休上班'
    case 'small_week_saturday':
      return '小周周六上班'
    case 'big_week_saturday':
      return '大周周六休息'
    case 'sunday_rest':
      return '周日休息'
    default:
      return '工作日'
  }
}

export default function EmployeeShiftConfig() {
  const qc = useQueryClient()
  const [selectedRowKeys, setSelectedRowKeys] = useState<string[]>([])
  const [batchModalOpen, setBatchModalOpen] = useState(false)
  const [batchForm, setBatchForm] = useState<BatchFormState>(makeDefaultBatchForm(false))

  const { data: items = [], isLoading } = useQuery({
    queryKey: ['shiftConfigs'],
    queryFn: async () => {
      const data = unwrapEnvelope<{ items: EmployeeShiftItem[] }>(await shiftConfigAPI.list())
      return data.items ?? []
    },
    staleTime: 5 * 60 * 1000,
    retry: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  const { data: catalogs = [] } = useQuery({
    queryKey: ['shiftCatalogs'],
    queryFn: async () => {
      const data = unwrapEnvelope<{ items: ShiftCatalogItem[] }>(await shiftConfigAPI.catalogs())
      return data.items ?? []
    },
    staleTime: 5 * 60 * 1000,
    retry: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  const shiftOptions = useMemo(
    () =>
      catalogs.map((item) => ({
        value: item.shift_id,
        label: `${item.name} (${item.check_in || '--'} - ${item.check_out || '--'})${item.attached_to_group === false ? ' [未加入考勤组]' : ''}`,
        checkOut: item.check_out,
        attachedToGroup: item.attached_to_group !== false,
        groupName: item.group_name,
        disabled: item.attached_to_group === false,
      })),
    [catalogs]
  )

  const selectedUsers = useMemo(
    () => items.filter((item) => selectedRowKeys.includes(item.user_id)),
    [items, selectedRowKeys]
  )

  const previewPayload = useMemo(() => {
    const [startDate, endDate] = batchForm.date_range
    if (!batchModalOpen || selectedRowKeys.length === 0 || !startDate || !endDate) {
      return null
    }

    if (batchForm.mode === 'existing') {
      if (!batchForm.shift_id) {
        return null
      }
      const selectedOption = shiftOptions.find((option) => option.value === batchForm.shift_id)
      return {
        user_ids: selectedRowKeys,
        shift_id: batchForm.shift_id,
        end_time: selectedOption?.checkOut || batchForm.check_out,
        start_date: startDate.format('YYYY-MM-DD'),
        end_date: endDate.format('YYYY-MM-DD'),
      }
    }

    if (!batchForm.name || !batchForm.check_in || !batchForm.check_out) {
      return null
    }
    return {
      user_ids: selectedRowKeys,
      name: batchForm.name,
      check_in: batchForm.check_in,
      check_out: batchForm.check_out,
      end_time: batchForm.check_out,
      start_date: startDate.format('YYYY-MM-DD'),
      end_date: endDate.format('YYYY-MM-DD'),
    }
  }, [batchForm, batchModalOpen, selectedRowKeys, shiftOptions])

  const { data: previewItems = [], isFetching: previewLoading } = useQuery({
    queryKey: ['shiftConfigPreview', previewPayload],
    queryFn: async () => {
      const data = unwrapEnvelope<{ items: ShiftPreviewItem[] }>(await shiftConfigAPI.preview(previewPayload!))
      return data.items ?? []
    },
    enabled: !!previewPayload,
    retry: false,
    staleTime: 30 * 1000,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  const previewRows = useMemo(() => {
    const rowMap = new Map<
      string,
      { key: string; work_date: string; rule: string; sync_status: string; [key: string]: any }
    >()
    previewItems.forEach((item) => {
      const key = item.work_date
      if (!rowMap.has(key)) {
        rowMap.set(key, {
          key,
          work_date: item.work_date,
          rule: previewReasonLabel(item),
          sync_status: item.will_sync ? '会下发到钉钉' : '沿用公司默认规则',
        })
      }
      rowMap.get(key)![item.user_id] = item
    })
    return Array.from(rowMap.values()).sort((a, b) => a.work_date.localeCompare(b.work_date))
  }, [previewItems])

  const previewColumns = useMemo(() => {
    const baseColumns: any[] = [
      {
        title: '日期',
        dataIndex: 'work_date',
        width: 120,
      },
      {
        title: '规则',
        dataIndex: 'rule',
        width: 180,
      },
      {
        title: '同步',
        dataIndex: 'sync_status',
        width: 140,
      },
    ]

    const userColumns = selectedUsers.map((user) => ({
      title: user.user_name,
      dataIndex: user.user_id,
      key: user.user_id,
      width: 140,
      render: (item?: ShiftPreviewItem) => {
        if (!item) {
          return <Text type="secondary">-</Text>
        }
        return item.is_rest ? <Tag>休</Tag> : <Tag color="blue">{item.shift_name}</Tag>
      },
    }))

    return [...baseColumns, ...userColumns]
  }, [selectedUsers])

  const resetBatchForm = () => {
    setBatchForm(makeDefaultBatchForm(catalogs.length > 0))
  }

  const applyMutation = useMutation({
    mutationFn: shiftConfigAPI.apply,
    onSuccess: async (response: any) => {
      const result = unwrapEnvelope<any>(response) ?? {}

      await Promise.all([
        qc.invalidateQueries({ queryKey: ['shiftConfigs'] }),
        qc.invalidateQueries({ queryKey: ['shiftCatalogs'] }),
      ])

      if (result.status === 'success') {
        message.success(result.message || '已创建并下发到钉钉')
      } else {
        message.warning(result.message || '本地已保存，局部同步存在部分未完成项')
        Modal.warning({
          title: '局部同步未完成',
          width: 720,
          content: (
            <div>
              <div style={{ marginBottom: 8 }}>结果：{result.message || '本地已保存，但钉钉同步未完全成功。'}</div>
              {result.group_id ? (
                <div style={{ marginBottom: 8 }}>
                  考勤组：{result.group_name || '未命名考勤组'} ({result.group_id})
                </div>
              ) : null}
              {result.error_detail ? (
                <div style={{ marginBottom: 8, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                  钉钉返回：{result.error_detail}
                </div>
              ) : null}
              {result.failed_count ? <div>失败条目：{result.failed_count}</div> : null}
            </div>
          ),
        })
      }

      setBatchModalOpen(false)
      setSelectedRowKeys([])
      resetBatchForm()
    },
    onError: (err: any) => {
      message.error('一站式设置失败: ' + (err.response?.data?.message ?? err.message))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: shiftConfigAPI.remove,
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['shiftConfigs'] })
      message.success('已恢复为默认 18:30')
    },
    onError: (err: any) => message.error('操作失败: ' + (err.response?.data?.message ?? err.message)),
  })

  const openBatchModal = (userIDs: string[]) => {
    setSelectedRowKeys(userIDs)
    resetBatchForm()
    setBatchModalOpen(true)
  }

  const handleSingleSet = (record: EmployeeShiftItem) => {
    openBatchModal([record.user_id])
  }

  const handleBatchSet = () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先勾选员工')
      return
    }
    openBatchModal(selectedRowKeys)
  }

  const confirmBatchSet = () => {
    const [startDate, endDate] = batchForm.date_range
    if (!startDate || !endDate) {
      message.warning('请选择局部下发日期范围')
      return
    }

    const payload: Record<string, unknown> = {
      user_ids: selectedRowKeys,
      note: batchForm.note,
      start_date: startDate.format('YYYY-MM-DD'),
      end_date: endDate.format('YYYY-MM-DD'),
    }

    if (batchForm.mode === 'existing') {
      if (!batchForm.shift_id) {
        message.warning('请选择已有班次')
        return
      }
      const selectedOption = shiftOptions.find((option) => option.value === batchForm.shift_id)
      if (!selectedOption?.attachedToGroup) {
        message.warning(`该班次尚未加入考勤组${selectedOption?.groupName ? `：${selectedOption.groupName}` : ''}，请先在钉钉中将班次加入考勤组后再同步`)
        return
      }
      payload.shift_id = batchForm.shift_id
      payload.end_time = selectedOption?.checkOut || batchForm.check_out
    } else {
      if (!batchForm.name || !batchForm.check_in || !batchForm.check_out) {
        message.warning('请填写班次名称、上班时间和下班时间')
        return
      }
      payload.name = batchForm.name
      payload.check_in = batchForm.check_in
      payload.check_out = batchForm.check_out
      payload.end_time = batchForm.check_out
    }

    applyMutation.mutate(payload as any)
  }

  const columns = [
    { title: '姓名', dataIndex: 'user_name', width: 100 },
    { title: '部门', dataIndex: 'department_name', width: 140 },
    {
      title: '下班时间',
      dataIndex: 'end_time',
      width: 120,
      render: (value: string, record: EmployeeShiftItem) =>
        record.has_custom ? (
          <Tag color="blue" icon={<ClockCircleOutlined />}>
            {value}
          </Tag>
        ) : (
          <Tag color="default">18:30(默认)</Tag>
        ),
    },
    { title: '备注', dataIndex: 'note', ellipsis: true },
    {
      title: '操作',
      width: 160,
      render: (_: unknown, record: EmployeeShiftItem) => (
        <Space>
          <Tooltip title="设置下班时间">
            <Button size="small" icon={<SettingOutlined />} onClick={() => handleSingleSet(record)}>
              设置
            </Button>
          </Tooltip>
          {record.has_custom && (
            <Tooltip title="恢复默认 18:30">
              <Button
                size="small"
                danger
                icon={<DeleteOutlined />}
                onClick={() =>
                  Modal.confirm({
                    title: `恢复 ${record.user_name} 为默认 18:30 下班？`,
                    onOk: () => deleteMutation.mutateAsync(record.user_id),
                  })
                }
              />
            </Tooltip>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', gap: 8, alignItems: 'center' }}>
        <h2 style={{ margin: 0 }}>员工下班时间配置</h2>
      </div>

      <Alert
        style={{ marginBottom: 16 }}
        type="info"
        showIcon
        message="使用说明"
        description={
          <>
            <div>默认规则：9:00 上班 / 18:30 下班。这里可以为员工设置专属下班班次。</div>
            <div>现在支持一站式处理：创建/复用钉钉班次、写入本地配置、并按选定日期范围局部下发到钉钉。</div>
            <div>钉钉调用次数最小化：优先命中内存缓存和本地目录，只有缺失时才查询或创建钉钉班次。</div>
          </>
        }
      />

      <div style={{ marginBottom: 12, display: 'flex', gap: 8 }}>
        <Button type="primary" icon={<TeamOutlined />} disabled={selectedRowKeys.length === 0} onClick={handleBatchSet}>
          一站式设置(已选 {selectedRowKeys.length} 人)
        </Button>
        <Text type="secondary" style={{ lineHeight: '32px' }}>
          已缓存班次 {shiftOptions.length} 个
        </Text>
      </div>

      {catalogs.length > 0 ? (
        <Alert
          style={{ marginBottom: 12 }}
          type={catalogs.some((item) => item.attached_to_group === false) ? 'warning' : 'success'}
          showIcon
          message="考勤组班次状态"
          description={
            <div>
              {catalogs.map((item) => (
                <div key={item.shift_id}>
                  {item.name}：{item.attached_to_group === false ? '未加入考勤组' : '已加入考勤组'}
                  {item.group_name ? `（${item.group_name}）` : ''}
                </div>
              ))}
            </div>
          }
        />
      ) : null}

      <Table
        rowKey="user_id"
        dataSource={items}
        columns={columns}
        loading={isLoading}
        locale={{
          emptyText: '暂无可配置员工。若当前环境只有管理员账号，现在重载后会显示管理员；若仍为空，请先同步员工数据。',
        }}
        rowSelection={{
          selectedRowKeys,
          onChange: (keys) => setSelectedRowKeys(keys as string[]),
        }}
        pagination={{ pageSize: 20, showTotal: (total) => `共 ${total} 人` }}
        size="small"
      />

      <Modal
        title={`一站式设置下班班次 (${selectedRowKeys.length} 人)`}
        open={batchModalOpen}
        onOk={confirmBatchSet}
        onCancel={() => setBatchModalOpen(false)}
        confirmLoading={applyMutation.isPending}
        okText="创建并下发"
        width={920}
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          message="会一次完成：创建/复用钉钉班次、保存本地配置、并只给选中员工在指定日期范围内批量下发。"
        />

        <div style={{ marginBottom: 16 }}>
          <label>班次来源</label>
          <Radio.Group
            style={{ display: 'block', marginTop: 8 }}
            value={batchForm.mode}
            onChange={(e) => setBatchForm((prev) => ({ ...prev, mode: e.target.value as ShiftMode }))}
          >
            <Radio.Button value="existing" disabled={shiftOptions.length === 0}>
              选择已有班次
            </Radio.Button>
            <Radio.Button value="create">直接新建班次</Radio.Button>
          </Radio.Group>
        </div>

        {batchForm.mode === 'existing' ? (
          <div style={{ marginBottom: 16 }}>
            <label>已有班次</label>
            <Select
              style={{ width: '100%', marginTop: 8 }}
              placeholder={shiftOptions.length === 0 ? '暂无本地班次，请切换到直接新建班次' : '选择已有班次'}
              value={batchForm.shift_id || undefined}
              options={shiftOptions}
              onChange={(value) =>
                setBatchForm((prev) => ({
                  ...prev,
                  shift_id: value,
                }))
              }
            />
          </div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginBottom: 16 }}>
            <div style={{ gridColumn: '1 / -1' }}>
              <label>班次名称</label>
              <Input
                style={{ marginTop: 8 }}
                value={batchForm.name}
                onChange={(e) => setBatchForm((prev) => ({ ...prev, name: e.target.value }))}
                placeholder="例如：17:30下班"
              />
            </div>
            <div>
              <label>上班时间</label>
              <Input
                style={{ marginTop: 8 }}
                value={batchForm.check_in}
                onChange={(e) => setBatchForm((prev) => ({ ...prev, check_in: e.target.value }))}
                placeholder="09:00"
              />
            </div>
            <div>
              <label>下班时间</label>
              <Input
                style={{ marginTop: 8 }}
                value={batchForm.check_out}
                onChange={(e) => setBatchForm((prev) => ({ ...prev, check_out: e.target.value }))}
                placeholder="17:30"
              />
            </div>
          </div>
        )}

        <div style={{ marginBottom: 16 }}>
          <label>局部下发日期范围</label>
          <RangePicker
            style={{ width: '100%', marginTop: 8 }}
            value={batchForm.date_range}
            onChange={(dates) => {
              if (!dates || !dates[0] || !dates[1]) {
                return
              }
              setBatchForm((prev) => ({ ...prev, date_range: [dates[0], dates[1]] }))
            }}
          />
          <div style={{ marginTop: 6 }}>
            <Text type="secondary">会按员工自定义班次和公司节假日、调休、大小周规则生成最终结果，只对这段日期做局部下发。</Text>
          </div>
        </div>

        <div style={{ marginBottom: 16 }}>
          <Alert
            type="info"
            showIcon
            message="直接预览"
            description="这里直接显示选中员工在当前日期范围内的最终结果：员工自定义下班时间优先，节假日、调休和大小周仍按公司规则计算。"
          />
          <Table
            style={{ marginTop: 12 }}
            size="small"
            loading={previewLoading}
            columns={previewColumns}
            dataSource={previewRows}
            pagination={false}
            scroll={{ x: true, y: 260 }}
            locale={{
              emptyText: previewPayload ? '当前条件下暂无可预览数据' : '先选择班次和日期范围后，这里会直接显示最终结果',
            }}
          />
        </div>

        <div>
          <label>备注(可选)</label>
          <Input
            style={{ marginTop: 8 }}
            value={batchForm.note}
            onChange={(e) => setBatchForm((prev) => ({ ...prev, note: e.target.value }))}
            placeholder="例如：研发团队弹性下班"
          />
        </div>
      </Modal>
    </div>
  )
}
