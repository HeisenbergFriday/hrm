import { describe, expect, it } from 'vitest'
import { buildSiderMenuItems } from './App'
import { filterMenuByKeys, menuConfig, menuPermissionKey } from './config/menu'

describe('buildSiderMenuItems', () => {
  it('does not add logout to the sider when the user has no visible business menus', () => {
    const items = buildSiderMenuItems([])

    expect(items).toEqual([])
  })

  it('考勤处理只展示工具箱入口，不再并列展示旧数据处理入口', () => {
    const items = filterMenuByKeys(menuConfig, [menuPermissionKey('attendance-processing')])
    const attendanceItems = items.flatMap((item) => item.children || [])

    expect(attendanceItems.some((item) => item.key === menuPermissionKey('attendance-toolbox'))).toBe(true)
    expect(attendanceItems.some((item) => item.key === menuPermissionKey('attendance-processing'))).toBe(false)
  })
})
