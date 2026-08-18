'use client';

/**
 * 管理后台侧边栏布局：Sider 菜单 + Header（登出）。
 * - 菜单根据当前路由高亮
 * - 未登录（无 aegis_token）时跳转 /login
 * - 登出：清除 token 后跳转 /login
 */
import { useEffect, useState } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { Button, Layout, Menu, Space, theme, Typography } from 'antd';
import {
  AccountBookOutlined,
  CloudServerOutlined,
  DashboardOutlined,
  ExperimentOutlined,
  FileTextOutlined,
  KeyOutlined,
  LogoutOutlined,
} from '@ant-design/icons';
import { TOKEN_KEY } from '@/lib/api';

const { Header, Sider, Content } = Layout;

const MENU_ITEMS = [
  { key: '/dashboard', icon: <DashboardOutlined />, label: '仪表盘' },
  { key: '/keys', icon: <KeyOutlined />, label: 'Key 管理' },
  { key: '/providers', icon: <CloudServerOutlined />, label: '提供商' },
  { key: '/logs', icon: <FileTextOutlined />, label: '调用日志' },
  { key: '/billing', icon: <AccountBookOutlined />, label: '账单' },
  { key: '/evals', icon: <ExperimentOutlined />, label: '评测' },
];

export default function SideLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [authed, setAuthed] = useState(false);
  const { token } = theme.useToken();

  // 未登录时跳转登录页（客户端挂载后执行，避免 SSR 访问 localStorage）
  useEffect(() => {
    const token = localStorage.getItem(TOKEN_KEY);
    if (!token) {
      router.replace('/login');
      return;
    }
    setAuthed(true);
  }, [router]);

  const handleLogout = () => {
    localStorage.removeItem(TOKEN_KEY);
    router.replace('/login');
  };

  // 菜单高亮：按当前路径前缀匹配
  const selectedKey =
    MENU_ITEMS.find((item) => pathname?.startsWith(item.key))?.key ?? '/dashboard';

  // 未登录时不渲染业务内容，避免子页面抢先发起带 token 的请求
  if (!authed) {
    return null;
  }

  return (
    <Layout style={{ minHeight: '100vh', background: token.colorBgLayout }}>
      <Sider
        theme="dark"
        breakpoint="lg"
        collapsedWidth={64}
        style={{ background: token.colorPrimaryActive }}
      >
        <div
          className="aegis-brand"
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'flex-start',
            justifyContent: 'center',
            padding: `0 ${token.paddingLG}px`,
            background: token.colorPrimaryActive,
          }}
        >
          <Typography.Text strong style={{ color: token.colorWhite, fontSize: 18, letterSpacing: 1 }}>
            AEGIS
          </Typography.Text>
          <Typography.Text
            style={{
              color: token.colorWhite,
              fontSize: token.fontSizeSM,
              letterSpacing: 1.5,
              opacity: 0.68,
            }}
          >
            AI GATEWAY
          </Typography.Text>
        </div>
        <Menu
          className="aegis-menu"
          theme="dark"
          mode="inline"
          selectedKeys={[selectedKey]}
          items={MENU_ITEMS}
          onClick={({ key }) => router.push(key)}
        />
      </Sider>
      <Layout>
        <Header
          className="aegis-header"
          style={{
            background: token.colorBgContainer,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            borderBottom: `1px solid ${token.colorBorderSecondary}`,
          }}
        >
          <Space size="middle">
            <Typography.Text type="secondary">AEGIS 管理后台</Typography.Text>
            <Button type="text" icon={<LogoutOutlined />} onClick={handleLogout}>
              登出
            </Button>
          </Space>
        </Header>
        <Content
          className="aegis-content"
          style={{
            width: '100%',
            maxWidth: 1440,
            margin: '0 auto',
            padding: `${token.paddingLG}px ${token.paddingLG}px`,
          }}
        >
          {children}
        </Content>
      </Layout>
      <style jsx global>{`
        .aegis-brand {
          height: 72px;
        }

        .aegis-menu.ant-menu-dark {
          background: ${token.colorPrimaryActive};
        }

        .aegis-menu.ant-menu-dark .ant-menu-item-selected {
          background: ${token.colorPrimaryBg};
          color: ${token.colorWhite};
          box-shadow: inset 3px 0 0 ${token.colorPrimary};
        }

        .aegis-menu.ant-menu-dark .ant-menu-item-selected .ant-menu-item-icon,
        .aegis-menu.ant-menu-dark .ant-menu-item-selected a {
          color: ${token.colorWhite};
        }

        .aegis-header {
          padding: 0 ${token.paddingLG}px;
        }

        @media (max-width: 575px) {
          .aegis-header {
            padding: 0 ${token.padding}px;
          }

          .aegis-content {
            padding: ${token.padding}px !important;
          }
        }
      `}</style>
    </Layout>
  );
}
