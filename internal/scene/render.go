package scene

import (
	"fmt"
	"image"
	"io"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

type Result struct {
	Frame       *display.Frame
	Orientation display.Orientation
	Report      compose.Report
}

func (d Decoder) Render(reader io.Reader) (Result, error) {
	document, err := d.Decode(reader)
	if err != nil {
		return Result{}, err
	}
	return Render(document)
}

func (d Decoder) RenderForSize(reader io.Reader, size image.Point) (Result, error) {
	document, err := d.Decode(reader)
	if err != nil {
		return Result{}, err
	}
	return RenderForSize(document, size)
}

func (d Decoder) RenderFile(path string) (Result, error) {
	document, err := d.DecodeFile(path)
	if err != nil {
		return Result{}, err
	}
	return Render(document)
}

func (d Decoder) RenderFileForSize(path string, size image.Point) (Result, error) {
	document, err := d.DecodeFile(path)
	if err != nil {
		return Result{}, err
	}
	return RenderForSize(document, size)
}

func Render(document compose.Document) (Result, error) {
	return render(document)
}

func RenderForSize(document compose.Document, size image.Point) (Result, error) {
	if size.X <= 0 || size.Y <= 0 {
		return Result{}, fmt.Errorf("target size must be positive, got %v", size)
	}
	logical := size
	switch document.Orientation {
	case display.OrientationLandscape:
	case display.OrientationPortraitClockwise, display.OrientationPortraitCounterClockwise:
		logical = image.Pt(size.Y, size.X)
	default:
		return Result{}, fmt.Errorf("invalid orientation %d", document.Orientation)
	}
	stated := document.Size
	document.Size = logical
	result, err := render(document)
	if err != nil {
		return Result{}, err
	}
	// A page written for one panel and sent to another is laid out again for
	// the panel it reached, and said so rather than refused. Whoever asked for
	// this has a tag in front of them and a page they want on it; deciding for
	// them that it cannot be done is the one outcome that helps nobody.
	//
	// It is laid out again rather than scaled. The fonts are bitmaps with a
	// few fixed sizes, so shrinking a finished 400x300 render onto a 296x128
	// panel turns every word to mush, while laying it out again leaves
	// anything sized in percentages or fr fitting and anything placed
	// absolutely clipped at the edge — a loss that can be seen and corrected.
	if stated != (image.Point{}) && stated != logical {
		result.Report.Warnings = append(result.Report.Warnings, compose.Warning{
			Path: "document",
			Code: "size-mismatch",
			Message: fmt.Sprintf("scene declares %dx%d and the panel is %dx%d, so it was laid out again for the panel; anything placed beyond it is clipped",
				stated.X, stated.Y, logical.X, logical.Y),
		})
	}
	return result, nil
}

func render(document compose.Document) (Result, error) {
	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		return Result{}, err
	}
	compiled, report, err := compiler.Compile(document)
	if err != nil {
		return Result{}, fmt.Errorf("compile scene: %w", err)
	}
	frame, err := compiled.Render()
	if err != nil {
		return Result{}, fmt.Errorf("render scene: %w", err)
	}
	return Result{Frame: frame, Orientation: document.Orientation, Report: report}, nil
}
