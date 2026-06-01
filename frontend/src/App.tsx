import { useEffect, useState, lazy, Suspense } from 'react'
import { Layout, Menu, ConfigProvider, Spin, message, Button } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { Link, Routes, Route, useLocation, useNavigate } from 'react-router-dom'
import {
  LoadingOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  LogoutOutlined,
} from '@ant-design/icons'
import axios from 'axios'
import { menuConfig, logoutMenuItem, filterMenuByKeys, menuPermissionKey } from './config/menu'
import { refreshMenuKeys } from './services/api'
import RouteGuard from './components/RouteGuard'
import ErrorBoundary from './components/ErrorBoundary'

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

const routeMenuKeys: Record<string, string> = {
  '/': menuPermissionKey('home'),
  '/organization': menuPermissionKey('organization-dashboard'),
  '/department-tree': menuPermissionKey('department-tree'),
  '/employees': menuPermissionKey('employees'),
  '/sync-log': menuPermissionKey('sync-log'),
  '/attendance': menuPermissionKey('attendance'),
  '/attendance-stats': menuPermissionKey('attendance-stats'),
  '/attendance-export': menuPermissionKey('attendance-export'),
  '/week-schedule': menuPermissionKey('week-schedule'),
  '/employee-shift-config': menuPermissionKey('employee-shift-config'),
  '/approval': menuPermissionKey('approval-templates'),
  '/approval-templates': menuPermissionKey('approval-templates'),
  '/approval-instances': menuPermissionKey('approval-instances'),
  '/approval-stats': menuPermissionKey('approval-stats'),
  '/role-management': menuPermissionKey('permission'),
  '/sync-jobs': menuPermissionKey('sync-jobs'),
  '/audit-logs': menuPermissionKey('audit-logs'),
  '/employee-profile': menuPermissionKey('employee-profile'),
  '/employee-flow': menuPermissionKey('employee-flow'),
  '/talent-analysis': menuPermissionKey('talent-analysis'),
  '/leave-overtime': menuPermissionKey('leave-overtime'),
  '/performance-overview': menuPermissionKey('performance-overview'),
  '/performance-indicator-library': menuPermissionKey('performance-indicator-library'),
  '/permission': menuPermissionKey('permission'),
  '/setting': menuPermissionKey('setting'),
}

function selectedMenuKeyForPath(pathname: string) {
  if (pathname.startsWith('/employees/')) return menuPermissionKey('employees')
  if (pathname.startsWith('/approval-detail/')) return menuPermissionKey('approval-instances')
  if (pathname.startsWith('/performance-result/')) return menuPermissionKey('performance-result')
  if (pathname.startsWith('/performance-self-eval/')) return menuPermissionKey('performance-self-eval')
  if (pathname.startsWith('/performance-manager-eval/')) return menuPermissionKey('performance-manager-eval')
  if (pathname.startsWith('/performance-goal-setting/')) return menuPermissionKey('performance-goal-setting')
  return routeMenuKeys[pathname] || ''
}

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
  const location = useLocation()
  return (
    <ErrorBoundary resetKey={location.pathname}>
      <Suspense fallback={<PageLoading />}>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/callback" element={<Callback />} />
          <Route path="/login-error" element={<LoginError />} />
        </Routes>
      </Suspense>
    </ErrorBoundary>
  )
}

