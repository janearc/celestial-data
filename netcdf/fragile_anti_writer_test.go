//go:build fragile

package netcdf_test

// The anti-tests of writer_test.go, written against his own package.

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	classic "github.com/scigolib/netcdf"
)

// antiLayoutSchema is the layout of netcdf's own tests: a fixed 3-D variable, a
// byte record variable and an int record variable, in a format.
func antiLayoutSchema(format classic.Format) *classic.Schema {
	s := classic.NewSchema(format)
	time := s.AddDimension("time", 0)
	x := s.AddDimension("x", 3)
	y := s.AddDimension("y", 2)
	z := s.AddDimension("z", 4)
	s.AddVariable("cube", classic.TypeFloat, z, y, x)
	s.AddVariable("b", classic.TypeByte, time)
	s.AddVariable("temp", classic.TypeInt, time, x)
	return s
}

// antiWriteLayout writes the values of the layout, with the changes: the
// cube's value at 7, and how many records of temp.
func antiWriteLayout(t *testing.T, w *classic.Writer, seven float32,
	records int) {
	t.Helper()
	cube := make([]float32, 24)
	for at := range cube {
		cube[at] = float32(at)
	}
	cube[7] = seven
	temp := make([]int32, 3*records)
	for at := range temp {
		temp[at] = int32(10 + at)
	}
	b := make([]int8, records)
	for _, err := range []error{antiWriter(t, w, "cube").Write(cube),
		antiWriter(t, w, "b").Write(b), antiWriter(t, w, "temp").Write(temp)} {
		if err != nil {
			t.Error(err)
		}
	}
}

