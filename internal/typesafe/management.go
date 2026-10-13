package typesafe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type NativeRegistrationRequest struct {
	Plugin   pluginapi.Metadata
	BasePath string
}
type NativeRegistrationResponse struct{ Routes []NativeRoute }
type NativeRoute struct {
	Method  string
	Path    string
	Handler NativeHandler `json:"-"`
}
type NativeHandler interface {
	HandleNative(context.Context, NativeRequest) (NativeResponse, error)
}
type NativeRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Query   url.Values
	Body    []byte
}
type NativeResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

type managementHandler struct{ plugin *Plugin }

func (h managementHandler) HandleManagement(ctx context.Context, request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	if request.Method == http.MethodGet && request.Path == "/v0/resource/plugins/typesafe/settings" {
		return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}, Body: []byte(h.plugin.ResourceHTML())}, nil
	}
	var result Result
	switch {
	case request.Method == http.MethodGet && request.Path == "/v0/management/plugins/typesafe/settings":
		result = h.plugin.settingsResponse()
	case request.Method == http.MethodPut && request.Path == "/v0/management/plugins/typesafe/settings":
		var update Settings
		if err := json.Unmarshal(request.Body, &update); err != nil {
			result = jsonError(http.StatusBadRequest, "invalid settings")
		} else if err := h.plugin.SaveSettings(ctx, update); err != nil {
			result = jsonError(http.StatusInternalServerError, "could not save settings")
		} else {
			result = h.plugin.settingsResponse()
		}
	case request.Method == http.MethodPost && request.Path == "/v0/management/plugins/typesafe/test":
		if err := h.plugin.UpstreamTest(ctx); err != nil {
			result = jsonError(http.StatusBadGateway, "upstream connection test failed")
		} else {
			result = jsonResponseJSON(http.StatusOK, map[string]any{"ok": true})
		}
	default:
		return pluginapi.ManagementResponse{StatusCode: http.StatusNotFound}, nil
	}
	return pluginapi.ManagementResponse{StatusCode: result.StatusCode, Headers: result.Headers, Body: result.Body}, nil
}

func (p *Plugin) RegisterManagement(context.Context, pluginapi.ManagementRegistrationRequest) (pluginapi.ManagementRegistrationResponse, error) {
	h := managementHandler{plugin: p}
	return pluginapi.ManagementRegistrationResponse{
		Routes:    []pluginapi.ManagementRoute{{Method: http.MethodGet, Path: "plugins/typesafe/settings", Handler: h}, {Method: http.MethodPut, Path: "plugins/typesafe/settings", Handler: h}, {Method: http.MethodPost, Path: "plugins/typesafe/test", Handler: h}},
		Resources: []pluginapi.ResourceRoute{{Path: "settings", Menu: "TypeSafe", Description: "TypeSafe provider settings", Handler: h}},
	}, nil
}

func (p *Plugin) settingsResponse() Result {
	p.mu.RLock()
	model := p.model
	configured := p.key != ""
	p.mu.RUnlock()
	return jsonResponseJSON(http.StatusOK, map[string]any{"model": model, "configured": configured})
}

func (p *Plugin) RegisterNative(context.Context, NativeRegistrationRequest) (NativeRegistrationResponse, error) {
	return NativeRegistrationResponse{Routes: []NativeRoute{{Method: http.MethodPost, Path: "/v1/systemone", Handler: p}}}, nil
}
func (p *Plugin) HandleNative(ctx context.Context, req NativeRequest) (NativeResponse, error) {
	if req.Method != http.MethodPost || req.Path != "/v1/systemone" {
		return NativeResponse{StatusCode: http.StatusNotFound}, nil
	}
	result := p.NativeForward(ctx, req.Body)
	return NativeResponse{StatusCode: result.StatusCode, Headers: result.Headers, Body: result.Body}, nil
}
func (p *Plugin) Management(ctx context.Context, method, path string, body []byte) Result {
	response, err := (managementHandler{plugin: p}).HandleManagement(ctx, pluginapi.ManagementRequest{Method: method, Path: path, Body: body})
	if err != nil {
		return jsonError(http.StatusInternalServerError, "request failed")
	}
	return Result{StatusCode: response.StatusCode, Headers: response.Headers, Body: response.Body}
}
func (p *Plugin) Resource(method, path string) Result {
	response, err := (managementHandler{plugin: p}).HandleManagement(context.Background(), pluginapi.ManagementRequest{Method: method, Path: path})
	if err != nil {
		return jsonError(http.StatusInternalServerError, "request failed")
	}
	return Result{StatusCode: response.StatusCode, Headers: response.Headers, Body: response.Body}
}
func (p *Plugin) ResourceHTML() string { return resourceHTML }
func jsonError(status int, message string) Result {
	return jsonResponseJSON(status, map[string]string{"error": message})
}
func jsonResponseJSON(status int, value any) Result {
	data, err := json.Marshal(value)
	if err != nil {
		return Result{StatusCode: http.StatusInternalServerError, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"error":"internal error"}`)}
	}
	return Result{StatusCode: status, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: data}
}

var _ pluginapi.ManagementAPI = (*Plugin)(nil)
