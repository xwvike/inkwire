package compose

import (
	"fmt"
	"image"

	"github.com/xwvike/inkwire/internal/display"
)

// Track is one column or row of a grid.
//
// A track is sized one of three ways, and the difference is the whole point of
// having a grid at all. A stated length is settled. An automatic track takes
// the size of the widest thing in it, which is what makes a column line up
// across rows that know nothing about each other. A fractional track divides
// what is left over.
type Track struct {
	// Size is a stated length. When it is absent the track is automatic,
	// unless Fraction says otherwise.
	Size Length
	// Fraction is the track's share of the space no other track claimed,
	// counted in fr units. Zero means it takes none.
	Fraction int
}

func autoTrack() Track           { return Track{} }
func (t Track) automatic() bool  { return !t.Size.IsSet() && t.Fraction == 0 }
func (t Track) fractional() bool { return !t.Size.IsSet() && t.Fraction > 0 }
func (t Track) valid() bool      { return t.Size.valid() && t.Fraction >= 0 }
func tracksValid(tracks []Track) bool {
	for _, track := range tracks {
		if !track.valid() {
			return false
		}
	}
	return true
}

// GridChild places one node in the grid. Column and Row are one-based line
// numbers; zero means take the next free cell.
type GridChild struct {
	Node                Node
	Column, Row         int
	ColumnSpan, RowSpan int
	// AlignSelf and JustifySelf place the node inside its cell, across and
	// along the row respectively. Absent, the grid's own settings apply.
	AlignSelf, JustifySelf *CrossAlignment
}

func (c GridChild) columnSpan() int { return max(1, c.ColumnSpan) }
func (c GridChild) rowSpan() int    { return max(1, c.RowSpan) }

// Grid lays children out on declared and automatically created tracks.
//
// It exists because a row of rows cannot line up. Three separate rows each
// containing a label and a value have no way to agree on how wide the label
// column should be; each measures its own and they come out ragged. A grid
// measures the column once, across every row that uses it.
type Grid struct {
	Size              image.Point
	Columns, Rows     []Track
	ColumnGap, RowGap int
	// AlignItems places children across their row, JustifyItems along it.
	AlignItems, JustifyItems CrossAlignment
	Children                 []GridChild
}

func (Grid) composeNode() {}

func (g Grid) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if err := g.validate(path); err != nil {
		return image.Point{}, err
	}
	placement, columns, rows, err := g.place(ctx, maximum, path)
	if err != nil {
		return image.Point{}, err
	}
	natural := image.Pt(
		intrinsicTracks(columns, placement.columnContent)+g.ColumnGap*(len(columns)-1),
		intrinsicTracks(rows, placement.rowContent)+g.RowGap*(len(rows)-1),
	)
	return preferredSize(natural, g.Size, maximum)
}

func (g Grid) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	if err := g.validate(path); err != nil {
		return err
	}
	placement, columns, rows, err := g.place(ctx, bounds.Size(), path)
	if err != nil {
		return err
	}
	implicitColumns := len(columns) - len(g.columnsOrOne())
	implicitRows := len(rows) - len(g.rowsOrOne())
	if implicitColumns != 0 || implicitRows != 0 {
		ctx.report.GridExpansions = append(ctx.report.GridExpansions, GridExpansion{
			Path: path, ImplicitColumns: implicitColumns, ImplicitRows: implicitRows,
		})
	}
	columnSizes := g.tracksFor(columns, placement.columnContent, bounds.Dx(), g.ColumnGap, true)
	rowSizes := g.tracksFor(rows, placement.rowContent, bounds.Dy(), g.RowGap, false)
	columnOffsets := offsets(columnSizes, g.ColumnGap)
	rowOffsets := offsets(rowSizes, g.RowGap)

	if used := sum(columnSizes) + g.ColumnGap*(len(columnSizes)-1); used > bounds.Dx() {
		ctx.warn(path, "layout-overflow", fmt.Sprintf(
			"the columns need %d pixels, only %d are available", used, bounds.Dx()))
	}
	if used := sum(rowSizes) + g.RowGap*(len(rowSizes)-1); used > bounds.Dy() {
		ctx.warn(path, "layout-overflow", fmt.Sprintf(
			"the rows need %d pixels, only %d are available", used, bounds.Dy()))
	}

	for index, child := range g.Children {
		at := placement.cells[index]
		nodePath := childPath(path, "children", index)
		cell := image.Rect(
			bounds.Min.X+columnOffsets[at.column],
			bounds.Min.Y+rowOffsets[at.row],
			bounds.Min.X+columnOffsets[at.column]+span(columnSizes, at.column, at.columnSpan, g.ColumnGap),
			bounds.Min.Y+rowOffsets[at.row]+span(rowSizes, at.row, at.rowSpan, g.RowGap),
		)
		placedBounds, err := g.placeInCell(ctx, child, cell, nodePath)
		if err != nil {
			return err
		}
		if err := ctx.paintWithContaining(child.Node, list, placedBounds, bounds, nodePath); err != nil {
			return err
		}
	}
	return nil
}

