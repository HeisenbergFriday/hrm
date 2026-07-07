import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { clearRememberedOrgId } from '../utils/org'

interface AuthState {
  user: any
  token: string
  isLoggedIn: boolean
  menuKeys: string[]
  permissions: string[]
  orgId: string  // 当前组织ID
  login: (user: any, token: string) => void
  setMenuKeys: (keys: string[]) => void
  setPermissions: (perms: string[]) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      token: '',
      isLoggedIn: false,
      menuKeys: [],
      permissions: [],
      orgId: '',
      login: (user, token) => set({
        user,
        token,
        isLoggedIn: true,
        menuKeys: user?.menu_keys || [],
        permissions: user?.permissions || [],
        orgId: user?.org_id || ''
      }),
      setMenuKeys: (keys) => set({ menuKeys: keys }),
      setPermissions: (perms) => set({ permissions: perms }),
      logout: () => {
        clearRememberedOrgId()
        set({ user: null, token: '', isLoggedIn: false, menuKeys: [], permissions: [], orgId: '' })
      },
    }),
    {
      name: 'peopleops-auth',
    },
  ),
)
