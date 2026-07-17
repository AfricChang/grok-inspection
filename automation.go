package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
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
	RunningRuleName string              `json:"running_rule_name,omitempty"`
	LastError       string              `json:"last_error,omitempty"`
	TimeZone        string              `json:"time_zone"`
}

type automationManager struct {
	mu          sync.Mutex
	rules       []automationRule
	history     []automationHistory
	runningID   string
	runningName string
	lastError   string
	started     bool
	stopCh      chan struct{}
	wakeCh      chan struct{}
	wg          sync.WaitGroup
}

var automation = newAutomationManager()

func newAutomationManager() *automationManager {
	a := &automationManager{stopCh: make(chan struct{}), wakeCh: make(chan struct{}, 1)}
	if rules, err := loadAutomationRules(); err == nil {
		a.rules = rules
	}
	if history, err := loadAutomationHistory(); err == nil {
		a.history = history
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
	for i := range rules {
		rules[i].NextRunAt = nextRuleRunAt(rules[i], time.Now())
	}
	snap := automationSnapshot{Rules: rules, RunningRuleID: a.runningID, RunningRuleName: a.runningName, LastError: a.lastError, TimeZone: time.Now().Location().String()}
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
		a.rules = append(a.rules, rule)
	}
	rules := append([]automationRule(nil), a.rules...)
	a.mu.Unlock()
	if err := saveAutomationRules(rules); err != nil {
		return rule, err
	}
	return rule, nil
}

func (a *automationManager) deleteRule(id string) error {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	if a.runningID == id {
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
	return saveAutomationRules(rules)
}

func (a *automationManager) runDue(now time.Time) {
	a.mu.Lock()
	if a.runningID != "" {
		a.mu.Unlock()
		return
	}
	var due *automationRule
	for i := range a.rules {
		if ruleDue(a.rules[i], now) {
			copyRule := a.rules[i]
			due = &copyRule
			break
		}
	}
	a.mu.Unlock()
	if due != nil {
		_ = a.runRule(due.ID, false)
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
	a.runningID, a.runningName = rule.ID, rule.Name
	a.mu.Unlock()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.executeRule(rule)
	}()
	return nil
}

func (a *automationManager) executeRule(rule automationRule) {
	started := time.Now()
	history := automationHistory{ID: fmt.Sprintf("run-%d", started.UnixNano()), RuleID: rule.ID, RuleName: rule.Name, Kind: "scheduled", StartedAt: started.Format(time.RFC3339), Status: "running"}
	engine.mu.Lock()
	if engine.running || engine.applying || engine.actionInFlight > 0 {
		engine.mu.Unlock()
		history.Status = "skipped"
		history.Error = "当前有其他巡检或账号操作"
		a.finishRule(rule.ID, started, history, false)
		return
	}
	want := stringSet(rule.Scope)
	ids := make([]string, 0)
	classes := map[string]struct{}{}
	now := time.Now()
	for _, item := range engine.results {
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
	engine.mu.Unlock()
	if len(ids) == 0 {
		history.Status = "success"
		history.FinishedAt = time.Now().Format(time.RFC3339)
		a.finishRule(rule.ID, started, history, true)
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
		history.Status, history.Error = "failed", err.Error()
		a.finishRule(rule.ID, started, history, true)
		return
	}
	for engine.snapshot(false).Running {
		time.Sleep(200 * time.Millisecond)
	}
	history.Inspected = len(ids)
	if rule.ApplyRecommendations && !engine.snapshot(false).Stopped {
		err = engine.startApply(applyRequest{AuthIndexes: ids, Actions: []string{"enable", "disable"}, Automatic: true}, resolveManagementPassword(nil), nil)
		if err == nil {
			for engine.snapshot(false).Applying {
				time.Sleep(200 * time.Millisecond)
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
	a.finishRule(rule.ID, started, history, true)
}

func (a *automationManager) finishRule(ruleID string, started time.Time, history automationHistory, updateLastRun bool) {
	engine.clearAutomationContext(ruleID)
	history.FinishedAt = time.Now().Format(time.RFC3339)
	a.mu.Lock()
	for i := range a.rules {
		if updateLastRun && a.rules[i].ID == ruleID {
			a.rules[i].LastRunAt = started.Format(time.RFC3339)
		}
	}
	a.runningID, a.runningName = "", ""
	a.lastError = history.Error
	a.history = append([]automationHistory{history}, a.history...)
	if len(a.history) > maxAutomationHistory {
		a.history = a.history[:maxAutomationHistory]
	}
	rules := append([]automationRule(nil), a.rules...)
	historyCopy := append([]automationHistory(nil), a.history...)
	a.mu.Unlock()
	_ = saveAutomationRules(rules)
	_ = saveAutomationHistory(historyCopy)
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
	h := automationHistory{ID: fmt.Sprintf("usage-%d", now.UnixNano()), Kind: "usage", StartedAt: now.Format(time.RFC3339), FinishedAt: now.Format(time.RFC3339), Status: status, Account: firstNonEmpty(result.Email, result.Name, result.AuthIndex), Classification: result.Classification, HTTPStatus: result.HTTPStatus, Error: errText, Message: message}
	a.mu.Lock()
	a.history = append([]automationHistory{h}, a.history...)
	if len(a.history) > maxAutomationHistory {
		a.history = a.history[:maxAutomationHistory]
	}
	history := append([]automationHistory(nil), a.history...)
	a.mu.Unlock()
	_ = saveAutomationHistory(history)
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
		return true
	}
	last, err := time.Parse(time.RFC3339, rule.LastRunAt)
	return err != nil || now.Sub(last) >= time.Duration(rule.IntervalMinutes)*time.Minute
}

func nextRuleRunAt(rule automationRule, now time.Time) string {
	if !rule.Enabled {
		return ""
	}
	if ruleDue(rule, now) {
		return now.Format(time.RFC3339)
	}
	if last, err := time.Parse(time.RFC3339, rule.LastRunAt); err == nil {
		return last.Add(time.Duration(rule.IntervalMinutes) * time.Minute).Format(time.RFC3339)
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
