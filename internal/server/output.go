package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
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

func formatOutput(out string, limit int) string { return formatOutputAt(out, limit, 0) }

func formatOutputAt(out string, limit, offset int) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return "(no output)"
	}
	if compact, ok := compactJSON(out); ok {
		out = compact
	}
	offset = max(offset, 0)
	if strings.HasPrefix(out, "[") {
		var items []json.RawMessage
		if json.Unmarshal([]byte(out), &items) == nil {
			if offset == 0 && len(out) <= limit {
				return out
			}
			if offset >= len(items) {
				return fmt.Sprintf("[]\n\n[PAGE] offset %d is past the end; the result has %d items.", offset, len(items))
			}
			return truncateArray(items, offset, len(out), limit)
		}
	}
	if offset == 0 && len(out) <= limit {
		return out
	}
	return truncateText(out, offset, limit)
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

func truncateArray(all []json.RawMessage, offset, total, limit int) string {
	items := all[offset:]
	budget := limit - 2 // brackets
	var kept []json.RawMessage
	used := 0
	for _, it := range items {
		need := len(it)
		if len(kept) > 0 {
			need++ // comma
		}
		if used+need > budget && len(kept) > 0 {
			break
		}
		used += need
		kept = append(kept, it)
	}
	body, _ := json.Marshal(kept)
	end := offset + len(kept)
	if end >= len(all) {
		if offset == 0 {
			return string(body)
		}
		return fmt.Sprintf("%s\n\n[PAGE] Showing items %d-%d of %d (end of result).", body, offset+1, end, len(all))
	}
	return fmt.Sprintf("%s\n\n[TRUNCATED] Showing %d of %d items (items %d-%d, %d of %d chars). Call again with offset=%d for the next page. %s Or raise maxChars (max %d).",
		body, len(kept), len(all), offset+1, end, len(body), total, end, narrowHint, maxMaxChars)
}

func truncateText(out string, offset, limit int) string {
	for offset < len(out) && !utf8.RuneStart(out[offset]) {
		offset++
	}
	if offset >= len(out) {
		return fmt.Sprintf("(no output)\n\n[PAGE] offset %d is past the end; the result has %d chars.", offset, len(out))
	}
	rest := out[offset:]
	if len(rest) <= limit {
		if offset == 0 {
			return rest
		}
		return fmt.Sprintf("%s\n\n[PAGE] Showing chars %d-%d of %d (end of result).", rest, offset, len(out), len(out))
	}
	cut := rest[:limit]
	if i := strings.LastIndexByte(cut, '\n'); i > limit/2 {
		cut = cut[:i]
	}
	cut = strings.ToValidUTF8(cut, "")
	return fmt.Sprintf("%s\n\n[TRUNCATED] Showing %d of %d chars (%d of %d lines). Call again with offset=%d for the next page. %s Or raise maxChars (max %d).",
		cut, len(cut), len(out), strings.Count(cut, "\n")+1, strings.Count(out, "\n")+1, offset+len(cut), narrowHint, maxMaxChars)
}
