import { useAuthStore } from '../store/authStore'
import { menuPermissionKey } from '../config/menu'

export function hasPermission(permissionCode: string): boolean {
  const permissions = useAuthStore.getState().permissions || []
  return permissions.includes(permissionCode)
}

export function hasMenuPermission(menuKey: string): boolean {
  const menuKeys = useAuthStore.getState().menuKeys || []
  const normalizedMenuKeys = new Set(menuKeys.map(menuPermissionKey))
  return normalizedMenuKeys.has(menuPermissionKey(menuKey))
}
