---
purpose: 认证模块业务规则说明
last_updated: 2026-07-06
source_of_truth:
  - internal/api/handlers.go（认证相关 handler）
  - internal/middleware/jwt.go（JWT 中间件）
  - internal/database/models.go（User 模型）
  - frontend/src/pages/Login.tsx（登录页面）
  - frontend/src/pages/Callback.tsx（钉钉回调）
  - frontend/src/store/authStore.ts（认证状态管理）
update_when:
  - 修改登录流程时
  - 修改认证方式时
  - 修改 JWT 逻辑时
  - 修改钉钉免登流程时
---

# 认证模块

## 模块定位

处理用户登录、登出、JWT 认证、钉钉扫码登录、钉钉内免登。

---

## 数据模型

### User
用户模型

```go
type User struct {
    ID            uint
    OrgID         string  // 当前钉钉企业/租户 ID
    UserID        string  // 系统内用户 ID（多企业时可为 org_id:钉钉用户ID）
    DingTalkUserID string // 钉钉原始用户 ID
    Name          string
    Email         string
    Mobile        string
    Password      string  // 密码哈希，JSON 不输出
    DepartmentID  string
    Position      string
    Avatar        string
    Status        string
    ManagerUserID string
    ManagerName   string
    Extension     map[string]interface{}
    CreatedAt     time.Time
    UpdatedAt     time.Time
    DeletedAt     gorm.DeletedAt
}
```

### DingTalkBinding
本地用户↔钉钉账号绑定

```go
type DingTalkBinding struct {
    ID             uint
    OrgID          string
    UserID         string  // 本地用户 ID
    DingTalkUserID string  // 钉钉用户 ID
    UnionID        string
    OpenID         string
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

### UserSession
用户会话

```go
type UserSession struct {
    ID        uint
    OrgID     string
    UserID    string
    SessionID string
    Token     string
    ExpiresAt time.Time
    IP        string
    UserAgent string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### LoginLog
登录日志

```go
type LoginLog struct {
    ID          uint
    OrgID       string
    UserID      string
    UserName    string
    LoginType   string  // dingtalk_qr / dingtalk_in_app / dingtalk_account / local
    LoginStatus string  // success / failed
    IP          string
    UserAgent   string
    ErrorMsg    string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

---

## API 接口

当前前端只开放钉钉扫码登录和钉钉内免登；账号密码登录不作为当前业务入口。

### POST /api/v1/auth/logout
登出

需要 JWT 认证。

### GET /api/v1/auth/me
获取当前用户信息

需要 JWT 认证。

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "user": {
            "id": 1,
            "user_id": "admin",
            "name": "管理员",
            "email": "admin@example.com"
        }
    }
}
```

---

## 钉钉登录

### GET /api/v1/auth/dingtalk/qr/start
钉钉扫码登录（获取二维码）

Query 参数：
- `org_id` / `org`：推荐传入，指定本次扫码登录的本地企业

说明：
- 多企业场景下，前端登录页会先调用 `GET /api/v1/auth/orgs` 展示活跃企业列表，用户选择企业后再发起二维码登录。
- 后端只信任本地 `org_id`，再映射到该企业保存的钉钉 `appKey/appSecret/corpId`；前端不得直接提交 `corpId` 作为信任边界。
- 如果未传 `org_id` 且配置了 `DINGTALK_QR_DEFAULT_ORG_ID`，后端会把默认企业同时写入登录 `state` 并用于生成 OAuth 二维码；否则才尝试共享扫码入口。

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "qr_code_url": "https://oapi.dingtalk.com/connect/qrconnect?..."
    }
}
```

### POST /api/v1/auth/dingtalk/in-app
钉钉内应用免登

Body：
```json
{
    "code": "xxx",
    "org_id": "xiaotie"
}
```

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "user": {...},
        "expires_at": "2026-07-01T18:00:00+08:00",
        "auth_mode": "cookie",
        "org_id": "xiaotie"
    }
}
```

