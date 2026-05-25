# Cerebrum

> OpenWolf's learning memory. Updated automatically as the AI learns from interactions.
> Do not edit manually unless correcting an error.
> Last updated: 2026-05-25

## User Preferences

<!-- How the user likes things done. Code style, tools, patterns, communication. -->

## Key Learnings

- 前端样式统一使用 CSS 变量（Design Token），定义在 `frontend/src/index.css` 的 `:root` 中。禁止在 TSX 中硬编码颜色值、像素值、圆角值等。必须引用 `var(--xxx)` 令牌。
- Ant Design 的 ConfigProvider theme 中的颜色值需要与 CSS 变量保持同步，但因为是 JS 对象不能直接引用 CSS 变量，所以手动保持一致。
- 页面背景色：`--color-bg-page: #e4e8ee`，卡片背景：`--color-bg-card: #fff`，表头背景：`--color-bg-card-header: #fafbfc`
- 主色系：primary `#4338ca`、hover `#6366f1`、active `#3730a3`、light `#818cf8`、bg `#eef2ff`
- 圆角规范：xs=4, sm=6, md=8, lg=12, xl=14, 2xl=16，对应不同层级的容器
- 页面必须使用 PageContainer 包裹（提供背景、padding、标题），不要手写根 div 样式
- 卡片必须使用 PageCard 包裹（提供统一的 borderRadius、border、boxShadow），不要手写 Card 样式
- 状态标签使用 StatusTag（封装 Tag + borderRadius:6 + fontWeight:600），不要手写 Tag 内联样式
- Ant Design Button 默认 borderRadius:8、fontWeight:600 已在 App.tsx 主题中配置，按钮不需要内联这些值
- Ant Design Tag 默认 borderRadiusSM:6 已在主题中配置
- 全站 37 个页面已完成 Design Token + 组件规范替换（2026-05-25），Login/Callback 为全屏居中布局不适用标准模式
- 批量替换页面时使用并行 agent 可大幅提速，每个 agent 负责 3-6 个文件

## Layout Rules

- **导航栏层级**：如果页面有 sticky 导航栏，必须分两行——第一行放返回按钮+标题+姓名，第二行放状态摘要+操作按钮。禁止所有内容挤一行。
- **表格列宽**：表单型表格必须给**所有列**设明确的 width（可以用百分比 `'40%'` 或固定像素 `140`），不要留空让 Ant Design 自动分配。否则剩余空间平分会导致宽列独占大量空白，输入框只占一小半，视觉失衡。推荐比例：名称列 35-40%，描述列 30%，数值列 12-16%，操作列 48px。
- **展开行栅格**：展开行内多个字段用 `display: grid; gridTemplateColumns: 1fr 1fr 1fr 1fr; gap: 16` 四等分。标签用 `fontSize: xs, color: secondary, fontWeight: medium, marginBottom: 6`。
- **区块间距**：PageCard 之间用 `marginTop: 24`（不是 16），保持区块感。
- **"添加"按钮**：表格下方的 dashed 按钮用 `marginTop: 12`（不是 8），和表格保持适当距离。
- **表单标签**：展开行/表单内的字段标签统一用 `fontSize: var(--font-size-xs), color: var(--color-text-secondary), fontWeight: var(--font-weight-medium)`，不要用 `type="secondary"`（样式不够精确）。

## Do-Not-Repeat

<!-- Mistakes made and corrected. Each entry prevents the same mistake recurring. -->
<!-- Format: [YYYY-MM-DD] Description of what went wrong and what to do instead. -->

## Decision Log

<!-- Significant technical decisions with rationale. Why X was chosen over Y. -->
