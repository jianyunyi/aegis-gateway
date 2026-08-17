// Mock 上游：一个 OpenAI 兼容的假模型服务（仅用于本地演示与集成测试，零成本）。
// 用法：go run ./cmd/mockupstream （默认监听 :8099）
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handleChat)
	mux.HandleFunc("/v1/completions", handleChat)
	mux.HandleFunc("/v1/embeddings", handleEmbeddings)
	log.Println("mock upstream listening on :8099")
	log.Fatal(http.ListenAndServe(":8099", mux))
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	userMsg := ""
	if len(req.Messages) > 0 {
		userMsg = req.Messages[len(req.Messages)-1].Content
	}
	reply := fmt.Sprintf("这是 mock 上游（%s）对【%s】的流式回复。", req.Model, userMsg)

	if !req.Stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "mock-1", "object": "chat.completion", "created": time.Now().Unix(),
			"model": req.Model,
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": reply}, "finish_reason": "stop",
			}},
			"usage": usage{PromptTokens: 10, CompletionTokens: len([]rune(reply)), TotalTokens: 10 + len([]rune(reply))},
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fl, _ := w.(http.Flusher)

	runes := []rune(reply)
	for i, ch := range runes {
		finish := ""
		if i == len(runes)-1 {
			finish = "stop"
		}
		writeChunk(w, fl, req.Model, string(ch), finish)
	}

	// 结束块带 usage（模拟 DeepSeek/OpenAI 行为）
	u := usage{PromptTokens: 10, CompletionTokens: len(runes), TotalTokens: 10 + len(runes)}
	data, _ := json.Marshal(map[string]any{
		"id": "mock-1", "object": "chat.completion.chunk", "created": time.Now().Unix(),
		"model": req.Model, "choices": []any{}, "usage": u,
	})
	fmt.Fprintf(w, "data: %s\n\n", data)
	fl.Flush()
	fmt.Fprint(w, "data: [DONE]\n\n")
	fl.Flush()
}

func writeChunk(w http.ResponseWriter, fl http.Flusher, model, content, finish string) {
	data, _ := json.Marshal(map[string]any{
		"id": "mock-1", "object": "chat.completion.chunk", "created": time.Now().Unix(),
		"model": model,
		"choices": []map[string]any{{
			"index": 0, "delta": map[string]any{"content": content}, "finish_reason": finish,
		}},
	})
	fmt.Fprintf(w, "data: %s\n\n", data)
	fl.Flush()
	time.Sleep(60 * time.Millisecond)
}

func handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   []map[string]any{{"object": "embedding", "index": 0, "embedding": []float64{0.1, 0.2, 0.3}}},
		"model":  "mock-embedding", "usage": usage{PromptTokens: 5, TotalTokens: 5},
	})
}
