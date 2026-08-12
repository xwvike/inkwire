package display

import (
	"fmt"
	"image"
	"image/color"
)

// A one-bit panel wants opposite treatment for the two kinds of source it
// normally gets, and picking wrong is not a matter of degree: thresholding a
// photograph flattens it to blotches, while error-diffusing a drawing scatters
// its flat areas into speckle. ImageProfile measures the two properties that
// decide which is which, so callers choose from evidence rather than by naming
// a category.
type ImageProfile struct {
	// MidToneFraction is the share of pixels that are neither near-black nor
	// near-white. Continuous tone lives here; flat artwork barely registers.
	MidToneFraction float64
	// Threshold is the cut Otsu's method finds in the image's own histogram.
	// A fixed cut at 128 erases artwork that happens to sit entirely above it.
	Threshold int
	// ChromaticFraction is the share of pixels with a noticeable colour cast.
	// The red plane fires on anything merely warm, so a photograph of a room
	// full of wood and beige scatters red through a subject that has none;
	// this is what tells such an image apart from one that is actually red.
	ChromaticFraction float64
	// Photographic reports whether the image reads as continuous tone.
	Photographic bool
	// Monochrome reports that the image carries too little colour for the red
	// plane to mean anything.
	Monochrome bool
}

// photographicMidTone separates the two kinds. Measured across a sample of
// icons, logos, QR codes, line art and one photograph, graphics reached 44% at
// most while the photograph sat at 79%, so the cut is placed in the gap rather
// than tuned. It is one sample of each extreme, not a validated constant.
const (
	photographicMidTone = 0.60
	// monochromeChroma separates images whose colour is incidental from those
	// where it carries meaning. Across the sample, a nearly grey photograph
	// reached 3% while every image with real colour in it exceeded 12%.
	monochromeChroma = 0.05
	// chromaticSpread is how far apart a pixel's channels must sit before it
	// counts as coloured rather than as a warm or cool grey.
	chromaticSpread = 40
)

// ProfileImage measures src as the panel will see it: alpha composited over
// white, since that is what an unpainted panel is.
func ProfileImage(src image.Image) (ImageProfile, error) {
	if src == nil {
		return ImageProfile{}, fmt.Errorf("source image must not be nil")
	}
	bounds := src.Bounds()
	if bounds.Empty() {
		return ImageProfile{}, fmt.Errorf("source image bounds must not be empty")
	}

	var histogram [256]int
	total, chromatic := 0, 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b := channelsOverWhite(src.At(x, y))
			histogram[int(0.2126*r+0.7152*g+0.0722*b)]++
			if max(r, max(g, b))-min(r, min(g, b)) > chromaticSpread {
				chromatic++
			}
			total++
		}
	}

	midTone := 0
	for level := 41; level < 215; level++ {
		midTone += histogram[level]
	}
	profile := ImageProfile{
		MidToneFraction:   float64(midTone) / float64(total),
		Threshold:         otsuThreshold(histogram[:], total),
		ChromaticFraction: float64(chromatic) / float64(total),
	}
	profile.Photographic = profile.MidToneFraction > photographicMidTone
	profile.Monochrome = profile.ChromaticFraction < monochromeChroma
	return profile, nil
}

// SuggestOptions turns a measurement into a starting point. It is a
// suggestion, not a decision: DrawImage never profiles anything on its own,
// because a wrong guess buried inside drawing is one the caller cannot see.
func (p ImageProfile) SuggestOptions() ImageOptions {
	options := ImageOptions{
		Fit:      FitContain,
		Sampling: SampleBilinear,
		// An image with no real colour in it has no business reaching the red
		// plane, which fires on anything merely warm.
		DisableRed: p.Monochrome,
	}
	if p.Photographic {
		// Error diffusion holds the tone a photograph is made of.
		options.Dither = DitherFloydSteinberg
		return options
	}
	// Flat artwork wants a hard edge, cut where its own histogram divides.
	options.Dither = DitherThreshold
	options.Threshold = p.Threshold
	return options
}

