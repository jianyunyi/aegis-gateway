'use client';

/**
 * 调用日志页：
 * - 服务端分页：GET /logs?page=&page_size=&model_name=（docs/03 第 6.2 节）
 * - 顶部按 model_name 筛选输入框
 * - 列表：request_id / api_key_id / model_name / total_tokens / cost / latency_ms / status / cached / created_at
 * 接口未实现（501）或失败时展示 ErrorState，不崩溃；错误提示由响应拦截器统一 message.error。
 */
import { useCallback, useEffect, useState } from 'react';
import { Card, Input, Spin, Table, Tag, Typography } from 'antd';
import dayjs from 'dayjs';
import type { ColumnsType } from 'antd/es/table';
import SideLayout from '@/components/SideLayout';
import ErrorState from '@/components/ErrorState';
import { get } from '@/lib/api';
import type { PageResult, UsageLog } from '@/lib/types';

/** 时间格式化，空值返回占位符 */
function fmt(value?: string | null): string {
  if (!value) return '-';
  const d = dayjs(value);
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : value;
}

const columns: ColumnsType<UsageLog> = [
  { title: 'Request ID', dataIndex: 'request_id', key: 'request_id', ellipsis: true },
  { title: 'Key ID', dataIndex: 'api_key_id', key: 'api_key_id', width: 90 },
  { title: '模型', dataIndex: 'model_name', key: 'model_name', ellipsis: true },
  { title: 'Token 总数', dataIndex: 'total_tokens', key: 'total_tokens', width: 110 },
  {
    title: '成本（元）',
    dataIndex: 'cost',
    key: 'cost',
    width: 110,
    render: (value: number) => `¥ ${value.toFixed(4)}`,
  },
  {
    title: '延迟（ms）',
    dataIndex: 'latency_ms',
    key: 'latency_ms',
    width: 100,
    render: (value: number) => (value == null ? '-' : value),
  },
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
    width: 100,
    render: (value: number) =>
      value === 0 ? <Tag color="success">成功</Tag> : <Tag color="error">失败({value})</Tag>,
  },
  {
    title: '缓存',
    dataIndex: 'cached',
    key: 'cached',
    width: 100,
    render: (value: number) =>
      value === 1 ? <Tag color="cyan">缓存命中</Tag> : <Tag>未命中</Tag>,
  },
  { title: '调用时间', dataIndex: 'created_at', key: 'created_at', width: 170, render: fmt },
];

export default function LogsPage() {
  const [list, setList] = useState<UsageLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [total, setTotal] = useState(0);
  const [modelName, setModelName] = useState('');

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await get<PageResult<UsageLog> | UsageLog[]>('/logs', {
        page,
        page_size: pageSize,
        model_name: modelName || undefined,
      });
      setList(Array.isArray(data) ? data : data?.list ?? []);
      setTotal(Array.isArray(data) ? data.length : data?.total ?? 0);
    } catch (e) {
      setError(e instanceof Error ? e.message : '日志加载失败');
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, modelName]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  return (
    <SideLayout>
      <div className="aegis-page-header">
        <div>
          <Typography.Title level={4}>调用日志</Typography.Title>
          <Typography.Paragraph className="aegis-page-header-description">
            追踪请求状态、模型延迟与调用成本
          </Typography.Paragraph>
        </div>
      </div>

      {loading && (
        <div style={{ textAlign: 'center', padding: 80 }}>
          <Spin size="large" tip="加载中..." />
        </div>
      )}

      {!loading && error && <ErrorState description={error} />}

      {!loading && !error && (
        <Card title="全量调用日志">
          <div className="aegis-toolbar" style={{ marginBottom: 16 }}>
            <Typography.Text type="secondary">按模型名称筛选调用记录</Typography.Text>
            <Input.Search
              allowClear
              placeholder="按模型名筛选，例如 gpt-4o"
              style={{ width: 280, maxWidth: '100%' }}
              onSearch={(value) => {
                setModelName(value.trim());
                setPage(1);
              }}
            />
          </div>
          <Table
            rowKey="id"
            columns={columns}
            dataSource={list}
            scroll={{ x: 1080 }}
            pagination={{
              current: page,
              pageSize,
              total,
              showSizeChanger: true,
              showTotal: (t) => `共 ${t} 条`,
              onChange: (nextPage, nextPageSize) => {
                setPage(nextPage);
                setPageSize(nextPageSize);
              },
            }}
          />
        </Card>
      )}
    </SideLayout>
  );
}
