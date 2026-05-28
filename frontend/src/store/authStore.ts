import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthState {
  user: any
  token: string
  isLoggedIn: boolean
  menuKeys: string[]
  permissions: string[]
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
      login: (user, token) => set({ user, token, isLoggedIn: true, menuKeys: user?.menu_keys || [], permissions: user?.permissions || [] }),
      setMenuKeys: (keys) => set({ menuKeys: keys }),
      setPermissions: (perms) => set({ permissions: perms }),
      logout: () => set({ user: null, token: '', isLoggedIn: false, menuKeys: [], permissions: [] }),
    }),
    {
      name: 'peopleops-auth',
    },
  ),
)
