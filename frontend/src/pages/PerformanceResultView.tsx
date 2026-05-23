import React, { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  Card, Typography, Button, Space, message, Spin, Row, Col, Table, Tag, Descriptions, Timeline, InputNumber, Form, Modal, Image
} from 'antd'
import { ArrowLeftOutlined, CheckCircleOutlined, LockOutlined, EditOutlined, PrinterOutlined, FileExcelOutlined } from '@ant-design/icons'
import { performanceAPI, PerformanceActivity, PerformanceGoalRecord, PerformanceParticipant } from '../services/api'

const { Title, Text } = Typography

const LEVEL_COLOR: Record<string, string> = {
  S: 'red', A: 'orange', B: 'green', C: 'gold', D: 'volcano'
}

const SECTION_LABEL: Record<string, string> = {
  quantitative: '量化指标',
  key_action: '关键行动',
  bonus_penalty: '附加考核项'
}

function formatScore(value?: number) {
  if (value === undefined || value === null) return '-'
  return Number(value).toFixed(0)
}

function formatDecimal(value?: number) {
  if (value === undefined || value === null) return '-'
  return Number(value).toFixed(1)
}

function formatWeight(value?: number) {
  if (!value) return '-'
  return `${(value * 100).toFixed(0)}%`
}

function formatDate(value?: string) {
  if (!value) return '-'
  return value.substring(0, 10)
}

function formatSignature(name?: string, date?: string) {
  const normalizedName = name?.trim()
  const normalizedDate = formatDate(date)

  if (!normalizedName && normalizedDate === '-') return '-'
  return [normalizedName || '-', normalizedDate].filter(part => part && part !== '-').join(' ')
}

function formatPeriod(startDate?: string, endDate?: string) {
  if (!startDate || !endDate) return '-'

  const start = startDate.substring(0, 10)
  const end = endDate.substring(0, 10)
  const [startYear, startMonth] = start.split('-')
  const [endYear, endMonth] = end.split('-')

  if (startYear && startMonth && startYear === endYear && startMonth === endMonth) {
    return `${startYear}年${Number(startMonth)}月`
  }

  return `${start} 至 ${end}`
}

function getWeightedScore(record: PerformanceGoalRecord, scoreType: 'self' | 'manager') {
  const score = scoreType === 'self' ? record.self_score : record.manager_score
  return (score || 0) * (record.weight || 0)
}