function App() {
  const [collapsed, setCollapsed] = useState(false)
  const [autoLogging, setAutoLogging] = useState(false)
  const location = useLocation()
  const navigate = useNavigate()
  const { isLoggedIn, user, login, logout, menuKeys } = useAuthStore()
  const selectedMenuKey = selectedMenuKeyForPath(location.pathname)

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

  // 刷新菜单权限（启动时 + 页面获焦时）
  useEffect(() => {
    if (!isLoggedIn) return

    refreshMenuKeys()

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') refreshMenuKeys()
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange)
  }, [isLoggedIn])

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
      <ConfigProvider locale={zhCN}>
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
      <ConfigProvider locale={zhCN}>
        <ErrorBoundary resetKey={location.pathname}>
          <Suspense fallback={<PageLoading />}>
            <Login />
          </Suspense>
        </ErrorBoundary>
      </ConfigProvider>
    )
  }

  return (
    <ConfigProvider locale={zhCN}>
      <Layout>
        <Sider
          className={collapsed ? 'app-sider app-sider-collapsed' : 'app-sider'}
          collapsible
          collapsed={collapsed}
          collapsedWidth={80}
          onCollapse={setCollapsed}
          trigger={null}
          style={{ position: 'fixed', height: '100vh', overflow: 'hidden', zIndex: 100, left: 0, top: 0, transition: 'all 0.3s ease' }}
        >
          <div
            className="logo"
            style={{ height: 64, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', fontSize: 16, fontWeight: 'bold', whiteSpace: 'nowrap', overflow: 'hidden' }}
          >
            人事管理系统
          </div>
          <div className="app-sider-menu-scroll">
            <Menu theme="dark" mode="inline" selectedKeys={[selectedMenuKey]} defaultOpenKeys={location.pathname.startsWith('/performance') ? [menuPermissionKey('performance-group')] : [menuPermissionKey('organization-group')]}>
              {filterMenuByKeys(menuConfig, menuKeys).map((item) => {
                if (item.children) {
                  return (
                    <Menu.SubMenu key={item.key} icon={item.icon} title={item.label}>
                      {item.children.map((child) => (
                        <Menu.Item key={child.key} icon={child.icon}>
                          {child.label}
                        </Menu.Item>
                      ))}
                    </Menu.SubMenu>
                  )
                }
                return (
                  <Menu.Item key={item.key} icon={item.icon}>
                    {item.label}
                  </Menu.Item>
                )
              })}
              {menuKeys.length > 0 && (
                <Menu.Item key={logoutMenuItem.key} icon={logoutMenuItem.icon} onClick={handleLogout}>
                  {logoutMenuItem.label}
                </Menu.Item>
              )}
            </Menu>
          </div>
          <div
            className="app-sider-trigger"
            onClick={() => setCollapsed(!collapsed)}
            onMouseEnter={(e) => { e.currentTarget.style.color = '#fff'; e.currentTarget.style.background = 'rgba(0,0,0,0.3)' }}
            onMouseLeave={(e) => { e.currentTarget.style.color = 'rgba(255,255,255,0.65)'; e.currentTarget.style.background = 'rgba(0,0,0,0.15)' }}
          >
            {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
          </div>
        </Sider>
        <Layout style={{ marginLeft: collapsed ? 80 : 200, transition: 'margin-left 0.3s ease' }}>
          <Header style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', padding: '0 24px', gap: 16 }}>
            <span style={{ color: '#fff' }}>{user?.name || '管理员'}</span>
            <Button
              type="text"
              icon={<LogoutOutlined />}
              onClick={handleLogout}
              style={{ color: '#fff' }}
            >
              退出
            </Button>
          </Header>
          <Content style={{ margin: 0, padding: 0, minHeight: 'calc(100vh - 64px)', background: 'var(--color-bg-page)' }}>
            <ErrorBoundary resetKey={location.pathname}>
              <Suspense fallback={<PageLoading />}>
                <Routes>
                <Route path="/" element={<RouteGuard menuKey="menu:home"><Home /></RouteGuard>} />
                <Route path="/department-tree" element={<RouteGuard menuKey="menu:department-tree"><DepartmentTree /></RouteGuard>} />
                <Route path="/employees" element={<RouteGuard menuKey="menu:employees"><EmployeeList /></RouteGuard>} />
                <Route path="/employees/:id" element={<RouteGuard menuKey="menu:employees"><EmployeeDetail /></RouteGuard>} />
                <Route path="/sync-log" element={<RouteGuard menuKey="menu:sync-log"><SyncLog /></RouteGuard>} />
                <Route path="/organization" element={<RouteGuard menuKey="menu:organization-dashboard"><Organization /></RouteGuard>} />
                <Route path="/attendance" element={<RouteGuard menuKey="menu:attendance"><Attendance /></RouteGuard>} />
                <Route path="/attendance-stats" element={<RouteGuard menuKey="menu:attendance-stats"><AttendanceStats /></RouteGuard>} />
                <Route path="/attendance-export" element={<RouteGuard menuKey="menu:attendance-export"><AttendanceExport /></RouteGuard>} />
                <Route path="/week-schedule" element={<RouteGuard menuKey="menu:week-schedule"><WeekSchedule /></RouteGuard>} />
                <Route path="/employee-shift-config" element={<RouteGuard menuKey="menu:employee-shift-config"><EmployeeShiftConfig /></RouteGuard>} />
                <Route path="/approval" element={<RouteGuard menuKey="menu:approval-templates"><Approval /></RouteGuard>} />
                <Route path="/approval-templates" element={<RouteGuard menuKey="menu:approval-templates"><ApprovalTemplate /></RouteGuard>} />
                <Route path="/approval-instances" element={<RouteGuard menuKey="menu:approval-instances"><ApprovalInstance /></RouteGuard>} />
                <Route path="/approval-detail/:id" element={<RouteGuard menuKey="menu:approval-instances"><ApprovalDetail /></RouteGuard>} />
                <Route path="/approval-stats" element={<RouteGuard menuKey="menu:approval-stats"><ApprovalStats /></RouteGuard>} />
                <Route path="/role-management" element={<RouteGuard menuKey="menu:permission"><RoleManagement /></RouteGuard>} />
                <Route path="/sync-jobs" element={<RouteGuard menuKey="menu:sync-jobs"><SyncJobs /></RouteGuard>} />
                <Route path="/audit-logs" element={<RouteGuard menuKey="menu:audit-logs"><AuditLogs /></RouteGuard>} />
                <Route path="/employee-profile" element={<RouteGuard menuKey="menu:employee-profile"><EmployeeProfile /></RouteGuard>} />
                <Route path="/employee-flow" element={<RouteGuard menuKey="menu:employee-flow"><EmployeeFlow /></RouteGuard>} />
                <Route path="/talent-analysis" element={<RouteGuard menuKey="menu:talent-analysis"><TalentAnalysis /></RouteGuard>} />
                <Route path="/leave-overtime" element={<RouteGuard menuKey="menu:leave-overtime"><LeaveOvertime /></RouteGuard>} />
                <Route path="/performance-overview" element={<RouteGuard menuKey="menu:performance-overview"><PerformanceOverview /></RouteGuard>} />
                <Route path="/performance-indicator-library" element={<RouteGuard menuKey="menu:performance-indicator-library"><PerformanceIndicatorLibrary /></RouteGuard>} />
                <Route path="/performance-result/:activityId/:participantId" element={<RouteGuard menuKey="menu:performance-overview" permissionCode="performance:result:view"><PerformanceResultView /></RouteGuard>} />
                <Route path="/performance-self-eval/:activityId/:participantId" element={<RouteGuard menuKey="menu:performance-overview" permissionCode="performance:self_eval:submit"><PerformanceSelfEval /></RouteGuard>} />
                <Route path="/performance-manager-eval/:activityId/:participantId" element={<RouteGuard menuKey="menu:performance-overview" permissionCode="performance:manager_eval:submit"><PerformanceManagerEval /></RouteGuard>} />
                <Route path="/performance-goal-setting/:activityId/:participantId" element={<RouteGuard menuKey="menu:performance-overview" permissionCode="performance:goal:manage"><PerformanceGoalSetting /></RouteGuard>} />
                <Route path="/permission" element={<RouteGuard menuKey="menu:permission"><Permission /></RouteGuard>} />
                <Route path="/log" element={<Log />} />
                <Route path="/setting" element={<RouteGuard menuKey="menu:setting"><Setting /></RouteGuard>} />
              </Routes>
            </Suspense>
            </ErrorBoundary>
          </Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  )
}

export default App
