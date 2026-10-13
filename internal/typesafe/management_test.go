package typesafe

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestSecretFreeManagementAndResource(t *testing.T) {
	secret := "fake-key-management-redaction"
	p, err := NewWithStore(Config{APIKey: secret, CredentialsFile: privateTestDir(t) + "/credential.yaml"}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	response := p.Management(context.Background(), http.MethodGet, "/v0/management/plugins/typesafe/settings", nil)
	if strings.Contains(string(response.Body), secret) {
		t.Fatal("settings response leaked credential")
	}
	resource := p.ResourceHTML()
	if strings.Contains(resource, secret) || strings.Contains(resource, "TYPESAFE_API_KEY") {
		t.Fatal("public resource leaked credential source")
	}
	served, err := (managementHandler{plugin: p}).HandleManagement(context.Background(), pluginapi.ManagementRequest{Method: http.MethodGet, Path: "/v0/resource/plugins/typesafe/settings"})
	if err != nil || served.StatusCode != http.StatusOK || served.Headers.Get("Content-Type") != "text/html; charset=utf-8" || strings.Contains(string(served.Body), secret) {
		t.Fatalf("resource response=%#v err=%v", served, err)
	}
}

func TestManagementRejectsWrongMethodsAndPaths(t *testing.T) {
	p := newTestPlugin(t, Config{}, http.DefaultClient)
	for _, request := range []pluginapi.ManagementRequest{{Method: http.MethodPost, Path: "/v0/management/plugins/typesafe/settings"}, {Method: http.MethodGet, Path: "/v0/management/plugins/typesafe/test"}, {Method: http.MethodGet, Path: "/settings"}} {
		got, err := (managementHandler{plugin: p}).HandleManagement(context.Background(), request)
		if err != nil || got.StatusCode != http.StatusNotFound {
			t.Errorf("request=%+v response=%+v err=%v", request, got, err)
		}
	}
}

func TestManagementModelOnlyUpdatePreservesKey(t *testing.T) {
	path := t.TempDir() + "/private/credentials.yaml"
	p, err := NewWithStore(Config{Model: "jev-old", APIKey: "retained-key", CredentialsFile: path}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (managementHandler{plugin: p}).HandleManagement(context.Background(), pluginapi.ManagementRequest{Method: http.MethodPut, Path: "/v0/management/plugins/typesafe/settings", Body: []byte(`{"model":"jev-new"}`)})
	if err != nil || got.StatusCode != http.StatusOK {
		t.Fatalf("response=%+v err=%v", got, err)
	}
	if p.APIKey() != "retained-key" || p.Model() != "jev-new" {
		t.Fatal("model-only update changed saved key or missed model")
	}
}
