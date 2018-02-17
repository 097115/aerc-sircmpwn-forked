package ui

import (
	"fmt"
	"math"
)

type Grid struct {
	Rows         []DimSpec
	rowLayout    []dimLayout
	Columns      []DimSpec
	columnLayout []dimLayout
	Cells        []*GridCell
	onInvalidate func(d Drawable)
	invalid      bool
}

const (
	SIZE_EXACT  = iota
	SIZE_WEIGHT = iota
)

// Specifies the layout of a single row or column
type DimSpec struct {
	// One of SIZE_EXACT or SIZE_WEIGHT
	Strategy int
	// If Strategy = SIZE_EXACT, this is the number of cells this dim shall
	// occupy. If SIZE_WEIGHT, the space left after all exact dims are measured
	// is distributed amonst the remaining dims weighted by this value.
	Size int
}

// Used to cache layout of each row/column
type dimLayout struct {
	Offset int
	Size   int
}

type GridCell struct {
	Row     int
	Column  int
	RowSpan int
	ColSpan int
	Content Drawable
	invalid bool
}

func NewGrid() *Grid {
	return &Grid{invalid: true}
}

func (cell *GridCell) At(row, col int) *GridCell {
	cell.Row = row
	cell.Column = col
	return cell
}

func (cell *GridCell) Span(rows, cols int) *GridCell {
	cell.RowSpan = rows
	cell.ColSpan = cols
	return cell
}

func (grid *Grid) Draw(ctx *Context) {
	invalid := grid.invalid
	if invalid {
		grid.reflow(ctx)
	}
	for _, cell := range grid.Cells {
		if !cell.invalid && !invalid {
			continue
		}
		rows := grid.rowLayout[cell.Row : cell.Row+cell.RowSpan]
		cols := grid.columnLayout[cell.Column : cell.Column+cell.ColSpan]
		x := cols[0].Offset
		y := rows[0].Offset
		width := 0
		height := 0
		for _, col := range cols {
			width += col.Size
		}
		for _, row := range rows {
			height += row.Size
		}
		subctx := ctx.Subcontext(x, y, width, height)
		cell.Content.Draw(subctx)
	}
}

func (grid *Grid) reflow(ctx *Context) {
	grid.rowLayout = nil
	grid.columnLayout = nil
	flow := func(specs *[]DimSpec, layouts *[]dimLayout, extent int) {
		exact := 0
		weight := 0
		nweights := 0
		for _, dim := range *specs {
			if dim.Strategy == SIZE_EXACT {
				exact += dim.Size
			} else if dim.Strategy == SIZE_WEIGHT {
				nweights += 1
				weight += dim.Size
			}
		}
		offset := 0
		for _, dim := range *specs {
			layout := dimLayout{Offset: offset}
			if dim.Strategy == SIZE_EXACT {
				layout.Size = dim.Size
			} else if dim.Strategy == SIZE_WEIGHT {
				size := float64(dim.Size) / float64(weight)
				size *= float64(extent - exact)
				layout.Size = int(math.Floor(size))
			}
			offset += layout.Size
			*layouts = append(*layouts, layout)
		}
	}
	flow(&grid.Rows, &grid.rowLayout, ctx.Height())
	flow(&grid.Columns, &grid.columnLayout, ctx.Width())
	grid.invalid = false
}

func (grid *Grid) invalidateLayout() {
	grid.invalid = true
	if grid.onInvalidate != nil {
		grid.onInvalidate(grid)
	}
}

func (grid *Grid) Invalidate() {
	grid.invalidateLayout()
	for _, cell := range grid.Cells {
		cell.Content.Invalidate()
	}
}

func (grid *Grid) OnInvalidate(onInvalidate func(d Drawable)) {
	grid.onInvalidate = onInvalidate
}

func (grid *Grid) AddChild(content Drawable) *GridCell {
	cell := &GridCell{
		RowSpan: 1,
		ColSpan: 1,
		Content: content,
		invalid: true,
	}
	grid.Cells = append(grid.Cells, cell)
	cell.Content.OnInvalidate(grid.cellInvalidated)
	cell.invalid = true
	grid.invalidateLayout()
	return cell
}

func (grid *Grid) RemoveChild(cell *GridCell) {
	for i, _cell := range grid.Cells {
		if _cell == cell {
			grid.Cells = append(grid.Cells[:i], grid.Cells[i+1:]...)
			break
		}
	}
	grid.invalidateLayout()
}

func (grid *Grid) cellInvalidated(drawable Drawable) {
	var cell *GridCell
	for _, cell = range grid.Cells {
		if cell.Content == drawable {
			break
		}
		cell = nil
	}
	if cell == nil {
		panic(fmt.Errorf("Attempted to invalidate unknown cell"))
	}
	cell.invalid = true
	if grid.onInvalidate != nil {
		grid.onInvalidate(grid)
	}
}