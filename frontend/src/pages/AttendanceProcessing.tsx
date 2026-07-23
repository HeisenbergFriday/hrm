import React, { useState } from 'react'
import { Tabs, Upload, Button, message, Spin, Typography, Space, Alert, Tooltip } from 'antd'
import {
  UploadOutlined,
  FileExcelOutlined,
  DeleteOutlined,
  ToolOutlined,
} from '@ant-design/icons'
import type { UploadFile } from 'antd/es/upload/interface'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import { attendanceAPI } from '../services/api'
import { hasPermission } from '../utils/permission'

const { Text } = Typography

interface FileState {
  [key: string]: UploadFile[]
}

interface ProcessingTab {
  key: string
  label: string
  description: string
  files: {
    key: string
    label: string
    required: boolean
    accept?: string
    multiple?: boolean
  }[]
  apiFn: (formData: FormData) => Promise<any>
  outputName: string
}

const processingTabs: ProcessingTab[] = [
  {
    key: 'leave',
    label: '请假明细',
    description: '上传请假系统导出表和作息表，自动计算最终请假时长、天数和备注。',
    files: [
      { key: 'input', label: '请假系统导出表', required: true, accept: '.xlsx,.xls' },
      { key: 'schedule', label: '作息表', required: true, accept: '.xlsx,.xls' },
    ],
    apiFn: attendanceAPI.processing.leave,
    outputName: '请假明细表.xlsx',
  },
  {
    key: 'overtime',
    label: '加班明细',
    description: '上传加班系统导出表，自动回填加班类型、倍数、时长等字段。排班表、考勤打卡明细、花名册为可选项。',
    files: [
      { key: 'input', label: '加班系统导出表', required: true, accept: '.xlsx,.xls' },
      { key: 'schedule', label: '排班表', required: false, accept: '.xlsx,.xls' },
      { key: 'attendance', label: '考勤打卡明细', required: false, accept: '.xlsx,.xls' },
      { key: 'roster', label: '花名册', required: false, accept: '.xlsx,.xls' },
    ],
    apiFn: attendanceAPI.processing.overtime,
    outputName: '加班明细_回填.xlsx',
  },
  {
    key: 'subsidy',
    label: '补贴扣款',
    description: '上传补贴扣款表、考勤表和作息表，自动核对旷工、缺卡、晚走补贴等数据，生成核对表和差异清单。',
    files: [
      { key: 'source', label: '补贴扣款源表', required: true, accept: '.xlsx,.xls' },
      { key: 'attendance', label: '考勤月度汇总', required: true, accept: '.xlsx,.xls' },
      { key: 'schedule', label: '作息表', required: true, accept: '.xlsx,.xls' },
      { key: 'signin', label: '签到表', required: false, accept: '.xlsx,.xls' },
      { key: 'result', label: '考勤结果表', required: false, accept: '.xlsx,.xls' },
    ],
    apiFn: attendanceAPI.processing.subsidy,
    outputName: '补贴扣款表_核对.xlsx',
  },
  {
    key: 'final',
    label: '最终表',
    description: '汇总花名册、请假、加班、补贴扣款数据，生成最终考勤汇总表。',
    files: [
      { key: 'roster', label: '在职花名册', required: true, accept: '.xlsx,.xls' },
      { key: 'schedule', label: '作息表', required: true, accept: '.xlsx,.xls' },
      { key: 'leave', label: '请假明细表', required: true, accept: '.xlsx,.xls' },
      { key: 'overtime', label: '加班明细_回填', required: true, accept: '.xlsx,.xls' },
      { key: 'subsidy', label: '补贴扣款表_核对', required: true, accept: '.xlsx,.xls' },
      { key: 'resigned', label: '离职花名册', required: false, accept: '.xlsx,.xls' },
      { key: 'transfer', label: '异动流程表', required: false, accept: '.xlsx,.xls' },
    ],
    apiFn: attendanceAPI.processing.final,
    outputName: '最终表.xlsx',
  },
  {
    key: 'parttime',
    label: '兼职汇总',
    description: '汇总兼职/实习/外包人员的考勤数据，支持月度汇总、排班表、默认作息表等多种数据源。',
    files: [
      { key: 'default_schedule', label: '默认作息表', required: false, accept: '.xlsx,.xls' },
      { key: 'attendance_detail', label: '考勤明细', required: false, accept: '.xlsx,.xls' },
      { key: 'monthly_summary', label: '月度汇总', required: false, accept: '.xlsx,.xls', multiple: true },
      { key: 'schedule', label: '排班表', required: false, accept: '.xlsx,.xls', multiple: true },
    ],
    apiFn: attendanceAPI.processing.parttime,
    outputName: '兼职汇总.xlsx',
  },
]

