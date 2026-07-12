package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"grok-inspection/cpasdk/pluginabi"
	"grok-inspection/cpasdk/pluginapi"
)

type accountResult struct {
	AuthIndex      string `json:"auth_index"`
	Name           string `json:"name"`
	FileName       string `json:"file_name,omitempty"`
	Email          string `json:"email,omitempty"`
	Disabled       bool   `json:"disabled"`
	Classification string `json:"classification"`
	Action         string `json:"action"`
	Reason         string `json:"reason"`
	HTTPStatus     int    `json:"http_status,omitempty"`
	Model          string `json:"model,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

type jobSnapshot struct {
	Running         bool            `json:"running"`
	Stopped         bool            `json:"stopped"`
	Applying        bool            `json:"applying"`
	Done            int             `json:"done"`
	Total           int             `json:"total"`
	Workers         int             `json:"workers"`
	IncludeDisabled bool            `json:"include_disabled"`
	OnlyDisabled    bool            `json:"only_disabled"`
	ApplyDone       int             `json:"apply_done"`
	ApplyTotal      int             `json:"apply_total"`
	ApplyCurrent    string          `json:"apply_current,omitempty"`
	StartedAt       string          `json:"started_at,omitempty"`
	FinishedAt      string          `json:"finished_at,omitempty"`
	Results         []accountResult `json:"results"`
	Summary         map[string]int  `json:"summary"`
}

type startRequest struct {
	Workers         int  `json:"workers"`
	IncludeDisabled bool `json:"include_disabled"`
	OnlyDisabled    bool `json:"only_disabled"`
}

type applyRequest struct {
	// empty body means apply all recommended disable/enable actions
	AuthIndexes []string `json:"auth_indexes"`
}

type actionRequest struct {
	AuthIndex string `json:"auth_index"`
	Name      string `json:"name"`
	Disabled  bool   `json:"disabled"`
	Delete    bool   `json:"delete"`
}

type authListResponse struct {
	Files []pluginapi.HostAuthFileEntry `json:"files"`
}

type inspectionEngine struct {
	mu              sync.Mutex
	runWG           sync.WaitGroup
	running         bool
	stopped         bool
	applying        bool
	runID           uint64
	workers         int
	includeDisabled bool
	onlyDisabled    bool
	total           int
	results         []accountResult
	applyDone       int
	applyTotal      int
	applyCurrent    string
	startedAt       time.Time
	finishedAt      time.Time
}

var engine = &inspectionEngine{workers: 6}

func (e *inspectionEngine) snapshot() jobSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	results := append([]accountResult(nil), e.results...)
	summary := map[string]int{
		"total":             len(results),
		"healthy":           0,
		"permission_denied": 0,
		"quota_exhausted":   0,
		"reauth":            0,
		"other":             0,
	}
	for _, item := range results {
		switch item.Classification {
		case "healthy":
			summary["healthy"]++
		case "permission_denied":
			summary["permission_denied"]++
		case "quota_exhausted":
			summary["quota_exhausted"]++
		case "reauth":
			summary["reauth"]++
		default:
			summary["other"]++
		}
	}
	snap := jobSnapshot{
		Running:         e.running,
		Stopped:         e.stopped && !e.running,
		Applying:        e.applying,
		Done:            len(results),
		Total:           e.total,
		Workers:         e.workers,
		IncludeDisabled: e.includeDisabled,
		OnlyDisabled:    e.onlyDisabled,
		ApplyDone:       e.applyDone,
		ApplyTotal:      e.applyTotal,
		ApplyCurrent:    e.applyCurrent,
		Results:         results,
		Summary:         summary,
	}
	if !e.startedAt.IsZero() {
		snap.StartedAt = e.startedAt.Format(time.RFC3339)
	}
	if !e.finishedAt.IsZero() {
		snap.FinishedAt = e.finishedAt.Format(time.RFC3339)
	}
	return snap
}

func (e *inspectionEngine) start(req startRequest) error {
	e.mu.Lock()
	if e.running || e.applying {
		e.mu.Unlock()
		return fmt.Errorf("inspection already running")
	}
	workers := req.Workers
	if workers <= 0 {
		workers = 6
	}
	if workers > 16 {
		workers = 16
	}
	includeDisabled := req.IncludeDisabled
	onlyDisabled := req.OnlyDisabled
	if onlyDisabled {
		includeDisabled = false
	}
	e.running = true
	e.stopped = false
	e.applying = false
	e.workers = workers
	e.includeDisabled = includeDisabled
	e.onlyDisabled = onlyDisabled
	e.results = nil
	e.total = 0
	e.applyDone = 0
	e.applyTotal = 0
	e.applyCurrent = ""
	e.startedAt = time.Now()
	e.finishedAt = time.Time{}
	e.runID++
	runID := e.runID
	e.mu.Unlock()

	e.runWG.Add(1)
	go func() {
		defer e.runWG.Done()
		e.run(runID, workers, includeDisabled, onlyDisabled)
	}()
	return nil
}

func (e *inspectionEngine) stop() {
	e.mu.Lock()
	e.stopped = true
	e.mu.Unlock()
}

func (e *inspectionEngine) shutdown() {
	e.mu.Lock()
	e.stopped = true
	e.runID++
	e.mu.Unlock()
	e.runWG.Wait()
}

func (e *inspectionEngine) isStopped(runID uint64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopped || e.runID != runID
}

func (e *inspectionEngine) appendResult(runID uint64, result accountResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runID != runID || e.stopped {
		return
	}
	e.results = append(e.results, result)
}

func (e *inspectionEngine) finish(runID uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runID != runID {
		return
	}
	e.running = false
	e.finishedAt = time.Now()
}

func (e *inspectionEngine) run(runID uint64, workers int, includeDisabled, onlyDisabled bool) {
	defer e.finish(runID)

	list, errList := callHostAuthList()
	if errList != nil {
		e.appendResult(runID, accountResult{
			Name:           "system",
			Classification: "probe_error",
			Action:         "keep",
			Reason:         "列出账号失败: " + errList.Error(),
		})
		return
	}

	targets := make([]pluginapi.HostAuthFileEntry, 0)
	for _, file := range list.Files {
		if shouldInspectEntry(file.Provider, file.Name, file.Type, file.Disabled, file.Status, includeDisabled, onlyDisabled) {
			targets = append(targets, file)
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		return strings.ToLower(targets[i].Name) < strings.ToLower(targets[j].Name)
	})

	e.mu.Lock()
	if e.runID == runID {
		e.total = len(targets)
	}
	e.mu.Unlock()

	if len(targets) == 0 {
		return
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, file := range targets {
		if e.isStopped(runID) {
			break
		}
		file := file
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if e.isStopped(runID) {
				return
			}
			result := inspectAccount(file)
			e.appendResult(runID, result)
		}()
	}
	wg.Wait()
}

func inspectAccount(file pluginapi.HostAuthFileEntry) accountResult {
	name := firstNonEmpty(file.Email, file.Label, file.Name, file.AuthIndex, file.ID)
	base := accountResult{
		AuthIndex: file.AuthIndex,
		Name:      name,
		FileName:  file.Name,
		Email:     file.Email,
		Disabled:  file.Disabled || isDisabledEntry(file.Disabled, file.Status),
	}
	if strings.TrimSpace(file.AuthIndex) == "" {
		base.Classification = "probe_error"
		base.Action = "keep"
		base.Reason = "缺少 auth_index"
		return base
	}

	modelsResp, errModels := callHostAPICall(file.AuthIndex, http.MethodGet, "https://cli-chat-proxy.grok.com/v1/models", nil, false)
	model := "grok-4.5"
	if errModels == nil && modelsResp.StatusCode >= 200 && modelsResp.StatusCode < 300 {
		model = pickModel(modelsResp.Body)
	}
	base.Model = model

	chatBody := fmt.Sprintf(`{"model":%q,"input":"ping","stream":false}`, model)
	chatResp, errChat := callHostAPICall(file.AuthIndex, http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", []byte(chatBody), true)
	if errChat != nil {
		classified := classifyProbe(classifyInput{
			Disabled:     base.Disabled,
			RequestError: errChat.Error(),
		})
		base.Classification = classified.Classification
		base.Action = classified.Action
		base.Reason = classified.Reason
		base.ErrorMessage = errChat.Error()
		return base
	}

	status := chatResp.StatusCode
	parsed := extractError(chatResp.Body)
	if status == http.StatusForbidden || status == http.StatusUnauthorized || status == http.StatusTooManyRequests || status == http.StatusPaymentRequired {
		// fallback to chat completions
		fallbackBody := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"stream":false}`, model)
		fallbackResp, errFallback := callHostAPICall(file.AuthIndex, http.MethodPost, "https://cli-chat-proxy.grok.com/v1/chat/completions", []byte(fallbackBody), true)
		if errFallback == nil {
			chatResp = fallbackResp
			status = fallbackResp.StatusCode
			parsed = extractError(fallbackResp.Body)
		}
	}

	classified := classifyProbe(classifyInput{
		ChatStatus: status,
		ChatCode:   parsed.Code,
		ChatError:  parsed.Message,
		Disabled:   base.Disabled,
	})
	base.HTTPStatus = status
	base.ErrorCode = parsed.Code
	base.ErrorMessage = parsed.Message
	base.Classification = classified.Classification
	base.Action = classified.Action
	base.Reason = classified.Reason
	return base
}

