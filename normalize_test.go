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

// assertNoStrayTemp fails if any writeFileAtomic temp file is left in dir.
func assertNoStrayTemp(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".clean-") {
			t.Errorf("stray temp file left behind: %s", e.Name())
		}
	}
}

func TestWriteFileAtomicSuccessAndFailurePaths(t *testing.T) {
	body := []byte("synthetic body\n")
	for _, c := range []struct {
		name  string
		mode  os.FileMode
		setup func(t *testing.T, dir string) string // returns the target path
		check func(t *testing.T, path string)       // state of path after the call
		ok    bool
	}{
		{
			name:  "success 0600",
			mode:  0600,
			setup: func(t *testing.T, dir string) string { return filepath.Join(dir, "out.json") },
			ok:    true,
		},
		{
			name:  "success 0640",
			mode:  0640,
			setup: func(t *testing.T, dir string) string { return filepath.Join(dir, "out.json") },
			ok:    true,
		},
		{
			name: "target is an existing directory",
			mode: 0600,
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "out.json")
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				return path
			},
			check: func(t *testing.T, path string) {
				if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
					t.Errorf("existing directory not preserved: %v", err)
				}
			},
		},
		{
			name: "target is an existing file",
			mode: 0600,
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "out.json")
				if err := os.WriteFile(path, []byte("previous\n"), 0600); err != nil {
					t.Fatal(err)
				}
				return path
			},
			check: func(t *testing.T, path string) {
				if got, err := os.ReadFile(path); err != nil || string(got) != "previous\n" {
					t.Errorf("existing file changed: %q, %v", got, err)
				}
			},
		},
		{
			name: "parent directory missing",
			mode: 0600,
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "no-such-dir", "out.json")
			},
		},
		{
			name: "directory not writable",
			mode: 0600,
			setup: func(t *testing.T, dir string) string {
				if os.Geteuid() == 0 {
					t.Skip("running as root: directory permissions are not enforced")
				}
				sub := filepath.Join(dir, "ro")
				if err := os.Mkdir(sub, 0500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(sub, 0700) })
				return filepath.Join(sub, "out.json")
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := c.setup(t, t.TempDir())
			err := writeFileAtomic(path, body, c.mode)
			if c.ok {
				if err != nil {
					t.Fatal(err)
				}
				got, rerr := os.ReadFile(path)
				if rerr != nil || string(got) != string(body) {
					t.Fatalf("content %q, %v; want %q", got, rerr, body)
				}
				fi, serr := os.Stat(path)
				if serr != nil || fi.Mode().Perm() != c.mode {
					t.Fatalf("mode %v, %v; want %v", fi.Mode().Perm(), serr, c.mode)
				}
			} else if err == nil {
				t.Fatal("writeFileAtomic succeeded, want an error")
			} else if c.check != nil {
				c.check(t, path)
			} else if _, serr := os.Lstat(path); !os.IsNotExist(serr) {
				t.Errorf("target exists after a failed write: %v", serr)
			}
			if dir := filepath.Dir(path); dirExists(dir) {
				assertNoStrayTemp(t, dir)
			}
		})
	}
}

func dirExists(dir string) bool {
	fi, err := os.Stat(dir)
	return err == nil && fi.IsDir()
}
