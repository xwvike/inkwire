package scene

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// readmes are the schema reference in its two translations. They are kept as
// translations of one another, so an example that decodes in one and not the
// other is a translation that changed the document rather than the prose.
//
// The name is what it was when this file only had the READMEs to check; the
// node model moved to SCHEMA.md when the usage documents were cut down to what
// somebody needs in order to run the thing.
var readmes = []string{"../../SCHEMA.md", "../../SCHEMA.zh-CN.md"}

// pairs are the translated pairs this package checks against each other. The
// usage documents carry scene examples too, and an example that does not decode
// is just as wrong there.
var pairs = [][]string{
	readmes,
	{"../../README.md", "../../README.zh-CN.md"},
}

// The README documents this schema node by node, which means it is full of
// documents that either decode or are wrong. A reader who copies one and gets
// an error has been told something false by the thing that was meant to
// explain it, so every example is decoded here.
//
// Examples that elide part of their content with "..." are skipped: they are
// showing a shape, not offering a document.
func TestReadmeNodeExamplesDecode(t *testing.T) {
	total := 0
	for _, pair := range pairs {
		for _, path := range pair {
			t.Run(path, func(t *testing.T) {
				for _, body := range jsonBlocks(t, path) {
					var probe map[string]any
					if json.Unmarshal([]byte(body), &probe) != nil {
						continue
					}
					if _, isNode := probe["type"].(string); !isNode {
						continue
					}
					total++
					_, err := (Decoder{}).decodeNode([]byte(body), "readme")
					// An example is allowed to name a picture the repository
					// does not carry; that says nothing about whether the
					// document is well formed.
					if errors.Is(err, fs.ErrNotExist) {
						continue
					}
					if err != nil {
						t.Errorf("example does not decode: %v\n%s", err, body)
					}
				}
			})
		}
	}
	// If the extractor ever stops matching, it would pass by finding nothing.
	if total < 30 {
		t.Errorf("found only %d node examples across %d documents, which cannot be right",
			total, len(pairs)*2)
	}
}

// The two documents are the same document in two languages. Prose differs by
// definition; the examples must not, because an example is the one part a
// reader copies rather than reads, and a schema that differs between
// translations is two schemas.
func TestBothReadmesCarryTheSameExamples(t *testing.T) {
	for _, pair := range pairs {
		first := jsonBlocks(t, pair[0])
		for _, path := range pair[1:] {
			other := jsonBlocks(t, path)
			if len(other) != len(first) {
				t.Errorf("%s has %d examples, %s has %d", path, len(other), pair[0], len(first))
				continue
			}
			for index := range first {
				if other[index] != first[index] {
					t.Errorf("example %d differs between %s and %s:\n%s\n---\n%s",
						index, pair[0], path, first[index], other[index])
				}
			}
		}
	}
}

// Every heading in one has a counterpart in the other. Section counts drifting
// apart is how one language quietly stops documenting something.
func TestBothReadmesHaveTheSameSections(t *testing.T) {
	headings := regexp.MustCompile(`(?m)^(#{1,3}) `)
	for _, pair := range pairs {
		counts := map[string]int{}
		for _, path := range pair {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			counts[path] = len(headings.FindAllString(string(source), -1))
		}
		for _, path := range pair[1:] {
			if counts[path] != counts[pair[0]] {
				t.Errorf("%s has %d headings, %s has %d",
					path, counts[path], pair[0], counts[pair[0]])
			}
		}
	}
}

func jsonBlocks(t *testing.T, path string) []string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, block := range regexp.MustCompile("(?s)```json\n(.*?)```").FindAllStringSubmatch(string(source), -1) {
		body := strings.TrimSpace(block[1])
		if strings.Contains(body, "...") {
			continue
		}
		bodies = append(bodies, body)
	}
	return bodies
}

