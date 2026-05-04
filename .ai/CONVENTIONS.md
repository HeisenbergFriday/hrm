---
purpose: 编码规范、命名习惯、错误处理、测试规范
last_updated: 2026-04-30
source_of_truth:
  - 项目现有代码风格
  - internal/service/（Service 层示例）
  - internal/repository/（Repository 层示例）
  - frontend/src/pages/（前端组件示例）
update_when:
  - 修改命名规范时
  - 修改代码风格时
  - 修改错误处理方式时
  - 修改测试习惯时
  - 新增编码约定时
---

# 编码规范

## 后端规范（Go）

### 分层约定

```
Handler (api/)
    ↓ 调用
Service (service/)
    ↓ 调用
Repository (repository/)
    ↓ 调用
Model (database/models.go)
```

- **Handler**：处理 HTTP 请求，参数验证，调用 Service，返回 Response
- **Service**：业务逻辑，调用 Repository 和外部服务（钉钉）
- **Repository**：数据访问封装，GORM 操作
- **Model**：GORM 模型定义

### 命名规范

- **文件名**：小写 + 下划线，例如 `user_service.go`
- **包名**：小写，单数，例如 `package service`
- **结构体**：大驼峰，例如 `type UserService struct`
- **方法**：大驼峰（导出）或小驼峰（私有），例如 `func (s *UserService) GetUser()`
- **变量**：小驼峰，例如 `userID`
- **常量**：大驼峰或全大写，例如 `const MaxRetry = 3`

### Handler 规范

```go
func GetUser(c *gin.Context) {
    // 1. 参数验证
    id := c.Param("id")
    if id == "" {
        c.JSON(http.StatusBadRequest, Response{
            Code:    http.StatusBadRequest,
            Message: "id is required",
        })
        return
    }

    // 2. 调用 Service
    user, err := userService.GetUser(id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, Response{
            Code:    http.StatusInternalServerError,
            Message: err.Error(),
        })
        return
    }

    // 3. 返回 Response
    c.JSON(http.StatusOK, Response{
        Code:    http.StatusOK,
        Message: "success",
        Data:    user,
    })
}
```

### Service 规范

```go
type UserService struct {
    repo *repository.UserRepository
}

func NewUserService(repo *repository.UserRepository) *UserService {
    return &UserService{repo: repo}
}

func (s *UserService) GetUser(userID string) (*database.User, error) {
    // 业务逻辑
    user, err := s.repo.FindByUserID(userID)
    if err != nil {
        return nil, err
    }
    return user, nil
}
```

### Repository 规范

```go
type UserRepository struct {
    db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
    return &UserRepository{db: db}
}

func (r *UserRepository) FindByUserID(userID string) (*database.User, error) {
    var user database.User
    err := r.db.Where("user_id = ?", userID).First(&user).Error
    return &user, err
}
```

### 错误处理

- 使用 `errors.New()` 或 `fmt.Errorf()` 创建错误
- 不要吞掉错误，向上传递
- Handler 层统一处理错误，返回 Response

### 日志规范

```go
import "github.com/sirupsen/logrus"

logrus.Info("user logged in")
logrus.WithFields(logrus.Fields{
    "user_id": userID,
}).Error("failed to sync user")
```

---

## 前端规范（React + TypeScript）

### 文件命名

- **组件文件**：大驼峰 + `.tsx`，例如 `UserList.tsx`
- **工具文件**：小驼峰 + `.ts`，例如 `formatDate.ts`
- **样式文件**：小驼峰 + `.css`，例如 `userList.css`

### 组件规范

```tsx
import React from 'react';
import { Button } from 'antd';

interface UserListProps {
    users: User[];
    onSelect: (user: User) => void;
}

const UserList: React.FC<UserListProps> = ({ users, onSelect }) => {
    return (
        <div>
            {users.map(user => (
                <div key={user.id} onClick={() => onSelect(user)}>
                    {user.name}
                </div>
            ))}
        </div>
    );
};

export default UserList;
```

