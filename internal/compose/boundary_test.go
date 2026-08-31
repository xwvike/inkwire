package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The two ways of describing a page have different jobs, and this is where the
// division is written down in a form that can fail.
//
// It drifted once already. Grid, transforms, anchored boxes and shape clipping
// were added to compose for CSS and never reached the schema; arcs, polygons,
// patterns and paths were in the schema and never reachable from CSS. Nothing
// noticed, because the tests that compared the two formats only ever compared
// pages both could express, and passing read as agreement when it was
// measuring an overlap.
//
// So the table below says who may construct what, and a test walks the source
// to check. A new node has to be placed here before either side can use it,
// which is the point: the boundary is a decision, and this is where it is made.
type owner uint8

const (
	// layout is the page: boxes, text, pictures, and where they go. A
	// stylesheet says these, and a scene document may too, because a
	// generated page still needs somewhere to put things.
	layout owner = iota
	// drawing is geometry: the arcs, polygons and patterns a stylesheet has
	// no vocabulary for. Teaching CSS to ask for them would mean inventing a
	// dialect, so a page embeds a scene instead.
	drawing
	// internal nodes are built by compose itself and by neither front end.
	internalOnly
)

var nodeOwners = map[string]owner{
	// The page.
	"Row":       layout,
	"Column":    layout,
	"Grid":      layout,
	"Stack":     layout,
	"Absolute":  layout,
	"Anchored":  layout,
	"Relative":  layout,
	"Padding":   layout,
	"Spacer":    layout,
	"Text":      layout,
	"Image":     layout,
	"Rectangle": layout,
	"Clip":      layout,
	"ClipRect":  layout,
	"ClipShape": layout,
	"ClipPath":  layout,
	// A transform is a page property rather than geometry. It moves a subtree
	// that has already been drawn, which is why scale and rotate are CSS
	// properties and not shapes.
	"Transformed": layout,
	// Rotated is the same property said the other way round: it does not move
	// a drawn subtree, it puts a turn into the state everything under it works
	// out its own geometry through. That makes it exact at any angle where
	// Transformed is exact at a quarter, and it is still the page's — a
	// stylesheet turns a box, and turning a box is not a shape.
	"Rotated": layout,

	// Geometry.
	"Line":     drawing,
	"Polyline": drawing,
	"Polygon":  drawing,
	"Circle":   drawing,
	"Ellipse":  drawing,
	"Arc":      drawing,
	"Pie":      drawing,
	"Chord":    drawing,
	"Path":     drawing,
	"Pattern":  drawing,
	"Pixel":    drawing,
}

// Rectangle is on both sides of the line and stays there. A stylesheet needs
// it for every background and border, and a scene document needs it as a
// shape; it is the one node that is honestly both.
var sharedNodes = map[string]bool{"Rectangle": true}

func TestTheTwoFormatsKeepToTheirSideOfTheLine(t *testing.T) {
	tests := []struct {
		dir     string
		name    string
		allowed owner
	}{
		{"../markup", "markup", layout},
		{"../scene", "scene", drawing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, node := range constructedNodes(t, test.dir) {
				which, known := nodeOwners[node]
				if !known {
					t.Errorf("%s builds compose.%s, which nodeOwners does not place; "+
						"decide which side it belongs to", test.name, node)
					continue
				}
				if sharedNodes[node] {
					continue
				}
				// A scene document describes a page as well as its drawings,
				// so it may reach into layout; a stylesheet may not reach into
				// drawing, because that is what the scene element is for.
				if test.allowed == layout && which == drawing {
					t.Errorf("markup builds compose.%s, which is geometry; "+
						"a stylesheet has no vocabulary for it and should embed a scene instead", node)
				}
			}
		})
	}
}

// Every node compose offers has to be placed, or the table stops describing
// the thing it is meant to describe.
func TestEveryNodeIsPlacedOnOneSide(t *testing.T) {
	for _, node := range declaredNodes(t, ".") {
		if _, known := nodeOwners[node]; !known {
			t.Errorf("compose.%s is a node and nodeOwners does not say whose it is", node)
		}
	}
	for node := range nodeOwners {
		if _, exists := nodeDeclared(t, node); !exists {
			t.Errorf("nodeOwners names compose.%s, which no longer exists", node)
		}
	}
}

// constructedNodes reports the compose node types a package builds, found by
// looking for composite literals of the form compose.Name{}.
func constructedNodes(t *testing.T, dir string) []string {
	t.Helper()
	found := map[string]bool{}
	for _, file := range goFiles(t, dir) {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			literal, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			selector, ok := literal.Type.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || pkg.Name != "compose" {
				return true
			}
			found[selector.Sel.Name] = true
			return true
		})
	}
	var names []string
	for name := range found {
		// Only the nodes matter here; the supporting types are not the line.
		if _, isNode := nodeOwners[name]; isNode || isNodeName(t, name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// declaredNodes reports the types in this package that implement Node, found
// by looking for a composeNode method on them.
func declaredNodes(t *testing.T, dir string) []string {
	t.Helper()
	var names []string
	for _, file := range goFiles(t, dir) {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != "composeNode" || function.Recv == nil {
				continue
			}
			if name := receiverName(function.Recv); name != "" {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

func nodeDeclared(t *testing.T, name string) (string, bool) {
	t.Helper()
	for _, declared := range declaredNodes(t, ".") {
		if declared == name {
			return declared, true
		}
	}
	return "", false
}

func isNodeName(t *testing.T, name string) bool {
	_, ok := nodeDeclared(t, name)
	return ok
}

func receiverName(fields *ast.FieldList) string {
	if len(fields.List) == 0 {
		return ""
	}
	switch receiver := fields.List[0].Type.(type) {
	case *ast.Ident:
		return receiver.Name
	case *ast.StarExpr:
		if ident, ok := receiver.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	return files
}
