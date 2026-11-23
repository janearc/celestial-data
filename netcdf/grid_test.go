package netcdf_test

import (
	"bytes"
	"math"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/janearc/celestial-data/netcdf"
)

// The fixture's file describes its grid: name, long name, units, steps
// and coordinates.
func TestFieldsDescribeTheGrid(t *testing.T) {
	found := field(t, gridFile(fixtureLatitudes, fixtureLongitudes))
	if found.Name() != tccName || found.LongName() != tccLong ||
		found.Units() != tccUnits || found.Steps() != stepsOfFile {
		t.Errorf("%q %q %q %d", found.Name(), found.LongName(),
			found.Units(), found.Steps())
	}
	if !slices.Equal(found.Latitudes(), fixtureLatitudes) ||
		!slices.Equal(found.Longitudes(), fixtureLongitudes) {
		t.Errorf("%v %v", found.Latitudes(), found.Longitudes())
	}
}

// A step is unpacked by scale and offset, and a stored fill is no data.
func TestStepUnpacksAndSaysNoData(t *testing.T) {
	found := field(t, gridFile(fixtureLatitudes, fixtureLongitudes))
	step, err := found.Step(dataStep)
	check(t, err)
	if got, have := step.At(0, 0); !have || !near(got, value(1, 0, 0)) {
		t.Errorf("0,0: %v %t", got, have)
	}
	if got, have := step.At(2, 3); !have || !near(got, value(1, 2, 3)) {
		t.Errorf("2,3: %v %t", got, have)
	}
	if _, have := step.At(holeRow, holeColumn); have {
		t.Error("the stored fill has data")
	}
	if step.Rows() != 3 || step.Columns() != 4 || step.Index() != dataStep {
		t.Errorf("%d %d %d", step.Rows(), step.Columns(), step.Index())
	}
}

// All reads every step at once, with its time and whether it has data,
// and each is what Step gives alone.
func TestAllGivesEveryStepWithItsTime(t *testing.T) {
	found := field(t, gridFile(fixtureLatitudes, fixtureLongitudes))
	every, err := found.All()
	check(t, err)
	holds := []bool{false, true, false, true}
	origin := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	for at, step := range every {
		alone, err := found.Step(at)
		check(t, err)
		want := origin.Add(time.Duration(at*stepHours) * time.Hour)
		if step.Holds() != holds[at] || !step.Time().Equal(want) ||
			!slices.Equal(step.Latitudes(), alone.Latitudes()) {
			t.Errorf("step %d: %t %v", at, step.Holds(), step.Time())
		}
		first, have := step.At(0, 0)
		other, haveAlone := alone.At(0, 0)
		if have != haveAlone || (have && first != other) {
			t.Errorf("step %d: %v %v", at, first, other)
		}
	}
}

// Latitude stored from the north is turned to ascend, and a place gives
// the value of the same place.
func TestDescendingLatitudeIsTurned(t *testing.T) {
	descending := []float64{60, 0, -60}
	step, err := field(t, gridFile(descending, fixtureLongitudes)).Step(1)
	check(t, err)
	if !slices.Equal(step.Latitudes(), fixtureLatitudes) {
		t.Errorf("latitudes %v", step.Latitudes())
	}
	if got, _ := step.At(0, 0); !near(got, value(1, 2, 0)) {
		t.Errorf("first row %v, want the last stored", got)
	}
	at(t, step, 0, 60, value(1, 0, 0))
	at(t, step, 90, -60, value(1, 2, 1))
}

// Longitude wraps: the cell across the seam is between the last column
// and the first, on a grid from 0 and on one from -180.
func TestLongitudeWrapsAtTheSeam(t *testing.T) {
	east, err := field(t, gridFile(fixtureLatitudes,
		fixtureLongitudes)).Step(1)
	check(t, err)
	mid := (value(1, 0, 3) + value(1, 0, 0)) / 2
	at(t, east, 315, -60, mid)
	at(t, east, -45, -60, mid)
	west, err := field(t, gridFile(fixtureLatitudes,
		[]float64{-180, -90, 0, 90})).Step(1)
	check(t, err)
	at(t, west, 180, -60, value(1, 0, 0))
	at(t, west, 135, -60, (value(1, 0, 3)+value(1, 0, 0))/2)
}

// The poles clamp: beyond the last row is the last row.
func TestThePolesClamp(t *testing.T) {
	step, err := field(t, gridFile(fixtureLatitudes,
		fixtureLongitudes)).Step(1)
	check(t, err)
	at(t, step, 0, 90, value(1, 2, 0))
	at(t, step, 45, -90, (value(1, 0, 0)+value(1, 0, 1))/2)
}

