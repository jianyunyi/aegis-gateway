package responsecache

import "testing"

func TestKeyFor(t *testing.T) {
	body := []byte(`{"model":"m","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	k1 := KeyFor(body)
	k2 := KeyFor(body)
	if k1 != k2 {
		t.Fatalf("相同请求应产生相同 key: %s vs %s", k1, k2)
	}
	if !hasPrefix(k1, "pc:") {
		t.Errorf("key 应以 pc: 开头: %s", k1)
	}
	// 不同内容 → 不同 key
	other := KeyFor([]byte(`{"model":"m","stream":false,"messages":[{"role":"user","content":"bye"}]}`))
	if k1 == other {
		t.Errorf("不同请求不应命中同一 key")
	}
}

func hasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
