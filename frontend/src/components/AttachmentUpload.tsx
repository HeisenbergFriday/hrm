import React from 'react'
import { Upload, Button, message } from 'antd'
import { UploadOutlined } from '@ant-design/icons'
import type { UploadFile, UploadProps } from 'antd'
import { openAuthorizedFile } from '../utils/authFileUrl'
import { csrfHeadersForMethod } from '../utils/csrf'

const maxUploadSize = 10 * 1024 * 1024
const allowedAttachmentExtensions = [
  '.jpg', '.jpeg', '.png', '.gif', '.webp',
  '.pdf',
  '.docx', '.xlsx', '.pptx',
  '.wps', '.et', '.dps',
  '.txt', '.csv', '.md',
  '.zip', '.rar', '.7z',
]
const attachmentAccept = allowedAttachmentExtensions.join(',')
const allowedAttachmentText = allowedAttachmentExtensions.map(ext => ext.slice(1)).join('/')

interface AttachmentUploadProps {
  value?: string[]
  onChange?: (urls: string[]) => void
  maxCount?: number
  disabled?: boolean
}

const AttachmentUpload: React.FC<AttachmentUploadProps> = ({
  value = [],
  onChange,
  maxCount = 5,
  disabled = false,
}) => {
  const validateFile = (file: File) => {
    const ext = `.${file.name.split('.').pop() || ''}`.toLowerCase()
    if (file.size > maxUploadSize) {
      message.error('文件大小不能超过10MB')
      return false
    }
    if (!allowedAttachmentExtensions.includes(ext)) {
      message.error(`不支持的文件类型，允许: ${allowedAttachmentText}`)
      return false
    }
    return true
  }

  const fileList: UploadFile[] = value.map((url, index) => ({
    uid: `-${index}`,
    name: url.split('/').pop() || `附件${index + 1}`,
    status: 'done',
    url,
    response: { data: { url } },
  }))

  const handleUpload: UploadProps['customRequest'] = async (options) => {
    const { file, onSuccess, onError } = options

    try {
      const formData = new FormData()
      formData.append('file', file as File)

      const response = await fetch('/api/v1/upload', {
        method: 'POST',
        headers: csrfHeadersForMethod('POST'),
        credentials: 'include',
        body: formData,
      })
      const result = await response.json().catch(() => ({}))

      if (!response.ok) {
        throw new Error(result?.message || '上传失败')
      }

      const url = result.data?.url || result.url

      if (url) {
        const newUrls = [...value, url]
        onChange?.(newUrls)
        onSuccess?.(result)
        message.success('上传成功')
      } else {
        throw new Error('未获取到文件URL')
      }
    } catch (err) {
      onError?.(err as Error)
      message.error(err instanceof Error && err.message ? err.message : '上传失败，请重试')
    }
  }

  const handleRemove = (file: UploadFile) => {
    const url = file.response?.data?.url || file.url
    if (url) {
      const newUrls = value.filter((u) => u !== url)
      onChange?.(newUrls)
    }
    return true
  }

  const handlePreview: UploadProps['onPreview'] = async (file) => {
    const url = file.response?.data?.url || file.url
    if (!url) return
    try {
      await openAuthorizedFile(url)
    } catch {
      message.error('附件预览失败')
    }
  }

  if (disabled && value.length === 0) {
    return null
  }

  return (
    <div>
      <Upload
        fileList={fileList}
        accept={attachmentAccept}
        beforeUpload={(file) => validateFile(file) || Upload.LIST_IGNORE}
        customRequest={handleUpload}
        onPreview={handlePreview}
        onRemove={handleRemove}
        maxCount={maxCount}
        disabled={disabled}
      >
        {disabled ? null : (
          <Button
            type="link"
            size="small"
            icon={<UploadOutlined />}
            style={{ padding: 0 }}
          >
            上传附件
          </Button>
        )}
      </Upload>
    </div>
  )
}

export default AttachmentUpload
