import { useState } from 'react'
import { Layout, Menu, ConfigProvider, theme } from 'antd'
import { Link, Routes, Route, useLocation } from 'react-router-dom'
import { UserOutlined, TeamOutlined, ClockCircleOutlined, FileOutlined, KeyOutlined, HistoryOutlined, SettingOutlined, LogoutOutlined, WarningOutlined, FileExcelOutlined, FileTextOutlined, BarChartOutlined, SyncOutlined, LockOutlined, SwapOutlined, CalendarOutlined, ScheduleOutlined } from '@ant-design/icons'

import Login from './pages/Login'
import Home from './pages/Home'
import Organization from './pages/Organization'
import Attendance from './pages/Attendance'
import AttendanceStats from './pages/AttendanceStats'
import AttendanceExport from './pages/AttendanceExport'
import Approval from './pages/Approval'
import ApprovalTemplate from './pages/ApprovalTemplate'
import ApprovalInstance from './pages/ApprovalInstance'
import ApprovalDetail from './pages/ApprovalDetail'
import ApprovalStats from './pages/ApprovalStats'
import RoleManagement from './pages/RoleManagement'
import MenuPermission from './pages/MenuPermission'
import DataPermission from './pages/DataPermission'
import SyncJobs from './pages/SyncJobs'
import AuditLogs from './pages/AuditLogs'
import EmployeeProfile from './pages/EmployeeProfile'
import EmployeeFlow from './pages/EmployeeFlow'
import TalentAnalysis from './pages/TalentAnalysis'
import Permission from './pages/Permission'
import Log from './pages/Log'
import Setting from './pages/Setting'
import Callback from './pages/Callback'
import LoginError from './pages/LoginError'
import DepartmentTree from './pages/DepartmentTree'
import EmployeeList from './pages/EmployeeList'
import EmployeeDetail from './pages/EmployeeDetail'
import SyncLog from './pages/SyncLog'
import WeekSchedule from './pages/WeekSchedule'
import EmployeeShiftConfig from './pages/EmployeeShiftConfig'
import LeaveOvertime from './pages/LeaveOvertime'

import { useAuthStore } from './store/authStore'

const { Header, Sider, Content } = Layout

