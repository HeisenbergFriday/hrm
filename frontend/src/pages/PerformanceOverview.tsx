import React, { useState, useCallback } from 'react'
import {
  Alert, Card, Col, Row, Space, Statistic, Table, Tag, Typography, Button, Modal, Form, Input, InputNumber,
  Select, DatePicker, message, Spin, Drawer, Popconfirm, Tooltip, Divider, Descriptions, Progress
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import {
  performanceAPI,
  PerformanceActivity,
  PerformanceParticipant,
  PerformanceTemplate,
  PerformanceDistributionRule,
} from '../services/api'

const { Title, Text, Paragraph } = Typography
const { TextArea } = Input
const { RangePicker } = DatePicker

// 状态映射
const STATUS_MAP: Record<string, { label: string; color: string }> = {
  draft: { label: '草稿', color: 'default' },
  self_evaluation: { label: '自评中', color: 'processing' },
  manager_evaluation: { label: '主管评分', color: 'warning' },
  result_confirmed: { label: '已确认', color: 'success' },
  archived: { label: '已归档', color: 'default' },
}

// 参与人状态映射
const PARTICIPANT_STATUS_MAP: Record<string, { label: string; color: string }> = {
  pending: { label: '待自评', color: 'default' },
  self_submitted: { label: '已自评', color: 'processing' },
  manager_submitted: { label: '已评分', color: 'warning' },
  result_confirmed: { label: '已确认', color: 'success' },
  inactive: { label: '已离职', color: 'error' },
  removed_from_scope: { label: '已移除', color: 'error' },
}

const PerformanceOverview: React.FC = () => {
  const [activities, setActivities] = useState<PerformanceActivity[]>([])
  const [activitiesLoading, setActivitiesLoading] = useState(false)
  const [activitiesTotal, setActivitiesTotal] = useState(0)
  const [activityModalVisible, setActivityModalVisible] = useState(false)
  const [editingActivity, setEditingActivity] = useState<PerformanceActivity | null>(null)
  const [form] = Form.useForm()

  // 模板管理
  const [templates, setTemplates] = useState<PerformanceTemplate[]>([])
  const [templateModalVisible, setTemplateModalVisible] = useState(false)
  const [editingTemplate, setEditingTemplate] = useState<PerformanceTemplate | null>(null)
  const [templateForm] = Form.useForm()

  // 活动详情抽屉
  const [detailDrawerVisible, setDetailDrawerVisible] = useState(false)
  const [currentActivity, setCurrentActivity] = useState<PerformanceActivity | null>(null)
  const [participants, setParticipants] = useState<PerformanceParticipant[]>([])
  const [participantsLoading, setParticipantsLoading] = useState(false)
  const [summaryLoading, setSummaryLoading] = useState(false)
  const [distributionCheckLoading, setDistributionCheckLoading] = useState(false)
  const [summary, setSummary] = useState<any>(null)
  const [distributionCheck, setDistributionCheck] = useState<any>(null)
  const [distributionRules, setDistributionRules] = useState<PerformanceDistributionRule[]>([])

  // 评分弹窗
  const [selfEvalModalVisible, setSelfEvalModalVisible] = useState(false)
  const [managerEvalModalVisible, setManagerEvalModalVisible] = useState(false)
  const [currentParticipant, setCurrentParticipant] = useState<PerformanceParticipant | null>(null)
  const [selfEvalForm] = Form.useForm()
  const [managerEvalForm] = Form.useForm()

  // 强制分布弹窗
  const [distributionModalVisible, setDistributionModalVisible] = useState(false)
  const [distributionForm] = Form.useForm()

  // 超限警告弹窗
  const [distributionWarningModalVisible, setDistributionWarningModalVisible] = useState(false)

  // 批量评分相关
  const [batchEvalModalVisible, setBatchEvalModalVisible] = useState(false)
  const [batchEvalSelected, setBatchEvalSelected] = useState<number[]>([])
  const [batchEvalLoading, setBatchEvalLoading] = useState(false)

  // 批量确认相关
  const [batchConfirmModalVisible, setBatchConfirmModalVisible] = useState(false)
  const [batchConfirmSelected, setBatchConfirmSelected] = useState<number[]>([])
  const [batchConfirmLoading, setBatchConfirmLoading] = useState(false)

  // 加载活动列表
  const loadActivities = useCallback(async () => {
    setActivitiesLoading(true)
    try {
      const res: any = await performanceAPI.getActivities({ page: 1, page_size: 100 })
      const data = res.data || res
      setActivities(data.items || [])
      setActivitiesTotal(data.total || 0)
    } catch (err: any) {
      message.error(err?.response?.data?.message || '加载活动列表失败')
    } finally {
      setActivitiesLoading(false)
    }
  }, [])

  // 加载模板列表
  const loadTemplates = useCallback(async () => {
    try {
      const res: any = await performanceAPI.getTemplates()
      const data = res.data || res
      setTemplates(data.items || [])
    } catch (err: any) {
      message.error(err?.response?.data?.message || '加载模板列表失败')
    }
  }, [])

  React.useEffect(() => {
    loadActivities()
    loadTemplates()
  }, [loadActivities, loadTemplates])

  // 加载活动详情
  const loadActivityDetail = async (activity: PerformanceActivity) => {
    setCurrentActivity(activity)
    setDetailDrawerVisible(true)
    setParticipantsLoading(true)
    setSummaryLoading(true)
    setDistributionCheckLoading(true)

    // 使用 Promise.allSettled 避免单个接口失败阻塞整个流程
    const results = await Promise.allSettled([
      performanceAPI.getParticipants(activity.id, { page: 1, page_size: 200 }),
      performanceAPI.getResultSummary(activity.id),
      performanceAPI.getDistributionCheck(activity.id),
      performanceAPI.getDistributionRules(activity.id),
    ])

    // 处理参与人
    const participantsResult = results[0]
    if (participantsResult.status === 'fulfilled') {
      const res = participantsResult.value as any
      const pData = res?.data || res
      setParticipants(pData?.items || [])
    } else {
      setParticipants([])
    }
    setParticipantsLoading(false)

    // 处理统计摘要
    const summaryResult = results[1]
    if (summaryResult.status === 'fulfilled') {
      const res = summaryResult.value as any
      setSummary(res?.data || null)
    } else {
      setSummary(null)
    }
    setSummaryLoading(false)

    // 处理强制分布检查
    const distributionCheckResult = results[2]
    if (distributionCheckResult.status === 'fulfilled') {
      const res = distributionCheckResult.value as any
      const dcData = res?.data || res
      setDistributionCheck(dcData || null)
    } else {
      setDistributionCheck(null)
    }
    setDistributionCheckLoading(false)

    // 处理分布规则
    const rulesResult = results[3]
    if (rulesResult.status === 'fulfilled') {
      const res = rulesResult.value as any
      const rData = res?.data || res
      setDistributionRules(rData?.rules || [])
    } else {
      setDistributionRules([])
    }
  }

  // 创建/编辑活动
  const handleSaveActivity = async () => {
    try {
      const values = await form.validateFields()
      const data = {
        name: values.name,
        cycle_type: values.cycle_type,
        start_date: values.date_range[0].format('YYYY-MM-DD'),
        end_date: values.date_range[1].format('YYYY-MM-DD'),
        self_eval_start_at: values.self_eval_range[0].format('YYYY-MM-DD'),
        self_eval_end_at: values.self_eval_range[1].format('YYYY-MM-DD'),
        manager_eval_start_at: values.manager_eval_range[0].format('YYYY-MM-DD'),
        manager_eval_end_at: values.manager_eval_range[1].format('YYYY-MM-DD'),
        result_confirm_start_at: values.result_confirm_range[0].format('YYYY-MM-DD'),
        result_confirm_end_at: values.result_confirm_range[1].format('YYYY-MM-DD'),
        status: editingActivity?.status || 'draft',
        template_id: values.template_id,
        description: values.description,
      }
      if (editingActivity) {
        await performanceAPI.updateActivity(editingActivity.id, data)
        message.success('更新成功')
      } else {
        await performanceAPI.createActivity(data)
        message.success('创建成功')
      }
      setActivityModalVisible(false)
      form.resetFields()
      loadActivities()
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '操作失败')
    }
  }

  // 活动状态操作
  const handleActivityAction = async (action: string, activity: PerformanceActivity) => {
    try {
      const apiMap: Record<string, (id: number) => Promise<any>> = {
        start: performanceAPI.startActivity,
        'open-self-evaluation': performanceAPI.openSelfEvaluation,
        'open-manager-evaluation': performanceAPI.openManagerEvaluation,
        'confirm-results': performanceAPI.confirmResults,
        archive: performanceAPI.archiveActivity,
        publish: performanceAPI.publishActivity,
        close: performanceAPI.closeActivity,
        refresh: performanceAPI.refreshParticipants,
      }
      const apiFn = apiMap[action]
      if (!apiFn) return
      await apiFn(activity.id)
      message.success('操作成功')
      loadActivities()
      if (detailDrawerVisible && currentActivity?.id === activity.id) {
        const detailRes: any = await performanceAPI.getActivity(activity.id)
        const updated = detailRes.data?.activity || detailRes
        setCurrentActivity(updated)
      }
    } catch (err: any) {
      message.error(err?.response?.data?.message || '操作失败')
    }
  }

  // 保存模板
  const handleSaveTemplate = async () => {
    try {
      const values = await templateForm.validateFields()
      const sectionItems = values.items || []

      // 构建符合后端要求的模板数据
      const templateData = {
        name: values.name,
        description: values.description || '',
        status: values.status || 'active',
        sections: [{
          name: values.section_name || '默认维度',
          section_type: 'score',
          weight: 100,
          sort_order: 1,
          is_score_required: true,
          is_comment_required: false,
          items: sectionItems.map((item: any, idx: number) => ({
            name: item.name || '',
            description: item.description || '',
            max_score: item.max_score || 100,
            weight: item.weight || 100,
            sort_order: idx + 1,
          })),
        }],
      }

      if (editingTemplate) {
        await performanceAPI.updateTemplate(editingTemplate.id, templateData)
        message.success('模板更新成功')
      } else {
        await performanceAPI.createTemplate(templateData)
        message.success('模板创建成功')
      }
      setTemplateModalVisible(false)
      templateForm.resetFields()
      loadTemplates()
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '保存失败')
    }
  }

  // 提交自评
  const handleSubmitSelfEvaluation = async () => {
    if (!currentParticipant) return
    try {
      const values = await selfEvalForm.validateFields()
      await performanceAPI.submitReviewSelfEvaluation(currentParticipant.id, {
        self_content_json: {
          content: values.self_content || '',
        },
      })
      message.success('自评提交成功')
      setSelfEvalModalVisible(false)
      selfEvalForm.resetFields()
      if (currentActivity) loadActivityDetail(currentActivity)
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '提交失败')
    }
  }

  // 提交主管评分
  const handleSubmitManagerEvaluation = async () => {
    if (!currentParticipant) return
    try {
      const values = await managerEvalForm.validateFields()
      const scoreJson: Record<string, number> = {}
      if (values.scores && typeof values.scores === 'object') {
        Object.entries(values.scores).forEach(([key, val]) => {
          if (val !== undefined && val !== null) {
            scoreJson[key] = Number(val)
          }
        })
      }

      // 前置校验：检查强制分布配额
      if (currentActivity && distributionCheck && !distributionCheck.passed) {
        setDistributionWarningModalVisible(true)
        return
      }

      const totalScore = Object.values(scoreJson).reduce((sum: number, val: any) => sum + (Number(val) || 0), 0)
      await performanceAPI.submitReviewManagerEvaluation(currentParticipant.id, {
        manager_score_json: scoreJson,
        manager_comment: values.manager_comment || '',
        final_level: values.final_level || values.suggested_level || '',
        final_level_reason: values.final_level_reason || '',
      })
      message.success('主管评分提交成功')
      setManagerEvalModalVisible(false)
      managerEvalForm.resetFields()
      if (currentActivity) loadActivityDetail(currentActivity)
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '提交失败')
    }
  }

  // 保存强制分布规则
  const handleSaveDistribution = async () => {
    if (!currentActivity) return
    try {
      const values = await distributionForm.validateFields()
      const rules = ['S', 'A', 'B', 'C', 'D'].map(level => ({
        level,
        distribution_percent: values[`percent_${level}`] || 0,
        description: values[`desc_${level}`] || '',
      }))
      const total = Object.values(values).reduce((sum: number, v: any) => sum + (Number(v) || 0), 0)
      if (total !== 100) {
        message.warning(`比例总和 ${total}%，需等于 100%`)
        return
      }
      await performanceAPI.putDistributionRules(currentActivity.id, rules)
      message.success('强制分布规则已保存')
      setDistributionModalVisible(false)
      if (currentActivity) loadActivityDetail(currentActivity)
    } catch (err: any) {
      if (err.errorFields) return
      message.error(err?.response?.data?.message || '保存失败')
    }
  }

  // 打开活动表单
  const openActivityModal = (activity?: PerformanceActivity) => {
    setEditingActivity(activity || null)
    if (activity) {
      form.setFieldsValue({
        name: activity.name,
        cycle_type: activity.cycle_type,
        date_range: [dayjs(activity.start_date), dayjs(activity.end_date)],
        self_eval_range: [dayjs(activity.self_eval_start_at), dayjs(activity.self_eval_end_at)],
        manager_eval_range: [dayjs(activity.manager_eval_start_at), dayjs(activity.manager_eval_end_at)],
        result_confirm_range: [dayjs(activity.result_confirm_start_at), dayjs(activity.result_confirm_end_at)],
        template_id: activity.template_id,
        description: activity.description,
      })
    } else {
      form.resetFields()
    }
    setActivityModalVisible(true)
  }

  // 打开模板表单
  const openTemplateModal = (template?: PerformanceTemplate) => {
    setEditingTemplate(template || null)
    if (template) {
      templateForm.setFieldsValue({
        name: template.name,
        description: template.description,
        status: template.status,
        section_name: template.sections?.[0]?.name || '',
        items: template.sections?.[0]?.items?.map((item: any) => ({
          name: item.name,
          description: item.description,
          max_score: item.max_score,
          weight: item.weight,
        })) || [],
      })
    } else {
      templateForm.resetFields()
      templateForm.setFieldsValue({
        items: [{ name: '', description: '', max_score: 100, weight: 100 }],
      })
    }
    setTemplateModalVisible(true)
  }

  // 活动列表操作按钮
  const getActionButtons = (record: PerformanceActivity) => {
    const buttons: React.ReactNode[] = []
    const status = record.status

    buttons.push(
      <Button size="small" type="link" onClick={() => loadActivityDetail(record)} key="view">详情</Button>
    )

    if (status === 'draft') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('start', record)} key="start">开启自评</Button>
      )
    } else if (status === 'self_evaluation') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('open-self-evaluation', record)} key="notify-self">提醒自评</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('open-manager-evaluation', record)} key="open-mgr">开启主管评分</Button>
      )
    } else if (status === 'manager_evaluation') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('refresh', record)} key="refresh">刷新</Button>,
        <Button size="small" type="link" onClick={() => handleActivityAction('confirm-results', record)} key="confirm">确认结果</Button>
      )
    } else if (status === 'result_confirmed') {
      buttons.push(
        <Button size="small" type="link" onClick={() => handleActivityAction('archive', record)} key="archive">归档</Button>
      )
    }

    return buttons
  }

  // 活动列表 columns
  const activityColumns: ColumnsType<PerformanceActivity> = [
    { title: '活动名称', dataIndex: 'name', key: 'name', width: 180, ellipsis: true },
    { title: '周期', dataIndex: 'cycle_type', key: 'cycle_type', width: 80 },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (status: string) => {
        const s = STATUS_MAP[status] || { label: status, color: 'default' }
        return <Tag color={s.color}>{s.label}</Tag>
      }
    },
    { title: '自评时间', key: 'self_eval', width: 200, render: (_, r) => `${r.self_eval_start_at} ~ ${r.self_eval_end_at}` },
    { title: '主管评分时间', key: 'mgr_eval', width: 200, render: (_, r) => `${r.manager_eval_start_at} ~ ${r.manager_eval_end_at}` },
    { title: '操作', key: 'actions', fixed: 'right', width: 180, render: (_, record) => (
      <Space size={4}>{getActionButtons(record)}</Space>
    )},
  ]

  // 参与人 columns
  const participantColumns: ColumnsType<PerformanceParticipant> = [
    { title: '员工', dataIndex: 'employee_name', key: 'employee_name', width: 100 },
    { title: '部门', dataIndex: 'department_name', key: 'department_name', width: 120, ellipsis: true },
    { title: '岗位', dataIndex: 'position', key: 'position', width: 100, ellipsis: true },
    { title: '直属主管', dataIndex: 'manager_name', key: 'manager_name', width: 100 },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 90,
      render: (status: string, record: PerformanceParticipant) => {
        const s = PARTICIPANT_STATUS_MAP[status] || { label: status, color: 'default' }
        return (
          <Space size={4}>
            <Tag color={s.color}>{s.label}</Tag>
            {record.manager_id === null || record.manager_id === undefined || record.manager_id === '' ? (
              <Tooltip title="缺少主管">
                <Tag color="error">无主管</Tag>
              </Tooltip>
            ) : null}
          </Space>
        )
      }
    },
    { title: '自评分', dataIndex: 'self_score', key: 'self_score', width: 70 },
    { title: '主管分', dataIndex: 'manager_score', key: 'manager_score', width: 70 },
    { title: '等级', dataIndex: 'final_level', key: 'final_level', width: 60, render: (v: string) => v || '-' },
    {
      title: '操作', key: 'actions', fixed: 'right', width: 160,
      render: (_, record: PerformanceParticipant) => {
        const isArchived = currentActivity?.status === 'archived'
        const isConfirmed = record.status === 'result_confirmed'
        const canSelfEval = record.status === 'pending' || record.status === 'self_submitted'
        const canMgrEval = record.status === 'self_submitted' || record.status === 'manager_submitted'

        return (
          <Space size={4}>
            <Button
              size="small" type="link" disabled={isArchived || !canSelfEval}
              onClick={() => { setCurrentParticipant(record); setSelfEvalModalVisible(true); }}
            >自评</Button>
            <Button
              size="small" type="link" disabled={isArchived || !canMgrEval}
              onClick={() => { setCurrentParticipant(record); setManagerEvalModalVisible(true); }}
            >评分</Button>
          </Space>
        )
      }
    },
  ]

  // 统计数据
  const inProgressCount = activities.filter(a => ['self_evaluation', 'manager_evaluation'].includes(a.status)).length
  const confirmedCount = activities.filter(a => a.status === 'result_confirmed').length

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Space align="center" size={12} wrap style={{ marginBottom: 8 }}>
          <Title level={4} style={{ margin: 0 }}>绩效管理</Title>
        </Space>
        <Paragraph type="secondary" style={{ marginBottom: 0 }}>
          绩效活动管理与评分工作台。
        </Paragraph>
      </div>

      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} sm={12} lg={6}>
          <Card><Statistic title="绩效活动总数" value={activitiesTotal} suffix="个" /></Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card><Statistic title="进行中活动" value={inProgressCount} suffix="个" /></Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card><Statistic title="已确认结果" value={confirmedCount} suffix="个" /></Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card><Statistic title="已归档活动" value={activities.filter(a => a.status === 'archived').length} suffix="个" /></Card>
        </Col>
      </Row>

      {/* 活动列表 */}
      <Card
        title="绩效活动"
        style={{ marginBottom: 16 }}
        extra={
          <Space>
            <Button onClick={() => openActivityModal()}>新建活动</Button>
            <Button onClick={() => openTemplateModal()}>模板管理</Button>
            <Button onClick={() => loadActivities()} disabled={activitiesLoading}>刷新</Button>
          </Space>
        }
      >
        <Spin spinning={activitiesLoading}>
          <Table
            columns={activityColumns}
            dataSource={activities}
            rowKey="id"
            pagination={{ pageSize: 10, total: activitiesTotal }}
            size="small"
            scroll={{ x: 900 }}
          />
        </Spin>
      </Card>

      {/* 活动详情抽屉 */}
      <Drawer
        title={`活动详情：${currentActivity?.name || ''}`}
        placement="right"
        width={900}
        open={detailDrawerVisible}
        onClose={() => { setDetailDrawerVisible(false); setCurrentActivity(null); setParticipants([]); setSummary(null); setDistributionCheck(null); setDistributionRules([]); }}
      >
        {currentActivity && (
          <>
            <Descriptions column={2} size="small" style={{ marginBottom: 16 }} bordered>
              <Descriptions.Item label="状态">
                <Tag color={STATUS_MAP[currentActivity.status]?.color}>{STATUS_MAP[currentActivity.status]?.label}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="周期类型">{currentActivity.cycle_type}</Descriptions.Item>
              <Descriptions.Item label="绩效周期">{currentActivity.start_date} ~ {currentActivity.end_date}</Descriptions.Item>
              <Descriptions.Item label="自评时间">{currentActivity.self_eval_start_at} ~ {currentActivity.self_eval_end_at}</Descriptions.Item>
              <Descriptions.Item label="主管评分">{currentActivity.manager_eval_start_at} ~ {currentActivity.manager_eval_end_at}</Descriptions.Item>
              <Descriptions.Item label="结果确认">{currentActivity.result_confirm_start_at} ~ {currentActivity.result_confirm_end_at}</Descriptions.Item>
            </Descriptions>

            {/* 操作按钮 */}
            <Space style={{ marginBottom: 16 }}>
              {currentActivity.status === 'draft' && (
                <>
                  <Button type="primary" onClick={() => handleActivityAction('start', currentActivity)}>开启自评</Button>
                  <Button onClick={() => handleActivityAction('refresh', currentActivity)}>刷新参与人</Button>
                </>
              )}
              {currentActivity.status === 'self_evaluation' && (
                <>
                  <Button onClick={async () => {
                    try {
                      await performanceAPI.sendSelfEvalReminder(currentActivity.id)
                      message.success('已发送自评提醒')
                    } catch (err: any) {
                      message.error(err?.response?.data?.message || '发送提醒失败')
                    }
                  }}>提醒自评</Button>
                  <Button type="primary" onClick={() => handleActivityAction('open-manager-evaluation', currentActivity)}>开启主管评分</Button>
                  <Button onClick={() => handleActivityAction('refresh', currentActivity)}>刷新参与人</Button>
                </>
              )}
              {currentActivity.status === 'manager_evaluation' && (
                <>
                  <Button onClick={async () => {
                    try {
                      await performanceAPI.sendManagerEvalReminder(currentActivity.id)
                      message.success('已发送评分提醒')
                    } catch (err: any) {
                      message.error(err?.response?.data?.message || '发送提醒失败')
                    }
                  }}>提醒评分</Button>
                  <Button type="primary" onClick={() => handleActivityAction('confirm-results', currentActivity)}>确认结果</Button>
                  <Button onClick={() => handleActivityAction('refresh', currentActivity)}>刷新参与人</Button>
                  <Button onClick={() => setDistributionModalVisible(true)}>强制分布</Button>
                  <Divider type="vertical" />
                  <Button onClick={() => {
                    const selectable = participants.filter(p => p.status === 'self_submitted' || p.status === 'manager_submitted')
                    setBatchEvalSelected(selectable.map(p => p.id))
                    setBatchEvalModalVisible(true)
                  }}>批量评分</Button>
                  <Button onClick={() => {
                    const confirmable = participants.filter(p => p.status === 'manager_submitted')
                    setBatchConfirmSelected(confirmable.map(p => p.id))
                    setBatchConfirmModalVisible(true)
                  }}>批量确认</Button>
                </>
              )}
              {currentActivity.status === 'result_confirmed' && (
                <Button onClick={() => handleActivityAction('archive', currentActivity)}>归档活动</Button>
              )}
            </Space>

            <Divider>统计摘要</Divider>
            <Spin spinning={summaryLoading}>
              {summary ? (
                <Row gutter={12} style={{ marginBottom: 16 }}>
                  <Col span={6}><Card size="small"><Statistic title="参与人数" value={summary.total_participants} /></Card></Col>
                  <Col span={6}><Card size="small"><Statistic title="已自评" value={summary.self_submitted_count} /></Card></Col>
                  <Col span={6}><Card size="small"><Statistic title="已评分" value={summary.manager_submitted_count} /></Card></Col>
                  <Col span={6}><Card size="small"><Statistic title="已确认" value={summary.result_confirmed_count} /></Card></Col>
                </Row>
              ) : <Text type="secondary">暂无数据</Text>}
            </Spin>

            {distributionCheck && (
              <Card size="small" style={{ marginBottom: 16 }}>
                <Row gutter={[8, 8]}>
                  {['S', 'A', 'B', 'C', 'D'].map(level => {
                    const dist = distributionCheck.distribution?.[level]
                    if (!dist) return null
                    const statusColor = dist.status === 'exceeded' ? 'exception' : dist.status === 'warning' ? 'normal' : 'success'
                    return (
                      <Col span={4} key={level}>
                        <Card size="small" style={{ textAlign: 'center', background: dist.status === 'exceeded' ? '#fff2f0' : dist.status === 'warning' ? '#fffbe6' : '#f6ffed' }}>
                          <Text strong style={{ fontSize: 16 }}>{level}</Text>
                          <br />
                          <Text type="secondary">{dist.actual_count}/{dist.expected_count} 人</Text>
                          <br />
                          <Progress percent={Math.min(dist.progress, 100)} size="small" status={statusColor} showInfo={false} strokeWidth={6} />
                          <Text type="secondary" style={{ fontSize: 10 }}>期望 {dist.expected_percent}%</Text>
                        </Card>
                      </Col>
                    )
                  })}
                </Row>
                {!distributionCheck.passed && distributionCheck.warnings?.length > 0 && (
                  <Alert
                    type="warning"
                    showIcon
                    message="配额超限"
                    description={distributionCheck.warnings.join('；')}
                    style={{ marginTop: 8 }}
                  />
                )}
              </Card>
            )}

            <Divider>参与人列表</Divider>
            <Spin spinning={participantsLoading}>
              <Table
                columns={participantColumns}
                dataSource={participants}
                rowKey="id"
                pagination={{ pageSize: 10 }}
                size="small"
                scroll={{ x: 900 }}
              />
            </Spin>
          </>
        )}
      </Drawer>

      {/* 新建/编辑活动弹窗 */}
      <Modal
        title={editingActivity ? '编辑活动' : '新建活动'}
        open={activityModalVisible}
        onOk={handleSaveActivity}
        onCancel={() => { setActivityModalVisible(false); form.resetFields(); }}
        width={700}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="活动名称" rules={[{ required: true }]}>
            <Input placeholder="如：2026 Q2 绩效评估" />
          </Form.Item>
          <Form.Item name="cycle_type" label="周期类型" rules={[{ required: true }]}>
            <Select placeholder="选择周期类型">
              <Select.Option value="monthly">月度</Select.Option>
              <Select.Option value="quarterly">季度</Select.Option>
              <Select.Option value="annual">年度</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="template_id" label="关联模板" rules={[{ required: true }]}>
            <Select placeholder="请选择绩效模板" allowClear>
              {templates.map(t => (
                <Select.Option key={t.id} value={t.id}>{t.name}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="date_range" label="绩效周期" rules={[{ required: true }]}>
            <RangePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="self_eval_range" label="自评时间" rules={[{ required: true }]}>
            <RangePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="manager_eval_range" label="主管评分时间" rules={[{ required: true }]}>
            <RangePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="result_confirm_range" label="结果确认时间" rules={[{ required: true }]}>
            <RangePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 模板管理弹窗 */}
      <Modal
        title="模板管理"
        open={templateModalVisible}
        onOk={handleSaveTemplate}
        onCancel={() => { setTemplateModalVisible(false); templateForm.resetFields(); }}
        width={700}
      >
        <Form form={templateForm} layout="vertical">
          <Form.Item name="name" label="模板名称" rules={[{ required: true }]}>
            <Input placeholder="如：研发绩效模板" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <TextArea rows={2} />
          </Form.Item>
          <Form.Item name="section_name" label="维度名称" initialValue="目标结果">
            <Input />
          </Form.Item>
          <Form.Item label="评分项">
            <Form.List name="items">
              {(fields, { add, remove }) => (
                <>
                  {fields.map(({ key, name, fieldKey }) => (
                    <Card key={key} size="small" style={{ marginBottom: 8 }}>
                      <Space direction="vertical" style={{ width: '100%' }}>
                        <Space wrap>
                          <Form.Item name={[name, 'name']} fieldKey={fieldKey} label="名称" rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                            <Input placeholder="如：KPI 达成" style={{ width: 160 }} />
                          </Form.Item>
                          <Form.Item name={[name, 'max_score']} fieldKey={fieldKey} label="满分" initialValue={100} style={{ marginBottom: 0 }}>
                            <InputNumber min={1} max={100} style={{ width: 80 }} />
                          </Form.Item>
                          <Form.Item name={[name, 'weight']} fieldKey={fieldKey} label="权重%" initialValue={100} style={{ marginBottom: 0 }}>
                            <InputNumber min={1} max={100} style={{ width: 80 }} />
                          </Form.Item>
                          <Button type="link" danger onClick={() => remove(name)}>删除</Button>
                        </Space>
                      </Space>
                    </Card>
                  ))}
                  <Button type="dashed" onClick={() => add({ name: '', description: '', max_score: 100, weight: 100 })} block>添加评分项</Button>
                </>
              )}
            </Form.List>
          </Form.Item>
        </Form>
      </Modal>

      {/* 自评弹窗 */}
      <Modal
        title={`自评 - ${currentParticipant?.employee_name || ''}`}
        open={selfEvalModalVisible}
        onOk={handleSubmitSelfEvaluation}
        onCancel={() => { setSelfEvalModalVisible(false); selfEvalForm.resetFields(); }}
      >
        <Form form={selfEvalForm} layout="vertical">
          <Form.Item name="self_content" label="自评内容" rules={[{ required: true }]}>
            <TextArea rows={6} placeholder="请填写自评内容..." />
          </Form.Item>
        </Form>
      </Modal>

      {/* 主管评分弹窗 */}
      <Modal
        title={`主管评分 - ${currentParticipant?.employee_name || ''}`}
        open={managerEvalModalVisible}
        onOk={handleSubmitManagerEvaluation}
        onCancel={() => { setManagerEvalModalVisible(false); managerEvalForm.resetFields(); }}
      >
        <Form form={managerEvalForm} layout="vertical">
          <Form.Item name={['scores', 'KPI1']} label="KPI1 评分" rules={[{ required: true }]}>
            <InputNumber min={0} max={100} style={{ width: '100%' }} placeholder="请输入分数" />
          </Form.Item>
          <Form.Item name="final_level" label="最终等级" rules={[{ required: true }]}>
            <Select placeholder="选择等级">
              <Select.Option value="S">S - 杰出</Select.Option>
              <Select.Option value="A">A - 优秀</Select.Option>
              <Select.Option value="B">B - 良好</Select.Option>
              <Select.Option value="C">C - 合格</Select.Option>
              <Select.Option value="D">D - 不合格</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="final_level_reason" label="定级理由">
            <TextArea rows={2} placeholder="请填写定级理由..." />
          </Form.Item>
          <Form.Item name="manager_comment" label="主管评语" rules={[{ required: true }]}>
            <TextArea rows={3} placeholder="请填写评语..." />
          </Form.Item>
        </Form>
      </Modal>

      {/* 强制分布规则弹窗 */}
      <Modal
        title="强制分布规则"
        open={distributionModalVisible}
        onOk={handleSaveDistribution}
        onCancel={() => setDistributionModalVisible(false)}
      >
        <Alert showIcon type="info" style={{ marginBottom: 16 }} message="前端校验比例总和需等于 100%，但以后端校验为准。" />
        <Form form={distributionForm} layout="vertical">
          {['S', 'A', 'B', 'C', 'D'].map(level => (
            <Card key={level} size="small" style={{ marginBottom: 8 }}>
              <Space wrap>
                <Text strong style={{ width: 40 }}>等级 {level}：</Text>
                <Form.Item name={`percent_${level}`} label="比例%" initialValue={distributionRules.find(r => r.level === level)?.distribution_percent || 0} style={{ marginBottom: 0 }}>
                  <InputNumber min={0} max={100} />
                </Form.Item>
                <Form.Item name={`desc_${level}`} label="说明" initialValue={distributionRules.find(r => r.level === level)?.description || ''} style={{ marginBottom: 0 }}>
                  <Input placeholder="如：杰出贡献" style={{ width: 120 }} />
                </Form.Item>
              </Space>
            </Card>
          ))}
          <Text type="secondary">示例：S: 10%, A: 20%, B: 40%, C: 20%, D: 10%</Text>
        </Form>
      </Modal>

      {/* 强制分布超限警告弹窗 */}
      <Modal
        title="配额超限警告"
        open={distributionWarningModalVisible}
        onCancel={() => setDistributionWarningModalVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setDistributionWarningModalVisible(false)}>取消</Button>,
          <Button key="adjust" type="link" onClick={() => { setDistributionWarningModalVisible(false); setDistributionModalVisible(true) }}>调整配额</Button>,
          <Button key="force" type="primary" danger onClick={async () => {
            // 强制提交逻辑 - 用户确认后继续
            setDistributionWarningModalVisible(false)
            if (!currentParticipant) return
            try {
              const values = managerEvalForm.getFieldsValue()
              const scoreJson: Record<string, number> = {}
              if (values.scores && typeof values.scores === 'object') {
                Object.entries(values.scores).forEach(([key, val]) => {
                  if (val !== undefined && val !== null) scoreJson[key] = Number(val)
                })
              }
              await performanceAPI.submitReviewManagerEvaluation(currentParticipant.id, {
                manager_score_json: scoreJson,
                manager_comment: values.manager_comment || '',
                final_level: values.final_level || '',
                final_level_reason: values.final_level_reason || '',
              })
              message.success('主管评分提交成功（已确认超限）')
              setManagerEvalModalVisible(false)
              managerEvalForm.resetFields()
              if (currentActivity) loadActivityDetail(currentActivity)
            } catch (err: any) {
              message.error(err?.response?.data?.message || '提交失败')
            }
          }}>确认超限提交</Button>,
        ]}
      >
        <Alert
          type="error"
          message="当前绩效分布超出配额限制"
          description={
            <div>
              <p>以下等级超出预设配额：</p>
              {distributionCheck?.exceeded_levels?.map((e: any) => (
                <p key={e.level} style={{ color: '#ff4d4f' }}>{e.level}级：期望 {e.expected} 人，实际 {e.actual} 人（超出 {e.excess} 人）</p>
              ))}
              <p style={{ marginTop: 8 }}>请调整评分后重新提交，或联系管理员调整配额规则。</p>
            </div>
          }
        />
        <Divider />
        <Text type="secondary">如需调整配额，请点击「调整配额」按钮修改分布规则。</Text>
      </Modal>

      {/* 批量主管评分弹窗 */}
      <Modal
        title="批量主管评分"
        open={batchEvalModalVisible}
        onOk={async () => {
          if (!currentActivity || batchEvalSelected.length === 0) return
          setBatchEvalLoading(true)
          try {
            // TODO: 调用批量评分接口，弹出表单让用户输入统一的评分和评语
            message.info('批量评分功能开发中，请逐个评分')
            setBatchEvalModalVisible(false)
          } catch (err: any) {
            message.error(err?.response?.data?.message || '批量评分失败')
          } finally {
            setBatchEvalLoading(false)
          }
        }}
        onCancel={() => setBatchEvalModalVisible(false)}
        confirmLoading={batchEvalLoading}
      >
        <Alert type="info" message={`已选择 ${batchEvalSelected.length} 名员工进行批量评分`} style={{ marginBottom: 16 }} />
        <Text type="secondary">批量评分功能将允许主管一次性为多名员工提交评分和评语。</Text>
      </Modal>

      {/* 批量确认结果弹窗 */}
      <Modal
        title="批量确认结果"
        open={batchConfirmModalVisible}
        onOk={async () => {
          if (!currentActivity || batchConfirmSelected.length === 0) return
          setBatchConfirmLoading(true)
          try {
            await performanceAPI.batchConfirmResults(currentActivity.id, batchConfirmSelected)
            message.success(`成功确认 ${batchConfirmSelected.length} 名员工的结果`)
            setBatchConfirmModalVisible(false)
            if (currentActivity) loadActivityDetail(currentActivity)
          } catch (err: any) {
            message.error(err?.response?.data?.message || '批量确认失败')
          } finally {
            setBatchConfirmLoading(false)
          }
        }}
        onCancel={() => setBatchConfirmModalVisible(false)}
        confirmLoading={batchConfirmLoading}
      >
        <Alert type="warning" message={`确认后 ${batchConfirmSelected.length} 名员工的绩效结果将正式生效，无法撤回`} style={{ marginBottom: 16 }} />
      </Modal>
    </div>
  )
}

export default PerformanceOverview
