package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"gopkg.in/yaml.v3"
)

const (
	ProviderID      = "typesafe"
	DefaultModel    = "jev-latest"
	UpstreamURL     = "https://api.typesafe.ai/v1/systemone"
	maxResponseBody = 4 << 20
)

type Config struct{ Model, CredentialsFile, APIKey string }
type Result struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	chunks     [2][]byte
}
type HostHTTPRequest struct {
	Method           string
	URL              string
	Headers          http.Header
	Body             []byte
	DisableRedirects bool   `json:"disable_redirects"`
	MaxResponseBytes int64  `json:"max_response_bytes"`
	SuppressLogging  bool   `json:"suppress_logging,omitempty"`
	HostCallbackID   string `json:"host_callback_id,omitempty"`
	OperationID      string `json:"operation_id,omitempty"`
}
type HostHTTPClient interface {
	Do(context.Context, HostHTTPRequest) (pluginapi.HTTPResponse, error)
}
type Plugin struct {
	mu                         sync.RWMutex
	model, key, credentialPath string
	client                     *http.Client
	configuredCredential       bool
}
type requestClientKey struct{}
type credentialFile struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
}
type Settings struct {
	Model  string `json:"model"`
	APIKey string `json:"api_key,omitempty"`
}

func WithHostHTTPClient(ctx context.Context, client HostHTTPClient) context.Context {
	return context.WithValue(ctx, requestClientKey{}, client)
}
func NewWithStore(config Config, client *http.Client) (*Plugin, error) {
	if client == nil {
		client = http.DefaultClient
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if copyClient.Timeout == 0 || copyClient.Timeout > 120*time.Second {
		copyClient.Timeout = 120 * time.Second
	}
	if config.Model == "" {
		config.Model = DefaultModel
	}
	p := &Plugin{model: config.Model, client: &copyClient, credentialPath: config.CredentialsFile}
	if config.CredentialsFile != "" {
		data, err := readCredentialFile(config.CredentialsFile)
		if err == nil {
			var saved credentialFile
			if err = yaml.Unmarshal(data, &saved); err != nil {
				return nil, errors.New("invalid credential file")
			}
			p.key = saved.APIKey
			p.configuredCredential = saved.APIKey != ""
			if saved.Model != "" {
				p.model = saved.Model
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if p.key == "" && !p.configuredCredential {
		p.key = config.APIKey
		if p.key == "" {
			p.key = os.Getenv("TYPESAFE_API_KEY")
		}
	}
	if strings.ContainsAny(p.key, "\r\n") {
		return nil, errors.New("invalid upstream credential")
	}
	return p, nil
}
func (p *Plugin) Models() []string {
	p.mu.RLock()
	model := p.model
	p.mu.RUnlock()
	if model == DefaultModel {
		return []string{DefaultModel}
	}
	return []string{DefaultModel, model}
}
func (p *Plugin) Model() string  { p.mu.RLock(); defer p.mu.RUnlock(); return p.model }
func (p *Plugin) APIKey() string { p.mu.RLock(); defer p.mu.RUnlock(); return p.key }
func (p *Plugin) Forward(ctx context.Context, body []byte) (Result, error) {
	hostClient, _ := ctx.Value(requestClientKey{}).(HostHTTPClient)
	p.mu.RLock()
	key := p.key
	p.mu.RUnlock()
	if key == "" {
		return Result{}, errors.New("TypeSafe upstream credentials are not configured")
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+key)
	headers.Set("Content-Type", "application/json")
	var status int
	var responseHeaders http.Header
	var data []byte
	if hostClient != nil {
		response, err := hostClient.Do(ctx, HostHTTPRequest{Method: http.MethodPost, URL: UpstreamURL, Headers: headers, Body: body, DisableRedirects: true, MaxResponseBytes: maxResponseBody, SuppressLogging: true})
		if err != nil {
			return Result{}, redactError(err, key)
		}
		status, responseHeaders, data = response.StatusCode, response.Headers, response.Body
		if len(data) > maxResponseBody {
			return Result{}, errors.New("TypeSafe response exceeded size limit")
		}
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, UpstreamURL, bytes.NewReader(body))
		if err != nil {
			return Result{}, errors.New("could not create upstream request")
		}
		req.Header = headers
		resp, err := p.client.Do(req)
		if err != nil {
			return Result{}, redactError(err, key)
		}
		defer resp.Body.Close()
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
		if err != nil {
			return Result{}, errors.New("could not read TypeSafe response")
		}
		if len(data) > maxResponseBody {
			return Result{}, errors.New("TypeSafe response exceeded size limit")
		}
		status, responseHeaders = resp.StatusCode, resp.Header
	}
	outHeaders := http.Header{"Content-Type": {"application/json"}}
	if retry := responseHeaders.Get("Retry-After"); retry != "" && Redact(retry, key) == retry {
		outHeaders.Set("Retry-After", retry)
	}
	return Result{StatusCode: status, Headers: outHeaders, Body: redactResponseBody(data, key)}, nil
}
func (p *Plugin) Chat(ctx context.Context, body []byte, stream bool) (Result, error) {
	var request struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return Result{}, &ExecutionError{HTTPStatus: http.StatusBadRequest, Message: "invalid chat request"}
	}
	last := -1
	for i := len(request.Messages) - 1; i >= 0; i-- {
		if request.Messages[i].Role == "user" {
			last = i
			break
		}
	}
	if last < 0 {
		return Result{}, &ExecutionError{HTTPStatus: http.StatusBadRequest, Message: "a user message is required"}
	}
	question, ok := contentString(request.Messages[last].Content)
	question = strings.TrimSpace(question)
	if !ok || question == "" {
		return Result{}, &ExecutionError{HTTPStatus: http.StatusBadRequest, Message: "a non-empty text user message is required"}
	}
	var payload struct {
		Model     string          `json:"model"`
		State     json.RawMessage `json:"state"`
		Questions json.RawMessage `json:"questions"`
	}
	typed := json.Unmarshal([]byte(question), &payload) == nil &&
		len(payload.Questions) != 0 && !bytes.Equal(payload.Questions, []byte("null"))
	payload.Model = p.Model()
	if typed {
		if len(payload.State) == 0 {
			payload.State = json.RawMessage(`""`)
		}
	} else {
		history := make([]chatMessage, 0, last)
		for _, message := range request.Messages[:last] {
			if message.Role != "user" && message.Role != "assistant" {
				continue
			}
			if content, valid := contentString(message.Content); valid && content != "" {
				history = append(history, chatMessage{Role: message.Role, Content: content})
			}
		}
		if len(history) == 0 {
			payload.State, _ = json.Marshal(question)
		} else {
			payload.State, _ = json.Marshal(history)
		}
		payload.Questions, _ = json.Marshal(map[string]any{
			"answer": map[string]string{"type": "noul", "instructions": question},
		})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Result{}, &ExecutionError{HTTPStatus: http.StatusBadRequest, Message: "invalid typed request"}
	}
	upstream, err := p.Forward(ctx, encoded)
	if err != nil {
		return Result{}, err
	}
	if upstream.StatusCode < 200 || upstream.StatusCode >= 300 {
		return upstream, nil
	}
	var reply struct {
		Answers json.RawMessage `json:"answers"`
		Usage   *upstreamUsage  `json:"usage"`
	}
	if err := json.Unmarshal(upstream.Body, &reply); err != nil || len(reply.Answers) == 0 {
		return Result{}, &ExecutionError{HTTPStatus: http.StatusBadGateway, Message: "invalid TypeSafe response"}
	}
	answers := reply.Answers
	if !typed {
		var single map[string]json.RawMessage
		if err := json.Unmarshal(reply.Answers, &single); err != nil || len(single["answer"]) == 0 {
			return Result{}, &ExecutionError{HTTPStatus: http.StatusBadGateway, Message: "missing TypeSafe answer"}
		}
		answers = single["answer"]
	}
	return formatChatResult(payload.Model, answers, convertUsage(reply.Usage), stream)
}
func contentString(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, part := range parts {
			if part.Type == "text" || part.Type == "input_text" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(part.Text)
			}
		}
		return b.String(), b.Len() > 0
	}
	return "", false
}
func (p *Plugin) Execute(ctx context.Context, req pluginapi.ExecutorRequest) (pluginapi.ExecutorResponse, error) {
	result, err := p.Chat(ctx, req.Payload, false)
	if err != nil {
		return pluginapi.ExecutorResponse{}, err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return pluginapi.ExecutorResponse{}, &ExecutionError{HTTPStatus: result.StatusCode, Message: string(result.Body), ResponseHeaders: result.Headers}
	}
	return pluginapi.ExecutorResponse{Payload: result.Body, Headers: result.Headers}, nil
}
func (p *Plugin) ExecuteStream(ctx context.Context, req pluginapi.ExecutorRequest) (pluginapi.ExecutorStreamResponse, error) {
	result, err := p.Chat(ctx, req.Payload, true)
	if err != nil {
		return pluginapi.ExecutorStreamResponse{}, err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return pluginapi.ExecutorStreamResponse{}, &ExecutionError{HTTPStatus: result.StatusCode, Message: string(result.Body), ResponseHeaders: result.Headers}
	}
	chunks := make(chan pluginapi.ExecutorStreamChunk, len(result.chunks))
	for _, payload := range result.chunks {
		chunks <- pluginapi.ExecutorStreamChunk{Payload: payload}
	}
	close(chunks)
	return pluginapi.ExecutorStreamResponse{Headers: result.Headers, Chunks: chunks}, nil
}
func (p *Plugin) CountTokens(context.Context, pluginapi.ExecutorRequest) (pluginapi.ExecutorResponse, error) {
	return pluginapi.ExecutorResponse{}, &ExecutionError{HTTPStatus: http.StatusNotImplemented, Message: "TypeSafe does not support token counting"}
}

