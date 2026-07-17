package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxAutomationHistory = 50

type automationRule struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Enabled              bool     `json:"enabled"`
	Weekdays             []int    `json:"weekdays"`
	WindowStart          string   `json:"window_start"`
	WindowEnd            string   `json:"window_end"`
	IntervalMinutes      int      `json:"interval_minutes"`
	Scope                []string `json:"scope"`
	Workers              int      `json:"workers"`
	IncludeDisabled      bool     `json:"include_disabled"`
	ApplyRecommendations bool     `json:"apply_recommendations"`
	LastRunAt            string   `json:"last_run_at,omitempty"`
	NextRunAt            string   `json:"next_run_at,omitempty"`
	SourceRuleIDs        []string `json:"-"`
}

type automationHistory struct {
	ID             string `json:"id"`
	RuleID         string `json:"rule_id,omitempty"`
	RuleName       string `json:"rule_name,omitempty"`
	Kind           string `json:"kind"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at,omitempty"`
	Status         string `json:"status"`
	Inspected      int    `json:"inspected,omitempty"`
	Applied        int    `json:"applied,omitempty"`
	Account        string `json:"account,omitempty"`
	Classification string `json:"classification,omitempty"`
	HTTPStatus     int    `json:"http_status,omitempty"`
	Error          string `json:"error,omitempty"`
	Message        string `json:"message,omitempty"`
}

type automationSnapshot struct {
	Rules           []automationRule    `json:"rules"`
	History         []automationHistory `json:"history,omitempty"`
	RunningRuleID   string              `json:"running_rule_id,omitempty"`
	RunningRuleIDs  []string            `json:"running_rule_ids,omitempty"`
	RunningRuleName string              `json:"running_rule_name,omitempty"`
	LastError       string              `json:"last_error,omitempty"`
	PendingRuleID   string              `json:"pending_rule_id,omitempty"`
	PendingRuleIDs  []string            `json:"pending_rule_ids,omitempty"`
	PendingReason   string              `json:"pending_reason,omitempty"`
	PendingSince    string              `json:"pending_since,omitempty"`
	TimeZone        string              `json:"time_zone"`
}

type automationManager struct {
	mu            sync.Mutex
	rules         []automationRule
	history       []automationHistory
	runningID     string
	runningName   string
	runningIDs    []string
	pendingID     string
	pendingIDs    []string
	pendingAt     time.Time
	pendingReason string
	lastError     string
	started       bool
	stopCh        chan struct{}
	wakeCh        chan struct{}
	wg            sync.WaitGroup
}

var automationHistorySeq atomic.Uint64

var automation = newAutomationManager()

func newAutomationManager() *automationManager {
	a := &automationManager{stopCh: make(chan struct{}), wakeCh: make(chan struct{}, 1)}
	if rules, err := loadAutomationRules(); err == nil {
		now := time.Now()
		changed := false
		for i := range rules {
			normalized, errValidate := validateAutomationRule(rules[i])
			if errValidate != nil {
				normalized.Enabled = false
				normalized.LastRunAt = rules[i].LastRunAt
				rules[i] = normalized
				if a.lastError == "" {
					a.lastError = "自动巡检规则已停用: " + firstNonEmpty(rules[i].Name, rules[i].ID) + ": " + errValidate.Error()
				}
				changed = true
			} else {
				normalized.LastRunAt = rules[i].LastRunAt
				rules[i] = normalized
			}
			if _, errParse := time.Parse(time.RFC3339, rules[i].LastRunAt); rules[i].LastRunAt == "" || errParse != nil {
				rules[i].LastRunAt = now.Format(time.RFC3339)
				changed = true
			}
		}
		a.rules = rules
		if changed {
			if errSave := saveAutomationRules(rules); errSave != nil {
				a.lastError = "初始化自动巡检时间失败: " + errSave.Error()
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		a.lastError = "加载自动巡检规则失败: " + err.Error()
	}
	if history, err := loadAutomationHistory(); err == nil {
		a.history = history
	} else if !errors.Is(err, os.ErrNotExist) {
		a.lastError = "加载自动巡检历史失败: " + err.Error()
	}
	return a
}

func (a *automationManager) start() {
	a.mu.Lock()
	if a.started {
		a.mu.Unlock()
		return
	}
	a.started = true
	a.mu.Unlock()
	a.wg.Add(1)
	go a.loop()
}

func (a *automationManager) shutdown() {
	a.mu.Lock()
	if !a.started {
		a.mu.Unlock()
		return
	}
	a.started = false
	close(a.stopCh)
	a.mu.Unlock()
	a.wg.Wait()
}

func (a *automationManager) signalWake() {
	if a == nil {
		return
	}
	select {
	case a.wakeCh <- struct{}{}:
	default:
	}
}

func (a *automationManager) loop() {
	defer a.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.runDue(time.Now())
		case <-a.wakeCh:
			a.runDue(time.Now())
		case <-a.stopCh:
			return
		}
	}
}

