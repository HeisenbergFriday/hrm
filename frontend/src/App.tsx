import { useEffect, useState, lazy, Suspense } from 'react'
import { Layout, Menu, ConfigProvider, theme, Spin, message } from 'antd'
import { Link, Routes, Route, useLocation, useNavigate } from 'react-router-dom'
import {
  LoadingOutlined,
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
  LockOutlined,
  SwapOutlined,
  CalendarOutlined,
  ScheduleOutlined,
} from '@ant-design/icons'
import axios from 'axios'

const Login = lazy(() => import('./pages/Login'))
const Callback = lazy(() => import('./pages/Callback'))
const LoginError = lazy(() => import('./pages/LoginError'))
const Home = lazy(() => import('./pages/Home'))
const Organization = lazy(() => import('./pages/Organization'))
const DepartmentTree = lazy(() => import('./pages/DepartmentTree'))
const EmployeeList = lazy(() => import('./pages/EmployeeList'))
const EmployeeDetail = lazy(() => import('./pages/EmployeeDetail'))
const EmployeeProfile = lazy(() => import('./pages/EmployeeProfile'))
const EmployeeFlow = lazy(() => import('./pages/EmployeeFlow'))
const TalentAnalysis = lazy(() => import('./pages/TalentAnalysis'))
const SyncLog = lazy(() => import('./pages/SyncLog'))
const Attendance = lazy(() => import('./pages/Attendance'))
const AttendanceStats = lazy(() => import('./pages/AttendanceStats'))
const AttendanceExport = lazy(() => import('./pages/AttendanceExport'))
const WeekSchedule = lazy(() => import('./pages/WeekSchedule'))
const EmployeeShiftConfig = lazy(() => import('./pages/EmployeeShiftConfig'))
const LeaveOvertime = lazy(() => import('./pages/LeaveOvertime'))
const Approval = lazy(() => import('./pages/Approval'))
const ApprovalTemplate = lazy(() => import('./pages/ApprovalTemplate'))
const ApprovalInstance = lazy(() => import('./pages/ApprovalInstance'))
const ApprovalDetail = lazy(() => import('./pages/ApprovalDetail'))
const ApprovalStats = lazy(() => import('./pages/ApprovalStats'))
const RoleManagement = lazy(() => import('./pages/RoleManagement'))
const SyncJobs = lazy(() => import('./pages/SyncJobs'))
const AuditLogs = lazy(() => import('./pages/AuditLogs'))
const PerformanceOverview = lazy(() => import('./pages/PerformanceOverview'))
const PerformanceIndicatorLibrary = lazy(() => import('./pages/PerformanceIndicatorLibrary'))
const PerformanceResultView = lazy(() => import('./pages/PerformanceResultView'))
const PerformanceSelfEval = lazy(() => import('./pages/PerformanceSelfEval'))
const PerformanceManagerEval = lazy(() => import('./pages/PerformanceManagerEval'))
const PerformanceGoalSetting = lazy(() => import('./pages/PerformanceGoalSetting'))
const Permission = lazy(() => import('./pages/Permission'))
const Log = lazy(() => import('./pages/Log'))
const Setting = lazy(() => import('./pages/Setting'))

import { useAuthStore } from './store/authStore'

const { Header, Sider, Content } = Layout

const appTheme = {
  token: {
    colorPrimary: '#4338ca',
    colorPrimaryHover: '#6366f1',
    colorPrimaryActive: '#3730a3',
    borderRadius: 8,
    fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', 'Oxygen', 'Ubuntu', 'Cantarell', 'Fira Sans', 'Droid Sans', 'Helvetica Neue', sans-serif",
  },
  components: {
    Button: {
      borderRadius: 8,
      controlHeight: 36,
      fontWeight: 600,
    },
    Tag: {
      borderRadiusSM: 6,
    },
  },
}

const authPaths = ['/login', '/callback', '/login-error']

function PageLoading() {
  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 200 }}>
      <Spin indicator={<LoadingOutlined style={{ fontSize: 24 }} spin />} />
    </div>
  )
}

function isDingTalkEnv(): boolean {
  return /DingTalk/i.test(navigator.userAgent)
}

function getAxiosErrorMessage(error: unknown, fallback: string): string {
  if (axios.isAxiosError(error)) {
    const serverMessage = error.response?.data?.message
    if (typeof serverMessage === 'string' && serverMessage.trim() !== '') {
      return serverMessage
    }
  }

  return fallback
}

