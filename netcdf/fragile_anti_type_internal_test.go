//go:build fragile

package netcdf_test

// The anti-tests of type_internal_test.go, which is in his own package
// and reaches the unexported codecs and translate. What those check is
// reached here through bytes and the public calls.

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	classic "github.com/scigolib/netcdf"
)

// The test at type_internal_test.go:15 keeps two tables of sizes equal,
// his public type and his internal one; if both were wrong in the same
// way it would pass. Its anti checks the sizes against the widths of the
// specification (the harness's table), against the file the writer makes
// of one value of each type, and that a number that is no type has none.
func TestAntiTypeValuesMatchFormatLayer(t *testing.T) {
	for typ := classic.TypeByte; typ <= classic.TypeUInt64; typ++ {
		if got, want := typ.Size(), wireWidth(uint32(typ)); got != want {
			t.Errorf("%v.Size() is %d, the specification says %d", typ, got,
				want)
		}
	}
	for _, typ := range []classic.Type{0, 12, 255} {
		if got := typ.Size(); got != 0 {
			t.Errorf("Type(%d).Size() is %d, want 0", typ, got)
		}
	}
	schema := classic.NewSchema(classic.FormatClassic64Data)
	length := schema.AddDimension("n", 1)
	head := emptyHeader(cdfFive)
	head.Dims = []dimSpec{{Name: "n", Length: 1}}
	for typ := classic.TypeByte; typ <= classic.TypeUInt64; typ++ {
		name := "v" + typ.String()
		schema.AddVariable(name, typ, length)
		head.Vars = append(head.Vars, varSpec{Name: name, DimIDs: []uint64{0},
			Type: uint32(typ)})
	}
	raw := antiSaved(t, schema, nil)
	if want := head.place(uint64(len(head.Bytes()))).end; uint64(len(raw)) !=
		want {
		t.Errorf("a file of one value of each type is %d bytes, the "+
			"specification's layout is %d", len(raw), want)
	}
}

// antiLoneFile is a header of one variable of a type tag over x=2 in a
// version, with the data after it.
func antiLoneFile(version int, tag uint32) []byte {
	return fixedVariable(version, tag, 2).File()
}

// The test at type_internal_test.go:30 checks that three kinds of error
// of the format layer are found as their public sentinel and not as the
// internal one. Its anti: each of three inputs is found as its own
// sentinel and as neither of the other two, so that a translation that
// called every error one kind is seen. (That the internal sentinel does
// not leak cannot be asked from outside his package.)
func TestAntiErrorModel(t *testing.T) {
	junk := []byte("nope")
	cut := []byte("CDF\x01\x00\x00\x00")
	cases := []struct {
		what string
		err  error
		want error
	}{{"junk", openError(junk), classic.ErrNotNetCDF},
		{"a cut header", openError(cut), classic.ErrCorrupt},
		{"a slab past the end", slabError(t), classic.ErrOutOfRange},
		{"a header with no data", dataError(t), classic.ErrCorrupt}}
	for _, each := range cases {
		for _, kind := range []error{classic.ErrNotNetCDF, classic.ErrCorrupt,
			classic.ErrOutOfRange} {
			if got := errors.Is(each.err, kind); got != (kind == each.want) {
				t.Errorf("%s: %v found as %v is %t", each.what, each.err, kind,
					got)
			}
		}
	}
}

// openError is the error of opening bytes.
func openError(raw []byte) error {
	_, err := antiOpen(raw)
	return err
}

// slabError is the error of a slab past the end of a variable.
func slabError(t *testing.T) error {
	file, err := antiOpen(antiLoneFile(cdfOne, wireInt))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got := make([]int32, 2)
	return antiVariable(t, file, "v").ReadSlice(&got, []int64{1}, []int64{2})
}

// dataError is the error of reading a variable whose data the file does
// not hold.
func dataError(t *testing.T) error {
	file, err := antiOpen(fixedVariable(cdfOne, wireInt, 2).Bytes())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	_, err = readHis(antiVariable(t, file, "v"), 1<<20)
	return err
}

// The test at type_internal_test.go:68 asserts a codec for each of the
// eleven types and none for 12. Its anti: reading each type into the
// slice of every other Go type is a type mismatch and into its own is
// not; type 11 is a variable in CDF-5 and 12 and 0 are refused.
func TestAntiCodecsMatchTypes(t *testing.T) {
	for typ := classic.TypeByte; typ <= classic.TypeUInt64; typ++ {
		file, err := antiOpen(antiLoneFile(cdfFive, uint32(typ)))
		if err != nil {
			t.Fatalf("%v: %v", typ, err)
		}
		variable := antiVariable(t, file, "v")
		for other, each := range goTypes {
			made := reflect.New(reflect.SliceOf(each))
			made.Elem().Set(reflect.MakeSlice(reflect.SliceOf(each), 2, 2))
			err := variable.Read(made.Interface())
			same := each == goTypes[typ]
			if same != (err == nil) || (!same && !errors.Is(err,
				classic.ErrTypeMismatch)) {
				t.Errorf("%v read into %v: %v", typ, other, err)
			}
		}
		file.Close()
	}
	for _, tag := range []uint32{0, 12} {
		if _, err := antiOpen(antiLoneFile(cdfFive, tag)); !errors.Is(err,
			classic.ErrCorrupt) {
			t.Errorf("type tag %d: %v, want ErrCorrupt", tag, err)
		}
	}
}

