// Package privacy contains conservative, deterministic redaction helpers for
// optional prompt capture. Redaction is defense-in-depth, not a guarantee that
// arbitrary personal or confidential information can be identified.
package privacy

import (
	"bytes"
	"encoding/json"
	"io"
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

const redactedValue = "[REDACTED]"

type orderedField struct {
	key   string
	value any
}

type orderedObject []orderedField

func (object orderedObject) MarshalJSON() ([]byte, error) {
	var encoded bytes.Buffer
	encoded.WriteByte('{')
	for i, field := range object {
		if i > 0 {
			encoded.WriteByte(',')
		}
		key, err := json.Marshal(field.key)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(field.value)
		if err != nil {
			return nil, err
		}
		encoded.Write(key)
		encoded.WriteByte(':')
		encoded.Write(value)
	}
	encoded.WriteByte('}')
	return encoded.Bytes(), nil
}

func decodeOrderedJSON(encoded string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.UseNumber()
	value, err := readOrderedValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return value, nil
}

func readOrderedValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := orderedObject{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, io.ErrUnexpectedEOF
			}
			value, err := readOrderedValue(decoder)
			if err != nil {
				return nil, err
			}
			object = append(object, orderedField{key: key, value: value})
		}
		_, err := decoder.Token()
		return object, err
	case json.Delim('['):
		array := []any{}
		for decoder.More() {
			value, err := readOrderedValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		_, err := decoder.Token()
		return array, err
	case json.Delim(']'), json.Delim('}'):
		return nil, io.ErrUnexpectedEOF
	default:
		return token, nil
	}
}

// sensitiveField reports whether a JSON object key identifies a value that
// must be removed in its entirety. Normalizing separators also covers common
// spellings such as api-key, access_token, and client-secret.
func sensitiveField(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.NewReplacer("-", "", "_", "", " ", "").Replace(key)
	for _, name := range []string{
		"password", "passwd", "secret", "token", "apikey", "accesstoken",
		"refreshtoken", "clientsecret", "privatekey", "authorization",
		"credential", "credentials",
	} {
		if key == name || strings.HasSuffix(key, name) {
			return true
		}
	}
	return false
}

// redactJSONValue recursively preserves JSON objects and arrays while redacting
// every string leaf. A string containing a JSON object/array (for example
// tool_calls.function.arguments) is kept a string, but its nested JSON is
// decoded, redacted, and encoded again.
func redactJSONValue(value any) any {
	switch value := value.(type) {
	case orderedObject:
		for i := range value {
			if sensitiveField(value[i].key) {
				value[i].value = redactedValue
				continue
			}
			value[i].value = redactJSONValue(value[i].value)
		}
		return value
	case map[string]any:
		for key, child := range value {
			if sensitiveField(key) {
				value[key] = redactedValue
				continue
			}
			value[key] = redactJSONValue(child)
		}
		return value
	case []any:
		for i, child := range value {
			value[i] = redactJSONValue(child)
		}
		return value
	case string:
		trimmed := strings.TrimSpace(value)
		if len(trimmed) >= 2 && (trimmed[0] == '{' || trimmed[0] == '[') {
			if nested, err := decodeOrderedJSON(trimmed); err == nil {
				switch nested.(type) {
				case orderedObject, []any:
					encoded, err := json.Marshal(redactJSONValue(nested))
					if err == nil {
						return Redact(string(encoded))
					}
				}
			}
		}
		return Redact(value)
	default:
		return value
	}
}

// RedactMessages redacts every string leaf in a serialized OpenAI messages
// array while preserving the complete JSON message structure.
func RedactMessages(encoded string) (string, error) {
	messages, err := decodeOrderedJSON(encoded)
	if err != nil {
		return "", err
	}
	if messages != nil {
		if _, ok := messages.([]any); !ok {
			return "", io.ErrUnexpectedEOF
		}
	}
	redacted := redactJSONValue(messages)
	encodedMessages, err := json.Marshal(redacted)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(encodedMessages)), nil
}
