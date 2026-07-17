package main

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

var usageActionState = struct {
	sync.Mutex
	inFlight map[string]struct{}
}{inFlight: map[string]struct{}{}}

func handleUsage(raw []byte) ([]byte, error) {
	var record pluginapi.UsageRecord
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, err
		}
	}
	processUsageRecord(record)
	return okEnvelope(map[string]any{"accepted": true})
}

func processUsageRecord(record pluginapi.UsageRecord) {
	if !record.Failed || !isXAIEntry(record.Provider, record.AuthID, record.AuthType) {
		return
	}
	key := firstNonEmpty(record.AuthIndex, record.AuthID)
	if key == "" {
		return
	}
	errInfo := extractError(record.Failure.Body)
	classified := classifyProbe(classifyInput{
		ChatStatus: record.Failure.StatusCode,
		ChatCode:   errInfo.Code,
		ChatError:  firstNonEmpty(errInfo.Message, record.Failure.Body),
	})
	if classified.Classification != "quota_exhausted" && classified.Classification != "permission_denied" && classified.Classification != "reauth" {
		return
	}
	now := time.Now()
	result := accountResult{
		AuthIndex:       key,
		Name:            firstNonEmpty(record.AuthID, key),
		Classification:  classified.Classification,
		Action:          classified.Action,
		Reason:          classified.Reason,
		HTTPStatus:      record.Failure.StatusCode,
		Model:           record.Model,
		ErrorCode:       errInfo.Code,
		ErrorMessage:    truncateErrMessage(firstNonEmpty(errInfo.Message, record.Failure.Body), 400),
		LastInspectedAt: now.Format(time.RFC3339),
		NextInspectAt:   nextAutomaticInspectAt(classified.Classification, now),
	}
	if classified.Classification == "reauth" {
		result.Action = "keep"
		result.AutoInspectExcluded = true
		result.AutoInspectExcludeReason = "需重登"
	}

	engine.mu.Lock()
	if idx := findResultIndex(engine.results, result); idx >= 0 {
		old := engine.results[idx]
		result.Name = firstNonEmpty(old.Name, result.Name)
		result.FileName = old.FileName
		result.Email = old.Email
		result.FileID = old.FileID
		result.FileModUnix = old.FileModUnix
		result.FileSize = old.FileSize
		result.Disabled = old.Disabled
		if result.Disabled && result.Action == "disable" {
			result.Action = "keep"
		}
		engine.results[idx] = result
	} else {
		engine.results = append(engine.results, result)
	}
	engine.bumpResultsLocked()
	engine.persistLocked()
	engine.mu.Unlock()

	automation.recordUsageEvent(result, false, "")
	if result.Action != "disable" || result.Disabled {
		return
	}
	usageActionState.Lock()
	if _, exists := usageActionState.inFlight[key]; exists {
		usageActionState.Unlock()
		return
	}
	usageActionState.inFlight[key] = struct{}{}
	usageActionState.Unlock()

	engine.runWG.Add(1)
	go func() {
		defer engine.runWG.Done()
		defer func() {
			usageActionState.Lock()
			delete(usageActionState.inFlight, key)
			usageActionState.Unlock()
		}()
		err := setAuthDisabled(key, true, resolveManagementPassword(nil), nil, true)
		automation.recordUsageEvent(result, true, errorText(err))
	}()
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func nextAutomaticInspectAt(classification string, now time.Time) string {
	var wait time.Duration
	switch classification {
	case "permission_denied", "quota_exhausted":
		wait = 2 * time.Hour
	case "reauth":
		return ""
	default:
		wait = 30 * time.Minute
	}
	return now.Add(wait).Format(time.RFC3339)
}