func (a *automationManager) snapshot(includeHistory bool) automationSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	rules := append([]automationRule(nil), a.rules...)
	now := time.Now()
	for i := range rules {
		rules[i].NextRunAt = nextRuleRunAt(rules[i], now)
		if containsString(a.pendingIDs, rules[i].ID) {
			rules[i].NextRunAt = ""
		}
	}
	pendingSince := ""
	if !a.pendingAt.IsZero() {
		pendingSince = a.pendingAt.Format(time.RFC3339)
	}
	snap := automationSnapshot{Rules: rules, RunningRuleID: a.runningID, RunningRuleIDs: append([]string(nil), a.runningIDs...), RunningRuleName: a.runningName, LastError: a.lastError, PendingRuleID: a.pendingID, PendingRuleIDs: append([]string(nil), a.pendingIDs...), PendingReason: a.pendingReason, PendingSince: pendingSince, TimeZone: now.Location().String()}
	if includeHistory {
		snap.History = append([]automationHistory(nil), a.history...)
	}
	return snap
}

func validateAutomationRule(rule automationRule) (automationRule, error) {
	rule.ID = strings.TrimSpace(rule.ID)
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" {
		return rule, fmt.Errorf("规则名称不能为空")
	}
	if rule.ID == "" {
		rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
	}
	if _, err := parseClock(rule.WindowStart); err != nil {
		return rule, fmt.Errorf("开始时间无效")
	}
	if _, err := parseClock(rule.WindowEnd); err != nil {
		return rule, fmt.Errorf("结束时间无效")
	}
	if rule.IntervalMinutes < 15 || rule.IntervalMinutes > 10080 {
		return rule, fmt.Errorf("执行间隔必须在 15-10080 分钟之间")
	}
	workers, err := normalizeWorkers(rule.Workers)
	if err != nil {
		return rule, err
	}
	rule.Workers = workers
	if len(rule.Weekdays) == 0 {
		rule.Weekdays = []int{1, 2, 3, 4, 5, 6, 7}
	}
	seenDays := map[int]struct{}{}
	for _, day := range rule.Weekdays {
		if day < 1 || day > 7 {
			return rule, fmt.Errorf("星期必须在 1-7 之间")
		}
		seenDays[day] = struct{}{}
	}
	rule.Weekdays = rule.Weekdays[:0]
	for day := 1; day <= 7; day++ {
		if _, ok := seenDays[day]; ok {
			rule.Weekdays = append(rule.Weekdays, day)
		}
	}
	allowed := map[string]struct{}{"all": {}, "permission_denied": {}, "quota_exhausted": {}, "other": {}}
	scope := normalizeClassifications(rule.Scope)
	if len(scope) == 0 {
		return rule, fmt.Errorf("至少选择一个巡检范围")
	}
	for _, value := range scope {
		if _, ok := allowed[value]; !ok {
			return rule, fmt.Errorf("自动巡检不支持分类 %s", value)
		}
	}
	rule.Scope = scope
	return rule, nil
}

func (a *automationManager) upsertRule(rule automationRule) (automationRule, error) {
	rule, err := validateAutomationRule(rule)
	if err != nil {
		return rule, err
	}
	a.mu.Lock()
	found := false
	for i := range a.rules {
		if a.rules[i].ID == rule.ID {
			rule.LastRunAt = a.rules[i].LastRunAt
			a.rules[i] = rule
			found = true
			break
		}
	}
	if !found {
		rule.LastRunAt = time.Now().Format(time.RFC3339)
		a.rules = append(a.rules, rule)
	}
	rules := append([]automationRule(nil), a.rules...)
	a.mu.Unlock()
	if err := saveAutomationRules(rules); err != nil {
		return rule, err
	}
	a.signalWake()
	return rule, nil
}

