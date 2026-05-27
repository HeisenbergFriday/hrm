import { Component, type ReactNode } from 'react'
import { Result, Button, Typography } from 'antd'

const { Paragraph } = Typography

interface Props {
  children: ReactNode
  resetKey?: string
}

interface State {
  hasError: boolean
  error: Error | null
}

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidUpdate(prevProps: Props) {
    if (this.state.hasError && prevProps.resetKey !== this.props.resetKey) {
      this.setState({ hasError: false, error: null })
    }
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error('[ErrorBoundary]', error, errorInfo.componentStack)
  }

  render() {
    if (this.state.hasError) {
      return (
        <Result
          status="error"
          title="页面出错了"
          subTitle="渲染过程中发生异常，请尝试刷新页面。"
          extra={[
            <Button key="reload" type="primary" onClick={() => window.location.reload()}>
              刷新页面
            </Button>,
            <Button key="home" onClick={() => { window.location.href = '/' }}>
              返回首页
            </Button>,
          ]}
        >
          {import.meta.env.DEV && this.state.error && (
            <Paragraph>
              <pre style={{ textAlign: 'left', maxHeight: 200, overflow: 'auto', fontSize: 12, background: '#f5f5f5', padding: 12, borderRadius: 8 }}>
                {this.state.error.stack}
              </pre>
            </Paragraph>
          )}
        </Result>
      )
    }

    return this.props.children
  }
}
