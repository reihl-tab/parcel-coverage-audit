package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Read-only MCP server over stdio (JSON-RPC 2.0, newline-delimited).
// Data paths are fixed at startup; tool arguments never name files.
// Output is aggregate counts only and always carries the limitation warnings.

const mcpProtocol = "2024-11-05"

type mcpConfig struct {
	Parcels, Published, Checkpoint, Previous, Plans string
	BatchSize                                       int
}

type rpcRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

var mcpTools = []map[string]any{
	{"name": "coverage_status", "description": "Cursor progress of the parcel scan: published cursor, remaining parcels and batches, next range. Cursor progress is not proof that every parcel query succeeded.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}},
	{"name": "snapshot_changes", "description": "Counts of permit rows added, changed or absent between the previous and current public snapshot. Absence is not deletion; additions are not proof of new applications. Only available when the server was started with -previous.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}},
	{"name": "planner_insights", "description": "Aggregate observations: permit status counts, missing status dates, plan type counts, derived objection windows within 30 days of as_of. Derived dates are not official deadlines. Only available when the server was started with -plans.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"as_of": map[string]any{"type": "string", "description": "YYYY-MM-DD"}}, "required": []string{"as_of"}}},
}

func serveMCP(in io.Reader, out io.Writer, cfg mcpConfig) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			enc.Encode(rpcError(nil, -32700, "parse error"))
			continue
		}
		if resp := handleRPC(req, cfg); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

func rpcError(id json.RawMessage, code int, msg string) map[string]any {
	if id == nil {
		id = json.RawMessage("null")
	}
	return map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}}
}

func handleRPC(req rpcRequest, cfg mcpConfig) map[string]any {
	if req.ID == nil { // notification: no response
		return nil
	}
	ok := func(result any) map[string]any {
		return map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}
	}
	switch req.Method {
	case "initialize":
		return ok(map[string]any{"protocolVersion": mcpProtocol, "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{"name": "parcel-coverage-audit", "version": "0.1"}})
	case "ping":
		return ok(map[string]any{})
	case "tools/list":
		return ok(map[string]any{"tools": mcpTools})
	case "tools/call":
		var p struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return rpcError(req.ID, -32602, "invalid params")
		}
		value, err := callTool(p.Name, p.Args, cfg)
		if err != nil {
			return ok(map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": err.Error()}}})
		}
		body, _ := json.Marshal(value)
		return ok(map[string]any{"content": []map[string]any{{"type": "text", "text": string(body)}}})
	}
	return rpcError(req.ID, -32601, "method not found")
}

func callTool(name string, args json.RawMessage, cfg mcpConfig) (any, error) {
	switch name {
	case "coverage_status":
		var p Parcels
		var feed Feed
		if err := decode(cfg.Parcels, &p); err != nil {
			return nil, fmt.Errorf("parcels unavailable")
		}
		if err := decode(cfg.Published, &feed); err != nil {
			return nil, fmt.Errorf("published feed unavailable")
		}
		total, err := totalParcels(p)
		if err != nil {
			return nil, err
		}
		var cp *Feed
		if cfg.Checkpoint != "" {
			cp = &Feed{}
			if err := decode(cfg.Checkpoint, cp); err != nil {
				return nil, fmt.Errorf("checkpoint unavailable")
			}
		}
		return audit(total, feed, cp, cfg.BatchSize)
	case "snapshot_changes":
		if cfg.Previous == "" {
			return nil, fmt.Errorf("not configured: start the server with -previous")
		}
		cur, err := loadDataset(cfg.Published)
		if err != nil {
			return nil, err
		}
		old, err := loadDataset(cfg.Previous)
		if err != nil {
			return nil, err
		}
		c := compare(old, cur)
		return map[string]any{"added": len(c.Added), "changed": len(c.Changed), "absent_from_current_not_proven_deleted": len(c.Absent),
			"same_batch_range": c.Comparable, "warning": c.Warning}, nil
	case "planner_insights":
		if cfg.Plans == "" {
			return nil, fmt.Errorf("not configured: start the server with -plans")
		}
		var a struct {
			AsOf string `json:"as_of"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, fmt.Errorf("as_of (YYYY-MM-DD) required")
		}
		day, err := time.Parse("2006-01-02", a.AsOf)
		if err != nil {
			return nil, fmt.Errorf("as_of must be YYYY-MM-DD")
		}
		cur, err := loadDataset(cfg.Published)
		if err != nil {
			return nil, err
		}
		var rows struct {
			Rows []Plan `json:"rows"`
		}
		if err := decode(cfg.Plans, &rows); err != nil {
			return nil, fmt.Errorf("plans unavailable")
		}
		return observations(cur, rows.Rows, day), nil
	}
	return nil, fmt.Errorf("unknown tool")
}

func loadDataset(path string) (Dataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Dataset{}, fmt.Errorf("snapshot unavailable")
	}
	return normalize(raw)
}
