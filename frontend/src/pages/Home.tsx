import React from 'react'
import { Card, Row, Col, Statistic, Typography, Spin, Empty, Alert, Button } from 'antd'
import { UserOutlined, TeamOutlined, ClockCircleOutlined, FileOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { userAPI, departmentAPI, attendanceAPI, approvalAPI } from '../services/api'

const { Title } = Typography

const Home: React.FC = () => {
  // 获取员工总数
  const { data: usersData, isLoading: usersLoading, isError: usersError } = useQuery({
    queryKey: ['users'],
    queryFn: () => userAPI.getUsers({ page: 1, page_size: 1 })
  })

  // 获取部门总数
  const { data: departmentsData, isLoading: deptsLoading, isError: deptsError } = useQuery({
    queryKey: ['departments'],
    queryFn: departmentAPI.getDepartments
  })

  // 获取考勤统计
  const { data: attendanceData, isLoading: attendanceLoading, isError: attendanceError } = useQuery({
    queryKey: ['attendanceStats'],
    queryFn: () => attendanceAPI.getStats({})
  })

  // 获取审批数量
  const { data: approvalsData, isLoading: approvalsLoading, isError: approvalsError } = useQuery({
    queryKey: ['approvals'],
    queryFn: () => approvalAPI.getInstances({ page: 1, page_size: 1 })
  })

  const isLoading = usersLoading || deptsLoading || attendanceLoading || approvalsLoading
  const isError = usersError || deptsError || attendanceError || approvalsError

  if (isLoading) {
    return (
      <div className="loading-container">
        <Spin size="large" />
      </div>
    )
  }

  if (isError) {
    return (
      <div className="error-container">
        <Alert message="加载失败" type="error" showIcon />
        <Button className="retry-button" onClick={() => window.location.reload()}>重试</Button>
      </div>
    )
  }

  // 提取数据
  const userCount = usersData?.data?.total || 0
  const departmentCount = departmentsData?.data?.departments?.length || 0
  const attendanceRate = attendanceData?.data?.summary?.normal_rate ? parseFloat(attendanceData.data.summary.normal_rate) : 0
  const approvalCount = approvalsData?.data?.total || 0

  return (
    <div>
      <Title level={4}>系统概览</Title>
      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <Statistic title="员工总数" value={userCount} prefix={<UserOutlined />} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="部门总数" value={departmentCount} prefix={<TeamOutlined />} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="考勤率" value={attendanceRate} suffix="%" prefix={<ClockCircleOutlined />} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="审批数量" value={approvalCount} prefix={<FileOutlined />} />
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default Home