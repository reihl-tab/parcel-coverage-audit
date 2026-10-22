package main

import (
	"bytes"
	"encoding/json"
	"os"
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
