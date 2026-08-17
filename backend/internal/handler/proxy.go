package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"aegis-gateway/internal/middleware"
	"aegis-gateway/internal/model"
	"aegis-gateway/internal/proxy"
	"aegis-gateway/internal/repository"
	"aegis-gateway/internal/responsecache"
	"aegis-gateway/internal/routing"
)

// ---- OpenAI 协议请求结构（仅解析代理所需字段，其余透传）----

type openaiChatReq struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// promptText 拼接消息内容供启发式路由使用（截断避免过大）。
func (r openaiChatReq) promptText() string {
	parts := make([]string, 0, len(r.Messages))
	for _, m := range r.Messages {
		parts = append(parts, m.Content)
	}
	t := strings.Join(parts, " ")
	if len([]rune(t)) > 2000 {
		t = string([]rune(t)[:2000])
	}
	return t
}

// ---- 辅助 ----

func abortOpenAI(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"message": msg, "type": "aegis_error"},
	})
}

// newUsageLog 构造调用日志实体。
func newUsageLog(key *model.ApiKey, m *model.Model, requestID string, start time.Time) *model.UsageLog {
	return &model.UsageLog{
		RequestID:  requestID,
		APIKeyID:   key.ID,
		UserID:     key.UserID,
		ProviderID: m.ProviderID,
		ModelName:  m.Name,
		Kind:       "chat",
		RoutedBy:   "manual",
		CreatedAt:  time.Now(),
	}
}

// costOf 按模型单价计算费用（元）。
func costOf(m *model.Model, prompt, completion int) float64 {
	return float64(prompt)/1000.0*m.PriceIn + float64(completion)/1000.0*m.PriceOut
}

// persistUsage 落库调用日志 + Redis 配额计数 + MySQL used_tokens 更新。
func persistUsage(repo *repository.Repository, key *model.ApiKey, log *model.UsageLog) {
	if log.TotalTokens > 0 {
		ctx := context.Background()
		if key.QuotaTokens > 0 {
			_ = repo.Redis.IncrBy(ctx, "quota:"+strconv.FormatUint(key.ID, 10), int64(log.TotalTokens)).Err()
		}
		_ = repo.DB.Model(&model.ApiKey{}).Where("id = ?", key.ID).
			UpdateColumn("used_tokens", key.UsedTokens+int64(log.TotalTokens)).Error
	}
	if err := repo.DB.Create(log).Error; err != nil {
		slog.Error("write usage log failed", "request_id", log.RequestID, "error", err)
	}
}

// ---- 代理端点 ----

