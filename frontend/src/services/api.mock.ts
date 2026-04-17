import { delay } from '../utils/delay';

// 模拟API响应
const mockData = {
  // 认证相关
  auth: {
    login: {
      success: {
        code: 200,
        message: 'success',
        data: {
          token: 'mock-token-123',
          user: {
            id: 'user123',
            name: '张三',
            email: 'zhangsan@example.com',
            mobile: '13800138000',
            department_id: '1',
            position: '工程师',
            avatar: 'https://example.com/avatar.jpg',
            status: 'active'
          }
        }
      },
      fail: {
        code: 401,
        message: '用户名或密码错误',
        data: null
      }
    },
    dingtalkLogin: {
      success: {
        code: 200,
        message: 'success',
        data: {
          token: 'mock-token-456',
          user: {
            id: 'user123',
            name: '张三',
            email: 'zhangsan@example.com',
            mobile: '13800138000',
            department_id: '1',
            position: '工程师',
            avatar: 'https://example.com/avatar.jpg',
            status: 'active'
          }
        }
      },
      fail: {
        code: 401,
        message: '钉钉登录失败',
        data: null
      }
    },
    me: {
      success: {
        code: 200,
        message: 'success',
        data: {
          user: {
            id: 'user123',
            name: '张三',
            email: 'zhangsan@example.com',
            mobile: '13800138000',
            department_id: '1',
            position: '工程师',
            avatar: 'https://example.com/avatar.jpg',
            status: 'active'
          }
        }
      }
    }
  },
  // 部门相关
  departments: {
    success: {
      code: 200,
      message: 'success',
      data: {
        departments: [
          {
            id: '1',
            department_id: '1',
            name: '技术部',
            parent_id: '0',
            order: 1
          },
          {
            id: '2',
            department_id: '2',
            name: '市场部',
            parent_id: '0',
            order: 2
          },
          {
            id: '3',
            department_id: '3',
            name: '产品部',
            parent_id: '0',
            order: 3
          }
        ]
      }
    }
  },
  // 员工相关
  users: {
    success: {
      code: 200,
      message: 'success',
      data: {
        users: [
          {
            id: '1',
            user_id: 'user123',
            name: '张三',
            email: 'zhangsan@example.com',
            mobile: '13800138000',
            department_id: '1',
            position: '工程师',
            avatar: 'https://example.com/avatar.jpg',
            status: 'active'
          },
          {
            id: '2',
            user_id: 'user456',
            name: '李四',
            email: 'lisi@example.com',
            mobile: '13900139000',
            department_id: '1',
            position: '产品经理',
            avatar: 'https://example.com/avatar2.jpg',
            status: 'active'
          }
        ],
        total: 2
      }
    }
  },
  // 考勤相关
  attendance: {
    records: {
      success: {
        code: 200,
        message: 'success',
        data: {
          items: [
            {
              id: '1',
              user_id: 'user123',
              user_name: '张三',
              check_time: '2024-01-01 09:00:00',
              check_type: '上班',
              location: '公司',
              is_abnormal: false
            },
            {
              id: '2',
              user_id: 'user123',
              user_name: '张三',
              check_time: '2024-01-01 18:00:00',
              check_type: '下班',
              location: '公司',
              is_abnormal: false
            }
          ],
          total: 2
        }
      },
      empty: {
        code: 200,
        message: 'success',
        data: {
          items: [],
          total: 0
        }
      }
    },
    stats: {
      success: {
        code: 200,
        message: 'success',
        data: {
          statistics: {
            normal_days: 20,
            late_days: 2,
            absent_days: 1,
            overtime_hours: 10
          }
        }
      }
    },
    lastSync: {
      success: {
        code: 200,
        message: 'success',
        data: {
          attendance: {
            last_sync_time: '2024-01-01T12:00:00Z',
            status: 'success',
            record_count: 150
          }
        }
      }
    }
  },
  // 审批相关
  approvals: {
    success: {
      code: 200,
      message: 'success',
      data: {
        approvals: [
          {
            id: '1',
            process_id: 'process123',
            title: '请假申请',
            applicant_id: 'user123',
            applicant_name: '张三',
            status: 'completed',
            create_time: '2024-01-01T08:00:00Z',
            finish_time: '2024-01-01T10:00:00Z'
          }
        ],
        total: 1
      }
    }
  },
  // 同步相关
  sync: {
    success: {
      code: 200,
      message: 'success',
      data: {
        count: 10
      }
    },
    fail: {
      code: 500,
      message: '同步失败',
      data: null
    }
  }
};

// 模拟API延迟
const MOCK_DELAY = 500;

