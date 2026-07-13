package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"grok-inspection/cpasdk/pluginabi"
	"grok-inspection/cpasdk/pluginapi"
)

const (
	pluginName            = "grok-inspection"
	pluginVersion         = "0.1.3"
	resourceContentType   = "text/html; charset=utf-8"
	jsonContentType       = "application/json; charset=utf-8"
	managementRoutePrefix = "/plugins/" + pluginName
)

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginapi.Metadata       `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type registrationCapabilities struct {
	ManagementAPI bool `json:"management_api"`
}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return okEnvelope(pluginRegistration())
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRegistration())
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           "local",
			GitHubRepository: "https://github.com/router-for-me/CLIProxyAPI",
			ConfigFields:     []pluginapi.ConfigField{},
		},
		Capabilities: registrationCapabilities{ManagementAPI: true},
	}
}

func managementRegistration() pluginapi.ManagementRegistrationResponse {
	return pluginapi.ManagementRegistrationResponse{
		Routes: []pluginapi.ManagementRoute{
			{Method: http.MethodGet, Path: managementRoutePrefix + "/status", Description: "Get Grok inspection status."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/start", Description: "Start a Grok inspection job."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/stop", Description: "Stop the current Grok inspection job."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/apply", Description: "Apply recommended disable/enable actions."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/action", Description: "Disable or enable one Grok credential."},
		},
		Resources: []pluginapi.ResourceRoute{
			{
				Path:        "/status",
				Menu:        "Grok 账号巡检",
				Description: "服务端巡检 xAI/Grok 账号健康、权限与额度。",
			},
		},
	}
}

func handleManagement(raw []byte) ([]byte, error) {
	var req pluginapi.ManagementRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("decode management request: %w", err)
		}
	}
	return okEnvelope(dispatchManagement(req))
}

func dispatchManagement(req pluginapi.ManagementRequest) pluginapi.ManagementResponse {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	switch {
	case method == http.MethodGet && matchesResourcePath(req.Path, "/status"):
		return htmlResponse(http.StatusOK, renderUIPage(pluginName))
	case method == http.MethodGet && matchesManagementPath(req.Path, "/status"):
		return jsonResponse(http.StatusOK, engine.snapshot())
	case method == http.MethodPost && matchesManagementPath(req.Path, "/start"):
		var body startRequest
		if len(req.Body) > 0 {
			_ = json.Unmarshal(req.Body, &body)
		}
		if err := engine.start(body); err != nil {
			return jsonResponse(http.StatusConflict, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, engine.snapshot())
	case method == http.MethodPost && matchesManagementPath(req.Path, "/stop"):
		engine.stop()
		return jsonResponse(http.StatusOK, engine.snapshot())
	case method == http.MethodPost && matchesManagementPath(req.Path, "/apply"):
		var body applyRequest
		if len(req.Body) > 0 {
			_ = json.Unmarshal(req.Body, &body)
		}
		password := resolveManagementPassword(req.Headers)
		result, errApply := engine.applyRecommendations(body.AuthIndexes, password, req.Headers)
		if errApply != nil {
			return jsonResponse(http.StatusConflict, map[string]any{"error": errApply.Error()})
		}
		return jsonResponse(http.StatusOK, result)
	case method == http.MethodPost && matchesManagementPath(req.Path, "/action"):
		var body actionRequest
		if len(req.Body) > 0 {
			if err := json.Unmarshal(req.Body, &body); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
			}
		}
		name := firstNonEmpty(body.Name, body.AuthIndex)
		if name == "" {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "name or auth_index required"})
		}
		password := resolveManagementPassword(req.Headers)
		var errAction error
		if body.Delete {
			errAction = deleteAuthFile(name, password, req.Headers)
		} else {
			errAction = setAuthDisabled(name, body.Disabled, password, req.Headers)
		}
		if errAction != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": errAction.Error()})
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true})
	default:
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "not found", "path": req.Path, "method": method})
	}
}

func matchesManagementPath(path, suffix string) bool {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	return path == managementRoutePrefix+suffix ||
		path == "/v0/management"+managementRoutePrefix+suffix
}

func matchesResourcePath(path, suffix string) bool {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	return path == "/v0/resource/plugins/"+pluginName+suffix
}

func htmlResponse(statusCode int, body []byte) pluginapi.ManagementResponse {
	return pluginapi.ManagementResponse{
		StatusCode: statusCode,
		Headers:    http.Header{"Content-Type": []string{resourceContentType}},
		Body:       body,
	}
}

func jsonResponse(statusCode int, payload any) pluginapi.ManagementResponse {
	raw, _ := json.Marshal(payload)
	return pluginapi.ManagementResponse{
		StatusCode: statusCode,
		Headers:    http.Header{"Content-Type": []string{jsonContentType}},
		Body:       raw,
	}
}
