package privacy

import (
	"strings"
	"testing"
)

func TestRedactCommonSensitiveValues(t *testing.T) {
	input := `contact jianyunyi@example.com, call +1 (415) 555-2671, Authorization: Bearer abcdefghijklmnop, api_key=secretvalue123`
	got := Redact(input)
	for _, value := range []string{"jianyunyi@example.com", "415) 555-2671", "abcdefghijklmnop", "secretvalue123"} {
		if strings.Contains(got, value) {
			t.Fatalf("sensitive value %q remained in %q", value, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction markers, got %q", got)
	}
}

func TestRedactMessagesPreservesStructureAndCleansTextParts(t *testing.T) {
	got, err := RedactMessages(`[{"role":"system","content":"secret=abc1234567890"},{"role":"user","content":[{"type":"text","text":"mail me at a@example.com"},{"type":"image_url","image_url":{"url":"https://example.invalid/image"}}]}]`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "abc1234567890") || strings.Contains(got, "a@example.com") {
		t.Fatalf("sensitive text remained: %s", got)
	}
	if !strings.Contains(got, `"image_url"`) || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("message structure was not preserved: %s", got)
	}
}
