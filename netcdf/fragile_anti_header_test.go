//go:build fragile

package netcdf_test

// The anti-tests of internal/cdf/header_test.go. That file is in his
// internal package, which cannot be imported here; what it checks of the
// header and the geometry of a slab is reached through bytes made by the
// harness's encoder and the public reads. Where a test asserts an
// internal field (HeaderSize, RecSize, a run), its anti asserts what the
// field decides: where the data is read from.

import (
	"errors"
	"slices"
	"testing"

	classic "github.com/scigolib/netcdf"
)

// ints is values as the file stores 32-bit integers.
func ints(values ...int32) []byte { return encodeValues(classic.TypeInt, values) }

// antiValuesOf is the values of a variable of bytes, or the error.
func antiValuesOf(t *testing.T, raw []byte, name string) (any, error) {
	t.Helper()
	file, err := antiOpen(raw)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readHis(antiVariable(t, file, name), int64(len(raw)))
}

// The test at header_test.go:290 asserts the size of each type and that
// a type of CDF-5 is not valid in the others. Its anti: the six types of
// every version are accepted and read in all three; the five of CDF-5 are
// accepted in CDF-5 and refused in the other two, and 0 and 12 in all.
func TestAntiTypeSize(t *testing.T) {
	for tag := wireByte; tag <= wireUInt64; tag++ {
		for _, version := range antiVersions {
			_, err := antiOpen(antiLoneFile(version, tag))
			want := tag <= wireDouble || version == cdfFive
			if (err == nil) != want {
				t.Errorf("type %d in CDF-%d: %v, accepted should be %t", tag,
					version, err, want)
			}
			if err != nil && !errors.Is(err, classic.ErrCorrupt) {
				t.Errorf("type %d in CDF-%d: refused as %v", tag, version, err)
			}
		}
	}
	for _, tag := range []uint32{0, 12} {
		if _, err := antiOpen(antiLoneFile(cdfFive, tag)); err == nil {
			t.Errorf("type %d is accepted in CDF-5", tag)
		}
	}
}

// The test at header_test.go:312 reads the published empty file and asserts
// no content, version 1, 32 bytes. Its anti changes the version byte, the
// record count and the dimension list in turn: each is seen.
func TestAntiReadHeaderSpecEmpty(t *testing.T) {
	raw := emptyHeader(cdfOne).Bytes()
	antiDiffers(t, "the version", raw, antiSetAt(raw, 3, cdfTwo))
	antiDiffers(t, "the record count", raw, antiSetAt(raw, 7, 7))
	antiDiffers(t, "a dimension", raw, goodHeader(cdfOne).File())
}

// specTiny is the published file of the specification: a dimension of
// five, a variable of shorts over it, and 3, 1, 4, 1, 5 and a fill value.
// It is made by the harness's encoder and is 92 bytes: the header is 80,
// with the begin 80 and the vsize 12, as the specification prints it.
var specTiny = antiFile(header{Version: cdfOne,
	Dims: []dimSpec{{Name: "dim", Length: 5}},
	Vars: []varSpec{{Name: "vx", DimIDs: []uint64{0}, Type: wireShort}}},
	encodeValues(classic.TypeShort, []int16{3, 1, 4, 1, 5, -32767}))

// The test at header_test.go:328 reads the published tiny file and asserts
// the dimension, the variable, the begin 80 and its values. Its anti
// changes one thing each: the dimension's size, the variable's type, its
// begin, and a byte of a value. Each is seen.
func TestAntiReadHeaderSpecTiny(t *testing.T) {
	antiDiffers(t, "the size of dim", specTiny, antiSetAt(specTiny, 27, 6))
	antiDiffers(t, "the type of vx", specTiny, antiSetAt(specTiny, 71, 4))
	antiDiffers(t, "the begin of vx", specTiny, antiSetAt(specTiny, 79, 84))
	antiDiffers(t, "a value of vx", specTiny, antiSetAt(specTiny, 81, 1))
}