function getDownloadFileName(activity: PerformanceActivity | null, participant: PerformanceParticipant | null) {
  const base = `${activity?.name || '绩效考核'}-${participant?.employee_name || '员工'}-个人绩效考核表`
  return base.replace(/[\\/:*?"<>|]/g, '_')
}

interface ArchiveSheetProps {
  activity: PerformanceActivity | null
  participant: PerformanceParticipant | null
  records: PerformanceGoalRecord[]
}

const ArchivePerformanceSheet: React.FC<ArchiveSheetProps> = ({ activity, participant, records }) => {
  const mainRecords = records.filter(record => record.section_type !== 'bonus_penalty')
  const quantitativeRecords = mainRecords
    .filter(record => record.section_type === 'quantitative')
    .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0) || a.id - b.id)
  const keyActionRecords = mainRecords
    .filter(record => record.section_type === 'key_action')
    .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0) || a.id - b.id)
  const selfEvaluationGood = participant?.self_evaluation_good || ''
  const selfEvaluationImprovement = participant?.self_evaluation_improvement || ''
  const managerEvaluationGood = participant?.manager_evaluation_good || ''
  const managerEvaluationImprovement = participant?.manager_evaluation_improvement || ''
  const totalWeight = mainRecords.reduce((sum, record) => sum + (record.weight || 0), 0)
  const totalSelfScore = participant?.total_self_score ?? participant?.self_score ?? mainRecords.reduce((sum, record) => sum + getWeightedScore(record, 'self'), 0)
  const totalManagerScore = participant?.total_manager_score ?? participant?.manager_score ?? mainRecords.reduce((sum, record) => sum + getWeightedScore(record, 'manager'), 0)
  const period = formatPeriod(activity?.start_date, activity?.end_date)
  const archiveTitle = activity?.name?.includes('绩效考核表')
    ? activity.name
    : period !== '-'
      ? `${period}${participant?.department_name || ''}绩效考核表`
      : activity?.name || '个人绩效考核表'
  const participantExtra = participant as any
  const employeeResultSignature = formatSignature(
    participant?.employee_confirmed_by || participant?.employee_name,
    participant?.employee_confirmed_at || participant?.confirmed_at
  )
  const managerResultSignature = formatSignature(
    participant?.manager_confirmed_by || participant?.manager_name,
    participant?.manager_confirmed_at
  )
  const hrResultSignature = formatSignature(
    participant?.hr_confirmed_by,
    participant?.hr_confirmed_at
  )
  const employeeTargetSignature = formatSignature(
    participantExtra?.employee_target_confirmed_by || participant?.employee_confirmed_by || participant?.employee_name,
    participantExtra?.employee_target_confirmed_at || participant?.employee_confirmed_at || participant?.confirmed_at
  )
  const managerTargetSignature = formatSignature(
    participantExtra?.manager_target_confirmed_by || participant?.manager_confirmed_by || participant?.manager_name,
    participantExtra?.manager_target_confirmed_at || participant?.manager_confirmed_at
  )
  const hrTargetSignature = formatSignature(
    participantExtra?.hr_target_confirmed_by || participant?.hr_confirmed_by,
    participantExtra?.hr_target_confirmed_at || participant?.hr_confirmed_at
  )
  const levelConfirmSignature = formatSignature(
    participant?.manager_confirmed_by || participant?.manager_name || participant?.hr_confirmed_by,
    participant?.manager_confirmed_at || participant?.hr_confirmed_at
  )

  const getKeyActionCriteria = (record: PerformanceGoalRecord) => {
    const values = [record.target_value, record.scoring_rule].filter(Boolean)
    const uniqueValues = values.filter((value, index) => values.indexOf(value) === index)
    return uniqueValues.length > 0 ? uniqueValues.join('\n') : '-'
  }

  const renderSectionRows = (
    sectionRecords: PerformanceGoalRecord[],
    label: React.ReactNode,
    mode: 'quantitative' | 'key_action'
  ) => {
    const rowCount = Math.max(sectionRecords.length, 1)

    if (sectionRecords.length === 0) {
      return (
        <tr className="archive-data-row">
          <td className="archive-category-cell">{label}</td>
          <td>-</td>
          <td className="archive-text-cell">-</td>
          <td>-</td>
          {mode === 'quantitative' ? (
            <>
              <td>-</td>
              <td>-</td>
              <td>-</td>
              <td>-</td>
            </>
          ) : (
            <td colSpan={4}>-</td>
          )}
          <td>-</td>
          <td>-</td>
          <td>-</td>
        </tr>
      )
    }

    return sectionRecords.map((record, index) => (
      <tr key={record.id} className="archive-data-row">
        {index === 0 && (
          <td rowSpan={rowCount} className="archive-category-cell">{label}</td>
        )}
        <td>{record.item_name || '-'}</td>
        <td className="archive-text-cell">{record.item_definition || '-'}</td>
        <td>{formatWeight(record.weight)}</td>
        {mode === 'quantitative' ? (
          <>
            <td>{record.red_line_value || '-'}</td>
            <td>{record.target_value || '-'}</td>
            <td>{record.challenge_value || '-'}</td>
            <td className="archive-text-cell">{record.scoring_rule || '-'}</td>
          </>
        ) : (
          <td colSpan={4} className="archive-text-cell">{getKeyActionCriteria(record)}</td>
        )}
        <td className="archive-text-cell">{record.actual_result || '-'}</td>
        <td>{formatScore(record.self_score)}</td>
        <td>{formatScore(record.manager_score)}</td>
      </tr>
    ))
  }

  return (
    <div id="performance-archive-sheet" className="performance-archive-sheet">
      <table className="archive-table archive-excel-table">
        <colgroup>
          <col style={{ width: '5.5%' }} />
          <col style={{ width: '11%' }} />
          <col style={{ width: '22.5%' }} />
          <col style={{ width: '5%' }} />
          <col style={{ width: '7.2%' }} />
          <col style={{ width: '7.2%' }} />
          <col style={{ width: '7.2%' }} />
          <col style={{ width: '17.5%' }} />
          <col style={{ width: '8%' }} />
          <col style={{ width: '4.5%' }} />
          <col style={{ width: '4.4%' }} />
        </colgroup>
        <tbody>
          <tr className="archive-top-row">
            <td className="archive-logo-cell" colSpan={2}>
              <span className="archive-logo-mark" />
              <span className="archive-logo-main">小铁</span>
              <span className="archive-logo-sub">自助台球</span>
            </td>
            <td className="archive-title-cell" colSpan={9}>{archiveTitle}</td>
          </tr>
          <tr className="archive-info-head">
            <th rowSpan={2} colSpan={2}>基础信息</th>
            <th colSpan={2}>姓名</th>
            <th>一级部门</th>
            <th>二级部门</th>
            <th>三级部门</th>
            <th>职级</th>
            <th>岗位</th>
            <th>直属上级</th>
            <th>考核周期</th>
          </tr>
          <tr className="archive-info-value">
            <td colSpan={2}>{participant?.employee_name || '-'}</td>
            <td>{participant?.department_name || '-'}</td>
            <td>-</td>
            <td>-</td>
            <td>{participant?.level || '-'}</td>
            <td>{participant?.position || '-'}</td>
            <td>{participant?.manager_name || '-'}</td>
            <td>{period}</td>
          </tr>

          <tr className="archive-section-row">
            <th colSpan={11}>PARTB: 个人绩效（员工绩效）</th>
          </tr>
          <tr className="archive-main-head">
            <th rowSpan={2}>类别</th>
            <th rowSpan={2}>指标名称/重点计划</th>
            <th rowSpan={2}>
              指标定义及口径说明
              <div className="archive-head-note">（明确的指标范围和计算公式）</div>
            </th>
            <th rowSpan={2}>
              权重
              <div className="archive-head-note">（5%的倍数且单项不低于10%）</div>
            </th>
            <th colSpan={3}>定量/定性目标</th>
            <th rowSpan={2}>
              考核标准
              <div className="archive-head-note">（定量分段设置，上限120分；定性按达成度/质量分级，上限100分）</div>
            </th>
            <th rowSpan={2}>实际达成结果</th>
            <th rowSpan={2}>自评得分</th>
            <th rowSpan={2}>上级评分<br /><span className="archive-head-note">（上限120分）</span></th>
          </tr>
          <tr className="archive-main-subhead">
            <th>红线值</th>
            <th>目标值</th>
            <th>挑战值</th>
          </tr>

          {renderSectionRows(
            quantitativeRecords,
            <>
              <div>量化指标</div>
              <div className="archive-category-note">（2-5项，权重<br />70%）</div>
            </>,
            'quantitative'
          )}
          {renderSectionRows(
            keyActionRecords,
            <>
              <div>关键行动</div>
              <div className="archive-category-note">（3-5项，权重<br />30%）</div>
            </>,
            'key_action'
          )}

          <tr className="archive-total-row">
            <td colSpan={3}>合计</td>
            <td>{formatWeight(totalWeight)}</td>
            <td>-</td>
            <td>-</td>
            <td>-</td>
            <td>-</td>
            <td>-</td>
            <td>{formatDecimal(totalSelfScore)}</td>
            <td>{formatDecimal(totalManagerScore)}</td>
          </tr>
          <tr className="archive-review-head">
            <th colSpan={5}>做得好的地方</th>
            <th colSpan={6}>需要提高改进的地方</th>
          </tr>
          <tr className="archive-review-row">
            <td colSpan={2} className="archive-evaluation-title">员工自我评价</td>
            <td colSpan={3} className="archive-evaluation-cell">
              {selfEvaluationGood || ''}
            </td>
            <td colSpan={2} className="archive-evaluation-title">员工自我评价</td>
            <td colSpan={4} className="archive-evaluation-cell">
              {selfEvaluationImprovement || ''}
            </td>
          </tr>
          <tr className="archive-review-row">
            <td colSpan={2} className="archive-evaluation-title">上级总体评价</td>
            <td colSpan={3} className="archive-evaluation-cell">
              {managerEvaluationGood || ''}
            </td>
            <td colSpan={2} className="archive-evaluation-title">上级总体评价</td>
            <td colSpan={4} className="archive-evaluation-cell">
              {managerEvaluationImprovement || ''}
            </td>
          </tr>

          <tr className="archive-section-row">
            <th colSpan={11}>员工绩效等级（S A B C D）</th>
          </tr>
          <tr className="archive-level-label-row">
            <td colSpan={4}>个人绩效评定结果/等级</td>
            <td colSpan={4}>绩效面谈进度</td>
            <td colSpan={3}>个人价值观等级(季度评)</td>
          </tr>
          <tr className="archive-level-value-row">
            <td colSpan={4} className="archive-level-cell">{participant?.final_level || participant?.suggested_level || '-'}</td>
            <td colSpan={4}>{participant?.final_level === 'C' || participant?.final_level === 'D' ? '需完成绩效面谈' : '按需绩效面谈'}</td>
            <td colSpan={3} />
          </tr>
          <tr className="archive-sign-row">
            <td colSpan={3} className="archive-sign-cell">个人绩效目标确认签名/日期：{employeeTargetSignature}</td>
            <td colSpan={3} className="archive-sign-cell">上级绩效目标确认签名/日期：{managerTargetSignature}</td>
            <td colSpan={2} className="archive-sign-cell">个人绩效结果确认签名/日期：{employeeResultSignature}</td>
            <td colSpan={3} className="archive-sign-cell">上级绩效结果确认签名/日期：{managerResultSignature}</td>
          </tr>
          <tr className="archive-sign-row archive-sign-final-row">
            <td colSpan={4} className="archive-sign-confirm">人力确认签名/日期：{hrTargetSignature}</td>
            <td colSpan={4} className="archive-sign-confirm">人力结果确认签名/日期：{hrResultSignature}</td>
            <td colSpan={3} className="archive-sign-confirm">
              绩效等级确认：{levelConfirmSignature}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  )
}

