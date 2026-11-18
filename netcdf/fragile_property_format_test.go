//go:build fragile

package netcdf_test

// Properties of the format: what the writer writes is read back, the
// harness's encoder and the writer make the same header, the encoder's
// file is read back as the model, and the values lie where the grammar
// puts them. Each is a rapid.Check over a random model, so a failure is
// shrunk to the smallest model that shows it.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/janearc/celestial-data/netcdf"
)

// scratch hands out the names of files in a temporary folder, for a
// property that makes many.
type scratch struct {
	directory string
	made      int
}

// newScratch is a scratch in the test's own temporary folder.
func newScratch(t *testing.T) *scratch {
	return &scratch{directory: t.TempDir()}
}

// next is the path of a new file.
func (files *scratch) next() string {
	files.made++
	return filepath.Join(files.directory, fmt.Sprintf("m%d.nc", files.made))
}

// shownBytes is how many bytes either side of a difference are shown.
const shownBytes = 8

// byteDifference says where two byte strings first part, and what each
// holds there.
func byteDifference(want, got []byte) string {
	at := 0
	for at < len(want) && at < len(got) && want[at] == got[at] {
		at++
	}
	show := func(raw []byte) []byte {
		return raw[min(at, len(raw)):min(at+shownBytes, len(raw))]
	}
	return fmt.Sprintf("first difference at byte %d: want % x (of %d), got "+
		"% x (of %d)", at, show(want), len(want), show(got), len(got))
}

// What is written is read back: every dimension, attribute, variable and
// value of a random model, of any of the three formats and any type the
// format has.
func TestPropertyRoundTrip(t *testing.T) {
	files := newScratch(t)
	rapid.Check(t, func(t *rapid.T) {
		model := drawModel(t, modelWants{fills: true})
		path := files.next()
		defer os.Remove(path)
		writer, err := model.create(path)
		if err != nil {
			t.Fatalf("the writer refused a legal model: %v\n%s", err, model)
		}
		if err := model.fill(writer); err != nil {
			t.Fatalf("a legal write was refused: %v\n%s", err, model)
		}
		if got := writer.NumRecs(); got != model.Records {
			t.Fatalf("the writer holds %d records, want %d\n%s", got,
				model.Records, model)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("Close: %v\n%s", err, model)
		}
		file, err := netcdf.Open(path)
		if err != nil {
			t.Fatalf("a file the writer made does not open: %v\n%s", err, model)
		}
		defer file.Close()
		if line := model.differs(file); line != "" {
			t.Fatalf("read back differently: %s\n%s", line, model)
		}
	})
}

// saved is the bytes of the file the writer makes of a model.
func saved(t *rapid.T, files *scratch, model fragModel) []byte {
	path := files.next()
	defer os.Remove(path)
	if err := model.save(path); err != nil {
		t.Fatalf("the writer refused a legal model: %v\n%s", err, model)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The header the harness's encoder makes of a model, written from the
// grammar, is byte for byte the header the writer makes. When they
// differ one of the two is wrong about the format.
func TestPropertyTwoWritersAgree(t *testing.T) {
	files := newScratch(t)
	rapid.Check(t, func(t *rapid.T) {
		model := drawModel(t, modelWants{fills: true})
		raw := saved(t, files, model)
		mine := model.header().Bytes()
		if len(raw) < len(mine) || !bytes.Equal(raw[:len(mine)], mine) {
			t.Fatalf("the two headers differ: %s\n%s",
				byteDifference(mine, raw), model)
		}
	})
}

// The values of a model lie in the file where the grammar puts them,
// big-endian, for a fixed variable whole and for a record variable in
// each record. A write and a read that were wrong the same way, which a
// round trip cannot see, would fail here.
func TestPropertyValuesWhereTheGrammarPutsThem(t *testing.T) {
	files := newScratch(t)
	rapid.Check(t, func(t *rapid.T) { checkPlaced(t, files) })
}

// checkPlaced draws a model, writes it, and fails unless each value lies
// in the file where the grammar says.
func checkPlaced(t *rapid.T, files *scratch) {
	model := drawModel(t, modelWants{fills: true})
	raw := saved(t, files, model)
	head := model.header()
	found := head.place(uint64(len(head.Bytes())))
	for _, each := range model.regions(found) {
		end := each.At + uint64(len(each.Data))
		if end > uint64(len(raw)) {
			t.Fatalf("%s should end at %d, the file has %d\n%s", each.Name,
				end, len(raw), model)
		}
		if got := raw[each.At:end]; !bytes.Equal(got, each.Data) {
			t.Fatalf("%s at %d: %s\n%s", each.Name, each.At,
				byteDifference(each.Data, got), model)
		}
	}
}

// A file that the harness's encoder makes of a model, header and data,
// is read by the reader as the model.
func TestPropertyTwoReadersAgree(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		model := drawModel(t, modelWants{fills: true})
		raw := model.image()
		file, err := netcdf.Read(netcdf.Source{Name: "image",
			At: bytes.NewReader(raw), Size: int64(len(raw))})
		if err != nil {
			t.Fatalf("a file of the grammar does not open: %v\n%s", err, model)
		}
		defer file.Close()
		if line := model.differs(file); line != "" {
			t.Fatalf("read as another file: %s\n%s", line, model)
		}
	})
}

// A record variable of no records reads as no values, and a slab of no
// values reads as none: the netCDF-C call does the same, and an error
// here would make a file that has none yet unreadable.
func TestAZeroRecordVariableReads(t *testing.T) {
	for _, format := range formatsOf {
		model := fragModel{Format: format,
			Dims: []modelDim{{"t", 0}, {"x", 2}},
			Vars: []modelVar{{Name: "r", Type: netcdf.TypeInt,
				Dims: []int{0, 1}, Values: []int32{}}}}
		path := filepath.Join(t.TempDir(), "none.nc")
		if err := model.save(path); err != nil {
			t.Fatal(err)
		}
		file, err := netcdf.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		variable, err := file.Variable("r")
		if err != nil {
			t.Fatal(err)
		}
		if got, err := variable.Values(); err != nil || !slices.Equal(
			bitsOf(got), []uint64{}) {
			t.Errorf("%s: no records read as %v, %v", netcdf.FormatName(format),
				got, err)
		}
		_, err = variable.Slab(netcdf.Slab{Start: []int64{0, 0},
			Count: []int64{0, 2}})
		if err != nil {
			t.Errorf("%s: a slab of no records: %v", netcdf.FormatName(format),
				err)
		}
		file.Close()
	}
}