// ChatCompletions 处理 POST /v1/chat/completions（支持 SSE 流式）。
// M4 链路：预算预检 → 语义路由 → 请求缓存 → 上游（流式/非流式）。
func ChatCompletions(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.MustGet(middleware.CtxAPIKey).(*model.ApiKey)
		requestID := c.GetString(middleware.RequestIDKey)
		start := time.Now()

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			abortOpenAI(c, http.StatusBadRequest, "无法读取请求体")
			return
		}
		var req openaiChatReq
		if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
			abortOpenAI(c, http.StatusBadRequest, "model 字段必填")
			return
		}

		// 请求的模型必须存在于目录
		requested, err := d.Models.GetByName(req.Model)
		if err != nil {
			abortOpenAI(c, http.StatusBadRequest, "未知模型: "+req.Model)
			return
		}

		// 1) 语义路由（ADR-006：header > Key 默认模型 > 启发式）
		dec, err := d.Router.Decide(c, requested, apiKey.DefaultModel, c.GetHeader("X-AEGIS-Model"), req.promptText())
		if err != nil {
			abortOpenAI(c, http.StatusInternalServerError, "路由决策失败")
			return
		}

		// 2) 预算预检：月度预算超限 → 降级到最便宜模型并告警
		if apiKey.BudgetMonthly > 0 {
			exceeded, spent, err := d.Budget.Exceeded(c, apiKey.ID, apiKey.BudgetMonthly)
			if err == nil && exceeded {
				if cheap, changed := d.Router.DowngradeToCheapest(c, dec.Model); changed {
					slog.Warn("monthly budget exceeded, downgrading model",
						"api_key_id", apiKey.ID, "spent", spent, "budget", apiKey.BudgetMonthly,
						"from", dec.Model.Name, "to", cheap.Name)
					dec.Model = cheap
					dec.RoutedBy = "budget"
					dec.Downgraded = true
				}
			}
		}
		target := dec.Model

		// 3) 请求缓存（仅非流式；key 含实际路由模型，避免跨模型错配缓存）
		if !req.Stream {
			cacheKey := chatCacheKey(target.Name, body)
			if cachedBody, hit := d.Cache.Get(c, cacheKey); hit {
				prompt, completion := usageFromBody([]byte(cachedBody))
				log := newUsageLog(apiKey, target, requestID, start)
				log.PromptTokens = prompt
				log.CompletionTokens = completion
				log.TotalTokens = prompt + completion
				log.Cost = 0 // 缓存命中不产生上游费用
				log.LatencyMs = int(time.Since(start).Milliseconds())
				log.Cached = 1
				log.RoutedBy = dec.RoutedBy
				log.UpstreamModel = dec.Requested
				persistUsage(d.Repo, apiKey, log)
				c.Data(http.StatusOK, "application/json", []byte(cachedBody))
				return
			}
		}

		// 4) 提供商解析（按路由后的模型）
		p, err := d.Providers.Get(target.ProviderID)
		if err != nil || p.Enabled != 1 {
			abortOpenAI(c, http.StatusBadGateway, "提供商不可用")
			return
		}
		upstreamKey, err := d.Providers.DecryptKey(p)
		if err != nil {
			abortOpenAI(c, http.StatusBadGateway, "提供商凭证解析失败")
			return
		}
		if upstreamKey == "" {
			abortOpenAI(c, http.StatusBadGateway, "提供商 API Key 未配置，请在管理后台补充")
			return
		}

		// 配额预检（Redis 计数；与 MySQL 的对账在 M3 实现）
		if apiKey.QuotaTokens > 0 {
			used, _ := d.Repo.Redis.Get(c, "quota:"+strconv.FormatUint(apiKey.ID, 10)).Int64()
			if used >= apiKey.QuotaTokens {
				log := newUsageLog(apiKey, target, requestID, start)
				log.ErrorCode = "quota_exceeded"
				log.Status = http.StatusTooManyRequests
				log.LatencyMs = int(time.Since(start).Milliseconds())
				persistUsage(d.Repo, apiKey, log)
				abortOpenAI(c, http.StatusTooManyRequests, "配额不足")
				return
			}
		}

		// 流式请求注入 include_usage（支撑计费）
		forwardBody := body
		if req.Stream {
			forwardBody = proxy.EnsureStreamUsage(body)
		}
		upURL := proxy.ResolveURL(p.BaseURL, "/chat/completions")
		upReq, err := proxy.NewRequest(c.Request.Context(), http.MethodPost, upURL, upstreamKey, forwardBody)
		if err != nil {
			abortOpenAI(c, http.StatusBadGateway, "上游请求构造失败")
			return
		}

		resp, err := d.Upstream.Do(upReq)
		if err != nil {
			log := newUsageLog(apiKey, target, requestID, start)
			log.ErrorCode = "upstream_error"
			log.Status = http.StatusBadGateway
			log.LatencyMs = int(time.Since(start).Milliseconds())
			persistUsage(d.Repo, apiKey, log)
			abortOpenAI(c, http.StatusBadGateway, "上游调用失败: "+err.Error())
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= http.StatusBadRequest {
			raw, _ := io.ReadAll(resp.Body)
			log := newUsageLog(apiKey, target, requestID, start)
			log.ErrorCode = "upstream_http_" + strconv.Itoa(resp.StatusCode)
			log.Status = int16(resp.StatusCode)
			log.LatencyMs = int(time.Since(start).Milliseconds())
			persistUsage(d.Repo, apiKey, log)
			c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), raw)
			return
		}

		if req.Stream {
			streamChatForward(d, c, apiKey, target, dec, requestID, start, resp.Body)
			return
		}

		// 非流式：透传 JSON 并解析 usage；成功后写缓存
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			abortOpenAI(c, http.StatusBadGateway, "读取上游响应失败")
			return
		}
		prompt, completion := usageFromBody(raw)
		log := newUsageLog(apiKey, target, requestID, start)
		log.PromptTokens = prompt
		log.CompletionTokens = completion
		log.TotalTokens = prompt + completion
		log.Cost = costOf(target, prompt, completion)
		log.LatencyMs = int(time.Since(start).Milliseconds())
		log.RoutedBy = dec.RoutedBy
		log.UpstreamModel = dec.Requested
		persistUsage(d.Repo, apiKey, log)

		// 计费：预算累计（仅对设置了月度预算的 Key 生效）
		if log.Cost > 0 && apiKey.BudgetMonthly > 0 {
			_ = d.Budget.Add(c, apiKey.ID, log.Cost)
		}
		// 写缓存（非流式）
		_ = d.Cache.Set(c, chatCacheKey(target.Name, body), string(raw))

		c.Data(http.StatusOK, "application/json", raw)
	}
}