// channelsOverWhite flattens alpha against the panel's unpainted state, so a
// transparent icon is measured as it will look rather than by whatever colour
// happens to sit under its transparent pixels.
func channelsOverWhite(c color.Color) (r, g, b float64) {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	alpha := float64(n.A) / 255
	return float64(n.R)*alpha + 255*(1-alpha),
		float64(n.G)*alpha + 255*(1-alpha),
		float64(n.B)*alpha + 255*(1-alpha)
}

// EnhanceContrast returns src + amount*(src - blur(src)) in grayscale, which
// lifts local detail while leaving overall tone alone.
//
// A photograph of a dark subject needs this before it reaches one bit: the
// subject thresholds to a single flat mass and features inside it, being
// mid-tone against dark, are carried by a difference in dot density too small
// to see. Amplifying that difference first pushes it across the cut instead.
//
// radius sets which feature size survives and is measured in source pixels, so
// it has to be scaled to whatever the image will be drawn at: too small and
// only the finest texture lifts, too large and the subject turns into a
// silhouette. Flat artwork wants none of this.
//
// The result is grey, so red can no longer be detected in it. Anything that
// needs both local contrast and the red plane has to sharpen luminance while
// keeping chroma, which this does not do.
func EnhanceContrast(src image.Image, radius int, amount float64) (image.Image, error) {
	if src == nil {
		return nil, fmt.Errorf("source image must not be nil")
	}
	bounds := src.Bounds()
	if bounds.Empty() {
		return nil, fmt.Errorf("source image bounds must not be empty")
	}
	if radius < 0 {
		return nil, fmt.Errorf("contrast radius must not be negative, got %d", radius)
	}

	width, height := bounds.Dx(), bounds.Dy()
	gray := make([]float64, width*height)
	for y := range height {
		for x := range width {
			r, g, b := channelsOverWhite(src.At(bounds.Min.X+x, bounds.Min.Y+y))
			gray[y*width+x] = 0.2126*r + 0.7152*g + 0.0722*b
		}
	}
	blurred := blurAxis(blurAxis(gray, width, height, radius, true), width, height, radius, false)

	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	for index, value := range gray {
		level := uint8(min(255, max(0, int(value+amount*(value-blurred[index])))))
		out.SetNRGBA(index%width, index/width, color.NRGBA{R: level, G: level, B: level, A: 0xff})
	}
	return out, nil
}

func blurAxis(values []float64, width, height, radius int, horizontal bool) []float64 {
	out := make([]float64, len(values))
	for y := range height {
		for x := range width {
			sum, count := 0.0, 0
			for offset := -radius; offset <= radius; offset++ {
				sampleX, sampleY := x, y
				if horizontal {
					sampleX += offset
				} else {
					sampleY += offset
				}
				if sampleX >= 0 && sampleX < width && sampleY >= 0 && sampleY < height {
					sum += values[sampleY*width+sampleX]
					count++
				}
			}
			out[y*width+x] = sum / float64(count)
		}
	}
	return out
}

// otsuThreshold picks the cut that minimises variance within the two sides,
// which is the split the histogram itself suggests and needs no tuning.
//
// The result is kept inside [1,254]: an image that is already pure black and
// white makes every cut equivalent and the search settles on an end of the
// range, where zero would additionally collide with ImageOptions treating a
// zero threshold as "unset".
func otsuThreshold(histogram []int, total int) int {
	if total == 0 {
		return 128
	}
	sum := 0.0
	for level, count := range histogram {
		sum += float64(level) * float64(count)
	}
	below, sumBelow := 0, 0.0
	best, bestVariance := 128, -1.0
	for level := range histogram {
		below += histogram[level]
		if below == 0 {
			continue
		}
		above := total - below
		if above == 0 {
			break
		}
		sumBelow += float64(level) * float64(histogram[level])
		meanBelow := sumBelow / float64(below)
		meanAbove := (sum - sumBelow) / float64(above)
		difference := meanBelow - meanAbove
		variance := float64(below) * float64(above) * difference * difference
		if variance > bestVariance {
			bestVariance, best = variance, level
		}
	}
	return min(254, max(1, best))
}
