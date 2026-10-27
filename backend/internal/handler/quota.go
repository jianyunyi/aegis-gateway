package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/budget"
	"aegis-gateway/internal/model"
	"aegis-gateway/internal/proxy"
	"aegis-gateway/internal/repository"
)

type quotaLease struct {
	repo      *repository.Repository
	keyID     uint64
	requestID string
	keep      bool
	released  bool
}

type budgetLease struct {
	budget    *budget.Budget
	keyID     uint64
	monthKey  string
	requestID string
	keep      bool
	released  bool
}

func reserveQuota(c *gin.Context, d *Deps, key *model.ApiKey, requestID string, estimate int64) (*quotaLease, int) {
	if key.QuotaTokens <= 0 {
		return nil, 0
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), dependencyTimeout(d))
	defer cancel()
	allowed, err := d.Repo.ReserveQuota(ctx, key.ID, requestID, key.UsedTokens, key.QuotaTokens, estimate, 10*time.Minute)
	if err != nil {
		slog.Error("quota reservation failed", "request_id", requestID, "api_key_id", key.ID, "error", err)
		return nil, http.StatusServiceUnavailable
	}
	if !allowed {
		return nil, http.StatusTooManyRequests
	}
	return &quotaLease{repo: d.Repo, keyID: key.ID, requestID: requestID}, 0
}

func reserveBudget(c *gin.Context, d *Deps, key *model.ApiKey, target *model.Model, kind string, body []byte, requestID string) (*budgetLease, int) {
	if key.BudgetMonthly <= 0 {
		return nil, 0
	}
	estimate := estimateCost(kind, body, target)
	if estimate <= 0 {
		return nil, 0
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), dependencyTimeout(d))
	defer cancel()
	monthKey := budget.MonthKey(key.ID)
	allowed, err := d.Budget.ReserveForMonth(ctx, key.ID, monthKey, requestID, key.BudgetMonthly, estimate, 10*time.Minute)
	if err != nil {
		slog.Error("budget reservation failed", "request_id", requestID, "api_key_id", key.ID, "error", err)
		return nil, http.StatusServiceUnavailable
	}
	if !allowed {
		return nil, http.StatusTooManyRequests
	}
	return &budgetLease{budget: d.Budget, keyID: key.ID, monthKey: monthKey, requestID: requestID}, 0
}

func dependencyTimeout(d *Deps) time.Duration {
	if d.Cfg != nil && d.Cfg.RateLimitTimeout > 0 {
		return d.Cfg.RateLimitTimeout
	}
	return 500 * time.Millisecond
}

func (l *quotaLease) retainUntilExpiry() {
	if l != nil {
		l.keep = true
	}
}

func (l *quotaLease) Release() {
	if l == nil || l.released || l.keep {
		return
	}
	l.released = true
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := l.repo.ReleaseQuota(ctx, l.keyID, l.requestID); err != nil {
		slog.Error("quota reservation release failed", "request_id", l.requestID, "api_key_id", l.keyID, "error", err)
	}
}

func (l *budgetLease) retainUntilExpiry() {
	if l != nil {
		l.keep = true
	}
}

func (l *budgetLease) Release() {
	if l == nil || l.released || l.keep {
		return
	}
	l.released = true
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := l.budget.ReleaseForMonth(ctx, l.keyID, l.monthKey, l.requestID); err != nil {
		slog.Error("budget reservation release failed", "request_id", l.requestID, "api_key_id", l.keyID, "error", err)
	}
}

