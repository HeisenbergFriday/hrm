import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter } from 'react-router-dom';
import { useAuthStore } from '../store/authStore';
import Attendance from './Attendance';

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

describe('Attendance Component', () => {
  beforeEach(() => {
    // 清除localStorage的mock
    localStorage.getItem = jest.fn();
    localStorage.setItem = jest.fn();
    localStorage.removeItem = jest.fn();
  });

  test('渲染考勤页面', () => {
    renderWithProviders(<Attendance />);
    
    // 检查页面是否包含考勤标题
    expect(screen.getByText('考勤记录')).toBeInTheDocument();
  });

  test('考勤记录数据加载成功', async () => {
    renderWithProviders(<Attendance />);
    
    // 等待考勤记录数据加载
    await waitFor(() => {
      expect(screen.getByText('张三')).toBeInTheDocument();
      expect(screen.getByText('上班')).toBeInTheDocument();
      expect(screen.getByText('下班')).toBeInTheDocument();
    });
  });

  test('考勤记录筛选功能', async () => {
    renderWithProviders(<Attendance />);
    
    // 等待考勤记录数据加载
    await waitFor(() => {
      expect(screen.getByText('张三')).toBeInTheDocument();
    });
    
    // 输入筛选条件
    const startDateInput = screen.getByPlaceholderText('开始日期');
    const endDateInput = screen.getByPlaceholderText('结束日期');
    
    fireEvent.change(startDateInput, { target: { value: '2024-01-01' } });
    fireEvent.change(endDateInput, { target: { value: '2024-01-31' } });
    
    // 点击查询按钮
    const queryButton = screen.getByText('查询');
    fireEvent.click(queryButton);
    
    // 等待筛选结果
    await waitFor(() => {
      expect(screen.getByText('张三')).toBeInTheDocument();
    });
  });

  test('同步考勤数据', async () => {
    renderWithProviders(<Attendance />);
    
    // 检查同步按钮是否存在
    const syncButton = screen.getByText('同步考勤数据');
    expect(syncButton).toBeInTheDocument();
    
    // 点击同步按钮
    syncButton.click();
    
    // 等待同步完成
    await waitFor(() => {
      expect(screen.getByText('同步成功')).toBeInTheDocument();
    });
  });

  test('同步考勤数据失败', async () => {
    renderWithProviders(<Attendance />);
    
    // 检查同步按钮是否存在
    const syncButton = screen.getByText('同步考勤数据');
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

  test('导出考勤数据', async () => {
    renderWithProviders(<Attendance />);
    
    // 检查导出按钮是否存在
    const exportButton = screen.getByText('导出');
    expect(exportButton).toBeInTheDocument();
    
    // 点击导出按钮
    exportButton.click();
    
    // 等待导出完成
    await waitFor(() => {
      expect(screen.getByText('导出成功')).toBeInTheDocument();
    });
  });

  test('考勤记录为空', async () => {
    // 模拟空数据
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ code: 200, message: 'success', data: { items: [], total: 0 } })
    });
    
    renderWithProviders(<Attendance />);
    
    // 等待数据加载
    await waitFor(() => {
      expect(screen.getByText('暂无数据')).toBeInTheDocument();
    });
  });
});