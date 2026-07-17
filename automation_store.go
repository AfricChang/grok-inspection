package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const automationStoreVersion = 1

type automationDiskState struct {
	Version int              `json:"version"`
	Rules   []automationRule `json:"rules"`
	SavedAt string           `json:"saved_at"`
}

func automationFilePath(name string) string {
	return filepath.Join(filepath.Dir(storeFilePath()), name)
}

func loadAutomationRules() ([]automationRule, error) {
	raw, err := os.ReadFile(automationFilePath("automation.json"))
	if err != nil {
		return nil, err
	}
	var state automationDiskState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return state.Rules, nil
}

func saveAutomationRules(rules []automationRule) error {
	state := automationDiskState{Version: automationStoreVersion, Rules: rules, SavedAt: time.Now().Format(time.RFC3339)}
	return saveAutomationJSON("automation.json", state)
}

func loadAutomationHistory() ([]automationHistory, error) {
	raw, err := os.ReadFile(automationFilePath("automation-history.json"))
	if err != nil {
		return nil, err
	}
	var history []automationHistory
	if err := json.Unmarshal(raw, &history); err != nil {
		return nil, err
	}
	return history, nil
}

func saveAutomationHistory(history []automationHistory) error {
	return saveAutomationJSON("automation-history.json", history)
}

func saveAutomationJSON(name string, value any) error {
	path := automationFilePath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
