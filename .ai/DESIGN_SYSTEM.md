---
purpose: UI 设计规范约束，包括组件库、样式、布局、交互、命名、无障碍规范
last_updated: 2026-05-25
source_of_truth:
  - Ant Design 官方文档
  - 项目现有代码风格
update_when:
  - 更换 UI 组件库时
  - 修改视觉规范时
  - 新增设计约定时
  - 修改交互规范时
---

# 设计规范

## 重要提醒

**本文档是日常开发时的必读文档。**

每次描述需求后、开始写代码前，必须读取本文档，确保 UI 实现符合设计规范。

---

## 1. 组件库规范

### 1.1 基础组件库

- **版本**：Ant Design 5.x (antd@^5.12.8)
- **图标库**：@ant-design/icons@^5.2.6
- **React**：18.x

### 1.2 组件使用原则

1. **优先使用 antd 内置组件**
   - 不要自行实现已有 antd 组件
   - 不要使用其他 UI 库的组件

2. **保持一致性**
   - 同一功能使用相同的组件
   - 相同的数据结构使用相同的展示方式

3. **最小化自定义**
   - 优先使用组件的 props 配置
   - 只在必要时使用自定义样式

### 1.3 常用组件映射

| 场景 | 推荐组件 | 说明 |
|---|---|---|
| 数据表格 | `<Table>` | 支持排序、筛选、分页 |
| 数据列表 | `<List>` | 简单列表展示 |
| 数据卡片 | `<Card>` | 内容分组 |
| 搜索筛选 | `<Form>` + `<Input>` / `<Select>` | 统一表单体验 |
| 操作按钮 | `<Button>` | 主操作用 `type="primary"` |
| 状态标签 | `<Tag>` / `<Badge>` | 状态展示 |
| 弹窗确认 | `<Modal.confirm>` | 危险操作确认 |
| 消息提示 | `message` / `notification` | 轻量提示 |
| 加载状态 | `<Spin>` / `<Skeleton>` | 内容加载中 |
| 空状态 | `<Empty>` | 无数据展示 |
| 进度展示 | `<Progress>` / `<Steps>` | 流程进度 |
| 数据输入 | `<Input>` / `<InputNumber>` | 文本/数字输入 |
| 日期选择 | `<DatePicker>` / `<RangePicker>` | 日期/时间选择 |
| 文件上传 | `<Upload>` | 文件上传 |

### 1.4 组件复用

当多个页面需要相同组件时：
1. 优先在 `frontend/src/components/` 下创建通用组件
2. 组件命名：大驼峰 + 语义化，如 `StatusTag`、`PageCard`
3. 组件必须有 TypeScript 类型定义

---

## 2. 样式规范

### 2.1 颜色规范

#### 主色
- **Primary**：`#1677ff`（antd 默认蓝）
- **Success**：`#52c41a`
- **Warning**：`#faad14`
- **Error**：`#ff4d4f`
- **Info**：`#1677ff`

#### 中性色
- **Text Primary**：`rgba(0, 0, 0, 0.88)`
- **Text Secondary**：`rgba(0, 0, 0, 0.65)`
- **Text Tertiary**：`rgba(0, 0, 0, 0.45)`
- **Border**：`#d9d9d9`
- **Background**：`#fafafa`
- **White**：`#ffffff`

#### 语义色使用
- 成功/完成：`color: 'success'` 或 `color="green"`
- 警告/待处理：`color: 'warning'` 或 `color="orange"`
- 错误/失败：`color: 'error'` 或 `color="red"`
- 信息/默认：`color: 'default'` 或不指定

### 2.2 字体规范

#### 字体族
```css
font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial,
  'Noto Sans', sans-serif, 'Apple Color Emoji', 'Segoe UI Emoji', 'Segoe UI Symbol',
  'Noto Color Emoji';
```

#### 字号规范
| 场景 | 字号 | 行高 | 用途 |
|---|---|---|---|
| 标题 H1 | 24px | 32px | 页面主标题 |
| 标题 H2 | 20px | 28px | 区块标题 |
| 标题 H3 | 16px | 24px | 卡片标题 |
| 正文 | 14px | 22px | 默认文本 |
| 辅助文字 | 12px | 20px | 提示、说明文字 |

#### 字重
- **Regular**：400（正文）
- **Medium**：500（强调）
- **Semibold**：600（标题）

### 2.3 间距规范

#### 基础间距单位
- **基础单元**：8px
- **常用间距**：4px、8px、12px、16px、20px、24px、32px、40px、48px

#### 间距使用
| 场景 | 间距 |
|---|---|
| 表单元素间距 | 16px 或 24px |
| 卡片内部内边距 | 24px |
| 卡片之间间距 | 16px 或 24px |
| 页面标题与内容间距 | 16px 或 24px |
| 表格行间距 | 由 Table 组件控制 |
| 按钮组间距 | 8px |

### 2.4 圆角规范

| 元素 | 圆角 |
|---|---|
| 按钮 | 6px（antd 默认） |
| 输入框 | 6px（antd 默认） |
| 卡片 | 8px |
| 模态框 | 8px |
| 标签 | 4px |
| 头像 | 50%（圆形） |

