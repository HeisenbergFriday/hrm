import React from 'react'
import { Tag, TagProps } from 'antd'

interface StatusTagProps extends TagProps {
  children: React.ReactNode
}

const StatusTag: React.FC<StatusTagProps> = ({ children, style, ...props }) => {
  return (
    <Tag
      {...props}
      style={{ borderRadius: 'var(--radius-sm)', fontWeight: 'var(--font-weight-semibold)', margin: 0, ...style }}
    >
      {children}
    </Tag>
  )
}

export default StatusTag
