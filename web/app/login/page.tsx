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
        position: 'relative',
        overflow: 'hidden',
        background: 'linear-gradient(160deg, #f3efe6 0%, #ece2d3 100%)',
      }}
    >
      {/* 品牌几何装饰（克制，非图片） */}
      <div
        style={{
          position: 'absolute',
          top: -120,
          right: -120,
          width: 360,
          height: 360,
          borderRadius: '50%',
          border: '1.5px solid rgba(32,36,42,0.10)',
        }}
      />
      <div
        style={{
          position: 'absolute',
          top: -60,
          right: -60,
          width: 220,
          height: 220,
          borderRadius: '50%',
          border: '1.5px solid rgba(255,107,74,0.22)',
        }}
      />
      <div
        style={{
          position: 'absolute',
          bottom: -160,
          left: -100,
          width: 420,
          height: 420,
          borderRadius: '50%',
          border: '1.5px solid rgba(32,36,42,0.07)',
        }}
      />

      <Card
        style={{
          width: 380,
          borderRadius: 14,
          borderColor: '#ded5c7',
          boxShadow: '0 24px 64px -24px rgba(32,36,42,0.28), 0 4px 16px rgba(32,36,42,0.06)',
          background: '#fffaf1',
        }}
      >
        <div style={{ textAlign: 'center', marginBottom: 28 }}>
          <div
            style={{
              width: 44,
              height: 44,
              margin: '0 auto 14px',
              borderRadius: 10,
              background: '#20242a',
              color: '#fff7ec',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontWeight: 800,
              fontSize: 20,
              letterSpacing: '-0.02em',
            }}
          >
            A
          </div>
          <Typography.Title level={3} style={{ marginBottom: 4, letterSpacing: '-0.01em' }}>
            AEGIS 管理后台
          </Typography.Title>
          <Typography.Text type="secondary" style={{ fontSize: 12, letterSpacing: '0.12em', textTransform: 'uppercase' }}>
            AI Gateway Console
          </Typography.Text>
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
