/**
 * 与后端契约对齐的类型定义。
 * 参考：docs/03-技术方案设计.md「5. 数据库设计」「6. API 契约」，
 * 以及 backend/internal/model 下各 GORM 模型的 json tag。
 */

/** 统一响应包装：code=0 表示成功 */
export interface ApiResponse<T = unknown> {
  code: number;
  data: T;
  message: string;
}

/** 分页结果包装 */
export interface PageResult<T> {
  list: T[];
  total: number;
  page: number;
  page_size: number;
}

/** 登录请求 */
export interface LoginParams {
  username: string;
  password: string;
}

/** 登录响应（JWT） */
export interface LoginResult {
  token: string;
}

/** API Key（与后端 api_keys 表对齐） */
export interface ApiKey {
  id: number;
  name: string;
  key_prefix: string;
  user_id: number;
  status: number; // 1 启用 0 禁用
  quota_tokens: number; // 0 表示不限
  rps_limit: number;
  burst: number;
  expires_at: string | null;
  created_at: string;
  updated_at: string;
}

/** 创建 Key 的请求体 */
export interface ApiKeyCreatePayload {
  name: string;
  rps_limit: number;
  burst: number;
  quota_tokens: number;
  expires_at?: string | null;
}

/** 创建 Key 的响应：明文 Key 仅在此返回一次（ADR-007） */
export interface ApiKeyCreated {
  id: number;
  name: string;
  key: string; // 明文，形如 ak_xxxxxxxx
  key_prefix: string;
  expires_at: string | null;
}

/** 上游提供商（与后端 providers 表对齐） */
export interface Provider {
  id: number;
  name: string;
  base_url: string;
  enabled: number; // 1 启用 0 禁用
  priority: number; // 小者优先
  created_at: string;
  updated_at: string;
}

/** 创建提供商的请求体（api_key 仅提交，不返回） */
export interface ProviderCreatePayload {
  name: string;
  base_url: string;
  api_key: string;
  enabled: number;
  priority: number;
}

/** 模型目录（与后端 models 表对齐） */
export interface Model {
  id: number;
  provider_id: number;
  name: string;
  display_name: string;
  tier: string; // cheap / normal / strong
  context_window: number;
  price_in: number; // 每 1K 输入 token 成本（元）
  price_out: number; // 每 1K 输出 token 成本（元）
  enabled: number;
}

/** 调用日志（与后端 usage_logs 表对齐） */
export interface UsageLog {
  id: number;
  request_id: string;
  api_key_id: number;
  user_id: number;
  provider_id: number;
  model_name: string;
  kind: string; // chat / completion / embedding
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  cost: number; // 元
  latency_ms: number;
  ttft_ms: number | null; // 流式首字延迟
  status: number; // 0 成功 / 4xx / 5xx
  error_code: string;
  cached: number; // 1 缓存命中
  routed_by: string; // manual / rule / heuristic / llm
  upstream_model: string;
  created_at: string;
}

/** 每日账单（与后端 billing_daily 表对齐） */
export interface BillingDaily {
  id: number;
  date: string;
  api_key_id: number;
  request_count: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  cost: number; // 元
  created_at: string;
}

/** 今日概览 */
export interface StatsOverview {
  today_requests: number;
  today_cost: number; // 元
  today_tokens: number;
  success_rate: number; // 0~1（页面按百分比展示）
  avg_latency_ms: number;
}

/** 趋势数据点 */
export interface TrendPoint {
  date: string;
  requests: number;
  cost: number; // 元
  tokens: number;
  success_rate?: number;
}

/** 评测数据集（与后端 eval_datasets 表对齐） */
export interface EvalDataset {
  id: number;
  name: string;
  description: string;
  status: number; // 0 草稿 1 可用
  created_at: string;
  updated_at: string;
}

/** 评测样本（与后端 eval_samples 表对齐） */
export interface EvalSample {
  id: number;
  dataset_id: number;
  prompt: string;
  reference: string;
  source: string; // sampled / manual
  label: number | null; // 1 好 / 0 差 / null 未标
  created_at: string;
}

/** 评测运行（与后端 eval_runs 表对齐） */
export interface EvalRun {
  id: number;
  dataset_id: number;
  model_a: string;
  model_b: string;
  status: number; // 0 运行中 1 完成 2 失败
  score_a: number | null;
  score_b: number | null;
  cost_a: number | null;
  cost_b: number | null;
  latency_a: number | null;
  latency_b: number | null;
  report: string | null; // JSON 字符串（EvalSampleResult[]）
  created_at: string;
  finished_at: string | null;
}

/** 评测单样本结果（与后端 EvalSampleResult 对齐） */
export interface EvalSampleResult {
  index: number;
  prompt: string;
  score_a: number;
  latency_a_ms: number;
  cost_a: number;
  out_a_preview: string;
  score_b: number;
  latency_b_ms: number;
  cost_b: number;
  out_b_preview: string;
}