// usageFromBody 从 OpenAI 响应体解析 token 用量。
func usageFromBody(raw []byte) (prompt, completion int) {
	var r struct {
		Usage *proxy.Usage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || r.Usage == nil {
		return 0, 0
	}
	return r.Usage.PromptTokens, r.Usage.CompletionTokens
}

// chatCacheKey 缓存键 = 实际路由模型名 + 请求体（保证同请求同模型才命中）。
func chatCacheKey(modelName string, body []byte) string {
	key := make([]byte, 0, len(modelName)+1+len(body))
	key = append(key, modelName...)
	key = append(key, '\x00')
	key = append(key, body...)
	return responsecache.KeyFor(key)
}

// streamChatForward SSE 逐块转发上游响应，统计 TTFT 与 usage。
func streamChatForward(d *Deps, c *gin.Context, apiKey *model.ApiKey, target *model.Model, dec *routing.Decision, requestID string, start time.Time, upstream io.Reader) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		abortOpenAI(c, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	var (
		ttft    int
		first   = true
		prompt  int
		complet int
		status  int16
		errCode string
		scanner = bufio.NewScanner(upstream)
	)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if first {
			ttft = int(time.Since(start).Milliseconds())
			first = false
		}
		if u, hit := proxy.ParseUsageFromSSELine(line); hit {
			prompt, complet = u.PromptTokens, u.CompletionTokens
		}
		// 客户端提前断开（如用户取消）≠ 上游错误，需区分记录
		if _, werr := c.Writer.WriteString(line); werr != nil {
			status, errCode = 0, "client_disconnect"
			break
		}
		_, _ = c.Writer.WriteString("\n")
		flusher.Flush()
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		status, errCode = http.StatusBadGateway, "upstream_stream_error"
	}

	log := newUsageLog(apiKey, target, requestID, start)
	log.PromptTokens = prompt
	log.CompletionTokens = complet
	log.TotalTokens = prompt + complet
	log.Cost = costOf(target, prompt, complet)
	log.LatencyMs = int(time.Since(start).Milliseconds())
	if ttft > 0 {
		log.TTFTMs = &ttft
	}
	log.Status = status
	log.ErrorCode = errCode
	log.RoutedBy = dec.RoutedBy
	log.UpstreamModel = dec.Requested
	persistUsage(d.Repo, apiKey, log)

	if log.Cost > 0 && apiKey.BudgetMonthly > 0 {
		_ = d.Budget.Add(c, apiKey.ID, log.Cost)
	}
}

// Completions 处理 POST /v1/completions（透传，非流式）。
func Completions(d *Deps) gin.HandlerFunc {
	return passthrough(d, "completions")
}

// Embeddings 处理 POST /v1/embeddings（透传，非流式）。
func Embeddings(d *Deps) gin.HandlerFunc {
	return passthrough(d, "embeddings")
}

// passthrough 通用透传：模型名必须存在于目录，其余字段原样转发。
func passthrough(d *Deps, path string) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.MustGet(middleware.CtxAPIKey).(*model.ApiKey)
		requestID := c.GetString(middleware.RequestIDKey)
		start := time.Now()

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			abortOpenAI(c, http.StatusBadRequest, "无法读取请求体")
			return
		}
		var req struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
			abortOpenAI(c, http.StatusBadRequest, "model 字段必填")
			return
		}
		m, err := d.Models.GetByName(req.Model)
		if err != nil {
			abortOpenAI(c, http.StatusBadRequest, "未知模型: "+req.Model)
			return
		}
		p, err := d.Providers.Get(m.ProviderID)
		if err != nil || p.Enabled != 1 {
			abortOpenAI(c, http.StatusBadGateway, "提供商不可用")
			return
		}
		upstreamKey, err := d.Providers.DecryptKey(p)
		if err != nil || upstreamKey == "" {
			abortOpenAI(c, http.StatusBadGateway, "提供商 API Key 未配置")
			return
		}

		upURL := proxy.ResolveURL(p.BaseURL, "/"+path)
		upReq, err := proxy.NewRequest(c.Request.Context(), http.MethodPost, upURL, upstreamKey, body)
		if err != nil {
			abortOpenAI(c, http.StatusBadGateway, "上游请求构造失败")
			return
		}
		resp, err := d.Upstream.Do(upReq)
		if err != nil {
			log := newUsageLog(apiKey, m, requestID, start)
			log.ErrorCode = "upstream_error"
			log.Status = http.StatusBadGateway
			log.LatencyMs = int(time.Since(start).Milliseconds())
			persistUsage(d.Repo, apiKey, log)
			abortOpenAI(c, http.StatusBadGateway, "上游调用失败: "+err.Error())
			return
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)

		log := newUsageLog(apiKey, m, requestID, start)
		log.Kind = path
		if resp.StatusCode >= http.StatusBadRequest {
			log.ErrorCode = "upstream_http_" + strconv.Itoa(resp.StatusCode)
			log.Status = int16(resp.StatusCode)
		} else {
			if path == "embeddings" {
				var r struct {
					Usage *proxy.Usage `json:"usage"`
				}
				_ = json.Unmarshal(raw, &r)
				if r.Usage != nil {
					log.PromptTokens = r.Usage.PromptTokens
					log.TotalTokens = r.Usage.TotalTokens
				}
			}
			log.Cost = costOf(m, log.PromptTokens, log.CompletionTokens)
		}
		log.LatencyMs = int(time.Since(start).Milliseconds())
		persistUsage(d.Repo, apiKey, log)

		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), raw)
	}
}

// ProxyListModels 处理 GET /v1/models，返回网关模型目录（OpenAI 格式）。
func ProxyListModels(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		models, err := d.Models.List(true)
		if err != nil {
			abortOpenAI(c, http.StatusInternalServerError, "查询模型目录失败")
			return
		}
		items := make([]gin.H, 0, len(models))
		for _, m := range models {
			items = append(items, gin.H{
				"id":       m.Name,
				"object":   "model",
				"owned_by": "aegis",
				"tier":     m.Tier,
			})
		}
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": items})
	}
}
