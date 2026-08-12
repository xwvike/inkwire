package main

import (
	"flag"
	"fmt"
	"image"
	"os"

	"github.com/xwvike/inkwire/internal/display"
)

func main() {
	pngPath := flag.String("png", "state_showcase.png", "PNG preview output path")
	payloadPath := flag.String("payload", "", "optional Gicisky payload output path")
	flag.Parse()

	frame, err := renderStateShowcase()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writePNG(*pngPath, frame); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *payloadPath != "" {
		payload, err := display.EncodeGicisky(frame)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(*payloadPath, payload, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func renderStateShowcase() (*display.Frame, error) {
	frame, err := display.NewPage(display.OrientationLandscape, display.InkWhite)
	if err != nil {
		return nil, err
	}
	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		return nil, err
	}

	recording := &display.DisplayList{}
	view := stateShowcase{list: recording, fonts: fonts}
	if err := view.draw(); err != nil {
		return nil, err
	}
	commandCount, bounds := recording.Len(), recording.Bounds()

	replay := recording.Clone()
	recording.Reset()
	if recording.Len() != 0 || !recording.Bounds().Empty() {
		return nil, fmt.Errorf("DisplayList Reset did not clear the recording")
	}
	canvas := display.NewCanvas(frame)
	if err := replay.Replay(canvas); err != nil {
		return nil, err
	}

	metadata := &display.DisplayList{}
	meta := stateShowcase{list: metadata, fonts: fonts}
	if err := meta.drawMetadata(commandCount, bounds); err != nil {
		return nil, err
	}
	if err := metadata.Replay(canvas); err != nil {
		return nil, err
	}
	return frame, nil
}

type stateShowcase struct {
	list  *display.DisplayList
	fonts *display.FontRegistry
}

func (s stateShowcase) draw() error {
	black1 := display.StrokeStyle{Ink: display.InkBlack, Width: 1}
	red1 := display.StrokeStyle{Ink: display.InkRed, Width: 1}

	s.list.FillRect(image.Rect(0, 0, 296, 21), display.InkBlack)
	if err := s.text(image.Rect(6, 2, 213, 20), 18, display.AlignStart,
		run("DISPLAY LIST / ", "monaco", 14, display.InkWhite),
		run("状态栈", "hzk", 14, display.InkWhite),
	); err != nil {
		return err
	}
	if err := s.text(image.Rect(5, 23, 205, 36), 12, display.AlignStart,
		run("RECORD > CLONE > RESET > REPLAY", "monaco", 10, display.InkBlack),
	); err != nil {
		return err
	}

	left := image.Rect(3, 38, 98, 105)
	middle := image.Rect(101, 38, 198, 105)
	right := image.Rect(201, 38, 293, 105)
	for _, panel := range []image.Rectangle{left, middle, right} {
		s.list.StrokeRoundRect(panel, 4, black1)
		s.list.FillRect(image.Rect(panel.Min.X+1, panel.Min.Y+1, panel.Max.X-1, panel.Min.Y+15), display.InkBlack)
	}
	if err := s.panelLabel(left, "CLIP RECT"); err != nil {
		return err
	}
	if err := s.panelLabel(middle, "TRANSLATE"); err != nil {
		return err
	}
	if err := s.panelLabel(right, "SAVE / RESTORE"); err != nil {
		return err
	}

	clipArea := image.Rect(9, 57, 92, 98)
	s.list.Save()
	s.list.ClipRect(clipArea)
	for index, x := 0, -32; x < 116; index, x = index+1, x+8 {
		ink := display.InkBlack
		if index%3 == 0 {
			ink = display.InkRed
		}
		s.list.DrawLine(image.Pt(x, 104), image.Pt(x+52, 51), display.StrokeStyle{Ink: ink, Width: 2})
	}
	s.list.Save()
	s.list.ClipRect(image.Rect(20, 68, 81, 86))
	s.list.FillCircle(image.Pt(80, 69), 19, display.InkRed)
	s.list.Restore()
	s.list.Restore()
	s.list.StrokeRect(clipArea, red1)
	s.list.FillRect(image.Rect(10, 87, 91, 97), display.InkBlack)
	if err := s.text(image.Rect(14, 87, 88, 98), 11, display.AlignCenter,
		run("OUTSIDE CUT", "monaco", 10, display.InkWhite),
	); err != nil {
		return err
	}

	for index, offset := range []image.Point{image.Pt(109, 58), image.Pt(126, 68), image.Pt(143, 78)} {
		s.list.Save()
		s.list.Translate(offset)
		ink := display.InkBlack
		if index == 1 {
			ink = display.InkRed
		}
		stroke := display.StrokeStyle{Ink: ink, Width: 1}
		s.list.StrokeRoundRect(image.Rect(0, 0, 35, 20), 3, stroke)
		s.list.FillCircle(image.Pt(7, 10), 3, ink)
		s.list.DrawLine(image.Pt(13, 10), image.Pt(29, 10), stroke)
		s.list.Restore()
	}
	if err := s.text(image.Rect(109, 91, 193, 103), 11, display.AlignCenter,
		run("+17,+10 STEP", "monaco", 10, display.InkBlack),
	); err != nil {
		return err
	}

	s.list.Save()
	s.list.Translate(image.Pt(208, 59))
	s.list.Save()
	s.list.Translate(image.Pt(36, 20))
	s.list.FillRoundRect(image.Rect(0, 0, 21, 15), 2, display.InkRed)
	s.list.Restore()
	s.list.StrokeRoundRect(image.Rect(0, 0, 21, 15), 2, black1)
	s.list.Restore()
	s.list.DrawLine(image.Pt(230, 70), image.Pt(242, 78), display.StrokeStyle{
		Ink: display.InkRed, Width: 1, Dash: []int{2, 2},
	})
	if err := s.text(image.Rect(208, 60, 229, 73), 12, display.AlignCenter,
		run("A", "monaco", 10, display.InkBlack),
	); err != nil {
		return err
	}
	if err := s.text(image.Rect(244, 80, 265, 93), 12, display.AlignCenter,
		run("B", "monaco", 10, display.InkWhite),
	); err != nil {
		return err
	}
	if err := s.text(image.Rect(210, 91, 289, 103), 11, display.AlignCenter,
		run("A RESTORED", "monaco", 10, display.InkBlack),
	); err != nil {
		return err
	}

	s.list.FillRect(image.Rect(0, 108, 296, 128), display.InkBlack)
	if err := s.text(image.Rect(5, 110, 291, 126), 15, display.AlignCenter,
		run("SAVE", "monaco", 12, display.InkWhite),
		run(" > CLIP > ", "monaco", 12, display.InkRed),
		run("TRANSLATE", "monaco", 12, display.InkWhite),
		run(" > RESTORE", "monaco", 12, display.InkRed),
	); err != nil {
		return err
	}
	return nil
}

func (s stateShowcase) drawMetadata(commandCount int, bounds image.Rectangle) error {
	if err := s.text(image.Rect(210, 3, 291, 19), 16, display.AlignEnd,
		run(fmt.Sprintf("%02d CMD", commandCount), "monaco", 12, display.InkRed),
	); err != nil {
		return err
	}
	return s.text(image.Rect(205, 23, 291, 36), 12, display.AlignEnd,
		run(fmt.Sprintf("BOUNDS %dx%d", bounds.Dx(), bounds.Dy()), "monaco", 10, display.InkRed),
	)
}

func (s stateShowcase) panelLabel(panel image.Rectangle, label string) error {
	return s.text(image.Rect(panel.Min.X+4, panel.Min.Y+2, panel.Max.X-4, panel.Min.Y+14), 12, display.AlignCenter,
		run(label, "monaco", 10, display.InkWhite),
	)
}

func (s stateShowcase) text(bounds image.Rectangle, lineHeight int, align display.HorizontalAlign, runs ...display.TextRun) error {
	layout, err := s.list.DrawTextBox(s.fonts, display.TextBox{
		Bounds: bounds, Runs: runs, Align: align, LineHeight: lineHeight,
	})
	if err != nil {
		return err
	}
	if missing := layout.MissingRunes(); len(missing) != 0 {
		return fmt.Errorf("missing state showcase glyphs: %q", string(missing))
	}
	return nil
}

func run(text, font string, size int, ink display.Ink) display.TextRun {
	return display.TextRun{Text: text, Style: display.TextStyle{Font: font, Size: size, Ink: ink}}
}

func writePNG(path string, frame *display.Frame) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := display.WritePNG(file, frame); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
