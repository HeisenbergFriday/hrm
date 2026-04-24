import React, { useState } from 'react'
import { Card, Typography, Row, Col, Tree, Table, Spin, Empty, Alert, Button, Modal, Form, Input } from 'antd'
import { UserOutlined, TeamOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { orgAPI, departmentAPI } from '../services/api'

const { Title } = Typography

// 转换部门数据为树结构
const transformDepartmentsToTree = (departments: any[]) => {
  const nodeMap: Record<string, any> = {}
  const rootNodes: any[] = []
  
  // 构建节点映射
  departments.forEach(dept => {
    nodeMap[dept.department_id] = {
      title: dept.name,
      value: dept.department_id,
      children: [],
    }
  })
  
  // 构建树结构
  departments.forEach(dept => {
    const node = nodeMap[dept.department_id]
    if (dept.parent_id && nodeMap[dept.parent_id]) {
      nodeMap[dept.parent_id].children.push(node)
    } else {
      rootNodes.push(node)
    }
  })
  
  return rootNodes
}

const Organization: React.FC = () => {
  const [selectedDepartment, setSelectedDepartment] = useState<string>('')
  const [isModalVisible, setIsModalVisible] = useState(false)
  const [editingUser, setEditingUser] = useState<any>(null)

  const { data: departmentsData, isLoading: departmentsLoading, isError: departmentsError, refetch: refetchDepartments } = useQuery({
    queryKey: ['departments'],
    queryFn: () => departmentAPI.getDepartments()
  })
  
  // 转换部门数据为树结构
  const departments = departmentsData?.data?.departments ? transformDepartmentsToTree(departmentsData.data.departments) : []
  
  const { data: users, isLoading: usersLoading, isError: usersError, refetch: refetchUsers } = useQuery({
    queryKey: ['users', selectedDepartment],
    queryFn: () => orgAPI.getEmployees({ department_id: selectedDepartment })
  })

  const handleDepartmentSelect = (keys: string[]) => {
    setSelectedDepartment(keys[0] || '')
  }

  const handleEditUser = (user: any) => {
    setEditingUser(user)
    setIsModalVisible(true)
  }

  const handleModalOk = () => {
    setIsModalVisible(false)
    setEditingUser(null)
  }

  const handleModalCancel = () => {
    setIsModalVisible(false)
    setEditingUser(null)
  }

  const columns = [
    { title: '姓名', dataIndex: 'name', key: 'name' },
    { title: '邮箱', dataIndex: 'email', key: 'email' },
    { title: '手机号', dataIndex: 'mobile', key: 'mobile' },
    { title: '职位', dataIndex: 'position', key: 'position' },
    { 
      title: '操作', 
      key: 'action',
      render: (_: any, record: any) => (
        <>
          <Button icon={<EditOutlined />} size="small" onClick={() => handleEditUser(record)} />
          <Button icon={<DeleteOutlined />} size="small" danger />
        </>
      )
    },
  ]

  return (
    <div>
      <Title level={4}>组织架构</Title>
      <Row gutter={16}>
        <Col span={6}>
          <Card title="部门树" style={{ height: '100%' }}>
            {departmentsLoading ? (
              <div className="loading-container">
                <Spin size="small" />
              </div>
            ) : departmentsError ? (
              <div className="error-container">
                <Alert message="加载失败" type="error" showIcon />
                <Button className="retry-button" onClick={() => refetchDepartments()}>重试</Button>
              </div>
            ) : departments?.length ? (
              <Tree
                treeData={departments}
                onSelect={handleDepartmentSelect}
                defaultExpandAll
              />
            ) : (
              <div className="empty-container">
                <Empty description="暂无部门数据" />
              </div>
            )}
          </Card>
        </Col>
        <Col span={18}>
          <Card title="员工列表">
            {usersLoading ? (
              <div className="loading-container">
                <Spin size="small" />
              </div>
            ) : usersError ? (
              <div className="error-container">
                <Alert message="加载失败" type="error" showIcon />
                <Button className="retry-button" onClick={() => refetchUsers()}>重试</Button>
              </div>
            ) : users?.data?.items?.length ? (
              <Table
                columns={columns}
                dataSource={users.data.items}
                rowKey="id"
                pagination={{ total: users.data.total, pageSize: 10 }}
              />
            ) : (
              <div className="empty-container">
                <Empty description="暂无员工数据" />
              </div>
            )}
          </Card>
        </Col>
      </Row>

      <Modal
        title="编辑员工"
        open={isModalVisible}
        onOk={handleModalOk}
        onCancel={handleModalCancel}
        okText="确定"
        cancelText="取消"
      >
        <Form
          initialValues={editingUser}
        >
          <Form.Item label="姓名" name="name">
            <Input />
          </Form.Item>
          <Form.Item label="邮箱" name="email">
            <Input />
          </Form.Item>
          <Form.Item label="手机号" name="mobile">
            <Input />
          </Form.Item>
          <Form.Item label="职位" name="position">
            <Input />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Organization
