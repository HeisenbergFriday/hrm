import { describe, expect, it } from 'vitest'
import { filterMenuByKeys, menuConfig, menuPermissionKey } from './menu'

describe('organization-scoped menu', () => {
  const keys = [menuPermissionKey('oa-approval-data')]

  it('shows OA approval data for muteng', () => {
    const items = filterMenuByKeys(menuConfig, keys, 'muteng')
    expect(items.flatMap((item) => item.children || []).some((item) => item.key === menuPermissionKey('oa-approval-data'))).toBe(true)
  })

  it('hides OA approval data for other organizations', () => {
    const items = filterMenuByKeys(menuConfig, keys, 'xiaotie')
    expect(items.flatMap((item) => item.children || []).some((item) => item.key === menuPermissionKey('oa-approval-data'))).toBe(false)
  })
})
