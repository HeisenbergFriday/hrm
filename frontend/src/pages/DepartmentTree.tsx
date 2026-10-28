import React, { useEffect, useMemo, useState } from 'react'
import { Alert, Avatar, Button, Col, Empty, Input, List, Row, Space, Spin, Statistic, Tabs, Tree, Tooltip, Typography, message } from 'antd'
import { ApartmentOutlined, DownOutlined, ReloadOutlined, SyncOutlined, TeamOutlined, UpOutlined, UserOutlined } from '@ant-design/icons'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import StatusTag from '../components/StatusTag'
import type { DataNode } from 'antd/es/tree'
import { useNavigate } from 'react-router-dom'
import { orgAPI } from '../services/api'
import { formatDateTime } from '../utils/format'
import {
  canSyncOrgData,
  confirmOrgSync,
  missingOrgSyncPermissionTip,
} from '../utils/orgSyncAction'
import './organization.css'

const { Title, Text } = Typography

interface ScopeInfo {
  mode: string
  department_names?: string[]
}

interface DepartmentTreeNode {
  id: string
  name: string
  parent_id: string
  headcount: number
  active_count: number
  inactive_count: number
  direct_headcount: number
  direct_active_count: number
  virtual_kind?: 'organization' | 'unassigned'
  children?: DepartmentTreeNode[]
}

interface DepartmentOverviewSummary {
  active_employees: number
  probation_employee_count: number
  planned_regularization_count: number
}

interface DepartmentOverview {
  summary: DepartmentOverviewSummary
}

interface EmployeeItem {
  id: number
  user_id: string
  name: string
  email: string
  mobile: string
  department_id: string
  position: string
  avatar: string
  status: string
}

interface DepartmentChangeLog {
  id: number
  department_id: string
  department_name: string
  change_type: string
  field_name: string
  old_value: string
  new_value: string
  source: string
  changed_at?: string
  created_at?: string
}

const departmentEmployeePageSize = 6

const findTreeNodeByID = (nodes: DepartmentTreeNode[], targetID: string): DepartmentTreeNode | null => {
  for (const node of nodes) {
    if (node.id === targetID) {
      return node
    }
    if (node.children?.length) {
      const matched = findTreeNodeByID(node.children, targetID)
      if (matched) {
        return matched
      }
    }
  }
  return null
}

