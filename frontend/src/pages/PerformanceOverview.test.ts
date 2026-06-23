import { describe, it, expect } from 'vitest'

// ==================== 辅助函数测试 ====================

// 复制自 PerformanceOverview.tsx 的辅助函数
function normalizeIDArray(value?: string[] | string): string[] {
  if (Array.isArray(value)) return value.filter(Boolean)
  if (!value) return []
  return String(value).split(',').map(item => item.trim()).filter(Boolean)
}

function getListFromResponse(res: any, keys: string[]): any[] {
  const data = res?.data || res
  if (Array.isArray(data)) return data
  for (const key of keys) {
    if (Array.isArray(data?.[key])) return data[key]
  }
  return []
}

function getDepartmentOption(department: any) {
  const value = String(department.department_id || department.id || '')
  const name = department.name || department.department_name || value
  return value ? { value, label: `${name}（${value}）` } : null
}

function getUserOption(user: any) {
  const value = String(user.user_id || user.employee_id || user.id || '')
  const name = user.name || user.user_name || user.employee_name || value
  const departmentName = user.department_name ? ` - ${user.department_name}` : ''
  return value ? { value, label: `${name}（${value}）${departmentName}` } : null
}

function getImportedUserOption(user: any) {
  const value = String(user?.user_id || user?.employee_id || user?.id || '').trim()
  if (!value) return null
  const employeeID = String(user?.employee_id || '').trim()
  const name = String(user?.name || user?.user_name || user?.employee_name || value).trim()
  const employeeIDText = employeeID && employeeID !== value ? ` / ${employeeID}` : ''
  const departmentName = user?.department_name ? ` - ${user.department_name}` : ''
  return { value, label: `${name}（${value}${employeeIDText}）${departmentName}` }
}

function mergeSelectOptions(baseOptions: { label: any; value: string | number }[], extraOptions: { label: any; value: string | number }[]) {
  const merged = [...baseOptions]
  const seen = new Set(baseOptions.map(option => String(option.value)))

  extraOptions.forEach(option => {
    const key = String(option.value)
    if (!key || seen.has(key)) return
    seen.add(key)
    merged.push(option)
  })

  return merged
}

function getImportedUserOptions(
  result: { employees?: { user_id?: string; employee_id?: string; name?: string; department_name?: string }[] } | null | undefined,
  employeeIDs: string[],
) {
  const detailOptions = (result?.employees || []).flatMap(employee => {
    const option = getImportedUserOption(employee)
    return option ? [option] : []
  })
  const fallbackOptions = employeeIDs.flatMap(employeeID => {
    const option = getImportedUserOption({ user_id: employeeID })
    return option ? [option] : []
  })
  return mergeSelectOptions(detailOptions, fallbackOptions)
}

type AssessmentManagerAssignment = {
  user_id?: string
  employee_id?: string
  assessment_manager_user_id?: string
  assessment_manager_employee_id?: string
  assessment_manager_name?: string
  assessment_manager_source?: string
  manager_override_reason?: string
}

function normalizeImportedManagerAssignments(
  assignments?: AssessmentManagerAssignment[] | null,
  employeeIDs?: string[],
): AssessmentManagerAssignment[] {
  const allowedEmployeeIDs = employeeIDs ? new Set(normalizeIDArray(employeeIDs)) : null
  const byUserID = new Map<string, AssessmentManagerAssignment>()
  ;(assignments || []).forEach(assignment => {
    const userID = String(assignment.user_id || '').trim()
    const managerUserID = String(assignment.assessment_manager_user_id || '').trim()
    if (!userID || !managerUserID) return
    if (allowedEmployeeIDs && !allowedEmployeeIDs.has(userID)) return
    byUserID.set(userID, {
      ...assignment,
      user_id: userID,
      employee_id: String(assignment.employee_id || '').trim() || undefined,
      assessment_manager_user_id: managerUserID,
      assessment_manager_employee_id: String(assignment.assessment_manager_employee_id || '').trim() || undefined,
      assessment_manager_name: String(assignment.assessment_manager_name || '').trim(),
      assessment_manager_source: assignment.assessment_manager_source || 'IMPORT',
      manager_override_reason: String(assignment.manager_override_reason || '').trim() || undefined,
    })
  })
  return Array.from(byUserID.values())
}

