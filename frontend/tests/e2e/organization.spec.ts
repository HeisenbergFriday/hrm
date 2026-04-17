import { test, expect } from '@playwright/test';

test.describe('组织架构模块', () => {
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

  test('获取部门树', async ({ page }) => {
    // 导航到部门树页面
    await page.goto('/organization/department-tree');
    
    // 检查页面是否显示
    await expect(page).toHaveTitle('People Ops 后台系统');
    await expect(page.locator('h1')).toContainText('部门树');
    
    // 等待部门树数据加载
    await expect(page.locator('.ant-tree-title')).toContainText('技术部');
  });

  test('同步部门数据', async ({ page }) => {
    // 导航到部门树页面
    await page.goto('/organization/department-tree');
    
    // 点击同步按钮
    await page.click('button:has-text("同步部门数据")');
    
    // 等待同步成功提示
    await expect(page.locator('.ant-message-success')).toBeVisible();
  });

  test('获取员工列表', async ({ page }) => {
    // 导航到员工列表页面
    await page.goto('/organization/employee-list');
    
    // 检查页面是否显示
    await expect(page).toHaveTitle('People Ops 后台系统');
    await expect(page.locator('h1')).toContainText('员工列表');
    
    // 等待员工列表数据加载
    await expect(page.locator('.ant-table-row')).toContainText('张三');
    await expect(page.locator('.ant-table-row')).toContainText('李四');
  });

  test('员工列表筛选', async ({ page }) => {
    // 导航到员工列表页面
    await page.goto('/organization/employee-list');
    
    // 输入筛选条件
    await page.fill('input[placeholder="搜索员工"]', '张三');
    
    // 等待筛选结果
    await expect(page.locator('.ant-table-row')).toContainText('张三');
    await expect(page.locator('.ant-table-row')).not.toContainText('李四');
  });

  test('查看员工详情', async ({ page }) => {
    // 导航到员工列表页面
    await page.goto('/organization/employee-list');
    
    // 点击查看详情按钮
    await page.click('button:has-text("查看")');
    
    // 等待详情页面加载
    await expect(page.locator('h1')).toContainText('员工详情');
    await expect(page.locator('.ant-descriptions-item-content')).toContainText('张三');
  });

  test('同步员工数据', async ({ page }) => {
    // 导航到员工列表页面
    await page.goto('/organization/employee-list');
    
    // 点击同步按钮
    await page.click('button:has-text("同步员工数据")');
    
    // 等待同步成功提示
    await expect(page.locator('.ant-message-success')).toBeVisible();
  });
});
