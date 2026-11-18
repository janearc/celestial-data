//go:build fragile

package netcdf_test

// The anti-tests of internal/cdf/header_write_test.go. The encoder, the
// size, the layout and the fill table of that file are reached through the
// files his writer makes.

import (
	"bytes"
	"strings"
	"testing"

	classic "github.com/scigolib/netcdf"

	"github.com/janearc/celestial-data/netcdf"
)

// The test at header_write_test.go:52 asserts that re-encoding the header of
// each fixture gives the fixture's bytes. Its anti: a header that differs
// from a fixed one in one field of twelve, in each format, is written with
// the first byte that differs where the field lives.
func TestAntiEncodeHeaderByteIdentical(t *testing.T) {
	differAtTheSamePlace(t, true)
}

// The test at header_write_test.go:80 asserts that the size EncodeSize
// computes is the bytes WriteHeader writes. The size is what the layout
// takes the first begin from. Its anti: the first variable's data begins at
// the length of the header the grammar says, for a model, and with one
// attribute added it begins that many bytes later.
func TestAntiEncodeSizeMatchesBytes(t *testing.T) {
	for _, format := range formatsOf {
		was, now := antiModel(format), antiModel(format)
		now.Attrs = append(now.Attrs, modelAttr{"more", netcdf.TypeInt,
			[]int32{1}})
		wasBegin, nowBegin := headerEnd(fileOf(t, was)), headerEnd(fileOf(t, now))
		wantWas, wantNow := len(was.header().Bytes()), len(now.header().Bytes())
		if wasBegin != wantWas || nowBegin != wantNow || nowBegin <= wasBegin {
			t.Errorf("%s: data begins at %d and %d, the header is %d and %d",
				netcdf.FormatName(format), wasBegin, nowBegin, wantWas, wantNow)
		}
	}
}

// The test at header_write_test.go:102 asserts that the layout recomputes the
// begins, the record size and the header size of each fixture. Its anti:
// for each of twelve changes of a model, in each format, the whole header the
// writer makes, begins included, is the header the grammar makes of the
// changed model.
func TestAntiLayoutMatchesFixtures(t *testing.T) {
	for _, format := range formatsOf {
		for _, each := range antiChanges() {
			model := antiModel(format)
			each.apply(&model)
			mine, raw := model.header().Bytes(), fileOf(t, model)
			if len(raw) < len(mine) || !bytes.Equal(raw[:len(mine)], mine) {
				t.Errorf("%s, %s: the header differs: %s",
					netcdf.FormatName(format), each.what, byteDifference(mine,
						raw))
			}
		}
	}
}

// The test at header_write_test.go:145 pins the default fill values of each
// type to a table he wrote. Its anti: a variable of each type that is never
// written reads as the value in the specification's table, which the harness
// keeps, and a variable of a number that is no type is refused where the
// last one is not.
func TestAntiFillBytes(t *testing.T) {
	s := classic.NewSchema(classic.FormatClassic64Data)
	n := s.AddDimension("n", 2)
	for typ := classic.TypeByte; typ <= classic.TypeUInt64; typ++ {
		s.AddVariable("v"+typ.String(), typ, n)
	}
	file, err := antiOpen(antiSaved(t, s, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for typ := classic.TypeByte; typ <= classic.TypeUInt64; typ++ {
		got, err := readHis(antiVariable(t, file, "v"+typ.String()), 1<<20)
		if err != nil {
			t.Errorf("%v: %v", typ, err)
			continue
		}
		for _, each := range bitsOf(got) {
			if each != defaultFill[typ] {
				t.Errorf("%v: unwritten value %#x, the specification says %#x",
					typ, each, defaultFill[typ])
			}
		}
	}
	bad := classic.NewSchema(classic.FormatClassic64Data)
	bad.AddVariable("v", classic.Type(99), bad.AddDimension("n", 2))
	if _, err := classic.Create(t.TempDir()+"/bad.nc", bad); err == nil {
		t.Error("a variable of type 99 is created")
	}
}

// The test at header_write_test.go:178 asserts that valid names are
// accepted and invalid ones are refused. Its anti: each invalid name made
// valid by the one change nearest to it is accepted and read back as it was
// written, and each valid name made invalid by one change is refused.
func TestAntiCheckName(t *testing.T) {
	long := strings.Repeat("n", 256)
	cut := "имя"
	pairs := [][2]string{{"x", ""}, {"a_b", "a/b"}, {"lead", " lead"},
		{"trail", "trail "}, {"a b", "a\tb"}, {"a_b", "a\x00b"},
		{"ÿ", "\xff"}, {long, long + "n"}, {"temp", "te/mp"},
		{"Name with space", "Name with space "}, {cut, cut[:len(cut)-1]},
		{"a.b", "a.b\x00"}, {"x_1", " x_1"}}
	for _, pair := range pairs {
		right, wrong := pair[0], pair[1]
		antiNameRefused(t, wrong)
		s := classic.NewSchema(classic.FormatClassic)
		s.AddDimension(right, 2)
		file, err := antiOpen(antiSaved(t, s, nil))
		if err != nil {
			t.Errorf("%.20q: %v", right, err)
			continue
		}
		if dims := file.Dimensions(); len(dims) != 1 || dims[0].Name() != right {
			t.Errorf("%.20q reads back as %v", right, dims)
		}
		file.Close()
	}
}

// antiNameRefused fails the test unless a schema with a dimension of the
// name is refused.
func antiNameRefused(t *testing.T, name string) {
	t.Helper()
	s := classic.NewSchema(classic.FormatClassic)
	s.AddDimension(name, 2)
	if _, err := classic.Create(t.TempDir()+"/name.nc", s); err == nil {
		t.Errorf("the name %.20q is accepted", name)
	}
}
