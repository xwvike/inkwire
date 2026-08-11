package display

import (
	"image"
	"testing"
)

func newTestFrame(t *testing.T, width, height int) *Frame {
	t.Helper()
	frame, err := NewFrame(width, height, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func assertInk(t *testing.T, frame *Frame, x, y int, want Ink) {
	t.Helper()
	got, ok := frame.InkAt(x, y)
	if !ok {
		t.Fatalf("pixel (%d,%d) is outside frame", x, y)
	}
	if got != want {
		t.Fatalf("pixel (%d,%d) = %d, want %d", x, y, got, want)
	}
}

func assertInkConnected(t *testing.T, frame *Frame, ink Ink) {
	t.Helper()
	var start image.Point
	total := 0
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			got, _ := frame.InkAt(x, y)
			if got == ink {
				if total == 0 {
					start = image.Pt(x, y)
				}
				total++
			}
		}
	}
	if total == 0 {
		t.Fatalf("frame contains no ink %d", ink)
	}

	visited := map[image.Point]bool{start: true}
	queue := []image.Point{start}
	for head := 0; head < len(queue); head++ {
		point := queue[head]
		for yOffset := -1; yOffset <= 1; yOffset++ {
			for xOffset := -1; xOffset <= 1; xOffset++ {
				if xOffset == 0 && yOffset == 0 {
					continue
				}
				neighbor := point.Add(image.Pt(xOffset, yOffset))
				if visited[neighbor] {
					continue
				}
				got, ok := frame.InkAt(neighbor.X, neighbor.Y)
				if ok && got == ink {
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}
	}
	if len(visited) != total {
		t.Fatalf("ink %d has %d connected pixels out of %d", ink, len(visited), total)
	}
}
