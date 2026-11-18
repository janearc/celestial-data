//go:build fragile

package netcdf_test

// Writing: schemas that must be refused at each step, a file written,
// closed and reopened for every type and for a _FillValue, a gap in the
// records, a writer not closed, and writes that must be refused. Every
// file is a few megabytes at most, in t.TempDir().

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	classic "github.com/scigolib/netcdf"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/netcdf"
)

// The shapes the writing tests use.
const (
	// writeWidth is the values in a record of the tests' variables.
	writeWidth = 3
	// gapSmall and gapLarge are records written past the end: a gap of a
	// few megabytes at most.
	gapSmall, gapLarge = 25_000, 200_000
	// gapWidth is the values in a record of the gap file.
	gapWidth = 4
	// longName is a name of the most the writer takes, in bytes.
	longName = 256
	// rankLimit is the most dimensions a variable has.
	rankLimit = 1024
)

// formatsOf are the three formats the writer makes.
var formatsOf = []netcdf.Format{netcdf.FormatClassic,
	netcdf.FormatClassic64, netcdf.FormatClassic64Data}

// firstStep is a step of building a layout, which may fail.
type firstStep func(*netcdf.Layout) error

// created runs the steps on a new layout of a format and creates the
// file, gives the first error, or nil with the file closed.
func created(format netcdf.Format, path string, steps ...firstStep) error {
	layout, err := netcdf.NewLayout(format)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if err := step(layout); err != nil {
			return err
		}
	}
	writer, err := layout.Create(path)
	if err != nil {
		return err
	}
	return writer.Close()
}

// variableOver is a step that adds a dimension of a length, and a
// variable of a type over it, as many times as the dimensions given.
func variableOver(name string, typ netcdf.Type, lengths ...int64) firstStep {
	return func(layout *netcdf.Layout) error {
		var over []*netcdf.LayoutDimension
		for at, length := range lengths {
			each, err := layout.AddDimension(fmt.Sprintf("d%d", at), length)
			if err != nil {
				return err
			}
			over = append(over, each)
		}
		_, err := layout.AddVariable(netcdf.VariableSpec{Name: name,
			Type: typ, Dimensions: over})
		return err
	}
}

// dimensionNamed is a step that adds a dimension.
func dimensionNamed(name string, length int64) firstStep {
	return func(layout *netcdf.Layout) error {
		_, err := layout.AddDimension(name, length)
		return err
	}
}

// layoutFault is a layout that must be refused, as one of the kinds.
type layoutFault struct {
	name   string
	format netcdf.Format
	steps  []firstStep
	want   []error
}

// invalid is a fault refused as an invalid schema.
func invalid(name string, format netcdf.Format, steps ...firstStep) layoutFault {
	return layoutFault{name, format, steps, []error{netcdf.ErrInvalidSchema}}
}

// foreignDimension is a step that gives a variable a dimension that
// another layout made.
func foreignDimension(layout *netcdf.Layout) error {
	other, err := netcdf.NewLayout(netcdf.FormatClassic)
	if err != nil {
		return err
	}
	each, err := other.AddDimension("x", 2)
	if err != nil {
		return err
	}
	_, err = layout.AddVariable(netcdf.VariableSpec{Name: "v",
		Type: netcdf.TypeInt, Dimensions: []*netcdf.LayoutDimension{each}})
	return err
}

// attributeStep is a step that sets a global attribute of a type.
func attributeStep(name string, typ netcdf.Type, values any) firstStep {
	return func(layout *netcdf.Layout) error {
		return layout.SetTyped(name, typ, values)
	}
}

// fillStep is a step that adds an integer variable with a _FillValue of
// a type.
func fillStep(typ netcdf.Type, fill any) firstStep {
	return func(layout *netcdf.Layout) error {
		each, err := layout.AddDimension("x", 2)
		if err != nil {
			return err
		}
		variable, err := layout.AddVariable(netcdf.VariableSpec{Name: "v",
			Type: netcdf.TypeInt, Dimensions: []*netcdf.LayoutDimension{each}})
		if err != nil {
			return err
		}
		return variable.SetTyped("_FillValue", typ, fill)
	}
}

