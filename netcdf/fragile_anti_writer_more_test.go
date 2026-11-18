//go:build fragile

package netcdf_test

// The anti-tests of writer_more_test.go, written against his own package.

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	classic "github.com/scigolib/netcdf"
	"pgregory.net/rapid"
)

// antiMustSet stops the test if an attribute is refused.
func antiMustSet(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("an attribute was refused: %v", err)
	}
}

// The test at writer_more_test.go:15 asserts the accessors of a schema
// for one dimension and one variable. Its anti sets other names, lengths,
// types and orders and asks that each accessor says the new thing, so that
// an accessor that answers what the test happens to ask is seen.
func TestAntiSchemaAccessors(t *testing.T) {
	for _, each := range []struct {
		format classic.Format
		name   string
		length int64
		typ    classic.Type
	}{{classic.FormatClassic, "x", 7, classic.TypeShort},
		{classic.FormatClassic64, "lat", 1, classic.TypeDouble},
		{classic.FormatClassic64Data, "ß", 0, classic.TypeUInt64}} {
		s := classic.NewSchema(each.format)
		dim := s.AddDimension(each.name, each.length)
		other := s.AddDimension("other", 3)
		v := s.AddVariable("v"+each.name, each.typ, other, dim)
		if s.Format() != each.format || dim.Name() != each.name ||
			dim.Len() != each.length || dim.Unlimited() != (each.length == 0) ||
			other.Len() != 3 || other.Unlimited() {
			t.Errorf("%v: accessors say %v %q %d %t", each.format, s.Format(),
				dim.Name(), dim.Len(), dim.Unlimited())
		}
		if v.Name() != "v"+each.name || v.Type() != each.typ ||
			!slices.Equal(v.Dims(), []*classic.SchemaDim{other, dim}) {
			t.Errorf("%v: the variable is %q %v over %d", each.format, v.Name(),
				v.Type(), len(v.Dims()))
		}
		if len(s.Dimensions()) != 2 || len(s.Variables()) != 1 ||
			len(v.Attributes()) != 0 {
			t.Errorf("%v: %d dimensions, %d variables, %d attributes",
				each.format, len(s.Dimensions()), len(s.Variables()),
				len(v.Attributes()))
		}
	}
}

