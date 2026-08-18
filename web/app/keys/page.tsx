'use client';

/**
 * Key 管理页：
 * - 列表：name / key_prefix / status / rps_limit / quota_tokens / expires_at / created_at（GET /keys）
 * - 新建 Key：Modal 表单（name / rps_limit / burst / quota_tokens / expires_at），POST /keys
 * - 创建成功：Alert 展示明文 Key 一次（仅此一次展示，之后不可再查）
 * - 禁用/启用：PUT /keys/{id}，body { status: 0|1 }
 * 接口未实现（501）或失败时展示 ErrorState，不崩溃；错误提示由响应拦截器统一 message.error。
 */
import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import type { ColumnsType } from 'antd/es/table';
import SideLayout from '@/components/SideLayout';
import ErrorState from '@/components/ErrorState';
import { get, post, put } from '@/lib/api';
import type { ApiKey, ApiKeyCreatePayload, ApiKeyCreated, PageResult } from '@/lib/types';

/** 新建 Key 的 Modal 表单值（expires_at 为 dayjs 对象，提交时转字符串） */
interface KeyFormValues {
  name: string;
  rps_limit?: number;
  burst?: number;
  quota_tokens?: number;
  expires_at?: Dayjs | null;
}

/** 兼容后端返回「分页对象」或「数组」两种形态 */
function normalizeKeys(data: PageResult<ApiKey> | ApiKey[] | null | undefined): ApiKey[] {
  if (!data) return [];
  if (Array.isArray(data)) return data;
  return data.list ?? [];
}

/** 时间格式化，空值返回占位符 */
function fmt(value?: string | null): string {
  if (!value) return '-';
  const d = dayjs(value);
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : value;
}

