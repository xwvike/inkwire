package markup

import (
	"fmt"
	"sort"
	"strings"

	"github.com/andybalholm/cascadia"
	"github.com/aymerick/douceur/parser"
	"golang.org/x/net/html"
)

// rule is one selector paired with the declarations it carries. Selectors are
// kept apart even when written as a list, because each has its own
// specificity.
type rule struct {
	selector    cascadia.Sel
	order       int
	declaration []declaration
}

type declaration struct {
	property  string
	value     string
	important bool
}

// specificity is compared as CSS compares it, with document order breaking a
// tie, so a later rule of equal weight wins.
func (r rule) less(other rule) bool {
	mine, theirs := r.selector.Specificity(), other.selector.Specificity()
	for i := range mine {
		if mine[i] != theirs[i] {
			return mine[i] < theirs[i]
		}
	}
	return r.order < other.order
}

type stylesheet struct {
	rules []rule
	// variables holds custom properties by the selector that declared them,
	// so a value can be looked up for the element it applies to.
	variables []rule
}

// substitute replaces every var(--name) in a value with what the matching
// custom properties declared, and reports a name nothing declared.
func (s *stylesheet) substitute(value string, declared map[string]string) (string, string) {
	for {
		start := strings.Index(value, "var(")
		if start < 0 {
			return value, ""
		}
		end := strings.Index(value[start:], ")")
		if end < 0 {
			return value, "unclosed var()"
		}
		end += start
		inner := strings.TrimSpace(value[start+len("var(") : end])
		name, fallback, hasFallback := strings.Cut(inner, ",")
		name = strings.TrimSpace(name)
		replacement, ok := declared[name]
		if !ok {
			if !hasFallback {
				return value, fmt.Sprintf("%s was never declared", name)
			}
			replacement = strings.TrimSpace(fallback)
		}
		value = value[:start] + replacement + value[end+1:]
	}
}

func parseStylesheet(source string) (*stylesheet, error) {
	parsed, err := parser.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse stylesheet: %w", err)
	}
	sheet := &stylesheet{}
	for _, parsedRule := range parsed.Rules {
		if parsedRule.Kind != 0 { // qualified rules only; at-rules are reported by the caller
			continue
		}
		declarations := make([]declaration, 0, len(parsedRule.Declarations))
		for _, parsedDeclaration := range parsedRule.Declarations {
			declarations = append(declarations, declaration{
				property:  strings.ToLower(strings.TrimSpace(parsedDeclaration.Property)),
				value:     strings.TrimSpace(parsedDeclaration.Value),
				important: parsedDeclaration.Important,
			})
		}
		var custom []declaration
		kept := declarations[:0]
		for _, applied := range declarations {
			if strings.HasPrefix(applied.property, "--") {
				custom = append(custom, applied)
				continue
			}
			kept = append(kept, applied)
		}
		declarations = kept
		for _, selectorText := range parsedRule.Selectors {
			selector, err := cascadia.Parse(selectorText)
			if err != nil {
				return nil, fmt.Errorf("parse selector %q: %w", selectorText, err)
			}
			if len(custom) != 0 {
				sheet.variables = append(sheet.variables, rule{selector: selector, declaration: custom})
			}
			sheet.rules = append(sheet.rules, rule{
				selector:    selector,
				order:       len(sheet.rules),
				declaration: declarations,
			})
		}
	}
	return sheet, nil
}

// atRules reports the at-rules in the source, which this package does not
// implement. They are listed rather than ignored because @media in particular
// looks like it should work.
func atRules(source string) []string {
	parsed, err := parser.Parse(source)
	if err != nil {
		return nil
	}
	var names []string
	for _, parsedRule := range parsed.Rules {
		if parsedRule.Kind != 0 {
			names = append(names, "@"+parsedRule.Name)
		}
	}
	return names
}

// declarationsFor collects everything that applies to one element, in the
// order it should be applied: matching rules weakest first, then the element's
// own style attribute, which beats any selector.
func (s *stylesheet) declarationsFor(node *html.Node) []declaration {
	var matched []rule
	for _, candidate := range s.rules {
		if candidate.selector.Match(node) {
			matched = append(matched, candidate)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].less(matched[j]) })

	// Applied weakest first, so the last write wins. Important declarations
	// outrank every normal one however specific, and the style attribute
	// outranks every selector, so the four groups go on in that order.
	var normal, important []declaration
	for _, candidate := range matched {
		for _, applied := range candidate.declaration {
			if applied.important {
				important = append(important, applied)
			} else {
				normal = append(normal, applied)
			}
		}
	}
	declarations := normal
	if inline := attribute(node, "style"); inline != "" {
		// The parser drops the value of a final declaration that has no
		// terminating semicolon, and a style attribute is usually written
		// without one.
		if !strings.HasSuffix(strings.TrimSpace(inline), ";") {
			inline += ";"
		}
		parsed, err := parser.ParseDeclarations(inline)
		if err == nil {
			for _, parsedDeclaration := range parsed {
				inlineDeclaration := declaration{
					property:  strings.ToLower(strings.TrimSpace(parsedDeclaration.Property)),
					value:     strings.TrimSpace(parsedDeclaration.Value),
					important: parsedDeclaration.Important,
				}
				if inlineDeclaration.important {
					important = append(important, inlineDeclaration)
					continue
				}
				declarations = append(declarations, inlineDeclaration)
			}
		}
	}
	return append(declarations, important...)
}

func attribute(node *html.Node, name string) string {
	for _, candidate := range node.Attr {
		if candidate.Key == name {
			return candidate.Val
		}
	}
	return ""
}
