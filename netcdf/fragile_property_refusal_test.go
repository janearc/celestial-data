//go:build fragile

package netcdf_test

// Properties of refusal: any bytes at all open or are refused as one of
// the reader's own errors, a valid header with one field replaced by any
// value is the same, and a closed file refuses every read and write as
// closed, however often it was closed.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	classic "github.com/scigolib/netcdf"
	"pgregory.net/rapid"

	"github.com/janearc/celestial-data/netcdf"
)

// hisErrors are the errors the reader's package has, any of which an
// error from it must be found as.
var hisErrors = []error{classic.ErrNotNetCDF, classic.ErrCorrupt,
	classic.ErrOutOfRange, classic.ErrUnsupportedFormat,
	classic.ErrTypeMismatch, classic.ErrClosed, classic.ErrInvalidSchema}

// isHis is whether an error is one of the reader's.
func isHis(err error) bool {
	for _, each := range hisErrors {
		if errors.Is(err, each) {
			return true
		}
	}
	return false
}

// goTypes are the Go element types of the eleven types.
var goTypes = map[classic.Type]reflect.Type{
	classic.TypeByte:   reflect.TypeFor[int8](),
	classic.TypeChar:   reflect.TypeFor[byte](),
	classic.TypeShort:  reflect.TypeFor[int16](),
	classic.TypeInt:    reflect.TypeFor[int32](),
	classic.TypeFloat:  reflect.TypeFor[float32](),
	classic.TypeDouble: reflect.TypeFor[float64](),
	classic.TypeUByte:  reflect.TypeFor[byte](),
	classic.TypeUShort: reflect.TypeFor[uint16](),
	classic.TypeUInt:   reflect.TypeFor[uint32](),
	classic.TypeInt64:  reflect.TypeFor[int64](),
	classic.TypeUInt64: reflect.TypeFor[uint64](),
}

// readHis reads a whole variable through the reader's own package into a
// slice of its Go type, unless it claims more values than limit.
func readHis(variable *classic.Variable, limit int64) (any, error) {
	each, known := goTypes[variable.Type()]
	if !known || variable.Len() > limit {
		return nil, nil
	}
	size := int(variable.Len())
	made := reflect.New(reflect.SliceOf(each))
	made.Elem().Set(reflect.MakeSlice(reflect.SliceOf(each), size, size))
	err := variable.Read(made.Interface())
	return made.Elem().Interface(), err
}

// touchHis asks an attribute of the reader's package for each of its
// forms.
func touchHis(each *classic.Attribute) {
	_ = each.Value()
	_ = each.Bytes()
	_ = each.String()
	_ = each.Len()
}

// reachHis opens bytes with the reader's own package, reads all it holds,
// and gives what is wrong in what came back: an error that is none of
// its own.
func reachHis(raw []byte) []string {
	file, err := classic.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		if !isHis(err) {
			return []string{"open: an error of nobody's: " + err.Error()}
		}
		return nil
	}
	defer file.Close()
	var problems []string
	for _, each := range file.Attributes() {
		touchHis(each)
	}
	for _, each := range file.Variables() {
		for _, kept := range each.Attributes() {
			touchHis(kept)
		}
		if _, err := readHis(each, int64(len(raw))); err != nil && !isHis(err) {
			problems = append(problems, "read: an error of nobody's: "+
				err.Error())
		}
	}
	return problems
}

// refusalsOf runs reachHis and adds a panic or a hang to what is wrong.
func refusalsOf(raw []byte) []string {
	var problems []string
	got := attempt(func() { problems = reachHis(raw) })
	switch {
	case got.timedOut:
		problems = append(problems, "no result in time: a hang")
	case got.panicked != nil:
		problems = append(problems, fmt.Sprintf("panic: %v at %s",
			got.panicked, frames(got.stack)))
	}
	return problems
}

// interesting are the words of a header that mean something: tags,
// counts, small and large.
var interesting = []uint32{0, 1, 2, 3, 4, 5, 8, 10, 11, 12, 255, 256, 1 << 16,
	1<<31 - 1, 1 << 31, 1<<32 - 1}