// antiCDF2 is the file of the CDF-2 test: a text attribute and four ints.
func antiCDF2() header {
	head := emptyHeader(cdfTwo)
	head.Dims = []dimSpec{{Name: "d", Length: 4}}
	head.Attrs = []attrSpec{{Name: "title", Type: wireChar,
		Values: []byte("hi!\x00")}}
	head.Vars = []varSpec{{Name: "v", DimIDs: []uint64{0}, Type: wireInt}}
	return head
}

// The test at header_test.go:362 reads a CDF-2 file and asserts that begin
// is the header size and the four values 1 to 4. Its anti: the begin moved
// four bytes, a letter of the title, and the byte order of a value.
func TestAntiReadHeaderCDF2(t *testing.T) {
	data := ints(1, 2, 3, 4)
	honest := antiFile(antiCDF2(), data, make([]byte, 8))
	moved := antiCDF2()
	moved.Vars[0].BeginShift = 4
	antiDiffers(t, "the begin", honest, antiFile(moved, data, make([]byte, 8)))
	titled := antiCDF2()
	titled.Attrs[0].Values = []byte("hi?\x00")
	antiDiffers(t, "the title", honest, antiFile(titled, data, make([]byte,
		8)))
	antiDiffers(t, "the byte order", honest, antiFile(antiCDF2(),
		[]byte{1, 0, 0, 0}, ints(2, 3, 4), make([]byte, 8)))
}

// antiCDF5 is the file of the CDF-5 test: four variables, the third of
// three shorts, padded to eight, before one of two unsigned bytes.
func antiCDF5() header {
	head := emptyHeader(cdfFive)
	head.Dims = []dimSpec{{Name: "a", Length: 2}, {Name: "b", Length: 3}}
	for _, each := range []struct {
		name string
		dim  uint64
		tag  uint32
	}{{"i64", 0, wireInt64}, {"u64", 0, wireUInt64}, {"u16", 1, wireUShort},
		{"u8", 0, wireUByte}} {
		head.Vars = append(head.Vars, varSpec{Name: each.name,
			DimIDs: []uint64{each.dim}, Type: each.tag})
	}
	return head
}

// antiCDF5Data is the data of that file, with the two padding bytes.
func antiCDF5Data() []byte {
	return slices.Concat(encodeValues(classic.TypeInt64, []int64{1, 2}),
		encodeValues(classic.TypeInt64, []int64{3, 4}),
		[]byte{0, 5, 0, 6, 0, 7, 0, 0}, []byte{8, 9})
}

// The test at header_test.go:395 reads the CDF-5 file and asserts that u8
// begins after the padded u16. Its anti: u8's begin moved back two bytes,
// into the padding, and the length of b made two, are each seen.
func TestAntiReadHeaderCDF5(t *testing.T) {
	honest := antiFile(antiCDF5(), antiCDF5Data())
	moved := antiCDF5()
	moved.Vars[3].BeginShift = -2
	antiDiffers(t, "the begin of u8", honest, antiFile(moved, antiCDF5Data()))
	shorter := antiCDF5()
	shorter.Dims[1].Length = 2
	antiDiffers(t, "the length of b", honest, antiFile(shorter,
		antiCDF5Data()))
}

// antiRecordFile is the file of the record test: a fixed 2 by 3 double and
// two record variables, of shorts over x and of ints, two records.
func antiRecordFile() (header, []byte) {
	head := emptyHeader(cdfOne)
	head.Records = 2
	head.Dims = []dimSpec{{Name: "time"}, {Name: "x", Length: 2},
		{Name: "y", Length: 3}}
	head.Vars = []varSpec{{Name: "f", DimIDs: []uint64{1, 2}, Type: wireDouble},
		{Name: "r1", DimIDs: []uint64{0, 1}, Type: wireShort},
		{Name: "r2", DimIDs: []uint64{0}, Type: wireInt}}
	data := slices.Concat(encodeValues(classic.TypeDouble,
		[]float64{1, 2, 3, 4, 5, 6}),
		encodeValues(classic.TypeShort, []int16{10, 11}), ints(100),
		encodeValues(classic.TypeShort, []int16{12, 13}), ints(200))
	return head, data
}

