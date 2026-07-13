package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestNormalizeWorkers(t *testing.T) {
	got, err := normalizeWorkers(0)
	if err != nil || got != 6 {
		t.Fatalf("default workers = %d, %v", got, err)
	}
	got, err = normalizeWorkers(8)
	if err != nil || got != 8 {
		t.Fatalf("workers 8 = %d, %v", got, err)
	}
	if _, err := normalizeWorkers(17); err == nil {
		t.Fatal("expected error for workers=17")
	}
	if _, err := normalizeWorkers(-1); err == nil {
		t.Fatal("expected error for workers=-1")
	}
}

func TestPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")
	setStoreFilePathForTest(path)
	t.Cleanup(func() { setStoreFilePathForTest("") })

	snap := persistedSnapshot{
		Workers: 4,
		Results: []accountResult{
			{Name: "a@x.com", Classification: "reauth", Action: "delete"},
			{Name: "b@x.com", Classification: "healthy", Action: "keep"},
		},
	}
	if err := savePersistedSnapshot(snap); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadPersistedSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Workers != 4 || len(loaded.Results) != 2 {
		t.Fatalf("loaded = %+v", loaded)
	}
	if loaded.Results[0].Classification != "reauth" {
		t.Fatalf("classification = %s", loaded.Results[0].Classification)
	}
}

func TestCollectCandidatesFilters(t *testing.T) {
	e := &inspectionEngine{
		results: []accountResult{
			{Name: "a", AuthIndex: "1", Classification: "reauth", Action: "delete"},
			{Name: "b", AuthIndex: "2", Classification: "permission_denied", Action: "disable"},
			{Name: "c", AuthIndex: "3", Classification: "healthy", Action: "enable"},
			{Name: "d", AuthIndex: "4", Classification: "healthy", Action: "keep"},
		},
	}
	got, err := e.collectCandidates(applyRequest{
		Actions:         []string{"delete"},
		Classifications: []string{"reauth"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("got = %+v", got)
	}
	got, err = e.collectCandidates(applyRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("all recommended = %d", len(got))
	}
}

func TestFilterNewAuthEntriesIncremental(t *testing.T) {
	known := knownResultKeys([]accountResult{
		{AuthIndex: "old-1", FileName: "old-a.json", Name: "a@x.com"},
		// No auth_index: skip only by file fingerprint (name+size+mtime), never email alone.
		{FileName: "old-b.json", FileSize: 10, FileModUnix: 100},
	})
	files := []pluginapi.HostAuthFileEntry{
		{Provider: "xai", AuthIndex: "old-1", Name: "old-a.json", Email: "a@x.com"}, // known by auth_index
		{Provider: "xai", AuthIndex: "new-2", Name: "new-c.json", Email: "c@x.com"}, // new file
		// Same file name + fingerprint as known, but NEW auth_index → re-inspect (re-import).
		{Provider: "xai", AuthIndex: "new-3", Name: "old-b.json", Email: "b@x.com", Size: 10, ModTime: time.Unix(100, 0)},
		// No auth_index, same fingerprint → still known / skip
		{Provider: "xai", Name: "old-b.json", Size: 10, ModTime: time.Unix(100, 0)},
		{Provider: "openai", AuthIndex: "other", Name: "skip.json"}, // non-xai skipped
	}
	got := filterNewAuthEntries(files, known, false, false)
	if len(got) != 2 {
		t.Fatalf("incremental targets len=%d got=%+v", len(got), got)
	}
	gotIdx := map[string]bool{}
	for _, f := range got {
		gotIdx[f.AuthIndex] = true
	}
	if !gotIdx["new-2"] || !gotIdx["new-3"] {
		t.Fatalf("want new-2 and new-3, got %+v", got)
	}
}

func TestCollectCandidatesForceActionByIndexes(t *testing.T) {
	e := &inspectionEngine{
		results: []accountResult{
			{Name: "a", AuthIndex: "1", FileName: "a.json", Classification: "healthy", Action: "keep"},
			{Name: "b", AuthIndex: "2", FileName: "b.json", Classification: "permission_denied", Action: "disable"},
			{Name: "c", AuthIndex: "3", FileName: "c.json", Classification: "reauth", Action: "delete"},
		},
	}
	got, err := e.collectCandidates(applyRequest{
		ForceAction: "delete",
		AuthIndexes: []string{"1", "2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	for _, item := range got {
		if item.Action != "delete" {
			t.Fatalf("force action not applied: %+v", item)
		}
	}
	// force without selection is rejected
	if _, err := e.collectCandidates(applyRequest{ForceAction: "disable"}); err == nil {
		t.Fatal("expected error when force_action has no selection")
	}
}