function App() {
  const [collapsed, setCollapsed] = useState(false)
  const location = useLocation()
  const { isLoggedIn, user } = useAuthStore()

  const { token: { colorBgContainer, borderRadiusLG } } = theme.useToken()

  const noAuthPaths = ['/login', '/callback', '/login-error']
  if (!isLoggedIn && !noAuthPaths.includes(location.pathname)) {
    return <Login />
  }

  return (
    <ConfigProvider>
      <Layout>
        <Sider collapsible collapsed={collapsed} onCollapse={setCollapsed} style={{ position: 'fixed', height: '100vh', overflow: 'auto', zIndex: 100, left: 0, top: 0 }}>
          <div className="logo" style={{ height: 64, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', fontSize: 18, fontWeight: 'bold' }}>
            人事管理系统
          </div>
          <Menu theme="dark" mode="inline" selectedKeys={[location.pathname]}>
            <Menu.Item key="/" icon={<UserOutlined />}>
              <Link to="/">首页</Link>
            </Menu.Item>
            <Menu.Item key="/department-tree" icon={<TeamOutlined />}>
              <Link to="/department-tree">部门树</Link>
            </Menu.Item>
            <Menu.Item key="/employees" icon={<UserOutlined />}>
              <Link to="/employees">员工列表</Link>
            </Menu.Item>
            <Menu.Item key="/sync-log" icon={<HistoryOutlined />}>
              <Link to="/sync-log">同步日志</Link>
            </Menu.Item>
            <Menu.SubMenu key="attendance" icon={<ClockCircleOutlined />} title="考勤管理">
              <Menu.Item key="/attendance" icon={<ClockCircleOutlined />}>
                <Link to="/attendance">考勤查询</Link>
              </Menu.Item>
              <Menu.Item key="/attendance-stats" icon={<WarningOutlined />}>
                <Link to="/attendance-stats">异常统计</Link>
              </Menu.Item>
              <Menu.Item key="/attendance-export" icon={<FileExcelOutlined />}>
                <Link to="/attendance-export">导出记录</Link>
              </Menu.Item>
              <Menu.Item key="/week-schedule" icon={<CalendarOutlined />}>
                <Link to="/week-schedule">大小周与节假日</Link>
              </Menu.Item>
              <Menu.Item key="/employee-shift-config" icon={<ClockCircleOutlined />}>
                <Link to="/employee-shift-config">员工下班时间</Link>
              </Menu.Item>
            </Menu.SubMenu>
            <Menu.SubMenu key="approval" icon={<FileOutlined />} title="审批管理">
              <Menu.Item key="/approval-templates" icon={<FileTextOutlined />}>
                <Link to="/approval-templates">审批模板</Link>
              </Menu.Item>
              <Menu.Item key="/approval-instances" icon={<FileOutlined />}>
                <Link to="/approval-instances">审批实例</Link>
              </Menu.Item>
              <Menu.Item key="/approval-stats" icon={<BarChartOutlined />}>
                <Link to="/approval-stats">审批统计</Link>
              </Menu.Item>
            </Menu.SubMenu>
            <Menu.SubMenu key="permission" icon={<KeyOutlined />} title="权限管理">
              <Menu.Item key="/role-management" icon={<UserOutlined />}>
                <Link to="/role-management">角色管理</Link>
              </Menu.Item>
              <Menu.Item key="/menu-permission" icon={<FileTextOutlined />}>
                <Link to="/menu-permission">菜单权限</Link>
              </Menu.Item>
              <Menu.Item key="/data-permission" icon={<LockOutlined />}>
                <Link to="/data-permission">数据权限</Link>
              </Menu.Item>
            </Menu.SubMenu>
            <Menu.SubMenu key="jobs" icon={<SyncOutlined />} title="任务中心">
              <Menu.Item key="/sync-jobs" icon={<SyncOutlined />}>
                <Link to="/sync-jobs">同步任务</Link>
              </Menu.Item>
            </Menu.SubMenu>
            <Menu.SubMenu key="audit" icon={<HistoryOutlined />} title="审计日志">
              <Menu.Item key="/audit-logs" icon={<HistoryOutlined />}>
                <Link to="/audit-logs">操作日志</Link>
              </Menu.Item>
            </Menu.SubMenu>
            <Menu.SubMenu key="employee" icon={<UserOutlined />} title="员工档案中心">
              <Menu.Item key="/employee-profile" icon={<UserOutlined />}>
                <Link to="/employee-profile">员工档案</Link>
              </Menu.Item>
              <Menu.Item key="/employee-flow" icon={<SwapOutlined />}>
                <Link to="/employee-flow">入转调离</Link>
              </Menu.Item>
            </Menu.SubMenu>
            <Menu.SubMenu key="talent" icon={<BarChartOutlined />} title="人才分析">
              <Menu.Item key="/talent-analysis" icon={<BarChartOutlined />}>
                <Link to="/talent-analysis">人才分析</Link>
              </Menu.Item>
            </Menu.SubMenu>
            <Menu.Item key="/leave-overtime" icon={<ScheduleOutlined />}>
              <Link to="/leave-overtime">年假与调休</Link>
            </Menu.Item>
            <Menu.Item key="/setting" icon={<SettingOutlined />}>
              <Link to="/setting">系统设置</Link>
            </Menu.Item>
            <Menu.Item key="/logout" icon={<LogoutOutlined />}>
              <Link to="/login">退出登录</Link>
            </Menu.Item>
          </Menu>
        </Sider>
        <Layout style={{ marginLeft: collapsed ? 80 : 200, transition: 'margin-left 0.2s' }}>
          <Header style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', padding: '0 24px' }}>
            <div style={{ color: '#fff' }}>{user?.name || '管理员'}</div>
          </Header>
          <Content style={{ margin: '24px 16px', padding: 24, minHeight: 280, background: colorBgContainer, borderRadius: borderRadiusLG }}>
            <Routes>
              <Route path="/" element={<Home />} />
              <Route path="/login" element={<Login />} />
              <Route path="/callback" element={<Callback />} />
              <Route path="/login-error" element={<LoginError />} />
              <Route path="/department-tree" element={<DepartmentTree />} />
              <Route path="/employees" element={<EmployeeList />} />
              <Route path="/employees/:id" element={<EmployeeDetail />} />
              <Route path="/sync-log" element={<SyncLog />} />
              <Route path="/organization" element={<Organization />} />
              <Route path="/attendance" element={<Attendance />} />
              <Route path="/attendance-stats" element={<AttendanceStats />} />
              <Route path="/attendance-export" element={<AttendanceExport />} />
              <Route path="/week-schedule" element={<WeekSchedule />} />
              <Route path="/employee-shift-config" element={<EmployeeShiftConfig />} />
              <Route path="/approval" element={<Approval />} />
              <Route path="/approval-templates" element={<ApprovalTemplate />} />
              <Route path="/approval-instances" element={<ApprovalInstance />} />
              <Route path="/approval-detail/:id" element={<ApprovalDetail />} />
              <Route path="/approval-stats" element={<ApprovalStats />} />
              <Route path="/role-management" element={<RoleManagement />} />
              <Route path="/menu-permission" element={<MenuPermission />} />
              <Route path="/data-permission" element={<DataPermission />} />
              <Route path="/sync-jobs" element={<SyncJobs />} />
              <Route path="/audit-logs" element={<AuditLogs />} />
              <Route path="/employee-profile" element={<EmployeeProfile />} />
              <Route path="/employee-flow" element={<EmployeeFlow />} />
              <Route path="/talent-analysis" element={<TalentAnalysis />} />
              <Route path="/leave-overtime" element={<LeaveOvertime />} />
              <Route path="/permission" element={<Permission />} />
              <Route path="/log" element={<Log />} />
              <Route path="/setting" element={<Setting />} />
            </Routes>
          </Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  )
}

export default App
