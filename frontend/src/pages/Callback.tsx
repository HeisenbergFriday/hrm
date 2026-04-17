import React, { useEffect, useState } from 'react'
import { Card, Spin, Typography, Result, Button } from 'antd'
import { LoadingOutlined, CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { useNavigate, useSearchParams } from 'react-router-dom'
import axios from 'axios'
import { useAuthStore } from '../store/authStore'

const { Title } = Typography

const Callback: React.FC = () => {
  const [loading, setLoading] = useState(true)
  const [success, setSuccess] = useState(false)
  const [error, setError] = useState('')
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { login } = useAuthStore()

  useEffect(() => {
    const handleCallback = async () => {
      const code = searchParams.get('code')
      const state = searchParams.get('state')

      if (!code) {
        setError('缺少code参数')
        setLoading(false)
        return
      }

      try {
        const response = await axios.get('/api/v1/auth/dingtalk/callback', {
          params: { code, state }
        })
        
        if (response.data.code === 200) {
          setSuccess(true)
          // 使用auth store存储token
          const { token, user } = response.data.data
          login(user, token)
          // 延迟跳转到首页
          setTimeout(() => {
            navigate('/')
          }, 2000)
        } else {
          setError(response.data.message)
        }
      } catch (err) {
        setError('登录失败，请重试')
      } finally {
        setLoading(false)
      }
    }

    handleCallback()
  }, [searchParams, navigate, login])

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', background: '#f0f2f5' }}>
        <Card style={{ width: 400, textAlign: 'center' }}>
          <Spin indicator={<LoadingOutlined style={{ fontSize: 24 }} spin />} />
          <p style={{ marginTop: 16 }}>正在处理登录，请稍候...</p>
        </Card>
      </div>
    )
  }

  if (success) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', background: '#f0f2f5' }}>
        <Card style={{ width: 400 }}>
          <Result
            status="success"
            icon={<CheckCircleOutlined />}
            title="登录成功"
            subTitle="正在跳转到首页..."
          />
        </Card>
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', background: '#f0f2f5' }}>
      <Card style={{ width: 400 }}>
        <Result
          status="error"
          icon={<CloseCircleOutlined />}
          title="登录失败"
          subTitle={error}
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

export default Callback