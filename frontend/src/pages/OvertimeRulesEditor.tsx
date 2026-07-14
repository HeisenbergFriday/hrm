import React, { useState } from 'react'
import { Button, Card, Collapse, Input, InputNumber, Space, Table, Tag, Typography, Upload, message } from 'antd'
import type { UploadFile } from 'antd'
import { DownloadOutlined, UploadOutlined } from '@ant-design/icons'
import { attendanceToolboxAPI } from '../services/api'

const { Text } = Typography

interface PremiumRule {
  date_type: string
  department_group: string
  multiplier: number
  action: string
}

interface DepartmentRule {
  level: string
  match_type: string
  keywords: string[]
  group: string
}

interface RulesPreview {
  premium_rules: PremiumRule[]
  department_rules: DepartmentRule[]
  params: {
    rest_premium_excluded_names: string[]
    rest_premium_excluded_codes: string[]
  }
}

const DATE_TYPE_LABELS: Record<string, string> = {
  LEGAL_HOLIDAY: '法定节假日',
  HOLIDAY_ADJUST_REST: '调休休息日',
  ORDINARY_WEEKEND: '普通周末',
  MAKEUP_WORKDAY: '调休上班',
  NORMAL_WORKDAY: '普通工作日',
}

const ACTION_LABELS: Record<string, string> = {
  '3x': '3倍',
  '2x': '2倍',
  rest: '调休',
  'rest+2x': '调休+2倍',
}

const OvertimeRulesEditor: React.FC = () => {
  const [preview, setPreview] = useState<RulesPreview | null>(null)
  const [exporting, setExporting] = useState(false)
  const [importing, setImporting] = useState(false)
  const [uploadFile, setUploadFile] = useState<UploadFile | null>(null)

  const handleExport = async () => {
    setExporting(true)
    try {
      const blob = (await attendanceToolboxAPI.exportRules()) as unknown as Blob
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = '加班规则配置.xlsx'
      document.body.appendChild(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)
      message.success('规则配置已导出')
    } catch {
      message.error('导出失败')
    } finally {
      setExporting(false)
    }
  }

  const handleImport = async () => {
    if (!uploadFile?.originFileObj) {
      message.warning('请先选择规则配置文件')
      return
    }
    setImporting(true)
    try {
      const formData = new FormData()
      formData.append('rules_file', uploadFile.originFileObj)
      const response = await attendanceToolboxAPI.importRulesPreview(formData)
      const data = (response as { data?: RulesPreview })?.data
      if (data) {
        setPreview(data)
        message.success('规则已解析，预览如下')
      }
    } catch {
      message.error('导入预览失败')
    } finally {
      setImporting(false)
    }
  }

  const premiumColumns = [
    { title: '日期类型', dataIndex: 'date_type', key: 'date_type', render: (v: string) => DATE_TYPE_LABELS[v] || v },
    { title: '部门组', dataIndex: 'department_group', key: 'department_group' },
    { title: '倍数', dataIndex: 'multiplier', key: 'multiplier', render: (v: number) => <Tag color="blue">{v}x</Tag> },
    { title: '动作', dataIndex: 'action', key: 'action', render: (v: string) => ACTION_LABELS[v] || v },
  ]

  const departmentColumns = [
    { title: '部门层级', dataIndex: 'level', key: 'level' },
    { title: '匹配方式', dataIndex: 'match_type', key: 'match_type' },
    { title: '关键词', dataIndex: 'keywords', key: 'keywords', render: (v: string[]) => v?.map((k) => <Tag key={k}>{k}</Tag>) },
    { title: '所属组', dataIndex: 'group', key: 'group' },
  ]

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card size="small" title="加班规则配置">
        <Space wrap>
          <Button
            icon={<DownloadOutlined />}
            loading={exporting}
            onClick={handleExport}
          >
            导出默认规则
          </Button>
          <Upload
            accept=".xlsx,.xls"
            showUploadList={false}
            beforeUpload={(file) => {
              setUploadFile({ uid: '-1', name: file.name, originFileObj: file as any })
              return false
            }}
          >
            <Button icon={<UploadOutlined />}>选择规则文件</Button>
          </Upload>
          {uploadFile && (
            <Text type="secondary">{uploadFile.name}</Text>
          )}
          <Button
            type="primary"
            loading={importing}
            onClick={handleImport}
            disabled={!uploadFile}
          >
            导入并预览
          </Button>
        </Space>
      </Card>

      {preview && (
        <Collapse
          defaultActiveKey={['premium', 'department', 'params']}
          items={[
            {
              key: 'premium',
              label: '倍数规则',
              children: (
                <Table
                  dataSource={preview.premium_rules}
                  columns={premiumColumns}
                  rowKey={(r) => `${r.date_type}-${r.department_group}`}
                  pagination={false}
                  size="small"
                />
              ),
            },
            {
              key: 'department',
              label: '部门匹配规则',
              children: (
                <Table
                  dataSource={preview.department_rules}
                  columns={departmentColumns}
                  rowKey={(r) => `${r.level}-${r.group}`}
                  pagination={false}
                  size="small"
                />
              ),
            },
            {
              key: 'params',
              label: '排除名单',
              children: (
                <Space direction="vertical" size={8}>
                  <div>
                    <Text strong>调休倍数排除人员：</Text>
                    {preview.params?.rest_premium_excluded_names?.length > 0
                      ? preview.params.rest_premium_excluded_names.map((n) => <Tag key={n}>{n}</Tag>)
                      : <Text type="secondary">无</Text>}
                  </div>
                  <div>
                    <Text strong>调休倍数排除工号：</Text>
                    {preview.params?.rest_premium_excluded_codes?.length > 0
                      ? preview.params.rest_premium_excluded_codes.map((c) => <Tag key={c}>{c}</Tag>)
                      : <Text type="secondary">无</Text>}
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
