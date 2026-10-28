package display

import (
	"image"
	"math"
)

// tileIndex files pieces of geometry under the tiles of an area they come near,
// so a pixel asks only about the pieces filed under its own tile instead of
// every piece there is. That is what keeps an outline of thousands of segments,
// or one that runs far past the frame, costing in proportion to what is seen.
//
// Pieces are filed in the order they are added, so a tile lists them in that
// order too, and a search that breaks ties by order breaks them the same way.
type tileIndex struct {
	bounds  image.Rectangle
	size    int
	columns int
	cells   [][]int32
}

// maxTiles bounds how many cells an index allocates: a larger area is covered
// by larger tiles rather than by more of them.
const maxTiles = 4096

func newTileIndex(bounds image.Rectangle) *tileIndex {
	if bounds.Empty() {
		return &tileIndex{}
	}
	size := 16
	if area := float64(float64(bounds.Dx()) * float64(bounds.Dy())); area/float64(size*size) > maxTiles {
		size = int(math.Ceil(math.Sqrt(area / maxTiles)))
	}
	columns := (bounds.Dx() + size - 1) / size
	rows := (bounds.Dy() + size - 1) / size
	for columns*rows > maxTiles {
		size *= 2
		columns = (bounds.Dx() + size - 1) / size
		rows = (bounds.Dy() + size - 1) / size
	}
	return &tileIndex{bounds: bounds, size: size, columns: columns, cells: make([][]int32, columns*rows)}
}

// add files id under every tile box overlaps.
func (t *tileIndex) add(id int, box image.Rectangle) {
	box = box.Intersect(t.bounds)
	if box.Empty() {
		return
	}
	minColumn := (box.Min.X - t.bounds.Min.X) / t.size
	maxColumn := (box.Max.X - 1 - t.bounds.Min.X) / t.size
	minRow := (box.Min.Y - t.bounds.Min.Y) / t.size
	maxRow := (box.Max.Y - 1 - t.bounds.Min.Y) / t.size
	for row := minRow; row <= maxRow; row++ {
		for column := minColumn; column <= maxColumn; column++ {
			cell := row*t.columns + column
			t.cells[cell] = append(t.cells[cell], int32(id))
		}
	}
}

// at lists the pieces filed under the tile holding a pixel.
func (t *tileIndex) at(x, y int) []int32 {
	if !image.Pt(x, y).In(t.bounds) {
		return nil
	}
	return t.cells[(y-t.bounds.Min.Y)/t.size*t.columns+(x-t.bounds.Min.X)/t.size]
}

// segmentBox is the box a segment's pixels fall in, grown by margin on every
// side.
func segmentBox(from, to image.Point, margin int) image.Rectangle {
	return image.Rect(min(from.X, to.X)-margin, min(from.Y, to.Y)-margin,
		max(from.X, to.X)+margin+1, max(from.Y, to.Y)+margin+1)
}

// pointBox is the box of pixels within margin of a point.
func pointBox(point image.Point, margin int) image.Rectangle {
	return image.Rect(point.X-margin, point.Y-margin, point.X+margin+1, point.Y+margin+1)
}

// clipSegment is the part of a segment, as fractions of its length, that lies
// inside area; ok is false when none of it does.
func clipSegment(from, to floatPoint, area image.Rectangle) (low, high float64, ok bool) {
	low, high = 0, 1
	dx, dy := to.x-from.x, to.y-from.y
	edges := [4][2]float64{
		{-dx, from.x - float64(area.Min.X)},
		{dx, float64(area.Max.X) - from.x},
		{-dy, from.y - float64(area.Min.Y)},
		{dy, float64(area.Max.Y) - from.y},
	}
	for _, edge := range edges {
		p, q := edge[0], edge[1]
		if p == 0 {
			if q < 0 {
				return 0, 0, false
			}
			continue
		}
		r := q / p
		if p < 0 {
			low = math.Max(low, r)
		} else {
			high = math.Min(high, r)
		}
		if low > high {
			return 0, 0, false
		}
	}
	return low, high, true
}
