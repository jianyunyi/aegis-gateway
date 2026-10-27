package handler

import (
	"testing"

	"aegis-gateway/internal/model"
)

func TestEstimateTokensIncludesInputAndRequestedOutput(t *testing.T) {
	chat := []byte(`{"messages":[{"role":"user","content":"abcdefgh"}],"max_completion_tokens":32}`)
	if got := estimateTokens("chat/completions", chat); got != 34 {
		t.Fatalf("chat estimate=%d, want 34", got)
	}
	completion := []byte(`{"prompt":"abcd","max_tokens":12}`)
	if got := estimateTokens("completions", completion); got != 13 {
		t.Fatalf("completion estimate=%d, want 13", got)
	}
	embedding := []byte(`{"input":["abcd","efgh"]}`)
	if got := estimateTokens("embeddings", embedding); got != 2 {
		t.Fatalf("embedding estimate=%d, want 2", got)
	}
	if got := estimateTokens("chat/completions", []byte(`{"messages":[]}`)); got != 4096 {
		t.Fatalf("default output estimate=%d, want 4096", got)
	}
}

func TestEstimateTokensCapsUntrustedMaxTokens(t *testing.T) {
	if got := estimateTokens("completions", []byte(`{"prompt":"x","max_tokens":999999999}`)); got > 4_000_001 {
		t.Fatalf("untrusted output cap was not bounded: %d", got)
	}
}

func TestEstimateCostUsesInputOutputPricesAndNoEmbeddingOutput(t *testing.T) {
	m := &model.Model{PriceIn: 1, PriceOut: 2}
	chat := []byte(`{"messages":[{"role":"user","content":"abcdefgh"}],"max_tokens":32}`)
	if got := estimateCost("chat/completions", chat, m); got != 0.066 {
		t.Fatalf("chat cost estimate=%v, want 0.066", got)
	}
	embedding := []byte(`{"input":"abcdefgh"}`)
	if got := estimateCost("embeddings", embedding, m); got != 0.002 {
		t.Fatalf("embedding cost estimate=%v, want 0.002", got)
	}
}

func TestEstimateUsageWithoutUpstreamUsage(t *testing.T) {
	request := []byte(`{"prompt":"abcd"}`)
	response := []byte(`{"choices":[{"text":"hello"}]}`)
	prompt, completion := estimateUsage("completions", request, response)
	if prompt != 1 || completion != 2 {
		t.Fatalf("completions estimate=%d/%d, want 1/2", prompt, completion)
	}
	prompt, completion = estimateUsage("embeddings", []byte(`{"input":"abcdefgh"}`), []byte(`{"data":[]}`))
	if prompt != 2 || completion != 0 {
		t.Fatalf("embeddings estimate=%d/%d, want 2/0", prompt, completion)
	}
}