### 2.5 阴影规范

| 场景 | 阴影值 |
|---|---|
| 卡片 | `0 1px 2px 0 rgba(0, 0, 0, 0.03), 0 1px 6px -1px rgba(0, 0, 0, 0.02), 0 2px 4px 0 rgba(0, 0, 0, 0.02)` |
| 下拉菜单 | `0 6px 16px 0 rgba(0, 0, 0, 0.08), 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 9px 28px 8px rgba(0, 0, 0, 0.05)` |
| 模态框 | `0 6px 16px 0 rgba(0, 0, 0, 0.08), 0 3px 6px -4px rgba(0, 0, 0, 0.12), 0 9px 28px 8px rgba(0, 0, 0, 0.05)` |
| 按钮悬浮 | `0 2px 0 rgba(0, 0, 0, 0.02)` |

---

## 3. 布局规范

### 3.1 页面布局

#### 标准页面结构
```
┌─────────────────────────────────────┐
│  页面标题 + 副标题                    │
├─────────────────────────────────────┤
│  筛选栏 / 操作栏                     │
├─────────────────────────────────────┤
│  主内容区（表格 / 卡片 / 详情）       │
├─────────────────────────────────────┤
│  分页器                              │
└─────────────────────────────────────┘
```

#### 布局组件
- **页面容器**：使用 `PageContainer` 或 `<Card>` 包裹
- **筛选栏**：使用 `<Form layout="inline">` 或 `<Space>`
- **表格区**：使用 `<Table>`
- **分页**：使用 `<Pagination>` 或 Table 内置分页

### 3.2 响应式断点

| 断点 | 宽度 | 设备 |
|---|---|---|
| xs | < 576px | 手机竖屏 |
| sm | ≥ 576px | 手机横屏 |
| md | ≥ 768px | 平板 |
| lg | ≥ 992px | 小桌面 |
| xl | ≥ 1200px | 桌面 |
| xxl | ≥ 1600px | 大桌面 |

### 3.3 栅格系统

使用 antd 的 24 栏栅格系统：

```tsx
<Row gutter={[16, 16]}>
  <Col xs={24} sm={12} lg={8}>
    {/* 内容 */}
  </Col>
  <Col xs={24} sm={12} lg={8}>
    {/* 内容 */}
  </Col>
</Row>
```

### 3.4 页面宽度

- **最大宽度**：不限制（全屏）
- **最小宽度**：1200px（保证表格可用性）
- **内容区宽度**：使用 `<Layout>` 或 `<Space direction="vertical">` 自适应

---

## 4. 交互规范

### 4.1 表单校验

#### 校验时机
- **失焦校验**：`<Form>` 设置 `onBlur` 触发
- **提交校验**：表单提交时统一校验

#### 校验规则
```tsx
<Form.Item
  name="username"
  label="用户名"
  rules={[
    { required: true, message: '请输入用户名' },
    { min: 2, max: 20, message: '用户名长度 2-20 位' },
    { pattern: /^[a-zA-Z0-9_]+$/, message: '用户名只能包含字母、数字、下划线' }
  ]}
>
  <Input />
</Form.Item>
```

#### 校验提示
- 位置：输入框下方
- 颜色：`#ff4d4f`（error 色）
- 图标：antd 自带校验图标

### 4.2 加载状态

#### 场景与方式
| 场景 | 组件 | 示例 |
|---|---|---|
| 页面加载 | `<Spin>` 全屏 | `loading={isLoading}` |
| 表格加载 | `<Table loading>` | `loading={tableLoading}` |
| 按钮提交 | `<Button loading>` | `loading={submitting}` |
| 内容区加载 | `<Skeleton>` | 骨架屏 |
| 列表加载 | `<List loading>` | 列表项骨架 |

#### 加载文案
- 首次加载：`加载中...`
- 提交中：`提交中...`
- 保存中：`保存中...`
- 同步中：`同步中...`

### 4.3 错误处理

#### 前端错误提示
```tsx
// 轻量提示（2秒自动消失）
message.success('操作成功');
message.error('操作失败');
message.warning('请先完成');
message.info('提示信息');

// 通知提示（需要手动关闭）
notification.success({ message: '成功', description: '...' });
notification.error({ message: '失败', description: '...' });
```

#### 错误信息规范
- **成功**：`操作成功`、`保存成功`、`删除成功`、`同步成功`
- **失败**：`操作失败`、`保存失败`、`网络异常，请重试`
- **警告**：`请先选择`、`请填写完整`、`确认删除？`
- **信息**：`提示信息`

#### 空状态
```tsx
<Empty description="暂无数据" />
// 或自定义
<Empty description={<span>暂无考勤记录</span>}>
  <Button type="primary">去打卡</Button>
</Empty>
```

### 4.4 确认弹窗

#### 危险操作确认
```tsx
Modal.confirm({
  title: '确认删除',
  content: '删除后不可恢复，确定要删除吗？',
  okText: '确定',
  cancelText: '取消',
  okType: 'danger',
  onOk: () => handleDelete(),
});
```

