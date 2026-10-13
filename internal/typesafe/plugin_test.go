package typesafe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestModelsExposeJevLatestAndConfiguredModel(t *testing.T) {
	p := newTestPlugin(t, Config{Model: "jev-custom", APIKey: "test-only-key"}, http.DefaultClient)
	models := p.Models()
	if len(models) != 2 || models[0] != "jev-latest" || models[1] != "jev-custom" {
		t.Fatalf("models=%#v", models)
	}
}
func TestForwardPreservesStatusBodyAndRetryAfter(t *testing.T) {
	const payload = `{"state":"a","questions":{"q":{"type":"noul","instructions":"b"}}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Method != http.MethodPost {
			t.Errorf("request=%s %s", r.Method, r.URL)
		}
		got, _ := io.ReadAll(r.Body)
		if string(got) != payload {
			t.Errorf("body=%s", got)
		}
		w.Header().Set("Retry-After", "17")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"busy"}`))
	}))
	defer upstream.Close()
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		target := *r
		backend, _ := url.Parse(upstream.URL)
		target.URL = new(url.URL)
		*target.URL = *r.URL
		target.URL.Scheme, target.URL.Host = backend.Scheme, backend.Host
		return upstream.Client().Do(&target)
	})
	p := newTestPlugin(t, Config{APIKey: "test-only-key"}, &http.Client{Transport: transport})
	got, err := p.Forward(context.Background(), []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	if got.StatusCode != http.StatusTooManyRequests || got.Headers.Get("Retry-After") != "17" || string(got.Body) != `{"error":"busy"}` {
		t.Fatalf("response=%#v", got)
	}
}
func TestChatForwardsTypedStateQuestionsAndDoesNotInventUsage(t *testing.T) {
	var sent map[string]any
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			return nil, err
		}
		return jsonResponse(http.StatusOK, `{"model":"jev-1.13.0","answers":{"overdue":{"type":"noul","noul":0.97}},"usage":{"input_tokens":280,"output_tokens":21}}`), nil
	})
	p := newTestPlugin(t, Config{APIKey: "test-only-key"}, &http.Client{Transport: transport})
	body, _ := json.Marshal(map[string]any{"model": "jev-latest", "messages": []chatMessage{{
		Role: "user", Content: `{"state":{"invoice":"late"},"questions":{"overdue":{"type":"noul","instructions":"Is it overdue?"}}}`,
	}}})
	got, err := p.Chat(context.Background(), body, false)
	if err != nil {
		t.Fatal(err)
	}
	if sent["state"].(map[string]any)["invoice"] != "late" || sent["questions"].(map[string]any)["overdue"] == nil {
		t.Fatalf("typed payload=%#v", sent)
	}
	var response struct {
		Model   string    `json:"model"`
		Usage   chatUsage `json:"usage"`
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(got.Body, &response); err != nil {
		t.Fatal(err)
	}
	if response.Model != "jev-latest" || response.Usage.PromptTokens == nil || *response.Usage.PromptTokens != 280 ||
		response.Usage.CompletionTokens == nil || *response.Usage.CompletionTokens != 21 ||
		response.Usage.TotalTokens == nil || *response.Usage.TotalTokens != 301 {
		t.Fatalf("chat usage=%#v", response.Usage)
	}
	var answers map[string]struct {
		Noul float64 `json:"noul"`
	}
	if len(response.Choices) != 1 || json.Unmarshal([]byte(response.Choices[0].Message.Content), &answers) != nil || answers["overdue"].Noul != 0.97 {
		t.Fatalf("typed answer=%#v", response.Choices)
	}
}
func TestRedactNestedAndEscapedSecrets(t *testing.T) {
	secret := "fake-secret-regression-value"
	input := `{"nested":{"error":"` + secret + `"},"detail":"\\\"api_key\\\":\\\"` + secret + `\\\""}`
	got := Redact(input, secret)
	if strings.Contains(got, secret) {
		t.Fatalf("secret leaked: %s", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func newTestPlugin(t *testing.T, config Config, client *http.Client) *Plugin {
	t.Helper()
	plugin, err := NewWithStore(config, client)
	if err != nil {
		t.Fatal(err)
	}
	return plugin
}

func privateTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
