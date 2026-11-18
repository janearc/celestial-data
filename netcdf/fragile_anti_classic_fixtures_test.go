//go:build fragile

package netcdf_test

// The anti-tests of classic_fixtures_test.go, one for each of its tests
// (his file, by line of the test). Each changes one thing the test
// depends on, in a fixture or a file of the grammar, and asks that the
// reader now says something else; or, for a refusal, gives the nearest
// right input and asks that it is accepted.

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	classic "github.com/scigolib/netcdf"
)

// antiRecords is the mutation that sets the record count of a fixture.
func antiRecords(count uint64) antiMutation {
	return antiMutation{what: "the record count", change: func(t *testing.T,
		raw []byte) []byte {
		return antiSetAt(raw, 4, antiSize(int(raw[3]), count)...)
	}}
}

// antiFirstData is the mutation that flips the first byte of the data.
func antiFirstData() antiMutation {
	return antiMutation{what: "the first byte of the data",
		change: func(t *testing.T, raw []byte) []byte {
			at := headerEnd(raw)
			return antiSetAt(raw, at, raw[at]^0xff)
		}}
}

// antiText is the mutation that changes a run of bytes into another.
func antiText(what, old, replacement string) antiMutation {
	return antiMutation{what: what, change: func(t *testing.T,
		raw []byte) []byte {
		return antiPatch(t, raw, []byte(old), []byte(replacement))
	}}
}

// antiHex is the mutation that changes a run of bytes into another, both
// given as the bytes.
func antiHex(what string, old, replacement []byte) antiMutation {
	return antiMutation{what: what, change: func(t *testing.T,
		raw []byte) []byte {
		return antiPatch(t, raw, old, replacement)
	}}
}

// The test at classic_fixtures_test.go:93 reads the three simple files
// and compares their metadata and values. Each is made wrong in one way:
// a byte of a's data, the length of x, a letter of the title, the version
// byte. The reader must say something else, or refuse.
func TestAntiSimpleAllFormats(t *testing.T) {
	for _, each := range []struct {
		name    string
		version int
	}{{"simple-cdf1.nc", cdfOne}, {"simple-cdf2.nc", cdfTwo},
		{"simple-cdf5.nc", cdfFive}} {
		x := append(antiSize(each.version, 1), 'x', 0, 0, 0)
		longer := append(bytes.Clone(x), antiSize(each.version, 5)...)
		x = append(x, antiSize(each.version, 4)...)
		antiRun(t, []string{each.name}, []antiMutation{antiFirstData(),
			antiHex("the length of x, 4 to 5", x, longer),
			antiText("a letter of the title", "simple fixture",
				"simple fixtura"),
			{what: "the version byte", change: func(t *testing.T,
				raw []byte) []byte {
				return antiSetAt(raw, 3, byte(each.version%cdfFive+1))
			}}})
	}
}

// The test at classic_fixtures_test.go:158 reads every type from the two
// types files. Two values are made wrong: the byte order of one short, and
// of one float; and a letter of the char variable.
func TestAntiTypesAllTypes(t *testing.T) {
	antiRun(t, []string{"types-cdf1.nc", "types-cdf2.nc"}, []antiMutation{
		antiHex("the byte order of the short -32768",
			[]byte{0x80, 0x00, 0xff, 0xff}, []byte{0x00, 0x80, 0xff, 0xff}),
		antiHex("the byte order of the float -1.5",
			[]byte{0xbf, 0xc0, 0x00, 0x00}, []byte{0x00, 0x00, 0xc0, 0xbf}),
		antiText("a letter of hello", "hello", "hellp")})
}

// The test at classic_fixtures_test.go:172 reads the record files and
// slabs across the interleave. The record count is made one less and one
// more, and the byte order of two values of temp is made wrong.
func TestAntiRecordLayout(t *testing.T) {
	antiRun(t, []string{"record-cdf1.nc", "record-cdf2.nc", "record-cdf5.nc"},
		[]antiMutation{antiRecords(2), antiRecords(4),
			antiHex("the byte order of temp's first two values",
				[]byte{0, 0, 0, 10, 0, 0, 0, 11},
				[]byte{10, 0, 0, 0, 11, 0, 0, 0})})
}

