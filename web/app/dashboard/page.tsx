'use client';

/**
 * 仪表盘概览页：
 * - 顶部 4 张统计卡片：今日请求量 / 今日成本 / 今日 Token / 成功率（GET /stats/overview）
 * - 下方 7 日趋势折线图：请求量 + 成本 两条线（GET /stats/trends?range=7d）
 * 接口未实现（501）或失败时展示 ErrorState，不崩溃。
 */
import { useCallback, useEffect, useState } from 'react';
import { Card, Col, Row, Skeleton, Statistic, Typography } from 'antd';
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

  // 主题色板（与 AntdProvider 一致，遵守"单页锁色"）
  const C = {
    ink: '#20242a',
    accent: '#ff6b4a',
    textSecondary: '#6d6257',
    textFaint: '#8b7f73',
    grid: '#e8dfd2',
    axis: '#ded5c7',
    surface: '#fffaf1',
    surfaceAlt: '#f7f1e8',
  };

  const chartOption: EChartsOption = {
    color: [C.ink, C.accent], // 主数据线近黑、成本线 accent，与全站一致
    tooltip: {
      trigger: 'axis',
      backgroundColor: C.surface,
      borderColor: C.axis,
      borderWidth: 1,
      textStyle: { color: C.ink, fontSize: 12 },
      padding: [8, 12],
      valueFormatter: (v) =>
        typeof v === 'number' ? v.toLocaleString('zh-CN', { maximumFractionDigits: 6 }) : String(v),
    },
    legend: {
      data: ['请求量', '成本(元)'],
      textStyle: { color: C.textSecondary },
      itemWidth: 14,
      itemHeight: 3,
    },
    grid: { left: 56, right: 56, top: 48, bottom: 32 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: trends.map((t) => t.date),
      axisLine: { lineStyle: { color: C.axis } },
      axisLabel: { color: C.textFaint },
      axisTick: { show: false },
    },
    yAxis: [
      {
        type: 'value',
        name: '请求量',
        minInterval: 1,
        nameTextStyle: { color: C.textFaint },
        axisLine: { show: false },
        axisLabel: { color: C.textFaint },
        splitLine: { lineStyle: { color: C.grid } },
      },
      {
        type: 'value',
        name: '成本(元)',
        minInterval: 0.01,
        nameTextStyle: { color: C.textFaint },
        axisLine: { show: false },
        axisLabel: { color: C.textFaint },
        splitLine: { show: false },
      },
    ],
    series: [
      {
        name: '请求量',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2 },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(32, 36, 42, 0.08)' },
              { offset: 1, color: 'rgba(32, 36, 42, 0)' },
            ],
          },
        },
        data: trends.map((t) => t.requests),
      },
      {
        name: '成本(元)',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2 },
        yAxisIndex: 1,
        data: trends.map((t) => t.cost),
      },
    ],
  };

  return (
    <SideLayout>
      <div className="aegis-page-header">
        <div>
          <Typography.Text className="aegis-page-header-kicker">Gateway Control</Typography.Text>
          <Typography.Title level={4}>仪表盘</Typography.Title>
          <Typography.Paragraph className="aegis-page-header-description">
            监控今日调用、成本与模型稳定性
          </Typography.Paragraph>
        </div>
      </div>

      {loading && (
        // 骨架屏：形状匹配最终布局（4 张指标卡 + 趋势图），优于通用 spinner
        <>
          <Row gutter={[16, 16]}>
            {[0, 1, 2, 3].map((i) => (
              <Col xs={24} sm={12} lg={6} key={i}>
                <Card className="aegis-metric-card">
                  <Skeleton active title={{ width: 80 }} paragraph={{ rows: 1, width: 120 }} />
                </Card>
              </Col>
            ))}
          </Row>
          <Card className="aegis-table-card" style={{ marginTop: 16 }}>
            <Skeleton active paragraph={{ rows: 6 }} />
          </Card>
        </>
      )}

      {!loading && error && <ErrorState description={error} />}

      {!loading && !error && (
        <>
          <Row gutter={[16, 16]}>
            <Col xs={24} sm={12} lg={6}>
              <Card className="aegis-metric-card aegis-metric-card-feature">
                <Statistic
                  title="今日请求量"
                  value={overview?.today_requests ?? 0}
                  valueStyle={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace' }}
                />
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="aegis-metric-card">
                <Statistic
                  title="今日成本（元）"
                  value={overview?.today_cost ?? 0}
                  precision={4}
                  prefix="¥"
                  valueStyle={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace' }}
                />
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="aegis-metric-card">
                <Statistic
                  title="今日 Token"
                  value={overview?.today_tokens ?? 0}
                  valueStyle={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace' }}
                />
              </Card>
            </Col>
            <Col xs={24} sm={12} lg={6}>
              <Card className="aegis-metric-card">
                <Statistic
                  title="成功率"
                  value={normalizeSuccessRate(overview?.success_rate)}
                  precision={2}
                  suffix="%"
                  valueStyle={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace' }}
                />
              </Card>
            </Col>
          </Row>

          <Card
            className="aegis-table-card"
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
