package markup

import (
	"math"
	"strconv"
	"strings"
)

// parseColour reads a colour written in hexadecimal — three, four, six or
// eight digits — or as one of the colour functions a stylesheet is likely to
// hold: rgb() and rgba(), hsl() and hsla(), and oklab() and oklch(), which is
// how Tailwind 4 writes its palette. Arguments are separated by commas, or by
// spaces with the alpha after a slash. read says whether the colour was
// written in one of those ways; opaque whether it had no transparency. What
// the colour then is to the panel — an ink, a grey, or neither — is for the
// caller to say.
func parseColour(value string) (red, green, blue int, opaque, read bool) {
	value = strings.TrimSpace(value)
	if digits, hex := strings.CutPrefix(value, "#"); hex {
		return parseHex(digits)
	}

	lower := strings.ToLower(value)
	open := strings.IndexByte(lower, '(')
	if open < 0 || !strings.HasSuffix(lower, ")") {
		return 0, 0, 0, false, false
	}
	components, alpha := colourArguments(lower[open+1 : len(lower)-1])
	if len(components) != 3 {
		return 0, 0, 0, false, false
	}
	var (
		rgb [3]float64 // gamma-encoded sRGB, 0 to 1
		ok  = true
	)
	need := func(number float64, good bool) float64 {
		ok = ok && good
		return number
	}
	switch strings.TrimSpace(lower[:open]) {
	case "rgb", "rgba":
		for index, component := range components {
			rgb[index] = need(colourComponent(component, 255)) / 255
		}
	case "hsl", "hsla":
		hue := need(colourHue(components[0]))
		saturation := need(colourComponent(components[1], 100)) / 100
		lightness := need(colourComponent(components[2], 100)) / 100
		rgb = hslToRGB(hue, saturation, lightness)
	case "oklab":
		lightness := need(colourComponent(components[0], 1))
		a := need(colourComponent(components[1], 0.4))
		b := need(colourComponent(components[2], 0.4))
		rgb = oklabToRGB(lightness, a, b)
	case "oklch":
		lightness := need(colourComponent(components[0], 1))
		chroma := math.Max(need(colourComponent(components[1], 0.4)), 0)
		hue := need(colourHue(components[2])) * math.Pi / 180
		rgb = oklabToRGB(lightness, float64(chroma*math.Cos(hue)), float64(chroma*math.Sin(hue)))
	default:
		return 0, 0, 0, false, false
	}
	opaque = true
	if alpha != "" {
		opaque = need(colourComponent(alpha, 1)) >= 1
	}
	if !ok {
		return 0, 0, 0, false, false
	}
	var levels [3]int
	for index, channel := range rgb {
		levels[index] = int(math.Round(float64(math.Min(math.Max(channel, 0), 1) * 255)))
	}
	return levels[0], levels[1], levels[2], opaque, true
}

func parseHex(digits string) (red, green, blue int, opaque, read bool) {
	if len(digits) == 3 || len(digits) == 4 {
		doubled := make([]byte, 0, 8)
		for index := range len(digits) {
			doubled = append(doubled, digits[index], digits[index])
		}
		digits = string(doubled)
	}
	if len(digits) == 6 {
		digits += "ff"
	}
	if len(digits) != 8 {
		return 0, 0, 0, false, false
	}
	parsed, err := strconv.ParseUint(digits, 16, 32)
	if err != nil {
		return 0, 0, 0, false, false
	}
	return int(parsed >> 24), int(parsed >> 16 & 0xff), int(parsed >> 8 & 0xff), parsed&0xff == 0xff, true
}

// colourArguments splits what is inside a colour function into its three
// components and its alpha, in either of the ways CSS writes them.
func colourArguments(inside string) (components []string, alpha string) {
	if strings.Contains(inside, ",") {
		components = strings.Split(inside, ",")
		if len(components) == 4 {
			return components[:3], components[3]
		}
		return components, ""
	}
	colour, after, slashed := strings.Cut(inside, "/")
	if slashed {
		alpha = after
	}
	return strings.Fields(colour), alpha
}

// colourComponent reads a number, or a percentage of full, which is the value
// a component's 100% stands for. none, which CSS allows for a component that
// is missing, is zero.
func colourComponent(text string, full float64) (float64, bool) {
	text = strings.TrimSpace(text)
	if text == "none" {
		return 0, true
	}
	text, percent := strings.CutSuffix(text, "%")
	number, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	if percent {
		number = float64(number / 100 * full)
	}
	return number, true
}

// colourHue reads a hue in degrees: a bare number, or an angle in deg, grad,
// rad or turn.
func colourHue(text string) (float64, bool) {
	text = strings.TrimSpace(text)
	if text == "none" {
		return 0, true
	}
	for _, unit := range []struct {
		suffix  string
		degrees float64
	}{{"deg", 1}, {"grad", 0.9}, {"rad", 180 / math.Pi}, {"turn", 360}} {
		if number, cut := strings.CutSuffix(text, unit.suffix); cut {
			parsed, err := strconv.ParseFloat(number, 64)
			return float64(parsed * unit.degrees), err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
		}
	}
	parsed, err := strconv.ParseFloat(text, 64)
	return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}

// hslToRGB is the conversion CSS Color 4 gives.
func hslToRGB(hue, saturation, lightness float64) [3]float64 {
	hue = math.Mod(math.Mod(hue, 360)+360, 360)
	saturation = math.Min(math.Max(saturation, 0), 1)
	lightness = math.Min(math.Max(lightness, 0), 1)
	channel := func(n float64) float64 {
		k := math.Mod(n+hue/30, 12)
		a := float64(saturation * math.Min(lightness, 1-lightness))
		return lightness - float64(a*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1)))
	}
	return [3]float64{channel(0), channel(8), channel(4)}
}

// oklabToRGB converts Oklab to gamma-encoded sRGB, by the matrices Björn
// Ottosson published with it. Each row of the last matrix sums to one, so a
// colour with no chroma comes out with its red, green and blue equal, and an
// oklch() grey is read as the grey it is; every lightness in steps of 0.00001
// was checked to round to three equal levels.
func oklabToRGB(lightness, a, b float64) [3]float64 {
	l := cube(lightness + float64(0.3963377774*a) + float64(0.2158037573*b))
	m := cube(lightness - float64(0.1055613458*a) - float64(0.0638541728*b))
	s := cube(lightness - float64(0.0894841775*a) - float64(1.2914855480*b))
	return [3]float64{
		srgbEncode(float64(+4.0767416621*l) - float64(3.3077115913*m) + float64(0.2309699292*s)),
		srgbEncode(float64(-1.2684380046*l) + float64(2.6097574011*m) - float64(0.3413193965*s)),
		srgbEncode(float64(-0.0041960863*l) - float64(0.7034186147*m) + float64(1.7076147010*s)),
	}
}

func cube(x float64) float64 { return float64(x * x * x) }

func srgbEncode(linear float64) float64 {
	if linear <= 0.0031308 {
		return float64(12.92 * linear)
	}
	return float64(1.055*math.Pow(linear, 1/2.4)) - 0.055
}
