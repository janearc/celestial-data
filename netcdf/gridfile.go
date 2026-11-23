package netcdf

import (
	"errors"
	"math"
	"slices"
)

// The dimensions and coordinate variables a grid file is written with,
// named as the CF conventions and the ERA-40 file name them.
const (
	timeName      = "time"
	latitudeName  = "latitude"
	longitudeName = "longitude"
	degreesNorth  = "degrees_north"
	degreesEast   = "degrees_east"
)

// The counts the writing works with. They are defined.
const (
	// textPairWidth is an attribute's name and its text.
	textPairWidth = 2
	// noRecords is the length the unlimited dimension is given.
	noRecords = 0
)

// Packing says how real values are stored as 16-bit integers:
// stored = (value - Offset) / Scale, rounded. Fill is the stored value
// for no data, written as _FillValue and missing_value.
type Packing struct {
	Scale, Offset float64
	Fill          int16
}

// GridVariable is one gridded quantity to write. Each of Steps is one
// time step, rows by columns, the rows in the order of the file's
// latitudes; NaN is no data. With a Packing the values are stored as
// shorts, and with none as doubles.
type GridVariable struct {
	Name, LongName, Units string
	Packing               *Packing
	Steps                 [][]float64
}

// GridFile is a file of gridded fields to write. Times are counted in
// TimeUnits ("hours since 1900-01-01 00:00:0.0"); with no Times the
// variables have no time dimension and one step each.
//
// With Record the time dimension is the unlimited one, and the variables
// are written a step at a time, which extends it. A Format of
// FormatUnknown is CDF-1.
type GridFile struct {
	Format                Format
	Latitudes, Longitudes []float64
	TimeUnits             string
	Times                 []float64
	Record                bool
	Variables             []GridVariable
}

// gridDimensions are the dimensions of a grid file's layout. steps is
// nil when the file has no time.
type gridDimensions struct {
	steps, rows, columns *LayoutDimension
}

// planned is what a written variable must turn out to be.
type planned struct {
	name   string
	typ    Type
	record bool
	shape  []int64
}

// WriteGrid writes a grid file at a path, through Layout and Writer. It
// checks the file first, and the written variables after, each against
// what was planned.
//
// It refuses no coordinates, no variables, a step of the wrong size, a
// Record with no Times, and a packed value that does not fit.
func WriteGrid(path string, grid GridFile) error {
	if err := grid.check(path); err != nil {
		return err
	}
	values, err := grid.values(path)
	if err != nil {
		return err
	}
	layout, err := grid.layout()
	if err != nil {
		return failure(path, "has a layout that cannot be made", err)
	}
	writer, err := layout.Create(path)
	if err != nil {
		return err
	}
	err = grid.write(writer, values)
	if closed := writer.Close(); err == nil {
		err = closed
	}
	return err
}

// timed is whether the grid file has a time dimension.
func (grid GridFile) timed() bool { return len(grid.Times) > noRecords }

// stepCount is how many time steps each variable has.
func (grid GridFile) stepCount() int {
	if grid.timed() {
		return len(grid.Times)
	}
	return 1
}

// size is how many values a step holds.
func (grid GridFile) size() int {
	return len(grid.Latitudes) * len(grid.Longitudes)
}

// check refuses what cannot be written.
func (grid GridFile) check(path string) error {
	switch {
	case grid.size() == 0:
		return refusedIn(path, "the grid",
			"needs latitudes and longitudes")
	case grid.Record && !grid.timed():
		return refusedIn(path, "the grid", "is a record with no times")
	case len(grid.Variables) == 0:
		return refusedIn(path, "the grid", "has no variables")
	}
	for _, each := range grid.Variables {
		if err := grid.checkVariable(path, each); err != nil {
			return err
		}
	}
	return nil
}

// checkVariable refuses a variable whose steps are not the file's.
func (grid GridFile) checkVariable(path string, each GridVariable) error {
	what := variableWhat(each.Name)
	if len(each.Steps) != grid.stepCount() {
		return refusedIn(path, what,
			"has %d steps and the file has %d times",
			len(each.Steps), grid.stepCount())
	}
	for _, step := range each.Steps {
		if len(step) != grid.size() {
			return refusedIn(path, what,
				"has a step of %d values, not %d", len(step),
				grid.size())
		}
	}
	return nil
}

