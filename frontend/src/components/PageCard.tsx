import React from 'react'
import { Card, CardProps } from 'antd'

interface PageCardProps extends CardProps {
  children: React.ReactNode
}

const PageCard: React.FC<PageCardProps> = ({ children, className, style, styles, ...props }) => {
  const cardClassName = ['page-card', props.hoverable ? 'page-card-interactive' : '', className]
    .filter(Boolean)
    .join(' ')
  return (
    <Card
      {...props}
      className={cardClassName}
      style={{
        borderRadius: 'var(--radius-xl)',
        border: '1px solid var(--color-border)',
        boxShadow: 'var(--shadow-card)',
        ...style,
      }}
      styles={{
        header: {
          background: 'var(--color-bg-card-header)',
          borderBottom: '1px solid var(--color-border-light)',
          minHeight: 52,
          paddingInline: 20,
        },
        body: {
          padding: 20,
        },
        ...styles,
      }}
    >
      {children}
    </Card>
  )
}

export default PageCard
