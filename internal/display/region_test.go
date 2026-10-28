package display

import (
	"image"
	"math/rand"
	"testing"
)

// The row-at-a-time region gives the per-pixel even-odd answer for every
// pixel: inside, outside and on the boundary, for contours that cross
// themselves, nest, double back, lie flat or are too short to count.
func TestAPolygonRegionIsThePerPixelTest(t *testing.T) {
	random := rand.New(rand.NewSource(7))
	for trial := 0; trial < 3000; trial++ {
		span := 4 + random.Intn(60)
		var contours []pathContour
		var outlines [][]image.Point
		for count := 1 + random.Intn(3); count > 0; count-- {
			var points []image.Point
			for n := 1 + random.Intn(9); n > 0; n-- {
				point := image.Pt(random.Intn(span)-span/4, random.Intn(span)-span/4)
				switch random.Intn(4) {
				case 0: // share a row with the last point, for flat edges
					if len(points) > 0 {
						point.Y = points[len(points)-1].Y
					}
				case 1: // repeat a point, for zero-length edges
					if len(points) > 0 {
						point = points[random.Intn(len(points))]
					}
				}
				points = append(points, point)
			}
			contours = append(contours, pathContour{points: points})
			outlines = append(outlines, points)
		}
		// Ask about part of the area only, so rows outside it are worked out
		// the slow way too.
		area := image.Rect(-span/2, -span/2+random.Intn(span), span, span)
		region := newPolygonRegion(outlines, area)
		for y := -span / 2; y < span*5/4; y++ {
			for x := -span / 2; x < span*5/4; x++ {
				if got, want := region.contains(x, y), pointInPath(image.Pt(x, y), contours); got != want {
					t.Fatalf("trial %d: (%d,%d) is %v, want %v; contours %v", trial, x, y, got, want, outlines)
				}
			}
		}
	}
}

// Erosion by distance transform keeps exactly the pixels whose whole disc is
// set, at every radius, whatever the shape and wherever it meets the edge.
func TestErosionIsTheDiscTest(t *testing.T) {
	random := rand.New(rand.NewSource(11))
	for trial := 0; trial < 600; trial++ {
		bounds := image.Rect(random.Intn(9)-4, random.Intn(9)-4, 0, 0)
		bounds.Max = bounds.Min.Add(image.Pt(1+random.Intn(50), 1+random.Intn(50)))
		density := random.Float64()
		blobs := random.Intn(4)
		m := rasterizeMask(bounds, func(x, y int) bool {
			if blobs > 0 {
				return (x*x+y*y*2)%(37+blobs*11) > 5 || random.Float64() < density/8
			}
			return random.Float64() < 0.6+density*0.4
		})
		for radius := 0; radius <= 14; radius++ {
			got, want := m.erode(radius), m.erodeByDisc(radius)
			for index := range want.bits {
				if got.bits[index] != want.bits[index] {
					t.Fatalf("trial %d radius %d: pixel %d is %v, want %v", trial, radius, index, got.bits[index], want.bits[index])
				}
			}
		}
	}
}
