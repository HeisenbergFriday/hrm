// 测试环境设置
import '@testing-library/jest-dom';

// 全局mock配置
jest.mock('./services/api', () => {
  return {
    authAPI: require('./services/api.mock').authAPIMock,
    userAPI: require('./services/api.mock').userAPIMock,
    departmentAPI: require('./services/api.mock').departmentAPIMock,
    attendanceAPI: require('./services/api.mock').attendanceAPIMock,
    syncAPI: require('./services/api.mock').syncAPIMock,
    orgAPI: require('./services/api.mock').orgAPIMock,
  };
});

// 全局变量
(global as any).window = {
  ...global.window,
  location: {
    href: 'http://localhost:3000',
  },
  localStorage: {
    getItem: jest.fn(),
    setItem: jest.fn(),
    removeItem: jest.fn(),
  },
};