type apiCallResponse struct {
	StatusCode int                 `json:"status_code"`
	Header     map[string][]string `json:"header"`
	Body       string              `json:"body"`
}

const xaiInspectionClientVersion = "0.2.93"

func xaiInspectionHeaders(token string, jsonBody bool) http.Header {
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("Accept", "application/json")
	headers.Set("X-XAI-Token-Auth", "xai-grok-cli")
	headers.Set("x-grok-client-version", xaiInspectionClientVersion)
	headers.Set("User-Agent", "xai-grok-workspace/"+xaiInspectionClientVersion)
	if jsonBody {
		headers.Set("Content-Type", "application/json")
	}
	return headers
}

func callHostAPICall(authIndex, method, rawURL string, body []byte, jsonBody bool) (apiCallResponse, error) {
	// Prefer host.http.do with resolved token from auth JSON.
	token, errToken := resolveAccessToken(authIndex)
	if errToken != nil {
		return apiCallResponse{}, errToken
	}
	result, errCall := callHost(pluginabi.MethodHostHTTPDo, map[string]any{
		"method":  method,
		"url":     rawURL,
		"headers": xaiInspectionHeaders(token, jsonBody),
		"body":    body,
	})
	if errCall != nil {
		return apiCallResponse{}, errCall
	}
	var resp struct {
		StatusCode int                 `json:"StatusCode"`
		Headers    map[string][]string `json:"Headers"`
		Body       []byte              `json:"Body"`
	}
	// host returns pluginapi.HTTPResponse with capital fields through JSON marshal of struct tags? check tags
	// pluginapi.HTTPResponse uses StatusCode/Headers/Body with no lowercase tags in type definition;
	// encoding/json will use field names StatusCode, Headers, Body.
	if err := json.Unmarshal(result, &resp); err != nil {
		// try lowercase just in case
		var alt apiCallResponse
		if errAlt := json.Unmarshal(result, &alt); errAlt == nil {
			return alt, nil
		}
		return apiCallResponse{}, fmt.Errorf("decode host.http.do: %w", err)
	}
	return apiCallResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Headers,
		Body:       string(resp.Body),
	}, nil
}