type AssessmentManagerCandidate = {
  user_id?: string
  name?: string
  employee_no?: string
  department_name?: string
  candidate_source_label?: string
  candidate_source: string
}

const MANAGER_SOURCE_LABELS: Record<string, string> = {
  DIRECT_MANAGER: '直属主管',
  DEPARTMENT_HEAD: '部门负责人',
  CENTER_HEAD: '中心负责人',
  MANUAL: '手动指定',
  IMPORT: '导入指定',
  EMPTY: '暂未配置',
  SYSTEM: '系统兼容',
}

function getAssessmentCandidateOption(candidate: AssessmentManagerCandidate) {
  const value = String(candidate.user_id || '').trim()
  if (!value) return null
  const name = String(candidate.name || value).trim()
  const employeeNo = String(candidate.employee_no || '').trim()
  const departmentName = String(candidate.department_name || '').trim()
  const sourceLabel = candidate.candidate_source_label || MANAGER_SOURCE_LABELS[candidate.candidate_source] || '候选'
  return {
    value,
    searchText: [name, value, employeeNo, departmentName].filter(Boolean).join(' '),
    sourceLabel,
  }
}

function getAssessmentUserOption(user: any) {
  if (String(user?.status || 'active').trim() !== 'active') return null
  const value = String(user?.user_id || user?.employee_id || user?.id || '').trim()
  if (!value) return null
  const name = String(user?.name || user?.user_name || user?.employee_name || value).trim()
  const employeeNo = String(user?.employee_id || user?.employee_no || '').trim()
  const departmentName = String(user?.department_name || '').trim()
  return {
    value,
    searchText: [name, value, employeeNo, departmentName, user?.mobile].filter(Boolean).join(' '),
  }
}

function normalizedIdentityValues(values: unknown[]) {
  return values.map(value => String(value ?? '').trim()).filter(Boolean)
}

function identitiesIntersect(leftValues: unknown[], rightValues: unknown[]) {
  const right = new Set(normalizedIdentityValues(rightValues))
  return normalizedIdentityValues(leftValues).some(value => right.has(value))
}

function participantIdentityValues(participant?: any) {
  if (!participant) return []
  return normalizedIdentityValues([
    participant.employee_id,
    participant.user_id,
    participant.employee_no,
  ])
}

function candidateIdentityValues(candidate: any) {
  return normalizedIdentityValues([
    candidate.user_id,
    candidate.employee_no,
  ])
}

function userIdentityValues(user: any) {
  return normalizedIdentityValues([
    user?.user_id,
    user?.employee_id,
    user?.employee_no,
    user?.id,
  ])
}

function candidateMatchesAnyParticipant(candidate: any, participants: any[]) {
  return participants.some(participant => identitiesIntersect(candidateIdentityValues(candidate), participantIdentityValues(participant)))
}

function userMatchesAnyParticipant(user: any, participants: any[]) {
  return participants.some(participant => identitiesIntersect(userIdentityValues(user), participantIdentityValues(participant)))
}

function getManagerEvaluationBlockedReason(record: any) {
  if (!String(record.manager_id || '').trim()) return '请先配置考核上级'
  if (record.manager_config_status === 'PENDING') return '请先配置考核上级'
  if (record.manager_config_status === 'INVALID') return '考核上级不可用，请先调整'
  return ''
}

function formatRangeStart(range?: [any, any]) {
  return range?.[0]?.format?.('YYYY-MM-DD') || ''
}

function formatRangeEnd(range?: [any, any]) {
  return range?.[1]?.format?.('YYYY-MM-DD') || ''
}

const ACTIVITY_STATUS_FILTER_GROUPS: Record<string, string[]> = {
  __in_progress__: ['target_setting', 'self_evaluation', 'manager_evaluation', 'employee_confirmation', 'manager_confirmation', 'hr_confirmation'],
  __confirmed__: ['locked', 'result_confirmed'],
}

