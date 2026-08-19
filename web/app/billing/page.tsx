'use client';

/**
 * 每日账单页：
 * - 列表：date / api_key_id / request_count / total_tokens / cost（GET /billing/daily）
 * - 表格底部「本页合计」汇总行
 * 接口未实现（501）或失败时展示 ErrorState，不崩溃；错误提示由响应拦截器统一 message.error。
 */
import { useCallback, useEffect, useState } from 'react';
import { Card, Table, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import SideLayout from '@/components/SideLayout';
import ErrorState from '@/components/ErrorState';
import TableSkeleton from '@/components/TableSkeleton';
import { get } from '@/lib/api';
import type { BillingDaily } from '@/lib/types';

const columns: ColumnsType<BillingDaily> = [
  { title: '日期', dataIndex: 'date', key: 'date', width: 130 },
  { title: 'Key ID', dataIndex: 'api_key_id', key: 'api_key_id', width: 100 },
  { title: '请求数', dataIndex: 'request_count', key: 'request_count', width: 120 },
  { title: 'Token 总数', dataIndex: 'total_tokens', key: 'total_tokens', width: 140 },
  {
    title: '成本（元）',
    dataIndex: 'cost',
    key: 'cost',
    width: 140,
    render: (value: number) => `¥ ${value.toFixed(4)}`,
  },
];

export default function BillingPage() {
  const [list, setList] = useState<BillingDaily[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await get<BillingDaily[]>('/billing/daily');
      setList(Array.isArray(data) ? data : []);
    } catch (e) {
      setError(e instanceof Error ? e.message : '账单加载失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  return (
    <SideLayout>
      <div className="aegis-page-header">
        <div>
          <Typography.Title level={4}>每日账单</Typography.Title>
          <Typography.Paragraph className="aegis-page-header-description">
            按日核对请求量、Token 使用与模型成本
          </Typography.Paragraph>
        </div>
      </div>

      {loading && <TableSkeleton rows={6} />}

      {!loading && error && <ErrorState description={error} />}

      {!loading && !error && (
        <Card title="按日汇总（请求数 / Token / 成本）">
          <Table
            rowKey="id"
            columns={columns}
            dataSource={list}
            scroll={{ x: 640 }}
            pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条` }}
            summary={(pageData) => {
              const requestCount = pageData.reduce((sum, row) => sum + row.request_count, 0);
              const totalTokens = pageData.reduce((sum, row) => sum + row.total_tokens, 0);
              const cost = pageData.reduce((sum, row) => sum + row.cost, 0);
              return (
                <Table.Summary.Row>
                  <Table.Summary.Cell index={0} colSpan={2}>
                    <Typography.Text strong>本页合计</Typography.Text>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={1}>
                    <Typography.Text strong>{requestCount}</Typography.Text>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={2}>
                    <Typography.Text strong>{totalTokens}</Typography.Text>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={3}>
                    <Typography.Text strong>¥ {cost.toFixed(4)}</Typography.Text>
                  </Table.Summary.Cell>
                </Table.Summary.Row>
              );
            }}
          />
        </Card>
      )}
    </SideLayout>
  );
}
