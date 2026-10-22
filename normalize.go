package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Only explicitly public permit fields survive normalization. Unknown fields,
// personal-interest lists and nested service payloads never reach clean output.
type Permit struct {
	Tik     string `json:"tik_num"`
	Subject string `json:"mahut"`
	Type    string `json:"sugbakasha"`
	Status  string `json:"status"`
	Date    string `json:"status_date"`
	Gush    int    `json:"gush"`
	Helka   int    `json:"helka"`
	Start   string `json:"publication_start"`
	End     string `json:"publication_end_planned"`
	Address string `json:"address"`
}
type Dataset struct {
	Generated string   `json:"generated"`
	Batch     Batch    `json:"batch"`
	Requests  []Permit `json:"requests"`
	Warnings  []string `json:"validation_warnings"`
}
type Changes struct {
	Added      []string `json:"added"`
	Changed    []string `json:"changed"`
	Absent     []string `json:"absent_from_current_not_proven_deleted"`
	Comparable bool     `json:"same_batch_range"`
	Warning    string   `json:"warning"`
}

func isoDate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	for _, layout := range []string{"2006-01-02", "02/01/2006", "2/1/2006"} {
		if d, err := time.Parse(layout, raw); err == nil {
			return d.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("invalid date")
}
func normalize(raw []byte) (Dataset, error) {
	var data Dataset
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, fmt.Errorf("invalid permit dataset: %w", err)
	}
	if data.Requests == nil {
		return data, fmt.Errorf("requests array missing")
	}
	data.Warnings = []string{}
	seen := map[string]Permit{}
	for _, r := range data.Requests {
		r.Tik = strings.TrimSpace(r.Tik)
		if r.Tik == "" {
			return data, fmt.Errorf("request without stable identifier")
		}
		r.Subject = strings.TrimSpace(r.Subject)
		r.Type = strings.TrimSpace(r.Type)
		r.Status = strings.TrimSpace(r.Status)
		r.Address = strings.TrimSpace(r.Address)
		if r.Gush < 0 || r.Helka < 0 {
			return data, fmt.Errorf("negative parcel identifier")
		}
		for _, field := range []*string{&r.Date, &r.Start, &r.End} {
			v, err := isoDate(*field)
			if err != nil {
				return data, fmt.Errorf("request date invalid (row identifier withheld)")
			}
			*field = v
		}
		if prior, ok := seen[r.Tik]; ok {
			if prior != r {
				return data, fmt.Errorf("conflicting duplicate request; refusing arbitrary winner")
			}
			data.Warnings = append(data.Warnings, "identical duplicate removed")
		}
		seen[r.Tik] = r
	}
	data.Requests = []Permit{}
	missing := 0
	for _, r := range seen {
		data.Requests = append(data.Requests, r)
		if r.Date == "" {
			missing++
		}
	}
	sort.Slice(data.Requests, func(i, j int) bool { return data.Requests[i].Tik < data.Requests[j].Tik })
	if missing > 0 {
		data.Warnings = append(data.Warnings, fmt.Sprintf("%d requests lack status dates; status aging/speed cannot be inferred", missing))
	}
	return data, nil
}
func compare(previous, current Dataset) Changes {
	c := Changes{Added: []string{}, Changed: []string{}, Absent: []string{}, Comparable: previous.Batch.Offset == current.Batch.Offset && previous.Batch.Limit == current.Batch.Limit, Warning: "Differences describe public file snapshots, not proven real-world events or deletions. New batch ranges can introduce old records."}
	before := map[string]Permit{}
	after := map[string]Permit{}
	for _, r := range previous.Requests {
		before[r.Tik] = r
	}
	for _, r := range current.Requests {
		after[r.Tik] = r
	}
	for id, r := range after {
		old, ok := before[id]
		if !ok {
			c.Added = append(c.Added, id)
		} else if old != r {
			c.Changed = append(c.Changed, id)
		}
	}
	for id := range before {
		if _, ok := after[id]; !ok {
			c.Absent = append(c.Absent, id)
		}
	}
	sort.Strings(c.Added)
	sort.Strings(c.Changed)
	sort.Strings(c.Absent)
	return c
}
func writeClean(path string, data Dataset) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("clean output already exists; refusing overwrite")
	}
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(body, '\n'))
	return err
}