function resolveActivityStatusFilter(statusFilter?: string) {
  if (!statusFilter) return undefined
  return ACTIVITY_STATUS_FILTER_GROUPS[statusFilter] || [statusFilter]
}

const ACTIVITY_FLOW = [
  { status: 'target_setting', label: '目标设定' },
  { status: 'self_evaluation', label: '自评' },
  { status: 'manager_evaluation', label: '评分' },
  { status: 'employee_confirmation', label: '员工确认' },
  { status: 'manager_confirmation', label: '主管确认' },
  { status: 'hr_confirmation', label: 'HR确认' },
  { status: 'archived', label: '归档' },
]

function getActivityStepIndex(status?: string) {
  if (status === 'locked') return ACTIVITY_FLOW.length - 1
  if (status === 'draft') return 0
  const index = ACTIVITY_FLOW.findIndex(item => item.status === status)
  return index >= 0 ? index : 0
}

const STATUS_MAP: Record<string, { label: string; color: string }> = {
  draft: { label: '草稿', color: 'default' },
  target_setting: { label: '目标设定', color: 'cyan' },
  self_evaluation: { label: '自评中', color: 'processing' },
  manager_evaluation: { label: '主管评分', color: 'warning' },
  employee_confirmation: { label: '员工确认', color: 'blue' },
  manager_confirmation: { label: '主管确认', color: 'orange' },
  hr_confirmation: { label: 'HR确认', color: 'purple' },
  locked: { label: '已锁定', color: 'error' },
  result_confirmed: { label: '已确认', color: 'success' },
  archived: { label: '已归档', color: 'default' },
}

function getStatusMeta(status?: string) {
  return STATUS_MAP[status || ''] || { label: status || '-', color: 'default' }
}

const PARTICIPANT_STATUS_MAP: Record<string, { label: string; color: string }> = {
  pending: { label: '待目标', color: 'default' },
  target_pending_approval: { label: '目标待审', color: 'cyan' },
  target_rejected: { label: '目标驳回', color: 'red' },
  target_set: { label: '目标已定', color: 'cyan' },
  self_submitted: { label: '已自评', color: 'processing' },
  manager_submitted: { label: '已评分', color: 'warning' },
  employee_confirmed: { label: '已员工确认', color: 'blue' },
  manager_confirmed: { label: '已主管确认', color: 'orange' },
  hr_confirmed: { label: '已HR确认', color: 'purple' },
  locked: { label: '已冻结', color: 'orange' },
  result_confirmed: { label: '已确认', color: 'success' },
  inactive: { label: '已离职', color: 'error' },
  removed_from_scope: { label: '已移除', color: 'error' },
}

function getParticipantStatusMeta(status?: string) {
  return PARTICIPANT_STATUS_MAP[status || ''] || { label: status || '-', color: 'default' }
}

function formatDateRange(start?: string, end?: string) {
  if (!start && !end) return '-'
  return `${start || '-'} ~ ${end || '-'}`
}

const ADJUSTABLE_MANAGER_SOURCES = ['DIRECT_MANAGER', 'DEPARTMENT_HEAD', 'CENTER_HEAD', 'MANUAL']

function getAdjustableManagerSource(source?: string): string {
  return source && ADJUSTABLE_MANAGER_SOURCES.includes(source) ? source : 'MANUAL'
}

// ==================== normalizeIDArray 测试 ====================

describe('normalizeIDArray', () => {
  it('should return empty array for undefined', () => {
    expect(normalizeIDArray(undefined)).toEqual([])
  })

  it('should return empty array for empty string', () => {
    expect(normalizeIDArray('')).toEqual([])
  })

  it('should split comma-separated string', () => {
    expect(normalizeIDArray('123,456,789')).toEqual(['123', '456', '789'])
  })

  it('should trim whitespace', () => {
    expect(normalizeIDArray(' 123 , 456 ')).toEqual(['123', '456'])
  })

  it('should filter empty values after split', () => {
    expect(normalizeIDArray('123,,456,')).toEqual(['123', '456'])
  })

  it('should return array as-is if already array', () => {
    expect(normalizeIDArray(['123', '456'])).toEqual(['123', '456'])
  })

  it('should filter empty strings from array', () => {
    expect(normalizeIDArray(['123', '', '456'])).toEqual(['123', '456'])
  })

  it('should handle single value string', () => {
    expect(normalizeIDArray('123')).toEqual(['123'])
  })
})

