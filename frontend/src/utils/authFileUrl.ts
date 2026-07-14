import { useEffect, useState } from 'react'

export function isProtectedFileUrl(url: string): boolean {
  if (!url) return false
  if (url.startsWith('/api/v1/files/')) return true
  try {
    const parsed = new URL(url, window.location.origin)
    return parsed.pathname.startsWith('/api/v1/files/')
  } catch {
    return false
  }
}

export function withFileAccessToken(url: string): string {
  return url
}

export function useAuthorizedFileUrl(url?: string): string {
  const [resolvedUrl, setResolvedUrl] = useState('')

  useEffect(() => {
    let revokedUrl = ''
    let cancelled = false

    if (!url) {
      setResolvedUrl('')
      return () => undefined
    }

    if (!isProtectedFileUrl(url)) {
      setResolvedUrl(url)
      return () => undefined
    }

    if (typeof URL.createObjectURL !== 'function') {
      setResolvedUrl(url)
      return () => undefined
    }

    fetchAuthorizedFile(url)
      .then((blob) => {
      if (cancelled) return
      revokedUrl = URL.createObjectURL(blob)
        setResolvedUrl(revokedUrl)
      })
      .catch(() => {
        if (!cancelled) setResolvedUrl('')
      })

    return () => {
      cancelled = true
      if (revokedUrl && typeof URL.revokeObjectURL === 'function') URL.revokeObjectURL(revokedUrl)
    }
  }, [url])

  return resolvedUrl
}

export async function fetchAuthorizedFile(url: string): Promise<Blob> {
  const response = await fetch(url, {
    credentials: 'include',
  })

  if (!response.ok) {
    throw new Error('file request failed')
  }

  return response.blob()
}

export async function openAuthorizedFile(url: string): Promise<void> {
  if (!isProtectedFileUrl(url)) {
    window.open(url, '_blank', 'noopener,noreferrer')
    return
  }

  const blob = await fetchAuthorizedFile(url)
  const objectUrl = URL.createObjectURL(blob)
  window.open(objectUrl, '_blank', 'noopener,noreferrer')
  window.setTimeout(() => URL.revokeObjectURL(objectUrl), 60_000)
}

export async function downloadAuthorizedFile(url: string, filename?: string): Promise<void> {
  if (!isProtectedFileUrl(url)) {
    const link = document.createElement('a')
    link.href = url
    link.download = filename || filenameFromUrl(url)
    link.rel = 'noopener noreferrer'
    link.click()
    return
  }

  const blob = await fetchAuthorizedFile(url)
  const objectUrl = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = objectUrl
  link.download = filename || filenameFromUrl(url)
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(objectUrl)
}

export function filenameFromUrl(url: string): string {
  const path = url.split('?')[0]
  return decodeURIComponent(path.split('/').pop() || 'download')
}
