package netcdf_test

// What the adapter says where the reader under it says less: which
// variables are record variables, how many records a file holds, and
// what a write of the wrong Go type is told.

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/janearc/celestial-data/netcdf"
)

// mixedFile creates a file of two variables of shorts, "fixed" over x and
// then "grow" over t and x, and gives its writer, with nothing written.
func mixedFile(t *testing.T, path string) *netcdf.Writer {
	t.Helper()
	layout, err := netcdf.NewLayout(netcdf.FormatClassic64Data)
	check(t, err)
	steps, err := layout.AddDimension("t", 0)
	check(t, err)
	spots, err := layout.AddDimension("x", 2)
	check(t, err)
	_, err = layout.AddVariable(netcdf.VariableSpec{Name: "fixed",
		Type: netcdf.TypeShort, Dimensions: []*netcdf.LayoutDimension{spots}})
	check(t, err)
	_, err = layout.AddVariable(netcdf.VariableSpec{Name: "grow",
		Type: netcdf.TypeShort, Dimensions: []*netcdf.LayoutDimension{steps,
			spots}})
	check(t, err)
	writer, err := layout.Create(path)
	check(t, err)
	return writer
}

// A file whose unlimited dimension no variable runs over holds no records,
// and its fixed variable is not a record variable.
func TestAFileWithoutRecordVariablesHoldsNoRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixed.nc")
	writer := mixedFile(t, path)
	fixed, err := writer.Variable("fixed")
	check(t, err)
	check(t, fixed.Write([]int16{1, 2}))
	if fixed.Record() || writer.NumRecs() != 0 {
		t.Errorf("fixed is a record variable: %t, %d records", fixed.Record(),
			writer.NumRecs())
	}
	check(t, writer.Close())
	file, err := netcdf.Open(path)
	check(t, err)
	defer file.Close()
	variable, err := file.Variable("fixed")
	check(t, err)
	if variable.Record() || file.NumRecs() != 0 {
		t.Errorf("read back, %t and %d records", variable.Record(),
			file.NumRecs())
	}
}

// Anti: the record variable that is added after a fixed one is told from
// it, and the records are counted from it, not from the first variable.
func TestTheRecordVariableAfterAFixedOneIsToldFromIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.nc")
	writer := mixedFile(t, path)
	grow, err := writer.Variable("grow")
	check(t, err)
	check(t, grow.Write([]int16{1, 2, 3, 4, 5, 6}))
	if !grow.Record() || writer.NumRecs() != 3 {
		t.Errorf("grow is a record variable: %t, %d records", grow.Record(),
			writer.NumRecs())
	}
	check(t, writer.Close())
	file, err := netcdf.Open(path)
	check(t, err)
	defer file.Close()
	fixed, err := file.Variable("fixed")
	check(t, err)
	read, err := file.Variable("grow")
	check(t, err)
	if fixed.Record() || !read.Record() || file.NumRecs() != 3 {
		t.Errorf("read back, %t and %t, %d records", fixed.Record(),
			read.Record(), file.NumRecs())
	}
}

// Values of a Go type other than the one a variable is stored as are
// refused, whole and by slab, in an error that wraps ErrTypeMismatch and
// names the variable.
func TestValuesOfAnotherTypeAreRefused(t *testing.T) {
	writer := mixedFile(t, filepath.Join(t.TempDir(), "refused.nc"))
	defer writer.Close()
	grow, err := writer.Variable("grow")
	check(t, err)
	slab := netcdf.Slab{Start: []int64{0, 0}, Count: []int64{1, 2}}
	whole := grow.Write([]float64{1, 2})
	part := grow.WriteSlab([]float64{1, 2}, slab)
	for _, got := range []error{whole, part} {
		if !errors.Is(got, netcdf.ErrTypeMismatch) {
			t.Errorf("a float64 slice for shorts: %v", got)
		}
	}
	if writer.NumRecs() != 0 {
		t.Errorf("%d records after two refusals", writer.NumRecs())
	}
}

// Anti: of the eleven types, every variable takes the Go slice of its own
// type and no other, but that the text and unsigned byte share one.
func TestEachVariableTakesOnlyItsOwnGoType(t *testing.T) {
	path := typesFile(t)
	file, err := netcdf.Open(path)
	check(t, err)
	defer file.Close()
	writer := rewritten(t, file)
	defer writer.Close()
	for typ := range valuesOf() {
		variable, err := writer.Variable(netcdf.TypeName(typ))
		check(t, err)
		for other, values := range valuesOf() {
			got := variable.Write(values)
			same := reflect.TypeOf(values) == reflect.TypeOf(valuesOf()[typ])
			if same != (got == nil) ||
				!same && !errors.Is(got, netcdf.ErrTypeMismatch) {
				t.Errorf("%s given %s: %v", netcdf.TypeName(typ),
					netcdf.TypeName(other), got)
			}
		}
	}
}

// rewritten is a writer of a new file with the variables of a file of
// each of the eleven types, over a dimension of three.
func rewritten(t *testing.T, file *netcdf.File) *netcdf.Writer {
	t.Helper()
	layout, err := netcdf.NewLayout(file.Format())
	check(t, err)
	length, err := layout.AddDimension("n", 3)
	check(t, err)
	for _, each := range file.Variables() {
		_, err := layout.AddVariable(netcdf.VariableSpec{Name: each.Name(),
			Type: each.Type(), Dimensions: []*netcdf.LayoutDimension{length}})
		check(t, err)
	}
	writer, err := layout.Create(filepath.Join(t.TempDir(), "again.nc"))
	check(t, err)
	return writer
}