// ==================== getListFromResponse 测试 ====================

describe('getListFromResponse', () => {
  it('should return data array directly', () => {
    const res = { data: [1, 2, 3] }
    expect(getListFromResponse(res, ['items'])).toEqual([1, 2, 3])
  })

  it('should return nested items', () => {
    const res = { data: { items: [1, 2] } }
    expect(getListFromResponse(res, ['items'])).toEqual([1, 2])
  })

  it('should return nested departments', () => {
    const res = { data: { departments: [1, 2] } }
    expect(getListFromResponse(res, ['items', 'departments'])).toEqual([1, 2])
  })

  it('should return empty array if no match', () => {
    const res = { data: { foo: [1, 2] } }
    expect(getListFromResponse(res, ['items', 'departments'])).toEqual([])
  })

  it('should handle null response', () => {
    expect(getListFromResponse(null, ['items'])).toEqual([])
  })

  it('should handle direct array response', () => {
    const res = [1, 2, 3]
    expect(getListFromResponse(res, ['items'])).toEqual([1, 2, 3])
  })
})

// ==================== getDepartmentOption 测试 ====================

describe('getDepartmentOption', () => {
  it('should format department with id', () => {
    const dept = { department_id: '001', name: '技术部' }
    expect(getDepartmentOption(dept)).toEqual({ value: '001', label: '技术部（001）' })
  })

  it('should fallback to id field', () => {
    const dept = { id: '002', name: '产品部' }
    expect(getDepartmentOption(dept)).toEqual({ value: '002', label: '产品部（002）' })
  })

  it('should use department_name if name missing', () => {
    const dept = { department_id: '003', department_name: '设计部' }
    expect(getDepartmentOption(dept)).toEqual({ value: '003', label: '设计部（003）' })
  })

  it('should return null for empty value', () => {
    const dept = { department_id: '', name: 'test' }
    expect(getDepartmentOption(dept)).toBeNull()
  })

  it('should handle missing name fields', () => {
    const dept = { department_id: '004' }
    expect(getDepartmentOption(dept)).toEqual({ value: '004', label: '004（004）' })
  })
})

// ==================== getUserOption 测试 ====================

describe('getUserOption', () => {
  it('should format user with user_id', () => {
    const user = { user_id: 'U001', name: '张三' }
    expect(getUserOption(user)).toEqual({ value: 'U001', label: '张三（U001）' })
  })

  it('should include department name', () => {
    const user = { user_id: 'U002', name: '李四', department_name: '技术部' }
    expect(getUserOption(user)).toEqual({ value: 'U002', label: '李四（U002） - 技术部' })
  })

  it('should fallback to employee_id', () => {
    const user = { employee_id: 'E001', name: '王五' }
    expect(getUserOption(user)).toEqual({ value: 'E001', label: '王五（E001）' })
  })

  it('should return null for empty value', () => {
    const user = { user_id: '', name: 'test' }
    expect(getUserOption(user)).toBeNull()
  })

  it('should handle missing name fields', () => {
    const user = { user_id: 'U003' }
    expect(getUserOption(user)).toEqual({ value: 'U003', label: 'U003（U003）' })
  })
})

// ==================== getImportedUserOption 测试 ====================

