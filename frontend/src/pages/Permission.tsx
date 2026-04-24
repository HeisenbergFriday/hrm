import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, Modal, Form, Input, Tree } from 'antd'
import { KeyOutlined, EditOutlined, DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'

const { Title } = Typography

// 模拟API调用
const fetchRoles = async () => {
  await new Promise(resolve => setTimeout(resolve, 1000))
  return [
    { id: '1', name: '管理员', description: '拥有所有权限' },
    { id: '2', name: '部门经理', description: '拥有部门管理权限' },
    { id: '3', name: '普通员工', description: '拥有基本权限' },
  ]
}

const fetchPermissions = async () => {
  await new Promise(resolve => setTimeout(resolve, 1000))
  return [
    {
      key: 'user',
      title: '用户管理',
      value: 'user',
      children: [
        { key: 'user:view', title: '查看用户', value: 'user:view' },
        { key: 'user:edit', title: '编辑用户', value: 'user:edit' },
        { key: 'user:delete', title: '删除用户', value: 'user:delete' },
      ],
    },
    {
      key: 'department',
      title: '部门管理',
      value: 'department',
      children: [
        { key: 'department:view', title: '查看部门', value: 'department:view' },
        { key: 'department:edit', title: '编辑部门', value: 'department:edit' },
        { key: 'department:delete', title: '删除部门', value: 'department:delete' },
      ],
    },
    {
      key: 'attendance',
      title: '考勤管理',
      value: 'attendance',
      children: [
        { key: 'attendance:view', title: '查看考勤', value: 'attendance:view' },
        { key: 'attendance:edit', title: '编辑考勤', value: 'attendance:edit' },
      ],
    },
    {
      key: 'approval',
      title: '审批管理',
      value: 'approval',
      children: [
        { key: 'approval:view', title: '查看审批', value: 'approval:view' },
        { key: 'approval:process', title: '审批处理', value: 'approval:process' },
      ],
    },
  ]
}

const Permission: React.FC = () => {
  const [isModalVisible, setIsModalVisible] = useState(false)
  const [editingRole, setEditingRole] = useState<any>(null)
  const [selectedPermissions, setSelectedPermissions] = useState<string[]>([])

  const { data: roles, isLoading: rolesLoading, isError: rolesError, refetch: refetchRoles } = useQuery({
    queryKey: ['roles'],
    queryFn: fetchRoles
  })
  const { data: permissions, isLoading: permissionsLoading, isError: permissionsError, refetch: refetchPermissions } = useQuery({
    queryKey: ['permissions'],
    queryFn: fetchPermissions
  })

  const handleEditRole = (role: any) => {
    setEditingRole(role)
    setSelectedPermissions([]) // 这里应该根据角色获取已有的权限
    setIsModalVisible(true)
  }

  const handleModalOk = () => {
    setIsModalVisible(false)
    setEditingRole(null)
    setSelectedPermissions([])
  }

  const handleModalCancel = () => {
    setIsModalVisible(false)
    setEditingRole(null)
    setSelectedPermissions([])
  }

  const handlePermissionSelect = (keys: string[]) => {
    setSelectedPermissions(keys)
  }

  const columns = [
    { title: '角色名称', dataIndex: 'name', key: 'name' },
    { title: '描述', dataIndex: 'description', key: 'description' },
    { 
      title: '操作', 
      key: 'action',
      render: (_: any, record: any) => (
        <>
          <Button icon={<EditOutlined />} size="small" onClick={() => handleEditRole(record)} />
          <Button icon={<DeleteOutlined />} size="small" danger />
        </>
      )
    },
  ]

  return (
    <div>
      <Title level={4}>权限管理</Title>
      <Card>
        <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Button type="primary" icon={<PlusOutlined />}>
            创建角色
          </Button>
        </div>

        {rolesLoading ? (
          <div className="loading-container">
            <Spin size="small" />
          </div>
        ) : rolesError ? (
          <div className="error-container">
            <Alert message="加载失败" type="error" showIcon />
            <Button className="retry-button" onClick={() => refetchRoles()}>重试</Button>
          </div>
        ) : roles?.length ? (
          <Table
            columns={columns}
            dataSource={roles}
            rowKey="id"
          />
        ) : (
          <div className="empty-container">
            <Empty description="暂无角色数据" />
          </div>
        )}
      </Card>

      <Modal
        title="编辑角色权限"
        open={isModalVisible}
        onOk={handleModalOk}
        onCancel={handleModalCancel}
        width={600}
        okText="确定"
        cancelText="取消"
      >
        <Form
          initialValues={editingRole}
        >
          <Form.Item label="角色名称" name="name">
            <Input />
          </Form.Item>
          <Form.Item label="描述" name="description">
            <Input.TextArea />
          </Form.Item>
          <Form.Item label="权限设置">
            {permissionsLoading ? (
              <div className="loading-container">
                <Spin size="small" />
              </div>
            ) : permissionsError ? (
              <div className="error-container">
                <Alert message="加载失败" type="error" showIcon />
                <Button className="retry-button" onClick={() => refetchPermissions()}>重试</Button>
              </div>
            ) : permissions?.length ? (
              <Tree
                treeData={permissions}
                checkable
                onCheck={(_: any, { checkedKeys }: any) => handlePermissionSelect(checkedKeys as string[])}
              />
            ) : (
              <div className="empty-container">
                <Empty description="暂无权限数据" />
              </div>
            )}
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Permission
