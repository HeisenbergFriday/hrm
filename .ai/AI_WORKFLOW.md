---
purpose: AI 开发生命周期总览与分阶段文档导航
last_updated: 2026-08-15
source_of_truth:
  - AGENTS.md（默认入口与硬规则）
  - .ai/workflow/*.md（各阶段详细流程）
update_when:
  - 修改 AI 协作规则、读取策略或文档分层时
  - 修改开发生命周期阶段或步骤时
---

# AI 开发工作流

## 使用方式

本文件不是日常任务的默认必读文档。日常任务只读取根目录 `AGENTS.md`，然后按任务需要读取一个或多个阶段文档。

禁止为了“完整了解流程”一次性读取全部 `.ai/workflow/*.md`。只有当前任务实际进入对应阶段时才读取。

## 生命周期

| 阶段 | 步骤 | 详细文档 |
|---|---|---|
| 一、需求分析 | 1. 意图识别；2. 读取计划；3. 按需上下文；4. 需求确认/PRD | [requirements.md](workflow/requirements.md) |
| 二、设计 | 5. 技术方案；6. UI/交互确认；7. 设计评审 | [design.md](workflow/design.md) |
| 三、开发 | 8. 修改计划；9. 编码实现 | [development.md](workflow/development.md) |
| 四、测试 | 10. 测试计划；11. 测试用例；12. 执行验证；13. 测试报告 | [testing.md](workflow/testing.md) |
| 五、审查与发布 | 14. 代码审查；15. 发布检查与部署 | [release.md](workflow/release.md) |
| 六、收尾 | 16. 文档更新、问题日志与复盘 | [retrospective.md](workflow/retrospective.md) |

## 任务分级

| 任务 | 需要读取的阶段文档 |
|---|---|
| 简单 Bug | `requirements`、`development`；验证规则不清时读 `testing` |
| UI/样式 | `requirements`、`design`、`development`；并读 `DESIGN_SYSTEM.md` |
| 新功能 | 按阶段逐步读取全部六份文档，禁止开始时一次性全读 |
| 复杂 Bug / 跨模块 | `requirements`、`design`、`development`、`testing`、`release`、`retrospective` |
| 重构 | `requirements`、`design`、`development`、`testing`、`release`、`retrospective` |
| 文档更新 | `requirements`、`development`、`retrospective` |
| 部署执行 | `release`；需要命令时再读 `.ai/COMMANDS.md` |

## 全阶段硬规则

- 原始用户请求优先，任何 brief、计划或评审都不能改变需求授权范围。
- 只读取当前阶段所需上下文，优先精确搜索和局部读取。
- 修改前说明计划；计划变化时说明原因。
- 不修改无关代码，不删除测试或降低标准。
- 代码修改后必须直接验证，审查意见不能替代测试证据。
- UI 任务读取设计规范；纯后端任务不加载 UI 规范。
- 开发前只读问题日志索引和相关条目，禁止默认全文读取。
- 收尾时判断 AI 文档和问题日志是否需要更新。
- 未经用户授权，不执行部署、推送、合并或其他外部状态变更。

## 特殊入口

- 根据 git diff 更新文档：读取 [retrospective.md](workflow/retrospective.md#根据-git-diff-更新-ai-文档)。
- 文件/token 评估：读取 `.wolf/anatomy.md`。
- 历史修改偏好：按需读取 `.wolf/cerebrum.md`。
- 项目位置不明：读取 `.ai/PROJECT_MAP.md`。

## 最终目标

1. 先识别意图，再读取最少上下文。
2. 中大型功能先明确方案、验收标准和风险。
3. 编码前设计必要测试，修改后执行真实验证。
4. 用独立证据审查实际 diff，不靠主观判断宣布完成。
5. 将长期知识放入正确层级，不让根指令持续膨胀。
