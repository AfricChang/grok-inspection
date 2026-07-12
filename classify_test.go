package main

import (
	"net/http"
	"testing"
)

func TestClassifyPermissionDenied(t *testing.T) {
	got := classifyProbe(classifyInput{
		ChatStatus: 403,
		ChatCode:   "permission-denied",
		ChatError:  "Access to the chat endpoint is denied",
		Disabled:   false,
	})
	if got.Classification != "permission_denied" || got.Action != "disable" {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifyReauthRecommendsDelete(t *testing.T) {
	got := classifyProbe(classifyInput{
		ChatStatus: http.StatusUnauthorized,
		ChatError:  "Invalid or expired credentials",
	})
	if got.Classification != "reauth" || got.Action != "delete" {
		t.Fatalf("got %+v", got)
	}
}

func TestShouldInspectOnlyDisabled(t *testing.T) {
	if !shouldInspectEntry("xai", "xai-a.json", "xai", true, "", false, true) {
		t.Fatal("expected disabled xai account")
	}
	if shouldInspectEntry("xai", "xai-b.json", "xai", false, "", false, true) {
		t.Fatal("expected enabled account skipped")
	}
	if shouldInspectEntry("codex", "c.json", "codex", true, "", true, true) {
		t.Fatal("expected non-xai skipped")
	}
}

func TestPickModel(t *testing.T) {
	body := `{"data":[{"id":"grok-3"},{"id":"grok-4.5-build-free"}]}`
	if got := pickModel(body); got != "grok-4.5-build-free" {
		t.Fatalf("got %s", got)
	}
}

func TestXAIInspectionHeadersMatchCLIProxyIdentity(t *testing.T) {
	headers := xaiInspectionHeaders("test-token", true)

	if got := headers.Get("X-XAI-Token-Auth"); got != "xai-grok-cli" {
		t.Fatalf("X-XAI-Token-Auth = %q", got)
	}
	if got := headers.Get("x-grok-client-version"); got != "0.2.93" {
		t.Fatalf("x-grok-client-version = %q", got)
	}
	if got := headers.Get("User-Agent"); got != "xai-grok-workspace/0.2.93" {
		t.Fatalf("User-Agent = %q", got)
	}
	if got := headers.Get("x-grok-client-identifier"); got != "" {
		t.Fatalf("unexpected x-grok-client-identifier = %q", got)
	}
	if got := headers.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
}
