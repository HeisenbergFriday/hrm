import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthState {
  user: any
  token: string
  isLoggedIn: boolean
  login: (user: any, token: string) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      token: '',
      isLoggedIn: false,
      login: (user, token) => set({ user, token, isLoggedIn: true }),
      logout: () => set({ user: null, token: '', isLoggedIn: false }),
    }),
    {
      name: 'peopleops-auth',
    },
  ),
)
