package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatOutputEmpty(t *testing.T) {
	if got := formatOutput("  \n", 100); got != "(no output)" {
		t.Errorf("got %q", got)
	}
}

func TestFormatOutputCompactsJSON(t *testing.T) {
	in := "{\n  \"Title\": \"A\",\n  \"Id\": 1\n}\n"
	if got := formatOutput(in, 1000); got != `{"Title":"A","Id":1}` {
		t.Errorf("got %q", got)
	}
}

func TestFormatOutputPlainTextUntouchedWhenSmall(t *testing.T) {
	in := "Title  Id\n-----  --\nA      1"
	if got := formatOutput(in, 1000); got != in {
		t.Errorf("got %q", got)
	}
}

func TestFormatOutputTruncatesJSONArrayAtItemBoundary(t *testing.T) {
	var items []map[string]any
	for i := 0; i < 100; i++ {
		items = append(items, map[string]any{"Title": strings.Repeat("x", 20), "Id": i})
	}
	b, _ := json.MarshalIndent(items, "", "  ")

	got := formatOutput(string(b), 500)
	body, notice, found := strings.Cut(got, "\n\n[TRUNCATED]")
	if !found {
		t.Fatalf("expected truncation notice: %s", got)
	}
	var kept []map[string]any
	if err := json.Unmarshal([]byte(body), &kept); err != nil {
		t.Fatalf("truncated body must stay valid JSON: %v\n%s", err, body)
	}
	if len(kept) == 0 || len(kept) >= 100 || len(body) > 500 {
		t.Errorf("kept %d items, body %d bytes", len(kept), len(body))
	}
	if !strings.Contains(notice, "of 100 items") || !strings.Contains(notice, "-First") {
		t.Errorf("notice: %s", notice)
	}
}

func TestFormatOutputTruncatesTextAtLine(t *testing.T) {
	in := strings.Repeat("line of text\n", 200)
	got := formatOutput(in, 100)
	body, notice, found := strings.Cut(got, "\n\n[TRUNCATED]")
	if !found || len(body) > 100 || strings.HasSuffix(body, "lin") {
		t.Fatalf("unexpected: %q", got)
	}
	if !strings.Contains(notice, "of 2599 chars") {
		t.Errorf("notice: %s", notice)
	}
}

func TestFormatOutputKeepsUTF8Valid(t *testing.T) {
	got := formatOutput(strings.Repeat("é", 500), 101) // 101 bytes splits a rune
	body, _, _ := strings.Cut(got, "\n\n[TRUNCATED]")
	if strings.ContainsRune(body, '\uFFFD') {
		t.Error("cut a multi-byte rune")
	}
}

func TestClampMaxChars(t *testing.T) {
	for in, want := range map[int]int{0: defaultMaxChars, -5: defaultMaxChars, 10: minMaxChars, 5000: 5000, 1 << 30: maxMaxChars} {
		if got := clampMaxChars(in); got != want {
			t.Errorf("clamp(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestFormatOutputAtPagesJSONArray(t *testing.T) {
	var items []int
	for i := 0; i < 100; i++ {
		items = append(items, 100000+i)
	}
	b, _ := json.Marshal(items)
	first := formatOutputAt(string(b), 100, 0)
	if !strings.Contains(first, "offset=") {
		t.Fatalf("missing next offset: %s", first)
	}
	body, _, _ := strings.Cut(first, "\n\n")
	var page []int
	if err := json.Unmarshal([]byte(body), &page); err != nil || page[0] != 100000 {
		t.Fatalf("bad first page: %v %s", err, body)
	}
	n := len(page)
	next := formatOutputAt(string(b), 100, n)
	body, _, _ = strings.Cut(next, "\n\n")
	if err := json.Unmarshal([]byte(body), &page); err != nil || page[0] != 100000+n {
		t.Fatalf("bad second page: %v %s", err, body)
	}
	last := formatOutputAt(string(b), 100, 95)
	if !strings.Contains(last, "end of result") {
		t.Errorf("last page: %s", last)
	}
	if got := formatOutputAt(string(b), 100, 500); !strings.Contains(got, "past the end") {
		t.Errorf("past end: %s", got)
	}
}

func TestFormatOutputAtPagesText(t *testing.T) {
	in := strings.Repeat("line of text\n", 50)
	got := formatOutputAt(in, 100, 400)
	if !strings.Contains(got, "offset=") {
		t.Errorf("expected continuation offset: %s", got)
	}
	if got := formatOutputAt(in, 1000, 600); !strings.Contains(got, "end of result") {
		t.Errorf("expected end of result: %s", got)
	}
}
