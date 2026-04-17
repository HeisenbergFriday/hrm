import React from 'react'
import { Card, Result, Button } from 'antd'
import { CloseCircleOutlined } from '@ant-design/icons'
import { useNavigate, useSearchParams } from 'react-router-dom'

const LoginError: React.FC = () => {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const error = searchParams.get('error') || '登录失败，请重试'

  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', background: '#f0f2f5' }}>
      <Card style={{ width: 400 }}>
        <Result
          status="error"
          icon={<CloseCircleOutlined />}
          title="登录失败"
          subTitle={decodeURIComponent(error)}
          extra={[
            <Button type="primary" key="login" onClick={() => navigate('/login')}>
              返回登录页
            </Button>
          ]}
        />
      </Card>
    </div>
  )
}

export default LoginError