// The test at writer_test.go:14 writes the layout of five records and
// reads it back. Its anti: the same with the cube's value 7 changed reads
// back with that one changed, and with four records of temp the file
// holds four, writer and file agreeing.
func TestAntiWriterRoundTrip(t *testing.T) {
	for _, each := range []struct {
		seven   float32
		records int
	}{{7, 5}, {99, 5}, {7, 4}} {
		var count int64
		raw := antiSaved(t, antiLayoutSchema(classic.FormatClassic64Data),
			func(w *classic.Writer) {
				antiWriteLayout(t, w, each.seven, each.records)
				count = w.NumRecs()
			})
		file, err := antiOpen(raw)
		if err != nil {
			t.Fatal(err)
		}
		cube, err := readHis(antiVariable(t, file, "cube"), 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		temp, err := readHis(antiVariable(t, file, "temp"), 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		cubes, temps := cube.([]float32), temp.([]int32)
		if cubes[7] != each.seven || cubes[6] != 6 ||
			len(temps) != 3*each.records || count != int64(each.records) ||
			file.NumRecs() != count {
			t.Errorf("seven %v records %d: cube[7] %v temp %d values, writer "+
				"%d, file %d", each.seven, each.records, cubes[7], len(temps),
				count, file.NumRecs())
		}
		file.Close()
	}
}

// antiTwoRecordVariables is a schema of two record variables, temp of
// ints over x=2 and flag of shorts.
func antiTwoRecordVariables() *classic.Schema {
	s := classic.NewSchema(classic.FormatClassic)
	time := s.AddDimension("time", 0)
	x := s.AddDimension("x", 2)
	s.AddVariable("temp", classic.TypeInt, time, x)
	s.AddVariable("flag", classic.TypeShort, time)
	return s
}

// The test at writer_test.go:71 writes record 2 of temp and asserts that
// records 0 and 1 hold the fill value in both variables. Its anti: record
// 3 gives four records, three of them fill; record 0 gives one and no fill
// between; and the same write of flag leaves temp the fill value, so that
// the fill is of every record variable and of the variable not written.
func TestAntiWriterRecordExtension(t *testing.T) {
	const fillInt, fillShort = -2147483647, -32767
	for _, each := range []struct {
		name   string
		record int64
	}{{"temp", 3}, {"temp", 0}, {"flag", 2}} {
		raw := antiSaved(t, antiTwoRecordVariables(), func(w *classic.Writer) {
			values := any([]int32{99, 98})
			slab := []int64{each.record, 0}
			count := []int64{1, 2}
			if each.name == "flag" {
				values, slab, count = []int16{9}, slab[:1], count[:1]
			}
			if err := antiWriter(t, w, each.name).WriteSlice(values, slab,
				count); err != nil {
				t.Error(err)
			}
		})
		file, err := antiOpen(raw)
		if err != nil {
			t.Fatal(err)
		}
		temp, _ := readHis(antiVariable(t, file, "temp"), 1<<20)
		flag, _ := readHis(antiVariable(t, file, "flag"), 1<<20)
		file.Close()
		wantTemp := slices.Repeat([]int32{fillInt}, 2*(int(each.record)+1))
		wantFlag := slices.Repeat([]int16{fillShort}, int(each.record)+1)
		if each.name == "temp" {
			copy(wantTemp[2*each.record:], []int32{99, 98})
		} else {
			wantFlag[each.record] = 9
		}
		if !reflect.DeepEqual(temp, wantTemp) || !reflect.DeepEqual(flag,
			wantFlag) {
			t.Errorf("%s record %d: temp %v flag %v, want %v %v", each.name,
				each.record, temp, flag, wantTemp, wantFlag)
		}
	}
}

// The test at writer_test.go:124 asserts that _FillValue overrides the
// default in the initial fill and in the records appended. Its anti: a
// variable with no _FillValue holds the default beside one that has it, a
// _FillValue of another number reads as that number, and one variable's
// _FillValue is not another's.
func TestAntiWriterFillValue(t *testing.T) {
	s := classic.NewSchema(classic.FormatClassic)
	time := s.AddDimension("time", 0)
	x := s.AddDimension("x", 2)
	antiMustSet(t, s.AddVariable("fixed", classic.TypeInt, x).SetAttr(
		"_FillValue", []int32{43}))
	s.AddVariable("plain", classic.TypeInt, x)
	rec := s.AddVariable("rec", classic.TypeInt, time, x)
	antiMustSet(t, rec.SetAttr("_FillValue", []int32{-8}))
	s.AddVariable("plainrec", classic.TypeInt, time, x)
	raw := antiSaved(t, s, func(w *classic.Writer) {
		if err := antiWriter(t, w, "rec").WriteSlice([]int32{1, 2},
			[]int64{2, 0}, []int64{1, 2}); err != nil {
			t.Error(err)
		}
	})
	file, err := antiOpen(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	const fillInt = -2147483647
	for name, want := range map[string][]int32{"fixed": {43, 43},
		"plain": {fillInt, fillInt}, "rec": {-8, -8, -8, -8, 1, 2},
		"plainrec": slices.Repeat([]int32{fillInt}, 6)} {
		got, _ := readHis(antiVariable(t, file, name), 1<<20)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s reads %v, want %v", name, got, want)
		}
	}
}

// antiPair is a schema made two ways: wrong in one thing, and with that
// one thing right.
type antiPair struct {
	name string
	// want is the error the wrong one is refused as, or none to say any.
	want error
	// writer is whether the writer's own test has this case.
	writer bool
	build  func(bad bool) *classic.Schema
}

// antiOneDimension is a schema of one variable over the dimensions given
// and the format.
func antiOneDimension(format classic.Format, lengths ...int64) *classic.Schema {
	s := classic.NewSchema(format)
	var over []*classic.SchemaDim
	for at, length := range lengths {
		over = append(over, s.AddDimension(string(rune('a'+at)), length))
	}
	s.AddVariable("v", classic.TypeInt, over...)
	return s
}

// antiPairs are the cases of the validation of a schema, each with its
// nearest right schema.
func antiPairs() []antiPair {
	invalid := classic.ErrInvalidSchema
	pick := func(bad bool, wrong, right string) string {
		if bad {
			return wrong
		}
		return right
	}
	return []antiPair{
		{"a dimension twice", invalid, false, func(bad bool) *classic.Schema {
			s := classic.NewSchema(classic.FormatClassic)
			s.AddDimension("x", 2)
			s.AddDimension(pick(bad, "x", "y"), 2)
			return s
		}},
		{"a negative dimension", nil, false, func(bad bool) *classic.Schema {
			return antiOneDimension(classic.FormatClassic, map[bool]int64{
				true: -1, false: 1}[bad])
		}},
		{"two unlimited dimensions", invalid, true, func(bad bool) *classic.
			Schema {
			s := classic.NewSchema(classic.FormatClassic)
			s.AddDimension("t", 0)
			s.AddDimension("u", map[bool]int64{true: 0, false: 4}[bad])
			return s
		}},
		{"a bad dimension name", invalid, true, func(bad bool) *classic.Schema {
			s := classic.NewSchema(classic.FormatClassic)
			s.AddDimension(pick(bad, "a/b", "a_b"), 2)
			return s
		}},
		{"a duplicate variable", invalid, true, func(bad bool) *classic.Schema {
			s := classic.NewSchema(classic.FormatClassic)
			x := s.AddDimension("x", 2)
			s.AddVariable("v", classic.TypeInt, x)
			s.AddVariable(pick(bad, "v", "w"), classic.TypeInt, x)
			return s
		}},
		{"a dimension of another schema", invalid, true, func(bad bool) *classic.
			Schema {
			s := classic.NewSchema(classic.FormatClassic)
			own := s.AddDimension("x", 2)
			foreign := classic.NewSchema(classic.FormatClassic).AddDimension("x",
				2)
			s.AddVariable("v", classic.TypeInt, map[bool]*classic.SchemaDim{
				true: foreign, false: own}[bad])
			return s
		}},
		{"an unlimited dimension that is not first", invalid, false, func(
			bad bool) *classic.Schema {
			s := classic.NewSchema(classic.FormatClassic)
			x, t := s.AddDimension("x", 2), s.AddDimension("t", 0)
			if bad {
				s.AddVariable("v", classic.TypeInt, x, t)
			} else {
				s.AddVariable("v", classic.TypeInt, t, x)
			}
			return s
		}},
		{"a type that is not in the format", invalid, true, func(bad bool) *classic.
			Schema {
			s := classic.NewSchema(classic.FormatClassic)
			x := s.AddDimension("x", 2)
			s.AddVariable("v", map[bool]classic.Type{true: classic.TypeUByte,
				false: classic.TypeInt}[bad], x)
			return s
		}},
		{"a dimension above 32 bits", invalid, false, func(bad bool) *classic.
			Schema {
			format := map[bool]classic.Format{true: classic.FormatClassic,
				false: classic.FormatClassic64Data}[bad]
			s := classic.NewSchema(format)
			s.AddDimension("x", 1<<33)
			return s
		}},
		{"an attribute type that is not in the format", nil, false, func(
			bad bool) *classic.Schema {
			s := classic.NewSchema(classic.FormatClassic)
			if bad {
				_ = s.SetAttrTyped("a", classic.TypeUInt, []uint32{1})
			} else {
				_ = s.SetAttrTyped("a", classic.TypeInt, []int32{1})
			}
			return s
		}},
		{"a fill value of another type", invalid, true, func(bad bool) *classic.
			Schema {
			s := classic.NewSchema(classic.FormatClassic)
			v := s.AddVariable("v", classic.TypeInt, s.AddDimension("x", 2))
			if bad {
				_ = v.SetAttr("_FillValue", []float64{1})
			} else {
				_ = v.SetAttr("_FillValue", []int32{1})
			}
			return s
		}},
		{"a fill value of two values", invalid, false, func(bad bool) *classic.
			Schema {
			s := classic.NewSchema(classic.FormatClassic)
			v := s.AddVariable("v", classic.TypeInt, s.AddDimension("x", 2))
			if bad {
				_ = v.SetAttr("_FillValue", []int32{1, 2})
			} else {
				_ = v.SetAttr("_FillValue", []int32{1})
			}
			return s
		}},
		{"a format that is HDF5", classic.ErrUnsupportedFormat, true, func(
			bad bool) *classic.Schema {
			return classic.NewSchema(map[bool]classic.Format{
				true: classic.FormatHDF5, false: classic.FormatClassic}[bad])
		}},
	}
}

// antiRefusedAndAccepted runs the pairs: the wrong schema is refused as
// the kind it should be and leaves no file, the right schema is created
// and the file opens.
func antiRefusedAndAccepted(t *testing.T, writerOnly bool) {
	for _, each := range antiPairs() {
		if writerOnly && !each.writer {
			continue
		}
		t.Run(each.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.nc")
			_, err := classic.Create(path, each.build(true))
			if err == nil || (each.want != nil && !errors.Is(err, each.want)) {
				t.Errorf("the wrong schema: %v, want %v", err, each.want)
			}
			if _, statErr := os.Stat(path); statErr == nil && err != nil {
				t.Errorf("a refused schema left a file")
			}
			raw := antiSaved(t, each.build(false), nil)
			antiAccepted(t, "the right schema", raw)
		})
	}
}

// The test at writer_test.go:175 asserts that seven wrong schemas are
// refused and leave no file. Its anti: each is made right in the one thing
// that was wrong, and the schema is created and the file opens; a writer
// that refused every schema would pass the test.
func TestAntiWriterValidationCoversSchema(t *testing.T) {
	antiRefusedAndAccepted(t, true)
}

// The test at writer_test.go:238 asserts the refusals of a write: a wrong
// element type, a short slice, a slab past the end, and a write after
// Close. Its anti: the right type, the exact length and a slab inside the
// variable are accepted and land where they were put; two Closes are
// nothing; and a write before the Close is not refused as closed.
func TestAntiWriterUsageErrors(t *testing.T) {
	s := classic.NewSchema(classic.FormatClassic)
	s.AddVariable("v", classic.TypeInt, s.AddDimension("x", 3))
	path := filepath.Join(t.TempDir(), "usage.nc")
	w, err := classic.Create(path, s)
	if err != nil {
		t.Fatal(err)
	}
	v := antiWriter(t, w, "v")
	if err := v.Write([]int32{1, 2, 3}); err != nil {
		t.Errorf("a write before Close: %v", err)
	}
	if err := v.WriteSlice([]int32{7, 8}, []int64{1}, []int64{2}); err != nil {
		t.Errorf("a slab inside the variable: %v", err)
	}
	for range 2 {
		if err := w.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}
	if err := v.Write([]int32{1, 2, 3}); !errors.Is(err, classic.ErrClosed) {
		t.Errorf("a write after Close: %v, want the closed error", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := antiOpen(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, _ := readHis(antiVariable(t, file, "v"), 1<<20)
	if !reflect.DeepEqual(got, []int32{1, 7, 8}) {
		t.Errorf("the writes read as %v, want [1 7 8]", got)
	}
}