// The test at header_test.go:457 reads two record variables and asserts
// the record size 8 and where each record lies. Its anti: one record fewer,
// the second variable's begin moved two bytes, and the first variable made
// ints, each seen.
func TestAntiRecordVariables(t *testing.T) {
	head, data := antiRecordFile()
	honest := antiFile(head, data)
	fewer := head
	fewer.Records = 1
	antiDiffers(t, "one record fewer", honest, antiFile(fewer, data))
	moved, _ := antiRecordFile()
	moved.Vars[2].BeginShift = 2
	antiDiffers(t, "the begin of r2", honest, antiFile(moved, data))
	wider, _ := antiRecordFile()
	wider.Vars[1].Type = wireInt
	antiDiffers(t, "the type of r1", honest, antiFile(wider, data))
}

// The test at header_test.go:511 reads one record variable of bytes with no
// padding between records. Its anti: the same variable in a file whose
// records are padded to four bytes reads its second record as 0, not as the
// next byte; code that padded a lone record variable would read both files
// the same.
func TestAntiSingleRecordVariableNoPadding(t *testing.T) {
	head := emptyHeader(cdfOne)
	head.Records = 3
	head.Dims = []dimSpec{{Name: "time"}}
	head.Vars = []varSpec{{Name: "c", DimIDs: []uint64{0}, Type: wireByte}}
	honest := antiFile(head, []byte("abc"))
	padded := antiFile(head, []byte("a\x00\x00\x00b\x00\x00\x00c\x00\x00\x00"))
	antiDiffers(t, "padded records", honest, padded)
	got, err := antiValuesOf(t, padded, "c")
	if err != nil || !slices.Equal(bitsOf(got), []uint64{'a', 0, 0}) {
		t.Errorf("the padded file reads %v, %v, want [97 0 0]", got, err)
	}
}

// The test at header_test.go:533 asserts that a record count that is the
// streaming marker is computed from the size of the file, for five bytes.
// Its anti: six bytes give six, so that the count is not a constant, and a
// count of 0xFFFFFFFE, which is a count, is that number.
func TestAntiStreamingNumRecs(t *testing.T) {
	head := emptyHeader(cdfOne)
	head.NumRecs = some(uint64(streamingMarker))
	head.Dims = []dimSpec{{Name: "time"}}
	head.Vars = []varSpec{{Name: "b", DimIDs: []uint64{0}, Type: wireByte}}
	five, six := antiFile(head, []byte{1, 2, 3, 4, 5}),
		antiFile(head, []byte{1, 2, 3, 4, 5, 6})
	antiDiffers(t, "a byte more", five, six)
	for raw, want := range map[string]int64{string(five): 5, string(six): 6} {
		if file, err := antiOpen([]byte(raw)); err != nil ||
			file.NumRecs() != want {
			t.Errorf("streaming over %d bytes: %v, want %d records", want, err,
				want)
		}
	}
	head.NumRecs = some(uint64(0xFFFFFFFE))
	file, err := antiOpen(antiFile(head, []byte{1, 2, 3, 4, 5}))
	if err != nil || file.NumRecs() != 0xFFFFFFFE {
		t.Errorf("a count of 0xFFFFFFFE: %v", err)
	}
}

// The test at header_test.go:546 asserts that every prefix shorter than
// the header is refused and every longer one is not. Its anti: the file
// with any one byte of the header taken out, which is as long as a prefix
// of it can be, is not the file it was, and does not open as the same.
func TestAntiTruncatedHeaderNeverPanics(t *testing.T) {
	head := emptyHeader(cdfOne)
	head.Records = 1
	head.Dims = []dimSpec{{Name: "d", Length: 2}}
	head.Attrs = []attrSpec{{Name: "units", Type: wireChar,
		Values: []byte("m\x00")}}
	head.Vars = []varSpec{{Name: "v", DimIDs: []uint64{0}, Type: wireInt}}
	good := antiFile(head, ints(7, 8))
	for at := range len(head.Bytes()) {
		antiDiffers(t, "a byte taken out", good, slices.Delete(slices.Clone(
			good), at, at+1))
	}
}

