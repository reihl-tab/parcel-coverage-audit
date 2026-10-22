package main

import (
	"strings"
	"testing"
	"time"
)

func TestObservationsNearWindowsAndMissingDateLimits(t *testing.T) {
	d, _ := time.Parse("2006-01-02", "2026-10-22")
	p := []Plan{{ID: "A", Type: "local", Status: "הפקדה", Deadline: "2026-11-05"}, {ID: "B", Status: "אישור", Deadline: "2026-11-01"}, {ID: "C", Status: "הפקדה", Deadline: "bad"}}
	o := observations(Dataset{Requests: []Permit{{Tik: "1", Status: "pending"}}}, p, d)
	if len(o.Near) != 1 || o.Near[0].Days != 14 || o.MissingDates != 1 || o.PlanTypes["local"] != 1 {
		t.Fatalf("%+v", o)
	}
	if !strings.Contains(strings.Join(o.Flags, " "), "not legal") {
		t.Fatal("missing caveat")
	}
}
func TestObservationsExcludeExpiredAndDistantWindows(t *testing.T) {
	d, _ := time.Parse("2006-01-02", "2026-10-22")
	p := []Plan{{ID: "old", Status: "הפקדה", Deadline: "2026-10-21"}, {ID: "far", Status: "הפקדה", Deadline: "2027-01-01"}}
	if o := observations(Dataset{}, p, d); len(o.Near) != 0 {
		t.Fatal(o)
	}
}
