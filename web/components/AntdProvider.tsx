'use client';

/**
 * AntD 客户端包装：ConfigProvider（zhCN 中文 locale）+ App（静态方法上下文）。
 * layout.tsx 为服务端组件（需导出 metadata），antd 组件只能在客户端渲染，
 * 因此把 ConfigProvider 下沉到本客户端组件。
 */
import { App, ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';

dayjs.locale('zh-cn');

export default function AntdProvider({ children }: { children: React.ReactNode }) {
  return (
    <ConfigProvider locale={zhCN}>
      <App>{children}</App>
    </ConfigProvider>
  );
}
