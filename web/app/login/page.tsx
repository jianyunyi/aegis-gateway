'use client';

/**
 * 登录页（不套 SideLayout）：
 * - 用户名/密码表单，提交 POST /api/v1/admin/auth/login
 * - 成功后保存 token 至 localStorage('aegis_token') 并跳转 /dashboard
 * - 失败：由 lib/api.ts 响应拦截器统一 message.error（本页无需重复提示）
 */
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { Button, Card, Form, Input, Typography } from 'antd';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { post, TOKEN_KEY } from '@/lib/api';
import type { LoginParams, LoginResult } from '@/lib/types';

interface LoginFormValues extends LoginParams {
  remember?: boolean;
}

export default function LoginPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (values: LoginFormValues) => {
    setLoading(true);
    try {
      const data = await post<LoginResult>('/auth/login', {
        username: values.username,
        password: values.password,
      });
      if (!data?.token) {
        // 后端契约要求返回 token，缺失时按失败处理
        setLoading(false);
        return;
      }
      localStorage.setItem(TOKEN_KEY, data.token);
      router.replace('/dashboard');
    } catch {
      // 错误提示已由响应拦截器处理
      setLoading(false);
    }
  };

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: '#f0f2f5',
      }}
    >
      <Card style={{ width: 380, boxShadow: '0 2px 8px rgba(0,0,0,0.08)' }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Typography.Title level={3} style={{ marginBottom: 4 }}>
            AEGIS 管理后台
          </Typography.Title>
          <Typography.Text type="secondary">AI 网关统一管理平台</Typography.Text>
        </div>
        <Form<LoginFormValues>
          name="login"
          size="large"
          initialValues={{ remember: true }}
          onFinish={handleSubmit}
        >
          <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password
              prefix={<LockOutlined />}
              placeholder="密码"
              autoComplete="current-password"
            />
          </Form.Item>
          <Form.Item style={{ marginBottom: 0 }}>
            <Button type="primary" htmlType="submit" block loading={loading}>
              登录
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
}
