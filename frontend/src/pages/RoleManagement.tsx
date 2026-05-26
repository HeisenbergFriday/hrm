import React, { useState, useMemo } from 'react'
import { Typography, Table, Spin, Empty, Alert, Button, Modal, Form, Input, message, Card, Space, Tag, Tabs, Tree, Select, Switch, Radio, Divider, Badge, Tooltip } from 'antd'
import { formatDateTime } from '../utils/format'
import {
  UsergroupAddOutlined,
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  ReloadOutlined,
  SearchOutlined,
  SaveOutlined,
  SafetyOutlined,
  MenuOutlined,
  LockOutlined,
  CheckCircleOutlined,
  InfoCircleOutlined,
  GlobalOutlined,
  ApartmentOutlined,
  ExpandOutlined,
  CompressOutlined,
  CheckSquareOutlined,
  CloseSquareOutlined,
  SettingOutlined,
  KeyOutlined,
  UserOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { permissionAPI, orgAPI, userAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'

const { Text, Title } = Typography
const { Option } = Select

interface Role {
  id: string
  name: string
  description: string
  created_at: string
  updated_at: string
}

interface MenuItem {
  title: string
  key: string
  children?: MenuItem[]
}

interface Department {
  id: string
  name: string
  parent_id: string
  children?: Department[]
}

const RoleManagement: React.FC = () => {
  // 角色相关状态
  const [selectedRoleId, setSelectedRoleId] = useState<string>('')
  const [roleSearchText, setRoleSearchText] = useState('')
  const [modalVisible, setModalVisible] = useState(false)
  const [editingRole, setEditingRole] = useState<Role | null>(null)
  const [form] = Form.useForm()

  // 菜单权限相关状态
  const [menuCheckedKeys, setMenuCheckedKeys] = useState<string[]>([])
  const [menuSearchText, setMenuSearchText] = useState('')
  const [menuExpandAll, setMenuExpandAll] = useState(true)

  // 数据权限相关状态
  const [isAllDepartments, setIsAllDepartments] = useState(true)
  const [selectedDepartments, setSelectedDepartments] = useState<string[]>([])

  // 用户分配相关状态
  const [userSearchText, setUserSearchText] = useState('')
  const [assignModalVisible, setAssignModalVisible] = useState(false)
  const [selectedUserId, setSelectedUserId] = useState<string>('')
  const queryClient = useQueryClient()

  // 查询数据
  const { data: rolesData, isLoading: rolesLoading, isError, refetch, error } = useQuery({
    queryKey: ['roles'],
    queryFn: () => permissionAPI.getRoles(),
  })

  const { data: departmentTreeData, isLoading: departmentsLoading } = useQuery({
    queryKey: ['department-tree'],
    queryFn: () => orgAPI.getDepartmentTree(),
  })

  // 获取角色下的用户列表
  const { data: roleUsersData, isLoading: roleUsersLoading, refetch: refetchRoleUsers } = useQuery({
    queryKey: ['role-users', selectedRoleId],
    queryFn: () => permissionAPI.getRoleUsers(Number(selectedRoleId)),
    enabled: !!selectedRoleId,
  })

  // 获取所有用户列表（用于分配，仅在弹窗打开时请求）
  const { data: allUsersData, isLoading: allUsersLoading } = useQuery({
    queryKey: ['all-users'],
    queryFn: () => userAPI.getUsers({ page: 1, page_size: 1000 }),
    enabled: assignModalVisible,
  })

  // 获取选中角色的菜单权限
  const { data: menuPermData, isLoading: menuPermLoading } = useQuery({
    queryKey: ['menu-permission', selectedRoleId],
    queryFn: () => permissionAPI.getMenuPermission(Number(selectedRoleId)),
    enabled: !!selectedRoleId,
  })

  // 获取选中角色的数据权限
  const { data: dataPermData, isLoading: dataPermLoading } = useQuery({
    queryKey: ['data-permission', selectedRoleId],
    queryFn: () => permissionAPI.getDataPermission(Number(selectedRoleId)),
    enabled: !!selectedRoleId,
  })

  // 保存菜单权限 mutation
  const saveMenuPermMutation = useMutation({
    mutationFn: (data: { roleId: number; menuKeys: string[] }) =>
      permissionAPI.saveMenuPermission(data.roleId, data.menuKeys),
    onSuccess: () => {
      message.success('菜单权限保存成功')
      queryClient.invalidateQueries({ queryKey: ['menu-permission', selectedRoleId] })
    },
    onError: () => message.error('菜单权限保存失败'),
  })

  // 保存数据权限 mutation
  const saveDataPermMutation = useMutation({
    mutationFn: (data: { roleId: number; scope: string; departmentKeys: string[] }) =>
      permissionAPI.saveDataPermission(data.roleId, data.scope, data.departmentKeys),
    onSuccess: () => {
      message.success('数据权限保存成功')
      queryClient.invalidateQueries({ queryKey: ['data-permission', selectedRoleId] })
    },
    onError: () => message.error('数据权限保存失败'),
  })

  // 获取选中的角色
  const selectedRole = useMemo(() => {
    if (!selectedRoleId || !rolesData?.data?.items) return null
    return rolesData.data.items.find((r: Role) => r.id === selectedRoleId)
  }, [selectedRoleId, rolesData])

  // 过滤后的角色列表
  const filteredRoles = useMemo(() => {
    if (!rolesData?.data?.items) return []
    if (!roleSearchText) return rolesData.data.items
    return rolesData.data.items.filter((r: Role) =>
      r.name.toLowerCase().includes(roleSearchText.toLowerCase()) ||
      r.description.toLowerCase().includes(roleSearchText.toLowerCase())
    )
  }, [rolesData, roleSearchText])

  // 菜单项配置
  const menuItems: MenuItem[] = [
    { title: '首页', key: 'home' },
    {
      title: '组织管理',
      key: 'organization',
      children: [
        { title: '部门树', key: 'department-tree' },
        { title: '员工列表', key: 'employees' },
        { title: '同步日志', key: 'sync-log' },
      ],
    },
    {
      title: '考勤管理',
      key: 'attendance',
      children: [
        { title: '考勤查询', key: 'attendance-records' },
        { title: '异常统计', key: 'attendance-stats' },
        { title: '导出记录', key: 'attendance-export' },
      ],
    },
    {
      title: '审批管理',
      key: 'approval',
      children: [
        { title: '审批模板', key: 'approval-templates' },
        { title: '审批实例', key: 'approval-instances' },
        { title: '审批统计', key: 'approval-stats' },
      ],
    },
    {
      title: '权限管理',
      key: 'permission',
      children: [
        { title: '角色管理', key: 'role-management' },
        { title: '菜单权限', key: 'menu-permission' },
        { title: '数据权限', key: 'data-permission' },
      ],
    },
    {
      title: '任务中心',
      key: 'jobs',
      children: [{ title: '同步任务', key: 'sync-jobs' }],
    },
    {
      title: '审计日志',
      key: 'audit',
      children: [{ title: '操作日志', key: 'audit-logs' }],
    },
  ]

  // 计算所有菜单key
  const allMenuKeys = useMemo(() => {
    const keys: string[] = []
    const collectKeys = (items: MenuItem[]) => {
      items.forEach((item) => {
        keys.push(item.key)
        if (item.children) collectKeys(item.children)
      })
    }
    collectKeys(menuItems)
    return keys
  }, [])

  // 过滤菜单树（支持搜索）
  const filteredMenuItems = useMemo(() => {
    if (!menuSearchText) return menuItems
    const search = menuSearchText.toLowerCase()
    const filterTree = (items: MenuItem[]): MenuItem[] => {
      return items
        .map((item) => {
          const matchesSelf = item.title.toLowerCase().includes(search)
          const filteredChildren = item.children ? filterTree(item.children) : []
          if (matchesSelf || filteredChildren.length > 0) {
            return { ...item, children: filteredChildren.length > 0 ? filteredChildren : item.children }
          }
          return null
        })
        .filter(Boolean) as MenuItem[]
    }
    return filterTree(menuItems)
  }, [menuSearchText])

  // Mutations
  const createRoleMutation = useMutation({
    mutationFn: (data: { name: string; description: string }) => permissionAPI.createRole(data),
    onSuccess: () => {
      message.success('角色创建成功')
      setModalVisible(false)
      form.resetFields()
      refetch()
    },
    onError: () => message.error('角色创建失败'),
  })

  const updateRoleMutation = useMutation({
    mutationFn: (data: { id: number; name: string; description: string }) =>
      permissionAPI.updateRole(data.id, { name: data.name, description: data.description }),
    onSuccess: () => {
      message.success('角色更新成功')
      setModalVisible(false)
      setEditingRole(null)
      form.resetFields()
      refetch()
    },
    onError: () => message.error('角色更新失败'),
  })

  // 分配用户到角色
  const assignUserMutation = useMutation({
    mutationFn: (data: { user_id: string; role_id: number }) => permissionAPI.assignUserRole(data),
    onSuccess: () => {
      message.success('用户分配成功')
      setAssignModalVisible(false)
      setSelectedUserId('')
      refetchRoleUsers()
    },
    onError: () => message.error('用户分配失败'),
  })

  // 从角色移除用户
  const removeUserMutation = useMutation({
    mutationFn: (data: { user_id: string; role_id: number }) => permissionAPI.removeUserRole(data),
    onSuccess: () => {
      message.success('用户移除成功')
      refetchRoleUsers()
    },
    onError: () => message.error('用户移除失败'),
  })

  // 处理函数
  const handleSelectRole = (roleId: string) => {
    setSelectedRoleId(roleId)
    // 重置状态（实际数据由 query 加载）
    setMenuCheckedKeys([])
    setIsAllDepartments(true)
    setSelectedDepartments([])
  }

  // 当菜单权限数据加载完成后，同步到本地状态
  React.useEffect(() => {
    if (menuPermData?.data?.menu_keys) {
      try {
        const keys = JSON.parse(menuPermData.data.menu_keys)
        setMenuCheckedKeys(Array.isArray(keys) ? keys : [])
      } catch {
        setMenuCheckedKeys([])
      }
    }
  }, [menuPermData])

  // 当数据权限数据加载完成后，同步到本地状态
  React.useEffect(() => {
    if (dataPermData?.data) {
      const { scope, department_keys } = dataPermData.data
      setIsAllDepartments(scope === 'all')
      if (scope === 'department' && department_keys) {
        try {
          const keys = JSON.parse(department_keys)
          setSelectedDepartments(Array.isArray(keys) ? keys : [])
        } catch {
          setSelectedDepartments([])
        }
      } else {
        setSelectedDepartments([])
      }
    }
  }, [dataPermData])

  const handleCreateRole = () => {
    setEditingRole(null)
    form.resetFields()
    setModalVisible(true)
  }

  const handleEditRole = (role: Role) => {
    setEditingRole(role)
    form.setFieldsValue({ name: role.name, description: role.description })
    setModalVisible(true)
  }

  const handleSubmit = () => {
    form.validateFields().then((values) => {
      if (editingRole) {
        updateRoleMutation.mutate({ id: Number(editingRole.id), ...values })
      } else {
        createRoleMutation.mutate(values)
      }
    })
  }

  const handleSaveMenuPermission = () => {
    if (!selectedRoleId) return
    saveMenuPermMutation.mutate({ roleId: Number(selectedRoleId), menuKeys: menuCheckedKeys })
  }

  const handleSaveDataPermission = () => {
    if (!selectedRoleId) return
    saveDataPermMutation.mutate({
      roleId: Number(selectedRoleId),
      scope: isAllDepartments ? 'all' : 'department',
      departmentKeys: selectedDepartments,
    })
  }

  // 用户分配相关函数
  const handleAssignUser = () => {
    if (!selectedRoleId || !selectedUserId) {
      message.warning('请选择用户')
      return
    }
    assignUserMutation.mutate({ user_id: selectedUserId, role_id: Number(selectedRoleId) })
  }

  const handleRemoveUser = (userId: string) => {
    if (!selectedRoleId) return
    Modal.confirm({
      title: '确认移除',
      content: '确定要将该用户从当前角色中移除吗？',
      okText: '确定',
      cancelText: '取消',
      okType: 'danger',
      onOk: () => {
        removeUserMutation.mutate({ user_id: userId, role_id: Number(selectedRoleId) })
      },
    })
  }

  const handleSelectAllMenu = () => {
    setMenuCheckedKeys(allMenuKeys)
  }

  const handleDeselectAllMenu = () => {
    setMenuCheckedKeys([])
  }

  // 渲染部门树
  const renderDepartmentTree = (departments: Department[]): any[] => {
    return departments.map((dept) => ({
      title: dept.name,
      key: dept.id,
      children: dept.children && dept.children.length > 0 ? renderDepartmentTree(dept.children) : undefined,
    }))
  }

  // 角色列表卡片
  const renderRoleList = () => (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div style={{ marginBottom: 16 }}>
        <Input
          placeholder="搜索角色"
          prefix={<SearchOutlined style={{ color: '#bfbfbf' }} />}
          value={roleSearchText}
          onChange={(e) => setRoleSearchText(e.target.value)}
          allowClear
          style={{ marginBottom: 12 }}
        />
        <Button type="primary" icon={<PlusOutlined />} block onClick={handleCreateRole}>
          新建角色
        </Button>
      </div>
      <div style={{ flex: 1, overflow: 'auto' }}>
        {filteredRoles.map((role: Role) => (
          <Card
            key={role.id}
            size="small"
            hoverable
            style={{
              marginBottom: 8,
              borderColor: selectedRoleId === role.id ? '#4338ca' : undefined,
              backgroundColor: selectedRoleId === role.id ? '#f5f3ff' : undefined,
            }}
            onClick={() => handleSelectRole(role.id)}
          >
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <Space>
                <SafetyOutlined style={{ color: selectedRoleId === role.id ? '#4338ca' : '#8c8c8c', fontSize: 16 }} />
                <div>
                  <Text strong style={{ display: 'block', fontSize: 14 }}>{role.name}</Text>
                  <Text type="secondary" style={{ fontSize: 12 }}>{role.description || '暂无描述'}</Text>
                </div>
              </Space>
              <Tooltip title="编辑">
                <Button
                  type="text"
                  size="small"
                  icon={<EditOutlined />}
                  onClick={(e) => {
                    e.stopPropagation()
                    handleEditRole(role)
                  }}
                />
              </Tooltip>
            </div>
          </Card>
        ))}
        {filteredRoles.length === 0 && (
          <Empty description="暂无角色" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        )}
      </div>
    </div>
  )

  // 基本设置Tab
  const renderBasicSettings = () => (
    <Card title={<Space><SettingOutlined /> <Text strong>基本信息</Text></Space>}>
      {selectedRole ? (
        <Form layout="vertical" initialValues={selectedRole}>
          <Form.Item label="角色名称">
            <Input value={selectedRole.name} readOnly />
          </Form.Item>
          <Form.Item label="角色描述">
            <Input.TextArea value={selectedRole.description} readOnly rows={3} />
          </Form.Item>
          <Form.Item label="创建时间">
            <Input value={formatDateTime(selectedRole.created_at)} readOnly />
          </Form.Item>
          <Form.Item label="更新时间">
            <Input value={formatDateTime(selectedRole.updated_at)} readOnly />
          </Form.Item>
        </Form>
      ) : (
        <Empty description="请先选择一个角色" />
      )}
    </Card>
  )

  // 菜单权限Tab
  const renderMenuPermission = () => (
    <Card
      title={<Space><MenuOutlined /> <Text strong>菜单权限配置</Text></Space>}
      extra={
        <Button type="primary" icon={<SaveOutlined />} onClick={handleSaveMenuPermission} disabled={!selectedRole} loading={saveMenuPermMutation.isPending}>
          保存
        </Button>
      }
    >
      {!selectedRole ? (
        <Empty description="请先选择一个角色" />
      ) : (
        <>
          <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Space>
              <Input
                placeholder="搜索菜单"
                prefix={<SearchOutlined style={{ color: '#bfbfbf' }} />}
                value={menuSearchText}
                onChange={(e) => setMenuSearchText(e.target.value)}
                allowClear
                style={{ width: 200 }}
              />
              <Tooltip title={menuExpandAll ? '全部折叠' : '全部展开'}>
                <Button
                  icon={menuExpandAll ? <CompressOutlined /> : <ExpandOutlined />}
                  onClick={() => setMenuExpandAll(!menuExpandAll)}
                />
              </Tooltip>
            </Space>
            <Space>
              <Tooltip title="全选">
                <Button icon={<CheckSquareOutlined />} onClick={handleSelectAllMenu}>全选</Button>
              </Tooltip>
              <Tooltip title="全不选">
                <Button icon={<CloseSquareOutlined />} onClick={handleDeselectAllMenu}>全不选</Button>
              </Tooltip>
              <Badge count={menuCheckedKeys.length} showZero style={{ backgroundColor: '#4338ca' }}>
                <Tag color="processing">已选权限</Tag>
              </Badge>
            </Space>
          </div>
          <Divider style={{ margin: '12px 0' }} />
          <div style={{ minHeight: 300, maxHeight: 400, overflow: 'auto' }}>
            <Tree
              checkable
              treeData={filteredMenuItems}
              checkedKeys={menuCheckedKeys}
              onCheck={(checked) => setMenuCheckedKeys(checked as string[])}
              defaultExpandAll={menuExpandAll}
              style={{ fontSize: 14 }}
            />
          </div>
          <Divider style={{ margin: '12px 0' }} />
          <Alert
            message="勾选菜单后，对应角色将拥有访问权限"
            type="info"
            showIcon
          />
        </>
      )}
    </Card>
  )

  // 数据权限Tab
  const renderDataPermission = () => (
    <Card
      title={<Space><LockOutlined /> <Text strong>数据权限配置</Text></Space>}
      extra={
        <Button type="primary" icon={<SaveOutlined />} onClick={handleSaveDataPermission} disabled={!selectedRole} loading={saveDataPermMutation.isPending}>
          保存
        </Button>
      }
    >
      {!selectedRole ? (
        <Empty description="请先选择一个角色" />
      ) : (
        <>
          <Form layout="vertical">
            <Form.Item label="数据范围">
              <Radio.Group
                value={isAllDepartments ? 'all' : 'custom'}
                onChange={(e) => setIsAllDepartments(e.target.value === 'all')}
                optionType="button"
                buttonStyle="solid"
              >
                <Radio.Button value="all">
                  <Space><GlobalOutlined /> 全部部门</Space>
                </Radio.Button>
                <Radio.Button value="custom">
                  <Space><ApartmentOutlined /> 指定部门</Space>
                </Radio.Button>
              </Radio.Group>
            </Form.Item>
          </Form>

          {!isAllDepartments && (
            <>
              <Divider orientation="left">选择部门</Divider>
              <Card
                style={{ minHeight: 300, maxHeight: 400, overflow: 'auto', backgroundColor: '#fafafa' }}
                styles={{ body: { padding: 16 } }}
              >
                {departmentsLoading ? (
                  <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>
                ) : (
                  <Tree
                    checkable
                    treeData={renderDepartmentTree(departmentTreeData?.data?.tree || [])}
                    checkedKeys={selectedDepartments}
                    onCheck={(checked) => setSelectedDepartments(checked as string[])}
                    defaultExpandAll
                    style={{ fontSize: 14 }}
                  />
                )}
              </Card>
            </>
          )}

          <Divider style={{ margin: '16px 0' }} />
          <Alert
            message="权限说明"
            description={
              <ul style={{ margin: '8px 0 0 0', paddingLeft: 20 }}>
                <li><Tag color="success" style={{ marginRight: 4 }}>全部部门</Tag>可以查看所有部门的数据</li>
                <li><Tag color="processing" style={{ marginRight: 4 }}>指定部门</Tag>只能查看选中部门及其子部门的数据</li>
                <li><Tag color="warning" style={{ marginRight: 4 }}>部门负责人</Tag>默认可以查看自己负责部门及其子部门的数据</li>
              </ul>
            }
            type="info"
            icon={<InfoCircleOutlined />}
            showIcon
          />
        </>
      )}
    </Card>
  )

  // 用户分配Tab
  const renderUserAssignment = () => (
    <Card
      title={<Space><TeamOutlined /> <Text strong>用户分配</Text></Space>}
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setAssignModalVisible(true)} disabled={!selectedRole}>
          添加用户
        </Button>
      }
    >
      {!selectedRole ? (
        <Empty description="请先选择一个角色" />
      ) : (
        <>
          {roleUsersLoading ? (
            <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>
          ) : (
            <>
              <div style={{ marginBottom: 16 }}>
                <Input
                  placeholder="搜索用户"
                  prefix={<SearchOutlined style={{ color: '#bfbfbf' }} />}
                  value={userSearchText}
                  onChange={(e) => setUserSearchText(e.target.value)}
                  allowClear
                  style={{ width: 300 }}
                />
              </div>

              <Table
                dataSource={(roleUsersData?.data?.users || []).filter((user: any) =>
                  !userSearchText ||
                  user.name?.toLowerCase().includes(userSearchText.toLowerCase()) ||
                  user.user_id?.toLowerCase().includes(userSearchText.toLowerCase())
                )}
                columns={[
                  {
                    title: '用户ID',
                    dataIndex: 'user_id',
                    key: 'user_id',
                  },
                  {
                    title: '姓名',
                    dataIndex: 'name',
                    key: 'name',
                  },
                  {
                    title: '部门',
                    dataIndex: 'department_name',
                    key: 'department_name',
                  },
                  {
                    title: '操作',
                    key: 'action',
                    render: (_: any, record: any) => (
                      <Button
                        type="link"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={() => handleRemoveUser(record.user_id)}
                      >
                        移除
                      </Button>
                    ),
                  },
                ]}
                rowKey="user_id"
                pagination={{ pageSize: 10 }}
                locale={{ emptyText: '该角色下暂无用户' }}
              />

              <Divider style={{ margin: '16px 0' }} />
              <Alert
                message="用户分配说明"
                description={
                  <ul style={{ margin: '8px 0 0 0', paddingLeft: 20 }}>
                    <li>一个用户可以分配多个角色</li>
                    <li>移除用户后，该用户将失去此角色的所有权限</li>
                    <li>点击"添加用户"按钮可以为当前角色分配新用户</li>
                  </ul>
                }
                type="info"
                icon={<InfoCircleOutlined />}
                showIcon
              />
            </>
          )}
        </>
      )}

      {/* 添加用户弹窗 */}
      <Modal
        title="添加用户到角色"
        open={assignModalVisible}
        onCancel={() => {
          setAssignModalVisible(false)
          setSelectedUserId('')
        }}
        onOk={handleAssignUser}
        confirmLoading={assignUserMutation.isPending}
        okText="确定"
        cancelText="取消"
      >
        <Form layout="vertical">
          <Form.Item label="选择用户" required>
            <Select
              showSearch
              placeholder="请选择用户"
              value={selectedUserId || undefined}
              onChange={(value) => setSelectedUserId(value)}
              filterOption={(input, option) =>
                (option?.label ?? '').toString().toLowerCase().includes(input.toLowerCase())
              }
              options={(allUsersData?.data?.items || [])
                .filter((user: any) => !(roleUsersData?.data?.users || []).some((u: any) => u.user_id === user.user_id))
                .map((user: any) => ({
                  label: `${user.name} (${user.user_id})`,
                  value: user.user_id,
                }))}
              loading={allUsersLoading}
              style={{ width: '100%' }}
            />
          </Form.Item>
        </Form>
        <Alert
          message="只能选择尚未分配当前角色的用户"
          type="info"
          showIcon
          style={{ marginTop: 16 }}
        />
      </Modal>
    </Card>
  )

  // Tab配置
  const tabItems = [
    { key: 'basic', label: <Space><SettingOutlined /> 基本设置</Space>, children: renderBasicSettings() },
    { key: 'menu', label: <Space><MenuOutlined /> 菜单权限</Space>, children: renderMenuPermission() },
    { key: 'data', label: <Space><LockOutlined /> 数据权限</Space>, children: renderDataPermission() },
    { key: 'users', label: <Space><TeamOutlined /> 用户分配</Space>, children: renderUserAssignment() },
  ]

  if (rolesLoading) {
    return (
      <PageContainer title="权限管理" icon={<KeyOutlined />} subtitle="管理系统角色与权限配置">
        <div style={{ display: 'flex', justifyContent: 'center', padding: '60px' }}>
          <Spin size="large" />
        </div>
      </PageContainer>
    )
  }

  if (isError) {
    return (
      <PageContainer title="权限管理" icon={<KeyOutlined />} subtitle="管理系统角色与权限配置">
        <Alert
          message="加载失败"
          description={(error as Error)?.message || '获取角色列表失败，请稍后重试'}
          type="error"
          showIcon
          action={<Button onClick={() => refetch()}>重试</Button>}
        />
      </PageContainer>
    )
  }

  return (
    <PageContainer title="权限管理" icon={<KeyOutlined />} subtitle="管理系统角色与权限配置">
      <div style={{ display: 'flex', gap: 16, height: 'calc(100vh - 200px)' }}>
        {/* 左侧角色列表 */}
        <Card
          title={<Space><UsergroupAddOutlined /> <Text strong>角色列表</Text></Space>}
          extra={<Badge count={filteredRoles.length} style={{ backgroundColor: '#4338ca' }} />}
          style={{ width: 280, flexShrink: 0 }}
          styles={{ body: { padding: 16, height: 'calc(100% - 57px)', overflow: 'hidden' } }}
        >
          {renderRoleList()}
        </Card>

        {/* 右侧权限配置 */}
        <div style={{ flex: 1, minWidth: 0 }}>
          <Tabs items={tabItems} size="large" />
        </div>
      </div>

      {/* 新建/编辑角色弹窗 */}
      <Modal
        title={editingRole ? '编辑角色' : '新建角色'}
        open={modalVisible}
        onCancel={() => { setModalVisible(false); setEditingRole(null); form.resetFields() }}
        footer={[
          <Button key="cancel" onClick={() => { setModalVisible(false); setEditingRole(null); form.resetFields() }}>
            取消
          </Button>,
          <Button
            key="submit"
            type="primary"
            onClick={handleSubmit}
            loading={createRoleMutation.isPending || updateRoleMutation.isPending}
          >
            确认
          </Button>,
        ]}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="角色名称"
            rules={[{ required: true, message: '请输入角色名称' }]}
          >
            <Input placeholder="请输入角色名称" />
          </Form.Item>
          <Form.Item
            name="description"
            label="角色描述"
            rules={[{ required: true, message: '请输入角色描述' }]}
          >
            <Input.TextArea placeholder="请输入角色描述" rows={4} />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  )
}

export default RoleManagement
