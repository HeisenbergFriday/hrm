import React, { useEffect, useState } from 'react'
import { Result, Button, message } from 'antd'
import { CloseCircleOutlined } from '@ant-design/icons'
import { useNavigate, useSearchParams } from 'react-router-dom'
import axios from 'axios'
import { useAuthStore } from '../store/authStore'
import {
  alignAuthRedirectTargetWithOrg,
  directAuthOrgIDFromSearchParams,
  consumeAuthRedirect,
  rememberAuthOrgID,
} from '../utils/authRedirect'

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

const pendingCallbackRequests = new Map<string, Promise<any>>()

function requestDingTalkCallbackOnce(callbackKey: string, params: Record<string, string | undefined>) {
  const pendingRequest = pendingCallbackRequests.get(callbackKey)
  if (pendingRequest) return pendingRequest

  const request = axios.get('/api/v1/auth/dingtalk/callback', {
    params,
    withCredentials: true,
  }).finally(() => {
    pendingCallbackRequests.delete(callbackKey)
  })
  pendingCallbackRequests.set(callbackKey, request)
  return request
}

const Callback: React.FC = () => {
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { login } = useAuthStore()
  const requestedOrgID = directAuthOrgIDFromSearchParams(searchParams)
  const loginPath = requestedOrgID
    ? `/login?mode=scan&org_id=${encodeURIComponent(requestedOrgID)}`
    : '/login?mode=scan'

  useEffect(() => {
    const handleCallback = async () => {
      const code = searchParams.get('code')
      const state = searchParams.get('state')
      if (!code) {
        if (isDingTalkEnv()) {
          navigate(consumeAuthRedirect() || '/', { replace: true })
          return
        }
        setError('缺少 code 参数')
        setLoading(false)
        return
      }

      const callbackKey = state || code

      try {
        const response = await requestDingTalkCallbackOnce(callbackKey, { code, state, org_id: requestedOrgID || undefined })

        if (response.data.code === 200) {
          const { user } = response.data.data
          login(user)
          rememberAuthOrgID(user?.org_id)
          message.success('登录成功', 0.6)
          navigate(alignAuthRedirectTargetWithOrg(consumeAuthRedirect(), user?.org_id), { replace: true })
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
  }, [searchParams, navigate, login, requestedOrgID])

  if (loading) {
    return (
      <div className="callback-page" />
    )
  }

  return (
    <div className="callback-page">
      <Result
        status="error"
        icon={<CloseCircleOutlined />}
        title="登录失败"
        subTitle={error}
        extra={[
          <Button type="primary" key="login" onClick={() => navigate(loginPath, { replace: true })}>
            返回扫码页
          </Button>,
        ]}
      />
    </div>
  )
}

export default Callback
