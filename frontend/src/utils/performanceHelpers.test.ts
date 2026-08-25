import { describe, it, expect } from 'vitest'
import dayjs from 'dayjs'
import {
  normalizeIDArray,
  getListFromResponse,
  getDepartmentOption,
  getUserOption,
  getImportedUserOption,
  mergeSelectOptions,
  normalizeImportedManagerAssignments,
  getAssessmentCandidateOption,
  getAssessmentUserOption,
  formatRangeStart,
  formatRangeEnd,
  formatDateRange,
  getActivityStepIndex,
  getStatusMeta,
  getParticipantStatusMeta,
  resolveActivityStatusFilter,
  STATUS_MAP,
  PARTICIPANT_STATUS_MAP,
  ACTIVITY_FLOW,
} from './performanceHelpers'

describe('normalizeIDArray', () => {
  it('should handle undefined', () => {
    expect(normalizeIDArray(undefined)).toEqual([])
  })

  it('should handle empty string', () => {
    expect(normalizeIDArray('')).toEqual([])
  })

  it('should handle comma-separated string', () => {
    expect(normalizeIDArray('1,2,3')).toEqual(['1', '2', '3'])
  })

  it('should handle array input', () => {
    expect(normalizeIDArray(['1', '2', '3'])).toEqual(['1', '2', '3'])
  })

  it('should filter empty values', () => {
    expect(normalizeIDArray('1,,2,')).toEqual(['1', '2'])
  })

  it('should trim whitespace', () => {
    expect(normalizeIDArray(' 1 , 2 , 3 ')).toEqual(['1', '2', '3'])
  })
})

describe('getListFromResponse', () => {
  it('should handle array response', () => {
    const res = [1, 2, 3]
    expect(getListFromResponse(res, [])).toEqual([1, 2, 3])
  })

  it('should handle nested data', () => {
    const res = { data: { items: [1, 2] } }
    expect(getListFromResponse(res, ['items'])).toEqual([1, 2])
  })

  it('should try multiple keys', () => {
    const res = { data: { users: [1, 2] } }
    expect(getListFromResponse(res, ['items', 'users'])).toEqual([1, 2])
  })

  it('should return empty array if not found', () => {
    const res = { data: {} }
    expect(getListFromResponse(res, ['items'])).toEqual([])
  })
})

describe('getDepartmentOption', () => {
  it('should create option from department', () => {
    const dept = { department_id: 'D001', name: '技术部' }
    expect(getDepartmentOption(dept)).toEqual({
      value: 'D001',
      label: '技术部（D001）',
    })
  })

  it('should handle missing id', () => {
    const dept = { name: '技术部' }
    expect(getDepartmentOption(dept)).toBeNull()
  })

  it('should trim department id', () => {
    expect(getDepartmentOption({ department_id: ' D001 ', name: '技术部' })).toEqual({
      value: 'D001',
      label: '技术部（D001）',
    })
  })
})

describe('getUserOption', () => {
  it('should create option from user', () => {
    const user = { user_id: 'U001', name: '张三', department_name: '技术部' }
    expect(getUserOption(user)).toEqual({
      value: 'U001',
      label: '张三（U001） - 技术部',
    })
  })

  it('should handle missing id', () => {
    const user = { name: '张三' }
    expect(getUserOption(user)).toBeNull()
  })

  it('should not fallback to employee_id or database id', () => {
    expect(getUserOption({ employee_id: 'E001', name: '张三' })).toBeNull()
    expect(getUserOption({ id: 99, name: '张三' })).toBeNull()
  })
})

describe('getImportedUserOption', () => {
  it('should create option from imported user', () => {
    const user = { user_id: 'U001', name: '张三', employee_id: 'E001', department_name: '技术部' }
    expect(getImportedUserOption(user)).toEqual({
      value: 'U001',
      label: '张三（U001 / E001） - 技术部',
    })
  })

  it('should handle missing id', () => {
    const user = { name: '张三' }
    expect(getImportedUserOption(user)).toBeNull()
  })

  it('should not fallback to employee_id or database id without user_id', () => {
    expect(getImportedUserOption({ employee_id: 'E001', name: '张三' })).toBeNull()
    expect(getImportedUserOption({ id: 123, name: '张三' })).toBeNull()
  })
})

