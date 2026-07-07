import { cloneElement, useState, useEffect, useCallback, useRef, type ComponentProps, type ReactElement } from 'react'
import {
  Table, Button, Modal, Form, Input, Select, Tag, Space, Card, message,
  Popconfirm, Empty, InputNumber, Row, Col, AutoComplete, Tooltip, Alert
} from 'antd'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import { PlusOutlined, ArrowLeftOutlined, DeleteOutlined, DatabaseOutlined, EditOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { performanceAPI, departmentAPI, type PerformanceIndicatorLibrary as ILibrary, type PerformanceIndicatorItem, type PerformanceTemplate } from '../services/api'
import { hasPermission } from '../utils/permission'

const { TextArea } = Input

type DraftIndicatorItem = {
  name: string
  description: string
  weight: number
  indicator_type?: MutengIndicatorType | string
  red_line_value?: string
  target_value?: string
  challenge_value?: string
  scoring_rule?: string
}

type MutengIndicatorType = 'okr' | 'kpi'

type DepartmentOption = {
  department_id: string
  name: string
}

const isValidWeight = (weight: number) => Number.isFinite(weight) && weight >= 10 && weight % 5 === 0
const INDICATOR_MANAGE_PERMISSION = 'performance:indicator:manage'
const INDICATOR_MANAGE_PERMISSION_NAME = '绩效指标库管理'
const flowTemplateLabels: Record<string, string> = {
  old: '小铁文娱流程模版',
  new: '沐腾科技流程模版',
}

const builtInTemplateNames = new Set([
  '旧绩效流程模板',
  '新绩效流程模板',
  '旧绩效流程模版',
  '新绩效流程模版',
  '旧流程模板',
  '新流程模板',
  '旧流程模版',
  '新流程模版',
  '旧流程',
  '新流程',
])

function getTemplateDisplayName(template?: PerformanceTemplate | null) {
  if (!template) return '未绑定流程模板'
  const name = String(template.name || '').trim()
  const flowType = String(template.flow_type || '').trim()
  if (name && !builtInTemplateNames.has(name)) return name
  return flowTemplateLabels[flowType] || name || '未命名流程模版'
}

const newQuantItem = (): DraftIndicatorItem => ({
  name: '',
  description: '',
  weight: 10,
  red_line_value: '',
  target_value: '',
  challenge_value: '',
  scoring_rule: '',
})

const newActionItem = (): DraftIndicatorItem => ({
  name: '',
  description: '',
  weight: 10,
  target_value: '',
})

const newMutengItem = (indicatorType: MutengIndicatorType = 'okr', weight = 100): DraftIndicatorItem => ({
  indicator_type: indicatorType,
  name: '',
  description: '',
  weight,
  target_value: '',
})

const getMutengSectionType = (indicatorType?: string) => indicatorType === 'kpi' ? 'quantitative' : 'key_action'

export default function PerformanceIndicatorLibrary() {
  const navigate = useNavigate()
  const canManageIndicator = hasPermission(INDICATOR_MANAGE_PERMISSION)
  const indicatorManagePermissionTip = `你缺少${INDICATOR_MANAGE_PERMISSION_NAME}权限，需要联系管理员添加`
  const [libraries, setLibraries] = useState<ILibrary[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [selectedLib, setSelectedLib] = useState<ILibrary | null>(null)
  const detailCardRef = useRef<HTMLDivElement | null>(null)
  const [items, setItems] = useState<PerformanceIndicatorItem[]>([])
  const [itemsLoading, setItemsLoading] = useState(false)
  const [departments, setDepartments] = useState<DepartmentOption[]>([])
  const [departmentsLoading, setDepartmentsLoading] = useState(false)
  const [templates, setTemplates] = useState<PerformanceTemplate[]>([])
  const [templatesLoading, setTemplatesLoading] = useState(false)
  const [form] = Form.useForm()
  const [inheritOpen, setInheritOpen] = useState(false)
  const [inheritForm] = Form.useForm()
  const [inheriting, setInheriting] = useState(false)
  const [editItemOpen, setEditItemOpen] = useState(false)
  const [editItemForm] = Form.useForm()
  const [editingItem, setEditingItem] = useState<PerformanceIndicatorItem | null>(null)
  const [editingItemLoading, setEditingItemLoading] = useState(false)
  const [quantItems, setQuantItems] = useState<DraftIndicatorItem[]>([newQuantItem()])
  const [actionItems, setActionItems] = useState<DraftIndicatorItem[]>([newActionItem()])
  const [mutengItems, setMutengItems] = useState<DraftIndicatorItem[]>([newMutengItem()])
  const [quantSearchResults, setQuantSearchResults] = useState<Record<number, any[]>>({})
  const [actionSearchResults, setActionSearchResults] = useState<Record<number, any[]>>({})
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const selectedCreateTemplateId = Form.useWatch('template_id', form)
  const selectedCreateTemplate = templates.find(template => String(template.id) === String(selectedCreateTemplateId))
  const isCreateMutengTemplate = selectedCreateTemplate?.flow_type === 'new'
  const selectedLibTemplate = selectedLib
    ? templates.find(template => String(template.id) === String(selectedLib.template_id))
    : undefined
  const isSelectedLibMutengTemplate = selectedLibTemplate?.flow_type === 'new'
  const isEditingMutengItem = !!editingItem && isSelectedLibMutengTemplate
  const createRuleText = isCreateMutengTemplate
    ? '沐腾科技流程模版按 OKR/KPI 维护目标/关键职责事项，权重合计必须为 100%。'
    : '量化指标权重合计 70%，关键行动权重合计 30%，总权重必须为 100%。'
  const mutengWeightTotal = mutengItems.reduce((sum, item) => sum + (item.weight || 0), 0)

  const fetchLibraries = useCallback(async () => {
    setLoading(true)
    try {
      const res: any = await performanceAPI.getIndicatorLibraries({ page, page_size: pageSize })
      const data = res.data || res
      setLibraries(data?.items || [])
      setTotal(data?.total || 0)
    } catch {
      message.error('获取指标库列表失败')
    } finally {
      setLoading(false)
    }
  }, [page, pageSize])

  useEffect(() => { fetchLibraries() }, [fetchLibraries])

  const fetchDepartments = useCallback(async () => {
    setDepartmentsLoading(true)
    try {
      const res: any = await departmentAPI.getDepartments()
      const data = res.data || res
      setDepartments(data?.departments || [])
    } catch {
      message.error('获取部门列表失败')
    } finally {
      setDepartmentsLoading(false)
    }
  }, [])

  useEffect(() => { fetchDepartments() }, [fetchDepartments])

  const fetchTemplates = useCallback(async () => {
    setTemplatesLoading(true)
    try {
      const res: any = await performanceAPI.getTemplates({ page: 1, page_size: 1000, status: 'active' })
      const data = res.data || res
      setTemplates(data?.items || data?.templates || [])
    } catch {
      message.error('获取流程模板失败')
    } finally {
      setTemplatesLoading(false)
    }
  }, [])

  useEffect(() => { fetchTemplates() }, [fetchTemplates])

  const resetCreateState = () => {
    form.resetFields()
    setQuantItems([newQuantItem()])
    setActionItems([newActionItem()])
    setMutengItems([newMutengItem()])
    setQuantSearchResults({})
    setActionSearchResults({})
  }

  const guardIndicatorManage = () => {
    if (canManageIndicator) return true
    message.warning(indicatorManagePermissionTip)
    return false
  }

  const renderIndicatorManageButton = (button: ReactElement) => {
    if (canManageIndicator) return button
    return (
      <Tooltip title={indicatorManagePermissionTip}>
        <span>
          {cloneElement(button, {
            disabled: true,
            onClick: undefined,
          } as Partial<ComponentProps<typeof Button>>)}
        </span>
      </Tooltip>
    )
  }

  const fetchItems = async (libraryId: number) => {
    setItemsLoading(true)
    try {
      const res: any = await performanceAPI.getIndicatorItems(libraryId)
      const data = res.data || res
      setItems(data?.items || [])
    } catch {
      message.error('获取指标项失败')
    } finally {
      setItemsLoading(false)
    }
  }

  const handleCreate = async (values: any) => {
    if (!guardIndicatorManage()) return

    const selectedTemplate = templates.find(template => String(template.id) === String(values.template_id))
    const isMutengTemplate = selectedTemplate?.flow_type === 'new'
    let payloadItems: any[] = []

    if (isMutengTemplate) {
      const totalWeight = mutengItems.reduce((sum, item) => sum + (item.weight || 0), 0)

      if (mutengItems.some(item => !isValidWeight(item.weight))) {
        message.error('权重必须为 5% 的倍数，且单项不低于 10%')
        return
      }
      if (Math.abs(totalWeight - 100) > 0.001) {
        message.error(`权重合计必须为 100%，当前为 ${totalWeight}%`)
        return
      }
      if (mutengItems.some(item => !item.name.trim() || !item.description.trim() || !item.target_value?.trim())) {
        message.warning('请填写目标/关键职责事项、说明和完成情况')
        return
      }

      payloadItems = mutengItems.map((item, idx) => {
        const indicatorType = item.indicator_type === 'kpi' ? 'kpi' : 'okr'
        return {
          section_type: getMutengSectionType(indicatorType),
          indicator_type: indicatorType,
          name: item.name,
          description: item.description,
          weight: item.weight,
          target_value: item.target_value,
          is_default: true,
          sort_order: idx + 1,
        }
      })
    } else {
      const totalWeight = [...quantItems, ...actionItems].reduce((sum, item) => sum + (item.weight || 0), 0)
      const quantWeight = quantItems.reduce((sum, item) => sum + (item.weight || 0), 0)
      const actionWeight = actionItems.reduce((sum, item) => sum + (item.weight || 0), 0)

      if ([...quantItems, ...actionItems].some(item => !isValidWeight(item.weight))) {
        message.error('权重必须为 5% 的倍数，且单项不低于 10%')
        return
      }
      if (Math.abs(totalWeight - 100) > 0.001) {
        message.error(`权重合计必须为 100%，当前为 ${totalWeight}%`)
        return
      }
      if (Math.abs(quantWeight - 70) > 0.001) {
        message.error(`量化指标权重必须为 70%，当前为 ${quantWeight}%`)
        return
      }
      if (Math.abs(actionWeight - 30) > 0.001) {
        message.error(`关键行动权重必须为 30%，当前为 ${actionWeight}%`)
        return
      }
      if (quantItems.some(item => !item.name.trim() || !item.description.trim())) {
        message.warning('请填写量化指标名称和指标定义及口径说明')
        return
      }
      if (actionItems.some(item => !item.name.trim() || !item.description.trim() || !item.target_value?.trim())) {
        message.warning('请填写关键行动名称、指标定义及口径说明和定性目标')
        return
      }
      if (quantItems.some(item =>
        !item.red_line_value?.trim() ||
        !item.target_value?.trim() ||
        !item.challenge_value?.trim() ||
        !item.scoring_rule?.trim()
      )) {
        message.warning('请填写量化指标的红线值、目标值、挑战值和考核标准')
        return
      }

      payloadItems = [
        ...quantItems.map((item, idx) => ({
          section_type: 'quantitative',
          name: item.name,
          description: item.description,
          weight: item.weight,
          red_line_value: item.red_line_value,
          target_value: item.target_value,
          challenge_value: item.challenge_value,
          scoring_rule: item.scoring_rule,
          is_default: true,
          sort_order: idx + 1,
        })),
        ...actionItems.map((item, idx) => ({
          section_type: 'key_action',
          name: item.name,
          description: item.description,
          weight: item.weight,
          target_value: item.target_value,
          is_default: true,
          sort_order: quantItems.length + idx + 1,
        })),
      ]
    }

    const payload = {
      ...values,
      template_id: Number(values.template_id),
      items: payloadItems,
    }

    setCreating(true)
    try {
      const res: any = await performanceAPI.createIndicatorLibrary(payload)
      const data = res.data || res
      const library = data?.library
      message.success('创建成功')
      setCreateOpen(false)
      resetCreateState()
      await fetchLibraries()
      if (library?.id) {
        setSelectedLib(library)
        fetchItems(library.id)
      }
    } catch (err: any) {
      message.error(err.response?.data?.message || '创建失败')
    } finally {
      setCreating(false)
    }
  }

  const handleInherit = async (values: any) => {
    if (!guardIndicatorManage()) return

    const dept = departments.find(d => d.department_id === values.target_department_id)
    setInheriting(true)
    try {
      const res: any = await performanceAPI.inheritIndicatorLibrary({
        parent_library_id: values.parent_library_id,
        target_department_id: values.target_department_id,
        target_department_name: dept?.name || '',
        name: values.name || undefined,
        description: values.description || undefined,
      })
      const data = res.data || res
      const library = data?.library
      message.success('继承成功')
      setInheritOpen(false)
      inheritForm.resetFields()
      await fetchLibraries()
      if (library?.id) {
        setSelectedLib(library)
        fetchItems(library.id)
      }
    } catch (err: any) {
      message.error(err.response?.data?.message || '继承失败')
    } finally {
      setInheriting(false)
    }
  }

  const handleEditItem = (item: PerformanceIndicatorItem) => {
    if (!guardIndicatorManage()) return

    setEditingItem(item)
    editItemForm.setFieldsValue({
      name: item.name,
      description: item.description,
      indicator_type: item.indicator_type || (item.section_type === 'quantitative' ? 'kpi' : 'okr'),
      weight: item.weight ?? item.default_weight,
      red_line_value: item.red_line_value,
      target_value: item.target_value,
      challenge_value: item.challenge_value,
      scoring_rule: item.scoring_rule,
    })
    setEditItemOpen(true)
  }

  const handleEditItemSubmit = async (values: any) => {
    if (!editingItem) return
    if (!guardIndicatorManage()) return

    setEditingItemLoading(true)
    try {
      const payload = isEditingMutengItem
        ? {
          ...values,
          indicator_type: values.indicator_type || editingItem.indicator_type || (editingItem.section_type === 'quantitative' ? 'kpi' : 'okr'),
          red_line_value: '',
          challenge_value: '',
          scoring_rule: '',
        }
        : values
      await performanceAPI.updateIndicatorItem(editingItem.id, payload)
      message.success('修改成功')
      setEditItemOpen(false)
      setEditingItem(null)
      if (selectedLib) fetchItems(selectedLib.id)
    } catch (err: any) {
      message.error(err.response?.data?.message || '修改失败')
    } finally {
      setEditingItemLoading(false)
    }
  }

  const handleDeleteItem = async (itemId: number) => {
    if (!guardIndicatorManage()) return

    try {
      await performanceAPI.deleteIndicatorItem(itemId)
      message.success('删除成功')
      if (selectedLib) fetchItems(selectedLib.id)
    } catch {
      message.error('删除失败')
    }
  }

  const handleArchive = async (id: number) => {
    if (!guardIndicatorManage()) return

    try {
      await performanceAPI.archiveIndicatorLibrary(id)
      message.success('归档成功')
      fetchLibraries()
      if (selectedLib?.id === id) setSelectedLib(null)
    } catch {
      message.error('归档失败')
    }
  }

  const scrollToDetailOnMobile = () => {
    window.setTimeout(() => {
      if (window.matchMedia('(max-width: 767.98px), (pointer: coarse)').matches) {
        const target = detailCardRef.current
        if (!target) return

        const targetTop = target.getBoundingClientRect().top + window.scrollY - 62
        window.scrollTo({ top: Math.max(0, targetTop), behavior: 'smooth' })
      }
    }, 180)
  }

  const handleSelectLib = (record: ILibrary) => {
    setSelectedLib(record)
    fetchItems(record.id)
    scrollToDetailOnMobile()
  }

  const updateQuantItem = (index: number, patch: Partial<DraftIndicatorItem>) => {
    setQuantItems(prev => prev.map((item, idx) => idx === index ? { ...item, ...patch } : item))
  }

  const updateActionItem = (index: number, patch: Partial<DraftIndicatorItem>) => {
    setActionItems(prev => prev.map((item, idx) => idx === index ? { ...item, ...patch } : item))
  }

  const updateMutengItem = (index: number, patch: Partial<DraftIndicatorItem>) => {
    setMutengItems(prev => prev.map((item, idx) => idx === index ? { ...item, ...patch } : item))
  }

  const searchIndicators = useCallback((keyword: string, resultsSetter: React.Dispatch<React.SetStateAction<Record<number, any[]>>>, rowIndex: number, sectionType: string) => {
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current)
    if (!keyword || keyword.trim().length < 1) {
      resultsSetter(prev => ({ ...prev, [rowIndex]: [] }))
      return
    }
    searchTimerRef.current = setTimeout(async () => {
      try {
        const res: any = await performanceAPI.searchIndicatorItems({ keyword: keyword.trim(), section_type: sectionType })
        const data = res.data || res
        const raw: any[] = data?.items || []
        resultsSetter(prev => ({ ...prev, [rowIndex]: raw }))
      } catch {
        resultsSetter(prev => ({ ...prev, [rowIndex]: [] }))
      }
    }, 300)
  }, [])

  const getSearchOptions = (results: any[]) =>
    results.map((item: any) => ({
      value: item.name,
      label: `${item.name}${item.description ? ' — ' + item.description.slice(0, 40) : ''}`,
    }))

  const handleIndicatorSelect = (
    value: string,
    rowIndex: number,
    sourceItems: any[],
    setter: (items: DraftIndicatorItem[]) => void,
    allItems: DraftIndicatorItem[],
  ) => {
    const matched = sourceItems.find((item: any) => item.name === value)
    if (!matched) return
    const patch: Partial<DraftIndicatorItem> = {
      name: matched.name,
      description: matched.description || '',
      red_line_value: matched.red_line_value || '',
      target_value: matched.target_value || '',
      challenge_value: matched.challenge_value || '',
      scoring_rule: matched.scoring_rule || '',
    }
    setter(allItems.map((item, idx) => idx === rowIndex ? { ...item, ...patch } : item))
  }

  const handleDepartmentChange = (departmentId: string) => {
    const department = departments.find(item => item.department_id === departmentId)
    form.setFieldsValue({
      department_id: departmentId,
      department_name: department?.name || '',
    })
  }

  const columns = [
    {
      title: '指标库名称', dataIndex: 'name', key: 'name', width: 180,
      render: (text: string, record: ILibrary) => (
        <a
          onClick={() => handleSelectLib(record)}
          style={{ fontWeight: 'var(--font-weight-semibold)', color: selectedLib?.id === record.id ? 'var(--color-primary)' : 'var(--color-text-heading)', fontSize: 'var(--font-size-base)' }}
        >
          {text}
        </a>
      ),
    },
    { title: '所属部门', dataIndex: 'department_name', key: 'department_name', width: 140 },
    {
      title: '流程模板',
      dataIndex: 'template_id',
      key: 'template_id',
      width: 150,
      render: (templateId?: number) => {
        const template = templates.find(item => String(item.id) === String(templateId))
        return (
          <Tag color={template?.flow_type === 'new' ? 'purple' : template?.flow_type === 'old' ? 'blue' : 'default'}>
            {getTemplateDisplayName(template)}
          </Tag>
        )
      },
    },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 80,
      render: (status: string) => (
        <StatusTag
          color={status === 'active' ? 'success' : 'default'}
        >
          {status === 'active' ? '启用' : '已归档'}
        </StatusTag>
      )
    },
    {
      title: '来源', key: 'source', width: 70,
      render: (_: any, record: ILibrary) => (
        <StatusTag color={record.parent_library_id ? 'warning' : 'success'}>
          {record.parent_library_id ? '继承' : '自建'}
        </StatusTag>
      )
    },
    {
      title: '操作', key: 'action', width: 110,
      render: (_: any, record: ILibrary) => (
        <Space size={4}>
          <Button type="link" size="small" onClick={() => handleSelectLib(record)}>查看</Button>
          {record.status === 'active' && canManageIndicator && (
            <Popconfirm title="确认归档该指标库？" onConfirm={() => handleArchive(record.id)}>
              <Button type="link" size="small" danger>归档</Button>
            </Popconfirm>
          )}
          {record.status === 'active' && !canManageIndicator && renderIndicatorManageButton(
            <Button type="link" size="small" danger>归档</Button>
          )}
        </Space>
      )
    }
  ]

  const itemColumns = [
    { title: isSelectedLibMutengTemplate ? '目标/关键职责事项' : '指标名称/重点计划', dataIndex: 'name', key: 'name' },
    {
      title: '类别',
      key: 'section_type',
      width: 90,
      render: (_: any, record: PerformanceIndicatorItem) => {
        if (isSelectedLibMutengTemplate) {
          const indicatorType = record.indicator_type || (record.section_type === 'quantitative' ? 'kpi' : 'okr')
          return <Tag color={indicatorType === 'kpi' ? 'blue' : 'green'}>{indicatorType === 'kpi' ? 'KPI' : 'OKR'}</Tag>
        }
        return (
          <span>{
            ({
              quantitative: '量化指标',
              key_action: '关键行动',
              bonus_penalty: '附加项',
            } as Record<string, string>)[record.section_type || record.indicator_type] || record.section_type || record.indicator_type || '-'
          }</span>
        )
      }
    },
    {
      title: '权重',
      key: 'weight',
      width: 80,
      render: (_: any, record: PerformanceIndicatorItem) => {
        const value = record.weight ?? record.default_weight ?? 0
        return `${value}%`
      }
    },
    {
      title: isSelectedLibMutengTemplate ? '完成情况' : '目标',
      key: 'target',
      render: (_: any, record: PerformanceIndicatorItem) => {
        if (isSelectedLibMutengTemplate) return record.target_value || '-'
        if (record.section_type === 'quantitative') {
          return [record.red_line_value, record.target_value, record.challenge_value].filter(Boolean).join(' / ') || '-'
        }
        return record.target_value || '-'
      }
    },
    {
      title: '操作', key: 'action', width: 100,
      render: (_: any, record: PerformanceIndicatorItem) => (
        <Space size={8}>
          {renderIndicatorManageButton(
            <Button type="link" size="small" icon={<EditOutlined />} aria-label="编辑指标项" onClick={() => handleEditItem(record)} />
          )}
          {canManageIndicator ? (
            <Popconfirm title="确认删除该指标项？" onConfirm={() => handleDeleteItem(record.id)}>
              <Button type="link" size="small" icon={<DeleteOutlined />} danger aria-label="删除指标项" />
            </Popconfirm>
          ) : renderIndicatorManageButton(
            <Button type="link" size="small" icon={<DeleteOutlined />} danger aria-label="删除指标项" />
          )}
        </Space>
      )
    },
  ]

  return (
    <PageContainer
      className="performance-indicator-page"
      title="指标库管理"
      icon={<DatabaseOutlined />}
      subtitle="创建时一次性配置指标项，创建后的指标库仅支持查看与继承"
      extra={
        <Button
          type="text"
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate(-1)}
          style={{
            paddingLeft: 0,
            color: 'var(--color-text-secondary)',
            fontWeight: 'var(--font-weight-medium)',
            marginBottom: 4,
            fontSize: 'var(--font-size-base)',
          }}
        >
          返回
        </Button>
      }
    >

      <div style={{
        background: 'var(--color-primary-bg)',
        border: '1px solid var(--color-border-light)',
        borderRadius: 'var(--radius-md)',
        padding: '10px 16px',
        marginBottom: 20,
        fontSize: 'var(--font-size-sm)',
        color: 'var(--color-text-secondary)',
      }}>
        <strong style={{ color: 'var(--color-text-heading)' }}>创建规则：</strong>
        {createRuleText}
      </div>
      {!canManageIndicator && (
        <Alert
          type="warning"
          showIcon
          message="当前账号缺少绩效指标库管理权限"
          description="本页仅支持查看指标库和指标项；创建、继承、归档、编辑和删除操作需要管理员补充分配功能权限。"
          style={{ marginBottom: 20 }}
        />
      )}

      <div className="performance-indicator-layout" style={{ display: 'flex', gap: 20, alignItems: 'flex-start' }}>
        <PageCard
          className="performance-indicator-list-card"
          title="指标库列表"
          style={{ flexShrink: 0 }}
          extra={
            <Space>
              {renderIndicatorManageButton(
                <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}
                  style={{ height: 36, boxShadow: canManageIndicator ? '0 2px 6px rgba(67,56,202,0.3)' : undefined }}
                >
                  创建
                </Button>
              )}
              {renderIndicatorManageButton(
                <Button style={{ height: 36 }} onClick={() => setInheritOpen(true)}>继承</Button>
              )}
            </Space>
          }
        >
          <Table
            dataSource={libraries}
            columns={columns}
            rowKey="id"
            loading={loading}
            pagination={{ current: page, pageSize, total, onChange: (p, ps) => { setPage(p); setPageSize(ps) }, showSizeChanger: false }}
            locale={{ emptyText: <Empty description="暂无指标库" imageStyle={{ height: 60 }} /> }}
          />
        </PageCard>

        <div ref={detailCardRef} className="performance-indicator-detail-anchor" style={{ flex: 1, minWidth: 0 }}>
        <PageCard
          className="performance-indicator-detail-card"
          title="指标项"
          style={{
            minWidth: 0,
          }}
        >
          {selectedLib ? (
            <div>
              <div style={{
                background: 'var(--color-primary-bg)',
                borderRadius: 'var(--radius-md)',
                padding: '10px 14px',
                marginBottom: 16,
                border: '1px solid var(--color-border-light)',
                display: 'flex',
                alignItems: 'center',
                gap: 10,
              }}>
                <strong style={{ color: 'var(--color-text-heading)', fontSize: 'var(--font-size-base)' }}>{selectedLib.name}</strong>
                <StatusTag color="purple" style={{ fontWeight: 'var(--font-weight-medium)', margin: 0 }}>
                  {getTemplateDisplayName(templates.find(template => String(template.id) === String(selectedLib.template_id)))}
                </StatusTag>
                <StatusTag color="blue" style={{ fontWeight: 'var(--font-weight-medium)', margin: 0 }}>{selectedLib.department_name}</StatusTag>
              </div>
              <Table
                dataSource={items}
                columns={itemColumns}
                rowKey="id"
                loading={itemsLoading}
                pagination={false}
                size="small"
                locale={{
                  emptyText: selectedLib
                    ? '该指标库当前没有指标项，请先创建或从其他库继承'
                    : '请先选择一个指标库',
                }}
              />
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', padding: '60px 0', color: 'var(--color-text-tertiary)' }}>
              <DatabaseOutlined style={{ fontSize: 48, marginBottom: 12, opacity: 0.4 }} />
              <div style={{ fontSize: 'var(--font-size-base)' }}>请先选择一个指标库</div>
              <div style={{ fontSize: 'var(--font-size-sm)', marginTop: 4 }}>点击左侧列表中的指标库名称查看指标项</div>
            </div>
          )}
        </PageCard>
        </div>
      </div>

      <Modal
        title={
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <div style={{
              width: 36,
              height: 36,
              borderRadius: 'var(--radius-md)',
              background: 'linear-gradient(135deg, var(--color-primary), #6366f1)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              boxShadow: '0 2px 8px rgba(99,102,241,0.35)',
            }}>
              <PlusOutlined style={{ color: '#fff', fontSize: 16 }} />
            </div>
            <span style={{ fontWeight: 'var(--font-weight-bold)', fontSize: 17, color: 'var(--color-text-title)' }}>创建指标库</span>
          </div>
        }
        open={createOpen}
        onCancel={() => { setCreateOpen(false); resetCreateState() }}
        onOk={() => { if (guardIndicatorManage()) form.submit() }}
        confirmLoading={creating}
        okButtonProps={{ disabled: !canManageIndicator }}
        width={1280}
        styles={{
          header: {
            background: 'linear-gradient(180deg, #f8f9ff 0%, #fff 100%)',
            borderBottom: '2px solid #eef2ff',
            paddingBottom: 16,
            marginBottom: 0,
            borderRadius: '14px 14px 0 0',
          },
          body: { paddingTop: 24 },
          mask: { backdropFilter: 'blur(4px)' },
        }}
      >
        <Form form={form} layout="vertical" onFinish={handleCreate} style={{ marginTop: 4 }}>
          <div style={{
            background: 'var(--color-bg-light)',
            borderRadius: 'var(--radius-lg)',
            padding: '22px 22px 10px',
            marginBottom: 22,
            border: '1px solid #e2e5f0',
            boxShadow: 'inset 0 1px 3px rgba(0,0,0,0.03)',
          }}>
            <div style={{ marginBottom: 14, display: 'flex', alignItems: 'center', gap: 8 }}>
              <div style={{ width: 4, height: 18, borderRadius: 'var(--radius-xs)', background: 'linear-gradient(180deg, var(--color-primary), #6366f1)' }} />
              <span style={{ fontWeight: 'var(--font-weight-bold)', fontSize: 'var(--font-size-base)', color: 'var(--color-text-heading)' }}>基本信息</span>
            </div>
            <Row gutter={20}>
              <Col span={8}>
                <Form.Item name="department_id" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>所属部门</span>} rules={[{ required: true, message: '请选择所属部门' }]}>
                  <Select
                    showSearch
                    placeholder="请选择所属部门"
                    loading={departmentsLoading}
                    optionFilterProp="label"
                    options={departments.map(item => ({
                      label: item.name,
                      value: item.department_id,
                    }))}
                    onChange={handleDepartmentChange}
                  />
                </Form.Item>
              </Col>
              <Form.Item name="department_name" hidden>
                <Input />
              </Form.Item>
              <Col span={8}>
                <Form.Item name="template_id" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>流程模板</span>} rules={[{ required: true, message: '请选择流程模板' }]}>
                  <Select
                    showSearch
                    placeholder="请选择流程模板"
                    loading={templatesLoading}
                    optionFilterProp="label"
                    options={templates.map(template => ({
                      label: getTemplateDisplayName(template),
                      value: template.id,
                    }))}
                    onChange={(value) => {
                      const template = templates.find(item => String(item.id) === String(value))
                      if (template?.flow_type === 'new') {
                        setMutengItems([newMutengItem()])
                      } else {
                        setQuantItems([newQuantItem()])
                        setActionItems([newActionItem()])
                        setQuantSearchResults({})
                        setActionSearchResults({})
                      }
                    }}
                  />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item name="name" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>指标库名称</span>} rules={[{ required: true, message: '请输入指标库名称' }]}>
                  <Input />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={20}>
              <Col span={16}>
                <Form.Item name="description" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>描述</span>}>
                  <TextArea rows={2} />
                </Form.Item>
              </Col>
              <Col span={8}>
                <Form.Item name="default_cycle" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>默认周期</span>} initialValue="monthly">
                  <Select
                    options={[{ value: 'monthly', label: '月度' }, { value: 'quarterly', label: '季度' }, { value: 'annual', label: '年度' }]}
                  />
                </Form.Item>
              </Col>
            </Row>
          </div>
        </Form>

        {isCreateMutengTemplate ? (
          <Card
            size="small"
            title={
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <div style={{ width: 4, height: 18, borderRadius: 'var(--radius-xs)', background: 'linear-gradient(180deg, var(--color-primary), #6366f1)' }} />
                <span style={{ fontWeight: 'var(--font-weight-bold)', fontSize: 'var(--font-size-base)', color: 'var(--color-text-heading)' }}>目标/关键职责事项</span>
                <span style={{
                  background: mutengWeightTotal === 100 ? 'linear-gradient(135deg, var(--color-primary-bg), #dbeafe)' : '#fff1f0',
                  color: mutengWeightTotal === 100 ? 'var(--color-primary)' : 'var(--color-error)',
                  fontSize: 'var(--font-size-xs)',
                  fontWeight: 'var(--font-weight-bold)',
                  padding: '4px 12px',
                  borderRadius: 'var(--radius-xl)',
                  border: mutengWeightTotal === 100 ? '1px solid #c7d2fe' : '1px solid #ffccc7',
                  letterSpacing: 0.3,
                }}>
                  权重合计 {mutengWeightTotal}%
                </span>
              </div>
            }
            style={{
              marginTop: 10,
              borderRadius: 'var(--radius-lg)',
              border: '1px solid #e0e3ed',
              boxShadow: '0 1px 6px rgba(0,0,0,0.05)',
            }}
            styles={{ header: { background: '#f5f6fb', borderBottom: '1px solid #eef0f6' } }}
          >
            <Table
              dataSource={mutengItems}
              rowKey={(_, idx) => `m-${idx}`}
              pagination={false}
              size="small"
              bordered
              columns={[
                {
                  title: '序号',
                  width: 108,
                  render: (_: any, __: any, idx: number) => {
                    const indicatorType = mutengItems[idx].indicator_type === 'kpi' ? 'kpi' : 'okr'
                    return (
                      <Space direction="vertical" size={4} style={{ width: '100%' }}>
                        <span style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-xs)' }}>#{idx + 1}</span>
                        <Select
                          size="small"
                          value={indicatorType}
                          style={{ width: 78 }}
                          options={[
                            { value: 'okr', label: 'OKR' },
                            { value: 'kpi', label: 'KPI' },
                          ]}
                          onChange={(value) => updateMutengItem(idx, { indicator_type: value })}
                        />
                      </Space>
                    )
                  }
                },
                {
                  title: '目标/关键职责事项',
                  width: 220,
                  render: (_: any, __: any, idx: number) => (
                    <Input
                      value={mutengItems[idx].name}
                      onChange={e => updateMutengItem(idx, { name: e.target.value })}
                      placeholder="输入目标/关键职责事项"
                    />
                  )
                },
                {
                  title: '权重',
                  width: 100,
                  render: (_: any, __: any, idx: number) => (
                    <InputNumber min={10} max={100} step={5} value={mutengItems[idx].weight} onChange={value => updateMutengItem(idx, { weight: value || 0 })} addonAfter="%" style={{ width: '100%' }} />
                  )
                },
                {
                  title: (
                    <Space direction="vertical" size={0}>
                      <span>目标/关键职责事项说明</span>
                      <span style={{ color: 'var(--color-error)', fontSize: 'var(--font-size-xs)', fontWeight: 'var(--font-weight-medium)' }}>（若是 OKR 按 KR 填写，若是 KPI 按指标填写）</span>
                    </Space>
                  ),
                  width: 330,
                  render: (_: any, __: any, idx: number) => (
                    <TextArea
                      rows={3}
                      value={mutengItems[idx].description}
                      onChange={e => updateMutengItem(idx, { description: e.target.value })}
                      placeholder={mutengItems[idx].indicator_type === 'kpi' ? '填写指标定义、计算口径或公式' : '填写 KR1、KR2 等关键结果'}
                    />
                  )
                },
                {
                  title: '目标/关键职责事项的完成情况',
                  render: (_: any, __: any, idx: number) => (
                    <TextArea
                      rows={3}
                      value={mutengItems[idx].target_value}
                      onChange={e => updateMutengItem(idx, { target_value: e.target.value })}
                      placeholder="描述完成结果、交付物或完成标准"
                    />
                  )
                },
                {
                  title: '',
                  width: 48,
                  render: (_: any, __: any, idx: number) => (
                    <Button type="text" danger icon={<DeleteOutlined />} disabled={mutengItems.length <= 1} onClick={() => setMutengItems(prev => prev.filter((_, i) => i !== idx))} />
                  )
                }
              ]}
            />
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginTop: 12 }}>
              <Button
                type="dashed"
                icon={<PlusOutlined />}
                onClick={() => setMutengItems(prev => [...prev, newMutengItem('okr', 10)])}
                style={{
                  height: 40,
                  color: '#15803d',
                  borderColor: '#86efac',
                  fontSize: 'var(--font-size-sm)',
                }}
              >
                添加 OKR
              </Button>
              <Button
                type="dashed"
                icon={<PlusOutlined />}
                onClick={() => setMutengItems(prev => [...prev, newMutengItem('kpi', 10)])}
                style={{
                  height: 40,
                  color: 'var(--color-primary)',
                  borderColor: '#a5b4fc',
                  fontSize: 'var(--font-size-sm)',
                }}
              >
                添加 KPI
              </Button>
            </div>
          </Card>
        ) : (
          <>
        <Card
          size="small"
          title={
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <div style={{ width: 4, height: 18, borderRadius: 'var(--radius-xs)', background: 'linear-gradient(180deg, var(--color-primary), #6366f1)' }} />
              <span style={{ fontWeight: 'var(--font-weight-bold)', fontSize: 'var(--font-size-base)', color: 'var(--color-text-heading)' }}>量化指标</span>
              <span style={{
                background: 'linear-gradient(135deg, var(--color-primary-bg), #dbeafe)',
                color: 'var(--color-primary)',
                fontSize: 'var(--font-size-xs)',
                fontWeight: 'var(--font-weight-bold)',
                padding: '4px 12px',
                borderRadius: 'var(--radius-xl)',
                border: '1px solid #c7d2fe',
                letterSpacing: 0.3,
              }}>
                权重合计 70%
              </span>
            </div>
          }
          style={{
            marginTop: 10,
            borderRadius: 'var(--radius-lg)',
            border: '1px solid #e0e3ed',
            boxShadow: '0 1px 6px rgba(0,0,0,0.05)',
          }}
          styles={{ header: { background: '#f5f6fb', borderBottom: '1px solid #eef0f6' } }}
        >
          <Table
            dataSource={quantItems}
            rowKey={(_, idx) => `q-${idx}`}
            pagination={false}
            size="small"
            bordered
            columns={[
              {
                title: '指标名称',
                width: 200,
                render: (_: any, __: any, idx: number) => (
                  <AutoComplete
                    value={quantItems[idx].name}
                    options={getSearchOptions(quantSearchResults[idx] || [])}
                    onSearch={(val) => searchIndicators(val, setQuantSearchResults, idx, 'quantitative')}
                    onChange={(val) => updateQuantItem(idx, { name: val })}
                    onSelect={(val) => handleIndicatorSelect(val, idx, quantSearchResults[idx] || [], setQuantItems, quantItems)}
                    placeholder="输入关键词搜索指标"
                    style={{ width: '100%' }}
                  />
                )
              },
              {
                title: '指标定义及口径说明',
                width: 260,
                render: (_: any, __: any, idx: number) => (
                  <TextArea rows={2} value={quantItems[idx].description} onChange={e => updateQuantItem(idx, { description: e.target.value })} placeholder="明确指标范围和计算公式" />
                )
              },
              {
                title: '权重',
                width: 90,
                render: (_: any, __: any, idx: number) => (
                  <InputNumber min={10} max={100} step={5} value={quantItems[idx].weight} onChange={value => updateQuantItem(idx, { weight: value || 0 })} addonAfter="%" style={{ width: '100%' }} />
                )
              },
              {
                title: '红线值',
                width: 110,
                render: (_: any, __: any, idx: number) => (
                  <Input value={quantItems[idx].red_line_value} onChange={e => updateQuantItem(idx, { red_line_value: e.target.value })} />
                )
              },
              {
                title: '目标值',
                width: 110,
                render: (_: any, __: any, idx: number) => (
                  <Input value={quantItems[idx].target_value} onChange={e => updateQuantItem(idx, { target_value: e.target.value })} />
                )
              },
              {
                title: '挑战值',
                width: 110,
                render: (_: any, __: any, idx: number) => (
                  <Input value={quantItems[idx].challenge_value} onChange={e => updateQuantItem(idx, { challenge_value: e.target.value })} />
                )
              },
              {
                title: '考核标准',
                width: 240,
                render: (_: any, __: any, idx: number) => (
                  <TextArea rows={2} value={quantItems[idx].scoring_rule} onChange={e => updateQuantItem(idx, { scoring_rule: e.target.value })} placeholder="定量按区间/上限设置" />
                )
              },
              {
                title: '',
                width: 48,
                render: (_: any, __: any, idx: number) => (
                  <Button type="text" danger icon={<DeleteOutlined />} disabled={quantItems.length <= 1} onClick={() => setQuantItems(prev => prev.filter((_, i) => i !== idx))} />
                )
              }
            ]}
          />
          <Button
            type="dashed"
            icon={<PlusOutlined />}
            onClick={() => setQuantItems(prev => [...prev, newQuantItem()])}
            style={{
              marginTop: 12,
              width: '100%',
              height: 40,
              color: 'var(--color-primary)',
              borderColor: '#a5b4fc',
              fontSize: 'var(--font-size-sm)',
            }}
          >
            添加量化指标
          </Button>
        </Card>

        <Card
          size="small"
          title={
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <div style={{ width: 4, height: 18, borderRadius: 'var(--radius-xs)', background: 'linear-gradient(180deg, #15803d, #22c55e)' }} />
              <span style={{ fontWeight: 'var(--font-weight-bold)', fontSize: 'var(--font-size-base)', color: '#14532d' }}>关键行动</span>
              <span style={{
                background: 'linear-gradient(135deg, #f0fdf4, #dcfce7)',
                color: '#15803d',
                fontSize: 'var(--font-size-xs)',
                fontWeight: 'var(--font-weight-bold)',
                padding: '4px 12px',
                borderRadius: 'var(--radius-xl)',
                border: '1px solid #bbf7d0',
                letterSpacing: 0.3,
              }}>
                权重合计 30%
              </span>
            </div>
          }
          style={{
            marginTop: 16,
            borderRadius: 'var(--radius-lg)',
            border: '1px solid #e0e3ed',
            boxShadow: '0 1px 6px rgba(0,0,0,0.05)',
          }}
          styles={{ header: { background: '#f5f6fb', borderBottom: '1px solid #eef0f6' } }}
        >
          <Table
            dataSource={actionItems}
            rowKey={(_, idx) => `a-${idx}`}
            pagination={false}
            size="small"
            bordered
            columns={[
              {
                title: '重点计划',
                width: 200,
                render: (_: any, __: any, idx: number) => (
                  <AutoComplete
                    value={actionItems[idx].name}
                    options={getSearchOptions(actionSearchResults[idx] || [])}
                    onSearch={(val) => searchIndicators(val, setActionSearchResults, idx, 'key_action')}
                    onChange={(val) => updateActionItem(idx, { name: val })}
                    onSelect={(val) => handleIndicatorSelect(val, idx, actionSearchResults[idx] || [], setActionItems, actionItems)}
                    placeholder="输入关键词搜索指标"
                    style={{ width: '100%' }}
                  />
                )
              },
              {
                title: '指标定义及口径说明',
                width: 320,
                render: (_: any, __: any, idx: number) => (
                  <TextArea rows={2} value={actionItems[idx].description} onChange={e => updateActionItem(idx, { description: e.target.value })} placeholder="明确行动范围和完成口径" />
                )
              },
              {
                title: '权重',
                width: 100,
                render: (_: any, __: any, idx: number) => (
                  <InputNumber min={10} max={100} step={5} value={actionItems[idx].weight} onChange={value => updateActionItem(idx, { weight: value || 0 })} addonAfter="%" style={{ width: '100%' }} />
                )
              },
              {
                title: '定性目标',
                render: (_: any, __: any, idx: number) => (
                  <TextArea rows={3} value={actionItems[idx].target_value} onChange={e => updateActionItem(idx, { target_value: e.target.value })} placeholder="描述关键结果、交付物或完成标准" />
                )
              },
              {
                title: '',
                width: 48,
                render: (_: any, __: any, idx: number) => (
                  <Button type="text" danger icon={<DeleteOutlined />} disabled={actionItems.length <= 1} onClick={() => setActionItems(prev => prev.filter((_, i) => i !== idx))} />
                )
              }
            ]}
          />
          <Button
            type="dashed"
            icon={<PlusOutlined />}
            onClick={() => setActionItems(prev => [...prev, newActionItem()])}
            style={{
              marginTop: 12,
              width: '100%',
              height: 40,
              color: '#15803d',
              borderColor: '#86efac',
              fontSize: 'var(--font-size-sm)',
            }}
          >
            添加关键行动
          </Button>
        </Card>
          </>
        )}
      </Modal>

      <Modal
        title={<span style={{ fontWeight: 'var(--font-weight-bold)', fontSize: 17, color: 'var(--color-text-title)' }}>继承指标库</span>}
        open={inheritOpen}
        onCancel={() => { setInheritOpen(false); inheritForm.resetFields() }}
        onOk={() => { if (guardIndicatorManage()) inheritForm.submit() }}
        confirmLoading={inheriting}
        okButtonProps={{ disabled: !canManageIndicator }}
        width={480}
        styles={{ mask: { backdropFilter: 'blur(4px)' } }}
      >
        <Form form={inheritForm} layout="vertical" onFinish={handleInherit} style={{ marginTop: 16 }}>
          <Form.Item
            name="parent_library_id"
            label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>源指标库</span>}
            rules={[{ required: true, message: '请选择要继承的指标库' }]}
          >
            <Select
              showSearch
              placeholder="请选择要继承的指标库"
              optionFilterProp="label"
              options={libraries.filter(l => l.status === 'active').map(l => ({
                label: `${l.name}（${getTemplateDisplayName(templates.find(template => String(template.id) === String(l.template_id)))} / ${l.department_name}）`,
                value: l.id,
              }))}
            />
          </Form.Item>
          <Form.Item
            name="target_department_id"
            label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>目标部门</span>}
            rules={[{ required: true, message: '请选择目标部门' }]}
          >
            <Select
              showSearch
              placeholder="请选择目标部门"
              loading={departmentsLoading}
              optionFilterProp="label"
              options={departments.map(d => ({
                label: d.name,
                value: d.department_id,
              }))}
            />
          </Form.Item>
          <Form.Item
            name="name"
            label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>新指标库名称</span>}
            extra={<span style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)' }}>留空则沿用源指标库名称</span>}
          >
            <Input placeholder="不填则与源指标库同名" />
          </Form.Item>
          <Form.Item
            name="description"
            label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>描述</span>}
            extra={<span style={{ color: 'var(--color-text-tertiary)', fontSize: 'var(--font-size-xs)' }}>留空则沿用源指标库描述</span>}
          >
            <TextArea rows={2} placeholder="可补充本部门的差异说明" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={<span style={{ fontWeight: 'var(--font-weight-bold)', fontSize: 17, color: 'var(--color-text-title)' }}>编辑指标项</span>}
        open={editItemOpen}
        onCancel={() => { setEditItemOpen(false); setEditingItem(null) }}
        onOk={() => { if (guardIndicatorManage()) editItemForm.submit() }}
        confirmLoading={editingItemLoading}
        okButtonProps={{ disabled: !canManageIndicator }}
        width={560}
        styles={{ mask: { backdropFilter: 'blur(4px)' } }}
      >
        <Form form={editItemForm} layout="vertical" onFinish={handleEditItemSubmit} style={{ marginTop: 16 }}>
          {isEditingMutengItem && (
            <Form.Item name="indicator_type" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>类型</span>}>
              <Select
                disabled
                options={[
                  { value: 'okr', label: 'OKR' },
                  { value: 'kpi', label: 'KPI' },
                ]}
              />
            </Form.Item>
          )}
          <Form.Item name="name" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>{isEditingMutengItem ? '目标/关键职责事项' : '指标名称'}</span>} rules={[{ required: true, message: isEditingMutengItem ? '请输入目标/关键职责事项' : '请输入指标名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="description" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>{isEditingMutengItem ? '目标/关键职责事项说明' : '指标定义及口径说明'}</span>}>
            <TextArea rows={2} />
          </Form.Item>
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item name="weight" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>权重</span>}>
                <InputNumber min={isEditingMutengItem ? 10 : 5} max={100} step={5} addonAfter="%" style={{ width: '100%' }} />
              </Form.Item>
            </Col>
          </Row>
          {isEditingMutengItem && (
            <Form.Item name="target_value" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>目标/关键职责事项的完成情况</span>}>
              <TextArea rows={3} />
            </Form.Item>
          )}
          {!isEditingMutengItem && editingItem?.section_type === 'quantitative' && (
            <>
              <Row gutter={16}>
                <Col span={8}>
                  <Form.Item name="red_line_value" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>红线值</span>}>
                    <Input />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item name="target_value" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>目标值</span>}>
                    <Input />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item name="challenge_value" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>挑战值</span>}>
                    <Input />
                  </Form.Item>
                </Col>
              </Row>
              <Form.Item name="scoring_rule" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>考核标准</span>}>
                <TextArea rows={2} />
              </Form.Item>
            </>
          )}
          {!isEditingMutengItem && editingItem?.section_type === 'key_action' && (
            <Form.Item name="target_value" label={<span style={{ fontWeight: 'var(--font-weight-semibold)', color: 'var(--color-text)' }}>定性目标</span>}>
              <TextArea rows={2} />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </PageContainer>
  )
}
