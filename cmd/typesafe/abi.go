package main

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
typedef struct { void* ptr; size_t len; } cliproxy_buffer;
typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);
typedef struct { uint32_t abi_version; void* host_ctx; cliproxy_host_call_fn call; cliproxy_host_free_fn free_buffer; } cliproxy_host_api;
typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);
typedef struct { uint32_t abi_version; cliproxy_plugin_call_fn call; cliproxy_plugin_free_fn free_buffer; cliproxy_plugin_shutdown_fn shutdown; } cliproxy_plugin_api;
extern int TypeSafePluginCall(char*,uint8_t*,size_t,cliproxy_buffer*);
extern void TypeSafePluginFree(void*,size_t);
extern void TypeSafePluginShutdown(void);
static int typesafe_host_call(cliproxy_host_api* api,const char* method,const uint8_t* req,size_t len,cliproxy_buffer* out){return api->call(api->host_ctx,method,req,len,out);}
static void typesafe_host_free(cliproxy_host_api* api,void* ptr,size_t len){api->free_buffer(ptr,len);}
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"unsafe"

	"github.com/EcoRayDev/cpa-plugin-typesafe/internal/typesafe"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const (
	methodNativeRegister          = "native.register"
	methodNativeHandle            = "native.handle"
	schemaVersionNativeAPI uint32 = 7
)

var pluginVersion = "dev"
var errHostSchemaTooOld = errors.New("TypeSafe plugin requires CLIProxyAPI schema 7")
var state = struct {
	sync.RWMutex
	host   *C.cliproxy_host_api
	plugin *typesafe.Plugin
	schema uint32
}{}

type lifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
}
type registration struct {
	SchemaVersion uint32             `json:"schema_version"`
	Metadata      pluginapi.Metadata `json:"metadata"`
	Capabilities  capabilities       `json:"capabilities"`
}
type capabilities struct {
	ModelRegistrar        bool                         `json:"model_registrar"`
	ModelRouter           bool                         `json:"model_router"`
	Executor              bool                         `json:"executor"`
	ExecutorModelScope    pluginapi.ExecutorModelScope `json:"executor_model_scope"`
	ExecutorInputFormats  []string                     `json:"executor_input_formats,omitempty"`
	ExecutorOutputFormats []string                     `json:"executor_output_formats,omitempty"`
	ManagementAPI         bool                         `json:"management_api"`
	NativeAPI             bool                         `json:"native_api"`
}
type identifierResponse struct {
	Identifier string `json:"identifier"`
}
type nativeRegistration struct {
	Routes []nativeRoute `json:"routes"`
}
type nativeRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}
type managementRegistration struct {
	Routes    []managementRoute `json:"routes"`
	Resources []resourceRoute   `json:"resources"`
}
type managementRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}
type resourceRoute struct {
	Path        string `json:"path"`
	Menu        string `json:"menu"`
	Description string `json:"description"`
}
type nativeRequest struct {
	Method         string
	Path           string
	Headers        http.Header
	Query          url.Values
	Body           []byte
	HostCallbackID string `json:"host_callback_id,omitempty"`
}
type managementRequest struct {
	pluginapi.ManagementRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}
type managementResponse struct {
	StatusCode    int
	Headers       http.Header
	Body          []byte
	RefreshModels bool `json:"refresh_models,omitempty"`
}
type executorRequest struct {
	pluginapi.ExecutorRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}
type hostHTTPRequest struct {
	Method              string      `json:"method"`
	URL                 string      `json:"url"`
	Headers             http.Header `json:"headers,omitempty"`
	Body                []byte      `json:"body,omitempty"`
	DisableRedirects    bool        `json:"disable_redirects,omitempty"`
	MaxResponseBytes    int64       `json:"max_response_bytes,omitempty"`
	TimeoutMilliseconds int64       `json:"timeout_ms,omitempty"`
	SuppressLogging     bool        `json:"suppress_logging,omitempty"`
	HostCallbackID      string      `json:"host_callback_id,omitempty"`
	OperationID         string      `json:"operation_id,omitempty"`
}
type operationOpenRequest struct {
	HostCallbackID string `json:"host_callback_id,omitempty"`
}
type operationOpenResponse struct {
	OperationID string `json:"operation_id"`
}
type streamResponse struct {
	Headers http.Header                     `json:"headers,omitempty"`
	Chunks  []pluginapi.ExecutorStreamChunk `json:"chunks,omitempty"`
}
type hostHTTPClient struct{ callbackID string }

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if host == nil || plugin == nil || host.abi_version != C.uint32_t(pluginabi.ABIVersion) || host.call == nil || host.free_buffer == nil {
		return 1
	}
	state.Lock()
	state.host = host
	state.Unlock()
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.TypeSafePluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.TypeSafePluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.TypeSafePluginShutdown)
	return 0
}

//export TypeSafePluginCall
func TypeSafePluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		return writeError(response, "invalid_method", "method is required")
	}
	var data []byte
	if request != nil && requestLen > 0 {
		data = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	out, err := dispatch(C.GoString(method), data)
	if err != nil {
		return writeError(response, "plugin_error", err.Error())
	}
	writeResponse(response, out)
	return 0
}

