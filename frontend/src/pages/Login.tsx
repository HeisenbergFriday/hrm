import React, { useEffect, useState } from 'react'
import axios from 'axios'
import { Alert, Button, Card, Space, Spin, Typography, message } from 'antd'
import { LoadingOutlined, MobileOutlined, QrcodeOutlined } from '@ant-design/icons'
import { useLocation, useSearchParams } from 'react-router-dom'
import { useAuthStore } from '../store/authStore'
import {
  authRedirectTargetFromLocation,
  normalizeAuthRedirectTarget,
  rememberAuthRedirect,
} from '../utils/authRedirect'

const { Paragraph, Text, Title } = Typography

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

const Login: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [autoLogging, setAutoLogging] = useState(false)
  const [redirectUri, setRedirectUri] = useState('')
  const [inAppStatus, setInAppStatus] = useState('')
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const { login } = useAuthStore()
  const forceScanMode = searchParams.get('mode') === 'scan'
  const redirectTarget = normalizeAuthRedirectTarget(searchParams.get('redirect')) || authRedirectTargetFromLocation(location)

  useEffect(() => {
    const error = searchParams.get('error')
    if (error) {
      message.error(decodeURIComponent(error))
    }
  }, [searchParams])

  const handleDingTalkQRLogin = async () => {
    setLoading(true)
    rememberAuthRedirect(redirectTarget)
    try {
      const response = await axios.get('/api/v1/auth/dingtalk/qr/start', { withCredentials: true })
      const nextRedirectUri = response.data.data.redirect_uri || ''
      const loginUrl = response.data.data.qr_code_url

      setRedirectUri(nextRedirectUri)

      if (!loginUrl) {
        message.error('未获取到钉钉登录地址')
        return
      }

      window.location.href = loginUrl
    } catch (err) {
      message.error(getAxiosErrorMessage(err, '获取钉钉扫码登录地址失败'))
    } finally {
      setLoading(false)
    }
  }

  const handleDingTalkInAppLogin = async () => {
    if (!isDingTalkEnv()) {
      setInAppStatus('当前不在钉钉客户端内。')
      return
    }

    setAutoLogging(true)
    setInAppStatus('正在获取钉钉配置...')

    try {
      const configRes = await axios.get('/api/v1/auth/dingtalk/config', { withCredentials: true })
      const { corp_id: corpId, missing } = configRes.data.data
      const dd = (window as any).dd

      if (!corpId || (Array.isArray(missing) && missing.includes('DINGTALK_CORP_ID'))) {
        const text = '缺少 DINGTALK_CORP_ID，暂时无法使用钉钉内免登。'
        setInAppStatus(text)
        message.error(text)
        setAutoLogging(false)
        return
      }

      if (!dd?.runtime?.permission?.requestAuthCode) {
        const text = '钉钉 JS-SDK 未加载或未授权。'
        setInAppStatus(text)
        message.error(text)
        setAutoLogging(false)
        return
      }

      setInAppStatus('已获取配置，正在请求钉钉授权码...')

      dd.runtime.permission.requestAuthCode({
        corpId,
        onSuccess: async (result: { code: string }) => {
          try {
            setInAppStatus('已拿到授权码，正在请求后端登录...')
            const response = await axios.post('/api/v1/auth/dingtalk/in-app', {
              code: result.code,
            }, { withCredentials: true })
            const { user } = response.data.data
            login(user)
            message.success('登录成功', 0.6)
            window.location.replace(redirectTarget || '/')
          } catch (err) {
            const text = getAxiosErrorMessage(err, '钉钉内免登失败')
            console.error('[DingTalk InApp] login failed', err)
            setInAppStatus(text)
            message.error(text)
            setAutoLogging(false)
          }
        },
        onFail: (err: unknown) => {
          console.error('[DingTalk InApp] requestAuthCode failed', err)
          const text = `获取钉钉授权码失败：${JSON.stringify(err)}`
          setInAppStatus(text)
          message.error('获取钉钉授权码失败')
          setAutoLogging(false)
        },
      })
    } catch (err) {
      console.error('[DingTalk InApp] init failed', err)
      const text = getAxiosErrorMessage(err, '钉钉内免登初始化失败')
      setInAppStatus(text)
      message.error(text)
      setAutoLogging(false)
    }
  }

  useEffect(() => {
    if (isDingTalkEnv() && !forceScanMode) {
      void handleDingTalkInAppLogin()
    }
  }, [forceScanMode])

  if (autoLogging) {
    return (
      <div className="login-page">
        <Card className="login-loading-card">
          <Spin indicator={<LoadingOutlined style={{ fontSize: 24 }} spin />} />
          <p>正在通过钉钉自动登录，请稍候...</p>
          {inAppStatus ? (
            <Paragraph className="login-status">
              <Text type="secondary">{inAppStatus}</Text>
            </Paragraph>
          ) : null}
        </Card>
      </div>
    )
  }

  const inDingTalk = isDingTalkEnv()

  return (
    <div className="login-page">
      <Card title={<Title level={4} className="login-title">钉钉一体化人事后台</Title>} className="login-card">
        <Space direction="vertical" size="middle" className="login-space">
          <Alert
            type="info"
            showIcon
            message={inDingTalk && !forceScanMode ? '当前在钉钉内打开，将使用免登' : '当前将使用钉钉扫码登录'}
            description={
              inDingTalk && !forceScanMode
                ? '如果自动登录未成功，可以点击下方按钮重新发起免登。'
                : '点击下方按钮后，电脑当前页面会跳到钉钉官方登录页。电脑前页面会跳到钉钉官方登录页，用手机扫码确认后，电脑会自动回跳进入项目。'
            }
          />

          <Paragraph className="login-helper">
            {inDingTalk && !forceScanMode
              ? '钉钉微应用首页应配置为应用根地址，手机端打开后会自动发起免登。'
              : '电脑扫码登录的回调地址需要配置到钉钉开放平台，并与当前访问地址一致。'}
          </Paragraph>

          {inDingTalk && !forceScanMode ? (
            <Button type="primary" block icon={<MobileOutlined />} onClick={() => void handleDingTalkInAppLogin()}>
              重新发起钉钉免登
            </Button>
          ) : (
            <Button type="primary" block loading={loading} icon={<QrcodeOutlined />} onClick={() => void handleDingTalkQRLogin()}>
              打开钉钉官方扫码登录页
            </Button>
          )}

          {redirectUri ? (
            <Paragraph copyable className="login-helper">
              当前回调地址: {redirectUri}
            </Paragraph>
          ) : null}

          {inDingTalk && inAppStatus ? (
            <Alert
              type="warning"
              showIcon
              message="钉钉内打开状态"
              description={inAppStatus}
            />
          ) : null}
        </Space>
      </Card>
    </div>
  )
}

export default Login
