import React, { useState } from 'react'
import { Typography, Table, Spin, Empty, Alert, Button, DatePicker, Input, Select } from 'antd'
import { HistoryOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { auditAPI, userAPI } from '../services/api'
import dayjs from 'dayjs'
import PageContainer from '../components/PageContainer'
import PageCard from '../components/PageCard'
import { formatDateTime } from '../utils/format'

const { Text } = Typography
const { RangePicker } = DatePicker

interface AuditLog {
  id: string
  user_id: string
  user_name: string
  operation: string
  resource: string
  ip: string
  details: any
  created_at: string
}

const AuditLogs: React.FC = () => {
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null]>([null, null])
  const [searchText, setSearchText] = useState('')
  const [userID, setUserID] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)

  const queryParams = {
    page,
    page_size: pageSize,
    start_date: dateRange[0]?.format('YYYY-MM-DD'),
    end_date: dateRange[1]?.format('YYYY-MM-DD'),
    user_id: userID || undefined,
  }

  const { data: logsData, isLoading, isError, refetch, error } = useQuery({
    queryKey: ['audit-logs', queryParams],
    queryFn: () => auditAPI.getLogs(queryParams),
  })

  const { data: usersData } = useQuery({
    queryKey: ['users-list-audit'],
    queryFn: async () => {
      const res = await userAPI.getUsers({ page: 1, page_size: 200 })
      return res.data?.data?.users || []
    },
  })

  const columns = [
    {
      title: '操作人',
      dataIndex: 'user_name',
      key: 'user_name',
      render: (text: string, record: AuditLog) => (
        <div>
          <Text strong>{text}</Text>
          <div style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-tertiary)' }}>{record.user_id}</div>
        </div>
      ),
    },
    {
      title: '操作',
      dataIndex: 'operation',
      key: 'operation',
      render: (v: string) => {
        const map: Record<string, string> = {
          create_goal_template: '创建目标模板',
          create_template: '创建模板',
          update_template: '更新模板',
          delete_template: '删除模板',
          create_review: '创建评审',
          update_review: '更新评审',
          delete_review: '删除评审',
          create_activity: '创建绩效活动',
          update_activity: '更新绩效活动',
          batch_confirm: '批量确认',
          lock_activity: '锁定活动',
          unlock_activity: '解锁活动',
          archive_activity: '归档活动',
          sync_attendance: '同步考勤',
          sync_user: '同步用户',
          sync_department: '同步部门',
        }
        return map[v] || v
      },
    },
    {
      title: '资源',
      dataIndex: 'resource',
      key: 'resource',
      render: (v: string) => {
        const map: Record<string, string> = {
          performance_template: '绩效模板',
          performance_activity: '绩效活动',
          performance_review: '绩效评审',
          attendance: '考勤',
          user: '用户',
          department: '部门',
        }
        const prefix = v.split(':')[0]
        const suffix = v.includes(':') ? v.slice(v.indexOf(':')) : ''
        return (map[prefix] || prefix) + suffix
      },
    },
    {
      title: 'IP地址',
      dataIndex: 'ip',
      key: 'ip',
    },
    {
      title: '操作时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (v: string) => formatDateTime(v),
    },
    {
      title: '详情',
      key: 'details',
      render: (_: any, record: AuditLog) => {
        const keyMap: Record<string, string> = {
          applicable_cycles: '适用周期', department_id: '部门ID', department_name: '部门名称',
          template_id: '模板ID', template_name: '模板名称', status: '状态',
          activity_id: '活动ID', activity_name: '活动名称', user_id: '用户ID',
          user_name: '用户名', score: '分数', level: '等级', comment: '评论',
          review_id: '评审ID', goal_id: '目标ID', action: '操作', result: '结果',
        }
        const valMap: Record<string, string> = {
          monthly: '月度', quarterly: '季度', annual: '年度', weekly: '周度',
          active: '启用', inactive: '停用', draft: '草稿',
        }
        const translate = (obj: any): any => {
          if (Array.isArray(obj)) return obj.map(translate)
          if (obj && typeof obj === 'object') {
            return Object.fromEntries(
              Object.entries(obj).map(([k, v]) => [keyMap[k] || k, typeof v === 'string' ? (valMap[v] || v) : translate(v)])
            )
          }
          return obj
        }
        return <Text type="secondary" ellipsis>{JSON.stringify(translate(record.details))}</Text>
      },
    },
  ]

  return (
    <PageContainer
      title="审计日志"
      icon={<HistoryOutlined />}
      subtitle="查看系统操作审计记录"
    >
      <PageCard>
        <div style={{ marginBottom: 'var(--space-4)', display: 'flex', gap: 'var(--space-4)', alignItems: 'center', flexWrap: 'wrap' }}>
          <RangePicker onChange={setDateRange} />
          <Input
            placeholder="搜索操作"
            style={{ width: 200 }}
            prefix={<SearchOutlined />}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
          />
          <Select
            placeholder="操作人"
            style={{ width: 150 }}
            allowClear
            showSearch
            optionFilterProp="label"
            onChange={setUserID}
            options={(usersData || []).map((u: any) => ({ label: u.name || u.username, value: u.id }))}
          />
          <Button type="primary" onClick={() => refetch()}>
            查询
          </Button>
          <Button icon={<ReloadOutlined />} onClick={() => refetch()} loading={isLoading}>
            刷新
          </Button>
        </div>

        {isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 'var(--space-6) var(--space-6)' }}>
            <Spin size="large" />
          </div>
        ) : isError ? (
          <div style={{ padding: 'var(--space-5) var(--space-5)' }}>
            <Alert
              message="加载失败"
              description={(error as Error)?.message || '获取审计日志失败，请稍后重试'}
              type="error"
              showIcon
              action={
                <Button size="small" onClick={() => refetch()}>
                  重试
                </Button>
              }
            />
          </div>
        ) : logsData?.data?.items?.length ? (
          <Table
            columns={columns}
            dataSource={logsData.data.items as AuditLog[]}
            rowKey="id"
            pagination={{
              current: page,
              pageSize: pageSize,
              total: logsData.data.total,
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (v: number) => <span style={{ color: 'var(--color-text-secondary)' }}>共 {v} 条</span>,
              onChange: (newPage, newPageSize) => {
                setPage(newPage)
                setPageSize(newPageSize)
              },
            }}
          />
        ) : (
          <Empty description="暂无审计日志" imageStyle={{ height: 80 }} />
        )}
      </PageCard>
    </PageContainer>
  )
}

export default AuditLogs
