package scene_test

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// renderCore is what has to reach a panel, and nothing else is allowed to come
// with it. A scene document is the format a device speaks, so these three
// packages are the ones a firmware build links; internal/markup and the HTML
// and CSS parsers behind it belong to the machine an author writes on.
var renderCore = []string{
	"github.com/xwvike/inkwire/internal/display",
	"github.com/xwvike/inkwire/internal/compose",
	"github.com/xwvike/inkwire/internal/scene",
}

// renderCoreDependencies is every package outside this module that the render
// core reaches. The list is short because it has to stay short: the smallest
// panel this project drives has 371 KB of heap for the whole program, and a
// dependency that looks free on a laptop is a dependency that has to be paid
// for there.
//
// Adding a line here is a decision, which is why it fails until it is made.
var renderCoreDependencies = []string{
	// GB2312 is the encoding the bundled HZK strikes are indexed by, so the
	// text layer needs a decoder for it.
	"golang.org/x/text/encoding",
	"golang.org/x/text/encoding/internal",
	"golang.org/x/text/encoding/internal/identifier",
	"golang.org/x/text/encoding/simplifiedchinese",
	"golang.org/x/text/transform",
}

// The two front ends are meant to be separable, and the direction of the
// dependency is what makes them so: markup imports compose, and compose has
// never imported markup. Said that way it is a claim about the source; said
// as the transitive closure it is a claim about what the linker puts in the
// binary, which is the one that matters.
//
// Without this, adding an HTML front end to the repository would quietly put
// an HTML parser, a CSS parser and a selector engine on the path of a build
// that only ever renders a scene document.
func TestTheRenderCoreCarriesNothingItCannotAffordOnADevice(t *testing.T) {
	listed, err := exec.Command("go", append([]string{"list", "-deps"}, renderCore...)...).Output()
	if err != nil {
		t.Fatalf("list the render core's dependencies: %v", err)
	}

	allowed := map[string]bool{}
	for _, name := range renderCoreDependencies {
		allowed[name] = true
	}

	var unexpected []string
	for _, name := range strings.Fields(string(listed)) {
		// A package with no dot in its first element is from the standard
		// library, and one inside this module is the subject rather than a
		// dependency of it.
		first, _, _ := strings.Cut(name, "/")
		if !strings.Contains(first, ".") || strings.HasPrefix(name, "github.com/xwvike/inkwire/") {
			continue
		}
		if !allowed[name] {
			unexpected = append(unexpected, name)
		}
		delete(allowed, name)
	}

	sort.Strings(unexpected)
	for _, name := range unexpected {
		t.Errorf("the render core now reaches %s, which no device build asked for", name)
	}
	for name := range allowed {
		t.Errorf("%s is listed as a dependency of the render core and is no longer one", name)
	}
}
