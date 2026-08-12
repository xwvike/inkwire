// Package compose turns explicit user input into display-layer drawing
// parameters and commands. It may suggest how lossy content such as an RGB
// image should be reduced for the panel, but it does not infer or decorate the
// user's content.
package compose

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/xwvike/inkwire/internal/display"
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
	// RedSeparation is how convincingly red the pixels are that would reach the
	// red plane: the median distance between their red and green channels.
	// The plane's test admits anything merely warm, so skin, pink hair and
	// wood all qualify and can flood a picture with red that is not in it.
	// Genuine red sits far from that: across the sample, images whose red
	// carries meaning measured 122 and above while incidental warmth reached
	// 72 at most. Zero when nothing would reach the plane at all.
	RedSeparation int
	// LostToLuminance is the share of the image that is plainly not paper and
	// yet lands on the paper side of Threshold, because its colour is light
	// even though it is vivid. Artwork whose shapes differ in hue at one
	// brightness disappears this way; ToneByColourDistance is the answer.
	LostToLuminance float64
	// Photographic reports whether the image reads as continuous tone.
	Photographic bool
	// RedIsMeaningful reports whether the red plane carries signal rather than
	// a warm cast.
	RedIsMeaningful bool
	// ColourCarriesStructure reports that reducing this image by brightness
	// would throw away much of what is in it.
	ColourCarriesStructure bool
}

// photographicMidTone separates the two kinds. Measured across a sample of
// icons, logos, QR codes, line art and one photograph, graphics reached 44% at
// most while the photograph sat at 79%, so the cut is placed in the gap rather
// than tuned. It is one sample of each extreme, not a validated constant.
const (
	photographicMidTone = 0.60
	// meaningfulRedSeparation is where genuine red parts from warmth. Measured
	// across the sample: real red measured 122 to 186, incidental warmth 9 to
	// 72, so the cut sits in the gap rather than being tuned to either side.
	meaningfulRedSeparation = 100
	// structuralColourLoss is how much an image may lose to a brightness cut
	// before that cut is the wrong tool. Across the sample every drawing lost
	// under 6% except the one built from pastel wedges, which lost 32%.
	structuralColourLoss = 0.15
	// paperMargin is how far from white a pixel must sit to count as ink at
	// all, rather than as a tint of the page.
	paperMargin = 60
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

	defaults := display.DefaultImageOptions()
	var histogram [256]int
	// separations counts, for each distance between the red and green
	// channels, how many pixels that would reach the red plane show it.
	var separations [256]int
	total, redCandidates := 0, 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b := channelsOverWhite(src.At(x, y))
			histogram[int(0.2126*r+0.7152*g+0.0722*b)]++
			if r > float64(defaults.RedThreshold) && g < float64(defaults.RedMaxGreen) {
				separations[int(max(0, min(255, r-g)))]++
				redCandidates++
			}
			total++
		}
	}

	midTone := 0
	for level := 41; level < 215; level++ {
		midTone += histogram[level]
	}
	profile := ImageProfile{
		MidToneFraction: float64(midTone) / float64(total),
		Threshold:       otsuThreshold(histogram[:], total),
		RedSeparation:   medianOf(separations[:], redCandidates),
	}
	profile.Photographic = profile.MidToneFraction > photographicMidTone
	profile.RedIsMeaningful = profile.RedSeparation >= meaningfulRedSeparation

	// How much a brightness cut would discard can only be counted once that
	// cut is known, so it takes a second pass.
	lost := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b := channelsOverWhite(src.At(x, y))
			if paperDistance(r, g, b) <= paperMargin {
				continue // this really is paper
			}
			if 0.2126*r+0.7152*g+0.0722*b > float64(profile.Threshold) {
				lost++
			}
		}
	}
	profile.LostToLuminance = float64(lost) / float64(total)
	// A photograph is made of brightness, so losing some of it to the cut is
	// expected and error diffusion is what answers it, not a different tone.
	profile.ColourCarriesStructure = !profile.Photographic &&
		profile.LostToLuminance > structuralColourLoss
	return profile, nil
}

func medianOf(counts []int, total int) int {
	if total == 0 {
		return 0
	}
	seen := 0
	for value, count := range counts {
		seen += count
		if seen*2 >= total {
			return value
		}
	}
	return 0
}