// The test at classic_fixtures_test.go:219 reads the file of one record
// variable of bytes, five records. The count is made four and a value of
// b is made wrong.
func TestAntiSingleRecordVariable(t *testing.T) {
	antiRun(t, []string{"single_rec-cdf1.nc"}, []antiMutation{antiRecords(4),
		{what: "the value 42", change: func(t *testing.T, raw []byte) []byte {
			return antiPatch(t, raw, []byte{0x2a}, []byte{0x2b})
		}}})
}

// The test at classic_fixtures_test.go:234 reads the empty file and
// asserts that nothing is in it. Its anti makes a file that is not empty
// in one way each: another record count and another version, which the
// test reads as the same; a dimension, an attribute and a variable, which
// it must see.
func TestAntiEmptyFile(t *testing.T) {
	raw := antiFixture(t, "empty-cdf1.nc")
	for _, each := range []antiMutation{antiRecords(7), {what: "the version",
		change: func(t *testing.T, raw []byte) []byte {
			return antiSetAt(raw, 3, cdfTwo)
		}}} {
		antiDiffers(t, each.what, raw, each.change(t, raw))
	}
	full := goodHeader(cdfOne).File()
	file, err := antiOpen(full)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if len(file.Dimensions()) != 1 || len(file.Attributes()) != 1 ||
		len(file.Variables()) != 1 {
		t.Errorf("a file of one of each reads as %d, %d, %d",
			len(file.Dimensions()), len(file.Attributes()),
			len(file.Variables()))
	}
}

// The test at classic_fixtures_test.go:244 reads the attribute types
// file. A letter of the units, the byte order of one of the valid_range
// ints, and of one of the reals, are made wrong.
func TestAntiAttributeTypes(t *testing.T) {
	antiRun(t, []string{"attrs-cdf1.nc"}, []antiMutation{
		antiText("a letter of the units", "count", "cound"),
		antiHex("the byte order of 100", []byte{0, 0, 0, 100},
			[]byte{100, 0, 0, 0}),
		antiHex("the byte order of 1.5", []byte{0x3f, 0xf8, 0, 0, 0, 0, 0, 0},
			[]byte{0, 0, 0, 0, 0, 0, 0xf8, 0x3f})})
}

