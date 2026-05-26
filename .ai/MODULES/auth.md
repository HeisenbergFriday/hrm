---
purpose: 认证模块业务规则说明
last_updated: 2026-04-30
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
    UserID        string  // 钉钉用户 ID 或本地账号 ID（唯一键）
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

### POST /api/v1/auth/login
账号密码登录

Body：
```json
{
    "username": "admin",
    "password": "admin123"
}
```

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "user": {
            "id": 1,
            "user_id": "admin",
            "name": "管理员"
        },
        "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
    }
}
```

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

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "qr_code_url": "https://oapi.dingtalk.com/connect/qrconnect?...",
        "redirect_uri": "https://your-host/api/v1/auth/dingtalk/callback"
    }
}
```

### POST /api/v1/auth/dingtalk/in-app
钉钉内应用免登

Body：
```json
{
    "code": "xxx"
}
```

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "user": {...},
        "token": "..."
    }
}
```

### GET /api/v1/auth/dingtalk/callback
钉钉 OAuth 回调

Query 参数：
- `code`：钉钉返回的授权码

### GET /api/v1/auth/dingtalk/config
获取钉钉配置（前端免登用）

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "corp_id": "xxx",
        "agent_id": "xxx"
    }
}
```

---

## JWT 认证

### Claims 结构

```go
type Claims struct {
    UserID   string `json:"user_id"`
    UserName string `json:"user_name"`
    jwt.RegisteredClaims
}
```

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

账号密码登录接口仍保留在后端和 `authAPI.login` 中，但当前登录页不再展示账号密码表单。

### 钉钉免登流程

`frontend/src/pages/Login.tsx` 中实现：

1. 通过 User-Agent 判断是否在钉钉内
2. 调用 `dd.runtime.permission.requestAuthCode()` 获取授权码
3. 调用 `/api/v1/auth/dingtalk/in-app` 换取 token
4. 存储到 `authStore`

### 认证状态管理

`frontend/src/store/authStore.ts`：

```tsx
interface AuthState {
    user: User | null;
    token: string;
    isLoggedIn: boolean;
    login: (user: User, token: string) => void;
    logout: () => void;
}

export const useAuthStore = create<AuthState>()(
    persist(
        (set) => ({
            user: null,
            token: '',
            isLoggedIn: false,
            login: (user, token) => set({ user, token, isLoggedIn: true }),
            logout: () => set({ user: null, token: '', isLoggedIn: false }),
        }),
        {
            name: 'peopleops-auth',
        }
    )
);
```

### API 拦截器

`frontend/src/services/api.ts`：

```tsx
// 请求拦截：自动加 token
api.interceptors.request.use((config) => {
    const { token } = useAuthStore.getState();
    if (token) {
        config.headers.Authorization = `Bearer ${token}`;
    }
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
- `DINGTALK_APP_KEY`：钉钉应用 Key
- `DINGTALK_APP_SECRET`：钉钉应用 Secret
- `DINGTALK_CORP_ID`：钉钉企业 ID
- `DINGTALK_AGENT_ID`：钉钉应用 Agent ID
- `DINGTALK_REDIRECT_URI`：OAuth 回调地址

---

## 常见问题

### 登录后 token 无效
- 检查 `JWT_SECRET` 是否一致
- 检查 token 是否过期
- 检查 `Authorization` header 格式是否正确（`Bearer <token>`）

### 钉钉扫码登录失败
- 检查 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`、`DINGTALK_CORP_ID`
- 检查钉钉应用权限
- 检查 `DINGTALK_REDIRECT_URI` 是否正确

### 钉钉内免登失败
- 检查是否在钉钉内打开（当前前端通过 User-Agent 是否包含 DingTalk 判断）
- 检查 `DINGTALK_AGENT_ID` 是否正确
- 检查钉钉应用权限（需要"获取用户信息"权限）

### 401 错误
- 检查 token 是否存在
- 检查 token 是否过期
- 检查 `Authorization` header 是否正确
- 检查后端 JWT 中间件是否正常工作
