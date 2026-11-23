package netcdf

import (
	"math"
	"slices"
	"strings"
	"time"
)

// The attributes of a gridded variable, as the CF conventions spell them.
const (
	unitsName    = "units"
	longName     = "long_name"
	scaleName    = "scale_factor"
	offsetName   = "add_offset"
	fillName     = "_FillValue"
	missingName  = "missing_value"
	calendarName = "calendar"
)

// gridRank is the rank of a variable with no time: (latitude,
// longitude). With time it is one more. It's defined.
const gridRank = 2

// axis says how a coordinate variable shows that it is a latitude or a
// longitude: the CF units, or else a common name.
type axis struct {
	word  string
	units []string
	names []string
}

// latitudeAxis and longitudeAxis are the two axes of a grid.
var (
	latitudeAxis = axis{"latitude",
		[]string{"degrees_north", "degree_north", "degrees_n",
			"degree_n"},
		[]string{"latitude", "lat"}}
	longitudeAxis = axis{"longitude",
		[]string{"degrees_east", "degree_east", "degrees_e",
			"degree_e"},
		[]string{"longitude", "lon"}}
)

// Field is a gridded quantity in a file: one variable over (time,
// latitude, longitude), or over (latitude, longitude) alone, which has
// one step and no time. Its latitudes ascend here whichever way the file
// stores them.
type Field struct {
	variable   *Variable
	timeAxis   *Variable
	steps      int
	lats, lons []float64
	flipped    bool
	scale      float64
	offset     float64
	absent     []float64
}

// Fields are the file's variables that are grids, in the order written.
func (file *File) Fields() []*Field {
	var found []*Field
	for _, each := range file.Variables() {
		if field, err := newField(each); err == nil {
			found = append(found, field)
		}
	}
	return found
}

// Field is the grid of a variable's name. Each refusal is a sentence
// that names the file and the variable.
//
// It refuses a name the file does not have, a variable that is not a
// grid, a grid with no coordinates or with no latitudes or longitudes,
// longitudes that do not ascend and latitudes that do neither.
func (file *File) Field(name string) (*Field, error) {
	variable, err := file.Variable(name)
	if err != nil {
		return nil, err
	}
	return newField(variable)
}

// newField checks a variable and makes its field.
func newField(variable *Variable) (*Field, error) {
	dimensions := variable.Dimensions()
	rank := len(dimensions)
	if rank != gridRank && rank != gridRank+1 {
		return nil, refusedIn(variable.file.name,
			variableWhat(variable.Name()),
			"has %d dimensions, so it is not a grid; a grid is "+
				"(time, latitude, longitude) or "+
				"(latitude, longitude)", rank)
	}
	field := &Field{variable: variable, steps: 1}
	if err := field.coordinates(dimensions); err != nil {
		return nil, err
	}
	if err := field.holdsCells(); err != nil {
		return nil, err
	}
	if err := field.arrange(); err != nil {
		return nil, err
	}
	field.unpacking()
	return field, nil
}

// holdsCells refuses a field with no latitudes or no longitudes, which
// has no cell for a place to lie in.
func (field *Field) holdsCells() error {
	for _, each := range []struct {
		word   string
		values []float64
	}{{latitudeAxis.word + "s", field.lats},
		{longitudeAxis.word + "s", field.lons}} {
		if len(each.values) == noValues {
			return refusedIn(field.variable.file.name,
				variableWhat(field.variable.Name()),
				"has no %s, so it has no cell to read",
				each.word)
		}
	}
	return nil
}

// coordinates reads the time, latitude and longitude of the field from
// the variable's dimensions, the last two being latitude and longitude.
func (field *Field) coordinates(dimensions []*Dimension) error {
	var err error
	rank := len(dimensions)
	if rank > gridRank {
		field.steps = int(dimensions[0].Length())
		field.timeAxis, err = coordinateOf(field.variable,
			dimensions[0])
	}
	if err != nil {
		return err
	}
	field.lats, err = axisValues(field.variable, dimensions[rank-2],
		latitudeAxis)
	if err != nil {
		return err
	}
	field.lons, err = axisValues(field.variable, dimensions[rank-1],
		longitudeAxis)
	return err
}

// coordinateOf is the coordinate variable that shares a dimension's
// name, one value for each of the dimension's.
func coordinateOf(variable *Variable, dimension *Dimension) (*Variable, error) {
	coordinate, err := variable.file.Variable(dimension.Name())
	if err != nil {
		return nil, refusedIn(variable.file.name,
			variableWhat(variable.Name()),
			"has no coordinate variable %q for its dimension "+
				"of that name", dimension.Name())
	}
	shape := coordinate.Shape()
	if len(shape) != 1 || shape[0] != dimension.Length() {
		return nil, refusedIn(variable.file.name,
			variableWhat(coordinate.Name()),
			"must hold one value for each of the dimension's %d",
			dimension.Length())
	}
	return coordinate, nil
}