func resolveAccessToken(authIndex string) (string, error) {
	result, errCall := callHost(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: authIndex})
	if errCall != nil {
		return "", errCall
	}
	var resp pluginapi.HostAuthGetResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		return "", fmt.Errorf("decode host.auth.get: %w", err)
	}
	var data map[string]any
	if err := json.Unmarshal(resp.JSON, &data); err != nil {
		return "", fmt.Errorf("decode auth json: %w", err)
	}
	for _, key := range []string{"access_token", "token", "api_key", "id_token"} {
		if value := asString(data[key]); value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("token not found for auth_index %s", authIndex)
}

func callHostAuthList() (authListResponse, error) {
	result, errCall := callHost(pluginabi.MethodHostAuthList, map[string]any{})
	if errCall != nil {
		return authListResponse{}, errCall
	}
	var resp authListResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		return authListResponse{}, fmt.Errorf("decode host.auth.list: %w", err)
	}
	return resp, nil
}

func setAuthDisabledLegacy(name string, disabled bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	// load physical json, flip disabled, save back
	list, errList := callHostAuthList()
	if errList != nil {
		return errList
	}
	var target *pluginapi.HostAuthFileEntry
	for i := range list.Files {
		file := list.Files[i]
		if file.Name == name || file.ID == name || file.AuthIndex == name || file.Email == name {
			target = &file
			break
		}
	}
	if target == nil {
		return fmt.Errorf("auth not found: %s", name)
	}
	if strings.TrimSpace(target.AuthIndex) == "" {
		return fmt.Errorf("auth_index missing for %s", name)
	}
	getResult, errGet := callHost(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: target.AuthIndex})
	if errGet != nil {
		return errGet
	}
	var getResp pluginapi.HostAuthGetResponse
	if err := json.Unmarshal(getResult, &getResp); err != nil {
		return fmt.Errorf("decode host.auth.get: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(getResp.JSON, &payload); err != nil {
		return fmt.Errorf("decode auth json: %w", err)
	}
	payload["disabled"] = disabled
	raw, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return errMarshal
	}
	saveName := firstNonEmpty(getResp.Name, target.Name)
	if !strings.HasSuffix(strings.ToLower(saveName), ".json") {
		saveName += ".json"
	}
	_, errSave := callHost(pluginabi.MethodHostAuthSave, pluginapi.HostAuthSaveRequest{
		Name: saveName,
		JSON: raw,
	})
	if errSave != nil {
		return errSave
	}
	// reflect in current results
	engine.mu.Lock()
	for i := range engine.results {
		item := &engine.results[i]
		if item.AuthIndex == target.AuthIndex || item.Name == target.Name || item.Name == name {
			item.Disabled = disabled
			if disabled && (item.Action == "disable") {
				item.Action = "keep"
			}
			if !disabled && item.Action == "enable" {
				item.Action = "keep"
			}
			if !disabled && item.Classification == "healthy" {
				item.Action = "keep"
			}
			if disabled && item.Classification == "healthy" {
				item.Action = "enable"
				item.Disabled = true
			}
		}
	}
	engine.mu.Unlock()
	return nil
}

