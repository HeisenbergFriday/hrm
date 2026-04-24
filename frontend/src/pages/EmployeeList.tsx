import React, { useEffect, useState } from 'react'
import { Card, Table, Button, message, Spin, Typography, Input, Select, Space } from 'antd'
import { SyncOutlined, EditOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { orgAPI, departmentAPI } from '../services/api'

const { Title } = Typography
const { Option } = Select
const { Search } = Input

const EmployeeList: React.FC = () => {
  const [employees, setEmployees] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [searchText, setSearchText] = useState('')
  const [department, setDepartment] = useState('')
  const [departments, setDepartments] = useState<any[]>([])
  const navigate = useNavigate()

  useEffect(() => {
    fetchEmployees()
  }, [page, pageSize, searchText, department])

  useEffect(() => {
    fetchDepartments()
  }, [])

  const fetchDepartments = async () => {
    try {
      const response = await departmentAPI.getDepartments()
      setDepartments(response.data.departments)
    } catch (error) {
      message.error('获取部门列表失败')
    }
  }

  const fetchEmployees = async () => {
    setLoading(true)
    try {
      const response = await orgAPI.getEmployees({
        page,
        page_size: pageSize,
        department_id: department
      })
      setEmployees(response.data.items)
      setTotal(response.data.total)
    } catch (error) {
      message.error('获取员工列表失败')
    } finally {
      setLoading(false)
    }
  }

  const handleSync = async () => {
    setLoading(true)
    try {
      await orgAPI.syncOrg()
      message.success('同步成功')
      fetchEmployees()
    } catch (error) {
      message.error('同步失败')
    } finally {
      setLoading(false)
    }
  }

  const handleView = (id: string) => {
    navigate(`/employees/${id}`)
  }

  const columns = [
    {
      title: '姓名',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '邮箱',
      dataIndex: 'email',
      key: 'email',
    },
    {
      title: '手机号',
      dataIndex: 'mobile',
      key: 'mobile',
    },
    {
      title: '部门',
      dataIndex: 'department_id',
      key: 'department_id',
    },
    {
      title: '职位',
      dataIndex: 'position',
      key: 'position',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => {
        switch (status) {
          case 'active':
            return '在职';
          case 'inactive':
            return '离职';
          default:
            return status;
        }
      },
    },
    {
      title: '标签',
      dataIndex: 'extension',
      key: 'tags',
      render: (extension: any) => (
        <div>
          {extension?.tags?.map((tag: string, index: number) => (
            <span key={index} style={{ marginRight: '4px', padding: '2px 8px', background: '#f0f0f0', borderRadius: '4px' }}>
              {tag}
            </span>
          ))}
        </div>
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: any) => (
        <Space size="middle">
          <Button 
            type="link" 
            icon={<EditOutlined />} 
            onClick={() => handleView(record.id)}
          >
            查看
          </Button>
          <Button 
            type="link" 
            icon={<DeleteOutlined />} 
            danger
          >
            删除
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Card 
        title={<Title level={4}>员工列表</Title>} 
        extra={
          <Button 
            type="primary" 
            icon={<SyncOutlined />} 
            onClick={handleSync}
            loading={loading}
          >
            同步员工数据
          </Button>
        }
      >
        <div style={{ marginBottom: '16px', display: 'flex', gap: '16px' }}>
          <Search
            placeholder="搜索员工"
            allowClear
            enterButton={<SearchOutlined />}
            onSearch={(value) => setSearchText(value)}
            style={{ width: 300 }}
          />
          <Select
            placeholder="选择部门"
            allowClear
            style={{ width: 200 }}
            onChange={(value) => setDepartment(value)}
          >
            {departments.map(dept => (
              <Option key={dept.department_id} value={dept.department_id}>
                {dept.name}
              </Option>
            ))}
          </Select>
        </div>
        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : (
          <Table
            columns={columns}
            dataSource={employees}
            rowKey="id"
            pagination={{
              current: page,
              pageSize: pageSize,
              total: total,
              onChange: (page, pageSize) => {
                setPage(page)
                setPageSize(pageSize)
              },
            }}
          />
        )}
      </Card>
    </div>
  )
}

export default EmployeeList