// placeInCell shrinks the child to its measured size on whichever axis it is
// not being stretched along, which is what the alignment settings decide.
func (g Grid) placeInCell(ctx *compileContext, child GridChild, cell image.Rectangle, path string) (image.Rectangle, error) {
	across, along := g.AlignItems, g.JustifyItems
	if child.AlignSelf != nil {
		across = *child.AlignSelf
	}
	if child.JustifySelf != nil {
		along = *child.JustifySelf
	}
	if across == CrossStretch && along == CrossStretch {
		return cell, nil
	}
	measured, err := child.Node.measure(ctx, cell.Size(), path)
	if err != nil {
		return image.Rectangle{}, err
	}
	x, width := place(along, cell.Min.X, cell.Dx(), measured.X)
	y, height := place(across, cell.Min.Y, cell.Dy(), measured.Y)
	return image.Rect(x, y, x+width, y+height), nil
}

func place(alignment CrossAlignment, start, available, measured int) (int, int) {
	if alignment == CrossStretch || measured >= available {
		return start, available
	}
	switch alignment {
	case CrossCenter:
		return start + (available-measured)/2, measured
	case CrossEnd:
		return start + available - measured, measured
	}
	return start, measured
}

type cell struct {
	column, row         int
	columnSpan, rowSpan int
}

type placement struct {
	cells         []cell
	columnContent []int
	rowContent    []int
}

// place assigns every child a cell and records how large each automatic track
// has to be to hold what landed in it.
//
// Auto placement fills the grid row by row, taking the next free cell, which
// is what a document without explicit positions expects.
func (g Grid) place(ctx *compileContext, maximum image.Point, path string) (placement, []Track, []Track, error) {
	columns := append([]Track(nil), g.columnsOrOne()...)
	rows := append([]Track(nil), g.rowsOrOne()...)
	taken := map[[2]int]bool{}
	result := placement{cells: make([]cell, len(g.Children))}

	// CSS determines the implicit column grid before auto-placement. A stated
	// column can extend it directly, while an auto-positioned item can require
	// enough columns to hold its span.
	for index, child := range g.Children {
		nodePath := childPath(path, "children", index)
		if nilNode(child.Node) {
			return placement{}, nil, nil, fmt.Errorf("%s: node must not be nil", nodePath)
		}
		if child.Column < 0 || child.Row < 0 {
			return placement{}, nil, nil, fmt.Errorf("%s: column and row must not be negative", nodePath)
		}
		at := cell{columnSpan: child.columnSpan(), rowSpan: child.rowSpan()}
		result.cells[index] = at
		requiredColumns := at.columnSpan
		if child.Column > 0 {
			requiredColumns = child.Column - 1 + at.columnSpan
		}
		columns = ensureTracks(columns, requiredColumns)
		if child.Row > 0 {
			rows = ensureTracks(rows, child.Row-1+at.rowSpan)
		}
	}

	// Fully positioned items are placed first and may overlap deliberately,
	// just as CSS grid items with both lines stated can occupy the same area.
	for index, child := range g.Children {
		if child.Column == 0 || child.Row == 0 {
			continue
		}
		at := result.cells[index]
		at.column, at.row = child.Column-1, child.Row-1
		result.cells[index] = at
		occupy(taken, at)
	}

	// Items locked to a row search that row from the first column. When no
	// declared cell fits, the implicit grid grows to the right.
	for index, child := range g.Children {
		if child.Row == 0 || child.Column != 0 {
			continue
		}
		at := result.cells[index]
		at.row = child.Row - 1
		for column := 0; ; column++ {
			columns = ensureTracks(columns, column+at.columnSpan)
			if free(taken, column, at.row, at.columnSpan, at.rowSpan) {
				at.column = column
				break
			}
		}
		result.cells[index] = at
		occupy(taken, at)
		rows = ensureTracks(rows, at.row+at.rowSpan)
	}

	// The remaining items stay in document order and use one sparse row-major
	// cursor. A stated column moves the cursor to that column and searches
	// downward; a fully automatic item searches across and then down.
	cursorColumn, cursorRow := 0, 0
	for index, child := range g.Children {
		if child.Row != 0 {
			continue
		}
		at := result.cells[index]
		if child.Column > 0 {
			column := child.Column - 1
			if column < cursorColumn {
				cursorRow++
			}
			cursorColumn = column
			for !free(taken, cursorColumn, cursorRow, at.columnSpan, at.rowSpan) {
				cursorRow++
			}
			at.column, at.row = cursorColumn, cursorRow
		} else {
			for {
				if cursorColumn+at.columnSpan > len(columns) {
					cursorColumn, cursorRow = 0, cursorRow+1
					continue
				}
				if free(taken, cursorColumn, cursorRow, at.columnSpan, at.rowSpan) {
					at.column, at.row = cursorColumn, cursorRow
					cursorColumn += at.columnSpan
					break
				}
				cursorColumn++
			}
		}
		result.cells[index] = at
		occupy(taken, at)
		rows = ensureTracks(rows, at.row+at.rowSpan)
	}

	result.columnContent = make([]int, len(columns))
	result.rowContent = make([]int, len(rows))
	for index, child := range g.Children {
		at := result.cells[index]
		size, err := child.Node.measure(ctx, maximum, childPath(path, "children", index))
		if err != nil {
			return placement{}, nil, nil, err
		}
		// A child spanning several tracks says nothing about any one of them,
		// so only single-track children set an automatic track's size.
		if at.columnSpan == 1 {
			result.columnContent[at.column] = max(result.columnContent[at.column], size.X)
		}
		if at.rowSpan == 1 {
			result.rowContent[at.row] = max(result.rowContent[at.row], size.Y)
		}
	}
	return result, columns, rows, nil
}

