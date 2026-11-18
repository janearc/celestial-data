//go:build fragile

package netcdf_test

// The anti-tests of internal/cdf/header_write_extra_test.go. Of its ten
// tests two have no anti written: TestEncodeErrors, whose refusals need an
// impossible header or a begin past 2 GiB, neither of which the public
// calls can make without a file of that size; the others are reached
// through the files the writer makes, or the bytes the reader is given.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	classic "github.com/scigolib/netcdf"

	"github.com/janearc/celestial-data/netcdf"
)

// The test at header_write_extra_test.go:40 asserts that every fixture's
// header passes the writer's rules. Its anti: a model of the same kind with
// one defect is refused as an invalid schema: the name of a variable twice,
// a slash in a dimension's name, a type of CDF-5 in CDF-1.
func TestAntiValidateAcceptsFixtures(t *testing.T) {
	for what, defect := range map[string]func(*fragModel){
		"a variable twice": func(m *fragModel) { m.Vars[1].Name = m.Vars[0].Name },
		"a slash in a dimension": func(m *fragModel) {
			m.Dims[1].Name = "x/y"
		},
		"a type of CDF-5 in CDF-1": func(m *fragModel) {
			m.Vars[1].Type = netcdf.TypeUByte
			*m = m.reramped()
		}} {
		model := antiModel(netcdf.FormatClassic)
		defect(&model)
		err := model.save(filepath.Join(t.TempDir(), "defect.nc"))
		if !errors.Is(err, netcdf.ErrInvalidSchema) {
			t.Errorf("%s: %v, want an invalid schema", what, err)
		}
	}
	if err := antiModel(netcdf.FormatClassic).save(filepath.Join(t.TempDir(),
		"fine.nc")); err != nil {
		t.Errorf("the model with no defect: %v", err)
	}
}

// The test at header_write_extra_test.go:53 asserts that fifteen schemas
// are refused. Its anti is the pairs of the writer's validation test, all
// of them: each wrong schema refused as its kind, and the same schema
// right in the one thing accepted, created and opened. (A duplicate
// attribute and a partial value cannot be made with the public calls, which
// replace the first and take whole values.)
func TestAntiValidateRejects(t *testing.T) {
	antiRefusedAndAccepted(t, false)
}

// The test at header_write_extra_test.go:116 asserts that four headers are
// refused by the layout, among them two whose variables are too large for
// the offset. Its anti: two byte variables of a thousand, in CDF-1 and CDF-2,
// are accepted, and the begin of the second is what the grammar says. The
// refusals themselves need a layout of 2 GiB and are not asked for.
func TestAntiLayoutErrors(t *testing.T) {
	for _, format := range []netcdf.Format{netcdf.FormatClassic,
		netcdf.FormatClassic64} {
		model := fragModel{Format: format, Dims: []modelDim{{"x", 1000}},
			Vars: []modelVar{{Name: "a", Type: netcdf.TypeByte, Dims: []int{0}},
				{Name: "b", Type: netcdf.TypeByte, Dims: []int{0}}}}.reramped()
		mine, raw := model.header().Bytes(), fileOf(t, model)
		if !bytes.HasPrefix(raw, mine) {
			t.Errorf("%s: %s", netcdf.FormatName(format), byteDifference(mine,
				raw))
		}
	}
}

// unwritten is the bytes of a file made of a model and closed with nothing
// written to it.
func unwritten(t *testing.T, model fragModel) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "unwritten.nc")
	writer, err := model.create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The test at header_write_extra_test.go:155 asserts that a fixed variable
