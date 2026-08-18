'use client';

/**
 * 评测页（完整交互版）：
 * - 数据集管理：新建 / 选中
 * - 样本管理：从真实日志采样、手动添加、人工打标（好/差）
 * - 评测运行：发起 A/B 回归（质量分 + 成本 + 延迟）、查看逐样本报告与结论
 * 接口：/evals/datasets、/evals/datasets/:id/samples、/evals/datasets/:id/sample、
 *       /evals/samples、/evals/samples/:id/label、/evals/runs、/evals/runs/:id/report
 */
import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  ExperimentOutlined,
  PlusOutlined,
  ReloadOutlined,
  RocketOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import type { ColumnsType } from 'antd/es/table';
import SideLayout from '@/components/SideLayout';
import ErrorState from '@/components/ErrorState';
import { get, post } from '@/lib/api';
import type {
  EvalDataset,
  EvalRun,
  EvalSample,
  EvalSampleResult,
  Model,
} from '@/lib/types';

function fmt(value?: string | null): string {
  if (!value) return '-';
  const d = dayjs(value);
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : value;
}

const fmtCost = (v?: number | null) => (v == null ? '-' : `¥${v.toFixed(6)}`);
const fmtScore = (v?: number | null) => (v == null ? '-' : v.toFixed(1));

