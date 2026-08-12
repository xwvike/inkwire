package scene

import (
	"fmt"
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

func (d Decoder) RenderFile(path string) (Result, error) {
	document, err := d.DecodeFile(path)
	if err != nil {
		return Result{}, err
	}
	return Render(document)
}

func Render(document compose.Document) (Result, error) {
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
