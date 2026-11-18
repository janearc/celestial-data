//go:build fragile

package netcdf_test

// A reader that ends a full read with io.EOF, data beyond 4 GiB and
// beyond 2^40 bytes, and many readers of one open file.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/janearc/celestial-data/netcdf"
)

// What the far file holds, by hand.
const (
	// farRecords is the records of the far file's record variable.
	farRecords = 4
	// farWidth is the values in a record and in the fixed variable.
	farWidth = 3
	// farGap is the bytes between the fixed variable and the records.
	farGap = 4096
	// farFirst is the first integer of the record variable.
	farFirst = 1000
	// intWidth and doubleWidth are the bytes of the two types.
	intWidth, doubleWidth = 4, 8
)

// farDoubles are the values of the far file's fixed variable.
var farDoubles = []float64{1.5, 2.5, 42.5}

// farParts is a file of one fixed variable "a" of three doubles that
// begins at scale, and, past CDF-1, a record variable "r" of four
// records of three integers that begins farGap after it. Only the bytes
// of the header and the values are there. It gives where the records
// begin.
func farParts(version int, scale uint64) (*notThere, uint64) {
	head := header{Version: version,
		Dims: []dimSpec{{Name: "x", Length: farWidth}, {Name: "t"}},
		Vars: []varSpec{{Name: "a", DimIDs: []uint64{0}, Type: wireDouble,
			Begin: some(scale)}}}
	recordBegin := scale + farGap
	size := int64(scale) + farWidth*doubleWidth
	var recordBytes []byte
	if version != cdfOne {
		head.Records = farRecords
		head.Vars = append(head.Vars, varSpec{Name: "r",
			DimIDs: []uint64{1, 0}, Type: wireInt, Begin: some(recordBegin)})
		for at := range farRecords * farWidth {
			recordBytes = binary.BigEndian.AppendUint32(recordBytes,
				uint32(farFirst+at))
		}
		size = int64(recordBegin) + int64(len(recordBytes))
	}
	var doubles []byte
	for _, each := range farDoubles {
		doubles = binary.BigEndian.AppendUint64(doubles, math.Float64bits(each))
	}
	return &notThere{size: size, parts: []part{{data: head.Bytes()},
		{at: int64(scale), data: doubles},
		{at: int64(recordBegin), data: recordBytes}}}, recordBegin
}

// farScales are where the data begins, by version.
func farScales(version int) []uint64 {
	if version == cdfOne {
		return []uint64{0x7FFFFFF8}
	}
	return []uint64{1 << 32, 5 << 30, 1 << 40, 1 << 41}
}

// askedLast is the last offset a reader was asked for, since it was
// last forgotten.
func askedLast(there *notThere) int64 {
	asked := there.forgotten()
	if len(asked) == 0 {
		return -1
	}
	return asked[len(asked)-1]
}

// checkFar opens a far file, reads the last value of each variable, and
// says what is wrong with the offsets asked and the values found.
func checkFar(version int, scale uint64) []string {
	there, recordBegin := farParts(version, scale)
	file, err := netcdf.Read(there.source("far"))
	if err != nil {
		return []string{"did not open: " + err.Error()}
	}
	defer file.Close()
	variable, _ := file.Variable("a")
	there.forgotten()
	got, err := variable.Slab(netcdf.Slab{Start: []int64{2},
		Count: []int64{1}})
	problems := farProblems("a[2]", got, err, []float64{42.5},
		askedLast(there), int64(scale)+2*doubleWidth)
	if version == cdfOne {
		return problems
	}
	record, _ := file.Variable("r")
	got, err = record.Slab(netcdf.Slab{Start: []int64{farRecords - 1, 2},
		Count: []int64{1, 1}})
	last := int32(farFirst + farRecords*farWidth - 1)
	problems = append(problems, farProblems("r[3][2]", got, err,
		[]int32{last}, askedLast(there), int64(recordBegin)+
			(farRecords-1)*farWidth*intWidth+2*intWidth)...)
	whole, err := record.Values()
	if err != nil || len(whole.([]int32)) != farRecords*farWidth {
		problems = append(problems, fmt.Sprintf("r whole: %v, %v", whole, err))
	}
	return problems
}

// farProblems compares what a read gave with what it should, and the
// offset it asked for with the one computed by hand.
func farProblems(what string, got any, err error, want any, asked,
	offset int64) []string {
	var problems []string
	if err != nil {
		return []string{what + ": " + err.Error()}
	}
	if !reflect.DeepEqual(got, want) {
		problems = append(problems, fmt.Sprintf("%s: read %v, want %v", what,
			got, want))
	}
	if asked != offset {
		problems = append(problems, fmt.Sprintf("%s: asked for offset %d, "+
			"computed by hand %d", what, asked, offset))
	}
	return problems
}

// A value of the last element of the last variable, beyond 2 GiB, 4 GiB
// and 2^40 bytes, from the offset computed by hand.
func TestDataBeyondTheBigOffsets(t *testing.T) {
	for _, version := range versions {
		for _, scale := range farScales(version) {
			t.Run(fmt.Sprintf("CDF-%d at %d", version, scale),
				func(t *testing.T) {
					var problems []string
					got := attempt(func() {
						problems = checkFar(version, scale)
					})
					problems = append(problems, got.problems(1<<10, 0)...)
					settle(problems)
					for _, each := range problems {
						t.Error(each)
					}
				})
		}
	}
}

