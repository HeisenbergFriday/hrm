import { test, expect } from '@playwright/test';

const qrStartRoute = '**/api/v1/auth/dingtalk/qr/start';
const qrLoginUrl = 'https://login.dingtalk.test/qr?state=e2e';

test.describe('登录模块', () => {
  test('显示钉钉扫码登录入口', async ({ page }) => {
    await page.goto('/login?mode=scan');

    await expect(page.getByRole('heading', { name: '钉钉一体化人事后台' })).toBeVisible();
    await expect(page.getByText('当前将使用钉钉扫码登录')).toBeVisible();
    await expect(page.getByRole('button', { name: /打开钉钉官方扫码登录页/ })).toBeVisible();
  });

  test('点击扫码登录会请求后端并跳转到钉钉登录页', async ({ page }) => {
    let qrStartRequested = false;

    await page.route(qrStartRoute, async (route) => {
      qrStartRequested = true;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          message: 'success',
          data: {
            qr_code_url: qrLoginUrl,
            redirect_uri: 'http://localhost:3000/api/v1/auth/dingtalk/callback',
          },
        }),
      });
    });

    await page.route('https://login.dingtalk.test/**', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<!doctype html><title>DingTalk Login Mock</title><main>DingTalk QR Mock</main>',
      });
    });

    await page.goto('/login?mode=scan');
    await page.getByRole('button', { name: /打开钉钉官方扫码登录页/ }).click();

    await expect(page).toHaveURL(/login\.dingtalk\.test/);
    await expect(page.getByText('DingTalk QR Mock')).toBeVisible();
    expect(qrStartRequested).toBe(true);
  });

  test('后端未返回二维码地址时提示错误', async ({ page }) => {
    await page.route(qrStartRoute, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 200,
          message: 'success',
          data: {
            qr_code_url: '',
            redirect_uri: 'http://localhost:3000/api/v1/auth/dingtalk/callback',
          },
        }),
      });
    });

    await page.goto('/login?mode=scan');
    await page.getByRole('button', { name: /打开钉钉官方扫码登录页/ }).click();

    await expect(page.getByText('未获取到钉钉登录地址')).toBeVisible();
    await expect(page).toHaveURL(/\/login\?mode=scan$/);
  });
});
