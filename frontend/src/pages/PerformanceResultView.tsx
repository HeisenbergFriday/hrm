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

const LEVEL_DEFINITIONS = [
  { level: 'S（100分以上）', label: '杰出', description: '整体绩效持续超过期望，工作业绩非常突出，团队标杆，具有重大贡献。绩效系数为1.2' },
  { level: 'A（90-99分）', label: '优秀', description: '整体绩效经常超过预期，工作业绩突出，团队积极榜样，具有较大贡献。绩效系数为1.1' },
  { level: 'B（80-89分）', label: '良好', description: '整体绩效符合期望，达成大部分或所有既定目标，需积极思考，追求进步。绩效系数为1' },
  { level: 'C（60-79分）', label: '待改进', description: '整体绩效略低期望，达成小部分既定目标，结果勉强接受，需积极整改，快速提升。绩效系数为0.8' },
  { level: 'D（60分以下）', label: '不合格', description: '整体绩效明显低于期望，结果不可接受，工作能力不胜任当前岗位要求。绩效系数为0.4' }
]

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
  const selfEvaluationGood = participant?.self_evaluation_good || ''
  const selfEvaluationImprovement = participant?.self_evaluation_improvement || ''
  const managerEvaluationGood = participant?.manager_evaluation_good || ''
  const managerEvaluationImprovement = participant?.manager_evaluation_improvement || ''
  const totalWeight = mainRecords.reduce((sum, record) => sum + (record.weight || 0), 0)
  const totalSelfScore = participant?.total_self_score || participant?.self_score || mainRecords.reduce((sum, record) => sum + getWeightedScore(record, 'self'), 0)
  const totalManagerScore = participant?.total_manager_score || participant?.manager_score || mainRecords.reduce((sum, record) => sum + getWeightedScore(record, 'manager'), 0)
  const period = activity?.start_date && activity?.end_date ? `${activity.start_date} 至 ${activity.end_date}` : '-'

  return (
    <div id="performance-archive-sheet" className="performance-archive-sheet">
      <table className="archive-table archive-header-table">
        <tbody>
          <tr>
            <td className="archive-logo-cell" colSpan={2}>小铁 自助台球</td>
            <td className="archive-title-cell" colSpan={8}>{activity?.name || '个人绩效考核表'}</td>
          </tr>
          <tr>
            <th rowSpan={2} colSpan={2}>基础信息</th>
            <th>姓名</th>
            <th>一级部门</th>
            <th>二级部门</th>
            <th>三级部门</th>
            <th>职级</th>
            <th>岗位</th>
            <th>直属上级</th>
            <th>考核周期</th>
          </tr>
          <tr>
            <td>{participant?.employee_name || '-'}</td>
            <td>{participant?.department_name || '-'}</td>
            <td>-</td>
            <td>-</td>
            <td>{participant?.level || '-'}</td>
            <td>{participant?.position || '-'}</td>
            <td>{participant?.manager_name || '-'}</td>
            <td>{period}</td>
          </tr>
        </tbody>
      </table>

      <table className="archive-table">
        <thead>
          <tr>
            <th colSpan={10}>PARTB: 个人绩效（员工绩效）</th>
          </tr>
          <tr>
            <th style={{ width: '8%' }}>类别</th>
            <th style={{ width: '12%' }}>指标名称/重点计划</th>
            <th style={{ width: '20%' }}>指标定义及口径说明</th>
            <th style={{ width: '7%' }}>权重</th>
            <th style={{ width: '7%' }}>红线值</th>
            <th style={{ width: '7%' }}>目标值</th>
            <th style={{ width: '7%' }}>挑战值</th>
            <th style={{ width: '14%' }}>考核标准</th>
            <th style={{ width: '12%' }}>实际达成结果</th>
            <th style={{ width: '6%' }}>自评/上级</th>
          </tr>
        </thead>
        <tbody>
          {mainRecords.length > 0 ? mainRecords.map(record => (
            <tr key={record.id}>
              <td>{SECTION_LABEL[record.section_type] || record.section_type}</td>
              <td>{record.item_name || '-'}</td>
              <td className="archive-text-cell">{record.item_definition || '-'}</td>
              <td>{formatWeight(record.weight)}</td>
              <td>{record.red_line_value || '-'}</td>
              <td>{record.target_value || '-'}</td>
              <td>{record.challenge_value || '-'}</td>
              <td className="archive-text-cell">{record.scoring_rule || '-'}</td>
              <td className="archive-text-cell">{record.actual_result || '-'}</td>
              <td>
                <div>自评 {formatScore(record.self_score)}</div>
                <div>上级 {formatScore(record.manager_score)}</div>
              </td>
            </tr>
          )) : (
            <tr>
              <td colSpan={10}>暂无指标明细</td>
            </tr>
          )}
          <tr className="archive-total-row">
            <td colSpan={3}>合计</td>
            <td>{formatWeight(totalWeight)}</td>
            <td>-</td>
            <td>-</td>
            <td>-</td>
            <td>-</td>
            <td>-</td>
            <td>
              <div>自评 {formatDecimal(totalSelfScore)}</div>
              <div>上级 {formatDecimal(totalManagerScore)}</div>
            </td>
          </tr>
          <tr>
            <th colSpan={5}>做得好的地方</th>
            <th colSpan={5}>需要提高改进的地方</th>
          </tr>
          <tr>
            <td colSpan={5} className="archive-evaluation-cell">
              <div className="archive-evaluation-title">员工自我评价</div>
              <div>{selfEvaluationGood || '1、\n2、'}</div>
            </td>
            <td colSpan={5} className="archive-evaluation-cell">
              <div className="archive-evaluation-title">员工自我评价</div>
              <div>{selfEvaluationImprovement || '1、\n2、'}</div>
            </td>
          </tr>
          <tr>
            <td colSpan={5} className="archive-evaluation-cell">
              <div className="archive-evaluation-title">上级总体评价</div>
              <div>{managerEvaluationGood || '1、\n2、'}</div>
            </td>
            <td colSpan={5} className="archive-evaluation-cell">
              <div className="archive-evaluation-title">上级总体评价</div>
              <div>{managerEvaluationImprovement || '1、\n2、'}</div>
            </td>
          </tr>
        </tbody>
      </table>

      <table className="archive-table archive-footer-table">
        <tbody>
          <tr>
            <th colSpan={10}>员工绩效等级（S A B C D）</th>
          </tr>
          <tr>
            <td colSpan={3}>个人绩效评定结果/等级</td>
            <td colSpan={2} className="archive-level-cell">{participant?.final_level || participant?.suggested_level || '-'}</td>
            <td colSpan={2}>绩效面谈进度</td>
            <td colSpan={3}>{participant?.final_level === 'C' || participant?.final_level === 'D' ? '需完成绩效面谈' : '按需绩效面谈'}</td>
          </tr>
          <tr>
            <td colSpan={3}>个人绩效目标确认签名/日期：</td>
            <td colSpan={2}>{participant?.employee_name || '-'}　{formatDate(participant?.employee_confirmed_at)}</td>
            <td colSpan={3}>个人绩效结果确认签名/日期：</td>
            <td colSpan={2}>{participant?.employee_name || '-'}　{formatDate(participant?.employee_confirmed_at)}</td>
          </tr>
          <tr>
            <td colSpan={5}>人力确认签名/日期：{participant?.hr_confirmed_by || '-'}　{formatDate(participant?.hr_confirmed_at)}</td>
            <td colSpan={5}>绩效等级确认：{participant?.manager_confirmed_by || participant?.manager_name || '-'}　{formatDate(participant?.manager_confirmed_at)}</td>
          </tr>
          <tr>
            <td colSpan={5} className="archive-note-cell">
              注：绩效结果可用于归档、复核、面谈与后续绩效沟通。若员工对结果有异议，应按公司流程发起申诉或线下复核。
            </td>
            <td colSpan={5} className="archive-note-cell">
              <strong>绩效评定定义如下：</strong>
              {LEVEL_DEFINITIONS.map(item => (
                <div key={item.level}><strong>{item.level}：{item.label}</strong>：{item.description}</div>
              ))}
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
.performance-archive-sheet {
  background: #fff;
  color: #000;
  font-family: Arial, "Microsoft YaHei", sans-serif;
  font-size: 12px;
  overflow-x: auto;
}
.archive-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}
.archive-table th,
.archive-table td {
  border: 1px solid #000;
  padding: 7px 6px;
  text-align: center;
  vertical-align: middle;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.35;
}
.archive-table th,
.archive-title-cell,
.archive-logo-cell,
.archive-total-row,
.archive-footer-table th {
  background: #ffc21f;
  font-weight: 700;
}
.archive-header-table th,
.archive-header-table td {
  background: #fff2cc;
}
.archive-header-table .archive-title-cell,
.archive-header-table .archive-logo-cell {
  background: #fff;
  font-size: 16px;
}
.archive-header-table .archive-logo-cell {
  text-align: left;
  font-weight: 700;
}
.archive-text-cell {
  text-align: left !important;
}
.archive-evaluation-cell {
  height: 56px;
  text-align: left !important;
  color: #d00;
}
.archive-evaluation-title {
  color: #000;
  font-weight: 700;
  margin-bottom: 4px;
}
.archive-level-cell {
  color: #d00;
  font-weight: 700;
}
.archive-note-cell {
  height: 150px;
  text-align: left !important;
  vertical-align: top !important;
}
@media print {
  @page {
    size: A4 landscape;
    margin: 8mm;
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
    overflow: visible;
    font-size: 10px;
  }
  .archive-table th,
  .archive-table td {
    padding: 4px;
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
