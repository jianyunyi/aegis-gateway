'use client';

/**
 * 表格型页面的统一加载骨架（taste-skill：loading 需匹配最终布局形状）。
 * 形态 = 卡片 + 可选标题工具条 + 表格行骨架，替代通用 spinner。
 */
import { Card, Skeleton } from 'antd';

interface TableSkeletonProps {
  /** 骨架行数 */
  rows?: number;
  /** 是否渲染顶部标题/工具条骨架 */
  toolbar?: boolean;
}

export default function TableSkeleton({ rows = 8, toolbar = true }: TableSkeletonProps) {
  return (
    <Card className="aegis-table-card">
      {toolbar && (
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: 16,
          }}
        >
          <Skeleton.Button active size="small" style={{ width: 140 }} />
          <Skeleton.Button active size="small" style={{ width: 96 }} />
        </div>
      )}
      <Skeleton active title={false} paragraph={{ rows, width: '100%' }} />
    </Card>
  );
}