// is filled for its padded size and a record for its slab. Its anti: fixed
// variables of 1 to 6 values of four types are filled for exactly the
// padded size the grammar says, and one record variable written at record
// two leaves three records of the size the grammar says.
func TestAntiFillFixedAndRecords(t *testing.T) {
	for _, typ := range []netcdf.Type{netcdf.TypeByte, netcdf.TypeShort,
		netcdf.TypeInt, netcdf.TypeDouble} {
		for length := int64(1); length <= 6; length++ {
			model := fragModel{Format: netcdf.FormatClassic64,
				Dims: []modelDim{{"x", length}},
				Vars: []modelVar{{Name: "v", Type: typ, Dims: []int{0}}}}
			model = model.reramped()
			head := model.header()
			want := head.place(uint64(len(head.Bytes()))).end
			if got := len(unwritten(t, model)); uint64(got) != want {
				t.Errorf("%v of %d: the file is %d bytes, the grammar says %d",
					typ, length, got, want)
			}
		}
	}
	model := fragModel{Format: netcdf.FormatClassic, Records: 3,
		Dims: []modelDim{{"t", 0}, {"x", 2}},
		Vars: []modelVar{{Name: "r", Type: netcdf.TypeShort, Dims: []int{0, 1}},
			{Name: "c", Type: netcdf.TypeByte, Dims: []int{0}}}}.reramped()
	raw := writtenAt(t, model, "r", 2, []int16{1, 2})
	head := model.header()
	if want := head.place(uint64(len(head.Bytes()))).end; uint64(len(raw)) !=
		want {
		t.Errorf("three records are %d bytes, the grammar says %d", len(raw),
			want)
	}
}

// writtenAt is the bytes of a file of a model after one record of one
// variable of two inner values was written.
func writtenAt(t *testing.T, model fragModel, name string, record int64,
	values any) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "written.nc")
	writer, err := model.create(path)
	if err != nil {
		t.Fatal(err)
	}
	target, err := writer.Variable(name)
	if err != nil {
		t.Fatal(err)
	}
	slab := netcdf.Slab{Start: []int64{record, 0}, Count: []int64{1, 2}}
	if err := target.WriteSlab(values, slab); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The test at header_write_extra_test.go:189 asserts that a lone record
// variable is filled with no padding between records. Its anti: a lone
// record variable of bytes written at record two is three bytes of records,
// the first two the fill value; with a second record variable of bytes the
// same write is three records of eight bytes, the padding the specification
// asks for.
func TestAntiFillRecordsSingleVariable(t *testing.T) {
	lone := fragModel{Format: netcdf.FormatClassic, Records: 3,
		Dims: []modelDim{{"t", 0}},
		Vars: []modelVar{{Name: "b", Type: netcdf.TypeByte, Dims: []int{0}}}}.
		reramped()
	head := lone.header()
	begin := uint64(len(head.Bytes()))
	raw := antiRecordWrite(t, lone, "b", 2)
	if uint64(len(raw)) != begin+3 || !bytes.Equal(raw[begin:], []byte{0x81,
		0x81, 1}) {
		t.Errorf("a lone record variable: % x after a header of %d", raw,
			begin)
	}
	pair := lone
	pair.Vars = []modelVar{lone.Vars[0], {Name: "c", Type: netcdf.TypeByte,
		Dims: []int{0}}}
	pair = pair.reramped()
	head = pair.header()
	raw = antiRecordWrite(t, pair, "b", 2)
	if want := head.place(uint64(len(head.Bytes()))).end; uint64(len(raw)) !=
		want {
		t.Errorf("two record variables are %d bytes, the grammar says %d",
			len(raw), want)
	}
}

