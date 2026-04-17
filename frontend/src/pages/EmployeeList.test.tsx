import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter } from 'react-router-dom';
import { useAuthStore } from '../store/authStore';
import EmployeeList from './EmployeeList';

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

describe('EmployeeList Component', () => {
  beforeEach(() => {
    // 清除localStorage的mock
    localStorage.getItem = jest.fn();
    localStorage.setItem = jest.fn();
    localStorage.removeItem = jest.fn();
  });

  test('渲染员工列表页面', () => {
    renderWithProviders(<EmployeeList />);
    
    // 检查页面是否包含员工列表标题
    expect(screen.getByText('员工列表')).toBeInTheDocument();
  });

  test('员工列表数据加载成功', async () => {
    renderWithProviders(<EmployeeList />);
    
    // 等待员工列表数据加载
    await waitFor(() => {
      expect(screen.getByText('张三')).toBeInTheDocument();
      expect(screen.getByText('李四')).toBeInTheDocument();
    });
  });

  test('员工列表分页功能', async () => {
    renderWithProviders(<EmployeeList />);
    
    // 等待员工列表数据加载
    await waitFor(() => {
      expect(screen.getByText('张三')).toBeInTheDocument();
    });
    
    // 检查分页组件是否存在
    expect(screen.getByText('1')).toBeInTheDocument();
  });

  test('员工列表筛选功能', async () => {
    renderWithProviders(<EmployeeList />);
    
    // 等待员工列表数据加载
    await waitFor(() => {
      expect(screen.getByText('张三')).toBeInTheDocument();
    });
    
    // 输入筛选条件
    const searchInput = screen.getByPlaceholderText('搜索员工');
    fireEvent.change(searchInput, { target: { value: '张三' } });
    
    // 等待筛选结果
    await waitFor(() => {
      expect(screen.getByText('张三')).toBeInTheDocument();
      expect(screen.queryByText('李四')).not.toBeInTheDocument();
    });
  });

  test('员工详情查看', async () => {
    renderWithProviders(<EmployeeList />);
    
    // 等待员工列表数据加载
    await waitFor(() => {
      expect(screen.getByText('张三')).toBeInTheDocument();
    });
    
    // 点击查看详情按钮
    const detailButton = screen.getByText('查看');
    fireEvent.click(detailButton);
    
    // 等待详情页面加载
    await waitFor(() => {
      expect(screen.getByText('员工详情')).toBeInTheDocument();
    });
  });

  test('同步员工数据', async () => {
    renderWithProviders(<EmployeeList />);
    
    // 检查同步按钮是否存在
    const syncButton = screen.getByText('同步员工数据');
    expect(syncButton).toBeInTheDocument();
    
    // 点击同步按钮
    syncButton.click();
    
    // 等待同步完成
    await waitFor(() => {
      expect(screen.getByText('同步成功')).toBeInTheDocument();
    });
  });

  test('同步员工数据失败', async () => {
    renderWithProviders(<EmployeeList />);
    
    // 检查同步按钮是否存在
    const syncButton = screen.getByText('同步员工数据');
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