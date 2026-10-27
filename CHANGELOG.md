# Changelog

项目版本遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## [Unreleased] — Security hardening

- 移除生产 JWT fallback、默认管理员和启动时演示 API Key；种子数据/管理员创建须显式启用并提供独立凭据。
- 真实对话采集默认关闭；显式开启时规则脱敏后才存储，支持单请求退出与按保留期清理。
- `make migrate`/Compose `migrate` 执行具备版本记录的 SQL migrations；补齐 API Key 账务列。
- chat、completions、embeddings 统一做 usage 计价/流水；token 配额与月度成本均使用 Redis Lua 原子预留，依赖故障关闭；预算每次以 MySQL 当月流水为准并在账单提交后结算实际成本。
- 上游不返回 usage 时按请求和响应内容估算 token 并标记 `usage_estimated`，避免 completions/embeddings 或流式请求静默记零。
- 为请求体设上限、管理员登录加 IP 限流、request ID 改由网关随机生成；Compose 使用认证 Redis、专用 MySQL 账号，端口只绑定 loopback。
- 升级 Next.js、Axios、ECharts 及相关生产依赖；`npm audit --omit=dev` 通过。

## [v0.1.0] - 2026-08-17

首个开源版本：企业大模型网关 M1-M5 全部里程碑完成。

### 核心能力
- **统一接入**：OpenAI 兼容代理（`/v1/chat/completions` 流式 + 非流式、`/v1/completions`、`/v1/embeddings`、`/v1/models`），任意 OpenAI SDK 改 base_url 即接入
- **安全鉴权**：API Key 哈希存储（ADR-007）、提供商 Key AES-256-GCM 加密、JWT 管理端登录、CORS
- **限流防滥用**：Redis Lua 令牌桶（每 Key 可配 rps/burst）
- **计费对账**：按 token 计价、调用日志落库、Redis↔MySQL 配额对账（以 MySQL 为准，定时 + 手动触发）
- **可观测**：大盘概览/趋势、调用日志分页查询、每日账单、TTFT 首字延迟记录
- **成本优化**：语义路由（Header > Key 默认模型 > 启发式）、请求缓存（含路由模型复合键）、月度预算自动降级 + 告警
- **评测飞轮**：真实调用采样、人工打标、A/B 模型回归（质量分 + 成本 + 延迟报告）
- **管理后台**：Next.js + AntD（登录/大盘/Key/提供商/模型/日志/账单/评测）

### 工程化
- Docker Compose 一键部署（网关/后台/MySQL/Redis/mock 沙箱，离线镜像构建）
- GitHub Actions CI（lint + test(race) + build）
- 压测工具与报告（网关自身 P95=60ms @200 并发）

### 已知限制
- mock 上游为演示沙箱，评测评分为启发式兜底（LLM-as-judge 扩展点已预留）
- Windows 单进程压测受 TCP backlog（~200）限制，生产建议 Linux/反向代理
- prompt_preview 仅截断未脱敏（企业版需接脱敏组件）
