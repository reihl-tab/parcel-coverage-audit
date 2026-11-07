package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/reihlinstinct/parcel-coverage-audit/internal/clispec"
)

// TestMain lets the exit-status tests re-run this test binary as buildman:
// with BUILDMAN_TEST_ARGS set, it runs main() with those arguments instead of
// the tests. That covers the os.Exit(1) paths without a production seam.
func TestMain(m *testing.M) {
	if args, ok := os.LookupEnv("BUILDMAN_TEST_ARGS"); ok {
		os.Args = append([]string{"buildman"}, strings.Split(args, "\n")...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestEsc(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"plain words", "plain words"},
		{"as-of", `as\-of`},
		{"--json", `\-\-json`},
		{`a\b`, `a\eb`},
		{`\-`, `\e\-`}, // backslash escaped first, so the dash escape is not doubled
		{`x\-y-z`, `x\e\-y\-z`},
	} {
		if got := esc(c.in); got != c.want {
			t.Errorf("esc(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTextLine(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{".TH injected", `\&.TH injected`},
		{"'quoted control", `\&'quoted control`},
		{".a-b", `\&.a\-b`},
		{"ends with a dot.", "ends with a dot."},
		{"mid 'quote' and .dot", "mid 'quote' and .dot"},
		{"x-y", `x\-y`},
		{"", ""},
	} {
		if got := textLine(c.in); got != c.want {
			t.Errorf("textLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPageNameAndTitle(t *testing.T) {
	for _, c := range []struct{ name, page, title string }{
		{"", "coverage-audit.1", "coverage-audit"},
		{"support", "coverage-audit-support.1", "coverage-audit-support"},
		{"x", "coverage-audit-x.1", "coverage-audit-x"},
	} {
		cmd := clispec.Command{Name: c.name}
		if got := pageName(cmd); got != c.page {
			t.Errorf("pageName(%q) = %q, want %q", c.name, got, c.page)
		}
		if got := title(cmd); got != c.title {
			t.Errorf("title(%q) = %q, want %q", c.name, got, c.title)
		}
	}
}

// section returns the lines of a rendered page from the .SH line named sh up
// to (not including) the next .SH line; nil if the section is absent.
func section(page, sh string) []string {
	var out []string
	in := false
	for _, l := range strings.Split(strings.TrimSuffix(page, "\n"), "\n") {
		if strings.HasPrefix(l, ".SH ") {
			if in {
				break
			}
			in = l == ".SH "+sh
		}
		if in {
			out = append(out, l)
		}
	}
	return out
}

func TestRenderAuditorPage(t *testing.T) {
	page := render(clispec.Commands[0])
	lines := strings.Split(page, "\n")
	if lines[0] != header {
		t.Errorf("first line = %q, want the generated header", lines[0])
	}
	if want := `.TH "COVERAGE\\-AUDIT" "1" "November 2026" "coverage-audit 0.1.0" "Coverage Audit Manual"`; lines[1] != want {
		t.Errorf(".TH line = %q, want %q", lines[1], want)
	}
	if got := section(page, "NAME"); len(got) != 2 || got[1] != `coverage\-audit \- audit published permit\-request coverage against local parcels` {
		t.Errorf("NAME = %q", got)
	}
	syn := strings.Join(section(page, "SYNOPSIS"), "\n")
	for _, want := range []string{`.B coverage\-audit [\-parcels VALUE]`, `[\-batch\-size VALUE]`, `[\-json] [\-mcp]`} {
		if !strings.Contains(syn, want) {
			t.Errorf("SYNOPSIS missing %q:\n%s", want, syn)
		}
	}
	if strings.Contains(syn, "SUBCOMMAND") || section(page, "SUBCOMMANDS") != nil {
		t.Error("auditor page must not document subcommands")
	}
	opts := strings.Join(section(page, "OPTIONS"), "\n")
	for _, want := range []string{
		".TP\n.B \\-parcels VALUE\nlocal parcels GeoJSON (default: data/ginot_hair_parcels.geojson)\n",
		".TP\n.B \\-batch\\-size VALUE\nplanned next batch size (default: 280)\n",
		".TP\n.B \\-json\nemit the report as JSON (default: false)\n",
		".TP\n.B \\-plans VALUE\noptional public planning rows for observations\n.TP",
	} {
		if !strings.Contains(opts+"\n", want) {
			t.Errorf("OPTIONS missing %q", want)
		}
	}
	if n := strings.Count(opts, ".TP\n.B \\-"); n != len(clispec.AuditOptions) {
		t.Errorf("OPTIONS documents %d flags, spec has %d", n, len(clispec.AuditOptions))
	}
	ex := strings.Join(section(page, "EXAMPLES"), "\n")
	if !strings.Contains(ex, ".PP\ncoverage\\-audit \\-json\n.RS\nPrint the coverage report as JSON.\n.RE") {
		t.Errorf("EXAMPLES:\n%s", ex)
	}
	if got := strings.Join(section(page, "EXIT STATUS"), "\n"); got != ".SH EXIT STATUS\n.TP\n.B 0\nSuccess.\n.TP\n.B 1\nError (usage, input or audit failure)." {
		t.Errorf("EXIT STATUS = %q", got)
	}
	if got := section(page, "SEE ALSO"); strings.Join(got, "\n") != ".SH SEE ALSO\n.BR coverage\\-audit\\-support (1)\n.BR man (1)" {
		t.Errorf("SEE ALSO = %q", got)
	}
	if !strings.HasSuffix(page, ".BR man (1)\n") {
		t.Error("page does not end with the man(1) reference and a newline")
	}
}

func TestRenderSupportPage(t *testing.T) {
	page := render(clispec.Commands[1])
	if got := section(page, "SYNOPSIS"); len(got) != 2 || got[1] != `.B coverage\-audit\-support SUBCOMMAND` {
		t.Errorf("SYNOPSIS = %q", got)
	}
	var subs []string
	for _, l := range section(page, "SUBCOMMANDS") {
		if strings.HasPrefix(l, ".B ") {
			subs = append(subs, strings.TrimPrefix(l, ".B "))
		}
	}
	if strings.Join(subs, ",") != strings.Join(clispec.SupportSubcommands, ",") {
		t.Errorf("SUBCOMMANDS = %v, want %v", subs, clispec.SupportSubcommands)
	}
	if section(page, "OPTIONS") != nil {
		t.Error("support page has an OPTIONS section but no flags")
	}
	if got := section(page, "SEE ALSO"); strings.Join(got, "\n") != ".SH SEE ALSO\n.BR coverage\\-audit (1)\n.BR man (1)" {
		t.Errorf("SEE ALSO = %q", got)
	}
}

// TestRenderSyntheticCommand renders an invented command to reach the paths
// the shipped spec does not: control-character escaping in descriptions,
// options and examples, empty defaults, no examples, and a comma-separated
// SEE ALSO list.
func TestRenderSyntheticCommand(t *testing.T) {
	c := clispec.Command{
		Name:    "demo",
		Binary:  "coverage-audit-demo",
		Summary: "an invented page",
		Details: []string{".SH injected", "second paragraph"},
		Options: []clispec.Opt{
			{Name: "flag-on", Kind: clispec.Bool, Usage: "'starts with a quote"},
			{Name: "count", Kind: clispec.Int, Usage: "how many"},
		},
	}
	page := render(c)
	desc := strings.Join(section(page, "DESCRIPTION"), "\n")
	if desc != ".SH DESCRIPTION\n.PP\n\\&.SH injected\n.PP\nsecond paragraph" {
		t.Errorf("DESCRIPTION = %q", desc)
	}
	if strings.Contains(page, "\n.SH injected") {
		t.Error("a description line starting with a dot became a request")
	}
	if got := section(page, "SYNOPSIS"); len(got) != 2 || got[1] != `.B coverage\-audit\-demo [\-flag\-on] [\-count VALUE]` {
		t.Errorf("SYNOPSIS = %q", got)
	}
	if got := strings.Join(section(page, "OPTIONS"), "\n"); got != ".SH OPTIONS\n.TP\n.B \\-flag\\-on\n\\&'starts with a quote\n.TP\n.B \\-count VALUE\nhow many" {
		t.Errorf("OPTIONS = %q", got)
	}
	if strings.Contains(page, "(default:") {
		t.Error("empty defaults must not render a (default: ) suffix")
	}
	if section(page, "EXAMPLES") != nil {
		t.Error("EXAMPLES rendered with no examples")
	}
	if got := strings.Join(section(page, "SEE ALSO"), "\n"); got != ".SH SEE ALSO\n.BR coverage\\-audit (1),\n.BR coverage\\-audit\\-support (1)\n.BR man (1)" {
		t.Errorf("SEE ALSO = %q", got)
	}
}

func TestRenderAllMatchesCommittedPages(t *testing.T) {
	pages := renderAll()
	if len(pages) != len(clispec.Commands) {
		t.Fatalf("renderAll returned %d pages for %d commands", len(pages), len(clispec.Commands))
	}
	for _, c := range clispec.Commands {
		if pages[pageName(c)] != render(c) {
			t.Errorf("renderAll()[%s] differs from render", pageName(c))
		}
	}
	committed, err := filepath.Glob(filepath.Join("..", "..", "man", "man1", "*.1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(committed) != len(pages) {
		t.Errorf("man/man1 has %d pages, renderAll %d", len(committed), len(pages))
	}
	for _, path := range committed {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if pages[filepath.Base(path)] != string(b) {
			t.Errorf("man/man1/%s is stale; run: go run ./tools/buildman", filepath.Base(path))
		}
	}
}

// runMain calls main in-process with fresh flags and returns its stdout.
// Only for argument sets where main returns instead of exiting.
func runMain(t *testing.T, args ...string) string {
	t.Helper()
	oldArgs, oldFlags, oldStdout := os.Args, flag.CommandLine, os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan []byte)
	go func() { b, _ := io.ReadAll(r); done <- b }()
	os.Args = append([]string{"buildman"}, args...)
	flag.CommandLine = flag.NewFlagSet("buildman", flag.ContinueOnError)
	os.Stdout = w
	defer func() {
		os.Args, flag.CommandLine, os.Stdout = oldArgs, oldFlags, oldStdout
	}()
	main()
	w.Close()
	return string(<-done)
}

// runExit re-runs this test binary as buildman (see TestMain) for argument
// sets that end in os.Exit, and returns stdout, stderr and the exit code.
func runExit(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "BUILDMAN_TEST_ARGS="+strings.Join(args, "\n"))
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

func TestMainWritesPages(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "man1")
	out := runMain(t, "-dir", dir)
	if want := "wrote 2 man page(s) to " + dir + "\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	pages := renderAll()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(pages) {
		t.Errorf("wrote %d files, want %d", len(entries), len(pages))
	}
	for name, body := range pages {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != body {
			t.Errorf("%s content differs from render", name)
		}
	}
}

func TestMainCheckUpToDate(t *testing.T) {
	dir := t.TempDir()
	runMain(t, "-dir", dir)
	if out := runMain(t, "-check", "-dir", dir); out != "OK: 2 man page(s) up to date\n" {
		t.Errorf("stdout = %q", out)
	}
	// The committed pages pass the same check.
	if out := runMain(t, "-check", "-dir", filepath.Join("..", "..", "man", "man1")); out != "OK: 2 man page(s) up to date\n" {
		t.Errorf("committed pages: stdout = %q", out)
	}
}

func TestMainCheckStale(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(dir string) error
		want  string
	}{
		{"empty dir", func(string) error { return nil },
			"stale man pages: coverage-audit-support.1, coverage-audit.1\n"},
		{"edited page", func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "coverage-audit.1"), []byte("hand edit\n"), 0o644)
		}, "stale man pages: coverage-audit.1\n"},
		{"missing page", func(dir string) error {
			return os.Remove(filepath.Join(dir, "coverage-audit-support.1"))
		}, "stale man pages: coverage-audit-support.1\n"},
		{"page not in spec", func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "coverage-audit-old.1"), []byte("x\n"), 0o644)
		}, "stale man pages: coverage-audit-old.1 (untracked by clispec)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if c.name != "empty dir" {
				runMain(t, "-dir", dir)
			}
			if err := c.setup(dir); err != nil {
				t.Fatal(err)
			}
			out, _, code := runExit(t, "-check", "-dir", dir)
			if code != 1 || out != c.want {
				t.Errorf("exit %d, stdout %q; want exit 1, %q", code, out, c.want)
			}
		})
	}
}

