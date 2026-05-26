import React from 'react'
import { Result, Button } from 'antd'
import { useAuthStore } from '../store/authStore'

interface RouteGuardProps {
  menuKey: string
  children: React.ReactNode
}

export default function RouteGuard({ menuKey, children }: RouteGuardProps) {
  const { menuKeys } = useAuthStore()

  if (menuKeys.length === 0) {
    return (
      <Result
        status="403"
        title="无访问权限"
        subTitle="您尚未被分配任何角色，请联系管理员。"
        extra={<Button type="primary" onClick={() => window.location.href = '/'}>返回首页</Button>}
      />
    )
  }

  if (!menuKeys.includes(menuKey)) {
    return (
      <Result
        status="403"
        title="无访问权限"
        subTitle="您没有访问此页面的权限。"
        extra={<Button type="primary" onClick={() => window.location.href = '/'}>返回首页</Button>}
      />
    )
  }

  return <>{children}</>
}
