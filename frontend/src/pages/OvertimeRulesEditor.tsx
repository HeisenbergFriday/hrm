import React, { useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Collapse,
  Input,
  InputNumber,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
  message,
} from 'antd'
import type { UploadFile } from 'antd'
import { DownloadOutlined, ReloadOutlined, UploadOutlined, CheckOutlined } from '@ant-design/icons'
import { attendanceToolboxAPI } from '../services/api'

const { Text } = Typography

export interface PremiumRule {
  priority?: number
  date_type: string
  department_group: string
  multiplier: number
  action: string
}

export interface DepartmentRule {
  group_name: string
  match_field: string
  match_method: string
  match_value: string
  level?: string
  match_type?: string
  keywords?: string[]
  group?: string
}

export interface RulesPreview {
  premium_rules: PremiumRule[]
  department_rules: DepartmentRule[]
  params: {
    standard_hours_per_day?: number
    no_punch_mark?: string
    schedule_augment_holidays?: boolean
    schedule_augment_rest_dept_group?: string
    chengdu_use_separate_calendar?: boolean
    rest_premium_excluded_names?: string[]
    rest_premium_excluded_codes?: string[]
  }
  legal_holidays_override?: string[]
  meta?: { source?: string }
}

export type RulesSource = 'default' | 'custom'

export interface AppliedRulesState {
  source: RulesSource
  rules: RulesPreview | null
  rulesJson: string | null
}

interface OvertimeRulesEditorProps {
  value?: AppliedRulesState
  onChange?: (next: AppliedRulesState) => void
  canEdit?: boolean
  disabledReason?: string
}

const DATE_TYPE_LABELS: Record<string, string> = {
  LEGAL_HOLIDAY: '法定节假日',
  HOLIDAY_ADJUST_REST: '调休休息日',
  ORDINARY_WEEKEND: '普通周末',
  MAKEUP_WORKDAY: '调休上班',
  NORMAL_WORKDAY: '普通工作日',
}

const ACTION_LABELS: Record<string, string> = {
  加班工资: '加班工资',
  调休: '调休',
  '3x': '3倍',
  '2x': '2倍',
  rest: '调休',
  'rest+2x': '调休+2倍',
}

function normalizeDepartmentRules(list: DepartmentRule[] | undefined): DepartmentRule[] {
  return (list || []).map((r) => ({
    group_name: r.group_name || r.group || '',
    match_field: r.match_field || r.level || '',
    match_method: r.match_method || r.match_type || '包含',
    match_value: r.match_value || (r.keywords || []).join(','),
  }))
}

