import React, { useEffect, useState } from 'react'
import axios from 'axios'
import { Alert, Button, Card, Divider, Form, Input, Modal, Radio, Select, Space, Spin, Typography, message } from 'antd'
import { LoadingOutlined, LockOutlined, MobileOutlined, QrcodeOutlined, UserOutlined } from '@ant-design/icons'
import { useLocation, useSearchParams } from 'react-router-dom'
import { useAuthStore } from '../store/authStore'
import {
  authRedirectTargetFromLocation,
  normalizeAuthRedirectTarget,
  rememberAuthRedirect,
} from '../utils/authRedirect'
import { orgIdParams, rememberOrgId } from '../utils/org'

const { Paragraph, Text, Title } = Typography

interface OrganizationOption {
  org_id: string
  name: string
  corp_id: string
  agent_id: string
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

function getAvailableOrganizations(error: unknown): OrganizationOption[] {
  if (!axios.isAxiosError(error)) {
    return []
  }

  const orgs = error.response?.data?.data?.available_organizations
  if (!Array.isArray(orgs)) {
    return []
  }

  return orgs.filter((org): org is OrganizationOption => (
    org &&
    typeof org.org_id === 'string' &&
    typeof org.name === 'string' &&
    typeof org.corp_id === 'string'
  ))
}

const Login: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [autoLogging, setAutoLogging] = useState(false)
  const [inAppStatus, setInAppStatus] = useState('')
  const [orgModalVisible, setOrgModalVisible] = useState(false)
  const [orgList, setOrgList] = useState<OrganizationOption[]>([])
  const [selectedOrgId, setSelectedOrgId] = useState<string>('')
  const [orgSelectLoading, setOrgSelectLoading] = useState(false)
  const [pendingLoginMode, setPendingLoginMode] = useState<'qr' | 'inapp'>('qr')
  const [localLoginLoading, setLocalLoginLoading] = useState(false)
  const [localOrgList, setLocalOrgList] = useState<OrganizationOption[]>([])
  const [form] = Form.useForm()
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

  // 获取组织列表用于本地登录
  useEffect(() => {
    const fetchOrgs = async () => {
      try {
        const response = await axios.get('/api/v1/auth/dingtalk/config')
        const orgs: OrganizationOption[] = response.data.data.organizations || []
        setLocalOrgList(orgs)
        if (orgs.length > 0) {
          form.setFieldsValue({ org_id: orgs[0].org_id })
        }
      } catch (err) {
        console.error('获取组织列表失败', err)
      }
    }
    void fetchOrgs()
  }, [form])

  // 钉钉内自动免登：也需要先选择组织
  useEffect(() => {
    if (isDingTalkEnv() && !forceScanMode) {
      void fetchOrganizationsAndSelect('inapp')
    }
  }, [forceScanMode])

  // 获取组织列表，决定是否弹出选择框
  const fetchOrganizationsAndSelect = async (mode: 'qr' | 'inapp') => {
    setPendingLoginMode(mode)
    setOrgSelectLoading(true)
    try {
      const response = await axios.get('/api/v1/auth/dingtalk/config', {
        params: orgIdParams(),
      })
      const orgs: OrganizationOption[] = response.data.data.organizations || []
      if (orgs.length === 0) {
        message.error('暂无可用组织，请联系管理员配置')
        return
      }
      if (orgs.length === 1) {
        // 只有一个组织，直接登录
        rememberOrgId(orgs[0].org_id)
        if (mode === 'qr') {
          await executeQRLogin(orgs[0].org_id)
        } else {
          await executeInAppLogin(orgs[0].org_id)
        }
        return
      }
      // 多个组织，弹出选择框
      setOrgList(orgs)
      setSelectedOrgId(orgs[0].org_id)
      setOrgModalVisible(true)
    } catch (err) {
      message.error(getAxiosErrorMessage(err, '获取组织列表失败'))
    } finally {
      setOrgSelectLoading(false)
    }
  }

  // 确认选择组织后执行登录
  const handleOrgConfirm = async () => {
    if (!selectedOrgId) {
      message.warning('请选择一个组织')
      return
    }
    rememberOrgId(selectedOrgId)
    setOrgModalVisible(false)
    if (pendingLoginMode === 'qr') {
      await executeQRLogin(selectedOrgId)
    } else {
      await executeInAppLogin(selectedOrgId)
    }
  }

