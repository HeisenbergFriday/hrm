import React, { useState, useEffect } from 'react'
import { Typography, Spin, Button, Select, Tree, message, Card, Space, Tag, Alert, Divider } from 'antd'
import { MenuOutlined, SaveOutlined, CheckCircleOutlined, SafetyOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { permissionAPI } from '../services/api'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'

const { Text, Title } = Typography
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
  const [selectedRole, setSelectedRole] = useState<string>('')
  const [checkedKeys, setCheckedKeys] = useState<string[]>([])

  const { data: rolesData, isLoading: rolesLoading } = useQuery({
    queryKey: ['roles'],
    queryFn: () => permissionAPI.getRoles(),
  })

  const { data: permissionsData, isLoading: permissionsLoading } = useQuery({
    queryKey: ['permissions'],
    queryFn: () => permissionAPI.getPermissions(),
  })

  // 默认选中第一个角色
  useEffect(() => {
    if (rolesData?.data?.items?.length && !selectedRole) {
      setSelectedRole(rolesData.data.items[0].id)
    }
  }, [rolesData, selectedRole])

  const menuItems: MenuItem[] = [
    {
      title: '首页',
      key: 'home',
    },
    {
      title: '组织管理',
      key: 'organization',
      children: [
        { title: '人才管理驾驶舱', key: 'organization-dashboard' },
        { title: '组织架构', key: 'department-tree' },
        { title: '组织花名册', key: 'employees' },
        { title: '员工档案', key: 'employee-profile' },
        { title: '入转调离', key: 'employee-flow' },
        { title: '人才分析', key: 'talent-analysis' },
        { title: '同步日志', key: 'sync-log' },
      ],
    },
    {
      title: '考勤管理',
      key: 'attendance',
      children: [
        { title: '考勤查询', key: 'attendance' },
        { title: '异常统计', key: 'attendance-stats' },
        { title: '导出记录', key: 'attendance-export' },
        { title: '大小周与节假日', key: 'week-schedule' },
        { title: '员工下班时间', key: 'employee-shift-config' },
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
      key: 'role-management',
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
    {
      title: '年假与调休',
      key: 'leave-overtime',
    },
    {
      title: '绩效管理',
      key: 'performance',
      children: [
        { title: '绩效活动', key: 'performance-overview' },
        { title: '指标库管理', key: 'performance-indicator-library' },
      ],
    },
    {
      title: '系统设置',
      key: 'setting',
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
      <PageContainer title="菜单权限" icon={<MenuOutlined />} subtitle="配置角色的菜单访问权限">
        <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
          <Spin size="large" />
        </div>
      </PageContainer>
    )
  }

  return (
    <PageContainer title="菜单权限" icon={<MenuOutlined />} subtitle="配置角色的菜单访问权限">
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
                {rolesData?.data?.items?.map((role: any) => (
                  <Option key={role.id} value={role.id}>
                    <Space>
                      <SafetyOutlined style={{ color: 'var(--color-primary)' }} />
                      {role.name}
                    </Space>
                  </Option>
                ))}
              </Select>
              <Tag icon={<CheckCircleOutlined />} color="success" style={{ marginLeft: 8 }}>
                已选择 {checkedKeys.length} 项权限
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

        <Card
          title={
            <Space>
              <MenuOutlined style={{ color: 'var(--color-primary)' }} />
              <Text strong>菜单列表</Text>
            </Space>
          }
          extra={
            <Button
              type="link"
              onClick={() => setCheckedKeys(menuItems.flatMap(item => [item.key, ...(item.children?.map(c => c.key) || [])]))}
            >
              全选
            </Button>
          }
          style={{ minHeight: 400 }}
        >
          <Alert
            message="勾选菜单后，对应角色将拥有访问权限"
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
          />
          <div style={{ minHeight: 350 }}>
            <Tree
              checkable
              treeData={menuItems}
              checkedKeys={checkedKeys}
              onCheck={(checked, info) => setCheckedKeys(checked as string[])}
              defaultExpandAll
              style={{ fontSize: 14 }}
            />
          </div>
        </Card>
      </PageCard>
    </PageContainer>
  )
}

export default MenuPermission