### GET /api/v1/auth/dingtalk/callback
钉钉 OAuth 回调

Query 参数：
- `code`：钉钉返回的授权码
- `state`：服务端生成；如果扫码前显式指定了 `org_id`，会保留该企业上下文；否则回调后按钉钉实际登录身份反查本地企业

### GET /api/v1/auth/dingtalk/config
获取钉钉配置（前端免登用）

Query 参数：
- `org_id` / `org`：可选，指定钉钉企业；前端会透传当前 URL 中的企业参数

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "corp_id": "xxx",
        "agent_id": "xxx",
        "org_id": "xiaotie"
    }
}
```

---

## JWT 认证

### Claims 结构

```go
type Claims struct {
    UserID    string `json:"user_id"`
    UserDBID  string `json:"user_db_id,omitempty"`
    UserName  string `json:"user_name"`
    OrgID     string `json:"org_id,omitempty"`
    SessionID string `json:"session_id,omitempty"`
    jwt.RegisteredClaims
}
```

### 认证安全约束

- `JWTAuth` 从 `Authorization: Bearer <token>` 或 HttpOnly Cookie 读取 token，不接受 URL query token。
- JWT 验签后必须加载当前用户，并要求 `users.status = active` 且未删除；禁用用户不能登录，也不能继续使用未过期 token。
- JWT 中的 `org_id` 是后续业务数据隔离的上下文来源；旧 token 未携带 `org_id` 时会回退到 `default`，随后以当前用户记录的 `OrgID` 为准。
- 登录成功会写入 `UserSession`，新 token 必须带 `session_id` 和当前 `session_version`；旧版缺少这些声明或版本不匹配的 token 会被拒绝，用户需重新登录。
- 浏览器登录态通过 `peopleops_auth` HttpOnly Cookie 维护，写操作通过 `peopleops_csrf` + `X-CSRF-Token` 双提交校验。
- JWT 默认有效期为 480 分钟，可通过 `JWT_TTL_MINUTES` 配置，代码会限制在 5-1440 分钟范围内。
- 钉钉组织同步将用户置为非 active 时，会撤销该用户仍未撤销的服务端 session。
- 文件访问 `/api/v1/files/:filename` 必须通过 Authorization header 或认证 Cookie 访问，前端使用授权 fetch + object URL 预览，不把主 JWT 拼进 URL。
- 前端 `authStore` 不保存 JWT，只保存用户、菜单和权限状态；刷新页面后通过 `/auth/me` 和 HttpOnly Cookie 恢复会话。

### 中间件

`internal/middleware/jwt.go`：

```go
func JWTAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. 从 Header 读取 token
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.JSON(401, Response{Code: 401, Message: "unauthorized"})
            c.Abort()
            return
        }

        // 2. 验证 token
        token := strings.TrimPrefix(authHeader, "Bearer ")
        claims, err := parseToken(token)
        if err != nil {
            c.JSON(401, Response{Code: 401, Message: "invalid token"})
            c.Abort()
            return
        }

        // 3. 写入 context
        c.Set("userID", claims.UserID)
        c.Set("userName", claims.UserName)
        c.Next()
    }
}
```

### Handler 中获取当前用户

```go
func SomeHandler(c *gin.Context) {
    userID, _ := c.Get("userID")
    userName, _ := c.Get("userName")
    
    // 使用 userID 和 userName
}
```

---

## 前端集成

### 登录页面

`frontend/src/pages/Login.tsx`

功能：
- 钉钉扫码登录
- 钉钉内免登

账号密码登录不是当前业务入口，登录页只提供钉钉扫码和钉钉内免登。

### 钉钉免登流程

`frontend/src/pages/Login.tsx` 中实现：

1. 通过 User-Agent 判断是否在钉钉内
2. 调用 `dd.runtime.permission.requestAuthCode()` 获取授权码
3. 调用 `/api/v1/auth/dingtalk/in-app` 建立服务端 session 和 HttpOnly Cookie
4. 将返回的用户、菜单和权限状态写入 `authStore`

### 认证状态管理

`frontend/src/store/authStore.ts`：

```tsx
interface AuthState {
    user: User | null;
    isLoggedIn: boolean;
    login: (user: User) => void;
    logout: () => void;
}

