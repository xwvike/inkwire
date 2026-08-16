package scene

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

// readmes are the two documents describing this schema. They are kept as
// translations of one another, so an example that decodes in one and not the
// other is a translation that changed the document rather than the prose.
var readmes = []string{"../../README.md", "../../README.zh-CN.md"}

// The README documents this schema node by node, which means it is full of
// documents that either decode or are wrong. A reader who copies one and gets
// an error has been told something false by the thing that was meant to
// explain it, so every example is decoded here.
//
// Examples that elide part of their content with "..." are skipped: they are
// showing a shape, not offering a document.
func TestReadmeNodeExamplesDecode(t *testing.T) {
	for _, path := range readmes {
		t.Run(path, func(t *testing.T) {
			checked := 0
			for _, body := range jsonBlocks(t, path) {
				var probe map[string]any
				if json.Unmarshal([]byte(body), &probe) != nil {
					continue
				}
				if _, isNode := probe["type"].(string); !isNode {
					continue
				}
				checked++
				_, err := (Decoder{}).decodeNode([]byte(body), "readme")
				// An example is allowed to name a picture the repository does
				// not carry; that says nothing about whether the document is
				// well formed.
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					t.Errorf("example does not decode: %v\n%s", err, body)
				}
			}
			// If the extractor ever stops matching, it would pass by finding
			// nothing.
			if checked < 15 {
				t.Errorf("found only %d node examples, which cannot be right", checked)
			}
		})
	}
}

// The two documents are the same document in two languages. Prose differs by
// definition; the examples must not, because an example is the one part a
// reader copies rather than reads, and a schema that differs between
// translations is two schemas.
func TestBothReadmesCarryTheSameExamples(t *testing.T) {
	first := jsonBlocks(t, readmes[0])
	for _, path := range readmes[1:] {
		other := jsonBlocks(t, path)
		if len(other) != len(first) {
			t.Fatalf("%s has %d examples, %s has %d", path, len(other), readmes[0], len(first))
		}
		for index := range first {
			if other[index] != first[index] {
				t.Errorf("example %d differs between %s and %s:\n%s\n---\n%s",
					index, readmes[0], path, first[index], other[index])
			}
		}
	}
}

// Every heading in one has a counterpart in the other. Section counts drifting
// apart is how one language quietly stops documenting something.
func TestBothReadmesHaveTheSameSections(t *testing.T) {
	headings := regexp.MustCompile(`(?m)^(#{1,3}) `)
	counts := map[string]int{}
	for _, path := range readmes {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		counts[path] = len(headings.FindAllString(string(source), -1))
	}
	for _, path := range readmes[1:] {
		if counts[path] != counts[readmes[0]] {
			t.Errorf("%s has %d headings, %s has %d",
				path, counts[path], readmes[0], counts[readmes[0]])
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
		for tag := range tags {
			// A field is named whether it is written bare or qualified, as
			// `children[].basis` and `shape.kind` are, so this matches the
			// name itself rather than the whole span it sits in.
			if !regexp.MustCompile(`\b` + regexp.QuoteMeta(tag) + `\b`).MatchString(string(doc)) {
				t.Errorf("%s never names the %q field", path, tag)
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
				t.Errorf("%s never documents %q, accepted by the switch on %s at decode.go:%d",
					path, value, set.subject, set.line)
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