const archiveStyles = `
.performance-page .archive-actions {
  margin-bottom: 16px;
}
.performance-page .performance-archive-card {
  margin-top: 24px;
}
.performance-page .performance-archive-card .ant-card-body {
  overflow-x: auto;
}
.performance-archive-sheet {
  background: #fff;
  color: #000;
  font-family: SimSun, "Microsoft YaHei", Arial, sans-serif;
  font-size: 12px;
  min-width: 1560px;
  overflow-x: visible;
}
.archive-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
  border: 2px solid #000;
}
.archive-table th,
.archive-table td {
  border: 1px solid #000;
  padding: 3px 4px;
  text-align: center;
  vertical-align: middle;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.2;
}
.archive-table th {
  font-weight: 700;
}
.archive-top-row td {
  height: 28px;
  border-top: 3px solid #2b64ff;
}
.archive-logo-cell {
  background: #fff;
  text-align: left !important;
  font-family: "Microsoft YaHei", SimHei, Arial, sans-serif;
  font-weight: 700;
  white-space: nowrap !important;
}
.archive-logo-mark {
  display: inline-block;
  width: 22px;
  height: 22px;
  margin-right: 4px;
  vertical-align: middle;
  border: 2px solid #2b64ff;
  border-top-color: #f5cc17;
  border-radius: 50%;
}
.archive-logo-main {
  display: inline-block;
  margin-right: 6px;
  vertical-align: middle;
  font-size: 22px;
  font-style: italic;
  line-height: 1;
}
.archive-logo-sub {
  display: inline-block;
  vertical-align: middle;
  font-size: 14px;
}
.archive-title-cell {
  background: #fff;
  font-family: SimHei, "Microsoft YaHei", Arial, sans-serif;
  font-size: 16px;
  font-weight: 700;
}
.archive-info-head th,
.archive-info-value td,
.archive-main-head th,
.archive-main-subhead th,
.archive-level-label-row td,
.archive-level-value-row td {
  background: #fff2cc;
}
.archive-info-head th,
.archive-info-value td {
  height: 32px;
}
.archive-section-row th,
.archive-review-head th {
  background: #ffc000;
  height: 28px;
  font-weight: 700;
}
.archive-main-head th {
  height: 50px;
}
.archive-main-subhead th {
  height: 28px;
}
.archive-head-note {
  margin-top: 2px;
  font-size: 10px;
  font-weight: 400;
  line-height: 1.15;
}
.archive-text-cell {
  text-align: left !important;
  vertical-align: top !important;
}
.archive-data-row td {
  height: 44px;
  min-height: 44px;
}
.archive-category-cell {
  background: #fff;
  font-weight: 700;
}
.archive-category-note {
  margin-top: 4px;
  font-weight: 400;
  line-height: 1.25;
}
.archive-total-row td {
  background: #fff2cc;
  height: 30px;
  font-weight: 700;
}
.archive-review-head th {
  background: #fff2cc;
  height: 30px;
}
.archive-review-row td {
  height: 30px;
}
.archive-evaluation-cell {
  text-align: left !important;
  color: #d00;
  vertical-align: top !important;
}
.archive-evaluation-title {
  color: #000;
  font-weight: 700;
}
.archive-level-label-row td,
.archive-level-value-row td {
  height: 40px;
  font-weight: 700;
}
.archive-level-cell {
  color: #d00;
  font-weight: 700;
}
.archive-sign-row td {
  height: 44px;
  background: #fff;
  text-align: left !important;
  font-weight: 700;
}
.archive-sign-final-row td {
  color: #f00;
  text-align: center !important;
}
.archive-sign-cell,
.archive-sign-confirm {
  padding-left: 4px !important;
}
@media print {
  @page {
    size: A4 landscape;
    margin: 6mm;
  }
  body * {
    visibility: hidden !important;
  }
  #performance-archive-sheet,
  #performance-archive-sheet * {
    visibility: visible !important;
  }
  #performance-archive-sheet {
    position: absolute;
    left: 0;
    top: 0;
    width: 100%;
    min-width: 0;
    overflow: visible;
    font-size: 9px;
  }
  .archive-table th,
  .archive-table td {
    padding: 2px 3px;
  }
  .archive-logo-main {
    font-size: 18px;
  }
  .archive-logo-sub {
    font-size: 11px;
  }
  .archive-title-cell {
    font-size: 14px;
  }
  .archive-head-note {
    font-size: 8px;
  }
}
`

