package markup

import (
	"strings"
	"testing"
)

func TestThePlaceShorthandsSetBothAxes(t *testing.T) {
	body := `<div class="page"><div class="a">x</div><div class="b">y</div></div>`
	const page = `.page { display: grid; grid-template-columns: 1fr 1fr; width: 100px; height: 80px; } `
	for shorthand, longhands := range map[string]string{
		`.page { place-items: center; }`:                              `.page { align-items: center; justify-items: center; }`,
		`.page { place-items: end start; }`:                           `.page { align-items: end; justify-items: start; }`,
		`.a { place-self: end; }`:                                     `.a { align-self: end; justify-self: end; }`,
		`.a { place-self: start center; }`:                            `.a { align-self: start; justify-self: center; }`,
		`.page { place-content: center; }`:                            `.page { align-content: center; justify-content: center; }`,
		`.page { place-content: end space-between; }`:                 `.page { align-content: end; justify-content: space-between; }`,
		`.page { align-items: end; } .page { place-items: initial; }`: `.page { }`,
	} {
		got, err := Compile(body, page+shorthand)
		if err != nil {
			t.Fatal(err)
		}
		want, err := Compile(body, page+longhands)
		if err != nil {
			t.Fatal(err)
		}
		if string(got.JSON) != string(want.JSON) || len(got.Warnings) != len(want.Warnings) {
			t.Errorf("%s did not compile as %s: %v", shorthand, longhands, got.Warnings)
		}
	}
	if said := warningsFor(t, `<div class="a">x</div>`, `.page { place-items: center center center; }`); !strings.Contains(said, "takes one or two values") {
		t.Errorf("three values for place-items said %q", said)
	}
}
