import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthState {
  user: any
  token: string
  isLoggedIn: boolean
  menuKeys: string[]
  login: (user: any, token: string) => void
  setMenuKeys: (keys: string[]) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      token: '',
      isLoggedIn: false,
      menuKeys: [],
      login: (user, token) => set({ user, token, isLoggedIn: true, menuKeys: user?.menu_keys || [] }),
      setMenuKeys: (keys) => set({ menuKeys: keys }),
      logout: () => set({ user: null, token: '', isLoggedIn: false, menuKeys: [] }),
    }),
    {
      name: 'peopleops-auth',
    },
  ),
)