// A corner the place has a share of with no data gives no data; one on
// a grid line, with no share, is not asked.
func TestNoDataInACornerGivesNoData(t *testing.T) {
	step, err := field(t, gridFile(fixtureLatitudes,
		fixtureLongitudes)).Step(1)
	check(t, err)
	none(t, step, 180, 0)
	none(t, step, 135, 0)
	none(t, step, 100, 0)
	none(t, step, 180, 30)
	at(t, step, 90, 0, value(1, 1, 1))
	at(t, step, 270, 0, value(1, 1, 3))
	empty, err := field(t, gridFile(fixtureLatitudes,
		fixtureLongitudes)).Step(0)
	check(t, err)
	none(t, empty, 90, 0)
}

// The time units of days, seconds and minutes are read, as hours are,
// with a leading space, a T and a fraction of a second.
func TestTimeUnitsAreRead(t *testing.T) {
	const halfSecond = 500e6 // defined: nanoseconds in half a second
	cases := []struct {
		units string
		count float64
		want  time.Time
	}{
		{"days since 2000-01-01", 1.5,
			time.Date(2000, 1, 2, 12, 0, 0, 0, time.UTC)},
		{"seconds since 1970-01-01T00:00:00", 1.5,
			time.Date(1970, 1, 1, 0, 0, 1, halfSecond, time.UTC)},
		{" minutes since 1999-12-31 23:00:0.0", 90,
			time.Date(2000, 1, 1, 0, 30, 0, 0, time.UTC)},
	}
	for _, each := range cases {
		grid := gridWith(fixtureLatitudes, fixtureLongitudes, each.units,
			[]float64{each.count})
		times, err := field(t, grid).Times()
		check(t, err)
		if len(times) != 1 || !times[0].Equal(each.want) {
			t.Errorf("%q: %v, want %v", each.units, times, each.want)
		}
	}
}

// A grid with no time has one step, no time, and the file's order.
func TestAGridWithNoTimeHasOneStep(t *testing.T) {
	grid := gridWith(fixtureLatitudes, fixtureLongitudes, "", nil)
	grid.Variables[0].Steps = stepsOf(dataStep+1, 3, 4)[dataStep:]
	found := field(t, grid)
	step, err := found.Step(0)
	check(t, err)
	if found.Steps() != 1 || !step.Time().IsZero() {
		t.Errorf("%d steps, time %v", found.Steps(), step.Time())
	}
	at(t, step, 0, -60, value(1, 0, 0))
}

// A record file, its time dimension unlimited and its steps written one
// at a time, reads as any other.
func TestARecordFileExtendsAndReads(t *testing.T) {
	grid := gridFile(fixtureLatitudes, fixtureLongitudes)
	grid.Record = true
	path := written(t, grid)
	file, err := netcdf.Open(path)
	check(t, err)
	defer file.Close()
	found, err := file.Field(tccName)
	check(t, err)
	step, err := found.Step(3)
	check(t, err)
	if file.NumRecs() != stepsOfFile || found.Steps() != stepsOfFile {
		t.Errorf("%d records, %d steps", file.NumRecs(), found.Steps())
	}
	at(t, step, 0, -60, value(3, 0, 0))
}

// The same file read from its bytes is the same grid, and a value is
// the same by path and by bytes, in each classic format.
func TestReadingBytesIsReadingThePath(t *testing.T) {
	for _, format := range []netcdf.Format{netcdf.FormatClassic,
		netcdf.FormatClassic64, netcdf.FormatClassic64Data} {
		grid := gridFile(fixtureLatitudes, fixtureLongitudes)
		grid.Format = format
		raw, err := os.ReadFile(written(t, grid))
		check(t, err)
		file, err := netcdf.Read(netcdf.Source{Name: "memory",
			At: bytes.NewReader(raw), Size: int64(len(raw))})
		check(t, err)
		defer file.Close()
		step, err := file.Fields()[0].Step(1)
		check(t, err)
		if file.Format() != format || file.Name() != "memory" {
			t.Errorf("%s %s", netcdf.FormatName(file.Format()), file.Name())
		}
		at(t, step, 90, 0, value(1, 1, 1))
	}
}

// A longitude that is not a number has no value, nor does a latitude.
func TestAPlaceThatIsNotANumberHasNoValue(t *testing.T) {
	step, err := field(t, gridFile(fixtureLatitudes,
		fixtureLongitudes)).Step(1)
	check(t, err)
	none(t, step, math.NaN(), 0)
	none(t, step, 0, math.Inf(1))
}
