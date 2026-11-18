//go:build fragile

package netcdf_test

// Oracles: files whose bytes are placed by hand, from the encoder, and
// read back every way: every hyperslab of each variable against an
// extraction from a flat array, the record layout, attributes that cross
// the reader's 32 KiB window, and a file that changes under an open
// reader.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/netcdf"
)

// The shape of the oracle file.
const (
	oracleRecords = 3 // records of the oracle file
	oracleSeven   = 7 // the stride of the fixed variable's values
	oracleThou    = 1000
	mismatchKept  = 10 // mismatches reported of one file
)

// asInts is the values of a read, as integers.
func asInts(values any) []int64 {
	var found []int64
	switch each := values.(type) {
	case []int32:
		for _, one := range each {
			found = append(found, int64(one))
		}
	case []int16:
		for _, one := range each {
			found = append(found, int64(one))
		}
	case []int8:
		for _, one := range each {
			found = append(found, int64(one))
		}
	}
	return found
}

// extract is the values of a hyperslab of a row-major array of a shape.
func extract(shape, start, count, flat []int64) []int64 {
	total := int64(1)
	for _, each := range count {
		total *= each
	}
	found := make([]int64, 0, total)
	if total == 0 {
		return found
	}
	index := make([]int64, len(shape))
	for {
		offset, stride := int64(0), int64(1)
		for at := len(shape) - 1; at >= 0; at-- {
			offset += (start[at] + index[at]) * stride
			stride *= shape[at]
		}
		found = append(found, flat[offset])
		at := len(shape) - 1
		for ; at >= 0; at-- {
			if index[at]++; index[at] < count[at] {
				break
			}
			index[at] = 0
		}
		if at < 0 {
			return found
		}
	}
}

// eachSlab calls visit with every start and count inside a shape, the
// empty ones included.
func eachSlab(shape []int64, visit func(start, count []int64)) {
	start, count := make([]int64, len(shape)), make([]int64, len(shape))
	var walk func(at int)
	walk = func(at int) {
		if at == len(shape) {
			visit(slices.Clone(start), slices.Clone(count))
			return
		}
		for from := int64(0); from <= shape[at]; from++ {
			for length := int64(0); from+length <= shape[at]; length++ {
				start[at], count[at] = from, length
				walk(at + 1)
			}
		}
	}
	walk(0)
}

// oracleHeader is a header with a fixed variable A of three dimensions,
// and two record variables, R of two and S of none besides the records.
func oracleHeader(version int) header {
	return header{Version: version, Records: oracleRecords,
		Dims: []dimSpec{{Name: "a", Length: 2}, {Name: "b", Length: 3},
			{Name: "c", Length: 4}, {Name: "e", Length: 2}, {Name: "t"}},
		Vars: []varSpec{
			{Name: "A", DimIDs: []uint64{0, 1, 2}, Type: wireInt},
			{Name: "R", DimIDs: []uint64{4, 1, 3}, Type: wireInt},
			{Name: "S", DimIDs: []uint64{4}, Type: wireShort}}}
}

// oracleFile is the oracle header's file with its values put where the
// header places them, and the flat arrays they are.
func oracleFile(version int) ([]byte, map[string][]int64) {
	head := oracleHeader(version)
	raw := head.File()
	placed := head.place(uint64(len(head.Bytes())))
	flat := map[string][]int64{}
	for at := range 24 {
		flat["A"] = append(flat["A"], int64(at*oracleSeven+1))
		binary.BigEndian.PutUint32(raw[placed.begins[0]+uint64(4*at):],
			uint32(flat["A"][at]))
	}
	for record := range oracleRecords {
		start := placed.begins[1] + uint64(record)*placed.recordSize
		for at := range 6 {
			value := int64(record*oracleThou + at)
			flat["R"] = append(flat["R"], value)
			binary.BigEndian.PutUint32(raw[start+uint64(4*at):], uint32(value))
		}
		flat["S"] = append(flat["S"], int64(record+1))
		binary.BigEndian.PutUint16(raw[placed.begins[2]+
			uint64(record)*placed.recordSize:], uint16(record+1))
	}
	return raw, flat
}

// slabProblems reads every hyperslab of one variable and says where it
// differs from the oracle.
func slabProblems(variable *netcdf.Variable, flat []int64) []string {
	var problems []string
	shape := variable.Shape()
	eachSlab(shape, func(start, count []int64) {
		got, err := variable.Slab(netcdf.Slab{Start: start, Count: count})
		want := extract(shape, start, count, flat)
		if err != nil || !slices.Equal(asInts(got), want) {
			if len(problems) < mismatchKept {
				problems = append(problems, fmt.Sprintf(
					"%s start %v count %v: %v, %v; want %v",
					variable.Name(), start, count, got, err, want))
			}
		}
	})
	return problems
}

// Every hyperslab of every variable of a file placed by hand, in each
// version, is what an extraction from the flat array says.
func TestEverySlabAgainstAnOracle(t *testing.T) {
	for _, version := range versions {
		raw, flat := oracleFile(version)
		file, err := netcdf.Read(fromBytes(raw).source("oracle"))
		check(t, err)
		var problems []string
		for _, name := range []string{"A", "R", "S"} {
			variable, err := file.Variable(name)
			check(t, err)
			problems = append(problems, slabProblems(variable, flat[name])...)
		}
		settle(problems)
		for _, each := range problems {
			t.Errorf("CDF-%d: %s", version, each)
		}
	}
}

