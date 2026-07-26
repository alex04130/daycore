package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func rawOr(raw json.RawMessage, def string) string {
	if len(raw) == 0 || string(raw) == "null" {
		return def
	}
	return string(raw)
}

// extractJSONObject pulls the first {...} object out of a model response (which
// may be wrapped in code fences) and decodes it.
func extractJSONObject(s string) (map[string]any, bool) {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s[start:end+1]), &m); err != nil {
		return nil, false
	}
	return m, true
}

// attachBlocks gives every block a stable id and a fallback date, matching v1.
func attachBlocks(result map[string]any, fallbackDate string) {
	blocks, ok := result["blocks"].([]any)
	if !ok {
		return
	}
	base := time.Now().UnixNano()
	for i, ba := range blocks {
		bm, ok := ba.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := bm["id"].(string); id == "" {
			bm["id"] = fmt.Sprintf("block-%d-%d", base, i)
		}
		if d, _ := bm["date"].(string); d == "" && fallbackDate != "" {
			bm["date"] = fallbackDate
		}
	}
}
