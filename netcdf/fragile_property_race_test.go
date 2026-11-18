//go:build fragile

package netcdf_test

// The race property: sixteen goroutines read random slabs of one open
// file at once, and each slab is compared with the same elements of the
// values the model was written from. Run under -race.

import (
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"sync"
	"testing"

	"pgregory.net/rapid"

	"github.com/janearc/celestial-data/netcdf"
)

// The shape of the race.
const (
	// goroutineCount is how many goroutines read one file.
	goroutineCount = 16
	// slabsEach is how many slabs each reads.
	slabsEach = 25
)

// randomSlab is a start and count inside a shape, drawn from a source
// that one goroutine owns.
func randomSlab(shape []int64, source *rand.Rand) (start, count []int64) {
	for _, length := range shape {
		first := source.Int64N(length)
		start = append(start, first)
		count = append(count, 1+source.Int64N(length-first))
	}
	return start, count
}

// readOneSlab reads a random slab of a random variable that holds values,
// and what is wrong in it, or nothing.
func readOneSlab(file *netcdf.File, model fragModel, holds []int,
	source *rand.Rand) string {
	each := model.Vars[holds[source.IntN(len(holds))]]
	shape := model.shape(each)
	start, count := randomSlab(shape, source)
	variable, err := file.Variable(each.Name)
	if err != nil {
		return err.Error()
	}
	got, err := variable.Slab(netcdf.Slab{Start: start, Count: count})
	if err != nil {
		return fmt.Sprintf("%q start %v count %v: %v", each.Name, start,
			count, err)
	}
	want := extract(shape, start, count, signedBits(each.Values))
	if !slices.Equal(signedBits(got), want) {
		return fmt.Sprintf("%q start %v count %v read %v, want %v", each.Name,
			start, count, signedBits(got), want)
	}
	for _, kept := range file.Attributes() {
		_ = kept.Bytes()
	}
	return ""
}

// holders are the variables of a model that hold values, rank or none.
func holders(model fragModel) []int {
	var found []int
	for at, each := range model.Vars {
		if countOf(model.shape(each)) > 0 {
			found = append(found, at)
		}
	}
	return found
}

// Sixteen goroutines read random slabs of one open file, and every slab
// is the model's. A race, or a read that leaves state behind, shows here
// under -race or as a wrong slab.
func TestPropertyManyGoroutinesReadRandomSlabs(t *testing.T) {
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
		seeds := rapid.SliceOfN(rapid.Uint64(), goroutineCount,
			goroutineCount).Draw(t, "seeds")
		holds := holders(model)
		var wait sync.WaitGroup
		var lock sync.Mutex
		var failures []string
		for who := range goroutineCount {
			wait.Go(func() {
				source := rand.New(rand.NewPCG(seeds[who], uint64(who)))
				for range slabsEach {
					if line := readOneSlab(file, model, holds, source); line != "" {
						lock.Lock()
						failures = append(failures, line)
						lock.Unlock()
						return
					}
				}
			})
		}
		wait.Wait()
		_ = file.Close()
		if len(failures) > 0 {
			t.Fatalf("%d of %d goroutines read a wrong slab, first: %s\n%s",
				len(failures), goroutineCount, failures[0], model)
		}
	})
}

// A test that the race is built to be found: the harness's own check on
// itself, a slab the model has wrong must be reported.
func TestPropertyRaceCheckSeesAWrongSlab(t *testing.T) {
	model := fragModel{Format: netcdf.FormatClassic,
		Dims: []modelDim{{"x", 3}},
		Vars: []modelVar{{Name: "v", Type: netcdf.TypeInt, Dims: []int{0},
			Values: []int32{1, 2, 3}}}}
	path := newScratch(t).next()
	if err := model.save(path); err != nil {
		t.Fatal(err)
	}
	file, err := netcdf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	source := rand.New(rand.NewPCG(1, 2))
	if line := readOneSlab(file, model, []int{0}, source); line != "" {
		t.Fatalf("a right model reported wrong: %s", line)
	}
	model.Vars[0].Values = []int32{1, 2, 4}
	for range slabsEach {
		if readOneSlab(file, model, []int{0}, source) != "" {
			return
		}
	}
	t.Fatal("a wrong model was never reported")
}
