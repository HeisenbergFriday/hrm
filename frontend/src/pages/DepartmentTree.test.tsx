import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter } from 'react-router-dom';
import { useAuthStore } from '../store/authStore';
import DepartmentTree from './DepartmentTree';

// 创建QueryClient实例
const queryClient = new QueryClient();

// 包装组件，提供必要的上下文
const renderWithProviders = (ui: React.ReactElement) => {
  // 模拟认证状态
  useAuthStore.setState({ token: 'mock-token', user: { id: 'user123', name: '张三' } });
  
  return render(
    <BrowserRouter>
      <QueryClientProvider client={queryClient}>
        {ui}
      </QueryClientProvider>
    </BrowserRouter>
  );
};

describe('DepartmentTree Component', () => {
  beforeEach(() => {
    // 清除localStorage的mock
    localStorage.getItem = jest.fn();
    localStorage.setItem = jest.fn();
    localStorage.removeItem = jest.fn();
  });

  test('渲染部门树页面', () => {
    renderWithProviders(<DepartmentTree />);
    
    // 检查页面是否包含部门树标题
    expect(screen.getByText('部门树')).toBeInTheDocument();
  });

  test('部门树数据加载成功', async () => {
    renderWithProviders(<DepartmentTree />);
    
    // 等待部门树数据加载
    await waitFor(() => {
      expect(screen.getByText('技术部')).toBeInTheDocument();
      expect(screen.getByText('前端组')).toBeInTheDocument();
      expect(screen.getByText('后端组')).toBeInTheDocument();
    });
  });

  test('部门树节点点击', async () => {
    renderWithProviders(<DepartmentTree />);
    
    // 等待部门树数据加载
    await waitFor(() => {
      expect(screen.getByText('技术部')).toBeInTheDocument();
    });
    
    // 点击技术部节点
    const techDepartment = screen.getByText('技术部');
    techDepartment.click();
    
    // 检查子节点是否展开
    await waitFor(() => {
      expect(screen.getByText('前端组')).toBeInTheDocument();
      expect(screen.getByText('后端组')).toBeInTheDocument();
    });
  });

  test('同步部门数据', async () => {
    renderWithProviders(<DepartmentTree />);
    
    // 检查同步按钮是否存在
    const syncButton = screen.getByText('同步部门数据');
    expect(syncButton).toBeInTheDocument();
    
    // 点击同步按钮
    syncButton.click();
    
    // 等待同步完成
    await waitFor(() => {
      expect(screen.getByText('同步成功')).toBeInTheDocument();
    });
  });

  test('同步部门数据失败', async () => {
    renderWithProviders(<DepartmentTree />);
    
    // 检查同步按钮是否存在
    const syncButton = screen.getByText('同步部门数据');
    expect(syncButton).toBeInTheDocument();
    
    // 模拟同步失败
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ code: 500, message: '同步失败' })
    });
    
    // 点击同步按钮
    syncButton.click();
    
    // 等待错误提示
    await waitFor(() => {
      expect(screen.getByText('同步失败')).toBeInTheDocument();
    });
  });
});