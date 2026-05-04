import React, { useEffect, useState } from 'react'
import { Result, Button, message } from 'antd'
import { CloseCircleOutlined } from '@ant-design/icons'
import { useNavigate, useSearchParams } from 'react-router-dom'
import axios from 'axios'
import { useAuthStore } from '../store/authStore'

function isDingTalkEnv(): boolean {
  return /DingTalk/i.test(navigator.userAgent)
}

function getAxiosErrorMessage(error: unknown, fallback: string): string {
  if (axios.isAxiosError(error)) {
    const serverMessage = error.response?.data?.message
    if (typeof serverMessage === 'string' && serverMessage.trim() !== '') {
      return serverMessage
    }
  }

  return fallback
}

const Callback: React.FC = () => {
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { login } = useAuthStore()

  useEffect(() => {
    const handleCallback = async () => {
      const code = searchParams.get('code')
      const state = searchParams.get('state')

      if (!code) {
        if (isDingTalkEnv()) {
          navigate('/', { replace: true })
          return
        }
        setError('缺少 code 参数')
        setLoading(false)
        return
      }

      try {
        const response = await axios.get('/api/v1/auth/dingtalk/callback', {
          params: { code, state },
        })

        if (response.data.code === 200) {
          const { token, user } = response.data.data
          login(user, token)
          message.success('登录成功', 0.6)
          navigate('/', { replace: true })
          return
        }

        setError(response.data.message || '登录失败')
      } catch (err) {
        setError(getAxiosErrorMessage(err, '登录失败，请重试'))
      } finally {
        setLoading(false)
      }
    }

    void handleCallback()
  }, [searchParams, navigate, login])

  if (loading) {
    return (
      <div style={{ minHeight: '100vh', background: '#f0f2f5' }} />
    )
  }

  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', background: '#f0f2f5' }}>
      <Result
        status="error"
        icon={<CloseCircleOutlined />}
        title="登录失败"
        subTitle={error}
        extra={[
          <Button type="primary" key="login" onClick={() => navigate('/login?mode=scan', { replace: true })}>
            返回扫码页
          </Button>,
        ]}
      />
    </div>
  )
}

export default Callback