// schemaFaults are the faults at each step of making a layout.
func schemaFaults() []layoutFault {
	faults := []layoutFault{
		invalid("a dimension twice", netcdf.FormatClassic,
			dimensionNamed("x", 2), dimensionNamed("x", 3)),
		invalid("a variable twice", netcdf.FormatClassic,
			variableOver("v", netcdf.TypeInt, 2), variableOver("v",
				netcdf.TypeInt, 2)),
		invalid("an unlimited dimension that is not first",
			netcdf.FormatClassic, variableOver("v", netcdf.TypeInt, 3, 0)),
		invalid("two unlimited dimensions", netcdf.FormatClassic,
			dimensionNamed("t", 0), variableOver("v", netcdf.TypeInt, 0)),
		invalid("an attribute of the wrong type tag", netcdf.FormatClassic,
			attributeStep("a", netcdf.Type(0), []int8{1})),
		invalid("an int64 attribute in CDF-2", netcdf.FormatClassic64,
			attributeStep("a", netcdf.TypeInt64, []int64{1})),
		invalid("a _FillValue of another type", netcdf.FormatClassic,
			fillStep(netcdf.TypeDouble, []float64{1})),
		invalid("a _FillValue of two values", netcdf.FormatClassic,
			fillStep(netcdf.TypeInt, []int32{1, 2})),
		invalid("a rank of 1025", netcdf.FormatClassic64Data,
			sameDimension(rankLimit+1)),
		invalid("a CDF-2 dimension of 1<<32", netcdf.FormatClassic64,
			dimensionNamed("x", 1<<32)),
		{"a dimension of another layout", netcdf.FormatClassic,
			[]firstStep{foreignDimension}, []error{celestialdata.Refused()}},
	}
	return append(faults, nameFaults()...)
}

// sameDimension is a step that adds a variable over one dimension as
// many times as asked.
func sameDimension(rank int) firstStep {
	return func(layout *netcdf.Layout) error {
		each, err := layout.AddDimension("x", 1)
		if err != nil {
			return err
		}
		over := make([]*netcdf.LayoutDimension, rank)
		for at := range over {
			over[at] = each
		}
		_, err = layout.AddVariable(netcdf.VariableSpec{Name: "v",
			Type: netcdf.TypeInt, Dimensions: over})
		return err
	}
}

// nameFaults are names of dimensions, variables and attributes that the
// format does not allow, and types it does not hold.
func nameFaults() []layoutFault {
	var faults []layoutFault
	for _, bad := range []string{" x", "x ", "a/b", "\xff", "tab\there",
		strings.Repeat("n", longName+1)} {
		faults = append(faults,
			invalid(fmt.Sprintf("a dimension named %.12q", bad),
				netcdf.FormatClassic, dimensionNamed(bad, 2)),
			invalid(fmt.Sprintf("a variable named %.12q", bad),
				netcdf.FormatClassic, variableOver(bad, netcdf.TypeInt, 2)),
			invalid(fmt.Sprintf("an attribute named %.12q", bad),
				netcdf.FormatClassic, attributeStep(bad, netcdf.TypeInt,
					[]int32{1})))
	}
	for _, format := range formatsOf {
		for _, typ := range []netcdf.Type{0, 12, 255, netcdf.TypeUByte,
			netcdf.TypeUShort, netcdf.TypeUInt, netcdf.TypeInt64,
			netcdf.TypeUInt64} {
			if format == netcdf.FormatClassic64Data && typ >= 1 && typ <= 11 {
				continue
			}
			faults = append(faults, invalid(fmt.Sprintf(
				"a variable of type %d in %s", typ, netcdf.FormatName(format)),
				format, variableOver("v", typ, 2)))
		}
	}
	return faults
}

// Faults at each step of making a layout are refused, as the kind of
// error they are, and no file is made.
func TestSchemaFaults(t *testing.T) {
	for _, each := range schemaFaults() {
		t.Run(each.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fault.nc")
			var err error
			got := attempt(func() {
				err = created(each.format, path, each.steps...)
			})
			problems := got.problems(0, 0)
			if err == nil {
				problems = append(problems, "a faulty layout made a file")
			} else if !hasOneOf(err, each.want) {
				problems = append(problems, fmt.Sprintf(
					"refused as none of %v: %v", each.want, err))
			}
			settle(problems)
			for _, line := range problems {
				t.Error(line)
			}
		})
	}
}

// hasOneOf is whether an error is any of the kinds.
func hasOneOf(err error, kinds []error) bool {
	for _, kind := range kinds {
		if errors.Is(err, kind) {
			return true
		}
	}
	return false
}

