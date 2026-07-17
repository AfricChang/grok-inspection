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
	pluginVersion         = "0.2.4"
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
	UsagePlugin   bool `json:"usage_plugin"`
}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return okEnvelope(pluginRegistration())
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRegistration())
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
	case pluginabi.MethodUsageHandle:
		return handleUsage(request)
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
			Author:           "ywddd",
			GitHubRepository: "https://github.com/ywddd/grok-inspection",
			ConfigFields:     []pluginapi.ConfigField{},
		},
		Capabilities: registrationCapabilities{ManagementAPI: true, UsagePlugin: true},
	}
}

func managementRegistration() pluginapi.ManagementRegistrationResponse {
	return pluginapi.ManagementRegistrationResponse{
		Routes: []pluginapi.ManagementRoute{
			{Method: http.MethodGet, Path: managementRoutePrefix + "/status", Description: "Get Grok inspection status."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/start", Description: "Start a full, incremental, or classify-scoped Grok inspection job."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/stop", Description: "Stop the current Grok inspection job."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/apply", Description: "Apply recommended disable/enable/delete actions asynchronously."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/action", Description: "Disable, enable, or delete one Grok credential asynchronously."},
			{Method: http.MethodGet, Path: managementRoutePrefix + "/automation", Description: "Get automatic inspection rules and runtime status."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/automation/rules", Description: "Create or update an automatic inspection rule."},
			{Method: http.MethodDelete, Path: managementRoutePrefix + "/automation/rules", Description: "Delete an automatic inspection rule."},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/automation/run", Description: "Run one automatic inspection rule immediately."},
			{Method: http.MethodGet, Path: managementRoutePrefix + "/automation/history", Description: "Get automatic inspection history."},
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
	// Management routes are already authenticated by CPA. Cache the key in memory
	// so asynchronous Usage callbacks can perform enable/disable without requiring
	// a process restart solely to add MANAGEMENT_PASSWORD.
	rememberManagementPassword(req.Headers)
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	switch {
	case method == http.MethodGet && matchesResourcePath(req.Path, "/status"):
		return htmlResponse(http.StatusOK, renderUIPage(pluginName))
	case method == http.MethodGet && matchesManagementPath(req.Path, "/status"):
		// Pure memory snapshot — never blocks on host or management HTTP.
		// light=1 / include_results=0: progress meta only (cheap poll during inspect/apply).
		return jsonResponse(http.StatusOK, engine.snapshot(statusWantsResults(req)))
	case method == http.MethodPost && matchesManagementPath(req.Path, "/start"):
		var body startRequest
		if len(req.Body) > 0 {
			_ = json.Unmarshal(req.Body, &body)
		}
		if err := engine.start(body); err != nil {
			status := http.StatusConflict
			msg := err.Error()
			if strings.Contains(msg, "workers must") || strings.Contains(msg, "增量巡检") || strings.Contains(msg, "分类巡检") || strings.Contains(msg, "当前分类") || strings.Contains(msg, "busy") {
				status = http.StatusBadRequest
				if strings.Contains(msg, "busy") || strings.Contains(msg, "already running") {
					status = http.StatusConflict
				}
			}
			return jsonResponse(status, map[string]any{"error": msg})
		}
		return jsonResponse(http.StatusOK, engine.snapshot(true))
	case method == http.MethodPost && matchesManagementPath(req.Path, "/stop"):
		engine.stop()
		return jsonResponse(http.StatusOK, engine.snapshot(false))
	case method == http.MethodPost && matchesManagementPath(req.Path, "/apply"):
		var body applyRequest
		if len(req.Body) > 0 {
			_ = json.Unmarshal(req.Body, &body)
		}
		// Async: returns immediately so status/action stay responsive and delete
		// can call management HTTP without re-entering the same request lock.
		// Capture page Management Key for background delete/auth API calls.
		password := resolveManagementPassword(req.Headers)
		if err := engine.startApply(body, password, req.Headers); err != nil {
			status := http.StatusConflict
			msg := err.Error()
			if strings.Contains(msg, "force_action") || strings.Contains(msg, "requires") || strings.Contains(msg, "no accounts") {
				status = http.StatusBadRequest
			}
			return jsonResponse(status, map[string]any{"error": msg})
		}
		// Slim ack — full account list is only on GET /status (include_results=1).
		snap := engine.snapshot(false)
		return jsonResponse(http.StatusAccepted, map[string]any{
			"ok":          true,
			"accepted":    true,
			"applying":    snap.Applying,
			"apply_total": snap.ApplyTotal,
			"apply_done":  snap.ApplyDone,
		})
	case method == http.MethodPost && matchesManagementPath(req.Path, "/action"):
		var body actionRequest
		if len(req.Body) > 0 {
			if err := json.Unmarshal(req.Body, &body); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
			}
		}
		password := resolveManagementPassword(req.Headers)
		seq, action, err := engine.startAction(body, password, req.Headers)
		if err != nil {
			status := http.StatusConflict
			if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "busy") {
				status = http.StatusBadRequest
				if strings.Contains(err.Error(), "busy") {
					status = http.StatusConflict
				}
			}
			return jsonResponse(status, map[string]any{"error": err.Error(), "ok": false})
		}
		// 202 = accepted only. Clients must poll light /status for recent_row_actions[seq].
		return jsonResponse(http.StatusAccepted, map[string]any{
			"ok":         true,
			"accepted":   true,
			"action":     action,
			"action_seq": seq,
			"name":       firstNonEmpty(body.Name, body.AuthIndex),
		})
	case method == http.MethodGet && matchesManagementPath(req.Path, "/automation"):
		return jsonResponse(http.StatusOK, automation.snapshot(false))
	case method == http.MethodGet && matchesManagementPath(req.Path, "/automation/history"):
		return jsonResponse(http.StatusOK, automation.snapshot(true))
	case method == http.MethodPost && matchesManagementPath(req.Path, "/automation/rules"):
		var rule automationRule
		if len(req.Body) > 0 {
			if err := json.Unmarshal(req.Body, &rule); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
			}
		}
		saved, err := automation.upsertRule(rule)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, saved)
	case method == http.MethodDelete && matchesManagementPath(req.Path, "/automation/rules"):
		id := ""
		if req.Query != nil {
			id = req.Query.Get("id")
		}
		if id == "" && len(req.Body) > 0 {
			var body struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(req.Body, &body)
			id = body.ID
		}
		if err := automation.deleteRule(id); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true})
	case method == http.MethodPost && matchesManagementPath(req.Path, "/automation/run"):
		var body struct {
			ID string `json:"id"`
		}
		if len(req.Body) > 0 {
			_ = json.Unmarshal(req.Body, &body)
		}
		if err := automation.runRule(body.ID, true); err != nil {
			return jsonResponse(http.StatusConflict, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusAccepted, map[string]any{"ok": true, "accepted": true})
	default:
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "not found", "path": req.Path, "method": method})
	}
}

// statusWantsResults defaults to full results; light polls pass include_results=0 or light=1.
func statusWantsResults(req pluginapi.ManagementRequest) bool {
	if req.Query == nil {
		return true
	}
	if v := strings.TrimSpace(req.Query.Get("include_results")); v != "" {
		return !(v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "no"))
	}
	if v := strings.TrimSpace(req.Query.Get("light")); v != "" {
		return !(v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes"))
	}
	return true
}

func matchesManagementPath(path, suffix string) bool {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	// Strip query if a gateway put it on Path.
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
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
