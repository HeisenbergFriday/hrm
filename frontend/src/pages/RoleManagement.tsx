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
import { permissionAPI, orgAPI, userAPI, refreshMenuKeys } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import { menuConfig, toTreeData, type TreeNode } from '../config/menu'
import { hasPermission } from '../utils/permission'

const { Text, Title } = Typography
const { Option } = Select

interface Role {
  id: string
  name: string
  description: string
  created_at: string
  updated_at: string
}

interface PermissionItem {
  id: number
  name: string
  code: string
  description?: string
}

// 使用 config/menu 中定义的 TreeNode 类型
type MenuItem = TreeNode

interface Department {
  id: string
  name: string
  parent_id: string
  children?: Department[]
}

const RoleManagement: React.FC = () => {
  const canManagePermission = hasPermission('permission_manage')
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

  const [permissionCheckedKeys, setPermissionCheckedKeys] = useState<React.Key[]>([])
  const [permissionSearchText, setPermissionSearchText] = useState('')

  // 数据权限相关状态
  const [dataScope, setDataScope] = useState<string>('all')
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
    queryKey: ['department-tree-all'],
    queryFn: () => orgAPI.getDepartmentTree({ all: true }),
    enabled: canManagePermission,
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
    enabled: assignModalVisible && canManagePermission,
  })

  const { data: permissionsData, isLoading: permissionsLoading } = useQuery({
    queryKey: ['permissions'],
    queryFn: () => permissionAPI.getPermissions(),
    enabled: canManagePermission,
  })

  const { data: rolePermissionsData, isLoading: rolePermissionsLoading } = useQuery({
    queryKey: ['role-permissions', selectedRoleId],
    queryFn: () => permissionAPI.getRolePermissions(Number(selectedRoleId)),
    enabled: !!selectedRoleId,
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

  // 获取选中的角色
  const saveRolePermMutation = useMutation({
    mutationFn: (data: { roleId: number; permissionIds: number[] }) =>
      permissionAPI.saveRolePermissions(data.roleId, data.permissionIds),
    onSuccess: () => {
      message.success('功能权限保存成功')
      queryClient.invalidateQueries({ queryKey: ['role-permissions', selectedRoleId] })
      queryClient.invalidateQueries({ queryKey: ['permissions'] })
      refreshMenuKeys()
    },
    onError: () => message.error('功能权限保存失败'),
  })

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

  // 菜单项配置（统一从 config/menu.tsx 导入）
  const menuItems: MenuItem[] = useMemo(() => toTreeData(menuConfig), [])

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
  }, [menuItems])

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
  }, [menuItems, menuSearchText])

  const permissions: PermissionItem[] = useMemo(() => {
    return permissionsData?.data?.items || []
  }, [permissionsData])

  const filteredPermissions = useMemo(() => {
    const search = permissionSearchText.trim().toLowerCase()
    if (!search) return permissions
    return permissions.filter((item) =>
      item.name?.toLowerCase().includes(search) ||
      item.code?.toLowerCase().includes(search) ||
      item.description?.toLowerCase().includes(search)
    )
  }, [permissions, permissionSearchText])

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
    setMenuCheckedKeys([])
    setPermissionCheckedKeys([])
    setDataScope('all')
    setSelectedDepartments([])
  }

  React.useEffect(() => {
    const rolePermissions = rolePermissionsData?.data?.permissions
    if (Array.isArray(rolePermissions)) {
      setPermissionCheckedKeys(rolePermissions.map((item: PermissionItem) => item.id))
    }
  }, [rolePermissionsData])

  // 当菜单权限数据加载完成后，同步到本地状态
  React.useEffect(() => {
    const rawMenuKeys = menuPermData?.data?.menu_keys
    if (Array.isArray(rawMenuKeys)) {
      setMenuCheckedKeys(rawMenuKeys)
      return
    }
    if (typeof rawMenuKeys === 'string' && rawMenuKeys) {
      try {
        const keys = JSON.parse(rawMenuKeys)
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
      setDataScope(scope || 'all')
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
    if (!canManagePermission) return
    setEditingRole(null)
    form.resetFields()
    setModalVisible(true)
  }

  const handleEditRole = (role: Role) => {
    if (!canManagePermission) return
    setEditingRole(role)
    form.setFieldsValue({ name: role.name, description: role.description })
    setModalVisible(true)
  }

  const handleSubmit = () => {
    if (!canManagePermission) return
    form.validateFields().then((values) => {
      if (editingRole) {
        updateRoleMutation.mutate({ id: Number(editingRole.id), ...values })
      } else {
        createRoleMutation.mutate(values)
      }
    })
  }

  const handleSaveDataPermission = () => {
    if (!selectedRoleId || !canManagePermission) return
    saveDataPermMutation.mutate({
      roleId: Number(selectedRoleId),
      scope: dataScope,
      departmentKeys: dataScope === 'department' ? selectedDepartments : [],
    })
  }

  const handleSaveMenuPermission = () => {
    if (!selectedRoleId || !canManagePermission) return
    saveMenuPermMutation.mutate({ roleId: Number(selectedRoleId), menuKeys: menuCheckedKeys })
  }

  const handleSaveRolePermission = () => {
    if (!selectedRoleId || !canManagePermission) return
    saveRolePermMutation.mutate({
      roleId: Number(selectedRoleId),
      permissionIds: permissionCheckedKeys.map((key) => Number(key)).filter(Boolean),
    })
  }

  const handleSelectAllMenu = () => {
    if (!canManagePermission) return
    setMenuCheckedKeys(allMenuKeys)
  }

  const handleDeselectAllMenu = () => {
    if (!canManagePermission) return
    setMenuCheckedKeys([])
  }

  // 用户分配相关函数
  const handleAssignUser = () => {
    if (!canManagePermission) return
    if (!selectedRoleId || !selectedUserId) {
      message.warning('请选择用户')
      return
    }
    assignUserMutation.mutate({ user_id: selectedUserId, role_id: Number(selectedRoleId) })
  }

  const handleRemoveUser = (userId: string) => {
    if (!selectedRoleId || !canManagePermission) return
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
        {canManagePermission && (
          <Button type="primary" icon={<PlusOutlined />} block onClick={handleCreateRole}>
            新建角色
          </Button>
        )}
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
              {canManagePermission && (
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
              )}
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

  // 菜单权限Tab（可编辑 Tree，保存到 menu_permissions 表）
  const renderRolePermission = () => (
    <Card
      title={<Space><KeyOutlined /> <Text strong>功能权限配置</Text></Space>}
      extra={
        <Button
          type="primary"
          icon={<SaveOutlined />}
          onClick={handleSaveRolePermission}
          disabled={!selectedRole || !canManagePermission}
          loading={saveRolePermMutation.isPending}
        >
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
                placeholder="搜索功能权限"
                prefix={<SearchOutlined style={{ color: '#bfbfbf' }} />}
                value={permissionSearchText}
                onChange={(e) => setPermissionSearchText(e.target.value)}
                allowClear
                style={{ width: 240 }}
              />
              <Button
                icon={<CheckSquareOutlined />}
                onClick={() => setPermissionCheckedKeys(filteredPermissions.map((item) => item.id))}
                disabled={!canManagePermission}
              >
                全选当前
              </Button>
              <Button
                icon={<CloseSquareOutlined />}
                onClick={() => setPermissionCheckedKeys([])}
                disabled={!canManagePermission}
              >
                全不选
              </Button>
            </Space>
            <Badge count={permissionCheckedKeys.length} showZero style={{ backgroundColor: '#4338ca' }}>
              <Tag color="processing">已选权限</Tag>
            </Badge>
          </div>
          <Table
            rowKey="id"
            loading={permissionsLoading || rolePermissionsLoading}
            dataSource={filteredPermissions}
            rowSelection={{
              selectedRowKeys: permissionCheckedKeys,
              onChange: setPermissionCheckedKeys,
              getCheckboxProps: () => ({ disabled: !canManagePermission }),
            }}
            columns={[
              { title: '权限名称', dataIndex: 'name', key: 'name', width: 180 },
              {
                title: '权限码',
                dataIndex: 'code',
                key: 'code',
                width: 260,
                render: (code: string) => <Tag color={code.startsWith('performance:') ? 'blue' : 'default'}>{code}</Tag>,
              },
              {
                title: '说明',
                dataIndex: 'description',
                key: 'description',
                render: (text: string) => text || '-',
              },
            ]}
            pagination={{ pageSize: 10 }}
            locale={{ emptyText: '暂无功能权限' }}
          />
          <Divider style={{ margin: '16px 0' }} />
          <Alert
            message="勾选并保存后，对应角色会获得这些后端接口操作权限。权限字典缺失时，系统会在加载列表时自动补齐内置权限。"
            type="info"
            icon={<InfoCircleOutlined />}
            showIcon
          />
        </>
      )}
    </Card>
  )

  const renderMenuPermission = () => (
    <Card
      title={<Space><MenuOutlined /> <Text strong>菜单权限配置</Text></Space>}
      extra={
        <Button type="primary" icon={<SaveOutlined />} onClick={handleSaveMenuPermission} disabled={!selectedRole || !canManagePermission} loading={saveMenuPermMutation.isPending}>
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
              <Tooltip title="全选">
                <Button icon={<CheckSquareOutlined />} onClick={handleSelectAllMenu} disabled={!canManagePermission}>全选</Button>
              </Tooltip>
              <Tooltip title="全不选">
                <Button icon={<CloseSquareOutlined />} onClick={handleDeselectAllMenu} disabled={!canManagePermission}>全不选</Button>
              </Tooltip>
            </Space>
            <Badge count={menuCheckedKeys.length} showZero style={{ backgroundColor: '#4338ca' }}>
              <Tag color="processing">已选菜单</Tag>
            </Badge>
          </div>
          <Divider style={{ margin: '12px 0' }} />
          <div style={{ minHeight: 300, maxHeight: 400, overflow: 'auto' }}>
            <Tree
              checkable
              treeData={filteredMenuItems}
              checkedKeys={menuCheckedKeys}
              onCheck={(checked) => setMenuCheckedKeys(checked as string[])}
              defaultExpandAll={menuExpandAll}
              disabled={!canManagePermission}
              style={{ fontSize: 14 }}
            />
          </div>
          <Divider style={{ margin: '12px 0' }} />
          <Alert
            message="勾选菜单后保存，对应角色将拥有页面访问权限"
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
        <Button type="primary" icon={<SaveOutlined />} onClick={handleSaveDataPermission} disabled={!selectedRole || !canManagePermission} loading={saveDataPermMutation.isPending}>
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
                value={dataScope}
                onChange={(e) => {
                  setDataScope(e.target.value)
                  if (e.target.value !== 'department') {
                    setSelectedDepartments([])
                  }
                }}
                optionType="button"
                buttonStyle="solid"
                disabled={!canManagePermission}
              >
                <Radio.Button value="all">
                  <Space><GlobalOutlined /> 全部部门</Space>
                </Radio.Button>
                <Radio.Button value="department">
                  <Space><ApartmentOutlined /> 指定部门</Space>
                </Radio.Button>
                <Radio.Button value="self">
                  <Space><UserOutlined /> 仅本人</Space>
                </Radio.Button>
              </Radio.Group>
            </Form.Item>
          </Form>

          {dataScope === 'department' && (
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
                    disabled={!canManagePermission}
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
                <li><Tag color="warning" style={{ marginRight: 4 }}>仅本人</Tag>只能查看自己的数据，适用于普通员工</li>
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
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setAssignModalVisible(true)} disabled={!selectedRole || !canManagePermission}>
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
                    render: (_: any, record: any) => canManagePermission ? (
                      <Button
                        type="link"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={() => handleRemoveUser(record.user_id)}
                      >
                        移除
                      </Button>
                    ) : null,
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
        okButtonProps={{ disabled: !canManagePermission }}
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
    { key: 'permissions', label: <Space><KeyOutlined /> 功能权限</Space>, children: renderRolePermission() },
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
            disabled={!canManagePermission}
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