// The test at classic_fixtures_test.go:255 asserts that reads that are
// wrong are refused: a destination of another type, a short one, a
// selection past the edge, a negative start, nil and an empty slice. Its
// anti gives the nearest read that is right in each and asserts it is
// accepted and reads the values.
func TestAntiReadErrors(t *testing.T) {
	file, err := antiOpen(antiFixture(t, "simple-cdf1.nc"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	a := antiVariable(t, file, "a")
	for _, each := range []struct {
		what         string
		start, count []int64
		want         []float32
	}{{"the whole", []int64{0, 0}, []int64{4, 3}, []float32{0, 1, 2, 3, 4, 5,
		6, 7, 8, 9, 10, 11}},
		{"the last row, to the edge", []int64{3, 0}, []int64{1, 3},
			[]float32{9, 10, 11}},
		{"the first row, from zero", []int64{0, 0}, []int64{1, 3},
			[]float32{0, 1, 2}},
		{"nothing, to a destination that holds nothing", []int64{0, 0},
			[]int64{0, 3}, []float32{}}} {
		got := make([]float32, len(each.want))
		if err := a.ReadSlice(&got, each.start, each.count); err != nil {
			t.Errorf("%s: refused: %v", each.what, err)
			continue
		}
		for at := range each.want {
			if got[at] != each.want[at] {
				t.Errorf("%s: value %d is %v, want %v", each.what, at, got[at],
					each.want[at])
			}
		}
	}
}

// The test at classic_fixtures_test.go:284 asserts that a read after
// Close gives the closed error and a second Close is nothing. Its anti: a
// read before Close is not refused; closing one file does not close
// another; and closing a file made over a reader of the caller's leaves
// that reader open, as the comment on NewReader says.
func TestAntiClosedFile(t *testing.T) {
	raw := antiFixture(t, "simple-cdf1.nc")
	first, err := antiOpen(raw)
	if err != nil {
		t.Fatal(err)
	}
	second, err := antiOpen(raw)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]float32, 12)
	if err := antiVariable(t, first, "a").Read(&out); err != nil {
		t.Errorf("a read before Close: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := antiVariable(t, second, "a").Read(&out); err != nil {
		t.Errorf("a read of a file that was not closed: %v", err)
	}
	antiCallerKeepsReader(t, raw)
}

// antiCallerKeepsReader closes a file made over an *os.File and reads
// the *os.File.
func antiCallerKeepsReader(t *testing.T, raw []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "caller.nc")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	held, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	file, err := classic.NewReader(held, int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := held.ReadAt(make([]byte, 4), 0); err != nil &&
		!errors.Is(err, io.EOF) {
		t.Errorf("the caller's reader was closed by Close: %v", err)
	}
}

// The test at classic_fixtures_test.go:302 reads the corpus file whose
// padding is not NUL. A byte of its data is made wrong: it must not read
// as before.
func TestAntiCorpusNullBytePadding(t *testing.T) {
	antiRun(t, []string{"null_byte_padding-cdf2.nc"},
		[]antiMutation{antiFirstData()})
}

// The test at classic_fixtures_test.go:382 asserts that the corpus file
// with begins that break the section order is refused as corrupt. Its
// anti is the same file of the grammar made two ways: with its two begins
// in order, accepted; with the two begins swapped, refused as corrupt.
func TestAntiCorpusBadCDF5Begin(t *testing.T) {
	head := emptyHeader(cdfFive)
	head.Dims = []dimSpec{{Name: "x", Length: 2}}
	head.Vars = []varSpec{{Name: "a", DimIDs: []uint64{0}, Type: wireInt},
		{Name: "b", DimIDs: []uint64{0}, Type: wireInt}}
	honest := head.place(uint64(len(head.Bytes())))
	antiAccepted(t, "two begins in order", head.File())
	head.Vars[0].Begin = some(honest.begins[1])
	head.Vars[1].Begin = some(honest.begins[0])
	_, err := antiOpen(head.File())
	if !errors.Is(err, classic.ErrCorrupt) {
		t.Errorf("two begins swapped: %v, want ErrCorrupt", err)
	}
}

// The test at classic_fixtures_test.go:396 reads the layout files and
// every slab of two variables against the full read. The record count is
// made one less, the first byte of the cube's data and the byte order of
// two values of temp are made wrong.
func TestAntiLayoutFixtures(t *testing.T) {
	antiRun(t, []string{"layout-cdf1.nc", "layout-cdf2.nc", "layout-cdf5.nc"},
		[]antiMutation{antiRecords(4), antiFirstData(),
			antiHex("the byte order of temp's first two values",
				[]byte{0, 0, 0, 10, 0, 0, 0, 11},
				[]byte{10, 0, 0, 0, 11, 0, 0, 0})})
}

// The test at classic_fixtures_test.go:509 asserts that an HDF5 file is
// unsupported, junk is not netCDF and a cut classic header is corrupt.
// Its anti: a whole classic header is accepted and is not called
// unsupported; the HDF5 signature with one byte changed, and with a byte
// cut off, is not netCDF and not unsupported.
func TestAntiFormatDispatch(t *testing.T) {
	whole := emptyHeader(cdfOne).Bytes()
	file, err := antiOpen(whole)
	if err != nil {
		t.Errorf("a whole classic header is refused: %v", err)
	} else {
		file.Close()
	}
	signature := []byte{0x89, 'H', 'D', 'F', '\r', '\n', 0x1a, '\n'}
	for what, raw := range map[string][]byte{
		"the signature with one byte changed": antiSetAt(signature, 1, 'h'),
		"the signature cut by a byte":         signature[:len(signature)-1]} {
		_, err := antiOpen(append(bytes.Clone(raw), 0, 0, 0, 0))
		if errors.Is(err, classic.ErrUnsupportedFormat) ||
			!errors.Is(err, classic.ErrNotNetCDF) {
			t.Errorf("%s: %v, want ErrNotNetCDF and not unsupported", what,
				err)
		}
	}
}