export const useAuthStore = create<AuthState>()(
    (set) => ({
        user: null,
        isLoggedIn: false,
        login: (user) => set({ user, isLoggedIn: true }),
        logout: () => set({ user: null, isLoggedIn: false }),
    })
);
```

### API 拦截器

`frontend/src/services/api.ts`：

```tsx
// 请求拦截：Cookie 自动随请求发送，写操作补充 CSRF header
api.interceptors.request.use((config) => {
    config.withCredentials = true;
    config.headers['X-CSRF-Token'] = readCookie('peopleops_csrf');
    return config;
});

// 响应拦截：401 自动登出
api.interceptors.response.use(
    (response) => response.data,
    (error) => {
        if (error.response?.status === 401) {
            useAuthStore.getState().logout();
            window.location.href = '/login';
        }
        return Promise.reject(error);
    }
);
```

---

## 环境变量

- `JWT_SECRET`：JWT 签名密钥
- `JWT_TTL_MINUTES`：JWT 有效期分钟数，默认 480，限制 5-1440
- `AUTH_SESSION_VERSION`：认证会话版本，默认 `cookie-v1`；变更该值可强制所有旧 token 失效
- `AUTH_COOKIE_SECURE`：生产 HTTPS 部署建议设为 `true`
- `AUTH_COOKIE_SAMESITE`：认证 Cookie SameSite 策略，默认 `lax`
- `DINGTALK_APP_KEY`：钉钉应用 Key
- `DINGTALK_APP_SECRET`：钉钉应用 Secret
- `DINGTALK_CORP_ID`：钉钉企业 ID
- `DINGTALK_AGENT_ID`：钉钉应用 Agent ID
- `DINGTALK_REDIRECT_URI`：OAuth 回调地址
- `DINGTALK_QR_DEFAULT_ORG_ID`：可选；多企业电脑扫码无 `org_id` 时写入登录 `state` 的默认本地企业 ID，用于域名固定企业入口

---

## 常见问题

### 登录后认证失效
- 检查 `JWT_SECRET` 是否一致
- 检查 token 是否过期，或 `UserSession` 是否已撤销
- 浏览器访问检查 `peopleops_auth` Cookie 是否存在、`AUTH_COOKIE_SECURE` 与 HTTPS 是否匹配
- API 客户端访问检查 `Authorization` header 格式是否正确（`Bearer <token>`）

### 钉钉扫码登录失败
- 检查 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`、`DINGTALK_CORP_ID`
- 检查钉钉应用权限
- 检查 `DINGTALK_REDIRECT_URI` 是否正确

### 钉钉内免登失败
- 检查是否在钉钉内打开（当前前端通过 User-Agent 是否包含 DingTalk 判断）
- 检查 `DINGTALK_AGENT_ID` 是否正确
- 检查钉钉应用权限（需要"获取用户信息"权限）

### 401 错误
- 检查是否已扫码登录
- 检查 `peopleops_auth` Cookie 或 `Authorization` header 是否存在
- 检查 token 是否过期、是否缺少 `session_id`
- 检查后端 JWT 中间件是否正常工作

### 2026-07-01 钉钉多企业登录隔离

