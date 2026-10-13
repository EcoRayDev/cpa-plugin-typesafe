package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func registerContractPlugin(t *testing.T) registration {
	t.Helper()
	t.Setenv("TYPESAFE_API_KEY", "")
	path := filepath.Join(t.TempDir(), "private", "credentials.yaml")
	config := []byte("model: jev-ship-pin\ncredentials_file: " + path + "\n")
	request, err := json.Marshal(lifecycleRequest{SchemaVersion: 7, ConfigYAML: config})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := register(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Lock(); state.plugin = nil; state.Unlock() })
	var response struct {
		OK     bool         `json:"ok"`
		Result registration `json:"result"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || !response.OK {
		t.Fatalf("registration failed: %v", err)
	}
	return response.Result
}

func TestSchemaSevenAdvertisesAndRoutesStandaloneJev(t *testing.T) {
	registered := registerContractPlugin(t)
	// Without these advertised capabilities the host either cannot dispatch the
	// native endpoint or sends chat through OAuth selection (auth_not_found).
	if !registered.Capabilities.NativeAPI || !registered.Capabilities.ModelRouter {
		t.Fatal("host cannot select the native and standalone chat execution paths")
	}
	for _, model := range []string{"jev-latest", "jev-ship-pin", "unrelated-provider-model"} {
		request, _ := json.Marshal(pluginapi.ModelRouteRequest{RequestedModel: model})
		raw, err := dispatch(pluginabi.MethodModelRoute, request)
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			OK     bool                         `json:"ok"`
			Result pluginapi.ModelRouteResponse `json:"result"`
		}
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatal(err)
		}
		wantHandled := model != "unrelated-provider-model"
		if !response.OK || response.Result.Handled != wantHandled || (wantHandled && response.Result.TargetKind != pluginapi.ModelRouteTargetSelf) {
			t.Fatalf("model %q route=%+v", model, response.Result)
		}
	}
}

func TestManagementRoutesAndRegistration(t *testing.T) {
	registerContractPlugin(t)
	raw, err := dispatch(pluginabi.MethodManagementRegister, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	// This is the host's consumer type. A serialized non-nil Handler interface
	// cannot be decoded into it and prevents every management route from loading.
	var registration struct {
		OK     bool                                     `json:"ok"`
		Result pluginapi.ManagementRegistrationResponse `json:"result"`
	}
	if err := json.Unmarshal(raw, &registration); err != nil || !registration.OK {
		t.Fatalf("host could not consume management registration: %v", err)
	}
	var settingsPath, resourcePath string
	for _, route := range registration.Result.Routes {
		if route.Method == http.MethodPut {
			settingsPath = "/v0/management/" + route.Path
		}
	}
	for _, resource := range registration.Result.Resources {
		resourcePath = "/v0/resource/plugins/typesafe/" + resource.Path
	}
	request, _ := json.Marshal(managementRequest{ManagementRequest: pluginapi.ManagementRequest{Method: http.MethodPut, Path: settingsPath, Body: []byte(`{"model":"jev-hot-pin","api_key":"fixture-never-real"}`)}})
	raw, err = dispatch(pluginabi.MethodManagementHandle, request)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		OK     bool                         `json:"ok"`
		Result pluginapi.ManagementResponse `json:"result"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.OK || saved.Result.StatusCode != http.StatusOK {
		t.Fatalf("save did not commit: %+v", saved.Result)
	}
	var settings struct {
		Model      string `json:"model"`
		Configured bool   `json:"configured"`
		APIKey     string `json:"api_key"`
	}
	if err := json.Unmarshal(saved.Result.Body, &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Model != "jev-hot-pin" || !settings.Configured || settings.APIKey != "" {
		t.Fatalf("saved public settings=%+v", settings)
	}
	request, _ = json.Marshal(managementRequest{ManagementRequest: pluginapi.ManagementRequest{Method: http.MethodGet, Path: resourcePath}})
	raw, err = dispatch(pluginabi.MethodManagementHandle, request)
	if err != nil {
		t.Fatal(err)
	}
	var resource struct {
		OK     bool                         `json:"ok"`
		Result pluginapi.ManagementResponse `json:"result"`
	}
	if err := json.Unmarshal(raw, &resource); err != nil {
		t.Fatal(err)
	}
	if !resource.OK || resource.Result.StatusCode != http.StatusOK || resource.Result.Headers.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("public resource response=%+v", resource.Result)
	}
}
