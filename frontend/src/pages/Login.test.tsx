import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter } from 'react-router-dom';
import Login from './Login';

// 创建QueryClient实例
const queryClient = new QueryClient();

// 包装组件，提供必要的上下文
const renderWithProviders = (ui: React.ReactElement) => {
  return render(
    <BrowserRouter>
      <QueryClientProvider client={queryClient}>
        {ui}
      </QueryClientProvider>
    </BrowserRouter>
  );
};

describe('Login Component', () => {
  beforeEach(() => {
    // 清除localStorage的mock
    localStorage.getItem = jest.fn();
    localStorage.setItem = jest.fn();
    localStorage.removeItem = jest.fn();
  });

  test('渲染登录页面', () => {
    renderWithProviders(<Login />);
    
    // 检查页面是否包含登录标题
    expect(screen.getByText('登录')).toBeInTheDocument();
    
    // 检查是否包含用户名和密码输入框
    expect(screen.getByPlaceholderText('用户名')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('密码')).toBeInTheDocument();
    
    // 检查是否包含登录按钮
    expect(screen.getByText('登录')).toBeInTheDocument();
    
    // 检查是否包含钉钉登录按钮
    expect(screen.getByText('钉钉登录')).toBeInTheDocument();
  });

  test('账号密码登录成功', async () => {
    renderWithProviders(<Login />);
    
    // 输入用户名和密码
    fireEvent.change(screen.getByPlaceholderText('用户名'), { target: { value: 'admin' } });
    fireEvent.change(screen.getByPlaceholderText('密码'), { target: { value: '123456' } });
    
    // 点击登录按钮
    fireEvent.click(screen.getByText('登录'));
    
    // 等待登录成功后的跳转
    await waitFor(() => {
      expect(localStorage.setItem).toHaveBeenCalledWith('token', 'mock-token-123');
    });
  });

  test('账号密码登录失败', async () => {
    renderWithProviders(<Login />);
    
    // 输入错误的用户名和密码
    fireEvent.change(screen.getByPlaceholderText('用户名'), { target: { value: 'wrong' } });
    fireEvent.change(screen.getByPlaceholderText('密码'), { target: { value: 'wrong' } });
    
    // 点击登录按钮
    fireEvent.click(screen.getByText('登录'));
    
    // 等待错误提示
    await waitFor(() => {
      expect(screen.getByText('用户名或密码错误')).toBeInTheDocument();
    });
  });

  test('钉钉登录成功', async () => {
    renderWithProviders(<Login />);
    
    // 模拟钉钉登录成功
    window.location.href = 'http://localhost:3000/login?code=success';
    
    // 点击钉钉登录按钮
    fireEvent.click(screen.getByText('钉钉登录'));
    
    // 等待登录成功后的跳转
    await waitFor(() => {
      expect(localStorage.setItem).toHaveBeenCalledWith('token', 'mock-token-456');
    });
  });

  test('钉钉登录失败', async () => {
    renderWithProviders(<Login />);
    
    // 模拟钉钉登录失败
    window.location.href = 'http://localhost:3000/login?code=fail';
    
    // 点击钉钉登录按钮
    fireEvent.click(screen.getByText('钉钉登录'));
    
    // 等待错误提示
    await waitFor(() => {
      expect(screen.getByText('钉钉登录失败')).toBeInTheDocument();
    });
  });

  test('表单验证', () => {
    renderWithProviders(<Login />);
    
    // 点击登录按钮，不输入任何内容
    fireEvent.click(screen.getByText('登录'));
    
    // 检查是否显示验证提示
    expect(screen.getByText('请输入用户名')).toBeInTheDocument();
    expect(screen.getByText('请输入密码')).toBeInTheDocument();
  });
});
