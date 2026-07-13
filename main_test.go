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
