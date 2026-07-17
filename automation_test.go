package main

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestPluginRegistersUsageCapability(t *testing.T) {
	registration := pluginRegistration()
	if !registration.Capabilities.ManagementAPI || !registration.Capabilities.UsagePlugin {
		t.Fatalf("capabilities = %+v", registration.Capabilities)
	}
}

func TestManagementKeyCacheFeedsAsyncUsageActions(t *testing.T) {
	managementPasswordCache.RLock()
	old := managementPasswordCache.value
	managementPasswordCache.RUnlock()
	t.Cleanup(func() {
		managementPasswordCache.Lock()
		managementPasswordCache.value = old
		managementPasswordCache.Unlock()
	})
	managementPasswordCache.Lock()
	managementPasswordCache.value = ""
	managementPasswordCache.Unlock()
	rememberManagementPassword(http.Header{"Authorization": []string{"Bearer cached-key"}})
	if got := resolveManagementPassword(nil); got != "cached-key" {
		t.Fatalf("cached management password = %q", got)
	}
}

func TestUsageQuotaFailureUpdatesExactAuthWithoutAutoDelete(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(dir + string(os.PathSeparator) + "results.json")
	t.Cleanup(func() { setStoreFilePathForTest("") })

	oldEngine := engine
	oldAutomation := automation
	engine = &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{{
			AuthIndex:      "auth-quota",
			Name:           "quota.json",
			FileName:       "quota.json",
			Disabled:       true,
			Classification: "healthy",
			Action:         "keep",
		}},
	}
	automation = newAutomationManager()
	t.Cleanup(func() {
		engine.runWG.Wait()
		engine = oldEngine
		automation = oldAutomation
	})

	processUsageRecord(pluginapi.UsageRecord{
		Provider:  "xai",
		AuthID:    "runtime-id",
		AuthIndex: "auth-quota",
		Model:     "grok-4.5",
		Failed:    true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 429,
			Body:       `{"code":"subscription:free-usage-exhausted","error":"You've used all the included free usage"}`,
		},
	})

	snap := engine.snapshot(true)
	if len(snap.Results) != 1 {
		t.Fatalf("results = %+v", snap.Results)
	}
	got := snap.Results[0]
	if got.AuthIndex != "auth-quota" || got.Classification != "quota_exhausted" {
		t.Fatalf("result = %+v", got)
	}
	if got.Action != "keep" {
		t.Fatalf("disabled quota account action = %q, want keep", got.Action)
	}
	if got.LastInspectedAt == "" || got.NextInspectAt == "" {
		t.Fatalf("timestamps missing: %+v", got)
	}
}

func TestUsageBare429DoesNotDisableHealthyAccount(t *testing.T) {
	oldEngine := engine
	engine = &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{{AuthIndex: "auth-rate", Classification: "healthy", Action: "keep"}},
	}
	t.Cleanup(func() { engine = oldEngine })

	processUsageRecord(pluginapi.UsageRecord{
		Provider:  "xai",
		AuthIndex: "auth-rate",
		Failed:    true,
		Failure:   pluginapi.UsageFailure{StatusCode: 429, Body: `{"error":"rate limited"}`},
	})
	got := engine.snapshot(true).Results[0]
	if got.Classification != "healthy" || got.Action != "keep" {
		t.Fatalf("bare 429 changed healthy result: %+v", got)
	}
}

func TestUsageReauthIsExcludedAndNeverDeletes(t *testing.T) {
	oldEngine := engine
	oldAutomation := automation
	engine = &inspectionEngine{workers: defaultWorkers}
	setStoreFilePathForTest(t.TempDir() + string(os.PathSeparator) + "results.json")
	automation = newAutomationManager()
	t.Cleanup(func() {
		setStoreFilePathForTest("")
		engine = oldEngine
		automation = oldAutomation
	})

	processUsageRecord(pluginapi.UsageRecord{
		Provider:  "grok",
		AuthIndex: "auth-expired",
		Failed:    true,
		Failure:   pluginapi.UsageFailure{StatusCode: 401, Body: `{"error":"token is expired"}`},
	})
	got := engine.snapshot(true).Results[0]
	if got.Classification != "reauth" || got.Action != "keep" || !got.AutoInspectExcluded {
		t.Fatalf("reauth result = %+v", got)
	}
}

