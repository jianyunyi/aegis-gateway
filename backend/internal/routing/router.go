// Package routing 语义路由（ADR-006）：
// 优先级 显式指定(Header) > 规则(Key 默认模型) > 启发式(prompt 特征选档)。
// 可解释、零额外成本；LLM 分类器作为扩展点预留（接口化）。
package routing

import (
	"context"
	"errors"
	"strings"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

// ErrNoModelInTier 该档位无可用模型。
var ErrNoModelInTier = errors.New("no enabled model in tier")

// Decision 路由决策结果。
type Decision struct {
	Model      *model.Model // 实际使用的模型
	Requested  string       // 客户端请求的模型名
	RoutedBy   string       // manual / header / rule / heuristic / budget
	Downgraded bool         // 是否因预算超限被降级
}

// Router 语义路由实现。
type Router struct {
	repo *repository.Repository
}

// NewRouter 构造 Router。
func NewRouter(repo *repository.Repository) *Router {
	return &Router{repo: repo}
}

// Decide 决定实际调用的模型。
func (r *Router) Decide(ctx context.Context, requested *model.Model, keyDefault, headerPrefer, promptText string) (*Decision, error) {
	d := &Decision{Requested: requested.Name, Model: requested, RoutedBy: "manual"}

	// 1. 显式指定（请求头 X-AEGIS-Model）
	if headerPrefer != "" {
		if m, err := r.modelByName(ctx, headerPrefer); err == nil {
			d.Model, d.RoutedBy = m, "header"
			return d, nil
		}
	}

	// 2. Key 级默认模型（规则）
	if keyDefault != "" {
		if m, err := r.modelByName(ctx, keyDefault); err == nil {
			d.Model, d.RoutedBy = m, "rule"
			return d, nil
		}
	}

	// 3. 启发式：按 prompt 特征选档（短问答→便宜档，编程/代码→强档，其余→标准档）
	if tier := HeuristicTier(promptText); tier != "" && requested.Tier != tier {
		if m, err := r.pickInTier(ctx, tier, requested.ProviderID); err == nil {
			d.Model, d.RoutedBy = m, "heuristic"
		}
	}
	return d, nil
}

// DowngradeToCheapest 预算超限时降级到最便宜的可选模型（不含当前模型时）。
func (r *Router) DowngradeToCheapest(ctx context.Context, current *model.Model) (*model.Model, bool) {
	if current.Tier == "cheap" {
		return current, false
	}
	var ms []model.Model
	if err := r.repo.DB.Where("enabled = 1").Order("price_in ASC, price_out ASC").Find(&ms).Error; err != nil {
		return current, false
	}
	if len(ms) == 0 {
		return current, false
	}
	return &ms[0], ms[0].Name != current.Name
}

// HeuristicTier 启发式选档：返回 cheap / normal / strong / ""（不干预）。
// 优先级：编程/深度任务关键词 > 短问答 > 默认标准档。
func HeuristicTier(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	// 编程/代码/深度解释类任务 → 强档（无论长短）
	if containsAny(t, []string{"代码", "code", "函数", "bug", "调试", "算法", "SQL", "sql", "编程", "报错", "解释", "架构"}) {
		return "strong"
	}
	// 短问答 → 便宜档
	if len([]rune(t)) <= 40 {
		return "cheap"
	}
	return "normal"
}

func containsAny(s string, subs []string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func (r *Router) modelByName(ctx context.Context, name string) (*model.Model, error) {
	var m model.Model
	if err := r.repo.DB.Where("name = ? AND enabled = 1", name).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Router) pickInTier(ctx context.Context, tier string, preferProvider uint64) (*model.Model, error) {
	var ms []model.Model
	if err := r.repo.DB.Where("tier = ? AND enabled = 1", tier).
		Order("price_in ASC, price_out ASC").Find(&ms).Error; err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		return nil, ErrNoModelInTier
	}
	// 优先同提供商（如 deepseek 内部 cheap 档），否则取该档最便宜的
	for i := range ms {
		if ms[i].ProviderID == preferProvider {
			return &ms[i], nil
		}
	}
	return &ms[0], nil
}
