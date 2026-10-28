package repository

import "testing"

func TestValidIdempotencyKey(t *testing.T) {
	for _, key := range []string{"order-123", "a.b_~1"} {
		if !ValidIdempotencyKey(key) {
			t.Fatalf("key %q should be valid", key)
		}
	}
	for _, key := range []string{"", "bad key", "a/b", "a:b"} {
		if ValidIdempotencyKey(key) {
			t.Fatalf("key %q should be invalid", key)
		}
	}
	if ValidIdempotencyKey(string(make([]byte, 129))) {
		t.Fatal("oversized key should be invalid")
	}
}

func TestIdempotencyFingerprintChangesWithBody(t *testing.T) {
	base := IdempotencyFingerprint("POST", "/v1/chat/completions?x=1", "application/json", []byte("a"))
	if base == IdempotencyFingerprint("POST", "/v1/chat/completions?x=1", "application/json", []byte("b")) {
		t.Fatal("different bodies must not share a fingerprint")
	}
	if base != IdempotencyFingerprint("POST", "/v1/chat/completions?x=1", "application/json", []byte("a")) {
		t.Fatal("same request must have a stable fingerprint")
	}
	for _, tc := range []struct {
		name, method, uri, contentType string
	}{
		{"method", "GET", "/v1/chat/completions?x=1", "application/json"},
		{"path", "POST", "/v1/embeddings", "application/json"},
		{"query", "POST", "/v1/chat/completions?x=2", "application/json"},
		{"content-type", "POST", "/v1/chat/completions?x=1", "application/json; charset=utf-8"},
	} {
		got := IdempotencyFingerprint(tc.method, tc.uri, tc.contentType, []byte("a"))
		if got == base {
			t.Fatalf("different %s must not share a fingerprint", tc.name)
		}
	}
}
