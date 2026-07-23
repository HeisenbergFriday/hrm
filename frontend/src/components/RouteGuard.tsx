import React, { useMemo } from 'react'
import { Result, Button } from 'antd'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '../store/authStore'
import { menuConfig, menuPermissionKey, type MenuItem } from '../config/menu'

interface RouteGuardProps {
  /**
   * 菜单权限 key。默认必须命中 menuKeys 才放行。
   * 任务深链页可配合 menuOptional，仅用功能权限放行。
   */
  menuKey: string
  /**
   * 功能权限：有任意一个即通过。
   * 与 menuOptional 合用时，深链入口只靠功能权限，不强制 overview 菜单。
   */
  permissionCode?: string | string[]
  /**
   * true：不强制菜单 key（适合钉钉通知跳转的自评/评分/目标/结果页）。
   * 仍会校验 permissionCode（若提供）；二者都缺则拒绝。
   */
  menuOptional?: boolean
  children: React.ReactNode
}

function normalizePermissionCodes(permissionCode?: string | string[]) {
  if (!permissionCode) return [] as string[]
  return Array.isArray(permissionCode) ? permissionCode : [permissionCode]
}

/** 菜单 key → 首个可达路由 path（叶子 Link 的 to） */
function collectMenuPathMap(items: MenuItem[], map: Map<string, string> = new Map()) {
  for (const item of items) {
    if (item.children?.length) {
      collectMenuPathMap(item.children, map)
      continue
    }
    const key = menuPermissionKey(item.key)
    if (map.has(key)) continue
    const label = item.label
    if (React.isValidElement(label)) {
      const to = (label.props as { to?: string })?.to
      if (typeof to === 'string' && to.startsWith('/')) {
        map.set(key, to)
      }
    }
  }
  return map
}

const MENU_KEY_TO_PATH = collectMenuPathMap(menuConfig)

/** 有 menu:home 回 /；否则第一个有权叶子路由；都没有则仍回 /（Home 空态） */
export function resolveFallbackPath(menuKeys: string[] | undefined | null): string {
  const keys = (menuKeys || []).map(menuPermissionKey)
  if (keys.length === 0) return '/'
  if (keys.includes(menuPermissionKey('home'))) return '/'
  for (const key of keys) {
    const path = MENU_KEY_TO_PATH.get(key)
    if (path) return path
  }
  return '/'
}

export default function RouteGuard({
  menuKey,
  permissionCode,
  menuOptional = false,
  children,
}: RouteGuardProps) {
  const navigate = useNavigate()
  const { menuKeys, permissions } = useAuthStore()
  const normalizedMenuKey = menuPermissionKey(menuKey)
  const isHomeMenu = normalizedMenuKey === menuPermissionKey('home')
  const normalizedMenuKeys = new Set((menuKeys || []).map(menuPermissionKey))
  const permissionCodes = normalizePermissionCodes(permissionCode)
  const hasFeaturePermission =
    permissionCodes.length === 0 || permissionCodes.some(code => permissions.includes(code))
  const fallbackPath = useMemo(() => resolveFallbackPath(menuKeys), [menuKeys])
  const goHome = () => navigate(fallbackPath, { replace: true })

  // 无任何菜单：首页放行给 Home 空态；任务深链若功能权限足够也放行
  if (!menuKeys || menuKeys.length === 0) {
    if (isHomeMenu) {
      return <>{children}</>
    }
    if (menuOptional && permissionCodes.length > 0 && hasFeaturePermission) {
      return <>{children}</>
    }
    return (
      <Result
        status="403"
        title="无访问权限"
        subTitle="您尚未被分配任何角色，请联系管理员。"
        extra={<Button type="primary" onClick={goHome}>返回首页</Button>}
      />
    )
  }

  const hasMenu = normalizedMenuKeys.has(normalizedMenuKey)
  if (!menuOptional && !hasMenu) {
    return (
      <Result
        status="403"
        title="无访问权限"
        subTitle="您没有访问此页面的权限。"
        extra={<Button type="primary" onClick={goHome}>返回首页</Button>}
      />
    )
  }

  // 菜单可选时：有菜单或有功能权限即可继续到功能权限校验
  if (menuOptional && !hasMenu && permissionCodes.length === 0) {
    return (
      <Result
        status="403"
        title="无访问权限"
        subTitle="您没有访问此页面的权限。"
        extra={<Button type="primary" onClick={goHome}>返回首页</Button>}
      />
    )
  }

  if (permissionCodes.length > 0 && !hasFeaturePermission) {
    return (
      <Result
        status="403"
        title="无访问权限"
        subTitle="您没有访问此功能的操作权限。"
        extra={<Button type="primary" onClick={goHome}>返回首页</Button>}
      />
    )
  }

  return <>{children}</>
}