// magics are the starts a drawn input may have.
var magics = []string{"", "C", "CDF", "CDF\x01", "CDF\x02", "CDF\x05",
	"\x89HDF\r\n\x1a\n"}

// maxRaw is the most bytes a drawn input has after its start, and maxWords
// the most words.
const maxRaw, maxWords = 300, 60

// drawInput is bytes: a start, then random bytes or random words of the
// header's kind.
func drawInput(t *rapid.T) []byte {
	raw := []byte(rapid.SampledFrom(magics).Draw(t, "start"))
	if !rapid.Bool().Draw(t, "in words") {
		return append(raw, rapid.SliceOfN(rapid.Byte(), 0,
			maxRaw).Draw(t, "bytes")...)
	}
	words := rapid.SliceOfN(rapid.OneOf(rapid.SampledFrom(interesting),
		rapid.Uint32()), 0, maxWords).Draw(t, "words")
	for _, each := range words {
		raw = binary.BigEndian.AppendUint32(raw, each)
	}
	return raw
}

// Any bytes at all open, or are refused with an error that is found as
// one of the reader's own. Never a panic, never an error of nobody's.
func TestPropertyAnyBytesAreRefusedAsHis(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := drawInput(t)
		if problems := refusalsOf(raw); len(problems) > 0 {
			t.Fatalf("%v\ninput % x", problems, raw)
		}
	})
}

// lies are the fields of a header that may be replaced: each sets one
// field of one item to a value, and does nothing if there is no item.
var lies = []func(head *header, at, other int, value uint64){
	func(head *header, _, _ int, value uint64) { head.NumRecs = some(value) },
	func(head *header, _, _ int, value uint64) {
		head.DimTag = some(uint32(value))
	},
	func(head *header, _, _ int, value uint64) { head.DimCount = some(value) },
	func(head *header, _, _ int, value uint64) {
		head.AttrTag = some(uint32(value))
	},
	func(head *header, _, _ int, value uint64) { head.AttrCount = some(value) },
	func(head *header, _, _ int, value uint64) {
		head.VarTag = some(uint32(value))
	},
	func(head *header, _, _ int, value uint64) { head.VarCount = some(value) },
	func(head *header, _, _ int, value uint64) {
		head.VersionByte = some(byte(value))
	},
	func(head *header, at, _ int, value uint64) {
		if len(head.Dims) > 0 {
			head.Dims[at%len(head.Dims)].Length = value
		}
	},
	func(head *header, at, _ int, value uint64) {
		if len(head.Dims) > 0 {
			head.Dims[at%len(head.Dims)].NameLength = some(value)
		}
	},
	func(head *header, at, _ int, value uint64) {
		if len(head.Attrs) > 0 {
			head.Attrs[at%len(head.Attrs)].Type = uint32(value)
		}
	},
	func(head *header, at, _ int, value uint64) {
		if len(head.Attrs) > 0 {
			head.Attrs[at%len(head.Attrs)].Count = some(value)
		}
	},
	func(head *header, at, _ int, value uint64) {
		if len(head.Attrs) > 0 {
			head.Attrs[at%len(head.Attrs)].NameLength = some(value)
		}
	},
}

// variableLies are the fields of a variable of a header that may be
// replaced.
var variableLies = []func(each *varSpec, other int, value uint64){
	func(each *varSpec, _ int, value uint64) { each.NameLength = some(value) },
	func(each *varSpec, _ int, value uint64) { each.Rank = some(value) },
	func(each *varSpec, other int, value uint64) {
		if len(each.DimIDs) > 0 {
			each.DimIDs[other%len(each.DimIDs)] = value
		}
	},
	func(each *varSpec, _ int, value uint64) {
		each.AttrTag = some(uint32(value))
	},
	func(each *varSpec, _ int, value uint64) { each.AttrCount = some(value) },
	func(each *varSpec, _ int, value uint64) { each.Type = uint32(value) },
	func(each *varSpec, _ int, value uint64) { each.VSize = some(value) },
	func(each *varSpec, _ int, value uint64) { each.Begin = some(value) },
}

