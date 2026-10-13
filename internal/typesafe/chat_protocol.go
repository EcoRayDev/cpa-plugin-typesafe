package typesafe

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"time"
)

type upstreamUsage struct {
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
}

type chatUsage struct {
	PromptTokens     *int64 `json:"prompt_tokens,omitempty"`
	CompletionTokens *int64 `json:"completion_tokens,omitempty"`
	TotalTokens      *int64 `json:"total_tokens,omitempty"`
}

func convertUsage(upstream *upstreamUsage) *chatUsage {
	if upstream == nil || (upstream.InputTokens == nil && upstream.OutputTokens == nil) {
		return nil
	}
	usage := &chatUsage{PromptTokens: upstream.InputTokens, CompletionTokens: upstream.OutputTokens}
	if upstream.InputTokens != nil && upstream.OutputTokens != nil {
		total := *upstream.InputTokens + *upstream.OutputTokens
		usage.TotalTokens = &total
	}
	return usage
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chatCompletion struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices [1]chatChoice `json:"choices"`
	Usage   *chatUsage    `json:"usage,omitempty"`
}

type streamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type streamChoice struct {
	Index        int         `json:"index"`
	Delta        streamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type streamCompletion struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices [1]streamChoice `json:"choices"`
	Usage   *chatUsage      `json:"usage,omitempty"`
}

func formatChatResult(model string, answers []byte, usage *chatUsage, stream bool) (Result, error) {
	id := "chatcmpl-" + rand.Text()
	created := time.Now().Unix()
	if !stream {
		body, err := json.Marshal(chatCompletion{
			ID: id, Object: "chat.completion", Created: created, Model: model,
			Choices: [1]chatChoice{{Message: chatMessage{Role: "assistant", Content: string(answers)}, FinishReason: "stop"}},
			Usage:   usage,
		})
		return Result{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": {"application/json"}}, Body: body}, err
	}
	var payloads [2][]byte
	chunk := streamCompletion{
		ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
		Choices: [1]streamChoice{{Delta: streamDelta{Role: "assistant", Content: string(answers)}}},
	}
	for i := range payloads {
		encoded, err := json.Marshal(chunk)
		if err != nil {
			return Result{}, err
		}
		payloads[i] = encoded
		stop := "stop"
		chunk.Choices[0].Delta = streamDelta{}
		chunk.Choices[0].FinishReason = &stop
		chunk.Usage = usage
	}
	return Result{StatusCode: http.StatusOK, Headers: http.Header{
		"Content-Type": {"text/event-stream"}, "Cache-Control": {"no-cache"},
	}, chunks: payloads}, nil
}