// codecCase is a type, the bytes a file holds for two values of it, and
// the values.
type codecCase struct {
	typ  classic.Type
	raw  []byte
	want any
}

// codecCases are his, as the table of TestCodecDecode.
var codecCases = []codecCase{
	{classic.TypeByte, []byte{0x80, 0x7f}, []int8{-128, 127}},
	{classic.TypeChar, []byte("ab"), []byte("ab")},
	{classic.TypeShort, []byte{0x80, 0x01, 0x7f, 0xff}, []int16{-32767,
		32767}},
	{classic.TypeInt, []byte{0xff, 0xff, 0xff, 0xff, 0, 0, 0, 1}, []int32{-1,
		1}},
	{classic.TypeFloat, []byte{0xbf, 0xc0, 0, 0, 0x40, 0x10, 0, 0},
		[]float32{-1.5, 2.25}},
	{classic.TypeDouble, []byte{0xbf, 0xf8, 0, 0, 0, 0, 0, 0, 0x40, 0x02, 0, 0,
		0, 0, 0, 0}, []float64{-1.5, 2.25}},
	{classic.TypeUByte, []byte{0, 0xff}, []byte{0, 255}},
	{classic.TypeUShort, []byte{0, 1, 0xff, 0xff}, []uint16{1, 65535}},
	{classic.TypeUInt, []byte{0, 0, 0, 1, 0xff, 0xff, 0xff, 0xff},
		[]uint32{1, 4294967295}},
	{classic.TypeInt64, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0, 0, 0, 0, 0, 0, 0, 1}, []int64{-1, 1}},
	{classic.TypeUInt64, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff}, []uint64{0, 18446744073709551615}},
}

// reversedEach is bytes with each element of a width reversed, the byte
// order of each value made wrong; for one byte, the low bit flipped.
func reversedEach(raw []byte, width int) []byte {
	found := slices.Clone(raw)
	for at := 0; at+width <= len(found); at += width {
		slices.Reverse(found[at : at+width])
	}
	if width == 1 {
		found[0] ^= 1
	}
	if slices.Equal(found, raw) {
		// Every element reads the same either way round (his uint64
		// case: all zeros, then all ones), so swap the two elements.
		copy(found, raw[width:2*width])
		copy(found[width:], raw[:width])
	}
	return found
}

// The test at type_internal_test.go:85 decodes each codec from hand
// written bytes, and into the middle of a slice. Its anti: those bytes
// in a file read as those values, the bytes with the byte order of each
// value reversed (one bit for the one-byte types) read as something else,
// and a slab that starts in the middle of a row, which decodes its runs
// one after another into one slice, reads the elements it names and not
// those of the slab one column to the left.
func TestAntiCodecDecode(t *testing.T) {
	for _, each := range codecCases {
		width := wireWidth(uint32(each.typ))
		count := len(each.raw) / width
		head := fixedVariable(cdfFive, uint32(each.typ), uint64(count))
		padding := make([]byte, 8)
		file, err := antiOpen(antiFile(head, each.raw, padding))
		if err != nil {
			t.Fatalf("%v: %v", each.typ, err)
		}
		got, err := readHis(antiVariable(t, file, "v"), 1<<20)
		file.Close()
		if err != nil || !slices.Equal(bitsOf(got), bitsOf(each.want)) {
			t.Errorf("%v reads %v, %v, want %v", each.typ, got, err, each.want)
		}
		wrong, err := antiOpen(antiFile(head, reversedEach(each.raw, width),
			padding))
		if err != nil {
			t.Fatalf("%v: %v", each.typ, err)
		}
		got, err = readHis(antiVariable(t, wrong, "v"), 1<<20)
		wrong.Close()
		if err != nil || slices.Equal(bitsOf(got), bitsOf(each.want)) {
			t.Errorf("%v with the byte order reversed reads %v, %v: not seen",
				each.typ, got, err)
		}
	}
	antiMiddleOfARow(t)
}

// antiMiddleOfARow reads two slabs of a 2 by 3 variable of ints, one
// column apart, which must differ and be the elements named.
func antiMiddleOfARow(t *testing.T) {
	t.Helper()
	data := encodeValues(classic.TypeInt, []int32{0, 1, 2, 3, 4, 5})
	file, err := antiOpen(antiFile(fixedVariable(cdfOne, wireInt, 2, 3), data,
		make([]byte, 8)))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	variable := antiVariable(t, file, "v")
	for start, want := range map[int64][]int32{0: {0, 1, 3, 4},
		1: {1, 2, 4, 5}} {
		got := make([]int32, 4)
		err := variable.ReadSlice(&got, []int64{0, start}, []int64{2, 2})
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("slab from column %d reads %v, %v, want %v", start, got,
				err, want)
		}
	}
}
