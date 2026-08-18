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
          colorPrimary: '#2f6fed',
          colorText: '#10253f',
          colorTextHeading: '#10253f',
          colorBgLayout: '#f3f6fa',
          borderRadius: 10,
          controlHeight: 38,
        },
        components: {
          Layout: {
            bodyBg: '#f3f6fa',
            headerBg: '#ffffff',
            siderBg: '#ffffff',
            triggerBg: '#ffffff',
            triggerColor: '#10253f',
          },
          Menu: {
            itemBg: '#ffffff',
            itemColor: '#52657d',
            itemHoverBg: '#edf3ff',
            itemHoverColor: '#2f6fed',
            itemSelectedBg: '#e5edff',
            itemSelectedColor: '#2f6fed',
            subMenuItemBg: '#ffffff',
          },
          Card: {
            colorBgContainer: '#ffffff',
            headerBg: '#ffffff',
            borderRadiusLG: 10,
          },
          Table: {
            headerBg: '#f3f6fa',
            headerColor: '#10253f',
            rowHoverBg: '#f7f9fc',
            borderColor: '#e5eaf2',
          },
          Button: {
            primaryColor: '#ffffff',
            defaultBg: '#ffffff',
            defaultBorderColor: '#d7e0ed',
            defaultColor: '#10253f',
            borderRadius: 10,
            controlHeight: 38,
          },
          Input: {
            activeBorderColor: '#2f6fed',
            hoverBorderColor: '#7da5f5',
            activeShadow: '0 0 0 2px rgba(47, 111, 237, 0.12)',
            borderRadius: 10,
            controlHeight: 38,
          },
          Modal: {
            contentBg: '#ffffff',
            headerBg: '#ffffff',
            footerBg: '#ffffff',
            titleColor: '#10253f',
            borderRadiusLG: 10,
          },
        },
      }}
    >
      <App>{children}</App>
    </ConfigProvider>
  );
}