describe('mergeSelectOptions', () => {
  it('should merge options without duplicates', () => {
    const base = [{ value: '1', label: 'A' }]
    const extra = [{ value: '2', label: 'B' }]
    expect(mergeSelectOptions(base, extra)).toEqual([
      { value: '1', label: 'A' },
      { value: '2', label: 'B' },
    ])
  })

  it('should skip duplicates', () => {
    const base = [{ value: '1', label: 'A' }]
    const extra = [{ value: '1', label: 'A2' }]
    expect(mergeSelectOptions(base, extra)).toEqual([{ value: '1', label: 'A' }])
  })
})

describe('normalizeImportedManagerAssignments', () => {
  it('should normalize assignments', () => {
    const assignments = [
      {
        user_id: 'U001',
        assessment_manager_user_id: 'M001',
        assessment_manager_name: '李四',
        assessment_manager_source: 'DIRECT_MANAGER' as const,
      },
    ]
    const result = normalizeImportedManagerAssignments(assignments)
    expect(result).toHaveLength(1)
    expect(result[0].user_id).toBe('U001')
    expect(result[0].assessment_manager_user_id).toBe('M001')
  })

  it('should filter by employee IDs', () => {
    const assignments = [
      { user_id: 'U001', assessment_manager_user_id: 'M001', assessment_manager_name: '李四', assessment_manager_source: 'DIRECT_MANAGER' as const },
      { user_id: 'U002', assessment_manager_user_id: 'M002', assessment_manager_name: '王五', assessment_manager_source: 'DIRECT_MANAGER' as const },
    ]
    const result = normalizeImportedManagerAssignments(assignments, ['U001'])
    expect(result).toHaveLength(1)
    expect(result[0].user_id).toBe('U001')
  })
})

describe('getAssessmentCandidateOption', () => {
  it('should create option from candidate', () => {
    const candidate = {
      user_id: 'U001',
      name: '张三',
      employee_no: 'E001',
      department_name: '技术部',
      candidate_source: 'DIRECT_MANAGER' as const,
      candidate_source_label: '直接上级',
    }
    const option = getAssessmentCandidateOption(candidate)
    expect(option).not.toBeNull()
    expect(option!.value).toBe('U001')
    expect(option!.searchText).toContain('张三')
  })

  it('should return null for empty user_id', () => {
    const candidate = { user_id: '', name: '张三', employee_no: 'E001', department_name: '技术部', candidate_source: 'DIRECT_MANAGER' as const, candidate_source_label: '直接上级' }
    expect(getAssessmentCandidateOption(candidate)).toBeNull()
  })
})

describe('getAssessmentUserOption', () => {
  it('should create option from active user', () => {
    const user = { user_id: 'U001', name: '张三', employee_id: 'E001', department_name: '技术部', status: 'active' }
    const option = getAssessmentUserOption(user)
    expect(option).not.toBeNull()
    expect(option!.value).toBe('U001')
  })

  it('should return null for inactive user', () => {
    const user = { user_id: 'U001', name: '张三', status: 'inactive' }
    expect(getAssessmentUserOption(user)).toBeNull()
  })
})

describe('formatRangeStart', () => {
  it('should format start date', () => {
    const range = [dayjs('2024-01-15'), dayjs('2024-01-31')] as [dayjs.Dayjs, dayjs.Dayjs]
    expect(formatRangeStart(range)).toBe('2024-01-15')
  })

  it('should return empty for undefined', () => {
    expect(formatRangeStart(undefined)).toBe('')
  })
})

describe('formatRangeEnd', () => {
  it('should format end date', () => {
    const range = [dayjs('2024-01-15'), dayjs('2024-01-31')] as [dayjs.Dayjs, dayjs.Dayjs]
    expect(formatRangeEnd(range)).toBe('2024-01-31')
  })

  it('should return empty for undefined', () => {
    expect(formatRangeEnd(undefined)).toBe('')
  })
})