// The schema reference is a second copy of decode.go written in prose, and the
// two drifted the moment the document was rewritten: fit lost its default, the
// image options and overrides were flattened into fields that do not exist,
// sampling vanished, and every alignment field in row, column and grid was
// documented under a name the decoder would reject. Each of those reads as
// working — a reader copies it, the decoder refuses it, and the document is
// what was wrong.
//
// So the field names come from the struct tags rather than from anybody's
// memory of them. An earlier version of this test carried a hand-written list
// of names it would not check, which put the author's memory back in charge of
// the thing the test exists to take away from it; every tag is checked now.
func TestBothReadmesNameEveryDecodableField(t *testing.T) {
	source, err := os.ReadFile("decode.go")
	if err != nil {
		t.Fatal(err)
	}
	tags := map[string]bool{}
	for _, match := range regexp.MustCompile(`json:"(\w+)`).FindAllStringSubmatch(string(source), -1) {
		tags[match[1]] = true
	}
	if len(tags) < 80 {
		t.Fatalf("found %d field names, which cannot be right", len(tags))
	}
	for _, path := range readmes {
		doc, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// Only the table rows count. Naming a field anywhere in the document
		// used to be enough, and a JSON example satisfied it — which is how
		// `pattern` came to have a whole section with no property table at
		// all, its two fields "documented" by appearing in the example nobody
		// could look a type or a default up in.
		rows := tableRows(string(doc))
		for tag := range tags {
			// A field is named whether it is written bare or qualified, as
			// `children[].basis` and `shape.kind` are, so this matches the
			// name itself rather than the whole span it sits in.
			if !regexp.MustCompile("`[^`]*\\b" + regexp.QuoteMeta(tag) + "\\b[^`]*`").MatchString(rows) {
				t.Errorf("%s never gives the %q field a table row", path, tag)
			}
		}
	}
}

// Every value the decoder accepts anywhere it accepts a fixed set of them:
// node types, shape kinds, path operations, processing modes and the enums
// behind ink, alignment, fit, sampling and dithering.
//
// An earlier pair of tests found these by looking for the two shapes the author
// remembered writing — enumError calls, and the switch inside decodeNode. Shape
// kinds and path operations use neither, so they went unchecked, which is the
// same failure the tests were written to stop. What actually makes a set of
// values the accepted set is that the decoder refuses everything else, so that
// is what is looked for here: a switch whose default branch returns an error.
func TestBothReadmesDocumentEveryAcceptedValue(t *testing.T) {
	source, err := os.ReadFile("decode.go")
	if err != nil {
		t.Fatal(err)
	}
	sets := acceptedValues(strings.Split(string(source), "\n"))
	// Three of these sets left this file when the words moved next to the
	// names they produce, so that a report saying "cover" and a scene asking
	// for "cover" read the same list. They come from the functions that hold
	// that list now, which is a better source than a regular expression over a
	// switch — it is the list itself rather than a shape somebody wrote it in.
	sets = append(sets,
		valueSet{subject: "display.ImageFitNames()", values: display.ImageFitNames()},
		valueSet{subject: "display.SamplingModeNames()", values: display.SamplingModeNames()},
		valueSet{subject: "display.DitherModeNames()", values: display.DitherModeNames()},
	)
	total := 0
	for _, set := range sets {
		total += len(set.values)
	}
	if len(sets) < 14 || total < 60 {
		t.Fatalf("found %d value sets holding %d values, which cannot be right", len(sets), total)
	}

	for _, path := range readmes {
		doc, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, set := range sets {
			for _, value := range set.values {
				// Values are written either as prose in backticks or as a
				// string in an example, and both count as documenting one.
				// Anything looser would pass on a word that merely appears.
				if strings.Contains(string(doc), "`"+value+"`") ||
					strings.Contains(string(doc), `"`+value+`"`) {
					continue
				}
				t.Errorf("%s never documents %q, accepted by %s", path, value, set.subject)
			}
		}
	}
}

// valueSet is one switch's worth of accepted values. The parse functions all
// switch on a variable called `value`, so these are kept in order rather than
// keyed by the expression, which would collapse ten of them into one.
type valueSet struct {
	subject string
	line    int
	values  []string
}

// acceptedValues returns the case values of every switch whose default branch
// returns an error. A switch that falls through to a default doing something
// other than refusing the value is not enumerating a schema, so it is left out.
func acceptedValues(lines []string) []valueSet {
	opening := regexp.MustCompile(`^\s*switch (.+) \{\s*$`)
	label := regexp.MustCompile(`^\s*case ("[^:]*):`)
	quoted := regexp.MustCompile(`"([^"]*)"`)

	var sets []valueSet
	for index := 0; index < len(lines); index++ {
		header := opening.FindStringSubmatch(lines[index])
		if header == nil {
			continue
		}
		var values []string
		refuses, depth, cursor := false, 1, index+1
		for ; cursor < len(lines); cursor++ {
			depth += strings.Count(lines[cursor], "{") - strings.Count(lines[cursor], "}")
			if depth <= 0 {
				break
			}
			if match := label.FindStringSubmatch(lines[cursor]); match != nil {
				for _, value := range quoted.FindAllStringSubmatch(match[1], -1) {
					// The empty case is how a field says it is optional, and
					// carries no value a reader could write down.
					if value[1] != "" {
						values = append(values, value[1])
					}
				}
			}
			if strings.HasPrefix(strings.TrimSpace(lines[cursor]), "default:") &&
				strings.Contains(strings.Join(lines[cursor:min(cursor+4, len(lines))], " "), "rror") {
				refuses = true
			}
		}
		if refuses && len(values) > 0 {
			sets = append(sets, valueSet{subject: header[1], line: index + 1, values: values})
		}
		index = cursor
	}
	return sets
}

