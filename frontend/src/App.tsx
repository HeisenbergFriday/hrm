import { useEffect, useMemo, useState, lazy, Suspense } from 'react'
import { Layout, Menu, ConfigProvider, Spin, message, Button, Drawer, Grid, Empty, Result } from 'antd'
import type { MenuProps } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom'
import {
  LoadingOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  LogoutOutlined,
} from '@ant-design/icons'
import axios from 'axios'
import { menuConfig, filterMenuByKeys, menuPermissionKey } from './config/menu'
import { authAPI, refreshMenuKeys } from './services/api'
import RouteGuard, { resolveFallbackPath } from './components/RouteGuard'
import ErrorBoundary from './components/ErrorBoundary'
import MobileTableEnhancer from './components/MobileTableEnhancer'
import {
  authOrgIDFromSearchParamsOrStorage,
  authRedirectTargetFromLocation,
  loginPathWithRedirectAndOrg,
  loginPathWithRedirect,
  normalizeAuthOrgID,
  rememberAuthOrgID,
  rememberAuthRedirect,
} from './utils/authRedirect'
import { resolveMobileLayout, useMobileRuntime } from './utils/responsive'
import { useAuthStore } from './store/authStore'

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
const AttendanceExport = lazy(() => import('./pages/AttendanceExport'))
const AttendanceProcessing = lazy(() => import('./pages/AttendanceProcessing'))
const AttendanceExternalSync = lazy(() => import('./pages/AttendanceExternalSync'))
const AttendanceToolbox = lazy(() => import('./pages/AttendanceToolbox'))
const WeekSchedule = lazy(() => import('./pages/WeekSchedule'))
const EmployeeShiftConfig = lazy(() => import('./pages/EmployeeShiftConfig'))
const LeaveOvertime = lazy(() => import('./pages/LeaveOvertime'))
const Approval = lazy(() => import('./pages/Approval'))
const ApprovalTemplate = lazy(() => import('./pages/ApprovalTemplate'))
const ApprovalInstance = lazy(() => import('./pages/ApprovalInstance'))
const ApprovalDetail = lazy(() => import('./pages/ApprovalDetail'))
const ApprovalStats = lazy(() => import('./pages/ApprovalStats'))
const OAApprovalData = lazy(() => import('./pages/OAApprovalData'))
const RoleManagement = lazy(() => import('./pages/RoleManagement'))
const SyncJobs = lazy(() => import('./pages/SyncJobs'))
const AuditLogs = lazy(() => import('./pages/AuditLogs'))
const PerformanceOverview = lazy(() => import('./pages/PerformanceOverview'))
const PerformanceIndicatorLibrary = lazy(() => import('./pages/PerformanceIndicatorLibrary'))
const PerformanceReports = lazy(() => import('./pages/PerformanceReports'))
const PerformanceInterviews = lazy(() => import('./pages/PerformanceInterviews'))
const PerformanceAppeals = lazy(() => import('./pages/PerformanceAppeals'))
const PerformanceResultView = lazy(() => import('./pages/PerformanceResultView'))
const PerformanceSelfEval = lazy(() => import('./pages/PerformanceSelfEval'))
const PerformanceManagerEval = lazy(() => import('./pages/PerformanceManagerEval'))
const PerformanceGoalSetting = lazy(() => import('./pages/PerformanceGoalSetting'))
const Permission = lazy(() => import('./pages/Permission'))
const Setting = lazy(() => import('./pages/Setting'))

const { Header, Sider, Content } = Layout

/** 全局中文 locale：强制 Table/Empty 空状态为中文，避免个别版本回退到 "No data" */
const appLocale = {
  ...zhCN,
  Empty: {
    ...zhCN.Empty,
    description: '暂无数据',
  },
  Table: {
    ...zhCN.Table,
    emptyText: '暂无数据',
  },
}

const renderAppEmpty = () => (
  <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无数据" />
)

