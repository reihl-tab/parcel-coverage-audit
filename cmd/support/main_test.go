package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceValidation(t *testing.T) {
	m := metadata{Conclusion: "success", Path: ".github/workflows/permit-feed.yml"}
	if err := validate(strings.Repeat("a", 40), "123", m); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		s, r string
		m    metadata
	}{{"main; bad", "123", m}, {strings.Repeat("a", 40), "bad", m}, {strings.Repeat("a", 40), "123", metadata{Conclusion: "failure"}}, {strings.Repeat("a", 40), "123", metadata{Conclusion: "success", Path: "ci.yml"}}} {
		if validate(c.s, c.r, c.m) == nil {
			t.Fatal("accepted invalid source")
		}
	}
}
func TestCallbackBoundedFixedDestinationPayload(t *testing.T) {
	p, err := callbackPayload([]byte(`{"remaining":2}`), []byte("public"), strings.Repeat("a", 40), "1", "2")
	if err != nil {
		t.Fatal(err)
	}
	if p["ref"] != "main" {
		t.Fatal(p)
	}
	var r map[string]any
	json.Unmarshal([]byte(p["inputs"].(map[string]string)["report"]), &r)
	if r["source_digest"] == "" || r["schema_version"] != float64(1) {
		t.Fatal(r)
	}
	oversized, _ := json.Marshal(map[string]string{"too_long": strings.Repeat("a", 6001)})
	if _, err := callbackPayload(oversized, nil, strings.Repeat("a", 40), "1", "2"); err == nil {
		t.Fatal("unbounded callback")
	}
}
func TestPrivacyRecognizesLeaksAndPrivateFiles(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "safe.txt"), []byte("public aggregate only"), 0600)
	if err := privacy(root); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"gh" + "p_" + strings.Repeat("a", 25), "owner" + "@" + "example.invalid", "+" + "12345678901", "-----BEGIN " + "PRIVATE KEY-----", "12" + "-345-67"} {
		os.WriteFile(filepath.Join(root, "safe.txt"), []byte(bad), 0600)
		if privacy(root) == nil {
			t.Fatal("missed private material")
		}
	}
	os.WriteFile(filepath.Join(root, "safe.txt"), []byte("safe"), 0600)
	os.WriteFile(filepath.Join(root, "raw.geojson"), []byte("{}"), 0600)
	if privacy(root) == nil {
		t.Fatal("private input accepted")
	}
}
func TestSummarySourceEscapeTotalsAndSkipCaveat(t *testing.T) {
	events := []byte("{\"Test\":\"TestA\",\"Action\":\"pass\"}\n{\"Test\":\"TestB\",\"Action\":\"skip\"}\n")
	text := render(events, "80.0%", map[string]string{"TestA": "<script>"})
	for _, want := range []string{"| 2 | 1 | 0 | 1 |", "&lt;script&gt;", "not captured", "checks were not executed", "80.0%"} {
		if !strings.Contains(text, want) {
			t.Fatal(want)
		}
	}
	if strings.Contains(text, "<script>") {
		t.Fatal("unescaped source")
	}
}
func TestNestedTestSourceDiscovery(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "cmd/x"), 0700)
	os.WriteFile(filepath.Join(root, "cmd/x/main_test.go"), []byte("func TestNested(t *testing.T) {\n}\n"), 0600)
	src, err := testSources(root)
	if err != nil || src["TestNested"] == "" {
		t.Fatal(src, err)
	}
}
func TestBadgeMetricsMeasuredAndFailureClosed(t *testing.T) {
	events := []byte("{\"Test\":\"TestA\",\"Action\":\"pass\"}\n")
	values, err := badgeMetrics(events, "total: (statements) 67.3%")
	if err != nil || values["coverage"]["message"] != "67.3%" || values["tests"]["message"] != "1 passed, 0 skipped" {
		t.Fatal(values, err)
	}
	if _, err := badgeMetrics([]byte("{\"Test\":\"TestA\",\"Action\":\"fail\"}\n"), "67.3%"); err == nil {
		t.Fatal("published failure as success")
	}
}

func sandbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(cwd) })
	return root
}
func stub(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
}
func TestFetchOfflineSuccessAndAncestryRejection(t *testing.T) {
	root := sandbox(t)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOURCE_SHA", strings.Repeat("a", 40))
	t.Setenv("SOURCE_RUN", "1")
	stub(t, root, "git", `printf '%s\n' "$*" >> git-log`)
	stub(t, root, "gh", `case "$2" in
 *actions/runs*) printf '%s' '{"status":"completed","conclusion":"success","path":".github/workflows/permit-feed.yml"}' ;;
 *compare*) printf '%s' '{"status":"ahead"}' ;;
 *) exit 2 ;;
 esac`)
	if err := fetch(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile("git-log")
	if !strings.Contains(string(b), "checkout "+strings.Repeat("a", 40)+" -- data/ginot_hair_parcels.geojson data/permit_requests.json docs/data.json") {
		t.Fatal(string(b))
	}
	stub(t, root, "gh", `case "$2" in
 *actions/runs*) printf '%s' '{"status":"completed","conclusion":"success","path":".github/workflows/permit-feed.yml"}' ;;
 *compare*) printf '%s' '{"status":"diverged"}' ;;
 esac`)
	if fetch() == nil {
		t.Fatal("accepted diverged SHA")
	}
	t.Setenv("SOURCE_SHA", "bad")
	if fetch() == nil {
		t.Fatal("accepted invalid SHA")
	}
}
func TestAPICommandErrorsJSONAndPayload(t *testing.T) {
	root := sandbox(t)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	stub(t, root, "gh", `cat > payload; printf '%s' '{"ok":true}'`)
	var out map[string]bool
	if err := api("test", map[string]string{"safe": "public"}, &out); err != nil || !out["ok"] {
		t.Fatal(out, err)
	}
	b, _ := os.ReadFile("payload")
	if string(b) != `{"safe":"public"}` {
		t.Fatal(string(b))
	}
	if api("test", make(chan int), nil) == nil {
		t.Fatal("unsupported payload accepted")
	}
	stub(t, root, "gh", `printf '%s' 'invalid json'`)
	if api("test", nil, &out) == nil {
		t.Fatal("invalid JSON accepted")
	}
	stub(t, root, "gh", `exit 2`)
	if api("test", nil, nil) == nil {
		t.Fatal("command error swallowed")
	}
}
func TestCallbackDispatchOfflineAndMissingSource(t *testing.T) {
	root := sandbox(t)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOURCE_SHA", strings.Repeat("a", 40))
	t.Setenv("SOURCE_RUN", "1")
	t.Setenv("GITHUB_RUN_ID", "2")
	if callback() == nil {
		t.Fatal("missing report accepted")
	}
	os.WriteFile("audit.json", []byte(`{"remaining":2}`), 0600)
	if callback() == nil {
		t.Fatal("missing source accepted")
	}
	os.MkdirAll("source/data", 0700)
	os.WriteFile("source/data/permit_requests.json", []byte("public"), 0600)
	stub(t, root, "gh", `printf '%s' "$*" > route; cat > payload`)
	if err := callback(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile("route")
	if !strings.Contains(string(b), "actions/workflows/audit-result.yml/dispatches") {
		t.Fatal(string(b))
	}
	b, _ = os.ReadFile("payload")
	if strings.Contains(string(b), "permit-feed.yml") {
		t.Fatal("callback loop")
	}
}
func TestSummaryActualOutputAndMissingCoverage(t *testing.T) {
	sandbox(t)
	t.Setenv("GITHUB_STEP_SUMMARY", "summary.md")
	os.WriteFile("test-events.json", []byte("{\"Test\":\"TestA\",\"Action\":\"fail\"}\n"), 0600)
	if err := summary(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile("summary.md")
	if !strings.Contains(string(b), "unavailable") || !strings.Contains(string(b), "FAIL") {
		t.Fatal(string(b))
	}
	os.WriteFile("coverage-total.txt", []byte("total 85.0%"), 0600)
	if err := summary(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_STEP_SUMMARY", "missing/summary.md")
	if summary() == nil {
		t.Fatal("summary output error hidden")
	}
}
func TestBadgeFilesAndCommandRouting(t *testing.T) {
	sandbox(t)
	for _, args := range [][]string{nil, {"unknown"}, {"fetch", "extra"}} {
		if run(args) == nil {
			t.Fatal("accepted bad CLI", args)
		}
	}
	if run([]string{"badges"}) == nil {
		t.Fatal("missing events accepted")
	}
	os.WriteFile("test-events.json", []byte("{\"Test\":\"TestA\",\"Action\":\"pass\"}\n"), 0600)
	if badges() == nil {
		t.Fatal("missing coverage accepted")
	}
	os.WriteFile("coverage-total.txt", []byte("total 90.0%"), 0600)
	if err := run([]string{"badges"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("/tmp/badges/tests.json")
	if err != nil || !strings.Contains(string(b), "1 passed") {
		t.Fatal(string(b), err)
	}
	t.Setenv("GITHUB_STEP_SUMMARY", "summary.md")
	if err := run([]string{"summary"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"privacy"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOURCE_SHA", "bad")
	if run([]string{"fetch"}) == nil || run([]string{"callback"}) == nil {
		t.Fatal("validation did not stop dispatch")
	}
}
func TestHistoryAndBundleManifest(t *testing.T) {
	root := sandbox(t)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOURCE_SHA", strings.Repeat("a", 40))
	t.Setenv("SOURCE_RUN", "1")
	stub(t, root, "git", `case "$3" in
 log) printf '%s\n' aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb ;;
 show) printf '%s' '{"requests":[]}' ;;
 esac`)
	if err := history(strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile("previous.json")
	if !strings.Contains(string(b), "requests") {
		t.Fatal(string(b))
	}
	for _, n := range []string{"audit.json", "analysis.json", "clean.json"} {
		os.WriteFile(n, []byte("{}"), 0600)
	}
	if err := run([]string{"bundle"}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile("manifest.json")
	if !strings.Contains(string(b), "source_commit") || !strings.Contains(string(b), "sha256") {
		t.Fatal(string(b))
	}
	os.WriteFile("clean.json", []byte("bad"), 0600)
	if bundle() == nil {
		t.Fatal("bad artifact accepted")
	}
	stub(t, root, "git", `printf '%s\n' aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`)
	os.Remove("previous.json")
	if err := history(strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile("history.json")
	if !strings.Contains(string(b), `"available":false`) {
		t.Fatal(string(b))
	}
}