// Defaults are the part of a schema a reader trusts without testing: nobody
// sends `"sampling": "nearest"` to check whether omitting it would have meant
// the same thing. The document claimed bilinear where the decoder means
// nearest, one row below a default that had just been corrected by hand — which
// is the whole argument for deriving these rather than reading them.
//
// A field's default is the parse function's empty-string branch only when the
// decoder actually reaches that branch on an absent key. Pointer fields do not:
// they are guarded by a nil check and default to something else entirely, as
// `background` defaults to white while parseInk's empty branch is black. The
// pointer star at the call site is what separates the two cases, so it decides
// which fields this test claims to know the default of.
func TestBothReadmesDocumentEveryDefault(t *testing.T) {
	source, err := os.ReadFile("decode.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)

	zeroValue := map[string]string{}
	for _, match := range regexp.MustCompile(`(?s)func (parse\w+)\(value string\).*?\n}`).FindAllStringSubmatch(text, -1) {
		if fallback := regexp.MustCompile(`case "", "([^"]+)"`).FindStringSubmatch(match[0]); fallback != nil {
			zeroValue[match[1]] = fallback[1]
		}
	}
	// The three image parsers delegate now, so what an omitted field means is
	// no longer written in this file. It is asked of the function instead,
	// which answers with the value rather than with the shape it was spelled in.
	fit, _ := display.ParseImageFit("")
	sampling, _ := display.ParseSamplingMode("")
	dither, _ := display.ParseDitherMode("")
	zeroValue["parseFit"] = fit.String()
	zeroValue["parseSampling"] = sampling.String()
	zeroValue["parseDither"] = dither.String()
	tags := map[string]string{}
	for _, match := range regexp.MustCompile("(?m)^\\s+(\\w+)\\s+[\\w\\[\\]*.]+\\s+`json:\"(\\w+)").FindAllStringSubmatch(text, -1) {
		tags[match[1]] = match[2]
	}
	want := map[string]string{}
	for _, match := range regexp.MustCompile(`(parse\w+)\((\*?)(\w+)\.(\w+)\)`).FindAllStringSubmatch(text, -1) {
		fallback, known := zeroValue[match[1]]
		if !known || match[2] == "*" {
			continue
		}
		if tag, named := tags[match[4]]; named {
			want[tag] = fallback
		}
	}
	if len(want) < 10 {
		t.Fatalf("derived %d defaults, which cannot be right", len(want))
	}

	for _, path := range readmes {
		doc, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for field, fallback := range want {
			documented := false
			for _, line := range strings.Split(string(doc), "\n") {
				if !strings.HasPrefix(line, "|") {
					continue
				}
				cells := strings.Split(strings.Trim(line, "|"), "|")
				if len(cells) < 2 {
					continue
				}
				// Fields sharing a default share a row, as `alignItems` and
				// `justifySelf` do, so the name is looked for within the first
				// cell rather than being the whole of it.
				if !strings.Contains(cells[0], "`"+field+"`") {
					continue
				}
				documented = true
				if got := strings.Trim(strings.TrimSpace(cells[len(cells)-1]), "`"); got != fallback {
					t.Errorf("%s says %s defaults to %q, decoder uses %q", path, field, got, fallback)
				}
			}
			if !documented {
				t.Errorf("%s has no table row for %s (defaults to %q)", path, field, fallback)
			}
		}
	}
}

// tableRows is the document with everything but its table rows removed, so a
// field named only in prose or in an example does not count as documented.
func tableRows(doc string) string {
	var rows []string
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			rows = append(rows, line)
		}
	}
	return strings.Join(rows, "\n")
}
