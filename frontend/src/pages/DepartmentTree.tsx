import React, { useEffect, useState } from 'react'
import { Card, Tree, Button, message, Spin, Typography } from 'antd'
import { SyncOutlined, PlusOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
import { orgAPI } from '../services/api'

const { Title } = Typography

const DepartmentTree: React.FC = () => {
  const [treeData, setTreeData] = useState<any[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetchDepartmentTree()
  }, [])

  const fetchDepartmentTree = async () => {
    setLoading(true)
    try {
      const response = await orgAPI.getDepartmentTree()
      setTreeData(response.data.tree)
    } catch (error) {
      message.error('获取部门树失败')
    } finally {
      setLoading(false)
    }
  }

  const handleSync = async () => {
    setLoading(true)
    try {
      await orgAPI.syncOrg()
      message.success('同步成功')
      fetchDepartmentTree()
    } catch (error) {
      message.error('同步失败')
    } finally {
      setLoading(false)
    }
  }

  const onSelect = (selectedKeys: React.Key[], info: any) => {
    console.log('Selected:', selectedKeys, info)
  }

  return (
    <div>
      <Card 
        title={<Title level={4}>部门树</Title>} 
        extra={
          <Button 
            type="primary" 
            icon={<SyncOutlined />} 
            onClick={handleSync}
            loading={loading}
          >
            同步部门数据
          </Button>
        }
      >
        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px' }}>
            <Spin size="large" />
          </div>
        ) : (
          <Tree
            showLine
            defaultExpandAll
            onSelect={onSelect}
            treeData={treeData}
            titleRender={(node) => {
              return (
                <span>
                  <span>{node.name}</span>
                  <span style={{ marginLeft: '8px' }}>
                    <Button 
                      type="link" 
                      icon={<EditOutlined />} 
                      size="small"
                    />
                    <Button 
                      type="link" 
                      icon={<DeleteOutlined />} 
                      size="small"
                      danger
                    />
                  </span>
                </span>
              )
            }}
          />
        )}
      </Card>
    </div>
  )
}

export default DepartmentTree