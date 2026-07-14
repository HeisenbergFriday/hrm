import React from 'react'
import { Grid, Typography } from 'antd'
import { resolveMobileLayout, useMobileRuntime } from '../utils/responsive'

const { Text } = Typography

interface PageContainerProps extends React.HTMLAttributes<HTMLDivElement> {
  title?: string
  subtitle?: React.ReactNode
  icon?: React.ReactNode
  extra?: React.ReactNode
  children: React.ReactNode
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
  ...props
}) => {
  const screens = Grid.useBreakpoint()
  const mobileRuntime = useMobileRuntime()
  const isMobile = resolveMobileLayout(screens.md, mobileRuntime)
  const containerClassName = ['page-container', noPadding ? 'page-container-no-padding' : '', className]
    .filter(Boolean)
    .join(' ')
  const containerStyle: React.CSSProperties = {
    padding: noPadding ? 0 : 'var(--page-padding)',
    background: 'var(--color-bg-page)',
    ...style,
  }
  const showHeader = isMobile ? Boolean(extra) : Boolean(title || extra)

  if (isMobile && !noPadding) {
    containerStyle.padding = 'var(--space-3)'
  }

  return (
    <div
      {...props}
      className={containerClassName}
      style={containerStyle}
    >
      {showHeader && (
        <div
          style={{
            display: 'flex',
            flexDirection: isMobile ? 'column' : 'row',
            alignItems: isMobile ? 'stretch' : 'flex-start',
            justifyContent: 'space-between',
            gap: isMobile ? 12 : 16,
            marginBottom: isMobile ? 12 : 16,
          }}
        >
          <div style={{ minWidth: 0 }}>
            {!isMobile && title && (
              <h2
                style={{
                  margin: '0 0 4px',
                  fontSize: isMobile ? 'var(--font-size-lg)' : 'var(--font-size-xl)',
                  fontWeight: 'var(--font-weight-bold)',
                  color: 'var(--color-text-title)',
                  lineHeight: isMobile ? '26px' : '32px',
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
          {extra && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
              {extra}
            </div>
          )}
        </div>
      )}
      {children}
    </div>
  )
}

export default PageContainer
