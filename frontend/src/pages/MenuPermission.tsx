import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, Select, Tree, message } from 'antd'
import { MenuOutlined, ReloadOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { permissionAPI } from '../services/api'

const { Title, Text } = Typography
const { Option } = Select

interface Permission {
  id: string
  name: string
  code: string
  description: string
  created_at: string
  updated_at: string
}

interface MenuItem {
  title: string
  key: string
  children?: MenuItem[]
}

const MenuPermission: React.FC = () => {
  const [selectedRole, setSelectedRole] = useState<string>('1')
  const [checkedKeys, setCheckedKeys] = useState<string[]>([])

  const { data: rolesData, isLoading: rolesLoading } = useQuery({
    queryKey: ['roles'],
    queryFn: () => permissionAPI.getRoles(),
  })

  const { data: permissionsData, isLoading: permissionsLoading } = useQuery({
    queryKey: ['permissions'],
    queryFn: () => permissionAPI.getPermissions(),
  })

  const menuItems: MenuItem[] = [
    {
      title: '首页',
      key: 'home',
    },
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
      children: [
        { title: '同步任务', key: 'sync-jobs' },
      ],
    },
    {
      title: '审计日志',
      key: 'audit',
      children: [
        { title: '操作日志', key: 'audit-logs' },
      ],
    },
  ]

  const handleSave = () => {
    message.success('菜单权限保存成功')
  }

  const handleRoleChange = (value: string) => {
    setSelectedRole(value)
    // 模拟根据角色加载权限
    setCheckedKeys(['home', 'organization', 'department-tree'])
  }

  if (rolesLoading || permissionsLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div>
      <Title level={4}>菜单权限</Title>
      <Card>
        <div style={{ marginBottom: 16, display: 'flex', gap: 16, alignItems: 'center' }}>
          <Text strong>选择角色：</Text>
          <Select
            style={{ width: 200 }}
            value={selectedRole}
            onChange={handleRoleChange}
          >
            {rolesData?.data?.items?.map((role: any) => (
              <Option key={role.id} value={role.id}>
                {role.name}
              </Option>
            ))}
          </Select>
          <Button type="primary" onClick={handleSave} style={{ marginLeft: 'auto' }}>
            保存权限
          </Button>
        </div>

        <div style={{ border: '1px solid #f0f0f0', borderRadius: 4, padding: 16, minHeight: 400 }}>
          <Tree
            checkable
            treeData={menuItems}
            checkedKeys={checkedKeys}
            onCheck={(checked, info) => setCheckedKeys(checked as string[])}
            defaultExpandAll
          />
        </div>
      </Card>
    </div>
  )
}

export default MenuPermission