// 模拟认证API
export const authAPIMock = {
  login: async (data: { username: string; password: string }) => {
    await delay(MOCK_DELAY);
    if (data.username === 'admin' && data.password === '123456') {
      return mockData.auth.login.success;
    } else {
      return mockData.auth.login.fail;
    }
  },
  dingtalkLogin: async (data: { code: string }) => {
    await delay(MOCK_DELAY);
    if (data.code === 'success') {
      return mockData.auth.dingtalkLogin.success;
    } else {
      return mockData.auth.dingtalkLogin.fail;
    }
  },
  logout: async () => {
    await delay(MOCK_DELAY);
    return { code: 200, message: 'success' };
  },
  getCurrentUser: async () => {
    await delay(MOCK_DELAY);
    return mockData.auth.me.success;
  }
};

// 模拟用户API
export const userAPIMock = {
  getUsers: async (params: { page: number; page_size: number }) => {
    await delay(MOCK_DELAY);
    return mockData.users.success;
  },
  getUser: async (id: string) => {
    await delay(MOCK_DELAY);
    return {
      code: 200,
      message: 'success',
      data: {
        user: mockData.users.success.data.users.find((user: any) => user.id === id)
      }
    };
  },
  updateUser: async (id: string, data: { extension: any }) => {
    await delay(MOCK_DELAY);
    return {
      code: 200,
      message: 'success',
      data: {
        user: {
          ...mockData.users.success.data.users.find((user: any) => user.id === id),
          extension: data.extension
        }
      }
    };
  }
};

// 模拟部门API
export const departmentAPIMock = {
  getDepartments: async () => {
    await delay(MOCK_DELAY);
    return mockData.departments.success;
  },
  getDepartment: async (id: string) => {
    await delay(MOCK_DELAY);
    return {
      code: 200,
      message: 'success',
      data: {
        department: mockData.departments.success.data.departments.find((dept: any) => dept.id === id)
      }
    };
  }
};

// 模拟考勤API
export const attendanceAPIMock = {
  getRecords: async (params: any) => {
    await delay(MOCK_DELAY);
    if (params.user_id === 'empty') {
      return mockData.attendance.records.empty;
    }
    return mockData.attendance.records.success;
  },
  getStats: async (params: any) => {
    await delay(MOCK_DELAY);
    return mockData.attendance.stats.success;
  },
  sync: async (data?: { start_date?: string; end_date?: string }) => {
    await delay(MOCK_DELAY);
    if (data?.start_date === 'fail') {
      return mockData.sync.fail;
    }
    return mockData.sync.success;
  },
  export: async (data: any) => {
    await delay(MOCK_DELAY);
    return {
      code: 200,
      message: 'success',
      data: {
        file_url: 'https://example.com/export.xlsx'
      }
    };
  },
  getExports: async (params: any) => {
    await delay(MOCK_DELAY);
    return {
      code: 200,
      message: 'success',
      data: {
        items: [
          {
            id: '1',
            file_name: 'attendance_2024_01.xlsx',
            status: 'completed',
            created_at: '2024-01-31T18:00:00Z'
          }
        ],
        total: 1
      }
    };
  },
  getLastSyncTime: async () => {
    await delay(MOCK_DELAY);
    return mockData.attendance.lastSync.success;
  }
};

// 模拟同步API
export const syncAPIMock = {
  syncDepartments: async () => {
    await delay(MOCK_DELAY);
    return mockData.sync.success;
  },
  syncUsers: async () => {
    await delay(MOCK_DELAY);
    return mockData.sync.success;
  },
  getSyncStatus: async () => {
    await delay(MOCK_DELAY);
    return {
      code: 200,
      message: 'success',
      data: {
        status: {
          departments: {
            last_sync_time: '2024-01-01T12:00:00Z',
            status: 'success'
          },
          users: {
            last_sync_time: '2024-01-01T12:00:00Z',
            status: 'success'
          }
        }
      }
    };
  }
};

// 模拟组织API
export const orgAPIMock = {
  getDepartmentTree: async () => {
    await delay(MOCK_DELAY);
    return {
      code: 200,
      message: 'success',
      data: {
        tree: [
          {
            id: '1',
            name: '技术部',
            children: [
              {
                id: '4',
                name: '前端组',
                children: []
              },
              {
                id: '5',
                name: '后端组',
                children: []
              }
            ]
          }
        ]
      }
    };
  },
  getEmployees: async (params: any) => {
    await delay(MOCK_DELAY);
    return mockData.users.success;
  },
  getEmployee: async (id: string) => {
    await delay(MOCK_DELAY);
    return {
      code: 200,
      message: 'success',
      data: {
        employee: mockData.users.success.data.users.find((user: any) => user.id === id)
      }
    };
  },
  syncOrg: async () => {
    await delay(MOCK_DELAY);
    return mockData.sync.success;
  }
};
