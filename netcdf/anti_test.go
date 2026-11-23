package netcdf_test

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/netcdf"
)

// refused checks an error is this package's refusal and says each of the
// words, so that it names what it is about.
func refused(t *testing.T, err error, words ...string) {
	t.Helper()
	if err == nil || !errors.Is(err, celestialdata.Refused()) {
		t.Fatalf("not a refusal: %v", err)
	}
	for _, word := range words {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("%q does not say %q", err, word)
		}
	}
}

// rawFile writes bytes to a file of a test and gives its path.
func rawFile(t *testing.T, raw []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "raw.nc")
	check(t, os.WriteFile(path, raw, 0o600))
	return path
}

// hdf5Signature is the eight bytes every HDF5 file, so every NetCDF-4
// file, begins with.
const hdf5Signature = "\x89HDF\r\n\x1a\n"

// Anti-test: a variable the file does not have is refused, naming the
// file and the variable and what the file has.
func TestAMissingVariableIsRefused(t *testing.T) {
	path := written(t, gridFile(fixtureLatitudes, fixtureLongitudes))
	file, err := netcdf.Open(path)
	check(t, err)
	defer file.Close()
	_, err = file.Field("nope")
	refused(t, err, path, `"nope"`, tccName)
	_, err = file.Variable("nope")
	refused(t, err, path)
	_, err = file.Dimension("nope")
	refused(t, err, path, "time")
	_, err = file.Attribute("nope")
	refused(t, err, path)
}

// Anti-test: a variable that is not a grid is refused as a field, and is
// not listed among the fields.
func TestAVariableThatIsNotAGridIsRefused(t *testing.T) {
	layout, err := netcdf.NewLayout(netcdf.FormatClassic)
	check(t, err)
	length, err := layout.AddDimension("x", 2)
	check(t, err)
	_, err = layout.AddVariable(netcdf.VariableSpec{Name: "profile",
		Type: netcdf.TypeShort, Dimensions: []*netcdf.LayoutDimension{length}})
	check(t, err)
	path := filepath.Join(t.TempDir(), "profile.nc")
	writer, err := layout.Create(path)
	check(t, err)
	check(t, writer.Close())
	file, err := netcdf.Open(path)
	check(t, err)
	defer file.Close()
	_, err = file.Field("profile")
	refused(t, err, path, `"profile"`, "not a grid")
	if len(file.Fields()) != 0 {
		t.Errorf("%d fields", len(file.Fields()))
	}
}

// Anti-test: a grid whose dimension has no coordinate is refused.
func TestAGridWithNoCoordinatesIsRefused(t *testing.T) {
	layout, err := netcdf.NewLayout(netcdf.FormatClassic)
	check(t, err)
	rows, err := layout.AddDimension("y", 2)
	check(t, err)
	columns, err := layout.AddDimension("x", 2)
	check(t, err)
	_, err = layout.AddVariable(netcdf.VariableSpec{Name: "bare",
		Type: netcdf.TypeShort, Dimensions: []*netcdf.LayoutDimension{rows,
			columns}})
	check(t, err)
	path := filepath.Join(t.TempDir(), "bare.nc")
	writer, err := layout.Create(path)
	check(t, err)
	check(t, writer.Close())
	file, err := netcdf.Open(path)
	check(t, err)
	defer file.Close()
	_, err = file.Field("bare")
	refused(t, err, path, `"bare"`, "no coordinate variable")
}

// Anti-test: a file with the HDF5 signature is NetCDF-4, and is refused
// in a sentence that says so, with the reader's error under it.
func TestANetCDF4FileIsRefusedAsSuch(t *testing.T) {
	raw := append([]byte(hdf5Signature), make([]byte, 64)...)
	path := rawFile(t, raw)
	_, err := netcdf.Open(path)
	if !errors.Is(err, netcdf.ErrUnsupportedFormat) ||
		!strings.Contains(err.Error(), "NetCDF-4") ||
		!strings.Contains(err.Error(), "classic") ||
		!strings.Contains(err.Error(), path) {
		t.Errorf("%v", err)
	}
	_, err = netcdf.Read(netcdf.Source{Name: "memory",
		At: bytes.NewReader(raw), Size: int64(len(raw))})
	if !errors.Is(err, netcdf.ErrUnsupportedFormat) ||
		!strings.Contains(err.Error(), "memory") {
		t.Errorf("%v", err)
	}
}

