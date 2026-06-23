import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AttachmentUpload from './AttachmentUpload'

vi.mock('../store/authStore', () => ({
  useAuthStore: {
    getState: () => ({ token: 'test-token' }),
  },
}))

vi.mock('../utils/authFileUrl', () => ({
  withFileAccessToken: (url: string) => url,
}))

function getFileInput(container: HTMLElement) {
  const input = container.querySelector('input[type="file"]')
  if (!input) throw new Error('file input not found')
  return input as HTMLInputElement
}

describe('AttachmentUpload', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('上传失败时应显示后端返回的具体原因', async () => {
    const user = userEvent.setup()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      json: async () => ({ message: '不支持的文件类型，允许: pdf/docx' }),
    }))

    const { container } = render(<AttachmentUpload />)
    await user.upload(getFileInput(container), new File(['proof'], 'proof.pdf', { type: 'application/pdf' }))

    await waitFor(() => {
      expect(screen.getByText('不支持的文件类型，允许: pdf/docx')).toBeInTheDocument()
    })
  })

  it('本地拦截不支持的附件类型', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    const { container } = render(<AttachmentUpload />)
    await user.upload(getFileInput(container), new File(['proof'], 'proof.exe', { type: 'application/octet-stream' }))

    await waitFor(() => {
      expect(screen.getByText(/不支持的文件类型/)).toBeInTheDocument()
    })
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
