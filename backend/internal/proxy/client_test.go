package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveURL(t *testing.T) {
	cases := []struct {
		base, path, want string
	}{
		{"https://api.deepseek.com", "/chat/completions", "https://api.deepseek.com/v1/chat/completions"},
		{"https://api.deepseek.com/v1", "/chat/completions", "https://api.deepseek.com/v1/chat/completions"},
		{"http://localhost:8099/", "chat/completions", "http://localhost:8099/v1/chat/completions"},
	}
	for _, c := range cases {
		if got := ResolveURL(c.base, c.path); got != c.want {
			t.Errorf("ResolveURL(%q, %q) = %q, want %q", c.base, c.path, got, c.want)
		}
	}
}

func TestEnsureStreamUsage(t *testing.T) {
	body := []byte(`{"model":"m","stream":true}`)
	out := EnsureStreamUsage(body)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("输出不是合法 JSON: %v", err)
	}
	so, ok := m["stream_options"].(map[string]any)
	if !ok || so["include_usage"] != true {
		t.Errorf("stream_options.include_usage 未注入: %s", out)
	}

	// 非流式请求不应被修改
	plain := EnsureStreamUsage([]byte(`{"model":"m","stream":false}`))
	if string(plain) != `{"model":"m","stream":false}` {
		t.Errorf("非流式请求被意外修改: %s", plain)
	}
}

func TestParseUsageFromSSELine(t *testing.T) {
	line := `data: {"id":"x","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`
	u, ok := ParseUsageFromSSELine(line)
	if !ok || u.TotalTokens != 30 || u.PromptTokens != 10 || u.CompletionTokens != 20 {
		t.Fatalf("usage 解析失败: %+v ok=%v", u, ok)
	}
	if _, ok := ParseUsageFromSSELine("data: [DONE]"); ok {
		t.Fatal("[DONE] 不应命中 usage")
	}
	if _, ok := ParseUsageFromSSELine(`data: {"choices":[{"delta":{"content":"hi"}}]}`); ok {
		t.Fatal("无 usage 的 chunk 不应命中")
	}
}

func TestHasContentDelta(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{": heartbeat", false},
		{`data: {"choices":[{"delta":{"role":"assistant"}}]}`, false},
		{`data: {"choices":[],"usage":{"total_tokens":30}}`, false},
		{`data: {"choices":[{"delta":{"content":"hello"}}]}`, true},
		{`data: {"choices":[{"delta":{"tool_calls":[{"index":0}]}}]}`, true},
		{`data: {"choices":[{"delta":{"function_call":null}}]}`, false},
		{"data: [DONE]", false},
	} {
		if got := HasContentDelta(tc.line); got != tc.want {
			t.Fatalf("%s got %t want %t", tc.line, got, tc.want)
		}
	}
}

func TestIsSSEDone(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"data:[DONE]", true}, {"data: [DONE]", true}, {"data:[DONE]\r", true},
		{": [DONE]", false}, {"event:[DONE]", false}, {"data: [DONE]extra", false},
		{"data: {\"choices\":[]}", false},
	} {
		if got := IsSSEDone(tc.line); got != tc.want {
			t.Errorf("%q: got %t want %t", tc.line, got, tc.want)
		}
	}
}

func TestParseUsageFromCompletionsAndEmbeddings(t *testing.T) {
	for _, raw := range []string{
		`{"object":"text_completion","choices":[{"text":"answer"}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`,
		`{"object":"list","data":[{"embedding":[]}],"usage":{"prompt_tokens":12,"total_tokens":12}}`,
	} {
		prompt, completion := ParseUsageFromBody([]byte(raw))
		if prompt != 12 {
			t.Fatalf("prompt usage=%d, want 12: %s", prompt, raw)
		}
		if strings.Contains(raw, "text_completion") && completion != 7 {
			t.Fatalf("completion usage=%d, want 7", completion)
		}
	}
	if prompt, completion := ParseUsageFromBody([]byte(`{"choices":[]}`)); prompt != 0 || completion != 0 {
		t.Fatalf("missing usage must remain distinguishable, got %d/%d", prompt, completion)
	}
}

func TestUsagePresenceAndSSECharacterEstimate(t *testing.T) {
	if HasUsageInBody([]byte(`{"usage":{"prompt_tokens":0}}`)) != true {
		t.Fatal("explicit zero usage must still be marked as reported")
	}
	if HasUsageInBody([]byte(`{"choices":[]}`)) {
		t.Fatal("missing usage must be distinguishable")
	}
	line := `data: {"choices":[{"delta":{"content":"hello"}}]}`
	if got := ContentDeltaTokenEstimate(line); got != 2 {
		t.Fatalf("SSE content token estimate=%d, want 2", got)
	}
	if got := ContentDeltaTokenEstimate(": heartbeat"); got != 0 {
		t.Fatalf("heartbeat token estimate=%d, want 0", got)
	}
	if got := EstimateTextTokens("你好"); got != 4 {
		t.Fatalf("CJK token estimate=%d, want 4", got)
	}
}

func TestRewriteModelPreservesRequestFields(t *testing.T) {
	input := []byte(`{"model":"client-model","stream":true,"max_tokens":10}`)
	var got map[string]any
	if err := json.Unmarshal(RewriteModel(input, "routed-model"), &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "routed-model" || got["stream"] != true || got["max_tokens"] != float64(10) {
		t.Fatalf("rewritten request lost fields: %+v", got)
	}
}