// The test at writer_more_test.go:46 sets an unsigned byte attribute and
// replaces a text one. Its anti: the same values set as TypeByte read as
// a byte and not an unsigned one; a name set three times is the third; and
// a second name is not a replacement, so there are three attributes.
func TestAntiSetAttrTypedAndReplace(t *testing.T) {
	s := classic.NewSchema(classic.FormatClassic64Data)
	antiMustSet(t, s.SetAttrTyped("raw", classic.TypeByte, []int8{-1, 2, 3}))
	for _, text := range []string{"first", "second", "third"} {
		antiMustSet(t, s.SetAttr("note", text))
	}
	antiMustSet(t, s.SetAttr("other", "text"))
	file, err := antiOpen(antiSaved(t, s, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	raw, _ := file.Attribute("raw")
	if raw == nil || raw.Type() != classic.TypeByte ||
		!reflect.DeepEqual(raw.Value(), []int8{-1, 2, 3}) {
		t.Errorf("raw reads as %v", raw)
	}
	if note, _ := file.Attribute("note"); note == nil ||
		note.String() != "third" {
		t.Errorf("note reads as %v, want third", note)
	}
	if len(file.Attributes()) != 3 {
		t.Errorf("%d attributes, want 3", len(file.Attributes()))
	}
}

// The test at writer_more_test.go:91 asserts that a wrong attribute is
// refused and keeps the schema from being created. Its anti: the right
// attribute in each of its four forms is accepted, and a schema whose
// every attribute was accepted is created.
func TestAntiSetAttrErrors(t *testing.T) {
	s := classic.NewSchema(classic.FormatClassic64Data)
	x := s.AddDimension("x", 2)
	v := s.AddVariable("v", classic.TypeInt, x)
	for _, err := range []error{s.SetAttr("ok", "text"),
		s.SetAttr("ints", []int32{1}),
		v.SetAttrTyped("units", classic.TypeFloat, []float32{1}),
		s.SetAttrTyped("x", classic.TypeUInt64, []uint64{1})} {
		if err != nil {
			t.Errorf("a right attribute was refused: %v", err)
		}
	}
	antiSaved(t, s, nil)
}

// The test at writer_more_test.go:114 writes 42 to a variable of no
// dimensions and reads it back. Its anti: 43 reads as 43, two values for a
// scalar are refused, and the file has one value and not two.
func TestAntiWriterScalarRoundTrip(t *testing.T) {
	for _, number := range []int32{42, 43, -1} {
		s := classic.NewSchema(classic.FormatClassic64)
		s.AddVariable("answer", classic.TypeInt)
		raw := antiSaved(t, s, func(w *classic.Writer) {
			if err := antiWriter(t, w, "answer").Write([]int32{number}); err != nil {
				t.Error(err)
			}
		})
		file, err := antiOpen(raw)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readHis(antiVariable(t, file, "answer"), 1<<20)
		file.Close()
		if err != nil || !reflect.DeepEqual(got, []int32{number}) {
			t.Errorf("%d reads as %v, %v", number, got, err)
		}
	}
	s := classic.NewSchema(classic.FormatClassic64)
	s.AddVariable("answer", classic.TypeInt)
	antiSaved(t, s, func(w *classic.Writer) {
		err := antiWriter(t, w, "answer").Write([]int32{1, 2})
		if !errors.Is(err, classic.ErrOutOfRange) {
			t.Errorf("two values for a scalar: %v", err)
		}
	})
}

// antiRecordSchema is a schema of a record variable over x=2.
func antiRecordSchema() *classic.Schema {
	s := classic.NewSchema(classic.FormatClassic)
	time := s.AddDimension("time", 0)
	x := s.AddDimension("x", 2)
	s.AddVariable("rec", classic.TypeInt, time, x)
	return s
}

// The test at writer_more_test.go:146 asserts that a partial record, a
// count of zero with a value, an empty selection and a wrong rank are as
// they should be. Its anti: whole records are accepted and each extends the
// count by one, a slab of one record and the whole width likewise, and an
// empty selection of the right rank is accepted.
func TestAntiWriterSelectionErrors(t *testing.T) {
	antiSaved(t, antiRecordSchema(), func(w *classic.Writer) {
		v := antiWriter(t, w, "rec")
		for at, step := range []func() error{
			func() error { return v.Write([]int32{1, 2}) },
			func() error {
				return v.WriteSlice([]int32{3, 4}, []int64{1, 0}, []int64{1, 2})
			},
			func() error {
				return v.WriteSlice([]int32{}, []int64{0, 0}, []int64{0, 0})
			}} {
			if err := step(); err != nil {
				t.Errorf("a right write %d was refused: %v", at, err)
			}
		}
		if w.NumRecs() != 2 {
			t.Errorf("%d records, want 2", w.NumRecs())
		}
	})
}

// The test at writer_more_test.go:178 asserts that a rejected slab does
// not extend the record count. Its anti: the same slab, accepted, extends
// it to one, and one at record three to four.
func TestAntiRejectedWriteDoesNotExtend(t *testing.T) {
	for _, each := range []struct {
		record int64
		want   int64
	}{{0, 1}, {3, 4}} {
		antiSaved(t, antiRecordSchema(), func(w *classic.Writer) {
			err := antiWriter(t, w, "rec").WriteSlice([]int32{5},
				[]int64{each.record, 1}, []int64{1, 1})
			if err != nil || w.NumRecs() != each.want {
				t.Errorf("record %d: %v, %d records, want %d", each.record,
					err, w.NumRecs(), each.want)
			}
		})
	}
}

// The test at writer_more_test.go:211 asserts that a zero count does not
// skip the check of the other dimensions. Its anti: a zero count with an
// inner start inside the variable is accepted, and so is one at its edge,
// from what netCDF-C's NCedgeck does (guessed, not checked).
func TestAntiZeroCountValidatesAllDimensions(t *testing.T) {
	s := classic.NewSchema(classic.FormatClassic)
	x := s.AddDimension("x", 3)
	y := s.AddDimension("y", 2)
	s.AddVariable("v", classic.TypeInt, x, y)
	antiSaved(t, s, func(w *classic.Writer) {
		v := antiWriter(t, w, "v")
		for _, start := range [][]int64{{0, 1}, {0, 2}, {3, 2}} {
			err := v.WriteSlice([]int32{}, start, []int64{0, 0})
			if err != nil {
				t.Errorf("a zero count from %v: %v", start, err)
			}
		}
	})
}

// The test at writer_more_test.go:249 asserts that what is written is read
// back, for thirty models. A write and a read that are wrong in the same
// way, the byte order of every value, say, pass it. Its anti draws models
// and asserts the values lie in the file where the grammar says, big-endian,
// which the reader's own decoding cannot tell.
func TestAntiRandomRoundTrip(t *testing.T) {
	files := newScratch(t)
	rapid.Check(t, func(t *rapid.T) { checkPlaced(t, files) })
}
