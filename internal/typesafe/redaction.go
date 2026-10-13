package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf16"
)

func redactLiteral(value, secret string) string {
	value = strings.ReplaceAll(value, secret, "[REDACTED]")
	query := url.QueryEscape(secret)
	if query != secret {
		value = strings.ReplaceAll(value, query, "[REDACTED]")
	}
	path := url.PathEscape(secret)
	if path != secret && path != query {
		value = strings.ReplaceAll(value, path, "[REDACTED]")
	}
	if strings.IndexFunc(secret, func(r rune) bool {
		return r < 32 || r == '"' || r == '\\' || r == '<' || r == '>' || r == '&' || r == '\u2028' || r == '\u2029'
	}) >= 0 {
		encoded, _ := json.Marshal(secret)
		value = strings.ReplaceAll(value, string(encoded[1:len(encoded)-1]), "[REDACTED]")
	}
	return value
}

func decodeJSON(value string) (any, bool) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if decoder.Decode(&decoded) != nil || strings.TrimSpace(value[decoder.InputOffset():]) != "" {
		return nil, false
	}
	return decoded, true
}

// Providers sometimes JSON-escape a credential twice inside an error string.
func decodeUnicodeEscapes(value string) string {
	if !strings.Contains(value, `\u`) {
		return value
	}
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); {
		if i+6 <= len(value) && value[i:i+2] == `\u` {
			code, err := strconv.ParseUint(value[i+2:i+6], 16, 16)
			if err == nil {
				r := rune(code)
				i += 6
				if utf16.IsSurrogate(r) && i+6 <= len(value) && value[i:i+2] == `\u` {
					low, err := strconv.ParseUint(value[i+2:i+6], 16, 16)
					if err == nil && code >= 0xd800 && code <= 0xdbff && low >= 0xdc00 && low <= 0xdfff {
						r = utf16.DecodeRune(r, rune(low))
						i += 6
					}
				}
				out.WriteRune(r)
				continue
			}
		}
		out.WriteByte(value[i])
		i++
	}
	return out.String()
}

func redactDecodedString(value, secret string, depth int) (string, bool) {
	masked := redactLiteral(value, secret)
	changed := masked != value
	decoded := decodeUnicodeEscapes(masked)
	if redacted := redactLiteral(decoded, secret); redacted != decoded {
		masked, changed = redacted, true
	}
	if !strings.Contains(masked, `\`) {
		return masked, changed
	}
	if depth >= 32 {
		return "[REDACTED]", true
	}
	if nested, valid := decodeJSON(masked); valid {
		if redacted, nestedChanged := redactJSONValue(nested, secret, depth+1); nestedChanged {
			encoded, _ := json.Marshal(redacted)
			return string(encoded), true
		}
	}
	return masked, changed
}

func redactJSONValue(value any, secret string, depth int) (any, bool) {
	changed := false
	switch current := value.(type) {
	case string:
		return redactDecodedString(current, secret, depth)
	case []any:
		for i := range current {
			item, itemChanged := redactJSONValue(current[i], secret, depth)
			current[i], changed = item, changed || itemChanged
		}
	case map[string]any:
		for key, item := range current {
			maskedKey, keyChanged := redactDecodedString(key, secret, depth)
			maskedItem, itemChanged := redactJSONValue(item, secret, depth)
			if keyChanged {
				delete(current, key)
			}
			current[maskedKey] = maskedItem
			changed = changed || keyChanged || itemChanged
		}
	}
	return value, changed
}

// Redact handles literal, URL-encoded and JSON-escaped credentials, including
// object keys and nested JSON strings, without rewriting unaffected responses.
func Redact(value, secret string) string {
	if secret == "" {
		return value
	}
	masked := redactLiteral(value, secret)
	if !strings.Contains(masked, `\`) {
		return masked
	}
	if decoded, valid := decodeJSON(masked); valid {
		if redacted, changed := redactJSONValue(decoded, secret, 0); changed {
			encoded, _ := json.Marshal(redacted)
			return string(encoded)
		}
	} else {
		decoded := decodeUnicodeEscapes(masked)
		if redacted := redactLiteral(decoded, secret); redacted != decoded {
			return redacted
		}
	}
	return masked
}

func redactError(err error, secret string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return errors.New(Redact(err.Error(), secret))
}

func redactResponseBody(body []byte, secret string) []byte {
	if !bytes.Contains(body, []byte(secret)) && bytes.IndexAny(body, "\\%") < 0 {
		return body
	}
	original := string(body)
	redacted := Redact(original, secret)
	if redacted == original {
		return body
	}
	return []byte(redacted)
}