describe('getImportedUserOption', () => {
  it('should format imported user', () => {
    const user = { user_id: 'U001', employee_id: 'E001', name: '张三' }
    const result = getImportedUserOption(user)
    expect(result).not.toBeNull()
    expect(result?.value).toBe('U001')
    expect(result?.label).toContain('张三')
    expect(result?.label).toContain('E001')
  })

  it('should handle user without employee_id', () => {
    const user = { user_id: 'U002', name: '李四' }
    const result = getImportedUserOption(user)
    expect(result).not.toBeNull()
    expect(result?.label).toContain('李四（U002）')
  })

  it('should return null for empty value', () => {
    const user = { user_id: '', name: 'test' }
    expect(getImportedUserOption(user)).toBeNull()
  })

  it('should handle null user', () => {
    expect(getImportedUserOption(null)).toBeNull()
  })

  it('should trim whitespace from value', () => {
    const user = { user_id: '  U003  ', name: '王五' }
    const result = getImportedUserOption(user)
    expect(result?.value).toBe('U003')
  })
})

// ==================== mergeSelectOptions 测试 ====================

describe('mergeSelectOptions', () => {
  it('should merge two option arrays', () => {
    const base = [{ value: 1, label: 'A' }]
    const extra = [{ value: 2, label: 'B' }]
    const result = mergeSelectOptions(base, extra)
    expect(result).toHaveLength(2)
  })

  it('should deduplicate by value', () => {
    const base = [{ value: 1, label: 'A' }]
    const extra = [{ value: 1, label: 'A2' }]
    const result = mergeSelectOptions(base, extra)
    expect(result).toHaveLength(1)
    expect(result[0].label).toBe('A')
  })

  it('should skip options with empty value', () => {
    const base = [{ value: 1, label: 'A' }]
    const extra = [{ value: '', label: 'B' }]
    const result = mergeSelectOptions(base, extra)
    expect(result).toHaveLength(1)
  })

  it('should handle empty base array', () => {
    const extra = [{ value: 1, label: 'A' }]
    const result = mergeSelectOptions([], extra)
    expect(result).toHaveLength(1)
  })

  it('should handle empty extra array', () => {
    const base = [{ value: 1, label: 'A' }]
    const result = mergeSelectOptions(base, [])
    expect(result).toHaveLength(1)
  })
})

// ==================== getImportedUserOptions 测试 ====================

describe('getImportedUserOptions', () => {
  it('should return options from result employees', () => {
    const result = {
      employees: [
        { user_id: 'U001', name: '张三' },
        { user_id: 'U002', name: '李四' },
      ],
    }
    const options = getImportedUserOptions(result, [])
    expect(options).toHaveLength(2)
  })

  it('should fallback to employeeIDs', () => {
    const options = getImportedUserOptions(null, ['U001', 'U002'])
    expect(options).toHaveLength(2)
  })

  it('should merge result and fallback', () => {
    const result = {
      employees: [{ user_id: 'U001', name: '张三' }],
    }
    const options = getImportedUserOptions(result, ['U001', 'U003'])
    expect(options).toHaveLength(2)
  })

  it('should deduplicate', () => {
    const result = {
      employees: [{ user_id: 'U001', name: '张三' }],
    }
    const options = getImportedUserOptions(result, ['U001'])
    expect(options).toHaveLength(1)
  })

  it('should handle empty inputs', () => {
    const options = getImportedUserOptions(null, [])
    expect(options).toHaveLength(0)
  })
})

// ==================== normalizeImportedManagerAssignments 测试 ====================

