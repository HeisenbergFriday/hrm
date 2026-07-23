import { defineConfig, devices } from '@playwright/test';

const chromiumProject = {
  name: 'chromium',
  use: { ...devices['Desktop Chrome'] },
};

const crossBrowserProjects = [
  chromiumProject,
  {
    name: 'firefox',
    use: { ...devices['Desktop Firefox'] },
  },
  {
    name: 'webkit',
    use: { ...devices['Desktop Safari'] },
  },
];

/**
 * e2e 使用专用端口，避免与本地 `npm run dev`（3000）冲突，
 * 并保证每次都冷启动一个干净的 Vite dev server 来执行测试。
 * 所有后端请求由各用例内的 page.route('**\/api/v1/**') mock，
 * 因此 e2e 不依赖真实后端环境。
 */
const E2E_PORT = Number(process.env.E2E_PORT) || 5273
const E2E_BASE_URL = `http://localhost:${E2E_PORT}`

/**
 * @see https://playwright.dev/docs/test-configuration
 */
export default defineConfig({
  testDir: './tests/e2e',
  /* Run tests in files in parallel */
  fullyParallel: true,
  /* Fail the build on CI if you accidentally left test.only in the source code. */
  forbidOnly: !!process.env.CI,
  /* Retry on CI only */
  retries: process.env.CI ? 2 : 0,
  /* Opt out of parallel tests on CI. */
  workers: process.env.CI ? 1 : undefined,
  /* Reporter to use. See https://playwright.dev/docs/test-reporters */
  reporter: [
    ['html', { outputFolder: 'tests/reports/html', open: 'never' }],
    ['json', { outputFile: 'tests/reports/playwright-report.json' }]
  ],
  /* Shared settings for all the projects below. See https://playwright.dev/docs/api/class-testoptions. */
  use: {
    /* Base URL to use in actions like `await page.goto('/')`. */
    baseURL: E2E_BASE_URL,

    /* Collect trace when retrying the failed test. See https://playwright.dev/docs/trace-viewer */
    trace: 'on-first-retry',
  },

  /* Configure projects for major browsers */
  projects: crossBrowserProjects,

  /*
   * Start a dedicated Vite dev server on E2E_PORT before running the tests.
   * 使用 --strictPort 确保端口被占用时直接报错，而不是悄悄换端口导致测试连到错误页面。
   * 默认不复用已有 server，保证 `npm run test:e2e` 会由 Playwright 冷启动 Vite。
   */
  webServer: {
    command: `npm run dev -- --port ${E2E_PORT} --strictPort`,
    url: E2E_BASE_URL,
    reuseExistingServer: process.env.PLAYWRIGHT_REUSE_SERVER === '1',
    timeout: 120_000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
