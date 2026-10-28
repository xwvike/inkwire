package markup

import (
	"strings"
	"testing"
	"time"
)

// A page that writes absurd numbers, or nests absurdly deep, is still drawn,
// and drawn in a moment. Each of these used to hang the renderer — and with
// it, on the server, the radio — because the work followed the numbers
// written rather than what could be seen.
func TestAnAbsurdPageIsDrawnQuickly(t *testing.T) {
	cases := map[string][2]string{
		"a tall dashed border": {`<div class="c"></div>`, `.c { border: 1px dashed black; width: 50px; height: 1e8px; }`},
		"a huge dotted box":    {`<div class="c"></div>`, `.c { border: 3px dotted black; width: 1e8px; height: 1e8px; }`},
		"a huge rounded dash":  {`<div class="c"></div>`, `.c { border: 2px dashed black; border-radius: 1e7px; width: 1e8px; height: 1e8px; }`},
		"a huge clip circle":   {`<div class="c"></div>`, `.c { clip-path: circle(1e8px); width: 1e8px; height: 10px; background: black; }`},
		"a turned dashed box":  {`<div class="c"></div>`, `.c { transform: rotate(13deg); border: 1px dashed black; width: 1e8px; height: 1e8px; }`},
		"a huge flex basis":    {`<div class="c">x</div><div class="d">y</div>`, `.c { flex: 1 1 1e15px; } .d { flex: 2 0 1e9px; }`},
		"a huge circle": {`<svg width="100" height="50"><circle r="1e8" fill="none" stroke="black" stroke-dasharray="3 2"/>` +
			`<ellipse rx="1e8" ry="1000" fill="none" stroke="black" stroke-dasharray="3 2" transform="rotate(10)"/></svg>`, ``},
		"a huge curve": {`<svg width="100" height="50"><path d="M0 0 C 1e8 0 0 1e8 5 5" stroke="black" fill="none"/>` +
			`<polygon points="0,0 1e8,0 0,1e8" fill="none" stroke="black" stroke-width="1e8"/></svg>`, ``},
		"deep nesting": {strings.Repeat(`<div class="d">`, 300) + "x" + strings.Repeat(`</div>`, 300),
			`.d { padding: 1px; display: flex; flex-direction: column; }`},
	}
	for name, page := range cases {
		start := time.Now()
		document, err := Compile(`<div class="page">`+page[0]+`</div>`,
			`.page { display: flex; width: 120px; height: 60px; background: white; font-family: ui; font-size: 12px; }`+page[1])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		frameOf(t, document)
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("%s took %v", name, elapsed)
		}
	}
}