const DepartmentTree: React.FC = () => {
  const navigate = useNavigate()
  const [tree, setTree] = useState<DepartmentTreeNode[]>([])
  const [scope, setScope] = useState<ScopeInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [syncing, setSyncing] = useState(false)
  const [selectedNode, setSelectedNode] = useState<DepartmentTreeNode | null>(null)
  const [departmentEmployees, setDepartmentEmployees] = useState<EmployeeItem[]>([])
  const [departmentEmployeeTotal, setDepartmentEmployeeTotal] = useState(0)
  const [departmentEmployeePage, setDepartmentEmployeePage] = useState(1)
  const [membersLoading, setMembersLoading] = useState(false)
  const [departmentHistory, setDepartmentHistory] = useState<DepartmentChangeLog[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)
  const [departmentOverview, setDepartmentOverview] = useState<DepartmentOverview | null>(null)
  const [overviewLoading, setOverviewLoading] = useState(false)
  const [treeSearch, setTreeSearch] = useState('')
  const [expandedKeys, setExpandedKeys] = useState<React.Key[]>([])

  const loadTree = async (showLoading = true) => {
    if (showLoading) {
      setLoading(true)
    }
    try {
      const response = await orgAPI.getDepartmentTree()
      const nextTree = response.data.tree || []
      setTree(nextTree)
      setScope(response.data.scope || null)
      setExpandedKeys((current) => current.length ? current : nextTree.map((node: DepartmentTreeNode) => node.id))
      const nextSelectedID = selectedNode?.id || nextTree[0]?.id
      if (nextSelectedID) {
        setSelectedNode(findTreeNodeByID(nextTree, nextSelectedID) || nextTree[0] || null)
      } else {
        setSelectedNode(null)
      }
    } catch (error) {
      message.error('获取组织架构失败')
    } finally {
      if (showLoading) {
        setLoading(false)
      }
    }
  }

  useEffect(() => {
    void loadTree()
  }, [])

  const loadDepartmentMembers = async (node: DepartmentTreeNode, pageNumber: number) => {
    if (!node.id || node.virtual_kind === 'unassigned') {
      setDepartmentEmployees([])
      setDepartmentEmployeeTotal(0)
      return
    }

    setMembersLoading(true)
    try {
      const response = await orgAPI.getEmployees({
        page: pageNumber,
        page_size: departmentEmployeePageSize,
        department_id: node.virtual_kind === 'organization' ? undefined : node.id,
      })
      setDepartmentEmployees(response.data.items || [])
      setDepartmentEmployeeTotal(response.data.total || 0)
    } catch (error) {
      message.error('获取部门员工失败')
    } finally {
      setMembersLoading(false)
    }
  }

  useEffect(() => {
    if (!selectedNode?.id) {
      setDepartmentEmployees([])
      setDepartmentEmployeeTotal(0)
      return
    }

    void loadDepartmentMembers(selectedNode, departmentEmployeePage)
  }, [selectedNode, departmentEmployeePage])

  const loadDepartmentHistory = async (departmentID: string) => {
    if (!departmentID) {
      setDepartmentHistory([])
      return
    }

    setHistoryLoading(true)
    try {
      const response = await orgAPI.getDepartmentHistory(departmentID, { limit: 50 })
      setDepartmentHistory(response.data.items || [])
    } catch (error) {
      message.error('获取部门变更历史失败')
    } finally {
      setHistoryLoading(false)
    }
  }

  useEffect(() => {
    if (!selectedNode?.id || selectedNode.virtual_kind) {
      setDepartmentHistory([])
      return
    }

    void loadDepartmentHistory(selectedNode.id)
  }, [selectedNode])

  const loadDepartmentOverview = async (node: DepartmentTreeNode) => {
    if (!node.id || node.virtual_kind === 'unassigned') {
      setDepartmentOverview(null)
      return
    }

    setOverviewLoading(true)
    try {
      const response = await orgAPI.getOverview(node.virtual_kind === 'organization' ? undefined : { department_id: node.id })
      setDepartmentOverview(response.data.overview || null)
    } catch (error) {
      message.error('获取部门统计失败')
      setDepartmentOverview(null)
    } finally {
      setOverviewLoading(false)
    }
  }

  useEffect(() => {
    if (!selectedNode?.id) {
      setDepartmentOverview(null)
      return
    }

    void loadDepartmentOverview(selectedNode)
  }, [selectedNode])

  const scopeLabel = useMemo(() => {
    if (!scope) {
      return '正在加载数据范围...'
    }
    if (scope.mode === 'all') {
      return '当前范围：全组织'
    }
    if (scope.department_names?.length) {
      return `当前范围：${scope.department_names.join(' / ')}`
    }
    return '当前范围：部门范围'
  }, [scope])

  const treeData = useMemo<DataNode[]>(() => {
    const keyword = treeSearch.trim().toLowerCase()
    const filterNodes = (nodes: DepartmentTreeNode[]): DepartmentTreeNode[] => nodes.flatMap((node) => {
      const children = filterNodes(node.children || [])
      if (!keyword || node.name.toLowerCase().includes(keyword) || children.length > 0) {
        return [{ ...node, children }]
      }
      return []
    })
    const mapNodes = (nodes: DepartmentTreeNode[]): DataNode[] =>
      nodes.map((node) => ({
        key: node.id,
        selectable: node.virtual_kind !== 'unassigned',
        title: (
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, width: '100%', padding: '4px 0' }}>
            <span style={{ fontWeight: node.children?.length ? 500 : 400 }}>{node.name}</span>
            <Tooltip title={
              node.direct_active_count !== node.active_count
                ? `含下级部门去重：${node.active_count} 人 | 直属本部门：${node.direct_active_count} 人`
                : `直属本部门：${node.active_count} 人`
            }>
              <span style={{
                color: 'var(--color-text-tertiary)',
                fontSize: 'var(--font-size-xs)',
                background: 'var(--color-bg-light)',
                padding: '2px 8px',
                borderRadius: 'var(--radius-sm)',
                cursor: 'default'
              }}>
                {node.active_count} 人{node.direct_active_count !== node.active_count ? ` (直属 ${node.direct_active_count})` : ''}
              </span>
            </Tooltip>
          </div>
        ),
        children: mapNodes(node.children || []),
      }))
    return mapNodes(filterNodes(tree))
  }, [tree, treeSearch])

  const searchExpandedKeys = useMemo(() => {
    const keyword = treeSearch.trim().toLowerCase()
    if (!keyword) return expandedKeys
    const result: string[] = []
    const collect = (nodes: DepartmentTreeNode[]) => {
      nodes.forEach((node) => {
        const hasMatchingChild = (node.children || []).some((child) => child.name.toLowerCase().includes(keyword))
        if (hasMatchingChild || (node.children || []).some((child) => {
          const descendants = [child, ...(child.children || [])]
          return descendants.some((item) => item.name.toLowerCase().includes(keyword))
        })) result.push(node.id)
        collect(node.children || [])
      })
    }
    collect(tree)
    return result
  }, [tree, treeSearch, expandedKeys])

  const expandAll = () => {
    const keys: string[] = []
    const collect = (nodes: DepartmentTreeNode[]) => nodes.forEach((node) => {
      if (node.children?.length) {
        keys.push(node.id)
        collect(node.children)
      }
    })
    collect(tree)
    setExpandedKeys(keys)
  }

  const collapseAll = () => setExpandedKeys([])

  const nodeMap = useMemo(() => {
    const result = new Map<string, DepartmentTreeNode>()
    const walk = (nodes: DepartmentTreeNode[]) => {
      nodes.forEach((node) => {
        result.set(node.id, node)
        if (node.children?.length) {
          walk(node.children)
        }
      })
    }
    walk(tree)
    return result
  }, [tree])

  const canSync = canSyncOrgData()

  const handleSync = () => {
    if (!canSync) return
    confirmOrgSync({
      onStart: () => setSyncing(true),
      onSettled: () => setSyncing(false),
      onCompleted: async () => {
        await loadTree(false)
      },
    })
  }

  const employeeStatusTag = (value?: string) => (
    <StatusTag color={value === 'active' ? 'success' : 'default'}>
      {value === 'active' ? '在职' : value === 'inactive' ? '离职/停用' : value || '未设置'}
    </StatusTag>
  )

  const statBoxStyle = {
    padding: '16px',
    borderRadius: 'var(--radius-lg)',
    background: 'linear-gradient(135deg, var(--color-bg-light) 0%, var(--color-bg-page) 100%)',
    border: '1px solid var(--color-border-light)',
    transition: 'all 0.2s ease',
  }

  const fieldLabel = (fieldName: string) => {
    const labels: Record<string, string> = {
      department: '部门',
      name: '部门名称',
      parent_id: '上级部门',
      order: '排序',
    }
    return labels[fieldName] || fieldName
  }

  const changeTypeTag = (value: string) => {
    const labels: Record<string, string> = {
      created: '新增',
      updated: '更新',
    }
    const colors: Record<string, string> = {
      created: 'green',
      updated: 'blue',
    }
    return <StatusTag color={colors[value] || 'default'}>{labels[value] || value}</StatusTag>
  }

  return (
      <PageContainer
        className="organization-tree-page"
      title="组织架构"
      subtitle={scopeLabel}
      extra={
        <div className="organization-tree-page-actions">
          <Button icon={<ReloadOutlined />} onClick={() => void loadTree()} loading={loading}>
            刷新
          </Button>
          <Tooltip title={canSync ? undefined : missingOrgSyncPermissionTip}>
            <Button type="primary" icon={<SyncOutlined />} onClick={handleSync} loading={syncing} disabled={!canSync}>
              同步组织数据
            </Button>
          </Tooltip>
        </div>
      }
    >

      {loading ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}>
          <Spin size="large" />
        </div>
      ) : tree.length === 0 ? (
        <div className="organization-tree-empty-state">
          <Empty description="暂无组织架构数据">
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => void loadTree()} loading={loading}>重新加载</Button>
              {canSync ? <Button type="primary" icon={<SyncOutlined />} onClick={handleSync} loading={syncing}>同步组织数据</Button> : null}
            </Space>
          </Empty>
        </div>
      ) : (
        <Row gutter={[16, 16]} className="organization-tree-layout">
          <Col xs={24} lg={14}>
            <PageCard title={<div className="organization-tree-card-title"><span>组织树</span><Text type="secondary">{treeData.length} 个节点</Text></div>}>
              <div className="organization-tree-toolbar">
                <Input.Search
                  allowClear
                  value={treeSearch}
                  onChange={(event) => setTreeSearch(event.target.value)}
                  placeholder="搜索部门名称，支持定位下级部门"
                  aria-label="搜索部门名称"
                  className="organization-tree-search"
                />
                <Space size={4} className="organization-tree-toolbar-actions">
                  <Button size="small" icon={<DownOutlined />} onClick={expandAll}>展开全部</Button>
                  <Button size="small" icon={<UpOutlined />} onClick={collapseAll}>收起全部</Button>
                </Space>
              </div>
              <Alert
                className="organization-tree-help"
                style={{ marginBottom: 16 }}
                type="info"
                showIcon
                message="人数口径说明"
                description="树中显示的数字为「含下级部门去重」的在职人数；括号内「直属」为本部门直接归属在职人数。多部门员工在各部门各计一次，父部门汇总时按员工 ID 去重。鼠标悬停人数标签可查看详细口径。"
              />
              <div className="organization-tree-scroll">
              {treeData.length > 0 ? <Tree
                expandedKeys={searchExpandedKeys}
                onExpand={(keys) => setExpandedKeys(keys)}
                selectedKeys={selectedNode ? [selectedNode.id] : []}
                treeData={treeData}
                onSelect={(keys) => {
                  const nextKey = String(keys[0] || '')
                  if (!nextKey) return
                  setDepartmentEmployeePage(1)
                  setSelectedNode(nodeMap.get(nextKey) || null)
                }}
              /> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的部门" />}
              </div>
            </PageCard>
          </Col>
          <Col xs={24} lg={10}>
            <PageCard title={<div className="organization-detail-card-title"><span>{selectedNode?.virtual_kind === 'organization' ? '组织基础统计' : '部门基础统计'}</span>{selectedNode ? <StatusTag color="processing">已选择</StatusTag> : null}</div>}>
              {selectedNode ? (
                <>
                  <div className="organization-selected-department">
                    <Title level={4} className="organization-selected-title">
                      {selectedNode.name}
                    </Title>
                    <Text type="secondary" className="organization-selected-hint">已加载该部门的统计、员工和变更记录</Text>
                    <Space size={[12, 8]} wrap className="organization-selected-meta">
                      <Tooltip title={selectedNode.virtual_kind === 'organization' || !selectedNode.parent_id || selectedNode.parent_id === '0' || selectedNode.parent_id === '1'
                        ? '根部门人数 = 全组织去重员工总数（含在职+停用，已排除软删除和管理员）'
                        : '含下级部门去重汇总人数（在职+停用）'
                      }>
                        <Text type="secondary">
                          <TeamOutlined style={{ marginRight: 4 }} />
                          总人数 {selectedNode.headcount}
                        </Text>
                      </Tooltip>
                      <Text type="secondary">
                        直属 {selectedNode.direct_headcount}
                      </Text>
                      {selectedNode.children?.length ? (
                        <Text type="secondary">
                          <ApartmentOutlined style={{ marginRight: 4 }} />
                          下级部门 {selectedNode.children.length} 个
                        </Text>
                      ) : null}
                    </Space>
                  </div>
                  {overviewLoading ? (
                    <div style={{ display: 'flex', justifyContent: 'center', padding: '24px 0' }}>
                      <Spin size="small" />
                    </div>
                  ) : (
                    <>
                      <div className="organization-stat-caption">
                        <Tooltip title="主数字为含下级部门去重的在职人数；括号内为直属本部门人数。多部门员工在父部门汇总时按员工 ID 去重。">
                          <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>统计口径：含下级部门汇总数据</Text>
                        </Tooltip>
                      </div>
                      <Row gutter={[12, 12]} className="organization-stat-grid">
                        <Col span={8}>
                          <div className="organization-stat-box" style={statBoxStyle}>
                            <Statistic
                              title={<Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>在职人数</Text>}
                              value={departmentOverview?.summary.active_employees ?? selectedNode.active_count}
                              valueStyle={{ fontSize: 24, fontWeight: 600, color: 'var(--color-success)' }}
                              suffix={selectedNode.direct_active_count !== selectedNode.active_count ? (
                                <Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>(直属 {selectedNode.direct_active_count})</Text>
                              ) : null}
                            />
                          </div>
                        </Col>
                        <Col span={8}>
                          <div className="organization-stat-box" style={statBoxStyle}>
                            <Statistic
                              title={<Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>试用期</Text>}
                              value={departmentOverview?.summary.probation_employee_count ?? 0}
                              valueStyle={{ fontSize: 24, fontWeight: 600, color: 'var(--color-warning)' }}
                            />
                          </div>
                        </Col>
                        <Col span={8}>
                          <div className="organization-stat-box" style={statBoxStyle}>
                            <Statistic
                              title={<Text type="secondary" style={{ fontSize: 'var(--font-size-xs)' }}>转正预警</Text>}
                              value={departmentOverview?.summary.planned_regularization_count ?? 0}
                              valueStyle={{ fontSize: 24, fontWeight: 600, color: departmentOverview?.summary.planned_regularization_count ? 'var(--color-error)' : 'var(--color-text-tertiary)' }}
                            />
                          </div>
                        </Col>
                      </Row>
                    </>
                  )}
                  <Tabs
                    className="organization-detail-tabs"
                    items={[
                      {
                        key: 'members',
                        label: '员工列表',
                        children: (
                          <>
                            <div className="organization-members-meta">
                              <Text type="secondary">含下级部门 {departmentEmployeeTotal} 人</Text>
                            </div>
                            <List
                              loading={membersLoading}
                              locale={{ emptyText: '当前部门暂无员工' }}
                              dataSource={departmentEmployees}
                              pagination={
                                departmentEmployeeTotal > departmentEmployeePageSize
                                  ? {
                                      current: departmentEmployeePage,
                                      pageSize: departmentEmployeePageSize,
                                      total: departmentEmployeeTotal,
                                      size: 'small',
                                      showSizeChanger: false,
                                      onChange: (page) => setDepartmentEmployeePage(page),
                                    }
                                  : false
                              }
                              renderItem={(item) => (
                                <List.Item
                                  style={{ padding: '12px 0' }}
                                  actions={[
                                    <StatusTag key="status" color={item.status === 'active' ? 'success' : 'default'}>
                                      {item.status === 'active' ? '在职' : '离职/停用'}
                                    </StatusTag>
                                  ]}
                                >
                                  <List.Item.Meta
                                    avatar={
                                      <Avatar
                                        src={item.avatar}
                                        icon={<UserOutlined />}
                                        style={{ backgroundColor: item.avatar ? 'transparent' : 'var(--color-primary-bg)' }}
                                      />
                                    }
                                    title={
                                      <Button
                                        type="link"
                                        style={{ padding: 0, height: 'auto', fontWeight: 500 }}
                                        onClick={() => navigate(`/employees/${item.id}`)}
                                      >
                                        {item.name}
                                      </Button>
                                    }
                                    description={
                                      <Space size={8} split={<span style={{ color: 'var(--color-border)' }}>·</span>}>
                                        <Text type="secondary">{item.position || '未设置岗位'}</Text>
                                        <Text type="secondary">{nodeMap.get(item.department_id)?.name || item.department_id || '未分配部门'}</Text>
                                      </Space>
                                    }
                                  />
                                </List.Item>
                              )}
                            />
                          </>
                        ),
                      },
                      {
                        key: 'history',
                        label: '变更历史',
                        children: (
                          <List
                            loading={historyLoading}
                            locale={{ emptyText: '暂无变更历史' }}
                            dataSource={departmentHistory}
                            renderItem={(item) => (
                              <List.Item>
                                  <div className="organization-history-item">
                                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center' }}>
                                    <Space size={4} wrap>
                                      {changeTypeTag(item.change_type)}
                                      <Text strong>{fieldLabel(item.field_name)}</Text>
                                    </Space>
                                    <Text type="secondary">{formatDateTime(item.changed_at || item.created_at)}</Text>
                                  </div>
                                  <div className="organization-history-change">
                                    <Text type="secondary">{item.old_value || '-'}</Text>
                                    <Text type="secondary"> {'->'} </Text>
                                    <Text>{item.new_value || '-'}</Text>
                                  </div>
                                  <div className="organization-history-source">
                                    来源：{item.source || 'system'}
                                  </div>
                                </div>
                              </List.Item>
                            )}
                          />
                        ),
                      },
                    ]}
                  />
                </>
              ) : (
                <Empty description="请选择一个部门" />
              )}
            </PageCard>
          </Col>
        </Row>
      )}
    </PageContainer>
  )
}

export default DepartmentTree
