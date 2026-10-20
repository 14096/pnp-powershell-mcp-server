package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	defaultMaxChars = 30000
	minMaxChars     = 1000
	maxMaxChars     = 200000
)

const narrowHint = "Narrow the query: filter server-side (-Identity, -Query, -Filter), limit with -First/-PageSize, or pick fewer properties with Select-Object."

func clampMaxChars(n int) int {
	switch {
	case n <= 0:
		return defaultMaxChars
	case n < minMaxChars:
		return minMaxChars
	case n > maxMaxChars:
		return maxMaxChars
	}
	return n
}

// formatOutput prepares command output for an LLM: valid JSON is compacted to
// save tokens, and output over limit bytes is cut with an explanatory notice.
// JSON arrays are cut at element boundaries so the result stays valid JSON.
func formatOutput(out string, limit int) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return "(no output)"
	}
	if compact, ok := compactJSON(out); ok {
		out = compact
	}
	if len(out) <= limit {
		return out
	}
	if strings.HasPrefix(out, "[") {
		var items []json.RawMessage
		if json.Unmarshal([]byte(out), &items) == nil {
			return truncateArray(items, len(out), limit)
		}
	}
	return truncateText(out, limit)
}

func compactJSON(s string) (string, bool) {
	if s[0] != '{' && s[0] != '[' {
		return "", false
	}
	var buf bytes.Buffer
	if json.Compact(&buf, []byte(s)) != nil {
		return "", false
	}
	return buf.String(), true
}

func truncateArray(items []json.RawMessage, total, limit int) string {
	budget := limit - 2 // brackets
	var kept []json.RawMessage
	used := 0
	for _, it := range items {
		need := len(it)
		if len(kept) > 0 {
			need++ // comma
		}
		if used+need > budget {
			break
		}
		used += need
		kept = append(kept, it)
	}
	body, _ := json.Marshal(kept)
	return fmt.Sprintf("%s\n\n[TRUNCATED] Showing %d of %d items (%d of %d chars). %s Or raise maxChars (max %d).",
		body, len(kept), len(items), len(body), total, narrowHint, maxMaxChars)
}

func truncateText(out string, limit int) string {
	cut := out[:limit]
	if i := strings.LastIndexByte(cut, '\n'); i > limit/2 {
		cut = cut[:i]
	}
	cut = strings.ToValidUTF8(cut, "")
	return fmt.Sprintf("%s\n\n[TRUNCATED] Showing %d of %d chars (%d of %d lines). %s Or raise maxChars (max %d).",
		cut, len(cut), len(out), strings.Count(cut, "\n")+1, strings.Count(out, "\n")+1, narrowHint, maxMaxChars)
}
