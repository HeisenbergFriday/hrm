import { create } from 'zustand'

interface AuthState {
  user: any
  isLoggedIn: boolean
  menuKeys: string[]
  permissions: string[]
  login: (user: any) => void
  setMenuKeys: (keys: string[]) => void
  setPermissions: (perms: string[]) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>()(
  (set) => ({
    user: null,
    isLoggedIn: false,
    menuKeys: [],
    permissions: [],
    login: (user) => set({ user, isLoggedIn: true, menuKeys: user?.menu_keys || [], permissions: user?.permissions || [] }),
    setMenuKeys: (keys) => set({ menuKeys: keys }),
    setPermissions: (perms) => set({ permissions: perms }),
    logout: () => set({ user: null, isLoggedIn: false, menuKeys: [], permissions: [] }),
  }),
)