export default function KeysPage() {
  const [list, setList] = useState<ApiKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [open, setOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [createdKey, setCreatedKey] = useState<ApiKeyCreated | null>(null);
  const [togglingId, setTogglingId] = useState<number | null>(null);

  const [form] = Form.useForm<KeyFormValues>();

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await get<PageResult<ApiKey> | ApiKey[]>('/keys');
      setList(normalizeKeys(data));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Key 列表加载失败');
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
    let values: KeyFormValues;
    try {
      values = await form.validateFields();
    } catch {
      return; // 表单校验失败，AntD 已展示提示
    }

    const payload: ApiKeyCreatePayload = {
      name: values.name.trim(),
      rps_limit: values.rps_limit ?? 0,
      burst: values.burst ?? 0,
      quota_tokens: values.quota_tokens ?? 0,
      expires_at: values.expires_at ? values.expires_at.format('YYYY-MM-DD HH:mm:ss') : null,
    };

    setSubmitting(true);
    try {
      const created = await post<ApiKeyCreated>('/keys', payload);
      setCreatedKey(created);
      message.success('Key 创建成功，请立即保存明文 Key');
      setOpen(false);
      form.resetFields();
      void loadData();
    } catch {
      // 失败：错误提示已由响应拦截器统一处理，保留弹窗供重试
    } finally {
      setSubmitting(false);
    }
  };

  const toggleStatus = async (record: ApiKey) => {
    const next = record.status === 1 ? 0 : 1;
    setTogglingId(record.id);
    try {
      await put<ApiKey>(`/keys/${record.id}`, { status: next });
      message.success(next === 1 ? 'Key 已启用' : 'Key 已禁用');
      void loadData();
    } catch {
      // 失败：错误提示已由响应拦截器统一处理
    } finally {
      setTogglingId(null);
    }
  };

  const columns: ColumnsType<ApiKey> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: 'Key 前缀',
      dataIndex: 'key_prefix',
      key: 'key_prefix',
      render: (value: string) => <Typography.Text code>{value}</Typography.Text>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (value: number) =>
        value === 1 ? <Tag color="success">启用</Tag> : <Tag>禁用</Tag>,
    },
    { title: 'RPS 限制', dataIndex: 'rps_limit', key: 'rps_limit' },
    {
      title: 'Token 配额',
      dataIndex: 'quota_tokens',
      key: 'quota_tokens',
      render: (value: number) => (value === 0 ? '不限' : value),
    },
    { title: '过期时间', dataIndex: 'expires_at', key: 'expires_at', render: fmt },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', render: fmt },
    {
      title: '操作',
      key: 'action',
      render: (_, record) => (
        <Popconfirm
          title={record.status === 1 ? '确认禁用该 Key？禁用后调用将返回 401。' : '确认启用该 Key？'}
          onConfirm={() => toggleStatus(record)}
        >
          <Button size="small" type="link" loading={togglingId === record.id}>
            {record.status === 1 ? '禁用' : '启用'}
          </Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <SideLayout>
      <div className="aegis-page-header">
        <div>
          <Typography.Title level={4}>Key 管理</Typography.Title>
          <Typography.Paragraph className="aegis-page-header-description">
            管理调用凭证、速率限制与 Token 配额
          </Typography.Paragraph>
        </div>
      </div>

      {createdKey && (
        <Alert
          type="success"
          showIcon
          closable
          onClose={() => setCreatedKey(null)}
          style={{ marginBottom: 16 }}
          message="Key 创建成功"
          description={
            <Space direction="vertical" size={4}>
              <Typography.Text type="warning">
                明文 Key 仅此一次展示，请立即复制保存；关闭后将无法再次查看。
              </Typography.Text>
              {createdKey.key && (
                <Typography.Paragraph copyable={{ text: createdKey.key }} style={{ marginBottom: 0 }}>
                  <Typography.Text code>{createdKey.key}</Typography.Text>
                </Typography.Paragraph>
              )}
            </Space>
          }
        />
      )}

      {loading && (
        <div style={{ textAlign: 'center', padding: 80 }}>
          <Spin size="large" tip="加载中..." />
        </div>
      )}

      {!loading && error && <ErrorState description={error} />}

      {!loading && !error && (
        <Card title="Key 列表">
          <div className="aegis-toolbar" style={{ marginBottom: 16 }}>
            <Typography.Text type="secondary">共 {list.length} 个凭证</Typography.Text>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleOpen}>
              新建 Key
            </Button>
          </div>
          <Table
            rowKey="id"
            columns={columns}
            dataSource={list}
            scroll={{ x: 960 }}
            pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条` }}
          />
        </Card>
      )}

      <Modal
        title="新建 Key"
        open={open}
        onOk={handleOk}
        onCancel={handleCancel}
        confirmLoading={submitting}
        okText="创建"
        cancelText="取消"
      >
        <Form
          form={form}
          layout="vertical"
          initialValues={{ rps_limit: 10, burst: 20, quota_tokens: 0 }}
        >
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, whitespace: true, message: '请输入 Key 名称' }]}
          >
            <Input placeholder="例如：生产环境-主服务" maxLength={64} />
          </Form.Item>
          <Form.Item
            name="rps_limit"
            label="RPS 限制"
            rules={[{ required: true, message: '请输入 RPS 限制' }]}
          >
            <InputNumber min={0} max={100000} style={{ width: '100%' }} placeholder="每秒请求上限" />
          </Form.Item>
          <Form.Item
            name="burst"
            label="突发上限（burst）"
            rules={[{ required: true, message: '请输入突发上限' }]}
          >
            <InputNumber min={0} max={1000000} style={{ width: '100%' }} placeholder="令牌桶容量" />
          </Form.Item>
          <Form.Item
            name="quota_tokens"
            label="Token 配额（0 表示不限）"
            rules={[{ required: true, message: '请输入 Token 配额' }]}
          >
            <InputNumber min={0} style={{ width: '100%' }} placeholder="0 表示不限制" />
          </Form.Item>
          <Form.Item name="expires_at" label="过期时间（可选）">
            <DatePicker style={{ width: '100%' }} showTime placeholder="不填则永不过期" />
          </Form.Item>
        </Form>
      </Modal>
    </SideLayout>
  );
}
