import type { Metadata } from 'next';
import AntdProvider from '@/components/AntdProvider';

export const metadata: Metadata = {
  title: 'AEGIS 管理后台',
  description: 'AEGIS AI 网关统一管理后台：大盘 / Key / 提供商 / 日志 / 账单 / 评测',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body>
        <AntdProvider>{children}</AntdProvider>
      </body>
    </html>
  );
}
