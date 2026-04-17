import { test, expect } from '@playwright/test';

test.describe('登录模块', () => {
  test('账号密码登录成功', async ({ page }) => {
    // 导航到登录页面
    await page.goto('/login');

    // 输入用户名和密码
    await page.fill('input[placeholder="用户名"]', 'admin');
    await page.fill('input[placeholder="密码"]', '123456');

    // 点击登录按钮
    await page.click('button:has-text("登录")');

    // 等待登录成功后跳转到首页
    await expect(page).toHaveURL('/');
    
    // 检查首页是否显示
    await expect(page).toHaveTitle('People Ops 后台系统');
  });

  test('账号密码登录失败', async ({ page }) => {
    // 导航到登录页面
    await page.goto('/login');

    // 输入错误的用户名和密码
    await page.fill('input[placeholder="用户名"]', 'wrong');
    await page.fill('input[placeholder="密码"]', 'wrong');

    // 点击登录按钮
    await page.click('button:has-text("登录")');

    // 等待错误提示
    await expect(page.locator('.ant-alert-error')).toBeVisible();
    await expect(page.locator('.ant-alert-error')).toContainText('用户名或密码错误');
  });

  test('钉钉登录', async ({ page }) => {
    // 导航到登录页面
    await page.goto('/login');

    // 点击钉钉登录按钮
    await page.click('button:has-text("钉钉登录")');

    // 等待登录成功后跳转到首页
    await expect(page).toHaveURL('/');
    
    // 检查首页是否显示
    await expect(page).toHaveTitle('People Ops 后台系统');
  });

  test('表单验证', async ({ page }) => {
    // 导航到登录页面
    await page.goto('/login');

    // 点击登录按钮，不输入任何内容
    await page.click('button:has-text("登录")');

    // 检查验证提示
    await expect(page.locator('.ant-form-item-explain-error')).toContainText('请输入用户名');
    await expect(page.locator('.ant-form-item-explain-error')).toContainText('请输入密码');
  });
});