type ExecutionError struct {
	HTTPStatus      int
	Message         string
	ResponseHeaders http.Header
}

func (e *ExecutionError) Error() string        { return e.Message }
func (e *ExecutionError) StatusCode() int      { return e.HTTPStatus }
func (e *ExecutionError) Headers() http.Header { return e.ResponseHeaders }
func (p *Plugin) HttpRequest(context.Context, pluginapi.ExecutorHTTPRequest) (pluginapi.ExecutorHTTPResponse, error) {
	return pluginapi.ExecutorHTTPResponse{}, errors.New("unsupported HTTP executor operation")
}
func (p *Plugin) Identifier() string { return ProviderID }
func (p *Plugin) RegisterModels(context.Context, pluginapi.ModelRegistrationRequest) (pluginapi.ModelRegistrationResponse, error) {
	models := p.Models()
	out := make([]pluginapi.ModelInfo, 0, len(models))
	for _, model := range models {
		out = append(out, pluginapi.ModelInfo{ID: model, Object: "model", OwnedBy: ProviderID, Name: model, DisplayName: model, SupportedGenerationMethods: []string{"chat.completions"}, SupportedInputModalities: []string{"text"}, SupportedOutputModalities: []string{"text"}})
	}
	return pluginapi.ModelRegistrationResponse{Provider: ProviderID, Models: out}, nil
}
func (p *Plugin) UpstreamTest(ctx context.Context) error {
	body, err := json.Marshal(map[string]any{
		"model": p.Model(), "state": "2 + 2 = 4",
		"questions": map[string]any{"connection": map[string]string{
			"type": "noul", "instructions": "Is the statement correct?",
		}},
	})
	if err != nil {
		return errors.New("could not create connection test")
	}
	result, err := p.Forward(ctx, body)
	if err != nil {
		return err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return fmt.Errorf("TypeSafe returned HTTP %d", result.StatusCode)
	}
	return nil
}
func (p *Plugin) SaveSettings(ctx context.Context, settings Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.credentialPath == "" {
		return errors.New("configure a private credentials_file before saving credentials")
	}
	if strings.TrimSpace(settings.Model) == "" {
		settings.Model = p.model
	}
	key := p.key
	if strings.TrimSpace(settings.APIKey) != "" {
		key = strings.TrimSpace(settings.APIKey)
		if strings.ContainsAny(key, "\r\n") {
			return errors.New("invalid upstream credential")
		}
	}
	if err := writeCredentialFile(p.credentialPath, credentialFile{APIKey: key, Model: settings.Model}); err != nil {
		return err
	}
	p.key, p.model, p.configuredCredential = key, settings.Model, key != ""
	return nil
}
func (p *Plugin) NativeForward(ctx context.Context, body []byte) Result {
	result, err := p.Forward(ctx, body)
	if err != nil {
		return Result{StatusCode: http.StatusBadGateway, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"error":"upstream request failed"}`)}
	}
	return result
}
func (p *Plugin) ConfigYAML() []byte {
	p.mu.RLock()
	defer p.mu.RUnlock()
	b, _ := yaml.Marshal(map[string]string{"model": p.model, "credentials_file": p.credentialPath})
	return b
}

var _ pluginapi.ModelRegistrar = (*Plugin)(nil)
var _ pluginapi.ProviderExecutor = (*Plugin)(nil)