// Anti-test: what is not netCDF is refused, a file of words, an empty
// one, and no file at all.
func TestWhatIsNotNetCDFIsRefused(t *testing.T) {
	for name, raw := range map[string][]byte{
		"words": []byte("this is not a netCDF file at all"),
		"empty": {}} {
		path := rawFile(t, raw)
		_, err := netcdf.Open(path)
		if !errors.Is(err, netcdf.ErrNotNetCDF) ||
			!strings.Contains(err.Error(), path) {
			t.Errorf("%s: %v", name, err)
		}
	}
	_, err := netcdf.Open("")
	refused(t, err, "no name")
	_, err = netcdf.Open(filepath.Join(t.TempDir(), "absent.nc"))
	if err == nil || !strings.Contains(err.Error(), "absent.nc") {
		t.Errorf("%v", err)
	}
	_, err = netcdf.Read(netcdf.Source{Name: "nothing"})
	refused(t, err, "nothing", "no bytes")
	_, err = netcdf.Read(netcdf.Source{At: bytes.NewReader(nil), Size: -1})
	refused(t, err, "bytes", "-1")
}

// Anti-test: a step that does not exist is refused, below and above.
func TestAStepOutOfRangeIsRefused(t *testing.T) {
	path := written(t, gridFile(fixtureLatitudes, fixtureLongitudes))
	file, err := netcdf.Open(path)
	check(t, err)
	defer file.Close()
	found, err := file.Field(tccName)
	check(t, err)
	for _, step := range []int{-1, stepsOfFile} {
		_, err := found.Step(step)
		refused(t, err, path, `"tcc"`, "4 time steps")
	}
}

// Anti-test: time units that cannot be read are refused in a sentence
// that names the text, the file and the time variable; no units at all
// is refused too.
func TestTimeUnitsThatCannotBeReadAreRefused(t *testing.T) {
	for _, units := range []string{"fortnights since 2000-01-01",
		"hours since yesterday", "hours", "since 2000-01-01"} {
		grid := gridWith(fixtureLatitudes, fixtureLongitudes, units,
			hoursOf(1))
		path := written(t, grid)
		file, err := netcdf.Open(path)
		check(t, err)
		found, err := file.Field(tccName)
		check(t, err)
		_, err = found.Times()
		refused(t, err, path, `"time"`, units)
		_, err = found.All()
		refused(t, err, units)
		check(t, file.Close())
	}
	grid := gridWith(fixtureLatitudes, fixtureLongitudes, "", hoursOf(1))
	_, err := field(t, grid).Times()
	refused(t, err, "no units")
}

// Anti-test: longitudes that do not ascend are refused.
func TestLongitudesThatDoNotAscendAreRefused(t *testing.T) {
	grid := gridFile(fixtureLatitudes, []float64{90, 0, 180, 270})
	file, err := netcdf.Open(written(t, grid))
	check(t, err)
	defer file.Close()
	_, err = file.Field(tccName)
	refused(t, err, `"tcc"`, "longitudes")
}

// Anti-test: a grid file that cannot be written is refused before a
// file is made: no coordinates, a record with no times, a step of the
// wrong size, a value that does not pack.
func TestAGridFileThatCannotBeWrittenIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.nc")
	grid := gridFile(nil, fixtureLongitudes)
	refused(t, netcdf.WriteGrid(path, grid), "latitudes")
	grid = gridWith(fixtureLatitudes, fixtureLongitudes, "", nil)
	grid.Record = true
	refused(t, netcdf.WriteGrid(path, grid), "no times")
	grid = gridFile(fixtureLatitudes, fixtureLongitudes)
	grid.Variables[0].Steps[0] = []float64{1}
	refused(t, netcdf.WriteGrid(path, grid), `"tcc"`, "1 values")
	grid = gridFile(fixtureLatitudes, fixtureLongitudes)
	grid.Variables[0].Steps[1][0] = math.MaxFloat32
	refused(t, netcdf.WriteGrid(path, grid), `"tcc"`, "pack")
	grid.Variables = nil
	refused(t, netcdf.WriteGrid(path, grid), "no variables")
	if _, err := os.Stat(path); err == nil {
		t.Error("a refused grid file was written")
	}
}

