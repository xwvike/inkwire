package display

import (
	"cmp"
	"image"
	"slices"
	"sort"
)

// polygonRegion answers pointInPath for every pixel of some contours, a row at
// a time instead of an edge at a time per pixel.
//
// The per-pixel test counts the edges a pixel is strictly left of, and an edge
// crossing row y at X is one that pixel x is left of exactly when x is less
// than X rounded up. So a row's inside is the runs between its crossings, once
// those are sorted, and the boundary — a pixel an edge passes through exactly —
// is added to it. That is the same answer for every pixel, worked out once per
// row and then looked up.
//
// Edges are filed under bands of rows across the area the caller will ask
// about, and a row is worked out only when it is first asked about, so the
// cost follows what is drawn rather than how long the outline is.
type polygonRegion struct {
	edges  []regionEdge
	bounds image.Rectangle
	area   image.Rectangle
	band   int
	bands  [][]int32
	rows   map[int][]regionSpan
}

type regionEdge struct{ from, to image.Point }

// regionSpan is a closed run of pixels, first to last.
type regionSpan struct{ first, last int }

// maxBands bounds how many bands of rows a region files its edges under.
const maxBands = 1024

// newPolygonRegion is the even-odd region of the contours that have at least
// three points, boundary included, with its rows filed for the rows of area.
func newPolygonRegion(contours [][]image.Point, area image.Rectangle) *polygonRegion {
	region := &polygonRegion{rows: map[int][]regionSpan{}}
	first := true
	for _, contour := range contours {
		if len(contour) < 3 {
			continue
		}
		bounds := polygonBounds(contour)
		if first {
			region.bounds, first = bounds, false
		} else {
			region.bounds = region.bounds.Union(bounds)
		}
		previous := contour[len(contour)-1]
		for _, current := range contour {
			region.edges = append(region.edges, regionEdge{from: previous, to: current})
			previous = current
		}
	}
	area = area.Intersect(region.bounds)
	if area.Empty() {
		return region
	}
	region.area = area
	region.band = max(16, (area.Dy()+maxBands-1)/maxBands)
	region.bands = make([][]int32, (area.Dy()+region.band-1)/region.band)
	for id, edge := range region.edges {
		low := max(min(edge.from.Y, edge.to.Y), area.Min.Y)
		high := min(max(edge.from.Y, edge.to.Y), area.Max.Y-1)
		for band := (low - area.Min.Y) / region.band; low <= high && band <= (high-area.Min.Y)/region.band; band++ {
			region.bands[band] = append(region.bands[band], int32(id))
		}
	}
	return region
}

func (r *polygonRegion) contains(x, y int) bool {
	if !image.Pt(x, y).In(r.bounds) {
		return false
	}
	spans, ok := r.rows[y]
	if !ok {
		spans = r.row(y)
		r.rows[y] = spans
	}
	index := sort.Search(len(spans), func(i int) bool { return spans[i].last >= x })
	return index < len(spans) && spans[index].first <= x
}

// row works out the runs of row y, from the edges filed for it or, for a row
// outside the area the edges were filed for, from all of them.
func (r *polygonRegion) row(y int) []regionSpan {
	var crossings []int
	var spans []regionSpan
	visit := func(edge regionEdge) {
		from, to := edge.from, edge.to
		if y < min(from.Y, to.Y) || y > max(from.Y, to.Y) {
			return
		}
		if from.Y == to.Y {
			spans = append(spans, regionSpan{min(from.X, to.X), max(from.X, to.X)})
			return
		}
		// X = to.X + (from.X-to.X)(y-to.Y)/(from.Y-to.Y), as an exact fraction.
		numerator := int64(from.X-to.X) * int64(y-to.Y)
		denominator := int64(from.Y - to.Y)
		if denominator < 0 {
			numerator, denominator = -numerator, -denominator
		}
		if numerator%denominator == 0 {
			spans = append(spans, regionSpan{to.X + int(numerator/denominator), to.X + int(numerator/denominator)})
		}
		if (to.Y > y) != (from.Y > y) {
			crossings = append(crossings, to.X+int(ceilDivide(numerator, denominator)))
		}
	}
	if y >= r.area.Min.Y && y < r.area.Max.Y {
		for _, id := range r.bands[(y-r.area.Min.Y)/r.band] {
			visit(r.edges[id])
		}
	} else {
		for _, edge := range r.edges {
			visit(edge)
		}
	}
	// A pixel is inside when it is left of an odd number of crossings. A
	// closed contour crosses a row an even number of times, so with them
	// sorted that is from each odd-numbered crossing up to the pixel before
	// the next.
	slices.Sort(crossings)
	for index := 0; index+1 < len(crossings); index += 2 {
		if crossings[index] < crossings[index+1] {
			spans = append(spans, regionSpan{crossings[index], crossings[index+1] - 1})
		}
	}
	return mergeSpans(spans)
}

func ceilDivide(numerator, denominator int64) int64 {
	quotient := numerator / denominator
	if numerator%denominator != 0 && numerator > 0 {
		quotient++
	}
	return quotient
}

func mergeSpans(spans []regionSpan) []regionSpan {
	if len(spans) < 2 {
		return spans
	}
	slices.SortFunc(spans, func(a, b regionSpan) int { return cmp.Compare(a.first, b.first) })
	merged := spans[:1]
	for _, span := range spans[1:] {
		last := &merged[len(merged)-1]
		if span.first <= last.last+1 {
			last.last = max(last.last, span.last)
			continue
		}
		merged = append(merged, span)
	}
	return merged
}
