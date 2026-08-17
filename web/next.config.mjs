/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  reactStrictMode: true,
  // 本地开发：/api 代理到网关（生产由部署层反向代理，见 deploy/）
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: process.env.NEXT_PUBLIC_API_PROXY || 'http://localhost:8081/api/:path*',
      },
    ];
  },
};

export default nextConfig;
