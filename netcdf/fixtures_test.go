package netcdf_test

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/janearc/celestial-data/netcdf"
)

// The fixture's numbers. A grid of three latitudes, four longitudes and
// four times is stored as shorts, value = stored * fixtureScale +
// fixtureOffset, and a real value is made from its step, row and column
// by value. Even steps hold no data, and one cell of step one is no data.
const (
	fixtureScale  = 0.5    // defined: the packing's scale
	fixtureOffset = 10.0   // defined: the packing's offset
	fixtureFill   = -32767 // defined: netCDF's default short fill
	fixtureUnits  = "hours since 1900-01-01 00:00:0.0"
	stepsOfFile   = 4     // defined: the time steps of the fixture
	tolerance     = 1e-9  // defined: the fixture's arithmetic is exact
	stepHours     = 6     // defined: the hours between steps
	rowStride     = 10    // defined: what a row adds to a stored value
	stepStride    = 100   // defined: what a step adds to a stored value
	holeRow       = 1     // defined: the row of step one's no data
	holeColumn    = 2     // defined: the column of step one's no data
	dataStep      = 1     // defined: the first step that holds data
	tccName       = "tcc" // defined: the fixture's variable
	tccLong       = "Total cloud cover"
	tccUnits      = "(0 - 1)"
)

// The fixture's coordinates, in degrees.
var (
	fixtureLatitudes  = []float64{-60, 0, 60}
	fixtureLongitudes = []float64{0, 90, 180, 270}
)

// check stops a test on an error.
func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// value is the real value of the fixture at a step, a row and a column
// of the file as written.
func value(step, row, column int) float64 {
	stored := step*stepStride + row*rowStride + column
	return float64(stored)*fixtureScale + fixtureOffset
}

// stepsOf are the fixture's steps: no data on the even ones, and on the
// odd ones, one cell with none.
func stepsOf(count, rows, columns int) [][]float64 {
	steps := make([][]float64, count)
	for step := range steps {
		steps[step] = make([]float64, rows*columns)
		for row := range rows {
			for column := range columns {
				each := value(step, row, column)
				hole := step == dataStep && row == holeRow &&
					column == holeColumn
				if step%2 == 0 || hole {
					each = math.NaN()
				}
				steps[step][row*columns+column] = each
			}
		}
	}
	return steps
}

// gridFile is the fixture over the given coordinates.
func gridFile(lats, lons []float64) netcdf.GridFile {
	return gridWith(lats, lons, fixtureUnits, hoursOf(stepsOfFile))
}

// hoursOf are the times of that many steps, in hours.
func hoursOf(count int) []float64 {
	times := make([]float64, count)
	for step := range times {
		times[step] = float64(step * stepHours)
	}
	return times
}

// gridWith is the fixture with its time units and times given.
func gridWith(lats, lons []float64, units string,
	times []float64) netcdf.GridFile {
	return netcdf.GridFile{Latitudes: lats, Longitudes: lons,
		TimeUnits: units, Times: times,
		Variables: []netcdf.GridVariable{{Name: tccName, LongName: tccLong,
			Units: tccUnits, Steps: stepsOf(len(times), len(lats), len(lons)),
			Packing: &netcdf.Packing{Scale: fixtureScale,
				Offset: fixtureOffset, Fill: fixtureFill}}}}
}

// written is the path of a file written from a grid file.
func written(t *testing.T, grid netcdf.GridFile) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "grid.nc")
	check(t, netcdf.WriteGrid(path, grid))
	return path
}

// field is the fixture's variable, opened from a written file.
func field(t *testing.T, grid netcdf.GridFile) *netcdf.Field {
	t.Helper()
	file, err := netcdf.Open(written(t, grid))
	check(t, err)
	t.Cleanup(func() { _ = file.Close() })
	found, err := file.Field(tccName)
	check(t, err)
	return found
}

// near is whether two numbers are the same within the fixture's
// tolerance.
func near(left, right float64) bool { return math.Abs(left-right) < tolerance }

// at checks a value at a place of a step, and that there is one.
func at(t *testing.T, step *netcdf.Slice, lon, lat, want float64) {
	t.Helper()
	got, have := step.ValueAt(lon, lat)
	if !have || !near(got, want) {
		t.Errorf("at %v, %v: %v, %t; want %v", lon, lat, got, have, want)
	}
}

// none checks that there is no value at a place of a step.
func none(t *testing.T, step *netcdf.Slice, lon, lat float64) {
	t.Helper()
	if got, have := step.ValueAt(lon, lat); have {
		t.Errorf("at %v, %v: %v, where there is no data", lon, lat, got)
	}
}
