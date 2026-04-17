import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, Select, Tree, message, Form, Switch } from 'antd'
import { LockOutlined, ReloadOutlined, TeamOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { permissionAPI, orgAPI } from '../services/api'

const { Title, Text } = Typography
const { Option } = Select

interface Role {
  id: string
  name: string
  description: string
}

interface Department {
  id: string
  name: string
  parent_id: string
  children?: Department[]
}

const DataPermission: React.FC = () => {
  const [selectedRole, setSelectedRole] = useState<string>('1')
  const [selectedDepartments, setSelectedDepartments] = useState<string[]>([])
  const [isAllDepartments, setIsAllDepartments] = useState(true)

  const { data: rolesData, isLoading: rolesLoading } = useQuery({
    queryKey: ['roles'],
    queryFn: () => permissionAPI.getRoles(),
  })

  const { data: departmentTreeData, isLoading: departmentsLoading } = useQuery({
    queryKey: ['department-tree'],
    queryFn: () => orgAPI.getDepartmentTree(),
  })

  const handleSave = () => {
    message.success('数据权限保存成功')
  }

  const handleRoleChange = (value: string) => {
    setSelectedRole(value)
    // 模拟根据角色加载数据权限
    setIsAllDepartments(true)
    setSelectedDepartments([])
  }

  const handleAllDepartmentsChange = (checked: boolean) => {
    setIsAllDepartments(checked)
    if (checked) {
      setSelectedDepartments([])
    }
  }

  const renderDepartmentTree = (departments: Department[]): any[] => {
    return departments.map((dept) => ({
      title: dept.name,
      key: dept.id,
      children: dept.children && dept.children.length > 0 ? renderDepartmentTree(dept.children) : undefined,
    }))
  }

  if (rolesLoading || departmentsLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div>
      <Title level={4}>数据权限</Title>
      <Card>
        <div style={{ marginBottom: 16, display: 'flex', gap: 16, alignItems: 'center' }}>
          <Text strong>选择角色：</Text>
          <Select
            style={{ width: 200 }}
            value={selectedRole}
            onChange={handleRoleChange}
          >
            {rolesData?.data?.items?.map((role: Role) => (
              <Option key={role.id} value={role.id}>
                {role.name}
              </Option>
            ))}
          </Select>
          <Button type="primary" onClick={handleSave} style={{ marginLeft: 'auto' }}>
            保存权限
          </Button>
        </div>

        <Form layout="vertical">
          <Form.Item label="数据范围">
            <Form.Item name="all_departments" valuePropName="checked" noStyle>
              <Switch
                checked={isAllDepartments}
                onChange={handleAllDepartmentsChange}
                checkedChildren="全部部门"
                unCheckedChildren="指定部门"
              />
            </Form.Item>
          </Form.Item>

          {!isAllDepartments && (
            <Form.Item label="指定部门">
              <div style={{ border: '1px solid #f0f0f0', borderRadius: 4, padding: 16, minHeight: 400 }}>
                <Tree
                  checkable
                  treeData={renderDepartmentTree(departmentTreeData?.data?.tree || [])}
                  checkedKeys={selectedDepartments}
                  onCheck={(checked) => setSelectedDepartments(checked as string[])}
                  defaultExpandAll
                />
              </div>
            </Form.Item>
          )}

          <div style={{ marginTop: 24, padding: 16, backgroundColor: '#f9f9f9', borderRadius: 4 }}>
            <Text strong>权限说明：</Text>
            <ul style={{ marginTop: 8, marginLeft: 20 }}>
              <li>全部部门：可以查看所有部门的数据</li>
              <li>指定部门：只能查看选中部门及其子部门的数据</li>
              <li>部门负责人：默认可以查看自己负责部门及其子部门的数据</li>
            </ul>
          </div>
        </Form>
      </Card>
    </div>
  )
}

export default DataPermission