func TestMainWriteErrors(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "a-file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name    string
		args    []string
		wantOut string
		wantErr string
	}{
		{"dir is a file", []string{"-dir", filepath.Join(file, "man1")}, "", "a-file"},
		{"page path is a directory", nil, "", "coverage-audit-support.1"},
		{"tarball dir missing", []string{"-dir", filepath.Join(tmp, "ok"), "-tarball", filepath.Join(tmp, "no", "such", "x.tar.gz")},
			"wrote 2 man page(s) to " + filepath.Join(tmp, "ok") + "\n", "x.tar.gz"},
	} {
		t.Run(c.name, func(t *testing.T) {
			args := c.args
			if args == nil {
				dir := t.TempDir()
				if err := os.Mkdir(filepath.Join(dir, "coverage-audit-support.1"), 0o755); err != nil {
					t.Fatal(err)
				}
				args = []string{"-dir", dir}
			}
			out, errOut, code := runExit(t, args...)
			if code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
			if out != c.wantOut {
				t.Errorf("stdout = %q, want %q", out, c.wantOut)
			}
			if !strings.Contains(errOut, c.wantErr) {
				t.Errorf("stderr %q does not name %q", errOut, c.wantErr)
			}
		})
	}
}

func readTarball(t *testing.T, path string) (map[string]string, []string, *gzip.Header) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	hdr := gz.Header
	tr := tar.NewReader(gz)
	files := map[string]string{}
	var order []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Mode != 0o644 {
			t.Errorf("%s: mode %o, want 644", h.Name, h.Mode)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		files[h.Name] = string(b)
		order = append(order, h.Name)
	}
	return files, order, &hdr
}

func TestMainTarball(t *testing.T) {
	tmp := t.TempDir()
	tgz := filepath.Join(tmp, "man.tar.gz")
	out := runMain(t, "-dir", filepath.Join(tmp, "a"), "-tarball", tgz)
	if !strings.HasSuffix(out, "wrote tarball "+tgz+"\n") {
		t.Errorf("stdout = %q", out)
	}
	files, order, hdr := readTarball(t, tgz)
	pages := renderAll()
	var want []string
	for name, body := range pages {
		want = append(want, "man1/"+name)
		if files["man1/"+name] != body {
			t.Errorf("tarball man1/%s differs from render", name)
		}
	}
	sort.Strings(want)
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("tarball entries %v, want sorted %v", order, want)
	}
	if !hdr.ModTime.IsZero() {
		t.Errorf("gzip mtime %v, want zero", hdr.ModTime)
	}

	// Deterministic: a second run (other -dir) produces identical bytes.
	tgz2 := filepath.Join(tmp, "again.tar.gz")
	runMain(t, "-dir", filepath.Join(tmp, "b"), "-tarball", tgz2)
	a, err := os.ReadFile(tgz)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(tgz2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Error("two tarball runs differ; the archive is not deterministic")
	}
}