// The same faults through the reader's own schema, where the adapter
// has refused them first.
func TestSchemaFaultsOfTheReaderItself(t *testing.T) {
	path := filepath.Join(t.TempDir(), "direct.nc")
	foreign := classic.NewSchema(classic.FormatClassic)
	other := classic.NewSchema(classic.FormatClassic)
	foreign.AddVariable("v", classic.TypeInt, other.AddDimension("x", 2))
	negative := classic.NewSchema(classic.FormatClassic)
	negative.AddDimension("x", -1)
	empty := classic.NewSchema(classic.FormatClassic)
	empty.AddDimension("", 2)
	noValues := classic.NewSchema(classic.FormatClassic)
	_ = noValues.SetAttr("a", []int{1})
	for name, schema := range map[string]*classic.Schema{
		"a dimension of another schema":  foreign,
		"a dimension of negative length": negative,
		"a dimension with no name":       empty,
		"an attribute of a Go int":       noValues} {
		var err error
		got := attempt(func() { _, err = classic.Create(path, schema) })
		problems := got.problems(0, 0)
		if err == nil {
			problems = append(problems, name+": a file was made")
		}
		settle(problems)
		for _, line := range problems {
			t.Errorf("%s: %s", name, line)
		}
	}
}

// roundTrip opens the file at a path that a step has just made, and
// stops the test if the step failed or the file does not open.
func roundTrip(t *testing.T, path string, err error) *netcdf.File {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	file, err := netcdf.Open(path)
	if err != nil {
		t.Fatalf("a file the writer made does not open: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

// An attribute of no values is refused, or written and read back as one.
func TestAnAttributeOfNoValues(t *testing.T) {
	for name, empty := range map[string]any{"int32": []int32{},
		"text": "", "bytes": []byte{}} {
		path := filepath.Join(t.TempDir(), "empty.nc")
		err := created(netcdf.FormatClassic, path, func(
			layout *netcdf.Layout) error {
			return layout.Set("a", empty)
		})
		var problems []string
		if err != nil && !errors.Is(err, netcdf.ErrInvalidSchema) {
			problems = append(problems, name+": untyped "+err.Error())
		}
		if err == nil {
			file := roundTrip(t, path, nil)
			found, lookup := file.Attribute("a")
			if lookup != nil || found.Length() != 0 {
				problems = append(problems, fmt.Sprintf(
					"%s: read back %v, %v", name, found, lookup))
			}
		}
		settle(problems)
		for _, line := range problems {
			t.Error(line)
		}
	}
}

// Everything the writer accepts at its limits is a file the reader opens.
func TestWhatTheWriterAcceptsTheReaderOpens(t *testing.T) {
	long := strings.Repeat("n", longName)
	cases := map[string][]firstStep{
		"names of 256 bytes": {variableOver(long, netcdf.TypeInt, 2)},
		"a rank of 1024":     {sameDimension(rankLimit)},
		"a long attribute name": {attributeStep(long, netcdf.TypeInt,
			[]int32{1})},
		"no variable at all": {dimensionNamed("x", 3)},
		"a record variable alone": {variableOver("r", netcdf.TypeDouble, 0,
			3)},
	}
	for name, steps := range cases {
		for _, format := range formatsOf {
			path := filepath.Join(t.TempDir(), "limit.nc")
			err := created(format, path, steps...)
			var problems []string
			if err == nil {
				file := roundTrip(t, path, nil)
				if got := readEverything(file); got != nil {
					problems = append(problems, got.Error())
				}
			} else if !errors.Is(err, netcdf.ErrInvalidSchema) {
				problems = append(problems, "untyped: "+err.Error())
			}
			settle(problems)
			for _, line := range problems {
				t.Errorf("%s in %s: %s", name, netcdf.FormatName(format), line)
			}
		}
	}
	huge := filepath.Join(t.TempDir(), "huge.nc")
	check(t, created(netcdf.FormatClassic64Data, huge, dimensionNamed("x",
		1<<62)))
	file := roundTrip(t, huge, nil)
	if dimension, err := file.Dimension("x"); err != nil ||
		dimension.Length() != 1<<62 {
		t.Errorf("a dimension of 1<<62: %v, %v", dimension, err)
	}
}

// defaultFills are the default fill value of each type, as a slice of
// one of the Go type the type is stored as.
var defaultFills = map[netcdf.Type]any{
	netcdf.TypeByte:   []int8{-127},
	netcdf.TypeChar:   []byte{0},
	netcdf.TypeShort:  []int16{-32767},
	netcdf.TypeInt:    []int32{-2147483647},
	netcdf.TypeFloat:  []float32{math.Float32frombits(0x7cf00000)},
	netcdf.TypeDouble: []float64{math.Float64frombits(0x479e000000000000)},
	netcdf.TypeUByte:  []byte{255},
	netcdf.TypeUShort: []uint16{65535},
	netcdf.TypeUInt:   []uint32{4294967295},
	netcdf.TypeInt64:  []int64{-9223372036854775806},
	netcdf.TypeUInt64: []uint64{18446744073709551614},
}

// repeated is a slice of a value n times.
func repeated(one any, n int) any {
	first := reflect.ValueOf(one)
	found := reflect.MakeSlice(first.Type(), 0, n)
	for range n {
		found = reflect.AppendSlice(found, first)
	}
	return found.Interface()
}

// joined is slices of one type end to end.
func joined(parts ...any) any {
	found := reflect.ValueOf(parts[0])
	found = reflect.AppendSlice(reflect.MakeSlice(found.Type(), 0, 0), found)
	for _, each := range parts[1:] {
		found = reflect.AppendSlice(found, reflect.ValueOf(each))
	}
	return found.Interface()
}

// typedNames are the five variables written of a type: fixed, fixed and
// never written, fixed with a _FillValue, record, record with one.
var typedNames = []string{"f", "fu", "fc", "r", "rc"}

// typesOf are the types a format holds.
func typesOf(format netcdf.Format) []netcdf.Type {
	if format == netcdf.FormatClassic64Data {
		return netcdf.AllTypes()
	}
	return netcdf.AllTypes()[:int(netcdf.TypeDouble)]
}

// typedLayout is a layout of five variables of each type of a format.
func typedLayout(t *testing.T, format netcdf.Format) *netcdf.Layout {
	t.Helper()
	layout, err := netcdf.NewLayout(format)
	check(t, err)
	across, err := layout.AddDimension("x", writeWidth)
	check(t, err)
	records, err := layout.AddDimension("t", 0)
	check(t, err)
	for _, typ := range typesOf(format) {
		for _, name := range typedNames {
			over := []*netcdf.LayoutDimension{across}
			if name[0] == 'r' {
				over = []*netcdf.LayoutDimension{records, across}
			}
			each, err := layout.AddVariable(netcdf.VariableSpec{
				Name: netcdf.TypeName(typ) + "_" + name, Type: typ,
				Dimensions: over})
			check(t, err)
			if strings.HasSuffix(name, "c") {
				first := reflect.ValueOf(valuesOf()[typ]).Slice(0, 1)
				check(t, each.SetTyped("_FillValue", typ, first.Interface()))
			}
		}
	}
	return layout
}

// writeTyped writes the f and r variables of every type: the record
// variables at record 0 and at record 3, which fills 1 and 2.
func writeTyped(t *testing.T, layout *netcdf.Layout, format netcdf.Format,
	path string) {
	t.Helper()
	writer, err := layout.Create(path)
	check(t, err)
	slab := netcdf.Slab{Start: []int64{3, 0}, Count: []int64{1, writeWidth}}
	for _, typ := range typesOf(format) {
		each := valuesOf()[typ]
		name := netcdf.TypeName(typ) + "_"
		one, err := writer.Variable(name + "f")
		check(t, err)
		check(t, one.Write(each))
		for _, record := range []string{"r", "rc"} {
			variable, err := writer.Variable(name + record)
			check(t, err)
			check(t, variable.Write(each))
			check(t, variable.WriteSlab(each, slab))
		}
	}
	check(t, writer.Close())
}

// expectedTyped is what each variable of a type reads back as.
func expectedTyped(typ netcdf.Type) map[string]any {
	written := valuesOf()[typ]
	fill := defaultFills[typ]
	own := reflect.ValueOf(written).Slice(0, 1).Interface()
	gap := 2 * writeWidth
	return map[string]any{"f": written, "fu": repeated(fill, writeWidth),
		"fc": repeated(own, writeWidth),
		"r":  joined(written, repeated(fill, gap), written),
		"rc": joined(written, repeated(own, gap), written)}
}

// Every type, in every format that has it, is written, closed, reopened
// and read back, and so are the fill values of what was never written
// and of a gap in the records, the default one and a _FillValue.
func TestEveryTypeWrittenAndReadBack(t *testing.T) {
	for _, format := range formatsOf {
		path := filepath.Join(t.TempDir(), "typed.nc")
		writeTyped(t, typedLayout(t, format), format, path)
		file := roundTrip(t, path, nil)
		var problems []string
		if file.NumRecs() != 4 {
			problems = append(problems, fmt.Sprintf("%d records, want 4",
				file.NumRecs()))
		}
		for _, typ := range typesOf(format) {
			for name, want := range expectedTyped(typ) {
				full := netcdf.TypeName(typ) + "_" + name
				variable, err := file.Variable(full)
				if err != nil {
					problems = append(problems, err.Error())
					continue
				}
				got, err := variable.Values()
				if err != nil || !reflect.DeepEqual(got, want) {
					problems = append(problems, fmt.Sprintf(
						"%s: read %v, %v; want %v", full, got, err, want))
				}
			}
		}
		settle(problems)
		for _, line := range problems {
			t.Errorf("%s: %s", netcdf.FormatName(format), line)
		}
	}
}

// gapFile writes a record variable's first record, then one a gap away,
// and gives the file's path and how long the gap took.
func gapFile(t *testing.T, records int64) (string, time.Duration) {
	t.Helper()
	layout, err := netcdf.NewLayout(netcdf.FormatClassic64)
	check(t, err)
	across, err := layout.AddDimension("x", gapWidth)
	check(t, err)
	every, err := layout.AddDimension("t", 0)
	check(t, err)
	_, err = layout.AddVariable(netcdf.VariableSpec{Name: "r",
		Type: netcdf.TypeInt, Dimensions: []*netcdf.LayoutDimension{every,
			across}})
	check(t, err)
	path := filepath.Join(t.TempDir(), "gap.nc")
	writer, err := layout.Create(path)
	check(t, err)
	variable, err := writer.Variable("r")
	check(t, err)
	check(t, variable.Write([]int32{1, 2, 3, 4}))
	start := time.Now()
	check(t, variable.WriteSlab([]int32{9, 9, 9, 9}, netcdf.Slab{
		Start: []int64{records - 1, 0}, Count: []int64{1, gapWidth}}))
	took := time.Since(start)
	check(t, writer.Close())
	return path, took
}

// A write past the end of the record dimension by a modest gap fills
// the gap and reads back, at a cost that is logged.
func TestAModestGapInTheRecords(t *testing.T) {
	for _, records := range []int64{gapSmall, gapLarge} {
		path, took := gapFile(t, records)
		t.Logf("GAP %d records filled in %v", records, took)
		file := roundTrip(t, path, nil)
		variable, err := file.Variable("r")
		check(t, err)
		var problems []string
		if file.NumRecs() != records {
			problems = append(problems, fmt.Sprintf("%d records, want %d",
				file.NumRecs(), records))
		}
		for record, want := range map[int64]int32{1: -2147483647,
			records - 1: 9} {
			got, err := variable.Slab(netcdf.Slab{Start: []int64{record, 0},
				Count: []int64{1, gapWidth}})
			if err != nil || !reflect.DeepEqual(got, repeated([]int32{want},
				gapWidth)) {
				problems = append(problems, fmt.Sprintf(
					"record %d: %v, %v", record, got, err))
			}
		}
		settle(problems)
		for _, line := range problems {
			t.Error(line)
		}
	}
}

// A writer that is not closed leaves a file that opens and reads, with
// the records the header says, which is none until Close.
func TestAWriterNotClosed(t *testing.T) {
	layout, err := netcdf.NewLayout(netcdf.FormatClassic64)
	check(t, err)
	across, err := layout.AddDimension("x", writeWidth)
	check(t, err)
	every, err := layout.AddDimension("t", 0)
	check(t, err)
	for name, over := range map[string][]*netcdf.LayoutDimension{
		"v": {across}, "r": {every, across}} {
		_, err = layout.AddVariable(netcdf.VariableSpec{Name: name,
			Type: netcdf.TypeInt, Dimensions: over})
		check(t, err)
	}
	path := filepath.Join(t.TempDir(), "open.nc")
	writer, err := layout.Create(path)
	check(t, err)
	t.Cleanup(func() { _ = writer.Close() })
	data := map[string][]int32{"v": {1, 2, 3}, "r": {1, 2, 3, 4, 5, 6, 7, 8, 9}}
	for name, values := range data {
		variable, err := writer.Variable(name)
		check(t, err)
		check(t, variable.Write(values))
	}
	file := roundTrip(t, path, nil)
	t.Logf("NOT CLOSED: the file opens with %d records", file.NumRecs())
	var problems []string
	if got := readEverything(file); got != nil {
		problems = append(problems, "read: "+got.Error())
	}
	if file.NumRecs() > 3 {
		problems = append(problems, "more records than were written")
	}
	settle(problems)
	for _, line := range problems {
		t.Error(line)
	}
}

// writeRefusal is a write that must be refused, as a kind, and must not
// change the file.
type writeRefusal struct {
	name   string
	format netcdf.Format
	do     func(*netcdf.Writer) error
	want   error
}

// recordWriter is the writer of a file with a record variable "r" of
// four integers a record, and a fixed variable "f" of three.
func recordWriter(t *testing.T, format netcdf.Format) *netcdf.Writer {
	t.Helper()
	layout, err := netcdf.NewLayout(format)
	check(t, err)
	across, err := layout.AddDimension("x", gapWidth)
	check(t, err)
	every, err := layout.AddDimension("t", 0)
	check(t, err)
	_, err = layout.AddVariable(netcdf.VariableSpec{Name: "r",
		Type: netcdf.TypeInt, Dimensions: []*netcdf.LayoutDimension{every,
			across}})
	check(t, err)
	_, err = layout.AddVariable(netcdf.VariableSpec{Name: "f",
		Type: netcdf.TypeInt, Dimensions: []*netcdf.LayoutDimension{across}})
	check(t, err)
	writer, err := layout.Create(filepath.Join(t.TempDir(), "w.nc"))
	check(t, err)
	t.Cleanup(func() { _ = writer.Close() })
	return writer
}

// slabWrite is a step that writes a slab of the record variable.
func slabWrite(start, count []int64, values any) func(*netcdf.Writer) error {
	return func(writer *netcdf.Writer) error {
		variable, _ := writer.Variable("r")
		return variable.WriteSlab(values, netcdf.Slab{Start: start,
			Count: count})
	}
}

// writeRefusals are the writes that must be refused. None starts a fill
// of more records than the format holds or the address space allows.
func writeRefusals() []writeRefusal {
	four := []int32{1, 2, 3, 4}
	return []writeRefusal{
		{"the wrong type", netcdf.FormatClassic64, slabWrite([]int64{0, 0},
			[]int64{1, 4}, []int8{1, 2, 3, 4}), netcdf.ErrTypeMismatch},
		{"the wrong length", netcdf.FormatClassic64, slabWrite([]int64{0, 0},
			[]int64{1, 4}, []int32{1}), netcdf.ErrOutOfRange},
		{"a negative start", netcdf.FormatClassic64, slabWrite(
			[]int64{-1, 0}, []int64{1, 4}, four), netcdf.ErrOutOfRange},
		{"past the inner dimension", netcdf.FormatClassic64, slabWrite(
			[]int64{0, 2}, []int64{1, 4}, four), netcdf.ErrOutOfRange},
		{"a record number that overflows", netcdf.FormatClassic64Data,
			slabWrite([]int64{math.MaxInt64, 0}, []int64{1, 4}, four),
			netcdf.ErrOutOfRange},
		{"a record count the size cannot address", netcdf.FormatClassic64Data,
			slabWrite([]int64{1 << 62, 0}, []int64{1, 4}, four),
			netcdf.ErrOutOfRange},
		{"a record count above the CDF-2 limit", netcdf.FormatClassic64,
			slabWrite([]int64{1<<32 - 1, 0}, []int64{1, 4}, four),
			netcdf.ErrOutOfRange},
		{"after Close", netcdf.FormatClassic64, func(w *netcdf.Writer) error {
			_ = w.Close()
			return slabWrite([]int64{0, 0}, []int64{1, 4}, four)(w)
		}, netcdf.ErrClosed},
	}
}

// Writes that must be refused are, as the kind they are, and leave the
// record count and the file's size as they were.
func TestWritesThatMustBeRefused(t *testing.T) {
	for _, each := range writeRefusals() {
		t.Run(each.name, func(t *testing.T) {
			writer := recordWriter(t, each.format)
			var err error
			got := attempt(func() { err = each.do(writer) })
			problems := got.problems(0, 0)
			if !errors.Is(err, each.want) {
				problems = append(problems, fmt.Sprintf(
					"want %v, got %v", each.want, err))
			}
			if writer.NumRecs() != 0 {
				problems = append(problems, fmt.Sprintf(
					"a refused write left %d records", writer.NumRecs()))
			}
			settle(problems)
			for _, line := range problems {
				t.Error(line)
			}
		})
	}
}