// antiHeaderPair is a header made two ways: wrong in one thing, and with
// that one thing right.
type antiHeaderPair struct {
	name string
	make func(bad bool) header
}

// ifBad is the wrong value for the wrong form of a pair, and the right
// value for the right form.
func ifBad[kind any](bad bool, wrong, right kind) kind {
	if bad {
		return wrong
	}
	return right
}

// antiBase is a header of one dimension and one variable, a record
// counted.
func antiBase() header {
	head := emptyHeader(cdfOne)
	head.Records = 1
	head.Dims = []dimSpec{{Name: "d", Length: 2}}
	head.Vars = []varSpec{{Name: "v", DimIDs: []uint64{0}, Type: wireInt}}
	return head
}

// antiHeaderPairs are the fifteen cases of the corrupt headers, each with
// its nearest right header.
func antiHeaderPairs() []antiHeaderPair {
	return []antiHeaderPair{
		{"bad magic", func(bad bool) header {
			head := antiBase()
			head.Magic = ifBad(bad, "XDF", "CDF")
			return head
		}},
		{"version 3", func(bad bool) header {
			head := antiBase()
			head.VersionByte = ifBad(bad, some(byte(3)), some(byte(1)))
			return head
		}},
		{"wrong dim tag", func(bad bool) header {
			head := antiBase()
			head.DimTag = ifBad(bad, some(uint32(99)), some(tagDimension))
			return head
		}},
		{"huge dim count", func(bad bool) header {
			head := antiBase()
			head.DimCount = ifBad(bad, some(uint64(1<<30)), some(uint64(1)))
			return head
		}},
		{"dimid out of range", func(bad bool) header {
			head := antiBase()
			head.Vars[0].DimIDs = []uint64{ifBad[uint64](bad, 9, 0)}
			return head
		}},
		{"unlimited dim not first", func(bad bool) header {
			head := emptyHeader(cdfOne)
			head.Records = 1
			head.Dims = []dimSpec{{Name: "a", Length: 2}, {Name: "z"}}
			head.Vars = []varSpec{{Name: "v", Type: wireInt,
				DimIDs: ifBad(bad, []uint64{0, 1}, []uint64{1, 0})}}
			return head
		}},
		{"two unlimited dims", func(bad bool) header {
			head := emptyHeader(cdfOne)
			head.Records = 1
			head.Dims = []dimSpec{{Name: "z1"}, {Name: "z2",
				Length: ifBad[uint64](bad, 0, 2)}}
			head.Vars = []varSpec{{Name: "a", DimIDs: []uint64{0}, Type: wireInt},
				{Name: "b", DimIDs: []uint64{1}, Type: wireInt}}
			return head
		}},
		{"CDF-5 type in CDF-1", func(bad bool) header {
			head := antiBase()
			head.Version = ifBad(bad, cdfOne, cdfFive)
			head.Vars[0].Type = wireUByte
			return head
		}},
		{"NC_STRING type", func(bad bool) header {
			head := antiBase()
			head.Vars[0].Type = ifBad(bad, uint32(12), wireInt)
			return head
		}},
		{"rank above limit", func(bad bool) header {
			head := antiBase()
			head.Vars[0].DimIDs = make([]uint64, ifBad(bad, 1025, 1))
			return head
		}},
		{"CDF-1 begin beyond the signed range", func(bad bool) header {
			head := antiBase()
			if bad {
				head.Vars[0].Begin = some(uint64(4294967295))
			}
			return head
		}},
		{"begin zero", func(bad bool) header {
			head := antiBase()
			if bad {
				head.Vars[0].Begin = some(uint64(0))
			}
			return head
		}},
		{"begin inside header", func(bad bool) header {
			head := antiBase()
			if bad {
				head.Vars[0].Begin = some(uint64(4))
			}
			return head
		}},
		{"fixed variables overlap", func(bad bool) header {
			head := antiBase()
			head.Vars = append(head.Vars, varSpec{Name: "w", DimIDs: []uint64{0},
				Type: wireInt})
			if bad {
				head.Vars[0].Begin = some(uint64(200))
				head.Vars[1].Begin = some(uint64(200))
			}
			return head
		}},
		{"record section before fixed data", func(bad bool) header {
			head := emptyHeader(cdfOne)
			head.Records = 1
			head.Dims = []dimSpec{{Name: "z"}, {Name: "d", Length: 2}}
			head.Vars = []varSpec{{Name: "f", DimIDs: []uint64{1}, Type: wireInt},
				{Name: "r", DimIDs: []uint64{0}, Type: wireInt}}
			if bad {
				head.Vars[0].Begin = some(uint64(300))
				head.Vars[1].Begin = some(uint64(304))
			}
			return head
		}},
	}
}

