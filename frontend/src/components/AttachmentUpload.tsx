import React from 'react'
import { Upload, Button, message, Space, Typography } from 'antd'
import { UploadOutlined, PaperClipOutlined, DeleteOutlined } from '@ant-design/icons'
import type { UploadFile, UploadProps } from 'antd'
import { useAuthStore } from '../store/authStore'

const { Text } = Typography

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
  const fileList: UploadFile[] = value.map((url, index) => ({
    uid: `-${index}`,
    name: url.split('/').pop() || `附件${index + 1}`,
    status: 'done',
    url,
  }))

  const handleUpload: UploadProps['customRequest'] = async (options) => {
    const { file, onSuccess, onError } = options

    try {
      const formData = new FormData()
      formData.append('file', file as File)

      const token = useAuthStore.getState().token
      const response = await fetch('/api/v1/upload', {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${token}`,
        },
        body: formData,
      })

      if (!response.ok) {
        throw new Error('上传失败')
      }

      const result = await response.json()
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
      message.error('上传失败，请重试')
    }
  }

  const handleRemove = (file: UploadFile) => {
    const url = file.url || file.response?.data?.url
    if (url) {
      const newUrls = value.filter((u) => u !== url)
      onChange?.(newUrls)
    }
    return true
  }

  if (disabled && value.length === 0) {
    return null
  }

  return (
    <div>
      <Upload
        fileList={fileList}
        customRequest={handleUpload}
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