func (a *automationManager) deleteRule(id string) error {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	if containsString(a.runningIDs, id) {
		a.mu.Unlock()
		return fmt.Errorf("规则正在执行")
	}
	kept := make([]automationRule, 0, len(a.rules))
	for _, rule := range a.rules {
		if rule.ID != id {
			kept = append(kept, rule)
		}
	}
	if len(kept) == len(a.rules) {
		a.mu.Unlock()
		return fmt.Errorf("规则不存在")
	}
	a.rules = kept
	rules := append([]automationRule(nil), kept...)
	a.mu.Unlock()
	if err := saveAutomationRules(rules); err != nil {
		return err
	}
	a.signalWake()
	return nil
}

func (a *automationManager) runDue(now time.Time) {
	a.mu.Lock()
	if a.runningID != "" {
		a.mu.Unlock()
		return
	}
	pendingRules := a.rulesByIDsLocked(a.pendingIDs)
	pendingAt := a.pendingAt
	a.mu.Unlock()

	if len(pendingRules) > 0 {
		if pendingExpired(pendingRules, pendingAt, now) {
			a.resetPendingSchedule(pendingRules, now, "等待空闲已超过一个执行间隔，本次补跑已跳过")
			a.signalWake()
			return
		}
		engine.mu.Lock()
		busy := engine.running || engine.applying || engine.actionInFlight > 0 || engine.automatic
		engine.mu.Unlock()
		if busy {
			return
		}
		_ = a.runRules(pendingRules, false)
		return
	}
	if !pendingAt.IsZero() {
		a.mu.Lock()
		a.pendingID, a.pendingReason = "", ""
		a.pendingIDs = nil
		a.pendingAt = time.Time{}
		a.mu.Unlock()
	}

	a.mu.Lock()
	if a.runningID != "" {
		a.mu.Unlock()
		return
	}
	due := make([]automationRule, 0)
	changed := false
	for i := range a.rules {
		if ruleMissedTooLong(a.rules[i], now) {
			a.rules[i].LastRunAt = now.Format(time.RFC3339)
			if containsString(a.pendingIDs, a.rules[i].ID) {
				a.pendingID, a.pendingReason = "", ""
				a.pendingIDs = nil
				a.pendingAt = time.Time{}
			}
			changed = true
			continue
		}
		if ruleDue(a.rules[i], now) {
			due = append(due, a.rules[i])
		}
	}
	rules := append([]automationRule(nil), a.rules...)
	a.mu.Unlock()
	if changed {
		if err := saveAutomationRules(rules); err != nil {
			a.mu.Lock()
			a.lastError = "保存自动巡检补跑状态失败: " + err.Error()
			a.mu.Unlock()
		}
		a.signalWake()
	}
	if len(due) > 0 {
		_ = a.runRules(due, false)
	}
}