func ensureTracks(tracks []Track, count int) []Track {
	for len(tracks) < count {
		tracks = append(tracks, autoTrack())
	}
	return tracks
}

func free(taken map[[2]int]bool, column, row, columns, rows int) bool {
	for r := row; r < row+rows; r++ {
		for c := column; c < column+columns; c++ {
			if taken[[2]int{c, r}] {
				return false
			}
		}
	}
	return true
}

func occupy(taken map[[2]int]bool, at cell) {
	for r := at.row; r < at.row+at.rowSpan; r++ {
		for c := at.column; c < at.column+at.columnSpan; c++ {
			taken[[2]int{c, r}] = true
		}
	}
}

// tracksFor turns the track list into pixel sizes: stated lengths first, then
// automatic tracks from what they hold, and whatever survives that is divided
// among the fractional ones.
func (g Grid) tracksFor(tracks []Track, content []int, available, gap int, horizontal bool) []int {
	sizes := make([]int, len(tracks))
	remaining := available - gap*(len(tracks)-1)
	fractions := 0
	for index, track := range tracks {
		switch {
		case track.Size.IsSet():
			sizes[index], _ = track.Size.Resolve(available)
		case track.automatic():
			if index < len(content) {
				sizes[index] = content[index]
			}
		default:
			fractions += track.Fraction
			continue
		}
		remaining -= sizes[index]
	}
	if remaining <= 0 {
		return sizes
	}
	if fractions == 0 {
		// Nothing claimed the leftover in fractions, so the automatic tracks
		// take it. This is what a grid does by default: align-content behaves
		// as stretch, and stretch only reaches tracks nobody sized. Without
		// it a grid of one automatic row is as tall as its contents and no
		// taller, which is not what a page filling its panel expects.
		return stretchAutomatic(tracks, sizes, remaining)
	}
	given := 0
	for index, track := range tracks {
		if !track.fractional() {
			continue
		}
		sizes[index] = remaining * track.Fraction / fractions
		given += sizes[index]
	}
	// Integer division leaves a few pixels over; they go to the fractional
	// tracks in order so the grid fills its box exactly.
	for index, track := range tracks {
		if given == remaining {
			break
		}
		if track.fractional() {
			sizes[index]++
			given++
		}
	}
	return sizes
}

// intrinsicTracks is how large the grid wants to be before it has been given a
// box. A fractional track wants nothing: it is a share of a leftover that does
// not exist yet, and counting it here makes the grid claim the whole page and
// push everything beside it off the layout.
func intrinsicTracks(tracks []Track, content []int) int {
	total := 0
	for index, track := range tracks {
		switch {
		case track.fractional():
		case track.Size.IsSet():
			if stated, ok := track.Size.intrinsic(); ok {
				total += stated
			}
		case index < len(content):
			total += content[index]
		}
	}
	return total
}

// stretchAutomatic shares what is left between the tracks nobody sized.
func stretchAutomatic(tracks []Track, sizes []int, remaining int) []int {
	count := 0
	for _, track := range tracks {
		if track.automatic() {
			count++
		}
	}
	if count == 0 {
		return sizes
	}
	given, share := 0, remaining/count
	for index, track := range tracks {
		if track.automatic() {
			sizes[index] += share
			given += share
		}
	}
	for index, track := range tracks {
		if given == remaining {
			break
		}
		if track.automatic() {
			sizes[index]++
			given++
		}
	}
	return sizes
}

func (g Grid) columnsOrOne() []Track {
	if len(g.Columns) == 0 {
		return []Track{autoTrack()}
	}
	return g.Columns
}

func (g Grid) rowsOrOne() []Track {
	if len(g.Rows) == 0 {
		return []Track{autoTrack()}
	}
	return g.Rows
}

func (g Grid) validate(path string) error {
	if !validSize(g.Size) {
		return fmt.Errorf("%s: size must not be negative, got %v", path, g.Size)
	}
	if g.ColumnGap < 0 || g.RowGap < 0 {
		return fmt.Errorf("%s: gaps must not be negative", path)
	}
	if !tracksValid(g.Columns) || !tracksValid(g.Rows) {
		return fmt.Errorf("%s: track sizes must not be negative", path)
	}
	return nil
}

func offsets(sizes []int, gap int) []int {
	result := make([]int, len(sizes)+1)
	for index, size := range sizes {
		result[index+1] = result[index] + size + gap
	}
	return result
}

func span(sizes []int, from, count, gap int) int {
	total := 0
	for index := from; index < from+count && index < len(sizes); index++ {
		total += sizes[index]
		if index > from {
			total += gap
		}
	}
	return total
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
