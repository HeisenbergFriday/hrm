# 绩效模块 E2E 测试

基于 Playwright，覆盖绩效核心用户路径。**所有后端接口均在用例内 mock**（`page.route('**/api/v1/**')`），
因此 e2e 不依赖真实后端、不依赖数据库，可在任意机器/CI 独立跑通。

## 一次性准备：安装浏览器依赖

首次运行或换机器时，需要安装 Playwright 浏览器（chromium/firefox/webkit）。
**不要硬编码本机路径**，统一用脚本安装到 Playwright 默认缓存：

```bash
cd frontend
npm install                # 安装 @playwright/test 等依赖
npm run test:e2e:install   # = playwright install --with-deps，下载三大浏览器
```

> `--with-deps` 会在 Linux/CI 上顺带安装系统依赖；Windows/macOS 本地会自动跳过系统依赖部分。
> 浏览器默认装到 `~/.cache/ms-playwright`（Win 为 `%LOCALAPPDATA%\ms-playwright`），与仓库解耦。

## 运行

```bash
cd frontend

# 默认：chromium + firefox + webkit 三浏览器全跑
npm run test:e2e

# 仅 chromium（本地快速回归用；CI 必须跑全三浏览器，不能用它声称 e2e 全绿）
npm run test:e2e:chromium

# 查看 HTML 报告
npm run test:e2e:report
```

### dev server 行为

`playwright.config.ts` 的 `webServer` 会**自动冷启动一个专用 Vite dev server**：

- 端口：`5273`（可用环境变量 `E2E_PORT` 覆盖），与本地 `npm run dev`（3000）隔离，避免互相干扰。
- `--strictPort`：端口被占用时直接报错，而非悄悄换端口导致测试连到错误页面。
- 默认不复用已有 server：每次 `npm run test:e2e` 都由 Playwright 冷启动 Vite，确保不是连到本机旧服务。

因此**无需手动先开 dev server**，直接 `npm run test:e2e` 即可。若端口被占用，请释放 5273 或用 `E2E_PORT` 临时指定其他端口。

## 覆盖的核心路径

| 用例 | 覆盖路径 |
|---|---|
| renders overview, validates activity form, imports and refreshes participants, and advances a stage | 进入绩效总览 → 新建活动表单校验 → 导入参与人 → 刷新参与人 → 推进活动阶段（开启目标设定）→ 进入目标设定页 |
| disables protected controls and blocks guarded routes without operation permission | 无权限用户：受控按钮禁用 + 受控路由被 RouteGuard 拦截 |
| edits goal records, saves a draft, loads suggestions, and submits target approval | 目标设定：加载建议 → 保存草稿 → 提交目标审批 |
| submits self evaluation and manager evaluation with mocked auto scoring | 员工自评提交 → 主管自动评分 → 主管评价提交 |
| confirms a performance result from the result page | 结果页员工确认 |

`warning-probe.spec.ts` 为诊断用例，探测 `useForm` 警告；默认配置会排除它，不计入主流程 E2E 结论。需要诊断时可临时调整 Playwright 配置单独运行，不要用诊断探针结果代替主流程通过结论。

## 测试数据

所有 fixture（活动、参与人、目标记录、分布检查等）定义在 `performance.spec.ts` 顶部，
通过 `page.route` 拦截返回，保证**稳定、可重复、与真实环境无关**。
