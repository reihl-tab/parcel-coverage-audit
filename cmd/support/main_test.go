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