func (l *budgetLease) CommitUsage(actualCost float64, persist func() error) error {
	if l == nil || l.released || l.keep {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := l.budget.CommitUsageForMonth(ctx, l.keyID, l.monthKey, l.requestID, actualCost, persist); err != nil {
		l.keep = true
		slog.Error("budget settlement failed", "request_id", l.requestID, "api_key_id", l.keyID, "error", err)
		return err
	}
	l.released = true
	return nil
}

// settleUsage commits the durable ledger first; failed ledger writes keep both
// Redis reservations until bounded expiry instead of admitting more traffic.
func settleUsage(repo *repository.Repository, key *model.ApiKey, log *model.UsageLog, quota *quotaLease, monthly *budgetLease) {
	persist := func() error { return persistUsage(repo, key, log) }
	if monthly != nil {
		attempted, persisted := false, false
		err := monthly.CommitUsage(log.Cost, func() error {
			attempted = true
			err := persist()
			persisted = err == nil
			return err
		})
		if err != nil && !attempted {
			// Preserve the durable ledger even when Redis cannot grant the lock.
			// Keep the monthly reservation until expiry to remain conservative.
			persisted = persist() == nil
			monthly.retainUntilExpiry()
		}
		if !persisted {
			if quota != nil {
				quota.retainUntilExpiry()
			}
			monthly.retainUntilExpiry()
			return
		}
		if quota != nil {
			quota.Release()
		}
		return
	}
	if err := persist(); err != nil {
		if quota != nil {
			quota.retainUntilExpiry()
		}
		return
	}
	if quota != nil {
		quota.Release()
	}
}

func estimateTokens(kind string, body []byte) int64 {
	input, output := estimateTokenBreakdown(kind, body)
	if input+output < 1 {
		return 1
	}
	return input + output
}

func estimateCost(kind string, body []byte, target *model.Model) float64 {
	if target == nil {
		return 0
	}
	input, output := estimateTokenBreakdown(kind, body)
	return float64(input)/1000*target.PriceIn + float64(output)/1000*target.PriceOut
}

func estimateUsage(kind string, requestBody, responseBody []byte) (int, int) {
	input := estimateInputTokens(kind, requestBody)
	if kind == "embeddings" {
		return int(input), 0
	}
	var response map[string]any
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return int(input), 0
	}
	var outputText strings.Builder
	if choices, ok := response["choices"].([]any); ok {
		for _, rawChoice := range choices {
			choice, ok := rawChoice.(map[string]any)
			if !ok {
				continue
			}
			appendText(&outputText, choice["text"])
			if message, ok := choice["message"].(map[string]any); ok {
				appendText(&outputText, message["content"])
				for _, field := range []string{"tool_calls", "function_call"} {
					if value, exists := message[field]; exists {
						if encoded, err := json.Marshal(value); err == nil {
							outputText.Write(encoded)
						}
					}
				}
			}
		}
	}
	output := proxy.EstimateTextTokens(outputText.String())
	return int(input), output
}

func estimateTokenBreakdown(kind string, body []byte) (int64, int64) {
	input := estimateInputTokens(kind, body)
	if kind == "embeddings" {
		return input, 0
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return input, 4096
	}
	output := requestedOutputTokens(payload)
	if output == 0 {
		output = 4096
	}
	return input, output
}

func estimateInputTokens(kind string, body []byte) int64 {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return 1
	}
	var inputText strings.Builder
	switch kind {
	case "chat/completions":
		if messages, ok := payload["messages"].([]any); ok {
			for _, item := range messages {
				if message, ok := item.(map[string]any); ok {
					appendText(&inputText, message["content"])
				}
			}
		}
	default:
		appendText(&inputText, payload["prompt"])
		appendText(&inputText, payload["input"])
	}
	return int64(proxy.EstimateTextTokens(inputText.String()))
}

func requestedOutputTokens(payload map[string]any) int64 {
	for _, field := range []string{"max_completion_tokens", "max_tokens"} {
		value, ok := payload[field]
		if !ok {
			continue
		}
		if number, ok := value.(float64); ok && number > 0 {
			return int64(math.Min(number, 4_000_000))
		}
	}
	return 0
}

func appendText(out *strings.Builder, value any) {
	switch item := value.(type) {
	case string:
		out.WriteString(item)
	case []any:
		for _, nested := range item {
			appendText(out, nested)
		}
	case map[string]any:
		if text, ok := item["text"]; ok {
			appendText(out, text)
		}
	}
}
