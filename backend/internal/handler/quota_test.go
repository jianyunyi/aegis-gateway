package handler

import (
	"testing"

	"aegis-gateway/internal/model"
)

func TestEstimateTokensIncludesInputAndRequestedOutput(t *testing.T) {
	chat := []byte(`{"messages":[{"role":"user","content":"abcdefgh"}],"max_completion_tokens":32}`)
	if got := estimateTokens("chat/completions", chat); got != 110 {
		t.Fatalf("chat estimate=%d, want 110", got)
	}
	completion := []byte(`{"prompt":"abcd","max_tokens":12}`)
	if got := estimateTokens("completions", completion); got != 45 {
		t.Fatalf("completion estimate=%d, want 45", got)
	}
	embedding := []byte(`{"input":["abcd","efgh"]}`)
	if got := estimateTokens("embeddings", embedding); got != 25 {
		t.Fatalf("embedding estimate=%d, want 25", got)
	}
	if got := estimateTokens("chat/completions", []byte(`{"messages":[]}`)); got != 32783 {
		t.Fatalf("default output estimate=%d, want 32783", got)
	}
}

func TestEstimateTokensHasConservativeOutputCeiling(t *testing.T) {
	got := estimateTokens("chat/completions", []byte(`{"messages":[{"content":"x"}],"max_tokens":999999999999}`))
	if got != 32824 {
		t.Fatalf("bounded output estimate=%d, want 32824", got)
	}
}

func TestEstimateTokensCapsUntrustedMaxTokens(t *testing.T) {
	if got := estimateTokens("completions", []byte(`{"prompt":"x","max_tokens":999999999}`)); got > 4_000_001 {
		t.Fatalf("untrusted output cap was not bounded: %d", got)
	}
}

func TestEstimateTokensDoesNotCapLargeInputBody(t *testing.T) {
	body := make([]byte, 8_000_001)
	copy(body, []byte(`{"input":"`))
	body[len(body)-2] = '"'
	body[len(body)-1] = '}'
	got := estimateTokens("embeddings", body)
	if got != int64(len(body)) {
		t.Fatalf("large input estimate=%d, want actual body bytes %d", got, len(body))
	}
}

func TestRequestedOutputTokensUsesLargestSupportedCap(t *testing.T) {
	payload := map[string]any{"max_completion_tokens": float64(1), "max_tokens": float64(32768)}
	if got := requestedOutputTokens(payload); got != 32768 {
		t.Fatalf("requested output estimate=%d, want 32768", got)
	}
	if got := estimateTokens("chat/completions", []byte(`{"messages":[],"max_completion_tokens":1,"max_tokens":32768}`)); got != int64(32768+len(`{"messages":[],"max_completion_tokens":1,"max_tokens":32768}`)) {
		t.Fatalf("combined estimate=%d, want input bytes plus largest output cap", got)
	}
}

func TestValidateConstrainedInputRejectsUnsupportedMultimodalShapes(t *testing.T) {
	tests := []struct {
		name string
		kind string
		body string
	}{
		{"embedding object", "embeddings", `{"input":[{"image_url":"https://example.test/a.png"}]}`},
		{"embedding image", "embeddings", `{"input":{"type":"input_image"}}`},
		{"embedding audio", "embeddings", `{"input":{"type":"input_audio"}}`},
		{"chat modalities", "chat/completions", `{"messages":[{"role":"user","content":"hello"}],"modalities":["text","audio"]}`},
		{"chat audio", "chat/completions", `{"messages":[{"role":"user","content":"hello"}],"audio":{"voice":"alloy"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateConstrainedInput(tt.kind, []byte(tt.body)); err == nil {
				t.Fatal("multimodal request should be rejected")
			}
		})
	}
	for _, tt := range []struct {
		kind string
		body string
	}{
		{"embeddings", `{"input":["hello","world"]}`},
		{"chat/completions", `{"messages":[{"role":"user","content":"hello"}]}`},
	} {
		if err := validateConstrainedInput(tt.kind, []byte(tt.body)); err != nil {
			t.Fatalf("plain text %s should remain supported: %v", tt.kind, err)
		}
	}
}

func TestEstimateCostUsesInputOutputPricesAndNoEmbeddingOutput(t *testing.T) {
	m := &model.Model{PriceIn: 1, PriceOut: 2}
	chat := []byte(`{"messages":[{"role":"user","content":"abcdefgh"}],"max_tokens":32}`)
	if got := estimateCost("chat/completions", chat, m); got != 0.131 {
		t.Fatalf("chat cost estimate=%v, want 0.131", got)
	}
	embedding := []byte(`{"input":"abcdefgh"}`)
	if got := estimateCost("embeddings", embedding, m); got != 0.020 {
		t.Fatalf("embedding cost estimate=%v, want 0.020", got)
	}
}

func TestEstimateUsageWithoutUpstreamUsage(t *testing.T) {
	request := []byte(`{"prompt":"abcd"}`)
	response := []byte(`{"choices":[{"text":"hello"}]}`)
	prompt, completion := estimateUsage("completions", request, response)
	if prompt != 17 || completion != 2 {
		t.Fatalf("completions estimate=%d/%d, want 17/2", prompt, completion)
	}
	prompt, completion = estimateUsage("embeddings", []byte(`{"input":"abcdefgh"}`), []byte(`{"data":[]}`))
	if prompt != 20 || completion != 0 {
		t.Fatalf("embeddings estimate=%d/%d, want 20/0", prompt, completion)
	}
}
