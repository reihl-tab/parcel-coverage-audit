package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Plan struct {
	ID       string `json:"pl_number"`
	Type     string `json:"entity_subtype_desc"`
	Status   string `json:"station_desc"`
	Received string `json:"receiving_date"`
	Deadline string `json:"objection_deadline_derived_60d_from_publication"`
}
type Window struct {
	ID   string `json:"plan"`
	Date string `json:"derived_date"`
	Days int    `json:"days"`
}
type Insights struct {
	AsOf           string         `json:"as_of"`
	PermitCount    int            `json:"permit_count"`
	MissingDates   int            `json:"permits_missing_status_date"`
	PermitStatuses map[string]int `json:"permit_status_counts"`
	PlanTypes      map[string]int `json:"plan_type_counts"`
	Near           []Window       `json:"near_derived_windows"`
	Flags          []string       `json:"limitations"`
}

func observations(data Dataset, plans []Plan, asOf time.Time) Insights {
	o := Insights{AsOf: asOf.Format("2006-01-02"), PermitCount: len(data.Requests), PermitStatuses: map[string]int{}, PlanTypes: map[string]int{}, Near: []Window{}, Flags: []string{
		"Public snapshot observations only, not legal conclusions or verified current deadlines.",
		"Partial parcel coverage biases spatial/type counts; no neighborhood labels inferred from address strings.",
		"No status age, processing speed or outlier duration inferred without dated repeated comparable snapshots.",
	}}
	for _, r := range data.Requests {
		o.PermitStatuses[r.Status]++
		if r.Date == "" {
			o.MissingDates++
		}
	}
	if o.MissingDates > 0 {
		o.Flags = append(o.Flags, fmt.Sprintf("%d permit rows have no status date; age/speed unavailable.", o.MissingDates))
	}
	for _, p := range plans {
		kind := strings.TrimSpace(p.Type)
		if kind == "" {
			kind = "unclassified"
		}
		o.PlanTypes[kind]++
		if !strings.Contains(p.Status, "הפקדה") {
			continue
		}
		d, err := time.Parse("2006-01-02", p.Deadline)
		if err != nil {
			continue
		}
		days := int(d.Sub(asOf).Hours() / 24)
		if days >= 0 && days <= 30 {
			o.Near = append(o.Near, Window{p.ID, p.Deadline, days})
		}
	}
	sort.Slice(o.Near, func(i, j int) bool {
		if o.Near[i].Date == o.Near[j].Date {
			return o.Near[i].ID < o.Near[j].ID
		}
		return o.Near[i].Date < o.Near[j].Date
	})
	return o
}
