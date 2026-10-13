package typesafe

import (
	"context"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func (p *Plugin) RouteModel(_ context.Context, request pluginapi.ModelRouteRequest) (pluginapi.ModelRouteResponse, error) {
	p.mu.RLock()
	model := p.model
	p.mu.RUnlock()
	if request.RequestedModel != DefaultModel && request.RequestedModel != model {
		return pluginapi.ModelRouteResponse{}, nil
	}
	return pluginapi.ModelRouteResponse{Handled: true, TargetKind: pluginapi.ModelRouteTargetSelf}, nil
}

var _ pluginapi.ModelRouter = (*Plugin)(nil)