// The test at header_test.go:570 asserts that fifteen headers are refused as
// corrupt. Its anti: each is made right in the one thing that was wrong and
// is accepted, and each of the wrong ones is refused as the kind the test
// says; a reader that refused every header would pass the test.
func TestAntiCorruptHeaders(t *testing.T) {
	for _, each := range antiHeaderPairs() {
		t.Run(each.name, func(t *testing.T) {
			_, err := antiOpen(each.make(true).File())
			if !errors.Is(err, classic.ErrCorrupt) &&
				!errors.Is(err, classic.ErrNotNetCDF) {
				t.Errorf("the wrong header: %v", err)
			}
			antiAccepted(t, "the right header", each.make(false).File())
		})
	}
}

// antiRamp is the ints 0 to count minus one.
func antiRamp(count int) []int32 {
	found := make([]int32, count)
	for at := range found {
		found[at] = int32(at)
	}
	return found
}

// antiGrid is a file of a fixed variable "a" of ints over the lengths,
// with the values 0 up.
func antiGrid(lengths ...uint64) []byte {
	count := 1
	for _, each := range lengths {
		count *= int(each)
	}
	return antiFile(fixedVariable(cdfOne, wireInt, lengths...), ints(
		antiRamp(count)...), make([]byte, 8))
}

