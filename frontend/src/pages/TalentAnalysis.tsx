import React, { useState } from 'react'
import { Card, Typography, Table, Spin, Empty, Alert, Button, Modal, Form, Input, Select, DatePicker, message, Tabs, Descriptions, Statistic, Row, Col, Progress } from 'antd'
import { UserOutlined, StarOutlined, AlertOutlined, ReloadOutlined, PlusOutlined, LineChartOutlined } from '@ant-design/icons'
import { useQuery, useMutation } from '@tanstack/react-query'
import { talentAPI } from '../services/api'

const { Title, Text } = Typography
const { Option } = Select

interface TalentAnalysis {
  id: string
  user_id: string
  user_name: string
  department_id: string
  department_name: string
  position: string
  performance_score: number
  performance_level: string
  performance_review: string
  skills_assessment: any
  potential_score: number
  potential_level: string
  training_records: any
  promotion_records: any
  turnover_risk_score: number
  turnover_risk_level: string
  analysis_date: string
  created_at: string
  updated_at: string
}

const TalentAnalysis: React.FC = () => {
  const [modalVisible, setModalVisible] = useState(false)
  const [currentAnalysis, setCurrentAnalysis] = useState<TalentAnalysis | null>(null)
  const [form] = Form.useForm()
  const [activeTab, setActiveTab] = useState('list')

  const { data: analysisData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['talent-analysis'],
    queryFn: () => talentAPI.getAnalysis(),
  })

  const createAnalysisMutation = useMutation({
    mutationFn: (data: any) => talentAPI.createAnalysis(data),
    onSuccess: () => {
      message.success('人才分析创建成功')
      setModalVisible(false)
      form.resetFields()
      refetch()
    },
    onError: () => {
      message.error('人才分析创建失败')
    },
  })

  const handleCreateAnalysis = () => {
    form.validateFields().then((values) => {
      createAnalysisMutation.mutate(values)
    })
  }

  const handleViewAnalysis = (analysis: TalentAnalysis) => {
    setCurrentAnalysis(analysis)
    setActiveTab('detail')
  }

  const getLevelType = (level: string) => {
    switch (level) {
      case '优秀':
      case '高':
        return 'success'
      case '良好':
      case '中':
        return 'success'
      case '一般':
      case '低':
        return 'warning'
      case '差':
        return 'danger'
      default:
        return 'success'
    }
  }

  const columns = [
    {
      title: '员工姓名',
      dataIndex: 'user_name',
      key: 'user_name',
      render: (text: string) => <Text strong>{text}</Text>,
    },
    {
      title: '部门',
      dataIndex: 'department_name',
      key: 'department_name',
    },
    {
      title: '职位',
      dataIndex: 'position',
      key: 'position',
    },
    {
      title: '绩效得分',
      dataIndex: 'performance_score',
      key: 'performance_score',
      render: (score: number) => (
        <div>
          <Text>{score}</Text>
          <Progress percent={score} size="small" style={{ marginTop: 4 }} />
        </div>
      ),
    },
    {
      title: '绩效等级',
      dataIndex: 'performance_level',
      key: 'performance_level',
      render: (level: string) => (
        <Text type={getLevelType(level)} strong>{level}</Text>
      ),
    },
    {
      title: '潜力得分',
      dataIndex: 'potential_score',
      key: 'potential_score',
      render: (score: number) => (
        <div>
          <Text>{score}</Text>
          <Progress percent={score} size="small" style={{ marginTop: 4 }} />
        </div>
      ),
    },
    {
      title: '潜力等级',
      dataIndex: 'potential_level',
      key: 'potential_level',
      render: (level: string) => (
        <Text type={getLevelType(level)} strong>{level}</Text>
      ),
    },
    {
      title: '离职风险',
      dataIndex: 'turnover_risk_level',
      key: 'turnover_risk_level',
      render: (level: string) => (
        <Text type={level === '高' ? 'danger' : level === '中' ? 'warning' : 'success'} strong>{level}</Text>
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: TalentAnalysis) => (
        <Button type="link" onClick={() => handleViewAnalysis(record)}>
          查看
        </Button>
      ),
    },
  ]

  const renderSkillsRadar = (skills: any[]) => {
    return (
      <div style={{ padding: 16 }}>
        {skills.map((skill, index) => (
          <div key={index} style={{ marginBottom: 12 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
              <Text>{skill.name}</Text>
              <Text>{skill.score}/100</Text>
            </div>
            <Progress percent={skill.score} />
          </div>
        ))}
      </div>
    )
  }

  const renderPerformanceBar = () => {
    const data = [
      { name: '优秀', value: 3 },
      { name: '良好', value: 5 },
      { name: '一般', value: 2 },
      { name: '差', value: 1 },
    ]

    return (
      <div style={{ padding: 16 }}>
        {data.map((item, index) => (
          <div key={index} style={{ marginBottom: 12 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
              <Text>{item.name}</Text>
              <Text>{item.value}人</Text>
            </div>
            <Progress percent={(item.value / 11) * 100} />
          </div>
        ))}
      </div>
    )
  }

  return (
    <div>
      <Title level={4}>人才分析</Title>
      {activeTab === 'list' ? (
        <Card
          extra={
            <div style={{ display: 'flex', gap: 8 }}>
              <Button icon={<ReloadOutlined />} onClick={() => refetch()} loading={isLoading}>
                刷新
              </Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalVisible(true)}>
                新建分析
              </Button>
            </div>
          }
        >
          <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
            <Col span={6}>
              <Card>
                <Statistic title="总分析人数" value={10} prefix={<UserOutlined />} />
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic title="优秀员工" value={3} prefix={<StarOutlined />} valueStyle={{ color: '#52c41a' }} />
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic title="高潜力员工" value={4} prefix={<LineChartOutlined />} valueStyle={{ color: '#1890ff' }} />
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic title="离职风险" value={2} prefix={<AlertOutlined />} valueStyle={{ color: '#faad14' }} />
              </Card>
            </Col>
          </Row>

          <Card title="绩效分布" style={{ marginBottom: 24 }}>
            {renderPerformanceBar()}
          </Card>

          {isLoading ? (
            <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
              <Spin size="large" />
            </div>
          ) : isError ? (
            <div style={{ padding: '20px' }}>
              <Alert
                message="加载失败"
                description={(error as Error)?.message || '获取人才分析失败，请稍后重试'}
                type="error"
                showIcon
                action={
                  <Button size="small" onClick={() => refetch()}>
                    重试
                  </Button>
                }
              />
            </div>
          ) : analysisData?.data?.items?.length ? (
            <Table
              columns={columns}
              dataSource={analysisData.data.items as TalentAnalysis[]}
              rowKey="id"
              pagination={{
                showTotal: (total: number) => `共 ${total} 条分析记录`,
              }}
            />
          ) : (
            <Empty description="暂无人才分析记录" />
          )}
        </Card>
      ) : (
        <Card
          title={
            <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
              <Text strong style={{ fontSize: 18 }}>{currentAnalysis?.user_name} - 人才分析</Text>
              <Button type="primary" onClick={() => setActiveTab('list')} style={{ marginLeft: 'auto' }}>
                返回列表
              </Button>
            </div>
          }
        >
          <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
            <Col span={6}>
              <Card>
                <Statistic title="绩效得分" value={currentAnalysis?.performance_score} suffix="/100" />
                <div style={{ marginTop: 8 }}>
                  <Text strong type={getLevelType(currentAnalysis?.performance_level || '')}>
                    {currentAnalysis?.performance_level}
                  </Text>
                </div>
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic title="潜力得分" value={currentAnalysis?.potential_score} suffix="/100" />
                <div style={{ marginTop: 8 }}>
                  <Text strong type={getLevelType(currentAnalysis?.potential_level || '')}>
                    {currentAnalysis?.potential_level}
                  </Text>
                </div>
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic title="离职风险" value={currentAnalysis?.turnover_risk_score} suffix="/100" />
                <div style={{ marginTop: 8 }}>
                  <Text strong type={
                    currentAnalysis?.turnover_risk_level === '高' ? 'danger' : 
                    currentAnalysis?.turnover_risk_level === '中' ? 'warning' : 'success'
                  }>
                    {currentAnalysis?.turnover_risk_level}
                  </Text>
                </div>
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic title="分析日期" value={currentAnalysis?.analysis_date} />
              </Card>
            </Col>
          </Row>

          <Tabs defaultActiveKey="basic" style={{ marginTop: 24 }}>
            <Tabs.TabPane tab="基本信息" key="basic">
              <Descriptions column={2} bordered>
                <Descriptions.Item label="员工姓名" span={1}>{currentAnalysis?.user_name}</Descriptions.Item>
                <Descriptions.Item label="部门" span={1}>{currentAnalysis?.department_name}</Descriptions.Item>
                <Descriptions.Item label="职位" span={1}>{currentAnalysis?.position}</Descriptions.Item>
                <Descriptions.Item label="分析日期" span={1}>{currentAnalysis?.analysis_date}</Descriptions.Item>
                <Descriptions.Item label="绩效评价" span={2}>{currentAnalysis?.performance_review}</Descriptions.Item>
              </Descriptions>
            </Tabs.TabPane>
            <Tabs.TabPane tab="技能评估" key="skills">
              {currentAnalysis?.skills_assessment?.skills ? (
                renderSkillsRadar(currentAnalysis.skills_assessment.skills)
              ) : (
                <Empty description="暂无技能评估数据" />
              )}
            </Tabs.TabPane>
            <Tabs.TabPane tab="培训记录" key="training">
              {currentAnalysis?.training_records?.trainings?.map((training: any, index: number) => (
                <Card key={index} style={{ marginBottom: 12 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Text strong>{training.name}</Text>
                    <Text type="secondary">{training.date}</Text>
                  </div>
                  <div style={{ marginTop: 8 }}>
                    <Text>培训得分：{training.score}</Text>
                  </div>
                </Card>
              )) || <Empty description="暂无培训记录" />}
            </Tabs.TabPane>
            <Tabs.TabPane tab="晋升记录" key="promotion">
              {currentAnalysis?.promotion_records?.promotions?.map((promotion: any, index: number) => (
                <Card key={index} style={{ marginBottom: 12 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Text strong>{promotion.position}</Text>
                    <Text type="secondary">{promotion.date}</Text>
                  </div>
                </Card>
              )) || <Empty description="暂无晋升记录" />}
            </Tabs.TabPane>
          </Tabs>
        </Card>
      )}

      <Modal
        title="新建人才分析"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setModalVisible(false)}>
            取消
          </Button>,
          <Button
            key="submit"
            type="primary"
            onClick={handleCreateAnalysis}
            loading={createAnalysisMutation.isPending}
          >
            确认
          </Button>,
        ]}
        width={800}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="user_id"
            label="员工ID"
            rules={[{ required: true, message: '请输入员工ID' }]}
          >
            <Input placeholder="请输入员工ID" />
          </Form.Item>
          <Form.Item
            name="user_name"
            label="员工姓名"
            rules={[{ required: true, message: '请输入员工姓名' }]}
          >
            <Input placeholder="请输入员工姓名" />
          </Form.Item>
          <Form.Item
            name="department_id"
            label="部门ID"
            rules={[{ required: true, message: '请输入部门ID' }]}
          >
            <Input placeholder="请输入部门ID" />
          </Form.Item>
          <Form.Item
            name="department_name"
            label="部门名称"
            rules={[{ required: true, message: '请输入部门名称' }]}
          >
            <Input placeholder="请输入部门名称" />
          </Form.Item>
          <Form.Item
            name="position"
            label="职位"
            rules={[{ required: true, message: '请输入职位' }]}
          >
            <Input placeholder="请输入职位" />
          </Form.Item>
          <Form.Item
            name="performance_score"
            label="绩效得分"
            rules={[{ required: true, message: '请输入绩效得分' }]}
          >
            <Input type="number" placeholder="请输入绩效得分" />
          </Form.Item>
          <Form.Item
            name="performance_level"
            label="绩效等级"
            rules={[{ required: true, message: '请选择绩效等级' }]}
          >
            <Select placeholder="请选择绩效等级">
              <Option value="优秀">优秀</Option>
              <Option value="良好">良好</Option>
              <Option value="一般">一般</Option>
              <Option value="差">差</Option>
            </Select>
          </Form.Item>
          <Form.Item
            name="performance_review"
            label="绩效评价"
          >
            <Input.TextArea placeholder="请输入绩效评价" rows={4} />
          </Form.Item>
          <Form.Item
            name="potential_score"
            label="潜力得分"
            rules={[{ required: true, message: '请输入潜力得分' }]}
          >
            <Input type="number" placeholder="请输入潜力得分" />
          </Form.Item>
          <Form.Item
            name="potential_level"
            label="潜力等级"
            rules={[{ required: true, message: '请选择潜力等级' }]}
          >
            <Select placeholder="请选择潜力等级">
              <Option value="高">高</Option>
              <Option value="中">中</Option>
              <Option value="低">低</Option>
            </Select>
          </Form.Item>
          <Form.Item
            name="turnover_risk_score"
            label="离职风险得分"
            rules={[{ required: true, message: '请输入离职风险得分' }]}
          >
            <Input type="number" placeholder="请输入离职风险得分" />
          </Form.Item>
          <Form.Item
            name="turnover_risk_level"
            label="离职风险等级"
            rules={[{ required: true, message: '请选择离职风险等级' }]}
          >
            <Select placeholder="请选择离职风险等级">
              <Option value="高">高</Option>
              <Option value="中">中</Option>
              <Option value="低">低</Option>
            </Select>
          </Form.Item>
          <Form.Item
            name="analysis_date"
            label="分析日期"
            rules={[{ required: true, message: '请选择分析日期' }]}
          >
            <DatePicker style={{ width: '100%' }} placeholder="选择日期" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default TalentAnalysis