import React, { useEffect, useState } from 'react'
import axios from 'axios'
import { Alert, Button, Card, Radio, Space, Spin, Typography, message } from 'antd'
import { LoadingOutlined, MobileOutlined, QrcodeOutlined } from '@ant-design/icons'
import { useLocation, useSearchParams } from 'react-router-dom'
import { useAuthStore } from '../store/authStore'
import { orgAPI } from '../services/api'
import {
  alignAuthRedirectTargetWithOrg,
  authOrgIDFromSearchParams,
  authOrgIDFromSearchParamsOrStorage,
  authRedirectTargetFromLocation,
  normalizeAuthRedirectTarget,
  rememberAuthOrgID,
  rememberAuthRedirect,
  resolveAuthOrgID,
} from '../utils/authRedirect'

const { Paragraph, Text, Title } = Typography

interface Organization {
  org_id: string
  name: string
}

interface DingTalkDebugConfig {
  org_id: string
  corp_id: string
  missing?: string[]
}

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
  const [inAppStatus, setInAppStatus] = useState('')
  const [organizations, setOrganizations] = useState<Organization[]>([])
  const [defaultQROrgID, setDefaultQROrgID] = useState('')
  const [orgsLoading, setOrgsLoading] = useState(true)
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const { login } = useAuthStore()
  const forceScanMode = searchParams.get('mode') === 'scan'
  const inDingTalk = isDingTalkEnv()
  const isQRCodeLogin = !inDingTalk || forceScanMode
  const requestedOrgID = authOrgIDFromSearchParams(searchParams)
  const rememberedOrgID = authOrgIDFromSearchParamsOrStorage(searchParams)
  const [selectedOrgID, setSelectedOrgID] = useState(requestedOrgID || rememberedOrgID)
  const redirectTarget = normalizeAuthRedirectTarget(searchParams.get('redirect')) || authRedirectTargetFromLocation(location)
  const fallbackOrgID = requestedOrgID || rememberedOrgID || (isQRCodeLogin ? defaultQROrgID : '')
  const effectiveOrgID = resolveAuthOrgID(selectedOrgID, fallbackOrgID, organizations)
  const needOrgSelection = organizations.length > 1
  const requireOrgSelectionForQRCode = isQRCodeLogin && needOrgSelection
  const requireOrgSelectionForInApp = inDingTalk && !forceScanMode && needOrgSelection
  const showOrgSelection = needOrgSelection && (isQRCodeLogin || requireOrgSelectionForInApp)

  // 加载企业列表：电脑扫码先选择本地企业，再用该企业配置发起钉钉 OAuth。
  useEffect(() => {
    setOrgsLoading(true)
    orgAPI.getOrganizations()
      .then((response: any) => {
        const orgs = response?.data?.organizations || []
        const nextDefaultQROrgID = response?.data?.default_qr_org_id || ''
        setOrganizations(orgs)
        setDefaultQROrgID(nextDefaultQROrgID)
      })
      .catch(() => {
        setOrganizations([])
        setDefaultQROrgID('')
      })
      .finally(() => {
        setOrgsLoading(false)
      })
  }, [])

  useEffect(() => {
    const hasOrg = (value: string) => organizations.some((org) => org.org_id === value)

    if (selectedOrgID && hasOrg(selectedOrgID)) {
      return
    }

    if (fallbackOrgID && hasOrg(fallbackOrgID)) {
      setSelectedOrgID(fallbackOrgID)
      return
    }

    if (organizations.length === 1) {
      setSelectedOrgID((currentOrgID) => currentOrgID || organizations[0].org_id)
      return
    }

    if (organizations.length > 1 && selectedOrgID && !hasOrg(selectedOrgID)) {
      setSelectedOrgID('')
    }
  }, [fallbackOrgID, organizations, selectedOrgID])

  useEffect(() => {
    const error = searchParams.get('error')
    if (error) {
      message.error(decodeURIComponent(error))
    }
  }, [searchParams])

  useEffect(() => {
    if (effectiveOrgID) rememberAuthOrgID(effectiveOrgID)
  }, [effectiveOrgID])

  const handleDingTalkQRLogin = async () => {
    if (requireOrgSelectionForQRCode && !effectiveOrgID) {
      message.warning('请先选择要登录的企业')
      return
    }

    setLoading(true)
    rememberAuthRedirect(redirectTarget)
    rememberAuthOrgID(effectiveOrgID)
    try {
      const params = effectiveOrgID ? { org_id: effectiveOrgID } : undefined
      const response = await axios.get('/api/v1/auth/dingtalk/qr/start', { params, withCredentials: true })
      const loginUrl = response.data.data.qr_code_url

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

    if (!effectiveOrgID && needOrgSelection) {
      setInAppStatus('请先选择要登录的企业')
      message.warning('请先选择要登录的企业')
      return
    }

    setAutoLogging(true)
    setInAppStatus('正在获取钉钉配置...')

    try {
      const params = effectiveOrgID ? { org_id: effectiveOrgID } : undefined
      const configRes = await axios.get('/api/v1/auth/dingtalk/config', { params, withCredentials: true })
      const configData = configRes.data.data as DingTalkDebugConfig
      const { corp_id: corpId, missing } = configData
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
              org_id: effectiveOrgID || undefined,
            }, { withCredentials: true })
            const { user } = response.data.data
            login(user)
            rememberAuthOrgID(user?.org_id || effectiveOrgID)
            message.success('登录成功', 0.6)
            window.location.replace(alignAuthRedirectTargetWithOrg(redirectTarget, user?.org_id || effectiveOrgID))
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
    if (orgsLoading) {
      return
    }

    if (requireOrgSelectionForInApp && !effectiveOrgID) {
      return
    }

    if (isDingTalkEnv() && !forceScanMode) {
      void handleDingTalkInAppLogin()
    }
  }, [effectiveOrgID, forceScanMode, orgsLoading, requireOrgSelectionForInApp])

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

  return (
    <div className="login-page">
      <Card title={<Title level={4} className="login-title">钉钉一体化人事后台</Title>} className="login-card">
        <Space direction="vertical" size="middle" className="login-space">
          {orgsLoading ? (
            <div className="login-orgs-loading">
              <Spin />
            </div>
          ) : showOrgSelection ? (
            <div className="login-org-selector">
              <Text strong>选择要登录的企业</Text>
              <Radio.Group
                className="login-org-radio"
                value={selectedOrgID}
                onChange={(e) => setSelectedOrgID(e.target.value)}
              >
                <Space direction="vertical" size="small" className="login-org-options">
                  {organizations.map((org) => (
                    <Radio key={org.org_id} value={org.org_id}>
                      {org.name}
                    </Radio>
                  ))}
                </Space>
              </Radio.Group>
            </div>
          ) : null}

          <Alert
            type="info"
            showIcon
            message={inDingTalk && !forceScanMode ? '当前在钉钉内打开，将使用免登' : '当前将使用钉钉扫码登录'}
            description={
              inDingTalk && !forceScanMode
                ? '如果自动登录未成功，可以点击下方按钮重新发起免登。'
                : '请先选择要登录的企业，跳到钉钉官方页后也请选择同一组织；不一致会被系统拦截。'
            }
          />

          <Paragraph className="login-helper">
            {inDingTalk && !forceScanMode
              ? '钉钉微应用首页应配置为应用根地址，手机端打开后会自动发起免登。'
              : '电脑扫码登录会使用所选企业的钉钉应用配置，回调地址需要与当前访问地址一致。'}
          </Paragraph>

          {inDingTalk && !forceScanMode ? (
            <Button
              type="primary"
              block
              icon={<MobileOutlined />}
              onClick={() => void handleDingTalkInAppLogin()}
              disabled={requireOrgSelectionForInApp && !effectiveOrgID}
            >
              重新发起钉钉免登
            </Button>
          ) : (
            <Button
              type="primary"
              block
              loading={loading}
              icon={<QrcodeOutlined />}
              onClick={() => void handleDingTalkQRLogin()}
              disabled={loading || orgsLoading || (requireOrgSelectionForQRCode && !effectiveOrgID)}
            >
              打开钉钉官方扫码登录页
            </Button>
          )}

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