  const executeQRLogin = async (orgId: string) => {
    setLoading(true)
    rememberAuthRedirect(redirectTarget)
    try {
      const response = await axios.get('/api/v1/auth/dingtalk/qr/start', {
        params: { org_id: orgId },
        withCredentials: true,
      })
      const loginUrl = response.data.data.qr_code_url

      console.info('[DingTalk QR] qr_code_url =', loginUrl)

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

  const executeInAppLogin = async (orgId: string) => {
    if (!isDingTalkEnv()) {
      setInAppStatus('当前不在钉钉客户端内。')
      return
    }

    setAutoLogging(true)
    setInAppStatus('正在获取钉钉配置...')

    try {
      const configRes = await axios.get('/api/v1/auth/dingtalk/config', {
        params: orgId ? { org_id: orgId } : {},
        withCredentials: true,
      })
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
              org_id: orgId,
            }, { withCredentials: true })
            const { user } = response.data.data
            login(user)
            message.success('登录成功', 0.6)
            window.location.replace(redirectTarget || '/')
          } catch (err) {
            const text = getAxiosErrorMessage(err, '钉钉内免登失败')
            console.error('[DingTalk InApp] login failed', err)
            const availableOrgs = getAvailableOrganizations(err)
            setInAppStatus(text)
            message.error(text)
            if (availableOrgs.length > 0) {
              setPendingLoginMode('inapp')
              setOrgList(availableOrgs)
              setSelectedOrgId(availableOrgs[0].org_id)
              setOrgModalVisible(true)
            }
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

  const handleDingTalkQRLogin = async () => {
    await fetchOrganizationsAndSelect('qr')
  }

  const handleDingTalkInAppLogin = async () => {
    await fetchOrganizationsAndSelect('inapp')
  }

  // 本地密码登录
  const handleLocalLogin = async (values: { username: string; password: string; org_id: string }) => {
    setLocalLoginLoading(true)
    try {
      const response = await axios.post('/api/v1/auth/login', {
        username: values.username,
        password: values.password,
        org_id: values.org_id,
      }, { withCredentials: true })
      const { user } = response.data.data
      login(user)
      message.success('登录成功', 0.6)
      window.location.replace(redirectTarget || '/')
    } catch (err) {
      message.error(getAxiosErrorMessage(err, '登录失败，请检查用户名和密码'))
    } finally {
      setLocalLoginLoading(false)
    }
  }

  if (autoLogging) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', background: '#f0f2f5' }}>
        <Card style={{ width: 460, textAlign: 'center' }}>
          <Spin indicator={<LoadingOutlined style={{ fontSize: 24 }} spin />} />
          <p style={{ marginTop: 16 }}>正在通过钉钉自动登录，请稍候...</p>
          {inAppStatus ? (
            <Paragraph style={{ marginTop: 12, marginBottom: 0 }}>
              <Text type="secondary">{inAppStatus}</Text>
            </Paragraph>
          ) : null}
        </Card>
      </div>
    )
  }

  const inDingTalk = isDingTalkEnv()

  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', background: '#f0f2f5' }}>
      <Card title={<Title level={4} style={{ margin: 0 }}>钉钉一体化人事后台</Title>} style={{ width: 440 }}>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
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

          <Paragraph style={{ marginBottom: 0 }}>
            {inDingTalk && !forceScanMode
              ? '钉钉微应用首页应配置为应用根地址，例如 http://your-host:8080/ 。'
              : '电脑扫码登录的回调地址需要配置到钉钉开放平台，并与当前访问地址一致。'}
          </Paragraph>

          {inDingTalk && !forceScanMode ? (
            <Button type="primary" block icon={<MobileOutlined />} loading={orgSelectLoading} onClick={() => void handleDingTalkInAppLogin()}>
              重新发起钉钉免登
            </Button>
          ) : (
            <Button type="primary" block loading={loading || orgSelectLoading} icon={<QrcodeOutlined />} onClick={() => void handleDingTalkQRLogin()}>
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

          <Divider plain>或使用账号密码登录</Divider>

          <Form
            form={form}
            onFinish={(values) => void handleLocalLogin(values)}
            layout="vertical"
            style={{ width: '100%' }}
          >
            <Form.Item
              name="org_id"
              label="选择组织"
              rules={[{ required: true, message: '请选择组织' }]}
            >
              <Select
                placeholder="请选择组织"
                options={localOrgList.map((org) => ({
                  value: org.org_id,
                  label: org.name || org.org_id,
                }))}
              />
            </Form.Item>

            <Form.Item
              name="username"
              label="用户名"
              rules={[{ required: true, message: '请输入用户名' }]}
            >
              <Input prefix={<UserOutlined />} placeholder="请输入用户名" />
            </Form.Item>

            <Form.Item
              name="password"
              label="密码"
              rules={[{ required: true, message: '请输入密码' }]}
            >
              <Input.Password prefix={<LockOutlined />} placeholder="请输入密码" />
            </Form.Item>

            <Form.Item style={{ marginBottom: 0 }}>
              <Button type="primary" htmlType="submit" block loading={localLoginLoading}>
                登录
              </Button>
            </Form.Item>
          </Form>
        </Space>
      </Card>

      <Modal
        title="选择加入的组织"
        open={orgModalVisible}
        onOk={() => void handleOrgConfirm()}
        onCancel={() => setOrgModalVisible(false)}
        okText="确认登录"
        cancelText="取消"
        confirmLoading={loading}
      >
        <div style={{ padding: '8px 0' }}>
          <Paragraph type="secondary" style={{ marginBottom: 16 }}>
            请选择你要登录的企业组织：
          </Paragraph>
          <Radio.Group
            value={selectedOrgId}
            onChange={(e) => setSelectedOrgId(e.target.value)}
            style={{ width: '100%' }}
          >
            <Space direction="vertical" style={{ width: '100%' }}>
              {orgList.map((org) => (
                <Radio key={org.org_id} value={org.org_id} style={{ width: '100%', padding: '8px 12px', border: '1px solid #d9d9d9', borderRadius: 6 }}>
                  <div>
                    <Text strong>{org.name || org.org_id}</Text>
                  </div>
                </Radio>
              ))}
            </Space>
          </Radio.Group>
        </div>
      </Modal>
    </div>
  )
}

export default Login
