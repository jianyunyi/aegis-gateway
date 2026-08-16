'use client';

/**
 * 统一的错误占位组件：后端管理接口当前多返回 501 占位，
 * 页面捕获异常后渲染 Alert（错误详情）+ Empty（空态），保证不崩溃。
 */
import { Alert, Empty, Space } from 'antd';

interface ErrorStateProps {
  /** 错误标题 */
  title?: string;
  /** 错误详情（来自后端 message 或拦截器） */
  description?: string;
}

export default function ErrorState({
  title = '数据加载失败',
  description,
}: ErrorStateProps) {
  return (
    <Space direction="vertical" style={{ width: '100%' }} size="middle">
      <Alert type="error" showIcon message={title} description={description} />
      <Empty description="暂无可用数据，请稍后重试（管理接口可能尚未实现）" />
    </Space>
  );
}
