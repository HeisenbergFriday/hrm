import React, { useState } from 'react'
import { Typography, Spin, Button, Select, Tree, message, Form, Switch, Card, Space, Tag, Alert, Divider, Radio } from 'antd'
import { LockOutlined, SaveOutlined, SafetyOutlined, TeamOutlined, InfoCircleOutlined, GlobalOutlined, ApartmentOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { permissionAPI, orgAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'

const { Text } = Typography
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
      <PageContainer title="数据权限" icon={<LockOutlined />} subtitle="配置角色的数据访问范围">
        <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
          <Spin size="large" />
        </div>
      </PageContainer>
    )
  }

  return (
    <PageContainer title="数据权限" icon={<LockOutlined />} subtitle="配置角色的数据访问范围">
      <PageCard>
        <Card
          style={{ marginBottom: 16 }}
          styles={{ body: { padding: '16px 24px' } }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Space size="large" align="center">
              <Text strong style={{ fontSize: 14 }}>选择角色：</Text>
              <Select
                style={{ width: 200 }}
                value={selectedRole}
                onChange={handleRoleChange}
                placeholder="请选择角色"
                suffixIcon={<SafetyOutlined style={{ color: 'var(--color-primary)' }} />}
              >
                {rolesData?.data?.items?.map((role: Role) => (
                  <Option key={role.id} value={role.id}>
                    <Space>
                      <SafetyOutlined style={{ color: 'var(--color-primary)' }} />
                      {role.name}
                    </Space>
                  </Option>
                ))}
              </Select>
              <Tag
                icon={isAllDepartments ? <GlobalOutlined /> : <ApartmentOutlined />}
                color={isAllDepartments ? 'success' : 'processing'}
                style={{ marginLeft: 8 }}
              >
                {isAllDepartments ? '全部部门' : `已选择 ${selectedDepartments.length} 个部门`}
              </Tag>
            </Space>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              onClick={handleSave}
              size="large"
            >
              保存权限
            </Button>
          </div>
        </Card>

        <Form layout="vertical">
          <Card
            title={
              <Space>
                <LockOutlined style={{ color: 'var(--color-primary)' }} />
                <Text strong>数据范围设置</Text>
              </Space>
            }
            style={{ marginBottom: 16 }}
          >
            <Form.Item label="数据范围">
              <Radio.Group
                value={isAllDepartments ? 'all' : 'custom'}
                onChange={(e) => handleAllDepartmentsChange(e.target.value === 'all')}
                optionType="button"
                buttonStyle="solid"
              >
                <Radio.Button value="all">
                  <Space>
                    <GlobalOutlined />
                    全部部门
                  </Space>
                </Radio.Button>
                <Radio.Button value="custom">
                  <Space>
                    <ApartmentOutlined />
                    指定部门
                  </Space>
                </Radio.Button>
              </Radio.Group>
            </Form.Item>

            {!isAllDepartments && (
              <>
                <Divider orientation="left">选择部门</Divider>
                <Card
                  style={{ minHeight: 350, backgroundColor: 'var(--color-bg-container)' }}
                  styles={{ body: { padding: 16 } }}
                >
                  <Tree
                    checkable
                    treeData={renderDepartmentTree(departmentTreeData?.data?.tree || [])}
                    checkedKeys={selectedDepartments}
                    onCheck={(checked) => setSelectedDepartments(checked as string[])}
                    defaultExpandAll
                    style={{ fontSize: 14 }}
                  />
                </Card>
              </>
            )}
          </Card>

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
        </Form>
      </PageCard>
    </PageContainer>
  )
}

export default DataPermission
