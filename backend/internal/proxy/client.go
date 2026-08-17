// Package proxy 上游模型调用客户端与 OpenAI 协议辅助。
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// Usage OpenAI 协议 usage 字段（用于计费）。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// UpstreamClient 上游 HTTP 客户端（支持流式与非流式）。
type UpstreamClient struct {
	http *http.Client
}

// NewUpstreamClient 构造客户端。
func NewUpstreamClient(timeout time.Duration) *UpstreamClient {
	return &UpstreamClient{
		http: &http.Client{Timeout: timeout},
	}
}

// Do 发送上游请求并返回响应（调用方负责关闭 Body）。
func (u *UpstreamClient) Do(req *http.Request) (*http.Response, error) {
	return u.http.Do(req)
}

// NewRequest 构造上游请求（统一注入 Content-Type 与鉴权头）。
func NewRequest(ctx context.Context, method, url, apiKey string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return req, nil
}

// ResolveURL 由提供商 base_url 与相对路径拼出完整 URL。
// 兼容 base_url 带/不带 "/v1" 两种写法。
func ResolveURL(baseURL, path string) string {
	b := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(b, "/v1") {
		b += "/v1"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return b + path
}

// EnsureStreamUsage 当请求为流式时，注入 stream_options.include_usage=true，
// 以便部分上游（如 OpenAI）在结束块返回 usage，支撑计费。
func EnsureStreamUsage(body []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	if stream, ok := m["stream"].(bool); ok && stream {
		so, _ := m["stream_options"].(map[string]any)
		if so == nil {
			so = map[string]any{}
		}
		so["include_usage"] = true
		m["stream_options"] = so
		if out, err := json.Marshal(m); err == nil {
			return out
		}
	}
	return body
}

// ParseUsageFromSSELine 从 SSE 的 data 行解析 usage（结束块通常带 usage）。
// 返回 Usage 与是否命中。
func ParseUsageFromSSELine(line string) (Usage, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "data:") {
		return Usage{}, false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if payload == "[DONE]" {
		return Usage{}, false
	}
	var chunk struct {
		Usage *Usage `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil || chunk.Usage == nil {
		return Usage{}, false
	}
	return *chunk.Usage, true
}

// CopyBody 读取并返回响应体（同时关闭）。
func CopyBody(r io.ReadCloser) ([]byte, error) {
	defer r.Close()
	return io.ReadAll(r)
}

// ParseUsageFromBody 从非流式响应体解析 token 用量。
func ParseUsageFromBody(raw []byte) (prompt, completion int) {
	var r struct {
		Usage *Usage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || r.Usage == nil {
		return 0, 0
	}
	return r.Usage.PromptTokens, r.Usage.CompletionTokens
}

// ParseContentFromBody 提取非流式响应首条 message content（评测打分用）。
func ParseContentFromBody(raw []byte) string {
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || len(r.Choices) == 0 {
		return ""
	}
	return r.Choices[0].Message.Content
}