const PerformanceResultView: React.FC = () => {
  const { activityId, participantId } = useParams<{ activityId: string; participantId: string }>()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(true)
  const [records, setRecords] = useState<PerformanceGoalRecord[]>([])
  const [participant, setParticipant] = useState<PerformanceParticipant | null>(null)
  const [activity, setActivity] = useState<PerformanceActivity | null>(null)
  const [confirming, setConfirming] = useState(false)
  const [confirmType, setConfirmType] = useState<'employee' | 'manager' | 'hr' | null>(null)
  const [bonusPenaltyModalVisible, setBonusPenaltyModalVisible] = useState(false)
  const [bonusPenaltyForm] = Form.useForm()
  const [savingBonusPenalty, setSavingBonusPenalty] = useState(false)

  const loadData = useCallback(async () => {
    if (!participantId) return
    setLoading(true)
    try {
      const [recordsRes, participantRes] = await Promise.all([
        performanceAPI.getGoalRecords(Number(participantId)),
        performanceAPI.getParticipant(Number(participantId))
      ])
      setRecords(recordsRes.data?.items || [])
      setParticipant(participantRes.data?.participant || participantRes.data)
      setActivity(participantRes.data?.activity || null)
    } catch {
      message.error('加载数据失败')
    } finally {
      setLoading(false)
    }
  }, [participantId])

  useEffect(() => { loadData() }, [loadData])

  const handleSetBonusPenalty = () => {
    bonusPenaltyForm.setFieldsValue({
      bonus_score: participant?.bonus_score || 0,
      penalty_score: participant?.penalty_score || 0
    })
    setBonusPenaltyModalVisible(true)
  }

  const handleSaveBonusPenalty = async () => {
    try {
      const values = await bonusPenaltyForm.validateFields()
      setSavingBonusPenalty(true)
      await performanceAPI.setBonusPenaltyScore(Number(participantId), values.bonus_score, values.penalty_score)
      message.success('附加项设置成功')
      setBonusPenaltyModalVisible(false)
      loadData()
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '设置失败')
    } finally {
      setSavingBonusPenalty(false)
    }
  }

  const doConfirm = async (type: 'employee' | 'manager' | 'hr') => {
    setConfirming(true)
    setConfirmType(type)
    try {
      switch (type) {
        case 'employee':
          await performanceAPI.confirmEmployeeResult(Number(participantId))
          message.success('确认成功')
          break
        case 'manager':
          await performanceAPI.confirmManagerResult(Number(participantId))
          message.success('主管确认成功，结果已冻结')
          break
        case 'hr':
          await performanceAPI.confirmHRResult(Number(participantId))
          message.success('HR确认成功')
          break
      }
      loadData()
    } catch (err: any) {
      message.error(err?.response?.data?.message || '确认失败')
    } finally {
      setConfirming(false)
      setConfirmType(null)
    }
  }

  const handleConfirm = async (type: 'employee' | 'manager' | 'hr') => {
    if (type === 'manager') {
      Modal.confirm({
        title: '主管确认并冻结绩效结果',
        content: '确认后该员工绩效结果将立即冻结，评分、等级和附加项将无法再修改。',
        okText: '确认并冻结',
        cancelText: '取消',
        onOk: () => doConfirm(type)
      })
      return
    }

    await doConfirm(type)
  }

  const handlePrint = () => {
    window.print()
  }

  const handleExportExcel = () => {
    const sheet = document.getElementById('performance-archive-sheet')
    if (!sheet) {
      message.error('未找到可导出的绩效考核表')
      return
    }

    const html = `<!doctype html><html><head><meta charset="UTF-8"><style>${archiveStyles}</style></head><body>${sheet.outerHTML}</body></html>`
    const blob = new Blob([html], { type: 'application/vnd.ms-excel;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `${getDownloadFileName(activity, participant)}.xls`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
  }

  const columns = [
    {
      title: '类别',
      dataIndex: 'section_type',
      key: 'section_type',
      width: 90,
      render: (val: string) => (
        <Tag color={val === 'quantitative' ? 'blue' : val === 'bonus_penalty' ? 'gold' : 'green'}>
          {SECTION_LABEL[val] || val}
        </Tag>
      )
    },
    { title: '指标名称', dataIndex: 'item_name', key: 'item_name', width: 150 },
    { title: '权重', dataIndex: 'weight', key: 'weight', width: 70, render: (v: number) => formatWeight(v) },
    { title: '实际达成', dataIndex: 'actual_result', key: 'actual_result', width: 200 },
    { title: '自评得分', dataIndex: 'self_score', key: 'self_score', width: 80 },
    { title: '上级评分', dataIndex: 'manager_score', key: 'manager_score', width: 80 },
    {
      title: '加权得分',
      key: 'weighted',
      width: 80,
      render: (_: any, r: PerformanceGoalRecord) => <Text strong>{getWeightedScore(r, 'manager').toFixed(1)}</Text>
    }
  ]

  if (loading) return <div style={{ textAlign: 'center', padding: 100 }}><Spin size="large" /></div>

  const isLocked = participant?.is_locked
  const status = participant?.status

  let confirmAction: { type: 'employee' | 'manager' | 'hr'; label: string } | null = null
  if (!isLocked) {
    if (status === 'manager_submitted') {
      confirmAction = { type: 'employee', label: '员工确认结果' }
    } else if (status === 'employee_confirmed') {
      confirmAction = { type: 'manager', label: '主管确认并冻结' }
    } else if (status === 'manager_confirmed') {
      confirmAction = { type: 'hr', label: 'HR确认' }
    }
  }

  return (
    <div className="performance-page" style={{ padding: 24 }}>
      <style>{archiveStyles}</style>
      <Space className="archive-actions">
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(-1)}>返回</Button>
        <Title level={4} style={{ margin: 0 }}>绩效结果查看</Title>
        {isLocked && <Tag icon={<LockOutlined />} color="red">已冻结</Tag>}
        <Button icon={<PrinterOutlined />} onClick={handlePrint}>打印 / 导出 PDF</Button>
        <Button icon={<FileExcelOutlined />} onClick={handleExportExcel}>导出 Excel</Button>
      </Space>

      <Row gutter={24}>
        <Col span={18}>
          <Card title="评分明细" style={{ marginBottom: 16 }}>
            <Table
              dataSource={records}
              columns={columns}
              rowKey="id"
              pagination={false}
              size="small"
              bordered
              summary={() => (
                <Table.Summary fixed>
                  <Table.Summary.Row>
                    <Table.Summary.Cell index={0} colSpan={4}><Text strong>合计</Text></Table.Summary.Cell>
                    <Table.Summary.Cell index={1}><Text strong>{participant?.total_self_score || participant?.self_score}</Text></Table.Summary.Cell>
                    <Table.Summary.Cell index={2}><Text strong>{participant?.total_manager_score || participant?.manager_score}</Text></Table.Summary.Cell>
                    <Table.Summary.Cell index={3}>
                      <Text strong style={{ fontSize: 16 }}>
                        {(participant?.total_manager_score || participant?.manager_score || 0).toFixed(1)}
                      </Text>
                    </Table.Summary.Cell>
                  </Table.Summary.Row>
                </Table.Summary>
              )}
            />
          </Card>

          {records.filter(r => r.section_type === 'bonus_penalty').length > 0 && (
            <Card title="附加考核项" style={{ marginBottom: 16 }}>
              <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>
                附加分仅作为参考或激励依据，不计入总分
              </Text>
              <Table
                dataSource={records.filter(r => r.section_type === 'bonus_penalty')}
                rowKey="id"
                pagination={false}
                size="small"
                bordered
                columns={[
                  { title: '指标名称', dataIndex: 'item_name', key: 'item_name', width: 200 },
                  { title: '权重', dataIndex: 'weight', key: 'weight', width: 80, render: (v: number) => formatWeight(v) },
                  { title: '员工自评', dataIndex: 'self_score', key: 'self_score', width: 100, render: (v: number) => v || '-' },
                  { title: '附加分', dataIndex: 'bonus_score', key: 'bonus_score', width: 100, render: (v: number) => v || '-' },
                  {
                    title: '附件',
                    dataIndex: 'attachments',
                    key: 'attachments',
                    width: 200,
                    render: (val: any) => {
                      const attachments = Array.isArray(val) ? val : []
                      if (attachments.length === 0) return '-'
                      return (
                        <Image.PreviewGroup>
                          <Space wrap size={4}>
                            {attachments.map((url: string, idx: number) => (
                              <Image
                                key={idx}
                                src={url}
                                width={48}
                                height={48}
                                style={{ objectFit: 'cover', borderRadius: 4 }}
                                preview={{ mask: '查看' }}
                              />
                            ))}
                          </Space>
                        </Image.PreviewGroup>
                      )
                    }
                  }
                ]}
              />
            </Card>
          )}

          <Row gutter={16}>
            <Col span={12}>
              <Card title="员工自我评价" size="small">
                <div style={{ marginBottom: 8 }}>
                  <Text strong>做得好的地方：</Text>
                  <div>{participant?.self_evaluation_good || '暂无'}</div>
                </div>
                <div>
                  <Text strong>需要改进的地方：</Text>
                  <div>{participant?.self_evaluation_improvement || '暂无'}</div>
                </div>
              </Card>
            </Col>
            <Col span={12}>
              <Card title="上级总体评价" size="small">
                <div style={{ marginBottom: 8 }}>
                  <Text strong>做得好的地方：</Text>
                  <div>{participant?.manager_evaluation_good || '暂无'}</div>
                </div>
                <div>
                  <Text strong>需要改进的地方：</Text>
                  <div>{participant?.manager_evaluation_improvement || '暂无'}</div>
                </div>
              </Card>
            </Col>
          </Row>
        </Col>

        <Col span={6}>
          <Card title="绩效结果" style={{ marginBottom: 16 }}>
            <Descriptions column={1} size="small">
              <Descriptions.Item label="基础分数">
                <Text strong>{(participant?.manager_score || 0).toFixed(1)}</Text>
              </Descriptions.Item>
              <Descriptions.Item label="附加项加分">
                <Text style={{ color: '#52c41a' }}>+{(participant?.bonus_score || 0).toFixed(1)}</Text>
              </Descriptions.Item>
              <Descriptions.Item label="附加项扣分">
                <Text style={{ color: '#ff4d4f' }}>-{(participant?.penalty_score || 0).toFixed(1)}</Text>
              </Descriptions.Item>
              <Descriptions.Item label="调整后分数">
                <Text strong style={{ fontSize: 18, color: '#1890ff' }}>
                  {(participant?.adjusted_score || participant?.manager_score || 0).toFixed(1)}
                </Text>
              </Descriptions.Item>
              <Descriptions.Item label="绩效等级">
                <Tag color={LEVEL_COLOR[participant?.final_level || ''] || 'default'} style={{ fontSize: 16, padding: '4px 12px' }}>
                  {participant?.final_level || '-'}
                </Tag>
              </Descriptions.Item>
              {participant?.revenue_coefficient && participant.revenue_coefficient !== 1 && (
                <Descriptions.Item label="收支系数">
                  <Text>{participant.revenue_coefficient}</Text>
                </Descriptions.Item>
              )}
            </Descriptions>
            {!isLocked && (status === 'manager_submitted' || status === 'employee_confirmed' || status === 'manager_confirmed') && (
              <Button
                type="dashed"
                icon={<EditOutlined />}
                onClick={handleSetBonusPenalty}
                block
                style={{ marginTop: 12 }}
              >
                设置附加项
              </Button>
            )}
          </Card>

          <Card title="确认进度" size="small" style={{ marginBottom: 16 }}>
            <Timeline
              items={[
                {
                  color: participant?.employee_confirmed_at ? 'green' : 'gray',
                  children: participant?.employee_confirmed_at
                    ? `员工已确认 (${participant.employee_confirmed_at?.substring(0, 10)})`
                    : '待员工确认'
                },
                {
                  color: participant?.manager_confirmed_at ? 'green' : 'gray',
                  children: participant?.manager_confirmed_at
                    ? `主管已确认并冻结 (${participant.manager_confirmed_at?.substring(0, 10)})`
                    : '待主管确认并冻结'
                },
                {
                  color: participant?.hr_confirmed_at ? 'green' : 'gray',
                  children: participant?.hr_confirmed_at
                    ? `人力已确认 (${participant.hr_confirmed_at?.substring(0, 10)})`
                    : '待人力确认'
                },
                {
                  color: isLocked ? 'red' : 'gray',
                  children: isLocked ? '已冻结' : '未冻结',
                  dot: isLocked ? <LockOutlined /> : undefined
                }
              ]}
            />
          </Card>

          {confirmAction && (
            <Button
              type="primary"
              icon={<CheckCircleOutlined />}
              loading={confirming && confirmType === confirmAction.type}
              onClick={() => handleConfirm(confirmAction!.type)}
              block
              size="large"
            >
              {confirmAction.label}
            </Button>
          )}
        </Col>
      </Row>

      <Card className="performance-archive-card" title="个人绩效考核表（归档 / 导出）">
        <ArchivePerformanceSheet activity={activity} participant={participant} records={records} />
      </Card>

      <Modal
        title="设置附加项分数"
        open={bonusPenaltyModalVisible}
        onOk={handleSaveBonusPenalty}
        onCancel={() => setBonusPenaltyModalVisible(false)}
        confirmLoading={savingBonusPenalty}
        okText="保存"
        cancelText="取消"
      >
        <Form form={bonusPenaltyForm} layout="vertical">
          <Form.Item name="bonus_score" label="附加项加分" rules={[{ required: true, message: '请输入加分' }]}>
            <InputNumber min={0} max={20} style={{ width: '100%' }} placeholder="0" />
          </Form.Item>
          <Form.Item name="penalty_score" label="附加项扣分" rules={[{ required: true, message: '请输入扣分' }]}>
            <InputNumber min={0} max={20} style={{ width: '100%' }} placeholder="0" />
          </Form.Item>
          <Text type="secondary" style={{ fontSize: 12 }}>
            调整后分数 = 基础分数 + 加分 - 扣分
          </Text>
        </Form>
      </Modal>
    </div>
  )
}

export default PerformanceResultView
