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

func (c *Catalog) Search(query string, limit int) []Cmdlet {
	terms := strings.Fields(strings.ToLower(query))
	type hit struct {
		cmdlet Cmdlet
		score  int
	}
	var hits []hit
	for _, cmd := range c.Commands {
		name, synopsis := strings.ToLower(cmd.Name), strings.ToLower(cmd.Synopsis)
		score := 0
		for _, t := range terms {
			if strings.Contains(name, t) {
				score += 3
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

	out := make([]Cmdlet, 0, min(limit, len(hits)))
	for _, h := range hits[:min(limit, len(hits))] {
		out = append(out, h.cmdlet)
	}
	return out
}
