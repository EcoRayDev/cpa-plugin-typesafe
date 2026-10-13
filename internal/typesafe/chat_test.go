package typesafe

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestChatReportsPartialUsageWithoutInventingTotals(t *testing.T) {
	p := newTestPlugin(t, Config{APIKey: "fake-key"}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0.9}},"usage":{"input_tokens":4}}`), nil
	})})
	input, _ := json.Marshal(map[string]any{"messages": []chatMessage{{
		Role: "user", Content: `{"state":{"invoice":"late"},"questions":{"q":{"type":"noul","instructions":"Is this overdue?"}}}`,
	}}})
	got, err := p.Chat(context.Background(), input, false)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Usage *chatUsage `json:"usage"`
	}
	if err = json.Unmarshal(got.Body, &response); err != nil {
		t.Fatal(err)
	}
	if response.Usage == nil || response.Usage.PromptTokens == nil || *response.Usage.PromptTokens != 4 ||
		response.Usage.CompletionTokens != nil || response.Usage.TotalTokens != nil {
		t.Fatalf("partial usage=%#v", response.Usage)
	}
}
func TestChatUsesAnswerQuestionAndPriorConversationAsState(t *testing.T) {
	var sent map[string]any
	p := newTestPlugin(t, Config{Model: "jev-configured", APIKey: "fake-key"}, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			return nil, err
		}
		return jsonResponse(http.StatusOK, `{"model":"jev-1.13.0","answers":{"answer":{"type":"noul","noul":0.98}},"usage":{"input_tokens":5}}`), nil
	})})
	input := `{"model":"client-alias","messages":[{"role":"user","content":"State: earlier"},{"role":"assistant","content":"ack"},{"role":"user","content":"What changed?"}]}`
	if _, err := p.Chat(context.Background(), []byte(input), false); err != nil {
		t.Fatal(err)
	}
	history, ok := sent["state"].([]any)
	if sent["model"] != "jev-configured" || !ok || len(history) != 2 ||
		history[0].(map[string]any)["role"] != "user" || history[0].(map[string]any)["content"] != "State: earlier" ||
		history[1].(map[string]any)["role"] != "assistant" || history[1].(map[string]any)["content"] != "ack" {
		t.Fatalf("conversation state=%#v", sent["state"])
	}
	question := sent["questions"].(map[string]any)["answer"].(map[string]any)
	if question["type"] != "noul" || question["instructions"] != "What changed?" {
		t.Fatalf("question=%#v", question)
	}
}
func TestExecutorStreamEmitsRawAnswerAndTerminalChunks(t *testing.T) {
	p := newTestPlugin(t, Config{APIKey: "fake-key"}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0.8}}}`), nil
	})})
	input, _ := json.Marshal(map[string]any{"stream": true, "messages": []chatMessage{{
		Role: "user", Content: `{"state":{},"questions":{"q":{"type":"noul","instructions":"Is this true?"}}}`,
	}}})
	got, err := p.ExecuteStream(context.Background(), pluginapi.ExecutorRequest{Payload: input, Stream: true})
	if err != nil {
		t.Fatal(err)
	}
	var events []json.RawMessage
	for chunk := range got.Chunks {
		events = append(events, chunk.Payload)
	}
	if len(events) != 2 {
		t.Fatalf("executor stream event count=%d", len(events))
	}
	var first, terminal streamCompletion
	if json.Unmarshal(events[0], &first) != nil || json.Unmarshal(events[1], &terminal) != nil {
		t.Fatal("executor chunks must be raw JSON, not pre-framed SSE")
	}
	var answers map[string]struct {
		Noul float64 `json:"noul"`
	}
	if json.Unmarshal([]byte(first.Choices[0].Delta.Content), &answers) != nil || answers["q"].Noul != 0.8 ||
		first.Choices[0].FinishReason != nil || terminal.Choices[0].FinishReason == nil ||
		*terminal.Choices[0].FinishReason != "stop" || terminal.Usage != nil ||
		terminal.ID != first.ID || terminal.Model != first.Model || terminal.Created != first.Created {
		t.Fatalf("SSE answer/termination=%#v %#v", first, terminal)
	}
}
