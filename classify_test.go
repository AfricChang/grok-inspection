package main

import (
	"net/http"
	"strings"
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

func TestResolveProbeOutcomeKeepsPrimaryQuotaWhenFallbackHealthy(t *testing.T) {
	primary := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusTooManyRequests,
		Body:       `{"code":"free-usage-exhausted","error":"Included free usage has been exhausted"}`,
	}, false)
	fallback := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusOK,
		Body:       `{"choices":[{"message":{"content":"pong"}}]}`,
	}, false)

	got := resolveProbeOutcome(primary, fallback)
	if got.Classified.Classification != "quota_exhausted" || got.Classified.Action != "disable" {
		t.Fatalf("got %+v", got)
	}
	if got.Response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", got.Response.StatusCode)
	}
	if !strings.Contains(got.Classified.Reason, "结果不一致") {
		t.Fatalf("reason = %q", got.Classified.Reason)
	}
}

func TestResolveProbeOutcomeUsesFallbackForAmbiguousPrimary(t *testing.T) {
	primary := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusInternalServerError,
		Body:       `{"error":"temporary upstream failure"}`,
	}, false)
	fallback := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusOK,
		Body:       `{"choices":[{"message":{"content":"pong"}}]}`,
	}, false)

	got := resolveProbeOutcome(primary, fallback)
	if got.Classified.Classification != "healthy" {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifyBare429IsProbeErrorNotQuota(t *testing.T) {
	got := classifyProbe(classifyInput{
		ChatStatus: http.StatusTooManyRequests,
		ChatError:  "rate limited",
	})
	if got.Classification != "probe_error" || got.Action != "keep" {
		t.Fatalf("bare 429 should be probe_error/keep, got %+v", got)
	}
}

func TestClassifyFreeUsageExhaustedIsQuota(t *testing.T) {
	got := classifyProbe(classifyInput{
		ChatStatus: http.StatusTooManyRequests,
		ChatCode:   "free-usage-exhausted",
		ChatError:  "Included free usage has been exhausted",
	})
	if got.Classification != "quota_exhausted" || got.Action != "disable" {
		t.Fatalf("free-usage should be quota_exhausted/disable, got %+v", got)
	}
}

func TestResolveProbeOutcomeUsesFallbackWhenPrimaryBare429(t *testing.T) {
	primary := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusTooManyRequests,
		Body:       `{"error":"too many requests"}`,
	}, false)
	fallback := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusOK,
		Body:       `{"choices":[{"message":{"content":"pong"}}]}`,
	}, false)
	got := resolveProbeOutcome(primary, fallback)
	if got.Classified.Classification != "healthy" {
		t.Fatalf("bare 429 + healthy fallback should be healthy, got %+v", got)
	}
}