// recordsByHand is a header of record variables of several shapes, with
// their values placed by hand: byte, short and double records, which
// the specification pads to four bytes, and one alone, which it does
// not.
func recordsByHand(version int, alone bool) ([]byte, map[string][]int64) {
	head := emptyHeader(version)
	head.Records = oracleRecords
	head.Dims = []dimSpec{{Name: "t"}, {Name: "c", Length: 3}}
	head.Vars = []varSpec{{Name: "T", DimIDs: []uint64{0, 1}, Type: wireByte}}
	if !alone {
		head.Vars = append(head.Vars, varSpec{Name: "U", DimIDs: []uint64{0},
			Type: wireByte}, varSpec{Name: "V", DimIDs: []uint64{0, 1},
			Type: wireShort})
	}
	raw := head.File()
	placed := head.place(uint64(len(head.Bytes())))
	flat := map[string][]int64{}
	for record := range oracleRecords {
		for at := range 3 {
			value := int64(record*10 + at)
			flat["T"] = append(flat["T"], value)
			raw[placed.begins[0]+uint64(record)*placed.recordSize+
				uint64(at)] = byte(value)
		}
		if alone {
			continue
		}
		flat["U"] = append(flat["U"], int64(record+50))
		raw[placed.begins[1]+uint64(record)*placed.recordSize] = byte(record + 50)
		for at := range 3 {
			value := int64(record*100 + at)
			flat["V"] = append(flat["V"], value)
			binary.BigEndian.PutUint16(raw[placed.begins[2]+
				uint64(record)*placed.recordSize+uint64(2*at):], uint16(value))
		}
	}
	return raw, flat
}

// Record variables of byte and short type read back from where the
// specification puts them: padded to four bytes among several, packed
// when alone.
func TestRecordLayoutByHand(t *testing.T) {
	for _, version := range versions {
		for _, alone := range []bool{false, true} {
			raw, flat := recordsByHand(version, alone)
			file, err := netcdf.Read(fromBytes(raw).source("records"))
			check(t, err)
			var problems []string
			for name, want := range flat {
				variable, err := file.Variable(name)
				check(t, err)
				got, err := variable.Values()
				if err != nil || !slices.Equal(asInts(got), want) {
					problems = append(problems, fmt.Sprintf(
						"%s: %v, %v; want %v", name, got, err, want))
				}
			}
			settle(problems)
			for _, each := range problems {
				t.Errorf("CDF-%d alone %t: %s", version, alone, each)
			}
		}
	}
}

// chunkLengths are lengths of attribute values on each side of the
// 32 KiB window the header reader takes, and past two of them.
var chunkLengths = []int{32765, 32766, 32767, 32768, 32769, 65535, 65536,
	65537, 100_000}

// pattern is n bytes that repeat every 251, so no window is like another.
func pattern(n int) []byte {
	found := make([]byte, n)
	for at := range found {
		found[at] = byte(at % 251)
	}
	return found
}

// Attributes and a name that cross the window of the header reader read
// back as written.
func TestAttributesAcrossTheChunkBoundary(t *testing.T) {
	for _, version := range versions {
		head := goodHeader(version)
		head.Vars[0].Name = strings.Repeat("v", 40000)
		head.Attrs = nil
		for _, length := range chunkLengths {
			head.Attrs = append(head.Attrs, attrSpec{Type: wireChar,
				Name: fmt.Sprintf("a%d", length), Values: pattern(length)})
		}
		file, err := netcdf.Read(fromBytes(head.File()).source("chunks"))
		check(t, err)
		var problems []string
		for _, length := range chunkLengths {
			found, lookup := file.Attribute(fmt.Sprintf("a%d", length))
			if lookup != nil || !slices.Equal(found.Bytes(), pattern(length)) {
				problems = append(problems, fmt.Sprintf(
					"attribute of %d bytes: %v", length, lookup))
			}
		}
		if _, err := file.Variable(head.Vars[0].Name); err != nil {
			problems = append(problems, "the long variable name: "+err.Error())
		}
		settle(problems)
		for _, each := range problems {
			t.Errorf("CDF-%d: %s", version, each)
		}
	}
}

// A file that is cut short under an open reader reads as an error, a
// closed one as ErrClosed, and what is not a file is refused.
func TestAnOpenFileThatChanges(t *testing.T) {
	path := written(t, gridFile(fixtureLatitudes, fixtureLongitudes))
	file, err := netcdf.Open(path)
	check(t, err)
	variable, err := file.Variable(tccName)
	check(t, err)
	info, err := os.Stat(path)
	check(t, err)
	check(t, os.Truncate(path, info.Size()/2))
	var problems []string
	var cut, closed error
	got := attempt(func() {
		_, cut = variable.Values()
		_ = file.Close()
		_, closed = variable.Values()
	})
	problems = append(problems, got.problems(info.Size(), 0)...)
	if cut == nil || !isOneOf(cut, readKinds) {
		problems = append(problems, fmt.Sprintf(
			"a variable cut away read as: %v", cut))
	}
	if !errors.Is(closed, netcdf.ErrClosed) {
		problems = append(problems, fmt.Sprintf("after Close: %v", closed))
	}
	if err := file.Close(); err != nil {
		problems = append(problems, "a second Close: "+err.Error())
	}
	for _, name := range []string{t.TempDir(), missingPath(t),
		emptyFile(t)} {
		if _, err := netcdf.Open(name); err == nil {
			problems = append(problems, name+" opened")
		}
	}
	settle(problems)
	for _, each := range problems {
		t.Error(each)
	}
}

// missingPath is a path in a directory that is not there.
func missingPath(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/missing/none.nc"
}

// emptyFile is the path of a file of no bytes.
func emptyFile(t *testing.T) string {
	t.Helper()
	return rawFile(t, nil)
}
