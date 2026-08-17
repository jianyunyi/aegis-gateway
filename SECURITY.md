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
| 日志 | 不记录 Key 明文；prompt_preview 仅截断（企业版需接脱敏） |
| 默认凭证 | 种子管理员 `admin/admin123` 仅用于演示，生产必须修改 `JWT_SECRET` 并禁用种子 |

## 支持版本

| 版本 | 支持状态 |
|---|---|
| v0.1.x | ✅ 当前版本 |
| < v0.1.0 | ❌ 未发布 |

## 生产部署检查清单

- [ ] 修改 `JWT_SECRET`（`JWT_SECRET` 环境变量）
- [ ] 修改默认管理员密码
- [ ] 设置 `AUTO_MIGRATE=false`，改用 `migrations/` SQL 迁移
- [ ] 配置限流 fail-closed（如需）
- [ ] 启用 HTTPS 反向代理
