# AEGIS — 企业大模型网关与智能观测平台

统一的企业大模型接入网关：**路由、限流、计费、观测、评测**一站式平台。

> 定位：AI 的基础设施层。业务方通过 AEGIS 接入多家大模型，获得统一鉴权、成本控制与可观测能力。
> 技术栈：Go + Gin + Gorm + Redis + MySQL ｜ Next.js + React + TypeScript ｜ Docker

## 快速开始

```bash
# 1. 一键启动全栈（MySQL + Redis + 网关 + 管理后台）
docker compose -f deploy/docker-compose.yml up -d --build

# 2. 健康检查
curl http://localhost:8081/healthz

# 3. 打开管理后台
open http://localhost:3000        # 登录（M3 里程碑可用）
```

本地开发（不经 Docker）：

```bash
docker compose -f deploy/docker-compose.yml up -d mysql redis   # 只起依赖
cd backend && go run ./cmd/gateway                                # 起网关
cd web && npm install && npm run dev                              # 起后台
```

Windows 无 make 时的命令对照：`make up` → `docker compose -f deploy/docker-compose.yml up -d --build`；`make test` → `cd backend; go test ./... -race`。

## 目录结构

```
aegis-gateway/
├── docs/          # 全部项目文档（章程/PRD/技术方案/规范/…）
├── backend/       # Go 网关（cmd / internal / migrations）
├── web/           # Next.js 管理后台
└── deploy/        # docker-compose / Dockerfile
```

## 里程碑状态

| 里程碑 | 内容 | 状态 |
|---|---|---|
| M1 | 文档 + 工程骨架（本目录） | 🔄 进行中 |
| M2 | 代理链路 + Key 管理 + 限流 | ⏳ |
| M3 | 计费 + 日志 + 管理后台大盘 | ⏳ |
| M4 | 语义路由 + 缓存 + 预算降级 | ⏳ |
| M5 | 评测飞轮 + 测试 + CI + 部署 + 简历 | ⏳ |

详细设计见 [docs/README.md](./docs/README.md)。
