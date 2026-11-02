# 安全策略（Security Policy）

## 报告漏洞（Reporting a Vulnerability）

如果你发现安全问题（如 Key 泄露、注入、越权、敏感数据未脱敏等）：

- **请勿**在 Issue、讨论区或任何公开渠道透露细节
- 通过 GitHub 私信或邮箱联系维护者：https://github.com/jianyunyi
- 报告时请包含：复现步骤、影响范围、建议修复方案（可选）

维护者会尽快确认并修复。修复前请对外保密。

## 安全设计说明

本项目遵循以下安全实践（详见 `docs/03-技术方案设计.md`）：

| 项 | 说明 |
|---|---|
| API Key 存储 | 仅存 SHA-256 哈希 + 展示前缀，明文只在创建时返回一次（ADR-007） |
| 提供商 Key | AES-256-GCM 加密落库，代理调用时才解密 |
| 日志与隐私 | 生产环境必须显式关闭 prompt 采集；规则脱敏仅尽力而为，人工录入的评测样本不自动过期，不得放入客户或敏感数据 |
| 初始化凭证 | 不含公开 JWT fallback、`admin/admin123` 或演示 API Key；首个管理员仅通过显式引导凭据创建 |
| 防滥用 | 管理员登录按客户端 IP 与 IP+账号联合限流，但无 MFA 或跨 IP 撞库检测；上线须额外落实后台 VPN/IP 白名单，或 MFA、WAF 与异常登录告警 |
| 部署 | Compose 使用专用 MySQL 用户、认证 Redis、只读秘密文件挂载和非 root Web；宿主端口只绑定 loopback；公网发布必须通过 HTTPS 反向代理 |

## 支持版本

| 版本 | 支持状态 |
|---|---|
| v0.1.x | ✅ 当前版本 |
| < v0.1.0 | ❌ 未发布 |

## 生产部署检查清单

- [ ] 为 MySQL root、应用账号、Redis 和 `JWT_SECRET` 设置独立随机秘密
- [ ] 将秘密通过只读文件挂载和对应 `*_FILE` 配置注入，不要把秘密写入 Compose `environment`、`.env` 或进程参数
- [ ] 首个管理员通过显式引导账号/密码创建，不启用公开默认值
- [ ] 保持 `AUTO_MIGRATE=false`，按版本执行 SQL migrations
- [ ] 保持 Redis 认证和限流 fail-closed
- [ ] 明确设置 `PROMPT_CAPTURE_ENABLED=false`；生产应用拒绝缺失或开启的设置。规则脱敏不是敏感数据保证
- [ ] 不向人工评测样本录入客户或敏感数据；手工样本不自动过期
- [ ] 限定精确 CORS 来源、设置请求体上限，并经 HTTPS 反向代理发布；仅在代理会清理并维护 `X-Forwarded-For` 时配置最窄的 `TRUSTED_PROXY_CIDRS`
- [ ] 管理后台上线前必须置于 VPN/ZTNA 与 IP 白名单后；如果需公网访问，先部署 MFA、WAF 和失败登录异常告警并验证告警链路
- [ ] 发布前在生产数据隔离副本上演练迁移、备份恢复与回滚；不得在生产数据本体上试演
