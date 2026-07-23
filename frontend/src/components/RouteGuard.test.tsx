import React from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import RouteGuard from './RouteGuard'
import { useAuthStore } from '../store/authStore'

const mockNavigate = vi.fn()

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  Link: ({ children, ...props }: any) => <a {...props}>{children}</a>,
  NavLink: ({ children, ...props }: any) => <a {...props}>{children}</a>,
}))

describe('RouteGuard', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    useAuthStore.setState({
      user: { name: 'tester' },
      isLoggedIn: true,
      menuKeys: [],
      permissions: [],
      orgId: 'org-1',
    })
  })

  it('空 menuKeys 时允许首页进入（避免 403 死循环）', () => {
    render(
      <RouteGuard menuKey="menu:home">
        <div data-testid="home-ok">home</div>
      </RouteGuard>,
    )
    expect(screen.getByTestId('home-ok')).toBeInTheDocument()
    expect(screen.queryByText('无访问权限')).not.toBeInTheDocument()
  })

  it('空 menuKeys 时非首页仍 403，返回首页可导航到 /', async () => {
    const user = userEvent.setup()
    render(
      <RouteGuard menuKey="menu:attendance">
        <div>secret</div>
      </RouteGuard>,
    )
    expect(screen.getByText('无访问权限')).toBeInTheDocument()
    expect(screen.queryByText('secret')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '返回首页' }))
    expect(mockNavigate).toHaveBeenCalledWith('/', { replace: true })
  })

  it('无 menu:home 时「返回首页」落到第一个有权叶子路由', async () => {
    const user = userEvent.setup()
    useAuthStore.setState({
      menuKeys: ['menu:attendance', 'menu:performance-overview'],
      permissions: [],
    })
    render(
      <RouteGuard menuKey="menu:setting">
        <div>blocked</div>
      </RouteGuard>,
    )
    expect(screen.getByText('无访问权限')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '返回首页' }))
    expect(mockNavigate).toHaveBeenCalledWith('/attendance', { replace: true })
  })

  it('menuOptional + 功能权限：无 overview 菜单也能进任务页', () => {
    useAuthStore.setState({
      menuKeys: ['menu:home'],
      permissions: ['performance:self_eval:submit'],
    })
    render(
      <RouteGuard
        menuKey="menu:performance-overview"
        menuOptional
        permissionCode="performance:self_eval:submit"
      >
        <div data-testid="self-eval-ok">self-eval</div>
      </RouteGuard>,
    )
    expect(screen.getByTestId('self-eval-ok')).toBeInTheDocument()
  })

  it('menuOptional 但缺少功能权限：拒绝', () => {
    useAuthStore.setState({
      menuKeys: ['menu:home'],
      permissions: [],
    })
    render(
      <RouteGuard
        menuKey="menu:performance-overview"
        menuOptional
        permissionCode="performance:self_eval:submit"
      >
        <div>self-eval</div>
      </RouteGuard>,
    )
    expect(screen.getByText('无访问权限')).toBeInTheDocument()
    expect(screen.getByText('您没有访问此功能的操作权限。')).toBeInTheDocument()
  })

  it('非 menuOptional：无菜单 key 时拒绝（即使有功能权限）', () => {
    useAuthStore.setState({
      menuKeys: ['menu:home'],
      permissions: ['performance:self_eval:submit'],
    })
    render(
      <RouteGuard menuKey="menu:performance-overview" permissionCode="performance:self_eval:submit">
        <div>blocked</div>
      </RouteGuard>,
    )
    expect(screen.getByText('您没有访问此页面的权限。')).toBeInTheDocument()
  })

  it('空 menuKeys + menuOptional + 功能权限：深链仍可进', () => {
    useAuthStore.setState({
      menuKeys: [],
      permissions: ['performance:manager_eval:submit'],
    })
    render(
      <RouteGuard
        menuKey="menu:performance-overview"
        menuOptional
        permissionCode="performance:manager_eval:submit"
      >
        <div data-testid="mgr-ok">mgr</div>
      </RouteGuard>,
    )
    expect(screen.getByTestId('mgr-ok')).toBeInTheDocument()
  })
})
