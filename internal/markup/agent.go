package markup

import (
	"fmt"
	"sync"

	"golang.org/x/net/html"
)

// agentCSS is the part of a browser's own stylesheet that a page counts on
// without writing it: a heading is large and bold, paragraphs are a line
// apart, a list is indented and marked. It is drawn from the one Chrome and
// Firefox share, and it is CSS so that it is read, cascaded and overridden
// exactly as a browser's is — every rule a page writes outranks every rule
// here, whatever its selector.
//
// What is left out is what this display has no use for: the display of the
// block elements, which inlineByDefault already settles; link colours, on
// a panel of black and white; and the faces of form controls.
const agentCSS = `
li { display: list-item; }
p, dl, ul, ol, menu, pre { margin-top: 1em; margin-bottom: 1em; }
blockquote, figure { margin: 1em 40px; }
dd { margin-left: 40px; }
h1 { font-size: 2em; margin-top: 0.67em; margin-bottom: 0.67em; font-weight: bold; }
h2 { font-size: 1.5em; margin-top: 0.83em; margin-bottom: 0.83em; font-weight: bold; }
h3 { font-size: 1.17em; margin-top: 1em; margin-bottom: 1em; font-weight: bold; }
h4 { margin-top: 1.33em; margin-bottom: 1.33em; font-weight: bold; }
h5 { font-size: 0.83em; margin-top: 1.67em; margin-bottom: 1.67em; font-weight: bold; }
h6 { font-size: 0.67em; margin-top: 2.33em; margin-bottom: 2.33em; font-weight: bold; }
b, strong { font-weight: bolder; }
th { font-weight: bold; text-align: center; }
caption, center { text-align: center; }
small { font-size: 0.83em; }
u, ins, a[href] { text-decoration: underline; }
s, strike, del { text-decoration: line-through; }
hr { margin-top: 0.5em; margin-bottom: 0.5em; border-top: 1px solid; }
pre { white-space: pre; }
ul, ol, menu { padding-left: 40px; }
ol { list-style-type: decimal; }
ul ul, ol ul, menu ul, ul menu, ol menu, menu menu { list-style-type: circle; }
ul ul ul, ul ol ul, ol ul ul, ol ol ul, menu ul ul, ul menu ul { list-style-type: square; }
ul ul, ul ol, ol ul, ol ol, menu ul, menu ol, ul menu, ol menu { margin-top: 0; margin-bottom: 0; }
`

// agentSheet is agentCSS read once. A rule in it that does not read is a
// mistake here and not in any page, so it stops the program rather than
// warning a page that did nothing wrong.
var agentSheet = sync.OnceValue(func() *stylesheet {
	sheet, err := parseStylesheet(agentCSS, func(code, message string) {
		panic(fmt.Sprintf("the default stylesheet: %s: %s", code, message))
	})
	if err != nil {
		panic(fmt.Sprintf("the default stylesheet: %v", err))
	}
	return sheet
})

// agentDeclarations are the default stylesheet's declarations for one HTML
// element, cascaded among themselves and marked as the agent's, so that they
// can be applied before the page's own.
func agentDeclarations(node *html.Node) []declaration {
	if node.Namespace != "" {
		return nil
	}
	declarations := cascade(agentSheet().rules, node, nil)
	for index := range declarations {
		declarations[index].agent = true
	}
	declarations = append(declarations, presentationalHints(node)...)
	return declarations
}

// presentationalHints are the attributes HTML lets stand in for a rule: an
// ordered list's type. A browser ranks them below every rule the page
// writes, and so does this.
func presentationalHints(node *html.Node) []declaration {
	if node.Data != "ol" && node.Data != "ul" && node.Data != "li" {
		return nil
	}
	kind, ok := listTypeAttributes[attribute(node, "type")]
	if !ok || !hasAttribute(node, "type") {
		return nil
	}
	return []declaration{{property: "list-style-type", value: kind, agent: true}}
}

// listTypeAttributes are what type="" names, case and all: a and A are two
// different lists.
var listTypeAttributes = map[string]string{
	"1": "decimal", "a": "lower-alpha", "A": "upper-alpha", "i": "lower-roman", "I": "upper-roman",
	"disc": "disc", "circle": "circle", "square": "square",
}