- 多企业配置场景下，钉钉内免登仍必须显式传入 `org_id` / `org`；电脑端扫码登录可以不预选企业，但缺少企业上下文时不得自动静默回落到 `default`。
- 钉钉免登和扫码回调只能按当前 `org_id` 下的钉钉身份（`user_id` / `dingtalk_user_id`）匹配本地用户；不得再用邮箱或手机号作为登录兜底匹配。
- 扫码回调 `GET /api/v1/auth/dingtalk/callback` 与钉钉内免登 `POST /api/v1/auth/dingtalk/in-app` 当前都已接入“首次登录自动补本地用户”逻辑：当本地按 `org_id + scoped user_id / dingtalk_user_id` 查不到用户时，会调用 `ensureLocalUserForDingTalkLogin()` 自动创建 `users`、默认普通员工角色与 `employee_profiles`，然后继续完成登录。
- 如果浏览器仍然落到 `登录失败 / dingtalk user not synced, please sync org data first`，说明扫码回调这条线上自动补本地用户没有成功落库；排查优先看 `internal/api/handlers.go` 中 `DingTalkCallback` 的 `auto provision failed` 日志，而不是先怀疑前端页面。
- JWT 校验用户和 session 时必须同时约束 `org_id`，token 中企业、用户记录企业、session 企业必须保持一致。
- 前端登录页需要从当前 URL 或 `redirect` 目标中恢复 `org_id`，确保业务页带企业参数跳转登录后不丢失企业上下文。
- 2026-07-04 起，电脑端扫码登录默认允许不预选企业：回调会优先用钉钉返回的 `corpId/corp_id` 反查本地企业，拿不到时再用 `unionId/openId/dingtalk_user_id` 做兜底解析；解析失败或匹配到多个企业时必须阻断登录，不能静默落到 `default`。
- 多企业扫码登录的 `state` 必须同时保存业务企业 `org_id` 和实际生成二维码的 OAuth 配置企业；回调换取用户 access token 时必须使用 OAuth 配置企业，后续本地用户匹配与 session 仍使用解析出的业务企业。
- OAuth 回调若只返回 `unionId/openId`，需要先通过 `DingTalkBinding` 兜底匹配本地用户；登录成功后要回写/更新绑定。只有能解析出企业内 `userid` 时才允许自动创建本地用户，不能用 openId 伪造企业内用户 ID。
- 2026-07-06 线上复测：`POST /v1.0/oauth2/user/getInfo` 在当前钉钉网关返回 `InvalidAction.NotFound`，不可作为企业识别路径；扫码 OAuth 用户信息继续使用 `/v1.0/contact/users/me`，并按其返回字段做安全判断。
- 扫码 OAuth 换取 `userAccessToken` 的响应也可能包含 `corpId/userid/openId/unionId`；这些身份字段必须合并到 `/v1.0/contact/users/me` 的用户信息后再做本地企业校验，避免“钉钉返回了企业信息但代码丢掉”的误判。
- 2026-07-06 复核：当前为钉钉企业内部应用，多企业无预选扫码时，钉钉官方组织选择页的选择结果不会可靠以 `corpId` / 企业内 `userid` 回传；如果回调只有 `unionId/openId`，不得再调用各企业通讯录接口反查后自动选择企业，否则会出现用户在钉钉页选“机器人集合”却进入 `xiaotie` 的跨企业误入风险。
- 当前电脑端扫码登录应优先在本系统预选本地企业：登录页展示活跃企业列表，选中的 `org_id` 会传给 `/auth/dingtalk/qr/start`，后端用同一个本地企业生成 OAuth 二维码并写入 `state`。
- 如果用户在本系统选择的企业与钉钉官方组织选择页实际选择的组织不一致，回调必须拦截：只有 `corpId` 匹配所选企业，或钉钉明确返回企业内 `userid/associated_user_id` 时才允许继续；仅有 `unionId/openId` 不能作为“选择了该企业”的证明。
- 如果某个域名固定对应一个本地企业，可以设置 `DINGTALK_QR_DEFAULT_ORG_ID`（如 `default`）让无参数电脑扫码同时使用该企业生成 OAuth 二维码并写入 `state`；不要用企业 A 的 OAuth 二维码回调到企业 B，否则 `unionId` 可能无法在企业 B 换取 `userid`。
