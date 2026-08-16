'use client';

/**
 * 评测页（占位，MVP）：
 * - 功能说明文案：自动采样积累评测样本 → 人工打标 → 一键回归对比模型（质量分 + 成本 + 延迟）
 * - 数据集列表：GET /evals/datasets（允许空态，空列表不视为错误）
 * 接口未实现（501）或失败时展示 ErrorState，不崩溃；错误提示由响应拦截器统一 message.error。
 */
import { useCallback, useEffect, useState } from 'react';
import { Card, Spin, Table, Tag, Typography } from 'antd';
import dayjs from 'dayjs';
import type { ColumnsType } from 'antd/es/table';
import SideLayout from '@/components/SideLayout';
import ErrorState from '@/components/ErrorState';
import { get } from '@/lib/api';
import type { EvalDataset } from '@/lib/types';

/** 时间格式化，空值返回占位符 */
function fmt(value?: string | null): string {
  if (!value) return '-';
  const d = dayjs(value);
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : value;
}

const columns: ColumnsType<EvalDataset> = [
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
    width: 100,
    render: (value: number) =>
      value === 1 ? <Tag color="success">可用</Tag> : <Tag color="default">草稿</Tag>,
  },
  { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 170, render: fmt },
];

export default function EvalsPage() {
  const [list, setList] = useState<EvalDataset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await get<EvalDataset[]>('/evals/datasets');
      setList(Array.isArray(data) ? data : []);
    } catch (e) {
      setError(e instanceof Error ? e.message : '评测数据集加载失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  return (
    <SideLayout>
      <Typography.Title level={4} style={{ marginTop: 0 }}>
        评测
      </Typography.Title>

      <Typography.Paragraph type="secondary">
        评测飞轮（MVP）：自动从真实调用中按模型/类型分层采样积累评测样本，支持人工打标，
        并可对同一批样本一键回归对比不同模型，输出质量分 + 成本 + 延迟报告。
        当前为占位页面，后端接口尚未实现，数据集列表可能为空。
      </Typography.Paragraph>

      {loading && (
        <div style={{ textAlign: 'center', padding: 80 }}>
          <Spin size="large" tip="加载中..." />
        </div>
      )}

      {!loading && error && <ErrorState description={error} />}

      {!loading && !error && (
        <Card title="评测数据集">
          <Table
            rowKey="id"
            columns={columns}
            dataSource={list}
            scroll={{ x: 640 }}
            pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条` }}
            locale={{ emptyText: '暂无评测数据集（接口可能尚未实现）' }}
          />
        </Card>
      )}
    </SideLayout>
  );
}
