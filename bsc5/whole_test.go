//go:build bsc5data

package bsc5_test

import (
	"os"
	"testing"

	"github.com/janearc/celestial-data/bsc5"
)

// the test: the whole catalogue, BSC5_FILE naming a copy of ybsc5.gz,
// reads as its readme says: 9,110 rows, numbered 1 to 9110 in order, all
// but the 14 with no place placed, and Sirius the brightest. It runs
// with go test -tags bsc5data.
func TestTheWholeCatalogueReads(test *testing.T) {
	file, err := os.Open(os.Getenv("BSC5_FILE"))
	if err != nil {
		test.Fatalf("BSC5_FILE: %v", err)
	}
	defer file.Close()
	stars, err := bsc5.Read(file)
	if err != nil {
		test.Fatal(err)
	}
	if len(stars) != 9110 {
		test.Fatalf("%d rows, the readme says 9110", len(stars))
	}
	unplaced, brightest := 0, stars[0]
	for i, star := range stars {
		if star.HR() != i+1 {
			test.Fatalf("row %d is HR %d", i+1, star.HR())
		}
		if star.Place() == nil {
			unplaced++
		}
		visual, has := star.Visual()
		least, _ := brightest.Visual()
		if has && visual < least {
			brightest = star
		}
	}
	if unplaced != 14 || brightest.HR() != 2491 {
		test.Errorf("%d rows unplaced; the brightest is HR %d",
			unplaced, brightest.HR())
	}
}
