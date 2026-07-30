import { describe, expect, it } from 'vitest'
import type { OrgSyncResponse } from '../services/api'
import { formatSyncResult } from './orgSyncAction'

const baseResponse = (): OrgSyncResponse => ({
  overall_status: 'success',
  departments: { status: 'success', success_count: 12, fail_count: 0, error: '' },
  employees: {
    status: 'success',
    success_count: 528,
    fail_count: 0,
    error: '',
    position_missing_count: 0,
    overwrite_empty: false,
    default_role_assigned_count: 0,
  },
  sync_time: '2026-07-28T00:00:00Z',
  duration_ms: 1000,
  request_id: 'request-1',
})

describe('组织同步智能人事字段提示', () => {
  it('展示各字段缺失人数', () => {
    const response = baseResponse()
    response.employees.employment_type_missing_count = 3
    response.employees.job_level_missing_count = 4
    response.employees.job_family_missing_count = 5
    response.employees.regularization_date_missing_count = 6

    const message = formatSyncResult(response)
    expect(message).toContain('员工类型 3 人')
    expect(message).toContain('职级 4 人')
    expect(message).toContain('岗位序列 5 人')
    expect(message).toContain('转正日期 6 人')
  })

  it('智能人事接口失败时说明基础资料已同步', () => {
    const response = baseResponse()
    response.overall_status = 'partial_failed'
    response.employees.status = 'partial_failed'
    response.employees.hrm_field_status = 'failed'
    response.employees.hrm_field_error = '<html>Forbidden: token=third-party-secret</html>'

    const message = formatSyncResult(response)
    expect(message).toContain('员工基础资料同步成功（528 名）')
    expect(message).toContain('智能人事字段同步失败，请检查钉钉应用的智能人事花名册权限')
    expect(message).not.toContain('失败 0')
    expect(message).not.toContain('Forbidden')
    expect(message).not.toContain('third-party-secret')
  })

  it('智能人事接口成功但字段全空时显示诊断信息', () => {
    const response = baseResponse()
    response.employees.hrm_field_status = 'success_no_fields'
    response.employees.hrm_field_error = '智能人事接口调用成功，但未获取到员工类型、职级、岗位序列字段，请检查钉钉应用的花名册字段权限或字段代码配置'

    const message = formatSyncResult(response)
    expect(message).toContain('同步成功')
    expect(message).toContain('智能人事接口调用成功，但未获取到员工类型、职级、岗位序列字段')
    expect(message).not.toContain('智能人事字段同步失败')
  })
})
