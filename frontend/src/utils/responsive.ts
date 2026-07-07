import { useEffect, useState } from 'react'

const mobileUserAgentPattern = /Android|iPhone|iPod|iPad|Mobile|Windows Phone/i
const dingTalkPattern = /DingTalk/i
const forceMobileParams = ['mobile', 'force_mobile', 'dingtalk_mobile']
const mobileRuntimeClassName = 'peopleops-mobile-runtime'

function getMinScreenSide(): number {
  if (typeof window === 'undefined' || !window.screen) return 0
  return Math.min(window.screen.width || 0, window.screen.height || 0)
}

function matchesMedia(query: string): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false

  try {
    return window.matchMedia(query).matches
  } catch {
    return false
  }
}

function hasForcedMobileFlag(): boolean {
  if (typeof window === 'undefined') return false

  try {
    const params = new URLSearchParams(window.location.search)
    const forcedByQuery = forceMobileParams.some((key) => {
      const value = params.get(key)
      return value === '1' || value === 'true' || value === 'yes'
    })
    if (forcedByQuery) {
      window.sessionStorage.setItem('peopleops-mobile-layout', '1')
      return true
    }

    return window.sessionStorage.getItem('peopleops-mobile-layout') === '1'
  } catch {
    return false
  }
}

export function isMobileRuntime(): boolean {
  if (typeof window === 'undefined' || typeof navigator === 'undefined') {
    return false
  }

  if (hasForcedMobileFlag()) return true

  const viewportWidth = Math.min(
    window.innerWidth || 0,
    window.visualViewport?.width || window.innerWidth || 0,
    document.documentElement.clientWidth || window.innerWidth || 0,
  )
  const minScreenSide = getMinScreenSide()
  const maxScreenSide = typeof window.screen === 'undefined'
    ? 0
    : Math.max(window.screen.width || 0, window.screen.height || 0)
  const ua = navigator.userAgent || ''
  const isMobileUA = mobileUserAgentPattern.test(ua)
  const hasTouch = navigator.maxTouchPoints > 0 || 'ontouchstart' in window
  const coarsePointer = matchesMedia('(pointer: coarse)') || matchesMedia('(any-pointer: coarse)')
  const noHover = matchesMedia('(hover: none)') || matchesMedia('(any-hover: none)')
  const cssSmallDevice = matchesMedia('(max-device-width: 820px)')
  const portraitTouchViewport = hasTouch && matchesMedia('(orientation: portrait)') && viewportWidth <= 1024
  const isTouchSmallScreen = hasTouch && (
    cssSmallDevice ||
    (minScreenSide > 0 && minScreenSide <= 820) ||
    (maxScreenSide > 0 && maxScreenSide <= 1180 && viewportWidth <= 1024) ||
    (coarsePointer && noHover && viewportWidth <= 1180) ||
    portraitTouchViewport
  )
  const isDingTalkMobile = dingTalkPattern.test(ua) && (isMobileUA || isTouchSmallScreen || coarsePointer || hasTouch)

  return viewportWidth < 768 || isMobileUA || isTouchSmallScreen || isDingTalkMobile
}

function syncMobileRuntimeClass(isMobile: boolean) {
  if (typeof document === 'undefined') return
  document.documentElement.classList.toggle(mobileRuntimeClassName, isMobile)
}

export function useMobileRuntime(): boolean {
  const [isMobile, setIsMobile] = useState(() => isMobileRuntime())

  useEffect(() => {
    const update = () => {
      const nextIsMobile = isMobileRuntime()
      syncMobileRuntimeClass(nextIsMobile)
      setIsMobile(nextIsMobile)
    }

    update()
    window.addEventListener('resize', update)
    window.addEventListener('orientationchange', update)
    return () => {
      window.removeEventListener('resize', update)
      window.removeEventListener('orientationchange', update)
    }
  }, [])

  return isMobile
}

export function resolveMobileLayout(mdBreakpoint: boolean | undefined, mobileRuntime: boolean): boolean {
  if (mobileRuntime) return true
  if (mdBreakpoint === undefined) return false
  return !mdBreakpoint
}
