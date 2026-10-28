// Package proxy 上游模型调用客户端与 OpenAI 协议辅助。
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const MaxOutputTokens = 32768

// EnforceOutputCap makes the provider request obey the gateway hard output cap.
// A client request above the cap is rejected rather than silently under-reserved.
func EnforceOutputCap(body []byte) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return nil, fmt.Errorf("invalid JSON request")
	}
	seen := false
	for _, field := range []string{"max_completion_tokens", "max_tokens"} {
		raw, ok := payload[field]
		if !ok {
			continue
		}
		value, err := strconv.ParseInt(string(raw), 10, 64)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("%s must be a positive integer", field)
		}
		if value > MaxOutputTokens {
			return nil, fmt.Errorf("%s exceeds maximum of %d", field, MaxOutputTokens)
		}
		seen = true
	}
	if !seen {
		encoded, _ := json.Marshal(MaxOutputTokens)
		payload["max_tokens"] = encoded
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return out, nil
}

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

// RewriteModel replaces the client-requested model with the resolved upstream model.
func RewriteModel(body []byte, model string) []byte {
	if model == "" {
		return body
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return body
	}
	encoded, err := json.Marshal(model)
	if err != nil {
		return body
	}
	payload["model"] = encoded
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return rewritten
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

// EstimateTextTokens is conservative: ASCII averages one token per four
// characters; non-ASCII runes reserve two tokens to avoid undercounting CJK.
func EstimateTextTokens(text string) int {
	ascii, nonASCII := 0, 0
	for _, r := range text {
		if r <= 0x7f {
			ascii++
		} else {
			nonASCII++
		}
	}
	return (ascii+3)/4 + 2*nonASCII
}

// HasUsageInBody distinguishes a reported zero-usage response from a missing usage object.
func HasUsageInBody(raw []byte) bool {
	var r struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return false
	}
	return len(r.Usage) > 0 && string(r.Usage) != "null"
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

// HasContentDelta excludes heartbeats, role-only events and usage-only events from TTFT.
func HasContentDelta(line string) bool {
	if !strings.HasPrefix(line, "data:") {
		return false
	}
	var event struct {
		Choices []struct {
			Delta struct {
				Content      string            `json:"content"`
				ToolCalls    []json.RawMessage `json:"tool_calls"`
				FunctionCall json.RawMessage   `json:"function_call"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) != nil {
		return false
	}
	for _, choice := range event.Choices {
		if choice.Delta.Content != "" || len(choice.Delta.ToolCalls) > 0 || (len(choice.Delta.FunctionCall) > 0 && string(choice.Delta.FunctionCall) != "null") {
			return true
		}
	}
	return false
}

// ContentDeltaTokenEstimate estimates output tokens in an SSE event when usage is absent.
func ContentDeltaTokenEstimate(line string) int {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "data:") {
		return 0
	}
	var event struct {
		Choices []struct {
			Delta struct {
				Content      string            `json:"content"`
				ToolCalls    []json.RawMessage `json:"tool_calls"`
				FunctionCall json.RawMessage   `json:"function_call"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))), &event) != nil {
		return 0
	}
	var output strings.Builder
	for _, choice := range event.Choices {
		output.WriteString(choice.Delta.Content)
		for _, call := range choice.Delta.ToolCalls {
			output.Write(call)
		}
		if len(choice.Delta.FunctionCall) > 0 && string(choice.Delta.FunctionCall) != "null" {
			output.Write(choice.Delta.FunctionCall)
		}
	}
	return EstimateTextTokens(output.String())
}

// IsSSEDone accepts either serialization of the SSE data field.
func IsSSEDone(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "data:") && strings.TrimSpace(strings.TrimPrefix(line, "data:")) == "[DONE]"
}
