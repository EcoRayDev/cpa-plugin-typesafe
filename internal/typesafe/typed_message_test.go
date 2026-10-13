package typesafe

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestChatUsesTypedQuestionsFromLastUserMessage(t *testing.T) {
	var sent struct {
		Model     string                     `json:"model"`
		State     map[string]string          `json:"state"`
		Questions map[string]json.RawMessage `json:"questions"`
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(request.Body).Decode(&sent); err != nil {
			return nil, err
		}
		return jsonResponse(http.StatusOK, `{"model":"jev-1.13.0","answers":{"overdue":{"type":"noul","noul":0.97}},"usage":{"input_tokens":280,"output_tokens":21}}`), nil
	})}
	plugin := newTestPlugin(t, Config{Model: "jev-pinned", APIKey: "fixture-key"}, client)
	message := `{"state":{"invoice":"late"},"questions":{"overdue":{"type":"noul","instructions":"Is the invoice overdue?"}}}`
	request, err := json.Marshal(map[string]any{
		"model":    "jev-latest",
		"messages": []map[string]string{{"role": "user", "content": message}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := plugin.Chat(context.Background(), request, false)
	if err != nil {
		t.Fatal(err)
	}
	if sent.Model != "jev-pinned" || sent.State["invoice"] != "late" || sent.Questions["overdue"] == nil || sent.Questions["answer"] != nil {
		t.Fatalf("typed message was converted to an unrelated question: %#v", sent)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", result.StatusCode)
	}
}
