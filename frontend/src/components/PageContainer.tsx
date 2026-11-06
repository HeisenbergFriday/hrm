import React, { useId } from 'react'
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
  const titleId = useId()
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
  // 页面标题在移动端同样保留，避免用户进入深层页面后失去上下文。
  const showHeader = Boolean(title || subtitle || extra)

  if (isMobile && !noPadding) {
    containerStyle.padding = 'var(--space-3)'
  }

  return (
    <main
      {...props}
      className={containerClassName}
      style={containerStyle}
      aria-labelledby={title ? titleId : undefined}
    >
      {showHeader && (
        <div
          className="page-container-header"
          style={{
            flexDirection: isMobile ? 'column' : 'row',
            alignItems: isMobile ? 'stretch' : 'flex-start',
          }}
        >
          <div className="page-container-heading">
            {title && (
              <h1 id={titleId} className="page-container-title">
                {icon && (
                  <span className="page-container-title-icon" aria-hidden="true">
                    {icon}
                  </span>
                )}
                {title}
              </h1>
            )}
            {subtitle && (
              <Text className="page-container-subtitle">
                {subtitle}
              </Text>
            )}
          </div>
          {extra && (
            <div className="page-container-actions">
              {extra}
            </div>
          )}
        </div>
      )}
      {children}
    </main>
  )
}

export default PageContainer