const OvertimeRulesEditor: React.FC<OvertimeRulesEditorProps> = ({
  value,
  onChange,
  canEdit = true,
  disabledReason,
}) => {
  const [messageApi, messageContextHolder] = message.useMessage()
  const [preview, setPreview] = useState<RulesPreview | null>(value?.rules || null)
  const [exporting, setExporting] = useState(false)
  const [importing, setImporting] = useState(false)
  const [uploadFile, setUploadFile] = useState<UploadFile | null>(null)

  const activeSource: RulesSource = value?.source || 'default'

  const emit = (next: AppliedRulesState) => {
    onChange?.(next)
  }

  const handleExport = async () => {
    if (!canEdit) {
      messageApi.warning(disabledReason || '缺少规则编辑权限')
      return
    }
    setExporting(true)
    try {
      // Export current session rules when custom rules are applied; otherwise defaults.
      const currentJson = value?.source === 'custom' ? value.rulesJson || undefined : undefined
      const blob = (await attendanceToolboxAPI.exportRules(currentJson || undefined)) as unknown as Blob
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = currentJson ? '加班规则配置_当前.xlsx' : '加班规则配置_默认.xlsx'
      document.body.appendChild(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)
      messageApi.success(currentJson ? '当前自定义规则已导出' : '默认规则配置已导出')
    } catch {
      messageApi.error('导出失败')
    } finally {
      setExporting(false)
    }
  }

  const handleImport = async () => {
    if (!uploadFile?.originFileObj) {
      messageApi.warning('请先选择规则配置文件')
      return
    }
    if (!canEdit) {
      messageApi.warning(disabledReason || '缺少规则编辑权限')
      return
    }
    setImporting(true)
    try {
      const formData = new FormData()
      formData.append('rules_file', uploadFile.originFileObj)
      const response = await attendanceToolboxAPI.importRulesPreview(formData)
      const data = (response as { data?: RulesPreview })?.data
      if (data) {
        const normalized: RulesPreview = {
          ...data,
          department_rules: normalizeDepartmentRules(data.department_rules),
        }
        setPreview(normalized)
        messageApi.success('规则已解析，预览如下。点击“应用规则”后才会参与计算。')
      }
    } catch {
      messageApi.error('导入预览失败')
    } finally {
      setImporting(false)
    }
  }

  const handleApply = () => {
    if (!preview) {
      messageApi.warning('请先导入或编辑规则')
      return
    }
    if (!canEdit) {
      messageApi.warning(disabledReason || '缺少规则编辑权限')
      return
    }
    const payload = {
      premium_rules: (preview.premium_rules || []).map((r, idx) => ({
        priority: r.priority ?? idx + 1,
        date_type: r.date_type,
        department_group: r.department_group,
        action: r.action,
        multiplier: r.multiplier,
      })),
      department_rules: normalizeDepartmentRules(preview.department_rules),
      params: {
        standard_hours_per_day: preview.params?.standard_hours_per_day ?? 8,
        no_punch_mark: preview.params?.no_punch_mark ?? '未加',
        schedule_augment_holidays: preview.params?.schedule_augment_holidays ?? true,
        schedule_augment_rest_dept_group: preview.params?.schedule_augment_rest_dept_group ?? '运营支撑部',
        chengdu_use_separate_calendar: preview.params?.chengdu_use_separate_calendar ?? true,
      },
      legal_holidays_override: preview.legal_holidays_override || [],
    }
    emit({
      source: 'custom',
      rules: preview,
      rulesJson: JSON.stringify(payload),
    })
    messageApi.success('已应用自定义规则，下次加班计算将使用该规则')
  }

  const handleResetDefault = () => {
    setPreview(null)
    setUploadFile(null)
    emit({ source: 'default', rules: null, rulesJson: null })
    messageApi.success('已恢复默认规则')
  }

  const updateDeptRule = (index: number, patch: Partial<DepartmentRule>) => {
    if (!preview) return
    const next = normalizeDepartmentRules(preview.department_rules).map((r, i) =>
      i === index ? { ...r, ...patch } : r,
    )
    setPreview({ ...preview, department_rules: next })
  }

  const updateParam = (key: keyof NonNullable<RulesPreview['params']>, val: unknown) => {
    if (!preview) return
    setPreview({
      ...preview,
      params: {
        ...(preview.params || {}),
        [key]: val as never,
      },
    })
  }

  const premiumColumns = useMemo(
    () => [
      {
        title: '日期类型',
        dataIndex: 'date_type',
        key: 'date_type',
        render: (v: string) => DATE_TYPE_LABELS[v] || v,
      },
      { title: '部门组', dataIndex: 'department_group', key: 'department_group' },
      {
        title: '倍数',
        dataIndex: 'multiplier',
        key: 'multiplier',
        render: (v: number) => <Tag color="blue">{v}x</Tag>,
      },
      {
        title: '动作',
        dataIndex: 'action',
        key: 'action',
        render: (v: string) => ACTION_LABELS[v] || v,
      },
    ],
    [],
  )

  const departmentColumns = useMemo(
    () => [
      {
        title: '部门组名称',
        dataIndex: 'group_name',
        key: 'group_name',
        render: (v: string, _: DepartmentRule, index: number) => (
          <Input
            value={v}
            disabled={!canEdit}
            onChange={(e) => updateDeptRule(index, { group_name: e.target.value })}
          />
        ),
      },
      {
        title: '匹配字段',
        dataIndex: 'match_field',
        key: 'match_field',
        render: (v: string, _: DepartmentRule, index: number) => (
          <Input
            value={v}
            disabled={!canEdit}
            onChange={(e) => updateDeptRule(index, { match_field: e.target.value })}
          />
        ),
      },
      {
        title: '匹配方式',
        dataIndex: 'match_method',
        key: 'match_method',
        render: (v: string, _: DepartmentRule, index: number) => (
          <Input
            value={v}
            disabled={!canEdit}
            onChange={(e) => updateDeptRule(index, { match_method: e.target.value })}
          />
        ),
      },
      {
        title: '匹配值',
        dataIndex: 'match_value',
        key: 'match_value',
        render: (v: string, _: DepartmentRule, index: number) => (
          <Input
            value={v}
            disabled={!canEdit}
            onChange={(e) => updateDeptRule(index, { match_value: e.target.value })}
          />
        ),
      },
    ],
    [canEdit, preview],
  )

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      {messageContextHolder}
      <Alert
        type={activeSource === 'custom' ? 'success' : 'info'}
        showIcon
        message={
          activeSource === 'custom'
            ? '当前计算使用：自定义规则'
            : '当前计算使用：默认规则'
        }
        description={
          !canEdit
            ? disabledReason || '你缺少考勤工具箱规则编辑权限，需要联系管理员添加'
            : '固定倍数规则只读；部门组与参数可编辑。导入预览后必须点击“应用规则”才会进入加班计算。'
        }
      />

      <Card size="small" title="加班规则配置">
        <Space wrap>
          <Button icon={<DownloadOutlined />} loading={exporting} onClick={handleExport}>
            导出默认规则
          </Button>
          <Upload
            accept=".xlsx,.xls"
            showUploadList={false}
            disabled={!canEdit}
            beforeUpload={(file) => {
              setUploadFile({ uid: '-1', name: file.name, originFileObj: file as any })
              return false
            }}
          >
            <Button icon={<UploadOutlined />} disabled={!canEdit}>
              选择规则文件
            </Button>
          </Upload>
          {uploadFile && <Text type="secondary">{uploadFile.name}</Text>}
          <Button
            type="default"
            loading={importing}
            onClick={handleImport}
            disabled={!uploadFile || !canEdit}
          >
            导入并预览
          </Button>
          <Button
            type="primary"
            icon={<CheckOutlined />}
            onClick={handleApply}
            disabled={!preview || !canEdit}
          >
            应用规则
          </Button>
          <Button icon={<ReloadOutlined />} onClick={handleResetDefault} disabled={!canEdit}>
            恢复默认
          </Button>
        </Space>
      </Card>

      {preview && (
        <Collapse
          defaultActiveKey={['premium', 'department', 'params']}
          items={[
            {
              key: 'premium',
              label: '倍数规则（只读）',
              children: (
                <Table
                  dataSource={preview.premium_rules}
                  columns={premiumColumns}
                  rowKey={(r) => `${r.date_type}-${r.department_group}-${r.priority ?? ''}`}
                  pagination={false}
                  size="small"
                />
              ),
            },
            {
              key: 'department',
              label: '部门匹配规则（可编辑）',
              children: (
                <Table
                  dataSource={normalizeDepartmentRules(preview.department_rules)}
                  columns={departmentColumns}
                  rowKey={(rule) => [rule.group_name, rule.match_field, rule.match_method, rule.match_value].join('|')}
                  pagination={false}
                  size="small"
                />
              ),
            },
            {
              key: 'params',
              label: '参数设置（可编辑）',
              children: (
                <Space direction="vertical" size={8} style={{ width: '100%' }}>
                  <div>
                    <Text strong>标准每日工时：</Text>
                    <InputNumber
                      min={1}
                      max={24}
                      step={0.5}
                      disabled={!canEdit}
                      value={preview.params?.standard_hours_per_day ?? 8}
                      onChange={(v) => updateParam('standard_hours_per_day', Number(v || 8))}
                    />
                  </div>
                  <div>
                    <Text strong>未打卡标记：</Text>
                    <Input
                      style={{ maxWidth: 200 }}
                      disabled={!canEdit}
                      value={preview.params?.no_punch_mark ?? '未加'}
                      onChange={(e) => updateParam('no_punch_mark', e.target.value)}
                    />
                  </div>
                  <div>
                    <Text strong>排班补录-休息日部门组：</Text>
                    <Input
                      style={{ maxWidth: 240 }}
                      disabled={!canEdit}
                      value={preview.params?.schedule_augment_rest_dept_group ?? '运营支撑部'}
                      onChange={(e) => updateParam('schedule_augment_rest_dept_group', e.target.value)}
                    />
                  </div>
                </Space>
              ),
            },
          ]}
        />
      )}
    </Space>
  )
}

export default OvertimeRulesEditor
