package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reihlinstinct/parcel-coverage-audit/internal/clispec"
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
	stateless := "ghs" + "_12345_" + strings.Repeat("A", 100) + "." + strings.Repeat("B", 300) + "." + strings.Repeat("C", 108)
	longStateless := "ghs" + "_12345_" + strings.Repeat("A", 100) + "." + strings.Repeat("B", 300) + "." + strings.Repeat("C", 200)
	for _, bad := range []string{"gh" + "p_" + strings.Repeat("a", 25), stateless, longStateless, "owner" + "@" + "example.invalid", "+" + "12345678901", "-----BEGIN " + "PRIVATE KEY-----", "12" + "-345-67"} {
		os.WriteFile(filepath.Join(root, "safe.txt"), []byte(bad), 0600)
		if privacy(root) == nil {
			t.Fatal("missed private material")
		}
	}
	for _, safe := range []string{"ghs" + "_12345_" + strings.Repeat("A", 10), "gh" + "p_short"} {
		os.WriteFile(filepath.Join(root, "safe.txt"), []byte(safe), 0600)
		if err := privacy(root); err != nil {
			t.Fatalf("short token fragment flagged: %v", err)
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
	oldBadgeDir := badgeDir
	badgeDir = t.TempDir()
	t.Cleanup(func() { badgeDir = oldBadgeDir })
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
	b, err := os.ReadFile(badgeDir + "/tests.json")
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

// TestMain lets the exit-status tests re-run this test binary as the support
// command: with SUPPORT_TEST_ARGS set, it runs main() with those arguments
// (newline-separated) instead of the tests, so main()'s os.Exit(1) path is
// observed through the child's exit status and stderr. Same pattern as
// tools/buildman (#61).
func TestMain(m *testing.M) {
	if args, ok := os.LookupEnv("SUPPORT_TEST_ARGS"); ok {
		os.Args = []string{"support"}
		if args != "" {
			os.Args = append(os.Args, strings.Split(args, "\n")...)
		}
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runMainExit re-executes the test binary as the support command in dir and
// returns its stdout, stderr and exit code. Child coverage is not merged into
// the parent's -coverprofile; TestMainInProcessPrivacy covers the call line.
func runMainExit(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "SUPPORT_TEST_ARGS="+strings.Join(args, "\n"))
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return o.String(), e.String(), code
}

// TestMainInProcessPrivacy runs main() in this process on an offline command
// that succeeds (main returns without os.Exit), so the call path counts in
// the coverage profile, and checks that it routes os.Args[1:] to run.
func TestMainInProcessPrivacy(t *testing.T) {
	root := sandbox(t)
	if err := os.WriteFile(filepath.Join(root, "safe.txt"), []byte("public aggregate only"), 0600); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "stdout")
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	oldArgs, oldStdout := os.Args, os.Stdout
	os.Args = []string{"support", "privacy"}
	os.Stdout = outFile
	defer func() { os.Args, os.Stdout = oldArgs, oldStdout }()
	main()
	os.Stdout = oldStdout
	if err := outFile.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := "PASS: 1 source/config/doc files scanned; no detected private material.\n"; string(body) != want {
		t.Fatalf("stdout = %q, want %q", body, want)
	}
}

// TestMainExitStatus checks main()'s process contract in a child process:
// exit 0 on success, and exit 1 with the bare error on stderr on failure.
func TestMainExitStatus(t *testing.T) {
	clean := t.TempDir()
	if err := os.WriteFile(filepath.Join(clean, "safe.txt"), []byte("public aggregate only"), 0600); err != nil {
		t.Fatal(err)
	}
	leaky := t.TempDir()
	if err := os.WriteFile(filepath.Join(leaky, "raw.geojson"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name     string
		dir      string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{"privacy pass", clean, []string{"privacy"}, 0, "PASS: 1 source/config/doc files scanned", ""},
		{"no command", clean, nil, 1, "", "expected one of: " + strings.Join(clispec.SupportSubcommands, ", ") + "\n"},
		{"unknown command", clean, []string{"no-such-command"}, 1, "", "unknown support command\n"},
		{"privacy finding", leaky, []string{"privacy"}, 1, "", "raw.geojson: prohibited input/output data file\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errOut, code := runMainExit(t, c.dir, c.args...)
			if code != c.wantCode {
				t.Fatalf("exit %d, want %d (stderr %q)", code, c.wantCode, errOut)
			}
			if c.wantCode == 0 {
				if !strings.Contains(out, c.wantOut) || errOut != "" {
					t.Errorf("stdout %q (want it to contain %q), stderr %q (want empty)", out, c.wantOut, errOut)
				}
				return
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty on failure", out)
			}
			if errOut != c.wantErr {
				t.Errorf("stderr = %q, want %q", errOut, c.wantErr)
			}
		})
	}
}
