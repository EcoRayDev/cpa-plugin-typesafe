package main

import (
	"context"
	"strings"

	"github.com/EcoRayDev/cpa-plugin-typesafe/internal/typesafe"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type pluginManagementHandler struct{ plugin *typesafe.Plugin }

func (h pluginManagementHandler) HandleManagement(ctx context.Context, request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	if strings.HasPrefix(request.Path, "/v0/resource/") {
		response := h.plugin.Resource(request.Method, request.Path)
		return pluginapi.ManagementResponse{StatusCode: response.StatusCode, Headers: response.Headers, Body: response.Body}, nil
	}
	result := h.plugin.Management(ctx, request.Method, request.Path, request.Body)
	return pluginapi.ManagementResponse{StatusCode: result.StatusCode, Headers: result.Headers, Body: result.Body}, nil
}

var _ pluginapi.ManagementHandler = pluginManagementHandler{}
