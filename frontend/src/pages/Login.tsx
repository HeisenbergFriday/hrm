import React, { useState, useEffect } from 'react'
import axios from 'axios'
import { Card, Form, Input, Button, message, Typography, Space, Modal, QRCode, Spin } from 'antd'
import { UserOutlined, LockOutlined, LoginOutlined, QrcodeOutlined, MobileOutlined, LoadingOutlined } from '@ant-design/icons'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { authAPI } from '../services/api'
import { useAuthStore } from '../store/authStore'

const { Title } = Typography

// 检测是否在钉钉客户端内
function isDingTalkEnv(): boolean {
  return /DingTalk/i.test(navigator.userAgent)
}

const Login: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [qrVisible, setQrVisible] = useState(false)
  const [qrCodeUrl, setQrCodeUrl] = useState('')
  const [autoLogging, setAutoLogging] = useState(false)
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { login } = useAuthStore()

  // 检查是否有登录错误
  const error = searchParams.get('error')
  if (error) {
    message.error(decodeURIComponent(error))
  }

  // 钉钉环境下自动免登
  useEffect(() => {
    if (!isDingTalkEnv()) return

    setAutoLogging(true)

    const doAutoLogin = async () => {
      try {
        // 获取 corpId
        const configRes = await axios.get('/api/v1/auth/dingtalk/config')
        const corpId = configRes.data.data.corp_id

        // 调用钉钉 JS-API 获取免登码
        const dd = (window as any).dd
        if (!dd) {
          message.error('钉钉 JS-SDK 未加载')
          setAutoLogging(false)
          return
        }

        dd.runtime.permission.requestAuthCode({
          corpId: corpId,
          onSuccess: async (result: { code: string }) => {
            try {
              const response = await axios.post('/api/v1/auth/dingtalk/in-app', {
                code: result.code,
              })
              const { token, user } = response.data.data
              login(user, token)
              message.success('登录成功')
              navigate('/')
            } catch {
              message.error('钉钉免登失败，请尝试其他方式登录')
              setAutoLogging(false)
            }
          },
          onFail: () => {
            message.error('获取钉钉授权码失败')
            setAutoLogging(false)
          },
        })
      } catch {
        message.error('钉钉免登初始化失败')
        setAutoLogging(false)
      }
    }

    doAutoLogin()
  }, [login, navigate])

  const onFinish = async (values: any) => {
    setLoading(true)
    try {
      const response = await authAPI.login({
        username: values.username,
        password: values.password
      })

      const { token, user } = response.data
      login(user, token)
      message.success('登录成功')
      navigate('/')
    } catch (error) {
      message.error('登录失败，请检查用户名和密码')
    } finally {
      setLoading(false)
    }
  }

  const handleDingTalkQRLogin = async () => {
    setLoading(true)
    try {
      const response = await axios.get('/api/v1/auth/dingtalk/qr/start')
      setQrCodeUrl(response.data.data.qr_code_url)
      setQrVisible(true)
    } catch (error) {
      message.error('获取二维码失败')
    } finally {
      setLoading(false)
    }
  }

  const handleDingTalkInAppLogin = async () => {
    if (!isDingTalkEnv()) {
      message.info('请在钉钉客户端中打开此页面')
      return
    }
    // 在钉钉内手动触发免登
    setAutoLogging(true)
    try {
      const configRes = await axios.get('/api/v1/auth/dingtalk/config')
      const corpId = configRes.data.data.corp_id
      const dd = (window as any).dd

      dd.runtime.permission.requestAuthCode({
        corpId: corpId,
        onSuccess: async (result: { code: string }) => {
          try {
            const response = await axios.post('/api/v1/auth/dingtalk/in-app', {
              code: result.code,
            })
            const { token, user } = response.data.data
            login(user, token)
            message.success('登录成功')
            navigate('/')
          } catch {
            message.error('钉钉免登失败')
            setAutoLogging(false)
          }
        },
        onFail: () => {
          message.error('获取钉钉授权码失败')
          setAutoLogging(false)
        },
      })
    } catch {
      message.error('钉钉免登初始化失败')
      setAutoLogging(false)
    }
  }

  // 钉钉环境下自动免登时显示 loading
  if (autoLogging) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', background: '#f0f2f5' }}>
        <Card style={{ width: 400, textAlign: 'center' }}>
          <Spin indicator={<LoadingOutlined style={{ fontSize: 24 }} spin />} />
          <p style={{ marginTop: 16 }}>正在通过钉钉自动登录，请稍候...</p>
        </Card>
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', background: '#f0f2f5' }}>
      <Card title={<Title level={4}>钉钉一体化人事后台</Title>} style={{ width: 400 }}>
        <Form
          name="login"
          initialValues={{ remember: true }}
          onFinish={onFinish}
        >
          <Form.Item
            name="username"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input prefix={<UserOutlined />} placeholder="用户名" />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder="密码" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block icon={<LoginOutlined />}>
              登录
            </Button>
          </Form.Item>
          <Form.Item>
            <Space direction="vertical" style={{ width: '100%' }}>
              <Button
                type="default"
                block
                icon={<QrcodeOutlined />}
                onClick={handleDingTalkQRLogin}
              >
                钉钉扫码登录
              </Button>
              <Button
                type="default"
                block
                icon={<MobileOutlined />}
                onClick={handleDingTalkInAppLogin}
              >
                钉钉内免登
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Modal
        title="钉钉扫码登录"
        open={qrVisible}
        onCancel={() => setQrVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setQrVisible(false)}>
            取消
          </Button>
        ]}
      >
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
          <QRCode value={qrCodeUrl} size={200} />
          <p style={{ marginTop: 16, textAlign: 'center' }}>请使用钉钉扫码登录</p>
        </div>
      </Modal>
    </div>
  )
}

export default Login
