package netcdf

import (
	"math"
	"slices"
	"time"
)

// Slice is one time step of a field, unpacked to real values. Its
// latitudes and longitudes ascend. No data is kept inside, never handed
// out: At and ValueAt say whether there is a value.
type Slice struct {
	index      int
	when       time.Time
	lats, lons []float64
	values     []float64
	holds      bool
}

// Step is one time step of the field, read from the file by itself. It
// refuses a step that does not exist, naming how many there are, and
// reads the time units as Times does.
func (field *Field) Step(index int) (*Slice, error) {
	if index < 0 || index >= field.steps {
		return nil, refusedIn(field.variable.file.name,
			variableWhat(field.Name()),
			"has %d time steps, so time step %d does not exist",
			field.steps, index)
	}
	times, err := field.Times()
	if err != nil {
		return nil, err
	}
	stored, err := field.variable.numbers(field.selection(index))
	if err != nil {
		return nil, err
	}
	return field.slice(index, times[index], stored), nil
}

// All is every time step, in one read of the whole variable. It is how a
// field should be read when more than one step is wanted.
func (field *Field) All() ([]*Slice, error) {
	times, err := field.Times()
	if err != nil {
		return nil, err
	}
	stored, err := field.variable.numbers(nil)
	if err != nil {
		return nil, err
	}
	size := len(field.lats) * len(field.lons)
	found := make([]*Slice, field.steps)
	for step := range found {
		found[step] = field.slice(step, times[step],
			stored[step*size:(step+1)*size])
	}
	return found, nil
}

// selection is the hyperslab of one step.
func (field *Field) selection(index int) *Slab {
	rows, columns := int64(len(field.lats)), int64(len(field.lons))
	if field.timeAxis == nil {
		return &Slab{Start: []int64{0, 0},
			Count: []int64{rows, columns}}
	}
	return &Slab{Start: []int64{int64(index), 0, 0},
		Count: []int64{1, rows, columns}}
}

// slice makes a step from its stored values: unpacked, and with the
// rows turned if the file stores latitude downward.
func (field *Field) slice(index int, when time.Time, stored []float64) *Slice {
	values := make([]float64, len(stored))
	holds := false
	for at, each := range stored {
		values[at] = field.unpacked(each)
		holds = holds || !math.IsNaN(values[at])
	}
	if field.flipped {
		values = turnedRows(values, len(field.lons))
	}
	return &Slice{index: index, when: when, lats: field.lats,
		lons: field.lons, values: values, holds: holds}
}

// unpacked is a stored value as a real one. A value that is not a number,
// or is a fill or missing value, is NaN, kept inside.
func (field *Field) unpacked(stored float64) float64 {
	if math.IsNaN(stored) || slices.Contains(field.absent, stored) {
		return math.NaN()
	}
	return stored*field.scale + field.offset
}

// turnedRows is rows of a width turned end for end.
func turnedRows(values []float64, width int) []float64 {
	turned := make([]float64, 0, len(values))
	for row := len(values)/width - 1; row >= 0; row-- {
		turned = append(turned, values[row*width:(row+1)*width]...)
	}
	return turned
}

// Index is the step's place in the field.
func (slice *Slice) Index() int { return slice.index }

// Time is the step's time in UTC, the zero time for a field with none.
func (slice *Slice) Time() time.Time { return slice.when }

// Holds is whether the step has any data at all.
func (slice *Slice) Holds() bool { return slice.holds }

// Rows is how many latitudes the grid has.
func (slice *Slice) Rows() int { return len(slice.lats) }

// Columns is how many longitudes the grid has.
func (slice *Slice) Columns() int { return len(slice.lons) }

// Latitudes are the grid's latitudes in degrees, ascending, a copy.
func (slice *Slice) Latitudes() []float64 { return slices.Clone(slice.lats) }

// Longitudes are the grid's longitudes in degrees, ascending, a copy.
func (slice *Slice) Longitudes() []float64 { return slices.Clone(slice.lons) }

// At is the value at a row and a column of the grid, rows counted from
// the first latitude, and whether there is one. A place outside the grid
// has none.
func (slice *Slice) At(row, column int) (float64, bool) {
	if row < 0 || row >= len(slice.lats) || column < 0 ||
		column >= len(slice.lons) {
		return 0, false
	}
	each := slice.values[row*len(slice.lons)+column]
	return each, !math.IsNaN(each)
}
