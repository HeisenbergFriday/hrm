import { QueryClient } from '@tanstack/react-query'

/** 单例 QueryClient：登出/换组织时必须 clear，避免跨用户/跨 org 缓存泄漏 */
export const queryClient = new QueryClient()