// SuggestOptions turns a measurement into a starting point. It is a
// suggestion, not a decision: DrawImage never profiles anything on its own,
// because a wrong guess buried inside drawing is one the caller cannot see.
func (p ImageProfile) SuggestOptions() display.ImageOptions {
	options := display.ImageOptions{
		Fit:      display.FitContain,
		Sampling: display.SampleBilinear,
		// Warmth is not red. Leaving the plane on for an image whose reds are
		// skin or wood floods it with colour the image does not contain.
		DisableRed: !p.RedIsMeaningful,
	}
	if p.Photographic {
		// Error diffusion holds the tone a photograph is made of.
		options.Dither = display.DitherFloydSteinberg
		return options
	}
	// Flat artwork wants a hard edge, cut where its own histogram divides.
	options.Dither = display.DitherThreshold
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

// ToneByColourDistance rewrites an image so that tone means "how far this pixel
// is from the paper" rather than "how bright it is".
//
// Flat artwork often carries its structure in hue: the shapes differ in colour
// while sitting at much the same brightness. Reduced by luminance, a yellow
// shape at 218 and a cyan one at 197 are both simply lighter than the cut and
// vanish into the page, taking the drawing with them. Measured as distance from
// white instead, both are plainly ink.
//
// This is wrong for a photograph, where brightness is the subject; it is meant
// for artwork whose colours sit on a light ground, which is what
// ImageProfile.ColourCarriesStructure identifies.
//
// Pixels that would reach the red plane are passed through as red rather than
// flattened to grey. Every other pass that rewrites tone has quietly emptied
// the red plane on the way, because grey cannot satisfy a test that wants red
// above green; there is no reason to throw away an ink the panel has. Whether
// the red is actually drawn remains ImageOptions.DisableRed's decision.
//
// The result carries a different distribution of tone from the original, so a
// threshold measured before this runs no longer describes it. Profile the
// result to get the cut, as SuggestOptionsFor does.
func ToneByColourDistance(src image.Image) (image.Image, error) {
	if src == nil {
		return nil, fmt.Errorf("source image must not be nil")
	}
	bounds := src.Bounds()
	if bounds.Empty() {
		return nil, fmt.Errorf("source image bounds must not be empty")
	}
	defaults := display.DefaultImageOptions()
	out := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := range bounds.Dy() {
		for x := range bounds.Dx() {
			r, g, b := channelsOverWhite(src.At(bounds.Min.X+x, bounds.Min.Y+y))
			if r > float64(defaults.RedThreshold) && g < float64(defaults.RedMaxGreen) {
				// Passed through unchanged rather than replaced with pure red:
				// the colour already satisfies the red test, and substituting
				// a saturated one would drag its luminance to 54 and skew any
				// threshold measured from the result.
				out.SetNRGBA(x, y, color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 0xff})
				continue
			}
			level := uint8(min(255, max(0, 255-int(paperDistance(r, g, b)))))
			out.SetNRGBA(x, y, color.NRGBA{R: level, G: level, B: level, A: 0xff})
		}
	}
	return out, nil
}

// paperDistance is how far a colour sits from white, on the same 0..255 scale
// as a channel so it can stand in for tone.
func paperDistance(r, g, b float64) float64 {
	dr, dg, db := 255-r, 255-g, 255-b
	return math.Sqrt((dr*dr + dg*dg + db*db) / 3)
}

// SuggestOptionsFor is SuggestOptions for an image that has been rewritten
// since it was profiled. A pass such as ToneByColourDistance or
// EnhanceContrast moves tone around, which leaves the measured cut describing
// pixels that no longer exist; the cut is taken from the drawn image while
// every other decision stays with the original measurement, because those
// decisions are about what the picture is and that has not changed.
func (p ImageProfile) SuggestOptionsFor(drawn image.Image) (display.ImageOptions, error) {
	options := p.SuggestOptions()
	if options.Dither != display.DitherThreshold {
		return options, nil
	}
	redrawn, err := ProfileImage(drawn)
	if err != nil {
		return options, err
	}
	options.Threshold = redrawn.Threshold
	return options, nil
}