func TestAutomationRuleValidationAndCrossMidnightWindow(t *testing.T) {
	rule, err := validateAutomationRule(automationRule{
		Name:            "night",
		Enabled:         true,
		Weekdays:        []int{1, 2, 3, 4, 5, 6, 7},
		WindowStart:     "22:00",
		WindowEnd:       "06:00",
		IntervalMinutes: 120,
		Scope:           []string{"quota_exhausted", "permission_denied"},
		Workers:         4,
	})
	if err != nil {
		t.Fatal(err)
	}
	late := time.Date(2026, 7, 17, 23, 30, 0, 0, time.Local)
	early := time.Date(2026, 7, 17, 5, 30, 0, 0, time.Local)
	day := time.Date(2026, 7, 17, 12, 0, 0, 0, time.Local)
	if !ruleWindowContains(rule, late) || !ruleWindowContains(rule, early) || ruleWindowContains(rule, day) {
		t.Fatalf("cross-midnight window mismatch")
	}
	_, err = validateAutomationRule(automationRule{Name: "bad", WindowStart: "00:00", WindowEnd: "23:59", IntervalMinutes: 60, Scope: []string{"healthy"}, Workers: 2})
	if err == nil || !strings.Contains(err.Error(), "不支持分类") {
		t.Fatalf("healthy automatic scope error = %v", err)
	}
}

func TestAutomaticApplyCannotDelete(t *testing.T) {
	e := &inspectionEngine{results: []accountResult{{AuthIndex: "reauth", Classification: "reauth", Action: "delete"}}}
	candidates, err := e.collectCandidates(applyRequest{Automatic: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("automatic candidates include delete: %+v", candidates)
	}
	_, err = e.collectCandidates(applyRequest{Automatic: true, ForceAction: "delete", AuthIndexes: []string{"reauth"}})
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("forced automatic delete error = %v", err)
	}
}

func TestAutomationManagementCRUDAndUI(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(dir + string(os.PathSeparator) + "results.json")
	oldAutomation := automation
	automation = newAutomationManager()
	t.Cleanup(func() {
		setStoreFilePathForTest("")
		automation = oldAutomation
	})

	create := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/automation/rules",
		Body:   []byte(`{"name":"quota recheck","enabled":true,"weekdays":[1,2,3,4,5,6,7],"window_start":"00:00","window_end":"23:59","interval_minutes":120,"scope":["quota_exhausted"],"workers":4,"include_disabled":true,"apply_recommendations":true}`),
	})
	if create.StatusCode != http.StatusOK {
		t.Fatalf("create status=%d body=%s", create.StatusCode, string(create.Body))
	}
	list := dispatchManagement(pluginapi.ManagementRequest{Method: http.MethodGet, Path: "/v0/management/plugins/grok-inspection/automation"})
	if list.StatusCode != http.StatusOK || !strings.Contains(string(list.Body), "quota recheck") {
		t.Fatalf("list status=%d body=%s", list.StatusCode, string(list.Body))
	}

	page := string(renderUIPage(pluginName))
	for _, marker := range []string{"自动巡检", "autoSave", "/automation/rules", "永不自动删除", "Usage", "PASSIVE_POLL_MS", "item.message"} {
		if !strings.Contains(page, marker) {
			t.Fatalf("automation UI missing %q", marker)
		}
	}
}

func TestUsageHistoryHasChineseActionMessage(t *testing.T) {
	setStoreFilePathForTest(t.TempDir() + string(os.PathSeparator) + "results.json")
	t.Cleanup(func() { setStoreFilePathForTest("") })
	a := newAutomationManager()
	result := accountResult{Name: "quota@example.com", Classification: "quota_exhausted", Action: "disable", HTTPStatus: 429}
	a.recordUsageEvent(result, false, "")
	a.recordUsageEvent(result, true, "")
	history := a.snapshot(true).History
	if len(history) < 2 || history[0].Message != "额度用尽：自动禁用成功" || !strings.Contains(history[1].Message, "准备自动禁用") {
		t.Fatalf("history messages = %+v", history)
	}
}
