/**
 * 统一 API 客户端。
 * - baseURL: /api/v1/admin（管理侧 REST 前缀，见 docs/03 第 6.2 节）
 * - 请求拦截器：从 localStorage('aegis_token') 注入 Authorization: Bearer <token>
 * - 响应拦截器：
 *   - 业务错误（data.code !== 0）：message.error + reject
 *   - HTTP 401：清 token 并跳转 /login
 *   - 后端管理接口当前为占位（501），统一走错误分支，页面自行展示 Empty，不崩溃
 */
import axios, { AxiosError } from 'axios';
import { message } from 'antd';
import type { ApiResponse } from './types';

/** localStorage 中保存 JWT 的键名 */
export const TOKEN_KEY = 'aegis_token';

/** 带业务错误码的异常 */
export class ApiError extends Error {
  readonly code: number;

  constructor(message: string, code: number) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
  }
}

/** 从后端响应里尽量提取可读的错误信息 */
function extractMessage(payload: unknown, fallback: string): string {
  if (
    payload &&
    typeof payload === 'object' &&
    'message' in payload &&
    typeof (payload as { message: unknown }).message === 'string'
  ) {
    const msg = (payload as { message: string }).message.trim();
    if (msg) return msg;
  }
  return fallback;
}

const instance = axios.create({
  baseURL: '/api/v1/admin',
  timeout: 20000,
  headers: { 'Content-Type': 'application/json' },
});

// 请求拦截器：附加 JWT
instance.interceptors.request.use((config) => {
  if (typeof window !== 'undefined') {
    const token = localStorage.getItem(TOKEN_KEY);
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
  }
  return config;
});

// 响应拦截器：统一错误处理
instance.interceptors.response.use(
  (response) => {
    const body = response.data as ApiResponse<unknown> | undefined;
    // 非标准响应（后端占位或异常）直接放行交给调用方
    if (!body || typeof body.code !== 'number') {
      return response;
    }
    if (body.code !== 0) {
      const msg = body.message || '请求失败，请稍后重试';
      message.error(msg);
      return Promise.reject(new ApiError(msg, body.code));
    }
    return response;
  },
  (error: AxiosError<ApiResponse<unknown>>) => {
    const status = error.response?.status;
    const payload = error.response?.data;
    const fallback =
      status != null ? `请求失败（HTTP ${status}）` : '网络异常，请检查连接后重试';
    const msg = extractMessage(payload, fallback);

    if (status === 401) {
      // 未认证：清除本地 token，跳转登录页（登录页自身 401 不重复跳转）
      if (typeof window !== 'undefined') {
        localStorage.removeItem(TOKEN_KEY);
        if (!window.location.pathname.startsWith('/login')) {
          window.location.href = '/login';
        }
      }
      message.error(msg);
      return Promise.reject(new ApiError(msg, payload?.code ?? 40101));
    }

    message.error(msg);
    return Promise.reject(new ApiError(msg, payload?.code ?? status ?? -1));
  }
);

/** 泛型请求方法：解包 data，直接返回业务数据 */
export async function get<T>(url: string, params?: Record<string, unknown>): Promise<T> {
  const res = await instance.get(url, { params });
  return (res.data as ApiResponse<T>).data;
}

export async function post<T>(url: string, data?: unknown): Promise<T> {
  const res = await instance.post(url, data);
  return (res.data as ApiResponse<T>).data;
}

export async function put<T>(url: string, data?: unknown): Promise<T> {
  const res = await instance.put(url, data);
  return (res.data as ApiResponse<T>).data;
}

export async function del<T>(url: string, params?: Record<string, unknown>): Promise<T> {
  const res = await instance.delete(url, { params });
  return (res.data as ApiResponse<T>).data;
}

export default instance;
