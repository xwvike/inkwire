// Command completions writes the vocabulary the editor completes from.
//
// A stock CSS completion offers every property a browser has. This renderer
// implements a fraction of them, so a stock list would spend most of its
// suggestions proposing declarations that compile to a warning — teaching the
// wrong vocabulary at exactly the moment someone is learning it.
//
// The right list is the one in MARKUP.md, which a test already holds to naming
// every implemented property. Reading it here rather than keeping a copy means
// a property added to the renderer reaches the editor when the manual does,
// and the two can never disagree without CI saying so.
//
//	go run ./web/tools/completions -o web/static/completions.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

type property struct {
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Values   []string `json:"values,omitempty"`
	Syntax   string   `json:"syntax"`
	Notes    string   `json:"notes,omitempty"`
}

type vocabulary struct {
	Properties  []property `json:"properties"`
	SVGElements []string   `json:"svgElements"`
}

var (
	backticked = regexp.MustCompile("`([^`]+)`")
	keyword    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	// Units and placeholders read as keywords but are not values anyone can
	// type on their own. A completion that offered "px" as a value would be
	// proposing an incomplete declaration.
	notAValue = map[string]bool{
		// Units. A cell listing the angle units a property takes reads as a
		// list of keywords, and completing "rotate: grad" proposes exactly the
		// declaration this vocabulary exists to stop being proposed.
		"px": true, "em": true, "rem": true, "fr": true,
		"deg": true, "grad": true, "rad": true, "turn": true,
		// Placeholders standing for a value rather than being one.
		"number": true, "size": true, "family": true, "line-height": true,
	}
)

func main() {
	in := flag.String("i", "MARKUP.md", "manual to read the table from")
	out := flag.String("o", "", "file to write (default stdout)")
	flag.Parse()

	source, err := os.ReadFile(*in)
	if err != nil {
		fail(err)
	}

	vocab := vocabulary{
		Properties:  properties(string(source)),
		SVGElements: svgElements(string(source)),
	}
	if len(vocab.Properties) == 0 {
		fail(fmt.Errorf("%s: found no property table; has its format changed?", *in))
	}

	encoded, err := json.MarshalIndent(vocab, "", "  ")
	if err != nil {
		fail(err)
	}
	encoded = append(encoded, '\n')
	if *out == "" {
		os.Stdout.Write(encoded)
		return
	}
	if err := os.WriteFile(*out, encoded, 0o644); err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d properties, %d svg elements)\n",
		*out, len(vocab.Properties), len(vocab.SVGElements))
}

// properties reads the manual's one table of implemented properties. Each row
// names a category, the properties that share a rule, what they accept, and
// what is worth knowing about them; all four are worth having at the cursor.
func properties(source string) []property {
	seen := map[string]bool{}
	var found []property
	for _, line := range strings.Split(source, "\n") {
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) < 4 {
			continue
		}
		category := strings.TrimSpace(cells[0])
		names := backticked.FindAllStringSubmatch(cells[1], -1)
		if category == "Category" || len(names) == 0 {
			continue
		}
		syntax := strings.TrimSpace(cells[2])
		notes := strings.TrimSpace(cells[3])
		if notes == "—" {
			notes = ""
		}
		values := valuesIn(syntax)
		for _, name := range names {
			if seen[name[1]] || !keyword.MatchString(name[1]) {
				continue
			}
			seen[name[1]] = true
			found = append(found, property{
				Name:     name[1],
				Category: category,
				Values:   values,
				Syntax:   plain(syntax),
				Notes:    plain(notes),
			})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })

	// The paint properties say "Supported inks" rather than listing them, so
	// the names are only written once. They are a closed set, and the colour
	// row is where it is written down.
	var inks []string
	for _, candidate := range found {
		if candidate.Name == "color" {
			inks = candidate.Values
			break
		}
	}
	for i := range found {
		if !strings.Contains(strings.ToLower(found[i].Syntax), "supported inks") {
			continue
		}
		// A new slice: every property on a row shares one, and appending to
		// it in place would rewrite its siblings'.
		found[i].Values = append(append([]string{}, inks...), found[i].Values...)
	}
	return found
}

// valuesIn pulls the keywords out of an accepted-values cell. What is left
// after the units and placeholders are dropped is the set a person can type
// literally, which is the set worth completing.
// Where a row covers several properties whose sets differ, the difference is
// written as a clause after a semicolon — "align-self and justify-self also
// accept auto". Attributing that to the right property means reading English,
// so the clause is dropped instead. Everything offered is then accepted by
// every property on the row: a completion may be missing a value, but it never
// proposes one that compiles to a warning.
func valuesIn(cell string) []string {
	if clause := strings.Index(cell, ";"); clause != -1 {
		cell = cell[:clause]
	}
	seen := map[string]bool{}
	var values []string
	for _, match := range backticked.FindAllStringSubmatch(cell, -1) {
		value := match[1]
		if !keyword.MatchString(value) || notAValue[value] || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}

// svgElements reads the sentence naming what inline SVG may contain. Unlike
// HTML, where an unknown element is still a box, an unsupported SVG element
// draws nothing, so completing the wrong one is a silent blank.
func svgElements(source string) []string {
	const marker = "Supported elements are "
	start := strings.Index(source, marker)
	if start == -1 {
		return nil
	}
	rest := source[start+len(marker):]
	if stop := strings.Index(rest, "."); stop != -1 {
		rest = rest[:stop]
	}
	var elements []string
	for _, match := range backticked.FindAllStringSubmatch(rest, -1) {
		if keyword.MatchString(match[1]) {
			elements = append(elements, match[1])
		}
	}
	sort.Strings(elements)
	return elements
}

// plain strips the manual's backticks so a tooltip reads as a sentence.
func plain(text string) string { return strings.ReplaceAll(text, "`", "") }

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