describe('normalizeImportedManagerAssignments', () => {
  it('should normalize valid assignments', () => {
    const assignments = [
      {
        user_id: 'U001',
        assessment_manager_user_id: 'M001',
        assessment_manager_name: '王主管',
      },
    ]
    const result = normalizeImportedManagerAssignments(assignments)
    expect(result).toHaveLength(1)
    expect(result[0].user_id).toBe('U001')
    expect(result[0].assessment_manager_user_id).toBe('M001')
    expect(result[0].assessment_manager_name).toBe('王主管')
    expect(result[0].assessment_manager_source).toBe('IMPORT')
  })

  it('should filter out missing manager', () => {
    const assignments = [
      { user_id: 'U001', assessment_manager_user_id: '' },
    ]
    const result = normalizeImportedManagerAssignments(assignments)
    expect(result).toHaveLength(0)
  })

  it('should filter out missing user_id', () => {
    const assignments = [
      { user_id: '', assessment_manager_user_id: 'M001' },
    ]
    const result = normalizeImportedManagerAssignments(assignments)
    expect(result).toHaveLength(0)
  })

  it('should filter by allowed employee IDs', () => {
    const assignments = [
      { user_id: 'U001', assessment_manager_user_id: 'M001' },
      { user_id: 'U002', assessment_manager_user_id: 'M002' },
    ]
    const result = normalizeImportedManagerAssignments(assignments, ['U001'])
    expect(result).toHaveLength(1)
    expect(result[0].user_id).toBe('U001')
  })

  it('should deduplicate by user_id', () => {
    const assignments = [
      { user_id: 'U001', assessment_manager_user_id: 'M001' },
      { user_id: 'U001', assessment_manager_user_id: 'M002' },
    ]
    const result = normalizeImportedManagerAssignments(assignments)
    expect(result).toHaveLength(1)
  })

  it('should handle null assignments', () => {
    const result = normalizeImportedManagerAssignments(null)
    expect(result).toHaveLength(0)
  })

  it('should trim whitespace from IDs', () => {
    const assignments = [
      { user_id: '  U001  ', assessment_manager_user_id: '  M001  ' },
    ]
    const result = normalizeImportedManagerAssignments(assignments)
    expect(result[0].user_id).toBe('U001')
    expect(result[0].assessment_manager_user_id).toBe('M001')
  })
})

// ==================== getAssessmentCandidateOption 测试 ====================

describe('getAssessmentCandidateOption', () => {
  it('should return candidate option', () => {
    const candidate = {
      user_id: 'U001',
      name: '张三',
      employee_no: 'E001',
      department_name: '技术部',
      candidate_source: 'DIRECT_MANAGER',
    }
    const result = getAssessmentCandidateOption(candidate)
    expect(result).not.toBeNull()
    expect(result?.value).toBe('U001')
    expect(result?.searchText).toContain('张三')
    expect(result?.searchText).toContain('E001')
    expect(result?.searchText).toContain('技术部')
    expect(result?.sourceLabel).toBe('直属主管')
  })

  it('should return null for empty user_id', () => {
    const candidate = {
      user_id: '',
      name: 'test',
      candidate_source: 'MANUAL',
    }
    expect(getAssessmentCandidateOption(candidate)).toBeNull()
  })

  it('should use custom source label', () => {
    const candidate = {
      user_id: 'U001',
      name: 'test',
      candidate_source: 'CUSTOM',
      candidate_source_label: '自定义',
    }
    const result = getAssessmentCandidateOption(candidate)
    expect(result?.sourceLabel).toBe('自定义')
  })

  it('should fallback to candidate label', () => {
    const candidate = {
      user_id: 'U001',
      name: 'test',
      candidate_source: 'UNKNOWN',
    }
    const result = getAssessmentCandidateOption(candidate)
    expect(result?.sourceLabel).toBe('候选')
  })
})

// ==================== getAssessmentUserOption 测试 ====================

describe('getAssessmentUserOption', () => {
  it('should return user option for active user', () => {
    const user = {
      user_id: 'U001',
      name: '张三',
      employee_id: 'E001',
      department_name: '技术部',
      status: 'active',
    }
    const result = getAssessmentUserOption(user)
    expect(result).not.toBeNull()
    expect(result?.value).toBe('U001')
    expect(result?.searchText).toContain('张三')
  })

  it('should return null for inactive user', () => {
    const user = { user_id: 'U001', name: 'test', status: 'inactive' }
    expect(getAssessmentUserOption(user)).toBeNull()
  })

  it('should return null for empty user_id', () => {
    const user = { user_id: '', name: 'test', status: 'active' }
    expect(getAssessmentUserOption(user)).toBeNull()
  })

  it('should handle missing status as active', () => {
    const user = { user_id: 'U001', name: 'test' }
    const result = getAssessmentUserOption(user)
    expect(result).not.toBeNull()
  })
})

// ==================== assessment manager identity helpers 测试 ====================

