package typesafe

import (
	"context"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestJevRoutesWithoutOAuthAndDoesNotCaptureOtherProviders(t *testing.T) {
	plugin := newTestPlugin(t, Config{Model: "jev-pinned", APIKey: "fixture-key"}, http.DefaultClient)
	for _, model := range []string{"jev-latest", "jev-pinned"} {
		route, err := plugin.RouteModel(context.Background(), pluginapi.ModelRouteRequest{RequestedModel: model})
		if err != nil || !route.Handled || route.TargetKind != pluginapi.ModelRouteTargetSelf {
			t.Fatalf("standalone Jev model %q did not route to its own executor: %+v %v", model, route, err)
		}
	}
	for _, model := range []string{"unrelated-model", "jev-unconfigured"} {
		route, err := plugin.RouteModel(context.Background(), pluginapi.ModelRouteRequest{RequestedModel: model})
		if err != nil || route.Handled {
			t.Fatalf("plugin captured unrelated model %q: %+v %v", model, route, err)
		}
	}
}
