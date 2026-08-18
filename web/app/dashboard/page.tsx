'use client';

/**
 * 仪表盘概览页：
 * - 顶部 4 张统计卡片：今日请求量 / 今日成本 / 今日 Token / 成功率（GET /stats/overview）
 * - 下方 7 日趋势折线图：请求量 + 成本 两条线（GET /stats/trends?range=7d）
 * 接口未实现（501）或失败时展示 ErrorState，不崩溃。
 */
import { useCallback, useEffect, useState } from 'react';
import { Card, Col, Row, Spin, Statistic, Typography } from 'antd';
import ReactECharts from 'echarts-for-react';
import type { EChartsOption } from 'echarts';
import SideLayout from '@/components/SideLayout';
import ErrorState from '@/components/ErrorState';
import { get } from '@/lib/api';
import type { StatsOverview, TrendPoint } from '@/lib/types';

/** 成功率规范化：后端可能返回 0~1 或 0~100 */
function normalizeSuccessRate(rate: number | undefined): number {
  if (rate == null) return 0;
  return rate > 1 ? rate : rate * 100;
}

export default function DashboardPage() {
  const [overview, setOverview] = useState<StatsOverview | null>(null);
  const [trends, setTrends] = useState<TrendPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [ov, tr] = await Promise.all([
        get<StatsOverview>('/stats/overview'),
        get<TrendPoint[]>('/stats/trends', { range: '7d' }),
      ]);
      setOverview(ov ?? null);
      setTrends(Array.isArray(tr) ? tr : []);
    } catch (e) {
      setError(e instanceof Error ? e.message : '数据加载失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const chartOption: EChartsOption = {
    tooltip: { trigger: 'axis' },
    legend: { data: ['请求量', '成本(元)'] },
    grid: { left: 56, right: 56, top: 48, bottom: 32 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: trends.map((t) => t.date),
    },
    yAxis: [
      { type: 'value', name: '请求量', minInterval: 1 },
      { type: 'value', name: '成本(元)', minInterval: 0.01 },
    ],
    series: [
      {
        name: '请求量',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: trends.map((t) => t.requests),
      },
      {
        name: '成本(元)',
        type: 'line',
        smooth: true,
        showSymbol: false,
        yAxisIndex: 1,
        data: trends.map((t) => t.cost),
      },
    ],
  };

  return (
    <SideLayout>
      <div className="aegis-page-header">
        <div>
          <Typography.Title level={4}>仪表盘</Typography.Title>
          <Typography.Paragraph className="aegis-page-header-description">
            监控今日调用、成本与模型稳定性
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
        <>
          <Row gutter={[16, 16]}>
            <Col xs={24} sm={12} lg={6}>
              <Card>
                <Statistic title="今日请求量" value={overview?.today_requests ?? 0} />
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card>
                <Statistic
                  title="今日成本（元）"
                  value={overview?.today_cost ?? 0}
                  precision={4}
                  prefix="¥"
                />
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card>
                <Statistic title="今日 Token" value={overview?.today_tokens ?? 0} />
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card>
                <Statistic
                  title="成功率"
                  value={normalizeSuccessRate(overview?.success_rate)}
                  precision={2}
                  suffix="%"
                />
              </Card>
            </Col>
          </Row>

          <Card
            title="近 7 日趋势"
            style={{ marginTop: 16 }}
            extra={
              <Typography.Text type="secondary">
                数据来源：GET /stats/trends?range=7d
              </Typography.Text>
            }
          >
            {trends.length === 0 ? (
              <ErrorState description="趋势数据为空（接口可能尚未实现）" />
            ) : (
              <ReactECharts option={chartOption} style={{ height: 360 }} />
            )}
          </Card>
        </>
      )}
    </SideLayout>
  );
}