// declared adds a variable to a layout with its text attributes, given
// in pairs of a name and its text. An attribute with no text is left
// out.
func declared(layout *Layout, spec VariableSpec,
	text ...string) (*LayoutVariable, error) {
	variable, err := layout.AddVariable(spec)
	for at := 0; err == nil && at+1 < len(text); at += textPairWidth {
		if text[at+1] != "" {
			err = variable.Set(text[at], text[at+1])
		}
	}
	return variable, err
}

// dimensions adds the file's dimensions to a layout. The time dimension
// is the unlimited one for a record.
func (grid GridFile) dimensions(layout *Layout) (gridDimensions, error) {
	var found gridDimensions
	var steps, rows, columns error
	if grid.timed() {
		length := int64(len(grid.Times))
		if grid.Record {
			length = noRecords
		}
		found.steps, steps = layout.AddDimension(timeName, length)
	}
	found.rows, rows = layout.AddDimension(latitudeName,
		int64(len(grid.Latitudes)))
	found.columns, columns = layout.AddDimension(longitudeName,
		int64(len(grid.Longitudes)))
	return found, errors.Join(steps, rows, columns)
}

// over is the dimensions of a gridded variable, in order.
func (found gridDimensions) over() []*LayoutDimension {
	if found.steps == nil {
		return []*LayoutDimension{found.rows, found.columns}
	}
	return []*LayoutDimension{found.steps, found.rows, found.columns}
}

// layout is the design of the file: its dimensions, coordinates and
// variables.
func (grid GridFile) layout() (*Layout, error) {
	format := grid.Format
	if format == FormatUnknown {
		format = FormatClassic
	}
	layout, err := NewLayout(format)
	if err != nil {
		return nil, err
	}
	found, err := grid.dimensions(layout)
	if err != nil {
		return nil, err
	}
	err = grid.coordinates(layout, found)
	for _, each := range grid.Variables {
		if err == nil {
			err = each.declare(layout, found.over())
		}
	}
	return layout, err
}

// coordinates declares the time, latitude and longitude variables.
func (grid GridFile) coordinates(layout *Layout, found gridDimensions) error {
	var steps error
	if grid.timed() {
		_, steps = declared(layout, VariableSpec{timeName, TypeDouble,
			[]*LayoutDimension{found.steps}}, unitsName,
			grid.TimeUnits)
	}
	_, rows := declared(layout, VariableSpec{latitudeName, TypeDouble,
		[]*LayoutDimension{found.rows}}, unitsName, degreesNorth)
	_, columns := declared(layout, VariableSpec{longitudeName, TypeDouble,
		[]*LayoutDimension{found.columns}}, unitsName, degreesEast)
	return errors.Join(steps, rows, columns)
}

// storedType is the type a gridded variable is written as.
func (each GridVariable) storedType() Type {
	if each.Packing != nil {
		return TypeShort
	}
	return TypeDouble
}

// declare adds the variable to a layout, with its long name and units
// and, when packed, its scale, offset and fill.
func (each GridVariable) declare(layout *Layout,
	over []*LayoutDimension) error {
	variable, err := declared(layout, VariableSpec{each.Name,
		each.storedType(), over}, longName, each.LongName, unitsName,
		each.Units)
	if err != nil || each.Packing == nil {
		return err
	}
	fill := []int16{each.Packing.Fill}
	return errors.Join(
		variable.SetTyped(scaleName, TypeDouble,
			[]float64{each.Packing.Scale}),
		variable.SetTyped(offsetName, TypeDouble,
			[]float64{each.Packing.Offset}),
		variable.SetTyped(fillName, TypeShort, fill),
		variable.SetTyped(missingName, TypeShort, fill))
}

// stored is all the variable's steps, one after another, as the Go type
// the file stores it as: int16 when packed, float64 when not.
func (each GridVariable) stored(path string) (any, error) {
	var flat []float64
	for _, step := range each.Steps {
		flat = append(flat, step...)
	}
	if each.Packing == nil {
		return flat, nil
	}
	packed := make([]int16, len(flat))
	for at, value := range flat {
		one, err := each.pack(path, value)
		if err != nil {
			return nil, err
		}
		packed[at] = one
	}
	return packed, nil
}

