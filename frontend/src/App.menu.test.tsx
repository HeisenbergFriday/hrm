import { describe, expect, it } from 'vitest'
import { buildSiderMenuItems } from './App'

describe('buildSiderMenuItems', () => {
  it('does not add logout to the sider when the user has no visible business menus', () => {
    const items = buildSiderMenuItems([])

    expect(items).toEqual([])
  })
})
