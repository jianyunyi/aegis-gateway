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
    MENU_ITEMS.find(
      (item) => pathname === item.key || pathname?.startsWith(`${item.key}/`),
    )?.key ?? '/dashboard';

  // 未登录时不渲染业务内容，避免子页面抢先发起带 token 的请求
  if (!authed) {
    return null;
  }

  return (
    <Layout className="aegis-page-shell" style={{ minHeight: '100vh', background: token.colorBgLayout }}>
      <Sider
        theme="dark"
        breakpoint="lg"
        collapsedWidth={64}
        style={{ background: '#20242a' }}
      >
        <div
          className="aegis-brand"
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 12,
            padding: `0 ${token.padding}px`,
            background: '#20242a',
          }}
        >
          <div className="aegis-brand-mark">A</div>
          <div className="aegis-brand-copy">
            <Typography.Text strong className="aegis-brand-title">
              AEGIS
            </Typography.Text>
            <Typography.Text className="aegis-brand-subtitle">AI GATEWAY</Typography.Text>
          </div>
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
            background: token.colorBgLayout,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: `1px solid ${token.colorBorder}`,
          }}
        >
          <Typography.Text className="aegis-header-context">Gateway Control</Typography.Text>
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
          height: 84px;
          border-bottom: 1px solid rgb(255 247 236 / 12%);
        }

        .aegis-brand-mark {
          width: 42px;
          height: 42px;
          display: grid;
          flex: 0 0 42px;
          place-items: center;
          border-radius: 10px;
          background: #ff6b4a;
          color: #20242a;
          font-size: 19px;
          font-weight: 900;
          line-height: 1;
        }

        .aegis-brand-copy {
          min-width: 0;
          display: flex;
          flex-direction: column;
        }

        .aegis-brand-title.ant-typography {
          color: #fff7ec;
          font-size: 19px;
          letter-spacing: 0.08em;
          line-height: 1.15;
        }

        .aegis-brand-subtitle.ant-typography {
          margin-top: 3px;
          color: rgb(255 247 236 / 58%);
          font-size: 11px;
          letter-spacing: 0.18em;
          line-height: 1.2;
        }

        .ant-layout-sider-collapsed .aegis-brand-copy {
          display: none;
        }

        .aegis-menu.ant-menu-dark {
          padding: 12px 10px;
          background: #20242a;
          border-inline-end: 0;
        }

        .aegis-menu.ant-menu-dark .ant-menu-item {
          height: 38px;
          margin: 4px 0;
          border-radius: 9px;
          color: rgb(255 247 236 / 72%);
        }

        .aegis-menu.ant-menu-dark .ant-menu-item:hover {
          color: #fff7ec;
          background: rgb(255 107 74 / 14%);
        }

        .aegis-menu.ant-menu-dark .ant-menu-item-selected {
          background: #ff6b4a;
          color: #20242a;
          font-weight: 760;
          box-shadow: none;
        }

        .aegis-menu.ant-menu-dark .ant-menu-item-selected .ant-menu-item-icon,
        .aegis-menu.ant-menu-dark .ant-menu-item-selected a {
          color: #20242a;
        }

        .aegis-header {
          height: 64px;
          padding: 0 28px;
        }

        .aegis-header-context.ant-typography {
          color: #84796c;
          font-size: 12px;
          font-weight: 700;
          letter-spacing: 0.12em;
          text-transform: uppercase;
        }

        .aegis-content {
          background:
            linear-gradient(180deg, rgb(255 250 241 / 38%), rgb(243 239 230 / 0%) 220px),
            ${token.colorBgLayout};
        }

        .aegis-loading-panel {
          padding: 80px;
          text-align: center;
        }

        .aegis-mono,
        .aegis-mono.ant-typography {
          font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
        }

        .aegis-surface-card.ant-card,
        .aegis-table-card.ant-card,
        .aegis-metric-card.ant-card {
          border-color: #ded5c7;
          background: #fffaf1;
          box-shadow: 0 14px 36px rgb(34 31 26 / 6%);
        }

        .aegis-table-card.ant-card > .ant-card-head {
          min-height: 58px;
          border-bottom-color: #e4dbcf;
          background: #fffaf1;
        }

        .aegis-table-card .ant-card-head-title {
          font-size: 16px;
          font-weight: 820;
        }

        .aegis-table-card .ant-table {
          background: #fffaf1;
        }

        .aegis-table-card .ant-table-thead > tr > th {
          font-size: 12px;
          font-weight: 700;
          color: #81756a;
        }

        .aegis-table-card .ant-table-tbody > tr > td {
          border-bottom-color: #ebe3d8;
        }

        .aegis-metric-card.ant-card {
          min-height: 118px;
        }

        .aegis-metric-card .ant-statistic-title {
          color: #8b7f73;
          font-size: 12px;
        }

        .aegis-metric-card .ant-statistic-content {
          color: #24211d;
          font-weight: 860;
        }

        .aegis-metric-card-feature.ant-card {
          border-color: #20242a;
          background: #20242a;
        }

        .aegis-metric-card-feature .ant-statistic-title {
          color: rgb(255 247 236 / 62%);
        }

        .aegis-metric-card-feature .ant-statistic-content {
          color: #fff7ec;
        }

        .aegis-page-header {
          display: flex;
          align-items: flex-end;
          justify-content: space-between;
          gap: 24px;
          margin-bottom: 26px;
        }

        .aegis-page-header .ant-typography {
          margin-bottom: 0;
        }

        .aegis-page-header h1.ant-typography,
        .aegis-page-header h2.ant-typography,
        .aegis-page-header h3.ant-typography,
        .aegis-page-header h4.ant-typography {
          margin-top: 4px;
          font-size: 32px;
          line-height: 1.08;
          font-weight: 850;
          letter-spacing: 0;
        }

        .aegis-page-header-kicker.ant-typography {
          display: block;
          color: #8b7f73;
          font-size: 12px;
          font-weight: 760;
          letter-spacing: 0.12em;
          text-transform: uppercase;
        }

        .aegis-page-header-description {
          margin: 6px 0 0 !important;
          color: #6d6257;
          line-height: 1.55;
        }

        .aegis-toolbar {
          display: flex;
          align-items: center;
          justify-content: space-between;
          gap: 16px;
          flex-wrap: wrap;
        }

        .aegis-toolbar + .ant-table-wrapper {
          margin-top: 4px;
        }

        .ant-btn-primary {
          box-shadow: none;
        }

        .ant-typography code {
          border-color: #e2d7ca;
          background: #f7f1e8;
          color: #20242a;
          font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
        }

        .ant-tag {
          font-weight: 650;
        }

        /* 数据表格数字等宽对齐（视觉密度与可读性） */
        .ant-table-tbody > tr > td,
        .ant-statistic-content {
          font-variant-numeric: tabular-nums;
        }

        @media (max-width: 575px) {
          .aegis-brand-copy {
            display: none;
          }

          .aegis-page-header {
            align-items: flex-start;
            flex-direction: column;
            gap: 12px;
            margin-bottom: 16px;
          }

          .aegis-header {
            padding: 0 16px;
          }

          .aegis-header-context.ant-typography {
            display: none;
          }

          .aegis-content {
            padding: ${token.padding}px !important;
          }
        }
      `}</style>
    </Layout>
  );
}
