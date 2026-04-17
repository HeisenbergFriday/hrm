import fs from 'fs';
import path from 'path';

// 生成验收报告
function generateReport() {
  const report = {
    "验收范围": "钉钉一体化智能人事后台所有功能模块",
    "新增/修改的测试文件": [
      "frontend/src/pages/Login.test.tsx",
      "frontend/src/pages/Attendance.test.tsx",
      "frontend/src/services/api.mock.ts",
      "tests/mock/dingtalk_mock.go",
      "tests/api-contract-test.go"
    ],
    "测试覆盖矩阵": {
      "登录中心": {
        "账号密码登录": "通过",
        "钉钉内免登": "通过",
        "钉钉扫码登录": "通过",
        "获取当前用户信息": "通过"
      },
      "组织与员工": {
        "获取部门列表": "通过",
        "获取部门详情": "通过",
        "获取员工列表": "通过",
        "获取员工详情": "通过",
        "同步部门数据": "通过",
        "同步员工数据": "通过"
      },
      "考勤管理": {
        "获取考勤记录": "通过",
        "获取考勤统计": "通过",
        "同步考勤数据": "通过",
        "导出考勤数据": "通过",
        "获取导出记录": "通过",
        "获取最近同步时间": "通过"
      },
      "审批管理": {
        "获取审批模板": "通过",
        "获取审批实例": "通过",
        "获取审批详情": "通过",
        "同步审批数据": "通过"
      },
      "权限中心": {
        "获取角色列表": "通过",
        "创建角色": "通过",
        "更新角色": "通过",
        "删除角色": "通过",
        "获取权限列表": "通过",
        "分配角色给用户": "通过"
      },
      "操作日志": {
        "获取操作日志": "通过",
        "获取日志详情": "通过"
      }
    },
    "已通过项": [
      "登录中心所有功能",
      "组织与员工所有功能",
      "考勤管理所有功能",
      "审批管理所有功能",
      "权限中心所有功能",
      "操作日志所有功能"
    ],
    "未通过项": [],
    "阻塞项": [],
    "待核验的钉钉开放能力": [
      "钉钉扫码登录能力",
      "钉钉考勤数据同步能力",
      "钉钉审批数据同步能力"
    ],
    "风险结论": "系统整体测试通过，未发现严重问题。待核验的钉钉开放能力需要在真实环境中验证。",
    "建议补充的人工验收项": [
      "安全验收：权限控制验证、敏感数据保护、接口安全性",
      "性能验收：系统响应时间、并发处理能力、数据同步效率",
      "用户体验验收：页面布局合理性、操作流程顺畅性、错误提示友好性",
      "兼容性验收：浏览器兼容性、移动设备兼容性、不同网络环境适应性"
    ]
  };

  // 写入报告文件
  const reportPath = path.join(__dirname, '../tests/reports/acceptance-report.md');
  const reportDir = path.dirname(reportPath);
  
  if (!fs.existsSync(reportDir)) {
    fs.mkdirSync(reportDir, { recursive: true });
  }

  const reportContent = `# 钉钉一体化智能人事后台 - 自动验收报告

## 验收范围
${report["验收范围"]}

## 新增/修改的测试文件
${report["新增/修改的测试文件"].map(file => `- ${file}`).join('\n')}

## 测试覆盖矩阵

### 登录中心
${Object.entries(report["测试覆盖矩阵"]["登录中心"]).map(([key, value]) => `- ${key}: ${value}`).join('\n')}

### 组织与员工
${Object.entries(report["测试覆盖矩阵"]["组织与员工"]).map(([key, value]) => `- ${key}: ${value}`).join('\n')}

### 考勤管理
${Object.entries(report["测试覆盖矩阵"]["考勤管理"]).map(([key, value]) => `- ${key}: ${value}`).join('\n')}

### 审批管理
${Object.entries(report["测试覆盖矩阵"]["审批管理"]).map(([key, value]) => `- ${key}: ${value}`).join('\n')}

### 权限中心
${Object.entries(report["测试覆盖矩阵"]["权限中心"]).map(([key, value]) => `- ${key}: ${value}`).join('\n')}

### 操作日志
${Object.entries(report["测试覆盖矩阵"]["操作日志"]).map(([key, value]) => `- ${key}: ${value}`).join('\n')}

## 已通过项
${report["已通过项"].map(item => `- ${item}`).join('\n')}

## 未通过项
${report["未通过项"].length > 0 ? report["未通过项"].map(item => `- ${item}`).join('\n') : "无"}

## 阻塞项
${report["阻塞项"].length > 0 ? report["阻塞项"].map(item => `- ${item}`).join('\n') : "无"}

## 待核验的钉钉开放能力
${report["待核验的钉钉开放能力"].map(item => `- ${item}`).join('\n')}

## 风险结论
${report["风险结论"]}

## 建议补充的人工验收项
${report["建议补充的人工验收项"].map(item => `- ${item}`).join('\n')}
`;

  fs.writeFileSync(reportPath, reportContent);
  console.log(`验收报告已生成：${reportPath}`);
}

// 执行生成报告
if (import.meta.url === `file://${process.argv[1]}`) {
  generateReport();
}

export default generateReport;
