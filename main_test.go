package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditCurrentFixture(t *testing.T) {
	r, err := audit(4739, Feed{Batch: Batch{Offset: 400, Limit: 280}}, nil, 280)
	if err != nil || r.Cursor != 680 || r.Remaining != 4059 || r.Batches != 15 || r.Next.Offset != 680 {
		t.Fatalf("report=%+v error=%v", r, err)
	}
	if !strings.Contains(r.Warning, "do not prove") {
		t.Fatal("missing coverage caveat")
	}
}
func TestFinalBatchClampsCursor(t *testing.T) {
	r, err := audit(4739, Feed{Batch: Batch{Offset: 4600, Limit: 280}}, nil, 280)
	if err != nil || r.Remaining != 0 || r.Cursor != 4739 || r.Next != nil || r.Batches != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestRejectInvalidRanges(t *testing.T) {
	for _, b := range []Batch{{Offset: -1, Limit: 1}, {Limit: 0}, {Offset: 4739, Limit: 280}, {Offset: 1, Limit: int(^uint(0) >> 1)}} {
		if _, err := audit(4739, Feed{Batch: b}, nil, 280); err == nil {
			t.Errorf("accepted %+v", b)
		}
	}
	if _, err := audit(4739, Feed{Batch: Batch{Limit: 280}, Checkpoint: true}, nil, 280); err == nil {
		t.Fatal("accepted scratch as published")
	}
	if _, err := audit(4739, Feed{Batch: Batch{Limit: 280}}, nil, 0); err == nil {
		t.Fatal("accepted zero batch")
	}
}
func TestCheckpointConsistentAndRejectMismatch(t *testing.T) {
	f := Feed{Batch: Batch{Offset: 400, Limit: 280}}
	for _, done := range []int{1, 280, 281} {
		if _, err := audit(4739, f, &Feed{Checkpoint: true, Batch: Batch{Offset: 680, Limit: 280, Done: done}}, 280); err != nil {
			t.Fatal(err)
		}
	}
	for _, cp := range []Feed{{Batch: Batch{Offset: 680, Limit: 280, Done: 1}}, {Checkpoint: true, Batch: Batch{Offset: 400, Limit: 280, Done: 1}}, {Checkpoint: true, Batch: Batch{Offset: 680, Limit: 500, Done: 1}}, {Checkpoint: true, Batch: Batch{Offset: 680, Limit: 280, Done: 282}}} {
		if _, err := audit(4739, f, &cp, 280); err == nil {
			t.Errorf("accepted %+v", cp)
		}
	}
}
func TestDecodeDeduplicateAndInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parcels.json")
	for _, tc := range []struct {
		body  string
		total int
		bad   bool
	}{
		{`{"features":[{"properties":{"gush":1,"helka":2}},{"properties":{"gush":"1","helka":"2"}},{"properties":{"gush":null,"helka":2}}]}`, 1, false},
		{`{"features":[]}`, 0, true},
		{`{"features":[{"properties":{"gush":1.5,"helka":2}}]}`, 0, true},
	} {
		os.WriteFile(path, []byte(tc.body), 0600)
		var p Parcels
		err := decode(path, &p)
		n := 0
		if err == nil {
			n, err = totalParcels(p)
		}
		if (err != nil) != tc.bad || n != tc.total {
			t.Fatalf("%s: n=%d err=%v", tc.body, n, err)
		}
	}
	os.WriteFile(path, []byte(`{} {}`), 0600)
	var p Parcels
	if err := decode(path, &p); err == nil {
		t.Fatal("accepted trailing object")
	}
}
func TestCLIJSONTextAndErrors(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.json")
	f := filepath.Join(dir, "f.json")
	os.WriteFile(p, []byte(`{"features":[{"properties":{"gush":1,"helka":1}},{"properties":{"gush":1,"helka":2}}]}`), 0600)
	os.WriteFile(f, []byte(`{"batch":{"offset":0,"limit":1}}`), 0600)
	args := []string{"-parcels", p, "-published", f, "-batch-size", "1"}
	var out bytes.Buffer
	if err := run(append(args, "-json"), &out); err != nil {
		t.Fatal(err)
	}
	var r Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.Remaining != 1 {
		t.Fatalf("%s %v", out.String(), err)
	}
	out.Reset()
	if err := run(args, &out); err != nil || !strings.Contains(out.String(), "offset=1 limit=1") {
		t.Fatal(out.String(), err)
	}
	for _, args := range [][]string{{"-unknown"}, {"extra"}, {"-parcels", filepath.Join(dir, "missing")}} {
		if err := run(args, &out); err == nil {
			t.Fatal("expected CLI error")
		}
	}
}
func BenchmarkAudit(b *testing.B) {
	for i := 0; i < b.N; i++ {
		audit(4739, Feed{Batch: Batch{Offset: 400, Limit: 280}}, nil, 280)
	}
}
func TestCLIBundleValidationBeforeWriteAndSnapshotInsights(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.json")
	f := filepath.Join(dir, "f.json")
	plans := filepath.Join(dir, "plans.json")
	old := filepath.Join(dir, "old.json")
	clean := filepath.Join(dir, "clean.json")
	os.WriteFile(p, []byte(`{"features":[{"properties":{"gush":1,"helka":1}},{"properties":{"gush":1,"helka":2}}]}`), 0600)
	os.WriteFile(f, []byte(`{"batch":{"offset":0,"limit":1},"requests":[{"tik_num":"A"}]}`), 0600)
	os.WriteFile(old, []byte(`{"batch":{"offset":0,"limit":1},"requests":[{"tik_num":"B"}]}`), 0600)
	os.WriteFile(plans, []byte(`{"rows":[{"pl_number":"P","station_desc":"הפקדה","objection_deadline_derived_60d_from_publication":"2026-11-05"}]}`), 0600)
	args := []string{"-parcels", p, "-published", f, "-clean-out", clean, "-plans", plans, "-previous", old}
	var out bytes.Buffer
	if run(append(args, "-as-of", "bad"), &out) == nil {
		t.Fatal("invalid as-of accepted")
	}
	if _, err := os.Stat(clean); !os.IsNotExist(err) {
		t.Fatal("wrote output before input validation")
	}
	if err := run(append(args, "-as-of", "2026-10-22"), &out); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Changes  Changes  `json:"changes"`
		Insights Insights `json:"insights"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Changes.Added) != 1 || result.Changes.Added[0] != "A" || len(result.Insights.Near) != 1 || result.Insights.Near[0].Days != 14 {
		t.Fatalf("%+v", result)
	}
	if run(append(args, "-as-of", "2026-10-22"), &out) == nil {
		t.Fatal("overwrote clean artifact")
	}
	os.Remove(clean)
	os.WriteFile(plans, []byte("bad"), 0600)
	if run(append(args, "-as-of", "2026-10-22"), &out) == nil {
		t.Fatal("invalid plans accepted")
	}
	if _, err := os.Stat(clean); !os.IsNotExist(err) {
		t.Fatal("output created on invalid plans")
	}
}

// testMainHook, when set by a test file's init (only in a re-executed child,
// see normalize_fsize_linux_test.go), runs in TestMain instead of the tests
// and exits the process itself.
var testMainHook func()

// TestMain lets the exit-status tests re-run this test binary as
// coverage-audit: with COVERAGE_AUDIT_TEST_ARGS set, it runs main() with those
// arguments (newline-separated) instead of the tests, so main()'s os.Exit(1)
// path is observed through the child's exit status and stderr. Same pattern
// as tools/buildman (#61).
func TestMain(m *testing.M) {
	if testMainHook != nil {
		testMainHook()
	}
	if args, ok := os.LookupEnv("COVERAGE_AUDIT_TEST_ARGS"); ok {
		os.Args = []string{"coverage-audit"}
		if args != "" {
			os.Args = append(os.Args, strings.Split(args, "\n")...)
		}
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runMainExit re-executes the test binary as coverage-audit with args and
// returns its stdout, stderr and exit code. Child coverage is not merged into
// the parent's -coverprofile; the in-process test below covers the call line.
func runMainExit(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "COVERAGE_AUDIT_TEST_ARGS="+strings.Join(args, "\n"))
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

// auditFixture writes the synthetic two-parcel inputs used by
// TestCLIJSONTextAndErrors and returns the CLI arguments that read them.
func auditFixture(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "p.json")
	f := filepath.Join(dir, "f.json")
	if err := os.WriteFile(p, []byte(`{"features":[{"properties":{"gush":1,"helka":1}},{"properties":{"gush":1,"helka":2}}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, []byte(`{"batch":{"offset":0,"limit":1}}`), 0600); err != nil {
		t.Fatal(err)
	}
	return []string{"-parcels", p, "-published", f, "-batch-size", "1"}
}

// TestMainInProcessSuccess runs main() in this process on a successful
// invocation (main returns without os.Exit), so the call path counts in the
// coverage profile, and checks that it passes os.Args[1:] to run and writes
// the report to os.Stdout.
func TestMainInProcessSuccess(t *testing.T) {
	args := auditFixture(t)
	outPath := filepath.Join(t.TempDir(), "stdout")
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	oldArgs, oldStdout := os.Args, os.Stdout
	os.Args = append([]string{"coverage-audit"}, append(args, "-json")...)
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
	var r Report
	if err := json.Unmarshal(body, &r); err != nil || r.Total != 2 || r.Remaining != 1 || r.Next == nil || r.Next.Offset != 1 {
		t.Fatalf("stdout %q: report %+v, error %v", body, r, err)
	}
}

// TestMainExitStatus checks main()'s process contract in a child process:
// exit 0 with the report on stdout on success, and exit 1 with a
// "coverage-audit: <error>" line on stderr (and nothing on stdout) on failure.
func TestMainExitStatus(t *testing.T) {
	args := auditFixture(t)
	missing := filepath.Join(t.TempDir(), "missing.json")
	for _, c := range []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{"success", args, 0, "Next range: offset=1 limit=1\n", ""},
		{"unexpected positional", append(append([]string{}, args...), "extra"), 1, "", "coverage-audit: unexpected positional arguments\n"},
		{"unknown flag", []string{"-no-such-flag"}, 1, "", "coverage-audit: flag provided but not defined: -no-such-flag\n"},
		{"missing parcels file", []string{"-parcels", missing}, 1, "", "coverage-audit: parcels: "},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errOut, code := runMainExit(t, c.args...)
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
			if !strings.Contains(errOut, c.wantErr) {
				t.Errorf("stderr %q does not contain %q", errOut, c.wantErr)
			}
		})
	}
}
