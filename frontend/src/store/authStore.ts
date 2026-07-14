import { create } from 'zustand'
import { clearRememberedOrgId } from '../utils/org'

interface AuthState {
  user: any
  isLoggedIn: boolean
  menuKeys: string[]
  permissions: string[]
  orgId: string  // 当前组织ID
  login: (user: any) => void
  setMenuKeys: (keys: string[]) => void
  setPermissions: (perms: string[]) => void
  logout: () => void
}

// 安全模型：token 存放于 HttpOnly cookie，JS 不再持有 token，也不再 persist 到 localStorage
// （避免 XSS 窃取）。刷新后由 App 调用 /me 携带 cookie 重新水合用户信息，orgId 亦随之恢复。
export const useAuthStore = create<AuthState>()((set) => ({
  user: null,
  isLoggedIn: false,
  menuKeys: [],
  permissions: [],
  orgId: '',
  login: (user) => set({
    user,
    isLoggedIn: true,
    menuKeys: user?.menu_keys || [],
    permissions: user?.permissions || [],
    orgId: user?.org_id || '',
  }),
  setMenuKeys: (keys) => set({ menuKeys: keys }),
  setPermissions: (perms) => set({ permissions: perms }),
  logout: () => {
    clearRememberedOrgId()
    set({ user: null, isLoggedIn: false, menuKeys: [], permissions: [], orgId: '' })
  },
}))