describe('assessment manager identity helpers', () => {
  it('should match candidate user_id against participant employee_id', () => {
    const participant = { employee_id: 'U001', employee_name: '张三' }
    const candidate = { user_id: 'U001', name: '张三', candidate_source: 'MANUAL' }

    expect(candidateMatchesAnyParticipant(candidate, [participant])).toBe(true)
  })

  it('should match candidate employee number against participant employee_no', () => {
    const participant = { employee_id: 'U001', employee_no: 'E001', employee_name: '张三' }
    const candidate = { user_id: 'OTHER', employee_no: 'E001', name: '张三', candidate_source: 'MANUAL' }

    expect(candidateMatchesAnyParticipant(candidate, [participant])).toBe(true)
  })

  it('should match local user options against selected participants', () => {
    const participant = { employee_id: 'U001', employee_name: '张三' }
    const user = { user_id: 'U001', employee_id: 'E001', name: '张三', status: 'active' }

    expect(userMatchesAnyParticipant(user, [participant])).toBe(true)
  })

  it('should block manager evaluation when assessment manager is not configured', () => {
    expect(getManagerEvaluationBlockedReason({ manager_id: '', manager_config_status: 'PENDING' })).toBe('请先配置考核上级')
    expect(getManagerEvaluationBlockedReason({ manager_id: 'M001', manager_config_status: 'INVALID' })).toBe('考核上级不可用，请先调整')
    expect(getManagerEvaluationBlockedReason({ manager_id: 'M001', manager_config_status: 'CONFIGURED' })).toBe('')
  })
})

// ==================== formatRangeStart/End 测试 ====================

describe('formatRangeStart', () => {
  it('should format start date', () => {
    const range = [{ format: () => '2024-01-01' }, { format: () => '2024-01-31' }]
    expect(formatRangeStart(range as any)).toBe('2024-01-01')
  })

  it('should return empty for undefined', () => {
    expect(formatRangeStart(undefined)).toBe('')
  })

  it('should return empty for empty array', () => {
    expect(formatRangeStart([] as any)).toBe('')
  })
})

describe('formatRangeEnd', () => {
  it('should format end date', () => {
    const range = [{ format: () => '2024-01-01' }, { format: () => '2024-01-31' }]
    expect(formatRangeEnd(range as any)).toBe('2024-01-31')
  })

  it('should return empty for undefined', () => {
    expect(formatRangeEnd(undefined)).toBe('')
  })
})

// ==================== resolveActivityStatusFilter 测试 ====================

describe('resolveActivityStatusFilter', () => {
  it('should return undefined for no filter', () => {
    expect(resolveActivityStatusFilter(undefined)).toBeUndefined()
  })

  it('should resolve in-progress status', () => {
    const result = resolveActivityStatusFilter('__in_progress__')
    expect(result).toContain('target_setting')
    expect(result).toContain('self_evaluation')
    expect(result).toContain('manager_evaluation')
    expect(result).toContain('employee_confirmation')
    expect(result).toContain('manager_confirmation')
    expect(result).toContain('hr_confirmation')
  })

  it('should resolve confirmed status', () => {
    const result = resolveActivityStatusFilter('__confirmed__')
    expect(result).toContain('locked')
    expect(result).toContain('result_confirmed')
  })

  it('should return single status for specific status', () => {
    const result = resolveActivityStatusFilter('draft')
    expect(result).toEqual(['draft'])
  })

  it('should return single status for unknown status', () => {
    const result = resolveActivityStatusFilter('unknown')
    expect(result).toEqual(['unknown'])
  })
})

// ==================== getActivityStepIndex 测试 ====================

describe('getActivityStepIndex', () => {
  it('should return 0 for draft', () => {
    expect(getActivityStepIndex('draft')).toBe(0)
  })

  it('should return last step for locked', () => {
    expect(getActivityStepIndex('locked')).toBe(ACTIVITY_FLOW.length - 1)
  })

  it('should return correct index for status', () => {
    expect(getActivityStepIndex('target_setting')).toBe(0)
    expect(getActivityStepIndex('self_evaluation')).toBe(1)
    expect(getActivityStepIndex('manager_evaluation')).toBe(2)
    expect(getActivityStepIndex('employee_confirmation')).toBe(3)
    expect(getActivityStepIndex('manager_confirmation')).toBe(4)
    expect(getActivityStepIndex('hr_confirmation')).toBe(5)
  })

  it('should return 0 for unknown status', () => {
    expect(getActivityStepIndex('unknown')).toBe(0)
  })

  it('should return 0 for undefined', () => {
    expect(getActivityStepIndex(undefined)).toBe(0)
  })
})

