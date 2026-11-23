package netcdf_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/janearc/celestial-data/netcdf"
)

// valuesOf are three values of each of the eleven types, as the Go type
// each is stored as. The unsigned and 64-bit ones go past the signed and
// 32-bit ranges.
func valuesOf() map[netcdf.Type]any {
	return map[netcdf.Type]any{
		netcdf.TypeByte:   []int8{-1, 2, 3},
		netcdf.TypeChar:   []byte("abc"),
		netcdf.TypeShort:  []int16{-300, 2, 3},
		netcdf.TypeInt:    []int32{-70000, 2, 3},
		netcdf.TypeFloat:  []float32{1.5, -2, 3},
		netcdf.TypeDouble: []float64{1.25, -2, 3},
		netcdf.TypeUByte:  []byte{1, 2, 200},
		netcdf.TypeUShort: []uint16{1, 2, 65535},
		netcdf.TypeUInt:   []uint32{1, 2, 4000000000},
		netcdf.TypeInt64:  []int64{-1 << 40, 2, 3},
		netcdf.TypeUInt64: []uint64{1, 2, 1 << 63},
	}
}

// sizesOf are the bytes one value of each type takes in a file.
var sizesOf = map[netcdf.Type]int{netcdf.TypeByte: 1, netcdf.TypeChar: 1,
	netcdf.TypeShort: 2, netcdf.TypeInt: 4, netcdf.TypeFloat: 4,
	netcdf.TypeDouble: 8, netcdf.TypeUByte: 1, netcdf.TypeUShort: 2,
	netcdf.TypeUInt: 4, netcdf.TypeInt64: 8, netcdf.TypeUInt64: 8}

// typesFile writes a CDF-5 file with one variable of each type.
func typesFile(t *testing.T) string {
	t.Helper()
	layout, err := netcdf.NewLayout(netcdf.FormatClassic64Data)
	check(t, err)
	length, err := layout.AddDimension("n", 3)
	check(t, err)
	for _, typ := range netcdf.AllTypes() {
		_, err := layout.AddVariable(netcdf.VariableSpec{
			Name: netcdf.TypeName(typ), Type: typ,
			Dimensions: []*netcdf.LayoutDimension{length}})
		check(t, err)
	}
	path := filepath.Join(t.TempDir(), "types.nc")
	writer, err := layout.Create(path)
	check(t, err)
	for typ, each := range valuesOf() {
		variable, err := writer.Variable(netcdf.TypeName(typ))
		check(t, err)
		check(t, variable.Write(each))
	}
	check(t, writer.Close())
	return path
}

// Each of the eleven types is written and read whole and by slab, comes
// back as the Go type it is stored as, and widens, but for text.
func TestEveryTypeRoundTrips(t *testing.T) {
	file, err := netcdf.Open(typesFile(t))
	check(t, err)
	defer file.Close()
	if len(netcdf.AllTypes()) != len(sizesOf) {
		t.Fatalf("%d types", len(netcdf.AllTypes()))
	}
	for typ, want := range valuesOf() {
		variable, err := file.Variable(netcdf.TypeName(typ))
		check(t, err)
		got, err := variable.Values()
		check(t, err)
		slab, err := variable.Slab(netcdf.Slab{Start: []int64{1},
			Count: []int64{2}})
		check(t, err)
		if !reflect.DeepEqual(got, want) || variable.Type() != typ ||
			!reflect.DeepEqual(slab, reflect.ValueOf(want).Slice(1, 3).Interface()) ||
			netcdf.TypeSize(typ) != sizesOf[typ] {
			t.Errorf("%s: %v, %v", netcdf.TypeName(typ), got, slab)
		}
		if typ != netcdf.TypeChar {
			widened, err := variable.Numbers()
			check(t, err)
			part, err := variable.NumbersSlab(netcdf.Slab{
				Start: []int64{1}, Count: []int64{2}})
			check(t, err)
			if len(widened) != 3 || !slices.Equal(part, widened[1:]) {
				t.Errorf("%s widened to %v and %v", netcdf.TypeName(typ),
					widened, part)
			}
		}
	}
}

// The layout, the writer and the file are written and read through every
// method: dimensions, attributes of both kinds of setting, a record
// dimension extended, and a gap filled.
func TestLayoutWriterAndFileSayTheSameThings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layout.nc")
	writeLayout(t, path)
	file, err := netcdf.Open(path)
	check(t, err)
	defer file.Close()
	if file.Format() != netcdf.FormatClassic64Data || file.NumRecs() != 4 ||
		file.Name() != path {
		t.Errorf("%s %d %s", netcdf.FormatName(file.Format()),
			file.NumRecs(), file.Name())
	}
	readDimensions(t, file)
	readAttributes(t, file)
	readVariable(t, file)
}

// writeLayout writes the file the test above reads: a record variable v
// of shorts over t and x, with a fill value, two records written whole,
// then one written two past the end.
func writeLayout(t *testing.T, path string) {
	t.Helper()
	layout, err := netcdf.NewLayout(netcdf.FormatClassic64Data)
	check(t, err)
	steps, err := layout.AddDimension("t", 0)
	check(t, err)
	spots, err := layout.AddDimension("x", 2)
	check(t, err)
	variable, err := layout.AddVariable(netcdf.VariableSpec{Name: "v",
		Type: netcdf.TypeShort, Dimensions: []*netcdf.LayoutDimension{steps,
			spots}})
	check(t, err)
	check(t, variable.Set("units", "K"))
	check(t, variable.SetTyped("_FillValue", netcdf.TypeShort, []int16{-1}))
	check(t, variable.SetTyped("flags", netcdf.TypeUByte, []byte{1, 2}))
	check(t, layout.Set("title", "a test"))
	check(t, layout.SetTyped("level", netcdf.TypeUInt, []uint32{7}))
	checkLayout(t, layout, variable)
	writer, err := layout.Create(path)
	check(t, err)
	checkWriter(t, writer)
	check(t, writer.Close())
	check(t, writer.Close())
}

