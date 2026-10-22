package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mcpFixture(t *testing.T) mcpConfig {
	t.Helper()
	dir := t.TempDir()
	w := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	return mcpConfig{
		Parcels:   w("parcels.json", `{"features":[{"properties":{"gush":1,"helka":1}},{"properties":{"gush":1,"helka":2}},{"properties":{"gush":1,"helka":3}},{"properties":{"gush":1,"helka":4}}]}`),
		Published: w("pub.json", `{"batch":{"offset":0,"limit":2},"requests":[{"tik_num":"A","status":"x","status_date":"2026-01-02","private_person":"SYNTHETIC"},{"tik_num":"B","status":"y"}]}`),
		Previous:  w("prev.json", `{"batch":{"offset":0,"limit":2},"requests":[{"tik_num":"A","status":"old"},{"tik_num":"C"}]}`),
		Plans:     w("plans.json", `{"rows":[{"pl_number":"101-1","entity_subtype_desc":"t","station_desc":"הפקדה","objection_deadline_derived_60d_from_publication":"2026-11-01"}]}`),
		BatchSize: 2,
	}
}

func rpc(t *testing.T, cfg mcpConfig, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := serveMCP(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, cfg); err != nil {
		t.Fatal(err)
	}
	var res []map[string]any
	dec := json.NewDecoder(&out)
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatal(err)
		}
		res = append(res, m)
	}
	return res
}

func toolText(t *testing.T, resp map[string]any) (string, bool) {
	t.Helper()
	r := resp["result"].(map[string]any)
	text := r["content"].([]any)[0].(map[string]any)["text"].(string)
	isErr, _ := r["isError"].(bool)
	return text, isErr
}

func TestMCPHandshakeListAndNotificationsGetNoReply(t *testing.T) {
	res := rpc(t, mcpFixture(t),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"nope"}`)
	if len(res) != 3 {
		t.Fatalf("expected 3 responses (notification gets none), got %d", len(res))
	}
	if res[0]["result"].(map[string]any)["protocolVersion"] != mcpProtocol {
		t.Fatal("bad protocol version")
	}
	if n := len(res[1]["result"].(map[string]any)["tools"].([]any)); n != 3 {
		t.Fatalf("tools=%d", n)
	}
	if res[2]["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Fatal("unknown method must be -32601")
	}
}

func TestMCPCoverageStatusKeepsCaveat(t *testing.T) {
	res := rpc(t, mcpFixture(t), `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"coverage_status","arguments":{}}}`)
	text, isErr := toolText(t, res[0])
	if isErr || !strings.Contains(text, `"published_cursor":2`) || !strings.Contains(text, `"remaining":2`) || !strings.Contains(text, "do not prove") {
		t.Fatalf("%v %s", isErr, text)
	}
}

func TestMCPSnapshotChangesAreCountsOnly(t *testing.T) {
	res := rpc(t, mcpFixture(t), `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"snapshot_changes"}}`)
	text, isErr := toolText(t, res[0])
	if isErr || !strings.Contains(text, `"added":1`) || !strings.Contains(text, `"changed":1`) || !strings.Contains(text, `"absent_from_current_not_proven_deleted":1`) {
		t.Fatalf("%v %s", isErr, text)
	}
	for _, leak := range []string{`"A"`, `"B"`, `"C"`, "SYNTHETIC", "private_person"} {
		if strings.Contains(text, leak) {
			t.Fatalf("snapshot_changes leaked %s: %s", leak, text)
		}
	}
}

func TestMCPPlannerInsightsAndNotConfigured(t *testing.T) {
	cfg := mcpFixture(t)
	res := rpc(t, cfg, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"planner_insights","arguments":{"as_of":"2026-10-22"}}}`)
	text, isErr := toolText(t, res[0])
	if isErr || !strings.Contains(text, `"permit_count":2`) || !strings.Contains(text, "101-1") || !strings.Contains(text, "limitations") {
		t.Fatalf("%v %s", isErr, text)
	}
	cfg.Plans, cfg.Previous = "", ""
	res = rpc(t, cfg,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"planner_insights","arguments":{"as_of":"2026-10-22"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"snapshot_changes"}}`)
	for _, r := range res {
		if text, isErr := toolText(t, r); !isErr || !strings.Contains(text, "not configured") {
			t.Fatalf("expected not-configured error, got %v %s", isErr, text)
		}
	}
}

// Tool arguments must never choose files: a path argument is ignored.
func TestMCPToolArgumentsCannotNameFiles(t *testing.T) {
	cfg := mcpFixture(t)
	secret := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(secret, []byte(`{"requests":[{"tik_num":"LEAK-SYNTHETIC"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"as_of": "2026-10-22", "path": secret, "published": secret, "plans": secret})
	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "planner_insights", "arguments": json.RawMessage(args)}})
	res := rpc(t, cfg, string(call))
	text, _ := toolText(t, res[0])
	if strings.Contains(text, "LEAK-SYNTHETIC") || !strings.Contains(text, `"permit_count":2`) {
		t.Fatalf("path argument was honoured: %s", text)
	}
}

func TestMCPBadInputsDoNotCrash(t *testing.T) {
	res := rpc(t, mcpFixture(t),
		`not json`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"planner_insights","arguments":{"as_of":"yesterday"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"delete_everything"}}`)
	if len(res) != 3 || res[0]["error"].(map[string]any)["code"].(float64) != -32700 {
		t.Fatalf("%v", res)
	}
	for _, r := range res[1:] {
		if _, isErr := toolText(t, r); !isErr {
			t.Fatal("expected tool error")
		}
	}
}
