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
    <ConfigProvider
      locale={zhCN}
      theme={{
        token: {
          colorPrimary: '#20242a',
          colorInfo: '#ff6b4a',
          colorText: '#24211d',
          colorTextHeading: '#24211d',
          colorTextSecondary: '#6d6257',
          colorBorder: '#ded5c7',
          colorBorderSecondary: '#e8dfd2',
          colorBgLayout: '#f3efe6',
          colorBgContainer: '#fffaf1',
          colorFillAlter: '#f7f1e8',
          borderRadius: 10,
          controlHeight: 38,
          fontFamily:
            '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
        },
        components: {
          Layout: {
            bodyBg: '#f3efe6',
            headerBg: '#f3efe6',
            siderBg: '#20242a',
            triggerBg: '#20242a',
            triggerColor: '#fff7ec',
          },
          Menu: {
            darkItemBg: '#20242a',
            darkSubMenuItemBg: '#20242a',
            darkItemColor: 'rgba(255, 247, 236, 0.72)',
            darkItemHoverBg: 'rgba(255, 107, 74, 0.14)',
            darkItemHoverColor: '#fff7ec',
            darkItemSelectedBg: '#ff6b4a',
            darkItemSelectedColor: '#20242a',
            itemBorderRadius: 9,
          },
          Card: {
            colorBgContainer: '#fffaf1',
            headerBg: '#fffaf1',
            borderRadiusLG: 10,
            colorBorderSecondary: '#ded5c7',
          },
          Table: {
            headerBg: '#f7f1e8',
            headerColor: '#6d6257',
            rowHoverBg: '#fff4e9',
            borderColor: '#e8dfd2',
            footerBg: '#f7f1e8',
          },
          Button: {
            primaryColor: '#fff7ec',
            defaultBg: '#fffaf1',
            defaultBorderColor: '#d8cfc3',
            defaultColor: '#24211d',
            borderRadius: 10,
            controlHeight: 38,
          },
          Input: {
            colorBgContainer: '#fffaf1',
            activeBorderColor: '#ff6b4a',
            hoverBorderColor: '#d88a73',
            activeShadow: '0 0 0 2px rgba(255, 107, 74, 0.14)',
            borderRadius: 10,
            controlHeight: 38,
          },
          Modal: {
            contentBg: '#fffaf1',
            headerBg: '#fffaf1',
            footerBg: '#fffaf1',
            titleColor: '#24211d',
            borderRadiusLG: 10,
          },
          Drawer: {
            colorBgElevated: '#fffaf1',
          },
          Tabs: {
            itemSelectedColor: '#20242a',
            itemHoverColor: '#ff6b4a',
            inkBarColor: '#ff6b4a',
          },
          Alert: {
            colorInfoBg: '#fff4e9',
            colorInfoBorder: '#ffd4c7',
          },
          Tag: {
            borderRadiusSM: 999,
          },
        },
      }}
    >
      <App>{children}</App>
    </ConfigProvider>
  );
}
