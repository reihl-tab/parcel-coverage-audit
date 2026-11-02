// Man pages and --help for every shipped CLI command (issue #45).
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reihlinstinct/parcel-coverage-audit/internal/clispec"
)

// TestHelpSmoke runs the built auditor with -h and requires every spec flag
// and its usage text to appear.
func TestHelpSmoke(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "coverage-audit")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, "-h")
	out, _ := cmd.CombinedOutput() // flag package exits 2 on -h; text is what matters
	help := string(out)
	for _, o := range clispec.AuditOptions {
		if !strings.Contains(help, "-"+o.Name) {
			t.Errorf("help missing flag -%s", o.Name)
		}
		if !strings.Contains(help, o.Usage) {
			t.Errorf("help missing usage text for -%s", o.Name)
		}
	}
}

// TestSupportUsageListsSpecSubcommands requires the support binary's usage
// error to name exactly the spec's subcommands.
func TestSupportUsageListsSpecSubcommands(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "support")
	if out, err := exec.Command("go", "build", "-o", bin, "./cmd/support").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	out, _ := exec.Command(bin).CombinedOutput()
	usage := string(out)
	for _, sub := range clispec.SupportSubcommands {
		if !strings.Contains(usage, sub) {
			t.Errorf("usage missing subcommand %q", sub)
		}
	}
}

// TestSupportHelpFlag requires coverage-audit-support -h/--help to print the
// subcommand list and exit 0 (review follow-up, #50).
func TestSupportHelpFlag(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "support")
	if out, err := exec.Command("go", "build", "-o", bin, "./cmd/support").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, arg := range []string{"-h", "--help", "help"} {
		cmd := exec.Command(bin, arg)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("%s: exit error %v\n%s", arg, err, out)
			continue
		}
		for _, sub := range clispec.SupportSubcommands {
			if !strings.Contains(string(out), sub) {
				t.Errorf("%s: output missing subcommand %q", arg, sub)
			}
		}
	}
}

// TestManPagesUpToDate regenerates the pages into a temp dir and diffs
// against the committed man/man1 pages (drift test).
func TestManPagesUpToDate(t *testing.T) {
	tmp := t.TempDir()
	if out, err := exec.Command("go", "run", "./tools/buildman", "-dir", tmp).CombinedOutput(); err != nil {
		t.Fatalf("buildman: %v\n%s", err, out)
	}
	for _, c := range clispec.Commands {
		name := clispec.Program + ".1"
		if c.Name != "" {
			name = clispec.Program + "-" + c.Name + ".1"
		}
		want, err := os.ReadFile(filepath.Join(tmp, name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join("man", "man1", name))
		if err != nil {
			t.Fatalf("committed page %s missing: %v", name, err)
		}
		if string(got) != string(want) {
			t.Errorf("man/man1/%s is stale; run: go run ./tools/buildman", name)
		}
	}
	// No committed page untracked by the spec.
	entries, err := filepath.Glob(filepath.Join("man", "man1", "*.1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(clispec.Commands) {
		t.Errorf("man/man1 has %d pages, spec covers %d commands", len(entries), len(clispec.Commands))
	}
}
