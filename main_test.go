package main

import (
	"net/http"
	"strings"
	"testing"

	"grok-inspection/cpasdk/pluginapi"
)

func TestManagementStatusReturnsJSON(t *testing.T) {
	response := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/status",
	})

	if got := response.Headers.Get("content-type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type = %q, want application/json", got)
	}
}

func TestResourceStatusReturnsHTML(t *testing.T) {
	response := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/resource/plugins/grok-inspection/status",
	})

	if got := response.Headers.Get("content-type"); !strings.Contains(got, "text/html") {
		t.Fatalf("content-type = %q, want text/html", got)
	}
}

func TestResourcePageDoesNotPollWithoutManagementKey(t *testing.T) {
	page := string(renderUIPage(pluginName))
	guard := "if (!keyInput.value.trim())"
	refresh := "async function refresh()"
	refreshIndex := strings.Index(page, refresh)
	guardIndex := strings.Index(page, guard)

	if refreshIndex < 0 || guardIndex < refreshIndex {
		t.Fatalf("refresh must guard management requests with %q", guard)
	}
}

func TestResourcePageHasMobileScopedDarkModeStyles(t *testing.T) {
	page := string(renderUIPage(pluginName))
	required := []string{
		`class="wrap grok-inspection-page"`,
		`.grok-inspection-page`,
		`@media (max-width:760px)`,
		`@media (prefers-color-scheme: dark)`,
		`overflow-x:auto`,
		`min-width:0`,
	}
	for _, marker := range required {
		if !strings.Contains(page, marker) {
			t.Fatalf("resource page missing mobile/dark-mode marker %q", marker)
		}
	}
}

func TestResourcePageShowsManagementKeyPrompt(t *testing.T) {
	page := string(renderUIPage(pluginName))
	required := []string{
		`请输入 CPA Management Key`,
		`const hasManagementKey = () => !!keyInput.value.trim();`,
		`$('runBtn').disabled = !hasManagementKey() ||`,
		`'请输入 CPA Management Key 后加载巡检状态'`,
	}
	for _, marker := range required {
		if !strings.Contains(page, marker) {
			t.Fatalf("resource page missing management-key UX marker %q", marker)
		}
	}
}

func TestResourcePageHasExportAndBatchOps(t *testing.T) {
	page := string(renderUIPage(pluginName))
	required := []string{
		`id="workers"`,
		`value="6"`,
		`parseWorkersStrict`,
		`id="batchExportBtn"`,
		`id="batchDisableBtn"`,
		`id="batchDeleteBtn"`,
		`id="confirmModal"`,
		`function confirmDialog`,
		`当前分类：`,
		`force_action: action`,
		`filteredAuthIndexes`,
		`批量禁用`,
		`批量删除`,
		`批量导出`,
		`function stopPolling()`,
		`function startPolling()`,
		`function syncPolling(snap)`,
		`snap.running || snap.applying`,
		`id="incrBtn"`,
		`增量巡检`,
		`incremental: !!incremental`,
		`['other','异常'`,
	}
	for _, marker := range required {
		if !strings.Contains(page, marker) {
			t.Fatalf("resource page missing marker %q", marker)
		}
	}
	if strings.Contains(page, `setInterval(refresh, 1500)`) {
		t.Fatal("page must not permanently poll /status every 1.5s when idle")
	}
	// Duplicate filter button row should be gone; cards are the only category UI.
	if strings.Contains(page, `id="filters"`) {
		t.Fatal("duplicate filter button row should be removed; use summary cards only")
	}
}

func TestApplyAcceptedAsync(t *testing.T) {
	// Without candidates, apply returns conflict quickly (no hang).
	response := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/apply",
		Body:   []byte(`{}`),
	})
	if response.StatusCode != http.StatusConflict && response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", response.StatusCode, string(response.Body))
	}
}