// Anti-test: a layout that cannot be made or written is refused, and
// the reader's refusal of a bad schema stays reachable.
func TestALayoutThatCannotBeMadeIsRefused(t *testing.T) {
	_, err := netcdf.NewLayout(netcdf.FormatHDF5)
	refused(t, err, "NetCDF-4")
	layout, err := netcdf.NewLayout(netcdf.FormatClassic)
	check(t, err)
	_, err = layout.AddDimension("", 1)
	refused(t, err, "dimension")
	other, err := netcdf.NewLayout(netcdf.FormatClassic)
	check(t, err)
	foreign, err := other.AddDimension("x", 1)
	check(t, err)
	_, err = layout.AddVariable(netcdf.VariableSpec{Name: "v",
		Type: netcdf.TypeShort, Dimensions: []*netcdf.LayoutDimension{foreign}})
	refused(t, err, `"v"`, "did not make")
	_, err = layout.AddVariable(netcdf.VariableSpec{})
	refused(t, err, "needs a name")
	_, err = layout.Create("")
	refused(t, err, "no name")
	first, _ := layout.AddDimension("a", 0)
	second, _ := layout.AddDimension("b", 0)
	_, err = layout.AddVariable(netcdf.VariableSpec{Name: "v",
		Type: netcdf.TypeShort, Dimensions: []*netcdf.LayoutDimension{first,
			second}})
	check(t, err)
	_, err = layout.Create(filepath.Join(t.TempDir(), "two.nc"))
	if !errors.Is(err, netcdf.ErrInvalidSchema) {
		t.Errorf("two unlimited dimensions gave %v", err)
	}
}

// Anti-test: a selection outside a variable, or one that asks for more
// than the file can hold, is refused; the reader's ErrOutOfRange is
// reachable for the first and this package's refusal stands for the rest.
func TestASelectionOutsideTheVariableIsRefused(t *testing.T) {
	file, err := netcdf.Open(typesFile(t))
	check(t, err)
	defer file.Close()
	short, err := file.Variable("short")
	check(t, err)
	_, err = short.Slab(netcdf.Slab{Start: []int64{2}, Count: []int64{2}})
	if !errors.Is(err, netcdf.ErrOutOfRange) ||
		!strings.Contains(err.Error(), `"short"`) {
		t.Errorf("%v", err)
	}
	_, err = short.Slab(netcdf.Slab{Start: []int64{0}, Count: []int64{-1}})
	refused(t, err, `"short"`)
	_, err = short.Slab(netcdf.Slab{Start: []int64{0},
		Count: []int64{1 << 40}})
	refused(t, err, `"short"`, "cannot hold")
	_, err = short.Slab(netcdf.Slab{})
	if !errors.Is(err, netcdf.ErrOutOfRange) {
		t.Errorf("a slab of no dimensions gave %v", err)
	}
	text, err := file.Variable("char")
	check(t, err)
	_, err = text.Numbers()
	refused(t, err, `"char"`, "text")
}

// Anti-test: a write of the wrong type, and a use after Close, keep the
// reader's errors reachable; a writer has no variable it was not given.
func TestAWriteOfTheWrongTypeOrAfterCloseIsRefused(t *testing.T) {
	layout, err := netcdf.NewLayout(netcdf.FormatClassic)
	check(t, err)
	length, err := layout.AddDimension("n", 2)
	check(t, err)
	_, err = layout.AddVariable(netcdf.VariableSpec{Name: "v",
		Type: netcdf.TypeShort, Dimensions: []*netcdf.LayoutDimension{length}})
	check(t, err)
	path := filepath.Join(t.TempDir(), "typed.nc")
	writer, err := layout.Create(path)
	check(t, err)
	variable, err := writer.Variable("v")
	check(t, err)
	err = variable.Write([]float64{1, 2})
	if !errors.Is(err, netcdf.ErrTypeMismatch) ||
		!strings.Contains(err.Error(), `"v"`) {
		t.Errorf("%v", err)
	}
	_, err = writer.Variable("nope")
	refused(t, err, path, "v")
	check(t, writer.Close())
	err = variable.Write([]int16{1, 2})
	if !errors.Is(err, netcdf.ErrClosed) {
		t.Errorf("a write after Close gave %v", err)
	}
	file, err := netcdf.Open(path)
	check(t, err)
	read, err := file.Variable("v")
	check(t, err)
	check(t, file.Close())
	if _, err := read.Values(); !errors.Is(err, netcdf.ErrClosed) {
		t.Errorf("a read after Close gave %v", err)
	}
}