// pack is a real value as stored. NaN is the fill. A value that does not
// fit in a short, or that would be stored as the fill, is refused.
func (each GridVariable) pack(path string, value float64) (int16, error) {
	if math.IsNaN(value) {
		return each.Packing.Fill, nil
	}
	rounded := math.Round((value - each.Packing.Offset) /
		each.Packing.Scale)
	if rounded < math.MinInt16 || rounded > math.MaxInt16 ||
		int16(rounded) == each.Packing.Fill {
		return 0, refusedIn(path, variableWhat(each.Name),
			"has the value %v, which does not pack into a short",
			value)
	}
	return int16(rounded), nil
}

// values are the values of every variable as they are written, the
// coordinates first. A value that does not pack is refused here, before
// a file is made.
func (grid GridFile) values(path string) ([]written, error) {
	found := grid.coordinateValues()
	for _, each := range grid.Variables {
		flat, err := each.stored(path)
		if err != nil {
			return nil, err
		}
		found = append(found, written{each.Name, flat})
	}
	return found, nil
}

// write writes every variable, then checks what was written.
func (grid GridFile) write(writer *Writer, values []written) error {
	for _, each := range values {
		if err := grid.put(writer, each); err != nil {
			return err
		}
	}
	return grid.confirm(writer)
}

// written is a variable's name and all of its values.
type written struct {
	name   string
	values any
}

// coordinateValues are the values of the time, latitude and longitude.
func (grid GridFile) coordinateValues() []written {
	found := []written{{latitudeName, grid.Latitudes},
		{longitudeName, grid.Longitudes}}
	if grid.timed() {
		found = append([]written{{timeName, grid.Times}}, found...)
	}
	return found
}

// put writes a variable's values. A gridded variable of a record file is
// written a step at a time, which extends the record dimension; the rest
// are written whole.
func (grid GridFile) put(writer *Writer, each written) error {
	variable, err := writer.Variable(each.name)
	if err != nil {
		return err
	}
	if !grid.Record || !variable.Record() || each.name == timeName {
		return variable.Write(each.values)
	}
	rows, columns := int64(len(grid.Latitudes)), int64(len(grid.Longitudes))
	for step := range grid.stepCount() {
		from, to := step*grid.size(), (step+1)*grid.size()
		slab := Slab{Start: []int64{int64(step), 0, 0},
			Count: []int64{1, rows, columns}}
		err := variable.WriteSlab(part(each.values, from, to), slab)
		if err != nil {
			return err
		}
	}
	return nil
}

// part is a range of a slice of the types that are stored here.
func part(values any, from, to int) any {
	if packed, ok := values.([]int16); ok {
		return packed[from:to]
	}
	return values.([]float64)[from:to]
}

// plan is what each variable of the file must turn out to be.
func (grid GridFile) plan() []planned {
	rows, columns := int64(len(grid.Latitudes)), int64(len(grid.Longitudes))
	steps := int64(grid.stepCount())
	found := []planned{{latitudeName, TypeDouble, false, []int64{rows}},
		{longitudeName, TypeDouble, false, []int64{columns}}}
	shape := []int64{rows, columns}
	if grid.timed() {
		found = append([]planned{{timeName, TypeDouble, grid.Record,
			[]int64{steps}}}, found...)
		shape = []int64{steps, rows, columns}
	}
	for _, each := range grid.Variables {
		found = append(found, planned{each.Name, each.storedType(),
			grid.Record, shape})
	}
	return found
}

// confirm checks the writer against the plan: the format, the variables
// it holds, and each one's type, record and shape.
func (grid GridFile) confirm(writer *Writer) error {
	plan := grid.plan()
	want := grid.Format
	if want == FormatUnknown {
		want = FormatClassic
	}
	if writer.Format() != want || len(writer.Variables()) != len(plan) {
		return refusedIn(writer.path, "the written file",
			"is %s with %d variables, not %s with %d",
			FormatName(writer.Format()), len(writer.Variables()),
			FormatName(want), len(plan))
	}
	for _, each := range plan {
		variable, err := writer.Variable(each.name)
		if err != nil {
			return err
		}
		if variable.Type() != each.typ ||
			variable.Record() != each.record ||
			!slices.Equal(variable.Shape(), each.shape) {
			return refusedIn(writer.path, variableWhat(each.name),
				"was written as a %s of shape %v, record %t, "+
					"not a %s of shape %v, record %t",
				TypeName(variable.Type()), variable.Shape(),
				variable.Record(), TypeName(each.typ),
				each.shape, each.record)
		}
	}
	return nil
}