// replacements are the numbers a replaced field is given, besides any
// at all.
var replacements = []uint64{0, 1, 2, 3, 4, 7, 8, 12, 255, 1 << 16, 1<<31 - 1, 1 << 31,
	1<<32 - 1, 1 << 32, 1 << 62, 1 << 63, 1<<64 - 1}

// drawLie is a header with one field replaced by a value.
func drawLie(t *rapid.T, head header) header {
	value := rapid.OneOf(rapid.SampledFrom(replacements),
		rapid.Uint64()).Draw(t, "replacement")
	at := rapid.IntRange(0, maxVariables).Draw(t, "item")
	other := rapid.IntRange(0, maxVariables).Draw(t, "inner item")
	which := rapid.IntRange(0, len(lies)+len(variableLies)-1).Draw(t, "field")
	if which < len(lies) {
		lies[which](&head, at, other, value)
		return head
	}
	if len(head.Vars) > 0 {
		variableLies[which-len(lies)](&head.Vars[at%len(head.Vars)], other,
			value)
	}
	return head
}

// A valid header with one field replaced by any value is opened or
// refused as one of the reader's own errors. The file is the header and
// the data its honest placement asks, so that a lie about a size reaches
// the reading of data.
func TestPropertyAReplacedFieldIsRefusedAsHis(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		model := drawModel(t, modelWants{fills: true})
		head := drawLie(t, model.header())
		raw := head.File()
		if problems := refusalsOf(raw); len(problems) > 0 {
			t.Fatalf("%v\n%s\nthe header with a field replaced: % x", problems,
				model, head.Bytes())
		}
	})
}

// Close is idempotent, for the writer and for the file read, and a
// closed one refuses every write and every read with the closed error.
func TestPropertyAClosedFileRefusesAsClosed(t *testing.T) {
	files := newScratch(t)
	rapid.Check(t, func(t *rapid.T) {
		model := drawModel(t, modelWants{fills: true})
		path := files.next()
		defer os.Remove(path)
		writer, err := model.create(path)
		if err != nil {
			t.Fatalf("the writer refused a legal model: %v\n%s", err, model)
		}
		writers := writer.Variables()
		for range 2 {
			if err := writer.Close(); err != nil {
				t.Fatalf("Close of a writer: %v\n%s", err, model)
			}
		}
		for at, each := range writers {
			refusedClosed(t, model, "write", each.Write(model.Vars[at].Values))
			refusedClosed(t, model, "write of a slab", each.WriteSlab(
				drawValues(t, model.Vars[at].Type, 1, "value after Close"),
				onesOf(model.shape(model.Vars[at]))))
		}
		file, err := netcdf.Open(path)
		if err != nil {
			t.Fatalf("a written file does not open: %v\n%s", err, model)
		}
		variables := file.Variables()
		for range 2 {
			if err := file.Close(); err != nil {
				t.Fatalf("Close of a file read: %v\n%s", err, model)
			}
		}
		for at, each := range variables {
			readsRefused(t, model, each, model.Vars[at])
		}
	})
}

// onesOf is a slab of one value at the start of each dimension of a
// shape.
func onesOf(shape []int64) netcdf.Slab {
	count := make([]int64, len(shape))
	for at := range count {
		count[at] = 1
	}
	return netcdf.Slab{Start: make([]int64, len(shape)), Count: count}
}

// refusedClosed fails the property unless an error is the closed error.
func refusedClosed(t *rapid.T, model fragModel, what string, err error) {
	t.Helper()
	if !errors.Is(err, netcdf.ErrClosed) {
		t.Fatalf("a %s after Close gave %v, want the closed error\n%s", what,
			err, model)
	}
}

// readsRefused asks a variable of a closed file for every kind of read.
func readsRefused(t *rapid.T, model fragModel, got *netcdf.Variable,
	want modelVar) {
	t.Helper()
	_, err := got.Values()
	refusedClosed(t, model, "whole read of "+want.Name, err)
	_, err = got.Slab(onesOf(model.shape(want)))
	refusedClosed(t, model, "slab read of "+want.Name, err)
	if want.Type != netcdf.TypeChar {
		_, err = got.NumbersSlab(onesOf(model.shape(want)))
		refusedClosed(t, model, "widened read of "+want.Name, err)
	}
}
