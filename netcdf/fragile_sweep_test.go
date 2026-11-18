//go:build fragile

package netcdf_test

// Sweeps over valid files: every prefix of the file, and every byte of
// the header set in turn to four values. A mutant must not panic, hang
// or allocate without bound. Whether it errs or reads is not judged;
// the mutants that open and read are counted, since each is a wrong file
// accepted.

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/netcdf"
)

// The variable naming where netcdf's own test fixtures are; unset, none are read.
const (
	fixtureDirectoryVariable = "FRAGILE_FIXTURES"
	// examplesShown is how many examples of a class a sweep logs.
	examplesShown = 8
)

// mutations are the values each byte of a header is set to.
var mutations = []byte{0x00, 0x7f, 0x80, 0xff}

// named is a file of a name.
type named struct {
	name string
	raw  []byte
}

// gridBytes is the bytes of a grid file written by the adapter.
func gridBytes(tb testing.TB, grid netcdf.GridFile) []byte {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "grid.nc")
	if err := netcdf.WriteGrid(path, grid); err != nil {
		tb.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	return raw
}

// writtenFiles are three small files written by the adapter, one in each
// format: packed shorts, a record variable, and two variables of doubles.
func writtenFiles(tb testing.TB) []named {
	tb.Helper()
	lats, lons := []float64{-60, 0, 60}, []float64{0, 90, 180, 270}
	times := hoursOf(3)
	packed := gridWith(lats, lons, fixtureUnits, times)
	record := gridWith(lats, lons, fixtureUnits, times)
	record.Format, record.Record = netcdf.FormatClassic64, true
	doubles := gridWith(lats, lons, fixtureUnits, times)
	doubles.Format = netcdf.FormatClassic64Data
	second := doubles.Variables[0]
	second.Name, second.Packing = "other", nil
	doubles.Variables[0].Packing = nil
	doubles.Variables = append(doubles.Variables, second)
	return []named{{"written CDF-1", gridBytes(tb, packed)},
		{"written CDF-2 record", gridBytes(tb, record)},
		{"written CDF-5", gridBytes(tb, doubles)}}
}

// fixtureFiles are netcdf's own test fixtures, when the variable names them.
func fixtureFiles(tb testing.TB) []named {
	tb.Helper()
	directory := os.Getenv(fixtureDirectoryVariable)
	if directory == "" {
		return nil
	}
	paths, _ := filepath.Glob(filepath.Join(directory, "*.nc"))
	var found []named
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			tb.Fatal(err)
		}
		found = append(found, named{"fixture " + filepath.Base(path), raw})
	}
	return found
}

// headerEnd is where the data of the file begins: the lowest offset a
// variable's data is read from. A file with no data is all header.
func headerEnd(raw []byte) int {
	there := fromBytes(raw)
	file, err := netcdf.Read(there.source(inputName))
	if err != nil {
		return len(raw)
	}
	defer file.Close()
	there.forgotten()
	for _, variable := range file.Variables() {
		_, _ = variable.Values()
	}
	lowest := int64(len(raw))
	for _, offset := range there.forgotten() {
		lowest = min(lowest, offset)
	}
	return int(lowest)
}

// sweep is the count of what a family of mutants did.
type sweep struct {
	name                           string
	original                       string
	mutants, opened, read          int
	inert, changed                 int
	inertExamples, changedExamples []string
	broken                         map[string]int
	brokenExamples                 map[string][]string
}

// newSweep is an empty sweep of a file, whose signature is its own.
func newSweep(name string, raw []byte) *sweep {
	found := &sweep{name: name, broken: map[string]int{},
		brokenExamples: map[string][]string{}}
	got := exercise(fromBytes(raw).source(inputName))
	found.original = got.signature
	return found
}

// try runs one mutant and counts it.
func (s *sweep) try(label string, raw []byte) {
	raw = slices.Clone(raw)
	var got trial
	gone := attempt(func() { got = exercise(fromBytes(raw).source(inputName)) })
	s.mutants++
	for _, each := range gone.problems(int64(len(raw)), 0) {
		class := strings.Fields(each)[0]
		s.broken[class]++
		if len(s.brokenExamples[class]) < examplesKept {
			s.brokenExamples[class] = append(s.brokenExamples[class],
				label+": "+each)
		}
	}
	if gone.timedOut {
		return
	}
	for _, each := range got.untyped {
		note(&ledger.untyped, s.name+" "+label+": "+each)
	}
	s.count(label, got)
}

// count adds what a mutant did to the sweep.
func (s *sweep) count(label string, got trial) {
	if !got.opened {
		return
	}
	s.opened++
	if !got.read {
		return
	}
	s.read++
	if got.signature == s.original {
		s.inert++
		s.inertExamples = append(s.inertExamples, label)
		return
	}
	s.changed++
	s.changedExamples = append(s.changedExamples, label+": "+
		firstDifference(s.original, got.signature))
}

// report logs the sweep and fails when a mutant broke the limits.
func (s *sweep) report(t *testing.T) {
	t.Helper()
	ledger.Lock()
	ledger.mutants += s.mutants
	for _, count := range s.broken {
		ledger.mutantsBroke += count
	}
	ledger.Unlock()
	t.Logf("SWEEP %s: %d mutants, %d opened, %d read whole, %d with the "+
		"file's own signature, %d with another", s.name, s.mutants, s.opened,
		s.read, s.inert, s.changed)
	for at, each := range s.changedExamples {
		if at < examplesShown {
			t.Logf("SWEEP changed %s: %s", s.name, each)
		}
	}
	for at, each := range s.inertExamples {
		if at < examplesShown {
			t.Logf("SWEEP inert %s: %s", s.name, each)
		}
	}
	for class, count := range s.broken {
		t.Errorf("%s: %d mutants broke (%s), for example %v", s.name, count,
			class, s.brokenExamples[class][:min(3, len(s.brokenExamples[class]))])
	}
}

// sweepInputs are the files the sweeps run over.
func sweepInputs(t *testing.T) []named {
	t.Helper()
	return append(writtenFiles(t), fixtureFiles(t)...)
}

// Every prefix of a valid file, from nothing up.
func TestSweepPrefixes(t *testing.T) {
	for _, input := range sweepInputs(t) {
		s := newSweep(input.name+", prefixes", input.raw)
		for length := range len(input.raw) {
			s.try(fmt.Sprintf("cut at %d", length), input.raw[:length])
		}
		s.try("whole", input.raw)
		s.report(t)
	}
}

// Every byte of the header set to 0x00, 0x7f, 0x80 and 0xff in turn.
func TestSweepHeaderBytes(t *testing.T) {
	for _, input := range sweepInputs(t) {
		s := newSweep(input.name+", header bytes", input.raw)
		buffer := slices.Clone(input.raw)
		for offset := range headerEnd(input.raw) {
			for _, value := range mutations {
				if input.raw[offset] == value {
					continue
				}
				buffer[offset] = value
				s.try(fmt.Sprintf("byte %d set to %#02x", offset, value),
					buffer)
			}
			buffer[offset] = input.raw[offset]
		}
		s.report(t)
	}
}