// antiRecordWrite is the file after one byte is written at a record of a
// byte variable over the unlimited dimension.
func antiRecordWrite(t *testing.T, model fragModel, name string,
	record int64) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "record.nc")
	writer, err := model.create(path)
	if err != nil {
		t.Fatal(err)
	}
	target, err := writer.Variable(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.WriteSlab([]int8{1}, netcdf.Slab{Start: []int64{record},
		Count: []int64{1}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The test at header_write_extra_test.go:208 asserts that a record count of
// 2^32-1 is refused as out of range in CDF-1, with the count unchanged. Its
// anti: the same write in CDF-2, which has the same field, is refused the
// same way; and a write at record 1000 is accepted in both and the count is
// 1001.
func TestAntiExtendRecsLimits(t *testing.T) {
	for _, format := range []classic.Format{classic.FormatClassic,
		classic.FormatClassic64} {
		for _, each := range []struct {
			record  int64
			refused bool
		}{{1<<32 - 1, true}, {1000, false}} {
			s := classic.NewSchema(format)
			s.AddVariable("b", classic.TypeByte, s.AddDimension("t", 0))
			antiSaved(t, s, func(w *classic.Writer) {
				err := antiWriter(t, w, "b").WriteSlice([]int8{1},
					[]int64{each.record}, []int64{1})
				want := each.record + 1
				if each.refused {
					want = 0
				}
				if (err != nil) != each.refused || w.NumRecs() != want ||
					(each.refused && !errors.Is(err, classic.ErrOutOfRange)) {
					t.Errorf("%v record %d: %v, %d records, want %d", format,
						each.record, err, w.NumRecs(), want)
				}
			})
		}
	}
}

// The test at header_write_extra_test.go:249 asserts that the record count
// lands where the parser reads it, 4 bytes wide, or 8 in CDF-5. Its anti:
// three records and four records written make files that first differ at
// the last byte of that field, byte 7, or byte 11 in CDF-5, and the field
// holds the count.
func TestAntiWriteNumRecs(t *testing.T) {
	for _, format := range formatsOf {
		was, now := antiModel(format), antiModel(format)
		now.Records = 3
		now = now.reramped()
		first, second := fileOf(t, was), fileOf(t, now)
		last := 7
		if format == netcdf.FormatClassic64Data {
			last = 11
		}
		if at := firstDifferent(first, second); at != last || first[last] != 2 ||
			second[last] != 3 {
			t.Errorf("%s: two and three records first differ at %d, want %d "+
				"holding 2 and 3", netcdf.FormatName(format), at, last)
		}
	}
}

// The test at header_write_extra_test.go:276 asserts that a variable of
// more than 2^32-4 bytes has the vsize 2^32-1 written, and that the parser
// ignores vsize. The writing needs a file of 4 GiB. Its anti is the
// parser's half: a file whose vsize is a lie, zero, one, the sentinel, a
// number far too large, reads as the file with the honest one.
func TestAntiWriteHeaderVsizeSentinel(t *testing.T) {
	for _, version := range antiVersions {
		honest := goodHeader(version)
		for _, lie := range []uint64{0, 1, 1<<32 - 1, 12345} {
			lied := goodHeader(version)
			lied.Vars[0].VSize = some(lie)
			antiSame(t, "a vsize that lies", honest.File(), lied.File())
		}
	}
}

// The test at header_write_extra_test.go:310 asserts that 2^32-2 is a record
// count the format stores and 2^32-1 is the marker that is not one. Its
// anti, from the reader's side: a header that says 2^32-2 records reads as
// that many, and one that says 2^32-1 reads as the streaming marker, which
// is the count the data gives, here none. The writing of 2^32-2 records is
// not asked for.
func TestAntiRecordCountLimits(t *testing.T) {
	for _, version := range []int{cdfOne, cdfTwo} {
		head := recordVariables(version, 1, 2, wireByte)
		head.NumRecs = some(uint64(1<<32 - 2))
		counted := head.Bytes()
		head.NumRecs = some(uint64(streamingMarker))
		streaming := head.Bytes()
		antiDiffers(t, "the marker, not a count", counted, streaming)
		if file, err := antiOpen(counted); err != nil ||
			file.NumRecs() != 1<<32-2 {
			t.Errorf("CDF-%d: 2^32-2 records: %v", version, err)
		}
		if file, err := antiOpen(streaming); err != nil || file.NumRecs() != 0 {
			t.Errorf("CDF-%d: the marker: %v", version, err)
		}
	}
}
