import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import ApprovalStatusTag from './ApprovalStatusTag'

describe('ApprovalStatusTag', () => {
  it.each([
    ['completed', '已完成'],
    ['APPROVED', '已完成'],
    ['running', '审批中'],
    ['refuse', '已拒绝'],
    ['terminated', '已终止'],
    ['cancelled', '已取消'],
    ['pending', '待处理'],
  ])('maps %s to %s', (status, label) => {
    render(<ApprovalStatusTag status={status} />)
    expect(screen.getByText(label)).toBeInTheDocument()
  })

  it('keeps unknown and empty status fallbacks', () => {
    const { rerender } = render(<ApprovalStatusTag status="custom" />)
    expect(screen.getByText('custom')).toBeInTheDocument()

    rerender(<ApprovalStatusTag status="" />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})
