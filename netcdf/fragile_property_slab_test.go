//go:build fragile

package netcdf_test

// Properties of slabs and records: a slab read is a part of the whole,
// slab writes that tile a variable leave the file one whole write leaves,
// and a record written past the end leaves the records before it, and
// the same record of the other variables, holding the fill value.

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/janearc/celestial-data/netcdf"
)

// maxCuts is the most cuts of a dimension a tiling makes.
const maxCuts = 2

// holding are the variables of a model that have dimensions and values.
func holding(model fragModel) []int {
	var found []int
	for at, each := range model.Vars {
		if len(each.Dims) > 0 && countOf(model.shape(each)) > 0 {
			found = append(found, at)
		}
	}
	return found
}

// signedBits is bits as signed numbers, for extract, which takes those.
func signedBits(values any) []int64 {
	found := make([]int64, 0, len(bitsOf(values)))
	for _, each := range bitsOf(values) {
		found = append(found, int64(each))
	}
	return found
}

// drawSlab is a start and count inside a shape, none of them empty.
func drawSlab(t *rapid.T, shape []int64) (start, count []int64) {
	for _, length := range shape {
		first := int64(rapid.IntRange(0, int(length)-1).Draw(t, "start"))
		size := int64(rapid.IntRange(1, int(length-first)).Draw(t, "count"))
		start, count = append(start, first), append(count, size)
	}
	return start, count
}

// A slab of a variable is the same elements of the whole variable: the
// whole as the file reads it, and the whole as the model wrote it.
func TestPropertySlabIsPartOfTheWhole(t *testing.T) {
	files := newScratch(t)
	rapid.Check(t, func(t *rapid.T) {
		model := drawModel(t, modelWants{ranked: true})
		path := files.next()
		defer os.Remove(path)
		if err := model.save(path); err != nil {
			t.Fatalf("the writer refused a legal model: %v\n%s", err, model)
		}
		file, err := netcdf.Open(path)
		if err != nil {
			t.Fatalf("a written file does not open: %v\n%s", err, model)
		}
		defer file.Close()
		each := model.Vars[rapid.SampledFrom(holding(model)).Draw(t,
			"variable")]
		shape := model.shape(each)
		start, count := drawSlab(t, shape)
		variable, err := file.Variable(each.Name)
		if err != nil {
			t.Fatal(err)
		}
		slab, err := variable.Slab(netcdf.Slab{Start: start, Count: count})
		if err != nil {
			t.Fatalf("a slab inside the shape is refused: %v\nvariable %q "+
				"start %v count %v\n%s", err, each.Name, start, count, model)
		}
		whole, err := variable.Values()
		if err != nil {
			t.Fatalf("the whole is refused: %v\n%s", err, model)
		}
		for name, flat := range map[string][]int64{
			"the whole read": signedBits(whole),
			"the model":      signedBits(each.Values)} {
			want := extract(shape, start, count, flat)
			if got := signedBits(slab); !slices.Equal(got, want) {
				t.Fatalf("variable %q start %v count %v reads %v, %s has %v\n%s",
					each.Name, start, count, got, name, want, model)
			}
		}
	})
}

// box is a part of a variable: a start and count in each dimension.
type box struct {
	start, count []int64
}

// drawSegments cuts one dimension into pieces, as (start, count) pairs.
func drawSegments(t *rapid.T, length int64) [][2]int64 {
	var cuts []int
	if length > 1 {
		cuts = rapid.SliceOfNDistinct(rapid.IntRange(1, int(length)-1), 0,
			min(maxCuts, int(length)-1), func(at int) int { return at }).
			Draw(t, "cuts")
	}
	slices.Sort(cuts)
	var found [][2]int64
	from := int64(0)
	for _, each := range append(cuts, int(length)) {
		found = append(found, [2]int64{from, int64(each) - from})
		from = int64(each)
	}
	return found
}

// drawBoxes tiles a shape: the product of the pieces of each dimension.
func drawBoxes(t *rapid.T, shape []int64) []box {
	found := []box{{}}
	for _, length := range shape {
		var next []box
		for _, each := range found {
			for _, piece := range drawSegments(t, length) {
				next = append(next, box{
					append(slices.Clone(each.start), piece[0]),
					append(slices.Clone(each.count), piece[1])})
			}
		}
		found = next
	}
	return found
}

// tile is one write: a box of one variable of a model.
type tile struct {
	variable int
	box      box
}

// drawTiles are the tiles of every variable that holds values, in an
// order drawn at random, the variables mixed.
func drawTiles(t *rapid.T, model fragModel) []tile {
	var tiles []tile
	for at, each := range model.Vars {
		shape := model.shape(each)
		if countOf(shape) == 0 {
			continue
		}
		for _, part := range drawBoxes(t, shape) {
			tiles = append(tiles, tile{at, part})
		}
	}
	return rapid.Permutation(tiles).Draw(t, "order of the tiles")
}

// picked is the values at the indices, as a slice of the same type.
func picked(values any, indices []int64) any {
	whole := reflect.ValueOf(values)
	found := reflect.MakeSlice(whole.Type(), len(indices), len(indices))
	for at, index := range indices {
		found.Index(at).Set(whole.Index(int(index)))
	}
	return found.Interface()
}

