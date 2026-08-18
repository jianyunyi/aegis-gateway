## 变更说明

（这次 PR 做了什么，为什么）

## 关联

- Closes #xxx（如有）
- 涉及 PRD/ADR：`docs/02-需求规格说明书.md` / `docs/03-技术方案设计.md`（如有变更）

## 验证

- [ ] `cd backend && go vet ./...`
- [ ] `cd backend && go test ./... -race -count=1`
- [ ] `cd web && npm run build`
- [ ] 本地运行演示点验证（描述验证方式）

## 代码评审自查

- [ ] 分层符合规范（handler → service → repository → model），无跨层调用
- [ ] 流式路径无全量缓冲（ADR-003）
- [ ] 敏感信息（Key / 上游 Key）不落日志
- [ ] 数据库访问走 GORM，无 SQL 拼接注入
- [ ] 核心逻辑有单测；行为变更更新既有用例
- [ ] 需求有变更时回写 PRD

## 截图（界面变更时）

（可选的演示截图）