export default function EvalsPage() {
  const [datasets, setDatasets] = useState<EvalDataset[]>([]);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [samples, setSamples] = useState<EvalSample[]>([]);
  const [runs, setRuns] = useState<EvalRun[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [loadingDs, setLoadingDs] = useState(true);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState('samples');

  // 弹窗状态
  const [createOpen, setCreateOpen] = useState(false);
  const [sampleOpen, setSampleOpen] = useState(false);
  const [addSampleOpen, setAddSampleOpen] = useState(false);
  const [runOpen, setRunOpen] = useState(false);
  const [reportRun, setReportRun] = useState<EvalRun | null>(null);
  const [reportDetail, setReportDetail] = useState<EvalSampleResult[]>([]);
  const [reportOpen, setReportOpen] = useState(false);
  const [reportLoading, setReportLoading] = useState(false);

  const [createForm] = Form.useForm();
  const [sampleForm] = Form.useForm();
  const [addForm] = Form.useForm();
  const [runForm] = Form.useForm();

  const selected = useMemo(
    () => datasets.find((d) => d.id === selectedId) ?? null,
    [datasets, selectedId]
  );

  const loadDatasets = useCallback(async () => {
    try {
      const data = await get<EvalDataset[]>('/evals/datasets');
      setDatasets(Array.isArray(data) ? data : []);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : '数据集加载失败');
    } finally {
      setLoadingDs(false);
    }
  }, []);

  const loadModels = useCallback(async () => {
    try {
      const data = await get<Model[]>('/models');
      setModels(Array.isArray(data) ? data : []);
    } catch {
      // 模型下拉加载失败不阻塞页面
    }
  }, []);

  const loadDetail = useCallback(async (id: number) => {
    setLoadingDetail(true);
    try {
      const [s, r] = await Promise.all([
        get<EvalSample[]>(`/evals/datasets/${id}/samples`),
        get<EvalRun[]>(`/evals/runs`),
      ]);
      setSamples(Array.isArray(s) ? s : []);
      setRuns(Array.isArray(r) ? r.filter((x) => x.dataset_id === id) : []);
    } catch (e) {
      message.error(e instanceof Error ? e.message : '样本/运行加载失败');
    } finally {
      setLoadingDetail(false);
    }
  }, []);

  useEffect(() => {
    void loadDatasets();
    void loadModels();
  }, [loadDatasets, loadModels]);

  // 选中数据集后加载样本与运行
  useEffect(() => {
    if (selectedId != null) {
      void loadDetail(selectedId);
    }
  }, [selectedId, loadDetail]);

  // ---- 数据集 ----
  const handleCreateDataset = async () => {
    const v = await createForm.validateFields();
    await post('/evals/datasets', { name: v.name, description: v.description ?? '' });
    message.success('数据集已创建');
    setCreateOpen(false);
    createForm.resetFields();
    await loadDatasets();
  };

  // ---- 样本 ----
  const handleSample = async () => {
    if (selectedId == null) return;
    const v = await sampleForm.validateFields();
    const res = await post<{ added: number }>(`/evals/datasets/${selectedId}/sample`, {
      count: v.count ?? 10,
      model_name: v.model_name ?? '',
    });
    message.success(`已从真实调用日志采样 ${res.added} 条`);
    setSampleOpen(false);
    sampleForm.resetFields();
    await loadDetail(selectedId);
  };

  const handleAddSample = async () => {
    if (selectedId == null) return;
    const v = await addForm.validateFields();
    await post('/evals/samples', {
      dataset_id: selectedId,
      prompt: v.prompt,
      reference: v.reference ?? '',
    });
    message.success('样本已添加');
    setAddSampleOpen(false);
    addForm.resetFields();
    await loadDetail(selectedId);
  };

  const handleLabel = async (sampleId: number, label: number) => {
    await post(`/evals/samples/${sampleId}/label`, { label });
    if (selectedId != null) await loadDetail(selectedId);
  };

  // ---- 评测运行 ----
  const handleRun = async () => {
    if (selectedId == null) return;
    const v = await runForm.validateFields();
    await post('/evals/runs', {
      dataset_id: selectedId,
      model_a: v.model_a,
      model_b: v.model_b,
    });
    message.success('A/B 回归评测已完成');
    setRunOpen(false);
    runForm.resetFields();
    if (selectedId != null) await loadDetail(selectedId);
  };

  const openReport = async (run: EvalRun) => {
    setReportRun(run);
    setReportOpen(true);
    setReportLoading(true);
    try {
      const data = await get<EvalRun>(`/evals/runs/${run.id}/report`);
      let detail: EvalSampleResult[] = [];
      if (data.report) {
        try {
          detail = JSON.parse(data.report) as EvalSampleResult[];
        } catch {
          detail = [];
        }
      }
      setReportDetail(detail);
    } catch (e) {
      message.error(e instanceof Error ? e.message : '报告加载失败');
    } finally {
      setReportLoading(false);
    }
  };

  // ---- 报告结论 ----
  const reportConclusion = useMemo(() => {
    if (!reportRun || reportRun.status !== 1) return null;
    const { score_a, score_b, cost_a, cost_b } = reportRun;
    if (score_a == null || score_b == null || cost_a == null || cost_b == null) return null;
    const betterQuality = score_a > score_b ? 'A' : score_b > score_a ? 'B' : null;
    const cheaper = cost_a < cost_b ? 'A' : cost_b < cost_a ? 'B' : null;
    const qualityDiff = Math.abs(score_a - score_b).toFixed(1);
    const costRatio = cheaper ? (Math.max(cost_a, cost_b) / Math.min(cost_a, cost_b)).toFixed(2) : null;
    if (betterQuality && cheaper && betterQuality === cheaper) {
      return `模型 ${betterQuality} 质量更高（差 ${qualityDiff} 分）且成本更低（省 ${costRatio} 倍），建议采用 ${betterQuality}。`;
    }
    if (betterQuality && cheaper) {
      return `质量上 ${betterQuality} 更优（差 ${qualityDiff} 分），但成本上 ${cheaper} 更省（${costRatio} 倍）。需要按业务目标权衡。`;
    }
    if (betterQuality) {
      return `质量上 ${betterQuality} 更优（差 ${qualityDiff} 分），两者成本相当。`;
    }
    if (cheaper) {
      return `质量相当，成本上 ${cheaper} 更省（${costRatio} 倍），建议采用 ${cheaper}。`;
    }
    return '两者质量与成本基本相当，可任选或做更大样本量复测。';
  }, [reportRun]);

  // ---- 表格列 ----
  const datasetColumns: ColumnsType<EvalDataset> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      ellipsis: true,
      render: (v: string, r) => (
        <a onClick={() => setSelectedId(r.id)}>{v}</a>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 80,
      render: (v: number) =>
        v === 1 ? <Tag color="success">可用</Tag> : <Tag>草稿</Tag>,
    },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', width: 150, render: fmt },
  ];

  const sampleColumns: ColumnsType<EvalSample> = [
    {
      title: 'Prompt',
      dataIndex: 'prompt',
      key: 'prompt',
      ellipsis: true,
      render: (v: string) => <Typography.Text copyable={{ text: v }}>{v}</Typography.Text>,
    },
    {
      title: '来源',
      dataIndex: 'source',
      key: 'source',
      width: 90,
      render: (v: string) =>
        v === 'sampled' ? <Tag color="blue">真实采样</Tag> : <Tag>手动</Tag>,
    },
    {
      title: '标注',
      dataIndex: 'label',
      key: 'label',
      width: 110,
      render: (v: number | null) =>
        v == null ? <Tag>未标注</Tag> : v === 1 ? <Tag color="success">好</Tag> : <Tag color="error">差</Tag>,
    },
    {
      title: '操作',
      key: 'action',
      width: 180,
      render: (_, r) => (
        <Space size={4}>
          <Button
            size="small"
            type={r.label === 1 ? 'primary' : 'default'}
            onClick={() => void handleLabel(r.id, 1)}
          >
            标好
          </Button>
          <Button
            size="small"
            danger={r.label === 0}
            type={r.label === 0 ? 'primary' : 'default'}
            onClick={() => void handleLabel(r.id, 0)}
          >
            标差
          </Button>
        </Space>
      ),
    },
  ];

  const runColumns: ColumnsType<EvalRun> = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
    {
      title: '模型对比',
      key: 'models',
      width: 220,
      render: (_, r) => (
        <Space size={4}>
          <Tag color="blue">{r.model_a}</Tag>
          <span>vs</span>
          <Tag color="purple">{r.model_b}</Tag>
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 90,
      render: (v: number) =>
        v === 1 ? <Tag color="success">完成</Tag> : v === 2 ? <Tag color="error">失败</Tag> : <Tag>运行中</Tag>,
    },
    {
      title: '质量分 A/B',
      key: 'score',
      width: 120,
      render: (_, r) => `${fmtScore(r.score_a)} / ${fmtScore(r.score_b)}`,
    },
    {
      title: '成本 A/B',
      key: 'cost',
      width: 160,
      render: (_, r) => `${fmtCost(r.cost_a)} / ${fmtCost(r.cost_b)}`,
    },
    {
      title: '平均延迟 A/B',
      key: 'latency',
      width: 160,
      render: (_, r) => `${r.latency_a ?? '-'}ms / ${r.latency_b ?? '-'}ms`,
    },
    { title: '完成时间', dataIndex: 'finished_at', key: 'finished_at', width: 160, render: fmt },
    {
      title: '操作',
      key: 'action',
      width: 90,
      render: (_, r) => (
        <Button size="small" onClick={() => void openReport(r)} disabled={r.status !== 1}>
          查看报告
        </Button>
      ),
    },
  ];

  const reportDetailColumns: ColumnsType<EvalSampleResult> = [
    { title: '#', dataIndex: 'index', key: 'index', width: 50 },
    { title: 'Prompt', dataIndex: 'prompt', key: 'prompt', ellipsis: true },
    {
      title: 'A 质量分',
      dataIndex: 'score_a',
      key: 'score_a',
      width: 100,
      render: (v: number) => fmtScore(v),
    },
    {
      title: 'A 延迟/成本',
      key: 'a_meta',
      width: 160,
      render: (_, r) => `${r.latency_a_ms}ms / ¥${r.cost_a.toFixed(6)}`,
    },
    {
      title: 'B 质量分',
      dataIndex: 'score_b',
      key: 'score_b',
      width: 100,
      render: (v: number) => fmtScore(v),
    },
    {
      title: 'B 延迟/成本',
      key: 'b_meta',
      width: 160,
      render: (_, r) => `${r.latency_b_ms}ms / ¥${r.cost_b.toFixed(6)}`,
    },
  ];

  return (
    <SideLayout>
      <div className="aegis-page-header">
        <div>
          <Typography.Title level={4}>评测</Typography.Title>
          <Typography.Paragraph className="aegis-page-header-description">
            从真实调用采样、人工打标到 A/B 回归，沉淀模型选型依据
          </Typography.Paragraph>
        </div>
      </div>

      {loadingDs && (
        <div style={{ textAlign: 'center', padding: 80 }}>
          <Spin size="large" />
        </div>
      )}

      {!loadingDs && error && <ErrorState description={error} onRetry={() => void loadDatasets()} />}

      {!loadingDs && !error && (
        <Row gutter={[20, 20]}>
          {/* 左侧：数据集 */}
          <Col xs={24} lg={8}>
            <Card
              title="评测数据集"
              extra={
                <Space>
                  <Button
                    size="small"
                    icon={<ReloadOutlined />}
                    onClick={() => void loadDatasets()}
                  />
                  <Button
                    size="small"
                    type="primary"
                    icon={<PlusOutlined />}
                    onClick={() => setCreateOpen(true)}
                  >
                    新建
                  </Button>
                </Space>
              }
            >
              <Table
                rowKey="id"
                size="small"
                columns={datasetColumns}
                dataSource={datasets}
                pagination={{ pageSize: 6 }}
                rowClassName={(r) => (r.id === selectedId ? 'ant-table-row-selected' : '')}
                onRow={(r) => ({ onClick: () => setSelectedId(r.id) })}
                locale={{ emptyText: '暂无数据集' }}
              />
            </Card>
          </Col>

          {/* 右侧：样本与评测 */}
          <Col xs={24} lg={16}>
            {!selected ? (
              <Card>
                <Empty description="请先选择或创建一个数据集" />
              </Card>
            ) : (
              <Card
                title={
                  <Space>
                    <ExperimentOutlined />
                    <span>{selected.name}</span>
                    <Tag color="blue">数据集 #{selected.id}</Tag>
                  </Space>
                }
              >
                <Tabs
                  activeKey={activeTab}
                  onChange={setActiveTab}
                  items={[
                    {
                      key: 'samples',
                      label: `样本（${samples.length}）`,
                      children: (
                        <Spin spinning={loadingDetail}>
                          <Space style={{ marginBottom: 12 }} wrap>
                            <Button
                              icon={<ReloadOutlined />}
                              onClick={() => void loadDetail(selected.id)}
                            >
                              刷新
                            </Button>
                            <Button type="primary" onClick={() => setSampleOpen(true)}>
                              从真实日志采样
                            </Button>
                            <Button onClick={() => setAddSampleOpen(true)}>手动添加样本</Button>
                          </Space>
                          <Table
                            rowKey="id"
                            size="small"
                            columns={sampleColumns}
                            dataSource={samples}
                            pagination={{ pageSize: 8, showTotal: (t) => `共 ${t} 条` }}
                            locale={{ emptyText: '暂无样本，可从真实日志采样或手动添加' }}
                          />
                        </Spin>
                      ),
                    },
                    {
                      key: 'runs',
                      label: `评测运行（${runs.length}）`,
                      children: (
                        <Spin spinning={loadingDetail}>
                          <Space style={{ marginBottom: 12 }}>
                            <Button
                              type="primary"
                              icon={<RocketOutlined />}
                              onClick={() => setRunOpen(true)}
                            >
                              发起 A/B 评测
                            </Button>
                            <Button icon={<ReloadOutlined />} onClick={() => void loadDetail(selected.id)}>
                              刷新
                            </Button>
                          </Space>
                          <Table
                            rowKey="id"
                            size="small"
                            columns={runColumns}
                            dataSource={runs}
                            pagination={{ pageSize: 6 }}
                            locale={{ emptyText: '暂无评测运行' }}
                            scroll={{ x: 1000 }}
                          />
                        </Spin>
                      ),
                    },
                  ]}
                />
              </Card>
            )}
          </Col>
        </Row>
      )}

      {/* 新建数据集 */}
      <Modal
        title="新建评测数据集"
        open={createOpen}
        onOk={() => void handleCreateDataset()}
        onCancel={() => setCreateOpen(false)}
        destroyOnClose
      >
        <Form form={createForm} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="如：后端八股测试集" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} placeholder="描述用途与来源" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 采样 */}
      <Modal
        title="从真实调用日志采样"
        open={sampleOpen}
        onOk={() => void handleSample()}
        onCancel={() => setSampleOpen(false)}
        destroyOnClose
      >
        <Form form={sampleForm} layout="vertical">
          <Form.Item
            name="count"
            label="采样数量"
            initialValue={10}
            rules={[{ required: true, message: '请输入数量' }]}
          >
            <InputNumber min={1} max={100} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="model_name" label="按模型筛选（可选）">
            <Select
              allowClear
              placeholder="全部模型"
              options={models.map((m) => ({ value: m.name, label: `${m.name}（${m.tier}）` }))}
            />
          </Form.Item>
        </Form>
      </Modal>

      {/* 手动添加样本 */}
      <Modal
        title="手动添加样本"
        open={addSampleOpen}
        onOk={() => void handleAddSample()}
        onCancel={() => setAddSampleOpen(false)}
        destroyOnClose
      >
        <Form form={addForm} layout="vertical">
          <Form.Item name="prompt" label="Prompt" rules={[{ required: true, message: '请输入 prompt' }]}>
            <Input.TextArea rows={4} />
          </Form.Item>
          <Form.Item name="reference" label="参考答案（可选）">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 发起 A/B */}
      <Modal
        title="发起 A/B 回归评测"
        open={runOpen}
        onOk={() => void handleRun()}
        onCancel={() => setRunOpen(false)}
        destroyOnClose
      >
        <Form form={runForm} layout="vertical">
          <Form.Item label="数据集">
            <Input value={`${selected?.name ?? ''}（#{${selectedId ?? ''}}）`} disabled />
          </Form.Item>
          <Form.Item name="model_a" label="模型 A" rules={[{ required: true, message: '请选择模型 A' }]}>
            <Select
              placeholder="选择模型"
              options={models.map((m) => ({ value: m.name, label: `${m.name}（${m.tier}）` }))}
            />
          </Form.Item>
          <Form.Item name="model_b" label="模型 B" rules={[{ required: true, message: '请选择模型 B' }]}>
            <Select
              placeholder="选择模型"
              options={models.map((m) => ({ value: m.name, label: `${m.name}（${m.tier}）` }))}
            />
          </Form.Item>
        </Form>
      </Modal>

      {/* 评测报告 */}
      <Drawer
        title={`评测报告 #${reportRun?.id ?? ''}：${reportRun?.model_a ?? ''} vs ${reportRun?.model_b ?? ''}`}
        width={720}
        open={reportOpen}
        onClose={() => setReportOpen(false)}
      >
        {reportLoading ? (
          <div style={{ textAlign: 'center', padding: 60 }}>
            <Spin />
          </div>
        ) : reportRun && reportRun.status === 1 ? (
          <>
            <Row gutter={16} style={{ marginBottom: 16 }}>
              <Col span={6}>
                <Card size="small">
                  <Statistic title={`${reportRun.model_a} 质量分`} value={reportRun.score_a ?? 0} precision={1} />
                </Card>
              </Col>
              <Col span={6}>
                <Card size="small">
                  <Statistic title={`${reportRun.model_b} 质量分`} value={reportRun.score_b ?? 0} precision={1} />
                </Card>
              </Col>
              <Col span={6}>
                <Card size="small">
                  <Statistic title={`${reportRun.model_a} 成本`} value={reportRun.cost_a ?? 0} precision={6} prefix="¥" />
                </Card>
              </Col>
              <Col span={6}>
                <Card size="small">
                  <Statistic title={`${reportRun.model_b} 成本`} value={reportRun.cost_b ?? 0} precision={6} prefix="¥" />
                </Card>
              </Col>
            </Row>
            <Descriptions size="small" column={3} style={{ marginBottom: 16 }}>
              <Descriptions.Item label="平均延迟 A">
                {reportRun.latency_a ?? '-'} ms
              </Descriptions.Item>
              <Descriptions.Item label="平均延迟 B">
                {reportRun.latency_b ?? '-'} ms
              </Descriptions.Item>
              <Descriptions.Item label="样本数">{reportDetail.length}</Descriptions.Item>
            </Descriptions>
            {reportConclusion && (
              <Alert
                type="success"
                showIcon
                message="评测结论"
                description={reportConclusion}
                style={{ marginBottom: 16 }}
              />
            )}
            <Typography.Title level={5}>逐样本明细</Typography.Title>
            <Table
              rowKey="index"
              size="small"
              columns={reportDetailColumns}
              dataSource={reportDetail}
              pagination={{ pageSize: 8 }}
              expandable={{
                expandedRowRender: (r) => (
                  <Row gutter={16}>
                    <Col span={12}>
                      <Typography.Text type="secondary">模型 A 输出：</Typography.Text>
                      <Typography.Paragraph style={{ whiteSpace: 'pre-wrap' }}>
                        {r.out_a_preview || '-'}
                      </Typography.Paragraph>
                    </Col>
                    <Col span={12}>
                      <Typography.Text type="secondary">模型 B 输出：</Typography.Text>
                      <Typography.Paragraph style={{ whiteSpace: 'pre-wrap' }}>
                        {r.out_b_preview || '-'}
                      </Typography.Paragraph>
                    </Col>
                  </Row>
                ),
              }}
            />
          </>
        ) : (
          <Empty description="该评测运行未完成或不存在" />
        )}
      </Drawer>
    </SideLayout>
  );
}