var (
	cpaManagementBaseURL = "http://127.0.0.1:8317"
	cpaManagementDo      = http.DefaultClient.Do
)

func cpaManagementPassword() string {
	return firstNonEmpty(os.Getenv("MANAGEMENT_PASSWORD"), os.Getenv("CPA_MANAGEMENT_KEY"))
}

func callCPAManagement(method, path string, body []byte) (int, []byte, error) {
	password := strings.TrimSpace(cpaManagementPassword())
	if password == "" {
		return 0, nil, fmt.Errorf("CPA management password is unavailable")
	}
	req, errRequest := http.NewRequest(method, strings.TrimRight(cpaManagementBaseURL, "/")+path, bytes.NewReader(body))
	if errRequest != nil {
		return 0, nil, errRequest
	}
	req.Header.Set("Authorization", "Bearer "+password)
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, errDo := cpaManagementDo(req)
	if errDo != nil {
		return 0, nil, errDo
	}
	defer resp.Body.Close()
	raw, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return resp.StatusCode, nil, errRead
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, raw, fmt.Errorf("CPA management API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return resp.StatusCode, raw, nil
}

func findAuthFile(name string) (*pluginapi.HostAuthFileEntry, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	list, errList := callHostAuthList()
	if errList != nil {
		return nil, errList
	}
	for i := range list.Files {
		file := &list.Files[i]
		if file.Name == name || file.ID == name || file.AuthIndex == name || file.Email == name {
			return file, nil
		}
	}
	return nil, fmt.Errorf("auth not found: %s", name)
}

func verifyAuthDisabled(authIndex, name string, disabled bool) error {
	list, errList := callHostAuthList()
	if errList != nil {
		return errList
	}
	for _, file := range list.Files {
		if file.AuthIndex == authIndex || file.Name == name || file.ID == name || file.Email == name {
			actual := file.Disabled || isDisabledEntry(file.Disabled, file.Status)
			if actual != disabled {
				return fmt.Errorf("CPA state verification failed for %s: disabled=%v, expected=%v", name, actual, disabled)
			}
			return nil
		}
	}
	return fmt.Errorf("auth disappeared while verifying %s", name)
}

func setAuthDisabled(name string, disabled bool) error {
	target, errTarget := findAuthFile(name)
	if errTarget != nil {
		return errTarget
	}
	if strings.TrimSpace(target.Name) == "" {
		return fmt.Errorf("auth file name missing for %s", name)
	}
	body, errMarshal := json.Marshal(map[string]any{
		"name":     target.Name,
		"disabled": disabled,
	})
	if errMarshal != nil {
		return errMarshal
	}
	if _, _, errPatch := callCPAManagement(http.MethodPatch, "/v0/management/auth-files/status", body); errPatch != nil {
		return errPatch
	}
	if errVerify := verifyAuthDisabled(target.AuthIndex, target.Name, disabled); errVerify != nil {
		return errVerify
	}
	engine.mu.Lock()
	for i := range engine.results {
		item := &engine.results[i]
		if item.AuthIndex == target.AuthIndex || item.FileName == target.Name || item.Name == name {
			item.Disabled = disabled
			if disabled && item.Action == "disable" {
				item.Action = "keep"
			}
			if !disabled && item.Action == "enable" {
				item.Action = "keep"
			}
			if disabled && item.Classification == "healthy" {
				item.Action = "enable"
			}
		}
	}
	engine.mu.Unlock()
	return nil
}

func deleteAuthFile(name string) error {
	target, errTarget := findAuthFile(name)
	if errTarget != nil {
		return errTarget
	}
	path := "/v0/management/auth-files?name=" + url.QueryEscape(target.Name)
	if _, _, errDelete := callCPAManagement(http.MethodDelete, path, nil); errDelete != nil {
		return errDelete
	}
	list, errList := callHostAuthList()
	if errList != nil {
		return errList
	}
	for _, file := range list.Files {
		if file.AuthIndex == target.AuthIndex || file.Name == target.Name || file.ID == target.ID {
			return fmt.Errorf("CPA state verification failed: deleted auth still present as %s", file.Name)
		}
	}
	engine.mu.Lock()
	for i := range engine.results {
		item := &engine.results[i]
		if item.AuthIndex == target.AuthIndex || item.FileName == target.Name {
			item.Action = "keep"
			item.Reason = "已删除"
		}
	}
	engine.mu.Unlock()
	return nil
}

func (e *inspectionEngine) applyRecommendations(indexes []string) (map[string]any, error) {
	e.mu.Lock()
	if e.running || e.applying {
		e.mu.Unlock()
		return nil, fmt.Errorf("busy")
	}
	candidates := make([]accountResult, 0)
	indexSet := map[string]struct{}{}
	for _, idx := range indexes {
		idx = strings.TrimSpace(idx)
		if idx != "" {
			indexSet[idx] = struct{}{}
		}
	}
	for _, item := range e.results {
		if item.Action != "disable" && item.Action != "enable" && item.Action != "delete" {
			continue
		}
		if len(indexSet) > 0 {
			if _, ok := indexSet[item.AuthIndex]; !ok {
				if _, okName := indexSet[item.Name]; !okName {
					continue
				}
			}
		}
		candidates = append(candidates, item)
	}
	if len(candidates) == 0 {
		e.mu.Unlock()
		return nil, fmt.Errorf("no recommended actions")
	}
	e.applying = true
	e.applyDone = 0
	e.applyTotal = len(candidates)
	e.applyCurrent = ""
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.applying = false
		e.applyCurrent = ""
		e.mu.Unlock()
	}()

	success := 0
	failures := make([]string, 0)
	for _, item := range candidates {
		e.mu.Lock()
		e.applyCurrent = item.Action + " " + item.Name
		e.mu.Unlock()
		targetName := firstNonEmpty(item.FileName, item.AuthIndex, item.Name)
		var errAction error
		if item.Action == "delete" {
			errAction = deleteAuthFile(targetName)
		} else {
			errAction = setAuthDisabled(targetName, item.Action == "disable")
		}
		if errAction != nil {
			failures = append(failures, item.Name+": "+errAction.Error())
		} else {
			success++
		}
		e.mu.Lock()
		e.applyDone++
		e.mu.Unlock()
	}
	return map[string]any{
		"success":  success,
		"failed":   len(failures),
		"failures": failures,
	}, nil
}
