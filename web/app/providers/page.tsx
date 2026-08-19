'use client';

/**
 * 提供商管理页：
 * - 列表：name / base_url / enabled / priority（GET /providers）
 * - 新建提供商：Modal 表单（name / base_url / api_key / enabled / priority），POST /providers
 * 注意：api_key 仅提交、不回显。
 * 接口未实现（501）或失败时展示 ErrorState，不崩溃；错误提示由响应拦截器统一 message.error。
 */
import { useCallback, useEffect, useState } from 'react';
import {
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import SideLayout from '@/components/SideLayout';
import ErrorState from '@/components/ErrorState';
import TableSkeleton from '@/components/TableSkeleton';
import { get, post, del } from '@/lib/api';
import type { PageResult, Provider, ProviderCreatePayload } from '@/lib/types';

/** 新建提供商的 Modal 表单值（enabled 为 Switch 的布尔值，提交时转 1/0） */
interface ProviderFormValues {
  name: string;
  base_url: string;
  api_key: string;
  enabled: boolean;
  priority: number;
}

/** 兼容后端返回「分页对象」或「数组」两种形态 */
function normalizeProviders(data: PageResult<Provider> | Provider[] | null | undefined): Provider[] {
  if (!data) return [];
  if (Array.isArray(data)) return data;
  return data.list ?? [];
}

export default function ProvidersPage() {
  const [list, setList] = useState<Provider[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [open, setOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm<ProviderFormValues>();

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await get<PageResult<Provider> | Provider[]>('/providers');
      setList(normalizeProviders(data));
    } catch (e) {
      setError(e instanceof Error ? e.message : '提供商列表加载失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const handleOpen = () => {
    form.resetFields();
    setOpen(true);
  };

  const handleCancel = () => {
    setOpen(false);
  };

  const handleOk = async () => {
    let values: ProviderFormValues;
    try {
      values = await form.validateFields();
    } catch {
      return; // 表单校验失败，AntD 已展示提示
    }

    const payload: ProviderCreatePayload = {
      name: values.name.trim(),
      base_url: values.base_url.trim(),
      api_key: values.api_key.trim(),
      enabled: values.enabled ? 1 : 0,
      priority: values.priority ?? 0,
    };

    setSubmitting(true);
    try {
      await post<Provider>('/providers', payload);
      message.success('提供商创建成功');
      setOpen(false);
      form.resetFields();
      void loadData();
    } catch {
      // 失败：错误提示已由响应拦截器统一处理，保留弹窗供重试
    } finally {
      setSubmitting(false);
    }
  };

  // 删除提供商（有模型的提供商后端会拒绝并提示）
  const handleDelete = async (id: number) => {
    try {
      await del(`/providers/${id}`);
      message.success('提供商已删除');
      void loadData();
    } catch {
      // 失败：错误提示已由响应拦截器统一处理
    }
  };

  const columns: ColumnsType<Provider> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: 'Base URL',
      dataIndex: 'base_url',
      key: 'base_url',
      ellipsis: true,
      render: (value: string) => <Typography.Text code>{value}</Typography.Text>,
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (value: number) =>
        value === 1 ? <Tag color="success">启用</Tag> : <Tag>禁用</Tag>,
    },
    { title: '优先级（小者优先）', dataIndex: 'priority', key: 'priority' },
    {
      title: '操作',
      key: 'action',
      width: 90,
      render: (_, r) => (
        <Popconfirm
          title="删除该提供商？"
          description="删除后不可恢复"
          okText="删除"
          cancelText="取消"
          okButtonProps={{ danger: true }}
          onConfirm={() => void handleDelete(r.id)}
        >
          <Button size="small" danger type="text">
            删除
          </Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <SideLayout>
      <div className="aegis-page-header">
        <div>
          <Typography.Text className="aegis-page-header-kicker">Provider Routing</Typography.Text>
          <Typography.Title level={4}>提供商管理</Typography.Title>
          <Typography.Paragraph className="aegis-page-header-description">
            配置上游模型服务与路由优先级
          </Typography.Paragraph>
        </div>
      </div>

      {loading && <TableSkeleton rows={6} />}

      {!loading && error && <ErrorState description={error} />}

      {!loading && !error && (
        <Card className="aegis-table-card" title="提供商列表">
          <div className="aegis-toolbar" style={{ marginBottom: 16 }}>
            <Typography.Text type="secondary">共 {list.length} 个上游服务</Typography.Text>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleOpen}>
              新建提供商
            </Button>
          </div>
          <Table
            rowKey="id"
            columns={columns}
            dataSource={list}
            scroll={{ x: 640 }}
            pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条` }}
          />
        </Card>
      )}

      <Modal
        title="新建提供商"
        open={open}
        onOk={handleOk}
        onCancel={handleCancel}
        confirmLoading={submitting}
        okText="创建"
        cancelText="取消"
      >
        <Form form={form} layout="vertical" initialValues={{ enabled: true, priority: 0 }}>
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, whitespace: true, message: '请输入提供商名称' }]}
          >
            <Input placeholder="例如：OpenAI" maxLength={64} />
          </Form.Item>
          <Form.Item
            name="base_url"
            label="Base URL"
            rules={[
              { required: true, whitespace: true, message: '请输入 Base URL' },
              { type: 'url', message: '请输入合法的 URL' },
            ]}
          >
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>
          <Form.Item
            name="api_key"
            label="API Key"
            rules={[{ required: true, whitespace: true, message: '请输入上游 API Key' }]}
          >
            <Input.Password placeholder="上游服务商的密钥（仅提交，不回显）" />
          </Form.Item>
          <Form.Item name="enabled" label="启用状态" valuePropName="checked">
            <Switch checkedChildren="启用" unCheckedChildren="禁用" />
          </Form.Item>
          <Form.Item
            name="priority"
            label="优先级（小者优先）"
            rules={[{ required: true, message: '请输入优先级' }]}
          >
            <InputNumber min={0} style={{ width: '100%' }} placeholder="0 为最高优先级" />
          </Form.Item>
        </Form>
      </Modal>
    </SideLayout>
  );
}
