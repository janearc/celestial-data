//go:build fragile

package netcdf_test

// A fuzz target over the bytes given to the adapter's reader, seeded from
// the written files, netcdf's own fixtures and the hostile cases. Run it with
//
//	go test -tags fragile -run '^$' -fuzz FuzzRead -fuzztime 60s ./netcdf
//
// and what it finds is kept under netcdf/testdata/fuzz.

import (
	"strings"
	"testing"
)

// seedLimit is the longest seed taken from a hostile case, in bytes.
const seedLimit = 16 << 10

// fuzzSeeds are the inputs the fuzzer starts from.
func fuzzSeeds(tb testing.TB) [][]byte {
	tb.Helper()
	var seeds [][]byte
	for _, each := range writtenFiles(tb) {
		seeds = append(seeds, each.raw)
	}
	for _, each := range fixtureFiles(tb) {
		seeds = append(seeds, each.raw)
	}
	for _, each := range hostileCases() {
		raw := each.raw
		if raw == nil {
			raw = each.head.Bytes()
		}
		if len(raw) <= seedLimit {
			seeds = append(seeds, raw)
		}
	}
	return seeds
}

// Bytes of any kind given to the reader must not panic, hang or allocate
// without bound.
func FuzzRead(f *testing.F) {
	for _, seed := range fuzzSeeds(f) {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		gone := attempt(func() { exercise(fromBytes(raw).source(inputName)) })
		if problems := gone.problems(int64(len(raw)), 0); len(problems) > 0 {
			t.Fatal(strings.Join(problems, "; "))
		}
	})
}
