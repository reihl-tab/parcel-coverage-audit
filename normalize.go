package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	return writeFileAtomic(path, append(body, '\n'), 0600)
}

// writeFileAtomic writes body to a temp file in dir(path), syncs it, closes
// it (checking the Close error, since some filesystems only report a failed
// flush there), then publishes it to path with Link so a file only ever
// appears at path once it is complete, and never overwrites an existing one
// (Link fails with ErrExist, race-free against a concurrent writer). Every
// error path removes the temp file, so a failed write leaves no file at path
// and no stray temp file behind.
// afterTempWriteHook, when set, runs after the temp file is written, synced
// and closed, but before it is published with Link. It exists only for
// tests, to inject a failure between "write complete" and "publish" (a
// vanished temp file, a read-only directory, ...) without adding a branch
// to the production path; it is nil in normal operation.
var afterTempWriteHook func(tmpPath string)

func writeFileAtomic(path string, body []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".clean-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { os.Remove(tmpPath) }

	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if afterTempWriteHook != nil {
		afterTempWriteHook(tmpPath)
	}
	if err := os.Link(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	cleanup()
	return nil
}
