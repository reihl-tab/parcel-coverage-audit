// Support commands are separate from the offline auditor CLI.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const source = "reihlinstinct/ginot-hair-plans"

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
var idRE = regexp.MustCompile(`^[0-9]+$`)

func command(input []byte, name string, args ...string) ([]byte, error) {
	c := exec.Command(name, args...)
	c.Stdin = bytes.NewReader(input)
	c.Stderr = os.Stderr
	return c.Output()
}
func api(path string, payload any, out any) error {
	args := []string{"api", path}
	var b []byte
	if payload != nil {
		var err error
		b, err = json.Marshal(payload)
		if err != nil {
			return err
		}
		args = append(args, "-X", "POST", "--input", "-")
	}
	data, err := command(b, "gh", args...)
	if err != nil {
		return err
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

type metadata struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Path       string `json:"path"`
}

func validate(sha, run string, m metadata) error {
	if !shaRE.MatchString(sha) || !idRE.MatchString(run) {
		return fmt.Errorf("invalid source identifiers")
	}
	if m.Conclusion != "success" || m.Path != ".github/workflows/permit-feed.yml" {
		return fmt.Errorf("source is not successful permit-feed workflow")
	}
	return nil
}
func fetch() error {
	sha, run := os.Getenv("SOURCE_SHA"), os.Getenv("SOURCE_RUN")
	if !shaRE.MatchString(sha) || !idRE.MatchString(run) {
		return fmt.Errorf("invalid source identifiers")
	}
	var m metadata
	for i := 0; i < 30; i++ {
		if err := api("repos/"+source+"/actions/runs/"+run, nil, &m); err != nil {
			return err
		}
		if m.Status == "completed" {
			break
		}
		time.Sleep(10 * time.Second)
	}
	if err := validate(sha, run, m); err != nil {
		return err
	}
	var comparison struct {
		Status string `json:"status"`
	}
	if err := api("repos/"+source+"/compare/"+sha+"...main", nil, &comparison); err != nil {
		return err
	}
	if comparison.Status != "identical" && comparison.Status != "ahead" {
		return fmt.Errorf("source SHA is not a main ancestor")
	}
	if _, err := command(nil, "git", "clone", "--filter=blob:none", "--no-checkout", "https://github.com/"+source+".git", "source"); err != nil {
		return err
	}
	if _, err := command(nil, "git", "-C", "source", "checkout", sha, "--", "data/ginot_hair_parcels.geojson", "data/permit_requests.json", "docs/data.json"); err != nil {
		return err
	}
	return history(sha)
}
func callbackPayload(report, raw []byte, sha, run, auditor string) (map[string]any, error) {
	if !shaRE.MatchString(sha) || !idRE.MatchString(run) || !idRE.MatchString(auditor) {
		return nil, fmt.Errorf("invalid identifiers")
	}
	var r map[string]any
	if err := json.Unmarshal(report, &r); err != nil {
		return nil, err
	}
	r["schema_version"] = 1
	r["source_commit"] = sha
	r["source_run"] = run
	r["auditor_run"] = auditor
	r["source_digest"] = fmt.Sprintf("%x", sha256.Sum256(raw))
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	if len(b) > 6000 {
		return nil, fmt.Errorf("callback exceeds bounded report size")
	}
	return map[string]any{"ref": "main", "inputs": map[string]string{"report": string(b)}}, nil
}
func callback() error {
	report, err := os.ReadFile("audit.json")
	if err != nil {
		return err
	}
	raw, err := os.ReadFile("source/data/permit_requests.json")
	if err != nil {
		return err
	}
	p, err := callbackPayload(report, raw, os.Getenv("SOURCE_SHA"), os.Getenv("SOURCE_RUN"), os.Getenv("GITHUB_RUN_ID"))
	if err != nil {
		return err
	}
	return api("repos/"+source+"/actions/workflows/audit-result.yml/dispatches", p, nil)
}

// Patterns are synthesized here so this source does not resemble a leaked secret.
var privacyPatterns = map[string]*regexp.Regexp{
	"GitHub credential":   regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{30,})\b`),
	"private key":         regexp.MustCompile(`-----BEGIN (?:RSA |OPENSSH |EC )?PRIVATE KEY-----`),
	"email":               regexp.MustCompile(`[A-Za-z0-9_.+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`),
	"phone":               regexp.MustCompile(`\+[0-9][0-9 -]{8,}[0-9]`),
	"plate-shaped number": regexp.MustCompile(`(?:^|[^0-9])[0-9]{2,3}-[0-9]{2,3}-[0-9]{2,3}(?:$|[^0-9])`),
}

func privacy(root string) error {
	count := 0
	var problems []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		count++
		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(root, "cmd/support/main.go")) {
			return nil
		}
		if strings.HasSuffix(path, ".geojson") || strings.HasSuffix(path, ".out") || strings.HasSuffix(path, ".local.json") {
			problems = append(problems, path+": prohibited input/output data file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for label, re := range privacyPatterns {
			if re.Match(data) {
				problems = append(problems, path+": "+label)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	fmt.Printf("PASS: %d source/config/doc files scanned; no detected private material.\n", count)
	return nil
}

type event struct {
	Test   string
	Action string
}

func states(events []byte) map[string]string {
	s := map[string]string{}
	for _, line := range bytes.Split(events, []byte("\n")) {
		var e event
		if json.Unmarshal(line, &e) == nil && e.Test != "" && (e.Action == "pass" || e.Action == "fail" || e.Action == "skip") {
			s[e.Test] = e.Action
		}
	}
	return s
}
func testSources(root string) (map[string]string, error) {
	result := map[string]string{}
	re := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		matches := re.FindAllSubmatchIndex(b, -1)
		for i, m := range matches {
			end := len(b)
			if i+1 < len(matches) {
				end = matches[i+1][0]
			}
			result[string(b[m[2]:m[3]])] = string(b[m[0]:end])
		}
		return nil
	})
	return result, err
}
func render(events []byte, coverage string, src map[string]string) string {
	s := states(events)
	counts := map[string]int{}
	names := []string{}
	for n, state := range s {
		counts[state]++
		names = append(names, n)
	}
	rank := map[string]int{"fail": 0, "skip": 1, "pass": 2}
	sort.Slice(names, func(i, j int) bool {
		if s[names[i]] != s[names[j]] {
			return rank[s[names[i]]] < rank[s[names[j]]]
		}
		return names[i] < names[j]
	})
	var b strings.Builder
	fmt.Fprintf(&b, "## Go audit tests\n\n| Tests | Passed | Failed | Skipped |\n| ---: | ---: | ---: | ---: |\n| %d | %d | %d | %d |\n\nCoverage: %s\n\n<details><summary>Per-test results</summary>\n", len(s), counts["pass"], counts["fail"], counts["skip"], html.EscapeString(strings.TrimSpace(coverage)))
	for _, n := range names {
		code := src[n]
		if code == "" {
			code = "Source unavailable."
		}
		fmt.Fprintf(&b, "<p><b>%s</b> %s</p>\n<details><summary>Exact synthetic inputs, steps and assertions</summary>\n<p>Checked-in source only. Actual passing operand values not captured. Skipped means checks were not executed; no alternative run assumed. Failures: consult job log for actual diagnostic.</p>\n<pre>%s</pre>\n</details>\n", strings.ToUpper(s[n]), html.EscapeString(n), html.EscapeString(code))
	}
	return b.String() + "</details>\n"
}
func summary() error {
	events, _ := os.ReadFile("test-events.json")
	coverage, err := os.ReadFile("coverage-total.txt")
	if err != nil {
		coverage = []byte("unavailable")
	}
	src, err := testSources(".")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(os.Getenv("GITHUB_STEP_SUMMARY"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(render(events, string(coverage), src))
	return err
}
func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("expected fetch, callback, privacy or summary")
	}
	switch args[0] {
	case "fetch":
		return fetch()
	case "callback":
		return callback()
	case "privacy":
		return privacy(".")
	case "bundle":
		return bundle()
	case "badges":
		return badges()
	case "summary":
		return summary()
	default:
		return fmt.Errorf("unknown support command")
	}
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Badge endpoints contain aggregate metrics only, derived from successful CI.
func badgeMetrics(events []byte, coverage string) (map[string]map[string]any, error) {
	s := states(events)
	passed, skipped := 0, 0
	for _, state := range s {
		switch state {
		case "pass":
			passed++
		case "skip":
			skipped++
		}
	}
	re := regexp.MustCompile(`([0-9.]+)%`)
	m := re.FindAllStringSubmatch(coverage, -1)
	if len(s) == 0 || passed+skipped != len(s) || len(m) == 0 {
		return nil, fmt.Errorf("empty/failing tests or missing coverage")
	}
	badge := func(label, msg string) map[string]any {
		return map[string]any{"schemaVersion": 1, "label": label, "message": msg, "color": "blue"}
	}
	return map[string]map[string]any{"tests": badge("Go suite", fmt.Sprintf("%d passed, %d skipped", passed, skipped)), "coverage": badge("test coverage", m[len(m)-1][1]+"%")}, nil
}
func badges() error {
	events, err := os.ReadFile("test-events.json")
	if err != nil {
		return err
	}
	coverage, err := os.ReadFile("coverage-total.txt")
	if err != nil {
		return err
	}
	values, err := badgeMetrics(events, string(coverage))
	if err != nil {
		return err
	}
	if err := os.MkdirAll("/tmp/badges", 0700); err != nil {
		return err
	}
	for name, value := range values {
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err := os.WriteFile("/tmp/badges/"+name+".json", append(b, '\n'), 0600); err != nil {
			return err
		}
	}
	return nil
}

// history selects the nearest earlier distinct feed from the immutable public source.
func history(sha string) error {
	revisions, err := command(nil, "git", "-C", "source", "log", "-n", "2", "--format=%H", sha, "--", "data/permit_requests.json")
	if err != nil {
		return err
	}
	ids := strings.Fields(string(revisions))
	info := map[string]any{"available": false, "reason": "No earlier public feed version in source history."}
	if len(ids) > 1 {
		if !shaRE.MatchString(ids[1]) {
			return fmt.Errorf("invalid history SHA")
		}
		b, err := command(nil, "git", "-C", "source", "show", ids[1]+":data/permit_requests.json")
		if err != nil {
			return err
		}
		if !json.Valid(b) {
			return fmt.Errorf("invalid previous feed JSON")
		}
		if err := os.WriteFile("previous.json", b, 0600); err != nil {
			return err
		}
		info = map[string]any{"available": true, "previous_commit": ids[1], "warning": "Different batch ranges can introduce older records; absence is not deletion."}
	}
	b, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile("history.json", b, 0600)
}
func bundle() error {
	names := []string{"audit.json", "analysis.json", "clean.json", "history.json"}
	hashes := map[string]string{}
	for _, name := range names {
		b, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if len(b) > 20*1024*1024 || !json.Valid(b) {
			return fmt.Errorf("invalid or oversized bundle member")
		}
		hashes[name] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	sha := os.Getenv("SOURCE_SHA")
	run := os.Getenv("SOURCE_RUN")
	if !shaRE.MatchString(sha) || !idRE.MatchString(run) {
		return fmt.Errorf("invalid source identifiers")
	}
	b, err := json.MarshalIndent(map[string]any{"schema_version": 1, "source_commit": sha, "source_run": run, "sha256": hashes}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("manifest.json", b, 0600)
}
