package catalog

import (
	"encoding/xml"
	"os"
	"strings"
)

func loadSynopses(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()

	var (
		dec                       = xml.NewDecoder(f)
		inDetails, inName, inDesc bool
		name                      string
		desc                      strings.Builder
	)
	for {
		tok, err := dec.Token()
		if err != nil { // io.EOF ends the document; other errors yield partial results
			return out
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch {
			case e.Name.Local == "details":
				inDetails, name = true, ""
				desc.Reset()
			case inDetails && e.Name.Local == "name":
				inName = true
			case inDetails && e.Name.Local == "description":
				inDesc = true
			}
		case xml.CharData:
			text := strings.TrimSpace(string(e))
			switch {
			case inName:
				name += text
			case inDesc:
				desc.WriteString(text + " ")
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "name":
				inName = false
			case "description":
				inDesc = false
			case "details":
				inDetails = false
				if name != "" {
					out[strings.ToLower(name)] = shorten(desc.String())
				}
			}
		}
	}
}

func shorten(d string) string {
	d, _, _ = strings.Cut(d, "<")
	d = strings.TrimSpace(d)
	if before, _, found := strings.Cut(d, ". "); found {
		d = before + "."
	}
	if len(d) > 200 {
		d = d[:200] + "…"
	}
	return d
}
