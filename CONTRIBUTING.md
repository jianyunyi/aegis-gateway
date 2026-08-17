# 参与贡献（Contributing）

感谢你愿意为 AEGIS 贡献力量！请先阅读 [开发规范](docs/04-开发规范.md)。

## 开发环境

```bash
# 1. 启动依赖（MySQL :3303 / Redis :6379）
docker compose -f deploy/docker-compose.yml up -d mysql redis

# 2. 本地起网关（:8081）
cd backend && go run ./cmd/gateway

# 3. 本地起后台（:3000，/api 自动代理到 8081）
cd web && npm install && npm run dev
```

## 提交流程

1. Fork 本仓库，从 `main` 新建功能分支：`feat/<模块>-<简述>` 或 `fix/<简述>`
2. 遵循 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/)：
   `feat(proxy): 描述` / `fix(billing): 描述` / `docs: 描述`
3. 本地验证通过后再提交 PR：
   ```bash
   cd backend && go vet ./... && go test ./... -race -count=1
   cd web && npm run build
   ```
4. 提交 PR 时说明：改动动机、测试方式、是否涉及接口/数据库变更

## 代码评审清单

- [ ] 符合分层规范（handler → service → repository → model），无跨层调用
- [ ] 流式路径无全量缓冲（ADR-003）
- [ ] 敏感信息（Key / 上游 Key）不落日志
- [ ] 数据库访问走 GORM，无 SQL 拼接注入
- [ ] 核心逻辑有单测；行为变更更新既有用例
- [ ] 需求有变更时回写 PRD（docs/02）

## 问题反馈

- Bug / 功能建议：开 [Issue](https://github.com/jianyunyi/aegis-gateway/issues/new)
- 安全漏洞：请勿公开，按 [SECURITY.md](SECURITY.md) 流程私下报告
