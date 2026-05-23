import React from 'react'
import { Typography } from 'antd'

const { Text } = Typography

interface PageContainerProps {
  title?: string
  subtitle?: string
  icon?: React.ReactNode
  extra?: React.ReactNode
  children: React.ReactNode
  style?: React.CSSProperties
  noPadding?: boolean
}

const PageContainer: React.FC<PageContainerProps> = ({
  title,
  subtitle,
  icon,
  extra,
  children,
  style,
  noPadding = false,
}) => {
  return (
    <div
      style={{
        padding: noPadding ? 0 : 'var(--page-padding)',
        background: 'var(--color-bg-page)',
        minHeight: '100vh',
        ...style,
      }}
    >
      {(title || extra) && (
        <div
          style={{
            display: 'flex',
            alignItems: 'flex-start',
            justifyContent: 'space-between',
            marginBottom: 16,
          }}
        >
          <div>
            {title && (
              <h2
                style={{
                  margin: '0 0 4px',
                  fontSize: 'var(--font-size-title)',
                  fontWeight: 'var(--font-weight-title)',
                  color: 'var(--color-text-primary)',
                }}
              >
                {icon && (
                  <span style={{ color: 'var(--color-primary)', marginRight: 8 }}>
                    {icon}
                  </span>
                )}
                {title}
              </h2>
            )}
            {subtitle && (
              <Text style={{ color: 'var(--color-text-secondary)', fontSize: 13.5 }}>
                {subtitle}
              </Text>
            )}
          </div>
          {extra && <div>{extra}</div>}
        </div>
      )}
      {children}
    </div>
  )
}

export default PageContainer
