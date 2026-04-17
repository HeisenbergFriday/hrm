import { test, expect } from '@playwright/test';

test.describe('考勤模块', () => {
  // 登录前置操作
  test.beforeEach(async ({ page }) => {
    // 导航到登录页面
    await page.goto('/login');
    
    // 输入用户名和密码
    await page.fill('input[placeholder="用户名"]', 'admin');
    await page.fill('input[placeholder="密码"]', '123456');
    
    // 点击登录按钮
    await page.click('button:has-text("登录")');
    
    // 等待登录成功后跳转到首页
    await expect(page).toHaveURL('/');
  });

  test('获取考勤记录', async ({ page }) => {
    // 导航到考勤记录页面
    await page.goto('/attendance');
    
    // 检查页面是否显示
    await expect(page).toHaveTitle('People Ops 后台系统');
    await expect(page.locator('h1')).toContainText('考勤记录');
    
    // 等待考勤记录数据加载
    await expect(page.locator('.ant-table-row')).toContainText('张三');
    await expect(page.locator('.ant-table-row')).toContainText('上班');
    await expect(page.locator('.ant-table-row')).toContainText('下班');
  });

  test('考勤记录筛选', async ({ page }) => {
    // 导航到考勤记录页面
    await page.goto('/attendance');
    
    // 输入筛选条件
    await page.fill('input[placeholder="开始日期"]', '2024-01-01');
    await page.fill('input[placeholder="结束日期"]', '2024-01-31');
    
    // 点击查询按钮
    await page.click('button:has-text("查询")');
    
    // 等待筛选结果
    await expect(page.locator('.ant-table-row')).toContainText('张三');
  });

  test('同步考勤数据', async ({ page }) => {
    // 导航到考勤记录页面
    await page.goto('/attendance');
    
    // 点击同步按钮
    await page.click('button:has-text("同步考勤数据")');
    
    // 等待同步成功提示
    await expect(page.locator('.ant-message-success')).toBeVisible();
  });

  test('导出考勤数据', async ({ page }) => {
    // 导航到考勤记录页面
    await page.goto('/attendance');
    
    // 点击导出按钮
    await page.click('button:has-text("导出")');
    
    // 等待导出成功提示
    await expect(page.locator('.ant-message-success')).toBeVisible();
  });

  test('获取考勤统计', async ({ page }) => {
    // 导航到考勤统计页面
    await page.goto('/attendance/stats');
    
    // 检查页面是否显示
    await expect(page).toHaveTitle('People Ops 后台系统');
    await expect(page.locator('h1')).toContainText('考勤统计');
    
    // 等待统计数据加载
    await expect(page.locator('.ant-card-body')).toContainText('正常天数');
    await expect(page.locator('.ant-card-body')).toContainText('迟到天数');
    await expect(page.locator('.ant-card-body')).toContainText('缺勤天数');
    await expect(page.locator('.ant-card-body')).toContainText('加班小时');
  });

  test('考勤记录为空', async ({ page }) => {
    // 导航到考勤记录页面
    await page.goto('/attendance');
    
    // 输入筛选条件，查询不存在的数据
    await page.fill('input[placeholder="开始日期"]', '2023-01-01');
    await page.fill('input[placeholder="结束日期"]', '2023-01-31');
    
    // 点击查询按钮
    await page.click('button:has-text("查询")');
    
    // 等待空数据提示
    await expect(page.locator('.ant-empty')).toBeVisible();
    await expect(page.locator('.ant-empty')).toContainText('暂无数据');
  });
});
