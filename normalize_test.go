package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeDeduplicatesSortsDatesAndStripsPrivate(t *testing.T) {
	raw := []byte(`{"requests":[{"tik_num":" B ","status_date":"2/1/2026","private_person":"SYNTHETIC"},{"tik_num":"A"},{"tik_num":"A"}]}`)
	d, err := normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Requests) != 2 || d.Requests[0].Tik != "A" || d.Requests[1].Date != "2026-01-02" {
		t.Fatalf("%+v", d)
	}
	path := filepath.Join(t.TempDir(), "clean.json")
	if err := writeClean(path, d); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "private_person") || strings.Contains(string(body), "SYNTHETIC") {
		t.Fatal("private field leaked")
	}
	if err := writeClean(path, d); err == nil {
		t.Fatal("overwrote source/output")
	}
}
func TestNormalizeRejectsConflictInvalidDatesAndMissingID(t *testing.T) {
	for _, raw := range []string{`{}`, `{"requests":[{}]}`, `{"requests":[{"tik_num":"A","status_date":"2026-02-30"}]}`, `{"requests":[{"tik_num":"A","status":"x"},{"tik_num":"A","status":"y"}]}`} {
		if _, err := normalize([]byte(raw)); err == nil {
			t.Fatal("accepted malformed/conflicting data")
		}
	}
}
func TestSnapshotChangesDoNotClaimDeletionOrNewRealWorldEvents(t *testing.T) {
	before := Dataset{Batch: Batch{Limit: 1}, Requests: []Permit{{Tik: "A"}, {Tik: "B"}}}
	after := Dataset{Batch: Batch{Offset: 1, Limit: 1}, Requests: []Permit{{Tik: "A", Status: "new"}, {Tik: "C"}}}
	c := compare(before, after)
	if c.Comparable || len(c.Added) != 1 || c.Added[0] != "C" || c.Changed[0] != "A" || c.Absent[0] != "B" {
		t.Fatalf("%+v", c)
	}
	if !strings.Contains(c.Warning, "not proven") {
		t.Fatal("missing caveat")
	}
}

func TestWriteCleanFailedPublishLeavesNoFileAndNoStrayTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.json")

	// Inject a failure between "temp file written, synced and closed" and
	// "published to path": the temp file vanishes (simulating, e.g., a
	// competing cleanup or a filesystem remount) just before the publish
	// step, so the Link that would create path fails.
	old := afterTempWriteHook
	defer func() { afterTempWriteHook = old }()
	afterTempWriteHook = func(tmpPath string) {
		if err := os.Remove(tmpPath); err != nil {
			t.Fatalf("setup: could not remove temp file to inject failure: %v", err)
		}
	}

	err := writeClean(path, Dataset{Requests: []Permit{{Tik: "A"}}})
	if err == nil {
		t.Fatal("expected the publish step to fail")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("partial/complete file left at path after a failed write: %v", statErr)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".clean-") {
			t.Fatalf("stray temp file left behind: %s", e.Name())
		}
	}
}
