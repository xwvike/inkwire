// Command panels writes the panel catalogue the editor offers as presets.
//
// The catalogue lives in the driver packages, which reach the radio and so
// cannot be compiled for a browser. Rather than keep a second copy of it in
// JavaScript — which would be wrong the first time a model was added — this
// runs on the host and emits what it finds. Regenerate with:
//
//	go run ./web/tools/panels -o web/static/panels.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/xwvike/inkwire/internal/panel"
)

type preset struct {
	Key      string `json:"key"`
	Family   string `json:"family"`
	Name     string `json:"name"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Palette  string `json:"palette"`
	Verified bool   `json:"verified"`
}

func main() {
	out := flag.String("o", "", "file to write (default stdout)")
	flag.Parse()

	panels := panel.All()
	presets := make([]preset, 0, len(panels))
	for _, p := range panels {
		size := p.Size()
		presets = append(presets, preset{
			Key:      p.ID(),
			Family:   p.Family,
			Name:     p.Name(),
			Width:    size.X,
			Height:   size.Y,
			Palette:  paletteOf(p),
			Verified: verifiedOf(p),
		})
	}
	// Smallest first, so the list reads as a ladder rather than as whatever
	// order two catalogues happened to be written in.
	sort.SliceStable(presets, func(i, j int) bool {
		a, b := presets[i], presets[j]
		if a.Width*a.Height != b.Width*b.Height {
			return a.Width*a.Height < b.Width*b.Height
		}
		return a.Key < b.Key
	})

	encoded, err := json.MarshalIndent(presets, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded = append(encoded, '\n')
	if *out == "" {
		os.Stdout.Write(encoded)
		return
	}
	if err := os.WriteFile(*out, encoded, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d panels)\n", *out, len(presets))
}

// paletteOf reports the inks a panel can show. The two families spell their
// palettes with separate types that print the same names.
func paletteOf(p panel.Panel) string {
	if p.NRFEPD.Width != 0 {
		return p.NRFEPD.Palette.String()
	}
	return p.Gicisky.Palette.String()
}

// verifiedOf reports whether the entry was confirmed against real hardware
// rather than read off a firmware table. An editor that offers both should say
// which is which.
func verifiedOf(p panel.Panel) bool {
	if p.NRFEPD.Width != 0 {
		return p.NRFEPD.Verified
	}
	return p.Gicisky.Verified
}
