import React from 'react'
import { Typography } from 'antd'

const { Text } = Typography

interface PageContainerProps {
  title?: string
  subtitle?: React.ReactNode
  icon?: React.ReactNode
  extra?: React.ReactNode
  children: React.ReactNode
  className?: string
  style?: React.CSSProperties
  noPadding?: boolean
}

const PageContainer: React.FC<PageContainerProps> = ({
  title,
  subtitle,
  icon,
  extra,
  children,
  className,
  style,
  noPadding = false,
}) => {
  const containerClassName = ['page-container', noPadding ? 'page-container-no-padding' : '', className]
    .filter(Boolean)
    .join(' ')

  return (
    <div
      className={containerClassName}
      style={{
        padding: noPadding ? 0 : 'var(--page-padding)',
        background: 'var(--color-bg-page)',
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
                  fontSize: 'var(--font-size-xl)',
                  fontWeight: 'var(--font-weight-bold)',
                  color: 'var(--color-text-title)',
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
              <Text style={{ color: 'var(--color-text-secondary)', fontSize: 'var(--font-size-sm)' }}>
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
