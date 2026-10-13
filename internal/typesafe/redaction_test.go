package typesafe

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactRemovesDecodedCredentialFromValuesAndObjectKeys(t *testing.T) {
	const secret = "fake-secret-regression-value"
	inputs := []string{
		`{"error":"Invalid credential: \u0066ake-secret-regression-value"}`,
		`{"\u0066ake-secret-regression-value":{"error":"denied"}}`,
		`{"detail":"{\"error\":\"\\u0066ake-secret-regression-value\"}"}`,
		`{"error":"Invalid credential: \\u0066ake-secret-regression-value"}`,
	}
	for _, input := range inputs {
		got := Redact(input, secret)
		var decoded any
		if err := json.Unmarshal([]byte(got), &decoded); err != nil {
			t.Fatalf("redacted response is not valid JSON: %v", err)
		}
		encoded, _ := json.Marshal(decoded)
		if strings.Contains(string(encoded), secret) || !strings.Contains(string(encoded), "[REDACTED]") {
			t.Fatalf("credential was not removed from decoded response: %s", encoded)
		}
	}
}