#### 确认文案规范
- **删除**：`确认删除？删除后不可恢复。`
- **锁定**：`确认锁定？锁定后不可修改。`
- **审批**：`确认通过 / 驳回？`
- **同步**：`确认同步到钉钉？`

### 4.5 Toast/通知规范

| 类型 | 方法 | 场景 |
|---|---|---|
| 成功 | `message.success()` | 操作成功反馈 |
| 警告 | `message.warning()` | 需要注意的情况 |
| 错误 | `message.error()` | 操作失败 |
| 信息 | `message.info()` | 一般提示 |
| 通知 | `notification.info()` | 需要详细说明 |

---

## 5. 命名规范

### 5.1 组件命名

- **文件名**：大驼峰，如 `EmployeeList.tsx`、`AttendanceStats.tsx`
- **组件名**：大驼峰，与文件名一致
- **props 接口**：大驼峰 + `Props` 后缀，如 `EmployeeListProps`

### 5.2 CSS/样式命名

- **文件名**：小驼峰，如 `pageCard.css`、`statusTag.css`
- **类名**：小驼峰，如 `pageCard`、`statusTag`
- **避免**：BEM 命名（antd 不使用）

### 5.3 组件目录结构

```
frontend/src/
├── components/          # 通用组件
│   ├── StatusTag.tsx
│   ├── StatusTag.css
│   ├── PageCard.tsx
│   └── PageCard.css
├── pages/               # 页面组件
│   ├── Attendance.tsx
│   └── Attendance.css
└── styles/              # 全局样式（如有）
```

---

## 6. 无障碍规范

### 6.1 基本要求

1. **键盘导航**
   - 所有交互元素可通过 Tab 键访问
   - 按钮、链接可通过 Enter/Space 触发
   - 下拉菜单可通过 Esc 关闭

2. **屏幕阅读器**
   - 图标按钮必须有 `aria-label`
   - 图片必须有 `alt` 属性
   - 表单必须有 `label`

3. **颜色对比**
   - 文字与背景对比度 ≥ 4.5:1
   - 不仅依赖颜色传达信息

### 6.2 实现示例

```tsx
// 图标按钮
<Button icon={<DeleteOutlined />} aria-label="删除" />

// 表单标签
<Form.Item label="用户名" name="username">
  <Input />
</Form.Item>

// 图片
<img src="avatar.png" alt="用户头像" />

// 状态标签（颜色+文字）
<Tag color="success">已完成</Tag>  // ✅ 正确：颜色+文字
<Tag color="success" />  // ❌ 错误：仅颜色
```

### 6.3 antd 无障碍支持

antd 组件默认支持无障碍访问：
- `<Button>` 自动支持键盘操作
- `<Modal>` 自动管理焦点
- `<Form>` 自动关联 label
- `<Table>` 支持键盘导航

---

## 7. 页面原型流程

### 7.1 标准流程

1. **需求描述**：用户说明页面需求
2. **文字描述**：AI 输出页面结构描述
3. **用户确认**：确认描述无误
4. **编写代码**：按描述实现页面

### 7.2 文字描述格式

```text
页面标题：<标题>
副标题：<副标题>

顶部：筛选栏
- <筛选项 1>（必选/可选）
- <筛选项 2>（必选/可选）
- 搜索按钮

主体：表格/卡片/详情
列：<列名 1>、<列名 2>、<列名 3>
- 状态用 StatusTag：<状态 1>=<颜色>、<状态 2>=<颜色>

底部：分页器
```

### 7.3 状态标签颜色映射

| 状态 | 颜色 | antd tag color |
|---|---|---|
| 成功/完成/正常/在职 | 绿色 | `success` |
| 进行中/处理中/待审批 | 蓝色 | `processing` |
| 警告/待处理/即将到期 | 橙色 | `warning` |
| 错误/失败/异常/离职 | 红色 | `error` |
| 默认/草稿/未开始 | 灰色 | `default` |

---

## 8. 代码审查清单

### 8.1 组件使用

- [ ] 是否优先使用 antd 组件？
- [ ] 是否避免了重复实现已有组件？
- [ ] 组件 props 是否使用了类型定义？

### 8.2 样式规范

- [ ] 是否使用了规范的颜色？
- [ ] 字号是否符合规范？
- [ ] 间距是否使用了 8px 基础单位？
- [ ] 圆角是否符合规范？

### 8.3 交互规范

- [ ] 表单是否有校验规则？
- [ ] 加载状态是否正确展示？
- [ ] 错误提示是否友好？
- [ ] 危险操作是否有确认弹窗？

### 8.4 无障碍

- [ ] 图标按钮是否有 aria-label？
- [ ] 表单是否有 label？
- [ ] 颜色是否不仅用于传达信息？

---

## 9. 禁止行为

- 不要使用其他 UI 库的组件（如 Material UI、Element UI）
- 不要硬编码颜色值，优先使用 antd token 或语义色
- 不要使用内联样式（除非动态计算）
- 不要忽略表单校验
- 不要忽略加载状态
- 不要忽略错误处理
- 不要使用非语义化的 HTML 标签
- 不要忘记无障碍属性（aria-label、alt、label）
