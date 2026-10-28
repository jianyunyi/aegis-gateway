package privacy

import (
	"encoding/json"
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

func TestRedactMessagesRedactsNestedSensitiveFieldsAndLeavesStructure(t *testing.T) {
	input := `[{"role":"user","name":"keep-me","content":[{"type":"text","text":"plain text"}],"metadata":{"password":{"nested":["credential"]},"items":[{"api_key":12345,"label":"keep-me-too"}],"openai_api_key":"short-api-secret","clientSecret":"short-client-secret","authorization":{"token":"short-auth-secret"}},"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\"token\":{\"value\":\"nested-token\"},\"query\":\"keep-query\"}"}}]}]`

	got, err := RedactMessages(input)
	if err != nil {
		t.Fatal(err)
	}
	var messages []map[string]any
	if err := json.Unmarshal([]byte(got), &messages); err != nil {
		t.Fatalf("redacted result is not JSON: %v", err)
	}
	if messages[0]["name"] != "keep-me" || messages[0]["role"] != "user" {
		t.Fatalf("non-sensitive message fields were not preserved: %s", got)
	}
	for _, secret := range []string{"short-api-secret", "short-client-secret", "short-auth-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sensitive structured value %q remained: %s", secret, got)
		}
	}
	metadata := messages[0]["metadata"].(map[string]any)
	if metadata["password"] != redactedValue {
		t.Fatalf("nested password was not replaced as a whole: %s", got)
	}
	items := metadata["items"].([]any)
	if items[0].(map[string]any)["api_key"] != redactedValue || items[0].(map[string]any)["label"] != "keep-me-too" {
		t.Fatalf("nested array structure or values were not preserved: %s", got)
	}
	calls := messages[0]["tool_calls"].([]any)
	function := calls[0].(map[string]any)["function"].(map[string]any)
	arguments := function["arguments"].(string)
	var decodedArguments map[string]any
	if err := json.Unmarshal([]byte(arguments), &decodedArguments); err != nil {
		t.Fatalf("serialized function arguments were not kept as JSON: %v", err)
	}
	if decodedArguments["token"] != redactedValue || decodedArguments["query"] != "keep-query" {
		t.Fatalf("nested function arguments were not redacted safely: %s", got)
	}
}

func TestRedactMessagesRedactsAllStringLeavesAndKeepsMultimodalShape(t *testing.T) {
	input := `[{"role":"assistant","content":[{"type":"text","text":"Contact user@example.com with Bearer abcdefghijklmnop"},{"type":"refusal","refusal":"secret=refusal-value"},{"type":"image_url","image_url":{"url":"https://example.invalid/image.png","detail":"high"}},{"type":"unknown_part","payload":{"values":["api_test_123456789","safe-value"]}}]}]`

	got, err := RedactMessages(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{"user@example.com", "abcdefghijklmnop", "refusal-value", "api_test_123456789"} {
		if strings.Contains(got, sensitive) {
			t.Fatalf("sensitive leaf %q remained: %s", sensitive, got)
		}
	}
	for _, preserved := range []string{"unknown_part", "safe-value", "https://example.invalid/image.png", `"detail":"high"`} {
		if !strings.Contains(got, preserved) {
			t.Fatalf("non-sensitive structure/value %q was lost: %s", preserved, got)
		}
	}
}

func TestRedactMessagesPreservesFieldOrderWhenNoRedactionIsNeeded(t *testing.T) {
	input := `[{"role":"user","content":"plain synthetic request"}]`
	got, err := RedactMessages(input)
	if err != nil {
		t.Fatal(err)
	}
	if got != input {
		t.Fatalf("message JSON changed without a redaction: got %s want %s", got, input)
	}
}
