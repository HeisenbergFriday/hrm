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
  title: string
  label: React.ReactNode
  icon?: React.ReactNode
  children?: MenuItem[]
}

export const menuPermissionKey = (key: string) => {
  const normalized = key.trim()
  return normalized.startsWith('menu:') ? normalized : `menu:${normalized}`
}

export const menuConfig: MenuItem[] = [
  {
    key: menuPermissionKey('home'),
    title: '首页',
    label: <Link to="/">首页</Link>,
    icon: <UserOutlined />,
  },
  {
    key: menuPermissionKey('organization-group'),
    title: '组织管理',
    label: '组织管理',
    icon: <TeamOutlined />,
    children: [
      { key: menuPermissionKey('organization-dashboard'), title: '人才管理驾驶舱', label: <Link to="/organization">人才管理驾驶舱</Link>, icon: <TeamOutlined /> },
      { key: menuPermissionKey('department-tree'), title: '组织架构', label: <Link to="/department-tree">组织架构</Link>, icon: <TeamOutlined /> },
      { key: menuPermissionKey('employees'), title: '组织花名册', label: <Link to="/employees">组织花名册</Link>, icon: <UserOutlined /> },
      { key: menuPermissionKey('employee-profile'), title: '员工档案', label: <Link to="/employee-profile">员工档案</Link>, icon: <UserOutlined /> },
      { key: menuPermissionKey('employee-flow'), title: '入转调离', label: <Link to="/employee-flow">入转调离</Link>, icon: <SwapOutlined /> },
      { key: menuPermissionKey('talent-analysis'), title: '人才分析', label: <Link to="/talent-analysis">人才分析</Link>, icon: <BarChartOutlined /> },
      { key: menuPermissionKey('sync-log'), title: '同步日志', label: <Link to="/sync-log">同步日志</Link>, icon: <HistoryOutlined /> },
    ],
  },
  {
    key: menuPermissionKey('attendance-group'),
    title: '考勤管理',
    label: '考勤管理',
    icon: <ClockCircleOutlined />,
    children: [
      { key: menuPermissionKey('attendance'), title: '考勤查询', label: <Link to="/attendance">考勤查询</Link>, icon: <ClockCircleOutlined /> },
      { key: menuPermissionKey('attendance-stats'), title: '异常统计', label: <Link to="/attendance-stats">异常统计</Link>, icon: <WarningOutlined /> },
      { key: menuPermissionKey('attendance-export'), title: '导出记录', label: <Link to="/attendance-export">导出记录</Link>, icon: <FileExcelOutlined /> },
      { key: menuPermissionKey('week-schedule'), title: '大小周与节假日', label: <Link to="/week-schedule">大小周与节假日</Link>, icon: <CalendarOutlined /> },
      { key: menuPermissionKey('employee-shift-config'), title: '员工下班时间', label: <Link to="/employee-shift-config">员工下班时间</Link>, icon: <ClockCircleOutlined /> },
    ],
  },
  {
    key: menuPermissionKey('approval-group'),
    title: '审批管理',
    label: '审批管理',
    icon: <FileOutlined />,
    children: [
      { key: menuPermissionKey('approval-templates'), title: '审批模板', label: <Link to="/approval-templates">审批模板</Link>, icon: <FileTextOutlined /> },
      { key: menuPermissionKey('approval-instances'), title: '审批实例', label: <Link to="/approval-instances">审批实例</Link>, icon: <FileOutlined /> },
      { key: menuPermissionKey('approval-stats'), title: '审批统计', label: <Link to="/approval-stats">审批统计</Link>, icon: <BarChartOutlined /> },
    ],
  },
  {
    key: menuPermissionKey('permission'),
    title: '权限管理',
    label: <Link to="/role-management">权限管理</Link>,
    icon: <KeyOutlined />,
  },
  {
    key: menuPermissionKey('jobs-group'),
    title: '任务中心',
    label: '任务中心',
    icon: <SyncOutlined />,
    children: [
      { key: menuPermissionKey('sync-jobs'), title: '同步任务', label: <Link to="/sync-jobs">同步任务</Link>, icon: <SyncOutlined /> },
    ],
  },
  {
    key: menuPermissionKey('audit-group'),
    title: '审计日志',
    label: '审计日志',
    icon: <HistoryOutlined />,
    children: [
      { key: menuPermissionKey('audit-logs'), title: '操作日志', label: <Link to="/audit-logs">操作日志</Link>, icon: <HistoryOutlined /> },
    ],
  },
  {
    key: menuPermissionKey('leave-overtime'),
    title: '年假与调休',
    label: <Link to="/leave-overtime">年假与调休</Link>,
    icon: <ScheduleOutlined />,
  },
  {
    key: menuPermissionKey('performance-group'),
    title: '绩效管理',
    label: '绩效管理',
    icon: <BarChartOutlined />,
    children: [
      { key: menuPermissionKey('performance-overview'), title: '绩效活动', label: <Link to="/performance-overview">绩效活动</Link> },
      { key: menuPermissionKey('performance-indicator-library'), title: '指标库管理', label: <Link to="/performance-indicator-library">指标库管理</Link> },
    ],
  },
  {
    key: menuPermissionKey('setting'),
    title: '系统设置',
    label: <Link to="/setting">系统设置</Link>,
    icon: <SettingOutlined />,
  },
]

export const logoutMenuItem: MenuItem = {
  key: 'logout',
  title: '退出登录',
  label: '退出登录',
  icon: <LogoutOutlined />,
}

export function filterMenuByKeys(items: MenuItem[], allowedKeys: string[]): MenuItem[] {
  if (!allowedKeys || allowedKeys.length === 0) return []

  const keySet = new Set(allowedKeys.map(menuPermissionKey))

  return items
    .map((item) => {
      if (item.key === 'logout') return item

      if (!item.children) {
        return keySet.has(menuPermissionKey(item.key)) ? item : null
      }

      const filteredChildren = item.children.filter((child) => keySet.has(menuPermissionKey(child.key)))
      if (filteredChildren.length === 0) return null

      return { ...item, children: filteredChildren }
    })
    .filter(Boolean) as MenuItem[]
}

export interface TreeNode {
  title: string
  key: string
  children?: TreeNode[]
}

export function toTreeData(items: MenuItem[]): TreeNode[] {
  return items
    .filter((item) => item.key !== 'logout')
    .map((item) => {
      const node: TreeNode = {
        title: item.title,
        key: item.key,
      }
      if (item.children && item.children.length > 0) {
        node.children = toTreeData(item.children)
      }
      return node
    })
}
