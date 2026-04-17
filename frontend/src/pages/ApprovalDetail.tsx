import React, { useState } from 'react'
import { Card, Typography, Descriptions, Timeline, Tag, Button, Spin, Alert, Divider } from 'antd'
import { ArrowLeftOutlined, CheckCircleOutlined, CloseCircleOutlined, SyncOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { approvalAPI } from '../services/api'

const { Title, Text, Paragraph } = Typography

interface FlowNode {
  node_name: string
  approver_id: string
  approver_name: string
  action: string
  comment: string
  time: string
}

const ApprovalDetail: React.FC = () => {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()

  const { data: approvalData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['approval-detail', id],
    queryFn: () => approvalAPI.getApproval(id!),
  })

  const syncMutation = useMutation({
    mutationFn: () => approvalAPI.sync(),
    onSuccess: () => {
      refetch()
    },
  })

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'completed':
        return <Tag color="green">已完成</Tag>
      case 'in_progress':
        return <Tag color="blue">处理中</Tag>
      case 'rejected':
        return <Tag color="red">已拒绝</Tag>
      case 'pending':
        return <Tag color="orange">待处理</Tag>
      default:
        return <Tag>{status}</Tag>
    }
  }

  const getActionIcon = (action: string) => {
    if (action === 'approved') {
      return <CheckCircleOutlined style={{ color: '#52c41a' }} />
    } else if (action === 'rejected') {
      return <CloseCircleOutlined style={{ color: '#ff4d4f' }} />
    }
    return null
  }

  const getActionText = (action: string) => {
    switch (action) {
      case 'approved':
        return '已通过'
      case 'rejected':
        return '已拒绝'
      case 'pending':
        return '待处理'
      default:
        return action
    }
  }

  return (
    <div>
      <Title level={4}>审批详情</Title>
      <Card>
        <Button
          icon={<ArrowLeftOutlined />}
          onClick={() => navigate('/approval-instances')}
          style={{ marginBottom: 24 }}
        >
          返回列表
        </Button>

        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <div style={{ padding: '20px' }}>
            <Alert
              message="加载失败"
              description={(error as Error)?.message || '获取审批详情失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()}>
                  重试
                </Button>
              }
            />
          </div>
        ) : approvalData?.data?.approval ? (
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
              <div>
                <Title level={5}>{approvalData.data.approval.title}</Title>
                <Text type="secondary">模板：{approvalData.data.approval.template_name}</Text>
              </div>
              {getStatusTag(approvalData.data.approval.status)}
            </div>

            <Descriptions bordered column={1} style={{ marginBottom: 24 }}>
              <Descriptions.Item label="发起人">{approvalData.data.approval.applicant_name}</Descriptions.Item>
              <Descriptions.Item label="发起时间">{approvalData.data.approval.create_time}</Descriptions.Item>
              {approvalData.data.approval.finish_time && (
                <Descriptions.Item label="结束时间">{approvalData.data.approval.finish_time}</Descriptions.Item>
              )}
            </Descriptions>

            <Title level={5}>审批内容</Title>
            <div style={{ border: '1px solid #f0f0f0', borderRadius: 4, padding: 16, marginBottom: 24 }}>
              {Object.entries(approvalData.data.approval.content || {}).map(([key, value]) => (
                <div key={key} style={{ marginBottom: 12 }}>
                  <Text strong>{key}：</Text>
                  <Text>{value}</Text>
                </div>
              ))}
            </div>

            <Title level={5}>审批流程</Title>
            <Timeline
              items={approvalData.data.approval.flow_history?.map((node: FlowNode, index: number) => ({
                color: node.action === 'approved' ? 'green' : node.action === 'rejected' ? 'red' : 'blue',
                children: (
                  <div>
                    <div style={{ display: 'flex', alignItems: 'center' }}>
                      <Text strong>{node.node_name}</Text>
                      <Text style={{ marginLeft: 12 }}>{node.approver_name}</Text>
                      {getActionIcon(node.action)}
                      <Text style={{ marginLeft: 8, color: node.action === 'approved' ? '#52c41a' : node.action === 'rejected' ? '#ff4d4f' : '#1890ff' }}>
                        {getActionText(node.action)}
                      </Text>
                    </div>
                    {node.comment && (
                      <Paragraph style={{ marginTop: 8, marginBottom: 0 }}>
                        备注：{node.comment}
                      </Paragraph>
                    )}
                    <Text type="secondary" style={{ fontSize: 12 }}>{node.time}</Text>
                  </div>
                ),
              })) || []}
            />

            <Divider />
            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <Button
                icon={<SyncOutlined />}
                onClick={() => syncMutation.mutate()}
                loading={syncMutation.isPending}
              >
                同步数据
              </Button>
            </div>
          </div>
        ) : (
          <Empty description="审批详情不存在" />
        )}
      </Card>
    </div>
  )
}

export default ApprovalDetail