// antiSlabOf is the values of a slab of v.
func antiSlabOf(t *testing.T, raw []byte, start, count []int64) []int32 {
	t.Helper()
	file, err := antiOpen(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got := make([]int32, countOf(count))
	if err := antiVariable(t, file, "v").ReadSlice(&got, start,
		count); err != nil {
		t.Fatalf("slab %v %v: %v", start, count, err)
	}
	return got
}

// The test at header_test.go:673 asserts the runs of four slabs of a 4 by 3
// variable. Its anti reads them: rows 1 and 2 of two columns are 3, 4, 6, 7;
// the same slab one column over is another four values; and with the length
// of y made four, the slab is 4, 5, 8, 9.
func TestAntiWalkVaraFixed(t *testing.T) {
	raw := antiGrid(4, 3)
	for _, each := range []struct {
		raw          []byte
		start, count []int64
		want         []int32
	}{{raw, []int64{1, 0}, []int64{2, 2}, []int32{3, 4, 6, 7}},
		{raw, []int64{1, 1}, []int64{2, 2}, []int32{4, 5, 7, 8}},
		{antiGrid(4, 4), []int64{1, 0}, []int64{2, 2}, []int32{4, 5, 8, 9}},
		{raw, []int64{2, 0}, []int64{1, 3}, []int32{6, 7, 8}}} {
		if got := antiSlabOf(t, each.raw, each.start, each.count); !slices.
			Equal(got, each.want) {
			t.Errorf("slab %v %v reads %v, want %v", each.start, each.count, got,
				each.want)
		}
	}
}

// The test at header_test.go:719 asserts that fully selected outer
// dimensions with a partial innermost selection are not one run. Its anti
// reads those slabs: the values are those of the elements named, and not
// the contiguous run a merged walk would read; one column over they are
// others.
func TestAntiWalkVaraCoalescing(t *testing.T) {
	flat := antiRamp(48)
	raw := antiGrid(4, 3, 4)
	got := antiSlabOf(t, raw, []int64{1, 0, 2}, []int64{2, 3, 2})
	want := extractInts(flat, []int64{4, 3, 4}, []int64{1, 0, 2},
		[]int64{2, 3, 2})
	if !slices.Equal(got, want) || slices.Equal(got, flat[14:26]) {
		t.Errorf("3-D slab reads %v, want %v and not the run %v", got, want,
			flat[14:26])
	}
	other := antiSlabOf(t, raw, []int64{1, 0, 1}, []int64{2, 3, 2})
	if slices.Equal(other, got) {
		t.Errorf("the innermost start is not applied: %v", other)
	}
	two := antiSlabOf(t, antiGrid(4, 3), []int64{0, 1}, []int64{4, 2})
	if !slices.Equal(two, []int32{1, 2, 4, 5, 7, 8, 10, 11}) {
		t.Errorf("2-D slab reads %v", two)
	}
}

// extractInts is the values of a slab of a row-major array of ints.
func extractInts(flat []int32, shape, start, count []int64) []int32 {
	wide := make([]int64, len(flat))
	for at, each := range flat {
		wide[at] = int64(each)
	}
	found := make([]int32, 0)
	for _, each := range extract(shape, start, count, wide) {
		found = append(found, int32(each))
	}
	return found
}

// The test at header_test.go:773 asserts the number of dimensions after
// which a selection is contiguous, for seven selections of a 4 by 3 by 4
// variable, a function it calls and I cannot. Its anti reads those seven
// and asserts the values of the elements each names, which is what the
// number decides; and a record variable's slab from the second dimension.
func TestAntiContiguousFrom(t *testing.T) {
	flat := antiRamp(48)
	raw := antiGrid(4, 3, 4)
	for _, each := range [][2][]int64{
		{{0, 0, 0}, {4, 3, 4}}, {{0, 0, 0}, {4, 3, 1}}, {{0, 0, 2}, {4, 3, 2}},
		{{0, 0, 0}, {4, 2, 4}}, {{1, 0, 0}, {2, 3, 4}}} {
		got := antiSlabOf(t, raw, each[0], each[1])
		want := extractInts(flat, []int64{4, 3, 4}, each[0], each[1])
		if !slices.Equal(got, want) {
			t.Errorf("slab %v reads %v, want %v", each, got, want)
		}
	}
	head := fixedVariable(cdfOne, wireInt, 0, 3, 4)
	head.Records = 3
	record := antiFile(head, ints(antiRamp(36)...), make([]byte, 8))
	got := antiSlabOf(t, record, []int64{1, 0, 0}, []int64{1, 3, 4})
	if !slices.Equal(got, antiRamp(36)[12:24]) {
		t.Errorf("record 1 reads %v", got)
	}
}

// The test at header_test.go:795 asserts that six selections outside a
// variable are refused as out of range. Its anti: the selections that
// touch the edges, counts to the last element, a start at the length with
// nothing counted, are accepted.
func TestAntiWalkVaraBounds(t *testing.T) {
	file, err := antiOpen(antiGrid(4))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, each := range [][2][]int64{{{0}, {4}}, {{3}, {1}}, {{0}, {0}},
		{{4}, {0}}, {{0}, {3}}} {
		got := make([]int32, each[1][0])
		if err := antiVariable(t, file, "v").ReadSlice(&got, each[0],
			each[1]); err != nil {
			t.Errorf("start %v count %v: %v", each[0], each[1], err)
		}
	}
}