// ramp is count steps of rows by columns, no value the same as another
// and none not a number.
func ramp(count, rows, columns int) [][]float64 {
	steps := make([][]float64, count)
	for step := range steps {
		steps[step] = make([]float64, rows*columns)
		for at := range steps[step] {
			steps[step][at] = float64(step*1_000_000 + at)
		}
	}
	return steps
}

// bigGrid is a grid file of doubles that is larger than the 32 KiB the
// header reader takes at once, in the format given.
func bigGrid(format netcdf.Format, rows, columns, count int) netcdf.GridFile {
	lats, lons := make([]float64, rows), make([]float64, columns)
	for at := range lats {
		lats[at] = float64(at) - float64(rows)/2
	}
	for at := range lons {
		lons[at] = float64(at)
	}
	grid := netcdf.GridFile{Format: format, Latitudes: lats, Longitudes: lons,
		TimeUnits: fixtureUnits, Times: hoursOf(count)}
	for _, name := range []string{"first", "last"} {
		grid.Variables = append(grid.Variables, netcdf.GridVariable{
			Name: name, Steps: ramp(count, rows, columns)})
	}
	return grid
}

// readsLikeTheFile compares every variable read through a reader that
// ends full reads with io.EOF with the same read through a plain one.
func readsLikeTheFile(raw []byte) error {
	plain, err := netcdf.Read(fromBytes(raw).source("plain"))
	if err != nil {
		return fmt.Errorf("harness: the plain reader: %w", err)
	}
	defer plain.Close()
	size := int64(len(raw))
	odd, err := netcdf.Read(netcdf.Source{Name: "eof",
		At: eofAtEnd{inner: bytes.NewReader(raw), size: size}, Size: size})
	if err != nil {
		return err
	}
	defer odd.Close()
	for _, variable := range plain.Variables() {
		want, _ := variable.Values()
		twin, _ := odd.Variable(variable.Name())
		got, err := twin.Values()
		if err != nil {
			return fmt.Errorf("variable %q: %w", variable.Name(), err)
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("variable %q reads differently", variable.Name())
		}
	}
	return nil
}

// A reader that returns io.EOF with a full read at the end of the
// input, as io.ReaderAt allows: a small file must open, and the last
// variable of a larger one must read.
func TestAReaderThatSaysEOFAtTheEnd(t *testing.T) {
	inputs := writtenFiles(t)
	for _, format := range []netcdf.Format{netcdf.FormatClassic,
		netcdf.FormatClassic64, netcdf.FormatClassic64Data} {
		inputs = append(inputs, named{"larger " + netcdf.FormatName(format),
			gridBytes(t, bigGrid(format, 90, 120, 2))})
	}
	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			var err error
			got := attempt(func() { err = readsLikeTheFile(input.raw) })
			problems := got.problems(int64(len(input.raw)), 0)
			if err != nil {
				problems = append(problems, err.Error())
			}
			settle(problems)
			for _, each := range problems {
				t.Error(each)
			}
		})
	}
}

// readers is how many goroutines read one file, and rounds how many times
// each reads.
const (
	readers = 16
	rounds  = 50
	gridRow = 8
	gridCol = 8
)

// readRound is what one goroutine of the many does: its own step of the
// variable by slab, widened, and through the field.
func readRound(file *netcdf.File, who int) error {
	variable, err := file.Variable("tcc")
	if err != nil {
		return err
	}
	slab := netcdf.Slab{Start: []int64{int64(who), 0, 0},
		Count: []int64{1, gridRow, gridCol}}
	got, err := variable.Slab(slab)
	if err != nil {
		return err
	}
	want := ramp(readers, gridRow, gridCol)[who]
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("step %d read differently", who)
	}
	part, err := variable.NumbersSlab(netcdf.Slab{
		Start: []int64{int64(who), 2, 3}, Count: []int64{1, 4, 2}})
	if err != nil || part[0] != want[2*gridCol+3] {
		return fmt.Errorf("step %d part: %v, %v", who, part, err)
	}
	found, err := file.Field("tcc")
	if err != nil {
		return err
	}
	step, err := found.Step(who)
	if err != nil {
		return err
	}
	if value, have := step.At(1, 1); !have || value != want[gridCol+1] {
		return fmt.Errorf("step %d at (1,1): %v, %t", who, value, have)
	}
	return nil
}

// Sixteen goroutines read different slabs of one open file, under -race.
func TestManyReadersOfOneFile(t *testing.T) {
	for _, format := range []netcdf.Format{netcdf.FormatClassic64,
		netcdf.FormatClassic64Data} {
		grid := bigGrid(format, gridRow, gridCol, readers)
		grid.Record = format == netcdf.FormatClassic64
		grid.Variables = grid.Variables[:1]
		grid.Variables[0].Name = "tcc"
		path := filepath.Join(t.TempDir(), "many.nc")
		check(t, netcdf.WriteGrid(path, grid))
		file, err := netcdf.Open(path)
		check(t, err)
		var wait sync.WaitGroup
		var lock sync.Mutex
		var failures []string
		for who := range readers {
			wait.Go(func() {
				for range rounds {
					if err := readRound(file, who); err != nil {
						lock.Lock()
						failures = append(failures, err.Error())
						lock.Unlock()
						return
					}
				}
			})
		}
		wait.Wait()
		check(t, file.Close())
		failures = slices.Compact(failures)
		settle(failures)
		if len(failures) > 0 {
			t.Errorf("%s: %s", netcdf.FormatName(format),
				strings.Join(failures, "; "))
		}
	}
}
