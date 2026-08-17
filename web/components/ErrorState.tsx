'use client';

/**
 * 统一的错误占位组件：渲染 Alert（错误详情）+ Empty（空态），保证页面不崩溃。
 * 支持可选的重试按钮（onRetry）。
 */
import { Alert, Button, Empty, Space } from 'antd';

interface ErrorStateProps {
  /** 错误标题 */
  title?: string;
  /** 错误详情（来自后端 message 或拦截器） */
  description?: string;
  /** 重试回调（可选） */
  onRetry?: () => void;
}

export default function ErrorState({
  title = '数据加载失败',
  description,
  onRetry,
}: ErrorStateProps) {
  return (
    <Space direction="vertical" style={{ width: '100%' }} size="middle">
      <Alert
        type="error"
        showIcon
        message={title}
        description={description}
        action={onRetry ? <Button size="small" onClick={onRetry}>重试</Button> : undefined}
      />
      <Empty description="暂无可用数据，请稍后重试（管理接口可能尚未实现）" />
    </Space>
  );
}