### API 调用规范

```tsx
import api from '@/services/api';

// 在组件中
const fetchUsers = async () => {
    try {
        const response = await api.get('/users');
        setUsers(response.data.data);
    } catch (error) {
        message.error('获取用户列表失败');
    }
};
```

### 状态管理规范

```tsx
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface AuthState {
    user: User | null;
    token: string | null;
    isLoggedIn: boolean;
    login: (user: User, token: string) => void;
    logout: () => void;
}

export const useAuthStore = create<AuthState>()(
    persist(
        (set) => ({
            user: null,
            token: null,
            isLoggedIn: false,
            login: (user, token) => set({ user, token, isLoggedIn: true }),
            logout: () => set({ user: null, token: null, isLoggedIn: false }),
        }),
        {
            name: 'peopleops-auth',
        }
    )
);
```

### 命名规范

- **组件**：大驼峰，例如 `UserList`
- **函数**：小驼峰，例如 `fetchUsers`
- **变量**：小驼峰，例如 `userList`
- **常量**：全大写 + 下划线，例如 `API_BASE_URL`
- **类型/接口**：大驼峰，例如 `interface User`

### 类型定义

```tsx
// 优先使用 interface
interface User {
    id: number;
    name: string;
    email: string;
}

// 复杂类型使用 type
type UserStatus = 'active' | 'inactive' | 'pending';
```

---

## 测试规范

### 后端测试（Go）

```go
func TestGetUser(t *testing.T) {
    // Setup
    db := setupTestDB()
    repo := repository.NewUserRepository(db)
    service := service.NewUserService(repo)

    // Test
    user, err := service.GetUser("test_user_id")

    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, user)
    assert.Equal(t, "test_user_id", user.UserID)
}
```

### 前端测试（Vitest）

```tsx
import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import UserList from './UserList';

describe('UserList', () => {
    it('renders user list', () => {
        const users = [
            { id: 1, name: 'Alice' },
            { id: 2, name: 'Bob' },
        ];
        render(<UserList users={users} onSelect={() => {}} />);
        expect(screen.getByText('Alice')).toBeInTheDocument();
        expect(screen.getByText('Bob')).toBeInTheDocument();
    });
});
```

---

## Git 提交规范

### Commit Message 格式

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Type 类型

- `feat`：新功能
- `fix`：Bug 修复
- `docs`：文档更新
- `style`：代码格式（不影响代码运行）
- `refactor`：重构
- `test`：测试相关
- `chore`：构建/工具相关

### 示例

```
feat(leave): 新增年假发放功能

- 新增 AnnualLeaveGrant 模型
- 新增季度发放接口
- 新增同步到钉钉功能

Closes #123
```

---

## 代码审查清单

### 后端

- [ ] Handler 是否有参数验证？
- [ ] 是否返回统一的 Response 格式？
- [ ] 错误是否正确处理和传递？
- [ ] 是否有必要的日志？
- [ ] 数据库操作是否在 Repository 层？
- [ ] 业务逻辑是否在 Service 层？
- [ ] 是否考虑了幂等性？
- [ ] 是否考虑了并发安全？

### 前端

- [ ] 组件是否有 TypeScript 类型定义？
- [ ] API 调用是否有错误处理？
- [ ] 是否使用了 Ant Design 组件？
- [ ] 是否有必要的 loading 状态？
- [ ] 是否有必要的错误提示？
- [ ] 是否考虑了空数据状态？

---

## 禁止行为

- 不要在 Handler 层直接操作数据库
- 不要在 Service 层直接返回 HTTP 响应
- 不要在 Repository 层写业务逻辑
- 不要硬编码配置，使用环境变量
- 不要提交敏感信息（密码、密钥）
- 不要提交 `node_modules`、`dist`、`.env`
- 不要修改 `.gitignore` 中的文件
- 不要在生产环境使用 `console.log`
- 不要在前端存储敏感信息（除了 token）
