package catalog

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

type Param struct {
	Name      string   `json:"name"`
	Aliases   []string `json:"aliases"`
	Type      string   `json:"type"`
	Mandatory bool     `json:"mandatory"`
}

type Cmdlet struct {
	Name          string   `json:"name"`
	Synopsis      string   `json:"synopsis,omitempty"`
	Parameters    []Param  `json:"parameters"`
	ParameterSets []string `json:"parameterSets"`
}

type Catalog struct {
	Version   string    `json:"version"`
	HelpPath  string    `json:"helpPath"`
	Commands  []Cmdlet  `json:"commands"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (c *Catalog) Find(name string) *Cmdlet {
	i := slices.IndexFunc(c.Commands, func(x Cmdlet) bool { return strings.EqualFold(x.Name, name) })
	if i < 0 {
		return nil
	}
	return &c.Commands[i]
}

type SearchOptions struct {
	Query string
	Verb  string
	Noun  string
	Limit int
}

func SplitName(name string) (verb, noun string) {
	verb, noun, _ = strings.Cut(name, "-")
	if len(noun) >= 3 && strings.EqualFold(noun[:3], "pnp") {
		noun = noun[3:]
	}
	return verb, noun
}

func (c *Catalog) Search(query string, limit int) []Cmdlet {
	return c.SearchWith(SearchOptions{Query: query, Limit: limit})
}

func (c *Catalog) SearchWith(o SearchOptions) []Cmdlet {
	terms := strings.Fields(strings.ToLower(o.Query))
	wantVerb := strings.ToLower(strings.TrimSpace(o.Verb))
	wantNoun := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(o.Noun)), "pnp")
	type hit struct {
		cmdlet Cmdlet
		score  int
	}
	var hits []hit
	for _, cmd := range c.Commands {
		name, synopsis := strings.ToLower(cmd.Name), strings.ToLower(cmd.Synopsis)
		verb, noun := SplitName(name)
		if wantVerb != "" && verb != wantVerb {
			continue
		}
		if wantNoun != "" && noun != wantNoun {
			continue
		}
		score := 0
		for _, t := range terms {
			switch {
			case noun == t || verb == t:
				score += 6
			case strings.Contains(name, t):
				score += 3
			case hasParam(cmd, t):
				score++
			}
			if strings.Contains(synopsis, t) {
				score++
			}
		}
		if len(terms) == 0 || score > 0 {
			hits = append(hits, hit{cmd, score})
		}
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return cmp.Compare(b.score, a.score) })

	n := min(o.Limit, len(hits))
	out := make([]Cmdlet, 0, n)
	for _, h := range hits[:n] {
		out = append(out, h.cmdlet)
	}
	return out
}

func hasParam(cmd Cmdlet, term string) bool {
	return slices.ContainsFunc(cmd.Parameters, func(p Param) bool {
		return strings.Contains(strings.ToLower(p.Name), term)
	})
}

func (c *Catalog) Related(name string, limit int) []string {
	_, noun := SplitName(strings.ToLower(name))
	if noun == "" {
		return nil
	}
	var same, extended []string
	for _, cmd := range c.Commands {
		if strings.EqualFold(cmd.Name, name) {
			continue
		}
		_, n := SplitName(strings.ToLower(cmd.Name))
		switch {
		case n == noun:
			same = append(same, cmd.Name)
		case strings.HasPrefix(n, noun):
			extended = append(extended, cmd.Name)
		}
	}
	all := append(same, extended...)
	return all[:min(limit, len(all))]
}
