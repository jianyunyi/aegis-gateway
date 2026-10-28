// Package privacy contains conservative, deterministic redaction helpers for
// optional prompt capture. Redaction is defense-in-depth, not a guarantee that
// arbitrary personal or confidential information can be identified.
package privacy

import (
	"encoding/json"
	"regexp"
	"strings"
)

var redactions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)\b(?:sk|pk|ak|api)[_-][A-Za-z0-9_-]{8,}\b`),
	regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`),
	regexp.MustCompile(`\b\+?[0-9][0-9 ()-]{7,}[0-9]\b`),
	regexp.MustCompile(`(?i)\b(?:\d[ -]*?){13,19}\b`),
	regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?key)(\s*[:=]\s*["']?)[^\s,"'}]+`),
}

// Redact replaces common credential and direct-identifier patterns.
func Redact(text string) string {
	out := text
	for i, pattern := range redactions {
		switch i {
		case 0:
			out = pattern.ReplaceAllString(out, `${1}[REDACTED]`)
		case 5:
			out = pattern.ReplaceAllString(out, `${1}${2}[REDACTED]`)
		default:
			out = pattern.ReplaceAllString(out, "[REDACTED]")
		}
	}
	return out
}

type message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// RedactMessages redacts textual content in a serialized OpenAI messages array.
func RedactMessages(encoded string) (string, error) {
	var messages []message
	if err := json.Unmarshal([]byte(encoded), &messages); err != nil {
		return "", err
	}
	for i := range messages {
		var content string
		if err := json.Unmarshal(messages[i].Content, &content); err == nil {
			redacted, err := json.Marshal(Redact(content))
			if err != nil {
				return "", err
			}
			messages[i].Content = redacted
			continue
		}
		var parts []map[string]any
		if err := json.Unmarshal(messages[i].Content, &parts); err == nil {
			for _, part := range parts {
				if text, ok := part["text"].(string); ok {
					part["text"] = Redact(text)
				}
			}
			redacted, err := json.Marshal(parts)
			if err != nil {
				return "", err
			}
			messages[i].Content = redacted
		}
	}
	encodedMessages, err := json.Marshal(messages)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(encodedMessages)), nil
}
