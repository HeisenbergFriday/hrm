import React from 'react'
import {
  UserOutlined,
  TeamOutlined,
  ClockCircleOutlined,
  FileOutlined,
  KeyOutlined,
  HistoryOutlined,
  SettingOutlined,
  LogoutOutlined,
  WarningOutlined,
  FileExcelOutlined,
  FileTextOutlined,
  BarChartOutlined,
  SyncOutlined,
  SwapOutlined,
  CalendarOutlined,
  ScheduleOutlined,
} from '@ant-design/icons'
import { Link } from 'react-router-dom'

export interface MenuItem {
  key: string
  label: React.ReactNode
  icon?: React.ReactNode
  children?: MenuItem[]
}

export const menuConfig: MenuItem[] = [
  {
    key: 'home',
    label: <Link to="/">首页</Link>,
    icon: <UserOutlined />,
  },
  {
    key: 'organization',
    label: '组织管理',
    icon: <TeamOutlined />,
    children: [
      { key: 'organization-dashboard', label: <Link to="/organization">人才管理驾驶舱</Link>, icon: <TeamOutlined /> },
      { key: 'department-tree', label: <Link to="/department-tree">组织架构</Link>, icon: <TeamOutlined /> },
      { key: 'employees', label: <Link to="/employees">组织花名册</Link>, icon: <UserOutlined /> },
      { key: 'employee-profile', label: <Link to="/employee-profile">员工档案</Link>, icon: <UserOutlined /> },
      { key: 'employee-flow', label: <Link to="/employee-flow">入转调离</Link>, icon: <SwapOutlined /> },
      { key: 'talent-analysis', label: <Link to="/talent-analysis">人才分析</Link>, icon: <BarChartOutlined /> },
      { key: 'sync-log', label: <Link to="/sync-log">同步日志</Link>, icon: <HistoryOutlined /> },
    ],
  },
  {
    key: 'attendance',
    label: '考勤管理',
    icon: <ClockCircleOutlined />,
    children: [
      { key: 'attendance', label: <Link to="/attendance">考勤查询</Link>, icon: <ClockCircleOutlined /> },
      { key: 'attendance-stats', label: <Link to="/attendance-stats">异常统计</Link>, icon: <WarningOutlined /> },
      { key: 'attendance-export', label: <Link to="/attendance-export">导出记录</Link>, icon: <FileExcelOutlined /> },
      { key: 'week-schedule', label: <Link to="/week-schedule">大小周与节假日</Link>, icon: <CalendarOutlined /> },
      { key: 'employee-shift-config', label: <Link to="/employee-shift-config">员工下班时间</Link>, icon: <ClockCircleOutlined /> },
    ],
  },
  {
    key: 'approval',
    label: '审批管理',
    icon: <FileOutlined />,
    children: [
      { key: 'approval-templates', label: <Link to="/approval-templates">审批模板</Link>, icon: <FileTextOutlined /> },
      { key: 'approval-instances', label: <Link to="/approval-instances">审批实例</Link>, icon: <FileOutlined /> },
      { key: 'approval-stats', label: <Link to="/approval-stats">审批统计</Link>, icon: <BarChartOutlined /> },
    ],
  },
  {
    key: 'permission',
    label: <Link to="/role-management">权限管理</Link>,
    icon: <KeyOutlined />,
  },
  {
    key: 'jobs',
    label: '任务中心',
    icon: <SyncOutlined />,
    children: [
      { key: 'sync-jobs', label: <Link to="/sync-jobs">同步任务</Link>, icon: <SyncOutlined /> },
    ],
  },
  {
    key: 'audit',
    label: '审计日志',
    icon: <HistoryOutlined />,
    children: [
      { key: 'audit-logs', label: <Link to="/audit-logs">操作日志</Link>, icon: <HistoryOutlined /> },
    ],
  },
  {
    key: 'leave-overtime',
    label: <Link to="/leave-overtime">年假与调休</Link>,
    icon: <ScheduleOutlined />,
  },
  {
    key: 'performance',
    label: '绩效管理',
    icon: <BarChartOutlined />,
    children: [
      { key: 'performance-overview', label: <Link to="/performance-overview">绩效活动</Link> },
      { key: 'performance-indicator-library', label: <Link to="/performance-indicator-library">指标库管理</Link> },
    ],
  },
  {
    key: 'setting',
    label: <Link to="/setting">系统设置</Link>,
    icon: <SettingOutlined />,
  },
]

export const logoutMenuItem: MenuItem = {
  key: 'logout',
  label: '退出登录',
  icon: <LogoutOutlined />,
}

// 根据用户拥有的 menuKeys 过滤菜单
export function filterMenuByKeys(items: MenuItem[], allowedKeys: string[]): MenuItem[] {
  if (!allowedKeys || allowedKeys.length === 0) return []

  const keySet = new Set(allowedKeys)

  return items
    .map((item) => {
      if (item.key === 'logout') return item

      if (!item.children) {
        return keySet.has(item.key) ? item : null
      }

      const filteredChildren = item.children.filter((child) => keySet.has(child.key))
      if (filteredChildren.length === 0) return null

      return { ...item, children: filteredChildren }
    })
    .filter(Boolean) as MenuItem[]
}
