import React, { useEffect, useMemo, useState } from 'react'
import { Alert, Avatar, Button, Card, Col, Empty, List, Row, Space, Spin, Statistic, Tabs, Tag, Tree, Typography, message } from 'antd'
import { ReloadOutlined, SyncOutlined, UserOutlined } from '@ant-design/icons'
import type { DataNode } from 'antd/es/tree'
import { useNavigate } from 'react-router-dom'
import { orgAPI } from '../services/api'

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

  const loadTree = async (showLoading = true) => {
    if (showLoading) {
      setLoading(true)
    }
    try {
      const response = await orgAPI.getDepartmentTree()
      const nextTree = response.data.tree || []
      setTree(nextTree)
      setScope(response.data.scope || null)
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

  const loadDepartmentMembers = async (departmentID: string, pageNumber: number) => {
    if (!departmentID) {
      setDepartmentEmployees([])
      setDepartmentEmployeeTotal(0)
      return
    }

    setMembersLoading(true)
    try {
      const response = await orgAPI.getEmployees({
        page: pageNumber,
        page_size: departmentEmployeePageSize,
        department_id: departmentID,
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

    void loadDepartmentMembers(selectedNode.id, departmentEmployeePage)
  }, [selectedNode?.id, departmentEmployeePage])

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
    if (!selectedNode?.id) {
      setDepartmentHistory([])
      return
    }

    void loadDepartmentHistory(selectedNode.id)
  }, [selectedNode?.id])

  const loadDepartmentOverview = async (departmentID: string) => {
    if (!departmentID) {
      setDepartmentOverview(null)
      return
    }

    setOverviewLoading(true)
    try {
      const response = await orgAPI.getOverview({ department_id: departmentID })
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

    void loadDepartmentOverview(selectedNode.id)
  }, [selectedNode?.id])

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
    const mapNodes = (nodes: DepartmentTreeNode[]): DataNode[] =>
      nodes.map((node) => ({
        key: node.id,
        title: (
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, width: '100%' }}>
            <span>{node.name}</span>
            <span style={{ color: '#8c8c8c', fontSize: 12 }}>
              在职 {node.active_count} / 直属在职 {node.direct_active_count}
            </span>
          </div>
        ),
        children: mapNodes(node.children || []),
      }))
    return mapNodes(tree)
  }, [tree])

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

  const handleSync = async () => {
    setSyncing(true)
    try {
      await orgAPI.syncOrg()
      message.success('组织数据同步成功')
      await loadTree(false)
    } catch (error) {
      message.error('组织数据同步失败')
    } finally {
      setSyncing(false)
    }
  }

  const employeeStatusTag = (value?: string) => (
    <Tag color={value === 'active' ? 'green' : 'default'}>
      {value === 'active' ? '在职' : value === 'inactive' ? '离职/停用' : value || '未设置'}
    </Tag>
  )

  const statBoxStyle = {
    padding: '10px 12px',
    border: '1px solid #f0f0f0',
    borderRadius: 6,
    background: '#fafafa',
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
    return <Tag color={colors[value] || 'default'}>{labels[value] || value}</Tag>
  }

  const formatDateTime = (value?: string) => {
    if (!value) {
      return '-'
    }
    return value.replace('T', ' ').slice(0, 19)
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div>
          <Title level={4} style={{ marginBottom: 4 }}>
            组织架构
          </Title>
          <Text type="secondary">{scopeLabel}</Text>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <Button icon={<ReloadOutlined />} onClick={() => void loadTree()} loading={loading}>
            刷新
          </Button>
          <Button type="primary" icon={<SyncOutlined />} onClick={() => void handleSync()} loading={syncing}>
            同步组织数据
          </Button>
        </div>
      </div>

      {loading ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}>
          <Spin size="large" />
        </div>
      ) : tree.length === 0 ? (
        <Empty description="暂无组织架构数据" />
      ) : (
        <Row gutter={[16, 16]}>
          <Col xs={24} lg={14}>
            <Card title="组织树">
              <Alert
                style={{ marginBottom: 16 }}
                type="info"
                showIcon
                message="展示部门层级中的在职人数，区分直属在职与含下级汇总在职，帮助快速查看组织结构。"
              />
              <Tree
                defaultExpandAll
                selectedKeys={selectedNode ? [selectedNode.id] : []}
                treeData={treeData}
                onSelect={(keys) => {
                  const nextKey = String(keys[0] || '')
                  if (!nextKey) {
                    return
                  }
                  setDepartmentEmployeePage(1)
                  setSelectedNode(nodeMap.get(nextKey) || null)
                }}
              />
            </Card>
          </Col>
          <Col xs={24} lg={10}>
            <Card title="部门基础统计">
              {selectedNode ? (
                <>
                  <Title level={5} style={{ marginTop: 0 }}>
                    {selectedNode.name}
                  </Title>
                  <div style={{ marginTop: 16, color: '#8c8c8c' }}>
                    总人数 {selectedNode.headcount}，直属人数 {selectedNode.direct_headcount}
                    {selectedNode.children?.length ? `，下级部门 ${selectedNode.children.length} 个` : '，当前部门没有下级部门'}
                  </div>
                  {overviewLoading ? (
                    <div style={{ display: 'flex', justifyContent: 'center', padding: '24px 0' }}>
                      <Spin size="small" />
                    </div>
                  ) : (
                    <>
                      <div style={{ marginTop: 12, marginBottom: 8 }}>
                        <Text type="secondary">统计口径：在职、试用期、计划转正预警默认含下级部门汇总。</Text>
                      </div>
                      <Row gutter={[12, 12]}>
                        <Col span={12}>
                          <div style={statBoxStyle}>
                            <Statistic title="直属在职" value={selectedNode.direct_active_count} />
                          </div>
                        </Col>
                        <Col span={12}>
                          <div style={statBoxStyle}>
                            <Statistic title="汇总在职" value={departmentOverview?.summary.active_employees ?? selectedNode.active_count} />
                          </div>
                        </Col>
                        <Col span={12}>
                          <div style={statBoxStyle}>
                            <Statistic title="试用期人数" value={departmentOverview?.summary.probation_employee_count ?? 0} />
                          </div>
                        </Col>
                        <Col span={12}>
                          <div style={statBoxStyle}>
                            <Statistic title="计划转正预警" value={departmentOverview?.summary.planned_regularization_count ?? 0} />
                          </div>
                        </Col>
                      </Row>
                    </>
                  )}
                  <Tabs
                    style={{ marginTop: 20 }}
                    items={[
                      {
                        key: 'members',
                        label: '员工列表',
                        children: (
                          <>
                            <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 8 }}>
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
                                <List.Item>
                                  <List.Item.Meta
                                    avatar={<Avatar src={item.avatar} icon={<UserOutlined />} />}
                                    title={
                                      <Button
                                        type="link"
                                        style={{ padding: 0, height: 'auto' }}
                                        onClick={() => navigate(`/employees/${item.id}`)}
                                      >
                                        {item.name}
                                      </Button>
                                    }
                                    description={
                                      <Space size={4} wrap>
                                        <Text type="secondary">{item.position || '未设置岗位'}</Text>
                                        <Text type="secondary">·</Text>
                                        <Text type="secondary">{nodeMap.get(item.department_id)?.name || item.department_id || '未分配部门'}</Text>
                                        {employeeStatusTag(item.status)}
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
                                <div style={{ width: '100%' }}>
                                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center' }}>
                                    <Space size={4} wrap>
                                      {changeTypeTag(item.change_type)}
                                      <Text strong>{fieldLabel(item.field_name)}</Text>
                                    </Space>
                                    <Text type="secondary">{formatDateTime(item.changed_at || item.created_at)}</Text>
                                  </div>
                                  <div style={{ marginTop: 6 }}>
                                    <Text type="secondary">{item.old_value || '-'}</Text>
                                    <Text type="secondary"> {'->'} </Text>
                                    <Text>{item.new_value || '-'}</Text>
                                  </div>
                                  <div style={{ marginTop: 4, color: '#8c8c8c', fontSize: 12 }}>
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
            </Card>
          </Col>
        </Row>
      )}
    </div>
  )
}

export default DepartmentTree
