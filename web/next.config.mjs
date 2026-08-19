/** @type {import('next').NextConfig} */
// 双模式：
// - 默认 standalone（Docker 容器内 node 运行）
// - NEXT_OUTPUT=export 时静态导出（Cloudflare Pages 部署，HTML 边缘服务，TTFB 最优）
const isExport = process.env.NEXT_OUTPUT === 'export';

const nextConfig = {
  output: isExport ? 'export' : 'standalone',
  reactStrictMode: true,
  // 静态导出不支持 rewrites；API 已通过 NEXT_PUBLIC_API_BASE 直连公网网关，无需代理
  ...(isExport
    ? {}
    : {
        // 本地开发：/api 代理到网关（生产由浏览器直连公网网关，见 deploy/）
        async rewrites() {
          return [
            {
              source: '/api/:path*',
              destination: process.env.NEXT_PUBLIC_API_PROXY || 'http://localhost:8081/api/:path*',
            },
          ];
        },
      }),
};

export default nextConfig;
