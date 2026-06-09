import '@testing-library/jest-dom'
import { vi, afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'
import { message, Modal } from 'antd'

// 每个用例结束后自动卸载组件，避免跨用例 DOM 残留导致随机失败
afterEach(() => {
  cleanup()
  message.destroy()
  Modal.destroyAll()
})

// ==================== jsdom 浏览器 API Polyfills ====================
// jsdom 不实现以下浏览器 API，但 antd（响应式栅格、Table、Select、Drawer 等）
// 在运行时会用到。缺失会导致 antd 走 findDOMNode 回退路径并产生随机失败。

// window.matchMedia —— antd useBreakpoint / Grid 依赖
if (!window.matchMedia) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    configurable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }),
  })
}

// ResizeObserver —— antd rc-resize-observer（Table/Select/Tooltip）依赖
class ResizeObserverMock {
  observe = vi.fn()
  unobserve = vi.fn()
  disconnect = vi.fn()
}
;(globalThis as unknown as { ResizeObserver: unknown }).ResizeObserver = ResizeObserverMock
;(window as unknown as { ResizeObserver: unknown }).ResizeObserver = ResizeObserverMock

// IntersectionObserver —— 懒加载 / 虚拟列表依赖
class IntersectionObserverMock {
  root = null
  rootMargin = ''
  thresholds = []
  observe = vi.fn()
  unobserve = vi.fn()
  disconnect = vi.fn()
  takeRecords = vi.fn(() => [])
}
;(globalThis as unknown as { IntersectionObserver: unknown }).IntersectionObserver = IntersectionObserverMock
;(window as unknown as { IntersectionObserver: unknown }).IntersectionObserver = IntersectionObserverMock

// getComputedStyle 对伪元素返回空对象（jsdom 不支持伪元素查询）
const originalGetComputedStyle = window.getComputedStyle
window.getComputedStyle = ((elt: Element, pseudoElt?: string | null) => {
  if (pseudoElt) {
    return {} as CSSStyleDeclaration
  }
  return originalGetComputedStyle(elt)
}) as typeof window.getComputedStyle

// Element 滚动相关 API（jsdom 未实现）
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = vi.fn()
}
if (!Element.prototype.scrollTo) {
  Element.prototype.scrollTo = vi.fn()
}
window.scrollTo = vi.fn() as typeof window.scrollTo

// matchMedia 在部分 antd 内部以 getComputedStyle 兜底，确保 requestAnimationFrame 存在
if (!window.requestAnimationFrame) {
  window.requestAnimationFrame = ((cb: FrameRequestCallback) => {
    const id = window.setTimeout(() => cb(performance.now()), 0)
    return id
  }) as typeof window.requestAnimationFrame
  window.cancelAnimationFrame = ((id: number) => window.clearTimeout(id)) as typeof window.cancelAnimationFrame
}
