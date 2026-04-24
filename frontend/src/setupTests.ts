import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'

vi.mock('./services/api', async () => {
  const mocks = await import('./services/api.mock')

  return {
    authAPI: mocks.authAPIMock,
    userAPI: mocks.userAPIMock,
    departmentAPI: mocks.departmentAPIMock,
    attendanceAPI: mocks.attendanceAPIMock,
    syncAPI: mocks.syncAPIMock,
    orgAPI: mocks.orgAPIMock,
  }
})

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

Object.defineProperty(window, 'scrollTo', {
  writable: true,
  value: vi.fn(),
})

class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}

;(globalThis as any).ResizeObserver = ResizeObserverMock

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})
