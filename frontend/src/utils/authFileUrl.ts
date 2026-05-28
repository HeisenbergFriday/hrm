import { useAuthStore } from '../store/authStore'

export function withFileAccessToken(url: string): string {
  if (!url || !url.startsWith('/api/v1/files/')) {
    return url
  }

  const token = useAuthStore.getState().token
  if (!token) {
    return url
  }

  const separator = url.includes('?') ? '&' : '?'
  return `${url}${separator}access_token=${encodeURIComponent(token)}`
}
