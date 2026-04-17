import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, Tag, Modal, Descriptions, Space } from 'antd'
import { FileOutlined, EditOutlined, DeleteOutlined, EyeOutlined, SyncOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { approvalAPI } from '../services/api'

const { Title, Text } = Typography

interface ApprovalTemplate {
  id: string
  template_id: string
  name: string
  description: string
  category: string
  status: string
  form_items: {
    items: Array<{
      name: string
      type: string
      options?: string[]
    }>
  }
  flow_nodes: {
    nodes: Array<{
      name: string
      type: string
      level: number
    }>
  }
  extension: any
  created_at: string
  updated_at: string
}

const ApprovalTemplate: React.FC = () => {
  const [selectedTemplate, setSelectedTemplate] = useState<ApprovalTemplate | null>(null)
  const [modalVisible, setModalVisible] = useState(false)

  const { data: templatesData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['approval-templates'],
    queryFn: () => approvalAPI.getTemplates(),
  })

  const syncMutation = useMutation({
    mutationFn: () => approvalAPI.sync(),
    onSuccess: () => {
      refetch()
    },
  })

  const handleViewTemplate = (template: ApprovalTemplate) => {
    setSelectedTemplate(template)
    setModalVisible(true)
  }

  const columns = [
    {
      title: '模板名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string) => (
        <Text strong>{text}</Text>
      ),
    },
    {
      title: '模板ID',
      dataIndex: 'template_id',
      key: 'template_id',
    },
    {
      title: '分类',
      dataIndex: 'category',
      key: 'category',
      render: (category: string) => (
        <Tag>{category}</Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag color={status === 'active' ? 'green' : 'red'}>
          {status === 'active' ? '启用' : '禁用'}
        </Tag>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: ApprovalTemplate) => (
        <Space>
          <Button
            type="link"
            icon={<EyeOutlined />}
            onClick={() => handleViewTemplate(record)}
          >
            查看
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Title level={4}>审批模板</Title>
      <Card
        extra={
          <Button
            icon={<SyncOutlined />}
            onClick={() => syncMutation.mutate()}
            loading={syncMutation.isPending}
          >
            同步模板
          </Button>
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
              description={(error as Error)?.message || '获取审批模板失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()}>
                  重试
                </Button>
              }
            />
          </div>
        ) : templatesData?.data?.items?.length ? (
          <Table
            columns={columns}
            dataSource={templatesData.data.items as ApprovalTemplate[]}
            rowKey="id"
            pagination={{
              showTotal: (total: number) => `共 ${total} 个模板`,
            }}
          />
        ) : (
          <Empty description="暂无审批模板" />
        )}
      </Card>

      <Modal
        title="审批模板详情"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={[
          <Button key="close" onClick={() => setModalVisible(false)}>
            关闭
          </Button>,
        ]}
        width={800}
      >
        {selectedTemplate && (
          <div>
            <Descriptions bordered column={1}>
              <Descriptions.Item label="模板名称">{selectedTemplate.name}</Descriptions.Item>
              <Descriptions.Item label="模板ID">{selectedTemplate.template_id}</Descriptions.Item>
              <Descriptions.Item label="分类">{selectedTemplate.category}</Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag color={selectedTemplate.status === 'active' ? 'green' : 'red'}>
                  {selectedTemplate.status === 'active' ? '启用' : '禁用'}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="描述">{selectedTemplate.description}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{selectedTemplate.created_at}</Descriptions.Item>
              <Descriptions.Item label="更新时间">{selectedTemplate.updated_at}</Descriptions.Item>
            </Descriptions>

            <div style={{ marginTop: 24 }}>
              <Title level={5}>表单字段</Title>
              <div style={{ border: '1px solid #f0f0f0', borderRadius: 4, padding: 16 }}>
                {selectedTemplate.form_items?.items?.map((item, index) => (
                  <div key={index} style={{ marginBottom: 12, paddingBottom: 12, borderBottom: '1px dashed #f0f0f0' }}>
                    <div style={{ display: 'flex', alignItems: 'center' }}>
                      <span style={{ fontWeight: 'bold', marginRight: 12 }}>{item.name}</span>
                      <Tag>{item.type}</Tag>
                    </div>
                    {item.options && item.options.length > 0 && (
                      <div style={{ marginTop: 8, marginLeft: 12 }}>
                        <Text type="secondary">选项: {item.options.join(', ')}</Text>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>

            <div style={{ marginTop: 24 }}>
              <Title level={5}>审批节点</Title>
              <div style={{ border: '1px solid #f0f0f0', borderRadius: 4, padding: 16 }}>
                {selectedTemplate.flow_nodes?.nodes?.map((node, index) => (
                  <div key={index} style={{ marginBottom: 12, paddingBottom: 12, borderBottom: '1px dashed #f0f0f0' }}>
                    <div style={{ display: 'flex', alignItems: 'center' }}>
                      <span style={{ fontWeight: 'bold', marginRight: 12 }}>第 {node.level} 级: {node.name}</span>
                      <Tag>{node.type}</Tag>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}

export default ApprovalTemplate