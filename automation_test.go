package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

func TestAutomaticSequenceBlocksManualStartsBetweenProbeAndApply(t *testing.T) {
	e := &inspectionEngine{workers: defaultWorkers, automatic: true, results: []accountResult{{AuthIndex: "quota", Classification: "quota_exhausted", Action: "disable"}}}
	if err := e.start(startRequest{Workers: 2}); err == nil || !strings.Contains(err.Error(), "automatic inspection sequence") {
		t.Fatalf("manual inspection error = %v", err)
	}
	if err := e.startApply(applyRequest{}, "", nil); err == nil || !strings.Contains(err.Error(), "automatic inspection sequence") {
		t.Fatalf("manual apply error = %v", err)
	}
	if _, _, err := e.startAction(actionRequest{Name: "quota", Disabled: true}, "", nil); err == nil || !strings.Contains(err.Error(), "automatic inspection sequence") {
		t.Fatalf("manual action error = %v", err)
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

func TestAutomationWakeSignalAndPendingSnapshot(t *testing.T) {
	a := &automationManager{wakeCh: make(chan struct{}, 1), stopCh: make(chan struct{})}
	a.signalWake()
	select {
	case <-a.wakeCh:
	default:
		t.Fatal("wake signal was not queued")
	}
	a.pendingID = "rule-pending"
	a.pendingIDs = []string{"rule-pending"}
	a.pendingReason = "等待空闲"
	a.rules = []automationRule{{ID: "rule-pending", Enabled: true, Weekdays: []int{1, 2, 3, 4, 5, 6, 7}, WindowStart: "00:00", WindowEnd: "00:00", IntervalMinutes: 60, LastRunAt: time.Now().Add(-time.Hour).Format(time.RFC3339)}}
	snap := a.snapshot(false)
	if snap.PendingRuleID != "rule-pending" || snap.PendingReason != "等待空闲" {
		t.Fatalf("pending snapshot = %+v", snap)
	}
	if snap.Rules[0].NextRunAt != "" {
		t.Fatalf("pending rule next_run_at = %q, want empty", snap.Rules[0].NextRunAt)
	}
}

func TestMergedRunningRuleBlocksDeletingEverySourceRule(t *testing.T) {
	a := &automationManager{
		rules:      []automationRule{{ID: "a"}, {ID: "b"}},
		runningID:  "a",
		runningIDs: []string{"a", "b"},
		wakeCh:     make(chan struct{}, 1),
		stopCh:     make(chan struct{}),
	}
	if err := a.deleteRule("b"); err == nil || !strings.Contains(err.Error(), "正在执行") {
		t.Fatalf("delete merged source error = %v", err)
	}
}

func TestExecuteRuleDefersWhileEngineBusyWithoutSkippedHistory(t *testing.T) {
	oldEngine := engine
	engine = &inspectionEngine{workers: defaultWorkers, running: true}
	t.Cleanup(func() { engine = oldEngine })

	rule := automationRule{ID: "busy-rule", Name: "busy", Enabled: true, WindowStart: "00:00", WindowEnd: "23:59", IntervalMinutes: 60, Scope: []string{"quota_exhausted"}, Workers: 2}
	a := &automationManager{rules: []automationRule{rule}, runningID: rule.ID, runningName: rule.Name, wakeCh: make(chan struct{}, 1), stopCh: make(chan struct{})}
	a.executeRule(rule)
	snap := a.snapshot(true)
	if snap.PendingRuleID != rule.ID || snap.RunningRuleID != "" {
		t.Fatalf("deferred snapshot = %+v", snap)
	}
	if len(snap.History) != 0 {
		t.Fatalf("busy deferral should not add skipped history: %+v", snap.History)
	}
}

func TestExecuteRuleWaitsForEngineCompletionNotification(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(filepath.Join(dir, "results.json"))
	oldEngine := engine
	oldAutomation := automation
	engine = &inspectionEngine{workers: defaultWorkers, results: []accountResult{{AuthIndex: "quota-auth", Name: "quota.json", FileName: "quota.json", Disabled: true, Classification: "quota_exhausted", Action: "keep", NextInspectAt: time.Now().Add(-time.Minute).Format(time.RFC3339)}}}
	rule := automationRule{ID: "completion-rule", Name: "completion", Enabled: true, WindowStart: "00:00", WindowEnd: "23:59", IntervalMinutes: 60, Scope: []string{"quota_exhausted"}, Workers: 2}
	a := &automationManager{rules: []automationRule{rule}, runningID: rule.ID, runningName: rule.Name, wakeCh: make(chan struct{}, 1), stopCh: make(chan struct{})}
	automation = a
	t.Cleanup(func() {
		engine.runWG.Wait()
		setStoreFilePathForTest("")
		engine = oldEngine
		automation = oldAutomation
	})

	done := make(chan struct{})
	go func() {
		a.executeRule(rule)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("executeRule did not finish after engine completion")
	}
	snap := a.snapshot(true)
	if snap.RunningRuleID != "" || len(snap.History) != 1 || snap.History[0].Inspected != 1 {
		t.Fatalf("completion snapshot = %+v", snap)
	}
}

func TestRuleDueCatchUpAndCooldownBoundaries(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.Local)
	rule := automationRule{Enabled: true, Weekdays: []int{1, 2, 3, 4, 5, 6, 7}, WindowStart: "00:00", WindowEnd: "23:59", IntervalMinutes: 60}
	if ruleDue(rule, now) {
		t.Fatal("rule without last_run_at should wait one interval")
	}
	rule.LastRunAt = now.Add(-90 * time.Minute).Format(time.RFC3339)
	if !ruleDue(rule, now) || ruleMissedTooLong(rule, now) {
		t.Fatal("rule missed within one interval should catch up once")
	}
	rule.LastRunAt = now.Add(-121 * time.Minute).Format(time.RFC3339)
	if !ruleMissedTooLong(rule, now) {
		t.Fatal("stale rule should skip historical catch-up")
	}
	item := accountResult{NextInspectAt: now.Format(time.RFC3339)}
	if !automaticResultDue(item, now) {
		t.Fatal("cooldown boundary should be due")
	}
	item.NextInspectAt = now.Add(time.Second).Format(time.RFC3339)
	if automaticResultDue(item, now) {
		t.Fatal("future cooldown should not be due")
	}
}

func TestNextRuleRunAtMovesToNextValidWindow(t *testing.T) {
	now := time.Date(2026, 7, 17, 23, 30, 0, 0, time.Local)
	rule := automationRule{
		Enabled:         true,
		Weekdays:        []int{1, 2, 3, 4, 5, 6, 7},
		WindowStart:     "08:00",
		WindowEnd:       "23:00",
		IntervalMinutes: 60,
		LastRunAt:       now.Add(-2 * time.Hour).Format(time.RFC3339),
	}
	got := nextRuleRunAt(rule, now)
	want := time.Date(2026, 7, 18, 8, 0, 0, 0, time.Local).Format(time.RFC3339)
	if got != want {
		t.Fatalf("next run = %s, want %s", got, want)
	}
}

func TestAutomationStoreRejectsUnknownVersion(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(filepath.Join(dir, "results.json"))
	t.Cleanup(func() { setStoreFilePathForTest("") })
	raw, err := json.Marshal(automationDiskState{Version: automationStoreVersion + 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "automation.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAutomationRules(); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unknown version error = %v", err)
	}
}

func TestAutomationManagerDisablesInvalidLoadedRule(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(filepath.Join(dir, "results.json"))
	t.Cleanup(func() { setStoreFilePathForTest("") })
	state := automationDiskState{Version: automationStoreVersion, Rules: []automationRule{{ID: "invalid", Name: "invalid", Enabled: true, WindowStart: "bad", WindowEnd: "23:59", IntervalMinutes: 60, Scope: []string{"quota_exhausted"}, Workers: 2}}}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "automation.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	a := newAutomationManager()
	snap := a.snapshot(false)
	if len(snap.Rules) != 1 || snap.Rules[0].Enabled || !strings.Contains(snap.LastError, "已停用") {
		t.Fatalf("invalid loaded rule snapshot = %+v", snap)
	}
}

func TestMergeAutomationRulesDeduplicatesScopes(t *testing.T) {
	merged := mergeAutomationRules([]automationRule{
		{ID: "a", Name: "quota", Enabled: true, Scope: []string{"quota_exhausted"}, Workers: 2, IncludeDisabled: true},
		{ID: "b", Name: "problems", Enabled: true, Scope: []string{"quota_exhausted", "permission_denied"}, Workers: 6, ApplyRecommendations: true},
	})
	if strings.Join(merged.SourceRuleIDs, ",") != "a,b" || strings.Join(merged.Scope, ",") != "permission_denied,quota_exhausted" {
		t.Fatalf("merged rule = %+v", merged)
	}
	if merged.Workers != 6 || !merged.IncludeDisabled || merged.ApplyRecommendations {
		t.Fatalf("merged execution options = %+v", merged)
	}
}

func TestAutomationLoopWakesAndDefersMergedDueRules(t *testing.T) {
	oldEngine := engine
	engine = &inspectionEngine{workers: defaultWorkers, running: true}
	t.Cleanup(func() { engine = oldEngine })
	now := time.Now()
	rules := []automationRule{
		{ID: "a", Name: "quota", Enabled: true, Weekdays: []int{1, 2, 3, 4, 5, 6, 7}, WindowStart: "00:00", WindowEnd: "00:00", IntervalMinutes: 60, LastRunAt: now.Add(-70 * time.Minute).Format(time.RFC3339), Scope: []string{"quota_exhausted"}, Workers: 2},
		{ID: "b", Name: "permission", Enabled: true, Weekdays: []int{1, 2, 3, 4, 5, 6, 7}, WindowStart: "00:00", WindowEnd: "00:00", IntervalMinutes: 60, LastRunAt: now.Add(-70 * time.Minute).Format(time.RFC3339), Scope: []string{"permission_denied"}, Workers: 4},
	}
	a := &automationManager{rules: rules, wakeCh: make(chan struct{}, 1), stopCh: make(chan struct{})}
	a.start()
	a.signalWake()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap := a.snapshot(false)
		if snap.PendingRuleID == "a" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	snap := a.snapshot(false)
	a.shutdown()
	if snap.PendingRuleID != "a" || !strings.Contains(snap.PendingReason, "空闲后补跑") {
		t.Fatalf("loop pending snapshot = %+v", snap)
	}
}

func TestPendingRuleRunsImmediatelyWhenEngineBecomesIdle(t *testing.T) {
	setStoreFilePathForTest(filepath.Join(t.TempDir(), "results.json"))
	oldEngine := engine
	oldAutomation := automation
	engine = &inspectionEngine{workers: defaultWorkers}
	now := time.Now()
	rule := automationRule{ID: "pending", Name: "pending", Enabled: true, Weekdays: []int{1, 2, 3, 4, 5, 6, 7}, WindowStart: "00:00", WindowEnd: "00:00", IntervalMinutes: 60, LastRunAt: now.Add(-70 * time.Minute).Format(time.RFC3339), Scope: []string{"quota_exhausted"}, Workers: 2}
	a := &automationManager{rules: []automationRule{rule}, pendingID: rule.ID, pendingIDs: []string{rule.ID}, pendingAt: now.Add(-time.Minute), pendingReason: "等待空闲", wakeCh: make(chan struct{}, 1), stopCh: make(chan struct{})}
	automation = a
	t.Cleanup(func() {
		a.wg.Wait()
		engine.runWG.Wait()
		engine.persistWG.Wait()
		engine = oldEngine
		automation = oldAutomation
		setStoreFilePathForTest("")
	})
	a.runDue(now)
	a.wg.Wait()
	snap := a.snapshot(true)
	if snap.PendingRuleID != "" || len(snap.History) != 1 || snap.History[0].Status != "success" {
		t.Fatalf("pending execution snapshot = %+v", snap)
	}
}

func TestPendingRuleExpiresAfterOneInterval(t *testing.T) {
	setStoreFilePathForTest(filepath.Join(t.TempDir(), "results.json"))
	t.Cleanup(func() { setStoreFilePathForTest("") })
	now := time.Now()
	rule := automationRule{ID: "pending", Name: "pending", Enabled: true, Weekdays: []int{1, 2, 3, 4, 5, 6, 7}, WindowStart: "00:00", WindowEnd: "00:00", IntervalMinutes: 60, LastRunAt: now.Add(-2 * time.Hour).Format(time.RFC3339), Scope: []string{"quota_exhausted"}, Workers: 2}
	a := &automationManager{rules: []automationRule{rule}, pendingID: rule.ID, pendingIDs: []string{rule.ID}, pendingAt: now.Add(-61 * time.Minute), pendingReason: "等待空闲", wakeCh: make(chan struct{}, 1), stopCh: make(chan struct{})}
	a.runDue(now)
	snap := a.snapshot(false)
	if snap.PendingRuleID != "" || !strings.Contains(snap.LastError, "补跑已跳过") {
		t.Fatalf("expired pending snapshot = %+v", snap)
	}
}

func TestExecuteRuleRecordsNonBusyStartFailure(t *testing.T) {
	setStoreFilePathForTest(filepath.Join(t.TempDir(), "results.json"))
	oldEngine := engine
	oldAutomation := automation
	engine = &inspectionEngine{workers: defaultWorkers, results: []accountResult{{AuthIndex: "quota", Name: "quota", Classification: "quota_exhausted", NextInspectAt: time.Now().Add(-time.Minute).Format(time.RFC3339)}}}
	rule := automationRule{ID: "invalid-workers", Name: "invalid", Enabled: true, Scope: []string{"quota_exhausted"}, Workers: maxWorkers + 1}
	a := &automationManager{rules: []automationRule{rule}, runningID: rule.ID, runningIDs: []string{rule.ID}, wakeCh: make(chan struct{}, 1), stopCh: make(chan struct{})}
	automation = a
	t.Cleanup(func() {
		engine = oldEngine
		automation = oldAutomation
		setStoreFilePathForTest("")
	})
	a.executeRule(rule)
	snap := a.snapshot(true)
	if len(snap.History) != 1 || snap.History[0].Status != "failed" || !strings.Contains(snap.History[0].Error, "workers") {
		t.Fatalf("failed history = %+v", snap.History)
	}
}

func TestPluginRuntimeLifecycleStartsAndStopsManagers(t *testing.T) {
	oldEngine := engine
	oldAutomation := automation
	engine = &inspectionEngine{workers: defaultWorkers}
	automation = &automationManager{wakeCh: make(chan struct{}, 1), stopCh: make(chan struct{})}
	t.Cleanup(func() {
		engine = oldEngine
		automation = oldAutomation
	})
	startPluginRuntime()
	automation.mu.Lock()
	startedAfterInit := automation.started
	automation.mu.Unlock()
	if !startedAfterInit {
		t.Fatal("automation manager did not start")
	}
	shutdownPluginRuntime()
	automation.mu.Lock()
	started := automation.started
	automation.mu.Unlock()
	engine.mu.Lock()
	stopped := engine.stopped
	engine.mu.Unlock()
	if started || !stopped {
		t.Fatalf("runtime shutdown started=%v stopped=%v", started, stopped)
	}
}

func TestManualStopWakesPendingScheduler(t *testing.T) {
	setStoreFilePathForTest(filepath.Join(t.TempDir(), "results.json"))
	oldEngine := engine
	oldAutomation := automation
	engine = &inspectionEngine{workers: defaultWorkers, running: true, runDoneCh: make(chan struct{})}
	automation = &automationManager{wakeCh: make(chan struct{}, 1), stopCh: make(chan struct{})}
	t.Cleanup(func() {
		engine = oldEngine
		automation = oldAutomation
		setStoreFilePathForTest("")
	})
	engine.stop()
	select {
	case <-automation.wakeCh:
	default:
		t.Fatal("manual stop did not wake pending scheduler")
	}
}