//export TypeSafePluginFree
func TypeSafePluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export TypeSafePluginShutdown
func TypeSafePluginShutdown() { state.Lock(); state.plugin = nil; state.host = nil; state.Unlock() }

func dispatch(method string, raw []byte) ([]byte, error) {
	if method == pluginabi.MethodPluginRegister || method == pluginabi.MethodPluginReconfigure {
		return register(raw)
	}
	p, err := current()
	if err != nil {
		return nil, err
	}
	switch method {
	case pluginabi.MethodExecutorIdentifier:
		return marshal(identifierResponse{typesafe.ProviderID})
	case pluginabi.MethodModelRegister:
		var req pluginapi.ModelRegistrationRequest
		if err = json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		res, e := p.RegisterModels(context.Background(), req)
		return envelope(res, e)
	case pluginabi.MethodModelRoute:
		var req pluginapi.ModelRouteRequest
		if err = json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		res, e := p.RouteModel(context.Background(), req)
		return envelope(res, e)
	case pluginabi.MethodExecutorExecute:
		var req executorRequest
		if err = json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		ctx := typesafe.WithHostHTTPClient(context.Background(), hostHTTPClient{callbackID: req.HostCallbackID})
		res, e := p.Execute(ctx, req.ExecutorRequest)
		return envelope(res, e)
	case pluginabi.MethodExecutorExecuteStream:
		var req executorRequest
		if err = json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		ctx := typesafe.WithHostHTTPClient(context.Background(), hostHTTPClient{callbackID: req.HostCallbackID})
		res, e := p.ExecuteStream(ctx, req.ExecutorRequest)
		if e != nil {
			return envelope(res, e)
		}
		return envelope(streamResponse{Headers: res.Headers, Chunks: collectChunks(res.Chunks)}, nil)
	case pluginabi.MethodExecutorCountTokens:
		var req executorRequest
		if err = json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		res, e := p.CountTokens(context.Background(), req.ExecutorRequest)
		return envelope(res, e)
	case pluginabi.MethodManagementRegister:
		var req pluginapi.ManagementRegistrationRequest
		if err = json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return envelope(managementRegistration{Routes: []managementRoute{{Method: http.MethodGet, Path: "plugins/typesafe/settings"}, {Method: http.MethodPut, Path: "plugins/typesafe/settings"}, {Method: http.MethodPost, Path: "plugins/typesafe/test"}}, Resources: []resourceRoute{{Path: "settings", Menu: "TypeSafe", Description: "TypeSafe provider settings"}}}, nil)
	case pluginabi.MethodManagementHandle:
		var req managementRequest
		if err = json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		previousModel := p.Model()
		ctx := typesafe.WithHostHTTPClient(context.Background(), hostHTTPClient{callbackID: req.HostCallbackID})
		res, e := (pluginManagementHandler{plugin: p}).HandleManagement(ctx, req.ManagementRequest)
		refresh := e == nil && req.Method == http.MethodPut &&
			req.Path == "/v0/management/plugins/typesafe/settings" &&
			res.StatusCode >= 200 && res.StatusCode < 300 && p.Model() != previousModel
		return envelope(managementResponse{StatusCode: res.StatusCode, Headers: res.Headers, Body: res.Body, RefreshModels: refresh}, e)
	case methodNativeRegister:
		state.RLock()
		schema := state.schema
		state.RUnlock()
		if schema < schemaVersionNativeAPI {
			return nil, errHostSchemaTooOld
		}
		return envelope(nativeRegistration{Routes: []nativeRoute{{Method: http.MethodPost, Path: "/v1/systemone"}}}, nil)
	case methodNativeHandle:
		var req nativeRequest
		if err = json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		ctx := typesafe.WithHostHTTPClient(context.Background(), hostHTTPClient{callbackID: req.HostCallbackID})
		res, e := p.HandleNative(ctx, typesafe.NativeRequest{Method: req.Method, Path: req.Path, Headers: req.Headers, Query: req.Query, Body: req.Body})
		return envelope(res, e)
	default:
		return marshal(pluginabi.NewError("unknown_method", "unknown method: "+method))
	}
}
func collectChunks(ch <-chan pluginapi.ExecutorStreamChunk) []pluginapi.ExecutorStreamChunk {
	out := make([]pluginapi.ExecutorStreamChunk, 0, 1)
	for chunk := range ch {
		out = append(out, chunk)
	}
	return out
}
func register(raw []byte) ([]byte, error) {
	var req lifecycleRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	if req.SchemaVersion < schemaVersionNativeAPI {
		return nil, errHostSchemaTooOld
	}
	config, err := typesafe.ParseConfig(req.ConfigYAML)
	if err != nil {
		return nil, err
	}
	p, err := typesafe.NewWithStore(config, nil)
	if err != nil {
		return nil, err
	}
	state.Lock()
	state.plugin = p
	state.schema = req.SchemaVersion
	state.Unlock()
	value := registration{SchemaVersion: schemaVersionNativeAPI, Metadata: pluginapi.Metadata{Name: "TypeSafe", Version: pluginVersion, Author: "EcoRayDev", GitHubRepository: "https://github.com/EcoRayDev/cpa-plugin-typesafe", ConfigFields: []pluginapi.ConfigField{{Name: "model", Type: pluginapi.ConfigFieldTypeString, Description: "Jev model identifier"}, {Name: "credentials_file", Type: pluginapi.ConfigFieldTypeString, Description: "Private credential file path"}}}, Capabilities: capabilities{ModelRegistrar: true, ModelRouter: true, Executor: true, ExecutorModelScope: pluginapi.ExecutorModelScopeStatic, ExecutorInputFormats: []string{"openai"}, ExecutorOutputFormats: []string{"openai"}, ManagementAPI: true, NativeAPI: true}}
	return marshal(value)
}
func current() (*typesafe.Plugin, error) {
	state.RLock()
	defer state.RUnlock()
	if state.plugin == nil {
		return nil, errors.New("TypeSafe plugin is not registered")
	}
	return state.plugin, nil
}
func (c hostHTTPClient) Do(ctx context.Context, req typesafe.HostHTTPRequest) (pluginapi.HTTPResponse, error) {
	openRaw, err := json.Marshal(operationOpenRequest{HostCallbackID: c.callbackID})
	if err != nil {
		return pluginapi.HTTPResponse{}, err
	}
	opened, err := callHost(pluginabi.MethodHostHTTPOperationOpen, openRaw)
	if err != nil {
		return pluginapi.HTTPResponse{}, err
	}
	var operation operationOpenResponse
	if err = json.Unmarshal(opened, &operation); err != nil {
		return pluginapi.HTTPResponse{}, err
	}
	raw, err := json.Marshal(hostHTTPRequest{Method: req.Method, URL: req.URL, Headers: req.Headers, Body: req.Body, DisableRedirects: true, MaxResponseBytes: 4 << 20, TimeoutMilliseconds: 120000, SuppressLogging: true, HostCallbackID: c.callbackID, OperationID: operation.OperationID})
	if err != nil {
		return pluginapi.HTTPResponse{}, err
	}
	result, err := callHost(pluginabi.MethodHostHTTPDo, raw)
	if err != nil {
		cancelOperation(operation.OperationID)
		return pluginapi.HTTPResponse{}, err
	}
	var response pluginapi.HTTPResponse
	if err = json.Unmarshal(result, &response); err != nil {
		cancelOperation(operation.OperationID)
		return pluginapi.HTTPResponse{}, err
	}
	cancelOperation(operation.OperationID)
	return response, nil
}
func cancelOperation(id string) {
	raw, _ := json.Marshal(map[string]string{"operation_id": id})
	_, _ = callHost(pluginabi.MethodHostHTTPCancel, raw)
}
func callHost(method string, request []byte) ([]byte, error) {
	state.RLock()
	host := state.host
	state.RUnlock()
	if host == nil {
		return nil, errors.New("plugin host unavailable")
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var ptr *C.uint8_t
	if len(request) > 0 {
		ptr = (*C.uint8_t)(C.CBytes(request))
		defer C.free(unsafe.Pointer(ptr))
	}
	var response C.cliproxy_buffer
	if C.typesafe_host_call(host, cMethod, ptr, C.size_t(len(request)), &response) != 0 {
		return nil, errors.New("host callback failed")
	}
	defer C.typesafe_host_free(host, response.ptr, response.len)
	raw := C.GoBytes(response.ptr, C.int(response.len))
	var env pluginabi.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if !env.OK {
		if env.Error != nil {
			return nil, env.Error
		}
		return nil, errors.New("host callback failed")
	}
	return env.Result, nil
}
func envelope(value any, err error) ([]byte, error) {
	if err != nil {
		status := 0
		var statusErr interface{ StatusCode() int }
		if errors.As(err, &statusErr) {
			status = statusErr.StatusCode()
		}
		apiErr := pluginabi.NewError("plugin_error", err.Error(), status)
		raw, _ := json.Marshal(pluginabi.Envelope{OK: false, Error: apiErr})
		return raw, nil
	}
	return marshal(value)
}
func marshal(value any) ([]byte, error) {
	result, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(pluginabi.Envelope{OK: true, Result: result})
}
func writeError(out *C.cliproxy_buffer, code, message string) C.int {
	raw, _ := json.Marshal(pluginabi.Envelope{OK: false, Error: pluginabi.NewError(code, message)})
	writeResponse(out, raw)
	return 1
}
func writeResponse(out *C.cliproxy_buffer, raw []byte) {
	if out == nil {
		return
	}
	out.ptr = nil
	out.len = 0
	if len(raw) == 0 {
		return
	}
	ptr := C.malloc(C.size_t(len(raw)))
	if ptr == nil {
		return
	}
	C.memcpy(ptr, unsafe.Pointer(&raw[0]), C.size_t(len(raw)))
	out.ptr = ptr
	out.len = C.size_t(len(raw))
}