func (a *automationManager) rulesByIDsLocked(ids []string) []automationRule {
	if len(ids) == 0 {
		return nil
	}
	want := stringSet(ids)
	rules := make([]automationRule, 0, len(ids))
	for _, rule := range a.rules {
		if _, ok := want[rule.ID]; ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

func pendingExpired(rules []automationRule, pendingAt, now time.Time) bool {
	if pendingAt.IsZero() || len(rules) == 0 {
		return false
	}
	maxWait := time.Duration(rules[0].IntervalMinutes) * time.Minute
	for _, rule := range rules[1:] {
		wait := time.Duration(rule.IntervalMinutes) * time.Minute
		if wait < maxWait {
			maxWait = wait
		}
	}
	return now.Sub(pendingAt) > maxWait
}

func (a *automationManager) resetPendingSchedule(rules []automationRule, now time.Time, reason string) {
	want := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		want[rule.ID] = struct{}{}
	}
	a.mu.Lock()
	for i := range a.rules {
		if _, ok := want[a.rules[i].ID]; ok {
			a.rules[i].LastRunAt = now.Format(time.RFC3339)
		}
	}
	a.pendingID, a.pendingReason = "", ""
	a.pendingIDs = nil
	a.pendingAt = time.Time{}
	a.lastError = reason
	rulesCopy := append([]automationRule(nil), a.rules...)
	a.mu.Unlock()
	if err := saveAutomationRules(rulesCopy); err != nil {
		a.mu.Lock()
		a.lastError = "保存自动巡检补跑状态失败: " + err.Error()
		a.mu.Unlock()
	}
}

func (a *automationManager) runRule(id string, manual bool) error {
	a.mu.Lock()
	if a.runningID != "" {
		a.mu.Unlock()
		return fmt.Errorf("已有自动巡检正在执行")
	}
	var rule automationRule
	found := false
	for _, item := range a.rules {
		if item.ID == id {
			rule = item
			found = true
			break
		}
	}
	if !found {
		a.mu.Unlock()
		return fmt.Errorf("规则不存在")
	}
	if !manual && !rule.Enabled {
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()
	return a.runRules([]automationRule{rule}, manual)
}

func (a *automationManager) runRules(rules []automationRule, manual bool) error {
	if len(rules) == 0 {
		return nil
	}
	a.mu.Lock()
	if a.runningID != "" {
		a.mu.Unlock()
		return fmt.Errorf("已有自动巡检正在执行")
	}
	rule := mergeAutomationRules(rules)
	if !manual && !rule.Enabled {
		a.mu.Unlock()
		return nil
	}
	a.runningID, a.runningName = rule.ID, rule.Name
	a.runningIDs = append([]string(nil), rule.SourceRuleIDs...)
	if len(a.runningIDs) == 0 {
		a.runningIDs = []string{rule.ID}
	}
	a.mu.Unlock()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.executeRule(rule)
	}()
	return nil
}

func mergeAutomationRules(rules []automationRule) automationRule {
	merged := rules[0]
	merged.Weekdays = nil
	merged.WindowStart = ""
	merged.WindowEnd = ""
	merged.IntervalMinutes = 0
	merged.LastRunAt = ""
	merged.NextRunAt = ""
	merged.SourceRuleIDs = make([]string, 0, len(rules))
	seenScope := map[string]struct{}{}
	merged.Scope = nil
	names := make([]string, 0, len(rules))
	for _, rule := range rules {
		merged.SourceRuleIDs = append(merged.SourceRuleIDs, rule.ID)
		names = append(names, rule.Name)
		if rule.Workers > merged.Workers {
			merged.Workers = rule.Workers
		}
		merged.IncludeDisabled = merged.IncludeDisabled || rule.IncludeDisabled
		// Merged runs only auto-apply when every source rule permits it. This is
		// deliberately conservative: a no-apply rule must never inherit another
		// simultaneously due rule's mutation policy.
		merged.ApplyRecommendations = merged.ApplyRecommendations && rule.ApplyRecommendations
		for _, scope := range rule.Scope {
			seenScope[scope] = struct{}{}
		}
	}
	for scope := range seenScope {
		merged.Scope = append(merged.Scope, scope)
	}
	sort.Strings(merged.Scope)
	if len(names) > 1 {
		merged.Name = strings.Join(names, " + ")
	}
	return merged
}

func (a *automationManager) executeRule(rule automationRule) {
	started := time.Now()
	history := automationHistory{ID: fmt.Sprintf("run-%d", started.UnixNano()), RuleID: rule.ID, RuleName: rule.Name, Kind: "scheduled", StartedAt: started.Format(time.RFC3339), Status: "running"}
	engine.mu.Lock()
	if engine.running || engine.applying || engine.actionInFlight > 0 {
		engine.mu.Unlock()
		a.deferRule(rule, started, "当前有其他巡检或账号操作")
		return
	}
	results := append([]accountResult(nil), engine.results...)
	engine.mu.Unlock()
	want := stringSet(rule.Scope)
	ids := make([]string, 0)
	classes := map[string]struct{}{}
	now := time.Now()
	for _, item := range results {
		if item.Classification == "healthy" || item.Classification == "reauth" || item.AutoInspectExcluded {
			continue
		}
		matches := classificationMatches(item.Classification, want)
		if _, all := want["all"]; all {
			matches = true
		}
		if !matches || !automaticResultDue(item, now) {
			continue
		}
		id := firstNonEmpty(item.AuthIndex, item.FileName, item.Name)
		if id == "" {
			continue
		}
		ids = append(ids, id)
		if item.Classification == "permission_denied" || item.Classification == "quota_exhausted" {
			classes[item.Classification] = struct{}{}
		} else {
			classes["other"] = struct{}{}
		}
	}
	if len(ids) == 0 {
		history.Status = "success"
		history.FinishedAt = time.Now().Format(time.RFC3339)
		a.finishRule(rule, started, history, true)
		return
	}
	classifications := make([]string, 0, len(classes))
	for class := range classes {
		classifications = append(classifications, class)
	}
	sort.Strings(classifications)
	err := engine.start(startRequest{
		Workers: rule.Workers, IncludeDisabled: rule.IncludeDisabled,
		Classifications: classifications, AuthIndexes: ids,
		Automatic: true, AutomaticRuleID: rule.ID, AutomaticRuleName: rule.Name,
	})
	if err != nil {
		if strings.Contains(err.Error(), "running") || strings.Contains(err.Error(), "busy") {
			a.deferRule(rule, started, "当前有其他巡检或账号操作")
			return
		}
		history.Status, history.Error = "failed", err.Error()
		a.finishRule(rule, started, history, true)
		return
	}
	select {
	case <-engine.runCompletion():
	case <-a.stopCh:
		history.Status = "stopped"
		history.Message = "插件卸载，自动巡检等待结束"
		a.finishRule(rule, started, history, false)
		return
	}
	history.Inspected = len(ids)
	if rule.ApplyRecommendations && !engine.snapshot(false).Stopped {
		err = engine.startApply(applyRequest{AuthIndexes: ids, Actions: []string{"enable", "disable"}, Automatic: true}, resolveManagementPassword(nil), nil)
		if err == nil {
			select {
			case <-engine.applyCompletion():
			case <-a.stopCh:
				history.Status = "stopped"
				history.Message = "插件卸载，自动操作等待结束"
				a.finishRule(rule, started, history, false)
				return
			}
			snap := engine.snapshot(false)
			history.Applied = snap.ApplyDone
			if len(snap.ApplyFailures) > 0 {
				history.Error = strings.Join(snap.ApplyFailures, "; ")
			}
		} else if !strings.Contains(err.Error(), "no recommended actions") {
			history.Error = err.Error()
		}
	}
	history.Status = "success"
	if history.Error != "" {
		history.Status = "partial"
	}
	a.finishRule(rule, started, history, true)
}

func (a *automationManager) deferRule(rule automationRule, started time.Time, reason string) {
	a.mu.Lock()
	if a.pendingID != rule.ID {
		a.pendingAt = started
	}
	a.pendingID = rule.ID
	a.pendingIDs = append([]string(nil), rule.SourceRuleIDs...)
	if len(a.pendingIDs) == 0 {
		a.pendingIDs = []string{rule.ID}
	}
	a.pendingReason = reason + "，将在空闲后补跑"
	a.runningID, a.runningName = "", ""
	a.runningIDs = nil
	a.lastError = a.pendingReason
	a.mu.Unlock()
}

func (a *automationManager) finishRule(rule automationRule, started time.Time, history automationHistory, updateLastRun bool) {
	engine.clearAutomationContext(rule.ID)
	completionIDs := rule.SourceRuleIDs
	if len(completionIDs) == 0 {
		completionIDs = []string{rule.ID}
	}
	completed := stringSet(completionIDs)
	history.FinishedAt = time.Now().Format(time.RFC3339)
	a.mu.Lock()
	for i := range a.rules {
		if _, ok := completed[a.rules[i].ID]; updateLastRun && ok {
			a.rules[i].LastRunAt = started.Format(time.RFC3339)
		}
	}
	a.runningID, a.runningName = "", ""
	a.runningIDs = nil
	if _, ok := completed[a.pendingID]; ok {
		a.pendingID, a.pendingReason = "", ""
		a.pendingIDs = nil
		a.pendingAt = time.Time{}
	}
	a.lastError = history.Error
	a.history = append([]automationHistory{history}, a.history...)
	if len(a.history) > maxAutomationHistory {
		a.history = a.history[:maxAutomationHistory]
	}
	rules := append([]automationRule(nil), a.rules...)
	historyCopy := append([]automationHistory(nil), a.history...)
	a.mu.Unlock()
	var saveErrors []string
	if err := saveAutomationRules(rules); err != nil {
		saveErrors = append(saveErrors, "保存规则失败: "+err.Error())
	}
	if err := saveAutomationHistory(historyCopy); err != nil {
		saveErrors = append(saveErrors, "保存历史失败: "+err.Error())
	}
	if len(saveErrors) > 0 {
		a.mu.Lock()
		a.lastError = strings.Join(saveErrors, "; ")
		a.mu.Unlock()
	}
	a.signalWake()
}

func (a *automationManager) recordUsageEvent(result accountResult, actionAttempted bool, errText string) {
	now := time.Now()
	status := "detected"
	if actionAttempted && errText == "" {
		status = "applied"
	} else if actionAttempted {
		status = "failed"
	}
	label := automaticClassificationLabel(result.Classification)
	message := "检测到" + label
	if result.Classification == "reauth" {
		message += "，已跳过自动操作"
	} else if actionAttempted && errText == "" {
		message = label + "：自动禁用成功"
	} else if actionAttempted {
		message = label + "：自动禁用失败"
	} else if result.Action == "disable" {
		message += "，准备自动禁用"
	}
	h := automationHistory{ID: fmt.Sprintf("usage-%d-%d", now.UnixNano(), automationHistorySeq.Add(1)), Kind: "usage", StartedAt: now.Format(time.RFC3339), FinishedAt: now.Format(time.RFC3339), Status: status, Account: firstNonEmpty(result.Email, result.Name, result.AuthIndex), Classification: result.Classification, HTTPStatus: result.HTTPStatus, Error: errText, Message: message}
	a.mu.Lock()
	a.history = append([]automationHistory{h}, a.history...)
	if len(a.history) > maxAutomationHistory {
		a.history = a.history[:maxAutomationHistory]
	}
	history := append([]automationHistory(nil), a.history...)
	a.mu.Unlock()
	if err := saveAutomationHistory(history); err != nil {
		a.mu.Lock()
		a.lastError = "保存自动巡检历史失败: " + err.Error()
		a.mu.Unlock()
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func automaticClassificationLabel(classification string) string {
	switch classification {
	case "quota_exhausted":
		return "额度用尽"
	case "permission_denied":
		return "权限被拒"
	case "reauth":
		return "需重登"
	default:
		return firstNonEmpty(classification, "账号异常")
	}
}

func parseClock(value string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}

func ruleWindowContains(rule automationRule, now time.Time) bool {
	day := int(now.Weekday())
	if day == 0 {
		day = 7
	}
	foundDay := false
	for _, value := range rule.Weekdays {
		if value == day {
			foundDay = true
			break
		}
	}
	if !foundDay {
		return false
	}
	start, errStart := parseClock(rule.WindowStart)
	end, errEnd := parseClock(rule.WindowEnd)
	if errStart != nil || errEnd != nil {
		return false
	}
	minute := now.Hour()*60 + now.Minute()
	if start == end {
		return true
	}
	if start < end {
		return minute >= start && minute <= end
	}
	return minute >= start || minute <= end
}

func ruleDue(rule automationRule, now time.Time) bool {
	if !rule.Enabled || !ruleWindowContains(rule, now) {
		return false
	}
	if rule.LastRunAt == "" {
		return false
	}
	last, err := time.Parse(time.RFC3339, rule.LastRunAt)
	return err == nil && now.Sub(last) >= time.Duration(rule.IntervalMinutes)*time.Minute
}

func ruleMissedTooLong(rule automationRule, now time.Time) bool {
	if !rule.Enabled || !ruleWindowContains(rule, now) || rule.LastRunAt == "" {
		return false
	}
	last, err := time.Parse(time.RFC3339, rule.LastRunAt)
	if err != nil {
		return true
	}
	return now.Sub(last) > 2*time.Duration(rule.IntervalMinutes)*time.Minute
}

func nextRuleRunAt(rule automationRule, now time.Time) string {
	if !rule.Enabled {
		return ""
	}
	last, err := time.Parse(time.RFC3339, rule.LastRunAt)
	if err != nil {
		return ""
	}
	dueAt := last.Add(time.Duration(rule.IntervalMinutes) * time.Minute)
	if dueAt.Before(now) {
		dueAt = now
	}
	if ruleWindowContains(rule, dueAt) {
		return dueAt.Format(time.RFC3339)
	}
	// Intervals may end outside the configured window or on a disabled weekday.
	// Search minute boundaries for the next valid slot; two weeks covers the
	// maximum seven-day interval plus a full weekday cycle.
	candidate := dueAt.Truncate(time.Minute).Add(time.Minute)
	limit := candidate.Add(14 * 24 * time.Hour)
	for !candidate.After(limit) {
		if ruleWindowContains(rule, candidate) {
			return candidate.Format(time.RFC3339)
		}
		candidate = candidate.Add(time.Minute)
	}
	return ""
}

func automaticResultDue(item accountResult, now time.Time) bool {
	if item.NextInspectAt == "" {
		return true
	}
	next, err := time.Parse(time.RFC3339, item.NextInspectAt)
	return err != nil || !next.After(now)
}