function AuthRoutes() {
  return (
    <Suspense fallback={<PageLoading />}>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/callback" element={<Callback />} />
        <Route path="/login-error" element={<LoginError />} />
      </Routes>
    </Suspense>
  )
}

function App() {
  const [collapsed, setCollapsed] = useState(false)
  const [autoLogging, setAutoLogging] = useState(false)
  const location = useLocation()
  const navigate = useNavigate()
  const { isLoggedIn, user, login, logout } = useAuthStore()
  const selectedMenuKey = location.pathname.startsWith('/employees/')
    ? '/employees'
    : location.pathname.startsWith('/performance/')
      ? location.pathname.includes('/indicator-library')
        ? '/performance-indicator-library'
        : '/performance-overview'
      : location.pathname

  const {
    token: { colorBgContainer, borderRadiusLG },
  } = theme.useToken()

  const handleLogout = async () => {
    try {
      await axios.post('/api/v1/auth/logout')
    } catch (err) {
      console.warn('[logout] request failed', err)
    } finally {
      logout()
      navigate('/login?mode=scan', { replace: true })
    }
  }

  useEffect(() => {
    if (!isDingTalkEnv() || isLoggedIn || authPaths.includes(location.pathname)) {
      return
    }

    setAutoLogging(true)

    const doAutoLogin = async () => {
      try {
        const configRes = await axios.get('/api/v1/auth/dingtalk/config')
        const { corp_id: corpId, missing } = configRes.data.data
        const dd = (window as any).dd

        if (!corpId || (Array.isArray(missing) && missing.includes('DINGTALK_CORP_ID'))) {
          message.error('缺少 DINGTALK_CORP_ID，暂时无法使用钉钉内免登')
          setAutoLogging(false)
          navigate('/login', { replace: true })
          return
        }

        if (!dd?.runtime?.permission?.requestAuthCode) {
          message.error('钉钉 JS-SDK 未加载或未授权')
          setAutoLogging(false)
          navigate('/login', { replace: true })
          return
        }

        dd.runtime.permission.requestAuthCode({
          corpId,
          onSuccess: async (result: { code: string }) => {
            try {
              const response = await axios.post('/api/v1/auth/dingtalk/in-app', {
                code: result.code,
              })
              const { token, user } = response.data.data
              login(user, token)
              message.success('登录成功', 0.6)
              setAutoLogging(false)
            } catch (err) {
              console.error('[DingTalk InApp] login failed', err)
              message.error(getAxiosErrorMessage(err, '钉钉内免登失败'))
              setAutoLogging(false)
              navigate('/login', { replace: true })
            }
          },
          onFail: (err: unknown) => {
            console.error('[DingTalk InApp] requestAuthCode failed', err)
            message.error('获取钉钉授权码失败')
            setAutoLogging(false)
            navigate('/login', { replace: true })
          },
        })
      } catch (err) {
        console.error('[DingTalk InApp] init failed', err)
        message.error(getAxiosErrorMessage(err, '钉钉内免登初始化失败'))
        setAutoLogging(false)
        navigate('/login', { replace: true })
      }
    }

    const timer = setTimeout(doAutoLogin, 300)
    return () => clearTimeout(timer)
  }, [isLoggedIn, location.pathname, login, navigate])

  if (authPaths.includes(location.pathname)) {
    return (
      <ConfigProvider>
        <AuthRoutes />
      </ConfigProvider>
    )
  }

  if (!isLoggedIn) {
    if (autoLogging) {
      return (
        <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', background: '#f0f2f5' }}>
          <div style={{ textAlign: 'center' }}>
            <Spin indicator={<LoadingOutlined style={{ fontSize: 24 }} spin />} />
            <p style={{ marginTop: 16 }}>正在通过钉钉自动登录，请稍候...</p>
          </div>
        </div>
      )
    }

    return (
      <ConfigProvider>
        <Suspense fallback={<PageLoading />}>
          <Login />
        </Suspense>
      </ConfigProvider>
    )
  }

  return (
    <ConfigProvider>
      <Layout>
        <Sider
          collapsible
          collapsed={collapsed}
          onCollapse={setCollapsed}
          style={{ position: 'fixed', height: '100vh', overflow: 'auto', zIndex: 100, left: 0, top: 0 }}
        >
          <div
            className="logo"
            style={{ height: 64, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', fontSize: 18, fontWeight: 'bold' }}
          >
            人事管理系统
          </div>
          <Menu theme="dark" mode="inline" selectedKeys={[selectedMenuKey]} defaultOpenKeys={location.pathname.startsWith('/performance/') ? ['performance'] : ['organization']}>
            <Menu.Item key="/" icon={<UserOutlined />}>
              <Link to="/">首页</Link>
            </Menu.Item>
            <Menu.SubMenu key="organization" icon={<TeamOutlined />} title="组织管理">
              <Menu.Item key="/organization" icon={<TeamOutlined />}>
                <Link to="/organization">人才管理驾驶舱</Link>
              </Menu.Item>
              <Menu.Item key="/department-tree" icon={<TeamOutlined />}>
                <Link to="/department-tree">组织架构</Link>
              </Menu.Item>
              <Menu.Item key="/employees" icon={<UserOutlined />}>
                <Link to="/employees">组织花名册</Link>
              </Menu.Item>
              <Menu.Item key="/employee-profile" icon={<UserOutlined />}>
                <Link to="/employee-profile">员工档案</Link>
              </Menu.Item>
              <Menu.Item key="/employee-flow" icon={<SwapOutlined />}>
                <Link to="/employee-flow">入转调离</Link>
              </Menu.Item>
              <Menu.Item key="/talent-analysis" icon={<BarChartOutlined />}>
                <Link to="/talent-analysis">人才分析</Link>
              </Menu.Item>
              <Menu.Item key="/sync-log" icon={<HistoryOutlined />}>
                <Link to="/sync-log">同步日志</Link>
              </Menu.Item>
            </Menu.SubMenu>
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
            <Menu.Item key="/role-management" icon={<KeyOutlined />}>
              <Link to="/role-management">权限管理</Link>
            </Menu.Item>
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
            <Menu.Item key="/leave-overtime" icon={<ScheduleOutlined />}>
              <Link to="/leave-overtime">年假与调休</Link>
            </Menu.Item>
            <Menu.SubMenu key="performance" icon={<BarChartOutlined />} title="绩效管理">
              <Menu.Item key="/performance-overview">
                <Link to="/performance-overview">绩效活动</Link>
              </Menu.Item>
              <Menu.Item key="/performance-indicator-library">
                <Link to="/performance-indicator-library">指标库管理</Link>
              </Menu.Item>
            </Menu.SubMenu>
            <Menu.Item key="/setting" icon={<SettingOutlined />}>
              <Link to="/setting">系统设置</Link>
            </Menu.Item>
            <Menu.Item key="/logout" icon={<LogoutOutlined />} onClick={handleLogout}>
              退出登录
            </Menu.Item>
          </Menu>
        </Sider>
        <Layout style={{ marginLeft: collapsed ? 80 : 200, transition: 'margin-left 0.2s' }}>
          <Header style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', padding: '0 24px' }}>
            <div style={{ color: '#fff' }}>{user?.name || '管理员'}</div>
          </Header>
          <Content style={{ margin: '24px 16px', padding: 24, minHeight: 280, background: colorBgContainer, borderRadius: borderRadiusLG }}>
            <Suspense fallback={<PageLoading />}>
              <Routes>
                <Route path="/" element={<Home />} />
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
                <Route path="/sync-jobs" element={<SyncJobs />} />
                <Route path="/audit-logs" element={<AuditLogs />} />
                <Route path="/employee-profile" element={<EmployeeProfile />} />
                <Route path="/employee-flow" element={<EmployeeFlow />} />
                <Route path="/talent-analysis" element={<TalentAnalysis />} />
                <Route path="/leave-overtime" element={<LeaveOvertime />} />
                <Route path="/performance-overview" element={<PerformanceOverview />} />
                <Route path="/performance-indicator-library" element={<PerformanceIndicatorLibrary />} />
                <Route path="/performance-result/:activityId/:participantId" element={<PerformanceResultView />} />
                <Route path="/performance-self-eval/:activityId/:participantId" element={<PerformanceSelfEval />} />
                <Route path="/performance-manager-eval/:activityId/:participantId" element={<PerformanceManagerEval />} />
                <Route path="/performance-goal-setting/:activityId/:participantId" element={<PerformanceGoalSetting />} />
                <Route path="/permission" element={<Permission />} />
                <Route path="/log" element={<Log />} />
                <Route path="/setting" element={<Setting />} />
              </Routes>
            </Suspense>
          </Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  )
}

export default App
