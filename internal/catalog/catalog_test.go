package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func testCatalog() *Catalog {
	return &Catalog{Version: "1.2.3", Commands: []Cmdlet{
		{Name: "Add-PnPListItem", Synopsis: "Adds an item to the list."},
		{Name: "Get-PnPList", Synopsis: "Returns lists from SharePoint."},
		{Name: "Get-PnPTeamsChannel", Synopsis: "Returns channels of a team."},
	}}
}

func TestFind(t *testing.T) {
	c := testCatalog()
	if got := c.Find("get-pnplist"); got == nil || got.Name != "Get-PnPList" {
		t.Fatalf("case-insensitive find failed: %v", got)
	}
	if c.Find("Get-PnPNope") != nil {
		t.Fatal("expected nil for unknown cmdlet")
	}
}

func TestSearch(t *testing.T) {
	c := testCatalog()
	tests := []struct {
		name, query string
		limit, want int
		first       string
	}{
		{"ranked", "list item", 10, 2, "Add-PnPListItem"},
		{"single", "channel", 10, 1, "Get-PnPTeamsChannel"},
		{"limit", "list", 1, 1, ""},
		{"none", "zzzz", 10, 0, ""},
		{"empty lists all", "", 10, 3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Search(tt.query, tt.limit)
			if len(got) != tt.want {
				t.Fatalf("got %d results %v, want %d", len(got), got, tt.want)
			}
			if tt.first != "" && got[0].Name != tt.first {
				t.Errorf("first = %s, want %s", got[0].Name, tt.first)
			}
		})
	}
}

func TestShorten(t *testing.T) {
	tests := map[string]string{
		"Returns lists. More detail here.":  "Returns lists.",
		"Adds an item <a href='x'>link</a>": "Adds an item",
		"  spaced  ":                        "spaced",
		string(make([]byte, 0)):             "",
	}
	for in, want := range tests {
		if got := shorten(in); got != want {
			t.Errorf("shorten(%q) = %q, want %q", in, got, want)
		}
	}
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	if got := shorten(string(long)); len(got) != 200+len("…") {
		t.Errorf("long input not truncated: len %d", len(got))
	}
}

func TestLoadSynopses(t *testing.T) {
	const xml = `<?xml version="1.0"?>
<helpItems xmlns:command="c" xmlns:maml="m">
 <command:command><command:details>
   <command:name>Get-PnPList</command:name>
   <maml:description><maml:para>Returns lists from SharePoint. Extra.</maml:para></maml:description>
 </command:details></command:command>
 <command:command><command:details>
   <command:name>Add-PnPField</command:name>
   <maml:description><maml:para>Adds a field</maml:para></maml:description>
 </command:details></command:command>
</helpItems>`
	p := filepath.Join(t.TempDir(), "help.xml")
	if err := os.WriteFile(p, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadSynopses(p)
	if got["get-pnplist"] != "Returns lists from SharePoint." || got["add-pnpfield"] != "Adds a field" {
		t.Errorf("unexpected synopses: %v", got)
	}
	if n := len(loadSynopses(filepath.Join(t.TempDir(), "missing.xml"))); n != 0 {
		t.Errorf("missing file should yield empty map, got %d", n)
	}
}

func TestSearchWithFiltersAndRanking(t *testing.T) {
	c := &Catalog{Commands: []Cmdlet{
		{Name: "Get-PnPListItem", Synopsis: "Retrieves list items."},
		{Name: "Get-PnPList", Synopsis: "Returns lists."},
		{Name: "Remove-PnPList", Synopsis: "Deletes a list."},
		{Name: "Get-PnPField", Synopsis: "Returns columns.", Parameters: []Param{{Name: "List"}}},
	}}
	if got := c.SearchWith(SearchOptions{Query: "list", Limit: 10}); got[0].Name != "Get-PnPList" && got[0].Name != "Remove-PnPList" {
		t.Errorf("exact noun should rank first, got %s", got[0].Name)
	}
	got := c.SearchWith(SearchOptions{Verb: "get", Noun: "PnPList", Limit: 10})
	if len(got) != 1 || got[0].Name != "Get-PnPList" {
		t.Errorf("verb/noun filter: %v", got)
	}
	if got := c.SearchWith(SearchOptions{Query: "list", Limit: 10}); got[len(got)-1].Name != "Get-PnPField" {
		t.Errorf("parameter-only match should rank last, got %v", got)
	}
}

func TestRelated(t *testing.T) {
	c := &Catalog{Commands: []Cmdlet{{Name: "Get-PnPList"}, {Name: "Set-PnPList"}, {Name: "Get-PnPListItem"}, {Name: "Get-PnPSite"}}}
	got := c.Related("Get-PnPList", 5)
	if len(got) != 2 || got[0] != "Set-PnPList" || got[1] != "Get-PnPListItem" {
		t.Errorf("related: %v", got)
	}
}
