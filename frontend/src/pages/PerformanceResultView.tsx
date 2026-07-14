import React, { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import {
  Typography, Button, Space, message, Spin, Row, Col, Table, Tag, Descriptions, Timeline, InputNumber, Form, Modal, Image, Card, Divider, Collapse, Empty
} from 'antd'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import AuthorizedImage from '../components/AuthorizedImage'
import { ArrowLeftOutlined, CheckCircleOutlined, LockOutlined, EditOutlined, PrinterOutlined, FileExcelOutlined, FileTextOutlined } from '@ant-design/icons'
import { performanceAPI, PerformanceActivity, PerformanceGoalRecord, PerformanceParticipant, PreviousPerformanceResult } from '../services/api'
import { hasPermission } from '../utils/permission'

const { Title, Text } = Typography

const LEVEL_COLOR: Record<string, string> = {
  S: 'red', A: 'orange', B: 'green', C: 'gold', D: 'volcano'
}

const SECTION_LABEL: Record<string, string> = {
  quantitative: '量化指标',
  key_action: '关键行动',
  bonus_penalty: '附加考核项'
}

const NEW_FLOW_RESULT_PROGRESS = [
  { status: 'hr_confirmation', label: 'HR确认', pending: '待HR确认', current: 'HR确认中', done: 'HR已确认' },
  { status: 'employee_confirmation', label: '员工确认', pending: '待员工确认', current: '员工确认中', done: '员工已确认' },
  { status: 'archived', label: '锁定/归档', pending: '未锁定', current: '已锁定', done: '已锁定/归档' },
]

function getNewFlowResultProgressIndex(status?: string) {
  if (status === 'locked' || status === 'result_confirmed') return NEW_FLOW_RESULT_PROGRESS.length - 1
  const index = NEW_FLOW_RESULT_PROGRESS.findIndex(item => item.status === status)
  return index >= 0 ? index : -1
}

type NewFlowResultProgressPhase = 'done' | 'current' | 'pending'

function getNewFlowResultActivityProgressPhases(status?: string, minimumDoneIndex = -1): NewFlowResultProgressPhase[] {
  const currentIndex = getNewFlowResultProgressIndex(status)
  const phases = NEW_FLOW_RESULT_PROGRESS.map((_, index): NewFlowResultProgressPhase => {
    if (currentIndex > index) return 'done'
    if (currentIndex === index) return 'current'
    return 'pending'
  })
  for (let index = 0; index <= minimumDoneIndex && index < phases.length; index += 1) {
    phases[index] = 'done'
  }
  return phases
}

function isNewFlowResultPublishedParticipantStatus(status?: string) {
  const normalized = String(status || '').trim()
  return normalized === 'employee_confirmed' || normalized === 'result_confirmed' || normalized === 'locked'
}

export function getNewFlowResultProgressPhases(activityStatus?: string, progressStatusOverride?: string): NewFlowResultProgressPhase[] {
  const override = String(progressStatusOverride || '').trim()
  if (!override) {
    return getNewFlowResultActivityProgressPhases(activityStatus)
  }
  if (override === 'manager_confirmed') {
    return ['current', 'pending', 'pending']
  }
  if (override === 'hr_confirmed') {
    return ['done', 'current', 'pending']
  }
  if (override === 'employee_confirmed') {
    return ['done', 'done', 'pending']
  }
  if (isNewFlowResultPublishedParticipantStatus(override)) {
    return getNewFlowResultActivityProgressPhases(activityStatus, 1)
  }
  return NEW_FLOW_RESULT_PROGRESS.map(() => 'pending')
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

function isPlaceholderSignature(value?: string) {
  const normalized = value?.trim().toLowerCase()
  return !normalized
}

function firstRealSignatureName(...names: (string | undefined)[]) {
  return names.find(name => !isPlaceholderSignature(name))?.trim()
}

function formatSignature(name?: string, date?: string) {
  const normalizedName = firstRealSignatureName(name)
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

function isNewPerformanceFlow(activity: PerformanceActivity | null) {
  return activity?.flow_type === 'new'
}

function isPlanGoalRecord(record: PerformanceGoalRecord) {
  return String(record.goal_phase || '').trim() === 'plan'
}

function isReviewGoalRecord(activity: PerformanceActivity | null, record: PerformanceGoalRecord) {
  if (record.section_type === 'bonus_penalty') return false
  if (!isNewPerformanceFlow(activity)) return true
  return !isPlanGoalRecord(record)
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
  const auditSignatureName = firstRealSignatureName(participantExtra?.updated_by, participant?.locked_by, participant?.confirmed_by)
  const employeeResultSignature = formatSignature(
    firstRealSignatureName(participant?.employee_confirmed_by, participant?.employee_name),
    participant?.employee_confirmed_at || participant?.confirmed_at
  )
  const managerResultSignature = formatSignature(
    firstRealSignatureName(participant?.manager_confirmed_by, participant?.manager_name, auditSignatureName),
    participant?.manager_confirmed_at
  )
  const hrResultSignature = formatSignature(
    firstRealSignatureName(participant?.hr_confirmed_by, auditSignatureName),
    participant?.hr_confirmed_at
  )
  const employeeTargetSignature = formatSignature(
    firstRealSignatureName(participantExtra?.employee_target_confirmed_by, participant?.employee_confirmed_by, participant?.employee_name),
    participantExtra?.employee_target_confirmed_at || participant?.employee_confirmed_at || participant?.confirmed_at
  )
  const managerTargetSignature = formatSignature(
    firstRealSignatureName(participantExtra?.manager_target_confirmed_by, participant?.manager_confirmed_by, participant?.manager_name, auditSignatureName),
    participantExtra?.manager_target_confirmed_at || participant?.manager_confirmed_at
  )
  const hrTargetSignature = formatSignature(
    firstRealSignatureName(participantExtra?.hr_target_confirmed_by, participant?.hr_confirmed_by, auditSignatureName),
    participantExtra?.hr_target_confirmed_at || participant?.hr_confirmed_at
  )
  const levelConfirmSignature = formatSignature(
    firstRealSignatureName(participant?.manager_confirmed_by, participant?.manager_name, participant?.hr_confirmed_by, auditSignatureName),
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
    <div className="performance-archive-sheet">
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
            <th>考核上级</th>
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
            <td colSpan={4}>员工确认状态</td>
            <td colSpan={3}>个人价值观等级(季度评)</td>
          </tr>
          <tr className="archive-level-value-row">
            <td colSpan={4} className="archive-level-cell">{participant?.final_level || participant?.suggested_level || '-'}</td>
            <td colSpan={4}>{participant?.employee_confirmed_at ? '员工已确认' : '员工待确认'}</td>
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

const NewFlowPlanArchiveSheet: React.FC<ArchiveSheetProps> = ({ activity, participant, records }) => {
  const mainRecords = records
    .filter(record => isReviewGoalRecord(activity, record))
    .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0) || a.id - b.id)
  const planRecords = records
    .filter(record => record.section_type !== 'bonus_penalty' && isPlanGoalRecord(record))
    .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0) || a.id - b.id)
  const totalWeight = mainRecords.reduce((sum, record) => sum + (record.weight || 0), 0)
  const totalSelfScore = participant?.total_self_score ?? participant?.self_score ?? mainRecords.reduce((sum, record) => sum + getWeightedScore(record, 'self'), 0)
  const totalManagerScore = participant?.total_manager_score ?? participant?.manager_score ?? mainRecords.reduce((sum, record) => sum + getWeightedScore(record, 'manager'), 0)
  const period = formatPeriod(activity?.start_date, activity?.end_date)
  const participantExtra = participant as any
  const employeeSignature = formatSignature(
    firstRealSignatureName(participant?.employee_confirmed_by, participant?.employee_name),
    participant?.employee_confirmed_at || participant?.confirmed_at
  )
  const managerSignature = formatSignature(
    firstRealSignatureName(participant?.manager_confirmed_by, participant?.manager_name, participantExtra?.updated_by),
    participant?.manager_confirmed_at
  )

  const renderReviewRows = () => {
    const rows = mainRecords.length ? mainRecords : Array.from({ length: 5 }, (_, index) => ({
      id: -index - 1,
      item_name: '',
      item_definition: '',
      weight: 0,
      actual_result: '',
      self_score: undefined,
      manager_score: undefined,
    } as unknown as PerformanceGoalRecord))

    return rows.map((record, index) => (
      <tr key={record.id || `review-${index}`} className={record.goal_type === 'okr' || record.goal_type === 'kpi' ? 'old-flow-example-row' : undefined}>
        <td>{index + 1}</td>
        <td colSpan={2}>{record.item_name || '-'}</td>
        <td>{formatWeight(record.weight)}</td>
        <td colSpan={2} className="archive-text-cell">{record.item_definition || record.scoring_rule || '-'}</td>
        <td colSpan={2} className="archive-text-cell">{record.actual_result || '-'}</td>
        <td>{formatScore(record.self_score)}</td>
        <td>{formatScore(record.manager_score)}</td>
        <td />
      </tr>
    ))
  }

  const renderPlanRows = () => {
    const rows = planRecords.length ? planRecords : Array.from({ length: 5 }, (_, index) => ({
      id: -index - 101,
      item_name: '',
      item_definition: '',
      weight: 0,
    } as PerformanceGoalRecord))

    return rows.map((record, index) => (
      <tr key={record.id || `plan-${index}`}>
        <td>{index + 1}</td>
        <td colSpan={2}>{record.item_name || '-'}</td>
        <td>{formatWeight(record.weight)}</td>
        <td colSpan={7} className="archive-text-cell">{record.item_definition || record.target_value || record.scoring_rule || '-'}</td>
      </tr>
    ))
  }

  return (
    <div className="performance-archive-sheet old-flow-archive-sheet">
      <table className="archive-table old-flow-table">
        <colgroup>
          <col style={{ width: '7%' }} />
          <col style={{ width: '8%' }} />
          <col style={{ width: '12%' }} />
          <col style={{ width: '6%' }} />
          <col style={{ width: '12%' }} />
          <col style={{ width: '18%' }} />
          <col style={{ width: '12%' }} />
          <col style={{ width: '18%' }} />
          <col style={{ width: '7%' }} />
          <col style={{ width: '7%' }} />
          <col style={{ width: '8%' }} />
        </colgroup>
        <tbody>
          <tr className="old-flow-title-row">
            <td colSpan={11}>员工绩效考核表</td>
          </tr>
          <tr className="old-flow-info-row">
            <th>部门</th>
            <td colSpan={2}>{participant?.department_name || '-'}</td>
            <th>工号</th>
            <td>{participant?.employee_id || '-'}</td>
            <th>姓名</th>
            <td>{participant?.employee_name || '-'}</td>
            <th>岗位</th>
            <td>{participant?.position || '-'}</td>
            <th>入职日期</th>
            <td>{formatDate(participantExtra?.hire_date || participantExtra?.entry_date)}</td>
          </tr>
          <tr className="old-flow-section-title">
            <td colSpan={11}>上季度指标完成情况</td>
          </tr>
          <tr className="old-flow-note-row">
            <td colSpan={11}>
              说明：十分制，单项得分最高为10分，最低为0分，最终得分按权重汇总；指标数据可根据实际进行删减。
            </td>
          </tr>
          <tr className="old-flow-head-row">
            <th>序号</th>
            <th colSpan={2}>目标/关键职责事项</th>
            <th>权重</th>
            <th colSpan={2}>目标/关键职责事项说明</th>
            <th colSpan={2}>目标/关键职责事项的完成情况</th>
            <th>自评</th>
            <th>上级评分</th>
            <th>备注</th>
          </tr>
          {renderReviewRows()}
          <tr className="old-flow-total-row">
            <td />
            <td colSpan={2}>合计</td>
            <td>{formatWeight(totalWeight)}</td>
            <td colSpan={4}>-</td>
            <td>{formatDecimal(totalSelfScore)}</td>
            <td>{formatDecimal(totalManagerScore)}</td>
            <td />
          </tr>
          <tr className="old-flow-level-row">
            <td colSpan={8}>绩效等级(依据分数自动带出)</td>
            <td>-</td>
            <td className="archive-level-cell">{participant?.final_level || participant?.suggested_level || '-'}</td>
            <td />
          </tr>
          <tr className="old-flow-section-title">
            <td colSpan={11}>下季度目标计划</td>
          </tr>
          <tr className="old-flow-note-row">
            <td colSpan={11}>说明：下季度目标计划按 OKR/KPI 自定义填写，权重总计100%。</td>
          </tr>
          <tr className="old-flow-head-row">
            <th>序号</th>
            <th colSpan={2}>目标/关键职责事项</th>
            <th>权重</th>
            <th colSpan={7}>目标/关键职责事项说明</th>
          </tr>
          {renderPlanRows()}
          <tr className="old-flow-total-row">
            <td />
            <td colSpan={2}>合计</td>
            <td>{formatWeight(planRecords.reduce((sum, record) => sum + (record.weight || 0), 0))}</td>
            <td colSpan={7}>-</td>
          </tr>
          <tr className="old-flow-sign-row">
            <td colSpan={5}>员工签名：{employeeSignature}</td>
            <td colSpan={6}>上级签名：{managerSignature}</td>
          </tr>
          <tr className="old-flow-sign-row">
            <td colSpan={5}>日期：{period}</td>
            <td colSpan={6}>日期：{formatDate(participant?.manager_confirmed_at || participant?.employee_confirmed_at || participant?.confirmed_at)}</td>
          </tr>
        </tbody>
      </table>
    </div>
  )
}

const archiveStyles = `
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
.old-flow-archive-sheet {
  min-width: 1180px;
}
.old-flow-table {
  border: 2px solid #333;
}
.old-flow-title-row td {
  height: 34px;
  font-family: SimHei, "Microsoft YaHei", Arial, sans-serif;
  font-size: 18px;
  font-weight: 700;
}
.old-flow-info-row th,
.old-flow-info-row td {
  height: 32px;
  background: #fff;
  font-weight: 700;
}
.old-flow-section-title td {
  height: 28px;
  background: #e6e6e6;
  font-weight: 700;
}
.old-flow-note-row td {
  height: 34px;
  background: #fff;
  text-align: left !important;
  font-size: 11px;
  font-weight: 700;
}
.old-flow-head-row th {
  height: 32px;
  background: #f4f4f4;
}
.old-flow-table tbody tr:not(.old-flow-title-row):not(.old-flow-info-row):not(.old-flow-section-title):not(.old-flow-note-row):not(.old-flow-head-row):not(.old-flow-total-row):not(.old-flow-level-row):not(.old-flow-sign-row) td {
  height: 38px;
}
.old-flow-example-row td {
  background: #f2dcdb;
}
.old-flow-total-row td,
.old-flow-level-row td {
  height: 26px;
  background: #dce6f1;
  font-weight: 700;
}
.old-flow-sign-row td {
  height: 34px;
  background: #fff;
  text-align: left !important;
  font-weight: 700;
}
@media print {
  @page {
    size: A4 landscape;
    margin: 6mm;
  }
  html,
  body {
    background: #fff !important;
  }
  body * {
    visibility: hidden !important;
  }
  #performance-print-root,
  #performance-print-root * {
    visibility: visible !important;
  }
  #performance-print-root {
    display: block !important;
    position: absolute;
    left: 0;
    top: 0;
    width: 100%;
    background: #fff;
  }
  #performance-print-root .performance-archive-sheet {
    width: 100%;
    min-width: 0;
    overflow: visible;
    font-size: 9px;
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

const EXCEL_NEW_FLOW_COLUMN_WIDTHS = [86, 172, 351, 78, 112, 112, 112, 273, 125, 70, 69]
const EXCEL_OLD_FLOW_COLUMN_WIDTHS = [83, 94, 142, 71, 142, 212, 142, 212, 83, 83, 94]

const EXCEL_BASE_CELL_STYLE = [
  'border: 0.5pt solid #000',
  'padding: 3px 4px',
  'text-align: center',
  'vertical-align: middle',
  'white-space: normal',
  'word-break: break-word',
  'font-family: SimSun, "Microsoft YaHei", Arial, sans-serif',
  'font-size: 9pt',
  'line-height: 1.2',
  'mso-number-format:"\\@"'
].join('; ')

const appendInlineStyle = (element: HTMLElement, style: string) => {
  const current = element.getAttribute('style')
  element.setAttribute('style', current ? `${current}; ${style}` : style)
}

const waitForBrowserPaint = async () => {
  await new Promise<void>(resolve => {
    if (typeof window.requestAnimationFrame === 'function') {
      window.requestAnimationFrame(() => resolve())
      return
    }
    window.setTimeout(resolve, 0)
  })
}

const hasAnyClass = (element: Element, classNames: string[]) => (
  classNames.some(className => element.classList.contains(className))
)

const getExcelRowHeight = (row: HTMLTableRowElement) => {
  if (hasAnyClass(row, ['archive-main-head'])) return 58
  if (hasAnyClass(row, ['archive-data-row', 'archive-sign-row'])) return 44
  if (hasAnyClass(row, ['archive-level-label-row', 'archive-level-value-row'])) return 40
  if (hasAnyClass(row, ['old-flow-title-row', 'old-flow-note-row', 'old-flow-sign-row'])) return 34
  if (hasAnyClass(row, ['archive-info-head', 'archive-info-value', 'old-flow-info-row', 'old-flow-head-row'])) return 32
  if (hasAnyClass(row, ['archive-total-row', 'archive-review-head', 'archive-review-row'])) return 30
  if (hasAnyClass(row, ['archive-top-row', 'archive-section-row', 'archive-main-subhead', 'old-flow-section-title'])) return 28
  if (hasAnyClass(row, ['old-flow-total-row', 'old-flow-level-row'])) return 26
  return 38
}

const getExcelHeadNoteLines = (text: string) => {
  const normalizedText = text.trim()
  if (normalizedText.includes('单项不低于10%')) {
    return ['（5%的倍数且', '单项不低于10%）']
  }
  if (normalizedText.includes('定量分段设置')) {
    return ['（定量分段设置，上限120分；定性按达成度/质量分级，上限100分）']
  }
  return [normalizedText]
}

const setSameCellBreak = (breakElement: HTMLBRElement) => {
  appendInlineStyle(breakElement, 'mso-data-placement: same-cell')
}

const normalizeExcelHeadCells = (root: HTMLElement) => {
  root.querySelectorAll('.archive-main-head th').forEach(cell => {
    const headCell = cell as HTMLTableCellElement
    const note = headCell.querySelector('.archive-head-note') as HTMLElement | null
    if (!note) return

    const titleText = Array.from(headCell.childNodes)
      .filter(node => node !== note && node.nodeName.toLowerCase() !== 'br')
      .map(node => node.textContent?.trim() || '')
      .filter(Boolean)
      .join('')
    const noteLines = getExcelHeadNoteLines(note.textContent || '')
    const titleSpan = document.createElement('span')
    titleSpan.className = 'excel-head-title'
    titleSpan.textContent = titleText

    const noteSpan = document.createElement('span')
    noteSpan.className = 'archive-head-note excel-head-note'
    noteLines.forEach((line, index) => {
      if (index > 0) {
        const breakElement = document.createElement('br')
        setSameCellBreak(breakElement)
        noteSpan.appendChild(breakElement)
      }
      noteSpan.appendChild(document.createTextNode(line))
    })

    headCell.textContent = ''
    headCell.appendChild(titleSpan)
    const breakElement = document.createElement('br')
    setSameCellBreak(breakElement)
    headCell.appendChild(breakElement)
    headCell.appendChild(noteSpan)
    appendInlineStyle(headCell, [
      'text-align: center',
      'vertical-align: middle',
      'line-height: 1.2',
      'white-space: normal'
    ].join('; '))
  })
}

const getExcelCellStyle = (cell: HTMLTableCellElement, row: HTMLTableRowElement) => {
  const styles: string[] = [EXCEL_BASE_CELL_STYLE]

  if (cell.tagName.toLowerCase() === 'th') {
    styles.push('font-weight: 700')
  }

  if (hasAnyClass(row, [
    'archive-info-head',
    'archive-info-value',
    'archive-main-head',
    'archive-main-subhead',
    'archive-total-row',
    'archive-review-head',
    'archive-level-label-row',
    'archive-level-value-row'
  ])) {
    styles.push('background: #fff2cc')
  }

  if (hasAnyClass(row, ['archive-section-row'])) {
    styles.push('background: #ffc000', 'font-weight: 700')
  }

  if (hasAnyClass(row, ['archive-main-head'])) {
    styles.push('line-height: 1.25')
  }

  if (hasAnyClass(row, ['archive-top-row'])) {
    styles.push('border-top: 2pt solid #2b64ff')
  }

  if (hasAnyClass(row, ['old-flow-section-title'])) {
    styles.push('background: #e6e6e6', 'font-weight: 700')
  }

  if (hasAnyClass(row, ['old-flow-head-row'])) {
    styles.push('background: #f4f4f4', 'font-weight: 700')
  }

  if (hasAnyClass(row, ['old-flow-total-row', 'old-flow-level-row'])) {
    styles.push('background: #dce6f1', 'font-weight: 700')
  }

  if (hasAnyClass(row, ['old-flow-example-row'])) {
    styles.push('background: #f2dcdb')
  }

  if (hasAnyClass(row, ['archive-sign-final-row'])) {
    styles.push('color: #f00', 'text-align: center', 'font-weight: 700')
  }

  if (hasAnyClass(row, ['archive-total-row', 'archive-level-label-row', 'archive-level-value-row', 'archive-sign-row', 'old-flow-info-row', 'old-flow-sign-row'])) {
    styles.push('font-weight: 700')
  }

  if (hasAnyClass(cell, ['archive-text-cell', 'archive-evaluation-cell'])) {
    styles.push('text-align: left', 'vertical-align: top')
  }

  if (hasAnyClass(cell, ['archive-logo-cell'])) {
    styles.push('text-align: left', 'font-weight: 700', 'font-family: "Microsoft YaHei", SimHei, Arial, sans-serif', 'white-space: nowrap')
  }

  if (hasAnyClass(cell, ['archive-title-cell'])) {
    styles.push('font-family: SimHei, "Microsoft YaHei", Arial, sans-serif', 'font-size: 12pt', 'font-weight: 700')
  }

  if (hasAnyClass(cell, ['archive-category-cell', 'archive-evaluation-title'])) {
    styles.push('font-weight: 700')
  }

  if (hasAnyClass(cell, ['archive-evaluation-cell', 'archive-level-cell'])) {
    styles.push('color: #d00', 'font-weight: 700')
  }

  if (hasAnyClass(cell, ['archive-sign-cell', 'archive-sign-confirm'])) {
    styles.push('text-align: left', 'padding-left: 4px', 'font-weight: 700')
  }

  return styles.join('; ')
}

const prepareArchiveSheetForExcel = (sheet: HTMLElement) => {
  const clone = sheet.cloneNode(true) as HTMLElement
  const table = clone.querySelector('table') as HTMLTableElement | null
  if (!table) return clone

  const isOldFlow = table.classList.contains('old-flow-table')
  const columnWidths = isOldFlow ? EXCEL_OLD_FLOW_COLUMN_WIDTHS : EXCEL_NEW_FLOW_COLUMN_WIDTHS

  clone.removeAttribute('id')
  appendInlineStyle(clone, [
    'background: #fff',
    'color: #000',
    'font-family: SimSun, "Microsoft YaHei", Arial, sans-serif',
    'font-size: 9pt',
    `width: ${isOldFlow ? 1180 : 1560}px`
  ].join('; '))

  table.setAttribute('border', '1')
  table.setAttribute('cellspacing', '0')
  table.setAttribute('cellpadding', '0')
  appendInlineStyle(table, [
    'border-collapse: collapse',
    'table-layout: fixed',
    'border: 1.5pt solid #000',
    `width: ${isOldFlow ? 1180 : 1560}px`,
    'mso-table-lspace: 0pt',
    'mso-table-rspace: 0pt'
  ].join('; '))

  table.querySelectorAll('col').forEach((col, index) => {
    const width = columnWidths[index]
    if (!width) return
    col.setAttribute('width', String(width))
    appendInlineStyle(col as HTMLElement, `width: ${width}px; mso-width-source: userset`)
  })

  table.querySelectorAll('tr').forEach(row => {
    const tableRow = row as HTMLTableRowElement
    const height = getExcelRowHeight(tableRow)
    tableRow.setAttribute('height', String(height))
    appendInlineStyle(tableRow, `height: ${height}px`)
    Array.from(tableRow.cells).forEach(cell => {
      appendInlineStyle(cell, getExcelCellStyle(cell, tableRow))
    })
  })

  normalizeExcelHeadCells(clone)

  clone.querySelectorAll('.archive-logo-mark').forEach(mark => {
    mark.textContent = '○'
    appendInlineStyle(mark as HTMLElement, 'color: #2b64ff; font-size: 16pt; font-weight: 700; vertical-align: middle')
  })
  clone.querySelectorAll('.archive-logo-main').forEach(text => {
    appendInlineStyle(text as HTMLElement, 'font-size: 16pt; font-style: italic; font-weight: 700; vertical-align: middle')
  })
  clone.querySelectorAll('.archive-logo-sub').forEach(text => {
    appendInlineStyle(text as HTMLElement, 'font-size: 10pt; font-weight: 700; vertical-align: middle')
  })
  clone.querySelectorAll('.excel-head-title').forEach(title => {
    appendInlineStyle(title as HTMLElement, [
      'font-size: 9pt',
      'font-weight: 700',
      'line-height: 1.25',
      'white-space: nowrap'
    ].join('; '))
  })
  clone.querySelectorAll('.archive-head-note').forEach(note => {
    const headNote = note as HTMLElement
    appendInlineStyle(headNote, [
      'display: inline',
      'font-size: 7pt',
      'font-weight: 400',
      'line-height: 1.15',
      'white-space: nowrap'
    ].join('; '))
  })
  clone.querySelectorAll('br').forEach(breakElement => setSameCellBreak(breakElement))
  clone.querySelectorAll('.archive-category-note').forEach(note => {
    appendInlineStyle(note as HTMLElement, 'font-size: 9pt; font-weight: 400; line-height: 1.25')
  })

  return clone
}

const buildStyledExcelHtml = (sheet: HTMLElement) => {
  const excelSheet = prepareArchiveSheetForExcel(sheet)

  return `<!doctype html>
<html xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:x="urn:schemas-microsoft-com:office:excel" xmlns="http://www.w3.org/TR/REC-html40">
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
<!--[if gte mso 9]><xml>
<x:ExcelWorkbook>
<x:ExcelWorksheets>
<x:ExcelWorksheet>
<x:Name>个人绩效考核表</x:Name>
<x:WorksheetOptions>
<x:Print>
<x:ValidPrinterInfo/>
<x:PaperSizeIndex>9</x:PaperSizeIndex>
<x:HorizontalResolution>600</x:HorizontalResolution>
<x:VerticalResolution>600</x:VerticalResolution>
</x:Print>
<x:PageSetup>
<x:Layout x:Orientation="Landscape"/>
<x:PageMargins x:Bottom="0.25" x:Left="0.2" x:Right="0.2" x:Top="0.25"/>
</x:PageSetup>
</x:WorksheetOptions>
</x:ExcelWorksheet>
</x:ExcelWorksheets>
</x:ExcelWorkbook>
</xml><![endif]-->
<style>
${archiveStyles}
@page {
  mso-page-orientation: landscape;
  margin: 0.25in 0.2in 0.25in 0.2in;
}
</style>
</head>
<body>${excelSheet.outerHTML}</body>
</html>`
}

const removePrintRoot = () => {
  document.getElementById('performance-print-root')?.remove()
}

const createPrintRoot = (sheet: HTMLElement) => {
  removePrintRoot()
  const printRoot = document.createElement('div')
  printRoot.id = 'performance-print-root'
  printRoot.setAttribute('aria-hidden', 'true')
  appendInlineStyle(printRoot, 'display: none')

  const sheetClone = sheet.cloneNode(true) as HTMLElement
  sheetClone.removeAttribute('id')
  printRoot.appendChild(sheetClone)
  document.body.appendChild(printRoot)

  return printRoot
}

const PerformanceResultView: React.FC = () => {
  const { activityId, participantId } = useParams<{ activityId: string; participantId: string }>()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(true)
  const [records, setRecords] = useState<PerformanceGoalRecord[]>([])
  const [participant, setParticipant] = useState<PerformanceParticipant | null>(null)
  const [activity, setActivity] = useState<PerformanceActivity | null>(null)
  const [progressStatusOverride, setProgressStatusOverride] = useState<string | undefined>(undefined)
  const [previousResult, setPreviousResult] = useState<PreviousPerformanceResult | null>(null)
  const [previousResultModalVisible, setPreviousResultModalVisible] = useState(false)
  const [resultHidden, setResultHidden] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [confirmType, setConfirmType] = useState<'employee' | 'manager' | 'hr' | null>(null)
  const [publishingResult, setPublishingResult] = useState(false)
  const [triggeringInterview, setTriggeringInterview] = useState(false)
  const [bonusPenaltyModalVisible, setBonusPenaltyModalVisible] = useState(false)
  const [bonusPenaltyForm] = Form.useForm()
  const [savingBonusPenalty, setSavingBonusPenalty] = useState(false)
  const [archiveActiveKeys, setArchiveActiveKeys] = useState<string[]>([])

  const loadData = useCallback(async () => {
    if (!participantId) return
    setLoading(true)
    setResultHidden(false)
    setProgressStatusOverride(undefined)
    setPreviousResult(null)
    setPreviousResultModalVisible(false)
    try {
      const [recordsRes, participantRes] = await Promise.all([
        performanceAPI.getGoalRecords(Number(participantId)),
        performanceAPI.getParticipant(Number(participantId))
      ])
      setRecords(recordsRes.data?.items || [])
      setParticipant(participantRes.data?.participant || participantRes.data)
      setActivity(participantRes.data?.activity || null)
      setProgressStatusOverride(participantRes.data?.progress_status_override || undefined)
      try {
        const previousRes = await performanceAPI.getPreviousParticipantResult(Number(participantId))
        setPreviousResult(previousRes.data || null)
      } catch {
        setPreviousResult(null)
      }
    } catch (err: any) {
      if (err?.response?.data?.data?.result_hidden) {
        setResultHidden(true)
      } else {
        message.error(err?.response?.data?.message || '加载数据失败')
      }
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
      const isRecheck = participant?.status === 'manager_recheck'
      Modal.confirm({
        title: isRecheck ? '确认已查看员工自评修改' : '主管确认并冻结绩效结果',
        content: isRecheck
          ? '确认后该员工绩效结果将重新冻结，并允许HR继续确认。'
          : '确认后该员工绩效结果将立即冻结，评分、等级和附加项将无法再修改。',
        okText: isRecheck ? '确认查看' : '确认并冻结',
        cancelText: '取消',
        onOk: () => doConfirm(type)
      })
      return
    }

    await doConfirm(type)
  }

  const handleOpenResultPublish = async () => {
    if (!activity?.id) return
    setPublishingResult(true)
    try {
      await performanceAPI.openResultPublish(activity.id)
      message.success('结果公布已开启')
      loadData()
    } catch (err: any) {
      message.error(err?.response?.data?.message || '开启结果公布失败')
    } finally {
      setPublishingResult(false)
    }
  }

  const handleTriggerPerformanceInterview = async () => {
    if (!participantId || !participant) return
    const finalLevel = String(participant.final_level || participant.suggested_level || '').trim()
    const interviewType = finalLevel === 'C' || finalLevel === 'D' ? 'required' : 'optional'
    setTriggeringInterview(true)
    try {
      await performanceAPI.triggerPerformanceInterview(Number(participantId), interviewType)
      message.success('绩效面谈通知已发送')
    } catch (err: any) {
      message.error(err?.response?.data?.message || '发起绩效面谈失败')
    } finally {
      setTriggeringInterview(false)
    }
  }

  const waitForArchiveSheet = async () => {
    let sheet = document.getElementById('performance-archive-sheet')
    if (sheet) {
      return sheet
    }

    setArchiveActiveKeys(prev => prev.includes('archive') ? prev : [...prev, 'archive'])
    await waitForBrowserPaint()

    sheet = document.getElementById('performance-archive-sheet')
    if (sheet) {
      return sheet
    }

    await new Promise<void>(resolve => window.setTimeout(resolve, 0))
    return document.getElementById('performance-archive-sheet')
  }

  const handlePrint = async () => {
    const sheet = await waitForArchiveSheet()
    if (!sheet) {
      message.error('未找到可导出的绩效考核表')
      return
    }
    createPrintRoot(sheet)
    await waitForBrowserPaint()
    window.addEventListener('afterprint', removePrintRoot, { once: true })
    window.print()
  }

  const handleExportExcel = async () => {
    const sheet = await waitForArchiveSheet()
    if (!sheet) {
      message.error('未找到可导出的绩效考核表')
      return
    }

    const html = buildStyledExcelHtml(sheet)
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
        <StatusTag color={val === 'quantitative' ? 'blue' : val === 'bonus_penalty' ? 'gold' : 'green'}>
          {SECTION_LABEL[val] || val}
        </StatusTag>
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

  if (loading) {
    return (
      <>
        <Form form={bonusPenaltyForm} component={false} />
        <div style={{ textAlign: 'center', padding: 100 }}><Spin size="large" /></div>
      </>
    )
  }

  if (resultHidden) {
    return (
      <PageContainer data-testid="performance-result-hidden-page" title="绩效结果" style={{ padding: '24px 32px' }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(-1)} style={{ marginBottom: 24 }}>返回</Button>
        <PageCard title="绩效结果">
          <Empty description="绩效结果暂未公布" />
        </PageCard>
      </PageContainer>
    )
  }

  const isLocked = participant?.is_locked
  const status = participant?.status
  const isNewFlow = isNewPerformanceFlow(activity)
  const displayRecords = isNewFlow
    ? records.filter(record => isReviewGoalRecord(activity, record))
    : records
  const bonusRecords = records.filter(r => r.section_type === 'bonus_penalty')
  const previousActivity = previousResult?.activity || null
  const previousParticipant = previousResult?.participant || null
  const previousRecords: PerformanceGoalRecord[] = Array.isArray(previousResult?.goal_records) ? previousResult.goal_records : []
  const previousDisplayRecords = previousActivity
    ? previousRecords.filter(record => isReviewGoalRecord(previousActivity, record))
    : previousRecords
  const previousBonusRecords = previousRecords.filter(record => record.section_type === 'bonus_penalty')
  const previousVersions = Array.isArray(previousResult?.versions) ? previousResult.versions : []
  const formatOptionalDecimal = (value?: number | null) =>
    value === undefined || value === null ? '-' : formatDecimal(value)
  const formatMetaValue = (value: unknown) => {
    if (value === undefined || value === null || value === '') return '-'
    if (typeof value === 'number') return formatDecimal(value)
    return String(value)
  }
  const formatVersionChange = (meta?: Record<string, unknown>) => {
    if (!meta) return '-'
    const oldScore = formatMetaValue(meta.old_adjusted_score ?? meta.old_total_manager_score ?? meta.old_manager_score)
    const newScore = formatMetaValue(meta.new_department_score)
    const oldLevel = formatMetaValue(meta.old_final_level)
    const newLevel = formatMetaValue(meta.new_final_level)
    const reason = formatMetaValue(meta.reason)
    const parts: string[] = []
    if (oldScore !== '-' || newScore !== '-') parts.push(`原分数 ${oldScore}，调整为 ${newScore}`)
    if (oldLevel !== '-' || newLevel !== '-') parts.push(`原等级 ${oldLevel}，调整为 ${newLevel}`)
    if (reason !== '-') parts.push(`原因：${reason}`)
    return parts.join('；') || '-'
  }
  const formatVersionReason = (record: {
    adjust_reason?: string
    confirm_comment?: string
    manager_comment?: string
    operation_meta?: Record<string, unknown>
  }) => {
    const change = formatVersionChange(record.operation_meta)
    if (change !== '-') return change
    return record.adjust_reason || record.confirm_comment || record.manager_comment || '-'
  }
  const previousPreviewColumns = [
    { title: '事项', dataIndex: 'item_name', key: 'item_name' },
    { title: '权重', dataIndex: 'weight', key: 'weight', width: 72, render: (v: number) => formatWeight(v) },
    { title: '完成情况', dataIndex: 'actual_result', key: 'actual_result', render: (v: string) => v || '-' },
    { title: '上级评分', dataIndex: 'manager_score', key: 'manager_score', width: 88, render: (v: number) => formatOptionalDecimal(v) },
  ]
  const previousDetailColumns = [
    {
      title: '类别',
      dataIndex: 'section_type',
      key: 'section_type',
      width: 100,
      render: (val: string) => (
        <StatusTag color={val === 'quantitative' ? 'blue' : val === 'bonus_penalty' ? 'gold' : 'green'}>
          {SECTION_LABEL[val] || val}
        </StatusTag>
      )
    },
    { title: '事项', dataIndex: 'item_name', key: 'item_name', width: 180, render: (v: string) => v || '-' },
    {
      title: '说明',
      dataIndex: 'item_definition',
      key: 'item_definition',
      width: 240,
      render: (v: string, record: PerformanceGoalRecord) => v || record.scoring_rule || '-'
    },
    { title: '权重', dataIndex: 'weight', key: 'weight', width: 80, render: (v: number) => formatWeight(v) },
    {
      title: '目标/标准',
      key: 'target',
      width: 180,
      render: (_: unknown, record: PerformanceGoalRecord) => record.target_value || record.scoring_rule || '-'
    },
    { title: '完成情况', dataIndex: 'actual_result', key: 'actual_result', width: 180, render: (v: string) => v || '-' },
    { title: '自评', dataIndex: 'self_score', key: 'self_score', width: 80, render: (v: number) => formatOptionalDecimal(v) },
    { title: '上级评分', dataIndex: 'manager_score', key: 'manager_score', width: 90, render: (v: number) => formatOptionalDecimal(v) },
    {
      title: '加权得分',
      key: 'weighted',
      width: 90,
      render: (_: unknown, record: PerformanceGoalRecord) => <Text strong>{getWeightedScore(record, 'manager').toFixed(1)}</Text>
    }
  ]
  const previousBonusColumns = [
    { title: '事项', dataIndex: 'item_name', key: 'item_name', width: 180, render: (v: string) => v || '-' },
    { title: '权重', dataIndex: 'weight', key: 'weight', width: 80, render: (v: number) => formatWeight(v) },
    { title: '完成情况', dataIndex: 'actual_result', key: 'actual_result', render: (v: string) => v || '-' },
    { title: '自评', dataIndex: 'self_score', key: 'self_score', width: 80, render: (v: number) => formatOptionalDecimal(v) },
    { title: '附加分', dataIndex: 'bonus_score', key: 'bonus_score', width: 90, render: (v: number) => formatOptionalDecimal(v) },
  ]
  const previousVersionColumns = [
    { title: '类型', dataIndex: 'review_type', key: 'review_type', width: 130, render: (v: string) => v || '-' },
    { title: '操作人', dataIndex: 'created_by', key: 'created_by', width: 120, render: (v: string) => v || '-' },
    { title: '最终等级', dataIndex: 'final_level', key: 'final_level', width: 100, render: (v: string) => v || '-' },
    { title: '上级评分', dataIndex: 'manager_score', key: 'manager_score', width: 100, render: (v: number) => formatOptionalDecimal(v) },
    {
      title: '调整留痕',
      key: 'change',
      render: (_: unknown, record: { adjust_reason?: string; confirm_comment?: string; manager_comment?: string; operation_meta?: Record<string, unknown> }) =>
        formatVersionReason(record)
    },
    { title: '时间', dataIndex: 'created_at', key: 'created_at', width: 120, render: (v: string) => formatDate(v) },
  ]

  let confirmAction: { type: 'employee' | 'manager' | 'hr'; label: string } | null = null
  if (isNewFlow) {
    if (activity?.status === 'hr_confirmation' && status === 'manager_confirmed' && hasPermission('performance:hr_confirm:submit')) {
      confirmAction = { type: 'hr', label: 'HR确认' }
    } else if (activity?.status === 'employee_confirmation' && status === 'hr_confirmed' && !isLocked) {
      confirmAction = { type: 'employee', label: '员工确认结果' }
    }
  } else {
    if (status === 'manager_submitted' && !isLocked) {
      confirmAction = { type: 'employee', label: '员工确认结果' }
    } else if (status === 'employee_confirmed' && !isLocked) {
      confirmAction = { type: 'manager', label: '主管确认并冻结' }
    } else if (status === 'manager_recheck' && ['manager_confirmation', 'hr_confirmation'].includes(activity?.status || '') && !isLocked) {
      confirmAction = { type: 'manager', label: '确认已查看' }
    } else if (activity?.status === 'hr_confirmation' && status === 'manager_confirmed') {
      confirmAction = { type: 'hr', label: 'HR确认' }
    }
  }
  const newFlowProgressPhases = getNewFlowResultProgressPhases(activity?.status, progressStatusOverride)
  const progressOverrideBlocksResultActions = Boolean(progressStatusOverride) && !isNewFlowResultPublishedParticipantStatus(progressStatusOverride)
  const canOpenResultPublish = isNewFlow &&
    activity?.status === 'hr_review' &&
    (!progressStatusOverride || progressStatusOverride === 'hr_confirmed') &&
    (hasPermission('performance:result_publish:manage') || hasPermission('performance:activity:manage'))
  const canTriggerPerformanceInterview = isNewFlow &&
    activity?.status === 'interview' &&
    !progressOverrideBlocksResultActions &&
    (hasPermission('performance:activity:manage') ||
      hasPermission('performance:department_eval:submit') ||
      hasPermission('performance:level_adjust:manage'))
  const resultProgressItems = isNewFlow
    ? NEW_FLOW_RESULT_PROGRESS.map((item, index) => ({
      color: newFlowProgressPhases[index] === 'done' ? 'green' : newFlowProgressPhases[index] === 'current' ? 'blue' : 'gray',
      children: newFlowProgressPhases[index] === 'done'
        ? item.done
        : newFlowProgressPhases[index] === 'current' ? item.current : item.pending,
    }))
    : [
      {
        color: participant?.employee_confirmed_at ? 'green' : 'gray',
        children: participant?.employee_confirmed_at
          ? `员工已确认 (${participant.employee_confirmed_at?.substring(0, 10)})`
          : '待员工确认'
      },
      {
        color: participant?.status === 'manager_recheck' ? 'orange' : participant?.manager_confirmed_at ? 'green' : 'gray',
        children: participant?.status === 'manager_recheck'
          ? '员工已修改自评，待主管复核'
          : participant?.manager_confirmed_at
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
    ]

  return (
    <PageContainer data-testid="performance-result-page" title="绩效结果" style={{ padding: '24px 32px' }}>
      <style>{archiveStyles}</style>
      <div style={{ marginBottom: 24 }}>
        <Space size={16} align="center">
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(-1)}>返回</Button>
          <Title level={4} style={{ margin: 0 }}>绩效结果查看</Title>
          {isLocked && <StatusTag icon={<LockOutlined />} color="red">已冻结</StatusTag>}
        </Space>
        <Space style={{ marginLeft: 'auto' }}>
          <Button icon={<PrinterOutlined />} onClick={handlePrint}>打印 / 导出 PDF</Button>
          <Button icon={<FileExcelOutlined />} onClick={handleExportExcel}>导出 Excel</Button>
        </Space>
      </div>

      <Row gutter={[32, 24]}>
        <Col xs={24} lg={16}>
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            <PageCard title="评分明细">
              <Table
                dataSource={displayRecords}
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
            </PageCard>

            {bonusRecords.length > 0 && (
              <PageCard title="附加考核项">
                <Text type="secondary" style={{ display: 'block', marginBottom: 8 }}>
                  附加分仅作为参考或激励依据，不计入总分
                </Text>
                <Table
                  dataSource={bonusRecords}
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
                                <AuthorizedImage
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
              </PageCard>
            )}

            <Row gutter={20}>
              <Col span={12}>
                <PageCard title="员工自我评价">
                  <div style={{ marginBottom: 12 }}>
                    <Text type="secondary" style={{ fontSize: 13 }}>做得好的地方：</Text>
                    <div style={{ marginTop: 4 }}>{participant?.self_evaluation_good || '暂无'}</div>
                  </div>
                  <div>
                    <Text type="secondary" style={{ fontSize: 13 }}>需要改进的地方：</Text>
                    <div style={{ marginTop: 4 }}>{participant?.self_evaluation_improvement || '暂无'}</div>
                  </div>
                </PageCard>
              </Col>
              <Col span={12}>
                <PageCard title="上级总体评价">
                  <div style={{ marginBottom: 12 }}>
                    <Text type="secondary" style={{ fontSize: 13 }}>做得好的地方：</Text>
                    <div style={{ marginTop: 4 }}>{participant?.manager_evaluation_good || '暂无'}</div>
                  </div>
                  <div>
                    <Text type="secondary" style={{ fontSize: 13 }}>需要改进的地方：</Text>
                    <div style={{ marginTop: 4 }}>{participant?.manager_evaluation_improvement || '暂无'}</div>
                  </div>
                </PageCard>
              </Col>
            </Row>

            {isNewFlow && previousResult && (
              <PageCard title="上一周期结果">
                {previousParticipant ? (
                  <Space direction="vertical" size={12} style={{ width: '100%' }}>
                    <Descriptions column={{ xs: 1, sm: 3 }} size="small">
                      <Descriptions.Item label="活动">{previousActivity?.name || '-'}</Descriptions.Item>
                      <Descriptions.Item label="最终等级">
                        <StatusTag color={LEVEL_COLOR[previousParticipant.final_level || ''] || 'default'}>
                          {previousParticipant.final_level || '-'}
                        </StatusTag>
                      </Descriptions.Item>
                      <Descriptions.Item label="最终分">
                        <Text strong>{formatDecimal(previousParticipant.adjusted_score || previousParticipant.total_manager_score || previousParticipant.manager_score)}</Text>
                      </Descriptions.Item>
                    </Descriptions>
                    <Table
                      dataSource={previousDisplayRecords.slice(0, 5)}
                      columns={previousPreviewColumns}
                      rowKey="id"
                      pagination={false}
                      size="small"
                    />
                    <Button
                      type="link"
                      style={{ paddingInline: 0 }}
                      onClick={() => setPreviousResultModalVisible(true)}
                    >
                      查看完整上一周期结果
                    </Button>
                  </Space>
                ) : (
                  <Empty description={previousActivity ? '上一周期没有该员工绩效结果' : '暂无上一周期结果'} />
                )}
              </PageCard>
            )}
          </Space>
        </Col>

        <Col xs={24} lg={8}>
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            <PageCard title="绩效结果">
              <Descriptions column={1} size="small">
                <Descriptions.Item label="基础分数">
                  <Text strong>{(participant?.manager_score || 0).toFixed(1)}</Text>
                </Descriptions.Item>
                <Descriptions.Item label="附加项加分">
                  <Text style={{ color: 'var(--color-success)' }}>+{(participant?.bonus_score || 0).toFixed(1)}</Text>
                </Descriptions.Item>
                <Descriptions.Item label="附加项扣分">
                  <Text style={{ color: 'var(--color-error)' }}>-{(participant?.penalty_score || 0).toFixed(1)}</Text>
                </Descriptions.Item>
                <Descriptions.Item label="调整后分数">
                  <Text strong style={{ fontSize: 18, color: 'var(--color-info)' }}>
                    {(participant?.adjusted_score || participant?.manager_score || 0).toFixed(1)}
                  </Text>
                </Descriptions.Item>
                <Descriptions.Item label="绩效等级">
                  <StatusTag color={LEVEL_COLOR[participant?.final_level || ''] || 'default'} style={{ fontSize: 16, padding: '4px 12px' }}>
                    {participant?.final_level || '-'}
                  </StatusTag>
                </Descriptions.Item>
                {participant?.revenue_coefficient && participant.revenue_coefficient !== 1 && (
                  <Descriptions.Item label="收支系数">
                    <Text>{participant.revenue_coefficient}</Text>
                  </Descriptions.Item>
                )}
              </Descriptions>
              {!isLocked && (status === 'manager_submitted' || status === 'employee_confirmed') && activity?.enable_bonus_score && (
                <Button
                  data-testid="performance-result-set-bonus-penalty"
                  type="dashed"
                  icon={<EditOutlined />}
                  onClick={handleSetBonusPenalty}
                  block
                  style={{ marginTop: 12 }}
                >
                  设置附加项
                </Button>
              )}
            </PageCard>

            {!isNewFlow && (
              <PageCard title="确认进度">
                <Timeline items={resultProgressItems} />
              </PageCard>
            )}

            {canOpenResultPublish && (
              <Button
                data-testid="performance-result-open-result-publish"
                type="primary"
                icon={<CheckCircleOutlined />}
                loading={publishingResult}
                onClick={handleOpenResultPublish}
                block
                size="large"
              >
                HR审核通过，开启结果公布
              </Button>
            )}

            {canTriggerPerformanceInterview && (
              <Button
                data-testid="performance-result-trigger-interview"
                type="primary"
                icon={<FileTextOutlined />}
                loading={triggeringInterview}
                onClick={handleTriggerPerformanceInterview}
                block
                size="large"
              >
                发起绩效面谈
              </Button>
            )}

            {confirmAction && (
              <Button
                data-testid={`performance-result-confirm-${confirmAction.type}`}
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
          </Space>
        </Col>
      </Row>

      <Divider style={{ margin: '24px 0 16px' }} />

      <Collapse
        activeKey={archiveActiveKeys}
        onChange={(keys) => setArchiveActiveKeys(Array.isArray(keys) ? keys.map(String) : (keys ? [String(keys)] : []))}
        expandIconPosition="start"
        style={{ marginBottom: 24 }}
        items={[
          {
            key: 'archive',
            label: (
              <Space>
                <FileTextOutlined />
                <Text strong>个人绩效考核表（归档 / 导出）</Text>
              </Space>
            ),
            children: (
              <div id="performance-archive-sheet" className="performance-archive-sheet">
                {isNewFlow ? (
                  <NewFlowPlanArchiveSheet activity={activity} participant={participant} records={records} />
                ) : (
                  <ArchivePerformanceSheet activity={activity} participant={participant} records={records} />
                )}
              </div>
            )
          }
        ]}
      />

      <Modal
        title="完整上一周期结果"
        open={previousResultModalVisible}
        footer={null}
        width={1120}
        onCancel={() => setPreviousResultModalVisible(false)}
      >
        {previousParticipant ? (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Descriptions bordered size="small" column={{ xs: 1, sm: 2 }}>
              <Descriptions.Item label="活动">{previousActivity?.name || '-'}</Descriptions.Item>
              <Descriptions.Item label="员工">{previousParticipant.employee_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="最终等级">
                <StatusTag color={LEVEL_COLOR[previousParticipant.final_level || ''] || 'default'}>
                  {previousParticipant.final_level || '-'}
                </StatusTag>
              </Descriptions.Item>
              <Descriptions.Item label="最终分">
                {formatOptionalDecimal(previousParticipant.adjusted_score ?? previousParticipant.total_manager_score ?? previousParticipant.manager_score)}
              </Descriptions.Item>
              <Descriptions.Item label="部门/中心调整">
                {previousParticipant.department_adjusted ? '已调整' : '未调整'}
              </Descriptions.Item>
              <Descriptions.Item label="调整原因">
                {previousParticipant.department_adjust_reason || previousParticipant.adjust_reason || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="自评做得好的地方" span={2}>
                {previousParticipant.self_evaluation_good || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="自评需要改进的地方" span={2}>
                {previousParticipant.self_evaluation_improvement || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="上级评价做得好的地方" span={2}>
                {previousParticipant.manager_evaluation_good || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="上级评价需要改进的地方" span={2}>
                {previousParticipant.manager_evaluation_improvement || '-'}
              </Descriptions.Item>
            </Descriptions>

            <Divider orientation="left">考核指标</Divider>
            <Table
              dataSource={previousDisplayRecords}
              columns={previousDetailColumns}
              rowKey="id"
              pagination={false}
              size="small"
              bordered
              scroll={{ x: 1100 }}
              locale={{ emptyText: '暂无考核指标' }}
            />

            {previousBonusRecords.length > 0 && (
              <>
                <Divider orientation="left">附加考核项</Divider>
                <Table
                  dataSource={previousBonusRecords}
                  columns={previousBonusColumns}
                  rowKey="id"
                  pagination={false}
                  size="small"
                  bordered
                />
              </>
            )}

            {previousVersions.length > 0 && (
              <>
                <Divider orientation="left">调整留痕</Divider>
                <Table
                  dataSource={previousVersions}
                  columns={previousVersionColumns}
                  rowKey="id"
                  pagination={false}
                  size="small"
                  bordered
                />
              </>
            )}
          </Space>
        ) : (
          <Empty description={previousActivity ? '上一周期没有该员工绩效结果' : '暂无上一周期结果'} />
        )}
      </Modal>

      <Modal
        title="设置附加项分数"
        open={bonusPenaltyModalVisible}
        forceRender
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
          <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>
            调整后分数 = 基础分数 + 加分 - 扣分
          </Text>
        </Form>
      </Modal>
    </PageContainer>
  )
}

export default PerformanceResultView
