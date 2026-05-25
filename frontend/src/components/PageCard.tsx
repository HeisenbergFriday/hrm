import React from 'react'
import { Card, CardProps } from 'antd'

interface PageCardProps extends CardProps {
  children: React.ReactNode
}

const PageCard: React.FC<PageCardProps> = ({ children, style, styles, ...props }) => {
  return (
    <Card
      {...props}
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
        },
        ...styles,
      }}
    >
      {children}
    </Card>
  )
}

export default PageCard