const AttendanceProcessing: React.FC = () => {
  const [activeTab, setActiveTab] = useState('leave')
  const [fileLists, setFileLists] = useState<FileState>({})
  const [loading, setLoading] = useState(false)
  const canManage = hasPermission('attendance_manage')

  const currentTab = processingTabs.find((t) => t.key === activeTab)!

  const getFileList = (key: string): UploadFile[] => fileLists[`${activeTab}_${key}`] || []

  const setFileList = (key: string, files: UploadFile[]) => {
    setFileLists((prev) => ({ ...prev, [`${activeTab}_${key}`]: files }))
  }

  const handleProcess = async () => {
    if (!canManage) {
      message.warning('你缺少 attendance_manage 权限，需要联系管理员添加')
      return
    }
    // 验证必选文件
    const missing = currentTab.files
      .filter((f) => f.required && getFileList(f.key).length === 0)
      .map((f) => f.label)

    if (missing.length > 0) {
      message.warning(`请上传必选文件: ${missing.join('、')}`)
      return
    }

    setLoading(true)
    try {
      const formData = new FormData()

      for (const fileDef of currentTab.files) {
        const files = getFileList(fileDef.key)
        for (const file of files) {
          if (file.originFileObj) {
            if (fileDef.multiple) {
              formData.append(fileDef.key, file.originFileObj)
            } else {
              formData.append(fileDef.key, file.originFileObj)
            }
          }
        }
      }

      const response = await currentTab.apiFn(formData)

      // api 拦截器已返回 response.data；blob 接口这里拿到的是 Blob 本身
      const blobData = response instanceof Blob ? response : response?.data
      const blob = blobData instanceof Blob ? blobData : new Blob([blobData])
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = currentTab.outputName
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      window.URL.revokeObjectURL(url)

      message.success('处理完成，文件已下载')
    } catch (err: any) {
      // 尝试解析 blob 错误响应
      if (err.response?.data instanceof Blob) {
        try {
          const text = await err.response.data.text()
          const json = JSON.parse(text)
          message.error(json.message || '处理失败')
        } catch {
          message.error('处理失败')
        }
      } else {
        message.error(err.message || '处理失败')
      }
    } finally {
      setLoading(false)
    }
  }

  const handleClearFiles = () => {
    const keys = currentTab.files.map((f) => `${activeTab}_${f.key}`)
    setFileLists((prev) => {
      const next = { ...prev }
      for (const key of keys) {
        delete next[key]
      }
      return next
    })
  }

  const renderTabContent = (tab: ProcessingTab) => (
    <div style={{ maxWidth: 640 }}>
      <Alert
        message={tab.description}
        type="info"
        showIcon
        style={{ marginBottom: 24 }}
      />

      {tab.files.map((fileDef) => (
        <div key={fileDef.key} style={{ marginBottom: 16 }}>
          <Text strong style={{ display: 'block', marginBottom: 4 }}>
            {fileDef.label}
            {fileDef.required && <span style={{ color: 'var(--color-error)' }}> *</span>}
            {!fileDef.required && (
              <Text type="secondary" style={{ fontSize: 12, marginLeft: 8 }}>
                可选
              </Text>
            )}
          </Text>
          <Upload
            accept={fileDef.accept}
            fileList={getFileList(fileDef.key)}
            beforeUpload={() => false}
            onChange={({ fileList }) => setFileList(fileDef.key, fileList)}
            multiple={fileDef.multiple}
            maxCount={fileDef.multiple ? undefined : 1}
          >
            <Button icon={<UploadOutlined />}>选择文件</Button>
          </Upload>
        </div>
      ))}

      <Space style={{ marginTop: 24 }}>
        <Tooltip title={canManage ? undefined : '你缺少 attendance_manage 权限，需要联系管理员添加'}>
          <Button
            type="primary"
            icon={<ToolOutlined />}
            onClick={handleProcess}
            loading={loading}
            disabled={!canManage}
            size="large"
          >
            开始处理
          </Button>
        </Tooltip>
        <Button icon={<DeleteOutlined />} onClick={handleClearFiles} disabled={loading}>
          清空文件
        </Button>
      </Space>
    </div>
  )

  return (
    <PageContainer
      title="考勤数据处理"
      icon={<FileExcelOutlined />}
      subtitle="上传 Excel 文件，自动计算请假、加班、补贴扣款等考勤数据"
    >
      <PageCard>
        <Spin spinning={loading} tip="处理中，请稍候...">
          <Tabs
            activeKey={activeTab}
            onChange={setActiveTab}
            items={processingTabs.map((tab) => ({
              key: tab.key,
              label: tab.label,
              children: renderTabContent(tab),
            }))}
          />
        </Spin>
      </PageCard>
    </PageContainer>
  )
}

export default AttendanceProcessing
