package compose

import (
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func render(t *testing.T, node Node) (Report, error) {
	t.Helper()
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := compiler.Compile(Document{Size: image.Pt(200, 40), Root: node})
	return report, err
}

func text(font string, size int, body string) Text {
	return Text{Runs: []display.TextRun{{Text: body, Style: display.TextStyle{Font: font, Size: size}}}}
}

// A character a font does not carry is nearly always the wrong font rather
// than a character nothing can draw, so the warning has to name both: the font
// that came up short and the one to use instead.
func TestTheMissingRuneWarningNamesAFontThatHasThem(t *testing.T) {
	report, err := render(t, text("monaco", 12, "08-14 周五"))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one", report.Warnings)
	}
	message := report.Warnings[0].Message
	for _, want := range []string{`"monaco"`, "周五", `"hzk"`, `"ui"`} {
		if !strings.Contains(message, want) {
			t.Errorf("message %q does not mention %s", message, want)
		}
	}
}

// When nothing can draw it, saying so is the useful answer; pointing at a font
// that cannot help would be worse than pointing at none.
func TestTheMissingRuneWarningSaysWhenNoFontHasThem(t *testing.T) {
	report, err := render(t, text("ui", 12, "✔"))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one", report.Warnings)
	}
	if message := report.Warnings[0].Message; !strings.Contains(message, "no bundled font") {
		t.Errorf("message = %q, want it to say nothing has the glyph", message)
	}
}

// A size is one of a list, so the error gives the list rather than leaving the
// reader to go and find it.
func TestTheStrikeErrorListsTheSizesThatExist(t *testing.T) {
	_, err := render(t, text("monaco", 13, "hi"))
	if err == nil {
		t.Fatal("13px was accepted")
	}
	message := err.Error()
	for _, want := range []string{`"monaco"`, "13px", "10, 12, 14, 16"} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not mention %s", message, want)
		}
	}
}

func TestAnUnknownFontIsNamedAsUnknown(t *testing.T) {
	_, err := render(t, text("comic", 12, "hi"))
	if err == nil || !strings.Contains(err.Error(), `unknown font "comic"`) {
		t.Fatalf("error = %v, want it to name the unknown font", err)
	}
}

func TestFamiliesWithReportsOnlyTheFamiliesThatCoverEveryRune(t *testing.T) {
	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if got := fonts.FamiliesWith([]rune("A")); len(got) == 0 {
		t.Error("no family claims to draw an ASCII letter")
	}
	// One rune every family has and one no family has: the answer is none.
	if got := fonts.FamiliesWith([]rune("A✔")); len(got) != 0 {
		t.Errorf("FamiliesWith(\"A✔\") = %v, want none, since ✔ is in no family", got)
	}
}
