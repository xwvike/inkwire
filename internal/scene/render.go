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
	if document.Size != (image.Point{}) && document.Size != logical {
		return Result{}, fmt.Errorf("scene declares size %dx%d but target page is %dx%d", document.Size.X, document.Size.Y, logical.X, logical.Y)
	}
	document.Size = logical
	return render(document)
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

func (r Result) Payload() ([]byte, error) {
	if r.Frame == nil {
		return nil, fmt.Errorf("render result has no frame")
	}
	return display.EncodeGiciskyOriented(r.Frame, r.Orientation)
}