// writeTiles writes a model to a path tile by tile, in the order given.
func writeTiles(model fragModel, path string, tiles []tile) error {
	writer, err := model.create(path)
	if err != nil {
		return err
	}
	for _, each := range tiles {
		source := model.Vars[each.variable]
		shape := model.shape(source)
		flat := make([]int64, countOf(shape))
		for at := range flat {
			flat[at] = int64(at)
		}
		values := picked(source.Values, extract(shape, each.box.start,
			each.box.count, flat))
		target, err := writer.Variable(source.Name)
		if err == nil {
			err = target.WriteSlab(values, netcdf.Slab{Start: each.box.start,
				Count: each.box.count})
		}
		if err != nil {
			return err
		}
	}
	return writer.Close()
}

// Slab writes that tile every variable of a model, in any order, leave
// the file that one whole write of each leaves, byte for byte.
func TestPropertySlabWritesTileTheWhole(t *testing.T) {
	files := newScratch(t)
	rapid.Check(t, func(t *rapid.T) {
		model := drawModel(t, modelWants{ranked: true})
		whole, tiled := files.next(), files.next()
		defer os.Remove(whole)
		defer os.Remove(tiled)
		tiles := drawTiles(t, model)
		if err := model.save(whole); err != nil {
			t.Fatalf("the writer refused a legal model: %v\n%s", err, model)
		}
		if err := writeTiles(model, tiled, tiles); err != nil {
			t.Fatalf("a legal slab write was refused: %v\n%s", err, model)
		}
		want, _ := os.ReadFile(whole)
		got, _ := os.ReadFile(tiled)
		if !bytes.Equal(want, got) {
			t.Fatalf("tiled in %d writes: %s\n%s", len(tiles),
				byteDifference(want, got), model)
		}
	})
}

// recordsOf are the variables of a model over the unlimited dimension.
func recordsOf(model fragModel) []int {
	var found []int
	for at, each := range model.Vars {
		if model.record(each) {
			found = append(found, at)
		}
	}
	return found
}

// Writing record k of one record variable of a file that has fewer
// leaves the record count k+1, every record before it holding the fill
// value in every record variable, and record k itself the fill value in
// the variables that were not written.
func TestPropertyARecordPastTheEndFillsTheRest(t *testing.T) {
	files := newScratch(t)
	rapid.Check(t, func(t *rapid.T) {
		model := drawModel(t, modelWants{record: true, fills: true})
		written := rapid.SampledFrom(recordsOf(model)).Draw(t,
			"written variable")
		record := rapid.IntRange(1, maxRecords).Draw(t, "record")
		source := model.Vars[written]
		inner := model.shape(source)[1:]
		values := drawValues(t, source.Type, int(countOf(inner)), "record")
		path := files.next()
		defer os.Remove(path)
		writer, err := model.create(path)
		if err != nil {
			t.Fatalf("the writer refused a legal model: %v\n%s", err, model)
		}
		target, _ := writer.Variable(source.Name)
		slab := netcdf.Slab{Start: append([]int64{int64(record)},
			make([]int64, len(inner))...), Count: append([]int64{1}, inner...)}
		if err := target.WriteSlab(values, slab); err != nil {
			t.Fatalf("a legal record write was refused: %v\nrecord %d of %q"+
				"\n%s", err, record, source.Name, model)
		}
		if got := writer.NumRecs(); got != int64(record)+1 {
			t.Fatalf("after record %d the writer holds %d records\n%s", record,
				got, model)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if line := recordsDiffer(path, model, written, record, values); line != "" {
			t.Fatalf("%s\nrecord %d of %q\n%s", line, record, source.Name, model)
		}
	})
}

// recordsDiffer is the first record of a file, after one record of one
// variable was written, that is not as it should be.
func recordsDiffer(path string, model fragModel, written, record int,
	values any) string {
	file, err := netcdf.Open(path)
	if err != nil {
		return "the file does not open: " + err.Error()
	}
	defer file.Close()
	if file.NumRecs() != int64(record)+1 {
		return fmt.Sprintf("the file holds %d records", file.NumRecs())
	}
	for _, at := range recordsOf(model) {
		each := model.Vars[at]
		got, err := mustVariable(file, each.Name).Values()
		if err != nil {
			return "a record variable is unreadable: " + err.Error()
		}
		size := int(countOf(model.shape(each)[1:]))
		want := slices.Repeat([]uint64{fillOf(each)}, size*(record+1))
		if at == written {
			copy(want[size*record:], bitsOf(values))
		}
		if have := bitsOf(got); !slices.Equal(have, want) {
			return "variable " + each.Name + " reads " + formatBits(have) +
				", want " + formatBits(want)
		}
	}
	return ""
}

// mustVariable is a variable of a file that is there.
func mustVariable(file *netcdf.File, name string) *netcdf.Variable {
	found, err := file.Variable(name)
	if err != nil {
		panic(err)
	}
	return found
}

// formatBits is bits in hexadecimal, for a failure to show.
func formatBits(bits []uint64) string {
	return fmt.Sprintf("%x", bits)
}
