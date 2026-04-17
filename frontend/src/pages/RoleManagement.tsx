import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, Modal, Form, Input, message } from 'antd'
import { UsergroupAddOutlined, PlusOutlined, EditOutlined, DeleteOutlined, ReloadOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { permissionAPI } from '../services/api'

const { Title, Text } = Typography

interface Role {
  id: string
  name: string
  description: string
  created_at: string
  updated_at: string
}

const RoleManagement: React.FC = () => {
  const [modalVisible, setModalVisible] = useState(false)
  const [form] = Form.useForm()

  const { data: rolesData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['roles'],
    queryFn: () => permissionAPI.getRoles(),
  })

  const createRoleMutation = useMutation({
    mutationFn: (data: { name: string; description: string }) => permissionAPI.createRole(data),
    onSuccess: () => {
      message.success('角色创建成功')
      setModalVisible(false)
      form.resetFields()
      refetch()
    },
    onError: (error) => {
      message.error('角色创建失败')
    },
  })

  const handleCreateRole = () => {
    form.validateFields().then((values) => {
      createRoleMutation.mutate(values)
    })
  }

  const columns = [
    {
      title: '角色名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string) => <Text strong>{text}</Text>,
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      key: 'updated_at',
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Role) => (
        <div style={{ display: 'flex', gap: 8 }}>
          <Button type="link" icon={<EditOutlined />}>编辑</Button>
          <Button type="link" danger icon={<DeleteOutlined />}>删除</Button>
        </div>
      ),
    },
  ]

  return (
    <div>
      <Title level={4}>角色管理</Title>
      <Card
        extra={
          <div style={{ display: 'flex', gap: 8 }}>
            <Button icon={<ReloadOutlined />} onClick={() => refetch()} loading={isLoading}>
              刷新
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalVisible(true)}>
              新建角色
            </Button>
          </div>
        }
      >
        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <div style={{ padding: '20px' }}>
            <Alert
              message="加载失败"
              description={(error as Error)?.message || '获取角色列表失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()}>
                  重试
                </Button>
              }
            />
          </div>
        ) : rolesData?.data?.items?.length ? (
          <Table
            columns={columns}
            dataSource={rolesData.data.items as Role[]}
            rowKey="id"
            pagination={{
              showTotal: (total: number) => `共 ${total} 个角色`,
            }}
          />
        ) : (
          <Empty description="暂无角色" />
        )}
      </Card>

      <Modal
        title="新建角色"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setModalVisible(false)}>
            取消
          </Button>,
          <Button
            key="submit"
            type="primary"
            onClick={handleCreateRole}
            loading={createRoleMutation.isPending}
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
    </div>
  )
}

export default RoleManagement