// ==================== getStatusMeta 测试 ====================

describe('getStatusMeta', () => {
  it('should return status meta for known status', () => {
    const meta = getStatusMeta('draft')
    expect(meta.label).toBe('草稿')
    expect(meta.color).toBe('default')
  })

  it('should return default for unknown status', () => {
    const meta = getStatusMeta('unknown')
    expect(meta.label).toBe('unknown')
    expect(meta.color).toBe('default')
  })

  it('should return default for undefined', () => {
    const meta = getStatusMeta(undefined)
    expect(meta.label).toBe('-')
    expect(meta.color).toBe('default')
  })

  it('should handle all defined statuses', () => {
    Object.keys(STATUS_MAP).forEach(status => {
      const meta = getStatusMeta(status)
      expect(meta.label).toBe(STATUS_MAP[status].label)
      expect(meta.color).toBe(STATUS_MAP[status].color)
    })
  })
})

// ==================== getParticipantStatusMeta 测试 ====================

describe('getParticipantStatusMeta', () => {
  it('should return participant status meta', () => {
    const meta = getParticipantStatusMeta('pending')
    expect(meta.label).toBe('待目标')
    expect(meta.color).toBe('default')
  })

  it('should return default for unknown status', () => {
    const meta = getParticipantStatusMeta('unknown')
    expect(meta.label).toBe('unknown')
    expect(meta.color).toBe('default')
  })

  it('should return default for undefined', () => {
    const meta = getParticipantStatusMeta(undefined)
    expect(meta.label).toBe('-')
    expect(meta.color).toBe('default')
  })

  it('should handle all defined statuses', () => {
    Object.keys(PARTICIPANT_STATUS_MAP).forEach(status => {
      const meta = getParticipantStatusMeta(status)
      expect(meta.label).toBe(PARTICIPANT_STATUS_MAP[status].label)
      expect(meta.color).toBe(PARTICIPANT_STATUS_MAP[status].color)
    })
  })
})

// ==================== formatDateRange 测试 ====================

describe('formatDateRange', () => {
  it('should format both dates', () => {
    expect(formatDateRange('2024-01-01', '2024-01-31')).toBe('2024-01-01 ~ 2024-01-31')
  })

  it('should handle missing start', () => {
    expect(formatDateRange(undefined, '2024-01-31')).toBe('- ~ 2024-01-31')
  })

  it('should handle missing end', () => {
    expect(formatDateRange('2024-01-01', undefined)).toBe('2024-01-01 ~ -')
  })

  it('should return dash for both missing', () => {
    expect(formatDateRange(undefined, undefined)).toBe('-')
  })
})

// ==================== getAdjustableManagerSource 测试 ====================

describe('getAdjustableManagerSource', () => {
  it('should return source if adjustable', () => {
    expect(getAdjustableManagerSource('DIRECT_MANAGER')).toBe('DIRECT_MANAGER')
    expect(getAdjustableManagerSource('DEPARTMENT_HEAD')).toBe('DEPARTMENT_HEAD')
    expect(getAdjustableManagerSource('CENTER_HEAD')).toBe('CENTER_HEAD')
    expect(getAdjustableManagerSource('MANUAL')).toBe('MANUAL')
  })

  it('should fallback to MANUAL for non-adjustable', () => {
    expect(getAdjustableManagerSource('IMPORT')).toBe('MANUAL')
    expect(getAdjustableManagerSource('EMPTY')).toBe('MANUAL')
    expect(getAdjustableManagerSource('SYSTEM')).toBe('MANUAL')
  })

  it('should fallback to MANUAL for undefined', () => {
    expect(getAdjustableManagerSource(undefined)).toBe('MANUAL')
  })
})
