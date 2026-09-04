package markup

import (
	"fmt"
	"sort"
	"strings"

	"github.com/andybalholm/cascadia"
	"github.com/aymerick/douceur/css"
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
// substitute replaces every var() in a value with what it was declared as.
//
// A custom property may be declared as another one, so this repeats until
// there are none left — and a property declared as itself, or two declared as
// each other, means there are always some left. CSS calls that a cyclic
// dependency and drops the declaration; here it used to be the request that
// never came back, which on a service is worse than wrong.
func (s *stylesheet) substitute(value string, declared map[string]string) (string, string) {
	for rounds := 0; ; rounds++ {
		if rounds > maxSubstitutions {
			return value, "this refers to a custom property that refers back to it"
		}
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

// maxSubstitutions is how many var() a value may resolve through. Nesting one
// inside another is ordinary; nesting a hundred is a cycle.
const maxSubstitutions = 100

// parseStylesheet reads a stylesheet into rules this package can match.
//
// What it cannot read it leaves out and reports, rather than refusing the
// file. A stylesheet copied from anywhere carries selectors no subset has —
// ::placeholder, :is(), :has(), a vendor's own pseudo-element — and losing the
// page over one of them means losing it over the parts that had nothing to do
// with the panel.
func parseStylesheet(source string, report func(string, string)) (*stylesheet, error) {
	parsed, err := parser.Parse(source)
	if err != nil {
		// The parser gives up on the whole file at the first thing it cannot
		// read. A browser does not: it skips the rule and carries on, which
		// is why a missing brace somewhere near the bottom of a stylesheet
		// does not blank the page. So the file is taken apart into rules and
		// each is offered on its own, and what is left is what parsed.
		parsed, err = parseRuleByRule(source, report)
		if err != nil {
			return nil, fmt.Errorf("parse stylesheet: %w", err)
		}
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
				report("unsupported-selector", fmt.Sprintf(
					"%s was skipped: %v. Everything else in the stylesheet still applies",
					selectorText, err))
				continue
			}
			if len(custom) != 0 {
				sheet.variables = append(sheet.variables, rule{
					selector: selector, order: len(sheet.variables), declaration: custom})
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

// parseRuleByRule offers each rule to the parser on its own, so that one it
// cannot read costs that rule and no more.
//
// Rules are found by counting braces rather than by parsing, because parsing
// is the thing that just failed. An at-rule carries a block of its own, so the
// depth has to be counted rather than the first closing brace taken.
func parseRuleByRule(source string, report func(string, string)) (*css.Stylesheet, error) {
	whole := &css.Stylesheet{}
	depth, start := 0, 0
	for index, character := range source {
		switch character {
		case '{':
			depth++
		case '}':
			depth--
			if depth > 0 {
				continue
			}
			// Depth below zero is a closing brace with nothing open, which is
			// the commonest way a stylesheet is broken. Drop it and go on.
			text := strings.TrimSpace(source[start : index+1])
			start, depth = index+1, 0
			if text == "" || text == "}" {
				continue
			}
			one, err := parser.Parse(text)
			if err != nil {
				report("unreadable-rule", fmt.Sprintf(
					"%s was skipped: %v. Everything else in the stylesheet still applies",
					summarise(text), err))
				continue
			}
			whole.Rules = append(whole.Rules, one.Rules...)
		}
	}
	if rest := strings.TrimSpace(source[start:]); rest != "" {
		report("unreadable-rule", fmt.Sprintf(
			"%s was skipped: it is never closed. Everything else in the stylesheet still applies",
			summarise(rest)))
	}
	return whole, nil
}

// summarise names a rule by its first line, so that a message about one is
// findable without quoting the whole of it.
func summarise(text string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	first = strings.TrimSpace(first)
	if len(first) > 60 {
		first = first[:60] + "…"
	}
	return first
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
			// The parser keeps the @ on the name, so putting another one
			// there said "@@media".
			names = append(names, strings.TrimPrefix(parsedRule.Name, "@"))
		}
	}
	return names
}

// declarationsFor collects everything that applies to one element, in the
// order it should be applied: matching rules weakest first, then the element's
// own style attribute, which beats any selector.
//
// Custom properties are not here. They are declared with the same cascade and
// read at a different time — a var() is resolved against the whole chain of
// ancestors, not just this element — so they live in their own table and come
// back through customFor.
func (s *stylesheet) declarationsFor(node *html.Node) []declaration {
	inline, _ := inlineDeclarations(node)
	return cascade(s.rules, node, ordinary(inline))
}

// customFor collects the custom properties declared on one element, cascaded
// the same way everything else is. Without this the first rule in the file won
// whatever its selector was, so an id could not override a class and a second
// declaration of the same name did nothing.
func (s *stylesheet) customFor(node *html.Node) []declaration {
	inline, _ := inlineDeclarations(node)
	return cascade(s.variables, node, custom(inline))
}

// cascade puts the declarations that apply to one element in the order they
// should be applied: matching rules weakest first, then the style attribute,
// then everything marked important in the same order again. The last write for
// a property is the one that counts, which is what makes this an order rather
// than a choice.
func cascade(rules []rule, node *html.Node, inline []declaration) []declaration {
	var matched []rule
	for _, candidate := range rules {
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
	for _, applied := range inline {
		if applied.important {
			important = append(important, applied)
			continue
		}
		normal = append(normal, applied)
	}
	return append(normal, important...)
}

// inlineDeclarations reads an element's style attribute. It returns what it
// read and what stopped it, because a style attribute the parser refuses is a
// silent loss of everything the author wrote in it.
func inlineDeclarations(node *html.Node) ([]declaration, error) {
	inline := attribute(node, "style")
	if inline == "" {
		return nil, nil
	}
	// The parser drops the value of a final declaration that has no
	// terminating semicolon, and a style attribute is usually written
	// without one.
	if !strings.HasSuffix(strings.TrimSpace(inline), ";") {
		inline += ";"
	}
	parsed, err := parser.ParseDeclarations(inline)
	if err != nil {
		return nil, err
	}
	declarations := make([]declaration, 0, len(parsed))
	for _, parsedDeclaration := range parsed {
		declarations = append(declarations, declaration{
			property:  strings.ToLower(strings.TrimSpace(parsedDeclaration.Property)),
			value:     strings.TrimSpace(parsedDeclaration.Value),
			important: parsedDeclaration.Important,
		})
	}
	return declarations, nil
}

// ordinary and custom split a run of declarations the way the stylesheet is
// split: a name beginning with two dashes is a custom property and is read by
// var(), everything else is a property this package implements or reports.
func ordinary(declarations []declaration) []declaration {
	return withNames(declarations, false)
}

func custom(declarations []declaration) []declaration {
	return withNames(declarations, true)
}

func withNames(declarations []declaration, wantCustom bool) []declaration {
	var kept []declaration
	for _, applied := range declarations {
		if strings.HasPrefix(applied.property, "--") == wantCustom {
			kept = append(kept, applied)
		}
	}
	return kept
}

func attribute(node *html.Node, name string) string {
	if prefix, local, ok := strings.Cut(name, ":"); ok {
		for _, candidate := range node.Attr {
			if candidate.Key == local && candidate.Namespace == prefix {
				return candidate.Val
			}
		}
	}
	for _, candidate := range node.Attr {
		if candidate.Key == name {
			return candidate.Val
		}
	}
	return ""
}