describe('formatDateRange', () => {
  it('should format date range', () => {
    expect(formatDateRange('2024-01-15', '2024-01-31')).toBe('2024-01-15 ~ 2024-01-31')
  })

  it('should handle missing start', () => {
    expect(formatDateRange(undefined, '2024-01-31')).toBe('- ~ 2024-01-31')
  })

  it('should handle missing end', () => {
    expect(formatDateRange('2024-01-15', undefined)).toBe('2024-01-15 ~ -')
  })

  it('should handle both missing', () => {
    expect(formatDateRange(undefined, undefined)).toBe('-')
  })
})

describe('getActivityStepIndex', () => {
  it('should return 0 for draft', () => {
    expect(getActivityStepIndex('draft')).toBe(0)
  })

  it('should return correct index for status', () => {
    expect(getActivityStepIndex('target_setting')).toBe(0)
    expect(getActivityStepIndex('self_evaluation')).toBe(1)
    expect(getActivityStepIndex('manager_evaluation')).toBe(2)
  })

  it('should return last index for locked', () => {
    expect(getActivityStepIndex('locked')).toBe(ACTIVITY_FLOW.length - 1)
  })

  it('should return 0 for unknown status', () => {
    expect(getActivityStepIndex('unknown')).toBe(0)
  })
})

describe('getStatusMeta', () => {
  it('should return correct status meta', () => {
    expect(getStatusMeta('draft')).toEqual({ label: '草稿', color: 'default' })
    expect(getStatusMeta('self_evaluation')).toEqual({ label: '自评中', color: 'processing' })
  })

  it('should return default for unknown status', () => {
    expect(getStatusMeta('unknown')).toEqual({ label: 'unknown', color: 'default' })
  })

  it('should handle undefined', () => {
    expect(getStatusMeta(undefined)).toEqual({ label: '-', color: 'default' })
  })
})

describe('getParticipantStatusMeta', () => {
  it('should return correct participant status meta', () => {
    expect(getParticipantStatusMeta('pending')).toEqual({ label: '待目标', color: 'default' })
    expect(getParticipantStatusMeta('self_submitted')).toEqual({ label: '已自评', color: 'processing' })
  })

  it('should return default for unknown status', () => {
    expect(getParticipantStatusMeta('unknown')).toEqual({ label: 'unknown', color: 'default' })
  })
})

describe('resolveActivityStatusFilter', () => {
  it('should return undefined for empty filter', () => {
    expect(resolveActivityStatusFilter(undefined)).toBeUndefined()
    expect(resolveActivityStatusFilter('')).toBeUndefined()
  })

  it('should resolve in-progress filter', () => {
    const result = resolveActivityStatusFilter('__in_progress__')
    expect(result).toContain('target_setting')
    expect(result).toContain('self_evaluation')
  })

  it('should resolve confirmed filter', () => {
    const result = resolveActivityStatusFilter('__confirmed__')
    expect(result).toContain('locked')
    expect(result).toContain('result_confirmed')
  })

  it('should return single status array for specific status', () => {
    expect(resolveActivityStatusFilter('draft')).toEqual(['draft'])
  })
})

describe('STATUS_MAP', () => {
  it('should have all required statuses', () => {
    expect(STATUS_MAP.draft).toBeDefined()
    expect(STATUS_MAP.self_evaluation).toBeDefined()
    expect(STATUS_MAP.manager_evaluation).toBeDefined()
    expect(STATUS_MAP.locked).toBeDefined()
    expect(STATUS_MAP.archived).toBeDefined()
  })
})

describe('PARTICIPANT_STATUS_MAP', () => {
  it('should have all required participant statuses', () => {
    expect(PARTICIPANT_STATUS_MAP.pending).toBeDefined()
    expect(PARTICIPANT_STATUS_MAP.self_submitted).toBeDefined()
    expect(PARTICIPANT_STATUS_MAP.manager_submitted).toBeDefined()
    expect(PARTICIPANT_STATUS_MAP.locked).toBeDefined()
  })
})
