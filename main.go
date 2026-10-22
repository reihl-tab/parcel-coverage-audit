// coverage-audit reads local files only. Cursor progress is not verified query success.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

type Batch struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
	Done   int `json:"done,omitempty"`
}
type Feed struct {
	Batch      Batch `json:"batch"`
	Checkpoint bool  `json:"checkpoint"`
}
type Parcels struct {
	Features []struct {
		Properties struct {
			Gush  json.Number `json:"gush"`
			Helka json.Number `json:"helka"`
		} `json:"properties"`
	} `json:"features"`
}
type Report struct {
	Total      int     `json:"total_unique_parcels"`
	Cursor     int     `json:"published_cursor"`
	Remaining  int     `json:"remaining"`
	Percent    float64 `json:"cursor_percent"`
	Batches    int     `json:"remaining_batches"`
	Next       *Batch  `json:"next_range,omitempty"`
	Checkpoint string  `json:"checkpoint_status"`
	Warning    string  `json:"warning"`
}

func decode(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("invalid trailing JSON in %s", path)
	}
	return nil
}
func totalParcels(p Parcels) (int, error) {
	unique := map[[2]int64]bool{}
	for _, feature := range p.Features {
		g, h := feature.Properties.Gush, feature.Properties.Helka
		if g == "" || h == "" || g == "0" || h == "0" {
			continue
		}
		gn, err := g.Int64()
		if err != nil || gn < 1 {
			return 0, fmt.Errorf("invalid parcel block")
		}
		hn, err := h.Int64()
		if err != nil || hn < 1 {
			return 0, fmt.Errorf("invalid parcel number")
		}
		unique[[2]int64{gn, hn}] = true
	}
	if len(unique) == 0 {
		return 0, fmt.Errorf("no valid parcels")
	}
	return len(unique), nil
}
func audit(total int, feed Feed, cp *Feed, size int) (Report, error) {
	r := Report{Total: total, Checkpoint: "not supplied", Warning: "Cursor/range progress only; files do not prove every parcel query succeeded or that earlier ranges are gap-free."}
	b := feed.Batch
	if total <= 0 || size <= 0 || b.Offset < 0 || b.Limit <= 0 || b.Offset >= total || b.Limit > int(^uint(0)>>1)-b.Offset || feed.Checkpoint {
		return r, fmt.Errorf("invalid published batch or batch size")
	}
	cursor := b.Offset + b.Limit
	if cursor > total {
		cursor = total
	} // final scheduled batch can extend beyond last parcel
	r.Cursor = cursor
	r.Remaining = total - cursor
	r.Percent = 100 * float64(cursor) / float64(total)
	r.Batches = r.Remaining / size
	if r.Remaining%size != 0 {
		r.Batches++
	}
	if r.Remaining > 0 {
		limit := size
		if limit > r.Remaining {
			limit = r.Remaining
		}
		r.Next = &Batch{Offset: cursor, Limit: limit}
	}
	if cp != nil {
		c := cp.Batch
		// Scanner adds one known control parcel outside the slice when absent.
		max := total - c.Offset
		if max > c.Limit {
			max = c.Limit
		}
		max++
		if !cp.Checkpoint || c.Offset != cursor || c.Limit != size || c.Done < 1 || c.Done > max || c.Offset >= total {
			return r, fmt.Errorf("checkpoint does not match next range or done count")
		}
		r.Checkpoint = "consistent with next range; done count may include one control parcel, not extra cursor coverage"
	}
	return r, nil
}
func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("coverage-audit", flag.ContinueOnError)
	parcels := flags.String("parcels", "data/ginot_hair_parcels.geojson", "local GeoJSON")
	published := flags.String("published", "data/permit_requests.json", "published feed")
	checkpoint := flags.String("checkpoint", "", "optional local checkpoint")
	size := flags.Int("batch-size", 280, "planned next batch size")
	plansPath := flags.String("plans", "", "optional public planning rows for observations")
	asOf := flags.String("as-of", "", "ISO observation date, explicit for reproducibility")
	cleanOut := flags.String("clean-out", "", "write separate normalized dataset, never overwrite")
	previous := flags.String("previous", "", "optional previous public permit snapshot for comparison")
	asJSON := flags.Bool("json", false, "JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	var p Parcels
	var feed Feed
	if err := decode(*parcels, &p); err != nil {
		return fmt.Errorf("parcels: %w", err)
	}
	if err := decode(*published, &feed); err != nil {
		return fmt.Errorf("published: %w", err)
	}
	total, err := totalParcels(p)
	if err != nil {
		return err
	}
	var cp *Feed
	if *checkpoint != "" {
		cp = &Feed{}
		if err := decode(*checkpoint, cp); err != nil {
			return fmt.Errorf("checkpoint: %w", err)
		}
	}
	report, err := audit(total, feed, cp, *size)
	if err != nil {
		return err
	}
	if *cleanOut != "" || *previous != "" || *plansPath != "" {
		raw, err := os.ReadFile(*published)
		if err != nil {
			return err
		}
		current, err := normalize(raw)
		if err != nil {
			return err
		}
		var diff *Changes
		if *previous != "" {
			old, err := os.ReadFile(*previous)
			if err != nil {
				return err
			}
			before, err := normalize(old)
			if err != nil {
				return err
			}
			value := compare(before, current)
			diff = &value
		}
		if *cleanOut != "" {
			if err := writeClean(*cleanOut, current); err != nil {
				return err
			}
		}
		var insight *Insights
		if *plansPath != "" {
			day, err := time.Parse("2006-01-02", *asOf)
			if err != nil {
				return fmt.Errorf("-as-of ISO date required for observations")
			}
			var p struct {
				Rows []Plan `json:"rows"`
			}
			if err := decode(*plansPath, &p); err != nil {
				return err
			}
			value := observations(current, p.Rows, day)
			insight = &value
		}
		return json.NewEncoder(out).Encode(struct {
			Coverage   Report    `json:"coverage"`
			Validation []string  `json:"validation"`
			Changes    *Changes  `json:"changes,omitempty"`
			Insights   *Insights `json:"insights,omitempty"`
		}{report, current.Warnings, diff, insight})
	}
	if *asJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	keys := []string{fmt.Sprintf("Cursor: %d/%d (%.2f%%)", report.Cursor, report.Total, report.Percent), fmt.Sprintf("Remaining: %d parcels; %d batches", report.Remaining, report.Batches)}
	// Output ordering is fixed and independent of map iteration.
	for _, line := range keys {
		fmt.Fprintln(out, line)
	}
	if report.Next != nil {
		fmt.Fprintf(out, "Next range: offset=%d limit=%d\n", report.Next.Offset, report.Next.Limit)
	}
	fmt.Fprintln(out, "Checkpoint:", report.Checkpoint)
	fmt.Fprintln(out, "Warning:", report.Warning)
	return nil
}
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "coverage-audit:", err)
		os.Exit(1)
	}
}
