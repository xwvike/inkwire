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
	property string
	value    string
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
				property: strings.ToLower(strings.TrimSpace(parsedDeclaration.Property)),
				value:    strings.TrimSpace(parsedDeclaration.Value),
			})
		}
		for _, selectorText := range parsedRule.Selectors {
			selector, err := cascadia.Parse(selectorText)
			if err != nil {
				return nil, fmt.Errorf("parse selector %q: %w", selectorText, err)
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

	var declarations []declaration
	for _, candidate := range matched {
		declarations = append(declarations, candidate.declaration...)
	}
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
				declarations = append(declarations, declaration{
					property: strings.ToLower(strings.TrimSpace(parsedDeclaration.Property)),
					value:    strings.TrimSpace(parsedDeclaration.Value),
				})
			}
		}
	}
	return declarations
}

func attribute(node *html.Node, name string) string {
	for _, candidate := range node.Attr {
		if candidate.Key == name {
			return candidate.Val
		}
	}
	return ""
}