const appTheme = {
  token: {
    colorPrimary: '#2563eb',
    colorPrimaryHover: '#3b82f6',
    colorPrimaryActive: '#1d4ed8',
    colorInfo: '#0891b2',
    colorSuccess: '#10b981',
    colorWarning: '#f59e0b',
    colorBgLayout: '#f6f8fb',
    colorBgContainer: '#ffffff',
    colorBorderSecondary: '#e6edf5',
    colorText: '#172033',
    colorTextSecondary: '#64748b',
    borderRadius: 8,
    fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', 'Oxygen', 'Ubuntu', 'Cantarell', 'Fira Sans', 'Droid Sans', 'Helvetica Neue', sans-serif",
    boxShadow: '0 12px 28px rgba(15, 23, 42, 0.08)',
  },
  components: {
    Layout: {
      headerBg: 'rgba(255, 255, 255, 0.88)',
      siderBg: '#ffffff',
      bodyBg: '#f6f8fb',
    },
    Menu: {
      itemBg: 'transparent',
      itemSelectedBg: '#eaf2ff',
      itemSelectedColor: '#1d4ed8',
      itemHoverBg: '#f1f6ff',
      itemHoverColor: '#1d4ed8',
      subMenuItemBg: 'transparent',
      darkItemBg: 'transparent',
    },
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
  '/attendance-export': menuPermissionKey('attendance-export'),
  '/attendance-processing': menuPermissionKey('attendance-processing'),
  '/attendance/external-sync': menuPermissionKey('attendance-external-sync'),
  '/attendance-toolbox': menuPermissionKey('attendance-toolbox'),
  '/week-schedule': menuPermissionKey('week-schedule'),
  '/employee-shift-config': menuPermissionKey('employee-shift-config'),
  '/approval': menuPermissionKey('approval-templates'),
  '/approval-templates': menuPermissionKey('approval-templates'),
  '/approval-instances': menuPermissionKey('approval-instances'),
  '/approval-stats': menuPermissionKey('approval-stats'),
  '/oa-approval-data': menuPermissionKey('oa-approval-data'),
  '/role-management': menuPermissionKey('permission'),
  '/sync-jobs': menuPermissionKey('sync-jobs'),
  '/audit-logs': menuPermissionKey('audit-logs'),
  '/employee-profile': menuPermissionKey('employee-profile'),
  '/employee-flow': menuPermissionKey('employee-flow'),
  '/talent-analysis': menuPermissionKey('talent-analysis'),
  '/leave-overtime': menuPermissionKey('leave-overtime'),
  '/performance-overview': menuPermissionKey('performance-overview'),
  '/performance-reports': menuPermissionKey('performance-reports'),
  '/performance-interviews': menuPermissionKey('performance-interviews'),
  '/performance-appeals': menuPermissionKey('performance-appeals'),
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

function defaultOpenKeysForPath(pathname: string) {
  if (pathname.startsWith('/performance')) return [menuPermissionKey('performance-group')]
  if (pathname.startsWith('/attendance') || pathname.startsWith('/week-schedule') || pathname.startsWith('/employee-shift-config') || pathname.startsWith('/leave-overtime')) {
    return [menuPermissionKey('attendance-group')]
  }
  if (pathname.startsWith('/approval') || pathname.startsWith('/oa-approval-data')) return [menuPermissionKey('approval-group')]
  if (pathname.startsWith('/sync-jobs')) return [menuPermissionKey('jobs-group')]
  if (pathname.startsWith('/audit')) return [menuPermissionKey('audit-group')]
  return [menuPermissionKey('organization-group')]
}

function findMenuTitle(items: ReturnType<typeof filterMenuByKeys>, key: string): string {
  for (const item of items) {
    if (item.key === key) return item.title
    if (item.children) {
      const childTitle = findMenuTitle(item.children, key)
      if (childTitle) return childTitle
    }
  }
  return ''
}

export function buildSiderMenuItems(
  items: ReturnType<typeof filterMenuByKeys>,
): MenuProps['items'] {
  return items.map(item => {
    if (item.children) {
      return {
        key: item.key,
        icon: item.icon,
        label: item.label,
        title: item.title,
        children: item.children.map(child => ({
          key: child.key,
          icon: child.icon,
          label: child.label,
          title: child.title,
        })),
      }
    }

    return {
      key: item.key,
      icon: item.icon,
      label: item.label,
      title: item.title,
    }
  })
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
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [sessionChecking, setSessionChecking] = useState(true)
  const screens = Grid.useBreakpoint()
  const mobileRuntime = useMobileRuntime()
  const location = useLocation()
  const navigate = useNavigate()
  const { isLoggedIn, user, login, logout, menuKeys, orgId } = useAuthStore()
  const selectedMenuKey = selectedMenuKeyForPath(location.pathname)
  const isMobile = resolveMobileLayout(screens.md, mobileRuntime)

  const handleLogout = async () => {
    // 必须走 authAPI（axios 实例带 withCredentials + CSRF），禁止裸 axios.post：
    // JWT 存在 HttpOnly cookie 时，POST /auth/logout 会校验 X-CSRF-Token，缺头会 403，
    // 若仅清前端则服务端会话仍有效（与组织切换路径 authAPI.logout 保持一致）。
    try {
      await authAPI.logout()
    } catch (err) {
      console.warn('[logout] request failed', err)
    } finally {
      const orgID = typeof user?.org_id === 'string' ? user.org_id.trim() : ''
      if (orgID) rememberAuthOrgID(orgID)
      logout()
      navigate(orgID ? `/login?mode=scan&org_id=${encodeURIComponent(orgID)}` : '/login?mode=scan', { replace: true })
    }
  }
  const filteredMenuItems = filterMenuByKeys(menuConfig, menuKeys, orgId)
  const siderMenuItems = buildSiderMenuItems(filteredMenuItems)
  const pathOpenKeys = defaultOpenKeysForPath(location.pathname)
  const [openKeys, setOpenKeys] = useState<string[]>(() => pathOpenKeys)
  const fallbackHomePath = useMemo(() => resolveFallbackPath(menuKeys), [menuKeys])
  const currentTitle = findMenuTitle(filteredMenuItems, selectedMenuKey) || '人事管理系统'

  // 跨模块导航时合并展开当前分组，避免 defaultOpenKeys 只生效一次导致分组仍折叠
  useEffect(() => {
    const nextOpen = defaultOpenKeysForPath(location.pathname)
    setOpenKeys((prev) => Array.from(new Set([...prev, ...nextOpen])))
  }, [location.pathname])

  useEffect(() => {
    let cancelled = false

    if (authPaths.includes(location.pathname) || isLoggedIn) {
      setSessionChecking(false)
      return () => {
        cancelled = true
      }
    }

    setSessionChecking(true)
    authAPI.getCurrentUser()
      .then((response: any) => {
        const currentUser = response?.data?.user
        if (!cancelled && currentUser) login(currentUser)
      })
      .catch(() => undefined)
      .finally(() => {
        if (!cancelled) setSessionChecking(false)
      })

    return () => {
      cancelled = true
    }
  }, [isLoggedIn, location.pathname, login])

  useEffect(() => {
    setMobileMenuOpen(false)
  }, [location.pathname])

  useEffect(() => {
    if (!isMobile) setMobileMenuOpen(false)
  }, [isMobile])

  useEffect(() => {
    if (!isLoggedIn) return

    const requestedOrgID = authOrgIDFromSearchParamsOrStorage(new URLSearchParams(location.search))
    if (!requestedOrgID) return

    const currentOrgID = normalizeAuthOrgID(user?.org_id)
    if (currentOrgID === requestedOrgID) {
      rememberAuthOrgID(requestedOrgID)
      return
    }

    let cancelled = false
    const redirectTarget = authRedirectTargetFromLocation(location)

    setAutoLogging(false)
    setSessionChecking(true)
    rememberAuthOrgID(requestedOrgID)
    rememberAuthRedirect(redirectTarget)

    authAPI.logout()
      .catch((err) => {
        console.warn('[auth-org-switch] logout current org session failed', err)
      })
      .finally(() => {
        if (cancelled) return
        logout()
        setSessionChecking(false)
        navigate(loginPathWithRedirectAndOrg(redirectTarget, requestedOrgID), { replace: true })
      })

    return () => {
      cancelled = true
    }
  }, [isLoggedIn, location, logout, navigate, user?.org_id])

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
    if (sessionChecking || !isDingTalkEnv() || isLoggedIn || authPaths.includes(location.pathname)) {
      return
    }

    setAutoLogging(true)
    const redirectTarget = authRedirectTargetFromLocation(location)
    const orgSearchParams = new URLSearchParams(location.search)
    const orgID = authOrgIDFromSearchParamsOrStorage(orgSearchParams)
    const orgParams = orgID ? { org_id: orgID } : undefined
    const navigateToLogin = () => {
      rememberAuthRedirect(redirectTarget)
      navigate(loginPathWithRedirect(redirectTarget), { replace: true })
    }

    const doAutoLogin = async () => {
      try {
        const configRes = await axios.get('/api/v1/auth/dingtalk/config', { params: orgParams, withCredentials: true })
        const { corp_id: corpId, missing } = configRes.data.data
        const dd = (window as any).dd

        if (!corpId || (Array.isArray(missing) && missing.includes('DINGTALK_CORP_ID'))) {
          message.error('缺少 DINGTALK_CORP_ID，暂时无法使用钉钉内免登')
          setAutoLogging(false)
          navigateToLogin()
          return
        }

        if (!dd?.runtime?.permission?.requestAuthCode) {
          message.error('钉钉 JS-SDK 未加载或未授权')
          setAutoLogging(false)
          navigateToLogin()
          return
        }

        dd.runtime.permission.requestAuthCode({
          corpId,
          onSuccess: async (result: { code: string }) => {
            try {
              const response = await axios.post('/api/v1/auth/dingtalk/in-app', {
                code: result.code,
                org_id: orgID || undefined,
              }, { withCredentials: true })
              const { user } = response.data.data
              login(user)
              rememberAuthOrgID(user?.org_id || orgID)
              message.success('登录成功', 0.6)
              setAutoLogging(false)
            } catch (err) {
              console.error('[DingTalk InApp] login failed', err)
              message.error(getAxiosErrorMessage(err, '钉钉内免登失败'))
              setAutoLogging(false)
              navigateToLogin()
            }
          },
          onFail: (err: unknown) => {
            console.error('[DingTalk InApp] requestAuthCode failed', err)
            message.error('获取钉钉授权码失败')
            setAutoLogging(false)
            navigateToLogin()
          },
        })
      } catch (err) {
        console.error('[DingTalk InApp] init failed', err)
        message.error(getAxiosErrorMessage(err, '钉钉内免登初始化失败'))
        setAutoLogging(false)
        navigateToLogin()
      }
    }

    const timer = setTimeout(doAutoLogin, 300)
    return () => clearTimeout(timer)
  }, [isLoggedIn, location, login, navigate, sessionChecking])

  if (authPaths.includes(location.pathname)) {
    return (
      <ConfigProvider locale={appLocale} theme={appTheme} renderEmpty={renderAppEmpty}>
        <AuthRoutes />
      </ConfigProvider>
    )
  }

  if (sessionChecking) {
    return (
      <ConfigProvider locale={appLocale} theme={appTheme} renderEmpty={renderAppEmpty}>
        <PageLoading />
      </ConfigProvider>
    )
  }

  if (!isLoggedIn) {
    if (autoLogging) {
      return (
        <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', background: 'var(--color-bg-page)' }}>
          <div style={{ textAlign: 'center' }}>
            <Spin indicator={<LoadingOutlined style={{ fontSize: 24 }} spin />} />
            <p style={{ marginTop: 16 }}>正在通过钉钉自动登录，请稍候...</p>
          </div>
        </div>
      )
    }

    return (
      <ConfigProvider locale={appLocale} theme={appTheme} renderEmpty={renderAppEmpty}>
        <ErrorBoundary resetKey={location.pathname}>
          <Suspense fallback={<PageLoading />}>
            <Login />
          </Suspense>
        </ErrorBoundary>
      </ConfigProvider>
    )
  }

  return (
    <ConfigProvider locale={appLocale} theme={appTheme} renderEmpty={renderAppEmpty}>
      <MobileTableEnhancer />
      <Layout className={isMobile ? 'app-layout app-layout-mobile' : 'app-layout'}>
        {!isMobile ? (
          <Sider
            className={collapsed ? 'app-sider app-sider-collapsed' : 'app-sider'}
            width={232}
            collapsible
            collapsed={collapsed}
            collapsedWidth={80}
            onCollapse={setCollapsed}
            trigger={null}
          >
            <div className="logo">人事管理系统</div>
            <div className="app-sider-menu-scroll">
              <Menu
                theme="light"
                mode="inline"
                selectedKeys={[selectedMenuKey]}
                openKeys={collapsed ? [] : openKeys}
                onOpenChange={(keys) => setOpenKeys(keys as string[])}
                items={siderMenuItems}
              />
            </div>
            <button
              type="button"
              className="app-sider-trigger"
              aria-label={collapsed ? '展开侧边栏' : '收起侧边栏'}
              onClick={() => setCollapsed(!collapsed)}
            >
              {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            </button>
          </Sider>
        ) : null}
        <Drawer
          className="app-mobile-drawer"
          title="人事管理系统"
          placement="left"
          open={mobileMenuOpen}
          onClose={() => setMobileMenuOpen(false)}
          width={288}
        >
          <Menu
            mode="inline"
            selectedKeys={[selectedMenuKey]}
            openKeys={openKeys}
            onOpenChange={(keys) => setOpenKeys(keys as string[])}
            items={siderMenuItems}
            onClick={() => setMobileMenuOpen(false)}
          />
        </Drawer>
        <Layout className="app-main-layout" style={{ marginLeft: isMobile ? 0 : collapsed ? 80 : 232 }}>
          <Header className="app-header">
            <Button
              type="text"
              className="app-mobile-menu-button"
              icon={<MenuUnfoldOutlined />}
              aria-label="打开菜单"
              onClick={() => setMobileMenuOpen(true)}
            />
            <span className="app-header-title">{isMobile ? currentTitle : ''}</span>
            <span className="app-header-spacer" />
            <span className="app-header-user app-user-chip">{user?.name || '管理员'}</span>
            <Button
              type="text"
              className="app-header-logout"
              icon={<LogoutOutlined />}
              aria-label="退出登录"
              onClick={handleLogout}
            >
              退出登录
            </Button>
          </Header>
          <Content className="app-content">
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
                <Route path="/attendance-export" element={<RouteGuard menuKey="menu:attendance-export"><AttendanceExport /></RouteGuard>} />
                <Route path="/attendance-processing" element={<RouteGuard menuKey="menu:attendance-processing"><AttendanceProcessing /></RouteGuard>} />
                <Route path="/attendance/external-sync" element={<RouteGuard menuKey="menu:attendance-external-sync"><AttendanceExternalSync /></RouteGuard>} />
                <Route path="/attendance-toolbox" element={<RouteGuard menuKey="menu:attendance-toolbox"><AttendanceToolbox /></RouteGuard>} />
                <Route path="/week-schedule" element={<RouteGuard menuKey="menu:week-schedule"><WeekSchedule /></RouteGuard>} />
                <Route path="/employee-shift-config" element={<RouteGuard menuKey="menu:employee-shift-config"><EmployeeShiftConfig /></RouteGuard>} />
                <Route path="/approval" element={<RouteGuard menuKey="menu:approval-templates"><Approval /></RouteGuard>} />
                <Route path="/approval-templates" element={<RouteGuard menuKey="menu:approval-templates"><ApprovalTemplate /></RouteGuard>} />
                <Route path="/approval-instances" element={<RouteGuard menuKey="menu:approval-instances"><ApprovalInstance /></RouteGuard>} />
                <Route path="/approval-detail/:id" element={<RouteGuard menuKey="menu:approval-instances"><ApprovalDetail /></RouteGuard>} />
                <Route path="/approval-stats" element={<RouteGuard menuKey="menu:approval-stats"><ApprovalStats /></RouteGuard>} />
                <Route path="/oa-approval-data" element={<RouteGuard menuKey="menu:oa-approval-data"><OAApprovalData /></RouteGuard>} />
                <Route path="/role-management" element={<RouteGuard menuKey="menu:permission"><RoleManagement /></RouteGuard>} />
                <Route path="/sync-jobs" element={<RouteGuard menuKey="menu:sync-jobs"><SyncJobs /></RouteGuard>} />
                <Route path="/audit-logs" element={<RouteGuard menuKey="menu:audit-logs" permissionCode="audit_log:read"><AuditLogs /></RouteGuard>} />
                <Route path="/employee-profile" element={<RouteGuard menuKey="menu:employee-profile"><EmployeeProfile /></RouteGuard>} />
                <Route path="/employee-flow" element={<RouteGuard menuKey="menu:employee-flow"><EmployeeFlow /></RouteGuard>} />
                <Route path="/talent-analysis" element={<RouteGuard menuKey="menu:talent-analysis"><TalentAnalysis /></RouteGuard>} />
                <Route path="/leave-overtime" element={<RouteGuard menuKey="menu:leave-overtime"><LeaveOvertime /></RouteGuard>} />
                <Route path="/performance-overview" element={<RouteGuard menuKey="menu:performance-overview"><PerformanceOverview /></RouteGuard>} />
                <Route path="/performance-reports" element={<RouteGuard menuKey="menu:performance-reports"><PerformanceReports /></RouteGuard>} />
                <Route path="/performance-interviews" element={<RouteGuard menuKey="menu:performance-interviews" permissionCode={['performance:result:view', 'performance:interview:manage', 'performance:activity:manage', 'performance:department_eval:submit']}><PerformanceInterviews /></RouteGuard>} />
                <Route path="/performance-appeals" element={<RouteGuard menuKey="menu:performance-appeals" permissionCode={['performance:result:view', 'performance:appeal:manage', 'performance:activity:manage', 'performance:hr_review:submit', 'performance:result_publish:manage']}><PerformanceAppeals /></RouteGuard>} />
                <Route path="/performance-indicator-library" element={<RouteGuard menuKey="menu:performance-indicator-library"><PerformanceIndicatorLibrary /></RouteGuard>} />
                {/* 任务深链：menuOptional，钉钉通知只带功能权限也能进，不强制 overview 菜单 */}
                <Route path="/performance-result/:activityId/:participantId" element={<RouteGuard menuKey="menu:performance-overview" menuOptional permissionCode={['performance:result:view', 'performance:activity:manage', 'performance:department_eval:submit', 'performance:hr_review:submit', 'performance:result_publish:manage', 'performance:appeal:manage', 'performance:level_adjust:manage', 'performance:manager_eval:submit', 'performance:manager_confirm:submit']}><PerformanceResultView /></RouteGuard>} />
                <Route path="/performance-self-eval/:activityId/:participantId" element={<RouteGuard menuKey="menu:performance-overview" menuOptional permissionCode="performance:self_eval:submit"><PerformanceSelfEval /></RouteGuard>} />
                <Route path="/performance-manager-eval/:activityId/:participantId" element={<RouteGuard menuKey="menu:performance-overview" menuOptional permissionCode="performance:manager_eval:submit"><PerformanceManagerEval /></RouteGuard>} />
                <Route path="/performance-goal-setting/:activityId/:participantId" element={<RouteGuard menuKey="menu:performance-overview" menuOptional permissionCode="performance:goal:manage"><PerformanceGoalSetting /></RouteGuard>} />
                <Route path="/permission" element={<RouteGuard menuKey="menu:permission"><Permission /></RouteGuard>} />
                <Route path="/log" element={<Navigate to="/audit-logs" replace />} />
                <Route path="/setting" element={<RouteGuard menuKey="menu:setting"><Setting /></RouteGuard>} />
                <Route
                  path="*"
                  element={
                    <Result
                      status="404"
                      title="页面不存在"
                      subTitle="您访问的地址不存在或已被移除。"
                      extra={
                        <Button type="primary" onClick={() => navigate(fallbackHomePath, { replace: true })}>
                          返回首页
                        </Button>
                      }
                    />
                  }
                />
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