// checkLayout checks the layout and its variable say what was set.
func checkLayout(t *testing.T, layout *netcdf.Layout,
	variable *netcdf.LayoutVariable) {
	t.Helper()
	dimensions, variables := layout.Dimensions(), layout.Variables()
	over := variable.Dimensions()
	if layout.Format() != netcdf.FormatClassic64Data ||
		len(dimensions) != 2 || len(variables) != 1 || len(over) != 2 ||
		!over[0].Unlimited() || over[1].Unlimited() ||
		over[0].Name() != "t" || over[1].Length() != 2 ||
		variable.Name() != "v" || variable.Type() != netcdf.TypeShort ||
		len(variable.Attributes()) != 3 ||
		variable.Attributes()[2].Name() != "flags" ||
		dimensions[1].Name() != "x" || variables[0].Name() != "v" {
		t.Errorf("%v %v %v", dimensions, variables, over)
	}
}

// checkWriter writes the values through the writer, checking what it says
// of itself as it goes.
func checkWriter(t *testing.T, writer *netcdf.Writer) {
	t.Helper()
	variable, err := writer.Variable("v")
	check(t, err)
	if writer.Format() != netcdf.FormatClassic64Data ||
		len(writer.Variables()) != 1 || variable.Name() != "v" ||
		variable.Type() != netcdf.TypeShort || !variable.Record() ||
		!slices.Equal(variable.Shape(), []int64{0, 2}) {
		t.Errorf("%v", variable.Shape())
	}
	check(t, variable.Write([]int16{1, 2, 3, 4}))
	if writer.NumRecs() != 2 {
		t.Errorf("%d records after a whole write", writer.NumRecs())
	}
	check(t, variable.WriteSlab([]int16{9, 9}, netcdf.Slab{
		Start: []int64{3, 0}, Count: []int64{1, 2}}))
	if writer.NumRecs() != 4 || !slices.Equal(variable.Shape(),
		[]int64{4, 2}) {
		t.Errorf("%d records after a slab", writer.NumRecs())
	}
}

// readDimensions checks the file's dimensions.
func readDimensions(t *testing.T, file *netcdf.File) {
	t.Helper()
	dimensions := file.Dimensions()
	spots, err := file.Dimension("x")
	check(t, err)
	if len(dimensions) != 2 || !dimensions[0].Unlimited() ||
		dimensions[0].Length() != 4 || spots.Length() != 2 {
		t.Errorf("%d dimensions", len(dimensions))
	}
}

// readAttributes checks the file's global attributes.
func readAttributes(t *testing.T, file *netcdf.File) {
	t.Helper()
	title, err := file.Attribute("title")
	check(t, err)
	level, err := file.Attribute("level")
	check(t, err)
	if len(file.Attributes()) != 2 || title.Name() != "title" ||
		title.Type() != netcdf.TypeChar || title.Length() != 6 ||
		title.Text() != "a test" || string(title.Bytes()) != "a test" ||
		!reflect.DeepEqual(title.Value(), []byte("a test")) ||
		!reflect.DeepEqual(level.Value(), []uint32{7}) ||
		level.Text() != "" || level.Type() != netcdf.TypeUInt {
		t.Errorf("%v %v", title.Value(), level.Value())
	}
	if number, have := level.Number(); !have || number != 7 {
		t.Errorf("level %v %t", number, have)
	}
	if _, have := title.Number(); have {
		t.Error("a text attribute has a number")
	}
}

// readVariable checks the variable, its attributes, and its values, with
// the gap the slab left filled with the _FillValue.
func readVariable(t *testing.T, file *netcdf.File) {
	t.Helper()
	variable, err := file.Variable("v")
	check(t, err)
	flags, err := variable.Attribute("flags")
	check(t, err)
	if len(file.Variables()) != 1 || len(variable.Dimensions()) != 2 ||
		!slices.Equal(variable.Shape(), []int64{4, 2}) ||
		!variable.Record() || variable.Length() != 8 ||
		len(variable.Attributes()) != 3 || flags.Type() != netcdf.TypeUByte ||
		!reflect.DeepEqual(flags.Value(), []byte{1, 2}) {
		t.Errorf("%v", variable.Shape())
	}
	got, err := variable.Values()
	check(t, err)
	if want := []int16{1, 2, 3, 4, -1, -1, 9, 9}; !reflect.DeepEqual(got, want) {
		t.Errorf("values %v, want %v", got, want)
	}
}

// The format is told from a file's first bytes, and every format has its
// name.
func TestFormatsAreToldFromTheFirstBytes(t *testing.T) {
	raw, err := os.ReadFile(typesFile(t))
	check(t, err)
	hdf5 := []byte("\x89HDF\r\n\x1a\n")
	for want, header := range map[netcdf.Format][]byte{
		netcdf.FormatClassic64Data: raw[:8], netcdf.FormatHDF5: hdf5,
		netcdf.FormatUnknown: []byte("hello")} {
		if got := netcdf.DetectFormat(header); got != want {
			t.Errorf("%q is %s, want %s", header, netcdf.FormatName(got),
				netcdf.FormatName(want))
		}
	}
	if netcdf.FormatName(netcdf.FormatClassic) != "CDF-1" ||
		netcdf.FormatName(netcdf.FormatClassic64) != "CDF-2" {
		t.Error("the classic formats are misnamed")
	}
}