// axisValues are the values of the coordinate of a dimension, which must
// be the axis it is asked to be.
func axisValues(variable *Variable, dimension *Dimension,
	kind axis) ([]float64, error) {
	coordinate, err := coordinateOf(variable, dimension)
	if err != nil {
		return nil, err
	}
	units := strings.ToLower(textOf(coordinate, unitsName))
	name := strings.ToLower(coordinate.Name())
	if !slices.Contains(kind.units, units) &&
		!slices.Contains(kind.names, name) {
		return nil, refusedIn(variable.file.name,
			variableWhat(variable.Name()),
			"has the dimension %q, which is not a %s: "+
				"its units are %q",
			dimension.Name(), kind.word, units)
	}
	return coordinate.numbers(nil)
}

// textOf is the text of a variable's attribute, none when it has none.
func textOf(variable *Variable, name string) string {
	attribute, err := variable.Attribute(name)
	if err != nil {
		return ""
	}
	return attribute.Text()
}

// numberOf is the first number of a variable's attribute, if it has one.
func numberOf(variable *Variable, name string) (float64, bool) {
	attribute, err := variable.Attribute(name)
	if err != nil {
		return 0, false
	}
	return attribute.Number()
}

// arrange checks the coordinates: longitudes must ascend and latitudes
// may run either way. Latitudes are turned to ascend, and the field
// remembers that its rows must be.
func (field *Field) arrange() error {
	file := field.variable.file.name
	what := variableWhat(field.variable.Name())
	if !slices.IsSorted(field.lons) || hasRepeat(field.lons) {
		return refusedIn(file, what,
			"has longitudes that do not ascend")
	}
	if slices.IsSorted(field.lats) && !hasRepeat(field.lats) {
		return nil
	}
	turned := slices.Clone(field.lats)
	slices.Reverse(turned)
	if !slices.IsSorted(turned) || hasRepeat(turned) {
		return refusedIn(file, what,
			"has latitudes that neither ascend nor descend")
	}
	field.lats, field.flipped = turned, true
	return nil
}

// hasRepeat is whether a sorted list holds the same value twice, or a
// value that is not a number.
func hasRepeat(list []float64) bool {
	for at, each := range list {
		if math.IsNaN(each) || (at > 0 && each == list[at-1]) {
			return true
		}
	}
	return false
}

// unpacking reads how the variable is packed: value = stored * scale +
// offset, with a scale of one and an offset of none when it says
// nothing, and the stored values that are no data.
func (field *Field) unpacking() {
	field.scale, field.offset = 1, 0
	if each, found := numberOf(field.variable, scaleName); found {
		field.scale = each
	}
	if each, found := numberOf(field.variable, offsetName); found {
		field.offset = each
	}
	for _, name := range []string{fillName, missingName} {
		if each, found := numberOf(field.variable, name); found {
			field.absent = append(field.absent, each)
		}
	}
}

// Name is the variable's name.
func (field *Field) Name() string { return field.variable.Name() }

// LongName is the variable's long_name, or nothing.
func (field *Field) LongName() string {
	return textOf(field.variable, longName)
}

// Units is the variable's units after unpacking, or nothing.
func (field *Field) Units() string { return textOf(field.variable, unitsName) }

// Steps is how many time steps the field has; one with no time.
func (field *Field) Steps() int { return field.steps }

// Latitudes are the grid's latitudes in degrees, ascending, a copy.
func (field *Field) Latitudes() []float64 { return slices.Clone(field.lats) }

// Longitudes are the grid's longitudes in degrees, ascending, a copy.
func (field *Field) Longitudes() []float64 { return slices.Clone(field.lons) }

// Times are the time of each step. A field with no time has one step and
// the zero time. The error says the units text it could not read, and
// names the file and the time variable.
func (field *Field) Times() ([]time.Time, error) {
	if field.timeAxis == nil {
		return []time.Time{{}}, nil
	}
	counting, err := field.clock()
	if err != nil {
		return nil, err
	}
	counts, err := field.timeAxis.numbers(nil)
	if err != nil {
		return nil, err
	}
	times := make([]time.Time, len(counts))
	for at, each := range counts {
		times[at] = counting.at(each)
	}
	return times, nil
}

// clock is the time variable's units, read. A calendar other than the
// standard one is refused: the counts would be wrong, not just odd.
func (field *Field) clock() (*clock, error) {
	file := field.variable.file.name
	what := variableWhat(field.timeAxis.Name())
	text := textOf(field.timeAxis, unitsName)
	if text == "" {
		return nil, refusedIn(file, what,
			"has no units, so its times cannot be read")
	}
	calendar := strings.ToLower(textOf(field.timeAxis, calendarName))
	if !slices.Contains(calendars, calendar) {
		return nil, refusedIn(file, what,
			"keeps the calendar %q, which this adapter does not "+
				"count in", calendar)
	}
	counting, err := parseClock(text)
	if err != nil {
		return nil, refusedIn(file, what, "has time units that do not "+
			"read: %v", err)
	}
	return counting, nil
}
